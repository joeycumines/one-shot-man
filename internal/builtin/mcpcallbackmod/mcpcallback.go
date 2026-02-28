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
