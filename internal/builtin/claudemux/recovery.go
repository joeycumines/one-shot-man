package claudemux

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// SupervisorState represents the lifecycle state of a supervised agent.
type SupervisorState int

const (
	SupervisorIdle       SupervisorState = iota // Not yet started
	SupervisorRunning                           // Agent is healthy and running
	SupervisorRecovering                        // Error detected, recovery in progress
	SupervisorDraining                          // Graceful shutdown initiated
	SupervisorStopped                           // Fully stopped, requires reset
)

// SupervisorStateName returns a human-readable name for a SupervisorState.
func SupervisorStateName(s SupervisorState) string {
	switch s {
	case SupervisorIdle:
		return "Idle"
	case SupervisorRunning:
		return "Running"
	case SupervisorRecovering:
		return "Recovering"
	case SupervisorDraining:
		return "Draining"
	case SupervisorStopped:
		return "Stopped"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// ErrorClass classifies errors for recovery decision-making.
type ErrorClass int

const (
	ErrorClassNone         ErrorClass = iota // No error (healthy)
	ErrorClassPTYEOF                         // Agent stdout closed (EOF)
	ErrorClassPTYCrash                       // Agent exited with non-zero code
	ErrorClassPTYError                       // Generic PTY I/O error
	ErrorClassMCPTimeout                     // MCP call timed out
	ErrorClassMCPMalformed                   // MCP response was malformed/invalid
	ErrorClassCancelled                      // Context was cancelled
)

// ErrorClassName returns a human-readable name for an ErrorClass.
func ErrorClassName(c ErrorClass) string {
	switch c {
	case ErrorClassNone:
		return "None"
	case ErrorClassPTYEOF:
		return "PTY-EOF"
	case ErrorClassPTYCrash:
		return "PTY-Crash"
	case ErrorClassPTYError:
		return "PTY-Error"
	case ErrorClassMCPTimeout:
		return "MCP-Timeout"
	case ErrorClassMCPMalformed:
		return "MCP-Malformed"
	case ErrorClassCancelled:
		return "Cancelled"
	default:
		return fmt.Sprintf("Unknown(%d)", int(c))
	}
}

// RecoveryAction specifies what the caller should do in response to an error.
type RecoveryAction int

const (
	RecoveryNone      RecoveryAction = iota // No action needed
	RecoveryRetry                           // Retry the failed operation
	RecoveryRestart                         // Restart the agent process
	RecoveryForceKill                       // Force-kill then restart
	RecoveryEscalate                        // Max retries exceeded, escalate
	RecoveryAbort                           // Unrecoverable, stop permanently
	RecoveryDrain                           // Drain in-flight operations then stop
)

// RecoveryActionName returns a human-readable name for a RecoveryAction.
func RecoveryActionName(a RecoveryAction) string {
	switch a {
	case RecoveryNone:
		return "None"
	case RecoveryRetry:
		return "Retry"
	case RecoveryRestart:
		return "Restart"
	case RecoveryForceKill:
		return "ForceKill"
	case RecoveryEscalate:
		return "Escalate"
	case RecoveryAbort:
		return "Abort"
	case RecoveryDrain:
		return "Drain"
	default:
		return fmt.Sprintf("Unknown(%d)", int(a))
	}
}

// RecoveryDecision is the supervisor's response to an error or lifecycle
// event. It tells the caller what to do and provides context for the decision.
type RecoveryDecision struct {
	Action   RecoveryAction
	Reason   string
	Details  map[string]string
	NewState SupervisorState
}

// SupervisorConfig configures the supervisor's recovery and lifecycle
// behavior. Zero values produce conservative defaults.
type SupervisorConfig struct {
	MaxRetries       int           // Max retries before escalation (default: 3)
	MaxForceKills    int           // Max force-kills before abort (default: 1)
	RetryDelay       time.Duration // Delay between retries (default: 5s)
	ShutdownTimeout  time.Duration // Max time for graceful shutdown (default: 30s)
	ForceKillTimeout time.Duration // Max time to wait for force-kill (default: 5s)
}

// DefaultSupervisorConfig returns production-ready supervisor configuration.
func DefaultSupervisorConfig() SupervisorConfig {
	return SupervisorConfig{
		MaxRetries:       3,
		MaxForceKills:    1,
		RetryDelay:       5 * time.Second,
		ShutdownTimeout:  30 * time.Second,
		ForceKillTimeout: 5 * time.Second,
	}
}

// Supervisor is a state machine that manages agent lifecycle transitions
// with error recovery and graceful shutdown. It does not directly manage
// agents — it advises the caller on what action to take based on events.
//
// Supervisor is safe for concurrent use from multiple goroutines.
type Supervisor struct {
	config SupervisorConfig

	mu             sync.Mutex
	state          SupervisorState
	retryCount     int
	forceKillCount int
	lastError      string
	lastErrorClass ErrorClass
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewSupervisor creates a supervisor with the given configuration and
// a context for cancellation. When the context is cancelled, the
// supervisor transitions to Draining.
func NewSupervisor(ctx context.Context, config SupervisorConfig) *Supervisor {
	derived, cancel := context.WithCancel(ctx)
	return &Supervisor{
		config: config,
		state:  SupervisorIdle,
		ctx:    derived,
		cancel: cancel,
	}
}

// Start transitions the supervisor from Idle to Running. Returns an error
// if the supervisor is not in the Idle state.
func (s *Supervisor) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != SupervisorIdle {
		return fmt.Errorf("claudemux: supervisor cannot start from state %s",
			SupervisorStateName(s.state))
	}

	s.state = SupervisorRunning
	return nil
}

// HandleError processes an error and returns a recovery decision. This is
// the primary decision point for the supervisor. It tracks retry/force-kill
// counts and transitions state appropriately.
func (s *Supervisor) HandleError(errMsg string, class ErrorClass) RecoveryDecision {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check context cancellation first.
	if s.ctx.Err() != nil {
		s.state = SupervisorDraining
		return RecoveryDecision{
			Action:   RecoveryDrain,
			Reason:   "context cancelled during error handling",
			NewState: SupervisorDraining,
			Details: map[string]string{
				"errorClass": ErrorClassName(class),
				"error":      errMsg,
			},
		}
	}

	s.lastError = errMsg
	s.lastErrorClass = class

	// If already stopped or draining, abort.
	if s.state == SupervisorStopped || s.state == SupervisorDraining {
		return RecoveryDecision{
			Action:   RecoveryAbort,
			Reason:   fmt.Sprintf("error during %s state", SupervisorStateName(s.state)),
			NewState: s.state,
			Details: map[string]string{
				"errorClass": ErrorClassName(class),
				"error":      errMsg,
			},
		}
	}

	s.state = SupervisorRecovering

	switch class {
	case ErrorClassCancelled:
		s.state = SupervisorDraining
		return RecoveryDecision{
			Action:   RecoveryDrain,
			Reason:   "operation cancelled",
			NewState: SupervisorDraining,
			Details: map[string]string{
				"error": errMsg,
			},
		}

	case ErrorClassPTYEOF:
		// EOF usually means the agent exited. Try restart.
		return s.decideRestartOrEscalate(class, errMsg)

	case ErrorClassPTYCrash:
		// Crash with non-zero exit. May need force-kill first.
		return s.decideForceKillOrRestart(class, errMsg)

	case ErrorClassPTYError:
		// Generic PTY error. Retry first, then restart.
		return s.decideRetryOrRestart(class, errMsg)

	case ErrorClassMCPTimeout:
		// MCP timeout. Retry the operation.
		return s.decideRetryOrRestart(class, errMsg)

	case ErrorClassMCPMalformed:
		// Malformed MCP response. Not much we can do about this.
		// Retry once in case it's transient, then escalate.
		return s.decideRetryOrEscalate(class, errMsg)

	default:
		// Unknown error class — escalate immediately.
		s.state = SupervisorStopped
		return RecoveryDecision{
			Action:   RecoveryEscalate,
			Reason:   fmt.Sprintf("unknown error class %d", int(class)),
			NewState: SupervisorStopped,
			Details: map[string]string{
				"errorClass": ErrorClassName(class),
				"error":      errMsg,
			},
		}
	}
}

// Shutdown initiates a graceful shutdown. Returns the appropriate drain
// decision. Idempotent — calling multiple times returns Abort after the first.
func (s *Supervisor) Shutdown() RecoveryDecision {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == SupervisorStopped {
		return RecoveryDecision{
			Action:   RecoveryAbort,
			Reason:   "already stopped",
			NewState: SupervisorStopped,
		}
	}

	if s.state == SupervisorDraining {
		// Second shutdown call — force-kill.
		s.state = SupervisorStopped
		s.cancel()
		return RecoveryDecision{
			Action:   RecoveryForceKill,
			Reason:   "shutdown requested during drain — force-killing",
			NewState: SupervisorStopped,
			Details: map[string]string{
				"timeout": s.config.ForceKillTimeout.String(),
			},
		}
	}

	// First shutdown: initiate drain.
	s.state = SupervisorDraining
	return RecoveryDecision{
		Action:   RecoveryDrain,
		Reason:   "graceful shutdown initiated",
		NewState: SupervisorDraining,
		Details: map[string]string{
			"timeout": s.config.ShutdownTimeout.String(),
		},
	}
}

// ConfirmRecovery should be called after a successful recovery (restart
// or retry succeeded). It transitions the supervisor back to Running.
func (s *Supervisor) ConfirmRecovery() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == SupervisorRecovering {
		s.state = SupervisorRunning
		s.lastError = ""
		s.lastErrorClass = ErrorClassNone
	}
}

// ConfirmStopped should be called after the agent has been fully stopped
// (either via shutdown or escalation). Transitions to Stopped.
func (s *Supervisor) ConfirmStopped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = SupervisorStopped
	s.cancel()
}

