package claudemux

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"
)

// ManagedSessionState represents the lifecycle of a managed session.
type ManagedSessionState int

const (
	SessionIdle   ManagedSessionState = iota // Created, not yet started
	SessionActive                            // Running and processing events
	SessionPaused                            // Temporarily paused (rate-limit backoff)
	SessionFailed                            // Failed, needs recovery or close
	SessionClosed                            // Fully closed
)

// ManagedSessionStateName returns a human-readable name for a session state.
func ManagedSessionStateName(s ManagedSessionState) string {
	switch s {
	case SessionIdle:
		return "Idle"
	case SessionActive:
		return "Active"
	case SessionPaused:
		return "Paused"
	case SessionFailed:
		return "Failed"
	case SessionClosed:
		return "Closed"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// ManagedSessionConfig holds configuration for a ManagedSession.
type ManagedSessionConfig struct {
	Guard      GuardConfig
	MCPGuard   MCPGuardConfig
	Supervisor SupervisorConfig
}

// DefaultManagedSessionConfig returns production-ready defaults.
func DefaultManagedSessionConfig() ManagedSessionConfig {
	return ManagedSessionConfig{
		Guard:      DefaultGuardConfig(),
		MCPGuard:   DefaultMCPGuardConfig(),
		Supervisor: DefaultSupervisorConfig(),
	}
}

// LineResult is the result of processing a single output line through
// the monitoring pipeline.
type LineResult struct {
	Event      OutputEvent // Parsed event
	GuardEvent *GuardEvent // Non-nil if guard triggered
	Action     string      // Summary action: "none", "pause", "reject", "restart", "escalate", "timeout"
}

// ToolCallResult is the result of processing a tool call through guards.
type ToolCallResult struct {
	GuardEvent *GuardEvent // Non-nil if MCP guard triggered
	Action     string      // Summary action: "none", "reject", "escalate", "timeout"
}

// ManagedSessionSnapshot captures the full state of a session for observability.
type ManagedSessionSnapshot struct {
	ID              string
	State           ManagedSessionState
	StateName       string
	LinesProcessed  int64
	EventCounts     map[string]int64 // EventTypeName -> count
	LastEvent       *OutputEvent
	GuardState      GuardState
	MCPGuardState   MCPGuardState
	SupervisorState SupervisorSnapshot
}

// ManagedSession composes a Parser, Guard, MCPGuard, and Supervisor into a
// single monitoring pipeline for a Claude Code instance. It does NOT own the
// PTY or agent handle — those are managed externally (by the Pool/Panel or
// direct user code). It provides the classification + monitoring layer.
//
// Thread-safe: all methods may be called from any goroutine.
type ManagedSession struct {
	id     string
	config ManagedSessionConfig

	parser     *Parser
	guard      *Guard
	mcpGuard   *MCPGuard
	supervisor *Supervisor

	mu             sync.Mutex
	state          ManagedSessionState
	linesProcessed int64
	eventCounts    map[string]int64
	lastEvent      *OutputEvent

	// OnEvent is called synchronously for every parsed event (if set).
	// The callback must not block.
	OnEvent func(OutputEvent)

	// OnGuardAction is called synchronously when any guard triggers (if set).
	// The callback must not block.
	OnGuardAction func(*GuardEvent)

	// OnRecoveryDecision is called when the supervisor makes a decision (if set).
	// The callback must not block.
	OnRecoveryDecision func(RecoveryDecision)
}

// NewManagedSession creates a session with the given ID and configuration.
// The session starts in Idle state; call Start() to activate it.
func NewManagedSession(ctx context.Context, id string, cfg ManagedSessionConfig) *ManagedSession {
	return &ManagedSession{
		id:          id,
		config:      cfg,
		parser:      NewParser(),
		guard:       NewGuard(cfg.Guard),
		mcpGuard:    NewMCPGuard(cfg.MCPGuard),
		supervisor:  NewSupervisor(ctx, cfg.Supervisor),
		state:       SessionIdle,
		eventCounts: make(map[string]int64),
	}
}

// Start transitions the session to Active state.
func (s *ManagedSession) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != SessionIdle {
		return fmt.Errorf("claudemux: cannot start session %q from state %s",
			s.id, ManagedSessionStateName(s.state))
	}
	if err := s.supervisor.Start(); err != nil {
		return fmt.Errorf("claudemux: supervisor start: %w", err)
	}
	s.state = SessionActive
	return nil
}

// ProcessLine parses and monitors a single output line. Returns the parsed
// event and any guard action. The now parameter enables deterministic testing.
func (s *ManagedSession) ProcessLine(line string, now time.Time) LineResult {
	ev := s.parser.Parse(line)

	s.mu.Lock()
	s.linesProcessed++
	s.eventCounts[EventTypeName(ev.Type)]++
	evCopy := ev
	s.lastEvent = &evCopy
	state := s.state

	// Guard call under lock — Guard is not internally thread-safe.
	var ge *GuardEvent
	if state == SessionActive {
		ge = s.guard.ProcessEvent(ev, now)
		if ge != nil {
			switch ge.Action {
			case GuardActionPause:
				s.state = SessionPaused
			case GuardActionRestart, GuardActionEscalate, GuardActionTimeout:
				s.state = SessionFailed
			}
		}
	}
	s.mu.Unlock()

	// Callbacks outside lock to avoid deadlock.
	if s.OnEvent != nil {
		s.OnEvent(ev)
	}
	if ge != nil && s.OnGuardAction != nil {
		s.OnGuardAction(ge)
	}

	if ge == nil {
		return LineResult{Event: ev, Action: "none"}
	}
	return LineResult{Event: ev, GuardEvent: ge, Action: guardActionToString(ge.Action)}
}

// ProcessCrash reports a crash (non-zero exit) to the guard and supervisor.
func (s *ManagedSession) ProcessCrash(exitCode int, now time.Time) (*GuardEvent, RecoveryDecision) {
	// Guard call under lock — Guard is not internally thread-safe.
	s.mu.Lock()
	ge := s.guard.ProcessCrash(exitCode, now)
	s.mu.Unlock()

	// Supervisor has its own internal synchronization.
	d := s.supervisor.HandleError(
		fmt.Sprintf("exit code %d", exitCode),
		ErrorClassPTYCrash,
	)

	// Callbacks outside lock.
	if s.OnGuardAction != nil && ge != nil {
		s.OnGuardAction(ge)
	}
	if s.OnRecoveryDecision != nil {
		s.OnRecoveryDecision(d)
	}

	// Update state based on guard OR supervisor escalation.
	s.mu.Lock()
	if d.Action == RecoveryEscalate || d.Action == RecoveryAbort {
		s.state = SessionFailed
	} else if ge != nil && (ge.Action == GuardActionEscalate || ge.Action == GuardActionTimeout) {
		s.state = SessionFailed
	}
	s.mu.Unlock()

	return ge, d
}

// ProcessToolCall routes an MCP tool call through the MCP guard.
func (s *ManagedSession) ProcessToolCall(call MCPToolCall) ToolCallResult {
	// MCPGuard call under lock — MCPGuard is not internally thread-safe.
	s.mu.Lock()
	ge := s.mcpGuard.ProcessToolCall(call)
	s.mu.Unlock()

	if ge == nil {
		return ToolCallResult{Action: "none"}
	}

	// Callback outside lock.
	if s.OnGuardAction != nil {
		s.OnGuardAction(ge)
	}

	return ToolCallResult{GuardEvent: ge, Action: guardActionToString(ge.Action)}
}

// CheckTimeout checks for both output timeout and MCP no-call timeout.
func (s *ManagedSession) CheckTimeout(now time.Time) *GuardEvent {
	// Guard/MCPGuard calls under lock — not internally thread-safe.
	s.mu.Lock()
	ge := s.guard.CheckTimeout(now)
	if ge != nil {
		s.state = SessionFailed
		s.mu.Unlock()
		if s.OnGuardAction != nil {
			s.OnGuardAction(ge)
		}
		return ge
	}
	ge = s.mcpGuard.CheckNoCallTimeout(now)
	if ge != nil {
		s.state = SessionFailed
	}
	s.mu.Unlock()

	if ge != nil {
		if s.OnGuardAction != nil {
			s.OnGuardAction(ge)
		}
		return ge
	}
	return nil
}

// HandleError sends an error to the supervisor and returns the recovery decision.
func (s *ManagedSession) HandleError(msg string, class ErrorClass) RecoveryDecision {
	d := s.supervisor.HandleError(msg, class)
	if s.OnRecoveryDecision != nil {
		s.OnRecoveryDecision(d)
	}

	s.mu.Lock()
	if d.Action == RecoveryEscalate || d.Action == RecoveryAbort {
		s.state = SessionFailed
	}
	s.mu.Unlock()

	return d
}

// ConfirmRecovery confirms that recovery completed and the session can resume.
func (s *ManagedSession) ConfirmRecovery() {
	s.supervisor.ConfirmRecovery()
	s.mu.Lock()
	s.state = SessionActive
	s.mu.Unlock()
}

// Resume returns the session from Paused to Active.
func (s *ManagedSession) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != SessionPaused {
		return fmt.Errorf("claudemux: cannot resume session %q from state %s",
			s.id, ManagedSessionStateName(s.state))
	}
	s.state = SessionActive
	return nil
}

