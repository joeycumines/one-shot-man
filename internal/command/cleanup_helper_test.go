package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/storage"
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
	// Isolate the session directory so the scheduler's immediate cleanup
	// run does not touch the real user directory or contend on locks with
	// other tests.
	storage.SetTestPaths(t.TempDir())
	defer storage.ResetPaths()

	cfg := config.NewConfig()
	cfg.Sessions.AutoCleanupEnabled = true

	stop := maybeStartCleanupScheduler(cfg, "test-session")
	// Calling stop should cancel the running scheduler goroutine and wait
	// for it to exit so no background goroutine leaks into subsequent tests.
	stop()
}
