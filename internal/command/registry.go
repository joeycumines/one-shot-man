package command

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// Registry manages the collection of available commands.
type Registry struct {
	commands        map[string]Command
	scriptPaths     []string
	scriptDiscovery *ScriptDiscovery
	config          *config.Config
}

// NewRegistryWithConfig creates a new command registry with configuration support.
func NewRegistryWithConfig(cfg *config.Config) *Registry {
	registry := &Registry{
		commands:        make(map[string]Command),
		scriptDiscovery: NewScriptDiscovery(cfg),
		config:          cfg,
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
	if slices.Contains(r.scriptPaths, path) {
		return
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
	slices.Sort(names)
	return removeDuplicates(names)
}

// listBuiltin returns only built-in commands.
func (r *Registry) listBuiltin() []string {
	var names []string
	for name := range r.commands {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// listScript returns only script commands.
func (r *Registry) listScript() []string {
	names := r.findScriptCommands()
	slices.Sort(names)
	return names
}

// findScriptCommand looks for a script command by name.
// JS files (.js extension or "osm script" shebang) are executed in-process
// via the Goja engine. All other executable files use external process execution.
func (r *Registry) findScriptCommand(name string) (Command, error) {
	for _, dir := range r.scriptPaths {
		scriptPath := filepath.Join(dir, name)

		info, err := os.Stat(scriptPath)
		if err != nil || info.IsDir() {
			continue
		}

		isExec := isExecutable(info)
		isJS := strings.EqualFold(filepath.Ext(name), ".js")

		// On Unix: script must have exec bits OR be a .js file.
		// On Windows: .js files are discovered by extension (no exec bits).
		if !isExec && !isJS {
			continue
		}

		// Peek at the file to determine execution strategy.
		peek := peekScriptFile(scriptPath)
		if peek.kind == scriptKindJS {
			return newJSScriptCommand(name, scriptPath, r.config, peek), nil
		}

		// Non-JS or unrecognized: fall through to external execution.
		if isExec {
			return newScriptCommand(name, scriptPath), nil
		}
	}

	return nil, fmt.Errorf("script command not found: %s", name)
}

// findScriptCommands returns all available script command names.
// JS files (.js extension) are included on all platforms, even on Windows
// where they lack execute bits.
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
				if err != nil {
					continue
				}
				if isExecutable(info) || strings.EqualFold(filepath.Ext(entry.Name()), ".js") {
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

// scriptKind determines how a discovered script file is executed.
type scriptKind int

const (
	// scriptKindExternal means the script runs as an external process.
	scriptKindExternal scriptKind = iota
	// scriptKindJS means the script runs in-process via the Goja engine.
	scriptKindJS
)

// scriptPeekInfo holds the result of peeking at a script file to determine
// its execution strategy and any shebang-derived flags.
type scriptPeekInfo struct {
	kind        scriptKind
	interactive bool // from shebang -i flag
	testMode    bool // from shebang --test flag
}

// peekScriptFile examines a file to determine how it should be executed.
// Extension-based detection (.js) is tried first (cheap, cross-platform).
// If the extension doesn't indicate JS, the shebang line is checked for
// "osm script". Shebang flags (-i, --test) are extracted for JS files.
func peekScriptFile(path string) scriptPeekInfo {
	// Fast path: check extension first.
	if strings.EqualFold(filepath.Ext(path), ".js") {
		info := scriptPeekInfo{kind: scriptKindJS}
		info.parseShebangFlags(path)
		return info
	}

	// Slow path: read first line for shebang.
	f, err := os.Open(path)
	if err != nil {
		return scriptPeekInfo{kind: scriptKindExternal}
	}
	defer f.Close()

	var firstLine string
	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		firstLine = scanner.Text()
	}

	if !strings.Contains(firstLine, "osm script") {
		return scriptPeekInfo{kind: scriptKindExternal}
	}

	info := scriptPeekInfo{kind: scriptKindJS}
	info.parseShebangFlagsFromLine(firstLine)
	return info
}

// parseShebangFlags reads the first line of the file and extracts osm script flags.
func (info *scriptPeekInfo) parseShebangFlags(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return
	}
	info.parseShebangFlagsFromLine(scanner.Text())
}

// parseShebangFlagsFromLine extracts flags from a shebang line containing "osm script".
func (info *scriptPeekInfo) parseShebangFlagsFromLine(line string) {
	_, rest, found := strings.Cut(line, "osm script")
	if !found {
		return
	}
	rest = strings.TrimSpace(rest)
	for _, token := range strings.Fields(rest) {
		switch token {
		case "-i", "-interactive", "--interactive":
			info.interactive = true
		case "--test":
			info.testMode = true
		}
	}
}

// scriptCommand represents a script-based command.
type scriptCommand struct {
	*BaseCommand
	scriptPath string
}

// newScriptCommand creates a new script command.
func newScriptCommand(name, scriptPath string) *scriptCommand {
	return &scriptCommand{
		BaseCommand: NewBaseCommand(
			name,
			fmt.Sprintf("Script command: %s", name),
			fmt.Sprintf("%s [options] [args...]", name),
		),
		scriptPath: scriptPath,
	}
}

// Execute runs the script command.
func (c *scriptCommand) Execute(args []string, stdout, stderr io.Writer) error {
	// Use background context for Execute without explicit context
	return c.ExecuteWithContext(context.Background(), args, stdout, stderr)
}

// ExecuteWithContext runs the script command with context support.
// When the context is cancelled, the command and its child processes are terminated.
func (c *scriptCommand) ExecuteWithContext(ctx context.Context, args []string, stdout, stderr io.Writer) error {
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
