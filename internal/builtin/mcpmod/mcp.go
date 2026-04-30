// Package mcpmod provides the osm:mcp native module for creating MCP servers
// from JavaScript. It wraps the MCP Go SDK (github.com/modelcontextprotocol/go-sdk/mcp)
// to allow JS scripts to define and run MCP tools accessible via stdio transport.
package mcpmod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const toolHandlerTimeout = 30 * time.Second

// PromisifyFunc is the signature for the event loop's Promisify method.
// Stored in internal data structures for easier mocking in tests.
type PromisifyFunc func(ctx context.Context, fn func(ctx context.Context) (any, error)) goeventloop.Promise

// Require returns a module loader for the osm:mcp module.
// The adapter is used for thread-safe JS callback invocation and Promisify support.
// If adapter is nil, the module loads but createServer is unavailable — matching exec.go behavior.
func Require(adapter *gojaeventloop.Adapter) require.ModuleLoader {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		// Guard against nil adapter to prevent segfault at module load time.
		// exec.go uses the same pattern: the module loads but spawn is unavailable.
		if adapter != nil {
			_ = exports.Set("createServer", jsCreateServer(runtime, adapter, adapter.Loop().Promisify))
		}
	}
}

// jsCreateServer returns the JS function: createServer(name, version) → server object
func jsCreateServer(runtime *goja.Runtime, adapter *gojaeventloop.Adapter, promisify PromisifyFunc) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		version := ""
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			version = call.Argument(1).String()
		}

		srv := mcp.NewServer(&mcp.Implementation{
			Name:    name,
			Version: version,
		}, nil)

		s := &mcpServer{
			server:  srv,
			adapter: adapter,
			runtime: runtime,
		}

		obj := runtime.NewObject()
		_ = obj.Set("addTool", s.jsAddTool())
		_ = obj.Set("run", s.jsRun())
		_ = obj.Set("close", s.jsClose())

		// Expose hidden properties for cross-module access (e.g., osm:mcpcallback).
		// __goServer: the underlying *mcp.Server for direct transport wiring.
		// __isRunning: callable that returns true if run() has been called.
		_ = obj.Set("__goServer", runtime.ToValue(s.server))
		_ = obj.Set("__isRunning", func(call goja.FunctionCall) goja.Value {
			s.mu.Lock()
			defer s.mu.Unlock()
			return runtime.ToValue(s.running)
		})

		return obj
	}
}

// handlerResult carries the result of a JS tool handler back to the MCP goroutine.
type handlerResult struct {
	result *mcp.CallToolResult
	err    error
}

// mcpServer wraps a Go MCP server for JS access.
// adapter is required; loop and promisify are derived at call time.
type mcpServer struct {
	server  *mcp.Server
	adapter *gojaeventloop.Adapter
	runtime *goja.Runtime

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
}

// jsAddTool returns the JS method: server.addTool(toolDef, handler)
//
// toolDef: { name: string, description: string, inputSchema?: object }
// handler: function(input) → { text?: string, error?: string, isError?: bool } | Promise
func (s *mcpServer) jsAddTool() func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		s.mu.Lock()
		defer s.mu.Unlock()

		if s.running {
			panic(s.runtime.NewGoError(errors.New("cannot add tools after server.run()")))
		}

		// Parse toolDef object
		defObj := call.Argument(0).ToObject(s.runtime)
		toolName := defObj.Get("name").String()
		toolDesc := ""
		if d := defObj.Get("description"); d != nil && !goja.IsUndefined(d) {
			toolDesc = d.String()
		}

		// Parse optional inputSchema → json.RawMessage
		var inputSchema any
		if isVal := defObj.Get("inputSchema"); isVal != nil && !goja.IsUndefined(isVal) && !goja.IsNull(isVal) {
			schemaObj := isVal.Export()
			schemaBytes, err := json.Marshal(schemaObj)
			if err != nil {
				panic(s.runtime.NewGoError(fmt.Errorf("invalid inputSchema: %w", err)))
			}
			inputSchema = json.RawMessage(schemaBytes)
		}

		// Handler is the second argument — must be callable
		handlerVal := call.Argument(1)
		handler, ok := goja.AssertFunction(handlerVal)
		if !ok {
			panic(s.runtime.NewGoError(errors.New("second argument to addTool must be a function")))
		}

		tool := &mcp.Tool{
			Name:        toolName,
			Description: toolDesc,
			InputSchema: inputSchema,
		}

		// Use low-level Server.AddTool (non-generic) with ToolHandler
		s.server.AddTool(tool, s.makeToolHandler(handler))

		return goja.Undefined()
	}
}

