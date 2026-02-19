package claudemux

import (
	"context"
	"io"

	"github.com/dop251/goja"
)

// Require returns a module loader for `osm:claudemux` that exposes the
// PTY output parser and provider registry to JavaScript scripts.
func Require(ctx context.Context) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// Event type constants matching Go EventType values.
		_ = exports.Set("EVENT_TEXT", int(EventText))
		_ = exports.Set("EVENT_RATE_LIMIT", int(EventRateLimit))
		_ = exports.Set("EVENT_PERMISSION", int(EventPermission))
		_ = exports.Set("EVENT_MODEL_SELECT", int(EventModelSelect))
		_ = exports.Set("EVENT_SSO_LOGIN", int(EventSSOLogin))
		_ = exports.Set("EVENT_COMPLETION", int(EventCompletion))
		_ = exports.Set("EVENT_TOOL_USE", int(EventToolUse))
		_ = exports.Set("EVENT_ERROR", int(EventError))
		_ = exports.Set("EVENT_THINKING", int(EventThinking))

		// newParser(): creates a new Parser and returns a wrapped JS object.
		_ = exports.Set("newParser", func(call goja.FunctionCall) goja.Value {
			p := NewParser()
			return wrapParser(runtime, p)
		})

		// eventTypeName(type: number): string — returns the string name for an event type.
		_ = exports.Set("eventTypeName", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("eventTypeName: type argument is required"))
			}
			t := EventType(call.Argument(0).ToInteger())
			return runtime.ToValue(EventTypeName(t))
		})

		// newRegistry(): creates a new provider Registry.
		_ = exports.Set("newRegistry", func(call goja.FunctionCall) goja.Value {
			r := NewRegistry()
			return wrapRegistry(runtime, r, ctx)
		})

		// claudeCode(opts?): creates a ClaudeCodeProvider.
		_ = exports.Set("claudeCode", func(call goja.FunctionCall) goja.Value {
			p := &ClaudeCodeProvider{}
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				opts := call.Argument(0).ToObject(runtime)
				if v := opts.Get("command"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					p.Command = v.String()
				}
			}
			return wrapProvider(runtime, p)
		})

		// Keystroke constants for TUI navigation.
		_ = exports.Set("KEY_ARROW_UP", KeyArrowUp)
		_ = exports.Set("KEY_ARROW_DOWN", KeyArrowDown)
		_ = exports.Set("KEY_ENTER", KeyEnter)

		// parseModelMenu(lines: string[]): { models: string[], selectedIndex: number }
		// Parses model selection TUI output into a structured ModelMenu.
		_ = exports.Set("parseModelMenu", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("parseModelMenu: lines argument is required"))
			}
			var lines []string
			if err := runtime.ExportTo(call.Argument(0), &lines); err != nil {
				panic(runtime.NewTypeError("parseModelMenu: lines must be an array of strings"))
			}
			menu := ParseModelMenu(lines)
			return modelMenuToJS(runtime, menu)
		})

		// navigateToModel(menu: object, target: string): string
		// Returns keystroke sequence to navigate to the target model.
		_ = exports.Set("navigateToModel", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				panic(runtime.NewTypeError("navigateToModel: menu and target arguments are required"))
			}
			menu := jsToModelMenu(runtime, call.Argument(0))
			target := call.Argument(1).String()
			keys, err := NavigateToModel(menu, target)
			if err != nil {
				panic(runtime.NewGoError(err))
			}
			return runtime.ToValue(keys)
		})
	}
}

// wrapParser creates a JS object wrapping a *Parser with methods.
func wrapParser(runtime *goja.Runtime, p *Parser) goja.Value {
	obj := runtime.NewObject()

	// parse(line: string): { type: number, line: string, fields: object, pattern: string }
	_ = obj.Set("parse", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("parse: line argument is required"))
		}
		line := call.Argument(0).String()
		ev := p.Parse(line)
		return eventToJS(runtime, ev)
	})

	// addPattern(name: string, pattern: string, eventType: number): void
	_ = obj.Set("addPattern", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(runtime.NewTypeError("addPattern: name, pattern, and eventType arguments are required"))
		}
		name := call.Argument(0).String()
		pattern := call.Argument(1).String()
		eventType := EventType(call.Argument(2).ToInteger())
		if err := p.AddPattern(name, pattern, eventType); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	return obj
}

// eventToJS converts an OutputEvent to a JS object.
func eventToJS(runtime *goja.Runtime, ev OutputEvent) goja.Value {
	result := runtime.NewObject()
	_ = result.Set("type", int(ev.Type))
	_ = result.Set("line", ev.Line)
	_ = result.Set("pattern", ev.Pattern)

	if ev.Fields != nil {
		fields := runtime.NewObject()
		for k, v := range ev.Fields {
			_ = fields.Set(k, v)
		}
		_ = result.Set("fields", fields)
	} else {
		_ = result.Set("fields", runtime.NewObject())
	}

	return result
}

