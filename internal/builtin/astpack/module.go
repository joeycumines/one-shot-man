package astpack

import (
	"context"
	"encoding/json"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// Require returns the osm:astpack module loader.
func Require(ctx context.Context, adapter *gojaeventloop.Adapter) func(*goja.Runtime, *goja.Object) {
	return func(rt *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		_ = exports.Set("pack", func(call goja.FunctionCall) goja.Value {
			if adapter == nil {
				panic(rt.NewGoError(errNoAdapter()))
			}
			var files map[string]string
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				// Accept either object map or JSON string.
				arg := call.Argument(0)
				if obj, ok := arg.Export().(map[string]string); ok {
					files = obj
				} else {
					// Try generic map[string]interface{} then convert.
					if m, ok := arg.Export().(map[string]interface{}); ok {
						files = make(map[string]string, len(m))
						for k, v := range m {
							if s, ok := v.(string); ok {
								files[k] = s
							} else {
								b, _ := json.Marshal(v)
								files[k] = string(b)
							}
						}
					} else {
						// Try to JSON round-trip via Export.
						b, err := json.Marshal(arg.Export())
						if err == nil {
							_ = json.Unmarshal(b, &files)
						}
					}
				}
			}
			// Async pack: offload to goroutine via async.Promise.
			return adapter.Promisify(ctx, func(_ context.Context) (any, error) {
				pkg := Pack(files)
				// Return as map for JS consumption.
				return pkg, nil
			})
		})

		_ = exports.Set("packDiff", func(call goja.FunctionCall) goja.Value {
			if adapter == nil {
				panic(rt.NewGoError(errNoAdapter()))
			}
			diff := ""
			if len(call.Arguments) > 0 {
				diff = call.Argument(0).String()
			}
			return adapter.Promisify(ctx, func(_ context.Context) (any, error) {
				pkg := PackDiff(diff)
				return pkg, nil
			})
		})
	}
}

func errNoAdapter() error {
	return errAdapterRequired("astpack: adapter is nil")
}

type adapterError string

func (e adapterError) Error() string { return string(e) }

func errAdapterRequired(msg string) error { return adapterError(msg) }
