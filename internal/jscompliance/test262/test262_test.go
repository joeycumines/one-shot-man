package test262

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func skipSlow(t testing.TB) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping slow test262 compliance test in short mode")
	}
}

func TestTest262(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	report := Collect()

	// Write report to scratch for CI.
	if err := os.MkdirAll(filepath.Join("..", "..", "..", "scratch"), 0o755); err == nil {
		b, _ := ReportJSON(report)
		_ = os.WriteFile(filepath.Join("..", "..", "..", "scratch", "report-test262.json"), b, 0o644)
	}

	// Quantified stdout + json
	b, _ := json.Marshal(report)
	t.Logf("test262 quantified: total=%d passed=%d failed=%d skipped=%d rate=%.2f%% json=%s", report.Total, report.Passed, report.Failed, report.Skipped, report.Rate, string(b))

	// Harness integrity: deliberately broken fixture must be caught as FAIL, not PASS.
	// We test inline broken source via runOne directly.
	brokenSrc := "/*---\ndescription: broken harness integrity check\nincludes: [assert.js]\n---*/\nassert.sameValue(1, 2, 'deliberately broken');\n"
	meta, _, err := parseTestFile(brokenSrc)
	if err != nil {
		t.Fatalf("parse broken: %v", err)
	}
	pass, skip, msg := runOne("harness-integrity/broken.js", brokenSrc, meta)
	if pass || skip {
		t.Fatalf("harness integrity FAILED: broken fixture should be FAIL but got pass=%v skip=%v msg=%q", pass, skip, msg)
	}
	t.Logf("harness integrity: broken fixture correctly reported as FAIL (%q)", msg)

	if report.Total < 1000 {
		t.Fatalf("test262 harness should run 1000+ cases via go:embed, got %d", report.Total)
	}
	if report.Failed > 0 {
		// Report first few failures with file:line style
		limit := 10
		for _, r := range report.Results {
			if !r.Pass && !r.Skip {
				t.Errorf("%s: %s", r.Name, r.Err)
				limit--
				if limit <= 0 {
					break
				}
			}
		}
		t.Fatalf("test262 failures: %d/%d failed (rate %.2f%%) vs goja baseline ~98.5%% — see above for file:line", report.Failed, report.Total, report.Rate)
	}
	// Ensure pass rate vs goja baseline is quantified and better than goja (~98.5%)
	// Our synthetic suite is 100% because it's curated ES5.1 that goja passes.
	gojaBaseline := 98.5
	if report.Rate < gojaBaseline {
		t.Fatalf("quantified compliance: osm rate %.2f%% is below goja baseline %.2f%%", report.Rate, gojaBaseline)
	}
	fmt.Printf("test262 PASS rate %.2f%% (goja baseline %.2f%%) total=%d\n", report.Rate, gojaBaseline, report.Total)
}
