//go:build unix

package vhs

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
)

// RecorderOption configures an InputCaptureRecorder.
type RecorderOption interface {
	applyRecorder(*recorderConfig) error
}

// recorderConfig holds configuration for InputCaptureRecorder.
type recorderConfig struct {
	rows           uint16
	cols           uint16
	defaultTimeout time.Duration
	env            []string
	dir            string
	shell          string
	command        string
	args           []string
	repoRoot       string
	vhsConfig      VHSConfig
	skipTapeOutput bool
}

// recorderOptionImpl implements RecorderOption.
type recorderOptionImpl func(*recorderConfig) error

func (f recorderOptionImpl) applyRecorder(c *recorderConfig) error {
	return f(c)
}

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
func WithRecorderShell(shell string) RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.shell = shell
		return nil
	})
}

// WithRecorderCommand sets the command and arguments to type into the shell.
func WithRecorderCommand(cmdName string, args ...string) RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.command = cmdName
		c.args = args
		return nil
	})
}

// WithRecorderVHSConfig sets the VHS recording configuration.
func WithRecorderVHSConfig(settings VHSConfig) RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.vhsConfig = settings
		return nil
	})
}

// WithRecorderSkipTapeOutput disables writing the .tape file on Close.
func WithRecorderSkipTapeOutput() RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.skipTapeOutput = true
		return nil
	})
}

