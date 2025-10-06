package scripting

import (
	"testing"

	"github.com/dop251/goja"
)

// TestSymbolObjectAccess tests how goja handles Symbol objects stored as properties
func TestSymbolObjectAccess(t *testing.T) {
	runtime := goja.New()

	// Method 1: JavaScript object literal
	t.Log("=== Method 1: Object Literal ===")
	jsCode1 := `
		(function() {
			return {
				myKey: Symbol('test:key')
			};
		})()
	`
	obj1, err := runtime.RunString(jsCode1)
	if err != nil {
		t.Fatalf("Failed to create object literal: %v", err)
	}

	objRef1 := obj1.ToObject(runtime)
	for _, key := range objRef1.Keys() {
		val := objRef1.Get(key)
		t.Logf("Key: %s, Value: %v, Type: %s, ExportType: %s",
			key, val, val.String(), val.ExportType().Name())
	}

	// Method 2: runtime.NewObject() + Set()
	t.Log("=== Method 2: NewObject + Set ===")
	symbolVal, err := runtime.RunString(`Symbol('test:key')`)
	if err != nil {
		t.Fatalf("Failed to create symbol: %v", err)
	}

	obj2 := runtime.NewObject()
	if err := obj2.Set("myKey", symbolVal); err != nil {
		t.Fatalf("Failed to set property: %v", err)
	}

	for _, key := range obj2.Keys() {
		val := obj2.Get(key)
		t.Logf("Key: %s, Value: %v, Type: %s, ExportType: %s",
			key, val, val.String(), val.ExportType().Name())
	}

	// Method 3: Check if the original symbolVal is different from obj2.Get()
	t.Log("=== Method 3: Compare Original vs Get ===")
	retrievedVal := obj2.Get("myKey")
	t.Logf("Original Symbol - Type: %s, ExportType: %s", symbolVal.String(), symbolVal.ExportType().Name())
	t.Logf("Retrieved Symbol - Type: %s, ExportType: %s", retrievedVal.String(), retrievedVal.ExportType().Name())
	t.Logf("Are they equal? %v", symbolVal.Equals(retrievedVal))

	// Method 4: Array access
	t.Log("=== Method 4: Array Access ===")
	arrScript := `
		(function() {
			return [Symbol('test:key')];
		})()
	`
	arr, err := runtime.RunString(arrScript)
	if err != nil {
		t.Fatalf("Failed to create array: %v", err)
	}

	arrObj := arr.ToObject(runtime)
	elem := arrObj.Get("0")
	t.Logf("Array element - Type: %s, ExportType: %s", elem.String(), elem.ExportType().Name())

	// Method 5: Direct property access via computed property in JS
	t.Log("=== Method 5: JS Computed Property Access ===")
	getScript := `
		(function(obj, key) {
			return obj[key];
		})
	`
	getFn, err := runtime.RunString(getScript)
	if err != nil {
		t.Fatalf("Failed to create getter function: %v", err)
	}

	callable, ok := goja.AssertFunction(getFn)
	if !ok {
		t.Fatal("Getter script did not return a function")
	}

	result, err := callable(goja.Undefined(), obj2, runtime.ToValue("myKey"))
	if err != nil {
		t.Fatalf("Failed to call getter: %v", err)
	}

	t.Logf("JS computed access - Type: %s, ExportType: %s", result.String(), result.ExportType().Name())
}
