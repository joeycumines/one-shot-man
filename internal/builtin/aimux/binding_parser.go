package aimux

import (
	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/aimuxcore"
)

// registerParserBindings exposes the aimuxcore parser to JavaScript.
func registerParserBindings(runtime *goja.Runtime, exports *goja.Object) {
	_ = exports.Set("newParser", func() *goja.Object {
		return newParserObject(runtime, aimuxcore.NewParser())
	})
	_ = exports.Set("eventTypeName", func(t int) string {
		return aimuxcore.EventTypeName(aimuxcore.EventType(t))
	})
}

func newParserObject(runtime *goja.Runtime, p *aimuxcore.Parser) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("parse", func(line string) map[string]any {
		ev := p.Parse(line)
		fields := map[string]any{}
		for k, v := range ev.Fields {
			fields[k] = v
		}
		return map[string]any{
			"type":    int(ev.Type),
			"line":    ev.Line,
			"fields":  fields,
			"pattern": ev.Pattern,
		}
	})
	_ = obj.Set("patterns", func() []map[string]any {
		infos := p.Patterns()
		out := make([]map[string]any, len(infos))
		for i, info := range infos {
			out[i] = map[string]any{
				"name":      info.Name,
				"eventType": int(info.EventType),
				"pattern":   info.Pattern,
			}
		}
		return out
	})
	_ = obj.Set("addPattern", func(name string, pattern string, eventType int) error {
		return p.AddPattern(name, pattern, aimuxcore.EventType(eventType))
	})
	return obj
}
