package nextintegerid

import (
	"fmt"

	"github.com/dop251/goja"
)

func Require(runtime *goja.Runtime, module *goja.Object) {
	// nextId(list: Array<{id?: number}>): number
	// Simple id generator.
	_ = module.Set("exports", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return runtime.ToValue(1)
		}
		listVal := call.Argument(0)
		if listVal == nil || goja.IsUndefined(listVal) || goja.IsNull(listVal) {
			return runtime.ToValue(1)
		}
		listObj := listVal.ToObject(runtime)

		if listObj == nil || goja.IsUndefined(listObj) || goja.IsNull(listObj) {
			return runtime.ToValue(1)
		}

		// Check if it's an array-like object with a length property
		lengthVal := listObj.Get("length")
		if lengthVal == nil || goja.IsUndefined(lengthVal) || goja.IsNull(lengthVal) {
			return runtime.ToValue(1)
		}
		length := lengthVal.ToInteger()

		var maxVal int64 = 0
		for i := int64(0); i < length; i++ {
			itemVal := listObj.Get(fmt.Sprintf("%d", i))
			if itemVal == nil || goja.IsUndefined(itemVal) || goja.IsNull(itemVal) {
				continue
			}
			itemObj := itemVal.ToObject(runtime)
			if itemObj == nil {
				continue
			}

			idVal := itemObj.Get("id")
			if idVal == nil || goja.IsUndefined(idVal) || goja.IsNull(idVal) {
				continue
			}
			id := idVal.ToInteger()
			if id > maxVal {
				maxVal = id
			}
		}
		return runtime.ToValue(maxVal + 1)
	})
}
