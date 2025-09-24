package command

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// CommitGenCommand provides commit message generation functionality integrated with interop
type CommitGenCommand struct {
	*BaseCommand
	interactive bool
	testMode    bool
	config      *config.Config
}

// NewCommitGenCommand creates a new commit generation command
func NewCommitGenCommand(cfg *config.Config) *CommitGenCommand {
	return &CommitGenCommand{
		BaseCommand: NewBaseCommand(
			"commit-gen",
			"Generate conventional commit messages from shared context or files",
			"commit-gen [options] [files...]",
		),
		config: cfg,
	}
}

// SetupFlags configures the flags for the commit generation command
func (c *CommitGenCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", false, "Start interactive commit generation mode")
	fs.BoolVar(&c.interactive, "i", false, "Start interactive commit generation mode (short form)")
	fs.BoolVar(&c.testMode, "test", false, "Enable test mode with verbose output")
}

// commitGenScript is the embedded JavaScript for commit generation
const commitGenScript = `
const interop = require('osm:interop');
const {openEditor: osOpenEditor, clipboardCopy} = require('osm:os');

// State keys
const STATE = {
    mode: "commit-gen",
    commitData: "commitData"
};

// Initialize the mode
ctx.run("register-mode", function () {
    tui.registerMode({
        name: STATE.mode,
        tui: {
            title: "Commit Generator",
            prompt: "(commit-gen) > ",
            enableHistory: true,
            historyFile: ".commit-gen_history"
        },
        onEnter: function () {
            if (!tui.getState(STATE.commitData)) {
                tui.setState(STATE.commitData, {});
            }
            banner();
            help();
        },
        onExit: function () {
            output.print("Exiting Commit Generator.");
        },
        commands: buildCommands()
    });

    tui.registerCommand({
        name: "commit-gen",
        description: "Switch to Commit Generator mode",
        handler: function () {
            tui.switchMode(STATE.mode);
        }
    });
});

function banner() {
    output.print("Commit Generator: Generate conventional commit messages from context");
    output.print("Type 'help' for commands. Use 'commit-gen' to return here later.");
}

function help() {
    output.print("Commands: load, generate, edit, show, copy, template, help, exit");
}

function buildCommands() {
    return {
        load: {
            description: "Load context from interop storage",
            usage: "load [name]",
            handler: function (args) {
                try {
                    if (args.length > 0) {
                        const customPath = args[0] + ".osm-interop.json";
                        interop.setCachePath(customPath);
                    }
                    
                    if (!interop.exists()) {
                        output.print("No shared context found at: " + interop.getCachePath());
                        if (args.length > 0) {
                            interop.setCachePath(".osm-interop.json");
                        }
                        return;
                    }
                    
                    const sharedContext = interop.load();
                    const items = sharedContext.ContextItems || [];
                    
                    output.print("Loaded " + items.length + " context items from: " + interop.getCachePath());
                    output.print("Source: " + (sharedContext.SourceMode || "unknown"));
                    
                    // Generate commit message immediately
                    const commitData = interop.generateCommitMessage(items, {
                        goal: (sharedContext.PromptFlow && sharedContext.PromptFlow.Goal) || ""
                    });
                    
                    tui.setState(STATE.commitData, commitData);
                    output.print("Generated commit message ready. Use 'show' to view it.");
                    
                    if (args.length > 0) {
                        interop.setCachePath(".osm-interop.json");
                    }
                } catch (e) {
                    output.print("Load error: " + (e && e.message ? e.message : e));
                }
            }
        },
        generate: {
            description: "Generate commit message from loaded context",
            handler: function () {
                try {
                    if (!interop.exists()) {
                        output.print("No shared context found. Use 'load' first.");
                        return;
                    }
                    
                    const sharedContext = interop.load();
                    const items = sharedContext.ContextItems || [];
                    
                    if (items.length === 0) {
                        output.print("No context items available for commit message generation");
                        return;
                    }
                    
                    const commitData = interop.generateCommitMessage(items, {
                        goal: (sharedContext.PromptFlow && sharedContext.PromptFlow.Goal) || ""
                    });
                    
                    tui.setState(STATE.commitData, commitData);
                    output.print("Generated new commit message. Use 'show' to view it.");
                } catch (e) {
                    output.print("Generate error: " + (e && e.message ? e.message : e));
                }
            }
        },
        edit: {
            description: "Edit the commit message",
            handler: function () {
                try {
                    const commitData = tui.getState(STATE.commitData) || {};
                    const currentMessage = (commitData.type || "feat") + ": " + (commitData.subject || "");
                    const currentBody = commitData.body || "";
                    
                    const subject = osOpenEditor("Edit commit subject", currentMessage);
                    if (subject && subject.trim()) {
                        // Parse conventional commit format
                        const parts = subject.trim().match(/^(\w+)(?:\(([^)]+)\))?\s*:\s*(.+)$/);
                        if (parts) {
                            commitData.type = parts[1];
                            commitData.scope = parts[2] || "";
                            commitData.subject = parts[3];
                        } else {
                            commitData.subject = subject.trim();
                        }
                    }
                    
                    const body = osOpenEditor("Edit commit body", currentBody);
                    if (body !== null) {
                        commitData.body = body;
                    }
                    
                    tui.setState(STATE.commitData, commitData);
                    output.print("Commit message updated. Use 'show' to view it.");
                } catch (e) {
                    output.print("Edit error: " + (e && e.message ? e.message : e));
                }
            }
        },
        show: {
            description: "Show the current commit message",
            handler: function () {
                const commitData = tui.getState(STATE.commitData) || {};
                if (!commitData.subject) {
                    output.print("No commit message generated yet. Use 'load' or 'generate'.");
                    return;
                }
                
                const conventionalCommit = (commitData.type || "feat") + 
                    (commitData.scope ? "(" + commitData.scope + ")" : "") + 
                    ": " + commitData.subject;
                
                output.print("Commit Message:");
                output.print(conventionalCommit);
                if (commitData.body && commitData.body.trim()) {
                    output.print("");
                    output.print(commitData.body);
                }
                if (commitData.footer && commitData.footer.trim()) {
                    output.print("");
                    output.print(commitData.footer);
                }
            }
        },
        copy: {
            description: "Copy commit message to clipboard",
            handler: function () {
                try {
                    const commitData = tui.getState(STATE.commitData) || {};
                    if (!commitData.subject) {
                        output.print("No commit message generated yet. Use 'load' or 'generate'.");
                        return;
                    }
                    
                    const conventionalCommit = (commitData.type || "feat") + 
                        (commitData.scope ? "(" + commitData.scope + ")" : "") + 
                        ": " + commitData.subject;
                    
                    clipboardCopy(conventionalCommit);
                    output.print("Commit message copied to clipboard!");
                } catch (e) {
                    output.print("Copy error: " + (e && e.message ? e.message : e));
                }
            }
        },
        template: {
            description: "Set custom commit message template",
            handler: function () {
                try {
                    const template = osOpenEditor("Edit commit template", 
                        "{type}: {subject}\n\n{body}\n\n{footer}");
                    if (template) {
                        output.print("Custom templates not yet implemented - using conventional commits format");
                    }
                } catch (e) {
                    output.print("Template error: " + (e && e.message ? e.message : e));
                }
            }
        },
        help: {description: "Show help", handler: help},
    };
}

// Auto-switch into commit-gen mode when this script loads if interactive
if (typeof args !== 'undefined' && args.includes('--interactive')) {
    ctx.run("enter-commit-gen", function () {
        tui.switchMode(STATE.mode);
    });
} else {
    // Non-interactive mode: just generate and print
    ctx.run("non-interactive-commit-gen", function () {
        try {
            if (interop.exists()) {
                const sharedContext = interop.load();
                // Use Go struct field names, not JSON field names when accessing from JavaScript
                const items = sharedContext.ContextItems || [];
                
                if (items.length > 0) {
                    const commitData = interop.generateCommitMessage(items, {
                        goal: (sharedContext.PromptFlow && sharedContext.PromptFlow.Goal) || ""
                    });
                    
                    const conventionalCommit = (commitData.Type || "feat") + 
                        (commitData.Scope ? "(" + commitData.Scope + ")" : "") + 
                        ": " + commitData.Subject;
                    
                    output.print(conventionalCommit);
                    if (commitData.Body && commitData.Body.trim()) {
                        output.print("");
                        output.print(commitData.Body);
                    }
                } else {
                    output.print("No context items found for commit message generation");
                }
            } else {
                output.print("No shared context found. Use 'osm code-review' or 'osm prompt-flow' first, then 'export'.");
            }
        } catch (e) {
            output.print("Error: " + (e && e.message ? e.message : e));
        }
    });
}
`

// Execute runs the commit generation command
func (c *CommitGenCommand) Execute(args []string, stdout, stderr io.Writer) error {
	ctx := context.Background()

	// Create scripting engine
	engine := scripting.NewEngine(ctx, stdout, stderr)
	defer engine.Close()

	if c.testMode {
		engine.SetTestMode(true)
	}

	// Set up global variables
	engine.SetGlobal("args", args)

	// Load the script
	script := engine.LoadScriptFromString("commit-gen", commitGenScript)
	if err := engine.ExecuteScript(script); err != nil {
		return fmt.Errorf("failed to execute commit-gen script: %w", err)
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