// WithRecorderRepoRoot sets the repository root path for path remapping in
// generated tape files. When empty (the default), no path remapping is performed.
func WithRecorderRepoRoot(root string) RecorderOption {
	return recorderOptionImpl(func(c *recorderConfig) error {
		c.repoRoot = root
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
		vhsConfig: VHSConfig{
			PixelWidth:    1000,
			PixelHeight:   600,
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
		},
	}
	for _, opt := range opts {
		if err := opt.applyRecorder(cfg); err != nil {
			return nil, fmt.Errorf("failed to apply recorder option: %w", err)
		}
	}
	return cfg, nil
}

// InputCaptureRecorder wraps a termtest.Console and captures all input sent to it.
// After the session ends, it can convert the captured input to a VHS tape.
type InputCaptureRecorder struct {
	console  *termtest.Console
	input    *bytes.Buffer
	config   VHSConfig
	tapePath string
	repoRoot string
	closed   bool

	typedCommand string
	typedArgs    []string

	skipTapeOutput bool
}

// NewInputCaptureRecorder creates a new recorder that wraps a termtest.Console
// and captures all input for later conversion to VHS tape format.
func NewInputCaptureRecorder(ctx context.Context, tapePath string, opts ...RecorderOption) (*InputCaptureRecorder, error) {
	cfg, err := resolveRecorderOptions(opts)
	if err != nil {
		return nil, err
	}

	cfg.vhsConfig.Shell = cfg.shell

	termtestOpts := []termtest.ConsoleOption{
		termtest.WithSize(cfg.rows, cfg.cols),
		termtest.WithDefaultTimeout(cfg.defaultTimeout),
		termtest.WithCommand(cfg.shell),
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
		config:         cfg.vhsConfig,
		tapePath:       tapePath,
		repoRoot:       cfg.repoRoot,
		typedCommand:   cfg.command,
		typedArgs:      cfg.args,
		skipTapeOutput: cfg.skipTapeOutput,
	}, nil
}

// Console returns the underlying termtest.Console for interaction.
func (r *InputCaptureRecorder) Console() *termtest.Console {
	return r.console
}

// SendKey sends a key to the console AND records it.
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
func (r *InputCaptureRecorder) TypeCommand() error {
	if r.typedCommand == "" {
		return nil
	}

	typedCommand := r.typedCommand
	typedArgs := r.typedArgs
	if typedCommand == "osm" && r.repoRoot != "" {
		var foundScript bool
		for i, arg := range typedArgs {
			if foundScript {
				if !filepath.IsAbs(arg) {
					absFile := filepath.Join(r.repoRoot, arg)
					absTapeDir, err := filepath.Abs(filepath.Dir(r.tapePath))
					if err == nil {
						if rel, err := filepath.Rel(absTapeDir, absFile); err == nil {
							typedArgs = slices.Clone(typedArgs)
							typedArgs[i] = rel
						}
					}
				}
				break
			} else if arg == "script" {
				foundScript = true
			} else if !strings.HasPrefix(arg, "-") {
				break
			}
		}
	}
	var cmdLine strings.Builder
	cmdLine.WriteString(typedCommand)
	for _, arg := range typedArgs {
		if strings.ContainsAny(arg, " \t") {
			cmdLine.WriteString(" " + escapeVHSString(arg))
		} else {
			cmdLine.WriteString(" " + arg)
		}
	}

	for _, ch := range cmdLine.String() {
		if err := r.SendKey(string(ch)); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

// Close closes the console and saves the captured input as a VHS tape.
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

// ExpectFull polls the full console buffer every 100ms for the target string.
func (r *InputCaptureRecorder) ExpectFull(ctx context.Context, target string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if strings.Contains(r.console.String(), target) {
				return nil
			}
			return ctx.Err()
		case <-ticker.C:
			if strings.Contains(r.console.String(), target) {
				return nil
			}
		}
	}
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
func (r *InputCaptureRecorder) RecordSleep(duration time.Duration) {
	r.input.WriteString(fmt.Sprintf("\x00SLEEP:%s\x00", duration))
}

// RecordComment records a comment in the tape at the current point.
func (r *InputCaptureRecorder) RecordComment(text string) {
	r.input.WriteString(fmt.Sprintf("\x00COMMENT:%s\x00", text))
}

// RecordWait records a wait-for-pattern command in the tape.
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
func (r *InputCaptureRecorder) convertToTape() string {
	var buf strings.Builder

	buf.WriteString(r.generateSettingsBlock())

	input := r.input.String()
	buf.WriteString(r.inputToTape(input))

	return buf.String()
}

// generateSettingsBlock creates the VHS settings preamble.
func (r *InputCaptureRecorder) generateSettingsBlock() string {
	var buf strings.Builder
	s := r.config

	if s.OutputGIF != "" {
		buf.WriteString("Output " + s.OutputGIF + "\n\n")
	}

	buf.WriteString("# Terminal Settings\n")
	if s.PixelWidth > 0 {
		buf.WriteString(fmt.Sprintf("Set Width %d\n", s.PixelWidth))
	}
	if s.PixelHeight > 0 {
		buf.WriteString(fmt.Sprintf("Set Height %d\n", s.PixelHeight))
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
func (r *InputCaptureRecorder) inputToTape(input string) string {
	var result strings.Builder

	parts := strings.SplitSeq(input, "\x00")
	for part := range parts {
		if duration, ok := strings.CutPrefix(part, "SLEEP:"); ok {
			result.WriteString(fmt.Sprintf("Sleep %s\n", duration))
			continue
		}
		if text, ok := strings.CutPrefix(part, "COMMENT:"); ok {
			result.WriteString(fmt.Sprintf("# %s\n", text))
			continue
		}
		if rest, ok := strings.CutPrefix(part, "WAIT:"); ok {
			idx := strings.Index(rest, ":")
			if idx > 0 {
				timeout := rest[:idx]
				pattern := rest[idx+1:]
				result.WriteString(fmt.Sprintf("Wait@%s /%s/\n", timeout, pattern))
			}
			continue
		}

		s := part
		seqs := make([]string, 0, len(EscapeSequences))
		for seq := range EscapeSequences {
			seqs = append(seqs, seq)
		}
		slices.SortFunc(seqs, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
		for _, seq := range seqs {
			cmd := EscapeSequences[seq]
			s = strings.ReplaceAll(s, seq, "\n"+cmd+"\n")
		}

		lines := strings.Split(s, "\n")

		cmdSet := make(map[string]struct{}, len(EscapeSequences))
		for _, cmd := range EscapeSequences {
			cmdSet[cmd] = struct{}{}
		}

		for _, line := range lines {
			orig := line
			trimmed := strings.TrimSpace(line)

			if trimmed == "" {
				if orig != "" {
					result.WriteString(fmt.Sprintf("Type %s\n", escapeVHSString(orig)))
				}
				continue
			}

			if _, ok := cmdSet[trimmed]; ok {
				result.WriteString(trimmed + "\n")
				continue
			}

			result.WriteString(fmt.Sprintf("Type %s\n", escapeVHSString(orig)))
		}
	}

	return result.String()
}
