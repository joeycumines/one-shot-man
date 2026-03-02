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
	"strconv"
	"strings"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/ui"
)

//go:embed pr_split_template.md
var prSplitTemplate string

//go:embed pr_split_script.js
var prSplitScript string

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
		verifyCommand: "make",
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
	fs.StringVar(&c.verifyCommand, "verify", "make", "Command to verify each split (default: make)")
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
	ctx := context.Background()

	// Apply config defaults — flags override config values. Config keys
	// are namespaced under the "pr-split" command section or global:
	//   pr-split.base=develop
	//   pr-split.strategy=extension
	//   pr-split.max=8
	//   pr-split.prefix=split/
	//   pr-split.verify=make
	//   pr-split.dry-run=true
	if c.config != nil {
		applyConfigDefault := func(key string, target *string, flagDefault string) {
			if v, ok := c.config.GetCommandOption("pr-split", key); ok && (*target == flagDefault || *target == "") {
				*target = v
			}
		}
		applyConfigDefault("base", &c.baseBranch, "main")
		applyConfigDefault("strategy", &c.strategy, "directory")
		if v, ok := c.config.GetCommandOption("pr-split", "max"); ok && (c.maxFiles == 10 || c.maxFiles == 0) {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				c.maxFiles = n
			}
		}
		applyConfigDefault("prefix", &c.branchPrefix, "split/")
		applyConfigDefault("verify", &c.verifyCommand, "make")
		if v, ok := c.config.GetCommandOption("pr-split", "dry-run"); ok && !c.dryRun {
			c.dryRun = v == "true" || v == "1" || v == "yes"
		}
		applyConfigDefault("claude-command", &c.claudeCommand, "")
		if v, ok := c.config.GetCommandOption("pr-split", "claude-arg"); ok && len(c.claudeArgs) == 0 {
			c.claudeArgs = append(c.claudeArgs, v)
		}
		applyConfigDefault("claude-model", &c.claudeModel, "")
		applyConfigDefault("claude-config-dir", &c.claudeConfigDir, "")
		applyConfigDefault("claude-env", &c.claudeEnv, "")
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

	// Validate flags after config defaults are applied.
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

	engine, cleanup, err := c.PrepareEngine(ctx, stdout, stderr)
	if err != nil {
		return err
	}
	defer cleanup()

	// Inject command name for state namespacing
	const commandName = "pr-split"
	engine.SetGlobal("config", map[string]interface{}{
		"name": commandName,
	})

	// Set up global variables
	engine.SetGlobal("args", args)
	engine.SetGlobal("prSplitTemplate", prSplitTemplate)

	// Expose split configuration to JS
	claudeArgsList := make([]string, len(c.claudeArgs))
	copy(claudeArgsList, c.claudeArgs)
	claudeEnvMap := map[string]string{}
	if c.claudeEnv != "" {
		for _, pair := range strings.Split(c.claudeEnv, ",") {
			pair = strings.TrimSpace(pair)
			if k, v, ok := strings.Cut(pair, "="); ok && k != "" {
				claudeEnvMap[k] = v
			}
		}
	}
	engine.SetGlobal("prSplitConfig", map[string]interface{}{
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

	// TUI Mux — terminal multiplexer between osm and child PTY (Claude Code).
	// Uses os.Stdin directly (not go-prompt's wrapped readers) because
	// the command-blocking model ensures go-prompt is paused during passthrough.
	// stdout is injected for testability; in production it's os.Stdout.
	termFd := int(os.Stdin.Fd())
	tuiMux := termmux.New(os.Stdin, stdout, termFd)
	// attachChild is a helper that handles ErrAlreadyAttached by
	// detaching first, then retrying the attach. This prevents panics
	// when the JS handler calls attach without a prior detach.
	attachChild := func(child io.ReadWriteCloser) error {
		err := tuiMux.Attach(child)
		if err != nil && errors.Is(err, termmux.ErrAlreadyAttached) {
			_ = tuiMux.Detach()
			err = tuiMux.Attach(child)
		}
		return err
	}

	engine.SetGlobal("tuiMux", map[string]interface{}{
		"attach": func(handle interface{}) {
			// Case 1: Direct Go StringIO interface (non-Goja callers, tests).
			if sio, ok := handle.(termmux.StringIO); ok {
				if err := attachChild(termmux.WrapStringIO(sio)); err != nil {
					panic(err.Error())
				}
				return
			}
			// Case 2: Goja-wrapped AgentHandle — exported as map[string]interface{}.
			// wrapAgentHandle stores the original Go handle as _goHandle.
			if m, ok := handle.(map[string]interface{}); ok {
				if goHandle, exists := m["_goHandle"]; exists && goHandle != nil {
					if sio, ok := goHandle.(termmux.StringIO); ok {
						if err := attachChild(termmux.WrapStringIO(sio)); err != nil {
							panic(err.Error())
						}
						return
					}
					// AgentHandle satisfies StringIO structurally — try io.ReadWriteCloser.
					if rwc, ok := goHandle.(io.ReadWriteCloser); ok {
						if err := attachChild(rwc); err != nil {
							panic(err.Error())
						}
						return
					}
				}
			}
			panic("tuiMux.attach: argument must implement Send/Receive/Close (or be a wrapped AgentHandle with _goHandle)")
		},
		"detach": func() {
			err := tuiMux.Detach()
			// Detach returns nil when no child is attached (idempotent).
			// Only panic on real errors like ErrPassthroughActive (logic bug).
			if err != nil {
				panic(err.Error())
			}
		},
		"switchToClaude": func() map[string]interface{} {
			reason, err := tuiMux.RunPassthrough(ctx)
			result := map[string]interface{}{
				"reason": reason.String(),
			}
			if err != nil {
				result["error"] = err.Error()
			}
			// When the child process exits, capture its last output for
			// diagnostics. This surfaces error messages (e.g., "unknown flag",
			// "API key not found") that would otherwise be lost.
			if reason == termmux.ExitChildExit {
				if output := tuiMux.ChildExitOutput(); output != "" {
					result["childOutput"] = output
				}
			}
			return result
		},
		"isClaudeActive": func() bool {
			return tuiMux.ActiveSide() == termmux.SideClaude
		},
		"setStatus": func(status string) {
			tuiMux.SetClaudeStatus(status)
		},
		"setToggleKey": func(key int) {
			tuiMux.SetToggleKey(byte(key))
		},
		"setStatusEnabled": func(enabled bool) {
			tuiMux.SetStatusEnabled(enabled)
		},
		"setResizeFunc": func(fn func(rows, cols int)) {
			tuiMux.SetResizeFunc(func(rows, cols uint16) error {
				fn(int(rows), int(cols))
				return nil
			})
		},
	})

	// Split-view TUI — dual-pane BubbleTea model.
	splitView := ui.NewSplitView(
		ui.WithSplitRatio(0.5),
		ui.WithMaxLines(1000),
		ui.WithToggleKey(termmux.DefaultToggleKey),
		ui.WithClaudeWriter(func(data []byte) error {
			// Forward to child PTY if attached.
			_, err := tuiMux.WriteToChild(data)
			return err
		}),
	)
	engine.SetGlobal("splitView", map[string]interface{}{
		"appendOsm": func(text string) {
			splitView.AppendOsmOutput(text)
		},
		"appendClaude": func(text string) {
			splitView.AppendClaudeOutput(text)
		},
		"setClaudeStatus": func(status string) {
			splitView.SetClaudeStatus(status)
		},
		"setRatio": func(ratio float64) {
			splitView.SetSplitRatio(ratio)
		},
		"activePane": func() string {
			if splitView.ActivePane() == ui.PaneClaude {
				return "claude"
			}
			return "osm"
		},
		"run": func() error {
			return splitView.Run()
		},
	})

	// Auto-split progress TUI — pipeline visualisation for automated splits.
	// The model is created here and exposed to JS; the JS automatedSplit()
	// function drives it by calling stepStart/stepDone/appendOutput/done.
	//
	// runAsync() starts the BubbleTea program in a background goroutine so
	// JS can continue driving the pipeline synchronously while the TUI
	// renders. wait() blocks until the TUI exits (user presses q / Ctrl+C).
	//
	// The toggle key (Ctrl+]) switches to Claude's terminal mid-pipeline.
	// The onToggle callback releases BubbleTea's terminal control, runs
	// passthrough (forwarding stdin/stdout to the child PTY), then restores
	// the BubbleTea terminal when the user toggles back.
	//
	// TIMING CONTRACT (BubbleTea async race mitigation):
	//
	// BubbleTea's ReleaseTerminal() and RestoreTerminal() are async — they
	// send messages through the event loop, so their effects (exit/enter
	// alt-screen, release/acquire raw mode) are NOT immediate. Without
	// mitigation, there's a race window where RunPassthrough starts before
	// BubbleTea has actually exited alt-screen, or the user toggles again
	// before RestoreTerminal has taken effect.
	//
	// The synchronous escape writes bracket RunPassthrough to guarantee
	// correct terminal state regardless of BubbleTea's event loop timing:
	//
	//   1. BEFORE RunPassthrough: Write \x1b[?1049l synchronously to exit
	//      alt-screen. This is idempotent — harmless if BubbleTea already
	//      processed ReleaseTerminal. Guarantees terminal is in normal
	//      screen mode before the child PTY output becomes visible.
	//
	//   2. AFTER RunPassthrough: Write \x1b[?1049h\x1b[2J\x1b[H to enter
	//      alt-screen and clear. Idempotent — harmless if BubbleTea then
	//      processes RestoreTerminal (which would re-enter alt-screen,
	//      a no-op since we're already there).
	//
	// This "belt and suspenders" approach means rapid toggling (even 20+
	// cycles) cannot corrupt terminal state. The synchronous writes are the
	// "belt"; BubbleTea's async processing is the "suspenders" — either
	// one alone is sufficient, and having both is safely idempotent.
	//
	// Pre-declare so the closure can reference autoSplitModel (the variable
	// is assigned on the very next line, before the closure is ever called).
	var autoSplitModel *ui.AutoSplitModel
	autoSplitModel = ui.NewAutoSplitModel(
		ui.WithAutoSplitMaxLines(1000),
		ui.WithAutoSplitToggleKey(termmux.DefaultToggleKey),
		ui.WithAutoSplitOnToggle(func() {
			// Check if a child is attached before switching — if not,
			// there's nothing to show and we'd just blank the screen.
			if !tuiMux.HasChild() {
				autoSplitModel.SendError("No Claude process attached — cannot toggle (Ctrl+])")
				return
			}

			// Release BubbleTea's terminal control (alt-screen, raw
			// mode, input listener) so RunPassthrough gets exclusive
			// access to stdin/stdout.
			if p := autoSplitModel.Program(); p != nil {
				p.ReleaseTerminal()
			}
			// Synchronously exit alt-screen. BubbleTea's ReleaseTerminal
			// is async (processed via event loop), so we write the escape
			// sequence directly to ensure the terminal has exited alt-screen
			// before RunPassthrough starts. Idempotent if BubbleTea already
			// processed it.
			_, _ = stdout.Write([]byte("\x1b[?1049l"))

			// Switch to Claude's terminal. This blocks until the
			// user presses the toggle key again or the child exits.
			reason, err := tuiMux.RunPassthrough(ctx)
			if err != nil {
				autoSplitModel.SendError("Toggle failed: " + err.Error())
			}
			// T47: Log the exit reason at debug level instead of discarding.
			slog.Debug("RunPassthrough exit", "reason", reason.String())

			// Synchronously re-enter alt-screen and clear before
			// RestoreTerminal. This ensures BubbleTea inherits a
			// clean alt-screen regardless of event loop timing.
			_, _ = stdout.Write([]byte("\x1b[?1049h\x1b[2J\x1b[H"))

			// Restore BubbleTea's terminal control so the auto-split
			// TUI resumes rendering.
			if p := autoSplitModel.Program(); p != nil {
				p.RestoreTerminal()
			}
		}),
	)
	var autoSplitErr error
	var autoSplitDone chan struct{}

	// extractGoHandle extracts the original Go handle from a Goja-wrapped
	// AgentHandle JS object (the _goHandle field set by wrapAgentHandle).
	extractGoHandle := func(handleObj interface{}) interface{} {
		if m, ok := handleObj.(map[string]interface{}); ok {
			if gh, exists := m["_goHandle"]; exists {
				return gh
			}
		}
		return nil
	}

	engine.SetGlobal("autoSplitTUI", map[string]interface{}{
		"runAsync": func() {
			autoSplitDone = make(chan struct{})
			go func() {
				defer close(autoSplitDone)
				autoSplitErr = autoSplitModel.Run()
			}()
		},
		"wait": func() error {
			if autoSplitDone != nil {
				<-autoSplitDone
			}
			return autoSplitErr
		},
		"stepStart": func(name string) {
			autoSplitModel.SendStepStart(name)
		},
		"stepDone": func(name string, errMsg string, elapsedMs int64) {
			autoSplitModel.SendStepDone(name, errMsg, time.Duration(elapsedMs)*time.Millisecond)
		},
		"appendOutput": func(text string) {
			autoSplitModel.SendOutput(text)
		},
		"appendError": func(text string) {
			autoSplitModel.SendError(text)
		},
		"done": func(summary string) {
			autoSplitModel.SendDone(summary)
		},
		"stepDetail": func(name, detail string) {
			autoSplitModel.SendStepDetail(name, detail)
		},
		"branchStart": func(name string) {
			autoSplitModel.SendBranchVerifyStart(name)
		},
		"branchDone": func(name string, passed bool, exitCode int, elapsedMs int64, skipped, preExisting bool) {
			autoSplitModel.SendBranchVerifyDone(ui.AutoSplitBranchVerifyDoneMsg{
				Branch:   name,
				Passed:   passed,
				ExitCode: exitCode,
				Elapsed:  time.Duration(elapsedMs) * time.Millisecond,
				Skipped:  skipped,
				PreExist: preExisting,
			})
		},
		"branchOutput": func(name, line string) {
			autoSplitModel.SendBranchVerifyOutput(name, line)
		},
		"cancelled": func() bool {
			return autoSplitModel.Cancelled()
		},
		"forceCancelled": func() bool {
			return autoSplitModel.ForceCancelled()
		},
		"paused": func() bool {
			return autoSplitModel.Paused()
		},
		"quit": func() {
			autoSplitModel.Quit()
		},
		// sendWithCancel writes text to a Claude handle's PTY stdin
		// in a background goroutine, polling for cancellation every
		// 200ms. Delegates to prSplitSendWithCancel for testability.
		//
		// Returns { error: null } on success, { error: "message" } on failure.
		"sendWithCancel": func(handleObj interface{}, text string) map[string]interface{} {
			goHandle := extractGoHandle(handleObj)
			if goHandle == nil {
				return map[string]interface{}{"error": "invalid handle: no _goHandle"}
			}

			type sender interface{ Send(string) error }
			s, ok := goHandle.(sender)
			if !ok {
				return map[string]interface{}{"error": "handle does not support Send"}
			}

			var kill func()
			type signaler interface{ Signal(string) error }
			if sig, ok := goHandle.(signaler); ok {
				kill = func() { _ = sig.Signal("SIGKILL") }
			} else {
				kill = func() {} // no-op if signaling unsupported
			}

			err := prSplitSendWithCancel(
				func() error { return s.Send(text) },
				kill,
				autoSplitModel.Cancelled,
				autoSplitModel.ForceCancelled,
			)
			if err != nil {
				return map[string]interface{}{"error": err.Error()}
			}
			return map[string]interface{}{"error": nil}
		},

		// T04a: sendTextWithEnter REMOVED — it blocked the JS event loop
		// because time.Sleep in Go blocks the calling goroutine which blocks
		// the JS function call. The correct approach is to use JS async/await
		// with setTimeout for the delay (implemented in pr_split_script.js
		// sendToHandle function). sendWithCancel is kept for individual writes.
	})

	// Plan editor — expose factory so JS can create editor instances.

	engine.SetGlobal("planEditorFactory", map[string]interface{}{
		"create": func(items []interface{}) map[string]interface{} {
			editorItems := make([]ui.PlanEditorItem, 0, len(items))
			for _, raw := range items {
				m, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				item := ui.PlanEditorItem{}
				if name, ok := m["name"].(string); ok {
					item.Name = name
				}
				if branch, ok := m["branchName"].(string); ok {
					item.BranchName = branch
				}
				if desc, ok := m["description"].(string); ok {
					item.Description = desc
				}
				if files, ok := m["files"].([]interface{}); ok {
					for _, f := range files {
						if s, ok := f.(string); ok {
							item.Files = append(item.Files, s)
						}
					}
				}
				editorItems = append(editorItems, item)
			}
			editor := ui.NewPlanEditor(editorItems, ui.WithOnChange(func(updated []ui.PlanEditorItem) {
				// Silently accept changes — JS can query items after run.
			}))
			return map[string]interface{}{
				"run": func() ([]interface{}, error) {
					result, err := editor.Run()
					if err != nil {
						return nil, err
					}
					// Convert back to JS-friendly maps.
					out := make([]interface{}, len(result))
					for i, item := range result {
						out[i] = map[string]interface{}{
							"name":        item.Name,
							"files":       item.Files,
							"branchName":  item.BranchName,
							"description": item.Description,
						}
					}
					return out, nil
				},
				"items": func() []interface{} {
					result := editor.Items()
					out := make([]interface{}, len(result))
					for i, item := range result {
						out[i] = map[string]interface{}{
							"name":        item.Name,
							"files":       item.Files,
							"branchName":  item.BranchName,
							"description": item.Description,
						}
					}
					return out
				},
			}
		},
	})

	// Load the embedded script
	script := engine.LoadScriptFromString("pr-split", prSplitScript)
	if err := engine.ExecuteScript(script); err != nil {
		return fmt.Errorf("failed to execute pr-split script: %w", err)
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

// prSplitSendWithCancel runs send() in a background goroutine, polling
// cancelled/forceCancelled every 200ms. During a long PTY write (e.g. a
// 293-file classification prompt), the child process may not read fast
// enough and the write blocks. This function unblocks by killing the
// child (via kill()) when the user cancels.
//
// On cancel or forceCancel, kill() is called to SIGKILL the child and
// unblock the write goroutine, then the cancel error is returned.
func prSplitSendWithCancel(
	send func() error,
	kill func(),
	cancelled, forceCancelled func() bool,
) error {
	done := make(chan error, 1)
	go func() { done <- send() }()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return err
		case <-ticker.C:
			if forceCancelled() {
				kill()
				// Wait for the send goroutine to finish, but don't
				// block forever — the kill might not unblock the
				// write immediately on all platforms.
				select {
				case <-done:
				case <-time.After(2 * time.Second):
				}
				return errors.New("force cancelled by user")
			}
			if cancelled() {
				kill()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
				}
				return errors.New("cancelled by user")
			}
		}
	}
}
