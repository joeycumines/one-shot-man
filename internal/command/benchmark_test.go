package command

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// BenchmarkGoalDiscovery benchmarks goal path discovery.
// Expected performance class: ~100-500μs/op (filesystem stat calls, symlink
// resolution, path normalization). Heavily influenced by OS file cache state.
func BenchmarkGoalDiscovery(b *testing.B) {
	b.Run("DiscoverGoalPaths", func(b *testing.B) {
		// Benchmark full goal discovery using a controlled config with standard
		// paths disabled to avoid filesystem noise from the host system.
		cfg := config.NewConfig()
		cfg.SetGlobalOption("goal.disable-standard-paths", "true")
		cfg.SetGlobalOption("goal.autodiscovery", "false")

		// Create a temp directory with goal subdirectories.
		dir := b.TempDir()
		goalDir := filepath.Join(dir, "osm-goals")
		os.MkdirAll(goalDir, 0755)
		// Write a few goal files to make it realistic.
		for _, name := range []string{"review.json", "document.json", "refactor.json"} {
			os.WriteFile(filepath.Join(goalDir, name), []byte(`{"name":"`+name+`"}`), 0644)
		}
		cfg.SetGlobalOption("goal.paths", goalDir)

		gd := NewGoalDiscovery(cfg)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			paths := gd.DiscoverGoalPaths()
			_ = paths
		}
	})

	b.Run("DiscoverPromptFilePaths", func(b *testing.B) {
		// Benchmark prompt file path discovery.
		cfg := config.NewConfig()
		cfg.SetGlobalOption("goal.disable-standard-paths", "true")

		dir := b.TempDir()
		promptDir := filepath.Join(dir, "prompts")
		os.MkdirAll(promptDir, 0755)
		cfg.SetGlobalOption("prompt.file-paths", promptDir)

		gd := NewGoalDiscovery(cfg)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			paths := gd.DiscoverPromptFilePaths()
			_ = paths
		}
	})

	b.Run("WithAutodiscovery", func(b *testing.B) {
		// Benchmark goal discovery with autodiscovery enabled (upward traversal).
		// Limited to 3 levels to avoid traversing the host filesystem extensively.
		cfg := config.NewConfig()
		cfg.SetGlobalOption("goal.disable-standard-paths", "true")
		cfg.SetGlobalOption("goal.autodiscovery", "true")
		cfg.SetGlobalOption("goal.max-traversal-depth", "3")

		// Create a nested directory structure with a goal directory.
		dir := b.TempDir()
		workDir := filepath.Join(dir, "project", "src", "pkg")
		goalDir := filepath.Join(dir, "project", "osm-goals")
		os.MkdirAll(workDir, 0755)
		os.MkdirAll(goalDir, 0755)
		os.WriteFile(filepath.Join(goalDir, "review.json"), []byte(`{}`), 0644)

		// Change to workDir so autodiscovery traverses upward.
		origDir, _ := os.Getwd()
		os.Chdir(workDir)
		b.Cleanup(func() { os.Chdir(origDir) })

		gd := NewGoalDiscovery(cfg)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			paths := gd.DiscoverGoalPaths()
			_ = paths
		}
	})
}

