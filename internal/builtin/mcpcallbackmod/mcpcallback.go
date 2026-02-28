// Package mcpcallbackmod provides the osm:mcpcallback native module for creating
// disposable MCP IPC channels. It wraps an MCP server (from osm:mcp) with a local
// socket transport (UDS on Unix, loopback TCP on Windows), enabling JS scripts to
// host MCP tools accessible by sub-processes via socket connection.
package mcpcallbackmod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"syscall"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Require returns a module loader for the osm:mcpcallback module.
// The adapter and loop are used for thread-safe JS callback invocation.
func Require(adapter *gojaeventloop.Adapter, loop *goeventloop.Loop) require.ModuleLoader {
	return func(rt *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		_ = exports.Set("MCPCallback", jsCallbackFactory(rt, adapter, loop))
	}
}

// jsCallbackFactory returns the JS factory/constructor: MCPCallback({server}) → callback object
//
// Usage from JS:
//
//	const { MCPCallback } = require('osm:mcpcallback');
//	const cb = MCPCallback({ server: srv });
//	await cb.init();
//	// cb.address, cb.scriptPath, cb.transport, cb.mcpConfigPath available
//	await cb.close();
func jsCallbackFactory(rt *goja.Runtime, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		opts := call.Argument(0)
		if opts == nil || goja.IsUndefined(opts) || goja.IsNull(opts) {
			panic(rt.NewGoError(errors.New("MCPCallback requires an options object: MCPCallback({server: srv})")))
		}
		optsObj := opts.ToObject(rt)

		// Extract the JS server object
		serverVal := optsObj.Get("server")
		if serverVal == nil || goja.IsUndefined(serverVal) || goja.IsNull(serverVal) {
			panic(rt.NewGoError(errors.New("MCPCallback options.server is required")))
		}
		serverObj := serverVal.ToObject(rt)

		// Extract Go *mcp.Server from the __goServer hidden property set by osm:mcp
		goServerVal := serverObj.Get("__goServer")
		if goServerVal == nil || goja.IsUndefined(goServerVal) {
			panic(rt.NewGoError(errors.New("server must be created with require('osm:mcp').createServer()")))
		}
		goServer, ok := goServerVal.Export().(*mcp.Server)
		if !ok {
			panic(rt.NewGoError(fmt.Errorf("invalid server object: expected *mcp.Server, got %T", goServerVal.Export())))
		}

		// Check if server is already running — caller must NOT call server.run() first
		isRunningVal := serverObj.Get("__isRunning")
		if isRunningVal != nil && !goja.IsUndefined(isRunningVal) {
			if isRunningFn, fnOK := goja.AssertFunction(isRunningVal); fnOK {
				result, err := isRunningFn(goja.Undefined())
				if err == nil && result.ToBoolean() {
					panic(rt.NewGoError(errors.New("server is already running — do not call server.run() before passing to MCPCallback")))
				}
			}
		}

		// Extract optional test context (Go context.Context wrapped in goja)
		var parentCtx context.Context
		if testCtxVal := optsObj.Get("__testContext"); testCtxVal != nil && !goja.IsUndefined(testCtxVal) && !goja.IsNull(testCtxVal) {
			if ctx, ctxOK := testCtxVal.Export().(context.Context); ctxOK {
				parentCtx = ctx
			}
		}

		cb := &mcpCallback{
			server:    goServer,
			adapter:   adapter,
			loop:      loop,
			runtime:   rt,
			parentCtx: parentCtx,
		}

		// Build JS object with methods and read-only property getters
		obj := rt.NewObject()
		_ = obj.Set("init", cb.jsInit())
		_ = obj.Set("close", cb.jsClose())
		_ = obj.Set("addTool", cb.jsAddTool())
		_ = obj.Set("initSync", cb.jsInitSync())
		_ = obj.Set("waitFor", cb.jsWaitFor())
		_ = obj.Set("closeSync", cb.jsCloseSync())
		_ = obj.Set("resetWaiter", cb.jsResetWaiter())

		_ = obj.DefineAccessorProperty("scriptPath",
			rt.ToValue(func(call goja.FunctionCall) goja.Value {
				cb.mu.Lock()
				defer cb.mu.Unlock()
				return rt.ToValue(cb.scriptPath)
			}),
			nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = obj.DefineAccessorProperty("address",
			rt.ToValue(func(call goja.FunctionCall) goja.Value {
				cb.mu.Lock()
				defer cb.mu.Unlock()
				return rt.ToValue(cb.address)
			}),
			nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = obj.DefineAccessorProperty("transport",
			rt.ToValue(func(call goja.FunctionCall) goja.Value {
				cb.mu.Lock()
				defer cb.mu.Unlock()
				return rt.ToValue(cb.transportType)
			}),
			nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = obj.DefineAccessorProperty("mcpConfigPath",
			rt.ToValue(func(call goja.FunctionCall) goja.Value {
				cb.mu.Lock()
				defer cb.mu.Unlock()
				return rt.ToValue(cb.configPath)
			}),
			nil, goja.FLAG_FALSE, goja.FLAG_TRUE)

		return obj
	}
}

// toolWaiter holds a channel for receiving Go-native tool call data.
// Used by addTool/waitFor for synchronous IPC that bypasses the JS event loop.
type toolWaiter struct {
	ch chan json.RawMessage // buffered(1): holds latest tool call arguments
}

// mcpCallback holds the state for a disposable MCP IPC channel.
type mcpCallback struct {
	server    *mcp.Server
	adapter   *gojaeventloop.Adapter
	loop      *goeventloop.Loop
	runtime   *goja.Runtime
	parentCtx context.Context // optional parent context; nil = context.Background()

	mu            sync.Mutex
	initialized   bool
	closed        bool
	listener      net.Listener
	tempDir       string
	scriptPath    string
	configPath    string
	address       string
	transportType string             // "unix" or "tcp"
	stop          context.CancelFunc // from signal.NotifyContext — cancels context AND deregisters signal handler
	ctx           context.Context    // context for the accept loop — cancelled on close

	toolMu      sync.RWMutex
	toolWaiters map[string]*toolWaiter
}

// jsInit returns the JS method: init() → Promise<void>
//
// Starts the transport listener, generates bootstrap files, and begins
// accepting MCP connections. The promise resolves when the listener is ready.
func (cb *mcpCallback) jsInit() func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		cb.mu.Lock()
		if cb.initialized {
			cb.mu.Unlock()
			panic(cb.runtime.NewGoError(errors.New("MCPCallback already initialized")))
		}
		if cb.closed {
			cb.mu.Unlock()
			panic(cb.runtime.NewGoError(errors.New("MCPCallback already closed")))
		}
		cb.initialized = true
		cb.mu.Unlock()

		promise, resolve, reject := cb.adapter.JS().NewChainedPromise()

		go func() {
			if err := cb.startListener(); err != nil {
				cb.cleanup()
				cb.mu.Lock()
				cb.initialized = false
				cb.mu.Unlock()
				if submitErr := cb.loop.Submit(func() { reject(err) }); submitErr != nil {
					// Event loop gone — nothing to do
				}
				return
			}

			if err := cb.generateFiles(); err != nil {
				cb.cleanup()
				cb.mu.Lock()
				cb.initialized = false
				cb.mu.Unlock()
				if submitErr := cb.loop.Submit(func() { reject(err) }); submitErr != nil {
					// Event loop gone
				}
				return
			}

			// Start accept loop for MCP connections.
			// Use signal.NotifyContext for automatic cleanup on SIGINT/SIGTERM.
			parent := cb.parentCtx
			if parent == nil {
				parent = context.Background()
			}
			ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
			cb.mu.Lock()
			cb.stop = stop
			cb.ctx = ctx
			listener := cb.listener
			cb.mu.Unlock()

			go cb.acceptLoop(ctx, listener)

			// Watch for context cancellation (signal or parent cancel) and auto-cleanup.
			go func() {
				<-ctx.Done()
				cb.mu.Lock()
				alreadyClosed := cb.closed
				cb.closed = true
				cb.mu.Unlock()
				if !alreadyClosed {
					cb.cleanup()
				}
			}()

			// Resolve promise on event loop thread
			if submitErr := cb.loop.Submit(func() {
				resolve(goja.Undefined())
			}); submitErr != nil {
				// Event loop gone
			}
		}()

		return cb.adapter.GojaWrapPromise(promise)
	}
}

