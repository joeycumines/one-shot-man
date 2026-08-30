package aimux

import (
	"time"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/aimuxcore"
)

// registerTUIStateBindings exposes the aimuxcore TUI state machine to JavaScript.
func registerTUIStateBindings(runtime *goja.Runtime, exports *goja.Object) {
	_ = exports.Set("defaultTUIStateConfig", func() *goja.Object {
		return configToJSObject(runtime, aimuxcore.DefaultTUIStateConfig())
	})
	_ = exports.Set("newTUIStateMachine", func(cfg *goja.Object) *goja.Object {
		var c aimuxcore.TUIStateMachineConfig
		if cfg == nil {
			c = aimuxcore.DefaultTUIStateConfig()
		} else if err := runtime.ExportTo(cfg, &c); err != nil {
			panic(runtime.NewTypeError(err.Error()))
		}
		sm, err := aimuxcore.NewTUIStateMachine(c)
		if err != nil {
			panic(runtime.NewTypeError(err.Error()))
		}
		return newTUIStateMachineObject(runtime, sm)
	})
	_ = exports.Set("tuiStateName", func(state int) string {
		return aimuxcore.TUIStateName(aimuxcore.TUIState(state))
	})
}

func newTUIStateMachineObject(runtime *goja.Runtime, sm *aimuxcore.TUIStateMachine) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("processOutput", func(line string) map[string]any {
		return tuiUpdateToMap(sm.ProcessOutput(line, time.Now()))
	})
	_ = obj.Set("checkTimeout", func() map[string]any {
		return tuiUpdateToMap(sm.CheckTimeout(time.Now()))
	})
	_ = obj.Set("state", func() int { return int(sm.State()) })
	_ = obj.Set("stateName", func() string { return sm.StateName() })
	_ = obj.Set("reset", func() { sm.Reset() })
	return obj
}

func tuiUpdateToMap(update aimuxcore.TUIStateUpdate) map[string]any {
	fields := map[string]any{}
	for k, v := range update.Fields {
		fields[k] = v
	}
	return map[string]any{
		"from":      int(update.From),
		"to":        int(update.To),
		"state":     int(update.State),
		"stateName": update.StateName,
		"pattern":   update.Pattern,
		"fields":    fields,
		"changed":   update.Changed,
	}
}

func configToJSObject(runtime *goja.Runtime, cfg aimuxcore.TUIStateMachineConfig) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("readyPatterns", cfg.ReadyPatterns)
	_ = obj.Set("processingPatterns", cfg.ProcessingPatterns)
	_ = obj.Set("errorPatterns", cfg.ErrorPatterns)
	_ = obj.Set("rateLimitPatterns", cfg.RateLimitPatterns)
	_ = obj.Set("permissionPatterns", cfg.PermissionPatterns)
	_ = obj.Set("startupTimeoutMs", cfg.StartupTimeout.Milliseconds())
	_ = obj.Set("processingTimeoutMs", cfg.ProcessingTimeout.Milliseconds())
	return obj
}
