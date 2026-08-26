package command

import (
	"errors"
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/gitops"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/storage"
	"github.com/joeycumines/one-shot-man/internal/termmux"

	termmuxmod "github.com/joeycumines/one-shot-man/internal/builtin/termmux"
	"golang.org/x/term"
)

//go:embed pr_split_template.md
var prSplitTemplate string

// Chunked script files — loaded in sequence as an alternative to the monolith.
// Each chunk is an IIFE that attaches exports to globalThis.prSplit.

//go:embed pr_split_manifest.json
var prSplitManifest string

//go:embed pr_split_*.js
var chunkFS embed.FS

type chunkManifestEntry struct {
	ID      string   `json:"id"`
	File    string   `json:"file"`
	Exports []string `json:"exports"`
}

type chunkManifest struct {
	Version string               `json:"version"`
	Chunks  []chunkManifestEntry `json:"chunks"`
}

var prSplitManifestData chunkManifest

func init() {
	if err := json.Unmarshal([]byte(prSplitManifest), &prSplitManifestData); err != nil {
		panic("pr-split: failed to parse manifest: " + err.Error())
	}
}

// loadChunkedScript loads all pr-split chunk files in order into the engine.
// Each chunk is loaded as a separate script with error reporting per-chunk.
func loadChunkedScript(engine *scripting.Engine) error {
	for _, entry := range prSplitManifestData.Chunks {
		data, err := chunkFS.ReadFile(entry.File)
		if err != nil {
			return fmt.Errorf("pr-split: chunk file %q not found in embedded FS: %w", entry.File, err)
		}
		name := "pr-split/" + entry.ID
		script := engine.LoadScriptString(name, string(data))
		if err := engine.ExecuteScript(script); err != nil {
			return fmt.Errorf("failed to load pr-split chunk %s: %w", entry.ID, err)
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

	// Agent execution configuration
	agentCommand   string          // explicit path/name of agent binary (empty = auto-detect)
	agentArgs      stringSliceFlag // additional CLI arguments for the agent (repeatable --agent-arg flags)
	agentModel     string          // model to use (provider-dependent)
	agentConfigDir string          // config directory override
	agentEnv       string          // extra environment variables (KEY=VALUE,KEY=VALUE)

	// Timeout for agent communication steps (classify, plan, resolve).
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
		baseBranch:    "", // empty = auto-detect
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
	fs.StringVar(&c.baseBranch, "base", "", "Base branch to split against (empty or \"auto\" = auto-detect)")
	fs.StringVar(&c.strategy, "strategy", "directory", "Grouping strategy: directory, directory-deep, extension, chunks, dependency, auto")
	fs.IntVar(&c.maxFiles, "max", 10, "Maximum files per split")
	fs.StringVar(&c.branchPrefix, "prefix", "split/", "Branch name prefix for splits")
	fs.StringVar(&c.verifyCommand, "verify", "", "Command to verify each split (empty=auto-detect from Makefile)")
	fs.BoolVar(&c.dryRun, "dry-run", false, "Show plan without executing")

	fs.BoolVar(&c.jsonOutput, "json", false, "Output results as JSON (combine with run or --dry-run)")

	// Agent execution
	fs.StringVar(&c.agentCommand, "agent-command", "", "Agent binary path (empty = auto-detect)")
	fs.Var(&c.agentArgs, "agent-arg", "Additional agent CLI argument (repeatable)")
	fs.StringVar(&c.agentModel, "agent-model", "", "Model name (provider-dependent)")
	fs.StringVar(&c.agentConfigDir, "agent-config-dir", "", "Agent config directory override")
	fs.StringVar(&c.agentEnv, "agent-env", "", "Extra environment variables (KEY=VALUE,KEY=VALUE)")

	fs.DurationVar(&c.timeout, "timeout", 0, "Timeout for agent communication steps (e.g. 5m); 0 = defaults")
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

	termFd, tuiMgr, err := c.setupEngineGlobals(ctx, engine, stdout)
	if err != nil {
		return err
	}

	// Clean up the persistence state file on normal exit so the next
	// startup doesn't offer stale resume data. Crash exits leave the
	// file in place intentionally — that's the resume case.
	if persistPath, dirErr := storage.SessionDirectory(); dirErr == nil {
		stateFile := filepath.Join(persistPath, "pr-split-mux.state.json")
		defer termmux.RemoveManagerState(stateFile)
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
		// on os.Exit; that path has its own explicit restore + session
		// cleanup above — see the forceCloseSessionManager call.
		defer func() {
			if savedTermState != nil {
				_ = term.Restore(termFd, savedTermState)
			}
			// Belt-and-suspenders: exit alt screen + show cursor.
			fmt.Fprint(os.Stderr, "\x1b[?1049l\x1b[?25h")
		}()

		// Double-SIGINT force-exit handler. After the first signal cancels
		// ctx (triggering BubbleTea's graceful quit via context propagation),
		// a second SIGINT force-exits with best-effort terminal restoration
		// and explicit session cleanup (SIGTERM→SIGKILL to child process
		// groups via SessionManager.Close).
		//
		// tuiMgr is captured by the closure; it was returned from
		// setupEngineGlobals above, so it is already assigned and
		// never reassigned — no data race.
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
				// Second interrupt — kill sessions then force exit.
				forceCloseSessionManager(tuiMgr)
				if savedTermState != nil {
					_ = term.Restore(termFd, savedTermState)
				}
				// Best-effort: exit alt screen + show cursor.
				fmt.Fprint(os.Stderr, "\x1b[?1049l\x1b[?25h")
				slog.Error("pr split force exit on double sigint")
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
		wizardScript := engine.LoadScriptString(
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
			return nil
		}
	}

	// Wait for any asynchronous work (timers, fetch, etc.) to complete naturally.
	// This uses the WithAutoExit(true) feature of the event loop.
	engine.Wait()

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
func (c *PrSplitCommand) setupEngineGlobals(ctx context.Context, engine *scripting.Engine, stdout io.Writer) (termFd int, sessionMgr *termmux.SessionManager, err error) {
	loop := engine.Loop()
	if loop == nil {
		return 0, nil, errors.New("event loop not available")
	}

	// All runtime mutation must happen on the event-loop goroutine: the
	// termmux wrapper's event bridge dispatches SessionManager events onto
	// the loop as soon as WrapSessionManager runs, and goja.Runtime is not
	// goroutine-safe. Inner Engine calls take their on-loop fast path.
	done := make(chan struct{})
	var runErr error
	if submitErr := loop.Submit(func() {
		defer close(done)
		termFd, sessionMgr, runErr = c.setupEngineGlobalsOnLoop(ctx, engine, stdout)
	}); submitErr != nil {
		return 0, nil, fmt.Errorf("event loop not running: %w", submitErr)
	}
	<-done
	if runErr != nil {
		return 0, nil, runErr
	}
	return termFd, sessionMgr, nil
}

func (c *PrSplitCommand) setupEngineGlobalsOnLoop(ctx context.Context, engine *scripting.Engine, stdout io.Writer) (termFd int, sessionMgr *termmux.SessionManager, err error) {
	// Inject command name for state namespacing.
	engine.SetGlobal("config", map[string]any{
		"name": "pr-split",
	})

	// Prompt template embedded from pr_split_template.md.
	engine.SetGlobal("prSplitTemplate", prSplitTemplate)

	// Compute the persistence state file path for session resume.
	// Uses the same session directory as the storage backend so
	// state files live alongside session data.
	persistStatePath := ""
	if dir, dirErr := storage.SessionDirectory(); dirErr == nil {
		persistStatePath = filepath.Join(dir, "pr-split-mux.state.json")
	}

	// Expose split configuration to JS.
	agentArgsList := make([]string, len(c.agentArgs))
	copy(agentArgsList, c.agentArgs)
	agentEnvMap := parseAgentEnv(c.agentEnv)
	engine.SetGlobal("prSplitConfig", map[string]any{
		"baseBranch":       c.baseBranch,
		"strategy":         c.strategy,
		"maxFiles":         c.maxFiles,
		"branchPrefix":     c.branchPrefix,
		"verifyCommand":    c.verifyCommand,
		"dryRun":           c.dryRun,
		"jsonOutput":       c.jsonOutput,
		"agentCommand":     c.agentCommand,
		"agentArgs":        agentArgsList,
		"agentModel":       c.agentModel,
		"agentConfigDir":   c.agentConfigDir,
		"agentEnv":         agentEnvMap,
		"timeoutMs":        int64(c.timeout / time.Millisecond),
		"resumeFromPlan":   c.resume,
		"cleanupOnFailure": c.cleanupOnFailure,
		"persistStatePath": persistStatePath,
	})

	// ── Session lifecycle: tuiMux ────────────────────────────────────
	//
	// The TUI mux owns the fullscreen passthrough between osm and a child
	// PTY (Agent Code). JS chunks interact with it via the tuiMux global:
	//
	//   1. pr_split_09_agent.js  → spawns agent, gets AgentHandle
	//   2. pr_split_10d_orchestrator.js → tuiMux.attach(handle)
	//   3. pr_split_16d_tui_handlers_agent.js → tuiMux.switchTo() (blocking)
	//   4. pr_split_10a_pipeline_config.js → executor.close() / deferred detach
	//
	// Verification sessions ARE registered with tuiMux via
	// tuiMux.register() in pr_split_16c_tui_handlers_verify.js and
	// accessed through a pinned SessionID proxy built by
	// _buildVerifyProxy() in pr_split_13_tui.js. The proxy uses
	// tuiMux.snapshot(sessionID) for reads, tuiMux.activate(sessionID)
	// + tuiMux.input() for writes, and tuiMux.unregister(sessionID)
	// for cleanup.
	//
	// Uses os.Stdin directly (not go-prompt's wrapped readers) because
	// the command-blocking model ensures go-prompt is paused during
	// passthrough. stdout is injected for testability.
	termFd = int(os.Stdin.Fd())

	// Create a SessionManager with default terminal dimensions. JS
	// chunks call run() to start the worker goroutine and register/
	// activate sessions as needed. The WrapSessionManager binding
	// provides the same API surface (attach, switchTo, session, etc.)
	// that JS scripts expect.
	tuiMgr := termmux.NewSessionManager()
	tuiMux := termmuxmod.WrapSessionManager(ctx, engine.Adapter(), engine.Loop(), engine.Runtime(), tuiMgr, os.Stdin, stdout, termFd, "")

	// Pre-configure session target metadata so attach() registers with
	// the correct identity from the start (not assigned lazily in JS).
	// Uses session().setTarget() on the wrapped object.
	targetObj := engine.Runtime().NewObject()
	_ = targetObj.Set("name", "agent")
	_ = targetObj.Set("kind", string(termmux.SessionKindPTY))
	setTargetFn, _ := goja.AssertFunction(tuiMux.ToObject(engine.Runtime()).Get("session"))
	if setTargetFn != nil {
		sessionObj, _ := setTargetFn(goja.Undefined())
		if sessionObj != nil {
			stFn, _ := goja.AssertFunction(sessionObj.ToObject(engine.Runtime()).Get("setTarget"))
			if stFn != nil {
				_, _ = stFn(goja.Undefined(), targetObj)
			}
		}
	}

	// Start the SessionManager worker goroutine so it's ready for
	// register/activate calls from JS.
	go func() { _ = tuiMgr.Run(ctx) }()
	<-tuiMgr.Started()

	// Expose the manager to JS through the standardized osm:termmux interface.
	engine.SetGlobal("tuiMux", tuiMux)

	// Session type constants: JS uses these to create and label sessions
	// consistently. Defined here so the Go bootstrap and all JS chunks
	// agree on the session vocabulary.
	engine.SetGlobal("sessionTypes", map[string]any{
		"agent": map[string]any{
			"name": "agent",
			"kind": "pty",
		},
		"verify": map[string]any{
			"name": "verify",
			"kind": "capture",
		},
	})

	// Load the chunked script files in dependency order.
	if err := loadChunkedScript(engine); err != nil {
		return 0, nil, err
	}

	return termFd, tuiMgr, nil
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
	applyStr("base", &c.baseBranch, "")
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
	applyStr("agent-command", &c.agentCommand, "")
	if v, ok := c.config.GetCommandOption("pr-split", "agent-arg"); ok && len(c.agentArgs) == 0 {
		c.agentArgs = append(c.agentArgs, v)
	}
	applyStr("agent-model", &c.agentModel, "")
	applyStr("agent-config-dir", &c.agentConfigDir, "")
	applyStr("agent-env", &c.agentEnv, "")
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
// Returns a clear error if the working directory is not inside a git repo,
// if the repository is bare, or if the specified (or auto-detected) base
// branch does not exist.
//
// All checks use the gitops Go package (go-git/v6) — no git CLI calls.
// When baseBranch is empty or "auto", the default branch is auto-detected
// via DefaultBranch() (origin/HEAD symbolic ref → common branch names → "main").
func (c *PrSplitCommand) validateGitRepo() error {
	wd := c.workingDir()

	// Open the repo, walking up parent directories to find .git.
	repo, err := gitops.OpenDetect(wd)
	if err != nil {
		return fmt.Errorf("not a git repository (or any parent up to mount point)")
	}

	// Reject bare repositories — pr-split requires a working tree.
	isWT, err := repo.IsWorkTree()
	if err != nil {
		return fmt.Errorf("git check failed: %w", err)
	}
	if !isWT {
		return fmt.Errorf("not inside a git working tree (bare repository?)")
	}

	// Auto-detect the default branch when not explicitly specified.
	if c.baseBranch == "" || c.baseBranch == "auto" {
		detected, detectErr := repo.DefaultBranch()
		if detectErr != nil {
			slog.Warn("pr split failed to auto detect default branch falling back to main", "error", detectErr)
			c.baseBranch = "main"
		} else {
			slog.Info("pr split auto detected base branch", "branch", detected)
			c.baseBranch = detected
		}
	}

	// Validate the base branch exists (local or remote tracking ref).
	if c.baseBranch != "" {
		exists, existsErr := repo.BranchExists(c.baseBranch)
		if existsErr != nil {
			return fmt.Errorf("git check failed: %w", existsErr)
		}
		if !exists {
			return fmt.Errorf("base branch %q not found (checked local and origin remote)", c.baseBranch)
		}
	}

	return nil
}

// workingDir returns the directory to operate in. When testWorkingDir is set
// (by tests), it is used directly. Otherwise the process CWD is returned.
func (c *PrSplitCommand) workingDir() string {
	if c.testWorkingDir != "" {
		return c.testWorkingDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// forceCloseSessionManager attempts to gracefully shut down the
// SessionManager within a hard 5-second deadline. This sends
// SIGTERM→SIGKILL to all child process groups via
// SessionManager.Close → CaptureSession.Close → Process.Close.
// If the deadline expires, we log a warning and return — the
// caller then proceeds with os.Exit, which closes PTY master FDs
// and sends SIGHUP to surviving process groups as a last resort.
func forceCloseSessionManager(mgr *termmux.SessionManager) {
	if mgr == nil {
		return
	}
	closeDone := make(chan struct{})
	go func() {
		mgr.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
		slog.Info("pr split session manager closed before force exit")
	case <-time.After(5 * time.Second):
		slog.Warn("pr split session manager close timed out proceeding with force exit")
	}
}

// parseAgentEnv parses a comma-separated KEY=VALUE string into a map.
// Malformed entries (empty key, no '=') are logged as warnings and skipped.
// Whitespace around pairs is trimmed.
func parseAgentEnv(raw string) map[string]string {
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
			slog.Warn("parse agent env entry has no equals delimiter skipping", "entry", pair)
			continue
		}
		if k == "" {
			slog.Warn("parse agent env entry has empty key skipping", "entry", pair)
			continue
		}
		m[k] = v
	}
	return m
}
