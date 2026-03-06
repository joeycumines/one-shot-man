package mcpcallbackmod

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// runOnLoop submits fn to the event loop and waits for completion.
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
	case <-time.After(30 * time.Second):
		t.Fatal("async test timed out")
	}
}

// loadModules loads both osm:mcp and osm:mcpcallback into the JS runtime.
func loadModules(t *testing.T, p *testutil.TestEventLoopProvider) {
	t.Helper()
	runOnLoop(t, p, func() {
		vm := p.Runtime()

		// Load osm:mcp module
		mcpLoader := func(rt *goja.Runtime, module *goja.Object) {
			exports := module.Get("exports").(*goja.Object)
			_ = exports.Set("createServer", jsCreateServerForTest(rt, p))
		}
		mcpModule := vm.NewObject()
		mcpExports := vm.NewObject()
		_ = mcpModule.Set("exports", mcpExports)
		mcpLoader(vm, mcpModule)
		_ = vm.Set("mcpMod", mcpExports)

		// Load osm:mcpcallback module
		cbLoader := Require(p.Adapter(), p.Loop())
		cbModule := vm.NewObject()
		cbExports := vm.NewObject()
		_ = cbModule.Set("exports", cbExports)
		cbLoader(vm, cbModule)
		_ = vm.Set("mcpCbMod", cbExports)
	})
}

// jsCreateServerForTest mirrors mcpmod.jsCreateServer for test use.
// It creates a real MCP server and exposes __goServer and __isRunning.
func jsCreateServerForTest(rt *goja.Runtime, p *testutil.TestEventLoopProvider) func(call goja.FunctionCall) goja.Value {
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

		obj := rt.NewObject()
		running := false

		// Add a real tool for E2E testing
		_ = obj.Set("addTool", func(call goja.FunctionCall) goja.Value {
			defObj := call.Argument(0).ToObject(rt)
			toolName := defObj.Get("name").String()
			toolDesc := ""
			if d := defObj.Get("description"); d != nil && !goja.IsUndefined(d) {
				toolDesc = d.String()
			}

			var inputSchema any
			if isVal := defObj.Get("inputSchema"); isVal != nil && !goja.IsUndefined(isVal) && !goja.IsNull(isVal) {
				schemaBytes, _ := json.Marshal(isVal.Export())
				inputSchema = json.RawMessage(schemaBytes)
			}

			handlerVal := call.Argument(1)
			handler, ok := goja.AssertFunction(handlerVal)
			if !ok {
				panic(rt.NewGoError(errors.New("handler must be a function")))
			}

			tool := &mcp.Tool{
				Name:        toolName,
				Description: toolDesc,
				InputSchema: inputSchema,
			}

			// Create Go handler that bridges to JS via event loop
			srv.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				resultCh := make(chan *mcp.CallToolResult, 1)
				errCh := make(chan error, 1)

				if submitErr := p.Loop().Submit(func() {
					var args any
					if req.Params.Arguments != nil {
						if unmarshalErr := json.Unmarshal(req.Params.Arguments, &args); unmarshalErr != nil {
							errCh <- unmarshalErr
							return
						}
					}
					retVal, callErr := handler(goja.Undefined(), rt.ToValue(args))
					if callErr != nil {
						errCh <- callErr
						return
					}
					result := &mcp.CallToolResult{}
					if retVal != nil && !goja.IsUndefined(retVal) && !goja.IsNull(retVal) {
						obj := retVal.ToObject(rt)
						if textVal := obj.Get("text"); textVal != nil && !goja.IsUndefined(textVal) {
							result.Content = []mcp.Content{&mcp.TextContent{Text: textVal.String()}}
						}
					}
					resultCh <- result
				}); submitErr != nil {
					return nil, submitErr
				}

				select {
				case r := <-resultCh:
					return r, nil
				case e := <-errCh:
					return nil, e
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			})

			return goja.Undefined()
		})

		_ = obj.Set("__goServer", rt.ToValue(srv))
		_ = obj.Set("__isRunning", func(call goja.FunctionCall) goja.Value {
			return rt.ToValue(running)
		})
		_ = obj.Set("run", func(call goja.FunctionCall) goja.Value {
			running = true
			return goja.Undefined()
		})
		_ = obj.Set("close", func(call goja.FunctionCall) goja.Value {
			running = false
			return goja.Undefined()
		})

		return obj
	}
}