// wrapRegistry creates a JS object wrapping a *Registry with methods.
func wrapRegistry(runtime *goja.Runtime, r *Registry, ctx context.Context) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("register", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("register: provider argument is required"))
		}
		prov := unwrapProvider(runtime, call.Argument(0))
		if err := r.Register(prov); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("get: name argument is required"))
		}
		name := call.Argument(0).String()
		p, err := r.Get(name)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return wrapProvider(runtime, p)
	})

	_ = obj.Set("list", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(r.List())
	})

	_ = obj.Set("spawn", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("spawn: provider name argument is required"))
		}
		name := call.Argument(0).String()
		opts := SpawnOpts{}
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			parseSpawnOpts(runtime, call.Argument(1).ToObject(runtime), &opts)
		}
		handle, err := r.Spawn(ctx, name, opts)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return wrapAgentHandle(runtime, handle)
	})

	return obj
}

// wrapProvider creates a JS object wrapping a Provider with methods.
func wrapProvider(runtime *goja.Runtime, p Provider) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("name", func() goja.Value { return runtime.ToValue(p.Name()) })
	_ = obj.Set("capabilities", func() goja.Value {
		caps := p.Capabilities()
		result := runtime.NewObject()
		_ = result.Set("mcp", caps.MCP)
		_ = result.Set("streaming", caps.Streaming)
		_ = result.Set("multiTurn", caps.MultiTurn)
		return result
	})
	// Store the Go provider for later use by registry.register().
	_ = obj.Set("_goProvider", p)
	return obj
}

// unwrapProvider extracts a Go Provider from a wrapped JS object.
func unwrapProvider(runtime *goja.Runtime, val goja.Value) Provider {
	obj := val.ToObject(runtime)
	goP := obj.Get("_goProvider")
	if goP == nil || goja.IsUndefined(goP) || goja.IsNull(goP) {
		panic(runtime.NewTypeError("register: argument is not a valid provider"))
	}
	p, ok := goP.Export().(Provider)
	if !ok {
		panic(runtime.NewTypeError("register: argument is not a valid provider"))
	}
	return p
}

// wrapAgentHandle creates a JS object wrapping an AgentHandle with methods.
func wrapAgentHandle(runtime *goja.Runtime, h AgentHandle) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("send", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("send: input argument is required"))
		}
		if err := h.Send(call.Argument(0).String()); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("receive", func(call goja.FunctionCall) goja.Value {
		data, err := h.Receive()
		if err != nil {
			if err == io.EOF {
				return runtime.ToValue("")
			}
			if data != "" {
				return runtime.ToValue(data)
			}
			return runtime.ToValue("")
		}
		return runtime.ToValue(data)
	})

	_ = obj.Set("close", func(call goja.FunctionCall) goja.Value {
		if err := h.Close(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	_ = obj.Set("isAlive", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(h.IsAlive())
	})

	_ = obj.Set("wait", func(call goja.FunctionCall) goja.Value {
		code, err := h.Wait()
		result := map[string]interface{}{"code": code, "error": nil}
		if err != nil {
			result["error"] = err.Error()
		}
		return runtime.ToValue(result)
	})

	return obj
}

// parseSpawnOpts extracts SpawnOpts fields from a JS options object.
func parseSpawnOpts(runtime *goja.Runtime, obj *goja.Object, opts *SpawnOpts) {
	if v := obj.Get("model"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts.Model = v.String()
	}
	if v := obj.Get("dir"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts.Dir = v.String()
	}
	if v := obj.Get("rows"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts.Rows = uint16(v.ToInteger())
	}
	if v := obj.Get("cols"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts.Cols = uint16(v.ToInteger())
	}
	if v := obj.Get("env"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		envMap := make(map[string]string)
		if err := runtime.ExportTo(v, &envMap); err != nil {
			panic(runtime.NewTypeError("spawn: env must be a {string: string} object"))
		}
		opts.Env = envMap
	}
	if v := obj.Get("args"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		var args []string
		if err := runtime.ExportTo(v, &args); err != nil {
			panic(runtime.NewTypeError("spawn: args must be an array of strings"))
		}
		opts.Args = args
	}
}

// modelMenuToJS converts a *ModelMenu to a JS object.
func modelMenuToJS(runtime *goja.Runtime, menu *ModelMenu) goja.Value {
	obj := runtime.NewObject()
	models := make([]interface{}, len(menu.Models))
	for i, m := range menu.Models {
		models[i] = m
	}
	_ = obj.Set("models", runtime.ToValue(models))
	_ = obj.Set("selectedIndex", menu.SelectedIndex)
	return obj
}

// jsToModelMenu converts a JS object back to a *ModelMenu.
func jsToModelMenu(runtime *goja.Runtime, val goja.Value) *ModelMenu {
	if goja.IsUndefined(val) || goja.IsNull(val) {
		panic(runtime.NewTypeError("navigateToModel: menu argument must be an object"))
	}
	obj := val.ToObject(runtime)
	menu := &ModelMenu{SelectedIndex: -1}

	if v := obj.Get("models"); v != nil && !goja.IsUndefined(v) {
		var models []string
		if err := runtime.ExportTo(v, &models); err != nil {
			panic(runtime.NewTypeError("navigateToModel: menu.models must be an array of strings"))
		}
		menu.Models = models
	}
	if v := obj.Get("selectedIndex"); v != nil && !goja.IsUndefined(v) {
		menu.SelectedIndex = int(v.ToInteger())
	}
	return menu
}
