package claudemux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

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

		// ollama(opts?): creates an OllamaProvider.
		_ = exports.Set("ollama", func(call goja.FunctionCall) goja.Value {
			p := &OllamaProvider{}
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				opts := call.Argument(0).ToObject(runtime)
				if v := opts.Get("command"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					p.Command = v.String()
				}
				if v := opts.Get("subArgs"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					var subArgs []string
					if err := runtime.ExportTo(v, &subArgs); err != nil {
						panic(runtime.NewTypeError("ollama: subArgs must be an array of strings"))
					}
					p.SubArgs = subArgs
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

		// defaultMCPGuardConfig(): object — returns the default MCP guard config.
		_ = exports.Set("defaultMCPGuardConfig", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultMCPGuardConfig()
			return mcpGuardConfigToJS(runtime, cfg)
		})

		// newMCPGuard(config?: object): object — creates an MCP guard monitor.
		_ = exports.Set("newMCPGuard", func(call goja.FunctionCall) goja.Value {
			var cfg MCPGuardConfig
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				cfg = jsToMCPGuardConfig(runtime, call.Argument(0))
			}
			g := NewMCPGuard(cfg)
			return wrapMCPGuard(runtime, g)
		})

		// Supervisor state constants.
		_ = exports.Set("SUPERVISOR_IDLE", int(SupervisorIdle))
		_ = exports.Set("SUPERVISOR_RUNNING", int(SupervisorRunning))
		_ = exports.Set("SUPERVISOR_RECOVERING", int(SupervisorRecovering))
		_ = exports.Set("SUPERVISOR_DRAINING", int(SupervisorDraining))
		_ = exports.Set("SUPERVISOR_STOPPED", int(SupervisorStopped))

		// Error class constants.
		_ = exports.Set("ERROR_CLASS_NONE", int(ErrorClassNone))
		_ = exports.Set("ERROR_CLASS_PTY_EOF", int(ErrorClassPTYEOF))
		_ = exports.Set("ERROR_CLASS_PTY_CRASH", int(ErrorClassPTYCrash))
		_ = exports.Set("ERROR_CLASS_PTY_ERROR", int(ErrorClassPTYError))
		_ = exports.Set("ERROR_CLASS_MCP_TIMEOUT", int(ErrorClassMCPTimeout))
		_ = exports.Set("ERROR_CLASS_MCP_MALFORMED", int(ErrorClassMCPMalformed))
		_ = exports.Set("ERROR_CLASS_CANCELLED", int(ErrorClassCancelled))

		// Recovery action constants.
		_ = exports.Set("RECOVERY_NONE", int(RecoveryNone))
		_ = exports.Set("RECOVERY_RETRY", int(RecoveryRetry))
		_ = exports.Set("RECOVERY_RESTART", int(RecoveryRestart))
		_ = exports.Set("RECOVERY_FORCE_KILL", int(RecoveryForceKill))
		_ = exports.Set("RECOVERY_ESCALATE", int(RecoveryEscalate))
		_ = exports.Set("RECOVERY_ABORT", int(RecoveryAbort))
		_ = exports.Set("RECOVERY_DRAIN", int(RecoveryDrain))

		// supervisorStateName(state: number): string
		_ = exports.Set("supervisorStateName", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("supervisorStateName: state argument is required"))
			}
			return runtime.ToValue(SupervisorStateName(SupervisorState(call.Argument(0).ToInteger())))
		})

		// errorClassName(class: number): string
		_ = exports.Set("errorClassName", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("errorClassName: class argument is required"))
			}
			return runtime.ToValue(ErrorClassName(ErrorClass(call.Argument(0).ToInteger())))
		})

		// recoveryActionName(action: number): string
		_ = exports.Set("recoveryActionName", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("recoveryActionName: action argument is required"))
			}
			return runtime.ToValue(RecoveryActionName(RecoveryAction(call.Argument(0).ToInteger())))
		})

		// defaultSupervisorConfig(): object
		_ = exports.Set("defaultSupervisorConfig", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultSupervisorConfig()
			return supervisorConfigToJS(runtime, cfg)
		})

		// newSupervisor(config?: object): object
		_ = exports.Set("newSupervisor", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultSupervisorConfig()
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				cfg = jsToSupervisorConfig(runtime, call.Argument(0))
			}
			s := NewSupervisor(ctx, cfg)
			return wrapSupervisor(ctx, runtime, s)
		})

		// defaultPoolConfig(): object
		_ = exports.Set("defaultPoolConfig", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultPoolConfig()
			return poolConfigToJS(runtime, cfg)
		})

		// newPool(config?: object): object
		_ = exports.Set("newPool", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultPoolConfig()
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				cfg = jsToPoolConfig(runtime, call.Argument(0))
			}
			p := NewPool(cfg)
			return wrapPool(runtime, p)
		})

		// defaultPanelConfig(): object
		_ = exports.Set("defaultPanelConfig", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultPanelConfig()
			return panelConfigToJS(runtime, cfg)
		})

		// newPanel(config?: object): object
		_ = exports.Set("newPanel", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultPanelConfig()
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				cfg = jsToPanelConfig(runtime, call.Argument(0))
			}
			panel := NewPanel(cfg)
			return wrapPanel(runtime, panel)
		})

		// Managed session state constants.
		_ = exports.Set("SESSION_IDLE", int(SessionIdle))
		_ = exports.Set("SESSION_ACTIVE", int(SessionActive))
		_ = exports.Set("SESSION_PAUSED", int(SessionPaused))
		_ = exports.Set("SESSION_FAILED", int(SessionFailed))
		_ = exports.Set("SESSION_CLOSED", int(SessionClosed))

		// managedSessionStateName(state: number): string
		_ = exports.Set("managedSessionStateName", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("managedSessionStateName: state argument is required"))
			}
			return runtime.ToValue(ManagedSessionStateName(ManagedSessionState(call.Argument(0).ToInteger())))
		})

		// defaultManagedSessionConfig(): object
		_ = exports.Set("defaultManagedSessionConfig", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultManagedSessionConfig()
			return managedSessionConfigToJS(runtime, cfg)
		})

		// createSession(id: string, config?: object): object
		_ = exports.Set("createSession", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("createSession: id argument is required"))
			}
			id := call.Argument(0).String()
			cfg := DefaultManagedSessionConfig()
			if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
				cfg = jsToManagedSessionConfig(runtime, call.Argument(1))
			}
			s := NewManagedSession(ctx, id, cfg)
			return wrapManagedSession(runtime, s)
		})

		// --- Safety Validation ---

		// Intent constants.
		_ = exports.Set("INTENT_UNKNOWN", int(IntentUnknown))
		_ = exports.Set("INTENT_READ_ONLY", int(IntentReadOnly))
		_ = exports.Set("INTENT_CODE", int(IntentCode))
		_ = exports.Set("INTENT_DESTRUCTIVE", int(IntentDestructive))
		_ = exports.Set("INTENT_NETWORK", int(IntentNetwork))
		_ = exports.Set("INTENT_CREDENTIAL", int(IntentCredential))

		// Scope constants.
		_ = exports.Set("SCOPE_UNKNOWN", int(ScopeUnknown))
		_ = exports.Set("SCOPE_FILE", int(ScopeFile))
		_ = exports.Set("SCOPE_REPO", int(ScopeRepo))
		_ = exports.Set("SCOPE_INFRA", int(ScopeInfra))

		// Risk level constants.
		_ = exports.Set("RISK_NONE", int(RiskNone))
		_ = exports.Set("RISK_LOW", int(RiskLow))
		_ = exports.Set("RISK_MEDIUM", int(RiskMedium))
		_ = exports.Set("RISK_HIGH", int(RiskHigh))
		_ = exports.Set("RISK_CRITICAL", int(RiskCritical))

		// Policy action constants.
		_ = exports.Set("POLICY_ALLOW", int(PolicyAllow))
		_ = exports.Set("POLICY_WARN", int(PolicyWarn))
		_ = exports.Set("POLICY_CONFIRM", int(PolicyConfirm))
		_ = exports.Set("POLICY_BLOCK", int(PolicyBlock))

		// intentName(intent: number): string
		_ = exports.Set("intentName", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("intentName: intent argument is required"))
			}
			return runtime.ToValue(IntentName(Intent(call.Argument(0).ToInteger())))
		})

		// scopeName(scope: number): string
		_ = exports.Set("scopeName", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("scopeName: scope argument is required"))
			}
			return runtime.ToValue(ScopeName(Scope(call.Argument(0).ToInteger())))
		})

		// riskLevelName(level: number): string
		_ = exports.Set("riskLevelName", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("riskLevelName: level argument is required"))
			}
			return runtime.ToValue(RiskLevelName(RiskLevel(call.Argument(0).ToInteger())))
		})

		// policyActionName(action: number): string
		_ = exports.Set("policyActionName", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("policyActionName: action argument is required"))
			}
			return runtime.ToValue(PolicyActionName(PolicyAction(call.Argument(0).ToInteger())))
		})

		// defaultSafetyConfig(): object
		_ = exports.Set("defaultSafetyConfig", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultSafetyConfig()
			return safetyConfigToJS(runtime, cfg)
		})

		// newSafetyValidator(config?: object): object
		_ = exports.Set("newSafetyValidator", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultSafetyConfig()
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				cfg = jsToSafetyConfig(runtime, call.Argument(0))
			}
			sv := NewSafetyValidator(cfg)
			return wrapSafetyValidator(runtime, sv)
		})

		// newCompositeValidator(validators: object[]): object
		_ = exports.Set("newCompositeValidator", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("newCompositeValidator: validators argument is required"))
			}
			arr := call.Argument(0).ToObject(runtime)
			length := int(arr.Get("length").ToInteger())
			validators := make([]Validator, 0, length)
			for i := 0; i < length; i++ {
				item := arr.Get(fmt.Sprintf("%d", i)).ToObject(runtime)
				v := item.Get("__goValidator")
				if v == nil || goja.IsUndefined(v) {
					panic(runtime.NewTypeError("newCompositeValidator: each element must be a validator"))
				}
				val, ok := v.Export().(Validator)
				if !ok {
					panic(runtime.NewTypeError("newCompositeValidator: each element must be a validator"))
				}
				validators = append(validators, val)
			}
			cv := NewCompositeValidator(validators...)
			return wrapCompositeValidator(runtime, cv)
		})

		// --- Choice Resolution ---

		// defaultChoiceConfig(): object
		_ = exports.Set("defaultChoiceConfig", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultChoiceConfig()
			return choiceConfigToJS(runtime, cfg)
		})

		// newChoiceResolver(config?: object): object
		_ = exports.Set("newChoiceResolver", func(call goja.FunctionCall) goja.Value {
			cfg := DefaultChoiceConfig()
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				cfg = jsToChoiceConfig(runtime, call.Argument(0))
			}
			cr := NewChoiceResolver(cfg)
			return wrapChoiceResolver(runtime, cr)
		})

		// --- Result File Readers ---
		// These read structured JSON result files written by MCP tools
		// (reportClassification, reportSplitPlan) via the --result-dir channel.

		// readClassificationResult(dir: string): object
		// Returns a map of file paths to category names.
		_ = exports.Set("readClassificationResult", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("readClassificationResult: dir argument is required"))
			}
			dir := call.Argument(0).String()
			result, err := ReadClassificationResult(dir)
			if err != nil {
				panic(runtime.NewGoError(err))
			}
			obj := runtime.NewObject()
			for k, v := range result {
				_ = obj.Set(k, v)
			}
			return obj
		})

		// readSplitPlanResult(dir: string): object[]
		// Returns an array of split plan stages.
		_ = exports.Set("readSplitPlanResult", func(call goja.FunctionCall) goja.Value {
			dir := call.Argument(0).String()
			result, err := ReadSplitPlanResult(dir)
			if err != nil {
				panic(runtime.NewGoError(err))
			}
			arr := runtime.NewArray()
			for i, stage := range result {
				obj := runtime.NewObject()
				_ = obj.Set("name", stage.Name)
				files := runtime.NewArray()
				for j, f := range stage.Files {
					_ = files.Set(fmt.Sprintf("%d", j), f)
				}
				_ = obj.Set("files", files)
				_ = obj.Set("message", stage.Message)
				_ = obj.Set("order", stage.Order)
				_ = arr.Set(fmt.Sprintf("%d", i), obj)
			}
			return arr
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

	// patterns(): { name: string, eventType: number, pattern: string }[]
	_ = obj.Set("patterns", func(call goja.FunctionCall) goja.Value {
		infos := p.Patterns()
		arr := runtime.NewArray()
		for i, info := range infos {
			item := runtime.NewObject()
			_ = item.Set("name", info.Name)
			_ = item.Set("eventType", int(info.EventType))
			_ = item.Set("eventTypeName", EventTypeName(info.EventType))
			_ = item.Set("pattern", info.Pattern)
			_ = arr.Set(fmt.Sprintf("%d", i), item)
		}
		return arr
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
			if errors.Is(err, io.EOF) {
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

	// osmBinary(): string — returns the path to the osm binary used in config.
	_ = obj.Set("osmBinary", func() goja.Value {
		return runtime.ToValue(cfg.OsmBinary)
	})

	// configDir(): string — returns the temp directory path.
	_ = obj.Set("configDir", func() goja.Value {
		return runtime.ToValue(cfg.configDir)
	})

	// configPath(): string — returns the config file path.
	_ = obj.Set("configPath", func() goja.Value {
		return runtime.ToValue(cfg.ConfigPath())
	})

	// resultDir(): string — returns the result directory path (may be empty).
	_ = obj.Set("resultDir", func() goja.Value {
		return runtime.ToValue(cfg.ResultDir)
	})

	// setResultDir(dir: string): void — sets the result directory for MCP tools.
	_ = obj.Set("setResultDir", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("setResultDir: dir argument is required"))
		}
		cfg.ResultDir = call.Argument(0).String()
		return goja.Undefined()
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
		_ = obj.Set("timeoutFired", st.TimeoutFired)
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

// mcpGuardConfigToJS converts an MCPGuardConfig to a JS object.
func mcpGuardConfigToJS(runtime *goja.Runtime, cfg MCPGuardConfig) goja.Value {
	obj := runtime.NewObject()

	nct := runtime.NewObject()
	_ = nct.Set("enabled", cfg.NoCallTimeout.Enabled)
	_ = nct.Set("timeoutMs", cfg.NoCallTimeout.Timeout.Milliseconds())
	_ = obj.Set("noCallTimeout", nct)

	fl := runtime.NewObject()
	_ = fl.Set("enabled", cfg.FrequencyLimit.Enabled)
	_ = fl.Set("windowMs", cfg.FrequencyLimit.Window.Milliseconds())
	_ = fl.Set("maxCalls", cfg.FrequencyLimit.MaxCalls)
	_ = obj.Set("frequencyLimit", fl)

	rd := runtime.NewObject()
	_ = rd.Set("enabled", cfg.RepeatDetection.Enabled)
	_ = rd.Set("maxRepeats", cfg.RepeatDetection.MaxRepeats)
	_ = rd.Set("windowSize", cfg.RepeatDetection.WindowSize)
	_ = rd.Set("matchTool", cfg.RepeatDetection.MatchTool)
	_ = rd.Set("matchArgHash", cfg.RepeatDetection.MatchArgHash)
	_ = obj.Set("repeatDetection", rd)

	al := runtime.NewObject()
	_ = al.Set("enabled", cfg.ToolAllowlist.Enabled)
	if cfg.ToolAllowlist.AllowedTools != nil {
		tools := make([]interface{}, len(cfg.ToolAllowlist.AllowedTools))
		for i, t := range cfg.ToolAllowlist.AllowedTools {
			tools[i] = t
		}
		_ = al.Set("allowedTools", runtime.ToValue(tools))
	}
	_ = obj.Set("toolAllowlist", al)

	return obj
}

// jsToMCPGuardConfig converts a JS object to an MCPGuardConfig.
func jsToMCPGuardConfig(runtime *goja.Runtime, val goja.Value) MCPGuardConfig {
	obj := val.ToObject(runtime)
	var cfg MCPGuardConfig

	if v := obj.Get("noCallTimeout"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		nct := v.ToObject(runtime)
		if b := nct.Get("enabled"); b != nil && !goja.IsUndefined(b) {
			cfg.NoCallTimeout.Enabled = b.ToBoolean()
		}
		if d := nct.Get("timeoutMs"); d != nil && !goja.IsUndefined(d) {
			cfg.NoCallTimeout.Timeout = time.Duration(d.ToInteger()) * time.Millisecond
		}
	}

	if v := obj.Get("frequencyLimit"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		fl := v.ToObject(runtime)
		if b := fl.Get("enabled"); b != nil && !goja.IsUndefined(b) {
			cfg.FrequencyLimit.Enabled = b.ToBoolean()
		}
		if d := fl.Get("windowMs"); d != nil && !goja.IsUndefined(d) {
			cfg.FrequencyLimit.Window = time.Duration(d.ToInteger()) * time.Millisecond
		}
		if m := fl.Get("maxCalls"); m != nil && !goja.IsUndefined(m) {
			cfg.FrequencyLimit.MaxCalls = int(m.ToInteger())
		}
	}

	if v := obj.Get("repeatDetection"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		rd := v.ToObject(runtime)
		if b := rd.Get("enabled"); b != nil && !goja.IsUndefined(b) {
			cfg.RepeatDetection.Enabled = b.ToBoolean()
		}
		if m := rd.Get("maxRepeats"); m != nil && !goja.IsUndefined(m) {
			cfg.RepeatDetection.MaxRepeats = int(m.ToInteger())
		}
		if w := rd.Get("windowSize"); w != nil && !goja.IsUndefined(w) {
			cfg.RepeatDetection.WindowSize = int(w.ToInteger())
		}
		if b := rd.Get("matchTool"); b != nil && !goja.IsUndefined(b) {
			cfg.RepeatDetection.MatchTool = b.ToBoolean()
		}
		if b := rd.Get("matchArgHash"); b != nil && !goja.IsUndefined(b) {
			cfg.RepeatDetection.MatchArgHash = b.ToBoolean()
		}
	}

	if v := obj.Get("toolAllowlist"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		al := v.ToObject(runtime)
		if b := al.Get("enabled"); b != nil && !goja.IsUndefined(b) {
			cfg.ToolAllowlist.Enabled = b.ToBoolean()
		}
		if t := al.Get("allowedTools"); t != nil && !goja.IsUndefined(t) && !goja.IsNull(t) {
			var tools []string
			if err := runtime.ExportTo(t, &tools); err == nil {
				cfg.ToolAllowlist.AllowedTools = tools
			}
		}
	}

	return cfg
}

// wrapMCPGuard creates a JS object wrapping an *MCPGuard with methods.
func wrapMCPGuard(runtime *goja.Runtime, g *MCPGuard) goja.Value {
	obj := runtime.NewObject()

	// processToolCall(call: object): object|null
	// call: { toolName: string, arguments?: string, timestampMs?: number }
	_ = obj.Set("processToolCall", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("processToolCall: call argument is required"))
		}
		callObj := call.Argument(0).ToObject(runtime)
		tc := MCPToolCall{Timestamp: time.Now()}

		if v := callObj.Get("toolName"); v != nil && !goja.IsUndefined(v) {
			tc.ToolName = v.String()
		}
		if v := callObj.Get("arguments"); v != nil && !goja.IsUndefined(v) {
			tc.Arguments = v.String()
		}
		if v := callObj.Get("timestampMs"); v != nil && !goja.IsUndefined(v) {
			tc.Timestamp = time.UnixMilli(v.ToInteger())
		}

		ge := g.ProcessToolCall(tc)
		return guardEventToJS(runtime, ge)
	})

	// checkNoCallTimeout(nowMs?: number): object|null
	_ = obj.Set("checkNoCallTimeout", func(call goja.FunctionCall) goja.Value {
		now := time.Now()
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			now = time.UnixMilli(call.Argument(0).ToInteger())
		}
		ge := g.CheckNoCallTimeout(now)
		return guardEventToJS(runtime, ge)
	})

	// state(): object
	_ = obj.Set("state", func() goja.Value {
		st := g.State()
		obj := runtime.NewObject()
		_ = obj.Set("totalCalls", st.TotalCalls)
		_ = obj.Set("recentCount", st.RecentCount)
		_ = obj.Set("started", st.Started)
		_ = obj.Set("noCallTimeoutFired", st.NoCallTimeoutFired)
		if !st.LastCallTime.IsZero() {
			_ = obj.Set("lastCallTimeMs", st.LastCallTime.UnixMilli())
		}
		return obj
	})

	// config(): object
	_ = obj.Set("config", func() goja.Value {
		return mcpGuardConfigToJS(runtime, g.Config())
	})

	return obj
}

