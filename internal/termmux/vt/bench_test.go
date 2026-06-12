package vt

import (
	"strconv"
	"strings"
	"testing"
)

// ── T086: Benchmark VTerm.Write() throughput ───────────────────────
//
// Baseline (Apple M2 Pro, darwin/arm64, 2026-06-13):
//
//	 BenchmarkVTermWrite_ANSI-10         4441   247536 ns/op   20.20 MB/s   1083392 B/op    899 allocs/op
//	 BenchmarkVTermWrite_ASCII-10        7683   140293 ns/op   29.51 MB/s   483951 B/op     179 allocs/op
//	 BenchmarkVTermWrite_UTF8-10         7596   143228 ns/op   28.84 MB/s   376432 B/op     139 allocs/op
//	 BenchmarkVTermRenderFullScreen-10   74031    37906 ns/op    5379 B/op     109 allocs/op
//
// Throughput (T018):
//
//	 BenchmarkVTerm_PlainASCII-10        12255   112978 ns/op    8.85 MB/s    67270 B/op      25 allocs/op
//	 BenchmarkVTerm_MixedTextCSI-10      10000   138502 ns/op   15.16 MB/s     5120 B/op     300 allocs/op
//	 BenchmarkVTerm_FullScreenRedraw-10   1279  1765358 ns/op   16.33 MB/s   194472 B/op   11520 allocs/op
//	 BenchmarkVTerm_RapidSmallWrites-10   6481   174287 ns/op    5.74 MB/s    67331 B/op      24 allocs/op
//	 BenchmarkVTerm_UTF8MultiByte-10     13435   112479 ns/op    9.10 MB/s    45893 B/op      17 allocs/op
//	 BenchmarkVTerm_ParserOnly-10           30 46471140 ns/op    0.18 MB/s 30937142 B/op   13510 allocs/op
//
// Allocation (T019):
//
//	 BenchmarkAlloc_ParserParams-10   1561304     684.4 ns/op      216 B/op       3 allocs/op
//	 BenchmarkAlloc_Snapshot-10        29080    50232 ns/op    65739 B/op      28 allocs/op
//	 BenchmarkAlloc_PutChar-10      21998737      58.17 ns/op       67 B/op       0 allocs/op
//	 BenchmarkAlloc_WriteGround-10    13068     88218 ns/op   131876 B/op      65 allocs/op
//	 BenchmarkAlloc_CSIDispatch-10  54480448      23.59 ns/op        0 B/op       0 allocs/op
//	 BenchmarkAlloc_ScrollUp-10      550798      2143 ns/op     5377 B/op       2 allocs/op

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

// ── T018: VTerm Throughput Benchmarks ──────────────────────────────

var (
	benchPlainASCII     []byte
	benchMixedTextCSI   []byte
	benchFullScreenRed  []byte
	benchRapidSingle    []byte
	benchUTF8MultiByte  []byte
	benchParserOnly     []byte
)

func init() {
	// Pre-build benchmark data in init so the hot loop only measures VTerm.

	// 1. Plain ASCII: 1000 characters of ground-state text.
	benchPlainASCII = make([]byte, 1000)
	for i := range benchPlainASCII {
		benchPlainASCII[i] = 'A' + byte(i%26)
	}

	// 2. Mixed text + CSI: cursor movement interleaved with short text.
	//    Pattern: CSI A n + "hello" repeated.
	var sb strings.Builder
	for range 100 {
		sb.WriteString("\x1b[A")   // CUU
		sb.WriteString("\x1b[B")   // CDD
		sb.WriteString("\x1b[1;1H") // CUP home
		sb.WriteString("hello\x1b[K") // text + EL
		sb.WriteByte('X')
	}
	benchMixedTextCSI = []byte(sb.String())

	// 3. Full-screen redraw: CSI row;colH + SGR + 80 printable chars per row, 24 rows.
	sb.Reset()
	for r := 1; r <= 24; r++ {
		for c := 1; c <= 80; c++ {
			sb.WriteString("\x1b[")
			sb.WriteString(strconv.Itoa(r))
			sb.WriteByte(';')
			sb.WriteString(strconv.Itoa(c))
			sb.WriteByte('H')
			switch (r + c) % 4 {
			case 0:
				sb.WriteString("\x1b[1;31m") // bold red
			case 1:
				sb.WriteString("\x1b[0;32m") // green
			case 2:
				sb.WriteString("\x1b[4;34m") // underlined blue
			default:
				sb.WriteString("\x1b[37m") // white
			}
			sb.WriteByte('A' + byte((r*80+c)%26))
		}
	}
	benchFullScreenRed = []byte(sb.String())

	// 4. Rapid small writes: 1000 individual 1-byte writes.
	benchRapidSingle = make([]byte, 1000)
	for i := range benchRapidSingle {
		benchRapidSingle[i] = byte('a' + i%26)
	}

	// 5. UTF-8 multi-byte: ~1000 bytes of CJK text.
	sb.Reset()
	cjk := "漢字テスト日本語の文章"
	for len(sb.String()) < 1000 {
		sb.WriteString(cjk)
	}
	benchUTF8MultiByte = []byte(sb.String())

	// 6. Parser-only: CSI sequences with no ground-state text.
	//    Measures raw parser throughput.
	sb.Reset()
	// Lots of small CSI sequences.
	for range 500 {
		sb.WriteString("\x1b[0m")  // SGR reset
		sb.WriteString("\x1b[1m")  // bold
		sb.WriteString("\x1b[K")   // erase line
		sb.WriteString("\x1b[J")   // erase display
		sb.WriteString("\x1b[H")   // cursor home
	}
	benchParserOnly = []byte(sb.String())
}

