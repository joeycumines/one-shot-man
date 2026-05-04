package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestMaybeStartCleanupScheduler_NilConfig(t *testing.T) {
	stop := maybeStartCleanupScheduler(nil, "")
	stop() // should not panic
}

func TestMaybeStartCleanupScheduler_Disabled(t *testing.T) {
	cfg := config.NewConfig()
	cfg.Sessions.AutoCleanupEnabled = false

	stop := maybeStartCleanupScheduler(cfg, "")
	stop() // should not panic and should not have started a goroutine
}

func TestMaybeStartCleanupScheduler_Enabled(t *testing.T) {
	cfg := config.NewConfig()
	cfg.Sessions.AutoCleanupEnabled = true

	stop := maybeStartCleanupScheduler(cfg, "test-session")
	// Calling stop should cancel the running scheduler goroutine.
	stop()
}