// supervisorConfigToJS converts a SupervisorConfig to a JS object.
func supervisorConfigToJS(runtime *goja.Runtime, cfg SupervisorConfig) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("maxRetries", cfg.MaxRetries)
	_ = obj.Set("maxForceKills", cfg.MaxForceKills)
	_ = obj.Set("retryDelayMs", cfg.RetryDelay.Milliseconds())
	_ = obj.Set("shutdownTimeoutMs", cfg.ShutdownTimeout.Milliseconds())
	_ = obj.Set("forceKillTimeoutMs", cfg.ForceKillTimeout.Milliseconds())
	return obj
}

// jsToSupervisorConfig converts a JS object to a SupervisorConfig.
func jsToSupervisorConfig(runtime *goja.Runtime, val goja.Value) SupervisorConfig {
	obj := val.ToObject(runtime)
	cfg := DefaultSupervisorConfig()

	if v := obj.Get("maxRetries"); v != nil && !goja.IsUndefined(v) {
		cfg.MaxRetries = int(v.ToInteger())
	}
	if v := obj.Get("maxForceKills"); v != nil && !goja.IsUndefined(v) {
		cfg.MaxForceKills = int(v.ToInteger())
	}
	if v := obj.Get("retryDelayMs"); v != nil && !goja.IsUndefined(v) {
		cfg.RetryDelay = time.Duration(v.ToInteger()) * time.Millisecond
	}
	if v := obj.Get("shutdownTimeoutMs"); v != nil && !goja.IsUndefined(v) {
		cfg.ShutdownTimeout = time.Duration(v.ToInteger()) * time.Millisecond
	}
	if v := obj.Get("forceKillTimeoutMs"); v != nil && !goja.IsUndefined(v) {
		cfg.ForceKillTimeout = time.Duration(v.ToInteger()) * time.Millisecond
	}

	return cfg
}