// --- Constructor tests ---

func TestMCPCallback_Constructor(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		val, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			var cb = mcpCbMod.MCPCallback({ server: srv });
			typeof cb === 'object' &&
			typeof cb.init === 'function' &&
			typeof cb.close === 'function';
		`)
		if err != nil {
			t.Fatalf("constructor failed: %v", err)
		}
		if !val.ToBoolean() {
			t.Fatal("MCPCallback did not return object with expected methods")
		}
	})
}

func TestMCPCallback_Constructor_NoOptions_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			try {
				mcpCbMod.MCPCallback();
				throw new Error('should have thrown');
			} catch (e) {
				if (!e.message.includes('options object')) {
					throw new Error('unexpected error: ' + e.message);
				}
			}
		`)
		if err != nil {
			t.Fatalf("expected error to be caught: %v", err)
		}
	})
}

func TestMCPCallback_Constructor_NoServer_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			try {
				mcpCbMod.MCPCallback({});
				throw new Error('should have thrown');
			} catch (e) {
				if (!e.message.includes('server is required')) {
					throw new Error('unexpected error: ' + e.message);
				}
			}
		`)
		if err != nil {
			t.Fatalf("expected error to be caught: %v", err)
		}
	})
}

func TestMCPCallback_Constructor_RunningServer_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			srv.run();  // Mark as running
			try {
				mcpCbMod.MCPCallback({ server: srv });
				throw new Error('should have thrown');
			} catch (e) {
				if (!e.message.includes('already running')) {
					throw new Error('unexpected error: ' + e.message);
				}
			}
		`)
		if err != nil {
			t.Fatalf("expected error to be caught: %v", err)
		}
	})
}

func TestMCPCallback_Constructor_InvalidServer_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			try {
				mcpCbMod.MCPCallback({ server: { notAServer: true } });
				throw new Error('should have thrown');
			} catch (e) {
				if (!e.message.includes('createServer')) {
					throw new Error('unexpected error: ' + e.message);
				}
			}
		`)
		if err != nil {
			t.Fatalf("expected error to be caught: %v", err)
		}
	})
}

// --- Lifecycle tests ---

func TestMCPCallback_InitClose(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		var cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.init();

		// Properties should be set after init
		if (!cb.address || cb.address === '') throw new Error('address should be set');
		if (!cb.scriptPath || cb.scriptPath === '') throw new Error('scriptPath should be set');
		if (!cb.transport || cb.transport === '') throw new Error('transport should be set');
		if (!cb.mcpConfigPath || cb.mcpConfigPath === '') throw new Error('mcpConfigPath should be set');

		await cb.close();
	`)
}

func TestMCPCallback_InitDoubleInit_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		var cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.init();
		try {
			await cb.init();
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('already initialized')) {
				throw new Error('unexpected error: ' + e.message);
			}
		}
		await cb.close();
	`)
}

func TestMCPCallback_CloseIdempotent(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		var cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.init();
		await cb.close();
		await cb.close();
		await cb.close();
	`)
}

func TestMCPCallback_CloseBeforeInit(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		var cb = mcpCbMod.MCPCallback({ server: srv });
		// close() without init() should be idempotent (no-op)
		await cb.close();
	`)
}

func TestMCPCallback_InitAfterClose_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		var cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.close();
		try {
			await cb.init();
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('already closed')) {
				throw new Error('unexpected error: ' + e.message);
			}
		}
	`)
}

// --- Transport tests ---

