package command

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

//go:embed super_document_template.md
var superDocumentTemplate string

//go:embed super_document_script.js
var superDocumentScript string

// SuperDocumentCommand provides the super-document TUI for document merging.
type SuperDocumentCommand struct {
	*BaseCommand
	interactive   bool
	shellMode     bool // Use shell mode instead of visual TUI
	testMode      bool
	config        *config.Config
	session       string
	store         string
	logLevel      string
	logPath       string
	logBufferSize int
}

// NewSuperDocumentCommand creates a new super-document command.
func NewSuperDocumentCommand(cfg *config.Config) *SuperDocumentCommand {
	return &SuperDocumentCommand{
		BaseCommand: NewBaseCommand(
			"super-document",
			"TUI for merging documents into a single internally consistent super-document",
			"super-document [options]",
		),
		config: cfg,
	}
}

// SetupFlags configures the flags for the super-document command.
func (c *SuperDocumentCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", true, "Start interactive TUI mode (default)")
	fs.BoolVar(&c.interactive, "i", true, "Start interactive TUI mode (short form, default)")
	fs.BoolVar(&c.shellMode, "shell", false, "Use shell mode instead of visual TUI")
	fs.BoolVar(&c.testMode, "test", false, "Enable test mode with verbose output")
	fs.StringVar(&c.session, "session", "", "Session ID for state persistence (overrides auto-discovery)")
	fs.StringVar(&c.store, "store", "", "Storage backend to use: 'fs' (default) or 'memory'")
	fs.StringVar(&c.logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	fs.StringVar(&c.logPath, "log-file", "", "Path to log file (JSON output)")
	fs.IntVar(&c.logBufferSize, "log-buffer", 1000, "Size of in-memory log buffer")
}

// Execute runs the super-document command.
func (c *SuperDocumentCommand) Execute(args []string, stdout, stderr io.Writer) error {
	ctx := context.Background()

	// Resolve logging configuration via config + flags.
	lc, err := resolveLogConfig(c.logPath, c.logLevel, c.logBufferSize, c.config)
	if err != nil {
		return err
	}
	if lc.logFile != nil {
		defer lc.logFile.Close()
	}

	// Create scripting engine with explicit session/storage and logging configuration
	engine, err := scripting.NewEngineDetailed(ctx, stdout, stderr, c.session, c.store, lc.logFile, lc.bufferSize, lc.level, modulePathOpts(c.config)...)
	if err != nil {
		return fmt.Errorf("failed to create scripting engine: %w", err)
	}
	defer engine.Close()

	// Start background session cleanup if enabled in config.
	stopCleanup := maybeStartCleanupScheduler(c.config, c.session)
	defer stopCleanup()

	if c.testMode {
		engine.SetTestMode(true)
	}

	// Build theme colors from config, with sensible adaptive defaults.
	// Uses AdaptiveColor format: {light: "...", dark: "..."} for automatic
	// terminal background detection. This eliminates "invisible text" issues.
	//
	// Palette philosophy:
	// - "Off-Black" and "Off-White" to reduce eye strain
	// - Primary accent (Indigo/Blue) looks good on both backgrounds
	// - Semantic colors for success, error, warning states
	themeColors := map[string]interface{}{
		// Text colors - primary content, secondary labels, tertiary/muted hints
		"textPrimary":   map[string]string{"light": "#24292f", "dark": "#e6edf3"},
		"textSecondary": map[string]string{"light": "#57606a", "dark": "#8b949e"},
		"textTertiary":  map[string]string{"light": "#6e7781", "dark": "#6e7681"},
		"textInverted":  map[string]string{"light": "#ffffff", "dark": "#0d1117"},

		// Accent colors - interactive elements, states
		"accentPrimary": map[string]string{"light": "#0969da", "dark": "#58a6ff"},
		"accentSubtle":  map[string]string{"light": "#ddf4ff", "dark": "#0d419d"},
		"accentSuccess": map[string]string{"light": "#1a7f37", "dark": "#3fb950"},
		"accentError":   map[string]string{"light": "#cf222e", "dark": "#f85149"},
		"accentWarning": map[string]string{"light": "#9a6700", "dark": "#d29922"},

		// UI chrome colors - borders, backgrounds
		"uiBorder":       map[string]string{"light": "#d0d7de", "dark": "#30363d"},
		"uiActiveBorder": map[string]string{"light": "#0969da", "dark": "#58a6ff"},
		"uiBg":           map[string]string{"light": "#ffffff", "dark": "#0d1117"},
		"uiBgSubtle":     map[string]string{"light": "#f6f8fa", "dark": "#161b22"},
	}
	if c.config != nil {
		for k, v := range c.config.Global {
			if strings.HasPrefix(k, "theme.") {
				key := strings.TrimPrefix(k, "theme.")
				if key != "" {
					themeColors[key] = v
				}
			}
		}
		// Also check command-specific theme overrides
		if cmdOpts, ok := c.config.Commands["super-document"]; ok {
			for k, v := range cmdOpts {
				if strings.HasPrefix(k, "theme.") {
					key := strings.TrimPrefix(k, "theme.")
					if key != "" {
						themeColors[key] = v
					}
				}
			}
		}
	}

	// Inject command name and configuration for state namespacing
	// The shellMode flag controls whether to start in shell or TUI mode
	const commandName = "super-document"
	engine.SetGlobal("config", map[string]interface{}{
		"name":      commandName,
		"shellMode": c.shellMode, // Wire --shell flag to JS state
		"theme":     themeColors, // Wire theme colors to JS
	})

	// Set up global variables
	engine.SetGlobal("args", args)
	engine.SetGlobal("superDocumentTemplate", superDocumentTemplate)

	// Load the embedded script
	script := engine.LoadScriptFromString("super-document", superDocumentScript)
	if err := engine.ExecuteScript(script); err != nil {
		return fmt.Errorf("failed to execute super-document script: %w", err)
	}

	// Only run interactive mode if requested and not in test mode
	if c.interactive && !c.testMode {
		// Apply prompt color overrides from config if present
		if c.config != nil {
			colorMap := make(map[string]string)
			for k, v := range c.config.Global {
				if strings.HasPrefix(k, "prompt.color.") {
					key := strings.TrimPrefix(k, "prompt.color.")
					if key != "" {
						colorMap[key] = v
					}
				}
			}
			if len(colorMap) > 0 {
				engine.GetTUIManager().SetDefaultColorsFromStrings(colorMap)
			}
		}
		terminal := scripting.NewTerminal(ctx, engine)
		terminal.Run()
	}

	return nil
}
