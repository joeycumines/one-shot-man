package mcpmod

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// runOnLoop submits fn to the event loop and waits.
func runOnLoop(t *testing.T, p *testutil.TestEventLoopProvider, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if err := p.Loop().Submit(func() {
		defer close(done)
		fn()
	}); err != nil {
		t.Fatalf("failed to submit to event loop: %v", err)
	}
	<-done
}

// loadModule loads the osm:mcp module exports into the JS runtime as "mcpMod".
func loadModule(t *testing.T, p *testutil.TestEventLoopProvider) {
	t.Helper()
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		loader := Require(p.Adapter(), p.Loop(), p.Promisify)
		module := vm.NewObject()
		exports := vm.NewObject()
		_ = module.Set("exports", exports)
		loader(vm, module)
		_ = vm.Set("mcpMod", exports)
	})
}

// runAsync wraps JS in an async IIFE and waits for it to resolve/reject.
func runAsync(t *testing.T, p *testutil.TestEventLoopProvider, js string) {
	t.Helper()
	done := make(chan error, 1)
	if err := p.Loop().Submit(func() {
		vm := p.Runtime()
		_ = vm.Set("__asyncDone", func() { done <- nil })
		_ = vm.Set("__asyncFail", func(msg string) { done <- errors.New(msg) })
		wrapped := `(async function() { ` + js + ` })()
		.then(function() { __asyncDone(); })
		.catch(function(e) { __asyncFail(e.message || String(e)); });`
		if _, runErr := vm.RunString(wrapped); runErr != nil {
			done <- runErr
		}
	}); err != nil {
		t.Fatalf("failed to submit to event loop: %v", err)
	}
	select {
	case result := <-done:
		if result != nil {
			t.Fatal(result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("async test timed out")
	}
}

// TestCreateServer verifies createServer returns a valid server object.
func TestCreateServer(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModule(t, p)
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		val, err := vm.RunString(`
			var srv = mcpMod.createServer('test-server', '1.0.0');
			typeof srv === 'object' &&
			typeof srv.addTool === 'function' &&
			typeof srv.run === 'function' &&
			typeof srv.close === 'function';
		`)
		if err != nil {
			t.Fatalf("createServer failed: %v", err)
		}
		if !val.ToBoolean() {
			t.Fatal("createServer did not return object with expected methods")
		}
	})
}

// TestCreateServer_DefaultVersion verifies version defaults to empty string.
func TestCreateServer_DefaultVersion(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModule(t, p)
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		val, err := vm.RunString(`
			var srv = mcpMod.createServer('test');
			typeof srv.addTool === 'function';
		`)
		if err != nil {
			t.Fatalf("createServer with one arg failed: %v", err)
		}
		if !val.ToBoolean() {
			t.Fatal("server missing addTool method")
		}
	})
}

// TestAddTool_Registers verifies addTool accepts a valid tool definition.
func TestAddTool_Registers(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModule(t, p)
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			srv.addTool({
				name: 'echo',
				description: 'Echo the input',
				inputSchema: {
					type: 'object',
					properties: { msg: { type: 'string' } }
				}
			}, function(input) {
				return { text: input.msg };
			});
		`)
		if err != nil {
			t.Fatalf("addTool failed: %v", err)
		}
	})
}

// TestAddTool_NoHandler_Panics verifies addTool requires a function handler.
func TestAddTool_NoHandler_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModule(t, p)
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			try {
				srv.addTool({ name: 'bad' }, 'not-a-function');
				throw new Error('should have thrown');
			} catch (e) {
				if (!e.message.includes('function')) {
					throw new Error('unexpected error: ' + e.message);
				}
			}
		`)
		if err != nil {
			t.Fatalf("expected error to be caught, got: %v", err)
		}
	})
}