// BenchmarkVTerm_PlainASCII measures ground-state plain-text throughput.
func BenchmarkVTerm_PlainASCII(b *testing.B) {
	vt := NewVTerm(24, 80)
	b.SetBytes(int64(len(benchPlainASCII)))
	b.ResetTimer()
	for b.Loop() {
		vt.Write(benchPlainASCII)
	}
}

// BenchmarkVTerm_MixedTextCSI measures cursor movement + text interleaved.
func BenchmarkVTerm_MixedTextCSI(b *testing.B) {
	vt := NewVTerm(24, 80)
	b.SetBytes(int64(len(benchMixedTextCSI)))
	b.ResetTimer()
	for b.Loop() {
		vt.Write(benchMixedTextCSI)
	}
}

// BenchmarkVTerm_FullScreenRedraw simulates a full 24x80 screen redraw
// with CUP positioning, SGR colors, and text for every cell.
func BenchmarkVTerm_FullScreenRedraw(b *testing.B) {
	vt := NewVTerm(24, 80)
	b.SetBytes(int64(len(benchFullScreenRed)))
	b.ResetTimer()
	for b.Loop() {
		vt.Write(benchFullScreenRed)
	}
}

// BenchmarkVTerm_RapidSmallWrites measures throughput when writing
// one byte at a time (simulates extreme fragmentation).
func BenchmarkVTerm_RapidSmallWrites(b *testing.B) {
	vt := NewVTerm(24, 80)
	totalBytes := int64(len(benchRapidSingle))
	b.SetBytes(totalBytes)
	b.ResetTimer()
	for b.Loop() {
		for _, ch := range benchRapidSingle {
			vt.Write([]byte{ch})
		}
	}
}

// BenchmarkVTerm_UTF8MultiByte measures throughput with UTF-8
// multi-byte characters (CJK, Japanese, etc.).
func BenchmarkVTerm_UTF8MultiByte(b *testing.B) {
	vt := NewVTerm(24, 80)
	b.SetBytes(int64(len(benchUTF8MultiByte)))
	b.ResetTimer()
	for b.Loop() {
		vt.Write(benchUTF8MultiByte)
	}
}

// BenchmarkVTerm_ParserOnly measures raw CSI parser throughput
// by feeding only escape sequences with no ground-state text.
func BenchmarkVTerm_ParserOnly(b *testing.B) {
	vt := NewVTerm(24, 80)
	b.SetBytes(int64(len(benchParserOnly)))
	b.ResetTimer()
	for b.Loop() {
		vt.Write(benchParserOnly)
	}
}

// ── T019: Memory Allocation Benchmarks ─────────────────────────────

// BenchmarkAlloc_ParserParams measures allocation per CSI Params() call.
func BenchmarkAlloc_ParserParams(b *testing.B) {
	p := NewParser()
	// Feed a multi-param CSI sequence: \x1b[38;2;255;128;0;48;5;232m
	input := []byte("\x1b[38;2;255;128;0;48;5;232m")
	for _, ch := range input {
		p.Feed(ch)
	}
	params := p.Params()
	// Ensure params is non-nil to avoid escape
	_ = params
	b.ResetTimer()
	for b.Loop() {
		// Reset parser and re-parse.
		p.Reset()
		for _, ch := range input {
			p.Feed(ch)
		}
		_ = p.Params()
	}
}

// BenchmarkAlloc_Snapshot measures allocation per Screen.Snapshot().
func BenchmarkAlloc_Snapshot(b *testing.B) {
	scr := NewScreen(24, 80)
	// Fill with some content so snapshot copies data.
	for r := 0; r < 24; r++ {
		for c := 0; c < 80; c++ {
			scr.Cells[r][c].Ch = rune('A' + (r+c)%26)
		}
	}
	b.ResetTimer()
	for b.Loop() {
		_ = scr.Snapshot()
	}
}

// BenchmarkAlloc_PutChar measures allocation per Screen.PutChar() call.
// This exercises the uniseg.StringWidth path.
func BenchmarkAlloc_PutChar(b *testing.B) {
	scr := NewScreen(24, 80)
	b.ResetTimer()
	for b.Loop() {
		scr.PutChar('A')
	}
}

// BenchmarkAlloc_WriteGround measures VTerm.Write per-char overhead
// for ground-state printable characters.
func BenchmarkAlloc_WriteGround(b *testing.B) {
	data := []byte("Hello, World! This is a test of per-char overhead.")
	b.ResetTimer()
	for b.Loop() {
		vt := NewVTerm(24, 80)
		vt.Write(data)
	}
}

// BenchmarkAlloc_CSIDispatch measures CSI dispatch overhead.
func BenchmarkAlloc_CSIDispatch(b *testing.B) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// SGR: bold + underline
	params := []int{1, 4}
	b.ResetTimer()
	for b.Loop() {
		h.Dispatch(scr, 'm', params, false)
	}
}

// BenchmarkAlloc_ScrollUp measures Screen.ScrollUp allocation.
func BenchmarkAlloc_ScrollUp(b *testing.B) {
	scr := NewScreen(24, 80)
	// Pre-fill the screen with content so scroll has data to move.
	for r := 0; r < 24; r++ {
		for c := 0; c < 80; c++ {
			scr.Cells[r][c].Ch = rune('A' + (r+c)%26)
		}
	}
	b.ResetTimer()
	for b.Loop() {
		scr.ScrollUp(1)
	}
}
