package claudemux

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadClassificationResult(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	want := ClassificationResult{
		"cmd/main.go":     "entry-point",
		"internal/foo.go": "impl",
		"docs/README.md":  "docs",
	}
	data, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "classification.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := ReadClassificationResult(dir)
	if err != nil {
		t.Fatalf("ReadClassificationResult: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d files, want 3", len(got))
	}
	if got["cmd/main.go"] != "entry-point" {
		t.Errorf("got[cmd/main.go] = %q, want entry-point", got["cmd/main.go"])
	}
	if got["internal/foo.go"] != "impl" {
		t.Errorf("got[internal/foo.go] = %q, want impl", got["internal/foo.go"])
	}
	if got["docs/README.md"] != "docs" {
		t.Errorf("got[docs/README.md] = %q, want docs", got["docs/README.md"])
	}
}

func TestReadClassificationResult_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := ReadClassificationResult(dir)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want os.ErrNotExist in chain", err)
	}
}

func TestReadClassificationResult_CorruptJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "classification.json"), []byte("not json{"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := ReadClassificationResult(dir)
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}

func TestReadSplitPlanResult(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	want := SplitPlanResult{
		{Name: "types", Files: []string{"types.go"}, Message: "add types", Order: 0},
		{Name: "impl", Files: []string{"impl.go", "impl_test.go"}, Message: "add impl", Order: 1},
	}
	data, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "split-plan.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := ReadSplitPlanResult(dir)
	if err != nil {
		t.Fatalf("ReadSplitPlanResult: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d stages, want 2", len(got))
	}
	if got[0].Name != "types" {
		t.Errorf("stage[0].Name = %q, want types", got[0].Name)
	}
	if len(got[0].Files) != 1 || got[0].Files[0] != "types.go" {
		t.Errorf("stage[0].Files = %v, want [types.go]", got[0].Files)
	}
	if got[1].Name != "impl" {
		t.Errorf("stage[1].Name = %q, want impl", got[1].Name)
	}
	if len(got[1].Files) != 2 {
		t.Errorf("stage[1] has %d files, want 2", len(got[1].Files))
	}
	if got[0].Order != 0 || got[1].Order != 1 {
		t.Errorf("orders = %d, %d; want 0, 1", got[0].Order, got[1].Order)
	}
}

func TestReadSplitPlanResult_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := ReadSplitPlanResult(dir)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want os.ErrNotExist in chain", err)
	}
}

func TestReadSplitPlanResult_CorruptJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "split-plan.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := ReadSplitPlanResult(dir)
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}
