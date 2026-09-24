package lipgloss

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"

	"charm.land/lipgloss/v2"
	"golang.org/x/term"
)

// Manager holds the lipgloss context for a specific engine instance.
// In v2, lipgloss no longer uses a Renderer - styles are standalone and
// auto-downsample based on terminal capabilities when using lipgloss.Println/Sprint.
type Manager struct {
	// detectedDarkBackground holds the cached result of HasDarkBackground.
	// nil means detection was unavailable and the dark fallback is used.
	detectedDarkBackground *bool
}

// ManagerOption configures a Manager.
type ManagerOption interface {
	applyManagerOption(cfg *managerConfig) error
}

type managerConfig struct {
	input  *os.File
	output *os.File
}

// FilesOption sets the terminal files used for adaptive color detection.
type FilesOption struct {
	input  *os.File
	output *os.File
}

// WithFiles configures the terminal files for adaptive color detection.
func WithFiles(input, output *os.File) *FilesOption {
	return &FilesOption{input: input, output: output}
}

func (o *FilesOption) applyManagerOption(cfg *managerConfig) error {
	cfg.input = o.input
	cfg.output = o.output
	return nil
}

var _ ManagerOption = (*FilesOption)(nil)

// NewManager creates a new lipgloss manager with the given options.
func NewManager(opts ...ManagerOption) (*Manager, error) {
	var cfg managerConfig
	for _, opt := range opts {
		if opt == nil {
			return nil, errors.New("lipgloss: nil ManagerOption")
		}
		if err := opt.applyManagerOption(&cfg); err != nil {
			return nil, fmt.Errorf("lipgloss: %w", err)
		}
	}
	m := &Manager{}
	if cfg.input != nil && cfg.output != nil && canQueryTerminalBackground(cfg.input, cfg.output) {
		m.detectedDarkBackground = new(bool)
		*m.detectedDarkBackground = lipgloss.HasDarkBackground(cfg.input, cfg.output)
	} else {
		m.detectedDarkBackground = new(bool)
		*m.detectedDarkBackground = true
	}
	return m, nil
}

// canQueryTerminalBackground returns true only if both in and out are real terminals,
// not running on Windows, and not in an automated test.
//
// On Windows, lipgloss.HasDarkBackground opens CONIN$ when redirected and uses a cancel
// reader that blocks indefinitely on ReadConsole in non-interactive/remote environments.
// During tests (testing.Testing()), querying the terminal is disabled because test runners
// do not respond to OSC 11 queries and synchronous timeouts degrade performance.
func canQueryTerminalBackground(in, out *os.File) bool {
	if in == nil || out == nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return false
	}
	if testing.Testing() {
		return false
	}
	return term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd()))
}
