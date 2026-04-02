package command

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/termmux"

	termmuxmod "github.com/joeycumines/one-shot-man/internal/builtin/termmux"
	"golang.org/x/term"
)

//go:embed pr_split_template.md
var prSplitTemplate string

// Chunked script files — loaded in sequence as an alternative to the monolith.
// Each chunk is an IIFE that attaches exports to globalThis.prSplit.
//
//go:embed pr_split_00_core.js
var prSplitChunk00Core string

//go:embed pr_split_01_analysis.js
var prSplitChunk01Analysis string

//go:embed pr_split_02_grouping.js
var prSplitChunk02Grouping string

//go:embed pr_split_03_planning.js
var prSplitChunk03Planning string

//go:embed pr_split_04_validation.js
var prSplitChunk04Validation string

//go:embed pr_split_05_execution.js
var prSplitChunk05Execution string

//go:embed pr_split_06_verification.js
var prSplitChunk06Verification string

//go:embed pr_split_06b_verify_shell.js
var prSplitChunk06bVerifyShell string

//go:embed pr_split_07_prcreation.js
var prSplitChunk07PRCreation string

//go:embed pr_split_08_conflict.js
var prSplitChunk08Conflict string

//go:embed pr_split_09_claude.js
var prSplitChunk09Claude string

//go:embed pr_split_10a_pipeline_config.js
var prSplitChunk10aPipelineConfig string

//go:embed pr_split_10b_pipeline_send.js
var prSplitChunk10bPipelineSend string

//go:embed pr_split_10c_pipeline_resolve.js
var prSplitChunk10cPipelineResolve string

//go:embed pr_split_10d_pipeline_orchestrator.js
var prSplitChunk10dPipelineOrchestrator string

//go:embed pr_split_11_utilities.js
var prSplitChunk11Utilities string

//go:embed pr_split_12_exports.js
var prSplitChunk12Exports string

//go:embed pr_split_13_tui.js
var prSplitChunk13TUI string

//go:embed pr_split_14a_tui_commands_core.js
var prSplitChunk14aTUICommandsCore string

//go:embed pr_split_14b_tui_commands_ext.js
var prSplitChunk14bTUICommandsExt string

//go:embed pr_split_15a_tui_styles.js
var prSplitChunk15aTUIStyles string

//go:embed pr_split_15b_tui_chrome.js
var prSplitChunk15bTUIChrome string

//go:embed pr_split_15c_tui_screens.js
var prSplitChunk15cTUIScreens string

//go:embed pr_split_15d_tui_dialogs.js
var prSplitChunk15dTUIDialogs string

//go:embed pr_split_16a_tui_focus.js
var prSplitChunk16aTUIFocus string

//go:embed pr_split_16b_tui_handlers_pipeline.js
var prSplitChunk16bTUIHandlersPipeline string

//go:embed pr_split_16c_tui_handlers_verify.js
var prSplitChunk16cTUIHandlersVerify string

//go:embed pr_split_16d_tui_handlers_claude.js
var prSplitChunk16dTUIHandlersClaude string

//go:embed pr_split_16e_tui_update.js
var prSplitChunk16eTUIUpdate string

//go:embed pr_split_16f_tui_model.js
var prSplitChunk16fTUIModel string

