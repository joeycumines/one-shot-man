//go:build unix

package mouseharness

import (
	"fmt"
	"testing"

	"github.com/joeycumines/go-prompt/termtest"
)

// Option configures Console creation.
type Option interface {
	applyOption(*consoleConfig) error
}

// optionFunc is the concrete implementation of Option.
type optionFunc func(*consoleConfig) error

func (f optionFunc) applyOption(c *consoleConfig) error { return f(c) }

// WithTermtestConsole sets the externally-managed termtest console.
// This is required - the Console wraps an existing *termtest.Console.
func WithTermtestConsole(cp *termtest.Console) Option {
	return optionFunc(func(c *consoleConfig) error {
		if cp == nil {
			return fmt.Errorf("termtest console cannot be nil")
		}
		c.cp = cp
		return nil
	})
}

// WithTestingTB sets the testing.TB for logging.
// This is required for methods that need test context.
func WithTestingTB(tb testing.TB) Option {
	return optionFunc(func(c *consoleConfig) error {
		if tb == nil {
			return fmt.Errorf("testing.TB cannot be nil")
		}
		c.tb = tb
		return nil
	})
}

// WithHeight sets the terminal height for viewport calculations.
// Default is 24 rows.
func WithHeight(h int) Option {
	return optionFunc(func(c *consoleConfig) error {
		if h <= 0 {
			return fmt.Errorf("height must be positive, got %d", h)
		}
		c.height = h
		return nil
	})
}

// WithWidth sets the terminal width.
// Default is 80 columns.
// Note: Width may have PTY API quirks in some scenarios.
func WithWidth(w int) Option {
	return optionFunc(func(c *consoleConfig) error {
		if w <= 0 {
			return fmt.Errorf("width must be positive, got %d", w)
		}
		c.width = w
		return nil
	})
}