// recoveryDecisionToJS converts a RecoveryDecision to a JS object.
func recoveryDecisionToJS(runtime *goja.Runtime, d RecoveryDecision) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("action", int(d.Action))
	_ = obj.Set("actionName", RecoveryActionName(d.Action))
	_ = obj.Set("reason", d.Reason)
	_ = obj.Set("newState", int(d.NewState))
	_ = obj.Set("newStateName", SupervisorStateName(d.NewState))
	if d.Details != nil {
		details := runtime.NewObject()
		for k, v := range d.Details {
			_ = details.Set(k, v)
		}
		_ = obj.Set("details", details)
	} else {
		_ = obj.Set("details", runtime.NewObject())
	}
	return obj
}

// wrapSupervisor creates a JS object wrapping a *Supervisor with methods.
func wrapSupervisor(ctx context.Context, runtime *goja.Runtime, s *Supervisor) goja.Value {
	obj := runtime.NewObject()

	// start(): void
	_ = obj.Set("start", func() goja.Value {
		if err := s.Start(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// handleError(msg: string, errorClass: number): object
	_ = obj.Set("handleError", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(runtime.NewTypeError("handleError: msg and errorClass arguments required"))
		}
		msg := call.Argument(0).String()
		class := ErrorClass(call.Argument(1).ToInteger())
		d := s.HandleError(msg, class)
		return recoveryDecisionToJS(runtime, d)
	})

	// shutdown(): object
	_ = obj.Set("shutdown", func() goja.Value {
		d := s.Shutdown()
		return recoveryDecisionToJS(runtime, d)
	})

	// confirmRecovery(): void
	_ = obj.Set("confirmRecovery", func() goja.Value {
		s.ConfirmRecovery()
		return goja.Undefined()
	})

	// confirmStopped(): void
	_ = obj.Set("confirmStopped", func() goja.Value {
		s.ConfirmStopped()
		return goja.Undefined()
	})

	// snapshot(): object
	_ = obj.Set("snapshot", func() goja.Value {
		snap := s.Snapshot()
		obj := runtime.NewObject()
		_ = obj.Set("state", int(snap.State))
		_ = obj.Set("stateName", snap.StateName)
		_ = obj.Set("retryCount", snap.RetryCount)
		_ = obj.Set("forceKillCount", snap.ForceKillCount)
		_ = obj.Set("lastError", snap.LastError)
		_ = obj.Set("lastErrorClass", int(snap.LastErrorClass))
		_ = obj.Set("cancelled", snap.Cancelled)
		return obj
	})

	// reset(): void — resets supervisor to Idle for reuse
	_ = obj.Set("reset", func() goja.Value {
		s.Reset(ctx)
		return goja.Undefined()
	})

	// cancelled(): boolean — whether the supervisor context is done
	_ = obj.Set("cancelled", func() goja.Value {
		return runtime.ToValue(s.Context().Err() != nil)
	})

	return obj
}

// poolConfigToJS converts a PoolConfig to a JS object.
func poolConfigToJS(runtime *goja.Runtime, cfg PoolConfig) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("maxSize", cfg.MaxSize)
	return obj
}