// prSplitChunks defines the ordered sequence of chunk files for the split
// architecture. Each entry is (name, source) loaded in order.
var prSplitChunks = []struct {
	name   string
	source *string
}{
	{"00_core", &prSplitChunk00Core},
	{"01_analysis", &prSplitChunk01Analysis},
	{"02_grouping", &prSplitChunk02Grouping},
	{"03_planning", &prSplitChunk03Planning},
	{"04_validation", &prSplitChunk04Validation},
	{"05_execution", &prSplitChunk05Execution},
	{"06_verification", &prSplitChunk06Verification},
	{"06b_verify_shell", &prSplitChunk06bVerifyShell},
	{"07_prcreation", &prSplitChunk07PRCreation},
	{"08_conflict", &prSplitChunk08Conflict},
	{"09_claude", &prSplitChunk09Claude},
	{"10a_pipeline_config", &prSplitChunk10aPipelineConfig},
	{"10b_pipeline_send", &prSplitChunk10bPipelineSend},
	{"10c_pipeline_resolve", &prSplitChunk10cPipelineResolve},
	{"10d_pipeline_orchestrator", &prSplitChunk10dPipelineOrchestrator},
	{"11_utilities", &prSplitChunk11Utilities},
	{"12_exports", &prSplitChunk12Exports},
	{"13_tui", &prSplitChunk13TUI},
	{"14a_tui_commands_core", &prSplitChunk14aTUICommandsCore},
	{"14b_tui_commands_ext", &prSplitChunk14bTUICommandsExt},
	{"15a_tui_styles", &prSplitChunk15aTUIStyles},
	{"15b_tui_chrome", &prSplitChunk15bTUIChrome},
	{"15c_tui_screens", &prSplitChunk15cTUIScreens},
	{"15d_tui_dialogs", &prSplitChunk15dTUIDialogs},
	{"16a_tui_focus", &prSplitChunk16aTUIFocus},
	{"16b_tui_handlers_pipeline", &prSplitChunk16bTUIHandlersPipeline},
	{"16c_tui_handlers_verify", &prSplitChunk16cTUIHandlersVerify},
	{"16d_tui_handlers_claude", &prSplitChunk16dTUIHandlersClaude},
	{"16e_tui_update", &prSplitChunk16eTUIUpdate},
	{"16f_tui_model", &prSplitChunk16fTUIModel},
}

// loadChunkedScript loads all pr-split chunk files in order into the engine.
// Each chunk is loaded as a separate script with error reporting per-chunk.
func loadChunkedScript(engine *scripting.Engine) error {
	for _, chunk := range prSplitChunks {
		name := "pr-split/" + chunk.name
		script := engine.LoadScriptFromString(name, *chunk.source)
		if err := engine.ExecuteScript(script); err != nil {
			return fmt.Errorf("failed to load pr-split chunk %s: %w", chunk.name, err)
		}
	}
	return nil
}

// PrSplitCommand splits a large PR into reviewable stacked branches.
// Supports heuristic grouping strategies including directory, extension,
// chunks, dependency (Go import graph), and auto.
type PrSplitCommand struct {
	*BaseCommand
	scriptCommandBase
	interactive bool

	// Split configuration flags
	baseBranch    string
	strategy      string
	maxFiles      int
	branchPrefix  string
	verifyCommand string
	dryRun        bool

	// JSON output flag
	jsonOutput bool

	// testWorkingDir is set by tests to specify a temporary git repo directory.
	// When set, validateGitRepo() will validate that directory explicitly.
	testWorkingDir string

	// Claude Code execution configuration
	claudeCommand   string          // explicit path/name of Claude binary (empty = auto-detect)
	claudeArgs      stringSliceFlag // additional CLI arguments for Claude (repeatable --claude-arg flags)
	claudeModel     string          // model to use (provider-dependent)
	claudeConfigDir string          // config directory override
	claudeEnv       string          // extra environment variables (KEY=VALUE,KEY=VALUE)

	// Timeout for Claude communication steps (classify, plan, resolve).
	timeout time.Duration

	// Resume a previously saved auto-split session.
	resume bool

	// Delete split branches if the pipeline fails.
	cleanupOnFailure bool
}

// stringSliceFlag implements [flag.Value] for repeatable string flags.
// Each occurrence of the flag appends to the slice, avoiding fragile
// string-splitting of shell arguments.
type stringSliceFlag []string

func (f *stringSliceFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ", ")
}

func (f *stringSliceFlag) Set(val string) error {
	*f = append(*f, val)
	return nil
}

// NewPrSplitCommand creates a new pr-split command.
func NewPrSplitCommand(cfg *config.Config) *PrSplitCommand {
	return &PrSplitCommand{
		BaseCommand: NewBaseCommand(
			"pr-split",
			"Split a large PR into reviewable stacked branches",
			"pr-split [options]",
		),
		scriptCommandBase: scriptCommandBase{config: cfg},

		// Defaults — mirrored in SetupFlags for flag-based parsing.
		interactive:   true,
		baseBranch:    "main",
		strategy:      "directory",
		maxFiles:      10,
		branchPrefix:  "split/",
		verifyCommand: "",
	}
}

