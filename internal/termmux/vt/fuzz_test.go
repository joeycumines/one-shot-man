package vt

import (
	"testing"
	"unicode/utf8"
)

// ── T083: Fuzz test for ANSI parser ────────────────────────────────

func FuzzParser(f *testing.F) {
	// Seed with known ANSI sequences.
	f.Add([]byte("\x1b[1;31mRED\x1b[0m"))                     // SGR color
	f.Add([]byte("\x1b[2J\x1b[H"))                             // Clear screen + home
	f.Add([]byte("\x1b[?25h\x1b[?25l"))                        // Cursor show/hide
	f.Add([]byte("\x1b]0;Window Title\x07"))                    // OSC title BEL
	f.Add([]byte("\x1b]0;Window Title\x1b\\"))                  // OSC title ST
	f.Add([]byte("\x1bP+q544e\x1b\\"))                         // DCS
	f.Add([]byte("\x1b[1;38;2;255;100;0;48;5;232m"))           // 24-bit + 256 color
	f.Add([]byte("Hello, World!\r\n"))                          // Plain text
	f.Add([]byte("\x1b[10;20H\x1b[K\x1b[1m\x1b[4mtext\x1b[0m")) // Move + erase + SGR
	f.Add([]byte{0xFF, 0xFE, 0x00, 0x1B, 0x5B, 0x41})         // Garbage + valid CSI

	f.Fuzz(func(t *testing.T, data []byte) {
		p := NewParser()
		for _, b := range data {
			act, _ := p.Feed(b)
			_ = act
		}
		// After processing, state must be a valid enum.
		st := p.CurState()
		switch st {
		case StateGround, StateEscape, StateCSI, StateOSC, StateDCS:
			// valid
		default:
			t.Fatalf("invalid state %d after feeding %d bytes", st, len(data))
		}
		// Params must not panic.
		_ = p.Params()
		// Reset must not panic.
		p.Reset()
		if p.CurState() != StateGround {
			t.Fatal("reset did not return to ground")
		}
	})
}

// ── T084: Fuzz test for VTerm.Write() ──────────────────────────────

func FuzzVTermWrite(f *testing.F) {
	f.Add([]byte("Hello"))
	f.Add([]byte("\x1b[1;31mRed Bold\x1b[0m"))
	f.Add([]byte("\x1b[2J\x1b[HNew Content"))
	f.Add([]byte("漢字テスト"))
	f.Add([]byte("\r\n\r\n\r\n"))
	f.Add([]byte("\x1b[?1049h\x1b[2J\x1b[?1049l")) // Alt screen enter/exit
	f.Add([]byte{0x00, 0x01, 0x7F, 0x80, 0xC0, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		vt := NewVTerm(4, 10)
		n, err := vt.Write(data)
		if err != nil {
			t.Fatalf("Write error: %v", err)
		}
		if n != len(data) {
			t.Fatalf("Write returned %d; want %d", n, len(data))
		}
		// Render must not panic and produce valid UTF-8.
		rendered := vt.Render()
		if !utf8.ValidString(rendered) {
			t.Fatal("Render() produced invalid UTF-8")
		}
	})
}

// ── T085: Fuzz test for UTF-8 accumulation ─────────────────────────

func FuzzUTF8Accum(f *testing.F) {
	f.Add([]byte("abc"))
	f.Add([]byte{0xC3, 0xA9})         // é
	f.Add([]byte{0xE6, 0xBC, 0xA2})   // 漢
	f.Add([]byte{0xF0, 0x9F, 0x98, 0x80}) // 😀
	f.Add([]byte{0xFF, 0xFE})          // Invalid
	f.Add([]byte{0xC3})               // Truncated 2-byte
	f.Add([]byte{0xE6, 0xBC})         // Truncated 3-byte
	f.Add([]byte{0x80, 0x80, 0x80})   // Stray continuations

	f.Fuzz(func(t *testing.T, data []byte) {
		var a UTF8Accum
		for _, b := range data {
			r, complete := a.Feed(b)
			if complete {
				_ = r
			}
			// Pending must be consistent.
			_ = a.Pending()
		}
	})
}