// jsToPoolConfig converts a JS object to a PoolConfig.
func jsToPoolConfig(runtime *goja.Runtime, val goja.Value) PoolConfig {
	obj := val.ToObject(runtime)
	cfg := DefaultPoolConfig()

	if v := obj.Get("maxSize"); v != nil && !goja.IsUndefined(v) {
		cfg.MaxSize = int(v.ToInteger())
	}

	return cfg
}

// workerStatsToJS converts a WorkerStats to a JS object.
func workerStatsToJS(runtime *goja.Runtime, ws WorkerStats) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("id", ws.ID)
	_ = obj.Set("state", int(ws.State))
	_ = obj.Set("stateName", ws.StateName)
	_ = obj.Set("taskCount", ws.TaskCount)
	_ = obj.Set("errorCount", ws.ErrorCount)
	if !ws.LastTaskAt.IsZero() {
		_ = obj.Set("lastTaskAt", ws.LastTaskAt.UnixMilli())
	}
	return obj
}

// poolStatsToJS converts a PoolStats to a JS object.
func poolStatsToJS(runtime *goja.Runtime, stats PoolStats) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("state", int(stats.State))
	_ = obj.Set("stateName", stats.StateName)
	_ = obj.Set("workerCount", stats.WorkerCount)
	_ = obj.Set("maxSize", stats.MaxSize)
	_ = obj.Set("inflight", stats.Inflight)

	workers := runtime.NewArray()
	for i, ws := range stats.Workers {
		_ = workers.Set(fmt.Sprintf("%d", i), workerStatsToJS(runtime, ws))
	}
	_ = obj.Set("workers", workers)
	return obj
}

// wrapPoolWorker creates a JS object wrapping a *PoolWorker.
func wrapPoolWorker(runtime *goja.Runtime, w *PoolWorker) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("id", w.ID)
	_ = obj.Set("state", func() goja.Value {
		return runtime.ToValue(int(w.State))
	})
	_ = obj.Set("stateName", func() goja.Value {
		return runtime.ToValue(WorkerStateName(w.State))
	})
	_ = obj.Set("taskCount", func() goja.Value {
		return runtime.ToValue(w.TaskCount)
	})
	_ = obj.Set("errorCount", func() goja.Value {
		return runtime.ToValue(w.ErrorCount)
	})
	return obj
}

// wrapPool creates a JS object wrapping a *Pool with methods.
func wrapPool(runtime *goja.Runtime, p *Pool) goja.Value {
	obj := runtime.NewObject()

	// start(): void
	_ = obj.Set("start", func() goja.Value {
		if err := p.Start(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// addWorker(id: string, inst?: object): object
	_ = obj.Set("addWorker", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("addWorker: id argument required"))
		}
		id := call.Argument(0).String()
		// Instance is optional — nil is valid for testing.
		w, err := p.AddWorker(id, nil)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return wrapPoolWorker(runtime, w)
	})

	// removeWorker(id: string): void
	_ = obj.Set("removeWorker", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("removeWorker: id argument required"))
		}
		id := call.Argument(0).String()
		if _, err := p.RemoveWorker(id); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// acquire(): object — blocks until worker available
	_ = obj.Set("acquire", func() goja.Value {
		w, err := p.Acquire()
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return wrapPoolWorker(runtime, w)
	})

	// tryAcquire(): object | null — non-blocking
	_ = obj.Set("tryAcquire", func() goja.Value {
		w, err := p.TryAcquire()
		if err != nil {
			return goja.Null()
		}
		return wrapPoolWorker(runtime, w)
	})

	// release(worker: object, error?: string): void
	_ = obj.Set("release", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("release: worker argument required"))
		}
		workerObj := call.Argument(0).ToObject(runtime)
		idVal := workerObj.Get("id")
		if idVal == nil || goja.IsUndefined(idVal) {
			panic(runtime.NewTypeError("release: worker must have id property"))
		}

		target := p.FindWorker(idVal.String())
		if target == nil {
			panic(runtime.NewTypeError("release: worker not found in pool"))
		}

		var taskErr error
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			taskErr = errors.New(call.Argument(1).String())
		}
		p.Release(target, taskErr, time.Now())
		return goja.Undefined()
	})

	// drain(): void
	_ = obj.Set("drain", func() goja.Value {
		p.Drain()
		return goja.Undefined()
	})

	// waitDrained(): void — blocks until all in-flight tasks are released
	_ = obj.Set("waitDrained", func() goja.Value {
		p.WaitDrained()
		return goja.Undefined()
	})

	// close(): object[] — returns closed workers
	_ = obj.Set("close", func() goja.Value {
		workers := p.Close()
		arr := runtime.NewArray()
		for i, w := range workers {
			_ = arr.Set(fmt.Sprintf("%d", i), wrapPoolWorker(runtime, w))
		}
		return arr
	})

	// stats(): object
	_ = obj.Set("stats", func() goja.Value {
		return poolStatsToJS(runtime, p.Stats())
	})

	// config(): object
	_ = obj.Set("config", func() goja.Value {
		return poolConfigToJS(runtime, p.Config())
	})

	return obj
}