// SetupFlags configures the flags for the pr-split command.
func (c *PrSplitCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", true, "Start interactive mode (default)")
	fs.BoolVar(&c.interactive, "i", true, "Start interactive mode (short form)")

	// Split configuration
	fs.StringVar(&c.baseBranch, "base", "main", "Base branch to split against")
	fs.StringVar(&c.strategy, "strategy", "directory", "Grouping strategy: directory, directory-deep, extension, chunks, dependency, auto")
	fs.IntVar(&c.maxFiles, "max", 10, "Maximum files per split")
	fs.StringVar(&c.branchPrefix, "prefix", "split/", "Branch name prefix for splits")
	fs.StringVar(&c.verifyCommand, "verify", "", "Command to verify each split (empty=auto-detect from Makefile)")
	fs.BoolVar(&c.dryRun, "dry-run", false, "Show plan without executing")

	fs.BoolVar(&c.jsonOutput, "json", false, "Output results as JSON (combine with run or --dry-run)")

	// Claude Code execution
	fs.StringVar(&c.claudeCommand, "claude-command", "", "Claude binary path (empty = auto-detect)")
	fs.Var(&c.claudeArgs, "claude-arg", "Additional Claude CLI argument (repeatable)")
	fs.StringVar(&c.claudeModel, "claude-model", "", "Model name (provider-dependent)")
	fs.StringVar(&c.claudeConfigDir, "claude-config-dir", "", "Claude config directory override")
	fs.StringVar(&c.claudeEnv, "claude-env", "", "Extra environment variables (KEY=VALUE,KEY=VALUE)")

	fs.DurationVar(&c.timeout, "timeout", 0, "Timeout for Claude communication steps (e.g. 5m); 0 = defaults")
	fs.BoolVar(&c.resume, "resume", false, "Resume a previously saved auto-split session")
	fs.BoolVar(&c.cleanupOnFailure, "cleanup-on-failure", false, "Delete split branches if the pipeline fails")

	c.RegisterFlags(fs)
}

