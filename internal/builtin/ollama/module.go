package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// Require returns a module loader for osm:ollama.
//
// The module provides a Promise-based API for interacting with an Ollama server,
// including tool-calling support and an agentic execution loop.
//
// Usage:
//
//	const ollama = require('osm:ollama');
//	const client = ollama.createClient();           // default localhost:11434
//	const client = ollama.createClient('http://host:11434');
//	const ver = await client.version();
//	const resp = await client.chat({model: 'llama3.2', messages: [{role: 'user', content: 'hi'}]});
//
//	// Tool registry + agentic loop
//	const reg = ollama.createToolRegistry();
//	ollama.registerBuiltinTools(reg, '/path/to/workdir');
//	const agent = ollama.createAgent({client, model: 'llama3.2', tools: reg, systemPrompt: '...'});
//	const result = await agent.run('What files are here?');
func Require(ctx context.Context, adapter *gojaeventloop.Adapter) require.ModuleLoader {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		_ = exports.Set("createClient", jsCreateClient(ctx, runtime, adapter))
		_ = exports.Set("createToolRegistry", jsCreateToolRegistry(runtime))
		_ = exports.Set("registerBuiltinTools", jsRegisterBuiltinTools(runtime))
		_ = exports.Set("createAgent", jsCreateAgent(ctx, runtime, adapter))
		_ = exports.Set("formatToolCallSummary", func(call goja.FunctionCall) goja.Value {
			name := call.Argument(0).String()
			var args map[string]interface{}
			if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
				if m, ok := call.Argument(1).Export().(map[string]interface{}); ok {
					args = m
				}
			}
			return runtime.ToValue(FormatToolCallSummary(name, args))
		})
	}
}

// jsCreateClient returns: createClient(baseURL?, options?) → ClientObject
//
// Options:
//
//	timeout - request timeout in seconds (number)
func jsCreateClient(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		var client *Client
		var opts []ClientOption

		// Apply a default HTTP client with reasonable timeout for JS callers.
		opts = append(opts, WithHTTPClient(&http.Client{Timeout: 30 * time.Second}))

		// Parse options from second argument.
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			if optMap, ok := call.Argument(1).Export().(map[string]interface{}); ok {
				if t, ok := optMap["timeout"]; ok {
					switch v := t.(type) {
					case int64:
						opts = append(opts, WithTimeout(time.Duration(v)*time.Second))
					case float64:
						opts = append(opts, WithTimeout(time.Duration(v*float64(time.Second))))
					}
				}
			}
		}

		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			baseURL := call.Argument(0).String()
			c, err := NewClient(baseURL, opts...)
			if err != nil {
				panic(runtime.NewGoError(err))
			}
			client = c
		} else {
			client = DefaultClient()
		}
		return wrapClientJS(ctx, runtime, adapter, client)
	}
}

