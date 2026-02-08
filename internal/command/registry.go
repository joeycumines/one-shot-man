package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// Registry manages the collection of available commands.
type Registry struct {
	commands        map[string]Command
	scriptPaths     []string
	scriptDiscovery *ScriptDiscovery
}

// NewRegistryWithConfig creates a new command registry with configuration support.
func NewRegistryWithConfig(cfg *config.Config) *Registry {
	registry := &Registry{
		commands:        make(map[string]Command),
		scriptPaths:     make([]string, 0),
		scriptDiscovery: NewScriptDiscovery(cfg),
	}

	// Discover and add script paths
	discoveredPaths := registry.scriptDiscovery.DiscoverScriptPaths()
	registry.scriptPaths = append(registry.scriptPaths, discoveredPaths...)

	return registry
}

// Register adds a built-in command to the registry.
func (r *Registry) Register(cmd Command) {
	r.commands[cmd.Name()] = cmd
}

// AddScriptPath adds a directory to search for script commands.
// Duplicates are ignored.
func (r *Registry) AddScriptPath(path string) {
	// Check if path already exists
	for _, existing := range r.scriptPaths {
		if existing == path {
			return
		}
	}
	r.scriptPaths = append(r.scriptPaths, path)
}

// Get returns a command by name, checking built-in commands first, then scripts.
func (r *Registry) Get(name string) (Command, error) {
	// Check built-in commands first
	if cmd, exists := r.commands[name]; exists {
		return cmd, nil
	}

	// Check script commands
	scriptCmd, err := r.findScriptCommand(name)
	if err != nil {
		return nil, fmt.Errorf("command not found: %s", name)
	}

	return scriptCmd, nil
}

// List returns all available commands (built-in and script).
func (r *Registry) List() []string {
	var names []string

	// Add built-in commands
	for name := range r.commands {
		names = append(names, name)
	}

	// Add script commands
	scriptNames := r.findScriptCommands()
	names = append(names, scriptNames...)

	// Sort and deduplicate
	sort.Strings(names)
	return removeDuplicates(names)
}

// ListBuiltin returns only built-in commands.
func (r *Registry) ListBuiltin() []string {
	var names []string
	for name := range r.commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListScript returns only script commands.
func (r *Registry) ListScript() []string {
	names := r.findScriptCommands()
	sort.Strings(names)
	return names
}

// findScriptCommand looks for a script command by name.
func (r *Registry) findScriptCommand(name string) (Command, error) {
	for _, dir := range r.scriptPaths {
		scriptPath := filepath.Join(dir, name)

		// Check if the file exists and is executable
		if info, err := os.Stat(scriptPath); err == nil && !info.IsDir() {
			if isExecutable(info) {
				return NewScriptCommand(name, scriptPath), nil
			}
		}
	}

	return nil, fmt.Errorf("script command not found: %s", name)
}

// findScriptCommands returns all available script command names.
func (r *Registry) findScriptCommands() []string {
	var names []string

	for _, dir := range r.scriptPaths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // Skip directories that can't be read
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				info, err := entry.Info()
				if err == nil && isExecutable(info) {
					names = append(names, entry.Name())
				}
			}
		}
	}

	return names
}

// isExecutable checks if a file is executable.
func isExecutable(info os.FileInfo) bool {
	// On Unix-like systems executability is determined by execute bits.
	if runtime.GOOS != "windows" {
		mode := info.Mode()
		return mode&0111 != 0 // Check if any execute bit is set
	}

	// On Windows, the file mode bits are not a reliable indicator.
	// Conservatively treat a small set of well-known executable extensions
	// as executable (these are discovered but some — e.g. .bat/.cmd — need
	// an interpreter to be executed via cmd /c).
	name := strings.ToLower(info.Name())
	switch filepath.Ext(name) {
	case ".exe", ".com", ".bat", ".cmd":
		return true
	default:
		return false
	}
}

// removeDuplicates removes duplicate strings from a sorted slice.
func removeDuplicates(sorted []string) []string {
	if len(sorted) <= 1 {
		return sorted
	}

	result := make([]string, 0, len(sorted))
	result = append(result, sorted[0])

	for i := 1; i < len(sorted); i++ {
		if sorted[i] != sorted[i-1] {
			result = append(result, sorted[i])
		}
	}

	return result
}

// ScriptCommand represents a script-based command.
type ScriptCommand struct {
	*BaseCommand
	scriptPath string
}

// NewScriptCommand creates a new script command.
func NewScriptCommand(name, scriptPath string) *ScriptCommand {
	return &ScriptCommand{
		BaseCommand: NewBaseCommand(
			name,
			fmt.Sprintf("Script command: %s", name),
			fmt.Sprintf("%s [options] [args...]", name),
		),
		scriptPath: scriptPath,
	}
}

// Execute runs the script command.
func (c *ScriptCommand) Execute(args []string, stdout, stderr io.Writer) error {
	// Use background context for Execute without explicit context
	return c.ExecuteWithContext(context.Background(), args, stdout, stderr)
}

// ExecuteWithContext runs the script command with context support.
// When the context is cancelled, the command and its child processes are terminated.
func (c *ScriptCommand) ExecuteWithContext(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	var cmd *exec.Cmd

	// Windows: some script file types (like .bat/.cmd) must be launched
	// via the command interpreter. Detect those and invoke `cmd /c`.
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(c.scriptPath))
		if ext == ".bat" || ext == ".cmd" {
			cmd = exec.CommandContext(ctx, "cmd", append([]string{"/c", c.scriptPath}, args...)...)
		}
	}

	if cmd == nil {
		cmd = exec.CommandContext(ctx, c.scriptPath, args...)
	}

	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin

	// Set up platform-specific process attributes
	c.setupSysProcAttr(cmd)

	// Start the command
	if err := cmd.Start(); err != nil {
		return err
	}

	// Wait for command to complete or context to be cancelled
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// Context was cancelled - kill the entire process group
		c.killProcessGroup(cmd)
		// Wait for the process to actually exit
		select {
		case <-done:
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return fmt.Errorf("timeout waiting for process to terminate after context cancellation")
		}
	}
}