// startListener creates the temp directory and starts the platform-appropriate listener.
func (cb *mcpCallback) startListener() error {
	// Create temp directory — use /tmp on macOS to keep UDS path under 104-char limit
	var tempDir string
	var err error
	if goruntime.GOOS == "darwin" {
		tempDir, err = os.MkdirTemp("/tmp", "osm-mcpcb-")
	} else {
		tempDir, err = os.MkdirTemp("", "osm-mcpcb-")
	}
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	// Set directory permissions to 0700 (owner-only access)
	if err := os.Chmod(tempDir, 0700); err != nil {
		_ = os.RemoveAll(tempDir)
		return fmt.Errorf("chmod temp dir: %w", err)
	}

	cb.mu.Lock()
	cb.tempDir = tempDir
	cb.mu.Unlock()

	// Start platform-appropriate listener
	if goruntime.GOOS == "windows" {
		return cb.startTCPListener()
	}
	return cb.startUDSListener()
}

// startUDSListener creates a Unix Domain Socket listener.
func (cb *mcpCallback) startUDSListener() error {
	cb.mu.Lock()
	tempDir := cb.tempDir
	cb.mu.Unlock()

	sockPath := filepath.Join(tempDir, "osm.sock")

	// Remove stale socket file if present (defensive)
	_ = os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", sockPath, err)
	}

	// Restrict socket permissions to owner only
	if chErr := os.Chmod(sockPath, 0600); chErr != nil {
		_ = ln.Close()
		return fmt.Errorf("chmod socket: %w", chErr)
	}

	cb.mu.Lock()
	cb.listener = ln
	cb.address = sockPath
	cb.transportType = "unix"
	cb.mu.Unlock()

	return nil
}

