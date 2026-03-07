package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// ── writeResolvedTable (unexported, directly tested) ───────────────

func TestWriteResolvedTable_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := writeResolvedTable(&buf, nil); err != nil {
		t.Fatal(err)
	}
	// Should still have the header line.
	out := buf.String()
	if !strings.Contains(out, "KEY") || !strings.Contains(out, "VALUE") || !strings.Contains(out, "SOURCE") {
		t.Errorf("header missing: got %q", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line (header only), got %d: %v", len(lines), lines)
	}
}

func TestWriteResolvedTable_SingleRow(t *testing.T) {
	var buf bytes.Buffer
	if err := writeResolvedTable(&buf, []config.ResolvedOption{
		{Key: "editor", Value: "vim", Source: "config-file"},
	}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "editor") {
		t.Errorf("missing key: %q", out)
	}
	if !strings.Contains(out, "vim") {
		t.Errorf("missing value: %q", out)
	}
	if !strings.Contains(out, "config-file") {
		t.Errorf("missing source: %q", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines (header + 1 row), got %d", len(lines))
	}
}

func TestWriteResolvedTable_MultipleRows(t *testing.T) {
	var buf bytes.Buffer
	if err := writeResolvedTable(&buf, []config.ResolvedOption{
		{Key: "editor", Value: "vim", Source: "config-file"},
		{Key: "log-level", Value: "debug", Source: "env"},
		{Key: "session-store", Value: "fs", Source: "default"},
	}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines (header + 3 rows), got %d", len(lines))
	}
	// Verify all three keys appear
	for _, key := range []string{"editor", "log-level", "session-store"} {
		if !strings.Contains(out, key) {
			t.Errorf("missing key %q in output: %q", key, out)
		}
	}
}

func TestWriteResolvedTable_TabAlignment(t *testing.T) {
	var buf bytes.Buffer
	if err := writeResolvedTable(&buf, []config.ResolvedOption{
		{Key: "a", Value: "short", Source: "s1"},
		{Key: "long-key-name", Value: "also-a-long-value", Source: "source-name"},
	}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// tabwriter should produce aligned columns — verify that the header
	// and each row contain multiple spaces (tabwriter padding).
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i, line := range lines {
		if !strings.Contains(line, "  ") {
			t.Errorf("line %d lacks tabwriter padding: %q", i, line)
		}
	}
}
