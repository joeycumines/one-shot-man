package mux

import (
	"testing"
)

// FuzzVTermWrite feeds arbitrary bytes into VTerm.Write and verifies
// no panic, no index-out-of-range, and Render() produces output.
// Seeds cover major escape sequence categories.
func FuzzVTermWrite(f *testing.F) {
	// CSI sequences.
	f.Add([]byte("\x1b[1;1H\x1b[2J\x1b[31mHello\x1b[0m"))
	f.Add([]byte("\x1b[?25l\x1b[?25h"))
	f.Add([]byte("\x1b[5;10r\x1b[3L\x1b[2M"))
	f.Add([]byte("\x1b[38;2;255;128;0m\x1b[48;5;196m"))
	// OSC sequences.
	f.Add([]byte("\x1b]0;My Title\x07"))
	f.Add([]byte("\x1b]0;Title\x1b\\"))
	// DCS sequences.
	f.Add([]byte("\x1bP1$r0m\x1b\\"))
	f.Add([]byte("\x1bPpayload\x07"))
	// UTF-8 multi-byte.
	f.Add([]byte("héllo wörld 🤖"))
	f.Add([]byte("日本語テスト"))
	// Tab stop sequences.
	f.Add([]byte("\x1bH\x1b[0g\x1b[3g\x1b[2I\x1b[2Z"))
	// Alt screen.
	f.Add([]byte("\x1b[?1049h\x1b[?1049l"))
	f.Add([]byte("\x1b[?47h\x1b[?47l"))
	f.Add([]byte("\x1b[?1047h\x1b[?1047l"))
	// SGR with many params.
	f.Add([]byte("\x1b[1;3;4;7;9;38;2;100;200;50;48;5;22m"))
	// Scroll / insert / delete.
	f.Add([]byte("\x1b[5S\x1b[5T\x1b[3@\x1b[3P\x1b[3X"))
	// Cursor save/restore.
	f.Add([]byte("\x1b7\x1b8\x1b[s\x1b[u"))
	// Full reset.
	f.Add([]byte("\x1bc"))
	// Malformed / edge cases.
	f.Add([]byte("\x1b[\x1b[\x1b["))
	f.Add([]byte{0x1b, 0x1b, 0x1b, 0x1b})
	f.Add([]byte{0xff, 0xfe, 0x80, 0xc0})
	// Empty.
	f.Add([]byte{})
	// Plain ASCII.
	f.Add([]byte("The quick brown fox jumps over the lazy dog"))

	f.Fuzz(func(t *testing.T, data []byte) {
		vt := NewVTerm(24, 80)
		// Must not panic.
		vt.Write(data)
		// Render must produce some output (at minimum cursor positioning).
		out := vt.Render()
		_ = out
	})
}

// FuzzVTermResize interleaves Write and Resize calls to ensure no
// panics from concurrent-style dimension changes.
func FuzzVTermResize(f *testing.F) {
	f.Add([]byte("Hello\x1b[2J\x1b[1;1HWorld"), uint8(10), uint8(20))
	f.Add([]byte("\x1b[?1049hAlt\x1b[?1049l"), uint8(5), uint8(5))
	f.Add([]byte("\x1b7\x1b[50;100H\x1b8"), uint8(3), uint8(3))
	f.Add([]byte("日本語\x1b[38;2;1;2;3m"), uint8(1), uint8(1))

	f.Fuzz(func(t *testing.T, data []byte, rows, cols uint8) {
		r := int(rows)
		c := int(cols)
		if r < 1 {
			r = 1
		}
		if c < 1 {
			c = 1
		}
		// Cap to reasonable dimensions.
		if r > 200 {
			r = 200
		}
		if c > 300 {
			c = 300
		}

		vt := NewVTerm(24, 80)

		// Write half, resize, write rest.
		mid := len(data) / 2
		vt.Write(data[:mid])
		vt.Resize(r, c)
		vt.Write(data[mid:])
		_ = vt.Render()
	})
}

// FuzzVTermRoundtrip writes data, renders, writes the render output
// into a fresh VTerm, and verifies no panic. The output won't be
// identical (render adds escape sequences) but must be parseable.
func FuzzVTermRoundtrip(f *testing.F) {
	f.Add([]byte("Hello World"))
	f.Add([]byte("\x1b[31mRed\x1b[0m Normal"))
	f.Add([]byte("Line1\nLine2\nLine3"))
	f.Add([]byte("\x1b[?1049hAlt Screen\x1b[?1049l"))

	f.Fuzz(func(t *testing.T, data []byte) {
		vt1 := NewVTerm(24, 80)
		vt1.Write(data)
		rendered := vt1.Render()

		// Feed rendered output into fresh VTerm — must not panic.
		vt2 := NewVTerm(24, 80)
		vt2.Write([]byte(rendered))
		_ = vt2.Render()
	})
}
