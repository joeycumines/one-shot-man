// Package freezeterm invokes the external freeze CLI to render terminal
// captures as SVG or PNG artifacts.
//
// The package is deliberately independent from termmux: it accepts bytes or a
// file path, discovers and runs the freeze executable, and returns artifact
// paths or SVG text. Consumers compose it with anything that produces
// terminal text; nothing here imports termmux.
package freezeterm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Sentinel errors reported by this package. Use errors.Is to classify.
var (
	// ErrUnavailable reports that the freeze executable could not be found or
	// started.
	ErrUnavailable = errors.New("freezeterm: freeze executable is unavailable")

	// ErrExitFailure reports that freeze ran but exited with an error.
	ErrExitFailure = errors.New("freezeterm: freeze exited with an error")

	// ErrInvalidOptions reports options rejected before any process runs.
	ErrInvalidOptions = errors.New("freezeterm: invalid options")
)

// Format identifies the rendered artifact format.
type Format string

const (
	// FormatSVG writes the SVG document produced by freeze.
	FormatSVG Format = "svg"
	// FormatPNG converts the SVG document to PNG through freeze.
	FormatPNG Format = "png"
)

// Valid reports whether f is a supported format.
func (f Format) Valid() bool {
	return f == FormatSVG || f == FormatPNG
}

// VersionInfo describes the discovered freeze executable.
type VersionInfo struct {
	// Executable is the resolved path of the executable.
	Executable string
	// Version is the version token, e.g. "v0.2.2" or "unknown".
	Version string
	// Commit is the short commit SHA embedded in the binary, or empty when
	// freeze was built from source without version metadata.
	Commit string
	// Raw is the complete version output.
	Raw string
}

// Locate returns the path of the freeze executable on PATH.
func Locate() (string, error) {
	path, err := exec.LookPath("freeze")
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrUnavailable, err)
	}
	return path, nil
}

// Info runs the executable's version command and parses its output.
func Info(ctx context.Context, executable string) (VersionInfo, error) {
	if executable == "" {
		var err error
		executable, err = Locate()
		if err != nil {
			return VersionInfo{}, err
		}
	}
	cmd := exec.CommandContext(ctx, executable, "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return VersionInfo{}, ctx.Err()
		}
		if _, ok := errors.AsType[*exec.ExitError](err); ok {
			return VersionInfo{}, fmt.Errorf("%w: %s: %s", ErrExitFailure, executable, summarize(out.String()))
		}
		return VersionInfo{}, fmt.Errorf("%w: %s: %s", ErrUnavailable, executable, summarize(out.String()))
	}
	info := parseVersion(executable, out.String())
	return info, nil
}

// parseVersion extracts the version and commit tokens from freeze output of
// the form "freeze version v0.2.2 (80921ba)" or "freeze version unknown
// (built from source)".
func parseVersion(executable, raw string) VersionInfo {
	info := VersionInfo{Executable: executable, Raw: strings.TrimSpace(raw)}
	fields := strings.Fields(info.Raw)
	for i, field := range fields {
		if field != "version" || i+1 >= len(fields) {
			continue
		}
		info.Version = fields[i+1]
		rest := fields[i+2:]
		if len(rest) > 0 {
			commit := rest[len(rest)-1]
			if strings.HasPrefix(commit, "(") && strings.HasSuffix(commit, ")") {
				commit = strings.TrimSuffix(strings.TrimPrefix(commit, "("), ")")
				if commit != "built" {
					info.Commit = commit
				}
			}
		}
		break
	}
	if info.Version == "" {
		info.Version = info.Raw
	}
	return info
}

// Result describes a completed render.
type Result struct {
	// Path is the path of the rendered artifact. It is caller-owned when
	// Options.Output was supplied and a temporary file otherwise.
	Path string
	// Format is the rendered artifact format.
	Format Format
	// Temporary reports whether Path points at a package-created temporary
	// file that the caller owns.
	Temporary bool
	// Text holds the SVG document text when a temporary SVG artifact was
	// read back; it is empty for caller-owned outputs and PNG artifacts.
	Text string
}

