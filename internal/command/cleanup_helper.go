package command

import (
	"context"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/storage"
)

// maybeStartCleanupScheduler starts a background cleanup scheduler if
// automatic cleanup is enabled in the configuration. It returns a stop
// function that cancels the scheduler; callers must defer it.
//
// When cfg is nil, or AutoCleanupEnabled is false, the returned stop
// function is a no-op.
func maybeStartCleanupScheduler(cfg *config.Config, excludeID string) (stop func()) {
	if cfg == nil || !cfg.Sessions.AutoCleanupEnabled {
		return func() {}
	}

	cleaner := &storage.Cleaner{
		MaxAgeDays: cfg.Sessions.MaxAgeDays,
		MaxCount:   cfg.Sessions.MaxCount,
		MaxSizeMB:  cfg.Sessions.MaxSizeMB,
	}

	interval := time.Duration(cfg.Sessions.CleanupIntervalHours) * time.Hour

	scheduler := &storage.CleanupScheduler{
		Cleaner:   cleaner,
		ExcludeID: excludeID,
		Interval:  interval,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go scheduler.Run(ctx)

	return cancel
}
