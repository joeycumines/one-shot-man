package claudemux

import (
	"context"
	"io"
	"time"

	"github.com/dop251/goja"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

		// newMCPInstance(sessionId: string): object
		// Creates a per-instance MCP server config for spawning a Claude Code
		// instance with a dedicated MCP endpoint.
		_ = exports.Set("newMCPInstance", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("newMCPInstance: sessionId argument is required"))
			}
			sessionID := call.Argument(0).String()
			cfg, err := NewMCPInstanceConfig(sessionID)
			if err != nil {
				panic(runtime.NewGoError(err))
			}
			return wrapMCPInstance(runtime, cfg)
		})

		// newInstanceRegistry(baseDir: string): object
		// Creates an instance registry for managing isolated Claude Code instances.
		_ = exports.Set("newInstanceRegistry", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("newInstanceRegistry: baseDir argument is required"))
			}
			baseDir := call.Argument(0).String()
			reg, err := NewInstanceRegistry(baseDir)
			if err != nil {
				panic(runtime.NewGoError(err))
			}
			return wrapInstanceRegistry(runtime, reg)
		})

		// Guard action constants.
		_ = exports.Set("GUARD_ACTION_NONE", int(GuardActionNone))
		_ = exports.Set("GUARD_ACTION_PAUSE", int(GuardActionPause))
		_ = exports.Set("GUARD_ACTION_REJECT", int(GuardActionReject))
		_ = exports.Set("GUARD_ACTION_RESTART", int(GuardActionRestart))
		_ = exports.Set("GUARD_ACTION_ESCALATE", int(GuardActionEscalate))
		_ = exports.Set("GUARD_ACTION_TIMEOUT", int(GuardActionTimeout))

		// Permission policy constants.
		_ = exports.Set("PERMISSION_POLICY_DENY", int(PermissionPolicyDeny))
		_ = exports.Set("PERMISSION_POLICY_ALLOW", int(PermissionPolicyAllow))

		// guardActionName(action: number): string
		_ = exports.Set("guardActionName", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("guardActionName: action argument is required"))
			}
			a := GuardAction(call.Argument(0).ToInteger())
			return runtime.ToValue(GuardActionName(a))
		})

		// defaultGuardConfig(): object — returns the default production guard config.
		_ = exports.Set("defaultGuardConfig", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultGuardConfig()
			return guardConfigToJS(runtime, cfg)
		})

		// newGuard(config?: object): object — creates a guard monitor.
		_ = exports.Set("newGuard", func(call goja.FunctionCall) goja.Value {
			var cfg GuardConfig
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				cfg = jsToGuardConfig(runtime, call.Argument(0))
			}
			g := NewGuard(cfg)
			return wrapGuard(runtime, g)
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

// wrapMCPInstance creates a JS object wrapping an *MCPInstanceConfig.
func wrapMCPInstance(runtime *goja.Runtime, cfg *MCPInstanceConfig) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("sessionId", cfg.SessionID)

	// listenAndServe(): void — starts the MCP HTTP server on a unique endpoint.
	// Creates a minimal MCP server instance. Call before writeConfigFile.
	_ = obj.Set("listenAndServe", func() goja.Value {
		server := mcp.NewServer(&mcp.Implementation{
			Name:    "osm-claudemux",
			Version: "0.0.0",
		}, nil)
		if err := cfg.ListenAndServe(server); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// endpoint(): string — returns the MCP endpoint URL (empty if not listening).
	_ = obj.Set("endpoint", func() goja.Value {
		return runtime.ToValue(cfg.Endpoint())
	})

	// listenerAddr(): string — returns the raw network address, or empty string.
	_ = obj.Set("listenerAddr", func() goja.Value {
		addr := cfg.ListenerAddr()
		if addr == nil {
			return runtime.ToValue("")
		}
		return runtime.ToValue(addr.String())
	})

	// configPath(): string — returns the config file path.
	_ = obj.Set("configPath", func() goja.Value {
		return runtime.ToValue(cfg.ConfigPath())
	})

	// spawnArgs(): string[] — returns CLI args for Claude Code.
	_ = obj.Set("spawnArgs", func() goja.Value {
		return runtime.ToValue(cfg.SpawnArgs())
	})

	// writeConfigFile(): void — generates the MCP config JSON file.
	_ = obj.Set("writeConfigFile", func() goja.Value {
		if err := cfg.WriteConfigFile(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// validate(): void — checks config is usable before spawn.
	_ = obj.Set("validate", func() goja.Value {
		if err := cfg.Validate(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// close(): void — stops listener, removes temp files.
	_ = obj.Set("close", func() goja.Value {
		if err := cfg.Close(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	return obj
}

// wrapInstanceRegistry creates a JS object wrapping an *InstanceRegistry.
func wrapInstanceRegistry(runtime *goja.Runtime, reg *InstanceRegistry) goja.Value {
	obj := runtime.NewObject()

	// create(sessionId: string): object — creates a new isolated instance.
	_ = obj.Set("create", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("create: sessionId argument is required"))
		}
		sessionID := call.Argument(0).String()
		inst, err := reg.Create(sessionID)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return wrapInstance(runtime, inst)
	})

	// get(sessionId: string): object|null — retrieves an instance.
	_ = obj.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("get: sessionId argument is required"))
		}
		inst, ok := reg.Get(call.Argument(0).String())
		if !ok {
			return goja.Null()
		}
		return wrapInstance(runtime, inst)
	})

	// close(sessionId: string): void — closes and deregisters an instance.
	_ = obj.Set("close", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("close: sessionId argument is required"))
		}
		if err := reg.Close(call.Argument(0).String()); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// closeAll(): void — closes all instances.
	_ = obj.Set("closeAll", func() goja.Value {
		if err := reg.CloseAll(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// list(): string[] — returns active session IDs.
	_ = obj.Set("list", func() goja.Value {
		return runtime.ToValue(reg.List())
	})

	// len(): number — returns count of active instances.
	_ = obj.Set("len", func() goja.Value {
		return runtime.ToValue(reg.Len())
	})

	// baseDir(): string — returns the base directory.
	_ = obj.Set("baseDir", func() goja.Value {
		return runtime.ToValue(reg.BaseDir())
	})

	return obj
}

// wrapInstance creates a JS object wrapping an *Instance.
func wrapInstance(runtime *goja.Runtime, inst *Instance) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("id", inst.ID)
	_ = obj.Set("stateDir", inst.StateDir)
	_ = obj.Set("createdAt", inst.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))

	// isClosed(): boolean
	_ = obj.Set("isClosed", func() goja.Value {
		return runtime.ToValue(inst.IsClosed())
	})

	// close(): void — releases all instance resources.
	_ = obj.Set("close", func() goja.Value {
		if err := inst.Close(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	return obj
}

// guardConfigToJS converts a GuardConfig to a JS object.
func guardConfigToJS(runtime *goja.Runtime, cfg GuardConfig) goja.Value {
	obj := runtime.NewObject()

	rl := runtime.NewObject()
	_ = rl.Set("enabled", cfg.RateLimit.Enabled)
	_ = rl.Set("initialDelayMs", cfg.RateLimit.InitialDelay.Milliseconds())
	_ = rl.Set("maxDelayMs", cfg.RateLimit.MaxDelay.Milliseconds())
	_ = rl.Set("multiplier", cfg.RateLimit.Multiplier)
	_ = rl.Set("resetAfterMs", cfg.RateLimit.ResetAfter.Milliseconds())
	_ = obj.Set("rateLimit", rl)

	perm := runtime.NewObject()
	_ = perm.Set("enabled", cfg.Permission.Enabled)
	_ = perm.Set("policy", int(cfg.Permission.Policy))
	_ = obj.Set("permission", perm)

	crash := runtime.NewObject()
	_ = crash.Set("enabled", cfg.Crash.Enabled)
	_ = crash.Set("maxRestarts", cfg.Crash.MaxRestarts)
	_ = obj.Set("crash", crash)

	timeout := runtime.NewObject()
	_ = timeout.Set("enabled", cfg.OutputTimeout.Enabled)
	_ = timeout.Set("timeoutMs", cfg.OutputTimeout.Timeout.Milliseconds())
	_ = obj.Set("outputTimeout", timeout)

	return obj
}

// jsToGuardConfig converts a JS object to a GuardConfig.
func jsToGuardConfig(runtime *goja.Runtime, val goja.Value) GuardConfig {
	obj := val.ToObject(runtime)
	var cfg GuardConfig

	if v := obj.Get("rateLimit"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		rl := v.ToObject(runtime)
		if b := rl.Get("enabled"); b != nil && !goja.IsUndefined(b) {
			cfg.RateLimit.Enabled = b.ToBoolean()
		}
		if d := rl.Get("initialDelayMs"); d != nil && !goja.IsUndefined(d) {
			cfg.RateLimit.InitialDelay = time.Duration(d.ToInteger()) * time.Millisecond
		}
		if d := rl.Get("maxDelayMs"); d != nil && !goja.IsUndefined(d) {
			cfg.RateLimit.MaxDelay = time.Duration(d.ToInteger()) * time.Millisecond
		}
		if m := rl.Get("multiplier"); m != nil && !goja.IsUndefined(m) {
			cfg.RateLimit.Multiplier = m.ToFloat()
		}
		if d := rl.Get("resetAfterMs"); d != nil && !goja.IsUndefined(d) {
			cfg.RateLimit.ResetAfter = time.Duration(d.ToInteger()) * time.Millisecond
		}
	}

	if v := obj.Get("permission"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		perm := v.ToObject(runtime)
		if b := perm.Get("enabled"); b != nil && !goja.IsUndefined(b) {
			cfg.Permission.Enabled = b.ToBoolean()
		}
		if p := perm.Get("policy"); p != nil && !goja.IsUndefined(p) {
			cfg.Permission.Policy = PermissionPolicy(p.ToInteger())
		}
	}

	if v := obj.Get("crash"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		cr := v.ToObject(runtime)
		if b := cr.Get("enabled"); b != nil && !goja.IsUndefined(b) {
			cfg.Crash.Enabled = b.ToBoolean()
		}
		if m := cr.Get("maxRestarts"); m != nil && !goja.IsUndefined(m) {
			cfg.Crash.MaxRestarts = int(m.ToInteger())
		}
	}

	if v := obj.Get("outputTimeout"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		ot := v.ToObject(runtime)
		if b := ot.Get("enabled"); b != nil && !goja.IsUndefined(b) {
			cfg.OutputTimeout.Enabled = b.ToBoolean()
		}
		if d := ot.Get("timeoutMs"); d != nil && !goja.IsUndefined(d) {
			cfg.OutputTimeout.Timeout = time.Duration(d.ToInteger()) * time.Millisecond
		}
	}

	return cfg
}

// guardEventToJS converts a *GuardEvent to a JS object, or null if nil.
func guardEventToJS(runtime *goja.Runtime, ge *GuardEvent) goja.Value {
	if ge == nil {
		return goja.Null()
	}
	obj := runtime.NewObject()
	_ = obj.Set("action", int(ge.Action))
	_ = obj.Set("actionName", GuardActionName(ge.Action))
	_ = obj.Set("reason", ge.Reason)
	if ge.Details != nil {
		details := runtime.NewObject()
		for k, v := range ge.Details {
			_ = details.Set(k, v)
		}
		_ = obj.Set("details", details)
	} else {
		_ = obj.Set("details", runtime.NewObject())
	}
	return obj
}

// wrapGuard creates a JS object wrapping a *Guard with methods.
func wrapGuard(runtime *goja.Runtime, g *Guard) goja.Value {
	obj := runtime.NewObject()

	// processEvent(event: object, nowMs?: number): object|null
	// event must have {type: number, line: string, fields?: object, pattern?: string}
	_ = obj.Set("processEvent", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("processEvent: event argument is required"))
		}
		evObj := call.Argument(0).ToObject(runtime)
		var ev OutputEvent
		if t := evObj.Get("type"); t != nil && !goja.IsUndefined(t) {
			ev.Type = EventType(t.ToInteger())
		}
		if l := evObj.Get("line"); l != nil && !goja.IsUndefined(l) {
			ev.Line = l.String()
		}
		if p := evObj.Get("pattern"); p != nil && !goja.IsUndefined(p) {
			ev.Pattern = p.String()
		}
		if f := evObj.Get("fields"); f != nil && !goja.IsUndefined(f) && !goja.IsNull(f) {
			fields := make(map[string]string)
			if err := runtime.ExportTo(f, &fields); err == nil {
				ev.Fields = fields
			}
		}

		now := time.Now()
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			nowMs := call.Argument(1).ToInteger()
			now = time.UnixMilli(nowMs)
		}

		ge := g.ProcessEvent(ev, now)
		return guardEventToJS(runtime, ge)
	})

	// processCrash(exitCode: number, nowMs?: number): object|null
	_ = obj.Set("processCrash", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("processCrash: exitCode argument is required"))
		}
		exitCode := int(call.Argument(0).ToInteger())
		now := time.Now()
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			nowMs := call.Argument(1).ToInteger()
			now = time.UnixMilli(nowMs)
		}
		ge := g.ProcessCrash(exitCode, now)
		return guardEventToJS(runtime, ge)
	})

	// checkTimeout(nowMs?: number): object|null
	_ = obj.Set("checkTimeout", func(call goja.FunctionCall) goja.Value {
		now := time.Now()
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			nowMs := call.Argument(0).ToInteger()
			now = time.UnixMilli(nowMs)
		}
		ge := g.CheckTimeout(now)
		return guardEventToJS(runtime, ge)
	})

	// resetCrashCount(): void
	_ = obj.Set("resetCrashCount", func() goja.Value {
		g.ResetCrashCount()
		return goja.Undefined()
	})

	// state(): object
	_ = obj.Set("state", func() goja.Value {
		st := g.State()
		obj := runtime.NewObject()
		_ = obj.Set("rateLimitCount", st.RateLimitCount)
		_ = obj.Set("currentDelayMs", st.CurrentDelay.Milliseconds())
		_ = obj.Set("crashCount", st.CrashCount)
		_ = obj.Set("started", st.Started)
		if !st.LastEventTime.IsZero() {
			_ = obj.Set("lastEventTimeMs", st.LastEventTime.UnixMilli())
		}
		if !st.LastRateLimitTime.IsZero() {
			_ = obj.Set("lastRateLimitTimeMs", st.LastRateLimitTime.UnixMilli())
		}
		return obj
	})

	// config(): object
	_ = obj.Set("config", func() goja.Value {
		return guardConfigToJS(runtime, g.Config())
	})

	return obj
}
