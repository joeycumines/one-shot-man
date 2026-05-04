package command

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// logConfig holds resolved logging configuration for script-executing commands.
type logConfig struct {
	level      slog.Level
	logFile    io.WriteCloser // nil if no file logging
	bufferSize int
}

// resolveLogConfig resolves log configuration from flags and config defaults.
// Flag values take precedence; config values are used when flags have their
// zero/default value. The caller must Close() the returned logConfig.logFile
// when done (if non-nil).
func resolveLogConfig(flagPath, flagLevel string, flagBufferSize int, cfg *config.Config) (logConfig, error) {
	schema := config.DefaultSchema()
	var lc logConfig

	// Helper to safely resolve config values when cfg may be nil.
	resolveStr := func(key string) string {
		if cfg == nil {
			return ""
		}
		return schema.Resolve(cfg, key)
	}
	resolveInt := func(key string) int {
		if cfg == nil {
			return 0
		}
		return cfg.GetInt(key)
	}

	// Resolve log level: flag → config → "info".
	levelStr := flagLevel
	if levelStr == "" || levelStr == "info" {
		if v := resolveStr("log.level"); v != "" {
			levelStr = v
		}
	}
	switch strings.ToLower(levelStr) {
	case "debug":
		lc.level = slog.LevelDebug
	case "info", "":
		lc.level = slog.LevelInfo
	case "warn":
		lc.level = slog.LevelWarn
	case "error":
		lc.level = slog.LevelError
	default:
		return lc, fmt.Errorf("invalid log level: %s", levelStr)
	}

	// Resolve buffer size: flag → config → 1000.
	lc.bufferSize = flagBufferSize
	if lc.bufferSize <= 0 {
		lc.bufferSize = resolveInt("log.buffer-size")
		if lc.bufferSize <= 0 {
			lc.bufferSize = 1000
		}
	}

	// Resolve log path: flag → config → "".
	logPath := flagPath
	if logPath == "" {
		logPath = resolveStr("log.file")
	}

	if logPath != "" {
		// Resolve rotation settings from config.
		maxSizeMB := resolveInt("log.max-size-mb")
		if maxSizeMB <= 0 {
			maxSizeMB = 10 // default
		}
		maxFiles := resolveInt("log.max-files")
		if maxFiles < 0 {
			maxFiles = 5 // default
		}
		// Zero maxFiles is valid (no backups, just truncate on rotate).

		w, err := scripting.NewRotatingFileWriter(logPath, maxSizeMB, maxFiles)
		if err != nil {
			return lc, fmt.Errorf("failed to open log file %s: %w", logPath, err)
		}
		lc.logFile = w
	}

	return lc, nil
}