// Render runs the external freeze CLI and returns the rendered artifact.
//
// Exactly one of Options.Input, Options.InputPath and Options.Execute must be
// set. When Options.Output is empty the package creates a temporary artifact
// (SVG unless Options.Format selects PNG) and returns its path; the caller
// owns that file. Temporary files are removed when rendering fails.
func Render(ctx context.Context, opts Options) (Result, error) {
	if err := opts.validate(); err != nil {
		return Result{}, err
	}

	executable := opts.Executable
	if executable == "" {
		var err error
		executable, err = Locate()
		if err != nil {
			return Result{}, err
		}
	}

	format, outputPath, temporary, err := prepareOutput(opts)
	if err != nil {
		return Result{}, err
	}

	args := buildArgs(opts, format, outputPath)
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = opts.Dir
	if opts.Input != nil {
		cmd.Stdin = bytes.NewReader(opts.Input)
	} else {
		devNull, openErr := os.Open(os.DevNull)
		if openErr != nil {
			return Result{}, fmt.Errorf("%w: %s", ErrUnavailable, openErr)
		}
		defer func() { _ = devNull.Close() }()
		cmd.Stdin = devNull
	}
	var diagnostics bytes.Buffer
	cmd.Stdout = &diagnostics
	cmd.Stderr = &diagnostics

	runErr := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		removeTemporary(temporary, outputPath)
		return Result{}, ctxErr
	}
	if runErr != nil {
		removeTemporary(temporary, outputPath)
		if exitErr, ok := errors.AsType[*exec.ExitError](runErr); ok {
			return Result{}, fmt.Errorf("%w: exit status %d: %s", ErrExitFailure, exitErr.ExitCode(), summarize(diagnostics.String()))
		}
		return Result{}, fmt.Errorf("%w: %s", ErrUnavailable, runErr)
	}

	result := Result{Path: outputPath, Format: format, Temporary: temporary}
	if temporary && format == FormatSVG {
		text, readErr := os.ReadFile(outputPath)
		if readErr != nil {
			removeTemporary(temporary, outputPath)
			return Result{}, fmt.Errorf("freezeterm: read temporary artifact: %w", readErr)
		}
		result.Text = string(text)
	}
	return result, nil
}

// RenderText renders a temporary SVG artifact and returns its document text,
// removing the temporary file before returning. Options.Output must be empty
// because no caller-owned file is produced.
func RenderText(ctx context.Context, opts Options) (string, error) {
	if opts.Output != "" {
		return "", fmt.Errorf("%w: renderText requires temporary output", ErrInvalidOptions)
	}
	opts.Format = FormatSVG
	result, err := Render(ctx, opts)
	if err != nil {
		return "", err
	}
	removeErr := os.Remove(result.Path)
	if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return "", fmt.Errorf("freezeterm: remove temporary artifact: %w", removeErr)
	}
	return result.Text, nil
}

// prepareOutput resolves the artifact format and path, creating a temporary
// file when no caller-owned output is supplied. Caller-owned parent
// directories are created, matching the old SaveRasterPNG contract.
func prepareOutput(opts Options) (Format, string, bool, error) {
	if opts.Output != "" {
		format, ok := formatForExtension(opts.Output)
		if !ok {
			return "", "", false, fmt.Errorf("%w: unsupported output extension %q (want .svg or .png)", ErrInvalidOptions, filepath.Ext(opts.Output))
		}
		if opts.Format != "" && opts.Format != format {
			return "", "", false, fmt.Errorf("%w: format %q conflicts with output %q", ErrInvalidOptions, opts.Format, opts.Output)
		}
		if dir := filepath.Dir(opts.Output); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", "", false, fmt.Errorf("freezeterm: create output directory: %w", err)
			}
		}
		return format, opts.Output, false, nil
	}

	format := opts.Format
	if format == "" {
		format = FormatSVG
	}
	f, err := os.CreateTemp("", "freezeterm-*."+string(format))
	if err != nil {
		return "", "", false, fmt.Errorf("freezeterm: create temporary artifact: %w", err)
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", "", false, fmt.Errorf("freezeterm: create temporary artifact: %w", err)
	}
	return format, path, true, nil
}

// removeTemporary removes a package-created artifact after a failed render.
func removeTemporary(temporary bool, path string) {
	if temporary {
		_ = os.Remove(path)
	}
}

// formatForExtension resolves a lowercase .svg or .png extension. freeze only
// handles those two suffixes; uppercase and other extensions silently write
// raw SVG, so they are rejected here.
func formatForExtension(path string) (Format, bool) {
	switch filepath.Ext(path) {
	case ".svg":
		return FormatSVG, true
	case ".png":
		return FormatPNG, true
	default:
		return "", false
	}
}

// summarize collapses whitespace and truncates process diagnostics for error
// messages.
func summarize(diagnostics string) string {
	text := strings.Join(strings.Fields(diagnostics), " ")
	const limit = 400
	if len(text) > limit {
		text = text[:limit] + "…"
	}
	return text
}
