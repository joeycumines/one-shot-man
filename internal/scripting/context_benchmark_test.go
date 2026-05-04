package scripting

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkContextManagerOps benchmarks the ContextManager's core operations
// (AddPath, RemovePath, RefreshPath, ListPaths, ToTxtar, GetStats).
//
// Most operations involve disk I/O (stat + read) so performance is bounded by
// filesystem latency.  These benchmarks use ramdisk-backed TempDir provided by
// the test harness to minimise noise from disk scheduling.
func BenchmarkContextManagerOps(b *testing.B) {
	// Helper: create n files of the given size in dir and return their paths.
	makeFiles := func(b *testing.B, dir string, n int, size int) []string {
		b.Helper()
		paths := make([]string, n)
		data := make([]byte, size)
		for i := range data {
			data[i] = 'x'
		}
		for i := 0; i < n; i++ {
			p := filepath.Join(dir, fmt.Sprintf("file-%03d.txt", i))
			if err := os.WriteFile(p, data, 0644); err != nil {
				b.Fatal(err)
			}
			paths[i] = p
		}
		return paths
	}

	b.Run("AddPath/SingleFile", func(b *testing.B) {
		dir := b.TempDir()
		files := makeFiles(b, dir, 1, 256)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cm, err := NewContextManager(dir)
			if err != nil {
				b.Fatal(err)
			}
			if err := cm.AddPath(files[0]); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("AddPath/SmallDirectory", func(b *testing.B) {
		// 10 files × 256B each.
		dir := b.TempDir()
		sub := filepath.Join(dir, "src")
		os.MkdirAll(sub, 0755)
		makeFiles(b, sub, 10, 256)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cm, err := NewContextManager(dir)
			if err != nil {
				b.Fatal(err)
			}
			if err := cm.AddPath(sub); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("AddPath/MediumDirectory", func(b *testing.B) {
		// 50 files × 1KB each, spread across 5 subdirectories.
		dir := b.TempDir()
		for j := 0; j < 5; j++ {
			sub := filepath.Join(dir, "pkg", fmt.Sprintf("mod%d", j))
			os.MkdirAll(sub, 0755)
			makeFiles(b, sub, 10, 1024)
		}
		pkgDir := filepath.Join(dir, "pkg")

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cm, err := NewContextManager(dir)
			if err != nil {
				b.Fatal(err)
			}
			if err := cm.AddPath(pkgDir); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("RemovePath", func(b *testing.B) {
		dir := b.TempDir()
		files := makeFiles(b, dir, 5, 256)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			cm, err := NewContextManager(dir)
			if err != nil {
				b.Fatal(err)
			}
			for _, f := range files {
				if err := cm.AddPath(f); err != nil {
					b.Fatal(err)
				}
			}
			b.StartTimer()

			// Remove all five files.
			for _, f := range files {
				_ = cm.RemovePath(f)
			}
		}
	})

	b.Run("RefreshPath", func(b *testing.B) {
		dir := b.TempDir()
		files := makeFiles(b, dir, 1, 512)
		cm, err := NewContextManager(dir)
		if err != nil {
			b.Fatal(err)
		}
		if err := cm.AddPath(files[0]); err != nil {
			b.Fatal(err)
		}

		// Determine owner key for this file.
		owners := cm.ListPaths()
		if len(owners) == 0 {
			b.Fatal("expected tracked paths")
		}
		owner := owners[0]

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := cm.RefreshPath(owner); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ListPaths", func(b *testing.B) {
		dir := b.TempDir()
		files := makeFiles(b, dir, 20, 128)
		cm, err := NewContextManager(dir)
		if err != nil {
			b.Fatal(err)
		}
		for _, f := range files {
			if err := cm.AddPath(f); err != nil {
				b.Fatal(err)
			}
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			paths := cm.ListPaths()
			if len(paths) != 20 {
				b.Fatalf("expected 20 paths, got %d", len(paths))
			}
		}
	})

	b.Run("GetStats", func(b *testing.B) {
		dir := b.TempDir()
		files := makeFiles(b, dir, 20, 128)
		cm, err := NewContextManager(dir)
		if err != nil {
			b.Fatal(err)
		}
		for _, f := range files {
			if err := cm.AddPath(f); err != nil {
				b.Fatal(err)
			}
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			stats := cm.GetStats()
			if stats["files"].(int) != 20 {
				b.Fatalf("expected 20 files, got %v", stats["files"])
			}
		}
	})
}

// BenchmarkToTxtar benchmarks the txtar serialization path with varying numbers
// of tracked files.  This is the hot path when building the final prompt.
//
// Expected performance class: O(n) in number of tracked files.  Each file is
// re-read from disk during ToTxtar to capture the latest content.
func BenchmarkToTxtar(b *testing.B) {
	setup := func(b *testing.B, fileCount, fileSize int) (*ContextManager, string) {
		b.Helper()
		dir := b.TempDir()
		data := make([]byte, fileSize)
		for i := range data {
			data[i] = byte('a' + (i % 26))
		}
		cm, err := NewContextManager(dir)
		if err != nil {
			b.Fatal(err)
		}
		for j := 0; j < fileCount; j++ {
			p := filepath.Join(dir, fmt.Sprintf("file-%03d.go", j))
			if err := os.WriteFile(p, data, 0644); err != nil {
				b.Fatal(err)
			}
			if err := cm.AddPath(p); err != nil {
				b.Fatal(err)
			}
		}
		return cm, dir
	}

	b.Run("5_Files_256B", func(b *testing.B) {
		cm, _ := setup(b, 5, 256)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			archive := cm.ToTxtar()
			if len(archive.Files) != 5 {
				b.Fatalf("expected 5 files, got %d", len(archive.Files))
			}
		}
	})

	b.Run("20_Files_1KB", func(b *testing.B) {
		cm, _ := setup(b, 20, 1024)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			archive := cm.ToTxtar()
			if len(archive.Files) != 20 {
				b.Fatalf("expected 20 files, got %d", len(archive.Files))
			}
		}
	})

	b.Run("50_Files_4KB", func(b *testing.B) {
		cm, _ := setup(b, 50, 4096)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			archive := cm.ToTxtar()
			if len(archive.Files) != 50 {
				b.Fatalf("expected 50 files, got %d", len(archive.Files))
			}
		}
	})

	b.Run("GetTxtarString/20_Files", func(b *testing.B) {
		cm, _ := setup(b, 20, 512)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s := cm.GetTxtarString()
			if len(s) == 0 {
				b.Fatal("empty txtar string")
			}
		}
	})
}

// BenchmarkContextManagerFilter benchmarks FilterPaths and GetFilesByExtension.
// Expected performance class: <5μs/op for 20 paths (linear scan with glob match).
func BenchmarkContextManagerFilter(b *testing.B) {
	dir := b.TempDir()
	cm, err := NewContextManager(dir)
	if err != nil {
		b.Fatal(err)
	}
	// Create files with mixed extensions.
	for i := 0; i < 10; i++ {
		p := filepath.Join(dir, fmt.Sprintf("src-%d.go", i))
		os.WriteFile(p, []byte("package main"), 0644)
		cm.AddPath(p)
	}
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, fmt.Sprintf("data-%d.json", i))
		os.WriteFile(p, []byte("{}"), 0644)
		cm.AddPath(p)
	}
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, fmt.Sprintf("doc-%d.md", i))
		os.WriteFile(p, []byte("# doc"), 0644)
		cm.AddPath(p)
	}

	b.Run("FilterPaths/Glob", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			matches, err := cm.FilterPaths("*.go")
			if err != nil {
				b.Fatal(err)
			}
			_ = matches
		}
	})

	b.Run("GetFilesByExtension", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			files := cm.GetFilesByExtension(".go")
			_ = files
		}
	})
}
