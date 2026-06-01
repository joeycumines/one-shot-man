//go:build unix

package vhs

import (
	"testing"
	"time"
)

func TestRecorderOptions_Defaults(t *testing.T) {
	cfg, err := resolveRecorderOptions(nil)
	if err != nil {
		t.Fatalf("resolveRecorderOptions returned error: %v", err)
	}

	if cfg.rows != 24 {
		t.Fatalf("expected default rows 24, got %d", cfg.rows)
	}
	if cfg.cols != 80 {
		t.Fatalf("expected default cols 80, got %d", cfg.cols)
	}
	if cfg.defaultTimeout != 30*time.Second {
		t.Fatalf("expected default timeout 30s, got %v", cfg.defaultTimeout)
	}
	if cfg.shell != "bash" {
		t.Fatalf("expected default shell bash, got %q", cfg.shell)
	}
	if cfg.skipTapeOutput {
		t.Fatalf("expected default skipTapeOutput false, got true")
	}
	if cfg.repoRoot != "" {
		t.Fatalf("expected default repoRoot empty, got %q", cfg.repoRoot)
	}
	if cfg.vhsConfig.PixelWidth != 1000 {
		t.Fatalf("expected default PixelWidth 1000, got %d", cfg.vhsConfig.PixelWidth)
	}
	if cfg.vhsConfig.PixelHeight != 600 {
		t.Fatalf("expected default PixelHeight 600, got %d", cfg.vhsConfig.PixelHeight)
	}
	if cfg.vhsConfig.FontSize != 16 {
		t.Fatalf("expected default FontSize 16, got %d", cfg.vhsConfig.FontSize)
	}
	if cfg.vhsConfig.PlaybackSpeed != 0.7 {
		t.Fatalf("expected default PlaybackSpeed 0.7, got %f", cfg.vhsConfig.PlaybackSpeed)
	}
}

func TestRecorderOptions_WithRecorderSize(t *testing.T) {
	cfg, err := resolveRecorderOptions([]RecorderOption{
		WithRecorderSize(40, 120),
	})
	if err != nil {
		t.Fatalf("resolveRecorderOptions returned error: %v", err)
	}

	if cfg.rows != 40 {
		t.Fatalf("expected rows 40, got %d", cfg.rows)
	}
	if cfg.cols != 120 {
		t.Fatalf("expected cols 120, got %d", cfg.cols)
	}
}

