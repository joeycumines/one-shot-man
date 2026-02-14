package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

func TestModulePathOpts_NilConfig(t *testing.T) {
	t.Parallel()
	opts := modulePathOpts(nil)
	if opts != nil {
		t.Errorf("expected nil opts for nil config, got %v", opts)
	}
}

func TestModulePathOpts_NoKey(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	opts := modulePathOpts(cfg)
	if opts != nil {
		t.Errorf("expected nil opts when key is absent, got %v", opts)
	}
}

func TestModulePathOpts_EmptyValue(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.module-paths", "  ")
	opts := modulePathOpts(cfg)
	if opts != nil {
		t.Errorf("expected nil opts for empty value, got %v", opts)
	}
}

func TestModulePathOpts_SinglePath(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.module-paths", "/usr/local/osm-modules")
	opts := modulePathOpts(cfg)
	if len(opts) != 1 {
		t.Fatalf("expected 1 option, got %d", len(opts))
	}
	// Verify the option is a valid EngineOption (can apply without panic)
	var _ scripting.EngineOption = opts[0]
}

func TestModulePathOpts_MultiplePaths(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.module-paths", "/path/one,/path/two:/path/three")
	opts := modulePathOpts(cfg)
	if len(opts) != 1 {
		t.Fatalf("expected 1 option (bundling all paths), got %d", len(opts))
	}
}
