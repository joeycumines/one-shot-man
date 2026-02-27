package mux

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkVTerm_Write_ASCII(b *testing.B) {
	data := []byte(strings.Repeat("The quick brown fox jumps over the lazy dog.\n", 100))
	vt := NewVTerm(24, 80)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vt.Write(data)
	}
}

func BenchmarkVTerm_Write_UTF8(b *testing.B) {
	data := []byte(strings.Repeat("日本語テスト こんにちは世界 🤖🎉\n", 50))
	vt := NewVTerm(24, 80)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vt.Write(data)
	}
}

func BenchmarkVTerm_Write_EscapeSequences(b *testing.B) {
	// Simulates typical terminal output with colors and cursor movement.
	var buf []byte
	for i := 0; i < 100; i++ {
		buf = append(buf, []byte(fmt.Sprintf("\x1b[%d;1H\x1b[38;5;%dmLine %d: some content here\x1b[0m\n", i%24+1, i%256, i))...)
	}
	vt := NewVTerm(24, 80)
	b.SetBytes(int64(len(buf)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vt.Write(buf)
	}
}

func BenchmarkVTerm_Render_Sparse(b *testing.B) {
	vt := NewVTerm(200, 80)
	vt.Write([]byte("Hello, World!"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vt.Render()
	}
}

func BenchmarkVTerm_Render_Dense(b *testing.B) {
	vt := NewVTerm(24, 80)
	// Fill every row with text.
	for row := 0; row < 24; row++ {
		vt.Write([]byte(fmt.Sprintf("\x1b[%d;1H%s", row+1, strings.Repeat("X", 80))))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vt.Render()
	}
}

func BenchmarkVTerm_Render_WithSGR(b *testing.B) {
	vt := NewVTerm(24, 80)
	// Fill with colored text — alternating attributes.
	for row := 0; row < 24; row++ {
		for col := 0; col < 80; col++ {
			vt.Write([]byte(fmt.Sprintf("\x1b[%d;%dH\x1b[38;5;%dm%c", row+1, col+1, (row*80+col)%256, 'A'+byte(col%26))))
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vt.Render()
	}
}

func BenchmarkVTerm_Resize(b *testing.B) {
	vt := NewVTerm(24, 80)
	vt.Write([]byte("Some initial content\x1b[5;10HMore text"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			vt.Resize(40, 120)
		} else {
			vt.Resize(24, 80)
		}
	}
}

func BenchmarkVTerm_WriteRender_Cycle(b *testing.B) {
	data := []byte("\x1b[2J\x1b[1;1HStatus: OK\x1b[2;1H\x1b[32mAll systems nominal\x1b[0m")
	vt := NewVTerm(24, 80)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vt.Write(data)
		_ = vt.Render()
	}
}