// BenchmarkScriptDiscovery benchmarks script path discovery.
// Expected performance class: ~50-500μs/op (filesystem stat calls, path normalization).
func BenchmarkScriptDiscovery(b *testing.B) {
	b.Run("DiscoverScriptPaths", func(b *testing.B) {
		// Benchmark script discovery with standard paths disabled.
		cfg := config.NewConfig()
		cfg.SetGlobalOption("script.disable-standard-paths", "true")
		cfg.SetGlobalOption("script.autodiscovery", "false")

		dir := b.TempDir()
		scriptDir := filepath.Join(dir, "scripts")
		os.MkdirAll(scriptDir, 0755)
		for _, name := range []string{"hello.js", "build.js", "deploy.js"} {
			os.WriteFile(filepath.Join(scriptDir, name), []byte(`console.log("ok")`), 0644)
		}
		cfg.SetGlobalOption("script.paths", scriptDir)

		sd := NewScriptDiscovery(cfg)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			paths := sd.DiscoverScriptPaths()
			_ = paths
		}
	})

	b.Run("WithAutodiscovery", func(b *testing.B) {
		// Benchmark script discovery with autodiscovery and upward traversal.
		cfg := config.NewConfig()
		cfg.SetGlobalOption("script.disable-standard-paths", "true")
		cfg.SetGlobalOption("script.autodiscovery", "true")
		cfg.SetGlobalOption("script.max-traversal-depth", "3")

		dir := b.TempDir()
		workDir := filepath.Join(dir, "project", "src")
		scriptDir := filepath.Join(dir, "project", "scripts")
		os.MkdirAll(workDir, 0755)
		os.MkdirAll(scriptDir, 0755)
		os.WriteFile(filepath.Join(scriptDir, "test.js"), []byte(`ok`), 0644)

		origDir, _ := os.Getwd()
		os.Chdir(workDir)
		b.Cleanup(func() { os.Chdir(origDir) })

		sd := NewScriptDiscovery(cfg)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			paths := sd.DiscoverScriptPaths()
			_ = paths
		}
	})
}

// BenchmarkPromptFileParsing benchmarks .prompt.md file parsing and conversion.
// Expected performance class: <5μs/op (pure string scanning, no I/O).
func BenchmarkPromptFileParsing(b *testing.B) {
	b.Run("ParseSimple", func(b *testing.B) {
		// Benchmark parsing a minimal .prompt.md file with frontmatter.
		data := []byte(`---
name: review
description: Code review assistant
---
Review the code for bugs and issues.
`)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			pf, err := ParsePromptFile(data)
			if err != nil {
				b.Fatal(err)
			}
			if pf.Name != "review" {
				b.Fatal("unexpected name")
			}
		}
	})

	b.Run("ParseComplex", func(b *testing.B) {
		// Benchmark parsing a complex .prompt.md with all frontmatter fields
		// and a multi-paragraph body.
		data := []byte(`---
name: full-review
description: Comprehensive code review with security analysis
model: claude-3.5-sonnet
tools:
  - codebase
  - terminal
  - browser
---
## Instructions

You are a senior code reviewer. Your task is to:

1. Review the provided code changes
2. Identify potential bugs and security vulnerabilities
3. Suggest improvements for code quality
4. Check for proper error handling

## Guidelines

- Focus on correctness over style
- Flag any potential data races
- Check for resource leaks (files, connections, goroutines)
- Verify error messages are actionable

## Output Format

Provide your review as a structured list of findings, each with:
- **Severity**: Critical, High, Medium, Low
- **Location**: File and line number
- **Issue**: Description of the problem
- **Suggestion**: Recommended fix
`)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			pf, err := ParsePromptFile(data)
			if err != nil {
				b.Fatal(err)
			}
			if pf.Name != "full-review" {
				b.Fatal("unexpected name")
			}
		}
	})

	b.Run("ParseNoFrontmatter", func(b *testing.B) {
		// Benchmark parsing a .prompt.md file without frontmatter (body only).
		data := []byte(`Just a plain prompt without any frontmatter.

Review the code and provide feedback.
`)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			pf, err := ParsePromptFile(data)
			if err != nil {
				b.Fatal(err)
			}
			_ = pf
		}
	})

	b.Run("PromptFileToGoal", func(b *testing.B) {
		// Benchmark converting a parsed PromptFile into a Goal.
		pf := &PromptFile{
			Name:        "code-review",
			Description: "Comprehensive code review assistant",
			Model:       "claude-3.5-sonnet",
			Tools:       []string{"codebase", "terminal"},
			Body:        "Review the following code changes for bugs and security issues.\n\n## Context\n\nFocus on Go code quality.",
			SourcePath:  "/home/user/prompts/code-review.prompt.md",
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			goal := PromptFileToGoal(pf)
			if goal.Name != "code-review" {
				b.Fatal("unexpected name")
			}
		}
	})

	b.Run("ExpandPromptFileReferences", func(b *testing.B) {
		// Benchmark expanding file references in a prompt body.
		// Uses a temp directory with actual files for reference resolution.
		dir := b.TempDir()
		os.WriteFile(filepath.Join(dir, "schema.sql"), []byte("CREATE TABLE users (id INT, name TEXT);"), 0644)
		os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("key: value\nother: data"), 0644)

		body := `Review these files:

[Database Schema](schema.sql)

[Configuration](config.yaml)

And check for consistency.
`
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result := expandPromptFileReferences(body, dir)
			_ = result
		}
	})
}