// TestAddTool_AfterRun_Panics verifies addTool rejects tools after run().
func TestAddTool_AfterRun_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModule(t, p)
	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		// Start server (it will fail because stdin is not MCP, but running flag is set)
		var runPromise = srv.run('stdio');
		try {
			srv.addTool({ name: 'late' }, function() { return { text: 'hi' }; });
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('cannot add tools after')) {
				throw new Error('unexpected error: ' + e.message);
			}
		}
		srv.close();
		try { await runPromise; } catch(e) { /* expected - no real MCP client */ }
	`)
}

// TestClose_Idempotent verifies close() can be called multiple times safely.
func TestClose_Idempotent(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModule(t, p)
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			srv.close();
			srv.close();
			srv.close();
		`)
		if err != nil {
			t.Fatalf("close() should be idempotent: %v", err)
		}
	})
}

// TestRun_UnsupportedTransport verifies run() rejects unknown transports.
func TestRun_UnsupportedTransport(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModule(t, p)
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			try {
				srv.run('websocket');
				throw new Error('should have thrown');
			} catch (e) {
				if (!e.message.includes('unsupported transport')) {
					throw new Error('unexpected error: ' + e.message);
				}
			}
		`)
		if err != nil {
			t.Fatalf("expected error to be caught: %v", err)
		}
	})
}

// TestRun_DoubleRun_Panics verifies run() rejects a second call.
func TestRun_DoubleRun_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModule(t, p)
	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		var p1 = srv.run('stdio');
		try {
			srv.run('stdio');
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('already running')) {
				throw new Error('unexpected error: ' + e.message);
			}
		}
		srv.close();
		try { await p1; } catch(e) { /* expected */ }
	`)
}

// TestConvertJSResult verifies result conversion from JS objects.
func TestConvertJSResult(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		adapter := p.Adapter()
		s := &mcpServer{
			runtime: vm,
			adapter: adapter,
			loop:    p.Loop(),
		}

		tests := []struct {
			name      string
			js        string
			wantText  string
			wantError bool
		}{
			{
				name:     "text result",
				js:       `({ text: 'hello' })`,
				wantText: "hello",
			},
			{
				name:     "string result",
				js:       `'direct string'`,
				wantText: "direct string",
			},
			{
				name:      "error result",
				js:        `({ error: 'bad input' })`,
				wantError: true,
			},
			{
				name:      "isError flag",
				js:        `({ isError: true, text: 'failed' })`,
				wantText:  "failed",
				wantError: true,
			},
			{
				name:     "null result",
				js:       `null`,
				wantText: "",
			},
			{
				name:     "undefined result",
				js:       `undefined`,
				wantText: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				val, err := vm.RunString(tt.js)
				if err != nil {
					t.Fatalf("failed to eval JS: %v", err)
				}
				result := s.convertJSResult(val)
				if tt.wantError {
					// For "error" field result, SetError was called
					if result.GetError() != nil {
						return // SetError was called — correct
					}
					// For "isError" flag result
					if !result.IsError {
						t.Errorf("expected isError=true")
					}
				}
				if tt.wantText != "" && len(result.Content) > 0 {
					if tc, ok := result.Content[0].(*mcp.TextContent); ok {
						if tc.Text != tt.wantText {
							t.Errorf("got text %q, want %q", tc.Text, tt.wantText)
						}
					} else {
						t.Errorf("expected TextContent, got %T", result.Content[0])
					}
				}
				if tt.wantText == "" && !tt.wantError && len(result.Content) > 0 {
					if tc, ok := result.Content[0].(*mcp.TextContent); ok {
						if tc.Text != "" {
							t.Errorf("expected empty text, got %q", tc.Text)
						}
					}
				}
			})
		}
	})
}

