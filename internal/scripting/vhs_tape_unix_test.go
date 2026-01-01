//go:build unix

package scripting

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// VHSTheme defines terminal color themes for VHS recordings.
type VHSTheme struct {
	Name       string
	Background string
	Foreground string
	Black      string
	Red        string
	Green      string
	Yellow     string
	Blue       string
	Magenta    string
	Cyan       string
	White      string
}

// VHSDarkTheme is a pleasant dark theme for recordings.
var VHSDarkTheme = VHSTheme{
	Name:       "OSM Dark",
	Background: "#1a1b26",
	Foreground: "#c0caf5",
	Black:      "#15161e",
	Red:        "#f7768e",
	Green:      "#9ece6a",
	Yellow:     "#e0af68",
	Blue:       "#7aa2f7",
	Magenta:    "#bb9af7",
	Cyan:       "#7dcfff",
	White:      "#a9b1d6",
}

// VHSLightTheme is a pleasant light theme for recordings.
var VHSLightTheme = VHSTheme{
	Name:       "OSM Light",
	Background: "#f8f8f2",
	Foreground: "#2e3440",
	Black:      "#2e3440",
	Red:        "#bf616a",
	Green:      "#a3be8c",
	Yellow:     "#ebcb8b",
	Blue:       "#5e81ac",
	Magenta:    "#b48ead",
	Cyan:       "#88c0d0",
	White:      "#eceff4",
}

// VHSConfig holds VHS recording configuration.
type VHSConfig struct {
	// Output is the output directory.
	Output string

	// Width is the terminal width in characters.
	Width int

	// Height is the terminal height in characters.
	Height int

	// FontSize is the font size in pixels.
	FontSize int

	// FontFamily is the font family to use.
	FontFamily string

	// Theme is the color theme to use.
	Theme VHSTheme

	// TypingSpeed is the delay between keystrokes (e.g., "30ms").
	TypingSpeed string

	// PlaybackSpeed controls the final GIF playback speed (1.0 = normal).
	PlaybackSpeed float64

	// Shell is the shell to use for the recording.
	Shell string

	// WindowBar controls the window decoration style ("Colorful", "Rings", etc).
	WindowBar string

	// Padding is the padding around the terminal in pixels.
	Padding int

	// Margin is the margin around the video in pixels.
	Margin int

	// MarginFill is the color for the margin.
	MarginFill string

	// BorderRadius is the border radius for the terminal window.
	BorderRadius int

	// CursorBlink controls whether the cursor blinks.
	CursorBlink bool
}

// DefaultVHSConfig returns a default VHS recording configuration.
func DefaultVHSConfig() VHSConfig {
	return VHSConfig{
		Width:         100,
		Height:        30,
		FontSize:      10,
		Theme:         VHSDarkTheme,
		TypingSpeed:   "30ms",
		PlaybackSpeed: 1.0,
		Shell:         "bash",
		WindowBar:     "Colorful",
		Padding:       20,
		Margin:        10,
		MarginFill:    "#1a1b26",
		BorderRadius:  8,
		CursorBlink:   true,
	}
}

// DefaultVHSLightConfig returns a default light-theme VHS config.
func DefaultVHSLightConfig() VHSConfig {
	cfg := DefaultVHSConfig()
	cfg.Theme = VHSLightTheme
	cfg.MarginFill = "#f8f8f2"
	return cfg
}

// VHSTapeBuilder builds a VHS tape file from recorded actions.
// It is used by integration tests to generate VHS recordings as a side effect.
type VHSTapeBuilder struct {
	config  VHSConfig
	actions []string
	env     map[string]string
	output  []string
	require []string
}

// NewVHSTapeBuilder creates a new VHS tape builder with the given configuration.
func NewVHSTapeBuilder(config VHSConfig) *VHSTapeBuilder {
	return &VHSTapeBuilder{
		config: config,
		env:    make(map[string]string),
	}
}