func TestMCPCallback_TransportType_Unix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("UDS not available on Windows")
	}

	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		var cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.init();
		if (cb.transport !== 'unix') throw new Error('expected unix transport, got: ' + cb.transport);
		if (!cb.address.includes('osm.sock')) throw new Error('expected socket path, got: ' + cb.address);
		await cb.close();
	`)
}

func TestMCPCallback_SocketPathLength_macOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS socket path limit test only runs on darwin")
	}

	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	var address string
	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		var cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.init();
		__testAddress = cb.address;
		await cb.close();
	`)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		val := vm.Get("__testAddress")
		if val != nil && !goja.IsUndefined(val) {
			address = val.String()
		}
	})

	if len(address) >= 104 {
		t.Errorf("socket path too long for macOS (%d chars >= 104): %s", len(address), address)
	}
	if !strings.HasPrefix(address, "/tmp/") {
		t.Errorf("macOS socket should use /tmp/ prefix, got: %s", address)
	}
}

// --- File generation tests ---

func TestMCPCallback_BootstrapScript(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	var scriptPath string
	runAsync(t, p, `
		srv = mcpMod.createServer('test', '1.0.0');
		cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.init();
		__testScriptPath = cb.scriptPath;
		__testAddress = cb.address;
		__testTransport = cb.transport;
	`)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		scriptPath = vm.Get("__testScriptPath").String()
	})

	// Verify file exists and has correct permissions
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("bootstrap script should exist: %v", err)
	}
	if runtime.GOOS != "windows" {
		if info.Mode().Perm() != 0600 {
			t.Errorf("bootstrap script permissions should be 0600, got: %o", info.Mode().Perm())
		}
	}

	// Verify content includes connection parameters
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("failed to read bootstrap script: %v", err)
	}
	if !strings.Contains(string(content), "module.exports") {
		t.Error("bootstrap script should contain module.exports")
	}
	if !strings.Contains(string(content), "transport") {
		t.Error("bootstrap script should contain transport info")
	}

	// Clean up
	runAsync(t, p, `await cb.close();`)
}

func TestMCPCallback_MCPConfig(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	var configPath string
	runAsync(t, p, `
		srv = mcpMod.createServer('test', '1.0.0');
		cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.init();
		__testConfigPath = cb.mcpConfigPath;
	`)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		configPath = vm.Get("__testConfigPath").String()
	})

	// Verify file exists
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("MCP config should exist: %v", err)
	}

	// Verify it's valid JSON with expected structure
	var config map[string]any
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatalf("MCP config should be valid JSON: %v", err)
	}
	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("MCP config should have mcpServers key")
	}
	callback, ok := servers["osm-callback"].(map[string]any)
	if !ok {
		t.Fatal("MCP config should have osm-callback server")
	}
	if _, ok := callback["command"]; !ok {
		t.Error("osm-callback should have command field")
	}
	if _, ok := callback["args"]; !ok {
		t.Error("osm-callback should have args field")
	}

	// Clean up
	runAsync(t, p, `await cb.close();`)
}

// --- Resource cleanup tests ---

func TestMCPCallback_CloseRemovesTempDir(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	var scriptPath, configPath, address string
	runAsync(t, p, `
		srv = mcpMod.createServer('test', '1.0.0');
		cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.init();
		__testScriptPath = cb.scriptPath;
		__testConfigPath = cb.mcpConfigPath;
		__testAddress = cb.address;
	`)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		scriptPath = vm.Get("__testScriptPath").String()
		configPath = vm.Get("__testConfigPath").String()
		address = vm.Get("__testAddress").String()
	})

	// Verify files exist before close
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("script should exist before close: %v", err)
	}

	// Close and verify cleanup
	runAsync(t, p, `await cb.close();`)

	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Error("bootstrap script should be removed after close")
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("MCP config should be removed after close")
	}
	// Check that the temp directory is gone (parent of script)
	tempDir := scriptPath[:strings.LastIndex(scriptPath, string(os.PathSeparator))]
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Error("temp directory should be removed after close")
	}

	// On Unix, verify socket is gone
	if runtime.GOOS != "windows" && address != "" {
		if _, err := os.Stat(address); !os.IsNotExist(err) {
			t.Error("socket file should be removed after close")
		}
	}
}