// wrapClientJS creates a JS object with all async client methods.
func wrapClientJS(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter, client *Client) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("_goClient", client)

	_ = obj.Set("version", func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := adapter.JS().NewChainedPromise()
		go func() {
			ver, err := client.Version(ctx)
			if err != nil {
				reject(err)
				return
			}
			if e := adapter.Loop().Submit(func() {
				resolve(runtime.ToValue(map[string]interface{}{"version": ver.Version}))
			}); e != nil {
				reject(fmt.Errorf("event loop stopped"))
			}
		}()
		return adapter.GojaWrapPromise(promise)
	})

	_ = obj.Set("listModels", func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := adapter.JS().NewChainedPromise()
		go func() {
			models, err := client.ListModels(ctx)
			if err != nil {
				reject(err)
				return
			}
			if e := adapter.Loop().Submit(func() {
				var out []interface{}
				for _, m := range models {
					out = append(out, map[string]interface{}{
						"name": m.Name, "model": m.Model, "size": m.Size,
						"digest": m.Digest, "modifiedAt": m.ModifiedAt.String(),
					})
				}
				resolve(runtime.ToValue(out))
			}); e != nil {
				reject(fmt.Errorf("event loop stopped"))
			}
		}()
		return adapter.GojaWrapPromise(promise)
	})

	_ = obj.Set("showModel", func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		promise, resolve, reject := adapter.JS().NewChainedPromise()
		go func() {
			info, err := client.ShowModel(ctx, name)
			if err != nil {
				reject(err)
				return
			}
			if e := adapter.Loop().Submit(func() {
				resolve(modelInfoToJS(runtime, info))
			}); e != nil {
				reject(fmt.Errorf("event loop stopped"))
			}
		}()
		return adapter.GojaWrapPromise(promise)
	})

	_ = obj.Set("listRunning", func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := adapter.JS().NewChainedPromise()
		go func() {
			running, err := client.ListRunning(ctx)
			if err != nil {
				reject(err)
				return
			}
			if e := adapter.Loop().Submit(func() {
				var out []interface{}
				for _, r := range running {
					out = append(out, map[string]interface{}{
						"name": r.Name, "model": r.Model, "size": r.Size, "digest": r.Digest,
					})
				}
				resolve(runtime.ToValue(out))
			}); e != nil {
				reject(fmt.Errorf("event loop stopped"))
			}
		}()
		return adapter.GojaWrapPromise(promise)
	})

	_ = obj.Set("health", func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := adapter.JS().NewChainedPromise()
		go func() {
			if err := client.Health(ctx); err != nil {
				reject(err)
				return
			}
			if e := adapter.Loop().Submit(func() {
				resolve(goja.Undefined())
			}); e != nil {
				reject(fmt.Errorf("event loop stopped"))
			}
		}()
		return adapter.GojaWrapPromise(promise)
	})

	_ = obj.Set("isHealthy", func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := adapter.JS().NewChainedPromise()
		go func() {
			healthy := client.IsHealthy(ctx)
			if e := adapter.Loop().Submit(func() {
				resolve(runtime.ToValue(healthy))
			}); e != nil {
				reject(fmt.Errorf("event loop stopped"))
			}
		}()
		return adapter.GojaWrapPromise(promise)
	})

	_ = obj.Set("chat", func(call goja.FunctionCall) goja.Value {
		req := jsParseChatRequest(runtime, call)
		promise, resolve, reject := adapter.JS().NewChainedPromise()
		go func() {
			resp, err := client.Chat(ctx, req)
			if err != nil {
				reject(err)
				return
			}
			if e := adapter.Loop().Submit(func() {
				resolve(chatResponseToJS(runtime, resp))
			}); e != nil {
				reject(fmt.Errorf("event loop stopped"))
			}
		}()
		return adapter.GojaWrapPromise(promise)
	})

	return obj
}

// modelInfoToJS converts ModelInfo to JS, using HasCapability and SupportsTools.
func modelInfoToJS(runtime *goja.Runtime, info ModelInfo) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("modelfile", info.Modelfile)
	_ = obj.Set("template", info.Template)
	_ = obj.Set("parameters", info.Parameters)
	_ = obj.Set("capabilities", info.Capabilities)
	_ = obj.Set("supportsTools", info.SupportsTools())
	_ = obj.Set("hasCapability", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(info.HasCapability(call.Argument(0).String()))
	})
	return obj
}

// chatResponseToJS converts ChatResponse to JS.
func chatResponseToJS(runtime *goja.Runtime, resp ChatResponse) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("model", resp.Model)
	_ = obj.Set("done", resp.Done)
	msg := runtime.NewObject()
	_ = msg.Set("role", resp.Message.Role)
	_ = msg.Set("content", resp.Message.Content)
	if len(resp.Message.ToolCalls) > 0 {
		var calls []interface{}
		for _, tc := range resp.Message.ToolCalls {
			calls = append(calls, map[string]interface{}{
				"function": map[string]interface{}{
					"name": tc.Function.Name, "arguments": tc.Function.Arguments,
				},
			})
		}
		_ = msg.Set("toolCalls", calls)
	}
	_ = obj.Set("message", msg)
	if resp.TotalDuration > 0 {
		_ = obj.Set("totalDuration", resp.TotalDuration)
	}
	if resp.EvalCount > 0 {
		_ = obj.Set("evalCount", resp.EvalCount)
	}
	return obj
}

