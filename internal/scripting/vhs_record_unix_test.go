//go:build unix

package scripting

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
)

// RecorderOption configures an InputCaptureRecorder.
type RecorderOption interface {
	applyRecorder(*recorderConfig) error
}

// recorderConfig holds configuration for InputCaptureRecorder.
type recorderConfig struct {
	// Terminal dimensions (in characters for the pty)
	rows uint16
	cols uint16

	// Default timeout for Expect operations
	defaultTimeout time.Duration

	// Environment variables (additional, not replacement)
	env []string

	// Working directory
	dir string

	// Shell to use (e.g., "bash", the default)
	// This shell is spawned, and the command is typed into it.
	shell string

	// The command to execute interactively in the shell.
	// This is what gets typed after the shell starts.
	command string
	args    []string

	// VHS recording settings
	vhsSettings VHSRecordSettings

	// skipTapeOutput disables writing the .tape file on Close.
	// The test logic still runs; only file output is skipped.
	skipTapeOutput bool
}

// recorderOptionImpl implements RecorderOption.
type recorderOptionImpl func(*recorderConfig) error

func (f recorderOptionImpl) applyRecorder(c *recorderConfig) error {
	return f(c)
}

// --- Recorder Options ---

// WithRecorderSize sets the PTY dimensions in characters. Default is 24x80.
func WithRecorderSize(rows, cols uint16) RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.rows = rows
		c.cols = cols
		return nil
	})
}

// WithRecorderTimeout sets the default timeout for Expect operations.
func WithRecorderTimeout(d time.Duration) RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.defaultTimeout = d
		return nil
	})
}

// WithRecorderEnv appends environment variables for the recording session.
func WithRecorderEnv(env ...string) RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.env = append(c.env, env...)
		return nil
	})
}

// WithRecorderDir sets the working directory.
func WithRecorderDir(path string) RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.dir = path
		return nil
	})
}

// WithRecorderShell sets the shell to use (e.g. "bash", the default).
// The shell is spawned, and the command is typed into it interactively.
func WithRecorderShell(shell string) RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.shell = shell
		return nil
	})
}

// WithRecorderCommand sets the command and arguments to type into the shell.
// This command will be typed interactively after the shell starts.
func WithRecorderCommand(cmdName string, args ...string) RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.command = cmdName
		c.args = args
		return nil
	})
}

// WithRecorderVHSSettings sets the VHS recording settings.
func WithRecorderVHSSettings(settings VHSRecordSettings) RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.vhsSettings = settings
		return nil
	})
}

// WithRecorderSkipTapeOutput disables writing the .tape file on Close.
// The test logic still runs normally; only file output is skipped.
// This is used when running tests without the -record flag.
func WithRecorderSkipTapeOutput() RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.skipTapeOutput = true
		return nil
	})
}

// resolveRecorderOptions applies options and returns the config.
func resolveRecorderOptions(opts []RecorderOption) (*recorderConfig, error) {
	cfg := &recorderConfig{
		rows:           24,
		cols:           80,
		defaultTimeout: 30 * time.Second,
		shell:          "bash",
		vhsSettings:    DefaultVHSRecordSettings(),
	}
	for _, opt := range opts {
		if err := opt.applyRecorder(cfg); err != nil {
			return nil, fmt.Errorf("failed to apply recorder option: %w", err)
		}
	}
	return cfg, nil
}

// InputCaptureRecorder wraps a termtest.Console and captures all input sent to it.
// After the session ends, it can convert the captured input to a VHS tape using
// the same logic as `vhs record` (inputToTape).
//
// This approach avoids "double handling" - instead of manually constructing tape
// commands alongside test execution, we capture the actual input and convert it.
type InputCaptureRecorder struct {
	console  *termtest.Console
	input    *bytes.Buffer
	config   VHSRecordSettings
	tapePath string
	closed   bool

	// The command that was typed - for documentation in tape
	typedCommand string
	typedArgs    []string

	// skipTapeOutput disables writing the .tape file on Close.
	skipTapeOutput bool
}