// SetEnv sets an environment variable for the VHS recording.
func (b *VHSTapeBuilder) SetEnv(key, value string) *VHSTapeBuilder {
	b.env[key] = value
	return b
}

// Output adds an output file for the VHS recording.
func (b *VHSTapeBuilder) Output(path string) *VHSTapeBuilder {
	b.output = append(b.output, path)
	return b
}

// Require adds a required program check to the tape.
func (b *VHSTapeBuilder) Require(program string) *VHSTapeBuilder {
	b.require = append(b.require, program)
	return b
}

// Type adds a Type action to the tape.
func (b *VHSTapeBuilder) Type(text string) *VHSTapeBuilder {
	b.actions = append(b.actions, fmt.Sprintf("Type %s", escapeVHSString(text)))
	return b
}

// TypeWithSpeed adds a Type action with custom speed.
func (b *VHSTapeBuilder) TypeWithSpeed(text, speed string) *VHSTapeBuilder {
	b.actions = append(b.actions, fmt.Sprintf("Type@%s %s", speed, escapeVHSString(text)))
	return b
}

// Enter adds an Enter key press to the tape.
func (b *VHSTapeBuilder) Enter() *VHSTapeBuilder {
	b.actions = append(b.actions, "Enter")
	return b
}

// Key adds a special key press to the tape (Tab, Escape, Up, Down, etc.).
func (b *VHSTapeBuilder) Key(key string) *VHSTapeBuilder {
	b.actions = append(b.actions, key)
	return b
}

// Ctrl adds a Ctrl+key combination to the tape.
func (b *VHSTapeBuilder) Ctrl(key string) *VHSTapeBuilder {
	b.actions = append(b.actions, fmt.Sprintf("Ctrl+%s", key))
	return b
}

// Sleep adds a pause to the tape.
// Note: Use sparingly! Prefer WaitForText for determinism.
func (b *VHSTapeBuilder) Sleep(duration string) *VHSTapeBuilder {
	b.actions = append(b.actions, fmt.Sprintf("Sleep %s", duration))
	return b
}

// Wait adds a wait-for-pattern action to the tape (VHS native).
func (b *VHSTapeBuilder) Wait(pattern string, timeout string) *VHSTapeBuilder {
	cmd := fmt.Sprintf("Wait@%s /%s/", timeout, pattern)
	b.actions = append(b.actions, cmd)
	return b
}

// Click adds a mouse click at specific coordinates.
// Note: Coordinates should be calculated dynamically from buffer state!
func (b *VHSTapeBuilder) Click(x, y int) *VHSTapeBuilder {
	// VHS doesn't have native mouse support, but we can simulate via escape sequences
	// For now, we'll add a comment noting the intended click location
	b.actions = append(b.actions, fmt.Sprintf("# Click at (%d, %d) - simulated via key equivalent", x, y))
	return b
}

// Comment adds a comment to the tape for documentation.
func (b *VHSTapeBuilder) Comment(text string) *VHSTapeBuilder {
	b.actions = append(b.actions, fmt.Sprintf("# %s", text))
	return b
}

// Hide hides subsequent commands from the recording.
func (b *VHSTapeBuilder) Hide() *VHSTapeBuilder {
	b.actions = append(b.actions, "Hide")
	return b
}

// Show resumes showing commands in the recording.
func (b *VHSTapeBuilder) Show() *VHSTapeBuilder {
	b.actions = append(b.actions, "Show")
	return b
}

// Screenshot captures a screenshot at the current point.
func (b *VHSTapeBuilder) Screenshot(path string) *VHSTapeBuilder {
	b.actions = append(b.actions, fmt.Sprintf("Screenshot %s", path))
	return b
}

