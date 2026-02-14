package command

import (
	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// modulePathOpts returns EngineOption values for configurable module search paths,
// read from the "script.module-paths" config key. Returns nil if cfg is nil or
// no module paths are configured.
func modulePathOpts(cfg *config.Config) []scripting.EngineOption {
	if cfg == nil {
		return nil
	}
	mp, exists := cfg.GetGlobalOption("script.module-paths")
	if !exists {
		return nil
	}
	paths := parsePathList(mp)
	if len(paths) == 0 {
		return nil
	}
	return []scripting.EngineOption{scripting.WithModulePaths(paths...)}
}