// NewInputCaptureRecorder creates a new recorder that wraps a termtest.Console
// and captures all input for later conversion to VHS tape format.
//
// The recorder spawns a shell (defaulting to bash) and the test must
// type commands into it. This ensures the generated .tape files will
// replay correctly with the same shell and commands.
func NewInputCaptureRecorder(ctx context.Context, tapePath string, opts ...RecorderOption) (*InputCaptureRecorder, error) {
	cfg, err := resolveRecorderOptions(opts)
	if err != nil {
		return nil, err
	}

	// Sync shell setting between termtest and VHS
	cfg.vhsSettings.Shell = cfg.shell

	// Build termtest options - spawn the shell (not the command directly)
	termtestOpts := []termtest.ConsoleOption{
		termtest.WithSize(cfg.rows, cfg.cols),
		termtest.WithDefaultTimeout(cfg.defaultTimeout),
		termtest.WithCommand(cfg.shell), // Spawn shell, not the command
	}

	if len(cfg.env) > 0 {
		termtestOpts = append(termtestOpts, termtest.WithEnv(cfg.env))
	}
	if cfg.dir != "" {
		termtestOpts = append(termtestOpts, termtest.WithDir(cfg.dir))
	}

	console, err := termtest.NewConsole(ctx, termtestOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create console: %w", err)
	}

	return &InputCaptureRecorder{
		console:        console,
		input:          &bytes.Buffer{},
		config:         cfg.vhsSettings,
		tapePath:       tapePath,
		typedCommand:   cfg.command,
		typedArgs:      cfg.args,
		skipTapeOutput: cfg.skipTapeOutput,
	}, nil
}

// WithSettings sets custom VHS recording settings.
// Deprecated: Use WithRecorderVHSSettings option instead.
func (r *InputCaptureRecorder) WithSettings(settings VHSRecordSettings) *InputCaptureRecorder {
	r.config = settings
	return r
}

// Console returns the underlying termtest.Console for interaction.
func (r *InputCaptureRecorder) Console() *termtest.Console {
	return r.console
}

// SendKey sends a key to the console AND records it.
// This uses WriteString for raw characters and escape sequences.
func (r *InputCaptureRecorder) SendKey(key string) error {
	r.input.WriteString(key)
	_, err := r.console.WriteString(key)
	return err
}

// SendText sends text to the console AND records it.
func (r *InputCaptureRecorder) SendText(text string) error {
	r.input.WriteString(text)
	_, err := r.console.WriteString(text)
	return err
}

