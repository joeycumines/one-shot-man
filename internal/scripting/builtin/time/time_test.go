package time

import (
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestSleep(t *testing.T) {
	runtime := goja.New()
	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)
	LoadModule(runtime, module)

	sleepVal := exports.Get("sleep")
	sleepFn, ok := goja.AssertFunction(sleepVal)
	if !ok {
		t.Fatalf("sleep export is not callable")
	}

	start := time.Now()
	if _, err := sleepFn(goja.Undefined(), runtime.ToValue(2)); err != nil {
		t.Fatalf("sleep call failed: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 2*time.Millisecond {
		t.Fatalf("sleep returned too quickly: %v", elapsed)
	}

	if _, err := sleepFn(goja.Undefined()); err != nil {
		t.Fatalf("sleep without args should succeed: %v", err)
	}
}
