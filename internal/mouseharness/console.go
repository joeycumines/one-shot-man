//go:build unix

package mouseharness

import (
	"fmt"
	"testing"

	"github.com/joeycumines/go-prompt/termtest"
)

// Console wraps a PTY console for mouse interaction testing.
// The termtest.Console is managed externally - this type only adds mouse utilities.
type Console struct {
	cp     *termtest.Console
	tb     testing.TB
	height int
	width  int
}

// consoleConfig holds the internal configuration during construction.
type consoleConfig struct {
	cp     *termtest.Console
	tb     testing.TB
	height int
	width  int
}

// defaultConfig returns a consoleConfig with default values.
func defaultConfig() *consoleConfig {
	return &consoleConfig{
		height: 24,
		width:  80,
	}
}

// New creates a Console with the given options.
func New(options ...Option) (*Console, error) {
	cfg := defaultConfig()

	for _, opt := range options {
		if err := opt.applyOption(cfg); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	// Validate required fields
	if cfg.cp == nil {
		return nil, fmt.Errorf("WithTermtestConsole is required")
	}
	if cfg.tb == nil {
		return nil, fmt.Errorf("WithTestingTB is required")
	}

	return &Console{
		cp:     cfg.cp,
		tb:     cfg.tb,
		height: cfg.height,
		width:  cfg.width,
	}, nil
}

// Height returns the configured terminal height.
func (c *Console) Height() int {
	return c.height
}

// Width returns the configured terminal width.
func (c *Console) Width() int {
	return c.width
}

// String returns the current terminal buffer content.
func (c *Console) String() string {
	return c.cp.String()
}

// Snapshot returns a snapshot for use with Expect.
func (c *Console) Snapshot() termtest.Snapshot {
	return c.cp.Snapshot()
}

// WriteString writes raw bytes to the console.
func (c *Console) WriteString(s string) (int, error) {
	return c.cp.WriteString(s)
}

// TermtestConsole returns the underlying termtest.Console.
func (c *Console) TermtestConsole() *termtest.Console {
	return c.cp
}