// panelConfigToJS converts a PanelConfig to a JS object.
func panelConfigToJS(runtime *goja.Runtime, cfg PanelConfig) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("maxPanes", cfg.MaxPanes)
	_ = obj.Set("scrollbackSize", cfg.ScrollbackSize)
	return obj
}

// jsToPanelConfig converts a JS object to a PanelConfig.
func jsToPanelConfig(runtime *goja.Runtime, val goja.Value) PanelConfig {
	obj := val.ToObject(runtime)
	cfg := DefaultPanelConfig()

	if v := obj.Get("maxPanes"); v != nil && !goja.IsUndefined(v) {
		cfg.MaxPanes = int(v.ToInteger())
	}
	if v := obj.Get("scrollbackSize"); v != nil && !goja.IsUndefined(v) {
		cfg.ScrollbackSize = int(v.ToInteger())
	}

	return cfg
}

// inputResultToJS converts an InputResult to a JS object.
func inputResultToJS(runtime *goja.Runtime, r InputResult) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("targetPaneID", r.TargetPaneID)
	_ = obj.Set("consumed", r.Consumed)
	_ = obj.Set("action", r.Action)
	return obj
}

// paneSnapshotToJS converts a PaneSnapshot to a JS object.
func paneSnapshotToJS(runtime *goja.Runtime, ps PaneSnapshot) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("id", ps.ID)
	_ = obj.Set("title", ps.Title)
	_ = obj.Set("lines", ps.Lines)
	_ = obj.Set("scrollPos", ps.ScrollPos)
	_ = obj.Set("isActive", ps.IsActive)

	health := runtime.NewObject()
	_ = health.Set("state", ps.Health.State)
	_ = health.Set("errorCount", ps.Health.ErrorCount)
	_ = health.Set("taskCount", ps.Health.TaskCount)
	if !ps.Health.LastUpdate.IsZero() {
		_ = health.Set("lastUpdate", ps.Health.LastUpdate.UnixMilli())
	}
	_ = obj.Set("health", health)
	return obj
}

// panelSnapshotToJS converts a PanelSnapshot to a JS object.
func panelSnapshotToJS(runtime *goja.Runtime, snap PanelSnapshot) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("state", int(snap.State))
	_ = obj.Set("stateName", snap.StateName)
	_ = obj.Set("activeIdx", snap.ActiveIdx)
	_ = obj.Set("statusBar", snap.StatusBar)

	panes := runtime.NewArray()
	for i, ps := range snap.Panes {
		_ = panes.Set(fmt.Sprintf("%d", i), paneSnapshotToJS(runtime, ps))
	}
	_ = obj.Set("panes", panes)
	return obj
}

// wrapPanel creates a JS object wrapping a *Panel with methods.
func wrapPanel(runtime *goja.Runtime, panel *Panel) goja.Value {
	obj := runtime.NewObject()

	// start(): void
	_ = obj.Set("start", func() goja.Value {
		if err := panel.Start(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// addPane(id: string, title: string): number — returns index
	_ = obj.Set("addPane", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(runtime.NewTypeError("addPane: id and title arguments required"))
		}
		id := call.Argument(0).String()
		title := call.Argument(1).String()
		idx, err := panel.AddPane(id, title)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return runtime.ToValue(idx)
	})

	// removePane(id: string): void
	_ = obj.Set("removePane", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("removePane: id argument required"))
		}
		if err := panel.RemovePane(call.Argument(0).String()); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// routeInput(key: string): object
	_ = obj.Set("routeInput", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("routeInput: key argument required"))
		}
		result := panel.RouteInput(call.Argument(0).String())
		return inputResultToJS(runtime, result)
	})

	// setActive(index: number): void
	_ = obj.Set("setActive", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("setActive: index argument required"))
		}
		if err := panel.SetActive(int(call.Argument(0).ToInteger())); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// activeIndex(): number
	_ = obj.Set("activeIndex", func() goja.Value {
		return runtime.ToValue(panel.ActiveIndex())
	})

	// activePane(): object | null
	_ = obj.Set("activePane", func() goja.Value {
		pane := panel.ActivePane()
		if pane == nil {
			return goja.Null()
		}
		obj := runtime.NewObject()
		_ = obj.Set("id", pane.ID)
		_ = obj.Set("title", pane.Title)
		_ = obj.Set("scrollPos", pane.ScrollPos)
		_ = obj.Set("lines", len(pane.Scrollback))
		health := runtime.NewObject()
		_ = health.Set("state", pane.Health.State)
		_ = health.Set("errorCount", pane.Health.ErrorCount)
		_ = health.Set("taskCount", pane.Health.TaskCount)
		_ = obj.Set("health", health)
		return obj
	})

	// paneCount(): number
	_ = obj.Set("paneCount", func() goja.Value {
		return runtime.ToValue(panel.PaneCount())
	})

	// appendOutput(paneID: string, line: string): void
	_ = obj.Set("appendOutput", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(runtime.NewTypeError("appendOutput: paneID and line arguments required"))
		}
		if err := panel.AppendOutput(call.Argument(0).String(), call.Argument(1).String()); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// updateHealth(paneID: string, health: object): void
	_ = obj.Set("updateHealth", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(runtime.NewTypeError("updateHealth: paneID and health arguments required"))
		}
		paneID := call.Argument(0).String()
		hObj := call.Argument(1).ToObject(runtime)

		health := PaneHealth{}
		if v := hObj.Get("state"); v != nil && !goja.IsUndefined(v) {
			health.State = v.String()
		}
		if v := hObj.Get("errorCount"); v != nil && !goja.IsUndefined(v) {
			health.ErrorCount = v.ToInteger()
		}
		if v := hObj.Get("taskCount"); v != nil && !goja.IsUndefined(v) {
			health.TaskCount = v.ToInteger()
		}

		if err := panel.UpdateHealth(paneID, health); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// statusBar(): string
	_ = obj.Set("statusBar", func() goja.Value {
		return runtime.ToValue(panel.StatusBar())
	})

	// getVisibleLines(paneID: string, height: number): string[]
	_ = obj.Set("getVisibleLines", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(runtime.NewTypeError("getVisibleLines: paneID and height arguments required"))
		}
		lines, err := panel.GetVisibleLines(
			call.Argument(0).String(),
			int(call.Argument(1).ToInteger()),
		)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		arr := runtime.NewArray()
		for i, line := range lines {
			_ = arr.Set(fmt.Sprintf("%d", i), line)
		}
		return arr
	})

	// snapshot(): object
	_ = obj.Set("snapshot", func() goja.Value {
		return panelSnapshotToJS(runtime, panel.Snapshot())
	})

	// close(): void
	_ = obj.Set("close", func() goja.Value {
		panel.Close()
		return goja.Undefined()
	})

	// config(): object
	_ = obj.Set("config", func() goja.Value {
		return panelConfigToJS(runtime, panel.Config())
	})

	return obj
}

// managedSessionConfigToJS converts a ManagedSessionConfig to a JS object.
func managedSessionConfigToJS(runtime *goja.Runtime, cfg ManagedSessionConfig) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("guard", guardConfigToJS(runtime, cfg.Guard))
	_ = obj.Set("mcpGuard", mcpGuardConfigToJS(runtime, cfg.MCPGuard))
	_ = obj.Set("supervisor", supervisorConfigToJS(runtime, cfg.Supervisor))
	return obj
}

// jsToManagedSessionConfig converts a JS object to a ManagedSessionConfig.
func jsToManagedSessionConfig(runtime *goja.Runtime, val goja.Value) ManagedSessionConfig {
	obj := val.ToObject(runtime)
	cfg := DefaultManagedSessionConfig()

	if v := obj.Get("guard"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		cfg.Guard = jsToGuardConfig(runtime, v)
	}
	if v := obj.Get("mcpGuard"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		cfg.MCPGuard = jsToMCPGuardConfig(runtime, v)
	}
	if v := obj.Get("supervisor"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		cfg.Supervisor = jsToSupervisorConfig(runtime, v)
	}

	return cfg
}