// Reset returns the supervisor to Idle state for reuse. Clears all counters.
func (s *Supervisor) Reset(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cancel() // cancel old context
	derived, cancel := context.WithCancel(ctx)
	s.ctx = derived
	s.cancel = cancel
	s.state = SupervisorIdle
	s.retryCount = 0
	s.forceKillCount = 0
	s.lastError = ""
	s.lastErrorClass = ErrorClassNone
}

// SupervisorSnapshot holds observable supervisor state for debugging.
type SupervisorSnapshot struct {
	State          SupervisorState `json:"state"`
	StateName      string          `json:"stateName"`
	RetryCount     int             `json:"retryCount"`
	ForceKillCount int             `json:"forceKillCount"`
	LastError      string          `json:"lastError,omitempty"`
	LastErrorClass ErrorClass      `json:"lastErrorClass"`
	Cancelled      bool            `json:"cancelled"`
}

// Snapshot returns the current observable state of the supervisor.
func (s *Supervisor) Snapshot() SupervisorSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SupervisorSnapshot{
		State:          s.state,
		StateName:      SupervisorStateName(s.state),
		RetryCount:     s.retryCount,
		ForceKillCount: s.forceKillCount,
		LastError:      s.lastError,
		LastErrorClass: s.lastErrorClass,
		Cancelled:      s.ctx.Err() != nil,
	}
}