// GenerateTape generates the VHS tape file content.
func (b *VHSTapeBuilder) GenerateTape() string {
	var buf bytes.Buffer

	cfg := b.config

	// Output file names
	for _, v := range b.output {
		buf.WriteString("Output " + v + "\n")
	}
	if len(b.output) > 0 {
		buf.WriteString("\n")
	}

	// Required programs
	for _, req := range b.require {
		buf.WriteString("Require " + req + "\n")
	}
	if len(b.require) > 0 {
		buf.WriteString("\n")
	}

	// Settings
	buf.WriteString("# Terminal Settings\n")
	if cfg.Width > 0 {
		buf.WriteString(fmt.Sprintf("Set Width %d\n", cfg.Width))
	}
	if cfg.Height > 0 {
		buf.WriteString(fmt.Sprintf("Set Height %d\n", cfg.Height))
	}
	if cfg.FontSize > 0 {
		buf.WriteString(fmt.Sprintf("Set FontSize %d\n", cfg.FontSize))
	}
	if cfg.FontFamily != "" {
		buf.WriteString(fmt.Sprintf("Set FontFamily %q\n", cfg.FontFamily))
	}
	if cfg.Shell != "" {
		buf.WriteString(fmt.Sprintf("Set Shell %q\n", cfg.Shell))
	}
	buf.WriteString(fmt.Sprintf("Set TypingSpeed %s\n", cfg.TypingSpeed))
	if cfg.PlaybackSpeed != 1.0 {
		buf.WriteString(fmt.Sprintf("Set PlaybackSpeed %.1f\n", cfg.PlaybackSpeed))
	}
	if cfg.WindowBar != "" {
		buf.WriteString(fmt.Sprintf("Set WindowBar %s\n", cfg.WindowBar))
	}
	buf.WriteString(fmt.Sprintf("Set Padding %d\n", cfg.Padding))
	buf.WriteString(fmt.Sprintf("Set Margin %d\n", cfg.Margin))
	if cfg.MarginFill != "" {
		buf.WriteString(fmt.Sprintf("Set MarginFill %q\n", cfg.MarginFill))
	}
	if cfg.BorderRadius > 0 {
		buf.WriteString(fmt.Sprintf("Set BorderRadius %d\n", cfg.BorderRadius))
	}
	buf.WriteString(fmt.Sprintf("Set CursorBlink %t\n", cfg.CursorBlink))

	// Theme
	buf.WriteString("\n# Theme\n")
	buf.WriteString(fmt.Sprintf(`Set Theme { "name": %q, "background": %q, "foreground": %q, "black": %q, "red": %q, "green": %q, "yellow": %q, "blue": %q, "magenta": %q, "cyan": %q, "white": %q, "brightBlack": %q, "brightRed": %q, "brightGreen": %q, "brightYellow": %q, "brightBlue": %q, "brightMagenta": %q, "brightCyan": %q, "brightWhite": %q, "cursor": %q, "selection": "#44475a" }`,
		cfg.Theme.Name,
		cfg.Theme.Background,
		cfg.Theme.Foreground,
		cfg.Theme.Black,
		cfg.Theme.Red,
		cfg.Theme.Green,
		cfg.Theme.Yellow,
		cfg.Theme.Blue,
		cfg.Theme.Magenta,
		cfg.Theme.Cyan,
		cfg.Theme.White,
		cfg.Theme.Black,   // brightBlack
		cfg.Theme.Red,     // brightRed
		cfg.Theme.Green,   // brightGreen
		cfg.Theme.Yellow,  // brightYellow
		cfg.Theme.Blue,    // brightBlue
		cfg.Theme.Magenta, // brightMagenta
		cfg.Theme.Cyan,    // brightCyan
		cfg.Theme.White,   // brightWhite
		cfg.Theme.Foreground,
	))
	buf.WriteString("\n")

	// Environment variables
	if len(b.env) > 0 {
		buf.WriteString("\n# Environment\n")
		for k, v := range b.env {
			buf.WriteString(fmt.Sprintf("Env %s %q\n", k, v))
		}
	}

	// Actions
	buf.WriteString("\n# Recording Script\n")
	for _, action := range b.actions {
		buf.WriteString(action + "\n")
	}

	return buf.String()
}