// startTCPListener creates a loopback TCP listener on a random OS-assigned port.
func (cb *mcpCallback) startTCPListener() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen tcp: %w", err)
	}

	cb.mu.Lock()
	cb.listener = ln
	cb.address = ln.Addr().String()
	cb.transportType = "tcp"
	cb.mu.Unlock()

	return nil
}

// generateFiles creates the bootstrap JS script and MCP config JSON in the temp directory.
func (cb *mcpCallback) generateFiles() error {
	cb.mu.Lock()
	address := cb.address
	transport := cb.transportType
	tempDir := cb.tempDir
	cb.mu.Unlock()

	// Generate bootstrap JS script (data module with connection parameters)
	transportJSON, _ := json.Marshal(transport)
	addressJSON, _ := json.Marshal(address)
	scriptContent := fmt.Sprintf(`// Auto-generated by osm:mcpcallback — do not edit
// Transport: %s | Address: %s
module.exports = {
  transport: %s,
  address: %s
};
`, transport, address, transportJSON, addressJSON)

	scriptPath := filepath.Join(tempDir, "bootstrap.js")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0600); err != nil {
		return fmt.Errorf("write bootstrap script: %w", err)
	}

	// Generate MCP config JSON for Claude Code integration
	var connectArg string
	if transport == "unix" {
		connectArg = "UNIX-CONNECT:" + address
	} else {
		connectArg = "TCP:" + address
	}

	mcpConfig := map[string]any{
		"mcpServers": map[string]any{
			"osm-callback": map[string]any{
				"command": "socat",
				"args":    []string{"STDIO", connectArg},
			},
		},
	}

	configBytes, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mcp config: %w", err)
	}

	configPath := filepath.Join(tempDir, "mcp-config.json")
	if err := os.WriteFile(configPath, configBytes, 0600); err != nil {
		return fmt.Errorf("write mcp config: %w", err)
	}

	cb.mu.Lock()
	cb.scriptPath = scriptPath
	cb.configPath = configPath
	cb.mu.Unlock()

	return nil
}

