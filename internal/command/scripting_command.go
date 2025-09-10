package command

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// ScriptingCommand provides JavaScript scripting capabilities.
type ScriptingCommand struct {
	*BaseCommand
	interactive bool
	script      string
	testMode    bool
}

// NewScriptingCommand creates a new scripting command.
func NewScriptingCommand() *ScriptingCommand {
	return &ScriptingCommand{
		BaseCommand: NewBaseCommand(
			"script",
			"Execute JavaScript scripts with deferred/declarative API",
			"script [options] [script-file]",
		),
	}
}

// SetupFlags configures the flags for the scripting command.
func (c *ScriptingCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", false, "Start interactive scripting terminal")
	fs.BoolVar(&c.interactive, "i", false, "Start interactive scripting terminal (short form)")
	fs.StringVar(&c.script, "script", "", "JavaScript code to execute directly")
	fs.StringVar(&c.script, "e", "", "JavaScript code to execute directly (short form)")
	fs.BoolVar(&c.testMode, "test", false, "Enable test mode with verbose output")
}

// Execute runs the scripting command.
func (c *ScriptingCommand) Execute(args []string, stdout, stderr io.Writer) error {
	ctx := context.Background()
	
	// Create scripting engine
	engine := scripting.NewEngine(ctx, stdout, stderr)
	defer engine.Close()
	
	if c.testMode {
		engine.SetTestMode(true)
	}
	
	// Set up global variables
	engine.SetGlobal("args", args)
	
	// Interactive mode
	if c.interactive {
		terminal := scripting.NewTerminal(ctx, engine)
		terminal.Run()
		return nil
	}
	
	// Direct script execution
	if c.script != "" {
		script := engine.LoadScriptFromString("command-line", c.script)
		return engine.ExecuteScript(script)
	}
	
	// File-based script execution
	if len(args) == 0 {
		fmt.Fprintln(stderr, "No script file specified. Use -i for interactive mode or -e for direct execution.")
		return fmt.Errorf("no script specified")
	}
	
	scriptFile := args[0]
	
	// Resolve script path
	if !filepath.IsAbs(scriptFile) {
		// Look for script in common locations
		locations := []string{
			scriptFile,                           // Current directory
			filepath.Join("scripts", scriptFile), // Local scripts directory
		}
		
		// Try to find the script
		var found bool
		for _, path := range locations {
			if _, err := os.Stat(path); err == nil {
				scriptFile = path
				found = true
				break
			}
		}
		
		if !found {
			return fmt.Errorf("script file not found: %s", scriptFile)
		}
	}
	
	// Load and execute the script
	scriptName := filepath.Base(scriptFile)
	script, err := engine.LoadScript(scriptName, scriptFile)
	if err != nil {
		return fmt.Errorf("failed to load script: %w", err)
	}
	
	script.Description = fmt.Sprintf("Script from %s", scriptFile)
	
	return engine.ExecuteScript(script)
}