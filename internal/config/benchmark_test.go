package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BenchmarkSetKeyInFile benchmarks the SetKeyInFile config writer.
// SetKeyInFile involves disk I/O (ReadFile + AtomicWriteFile with fsync).
// Expected performance class: ~5ms/op (dominated by fsync in AtomicWriteFile,
// consistent with BenchmarkFileSystemIO/AtomicWriteFile in benchmark_test.go).
func BenchmarkSetKeyInFile(b *testing.B) {
	b.Run("UpdateExisting", func(b *testing.B) {
		// Benchmark updating an existing key in a config file with 3 global
		// options and 1 section. Measures: ReadFile + line scan + replace +
		// AtomicWriteFile.
		dir := b.TempDir()
		path := filepath.Join(dir, "config")
		initial := "key1 value1\nkey2 value2\nkey3 value3\n"
		if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := SetKeyInFile(path, "key2", "updated-value"); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("InsertBeforeSection", func(b *testing.B) {
		// Benchmark inserting a new key before the first [section] header.
		// After each iteration the key exists, so subsequent iterations become
		// updates. The first iteration is the true insert path.
		dir := b.TempDir()
		path := filepath.Join(dir, "config")
		initial := "key1 value1\n\n[section]\noption value\n"
		if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := SetKeyInFile(path, "new-key", "new-value"); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("CreateFile", func(b *testing.B) {
		// Benchmark creating a new config file from scratch (os.IsNotExist path).
		dir := b.TempDir()

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			path := filepath.Join(dir, fmt.Sprintf("config-%d", i))
			if err := SetKeyInFile(path, "key", "value"); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("LargeConfig", func(b *testing.B) {
		// Benchmark updating a key in a large config file (~100 lines).
		dir := b.TempDir()
		path := filepath.Join(dir, "config")

		var sb strings.Builder
		sb.WriteString("# Main configuration\n\n")
		for j := 0; j < 30; j++ {
			sb.WriteString(fmt.Sprintf("option-%d value-%d\n", j, j))
		}
		sb.WriteString("\n[section1]\n")
		for j := 0; j < 20; j++ {
			sb.WriteString(fmt.Sprintf("sec1-opt%d value%d\n", j, j))
		}
		sb.WriteString("\n[section2]\n")
		for j := 0; j < 20; j++ {
			sb.WriteString(fmt.Sprintf("sec2-opt%d value%d\n", j, j))
		}
		if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := SetKeyInFile(path, "option-15", "updated"); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkValidateConfig benchmarks config validation against a schema.
// Expected performance class: <10μs/op (pure in-memory map iteration + string ops).
func BenchmarkValidateConfig(b *testing.B) {
	b.Run("ValidConfig", func(b *testing.B) {
		// Benchmark validating a config with all valid options.
		schema := DefaultSchema()
		cfg := NewConfig()
		cfg.SetGlobalOption("verbose", "true")
		cfg.SetGlobalOption("color", "auto")
		cfg.SetGlobalOption("log.level", "info")
		cfg.SetGlobalOption("log.buffer-size", "1000")
		cfg.SetGlobalOption("goal.autodiscovery", "true")
		cfg.SetGlobalOption("script.max-traversal-depth", "10")
		cfg.SetCommandOption("sessions", "maxAgeDays", "90")
		cfg.SetCommandOption("sessions", "autoCleanupEnabled", "true")

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			issues := ValidateConfig(cfg, schema)
			if len(issues) != 0 {
				b.Fatalf("unexpected issues: %v", issues)
			}
		}
	})

	b.Run("ConfigWithErrors", func(b *testing.B) {
		// Benchmark validating a config with type errors and unknown keys.
		schema := DefaultSchema()
		cfg := NewConfig()
		cfg.SetGlobalOption("verbose", "not-a-bool")
		cfg.SetGlobalOption("log.buffer-size", "not-an-int")
		cfg.SetGlobalOption("unknown-key", "value")
		cfg.SetCommandOption("sessions", "maxAgeDays", "not-an-int")
		cfg.SetCommandOption("sessions", "unknown-option", "value")

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			issues := ValidateConfig(cfg, schema)
			if len(issues) == 0 {
				b.Fatal("expected validation issues")
			}
		}
	})

	b.Run("ManyOptions", func(b *testing.B) {
		// Benchmark validating a config with many valid options across sections.
		schema := DefaultSchema()
		cfg := NewConfig()
		// Fill with all default global options
		for _, opt := range schema.GlobalOptions() {
			if opt.Default != "" {
				cfg.SetGlobalOption(opt.Key, opt.Default)
			}
		}
		// Fill with all default command options
		for _, sec := range schema.Sections() {
			for _, opt := range schema.SectionOptions(sec) {
				if opt.Default != "" {
					cfg.SetCommandOption(sec, opt.Key, opt.Default)
				}
			}
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = ValidateConfig(cfg, schema)
		}
	})
}

// BenchmarkLoadFromReaderComplex benchmarks config loading with a realistic,
// multi-section config file.
// Expected performance class: ~5-15μs/op (string scanning + map insertion).
func BenchmarkLoadFromReaderComplex(b *testing.B) {
	b.Run("RealisticConfig", func(b *testing.B) {
		// A realistic config file with comments, multiple sections, and various
		// option types mimicking a production osm configuration.
		configContent := `# One-Shot-Man Configuration
# Generated by osm init

verbose false
color auto
editor vim
debug false
quiet false

# Script discovery
script.autodiscovery true
script.git-traversal false
script.max-traversal-depth 10
script.paths ~/my-scripts:/opt/scripts
script.path-patterns scripts

# Goal discovery
goal.autodiscovery true
goal.max-traversal-depth 10
goal.paths ~/my-goals
goal.path-patterns osm-goals,goals

# Logging
log.file /var/log/osm.log
log.level info
log.max-size-mb 10
log.max-files 5
log.buffer-size 1000

# Sync
sync.repository git@github.com:user/osm-config.git

[sessions]
maxAgeDays 90
maxCount 100
maxSizeMB 500
autoCleanupEnabled true
cleanupIntervalHours 24

[prompt]
template default
output clipboard
editor vim

[help]
pager less
format text

[version]
format text
`
		reader := strings.NewReader(configContent)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cfg, err := LoadFromReader(reader)
			if err != nil {
				b.Fatalf("failed to load config: %v", err)
			}
			if cfg == nil {
				b.Fatal("nil config")
			}
			reader.Reset(configContent)
		}
	})

	b.Run("MinimalConfig", func(b *testing.B) {
		// Minimal config: single key-value pair.
		configContent := "verbose true\n"
		reader := strings.NewReader(configContent)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := LoadFromReader(reader)
			if err != nil {
				b.Fatal(err)
			}
			reader.Reset(configContent)
		}
	})
}
