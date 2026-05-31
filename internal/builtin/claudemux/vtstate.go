package claudemux

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// VTStateDetector composes a VTerm emulator with a TUIStateMachine to detect
// the current state of a TUI-based agent from raw PTY output. It feeds bytes
// through the VTerm to maintain a screen buffer, extracts plain-text screen
// content, and runs the state machine on each line to classify the agent state.
type VTStateDetector struct {
	vt  *vt.VTerm
	sm  *TUIStateMachine
	mu  sync.Mutex
	row int
	col int

	state         TUIState
	trailingReady []*regexp.Regexp
}

// NewVTStateDetector creates a VTStateDetector with the given state machine
// configuration. The VTerm is initialized with 24 rows and 80 columns.
// Returns an error if any regex pattern in the config fails to compile.
func NewVTStateDetector(config TUIStateMachineConfig) (*VTStateDetector, error) {
	sm, err := NewTUIStateMachine(config)
	if err != nil {
		return nil, err
	}
	const (
		defaultRows = 24
		defaultCols = 80
	)

	// Pre-compile trailing-match variants of ready patterns (strip ^ anchor).
	// These are used when checking the last non-empty line for a trailing
	// ready indicator that shares a line with other content (e.g. "Running…❯ ").
	var trailingReady []*regexp.Regexp
	for _, pat := range sm.readyPatterns {
		trailingPat := strings.TrimPrefix(pat.re.String(), "^")
		re, err := regexp.Compile(trailingPat)
		if err != nil {
			return nil, fmt.Errorf("trailing ready pattern: %w", err)
		}
		trailingReady = append(trailingReady, re)
	}

	return &VTStateDetector{
		vt:            vt.NewVTerm(defaultRows, defaultCols),
		sm:            sm,
		row:           defaultRows,
		col:           defaultCols,
		state:         StateInitializing,
		trailingReady: trailingReady,
	}, nil
}

// ProcessRaw feeds raw PTY data through the VTerm emulator, extracts the
// plain-text screen content, and runs each line through the TUIStateMachine.
// Returns all TUIStateUpdates that resulted in a state change, in order.
// If no state change occurred, returns a single-element slice with Changed=false.
func (d *VTStateDetector) ProcessRaw(data []byte, now time.Time) []TUIStateUpdate {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.vt.Write(data)

	screen := d.vt.String()
	lines := strings.Split(screen, "\n")

	var transitions []TUIStateUpdate
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		update := d.sm.ProcessOutput(line, now)
		if update.Changed {
			d.state = update.State
			transitions = append(transitions, update)
		}
	}

	// If the state machine didn't detect a ready indicator because it
	// shares a line with other content (e.g., "Running…❯ "), check the
	// last non-empty line for a trailing ready indicator. VTerm appends
	// content to the current line when no CR/LF separates chunks.
	if d.state == StateProcessing {
		for i := len(lines) - 1; i >= 0; i-- {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "" {
				continue
			}
			for _, trailingRe := range d.trailingReady {
				loc := trailingRe.FindStringIndex(trimmed)
				if loc != nil {
					matched := trimmed[loc[0]:loc[1]]
					update := d.sm.ProcessOutput(matched, now)
					if update.Changed {
						d.state = update.State
						transitions = append(transitions, update)
					}
					break
				}
			}
			break
		}
	}

	if len(transitions) > 0 {
		return transitions
	}

	return []TUIStateUpdate{{
		From:      d.state,
		To:        d.state,
		State:     d.state,
		StateName: tuiStateName(d.state),
		Changed:   false,
		Timestamp: now,
	}}
}

// ScreenText returns the VTerm's visible screen as plain text with all ANSI
// escape sequences stripped. Thread-safe.
func (d *VTStateDetector) ScreenText() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.vt.String()
}

// LastNLines returns the last n non-empty lines from the VTerm screen buffer.
// If fewer than n non-empty lines exist, all non-empty lines are returned.
// Thread-safe.
func (d *VTStateDetector) LastNLines(n int) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	screen := d.vt.String()
	lines := strings.Split(screen, "\n")

	var result []string
	for i := len(lines) - 1; i >= 0 && len(result) < n; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// CursorPosition returns the VTerm's cursor row and column. Thread-safe.
func (d *VTStateDetector) CursorPosition() (row, col int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.vt.CursorPosition()
}

// State returns the current detected TUI state. Thread-safe.
func (d *VTStateDetector) State() TUIState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

func (d *VTStateDetector) VTerm() *vt.VTerm {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.vt
}

// Reset clears the VTerm screen buffer, resets the state machine, and returns
// the detector to StateInitializing. Thread-safe.
func (d *VTStateDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// VTerm has no public Reset; create a fresh instance.
	d.vt = vt.NewVTerm(d.row, d.col)
	d.sm.Reset()
	d.state = StateInitializing
}
