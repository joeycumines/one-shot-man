package command

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestResolveLogConfig_Defaults(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()

	lc, err := resolveLogConfig("", "info", 1000, cfg)
	if err != nil {
		t.Fatalf("resolveLogConfig: %v", err)
	}
	if lc.logFile != nil {
		t.Fatal("expected nil logFile when no path specified")
	}
	if lc.level != slog.LevelInfo {
		t.Fatalf("expected level Info, got %v", lc.level)
	}
	if lc.bufferSize != 1000 {
		t.Fatalf("expected bufferSize 1000, got %d", lc.bufferSize)
	}
}

func TestResolveLogConfig_FlagOverridesConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	cfg := config.NewConfig()
	cfg.SetGlobalOption("log.level", "warn")
	cfg.SetGlobalOption("log.file", "/should/not/use/this")

	lc, err := resolveLogConfig(logPath, "debug", 500, cfg)
	if err != nil {
		t.Fatalf("resolveLogConfig: %v", err)
	}
	defer func() {
		if lc.logFile != nil {
			lc.logFile.Close()
		}
	}()

	if lc.level != slog.LevelDebug {
		t.Fatalf("expected level Debug (flag override), got %v", lc.level)
	}
	if lc.logFile == nil {
		t.Fatal("expected logFile from flag path")
	}
	if lc.bufferSize != 500 {
		t.Fatalf("expected bufferSize 500 (flag override), got %d", lc.bufferSize)
	}
}

func TestResolveLogConfig_ConfigFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "config-log.log")

	cfg := config.NewConfig()
	cfg.SetGlobalOption("log.file", logPath)
	cfg.SetGlobalOption("log.level", "warn")
	cfg.SetGlobalOption("log.buffer-size", "2000")
	cfg.SetGlobalOption("log.max-size-mb", "5")
	cfg.SetGlobalOption("log.max-files", "3")

	// Empty flag values should fall back to config.
	lc, err := resolveLogConfig("", "info", 0, cfg)
	if err != nil {
		t.Fatalf("resolveLogConfig: %v", err)
	}
	defer func() {
		if lc.logFile != nil {
			lc.logFile.Close()
		}
	}()

	if lc.level != slog.LevelWarn {
		t.Fatalf("expected level Warn (config fallback), got %v", lc.level)
	}
	if lc.logFile == nil {
		t.Fatal("expected logFile from config fallback")
	}
	if lc.bufferSize != 2000 {
		t.Fatalf("expected bufferSize 2000 (config fallback), got %d", lc.bufferSize)
	}

	// Verify the file was created.
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("expected log file to be created: %v", err)
	}
}

func TestResolveLogConfig_InvalidLevel(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()

	_, err := resolveLogConfig("", "invalid", 1000, cfg)
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
}

func TestResolveLogConfig_NilConfig(t *testing.T) {
	t.Parallel()
	lc, err := resolveLogConfig("", "", 0, nil)
	if err != nil {
		t.Fatalf("resolveLogConfig with nil cfg: %v", err)
	}
	if lc.logFile != nil {
		t.Fatal("expected nil logFile with nil config and no flags")
	}
	if lc.level != slog.LevelInfo {
		t.Fatalf("expected default LevelInfo, got %v", lc.level)
	}
	if lc.bufferSize != 1000 {
		t.Fatalf("expected default bufferSize 1000, got %d", lc.bufferSize)
	}
}

func TestResolveLogConfig_RotatingWriter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "rotating.log")

	cfg := config.NewConfig()
	cfg.SetGlobalOption("log.max-size-mb", "1")
	cfg.SetGlobalOption("log.max-files", "2")

	lc, err := resolveLogConfig(logPath, "info", 1000, cfg)
	if err != nil {
		t.Fatalf("resolveLogConfig: %v", err)
	}
	defer func() {
		if lc.logFile != nil {
			lc.logFile.Close()
		}
	}()

	if lc.logFile == nil {
		t.Fatal("expected logFile to be created")
	}

	// Write some data to verify it works.
	msg := []byte("test log entry\n")
	n, err := lc.logFile.Write(msg)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(msg) {
		t.Fatalf("Write returned %d, want %d", n, len(msg))
	}
}