// lineResultToJS converts a LineResult to a JS object.
func lineResultToJS(runtime *goja.Runtime, r LineResult) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("event", eventToJS(runtime, r.Event))
	_ = obj.Set("guardEvent", guardEventToJS(runtime, r.GuardEvent))
	_ = obj.Set("action", r.Action)
	return obj
}

// toolCallResultToJS converts a ToolCallResult to a JS object.
func toolCallResultToJS(runtime *goja.Runtime, r ToolCallResult) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("guardEvent", guardEventToJS(runtime, r.GuardEvent))
	_ = obj.Set("action", r.Action)
	return obj
}

// managedSessionSnapshotToJS converts a ManagedSessionSnapshot to a JS object.
func managedSessionSnapshotToJS(runtime *goja.Runtime, snap ManagedSessionSnapshot) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("id", snap.ID)
	_ = obj.Set("state", int(snap.State))
	_ = obj.Set("stateName", snap.StateName)
	_ = obj.Set("linesProcessed", snap.LinesProcessed)

	counts := runtime.NewObject()
	for k, v := range snap.EventCounts {
		_ = counts.Set(k, v)
	}
	_ = obj.Set("eventCounts", counts)

	if snap.LastEvent != nil {
		_ = obj.Set("lastEvent", eventToJS(runtime, *snap.LastEvent))
	} else {
		_ = obj.Set("lastEvent", goja.Null())
	}

	// Guard state.
	gs := runtime.NewObject()
	_ = gs.Set("rateLimitCount", snap.GuardState.RateLimitCount)
	_ = gs.Set("crashCount", snap.GuardState.CrashCount)
	_ = gs.Set("started", snap.GuardState.Started)
	_ = obj.Set("guardState", gs)

	// MCP guard state.
	ms := runtime.NewObject()
	_ = ms.Set("totalCalls", snap.MCPGuardState.TotalCalls)
	_ = ms.Set("recentCount", snap.MCPGuardState.RecentCount)
	_ = ms.Set("started", snap.MCPGuardState.Started)
	_ = obj.Set("mcpGuardState", ms)

	// Supervisor state.
	ss := runtime.NewObject()
	_ = ss.Set("state", int(snap.SupervisorState.State))
	_ = ss.Set("stateName", snap.SupervisorState.StateName)
	_ = ss.Set("retryCount", snap.SupervisorState.RetryCount)
	_ = ss.Set("forceKillCount", snap.SupervisorState.ForceKillCount)
	_ = ss.Set("lastError", snap.SupervisorState.LastError)
	_ = ss.Set("cancelled", snap.SupervisorState.Cancelled)
	_ = obj.Set("supervisorState", ss)

	return obj
}

// wrapManagedSession creates a JS object wrapping a *ManagedSession with methods.
func wrapManagedSession(runtime *goja.Runtime, s *ManagedSession) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("id", s.ID())

	// start(): void
	_ = obj.Set("start", func() goja.Value {
		if err := s.Start(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// processLine(line: string, nowMs?: number): object
	_ = obj.Set("processLine", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("processLine: line argument is required"))
		}
		line := call.Argument(0).String()
		now := time.Now()
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			now = time.UnixMilli(call.Argument(1).ToInteger())
		}
		r := s.ProcessLine(line, now)
		return lineResultToJS(runtime, r)
	})

	// processCrash(exitCode: number, nowMs?: number): object
	_ = obj.Set("processCrash", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("processCrash: exitCode argument is required"))
		}
		exitCode := int(call.Argument(0).ToInteger())
		now := time.Now()
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			now = time.UnixMilli(call.Argument(1).ToInteger())
		}
		ge, d := s.ProcessCrash(exitCode, now)
		result := runtime.NewObject()
		_ = result.Set("guardEvent", guardEventToJS(runtime, ge))
		_ = result.Set("recovery", recoveryDecisionToJS(runtime, d))
		return result
	})

	// processToolCall(call: object): object
	_ = obj.Set("processToolCall", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("processToolCall: call argument is required"))
		}
		callObj := call.Argument(0).ToObject(runtime)
		tc := MCPToolCall{Timestamp: time.Now()}
		if v := callObj.Get("toolName"); v != nil && !goja.IsUndefined(v) {
			tc.ToolName = v.String()
		}
		if v := callObj.Get("arguments"); v != nil && !goja.IsUndefined(v) {
			tc.Arguments = v.String()
		}
		if v := callObj.Get("timestampMs"); v != nil && !goja.IsUndefined(v) {
			tc.Timestamp = time.UnixMilli(v.ToInteger())
		}
		r := s.ProcessToolCall(tc)
		return toolCallResultToJS(runtime, r)
	})

	// checkTimeout(nowMs?: number): object|null
	_ = obj.Set("checkTimeout", func(call goja.FunctionCall) goja.Value {
		now := time.Now()
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			now = time.UnixMilli(call.Argument(0).ToInteger())
		}
		ge := s.CheckTimeout(now)
		return guardEventToJS(runtime, ge)
	})

	// handleError(msg: string, errorClass: number): object
	_ = obj.Set("handleError", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(runtime.NewTypeError("handleError: msg and errorClass arguments required"))
		}
		msg := call.Argument(0).String()
		class := ErrorClass(call.Argument(1).ToInteger())
		d := s.HandleError(msg, class)
		return recoveryDecisionToJS(runtime, d)
	})

	// confirmRecovery(): void
	_ = obj.Set("confirmRecovery", func() goja.Value {
		s.ConfirmRecovery()
		return goja.Undefined()
	})

	// resume(): void
	_ = obj.Set("resume", func() goja.Value {
		if err := s.Resume(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// shutdown(): object
	_ = obj.Set("shutdown", func() goja.Value {
		d := s.Shutdown()
		return recoveryDecisionToJS(runtime, d)
	})

	// close(): void
	_ = obj.Set("close", func() goja.Value {
		s.Close()
		return goja.Undefined()
	})

	// state(): number
	_ = obj.Set("state", func() goja.Value {
		return runtime.ToValue(int(s.State()))
	})

	// snapshot(): object
	_ = obj.Set("snapshot", func() goja.Value {
		return managedSessionSnapshotToJS(runtime, s.Snapshot())
	})

	// parser(): object — returns the session's parser for custom patterns
	_ = obj.Set("parser", func() goja.Value {
		return wrapParser(runtime, s.Parser())
	})

	// onEvent(callback): void — set event callback
	_ = obj.Set("onEvent", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || goja.IsNull(call.Argument(0)) || goja.IsUndefined(call.Argument(0)) {
			s.OnEvent = nil
			return goja.Undefined()
		}
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(runtime.NewTypeError("onEvent: argument must be a function"))
		}
		s.OnEvent = func(ev OutputEvent) {
			_, _ = fn(goja.Undefined(), eventToJS(runtime, ev))
		}
		return goja.Undefined()
	})

	// onGuardAction(callback): void — set guard action callback
	_ = obj.Set("onGuardAction", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || goja.IsNull(call.Argument(0)) || goja.IsUndefined(call.Argument(0)) {
			s.OnGuardAction = nil
			return goja.Undefined()
		}
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(runtime.NewTypeError("onGuardAction: argument must be a function"))
		}
		s.OnGuardAction = func(ge *GuardEvent) {
			_, _ = fn(goja.Undefined(), guardEventToJS(runtime, ge))
		}
		return goja.Undefined()
	})

	// onRecoveryDecision(callback): void — set recovery decision callback
	_ = obj.Set("onRecoveryDecision", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || goja.IsNull(call.Argument(0)) || goja.IsUndefined(call.Argument(0)) {
			s.OnRecoveryDecision = nil
			return goja.Undefined()
		}
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(runtime.NewTypeError("onRecoveryDecision: argument must be a function"))
		}
		s.OnRecoveryDecision = func(d RecoveryDecision) {
			_, _ = fn(goja.Undefined(), recoveryDecisionToJS(runtime, d))
		}
		return goja.Undefined()
	})

	return obj
}

