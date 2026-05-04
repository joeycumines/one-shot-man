package vt

import (
	"strings"
	"testing"
)

// ── T086: Benchmark VTerm.Write() throughput ───────────────────────

var benchInputASCII []byte
var benchInputANSI []byte
var benchInputUTF8 []byte

func init() {
	// ~4KB realistic terminal output with ANSI sequences.
	var sb strings.Builder
	for range 100 {
		sb.WriteString("\x1b[1;32m$ \x1b[0mecho 'Hello, World!'\r\n")
		sb.WriteString("Hello, World!\r\n")
	}
	benchInputANSI = []byte(sb.String())

	// ~4KB of plain ASCII text.
	sb.Reset()
	line := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 2) + "\r\n"
	for len(sb.String()) < 4096 {
		sb.WriteString(line)
	}
	benchInputASCII = []byte(sb.String())

	// ~4KB of CJK text.
	sb.Reset()
	cjk := "漢字テスト日本語の文章を書いています。"
	for len(sb.String()) < 4096 {
		sb.WriteString(cjk)
		sb.WriteString("\r\n")
	}
	benchInputUTF8 = []byte(sb.String())
}

func BenchmarkVTermWrite_ANSI(b *testing.B) {
	vt := NewVTerm(24, 80)
	b.SetBytes(int64(len(benchInputANSI)))
	b.ResetTimer()
	for b.Loop() {
		vt.Write(benchInputANSI)
	}
}

func BenchmarkVTermWrite_ASCII(b *testing.B) {
	vt := NewVTerm(24, 80)
	b.SetBytes(int64(len(benchInputASCII)))
	b.ResetTimer()
	for b.Loop() {
		vt.Write(benchInputASCII)
	}
}

func BenchmarkVTermWrite_UTF8(b *testing.B) {
	vt := NewVTerm(24, 80)
	b.SetBytes(int64(len(benchInputUTF8)))
	b.ResetTimer()
	for b.Loop() {
		vt.Write(benchInputUTF8)
	}
}

func BenchmarkVTermRenderFullScreen(b *testing.B) {
	vt := NewVTerm(24, 80)
	// Fill screen with content.
	vt.Write(benchInputANSI)
	b.ResetTimer()
	for b.Loop() {
		_ = vt.RenderFullScreen()
	}
}

// ── T087: Benchmark SGR parsing ────────────────────────────────────

func BenchmarkParseSGR(b *testing.B) {
	// Complex SGR with truecolor + 256-color.
	sgr := []byte("\x1b[1;38;2;255;100;0;48;5;232m")
	vt := NewVTerm(4, 80)
	b.SetBytes(int64(len(sgr)))
	b.ResetTimer()
	for b.Loop() {
		vt.Write(sgr)
	}
}

func BenchmarkParseSGR_Simple(b *testing.B) {
	sgr := []byte("\x1b[1;31m")
	vt := NewVTerm(4, 80)
	b.SetBytes(int64(len(sgr)))
	b.ResetTimer()
	for b.Loop() {
		vt.Write(sgr)
	}
}