// Execute runs the pr-split command.
func (c *PrSplitCommand) Execute(args []string, stdout, stderr io.Writer) error {
	// Set up context with signal handling. In test mode, use a plain
	// cancellable context to avoid interfering with test harness signals.
	// In production, SIGINT/SIGTERM cancel the context for graceful shutdown.
	var ctx context.Context
	var stop context.CancelFunc
	if c.testMode {
		ctx, stop = context.WithCancel(context.Background())
	} else {
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	}
	defer stop()

	// Apply config-file defaults (flags take precedence) and validate.
	c.applyConfigDefaults()
	if err := c.validateFlags(); err != nil {
		return err
	}
	// T390: Fail fast on git-related misconfigurations before launching
	// the expensive scripting engine and full-screen TUI wizard.
	if err := c.validateGitRepo(); err != nil {
		return err
	}

	engine, cleanup, err := c.PrepareEngine(ctx, stdout, stderr)
	if err != nil {
		return err
	}
	defer cleanup()

	termFd, err := c.setupEngineGlobals(ctx, engine, stdout)
	if err != nil {
		return err
	}

	// Interactive mode: launch BubbleTea wizard with signal handling.
	if c.interactive && !c.testMode {
		// Save terminal state before BubbleTea enters alt screen / raw mode.
		// Used by the double-SIGINT handler AND the deferred finalizer below.
		var savedTermState *term.State
		if term.IsTerminal(termFd) {
			savedTermState, _ = term.GetState(termFd)
		}

		// Deferred terminal finalizer — defense-in-depth safety net.
		//
		// BubbleTea and termmux.RunPassthrough each manage their own
		// terminal restoration (alt screen, raw mode, cursor). This
		// defer catches any edge case where their cleanup does not
		// run — e.g., an engine error, unexpected panic path, or a
		// goja runtime interrupt that bypasses normal shutdown.
		//
		// The operations are idempotent: term.Restore to a previously
		// saved state is harmless if already restored, and the ANSI
		// escape sequences are no-ops when already in normal mode.
		//
		// Note on double-SIGINT (os.Exit): this defer does NOT run
		// on os.Exit; that path has its own explicit restore above.
		// Note on Claude process: os.Exit closes all FDs including
		// the PTY master, which sends SIGHUP to Claude — no orphan.
		defer func() {
			if savedTermState != nil {
				_ = term.Restore(termFd, savedTermState)
			}
			// Belt-and-suspenders: exit alt screen + show cursor.
			fmt.Fprint(os.Stderr, "\x1b[?1049l\x1b[?25h")
		}()

		// Double-SIGINT force-exit handler. After the first signal cancels
		// ctx (triggering BubbleTea's graceful quit via context propagation),
		// a second SIGINT force-exits with best-effort terminal restoration.
		// This prevents the user from being stuck if graceful shutdown hangs
		// (e.g., Claude subprocess won't terminate).
		done := make(chan struct{})
		defer close(done)
		go func() {
			<-ctx.Done()
			stop() // Deregister NotifyContext; next signal hits sigCh below.

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt)
			defer signal.Stop(sigCh)

			select {
			case <-sigCh:
				// Second interrupt — force exit with terminal restore.
				if savedTermState != nil {
					_ = term.Restore(termFd, savedTermState)
				}
				// Best-effort: exit alt screen + show cursor.
				fmt.Fprint(os.Stderr, "\x1b[?1049l\x1b[?25h")
				slog.Error("pr-split: force-exit on double SIGINT")
				os.Exit(130) // 128 + SIGINT(2)
			case <-done:
				// Graceful shutdown completed; goroutine exits cleanly.
			}
		}()

		// Launch BubbleTea wizard (full-screen TUI). ExecuteScript routes the
		// launch through the event loop; tea.run() starts BubbleTea in a
		// goroutine and returns immediately so the event loop stays free for
		// BubbleTea's RunJSSync callbacks. ExecuteScript automatically calls
		// WaitForProgram() on the calling goroutine, blocking until the user
		// exits the wizard or context is cancelled.
		wizardScript := engine.LoadScriptFromString(
			"pr-split/wizard-launch",
			`globalThis.prSplit.startWizard();`)
		if err := engine.ExecuteScript(wizardScript); err != nil {
			return fmt.Errorf("pr-split wizard: %w", err)
		}
	} else if !c.testMode {
		// Non-interactive mode: either batch-execute positional args as
		// TUI commands, or fall back to a go-prompt REPL for scripting
		// and PTY-based integration tests.
		if len(args) > 0 {
			// Batch mode: dispatch each positional argument as a TUI
			// command. Example: osm pr-split -interactive=false run
			tm := engine.GetTUIManager()
			if tm == nil {
				return fmt.Errorf("pr-split: TUI command manager not initialized")
			}
			for _, cmd := range args {
				if err := tm.ExecuteCommand(cmd, nil); err != nil {
					return fmt.Errorf("pr-split %s: %w", cmd, err)
				}
			}
		} else {
			// REPL mode: interactive go-prompt session, used by PTY-
			// based observation tests and advanced scripting workflows.
			terminal := scripting.NewTerminal(ctx, engine)
			terminal.Run()
		}
	}

	return nil
}

