package goja_compat

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
		t.Skip("skipping slow goja compat test in short mode")
	}
}

func TestGojaCompat(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	report := Collect()
	if err := os.MkdirAll(filepath.Join("..", "..", "..", "scratch"), 0o755); err == nil {
		b, _ := ReportJSON(report)
		_ = os.WriteFile(filepath.Join("..", "..", "..", "scratch", "report-goja-compat.json"), b, 0o644)
	}
	b, _ := json.Marshal(report)
	t.Logf("goja compat quantified: total=%d passed=%d failed=%d rate=%.2f%% json=%s", report.Total, report.Passed, report.Failed, report.Rate, string(b))
	if report.Total < 500 {
		t.Fatalf("goja compat harness should run 500+ cases, got %d", report.Total)
	}
	if report.Failed > 0 {
		limit := 10
		for _, r := range report.Results {
			if !r.Pass {
				t.Errorf("%s: %s", r.Name, r.Err)
				limit--
				if limit <= 0 {
					break
				}
			}
		}
		t.Fatalf("goja compat failures: %d/%d failed (rate %.2f%%) vs goja baseline", report.Failed, report.Total, report.Rate)
	}
	gojaBaseline := 98.9
	if report.Rate < gojaBaseline {
		t.Fatalf("osm rate %.2f%% below goja baseline %.2f%%", report.Rate, gojaBaseline)
	}
	fmt.Printf("goja_compat PASS rate %.2f%% (goja baseline %.2f%%) total=%d\n", report.Rate, gojaBaseline, report.Total)
}
