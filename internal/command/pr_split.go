package command

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/termui/mux"
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
	fs.StringVar(&c.verifyCommand, "verify", "make test", "Command to verify each split")
	fs.BoolVar(&c.dryRun, "dry-run", false, "Show plan without executing")

	fs.BoolVar(&c.jsonOutput, "json", false, "Output results as JSON (combine with run or --dry-run)")

	// Claude Code execution
	fs.StringVar(&c.claudeCommand, "claude-command", "", "Claude binary path (empty = auto-detect)")
	fs.Var(&c.claudeArgs, "claude-arg", "Additional Claude CLI argument (repeatable)")
	fs.StringVar(&c.claudeModel, "claude-model", "", "Model name (provider-dependent)")
	fs.StringVar(&c.claudeConfigDir, "claude-config-dir", "", "Claude config directory override")
	fs.StringVar(&c.claudeEnv, "claude-env", "", "Extra environment variables (KEY=VALUE,KEY=VALUE)")

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
	//   pr-split.verify=make test
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
		applyConfigDefault("verify", &c.verifyCommand, "make test")
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
		"baseBranch":      c.baseBranch,
		"strategy":        c.strategy,
		"maxFiles":        c.maxFiles,
		"branchPrefix":    c.branchPrefix,
		"verifyCommand":   c.verifyCommand,
		"dryRun":          c.dryRun,
		"jsonOutput":      c.jsonOutput,
		"claudeCommand":   c.claudeCommand,
		"claudeArgs":      claudeArgsList,
		"claudeModel":     c.claudeModel,
		"claudeConfigDir": c.claudeConfigDir,
		"claudeEnv":       claudeEnvMap,
	})

	// TUI Mux — terminal multiplexer between osm and child PTY (Claude Code).
	// Uses os.Stdin/Stdout directly (not go-prompt's wrapped readers) because
	// the command-blocking model ensures go-prompt is paused during passthrough.
	termFd := int(os.Stdin.Fd())
	tuiMux := mux.New(os.Stdin, os.Stdout, termFd)
	engine.SetGlobal("tuiMux", map[string]interface{}{
		"attach": func(handle interface{}) {
			// Case 1: Direct Go StringIO interface (non-Goja callers, tests).
			if sio, ok := handle.(mux.StringIO); ok {
				if err := tuiMux.Attach(mux.WrapStringIO(sio)); err != nil {
					panic(err.Error())
				}
				return
			}
			// Case 2: Goja-wrapped AgentHandle — exported as map[string]interface{}.
			// wrapAgentHandle stores the original Go handle as _goHandle.
			if m, ok := handle.(map[string]interface{}); ok {
				if goHandle, exists := m["_goHandle"]; exists && goHandle != nil {
					if sio, ok := goHandle.(mux.StringIO); ok {
						if err := tuiMux.Attach(mux.WrapStringIO(sio)); err != nil {
							panic(err.Error())
						}
						return
					}
					// AgentHandle satisfies StringIO structurally — try io.ReadWriteCloser.
					if rwc, ok := goHandle.(io.ReadWriteCloser); ok {
						if err := tuiMux.Attach(rwc); err != nil {
							panic(err.Error())
						}
						return
					}
				}
			}
			panic("tuiMux.attach: argument must implement Send/Receive/Close (or be a wrapped AgentHandle with _goHandle)")
		},
		"detach": func() {
			if err := tuiMux.Detach(); err != nil {
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
			return result
		},
		"isClaudeActive": func() bool {
			return tuiMux.ActiveSide() == mux.SideClaude
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
	splitView := mux.NewSplitView(
		mux.WithSplitRatio(0.5),
		mux.WithMaxLines(1000),
		mux.WithToggleKey(mux.DefaultToggleKey),
		mux.WithClaudeWriter(func(data []byte) error {
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
			if splitView.ActivePane() == mux.PaneClaude {
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
	autoSplitModel := mux.NewAutoSplitModel(mux.WithAutoSplitMaxLines(1000))
	var autoSplitErr error
	var autoSplitDone chan struct{}
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
	})

	// Plan editor — expose factory so JS can create editor instances.
	engine.SetGlobal("planEditorFactory", map[string]interface{}{
		"create": func(items []interface{}) map[string]interface{} {
			editorItems := make([]mux.PlanEditorItem, 0, len(items))
			for _, raw := range items {
				m, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				item := mux.PlanEditorItem{}
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
			editor := mux.NewPlanEditor(editorItems, mux.WithOnChange(func(updated []mux.PlanEditorItem) {
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