// --- Safety Validator JS Bindings ---

// safetyConfigToJS converts a SafetyConfig to a JS object.
func safetyConfigToJS(runtime *goja.Runtime, cfg SafetyConfig) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("enabled", cfg.Enabled)
	_ = obj.Set("defaultAction", int(cfg.DefaultAction))
	_ = obj.Set("warnThreshold", cfg.WarnThreshold)
	_ = obj.Set("confirmThreshold", cfg.ConfirmThreshold)
	_ = obj.Set("blockThreshold", cfg.BlockThreshold)

	blocked := runtime.NewArray()
	for i, t := range cfg.BlockedTools {
		_ = blocked.Set(fmt.Sprintf("%d", i), t)
	}
	_ = obj.Set("blockedTools", blocked)

	blockedPaths := runtime.NewArray()
	for i, p := range cfg.BlockedPaths {
		_ = blockedPaths.Set(fmt.Sprintf("%d", i), p)
	}
	_ = obj.Set("blockedPaths", blockedPaths)

	allowedPaths := runtime.NewArray()
	for i, p := range cfg.AllowedPaths {
		_ = allowedPaths.Set(fmt.Sprintf("%d", i), p)
	}
	_ = obj.Set("allowedPaths", allowedPaths)

	patterns := runtime.NewArray()
	for i, p := range cfg.SensitivePatterns {
		_ = patterns.Set(fmt.Sprintf("%d", i), p)
	}
	_ = obj.Set("sensitivePatterns", patterns)

	return obj
}

// jsToSafetyConfig converts a JS object to a SafetyConfig.
func jsToSafetyConfig(runtime *goja.Runtime, val goja.Value) SafetyConfig {
	cfg := DefaultSafetyConfig()
	obj := val.ToObject(runtime)

	if v := obj.Get("enabled"); v != nil && !goja.IsUndefined(v) {
		cfg.Enabled = v.ToBoolean()
	}
	if v := obj.Get("defaultAction"); v != nil && !goja.IsUndefined(v) {
		cfg.DefaultAction = PolicyAction(v.ToInteger())
	}
	if v := obj.Get("warnThreshold"); v != nil && !goja.IsUndefined(v) {
		cfg.WarnThreshold = v.ToFloat()
	}
	if v := obj.Get("confirmThreshold"); v != nil && !goja.IsUndefined(v) {
		cfg.ConfirmThreshold = v.ToFloat()
	}
	if v := obj.Get("blockThreshold"); v != nil && !goja.IsUndefined(v) {
		cfg.BlockThreshold = v.ToFloat()
	}
	if v := obj.Get("blockedTools"); v != nil && !goja.IsUndefined(v) {
		var tools []string
		if err := runtime.ExportTo(v, &tools); err == nil {
			cfg.BlockedTools = tools
		}
	}
	if v := obj.Get("blockedPaths"); v != nil && !goja.IsUndefined(v) {
		var paths []string
		if err := runtime.ExportTo(v, &paths); err == nil {
			cfg.BlockedPaths = paths
		}
	}
	if v := obj.Get("allowedPaths"); v != nil && !goja.IsUndefined(v) {
		var paths []string
		if err := runtime.ExportTo(v, &paths); err == nil {
			cfg.AllowedPaths = paths
		}
	}
	if v := obj.Get("sensitivePatterns"); v != nil && !goja.IsUndefined(v) {
		var patterns []string
		if err := runtime.ExportTo(v, &patterns); err == nil {
			cfg.SensitivePatterns = patterns
		}
	}

	return cfg
}

// safetyAssessmentToJS converts a SafetyAssessment to a JS object.
func safetyAssessmentToJS(runtime *goja.Runtime, a SafetyAssessment) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("intent", int(a.Intent))
	_ = obj.Set("intentName", IntentName(a.Intent))
	_ = obj.Set("scope", int(a.Scope))
	_ = obj.Set("scopeName", ScopeName(a.Scope))
	_ = obj.Set("riskScore", a.RiskScore)
	_ = obj.Set("riskLevel", int(a.RiskLevel))
	_ = obj.Set("riskLevelName", RiskLevelName(a.RiskLevel))
	_ = obj.Set("action", int(a.Action))
	_ = obj.Set("actionName", PolicyActionName(a.Action))
	_ = obj.Set("reason", a.Reason)

	details := runtime.NewObject()
	for k, v := range a.Details {
		_ = details.Set(k, v)
	}
	_ = obj.Set("details", details)

	return obj
}

// safetyStatsToJS converts SafetyStats to a JS object.
func safetyStatsToJS(runtime *goja.Runtime, s SafetyStats) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("totalChecks", s.TotalChecks)
	_ = obj.Set("allowCount", s.AllowCount)
	_ = obj.Set("warnCount", s.WarnCount)
	_ = obj.Set("confirmCount", s.ConfirmCount)
	_ = obj.Set("blockCount", s.BlockCount)

	intents := runtime.NewObject()
	for k, v := range s.IntentCounts {
		_ = intents.Set(k, v)
	}
	_ = obj.Set("intentCounts", intents)

	scopes := runtime.NewObject()
	for k, v := range s.ScopeCounts {
		_ = scopes.Set(k, v)
	}
	_ = obj.Set("scopeCounts", scopes)

	return obj
}

// jsToSafetyAction converts a JS object to a SafetyAction.
func jsToSafetyAction(runtime *goja.Runtime, val goja.Value) SafetyAction {
	obj := val.ToObject(runtime)
	action := SafetyAction{}

	if v := obj.Get("type"); v != nil && !goja.IsUndefined(v) {
		action.Type = v.String()
	}
	if v := obj.Get("name"); v != nil && !goja.IsUndefined(v) {
		action.Name = v.String()
	}
	if v := obj.Get("raw"); v != nil && !goja.IsUndefined(v) {
		action.Raw = v.String()
	}
	if v := obj.Get("args"); v != nil && !goja.IsUndefined(v) {
		args := make(map[string]string)
		if err := runtime.ExportTo(v, &args); err == nil {
			action.Args = args
		}
	}
	if v := obj.Get("filePaths"); v != nil && !goja.IsUndefined(v) {
		var paths []string
		if err := runtime.ExportTo(v, &paths); err == nil {
			action.FilePaths = paths
		}
	}

	return action
}

// wrapSafetyValidator creates a JS object wrapping a *SafetyValidator.
func wrapSafetyValidator(runtime *goja.Runtime, sv *SafetyValidator) goja.Value {
	obj := runtime.NewObject()

	// Store Go validator reference for composite validator extraction.
	_ = obj.Set("__goValidator", runtime.ToValue(sv))

	// validate(action: object): object
	_ = obj.Set("validate", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("validate: action argument is required"))
		}
		action := jsToSafetyAction(runtime, call.Argument(0))
		a := sv.Validate(action)
		return safetyAssessmentToJS(runtime, a)
	})

	// stats(): object
	_ = obj.Set("stats", func() goja.Value {
		return safetyStatsToJS(runtime, sv.Stats())
	})

	// config(): object
	_ = obj.Set("config", func() goja.Value {
		return safetyConfigToJS(runtime, sv.Config())
	})

	return obj
}

