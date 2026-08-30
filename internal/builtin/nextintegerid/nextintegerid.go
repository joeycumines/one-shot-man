package nextintegerid

import (
	"fmt"
	"math"

	"github.com/joeycumines/goja"
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

		// Initialize to MinInt64 so all-negative id lists compute the true max
		// (max+1). The previous 0-floor caused [{id:-5},{id:-1}] to return 1
		// instead of 0 (the correct max+1 for max=-1).
		var maxVal int64 = math.MinInt64
		var foundAny bool
		for i := range length {
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
			foundAny = true
		}
		// Empty list (or no items with an id): floor is 1.
		if !foundAny {
			return runtime.ToValue(1)
		}
		return runtime.ToValue(maxVal + 1)
	})
}
