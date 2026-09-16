package freezeterm

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Options configures one freeze invocation. Exactly one source field must be
// set; the remaining fields map to freeze's documented flags and are omitted
// from argv when left at their zero values, so freeze's own defaults apply.
type Options struct {
	// Executable overrides the freeze executable path; empty discovers
	// "freeze" on PATH.
	Executable string

	// Input is terminal text fed to freeze on stdin. Input, InputPath and
	// Execute are mutually exclusive.
	Input []byte
	// InputPath is a file rendered by freeze. Mutually exclusive with Input
	// and Execute.
	InputPath string
	// Execute is a command whose output freeze captures in a PTY. Mutually
	// exclusive with Input and InputPath.
	Execute string
	// Dir is the working directory for the freeze process, including an
	// Execute command; empty inherits the caller's directory.
	Dir string

	// Output is a caller-owned artifact path ending in .svg or .png. When
	// empty, Render creates a temporary artifact and returns its path.
	Output string
	// Format requests a format for temporary output; it must not conflict
	// with a caller-owned Output extension. Empty defaults to SVG.
	Format Format

	// Language sets freeze's --language; it defaults to "ansi" for Input
	// bytes because terminal captures are ANSI text.
	Language string

	// Theme, Background and Window map to freeze's --theme, --background and
	// --window flags (--window only when true).
	Theme      string
	Background string
	Window     bool

	// Width and Height map to --width/--height when positive.
	Width  float64
	Height float64

	// Margin and Padding map to --margin/--padding when non-empty.
	Margin  []float64
	Padding []float64

	// Wrap maps to --wrap when positive.
	Wrap int
	// LineHeight maps to --line-height when positive.
	LineHeight float64
	// Lines maps to --lines as a 1-indexed inclusive range; at most two
	// values are accepted.
	Lines []int
	// ShowLineNumbers maps to --show-line-numbers when true.
	ShowLineNumbers bool

	// Font, Border and Shadow map to freeze's decoration flags.
	Font   FontOptions
	Border BorderOptions
	Shadow ShadowOptions

	// Args are extra freeze flags appended after the modeled options. Flags
	// that select a source, output, config or interactive mode are rejected.
	Args []string
}

// FontOptions maps to freeze's --font.* flags.
type FontOptions struct {
	// Family maps to --font.family when non-empty.
	Family string
	// File maps to --font.file when non-empty.
	File string
	// Size maps to --font.size when positive.
	Size float64
	// Ligatures maps to --font.ligatures when non-nil, allowing callers to
	// disable freeze's default.
	Ligatures *bool
}

// BorderOptions maps to freeze's --border.* flags.
type BorderOptions struct {
	// Radius maps to --border.radius when positive.
	Radius float64
	// Width maps to --border.width when positive.
	Width float64
	// Color maps to --border.color when non-empty.
	Color string
}

// ShadowOptions maps to freeze's --shadow.* flags.
type ShadowOptions struct {
	// Blur maps to --shadow.blur when non-zero.
	Blur float64
	// X maps to --shadow.x when non-zero.
	X float64
	// Y maps to --shadow.y when non-zero.
	Y float64
}

// reservedArgs are flags that would override the typed source, output or
// process-mode contract and therefore may not be smuggled through Args.
var reservedArgs = map[string]bool{
	"-o":                true,
	"--output":          true,
	"-x":                true,
	"--execute":         true,
	"--input":           true,
	"-c":                true,
	"--config":          true,
	"-i":                true,
	"--interactive":     true,
	"-v":                true,
	"--version":         true,
	"-h":                true,
	"--help":            true,
	"--execute.timeout": true,
}

// validate rejects option combinations before any process is started.
func (o Options) validate() error {
	sources := 0
	if o.Input != nil {
		sources++
	}
	if o.InputPath != "" {
		sources++
	}
	if o.Execute != "" {
		sources++
	}
	if sources != 1 {
		return fmt.Errorf("%w: exactly one of Input, InputPath or Execute is required", ErrInvalidOptions)
	}
	if o.Format != "" && !o.Format.Valid() {
		return fmt.Errorf("%w: unknown format %q", ErrInvalidOptions, o.Format)
	}
	if len(o.Lines) > 2 {
		return fmt.Errorf("%w: at most two line range values are supported", ErrInvalidOptions)
	}
	if o.Wrap < 0 {
		return fmt.Errorf("%w: wrap must not be negative", ErrInvalidOptions)
	}
	for _, value := range []float64{o.Width, o.Height, o.LineHeight, o.Font.Size, o.Border.Radius, o.Border.Width} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("%w: dimensions must be finite", ErrInvalidOptions)
		}
	}
	if o.Output != "" {
		format, ok := formatForExtension(o.Output)
		if !ok {
			return fmt.Errorf("%w: unsupported output extension %q (want .svg or .png)", ErrInvalidOptions, o.Output)
		}
		if o.Format != "" && o.Format != format {
			return fmt.Errorf("%w: format %q conflicts with output %q", ErrInvalidOptions, o.Format, o.Output)
		}
	}
	return validateExtraArgs(o.Args)
}