// wrapCompositeValidator creates a JS object wrapping a *CompositeValidator.
func wrapCompositeValidator(runtime *goja.Runtime, cv *CompositeValidator) goja.Value {
	obj := runtime.NewObject()

	// Store Go validator reference for nested composite extraction.
	_ = obj.Set("__goValidator", runtime.ToValue(cv))

	// validate(action: object): object
	_ = obj.Set("validate", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("validate: action argument is required"))
		}
		action := jsToSafetyAction(runtime, call.Argument(0))
		a := cv.Validate(action)
		return safetyAssessmentToJS(runtime, a)
	})

	return obj
}

// --- Choice Resolver JS Bindings ---

// choiceConfigToJS converts a ChoiceConfig to a JS object.
func choiceConfigToJS(runtime *goja.Runtime, cfg ChoiceConfig) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("confirmThreshold", cfg.ConfirmThreshold)
	_ = obj.Set("minCandidates", cfg.MinCandidates)

	criteria := runtime.NewArray()
	for i, c := range cfg.DefaultCriteria {
		item := runtime.NewObject()
		_ = item.Set("name", c.Name)
		_ = item.Set("weight", c.Weight)
		_ = item.Set("description", c.Description)
		_ = criteria.Set(fmt.Sprintf("%d", i), item)
	}
	_ = obj.Set("defaultCriteria", criteria)

	return obj
}

// jsToChoiceConfig converts a JS object to a ChoiceConfig.
func jsToChoiceConfig(runtime *goja.Runtime, val goja.Value) ChoiceConfig {
	cfg := DefaultChoiceConfig()
	obj := val.ToObject(runtime)

	if v := obj.Get("confirmThreshold"); v != nil && !goja.IsUndefined(v) {
		cfg.ConfirmThreshold = v.ToFloat()
	}
	if v := obj.Get("minCandidates"); v != nil && !goja.IsUndefined(v) {
		cfg.MinCandidates = int(v.ToInteger())
	}
	if v := obj.Get("defaultCriteria"); v != nil && !goja.IsUndefined(v) {
		arr := v.ToObject(runtime)
		length := int(arr.Get("length").ToInteger())
		criteria := make([]Criterion, 0, length)
		for i := 0; i < length; i++ {
			item := arr.Get(fmt.Sprintf("%d", i)).ToObject(runtime)
			crit := Criterion{}
			if n := item.Get("name"); n != nil && !goja.IsUndefined(n) {
				crit.Name = n.String()
			}
			if w := item.Get("weight"); w != nil && !goja.IsUndefined(w) {
				crit.Weight = w.ToFloat()
			}
			if d := item.Get("description"); d != nil && !goja.IsUndefined(d) {
				crit.Description = d.String()
			}
			criteria = append(criteria, crit)
		}
		cfg.DefaultCriteria = criteria
	}

	return cfg
}

// choiceResultToJS converts a ChoiceResult to a JS object.
func choiceResultToJS(runtime *goja.Runtime, r ChoiceResult) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("recommendedID", r.RecommendedID)
	_ = obj.Set("justification", r.Justification)
	_ = obj.Set("needsConfirm", r.NeedsConfirm)

	rankings := runtime.NewArray()
	for i, cs := range r.Rankings {
		item := runtime.NewObject()
		_ = item.Set("candidateID", cs.CandidateID)
		_ = item.Set("name", cs.Name)
		_ = item.Set("totalScore", cs.TotalScore)
		_ = item.Set("rank", cs.Rank)
		_ = item.Set("justification", cs.Justification)

		scores := runtime.NewObject()
		for k, v := range cs.Scores {
			_ = scores.Set(k, v)
		}
		_ = item.Set("scores", scores)

		_ = rankings.Set(fmt.Sprintf("%d", i), item)
	}
	_ = obj.Set("rankings", rankings)

	return obj
}

// choiceStatsToJS converts ChoiceStats to a JS object.
func choiceStatsToJS(runtime *goja.Runtime, s ChoiceStats) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("totalAnalyses", s.TotalAnalyses)
	_ = obj.Set("totalCandidates", s.TotalCandidates)
	_ = obj.Set("confirmCount", s.ConfirmCount)
	return obj
}

// jsToCandidates converts a JS array to a Go []Candidate slice.
func jsToCandidates(runtime *goja.Runtime, val goja.Value) []Candidate {
	arr := val.ToObject(runtime)
	length := int(arr.Get("length").ToInteger())
	candidates := make([]Candidate, 0, length)
	for i := 0; i < length; i++ {
		item := arr.Get(fmt.Sprintf("%d", i)).ToObject(runtime)
		c := Candidate{}
		if v := item.Get("id"); v != nil && !goja.IsUndefined(v) {
			c.ID = v.String()
		}
		if v := item.Get("name"); v != nil && !goja.IsUndefined(v) {
			c.Name = v.String()
		}
		if v := item.Get("description"); v != nil && !goja.IsUndefined(v) {
			c.Description = v.String()
		}
		if v := item.Get("attributes"); v != nil && !goja.IsUndefined(v) {
			attrs := make(map[string]string)
			if err := runtime.ExportTo(v, &attrs); err == nil {
				c.Attributes = attrs
			}
		}
		candidates = append(candidates, c)
	}
	return candidates
}

// jsToCriteria converts a JS array to a Go []Criterion slice.
func jsToCriteria(runtime *goja.Runtime, val goja.Value) []Criterion {
	arr := val.ToObject(runtime)
	length := int(arr.Get("length").ToInteger())
	criteria := make([]Criterion, 0, length)
	for i := 0; i < length; i++ {
		item := arr.Get(fmt.Sprintf("%d", i)).ToObject(runtime)
		c := Criterion{}
		if v := item.Get("name"); v != nil && !goja.IsUndefined(v) {
			c.Name = v.String()
		}
		if v := item.Get("weight"); v != nil && !goja.IsUndefined(v) {
			c.Weight = v.ToFloat()
		}
		if v := item.Get("description"); v != nil && !goja.IsUndefined(v) {
			c.Description = v.String()
		}
		criteria = append(criteria, c)
	}
	return criteria
}

// wrapChoiceResolver creates a JS object wrapping a *ChoiceResolver.
func wrapChoiceResolver(runtime *goja.Runtime, cr *ChoiceResolver) goja.Value {
	obj := runtime.NewObject()

	// analyze(candidates: object[], criteria?: object[], scoreFn?: function): object
	_ = obj.Set("analyze", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("analyze: candidates argument is required"))
		}
		candidates := jsToCandidates(runtime, call.Argument(0))

		var criteria []Criterion
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			criteria = jsToCriteria(runtime, call.Argument(1))
		}

		var scoreFn ScoreFunc
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
			fn, ok := goja.AssertFunction(call.Argument(2))
			if !ok {
				panic(runtime.NewTypeError("analyze: scoreFn must be a function"))
			}
			scoreFn = func(cand Candidate, crit Criterion) float64 {
				candObj := runtime.NewObject()
				_ = candObj.Set("id", cand.ID)
				_ = candObj.Set("name", cand.Name)
				_ = candObj.Set("description", cand.Description)
				attrs := runtime.NewObject()
				for k, v := range cand.Attributes {
					_ = attrs.Set(k, v)
				}
				_ = candObj.Set("attributes", attrs)

				critObj := runtime.NewObject()
				_ = critObj.Set("name", crit.Name)
				_ = critObj.Set("weight", crit.Weight)
				_ = critObj.Set("description", crit.Description)

				result, err := fn(goja.Undefined(), candObj, critObj)
				if err != nil {
					return 0.5
				}
				return result.ToFloat()
			}
		}

		r, err := cr.Analyze(candidates, criteria, scoreFn)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return choiceResultToJS(runtime, r)
	})

	// stats(): object
	_ = obj.Set("stats", func() goja.Value {
		return choiceStatsToJS(runtime, cr.Stats())
	})

	// config(): object
	_ = obj.Set("config", func() goja.Value {
		return choiceConfigToJS(runtime, cr.Config())
	})

	return obj
}
