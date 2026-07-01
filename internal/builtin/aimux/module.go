package aimux

import (
	"context"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	"github.com/joeycumines/one-shot-man/internal/aimuxcore"
)

// Require returns a module loader for `osm:aimux`. It exposes event/state
// constants and bindings for provider, parser, TUI state, and event stream
// management via the binding_*.go files.
func Require(ctx context.Context, adapter *gojaeventloop.Adapter) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// Parser event type constants.
		_ = exports.Set("EVENT_TEXT", int(aimuxcore.EventText))
		_ = exports.Set("EVENT_RATE_LIMIT", int(aimuxcore.EventRateLimit))
		_ = exports.Set("EVENT_PERMISSION", int(aimuxcore.EventPermission))
		_ = exports.Set("EVENT_MODEL_SELECT", int(aimuxcore.EventModelSelect))
		_ = exports.Set("EVENT_SSO_LOGIN", int(aimuxcore.EventSSOLogin))
		_ = exports.Set("EVENT_COMPLETION", int(aimuxcore.EventCompletion))
		_ = exports.Set("EVENT_TOOL_USE", int(aimuxcore.EventToolUse))
		_ = exports.Set("EVENT_ERROR", int(aimuxcore.EventError))
		_ = exports.Set("EVENT_THINKING", int(aimuxcore.EventThinking))

		// TUI state constants.
		_ = exports.Set("STATE_INITIALIZING", int(aimuxcore.StateInitializing))
		_ = exports.Set("STATE_READY", int(aimuxcore.StateReady))
		_ = exports.Set("STATE_PROCESSING", int(aimuxcore.StateProcessing))
		_ = exports.Set("STATE_RESPONDING", int(aimuxcore.StateResponding))
		_ = exports.Set("STATE_ERROR", int(aimuxcore.StateError))
		_ = exports.Set("STATE_RATE_LIMITED", int(aimuxcore.StateRateLimited))
		_ = exports.Set("STATE_PERMISSION_PROMPT", int(aimuxcore.StatePermissionPrompt))

		registerProviderBindings(ctx, adapter, runtime, exports)
		registerParserBindings(runtime, exports)
		registerTUIStateBindings(runtime, exports)
		registerEventStreamBindings(runtime, exports)
	}
}
