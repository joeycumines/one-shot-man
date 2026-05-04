package session

import (
	"runtime"
	"strings"
	"testing"
)

// BenchmarkSanitizePayload benchmarks the sanitizePayload function, which uses
// a simple rune-by-rune whitelist scan (no regex). This is expected to be very
// fast; the benchmark captures the per-byte cost and allocation behaviour.
func BenchmarkSanitizePayload(b *testing.B) {
	b.Run("SafeInput", func(b *testing.B) {
		b.ReportAllocs()
		input := "hello-world_123.session"
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = sanitizePayload(input)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("UnicodeInput", func(b *testing.B) {
		b.ReportAllocs()
		// Multi-byte runes that are NOT in the safe whitelist — every rune is replaced.
		input := "ñoño-café-résumé-日本語テスト-العربية"
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = sanitizePayload(input)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("MixedInput", func(b *testing.B) {
		b.ReportAllocs()
		// Mix of safe and unsafe characters to exercise both branches.
		input := "hello/world:test*foo?bar<baz>qux|end"
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = sanitizePayload(input)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("LargeInput", func(b *testing.B) {
		b.ReportAllocs()
		// 10KB payload with a mix of safe and unsafe characters.
		base := "abcDEF.012-_/\\:*?\"<>|"
		input := strings.Repeat(base, (10*1024/len(base))+1)[:10*1024]
		b.SetBytes(int64(len(input)))
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = sanitizePayload(input)
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
				r = sanitizePayload("")
			}
			runtime.KeepAlive(r)
		})
	})
}
