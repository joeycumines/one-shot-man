package vt

import (
	"testing"
)

func TestDECALN_FillsScreenWithE(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VTerm integration test in short mode")
	}
	rows, cols := 5, 10
	v := NewVTerm(rows, cols)

	v.Write([]byte{0x1B, '#', '8'})

	for r := range rows {
		for c := range cols {
			cell := v.active.Cells[r][c]
			if cell.Ch != 'E' {
				t.Errorf("cell[%d][%d].Ch = %q, want 'E'", r, c, cell.Ch)
			}
		}
	}
}

func TestDECALN_PreservesCursorPosition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VTerm integration test in short mode")
	}
	v := NewVTerm(24, 80)

	// Set cursor to (5, 10)
	v.Write([]byte("\x1b[6;11H"))

	row, col := v.CursorPosition()
	if row != 5 || col != 10 {
		t.Fatalf("initial cursor = (%d, %d), want (5, 10)", row, col)
	}

	v.Write([]byte{0x1B, '#', '8'})

	row, col = v.CursorPosition()
	if row != 5 || col != 10 {
		t.Fatalf("cursor after DECALN = (%d, %d), want (5, 10)", row, col)
	}
}

func TestDECALN_DefaultAttributes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VTerm integration test in short mode")
	}
	rows, cols := 3, 5
	v := NewVTerm(rows, cols)

	// Write bold, red text
	v.Write([]byte("\x1b[1;31mHello"))

	// Verify bold red text was written
	s := v.active
	if !s.Cells[0][0].Attr.Bold {
		t.Fatal("cell[0][0] should be bold before DECALN")
	}

	// DECALN should clear all attributes
	v.Write([]byte{0x1B, '#', '8'})

	for r := range rows {
		for c := range cols {
			cell := v.active.Cells[r][c]
			if cell.Ch != 'E' {
				t.Errorf("cell[%d][%d].Ch = %q, want 'E'", r, c, cell.Ch)
			}
			if cell.Attr.Bold {
				t.Errorf("cell[%d][%d].Attr.Bold = true, want false (default)", r, c)
			}
		}
	}
}

func TestDECALN_OnAlternateScreen(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VTerm integration test in short mode")
	}
	rows, cols := 5, 10
	v := NewVTerm(rows, cols)

	// Write content to primary screen
	for c := range cols {
		v.Write([]byte{'A' + byte(c)})
	}
	v.Write([]byte("\n"))
	for c := range cols {
		v.Write([]byte{'1' + byte(c)})
	}

	// Switch to alternate screen (mode 1049)
	v.Write([]byte("\x1b[?1049h"))

	if v.active != v.alternate {
		t.Fatal("expected active screen to be alternate")
	}

	// DECALN on alternate screen
	v.Write([]byte{0x1B, '#', '8'})

	// Verify alternate screen is filled with 'E'
	for r := range rows {
		for c := range cols {
			cell := v.alternate.Cells[r][c]
			if cell.Ch != 'E' {
				t.Errorf("alt cell[%d][%d].Ch = %q, want 'E'", r, c, cell.Ch)
			}
		}
	}

	// Verify primary screen is unchanged
	if v.primary.Cells[0][0].Ch != 'A' {
		t.Error("primary screen should be unchanged after DECALN on alternate")
	}

	// Switch back to primary
	v.Write([]byte("\x1b[?1049l"))
	if v.active != v.primary {
		t.Fatal("expected active screen to be primary after 1049l")
	}
}

func TestDECALN_DoesNotAffectScrollback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VTerm integration test in short mode")
	}
	rows, cols := 3, 10
	v := NewVTerm(rows, cols)

	// Write enough lines to create scrollback
	for line := range 10 {
		for range cols {
			v.Write([]byte{'0' + byte(line)})
		}
		v.Write([]byte("\n"))
	}

	// Scrollback should have lines
	initialScrollback := v.ScrollbackLines()
	if initialScrollback == 0 {
		t.Fatal("expected scrollback lines before DECALN")
	}

	// Snapshot scrollback content
	snapshot := make([]string, initialScrollback)
	for i := range initialScrollback {
		row := v.primary.ScrollbackRow(i)
		var b []byte
		for _, cell := range row {
			if cell.Ch != ' ' && cell.Ch != 0 {
				b = append(b, byte(cell.Ch))
			}
		}
		snapshot[i] = string(b)
	}

	// DECALN on primary screen
	v.Write([]byte{0x1B, '#', '8'})

	// Verify scrollback is unchanged
	afterScrollback := v.ScrollbackLines()
	if afterScrollback != initialScrollback {
		t.Errorf("scrollback lines = %d, want %d", afterScrollback, initialScrollback)
	}

	for i := 0; i < initialScrollback && i < afterScrollback; i++ {
		row := v.primary.ScrollbackRow(i)
		var b []byte
		for _, cell := range row {
			if cell.Ch != ' ' && cell.Ch != 0 {
				b = append(b, byte(cell.Ch))
			}
		}
		if string(b) != snapshot[i] {
			t.Errorf("scrollback[%d] = %q, want %q", i, string(b), snapshot[i])
		}
	}

	// Verify visible screen is filled with 'E'
	for r := range rows {
		for c := range cols {
			cell := v.active.Cells[r][c]
			if cell.Ch != 'E' {
				t.Errorf("visible cell[%d][%d].Ch = %q, want 'E'", r, c, cell.Ch)
			}
		}
	}
}

func TestDECALN_ClearsRowWrapped(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VTerm integration test in short mode")
	}
	s := NewScreen(24, 80)
	s.RowWrapped[5] = true
	s.RowWrapped[10] = true
	s.FillScreen('E')
	for i, wrapped := range s.RowWrapped {
		if wrapped {
			t.Errorf("RowWrapped[%d] = true, want false after DECALN", i)
		}
	}
}
