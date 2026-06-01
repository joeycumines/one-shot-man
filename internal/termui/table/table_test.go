package table

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

func bounds(w, h int) coordinate.Rect {
	return coordinate.Rect{Size: coordinate.Size{Width: w, Height: h}}
}

func TestTable_NoHeaders(t *testing.T) {
	rows := []TableRow{
		{"a", "b"},
		{"c", "d"},
	}
	tbl := NewTable(WithTableRows(rows))
	got := tbl.Render(bounds(10, 5))
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (no headers), got %d: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], "a") || !strings.Contains(lines[0], "b") {
		t.Errorf("expected first row to contain 'a' and 'b', got %q", lines[0])
	}
}

func TestTable_WithHeaders(t *testing.T) {
	tbl := NewTable(
		WithTableHeaders(TableRow{"Name", "Value"}),
		WithTableRows([]TableRow{{"foo", "bar"}}),
	)
	got := tbl.Render(bounds(20, 5))
	lines := strings.Split(got, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines (header + divider + row), got %d", len(lines))
	}
	if !strings.Contains(lines[0], "Name") || !strings.Contains(lines[0], "Value") {
		t.Errorf("expected header line to contain 'Name' and 'Value', got %q", lines[0])
	}
	// Second line should be a horizontal divider.
	if !strings.Contains(lines[1], "─") {
		t.Errorf("expected divider line to contain '─', got %q", lines[1])
	}
	if !strings.Contains(lines[2], "foo") || !strings.Contains(lines[2], "bar") {
		t.Errorf("expected data row to contain 'foo' and 'bar', got %q", lines[2])
	}
}

func TestTable_ColumnWidthScaling(t *testing.T) {
	// Content needs 20 chars (10+10), but bounds only give 10.
	tbl := NewTable(
		WithTableHeaders(TableRow{"AAAAAAAAAA", "BBBBBBBBBB"}),
		WithTableRows([]TableRow{{"aaaaaaaaaa", "bbbbbbbbbb"}}),
	)
	got := tbl.Render(bounds(10, 5))
	lines := strings.Split(got, "\n")
	// Each line should not exceed 10 visible characters wide (excluding ANSI).
	for i, line := range lines {
		visible := stripANSI(line)
		if utf8.RuneCountInString(visible) > 10 {
			t.Errorf("line %d exceeds bounds width: visible rune count=%d, line=%q", i, utf8.RuneCountInString(visible), visible)
		}
	}
}

func TestTable_ZeroBounds(t *testing.T) {
	tbl := NewTable(
		WithTableHeaders(TableRow{"H"}),
		WithTableRows([]TableRow{{"R"}}),
	)
	tests := []struct {
		name string
		b    coordinate.Rect
	}{
		{"zero width", bounds(0, 5)},
		{"zero height", bounds(5, 0)},
		{"negative width", bounds(-1, 5)},
		{"negative height", bounds(5, -1)},
		{"both zero", bounds(0, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tbl.Render(tt.b)
			if got != "" {
				t.Errorf("expected empty string, got %q", got)
			}
		})
	}
}

func TestTable_WithBorder(t *testing.T) {
	tbl := NewTable(
		WithTableHeaders(TableRow{"A", "B"}),
		WithTableRows([]TableRow{{"1", "2"}}),
		WithTableBorder(lipgloss.RoundedBorder()),
	)
	got := tbl.Render(bounds(20, 6))
	if got == "" {
		t.Fatal("expected non-empty output")
	}
	lines := strings.Split(got, "\n")
	topRunes := []rune(lines[0])
	if topRunes[0] != '╭' {
		t.Errorf("expected rounded border top-left corner, got %c", topRunes[0])
	}
}

func TestTable_EmptyTable(t *testing.T) {
	tbl := NewTable()
	got := tbl.Render(bounds(10, 5))
	if got != "" {
		t.Errorf("expected empty string for empty table, got %q", got)
	}
}

func TestTable_SingleRow(t *testing.T) {
	tbl := NewTable(WithTableRows([]TableRow{{"x", "y"}}))
	got := tbl.Render(bounds(10, 5))
	lines := strings.Split(got, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], "x") || !strings.Contains(lines[0], "y") {
		t.Errorf("expected row to contain 'x' and 'y', got %q", lines[0])
	}
}

func TestTable_HeightTruncation(t *testing.T) {
	tbl := NewTable(
		WithTableHeaders(TableRow{"H"}),
		WithTableRows([]TableRow{{"1"}, {"2"}, {"3"}, {"4"}}),
	)
	// Height 3 = header(1) + divider(1) + 1 data row.
	got := tbl.Render(bounds(10, 3))
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], "H") {
		t.Errorf("expected header in first line, got %q", lines[0])
	}
	if !strings.Contains(lines[2], "1") {
		t.Errorf("expected first data row in third line, got %q", lines[2])
	}
}

func TestTable_OptionsChaining(t *testing.T) {
	tbl := NewTable(
		WithTableHeaders(TableRow{"A", "B"}),
		WithTableRows([]TableRow{{"1", "2"}}),
	)
	if len(tbl.headers) != 2 {
		t.Errorf("expected 2 headers, got %d", len(tbl.headers))
	}
	if len(tbl.rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(tbl.rows))
	}
}

// stripANSI removes ANSI escape sequences from a string for measuring visible width.
func stripANSI(s string) string {
	var result []byte
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
				inEscape = false
			}
			continue
		}
		result = append(result, s[i])
	}
	return string(result)
}