func TestMCPCallback_TempDirPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission checks not applicable on Windows")
	}

	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	var scriptPath string
	runAsync(t, p, `
		srv = mcpMod.createServer('test', '1.0.0');
		cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.init();
		__testScriptPath = cb.scriptPath;
	`)

	runOnLoop(t, p, func() {
		scriptPath = p.Runtime().Get("__testScriptPath").String()
	})

	// Check temp directory permissions (parent of script)
	tempDir := scriptPath[:strings.LastIndex(scriptPath, string(os.PathSeparator))]
	info, err := os.Stat(tempDir)
	if err != nil {
		t.Fatalf("temp dir should exist: %v", err)
	}
	if info.Mode().Perm() != 0700 {
		t.Errorf("temp dir permissions should be 0700, got: %o", info.Mode().Perm())
	}

	runAsync(t, p, `await cb.close();`)
}

// --- Socket connectivity tests ---

func TestMCPCallback_SocketAcceptsConnection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("UDS test only runs on Unix")
	}

	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	var address string
	runAsync(t, p, `
		srv = mcpMod.createServer('test', '1.0.0');
		cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.init();
		__testAddress = cb.address;
	`)

	runOnLoop(t, p, func() {
		address = p.Runtime().Get("__testAddress").String()
	})

	// Verify we can connect to the socket
	conn, err := net.Dial("unix", address)
	if err != nil {
		t.Fatalf("should be able to connect to UDS: %v", err)
	}
	conn.Close()

	runAsync(t, p, `await cb.close();`)
}

// --- E2E MCP tool call test ---

func TestMCPCallback_E2E_ToolCall(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	var address, transportType string
	runAsync(t, p, `
		srv = mcpMod.createServer('e2e-test', '1.0.0');
		srv.addTool({
			name: 'echo',
			description: 'Echo the input',
			inputSchema: {
				type: 'object',
				properties: { msg: { type: 'string' } },
				required: ['msg']
			}
		}, function(input) {
			return { text: 'echo: ' + input.msg };
		});
		cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.init();
		__testAddress = cb.address;
		__testTransport = cb.transport;
	`)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		address = vm.Get("__testAddress").String()
		transportType = vm.Get("__testTransport").String()
	})

	// Connect as an MCP client via the socket
	var network string
	if transportType == "unix" {
		network = "unix"
	} else {
		network = "tcp"
	}

	conn, err := net.Dial(network, address)
	if err != nil {
		t.Fatalf("failed to connect to callback: %v", err)
	}

	clientTransport := &mcp.IOTransport{
		Reader: conn,
		Writer: conn,
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		conn.Close()
		t.Fatalf("client connect failed: %v", err)
	}

	// Call the echo tool
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "echo",
		Arguments: map[string]any{"msg": "hello world"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	// Verify result
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content item")
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if textContent.Text != "echo: hello world" {
		t.Errorf("expected 'echo: hello world', got %q", textContent.Text)
	}

	// Close session and callback
	_ = session.Close()
	conn.Close()
	runAsync(t, p, `await cb.close();`)
}

// --- Properties before init ---

