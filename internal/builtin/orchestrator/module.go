package orchestrator

import (
	"github.com/dop251/goja"
)

// Require returns a module loader for `osm:orchestrator` that exposes the
// PTY output parser to JavaScript scripts.
func Require() func(runtime *goja.Runtime, module *goja.Object) {
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