func TestRecorderOptions_WithRecorderTimeout(t *testing.T) {
	cfg, err := resolveRecorderOptions([]RecorderOption{
		WithRecorderTimeout(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("resolveRecorderOptions returned error: %v", err)
	}

	if cfg.defaultTimeout != 10*time.Second {
		t.Fatalf("expected timeout 10s, got %v", cfg.defaultTimeout)
	}
}

func TestRecorderOptions_WithRecorderVHSConfig(t *testing.T) {
	custom := DefaultVHSConfig()
	custom.FontSize = 24
	custom.PlaybackSpeed = 2.0

	cfg, err := resolveRecorderOptions([]RecorderOption{
		WithRecorderVHSConfig(custom),
	})
	if err != nil {
		t.Fatalf("resolveRecorderOptions returned error: %v", err)
	}

	if cfg.vhsConfig.FontSize != 24 {
		t.Fatalf("expected FontSize 24, got %d", cfg.vhsConfig.FontSize)
	}
	if cfg.vhsConfig.PlaybackSpeed != 2.0 {
		t.Fatalf("expected PlaybackSpeed 2.0, got %f", cfg.vhsConfig.PlaybackSpeed)
	}
}

func TestRecorderOptions_WithRecorderSkipTapeOutput(t *testing.T) {
	cfg, err := resolveRecorderOptions([]RecorderOption{
		WithRecorderSkipTapeOutput(),
	})
	if err != nil {
		t.Fatalf("resolveRecorderOptions returned error: %v", err)
	}

	if !cfg.skipTapeOutput {
		t.Fatalf("expected skipTapeOutput true, got false")
	}
}

func TestRecorderOptions_WithRecorderRepoRoot(t *testing.T) {
	cfg, err := resolveRecorderOptions([]RecorderOption{
		WithRecorderRepoRoot("/home/user/project"),
	})
	if err != nil {
		t.Fatalf("resolveRecorderOptions returned error: %v", err)
	}

	if cfg.repoRoot != "/home/user/project" {
		t.Fatalf("expected repoRoot /home/user/project, got %q", cfg.repoRoot)
	}
}

func TestRecorderOptions_WithRecorderEnv(t *testing.T) {
	cfg, err := resolveRecorderOptions([]RecorderOption{
		WithRecorderEnv("TERM=xterm", "SHELL=/bin/zsh"),
	})
	if err != nil {
		t.Fatalf("resolveRecorderOptions returned error: %v", err)
	}

	if len(cfg.env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(cfg.env))
	}
	if cfg.env[0] != "TERM=xterm" {
		t.Fatalf("expected env[0] TERM=xterm, got %q", cfg.env[0])
	}
	if cfg.env[1] != "SHELL=/bin/zsh" {
		t.Fatalf("expected env[1] SHELL=/bin/zsh, got %q", cfg.env[1])
	}
}

func TestRecorderOptions_WithRecorderDir(t *testing.T) {
	cfg, err := resolveRecorderOptions([]RecorderOption{
		WithRecorderDir("/tmp/workdir"),
	})
	if err != nil {
		t.Fatalf("resolveRecorderOptions returned error: %v", err)
	}

	if cfg.dir != "/tmp/workdir" {
		t.Fatalf("expected dir /tmp/workdir, got %q", cfg.dir)
	}
}

func TestRecorderOptions_WithRecorderShell(t *testing.T) {
	cfg, err := resolveRecorderOptions([]RecorderOption{
		WithRecorderShell("zsh"),
	})
	if err != nil {
		t.Fatalf("resolveRecorderOptions returned error: %v", err)
	}

	if cfg.shell != "zsh" {
		t.Fatalf("expected shell zsh, got %q", cfg.shell)
	}
}

func TestRecorderOptions_WithRecorderCommand(t *testing.T) {
	cfg, err := resolveRecorderOptions([]RecorderOption{
		WithRecorderCommand("osm", "script", "demo.js"),
	})
	if err != nil {
		t.Fatalf("resolveRecorderOptions returned error: %v", err)
	}

	if cfg.command != "osm" {
		t.Fatalf("expected command osm, got %q", cfg.command)
	}
	if len(cfg.args) != 2 || cfg.args[0] != "script" || cfg.args[1] != "demo.js" {
		t.Fatalf("expected args [script demo.js], got %v", cfg.args)
	}
}

func TestRecorderOptions_Multiple(t *testing.T) {
	cfg, err := resolveRecorderOptions([]RecorderOption{
		WithRecorderSize(50, 160),
		WithRecorderTimeout(15 * time.Second),
		WithRecorderShell("zsh"),
		WithRecorderRepoRoot("/project"),
		WithRecorderSkipTapeOutput(),
	})
	if err != nil {
		t.Fatalf("resolveRecorderOptions returned error: %v", err)
	}

	if cfg.rows != 50 {
		t.Fatalf("expected rows 50, got %d", cfg.rows)
	}
	if cfg.cols != 160 {
		t.Fatalf("expected cols 160, got %d", cfg.cols)
	}
	if cfg.defaultTimeout != 15*time.Second {
		t.Fatalf("expected timeout 15s, got %v", cfg.defaultTimeout)
	}
	if cfg.shell != "zsh" {
		t.Fatalf("expected shell zsh, got %q", cfg.shell)
	}
	if cfg.repoRoot != "/project" {
		t.Fatalf("expected repoRoot /project, got %q", cfg.repoRoot)
	}
	if !cfg.skipTapeOutput {
		t.Fatalf("expected skipTapeOutput true, got false")
	}
}