// setupEngineGlobals injects JS globals (config, prSplitConfig, template,
// tuiMux, sessionTypes) and loads the 30 chunk files into the Goja engine.
// Returns the terminal file descriptor (needed for interactive-mode terminal
// state save/restore).
//
// This function is the sole owner of mux lifecycle initialization. All
// session-related globals are configured here — JS chunks use them but
// never create new mux instances.
func (c *PrSplitCommand) setupEngineGlobals(ctx context.Context, engine *scripting.Engine, stdout io.Writer) (termFd int, err error) {
	// Inject command name for state namespacing.
	engine.SetGlobal("config", map[string]any{
		"name": "pr-split",
	})

	// Prompt template embedded from pr_split_template.md.
	engine.SetGlobal("prSplitTemplate", prSplitTemplate)

	// Expose split configuration to JS.
	claudeArgsList := make([]string, len(c.claudeArgs))
	copy(claudeArgsList, c.claudeArgs)
	claudeEnvMap := parseClaudeEnv(c.claudeEnv)
	engine.SetGlobal("prSplitConfig", map[string]any{
		"baseBranch":       c.baseBranch,
		"strategy":         c.strategy,
		"maxFiles":         c.maxFiles,
		"branchPrefix":     c.branchPrefix,
		"verifyCommand":    c.verifyCommand,
		"dryRun":           c.dryRun,
		"jsonOutput":       c.jsonOutput,
		"claudeCommand":    c.claudeCommand,
		"claudeArgs":       claudeArgsList,
		"claudeModel":      c.claudeModel,
		"claudeConfigDir":  c.claudeConfigDir,
		"claudeEnv":        claudeEnvMap,
		"timeoutMs":        int64(c.timeout / time.Millisecond),
		"resumeFromPlan":   c.resume,
		"cleanupOnFailure": c.cleanupOnFailure,
	})

	// ── Session lifecycle: tuiMux ────────────────────────────────────
	//
	// The TUI mux owns the fullscreen passthrough between osm and a child
	// PTY (Claude Code). JS chunks interact with it via the tuiMux global:
	//
	//   1. pr_split_09_claude.js  → spawns Claude, gets AgentHandle
	//   2. pr_split_10d_orchestrator.js → tuiMux.attach(handle)
	//   3. pr_split_16d_tui_handlers_claude.js → tuiMux.switchTo() (blocking)
	//   4. pr_split_10a_pipeline_config.js → executor.close() / deferred detach
	//
	// Verification shells are standalone CaptureSession objects (see
	// pr_split_06b_verify_shell.js) — they are NOT attached to tuiMux.
	//
	// Uses os.Stdin directly (not go-prompt's wrapped readers) because
	// the command-blocking model ensures go-prompt is paused during
	// passthrough. stdout is injected for testability.
	termFd = int(os.Stdin.Fd())
	tuiMux := termmux.New(os.Stdin, stdout, termFd)

	// Pre-configure the mux's session target so switchTo() enters with
	// correct metadata from the start (not assigned lazily in JS chunks).
	tuiMux.SetActiveTarget(termmux.SessionTarget{
		Name: "claude",
		Kind: termmux.SessionKindPTY,
	})

	// Expose the mux to JS through the standardized osm:termmux interface.
	engine.SetGlobal("tuiMux", termmuxmod.WrapMux(ctx, engine.Runtime(), tuiMux))

	// Session type constants: JS uses these to create and label sessions
	// consistently. Defined here so the Go bootstrap and all JS chunks
	// agree on the session vocabulary.
	engine.SetGlobal("sessionTypes", map[string]any{
		"claude": map[string]any{
			"name": "claude",
			"kind": "pty",
		},
		"verify": map[string]any{
			"name": "verify",
			"kind": "capture",
		},
	})

	// Load the 30 chunked script files in dependency order.
	if err := loadChunkedScript(engine); err != nil {
		return 0, err
	}

	return termFd, nil
}

// applyConfigDefaults applies config-file values to command fields where the
// field still holds its flag default. Flags override config values —
// config keys are namespaced under the "pr-split" command section:
//
//	pr-split.base=develop
//	pr-split.strategy=extension
//	pr-split.max=8
//	pr-split.prefix=split/
//	pr-split.verify=make
//	pr-split.dry-run=true
func (c *PrSplitCommand) applyConfigDefaults() {
	if c.config == nil {
		return
	}
	applyStr := func(key string, target *string, flagDefault string) {
		if v, ok := c.config.GetCommandOption("pr-split", key); ok && (*target == flagDefault || *target == "") {
			*target = v
		}
	}
	applyStr("base", &c.baseBranch, "main")
	applyStr("strategy", &c.strategy, "directory")
	if v, ok := c.config.GetCommandOption("pr-split", "max"); ok && (c.maxFiles == 10 || c.maxFiles == 0) {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.maxFiles = n
		}
	}
	applyStr("prefix", &c.branchPrefix, "split/")
	applyStr("verify", &c.verifyCommand, "")
	if v, ok := c.config.GetCommandOption("pr-split", "dry-run"); ok && !c.dryRun {
		c.dryRun = v == "true" || v == "1" || v == "yes"
	}
	applyStr("claude-command", &c.claudeCommand, "")
	if v, ok := c.config.GetCommandOption("pr-split", "claude-arg"); ok && len(c.claudeArgs) == 0 {
		c.claudeArgs = append(c.claudeArgs, v)
	}
	applyStr("claude-model", &c.claudeModel, "")
	applyStr("claude-config-dir", &c.claudeConfigDir, "")
	applyStr("claude-env", &c.claudeEnv, "")
	if v, ok := c.config.GetCommandOption("pr-split", "timeout"); ok && c.timeout == 0 {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			c.timeout = d
		}
	}
	if v, ok := c.config.GetCommandOption("pr-split", "resume"); ok && !c.resume {
		c.resume = v == "true" || v == "1" || v == "yes"
	}
	if v, ok := c.config.GetCommandOption("pr-split", "cleanup-on-failure"); ok && !c.cleanupOnFailure {
		c.cleanupOnFailure = v == "true" || v == "1" || v == "yes"
	}
}