// jsParseChatRequest converts a JS request object to ChatRequest.
func jsParseChatRequest(runtime *goja.Runtime, call goja.FunctionCall) ChatRequest {
	if len(call.Arguments) == 0 {
		panic(runtime.NewTypeError("chat requires a request object"))
	}
	arg := call.Argument(0)
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		panic(runtime.NewTypeError("chat request must not be null"))
	}
	opts, ok := arg.Export().(map[string]interface{})
	if !ok {
		panic(runtime.NewTypeError("chat request must be an object"))
	}
	req := ChatRequest{Stream: BoolPtr(false)}
	if model, ok := opts["model"].(string); ok {
		req.Model = model
	}
	if msgs, ok := opts["messages"]; ok {
		if slice, ok := msgs.([]interface{}); ok {
			for _, m := range slice {
				if mObj, ok := m.(map[string]interface{}); ok {
					msg := Message{}
					if r, ok := mObj["role"].(string); ok {
						msg.Role = r
					}
					if c, ok := mObj["content"].(string); ok {
						msg.Content = c
					}
					req.Messages = append(req.Messages, msg)
				}
			}
		}
	}
	if tools, ok := opts["tools"]; ok {
		data, err := json.Marshal(tools)
		if err == nil {
			var ollamaTools []Tool
			if json.Unmarshal(data, &ollamaTools) == nil {
				req.Tools = ollamaTools
			}
		}
	}
	if options, ok := opts["options"]; ok {
		if optMap, ok := options.(map[string]interface{}); ok {
			req.Options = optMap
		}
	}
	return req
}

// jsCreateToolRegistry returns: createToolRegistry() → ToolRegistryObject
func jsCreateToolRegistry(runtime *goja.Runtime) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		return wrapToolRegistryJS(runtime, NewToolRegistry())
	}
}

func wrapToolRegistryJS(runtime *goja.Runtime, reg *ToolRegistry) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("_goRegistry", reg)

	_ = obj.Set("register", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("register requires a tool definition"))
		}
		defObj, ok := call.Argument(0).(*goja.Object)
		if !ok {
			panic(runtime.NewTypeError("tool definition must be an object"))
		}
		name := defObj.Get("name").String()
		var desc string
		if d := defObj.Get("description"); d != nil && !goja.IsUndefined(d) {
			desc = d.String()
		}
		var params json.RawMessage
		if p := defObj.Get("parameters"); p != nil && !goja.IsUndefined(p) {
			data, _ := json.Marshal(p.Export())
			params = data
		}
		handlerVal := defObj.Get("handler")
		if handlerVal == nil || goja.IsUndefined(handlerVal) {
			panic(runtime.NewTypeError("tool must have a handler function"))
		}
		handlerFn, ok := goja.AssertFunction(handlerVal)
		if !ok {
			panic(runtime.NewTypeError("handler must be a function"))
		}
		if err := reg.Register(ToolDef{
			Name: name, Description: desc, Parameters: params,
			Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
				result, callErr := handlerFn(goja.Undefined(), runtime.ToValue(args))
				if callErr != nil {
					return "", callErr
				}
				return result.String(), nil
			},
		}); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("names", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(reg.Names())
	})
	_ = obj.Set("len", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(reg.Len())
	})
	_ = obj.Set("ollamaTools", func(call goja.FunctionCall) goja.Value {
		tools := reg.OllamaTools()
		data, _ := json.Marshal(tools)
		var result interface{}
		json.Unmarshal(data, &result)
		return runtime.ToValue(result)
	})
	return obj
}