// acceptLoop accepts incoming connections and serves MCP on each.
// Each connection gets its own ServerSession via Server.Connect().
// The listener is passed as a parameter to avoid a data race with cleanup().
func (cb *mcpCallback) acceptLoop(ctx context.Context, listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			// Check if we're shutting down
			select {
			case <-ctx.Done():
				return
			default:
			}
			// listener.Close() causes Accept() to return net.ErrClosed
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue // Retry on temporary errors
		}

		// Serve MCP on this connection in a dedicated goroutine.
		// Each connection gets its own Transport (SDK requirement: one Transport per Connect call).
		go func() {
			defer conn.Close()
			transport := &mcp.IOTransport{
				Reader: conn,
				Writer: conn,
			}
			session, sErr := cb.server.Connect(ctx, transport, nil)
			if sErr != nil {
				return
			}
			_ = session.Wait()
		}()
	}
}

// jsClose returns the JS method: close() → Promise<void>
//
// Tears down all resources: cancels the context, closes the listener,
// removes temp directory. Idempotent — safe to call multiple times.
func (cb *mcpCallback) jsClose() func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		cb.mu.Lock()
		if cb.closed {
			cb.mu.Unlock()
			// Idempotent — return already-resolved promise
			promise, resolve, _ := cb.adapter.JS().NewChainedPromise()
			resolve(goja.Undefined())
			return cb.adapter.GojaWrapPromise(promise)
		}
		cb.closed = true
		cb.mu.Unlock()

		promise, resolve, _ := cb.adapter.JS().NewChainedPromise()

		go func() {
			cb.cleanup()

			if submitErr := cb.loop.Submit(func() {
				resolve(goja.Undefined())
			}); submitErr != nil {
				// Event loop gone
			}
		}()

		return cb.adapter.GojaWrapPromise(promise)
	}
}

// cleanup tears down all resources synchronously.
// Safe to call from signal handlers and from multiple goroutines concurrently.
func (cb *mcpCallback) cleanup() {
	cb.mu.Lock()
	stop := cb.stop
	listener := cb.listener
	tempDir := cb.tempDir
	cb.stop = nil
	cb.listener = nil
	cb.tempDir = ""
	cb.mu.Unlock()

	// Stop signal notification and cancel context — stops Server.Connect() goroutines
	if stop != nil {
		stop()
	}

	// Close listener — stops Accept() loop
	if listener != nil {
		_ = listener.Close()
	}

	// Remove temp directory and all contents (socket, scripts, config)
	if tempDir != "" {
		_ = os.RemoveAll(tempDir)
	}
}

// --- Go-native tool registration and synchronous IPC ---
//
// These methods enable synchronous JS code (e.g., automatedSplit) to register
// MCP tools with Go-native handlers and block until a tool call arrives.
// This avoids the event loop deadlock that would occur with JS-level handlers:
// the JS event loop is blocked by the waitFor call, but the Go-native handler
// runs on the MCP transport goroutine and doesn't need the event loop.

// jsAddTool returns the JS method: addTool(name, description, inputSchema?)
//
// Registers a Go-native MCP tool whose handler stores incoming call arguments
// in a channel. Use waitFor() to block until data arrives.
// Must be called before initSync()/init().
func (cb *mcpCallback) jsAddTool() func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		cb.mu.Lock()
		if cb.initialized {
			cb.mu.Unlock()
			panic(cb.runtime.NewGoError(errors.New("cannot add tools after init — call addTool before initSync/init")))
		}
		cb.mu.Unlock()

		name := call.Argument(0).String()
		if name == "" {
			panic(cb.runtime.NewGoError(errors.New("addTool: name is required")))
		}

		desc := ""
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			desc = call.Argument(1).String()
		}

		var inputSchema any
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
			raw := call.Argument(2).Export()
			schemaBytes, err := json.Marshal(raw)
			if err != nil {
				panic(cb.runtime.NewGoError(fmt.Errorf("addTool: invalid inputSchema: %w", err)))
			}
			inputSchema = json.RawMessage(schemaBytes)
		}
		if inputSchema == nil {
			// MCP SDK requires an input schema — default to empty object.
			inputSchema = json.RawMessage(`{"type":"object"}`)
		}

		waiter := &toolWaiter{
			ch: make(chan json.RawMessage, 1),
		}

		cb.toolMu.Lock()
		if cb.toolWaiters == nil {
			cb.toolWaiters = make(map[string]*toolWaiter)
		}
		if _, exists := cb.toolWaiters[name]; exists {
			cb.toolMu.Unlock()
			panic(cb.runtime.NewGoError(fmt.Errorf("addTool: tool %q already registered", name)))
		}
		cb.toolWaiters[name] = waiter
		cb.toolMu.Unlock()

		// Register Go-native handler on the MCP server.
		// This handler runs on the MCP transport goroutine, NOT the JS event loop.
		cb.server.AddTool(&mcp.Tool{
			Name:        name,
			Description: desc,
			InputSchema: inputSchema,
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Non-blocking send: drain old data if channel is full (last-write-wins).
			select {
			case waiter.ch <- req.Params.Arguments:
			default:
				select {
				case <-waiter.ch:
				default:
				}
				waiter.ch <- req.Params.Arguments
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "accepted"}},
			}, nil
		})

		return goja.Undefined()
	}
}