// makeToolHandler creates a Go ToolHandler that bridges MCP calls to JS callbacks.
func (s *mcpServer) makeToolHandler(handler goja.Callable) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resultCh := make(chan handlerResult, 1)

		// Schedule JS callback on event loop thread (goja.Runtime is not thread-safe)
		if err := s.adapter.Loop().Submit(func() {
			// Parse raw arguments to a JS object
			var args any
			if req.Params.Arguments != nil {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					resultCh <- handlerResult{err: fmt.Errorf("failed to parse tool arguments: %w", err)}
					return
				}
			}

			inputVal := s.runtime.ToValue(args)

			// Call the JS handler
			retVal, callErr := handler(goja.Undefined(), inputVal)
			if callErr != nil {
				result := &mcp.CallToolResult{}
				result.SetError(callErr)
				resultCh <- handlerResult{result: result}
				return
			}

			// Check if return value is a Promise (duck-type via callable .then)
			if s.isPromise(retVal) {
				s.handlePromiseResult(retVal, resultCh)
				return
			}

			// Synchronous result
			resultCh <- handlerResult{result: s.convertJSResult(retVal)}
		}); err != nil {
			return nil, fmt.Errorf("event loop not running: %w", err)
		}

		// Wait for result with timeout
		select {
		case hr := <-resultCh:
			return hr.result, hr.err
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(toolHandlerTimeout):
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("tool handler timeout after %s", toolHandlerTimeout))
			return result, nil
		}
	}
}

// isPromise duck-types a goja.Value as a Promise by checking for a callable .then property.
// goja's Export() behavior for Promises varies — a synchronously-resolved Promise may
// export as the resolved value directly, not as *goja.Promise. Duck-typing is robust.
func (s *mcpServer) isPromise(val goja.Value) bool {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return false
	}
	obj := val.ToObject(s.runtime)
	if obj == nil {
		return false
	}
	thenProp := obj.Get("then")
	if thenProp == nil || goja.IsUndefined(thenProp) {
		return false
	}
	_, ok := goja.AssertFunction(thenProp)
	return ok
}

// handlePromiseResult chains .then()/.catch() on a Promise-like goja value and sends
// the settled result to resultCh. Called on the event loop thread; the .then/.catch
// callbacks will also run on the event loop thread when the promise settles.
func (s *mcpServer) handlePromiseResult(promiseVal goja.Value, resultCh chan<- handlerResult) {
	obj := promiseVal.ToObject(s.runtime)

	thenFn, _ := goja.AssertFunction(obj.Get("then"))
	catchFn, _ := goja.AssertFunction(obj.Get("catch"))

	// .then handler — called when promise resolves
	onFulfilled := s.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		resolved := call.Argument(0)
		resultCh <- handlerResult{result: s.convertJSResult(resolved)}
		return goja.Undefined()
	})

	// .catch handler — called when promise rejects
	onRejected := s.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		rejected := call.Argument(0)
		result := &mcp.CallToolResult{}
		result.SetError(fmt.Errorf("%v", rejected.Export()))
		resultCh <- handlerResult{result: result}
		return goja.Undefined()
	})

	// Chain .then(onFulfilled).catch(onRejected)
	thenResult, err := thenFn(promiseVal, onFulfilled)
	if err != nil {
		result := &mcp.CallToolResult{}
		result.SetError(fmt.Errorf("failed to chain .then: %w", err))
		resultCh <- handlerResult{result: result}
		return
	}
	thenObj := thenResult.ToObject(s.runtime)
	thenCatchFn, _ := goja.AssertFunction(thenObj.Get("catch"))
	if thenCatchFn != nil {
		if _, err := thenCatchFn(thenResult, onRejected); err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("failed to chain .catch: %w", err))
			resultCh <- handlerResult{result: result}
		}
	} else if catchFn != nil {
		// Fallback: chain .catch on original if .then didn't return thenable
		if _, err := catchFn(promiseVal, onRejected); err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("failed to chain .catch: %w", err))
			resultCh <- handlerResult{result: result}
		}
	}
}