// validateExtraArgs rejects empty arguments, bare positionals (which freeze
// would treat as its input file) and reserved flags.
func validateExtraArgs(args []string) error {
	for _, arg := range args {
		if arg == "" {
			return fmt.Errorf("%w: extra args must not be empty", ErrInvalidOptions)
		}
		if !strings.HasPrefix(arg, "-") {
			return fmt.Errorf("%w: extra arg %q would be read as freeze's input file", ErrInvalidOptions, arg)
		}
		name, _, _ := strings.Cut(arg, "=")
		if reservedArgs[name] {
			return fmt.Errorf("%w: extra arg %q is reserved", ErrInvalidOptions, name)
		}
	}
	return nil
}

// buildArgs assembles the freeze argv without shell interpolation. The output
// path is always explicit so the artifact is never parsed from stdout.
func buildArgs(opts Options, format Format, outputPath string) []string {
	args := make([]string, 0, 32)

	switch {
	case opts.Execute != "":
		args = append(args, "-x", opts.Execute)
	case opts.InputPath != "":
		args = append(args, opts.InputPath)
	default:
		// Input bytes are read from stdin; the language default keeps plain
		// captured text from failing freeze's lexer detection.
		language := opts.Language
		if language == "" {
			language = "ansi"
		}
		args = append(args, "-l", language)
	}
	if opts.Language != "" && opts.InputPath != "" {
		args = append(args, "-l", opts.Language)
	}
	if opts.Theme != "" {
		args = append(args, "-t", opts.Theme)
	}
	if opts.Background != "" {
		args = append(args, "-b", opts.Background)
	}
	if opts.Window {
		args = append(args, "--window")
	}
	if opts.Width > 0 {
		args = append(args, "-W", formatFloat(opts.Width))
	}
	if opts.Height > 0 {
		args = append(args, "-H", formatFloat(opts.Height))
	}
	if len(opts.Margin) > 0 {
		args = append(args, "--margin", formatFloats(opts.Margin))
	}
	if len(opts.Padding) > 0 {
		args = append(args, "--padding", formatFloats(opts.Padding))
	}
	if opts.Wrap > 0 {
		args = append(args, "-w", strconv.Itoa(opts.Wrap))
	}
	if opts.LineHeight > 0 {
		args = append(args, "--line-height", formatFloat(opts.LineHeight))
	}
	if len(opts.Lines) > 0 {
		values := make([]string, len(opts.Lines))
		for i, line := range opts.Lines {
			values[i] = strconv.Itoa(line)
		}
		args = append(args, "--lines="+strings.Join(values, ","))
	}
	if opts.ShowLineNumbers {
		args = append(args, "--show-line-numbers")
	}
	if opts.Font.Family != "" {
		args = append(args, "--font.family", opts.Font.Family)
	}
	if opts.Font.File != "" {
		args = append(args, "--font.file", opts.Font.File)
	}
	if opts.Font.Size > 0 {
		args = append(args, "--font.size", formatFloat(opts.Font.Size))
	}
	if opts.Font.Ligatures != nil {
		args = append(args, "--font.ligatures="+strconv.FormatBool(*opts.Font.Ligatures))
	}
	if opts.Border.Radius > 0 {
		args = append(args, "--border.radius", formatFloat(opts.Border.Radius))
	}
	if opts.Border.Width > 0 {
		args = append(args, "--border.width", formatFloat(opts.Border.Width))
	}
	if opts.Border.Color != "" {
		args = append(args, "--border.color", opts.Border.Color)
	}
	if opts.Shadow.Blur != 0 {
		args = append(args, "--shadow.blur", formatFloat(opts.Shadow.Blur))
	}
	if opts.Shadow.X != 0 {
		args = append(args, "--shadow.x", formatFloat(opts.Shadow.X))
	}
	if opts.Shadow.Y != 0 {
		args = append(args, "--shadow.y", formatFloat(opts.Shadow.Y))
	}

	args = append(args, "-o", outputPath)
	args = append(args, opts.Args...)
	return args
}

// formatFloat renders a float without a trailing ".0" so argv stays stable.
func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// formatFloats renders a comma-separated float list for freeze's slice flags.
func formatFloats(values []float64) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = formatFloat(value)
	}
	return strings.Join(parts, ",")
}