// TypeCommand types the configured command into the shell.
// This should be called after waiting for the shell prompt.
func (r *InputCaptureRecorder) TypeCommand() error {
	if r.typedCommand == "" {
		return nil
	}

	// Build the full command line
	// WARNING: This filthy hack is CRITICAL for the recording of the .tape files, which need to resolve the script path correctly.
	// TODO: Real solution for the remapping of paths, also quoting of args is likely completely wrong
	typedCommand := r.typedCommand
	typedArgs := r.typedArgs
	if typedCommand == "osm" {
		var foundScript bool
		// [FIX] Don't prepend "../../../" when running from repoRoot - scripts/* is already relative to repo
		var scriptArgPrefix string
		for i, arg := range typedArgs {
			if foundScript {
				// Only prepend "../../../" if NOT running from repoRoot (dir not set or different)
				// When repoRoot is set via WithRecorderDir, we're already at correct path
				if r.console != nil && !r.closed {
					// Check if console dir matches what we expect (repoRoot at project level)
					// This is heuristic - if typed command already has scripts/ it was already adjusted
					// We'll only prepend "../../../" if the path looks like it came from docs/visuals/gifs/
					if strings.HasPrefix(arg, "scripts/") && !strings.HasPrefix(typedArgs[i], "../../../") {
						scriptArgPrefix = "../../../"
					}
					typedArgs = slices.Clone(typedArgs)
					typedArgs[i] = scriptArgPrefix + arg
					break
				}
			} else if arg == "script" {
				foundScript = true
			} else if !strings.HasPrefix(arg, "-") {
				break
			}
		}
	}
	cmdLine := typedCommand
	for _, arg := range typedArgs {
		// Quote args with spaces
		if strings.ContainsAny(arg, " \t") {
			cmdLine += " " + fmt.Sprintf("%q", arg)
		} else {
			cmdLine += " " + arg
		}
	}

	// Type each character with a small delay for visual effect
	for _, ch := range cmdLine {
		if err := r.SendKey(string(ch)); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

// Close closes the console and saves the captured input as a VHS tape.
// If skipTapeOutput is true, tape file writing is skipped but the console is still closed.
func (r *InputCaptureRecorder) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	closeErr := r.console.Close()
	if r.skipTapeOutput {
		return closeErr
	}
	if saveErr := r.saveTape(); saveErr != nil {
		return saveErr
	}
	return closeErr
}

// Snapshot returns a snapshot of the console buffer.
func (r *InputCaptureRecorder) Snapshot() termtest.Snapshot {
	return r.console.Snapshot()
}

// Expect waits for the expected content in the console.
func (r *InputCaptureRecorder) Expect(ctx context.Context, snap termtest.Snapshot, matcher termtest.Condition, desc string) error {
	return r.console.Expect(ctx, snap, matcher, desc)
}

// WaitExit waits for the process to exit.
func (r *InputCaptureRecorder) WaitExit(ctx context.Context) (int, error) {
	return r.console.WaitExit(ctx)
}

// String returns the current console buffer as a string.
func (r *InputCaptureRecorder) String() string {
	return r.console.String()
}

// RecordSleep records a sleep command at the current point.
// This is used to insert pauses in the tape for better visualization.
func (r *InputCaptureRecorder) RecordSleep(duration time.Duration) {
	// We'll store this as a special marker that inputToTape can recognize
	r.input.WriteString(fmt.Sprintf("\x00SLEEP:%s\x00", duration))
}

// RecordComment records a comment in the tape at the current point.
// Comments are for documentation and don't affect playback.
func (r *InputCaptureRecorder) RecordComment(text string) {
	r.input.WriteString(fmt.Sprintf("\x00COMMENT:%s\x00", text))
}

// RecordWait records a wait-for-pattern command in the tape.
// This tells VHS to wait for the pattern to appear before continuing.
func (r *InputCaptureRecorder) RecordWait(pattern string, timeout string) {
	r.input.WriteString(fmt.Sprintf("\x00WAIT:%s:%s\x00", timeout, pattern))
}

// saveTape converts captured input to VHS tape format and saves it.
func (r *InputCaptureRecorder) saveTape() error {
	if err := os.MkdirAll(filepath.Dir(r.tapePath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	tape := r.convertToTape()
	return os.WriteFile(r.tapePath, []byte(tape), 0644)
}

// convertToTape converts the captured input buffer to VHS tape format.
// This uses logic similar to vhs record's inputToTape function.
func (r *InputCaptureRecorder) convertToTape() string {
	var buf strings.Builder

	// Write settings header
	buf.WriteString(r.generateSettingsBlock())

	// Convert input to tape commands
	input := r.input.String()
	buf.WriteString(r.inputToTape(input))

	return buf.String()
}

// generateSettingsBlock creates the VHS settings preamble.
func (r *InputCaptureRecorder) generateSettingsBlock() string {
	var buf strings.Builder
	s := r.config

	// Output file
	if s.OutputGIF != "" {
		buf.WriteString("Output " + s.OutputGIF + "\n\n")
	}

	// Settings
	buf.WriteString("# Terminal Settings\n")
	if s.Width > 0 {
		buf.WriteString(fmt.Sprintf("Set Width %d\n", s.Width))
	}
	if s.Height > 0 {
		buf.WriteString(fmt.Sprintf("Set Height %d\n", s.Height))
	}
	if s.FontSize > 0 {
		buf.WriteString(fmt.Sprintf("Set FontSize %d\n", s.FontSize))
	}
	if s.FontFamily != "" {
		buf.WriteString(fmt.Sprintf("Set FontFamily %q\n", s.FontFamily))
	}
	if s.Shell != "" {
		buf.WriteString(fmt.Sprintf("Set Shell %q\n", s.Shell))
	}
	buf.WriteString(fmt.Sprintf("Set TypingSpeed %s\n", s.TypingSpeed))
	if s.PlaybackSpeed != 1.0 {
		buf.WriteString(fmt.Sprintf("Set PlaybackSpeed %.1f\n", s.PlaybackSpeed))
	}
	if s.WindowBar != "" {
		buf.WriteString(fmt.Sprintf("Set WindowBar %s\n", s.WindowBar))
	}
	buf.WriteString(fmt.Sprintf("Set Padding %d\n", s.Padding))
	buf.WriteString(fmt.Sprintf("Set Margin %d\n", s.Margin))
	if s.MarginFill != "" {
		buf.WriteString(fmt.Sprintf("Set MarginFill %q\n", s.MarginFill))
	}
	if s.BorderRadius > 0 {
		buf.WriteString(fmt.Sprintf("Set BorderRadius %d\n", s.BorderRadius))
	}
	buf.WriteString(fmt.Sprintf("Set CursorBlink %t\n", s.CursorBlink))

	// Theme
	buf.WriteString("\n# Theme\n")
	buf.WriteString(fmt.Sprintf(`Set Theme { "name": %q, "background": %q, "foreground": %q, "black": %q, "red": %q, "green": %q, "yellow": %q, "blue": %q, "magenta": %q, "cyan": %q, "white": %q, "brightBlack": %q, "brightRed": %q, "brightGreen": %q, "brightYellow": %q, "brightBlue": %q, "brightMagenta": %q, "brightCyan": %q, "brightWhite": %q, "cursor": %q, "selection": "#44475a" }`,
		s.Theme.Name,
		s.Theme.Background,
		s.Theme.Foreground,
		s.Theme.Black,
		s.Theme.Red,
		s.Theme.Green,
		s.Theme.Yellow,
		s.Theme.Blue,
		s.Theme.Magenta,
		s.Theme.Cyan,
		s.Theme.White,
		s.Theme.Black,
		s.Theme.Red,
		s.Theme.Green,
		s.Theme.Yellow,
		s.Theme.Blue,
		s.Theme.Magenta,
		s.Theme.Cyan,
		s.Theme.White,
		s.Theme.Foreground,
	))
	buf.WriteString("\n\n# Recorded Actions\n")

	return buf.String()
}

// EscapeSequences maps terminal escape sequences to VHS commands.
// Same as in vhs record.go
var EscapeSequences = map[string]string{
	"\x1b[A":  "Up",
	"\x1b[B":  "Down",
	"\x1b[C":  "Right",
	"\x1b[D":  "Left",
	"\x1b[1~": "Home",
	"\x1b[2~": "Insert",
	"\x1b[3~": "Delete",
	"\x1b[4~": "End",
	"\x1b[5~": "PageUp",
	"\x1b[6~": "PageDown",
	"\x01":    "Ctrl+A",
	"\x02":    "Ctrl+B",
	"\x03":    "Ctrl+C",
	"\x04":    "Ctrl+D",
	"\x05":    "Ctrl+E",
	"\x06":    "Ctrl+F",
	"\x07":    "Ctrl+G",
	"\x08":    "Backspace",
	"\x09":    "Tab",
	"\x0b":    "Ctrl+K",
	"\x0c":    "Ctrl+L",
	"\x0d":    "Enter",
	"\x0e":    "Ctrl+N",
	"\x0f":    "Ctrl+O",
	"\x10":    "Ctrl+P",
	"\x11":    "Ctrl+Q",
	"\x12":    "Ctrl+R",
	"\x13":    "Ctrl+S",
	"\x14":    "Ctrl+T",
	"\x15":    "Ctrl+U",
	"\x16":    "Ctrl+V",
	"\x17":    "Ctrl+W",
	"\x18":    "Ctrl+X",
	"\x19":    "Ctrl+Y",
	"\x1a":    "Ctrl+Z",
	"\x1b":    "Escape",
	"\x7f":    "Backspace",
}

// inputToTape converts raw terminal input to VHS tape commands.
// This is similar to vhs record's inputToTape but adapted for our use.
func (r *InputCaptureRecorder) inputToTape(input string) string {
	var result strings.Builder

	// Process markers (sleep, comment, wait) and regular input
	parts := strings.Split(input, "\x00")
	for _, part := range parts {
		if strings.HasPrefix(part, "SLEEP:") {
			duration := strings.TrimPrefix(part, "SLEEP:")
			result.WriteString(fmt.Sprintf("Sleep %s\n", duration))
			continue
		}
		if strings.HasPrefix(part, "COMMENT:") {
			text := strings.TrimPrefix(part, "COMMENT:")
			result.WriteString(fmt.Sprintf("# %s\n", text))
			continue
		}
		if strings.HasPrefix(part, "WAIT:") {
			rest := strings.TrimPrefix(part, "WAIT:")
			idx := strings.Index(rest, ":")
			if idx > 0 {
				timeout := rest[:idx]
				pattern := rest[idx+1:]
				result.WriteString(fmt.Sprintf("Wait@%s /%s/\n", timeout, pattern))
			}
			continue
		}

		// Process escape sequences (ensure longer sequences are replaced first to avoid prefix collisions)
		s := part
		// Build a slice of sequences sorted by descending length for deterministic prefix handling.
		seqs := make([]string, 0, len(EscapeSequences))
		for seq := range EscapeSequences {
			seqs = append(seqs, seq)
		}
		sort.Slice(seqs, func(i, j int) bool { return len(seqs[i]) > len(seqs[j]) })
		for _, seq := range seqs {
			cmd := EscapeSequences[seq]
			s = strings.ReplaceAll(s, seq, "\n"+cmd+"\n")
		}

		// Clean up and format
		lines := strings.Split(s, "\n")

		// Build a deterministic set of command names for quick lookup.
		cmdSet := make(map[string]struct{}, len(EscapeSequences))
		for _, cmd := range EscapeSequences {
			cmdSet[cmd] = struct{}{}
		}

		for _, line := range lines {
			orig := line
			trimmed := strings.TrimSpace(line)

			// If trimming yields empty but original isn't empty, it was whitespace-only input (e.g. a space).
			if trimmed == "" {
				if orig != "" {
					// Preserve whitespace-only input as a Type command (e.g. Type " ")
					result.WriteString(fmt.Sprintf("Type %s\n", quoteVHSString(orig)))
				}
				continue
			}

			// Check if it's a known command (use trimmed form for matching)
			if _, ok := cmdSet[trimmed]; ok {
				result.WriteString(trimmed + "\n")
				continue
			}

			// Otherwise it's text to type - preserve original spacing
			result.WriteString(fmt.Sprintf("Type %s\n", quoteVHSString(orig)))
		}
	}

	return result.String()
}

// quoteVHSString quotes a string for VHS tape format.
func quoteVHSString(s string) string {
	if !strings.ContainsRune(s, '"') {
		return `"` + s + `"`
	}
	if !strings.ContainsRune(s, '\'') {
		return "'" + s + "'"
	}
	if !strings.ContainsRune(s, '`') {
		return "`" + s + "`"
	}
	// All quotes present - use double quotes and hope for the best
	return `"` + strings.ReplaceAll(s, `"`, `'`) + `"`
}
func TestInputToTape_EscapeSequenceOrdering(t *testing.T) {
	r := &InputCaptureRecorder{}
	// Single sequence should map to the command directly
	got := r.inputToTape("\x1b[B")
	if !strings.Contains(got, "Down\n") {
		t.Fatalf("expected Down, got %q", got)
	}
	if strings.Contains(got, "Escape\nType") {
		t.Fatalf("unexpected Escape fallback, got %q", got)
	}

	// Up sequence should also map directly
	got = r.inputToTape("\x1b[A")
	if !strings.Contains(got, "Up\n") {
		t.Fatalf("expected Up, got %q", got)
	}
}

func TestInputToTape_PreservesSpace(t *testing.T) {
	r := &InputCaptureRecorder{}
	got := r.inputToTape(" ")
	if !strings.Contains(got, "Type \" \"") {
		t.Fatalf("expected Type \" \" for single space, got %q", got)
	}
}

func TestInputToTape_SpaceAfterEscape(t *testing.T) {
	r := &InputCaptureRecorder{}
	got := r.inputToTape("\x1b[A ")
	// Expect Up followed by a Type " " command
	if !strings.Contains(got, "Up\nType \" \"") {
		t.Fatalf("expected Up followed by Type \" \", got %q", got)
	}
}

func TestInputToTape_PreservesMultipleSpaces(t *testing.T) {
	r := &InputCaptureRecorder{}
	got := r.inputToTape("   ")
	if !strings.Contains(got, "Type \"   \"") {
		t.Fatalf("expected Type \"   \" for multiple spaces, got %q", got)
	}
}

// VHSRecorder provides an API for recording terminal sessions using `vhs record`.
// This is an alternative approach that wraps the vhs record command directly.
type VHSRecorder struct {
	tapePath string
	command  string
	args     []string
	env      []string
	dir      string
	settings VHSRecordSettings
}

// VHSRecordSettings holds settings that will be prepended to the tape.
type VHSRecordSettings struct {
	Width         int
	Height        int
	FontSize      int
	FontFamily    string
	Theme         VHSTheme
	TypingSpeed   string
	PlaybackSpeed float64
	Shell         string
	WindowBar     string
	Padding       int
	Margin        int
	MarginFill    string
	BorderRadius  int
	CursorBlink   bool
	OutputGIF     string
}

// DefaultVHSRecordSettings returns default recording settings.
// VHS Width/Height are in PIXELS, not characters.
// Typical values: 800-1200 width, 400-600 height.
func DefaultVHSRecordSettings() VHSRecordSettings {
	return VHSRecordSettings{
		Width:         1000,
		Height:        600,
		FontSize:      16,
		Theme:         VHSDarkTheme,
		TypingSpeed:   "30ms",
		PlaybackSpeed: 0.7,
		Shell:         "bash",
		WindowBar:     "Colorful",
		Padding:       20,
		Margin:        10,
		MarginFill:    "#1a1b26",
		BorderRadius:  8,
		CursorBlink:   true,
	}
}

// NewVHSRecorder creates a new VHS recorder.
func NewVHSRecorder(tapePath string, command string, args ...string) *VHSRecorder {
	return &VHSRecorder{
		tapePath: tapePath,
		command:  command,
		args:     args,
		settings: DefaultVHSRecordSettings(),
	}
}

// WithSettings sets custom VHS recording settings.
func (r *VHSRecorder) WithSettings(settings VHSRecordSettings) *VHSRecorder {
	r.settings = settings
	return r
}

// WithEnv adds environment variables for the recording session.
func (r *VHSRecorder) WithEnv(env ...string) *VHSRecorder {
	r.env = append(r.env, env...)
	return r
}

// WithDir sets the working directory for the recording session.
func (r *VHSRecorder) WithDir(dir string) *VHSRecorder {
	r.dir = dir
	return r
}

// ExecuteVHS runs VHS on the recorded tape to generate the GIF.
func (r *VHSRecorder) ExecuteVHS(ctx context.Context) error {
	vhsPath, err := exec.LookPath("vhs")
	if err != nil {
		return fmt.Errorf("vhs not found: %w", err)
	}

	cmd := exec.CommandContext(ctx, vhsPath, r.tapePath)
	cmd.Dir = filepath.Dir(r.tapePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("vhs execution failed: %w\nOutput: %s", err, output)
	}

	return nil
}