// Shutdown gracefully shuts down the supervisor.
func (s *ManagedSession) Shutdown() RecoveryDecision {
	d := s.supervisor.Shutdown()
	if s.OnRecoveryDecision != nil {
		s.OnRecoveryDecision(d)
	}
	return d
}

// Close transitions to Closed state and cleans up.
func (s *ManagedSession) Close() {
	s.supervisor.ConfirmStopped()
	s.mu.Lock()
	s.state = SessionClosed
	s.mu.Unlock()
}

// Parser returns the session's parser for custom pattern registration.
func (s *ManagedSession) Parser() *Parser {
	return s.parser
}

// ID returns the session identifier.
func (s *ManagedSession) ID() string {
	return s.id
}

// State returns the current session state.
func (s *ManagedSession) State() ManagedSessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Snapshot returns a complete observability snapshot.
func (s *ManagedSession) Snapshot() ManagedSessionSnapshot {
	s.mu.Lock()
	counts := make(map[string]int64, len(s.eventCounts))
	maps.Copy(counts, s.eventCounts)
	var lastEvCopy *OutputEvent
	if s.lastEvent != nil {
		cp := *s.lastEvent
		lastEvCopy = &cp
	}
	state := s.state
	lines := s.linesProcessed
	// Guard/MCPGuard State under lock — not internally thread-safe.
	guardState := s.guard.State()
	mcpGuardState := s.mcpGuard.State()
	s.mu.Unlock()

	return ManagedSessionSnapshot{
		ID:              s.id,
		State:           state,
		StateName:       ManagedSessionStateName(state),
		LinesProcessed:  lines,
		EventCounts:     counts,
		LastEvent:       lastEvCopy,
		GuardState:      guardState,
		MCPGuardState:   mcpGuardState,
		SupervisorState: s.supervisor.Snapshot(),
	}
}

// guardActionToString converts a GuardAction to a simple string tag.
func guardActionToString(a GuardAction) string {
	switch a {
	case GuardActionNone:
		return "none"
	case GuardActionPause:
		return "pause"
	case GuardActionReject:
		return "reject"
	case GuardActionRestart:
		return "restart"
	case GuardActionEscalate:
		return "escalate"
	case GuardActionTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}