// Context returns the supervisor's context, which is cancelled on shutdown
// or abort. Use this to propagate cancellation to agent operations.
func (s *Supervisor) Context() context.Context {
	return s.ctx
}

// --- internal decision helpers ---

// decideRestartOrEscalate chooses between restart and escalation based
// on the retry count.
func (s *Supervisor) decideRestartOrEscalate(class ErrorClass, errMsg string) RecoveryDecision {
	s.retryCount++

	maxRetries := s.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	if s.retryCount > maxRetries {
		s.state = SupervisorStopped
		return RecoveryDecision{
			Action:   RecoveryEscalate,
			Reason:   fmt.Sprintf("%s: max retries (%d) exceeded", ErrorClassName(class), maxRetries),
			NewState: SupervisorStopped,
			Details: map[string]string{
				"errorClass": ErrorClassName(class),
				"error":      errMsg,
				"retryCount": strconv.Itoa(s.retryCount),
				"maxRetries": strconv.Itoa(maxRetries),
			},
		}
	}

	return RecoveryDecision{
		Action:   RecoveryRestart,
		Reason:   fmt.Sprintf("%s: restarting (%d/%d)", ErrorClassName(class), s.retryCount, maxRetries),
		NewState: SupervisorRecovering,
		Details: map[string]string{
			"errorClass": ErrorClassName(class),
			"error":      errMsg,
			"retryCount": strconv.Itoa(s.retryCount),
			"maxRetries": strconv.Itoa(maxRetries),
			"delay":      s.config.RetryDelay.String(),
		},
	}
}