// SaveTape writes the tape file to disk.
func (b *VHSTapeBuilder) SaveTape(path string) error {
	content := b.GenerateTape()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// ExecuteVHS runs VHS on the tape file to generate the GIF.
// Returns an error if VHS is not available or execution fails.
func (b *VHSTapeBuilder) ExecuteVHS(ctx context.Context, tapePath string) error {
	if !VHSAvailable() {
		return fmt.Errorf("vhs not found in PATH")
	}

	vhsPath, err := exec.LookPath("vhs")
	if err != nil {
		return fmt.Errorf("vhs not found: %w", err)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(b.config.Output, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	cmd := exec.CommandContext(ctx, vhsPath, tapePath)
	cmd.Dir = filepath.Dir(tapePath)

	// Set up environment
	cmd.Env = os.Environ()
	for k, v := range b.env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("vhs execution failed: %w\nOutput: %s", err, output)
	}

	return nil
}

// GenerateAndExecute generates the tape, saves it, and executes VHS.
func (b *VHSTapeBuilder) GenerateAndExecute(ctx context.Context, tapePath string) error {
	if err := b.SaveTape(tapePath); err != nil {
		return fmt.Errorf("failed to save tape: %w", err)
	}
	return b.ExecuteVHS(ctx, tapePath)
}

// VHSAvailable checks if VHS is available in PATH.
func VHSAvailable() bool {
	_, err := exec.LookPath("vhs")
	return err == nil
}

// escapeVHSString determines a safe quoting strategy for a string literal
// compatible with the VHS lexer constraints.
//
// The VHS lexer implementation of `readString` has specific limitations:
// 1. It does not support escape sequences (e.g., `\"` is read as a literal backslash followed by a quote terminator).
// 2. It terminates reading immediately upon encountering a newline character.
//
// Permalink: https://github.com/charmbracelet/vhs/blob/cdd370832a90bea0ccedc9cffc941c7b06452df8/lexer/lexer.go#L137-L147
func escapeVHSString(s string) string {
	// Constraint 1: The lexer breaks on newlines (`\n` or `\r`).
	// Since there is no escape mechanism (e.g. `\n`), we cannot represent multi-line strings.
	if strings.ContainsAny(s, "\n\r") {
		panic(fmt.Sprintf("cannot quote string: input contains newlines which are not supported by the VHS lexer: %q", s))
	}

	// Constraint 2: The lexer does not support escaping the delimiter.
	// We must iterate through available delimiters to find one that is NOT present in the string.

	// Strategy A: Double Quotes
	if !strings.ContainsRune(s, '"') {
		return `"` + s + `"`
	}

	// Strategy B: Single Quotes
	if !strings.ContainsRune(s, '\'') {
		return "'" + s + "'"
	}

	// Strategy C: Backticks
	if !strings.ContainsRune(s, '`') {
		return "`" + s + "`"
	}

	// Failure: The string contains ", ', AND `.
	// Since the lexer interprets content literally and supports no escapes,
	// there is no way to represent this string.
	panic(fmt.Sprintf("cannot quote string: input contains all possible delimiters (\", ', `): %q", s))
}

// RecordingContext wraps a VHS tape builder with convenience methods
// for recording actions during integration test execution.
type RecordingContext struct {
	tape       *VHSTapeBuilder
	enabled    bool
	tapePath   string
	lastAction time.Time
}

// NewRecordingContext creates a new recording context.
// If enabled is false, all recording methods are no-ops.
func NewRecordingContext(config VHSConfig, tapePath string, enabled bool) *RecordingContext {
	return &RecordingContext{
		tape:       NewVHSTapeBuilder(config),
		enabled:    enabled,
		tapePath:   tapePath,
		lastAction: time.Now(),
	}
}

// IsEnabled returns whether recording is enabled.
func (r *RecordingContext) IsEnabled() bool {
	return r.enabled
}

// RecordType records a Type action if enabled.
// If the text contains newlines, it splits into multiple Type commands
// with Enter commands between them to work around VHS lexer limitations.
func (r *RecordingContext) RecordType(text string) {
	if !r.enabled {
		return
	}
	// VHS lexer doesn't support newlines in strings, so split and insert Enter
	if strings.ContainsAny(text, "\n\r") {
		lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
		for i, line := range lines {
			if line != "" {
				r.tape.Type(line)
			}
			if i < len(lines)-1 {
				r.tape.Enter()
			}
		}
	} else {
		r.tape.Type(text)
	}
	r.lastAction = time.Now()
}

// RecordEnter records an Enter key press if enabled.
func (r *RecordingContext) RecordEnter() {
	if !r.enabled {
		return
	}
	r.tape.Enter()
	r.lastAction = time.Now()
}

// RecordKey records a special key press if enabled.
func (r *RecordingContext) RecordKey(key string) {
	if !r.enabled {
		return
	}
	if strings.EqualFold(key, "Enter") ||
		strings.EqualFold(key, "Tab") ||
		strings.EqualFold(key, "Space") ||
		strings.EqualFold(key, "PageUp") ||
		strings.EqualFold(key, "PageDown") ||
		strings.EqualFold(key, "Up") ||
		strings.EqualFold(key, "Down") ||
		strings.EqualFold(key, "Left") ||
		strings.EqualFold(key, "Right") ||
		strings.EqualFold(key, "Backspace") ||
		(len(key) > 5 && strings.EqualFold(key[:5], "Ctrl+")) {
		r.tape.Key(key)
	} else {
		r.tape.Type(key)
	}
	r.lastAction = time.Now()
}

// RecordCtrl records a Ctrl+key combination if enabled.
func (r *RecordingContext) RecordCtrl(key string) {
	if !r.enabled {
		return
	}
	r.tape.Ctrl(key)
	r.lastAction = time.Now()
}

// RecordWait records a wait-for-pattern action if enabled.
func (r *RecordingContext) RecordWait(pattern, timeout string) {
	if !r.enabled {
		return
	}
	r.tape.Wait(pattern, timeout)
	r.lastAction = time.Now()
}

// RecordComment records a comment if enabled.
func (r *RecordingContext) RecordComment(text string) {
	if !r.enabled {
		return
	}
	r.tape.Comment(text)
}

// RecordSleep records a sleep action if enabled.
// Use sparingly - prefer RecordWait for determinism.
func (r *RecordingContext) RecordSleep(duration string) {
	if !r.enabled {
		return
	}
	r.tape.Sleep(duration)
	r.lastAction = time.Now()
}

// SetEnv sets an environment variable for the recording.
func (r *RecordingContext) SetEnv(key, value string) {
	if !r.enabled {
		return
	}
	r.tape.SetEnv(key, value)
}

func (r *RecordingContext) Output(path string) {
	if !r.enabled {
		return
	}
	r.tape.Output(path)
}

// Require adds a required program to the recording.
func (r *RecordingContext) Require(program string) {
	if !r.enabled {
		return
	}
	r.tape.Require(program)
}

// Finalize saves the tape and optionally executes VHS.
// If executeVHS is true and VHS is available, generates the GIF.
func (r *RecordingContext) Finalize(ctx context.Context, executeVHS bool) error {
	if !r.enabled {
		return nil
	}

	if err := r.tape.SaveTape(r.tapePath); err != nil {
		return fmt.Errorf("failed to save tape: %w", err)
	}

	if executeVHS {
		if !VHSAvailable() {
			// Skip VHS execution if not available
			return nil
		}
		return r.tape.ExecuteVHS(ctx, r.tapePath)
	}

	return nil
}

// GetTapeContent returns the generated tape content.
func (r *RecordingContext) GetTapeContent() string {
	if !r.enabled {
		return ""
	}
	return r.tape.GenerateTape()
}
