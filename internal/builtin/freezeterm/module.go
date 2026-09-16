// Package freezeterm provides the "osm:freezeterm" JavaScript binding for the
// external freeze CLI. It composes internal/freezeterm without importing
// termmux: scripts capture terminal text with osm:termmux and render it here.
package freezeterm

import (
	"context"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/freezeterm"
)

// Require returns the module loader for "osm:freezeterm". All subprocess and
// filesystem work is tracked through the event-loop adapter so promises stay
// bounded and loop shutdown joins the work.
func Require(ctx context.Context, adapter *gojaeventloop.Adapter) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		requireAdapter := func(name string) *gojaeventloop.Adapter {
			if adapter == nil {
				panic(runtime.NewGoError(errAdapterRequired(name)))
			}
			return adapter
		}

		_ = exports.Set("info", func(call goja.FunctionCall) goja.Value {
			actx := requireAdapter("info")
			opts := parseInfoOptions(runtime, call.Argument(0))
			return actx.TrackPromise(ctx, func(workerCtx context.Context, settleFn gojaeventloop.TrackedSettlement) {
				info, err := parent.Info(workerCtx, opts)
				if err != nil {
					settle(settleFn, false, func(rt *goja.Runtime) any {
						return map[string]any{
							"available":  false,
							"executable": opts,
							"version":    "",
							"commit":     "",
							"raw":        "",
							"error":      err.Error(),
						}
					})
					return
				}
				settle(settleFn, false, func(rt *goja.Runtime) any {
					return map[string]any{
						"available":  true,
						"executable": info.Executable,
						"version":    info.Version,
						"commit":     info.Commit,
						"raw":        info.Raw,
					}
				})
			})
		})

		_ = exports.Set("render", func(call goja.FunctionCall) goja.Value {
			actx := requireAdapter("render")
			text, opts := parseRenderCall(runtime, "render", call)
			return actx.TrackPromise(ctx, func(workerCtx context.Context, settleFn gojaeventloop.TrackedSettlement) {
				result, err := parent.Render(workerCtx, opts.withInput(text))
				if err != nil {
					settle(settleFn, true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
					return
				}
				settle(settleFn, false, func(rt *goja.Runtime) any { return captureResultMap(result) })
			})
		})

		_ = exports.Set("renderText", func(call goja.FunctionCall) goja.Value {
			actx := requireAdapter("renderText")
			text, opts := parseRenderCall(runtime, "renderText", call)
			return actx.TrackPromise(ctx, func(workerCtx context.Context, settleFn gojaeventloop.TrackedSettlement) {
				result, err := parent.RenderText(workerCtx, opts.withInput(text))
				if err != nil {
					settle(settleFn, true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
					return
				}
				settle(settleFn, false, func(rt *goja.Runtime) any { return rt.ToValue(result) })
			})
		})
	}
}

// captureResultMap converts a Go render result to its JS shape. Binary
// artifacts never cross as bytes: PNG results expose only their path.
func captureResultMap(result parent.Result) map[string]any {
	out := map[string]any{
		"path":      result.Path,
		"format":    string(result.Format),
		"temporary": result.Temporary,
	}
	if result.Text != "" {
		out["text"] = result.Text
	}
	return out
}