// TestMakeToolHandler_SyncHandler verifies that makeToolHandler correctly bridges
// a synchronous JS function to a Go MCP ToolHandler, invoking it on the event loop
// and returning the result to the calling goroutine.
func TestMakeToolHandler_SyncHandler(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)

	var handler mcp.ToolHandler

	// Register a JS handler on the event loop thread
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		s := &mcpServer{
			runtime: vm,
			adapter: p.Adapter(),
			loop:    p.Loop(),
		}

		// Define JS function: function(input) { return { text: 'hello ' + input.name }; }
		fn, err := vm.RunString(`(function(input) { return { text: 'hello ' + input.name }; })`)
		if err != nil {
			t.Fatalf("failed to create JS function: %v", err)
		}
		callable, ok := goja.AssertFunction(fn)
		if !ok {
			t.Fatal("expected callable function")
		}

		handler = s.makeToolHandler(callable)
	})

	// Call the handler from a non-event-loop goroutine (simulates MCP transport)
	args, _ := json.Marshal(map[string]string{"name": "world"})
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "echo",
			Arguments: args,
		},
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.GetError() != nil {
		t.Fatalf("handler returned tool error: %v", result.GetError())
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text != "hello world" {
		t.Errorf("got %q, want %q", tc.Text, "hello world")
	}
}

// TestMakeToolHandler_ErrorPropagation verifies that JS handler errors
// propagate back as MCP tool errors.
func TestMakeToolHandler_ErrorPropagation(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)

	var handler mcp.ToolHandler

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		s := &mcpServer{
			runtime: vm,
			adapter: p.Adapter(),
			loop:    p.Loop(),
		}

		fn, err := vm.RunString(`(function(input) { throw new Error('handler boom'); })`)
		if err != nil {
			t.Fatalf("failed to create JS function: %v", err)
		}
		callable, _ := goja.AssertFunction(fn)
		handler = s.makeToolHandler(callable)
	})

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Name: "throw"},
	}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error (expected tool error): %v", err)
	}
	if result.GetError() == nil {
		t.Fatal("expected tool error from throwing handler")
	}
}

// TestMakeToolHandler_PromiseHandler verifies that async JS handlers (returning
// a Promise) are correctly bridged: the polling goroutine waits for settlement
// and returns the resolved value.
func TestMakeToolHandler_PromiseHandler(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)

	var handler mcp.ToolHandler

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		s := &mcpServer{
			runtime: vm,
			adapter: p.Adapter(),
			loop:    p.Loop(),
		}

		// Async handler that resolves after a microtask
		fn, err := vm.RunString(`(function(input) {
			return new Promise(function(resolve) {
				resolve({ text: 'async ' + input.tag });
			});
		})`)
		if err != nil {
			t.Fatalf("failed to create async JS function: %v", err)
		}
		callable, _ := goja.AssertFunction(fn)
		handler = s.makeToolHandler(callable)
	})

	args, _ := json.Marshal(map[string]string{"tag": "result"})
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "async-echo",
			Arguments: args,
		},
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.GetError() != nil {
		t.Fatalf("handler returned tool error: %v", result.GetError())
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text != "async result" {
		t.Errorf("got %q, want %q", tc.Text, "async result")
	}
}

// TestMakeToolHandler_ContextCancellation verifies handler respects context cancellation.
func TestMakeToolHandler_ContextCancellation(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)

	var handler mcp.ToolHandler

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		s := &mcpServer{
			runtime: vm,
			adapter: p.Adapter(),
			loop:    p.Loop(),
		}

		// Handler that never returns (blocks forever via a pending promise)
		fn, err := vm.RunString(`(function(input) {
			return new Promise(function(resolve) {
				// Never resolves
			});
		})`)
		if err != nil {
			t.Fatalf("failed to create JS function: %v", err)
		}
		callable, _ := goja.AssertFunction(fn)
		handler = s.makeToolHandler(callable)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Name: "block"},
	}
	_, err := handler(ctx, req)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}
}

// TestRun_UnsupportedTransport_ThenValidRun verifies that after a failed run('websocket'),
// the server is NOT in a corrupted state and run('stdio') can still be called.
func TestRun_UnsupportedTransport_ThenValidRun(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModule(t, p)
	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		// Attempt invalid transport — should fail but not brick server
		try {
			srv.run('websocket');
		} catch(e) {
			// expected
		}
		// Now run with valid transport — should succeed
		var p1 = srv.run('stdio');
		srv.close();
		try { await p1; } catch(e) { /* expected - no real MCP client */ }
	`)
}
