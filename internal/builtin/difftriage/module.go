package difftriage

import (
	"context"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/one-shot-man/internal/builtin/async"
	"github.com/joeycumines/one-shot-man/internal/triage"
)

// Require returns the osm:diff_triage module loader.
func Require(ctx context.Context, adapter *gojaeventloop.Adapter) func(*goja.Runtime, *goja.Object) {
	return func(rt *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		_ = exports.Set("triage", func(call goja.FunctionCall) goja.Value {
			if adapter == nil {
				panic(rt.NewGoError(errAdapter()))
			}
			diff := ""
			if len(call.Arguments) > 0 {
				diff = call.Argument(0).String()
			}
			return async.Promise(adapter, ctx, func(_ context.Context) (any, error) {
				results := triage.TriageDiff(diff)
				return results, nil
			})
		})
		_ = exports.Set("triageSummary", func(call goja.FunctionCall) goja.Value {
			if adapter == nil {
				panic(rt.NewGoError(errAdapter()))
			}
			diff := ""
			if len(call.Arguments) > 0 {
				diff = call.Argument(0).String()
			}
			return async.Promise(adapter, ctx, func(_ context.Context) (any, error) {
				results := triage.TriageDiff(diff)
				summary := triage.TriageSummary(results)
				// Convert to map[string]int for JS.
				m := make(map[string]int, len(summary))
				for k, v := range summary {
					m[string(k)] = v
				}
				return m, nil
			})
		})
	}
}

type adapterErr string

func (e adapterErr) Error() string { return string(e) }

func errAdapter() error { return adapterErr("difftriage: adapter is nil") }
