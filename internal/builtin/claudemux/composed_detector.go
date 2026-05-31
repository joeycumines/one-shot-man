package claudemux

import (
	"strings"
	"sync"
	"time"
)

// DetectMode specifies whether the ComposedDetector operates in protocol mode
// (NDJSON event classification) or TUI mode (raw PTY bytes).
type DetectMode int

const (
	// DetectModeProtocol classifies NDJSON events and maps them to TUI states.
	DetectModeProtocol DetectMode = iota
	// DetectModeTUI processes raw PTY bytes through VTerm + state machine.
	DetectModeTUI
)

// DetectModeName returns the human-readable name for a DetectMode value.
func DetectModeName(m DetectMode) string {
	switch m {
	case DetectModeProtocol:
		return "Protocol"
	case DetectModeTUI:
		return "TUI"
	default:
		return "Unknown"
	}
}

// ComposedDetector integrates Parser event classification with TUIStateMachine
// state tracking. It provides a unified detection pipeline that works for both
// PTY mode (raw bytes) and protocol mode (classified events).
//
// In protocol mode, ProcessEvent maps classified OutputEvents to TUI state
// transitions using the same logic as protocolStateTracker, but delegates
// state tracking to the TUIStateMachine.
//
// In TUI mode, ProcessRaw delegates to VTStateDetector for state detection
// and also runs the Parser on extracted screen lines for supplementary
// event classification.
//
// ComposedDetector is safe for concurrent use from multiple goroutines.
type ComposedDetector struct {
	parser     *Parser
	sm         *TUIStateMachine
	mode       DetectMode
	vtDet      *VTStateDetector // non-nil in TUI mode
	mu         sync.Mutex
	state      TUIState
	lastEvents []OutputEvent
}

// NewComposedDetector creates a ComposedDetector with the given mode and
// state machine configuration. In TUI mode, a VTStateDetector is also
// created using the same configuration. Returns an error if any regex
// pattern fails to compile.
func NewComposedDetector(mode DetectMode, config TUIStateMachineConfig) (*ComposedDetector, error) {
	sm, err := NewTUIStateMachine(config)
	if err != nil {
		return nil, err
	}

	d := &ComposedDetector{
		parser:     NewParser(),
		sm:         sm,
		mode:       mode,
		state:      StateInitializing,
		lastEvents: make([]OutputEvent, 0, 16),
	}

	if mode == DetectModeTUI {
		vtDet, err := NewVTStateDetector(config)
		if err != nil {
			return nil, err
		}
		d.vtDet = vtDet
	}

	return d, nil
}

// eventToTargetState maps an OutputEvent type to a target TUIState.
// Returns the target state and true if the event maps to a state transition,
// or a zero TUIState and false if the event does not affect state.
// This is the same mapping used by protocolStateTracker.update.
func eventToTargetState(oe OutputEvent) (TUIState, bool) {
	switch oe.Type {
	case EventCompletion:
		switch oe.Pattern {
		case "system-init", "result-success":
			return StateReady, true
		default:
			return 0, false
		}
	case EventThinking:
		return StateProcessing, true
	case EventError:
		return StateError, true
	case EventRateLimit:
		return StateRateLimited, true
	case EventPermission:
		return StatePermissionPrompt, true
	default:
		return 0, false
	}
}

// ProcessEvent maps a classified OutputEvent to a TUI state transition.
// Only valid in protocol mode; panics in TUI mode.
func (d *ComposedDetector) ProcessEvent(oe OutputEvent, now time.Time) TUIStateUpdate {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.mode != DetectModeProtocol {
		panic("claudemux: ProcessEvent called on TUI-mode ComposedDetector; use ProcessRaw instead")
	}

	next, ok := eventToTargetState(oe)
	if !ok {
		return TUIStateUpdate{
			From:      d.state,
			To:        d.state,
			State:     d.state,
			StateName: tuiStateName(d.state),
			Changed:   false,
			Timestamp: now,
		}
	}

	// Hold sm.mu to ensure consistency with sm.State() and sm.Reset().
	d.sm.mu.Lock()
	update := d.sm.transition(next, oe.Pattern, oe.Fields, now)
	d.sm.mu.Unlock()

	d.state = update.State
	return update
}

// ProcessRaw feeds raw PTY data through the detection pipeline.
// Only valid in TUI mode; panics in protocol mode.
//
// State transitions come from the VTStateDetector. The Parser is also
// run on extracted screen lines for supplementary event classification,
// accessible via LastEvents().
func (d *ComposedDetector) ProcessRaw(data []byte, now time.Time) []TUIStateUpdate {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.mode != DetectModeTUI {
		panic("claudemux: ProcessRaw called on protocol-mode ComposedDetector; use ProcessEvent instead")
	}

	// Delegate state detection to VTStateDetector.
	updates := d.vtDet.ProcessRaw(data, now)
	d.state = d.vtDet.State()

	screen := d.vtDet.ScreenText()
	lines := strings.Split(screen, "\n")
	d.lastEvents = d.lastEvents[:0]
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		oe := d.parser.Parse(line)
		if oe.Type != EventText {
			d.lastEvents = append(d.lastEvents, oe)
		}
	}

	return updates
}

// State returns the current detected TUI state.
func (d *ComposedDetector) State() TUIState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

// LastEvents returns the supplementary event classifications from the most
// recent ProcessRaw call. Only populated in TUI mode.
func (d *ComposedDetector) LastEvents() []OutputEvent {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]OutputEvent, len(d.lastEvents))
	copy(result, d.lastEvents)
	return result
}

// Reset returns the detector to StateInitializing.
func (d *ComposedDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.sm.Reset()
	if d.vtDet != nil {
		d.vtDet.Reset()
	}
	d.state = StateInitializing
	d.lastEvents = d.lastEvents[:0]
}