func TestMCPCallback_PropertiesBeforeInit(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		val, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			var cb = mcpCbMod.MCPCallback({ server: srv });
			// Properties should be empty/falsy before init
			cb.address === '' && cb.scriptPath === '' && cb.transport === '' && cb.mcpConfigPath === '';
		`)
		if err != nil {
			t.Fatalf("property check failed: %v", err)
		}
		if !val.ToBoolean() {
			t.Error("properties should be empty before init()")
		}
	})
}

// --- Signal/context cleanup tests ---

func TestMCPCallback_ContextCancellation_TriggersCleanup(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	parentCtx, parentCancel := context.WithCancel(context.Background())

	// Set the test context as a JS global
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_ = vm.Set("__parentCtx", vm.ToValue(parentCtx))
	})

	var scriptPath string
	runAsync(t, p, `
		srv = mcpMod.createServer('test', '1.0.0');
		cb = mcpCbMod.MCPCallback({ server: srv, __testContext: __parentCtx });
		await cb.init();
		__testScriptPath = cb.scriptPath;
	`)

	runOnLoop(t, p, func() {
		scriptPath = p.Runtime().Get("__testScriptPath").String()
	})

	// Verify resources exist before cancellation
	tempDir := scriptPath[:strings.LastIndex(scriptPath, string(os.PathSeparator))]
	if _, err := os.Stat(tempDir); err != nil {
		t.Fatalf("temp dir should exist before cancellation: %v", err)
	}

	// Cancel the parent context — this should trigger automatic cleanup
	parentCancel()

	// Wait for cleanup goroutine to fire (with timeout)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(tempDir); os.IsNotExist(err) {
			// Temp dir removed — cleanup worked
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("temp directory was not cleaned up after context cancellation within 5s")
}

func TestMCPCallback_CloseAfterContextCancel(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	parentCtx, parentCancel := context.WithCancel(context.Background())

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_ = vm.Set("__parentCtx", vm.ToValue(parentCtx))
	})

	runAsync(t, p, `
		srv = mcpMod.createServer('test', '1.0.0');
		cb = mcpCbMod.MCPCallback({ server: srv, __testContext: __parentCtx });
		await cb.init();
	`)

	// Cancel the parent context first
	parentCancel()

	// Wait briefly for cleanup goroutine to detect cancellation
	time.Sleep(50 * time.Millisecond)

	// Calling close() after context cancellation should be idempotent (no error/panic)
	runAsync(t, p, `await cb.close();`)
}

func TestMCPCallback_SignalCleanup_CloseIsIdempotent(t *testing.T) {
	// Verify that close() returns cleanly even when the context watcher already cleaned up.
	// This tests the race between signal watcher goroutine and explicit close().
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	parentCtx, parentCancel := context.WithCancel(context.Background())

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_ = vm.Set("__parentCtx", vm.ToValue(parentCtx))
	})

	var scriptPath string
	runAsync(t, p, `
		srv = mcpMod.createServer('test', '1.0.0');
		cb = mcpCbMod.MCPCallback({ server: srv, __testContext: __parentCtx });
		await cb.init();
		__testScriptPath = cb.scriptPath;
	`)

	runOnLoop(t, p, func() {
		scriptPath = p.Runtime().Get("__testScriptPath").String()
	})

	tempDir := scriptPath[:strings.LastIndex(scriptPath, string(os.PathSeparator))]

	// Cancel parent context
	parentCancel()

	// Wait for cleanup
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(tempDir); os.IsNotExist(err) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Now call close() multiple times — should all be idempotent
	runAsync(t, p, `
		await cb.close();
		await cb.close();
		await cb.close();
	`)
}

// --- Tests for Go-native tool registration + synchronous IPC ---

func TestMCPCallback_AddTool(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		var cb = mcpCbMod.MCPCallback({ server: srv });
		cb.addTool('myTool', 'A test tool', {
			type: 'object',
			properties: {
				message: { type: 'string' }
			}
		});
		await cb.init();
		await cb.close();
	`)
}

func TestMCPCallback_AddTool_NoName_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		var cb = mcpCbMod.MCPCallback({ server: srv });
		try {
			cb.addTool('', 'desc');
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('name is required')) {
				throw new Error('unexpected error: ' + e.message);
			}
		}
	`)
}

func TestMCPCallback_AddTool_DuplicateName_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		var cb = mcpCbMod.MCPCallback({ server: srv });
		cb.addTool('myTool', 'first');
		try {
			cb.addTool('myTool', 'duplicate');
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('already registered')) {
				throw new Error('unexpected error: ' + e.message);
			}
		}
	`)
}

func TestMCPCallback_AddTool_AfterInit_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runAsync(t, p, `
		var srv = mcpMod.createServer('test', '1.0.0');
		var cb = mcpCbMod.MCPCallback({ server: srv });
		await cb.init();
		try {
			cb.addTool('lateTool', 'added after init');
			throw new Error('should have thrown');
		} catch (e) {
			if (!e.message.includes('after init')) {
				throw new Error('unexpected error: ' + e.message);
			}
		}
		await cb.close();
	`)
}

