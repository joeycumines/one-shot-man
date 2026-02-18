package scripting

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
)

// BenchmarkComputePathLCA benchmarks computePathLCA with varying numbers of
// paths and nesting depths. This is a pure-function benchmark (no I/O, no
// shared state), so b.RunParallel is used for all sub-benchmarks.
func BenchmarkComputePathLCA(b *testing.B) {
	// makePaths generates n realistic file paths under a common prefix
	// with the given directory depth.
	makePaths := func(n, depth int) []string {
		paths := make([]string, n)
		for i := 0; i < n; i++ {
			// Build a directory chain: dir0/dir1/.../dirN
			parts := make([]string, depth+1) // depth dirs + 1 filename
			for d := 0; d < depth; d++ {
				// Vary the last directory component to create divergence
				if d == depth-1 {
					parts[d] = fmt.Sprintf("pkg%d", i%7)
				} else {
					parts[d] = fmt.Sprintf("dir%d", d)
				}
			}
			parts[depth] = fmt.Sprintf("file%d.go", i)
			paths[i] = filepath.Join(parts...)
		}
		return paths
	}

	b.Run("Single", func(b *testing.B) {
		b.ReportAllocs()
		paths := makePaths(1, 3)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = computePathLCA(paths)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("TwoPaths", func(b *testing.B) {
		b.ReportAllocs()
		paths := makePaths(2, 3)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = computePathLCA(paths)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("TenPaths", func(b *testing.B) {
		b.ReportAllocs()
		paths := makePaths(10, 3)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = computePathLCA(paths)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("FiftyPaths", func(b *testing.B) {
		b.ReportAllocs()
		paths := makePaths(50, 4)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = computePathLCA(paths)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("HundredPaths", func(b *testing.B) {
		b.ReportAllocs()
		paths := makePaths(100, 4)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = computePathLCA(paths)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("DeepPaths", func(b *testing.B) {
		b.ReportAllocs()
		// 20 paths with 10 levels of nesting.
		paths := makePaths(20, 10)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = computePathLCA(paths)
			}
			runtime.KeepAlive(r)
		})
	})
}