// decideForceKillOrRestart chooses between force-kill and restart.
// Force-kill is used when the process may still be running.
func (s *Supervisor) decideForceKillOrRestart(class ErrorClass, errMsg string) RecoveryDecision {
	s.forceKillCount++

	maxForceKills := s.config.MaxForceKills
	if maxForceKills <= 0 {
		maxForceKills = 1
	}

	if s.forceKillCount > maxForceKills {
		// Too many force-kills — escalate via the restart path.
		return s.decideRestartOrEscalate(class, errMsg)
	}

	return RecoveryDecision{
		Action:   RecoveryForceKill,
		Reason:   fmt.Sprintf("%s: force-kill then restart (%d/%d)", ErrorClassName(class), s.forceKillCount, maxForceKills),
		NewState: SupervisorRecovering,
		Details: map[string]string{
			"errorClass":     ErrorClassName(class),
			"error":          errMsg,
			"forceKillCount": strconv.Itoa(s.forceKillCount),
			"maxForceKills":  strconv.Itoa(maxForceKills),
			"timeout":        s.config.ForceKillTimeout.String(),
		},
	}
}

// decideRetryOrRestart tries retry first, falls back to restart.
func (s *Supervisor) decideRetryOrRestart(class ErrorClass, errMsg string) RecoveryDecision {
	s.retryCount++

	maxRetries := s.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	// First half of retries: retry the operation.
	// Second half: restart the agent.
	retryThreshold := maxRetries / 2
	if retryThreshold < 1 {
		retryThreshold = 1
	}

	if s.retryCount <= retryThreshold {
		return RecoveryDecision{
			Action:   RecoveryRetry,
			Reason:   fmt.Sprintf("%s: retrying (%d/%d)", ErrorClassName(class), s.retryCount, retryThreshold),
			NewState: SupervisorRecovering,
			Details: map[string]string{
				"errorClass":     ErrorClassName(class),
				"error":          errMsg,
				"retryCount":     strconv.Itoa(s.retryCount),
				"retryThreshold": strconv.Itoa(retryThreshold),
				"delay":          s.config.RetryDelay.String(),
			},
		}
	}

	if s.retryCount > maxRetries {
		s.state = SupervisorStopped
		return RecoveryDecision{
			Action:   RecoveryEscalate,
			Reason:   fmt.Sprintf("%s: max retries (%d) exceeded", ErrorClassName(class), maxRetries),
			NewState: SupervisorStopped,
			Details: map[string]string{
				"errorClass": ErrorClassName(class),
				"error":      errMsg,
				"retryCount": strconv.Itoa(s.retryCount),
				"maxRetries": strconv.Itoa(maxRetries),
			},
		}
	}

	return RecoveryDecision{
		Action:   RecoveryRestart,
		Reason:   fmt.Sprintf("%s: restart after retries exhausted (%d/%d)", ErrorClassName(class), s.retryCount, maxRetries),
		NewState: SupervisorRecovering,
		Details: map[string]string{
			"errorClass": ErrorClassName(class),
			"error":      errMsg,
			"retryCount": strconv.Itoa(s.retryCount),
			"maxRetries": strconv.Itoa(maxRetries),
			"delay":      s.config.RetryDelay.String(),
		},
	}
}

// decideRetryOrEscalate retries once then escalates.
func (s *Supervisor) decideRetryOrEscalate(class ErrorClass, errMsg string) RecoveryDecision {
	s.retryCount++

	maxRetries := s.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	if s.retryCount > 1 {
		s.state = SupervisorStopped
		return RecoveryDecision{
			Action:   RecoveryEscalate,
			Reason:   fmt.Sprintf("%s: retry did not help, escalating", ErrorClassName(class)),
			NewState: SupervisorStopped,
			Details: map[string]string{
				"errorClass": ErrorClassName(class),
				"error":      errMsg,
				"retryCount": strconv.Itoa(s.retryCount),
			},
		}
	}

	return RecoveryDecision{
		Action:   RecoveryRetry,
		Reason:   fmt.Sprintf("%s: retrying once", ErrorClassName(class)),
		NewState: SupervisorRecovering,
		Details: map[string]string{
			"errorClass": ErrorClassName(class),
			"error":      errMsg,
			"retryCount": strconv.Itoa(s.retryCount),
			"delay":      s.config.RetryDelay.String(),
		},
	}
}