func TestMCPCallback_InitSync(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	// initSync is synchronous — use runOnLoop
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			var cb = mcpCbMod.MCPCallback({ server: srv });
			cb.initSync();

			if (!cb.address || cb.address === '') throw new Error('address should be set');
			if (!cb.scriptPath || cb.scriptPath === '') throw new Error('scriptPath should be set');
			if (!cb.transport || cb.transport === '') throw new Error('transport should be set');
			if (!cb.mcpConfigPath || cb.mcpConfigPath === '') throw new Error('mcpConfigPath should be set');

			cb.closeSync();
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestMCPCallback_InitSync_DoubleInit_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			var cb = mcpCbMod.MCPCallback({ server: srv });
			cb.initSync();
			try {
				cb.initSync();
				throw new Error('should have thrown');
			} catch (e) {
				if (!e.message.includes('already initialized')) {
					throw new Error('unexpected error: ' + e.message);
				}
			}
			cb.closeSync();
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestMCPCallback_CloseSync(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			var cb = mcpCbMod.MCPCallback({ server: srv });
			cb.initSync();
			cb.closeSync();
			// Idempotent
			cb.closeSync();
			cb.closeSync();
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestMCPCallback_WaitFor_Timeout(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			var cb = mcpCbMod.MCPCallback({ server: srv });
			cb.addTool('testTool', 'test');
			cb.initSync();

			var result = cb.waitFor('testTool', 200);
			if (!result.error) throw new Error('expected timeout error');
			if (result.error.indexOf('timeout') === -1) throw new Error('unexpected error: ' + result.error);
			if (result.data !== null) throw new Error('data should be null on timeout');

			cb.closeSync();
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestMCPCallback_WaitFor_NotRegistered(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			var cb = mcpCbMod.MCPCallback({ server: srv });
			cb.initSync();

			var result = cb.waitFor('nonExistent', 100);
			if (!result.error) throw new Error('expected error');
			if (result.error.indexOf('not registered') === -1) throw new Error('unexpected error: ' + result.error);

			cb.closeSync();
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestMCPCallback_WaitFor_AliveCheck(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			var cb = mcpCbMod.MCPCallback({ server: srv });
			cb.addTool('testTool', 'test');
			cb.initSync();

			var checkCount = 0;
			var result = cb.waitFor('testTool', 5000, {
				aliveCheck: function() {
					checkCount++;
					return checkCount < 3; // Die on 3rd check
				},
				checkIntervalMs: 100
			});
			if (!result.error) throw new Error('expected error');
			if (result.error.indexOf('process exited') === -1) throw new Error('unexpected: ' + result.error);
			if (checkCount < 3) throw new Error('expected at least 3 alive checks, got ' + checkCount);

			cb.closeSync();
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestMCPCallback_ResetWaiter(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	// Test that resetWaiter doesn't panic on empty channel
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			var cb = mcpCbMod.MCPCallback({ server: srv });
			cb.addTool('testTool', 'test');
			cb.initSync();

			// Reset on empty channel — should not panic
			cb.resetWaiter('testTool');

			cb.closeSync();
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestMCPCallback_ResetWaiter_NotRegistered_Panics(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			var cb = mcpCbMod.MCPCallback({ server: srv });
			cb.initSync();
			try {
				cb.resetWaiter('nope');
				throw new Error('should have thrown');
			} catch (e) {
				if (!e.message.includes('not registered')) {
					throw new Error('unexpected: ' + e.message);
				}
			}
			cb.closeSync();
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
}

// TestMCPCallback_WaitFor_EndToEnd tests the full cycle: addTool → initSync →
// external MCP client calls the tool → waitFor returns the data.
func TestMCPCallback_WaitFor_EndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix domain socket — skipped on Windows")
	}

	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	// Get the address and transport from JS after init
	type connInfo struct {
		address   string
		transport string
	}
	infoCh := make(chan connInfo, 1)

	// Inject helper to extract connection info from JS
	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_ = vm.Set("__reportConnInfo", func(addr, trans string) {
			infoCh <- connInfo{address: addr, transport: trans}
		})
	})

	// Run the JS that sets up server, registers tool, inits, and waits
	resultCh := make(chan error, 1)
	if err := p.Loop().Submit(func() {
		vm := p.Runtime()
		_, runErr := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			var cb = mcpCbMod.MCPCallback({ server: srv });
			cb.addTool('reportClassification', 'Report classification', {
				type: 'object',
				properties: {
					categories: {
						type: 'array',
						items: {
							type: 'object',
							properties: {
								name: { type: 'string' },
								files: { type: 'array', items: { type: 'string' } }
							}
						}
					}
				}
			});
			cb.initSync();
			__reportConnInfo(cb.address, cb.transport);

			// This blocks until the tool is called or timeout
			var result = cb.waitFor('reportClassification', 10000);
			if (result.error) throw new Error('waitFor failed: ' + result.error);
			if (!result.data) throw new Error('expected data');
			if (!result.data.categories) throw new Error('expected categories in data');
			if (result.data.categories.length !== 1) throw new Error('expected 1 category, got ' + result.data.categories.length);
			if (result.data.categories[0].name !== 'types') throw new Error('expected name=types, got ' + result.data.categories[0].name);
			if (result.data.categories[0].files.length !== 2) throw new Error('expected 2 files');

			cb.closeSync();
		`)
		if runErr != nil {
			resultCh <- runErr
		} else {
			resultCh <- nil
		}
	}); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// Wait for the JS to report connection info, then connect as MCP client
	select {
	case info := <-infoCh:
		// Small delay to ensure waitFor is blocking
		time.Sleep(100 * time.Millisecond)

		// Connect to the MCPCallback's socket
		var conn net.Conn
		var err error
		if info.transport == "unix" {
			conn, err = net.Dial("unix", info.address)
		} else {
			conn, err = net.Dial("tcp", info.address)
		}
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close()

		// Send MCP tool call as raw JSON-RPC
		toolArgs := `{"categories":[{"name":"types","files":["a.go","b.go"]}]}`
		reqJSON := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}` + "\n"
		_, err = conn.Write([]byte(reqJSON))
		if err != nil {
			t.Fatalf("failed to write initialize: %v", err)
		}

		// Read initialize response
		buf := make([]byte, 8192)
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("failed to read initialize response: %v", err)
		}
		_ = n // We just need to consume the response

		// Send initialized notification
		notifJSON := `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}` + "\n"
		_, err = conn.Write([]byte(notifJSON))
		if err != nil {
			t.Fatalf("failed to write initialized: %v", err)
		}
		time.Sleep(50 * time.Millisecond)

		// Call the tool
		callJSON := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"reportClassification","arguments":` + toolArgs + `}}` + "\n"
		_, err = conn.Write([]byte(callJSON))
		if err != nil {
			t.Fatalf("failed to write tool call: %v", err)
		}

	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for connection info")
	}

	// Wait for the JS to complete
	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("test timed out waiting for JS completion")
	}
}

func TestMCPCallback_WaitFor_ProgressCallback(t *testing.T) {
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	loadModules(t, p)

	runOnLoop(t, p, func() {
		vm := p.Runtime()
		_, err := vm.RunString(`
			var srv = mcpMod.createServer('test', '1.0.0');
			var cb = mcpCbMod.MCPCallback({ server: srv });
			cb.addTool('testTool', 'test');
			cb.initSync();

			var progressCalled = 0;
			var result = cb.waitFor('testTool', 500, {
				onProgress: function(elapsed, total) {
					progressCalled++;
					if (typeof elapsed !== 'number') throw new Error('elapsed should be number');
					if (typeof total !== 'number') throw new Error('total should be number');
				},
				checkIntervalMs: 100
			});
			// Should timeout, but progress should have been called
			if (!result.error) throw new Error('expected timeout');
			if (progressCalled === 0) throw new Error('expected onProgress calls, got 0');

			cb.closeSync();
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
}