// jsRegisterBuiltinTools returns: registerBuiltinTools(registry, workDir) → void
func jsRegisterBuiltinTools(runtime *goja.Runtime) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(runtime.NewTypeError("registerBuiltinTools requires (registry, workDir)"))
		}
		regObj, ok := call.Argument(0).(*goja.Object)
		if !ok {
			panic(runtime.NewTypeError("first argument must be a tool registry"))
		}
		goVal := regObj.Get("_goRegistry")
		if goVal == nil || goja.IsUndefined(goVal) {
			panic(runtime.NewTypeError("invalid registry object"))
		}
		reg, ok := goVal.Export().(*ToolRegistry)
		if !ok {
			panic(runtime.NewTypeError("not a ToolRegistry"))
		}
		if err := RegisterBuiltinTools(reg, call.Argument(1).String()); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	}
}

// jsCreateAgent returns: createAgent(config) → AgentObject
func jsCreateAgent(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("createAgent requires a config object"))
		}
		cfgObj, ok := call.Argument(0).(*goja.Object)
		if !ok {
			panic(runtime.NewTypeError("config must be an object"))
		}

		// Extract client.
		clientVal := cfgObj.Get("client")
		if clientVal == nil || goja.IsUndefined(clientVal) {
			panic(runtime.NewTypeError("config.client is required"))
		}
		cObj, ok := clientVal.(*goja.Object)
		if !ok {
			panic(runtime.NewTypeError("config.client must be a client object"))
		}
		client, ok := cObj.Get("_goClient").Export().(*Client)
		if !ok {
			panic(runtime.NewTypeError("invalid client"))
		}

		// Extract tools registry.
		toolsVal := cfgObj.Get("tools")
		if toolsVal == nil || goja.IsUndefined(toolsVal) {
			panic(runtime.NewTypeError("config.tools is required"))
		}
		tObj, ok := toolsVal.(*goja.Object)
		if !ok {
			panic(runtime.NewTypeError("config.tools must be a registry object"))
		}
		reg, ok := tObj.Get("_goRegistry").Export().(*ToolRegistry)
		if !ok {
			panic(runtime.NewTypeError("invalid tool registry"))
		}

		config := AgentConfig{
			Client: client,
			Model:  cfgObj.Get("model").String(),
			Tools:  reg,
		}
		if sp := cfgObj.Get("systemPrompt"); sp != nil && !goja.IsUndefined(sp) {
			config.SystemPrompt = sp.String()
		}
		if mt := cfgObj.Get("maxTurns"); mt != nil && !goja.IsUndefined(mt) {
			config.MaxTurns = int(mt.ToInteger())
		}
		if opts := cfgObj.Get("options"); opts != nil && !goja.IsUndefined(opts) {
			if m, ok := opts.Export().(map[string]interface{}); ok {
				config.Options = m
			}
		}

		runner, err := NewAgenticRunner(config)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return wrapAgentJS(ctx, runtime, adapter, runner)
	}
}

func wrapAgentJS(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter, runner *AgenticRunner) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("run", func(call goja.FunctionCall) goja.Value {
		message := call.Argument(0).String()
		promise, resolve, reject := adapter.JS().NewChainedPromise()
		go func() {
			result, err := runner.Run(ctx, message)
			if err != nil {
				reject(err)
				return
			}
			if e := adapter.Loop().Submit(func() {
				resolve(agentResultToJS(runtime, result))
			}); e != nil {
				reject(fmt.Errorf("event loop stopped"))
			}
		}()
		return adapter.GojaWrapPromise(promise)
	})
	return obj
}

func agentResultToJS(runtime *goja.Runtime, result *AgentResult) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("finalContent", result.FinalContent)
	_ = obj.Set("turnsUsed", result.TurnsUsed)
	_ = obj.Set("toolCallCount", result.ToolCallCount)
	var msgs []interface{}
	for _, m := range result.Messages {
		entry := map[string]interface{}{"role": m.Role, "content": m.Content}
		if len(m.ToolCalls) > 0 {
			var calls []interface{}
			for _, tc := range m.ToolCalls {
				calls = append(calls, map[string]interface{}{
					"function": map[string]interface{}{
						"name": tc.Function.Name, "arguments": tc.Function.Arguments,
					},
				})
			}
			entry["toolCalls"] = calls
		}
		msgs = append(msgs, entry)
	}
	_ = obj.Set("messages", msgs)
	return obj
}
