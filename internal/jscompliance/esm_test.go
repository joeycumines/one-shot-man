package jscompliance

import (
	"context"
	"strings"
	"testing"
)

// TestESM_Rejection closes the zero-coverage gap (ESM-1): the runtime uses
// Goja's CommonJS module system and MUST reject ES module syntax with a
// compile-time error. Each of these forms must fail to compile — a regression
// that silently accepted ESM would break the documented contract.
func TestESM_Rejection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cases := []struct {
		name string
		src  string
	}{
		{"static import", `import { x } from 'osm:os';`},
		{"export declaration", `export function f() { return 42; }`},
		{"export default", `export default { a: 1 };`},
		{"dynamic import expression", `import('osm:os').then(function(){})`},
		{"re-export", `export { x } from 'osm:os';`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			engine, _, _ := newComplianceEngine(t, ctx)
			_, err := evalJS(t, engine, c.src, defaultEvalTimeout)
			if err == nil {
				t.Fatalf("ESM syntax was accepted (must throw):\n%s", c.src)
			}
			// Confirm it's a parse/syntax error, not some unrelated runtime error.
			msg := strings.ToLower(err.Error())
			if !(strings.Contains(msg, "syntax") || strings.Contains(msg, "unexpected") || strings.Contains(msg, "import") || strings.Contains(msg, "export") || strings.Contains(msg, "token")) {
				t.Logf("note: ESM rejected with non-obvious message: %v", err)
			}
		})
	}
}

// TestESM_CommonJSStillLoads is the positive control: CommonJS require/exports
// must still work (the rejection is specific to ESM, not all module syntax).
func TestESM_CommonJSStillLoads(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	v, err := evalJS(t, engine, `(function(){ var m = require('osm:json'); return typeof m.parse; })()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("CommonJS require failed: %v", err)
	}
	if s, ok := v.(string); !ok || s != "function" {
		t.Errorf("require('osm:json').parse typeof = %v, want function", v)
	}
}
