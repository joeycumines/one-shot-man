package command

import (
	"github.com/joeycumines/one-shot-man/internal/config"
)

// newIsolatedGoalConfig returns a config with autodiscovery and standard paths disabled.
// This prevents tests from picking up goals from the filesystem (e.g., goals/ in the repo root),
// ensuring deterministic results across platforms and working directories.
func newIsolatedGoalConfig() *config.Config {
	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	return cfg
}