// jsInitSync returns the JS method: initSync()
//
// Synchronous version of init() — blocks the calling goroutine until the
// listener is ready. Does not use promises. Designed for synchronous JS code
// that cannot await (e.g., automatedSplit pipeline).
func (cb *mcpCallback) jsInitSync() func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		cb.mu.Lock()
		if cb.initialized {
			cb.mu.Unlock()
			panic(cb.runtime.NewGoError(errors.New("MCPCallback already initialized")))
		}
		if cb.closed {
			cb.mu.Unlock()
			panic(cb.runtime.NewGoError(errors.New("MCPCallback already closed")))
		}
		cb.initialized = true
		cb.mu.Unlock()

		if err := cb.startListener(); err != nil {
			cb.cleanup()
			cb.mu.Lock()
			cb.initialized = false
			cb.mu.Unlock()
			panic(cb.runtime.NewGoError(fmt.Errorf("initSync: start listener: %w", err)))
		}

		if err := cb.generateFiles(); err != nil {
			cb.cleanup()
			cb.mu.Lock()
			cb.initialized = false
			cb.mu.Unlock()
			panic(cb.runtime.NewGoError(fmt.Errorf("initSync: generate files: %w", err)))
		}

		// Start accept loop in background goroutine
		parent := cb.parentCtx
		if parent == nil {
			parent = context.Background()
		}
		ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
		cb.mu.Lock()
		cb.stop = stop
		cb.ctx = ctx
		listener := cb.listener
		cb.mu.Unlock()

		go cb.acceptLoop(ctx, listener)

		// Auto-cleanup on context cancellation
		go func() {
			<-ctx.Done()
			cb.mu.Lock()
			alreadyClosed := cb.closed
			cb.closed = true
			cb.mu.Unlock()
			if !alreadyClosed {
				cb.cleanup()
			}
		}()

		return goja.Undefined()
	}
}

