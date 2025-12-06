package time

import (
	"time"

	"github.com/dop251/goja"
)

func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)

	// sleep(ms: number): void
	_ = exports.Set("sleep", func(call goja.FunctionCall) goja.Value {
		time.Sleep(time.Duration(call.Argument(0).ToInteger()) * time.Millisecond)
		return goja.Undefined()
	})
}