// validateFlags checks that command flags hold valid values after config
// defaults have been applied. Returns a descriptive error on the first
// invalid value found.
func (c *PrSplitCommand) validateFlags() error {
	validStrategies := map[string]bool{
		"directory": true, "directory-deep": true, "extension": true,
		"chunks": true, "dependency": true, "auto": true,
	}
	if !validStrategies[c.strategy] {
		return fmt.Errorf("invalid --strategy %q: must be one of directory, directory-deep, extension, chunks, dependency, auto", c.strategy)
	}
	if c.maxFiles < 1 {
		return fmt.Errorf("invalid --max %d: must be at least 1", c.maxFiles)
	}
	if c.timeout < 0 {
		return fmt.Errorf("invalid --timeout %s: must be non-negative", c.timeout)
	}
	return nil
}

// validateGitRepo performs early detection of common git-related errors
// before launching the expensive scripting engine and TUI wizard.
// Returns a clear error if the working directory is not inside a git repo
// or if the specified base branch does not exist.
func (c *PrSplitCommand) validateGitRepo() error {
	// Check if we're inside a git working tree.
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	if c.testWorkingDir != "" {
		cmd.Dir = c.testWorkingDir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("git is not installed or not in PATH")
		}
		outStr := strings.TrimSpace(string(out))
		if strings.Contains(outStr, "not a git repository") {
			return fmt.Errorf("not a git repository (or any parent up to mount point)")
		}
		if outStr != "" {
			return fmt.Errorf("git check failed: %s", outStr)
		}
		return fmt.Errorf("git check failed: %w", err)
	}
	// Bare repos report "false" — not a valid working tree for pr-split.
	if strings.TrimSpace(string(out)) != "true" {
		return fmt.Errorf("not inside a git working tree (bare repository?)")
	}

	// Validate the base branch exists (local or remote tracking ref).
	base := c.baseBranch
	if base != "" {
		// Try local branch first, then remote tracking refs.
		cmd = exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/heads/"+base)
		if c.testWorkingDir != "" {
			cmd.Dir = c.testWorkingDir
		}
		if err := cmd.Run(); err != nil {
			// Not a local branch — try common remote refs.
			cmd = exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+base)
			if c.testWorkingDir != "" {
				cmd.Dir = c.testWorkingDir
			}
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("base branch %q not found (checked local and origin remote)", base)
			}
		}
	}

	return nil
}

// parseClaudeEnv parses a comma-separated KEY=VALUE string into a map.
// Malformed entries (empty key, no '=') are logged as warnings and skipped.
// Whitespace around pairs is trimmed.
func parseClaudeEnv(raw string) map[string]string {
	m := map[string]string{}
	if raw == "" {
		return m
	}
	for pair := range strings.SplitSeq(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			slog.Warn("parseClaudeEnv: entry has no '=' delimiter, skipping", "entry", pair)
			continue
		}
		if k == "" {
			slog.Warn("parseClaudeEnv: entry has empty key, skipping", "entry", pair)
			continue
		}
		m[k] = v
	}
	return m
}
