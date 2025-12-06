package nextintegerid

import (
	"testing"

	"github.com/dop251/goja"
)

func setupModule(t *testing.T) (*goja.Runtime, goja.Callable) {
	t.Helper()

	runtime := goja.New()
	module := runtime.NewObject()
	Require(runtime, module)

	export := module.Get("exports")
	callable, ok := goja.AssertFunction(export)
	if !ok {
		t.Fatalf("exports is not callable")
	}

	return runtime, callable
}

func TestNextIntegerID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args string
		want int64
	}{
		{
			name: "no arguments",
			args: "",
			want: 1,
		},
		{
			name: "empty array",
			args: "[]",
			want: 1,
		},
		{
			name: "array with ids",
			args: `[ { id: 2 }, { id: 7 }, { id: 3 } ]`,
			want: 8,
		},
		{
			name: "array with string ids",
			args: `[ { id: "9" }, { id: "not-a-number" } ]`,
			want: 10,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Each subtest gets its own runtime to avoid race conditions
			runtime, nextFn := setupModule(t)

			var args []goja.Value
			if tc.args != "" {
				args = []goja.Value{mustRunValue(t, runtime, tc.args)}
			}

			result, err := nextFn(goja.Undefined(), args...)
			if err != nil {
				t.Fatalf("call failed: %v", err)
			}
			if got := result.ToInteger(); got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func mustRunValue(t *testing.T, runtime *goja.Runtime, script string) goja.Value {
	t.Helper()

	value, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("failed to run js: %v", err)
	}
	return value
}