// jsWaitFor returns the JS method: waitFor(toolName, timeoutMs, opts?)
//
// Blocks the calling goroutine until the named tool is called via MCP,
// or the timeout expires. Returns {data: <parsed args>, error: null} on
// success, or {data: null, error: <string>} on timeout/error.
//
// opts (optional object):
//
//	aliveCheck: function() → bool — called periodically; false = abort
//	onProgress: function(elapsedMs, totalMs) — called periodically for TUI updates
//	checkIntervalMs: number — interval for aliveCheck/onProgress (default 5000)
func (cb *mcpCallback) jsWaitFor() func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		if name == "" {
			return cb.waitResult(nil, "waitFor: tool name is required")
		}

		timeoutMs := int64(600000) // default 10 min
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			timeoutMs = call.Argument(1).ToInteger()
		}
		if timeoutMs < 100 {
			timeoutMs = 100
		}

		// Parse optional opts object
		var aliveCheckFn goja.Callable
		var progressFn goja.Callable
		checkIntervalMs := int64(5000)

		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
			opts := call.Argument(2).ToObject(cb.runtime)
			if v := opts.Get("aliveCheck"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				aliveCheckFn, _ = goja.AssertFunction(v)
			}
			if v := opts.Get("onProgress"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				progressFn, _ = goja.AssertFunction(v)
			}
			if v := opts.Get("checkIntervalMs"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				checkIntervalMs = v.ToInteger()
				if checkIntervalMs < 100 {
					checkIntervalMs = 100
				}
			}
		}

		cb.toolMu.RLock()
		waiter, ok := cb.toolWaiters[name]
		cb.toolMu.RUnlock()

		if !ok {
			return cb.waitResult(nil, "waitFor: tool not registered: "+name)
		}

		// Get cancellation context
		cb.mu.Lock()
		ctx := cb.ctx
		cb.mu.Unlock()

		timeout := time.Duration(timeoutMs) * time.Millisecond
		deadline := time.NewTimer(timeout)
		defer deadline.Stop()

		interval := time.Duration(checkIntervalMs) * time.Millisecond
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		startTime := time.Now()

		for {
			select {
			case data := <-waiter.ch:
				var parsed any
				if len(data) > 0 {
					if err := json.Unmarshal(data, &parsed); err != nil {
						return cb.waitResult(nil, "waitFor: failed to parse tool data: "+err.Error())
					}
				}
				result := cb.runtime.NewObject()
				_ = result.Set("data", cb.runtime.ToValue(parsed))
				_ = result.Set("error", goja.Null())
				return result

			case <-ticker.C:
				// Alive check — call JS function on same goroutine (reentrant, safe)
				if aliveCheckFn != nil {
					ret, err := aliveCheckFn(goja.Undefined())
					if err == nil && !ret.ToBoolean() {
						return cb.waitResult(nil, "process exited during wait for "+name)
					}
				}
				// Progress callback
				if progressFn != nil {
					elapsed := time.Since(startTime).Milliseconds()
					_, _ = progressFn(goja.Undefined(),
						cb.runtime.ToValue(elapsed),
						cb.runtime.ToValue(timeoutMs))
				}

			case <-deadline.C:
				return cb.waitResult(nil, fmt.Sprintf("timeout waiting for %s after %dms", name, timeoutMs))

			case <-func() <-chan struct{} {
				if ctx != nil {
					return ctx.Done()
				}
				// No context — return a channel that never fires
				return make(chan struct{})
			}():
				return cb.waitResult(nil, "MCPCallback closed during wait for "+name)
			}
		}
	}
}

// waitResult builds a JS {data, error} result object.
func (cb *mcpCallback) waitResult(data any, errMsg string) goja.Value {
	result := cb.runtime.NewObject()
	if data != nil {
		_ = result.Set("data", cb.runtime.ToValue(data))
	} else {
		_ = result.Set("data", goja.Null())
	}
	if errMsg != "" {
		_ = result.Set("error", errMsg)
	} else {
		_ = result.Set("error", goja.Null())
	}
	return result
}

// jsCloseSync returns the JS method: closeSync()
//
// Synchronous version of close() — tears down all resources and returns
// immediately. Idempotent.
func (cb *mcpCallback) jsCloseSync() func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		cb.mu.Lock()
		if cb.closed {
			cb.mu.Unlock()
			return goja.Undefined()
		}
		cb.closed = true
		cb.mu.Unlock()

		cb.cleanup()
		return goja.Undefined()
	}
}

// jsResetWaiter returns the JS method: resetWaiter(toolName)
//
// Drains the channel for the named tool, discarding any pending data.
// Use before re-waiting (e.g., re-classification after a re-split).
func (cb *mcpCallback) jsResetWaiter() func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		if name == "" {
			panic(cb.runtime.NewGoError(errors.New("resetWaiter: tool name is required")))
		}

		cb.toolMu.RLock()
		waiter, ok := cb.toolWaiters[name]
		cb.toolMu.RUnlock()

		if !ok {
			panic(cb.runtime.NewGoError(fmt.Errorf("resetWaiter: tool not registered: %s", name)))
		}

		// Drain the channel (non-blocking)
		select {
		case <-waiter.ch:
		default:
		}

		return goja.Undefined()
	}
}
