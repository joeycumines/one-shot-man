package command

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// scriptCommandBase contains shared fields and methods for commands that
// execute JavaScript through the scripting engine. All five script commands
// (scripting, code-review, prompt-flow, goal, super-document) embed this
// struct to eliminate duplicated flag registration and engine setup boilerplate.
type scriptCommandBase struct {
	testMode      bool
	config        *config.Config
	session       string
	store         string
	logLevel      string
	logPath       string
	logBufferSize int
}

// RegisterFlags registers the common flags shared by all script commands:
// --test, --session, --store, --log-level, --log-file, --log-buffer.
func (b *scriptCommandBase) RegisterFlags(fs *flag.FlagSet) {
	fs.BoolVar(&b.testMode, "test", false, "Enable test mode with verbose output")
	fs.StringVar(&b.session, "session", "", "Session ID for state persistence (overrides auto-discovery)")
	fs.StringVar(&b.store, "store", "", "Storage backend to use: 'fs' (default) or 'memory'")
	fs.StringVar(&b.logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	fs.StringVar(&b.logPath, "log-file", "", "Path to log file (JSON output)")
	fs.IntVar(&b.logBufferSize, "log-buffer", 1000, "Size of in-memory log buffer")
}

// PrepareEngine creates a scripting engine with logging, cleanup scheduler,
// test mode, and hot-snippet injection — the common setup shared by all
// script commands.
//
// Returns the engine and a cleanup function. The caller MUST defer the cleanup
// function. Resources are released in the correct order: cleanup scheduler
// first, then engine, then log file.
func (b *scriptCommandBase) PrepareEngine(ctx context.Context, stdout, stderr io.Writer) (*scripting.Engine, func(), error) {
	noop := func() {}

	// Resolve logging configuration via config + flags.
	lc, err := resolveLogConfig(b.logPath, b.logLevel, b.logBufferSize, b.config)
	if err != nil {
		return nil, noop, err
	}

	// Create scripting engine with explicit session/storage and logging configuration.
	engine, err := scripting.NewEngineDetailed(ctx, stdout, stderr, b.session, b.store, lc.logFile, lc.bufferSize, lc.level, modulePathOpts(b.config)...)
	if err != nil {
		if lc.logFile != nil {
			lc.logFile.Close()
		}
		return nil, noop, fmt.Errorf("failed to create scripting engine: %w", err)
	}

	// Start background session cleanup if enabled in config.
	stopCleanup := maybeStartCleanupScheduler(b.config, b.session)

	if b.testMode {
		engine.SetTestMode(true)
	}

	// Inject config-defined hot-snippets.
	injectConfigHotSnippets(engine, b.config)

	// Combined cleanup: release in reverse-acquisition order.
	cleanup := func() {
		stopCleanup()
		engine.Close()
		if lc.logFile != nil {
			lc.logFile.Close()
		}
	}

	return engine, cleanup, nil
}
