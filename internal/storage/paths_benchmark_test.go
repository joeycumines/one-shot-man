package storage

import (
	"runtime"
	"strings"
	"testing"
)

// BenchmarkSanitizeFilename benchmarks the sanitizeFilename function at
// various input profiles. The three regex patterns (unsafePattern,
// collapsePattern, reservedPattern) are compiled once at package level.
func BenchmarkSanitizeFilename(b *testing.B) {
	b.Run("SafeInput", func(b *testing.B) {
		b.ReportAllocs()
		input := "hello-world_123.session"
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = sanitizeFilename(input)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("UnicodeInput", func(b *testing.B) {
		b.ReportAllocs()
		input := "ñoño-café_résumé-日本語テスト"
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = sanitizeFilename(input)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("PathTraversal", func(b *testing.B) {
		b.ReportAllocs()
		input := "../../etc/passwd"
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = sanitizeFilename(input)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("ReservedName", func(b *testing.B) {
		b.ReportAllocs()
		input := "CON.txt"
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = sanitizeFilename(input)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("LongInput", func(b *testing.B) {
		b.ReportAllocs()
		// 1000 characters of mixed safe and unsafe content.
		input := strings.Repeat("abc/def\\ghi:jkl*mno?pqr", 50)[:1000]
		b.SetBytes(int64(len(input)))
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = sanitizeFilename(input)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("EmptyInput", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = sanitizeFilename("")
			}
			runtime.KeepAlive(r)
		})
	})
}