// BenchmarkPathScoring benchmarks the path scoring algorithm used by discovery.
// Expected performance class: <1μs/op (pure string ops, no I/O).
func BenchmarkPathScoring(b *testing.B) {
	b.Run("SplitPathSegments", func(b *testing.B) {
		path := filepath.Join("a", "b", "c", "d", "e")
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			segments := splitPathSegments(path)
			_ = segments
		}
	})

	b.Run("HasDirPrefix", func(b *testing.B) {
		path := filepath.Join("/", "home", "user", "project", "scripts")
		prefix := filepath.Join("/", "home", "user")
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = hasDirPrefix(path, prefix)
		}
	})

	b.Run("CountRelSegments", func(b *testing.B) {
		segments := []string{"..", "..", "project", "scripts", "tools"}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			up, down := countRelSegments(segments)
			_, _ = up, down
		}
	})
}

// BenchmarkNormalizePath benchmarks path normalization with symlink resolution.
// Expected performance class: ~5-50μs/op (EvalSymlinks + string ops).
func BenchmarkNormalizePath(b *testing.B) {
	b.Run("Simple", func(b *testing.B) {
		// Normalize a simple temp directory path (no symlinks involved).
		dir := b.TempDir()
		subDir := filepath.Join(dir, "sub")
		os.MkdirAll(subDir, 0755)

		cfg := config.NewConfig()
		gd := NewGoalDiscovery(cfg)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			p, err := gd.normalizePath(subDir)
			_, _ = p, err
		}
	})

	if runtime.GOOS != "windows" {
		b.Run("WithSymlink", func(b *testing.B) {
			dir := b.TempDir()
			realDir := filepath.Join(dir, "real")
			os.MkdirAll(realDir, 0755)
			linkDir := filepath.Join(dir, "link")
			if err := os.Symlink(realDir, linkDir); err != nil {
				b.Skipf("Symlink not supported: %v", err)
			}

			cfg := config.NewConfig()
			gd := NewGoalDiscovery(cfg)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p, err := gd.normalizePath(linkDir)
				_, _ = p, err
			}
		})
	}
}

// BenchmarkAnnotatedPaths benchmarks annotated path discovery.
func BenchmarkAnnotatedPaths(b *testing.B) {
	b.Run("GoalPaths", func(b *testing.B) {
		dir := b.TempDir()
		goalDir := filepath.Join(dir, "goals")
		os.MkdirAll(goalDir, 0755)
		for _, name := range []string{"review.json", "refactor.json"} {
			os.WriteFile(filepath.Join(goalDir, name), []byte(`{}`), 0644)
		}

		cfg := config.NewConfig()
		cfg.SetGlobalOption("goal.disable-standard-paths", "true")
		cfg.SetGlobalOption("goal.autodiscovery", "false")
		cfg.SetGlobalOption("goal.paths", goalDir)

		gd := NewGoalDiscovery(cfg)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			paths := gd.DiscoverAnnotatedGoalPaths()
			_ = paths
		}
	})

	b.Run("ScriptPaths", func(b *testing.B) {
		dir := b.TempDir()
		scriptDir := filepath.Join(dir, "scripts")
		os.MkdirAll(scriptDir, 0755)
		for _, name := range []string{"build.js", "deploy.js"} {
			os.WriteFile(filepath.Join(scriptDir, name), []byte(`ok`), 0644)
		}

		cfg := config.NewConfig()
		cfg.SetGlobalOption("script.disable-standard-paths", "true")
		cfg.SetGlobalOption("script.autodiscovery", "false")
		cfg.SetGlobalOption("script.paths", scriptDir)

		sd := NewScriptDiscovery(cfg)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			paths := sd.DiscoverAnnotatedScriptPaths()
			_ = paths
		}
	})
}