// convertJSResult converts a JS return value to a CallToolResult.
// Expected shapes:
//   - { text: "..." }  → TextContent
//   - { error: "..." } → SetError
//   - { isError: true, text: "..." } → IsError + TextContent
//   - string → TextContent
func (s *mcpServer) convertJSResult(val goja.Value) *mcp.CallToolResult {
	result := &mcp.CallToolResult{}

	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		result.Content = []mcp.Content{&mcp.TextContent{Text: ""}}
		return result
	}

	// If it's a plain string, wrap as text content
	if val.ExportType() != nil && val.ExportType().Kind().String() == "string" {
		result.Content = []mcp.Content{&mcp.TextContent{Text: val.String()}}
		return result
	}

	// Object — check for text, error, isError fields
	obj := val.ToObject(s.runtime)
	if obj == nil {
		result.Content = []mcp.Content{&mcp.TextContent{Text: val.String()}}
		return result
	}

	// Check error field
	if errVal := obj.Get("error"); errVal != nil && !goja.IsUndefined(errVal) && !goja.IsNull(errVal) {
		result.SetError(fmt.Errorf("%s", errVal.String()))
		return result
	}

	// Check isError flag
	if isErrVal := obj.Get("isError"); isErrVal != nil && !goja.IsUndefined(isErrVal) {
		result.IsError = isErrVal.ToBoolean()
	}

	// Check text field
	text := ""
	if textVal := obj.Get("text"); textVal != nil && !goja.IsUndefined(textVal) && !goja.IsNull(textVal) {
		text = textVal.String()
	}
	result.Content = []mcp.Content{&mcp.TextContent{Text: text}}

	return result
}

// jsRun returns the JS method: server.run(transport) → Promise<void>
func (s *mcpServer) jsRun() func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		s.mu.Lock()
		if s.running {
			s.mu.Unlock()
			panic(s.runtime.NewGoError(errors.New("server is already running")))
		}

		// Validate transport BEFORE setting running flag to avoid stale state
		transport := "stdio"
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			transport = call.Argument(0).String()
		}
		if transport != "stdio" {
			s.mu.Unlock()
			panic(s.runtime.NewGoError(fmt.Errorf("unsupported transport %q (only 'stdio' is supported)", transport)))
		}

		s.running = true
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.mu.Unlock()

		// Create promise for async run
		promise, resolve, reject := s.adapter.JS().NewChainedPromise()

		s.adapter.Loop().Promisify(context.Background(), func(_ context.Context) (any, error) {
			err := s.server.Run(ctx, &mcp.StdioTransport{})
			if err != nil && !errors.Is(err, context.Canceled) {
				_ = s.adapter.Loop().Submit(func() {
					reject(err)
				})
				return nil, err
			} else {
				if submitErr := s.adapter.Loop().Submit(func() {
					resolve(goja.Undefined())
				}); submitErr != nil {
					// Event loop gone — nothing to do
				}
			}
			return nil, nil
		})

		return s.adapter.GojaWrapPromise(promise)
	}
}

// jsClose returns the JS method: server.close()
func (s *mcpServer) jsClose() func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		s.mu.Lock()
		defer s.mu.Unlock()

		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
		s.running = false

		return goja.Undefined()
	}
}
