package claudemux

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// GuardAction represents remediation actions triggered by guard monitors.
type GuardAction int

const (
	GuardActionNone     GuardAction = iota // No action needed
	GuardActionPause                       // Pause with backoff (rate limiting)
	GuardActionReject                      // Reject/deny the prompt (permission)
	GuardActionRestart                     // Restart the agent (crash recovery)
	GuardActionEscalate                    // Escalate — max retries exceeded
	GuardActionTimeout                     // Output timeout triggered
)

// GuardActionName returns a human-readable name for a GuardAction.
func GuardActionName(a GuardAction) string {
	switch a {
	case GuardActionNone:
		return "None"
	case GuardActionPause:
		return "Pause"
	case GuardActionReject:
		return "Reject"
	case GuardActionRestart:
		return "Restart"
	case GuardActionEscalate:
		return "Escalate"
	case GuardActionTimeout:
		return "Timeout"
	default:
		return fmt.Sprintf("Unknown(%d)", int(a))
	}
}

// GuardEvent is emitted by the guard when a remediation action is needed.
type GuardEvent struct {
	Action  GuardAction
	Reason  string
	Details map[string]string
}

// GuardConfig holds all guard monitor configuration. Zero values produce
// a guard with all monitors disabled.
type GuardConfig struct {
	RateLimit     RateLimitGuardConfig
	Permission    PermissionGuardConfig
	Crash         CrashGuardConfig
	OutputTimeout OutputTimeoutGuardConfig
}

// DefaultGuardConfig returns a production-ready guard configuration with
// all monitors enabled and safe defaults.
func DefaultGuardConfig() GuardConfig {
	return GuardConfig{
		RateLimit: RateLimitGuardConfig{
			Enabled:      true,
			InitialDelay: 5 * time.Second,
			MaxDelay:     5 * time.Minute,
			Multiplier:   2.0,
			ResetAfter:   10 * time.Minute,
		},
		Permission: PermissionGuardConfig{
			Enabled: true,
			Policy:  PermissionPolicyDeny,
		},
		Crash: CrashGuardConfig{
			Enabled:     true,
			MaxRestarts: 3,
		},
		OutputTimeout: OutputTimeoutGuardConfig{
			Enabled: true,
			Timeout: 5 * time.Minute,
		},
	}
}

// RateLimitGuardConfig controls exponential backoff when rate limits are
// detected in PTY output.
type RateLimitGuardConfig struct {
	Enabled      bool          // Enable rate-limit monitoring
	InitialDelay time.Duration // Initial backoff delay
	MaxDelay     time.Duration // Maximum backoff delay cap
	Multiplier   float64       // Backoff multiplier (e.g., 2.0 for doubling)
	ResetAfter   time.Duration // Reset backoff state after this idle period
}

// PermissionPolicy defines how permission prompts are handled.
type PermissionPolicy int

const (
	// PermissionPolicyDeny rejects all permission prompts (fail-closed, safe default).
	PermissionPolicyDeny PermissionPolicy = iota
	// PermissionPolicyAllow auto-accepts permission prompts (DANGEROUS).
	PermissionPolicyAllow
)

// PermissionGuardConfig controls permission prompt handling.
type PermissionGuardConfig struct {
	Enabled bool             // Enable permission monitoring
	Policy  PermissionPolicy // How to handle prompts
}

// CrashGuardConfig controls crash detection and restart policy.
type CrashGuardConfig struct {
	Enabled     bool // Enable crash monitoring
	MaxRestarts int  // Max restarts before escalation (0 = no restarts)
}

// OutputTimeoutGuardConfig controls output timeout detection.
type OutputTimeoutGuardConfig struct {
	Enabled bool          // Enable output timeout monitoring
	Timeout time.Duration // Duration of silence before triggering
}

// Guard monitors PTY output events and agent health, determining remediation
// actions when anomalies are detected. It tracks internal state for backoff,
// restart counts, and output timing. Time is passed explicitly for
// deterministic testing.
//
// Guard is NOT safe for concurrent use from multiple goroutines.
type Guard struct {
	config GuardConfig

	// Rate limit state.
	rateLimitCount    int
	currentDelay      time.Duration
	lastRateLimitTime time.Time

	// Crash state.
	crashCount int

	// Timeout state.
	lastEventTime time.Time
	started       bool
}

// NewGuard creates a guard monitor with the given configuration.
func NewGuard(config GuardConfig) *Guard {
	return &Guard{
		config: config,
	}
}

// ProcessEvent evaluates a parser output event and returns a guard event
// if remediation is needed, or nil if the event is benign. The now parameter
// allows deterministic testing without timers.
func (g *Guard) ProcessEvent(ev OutputEvent, now time.Time) *GuardEvent {
	// Track last event time for timeout detection.
	g.lastEventTime = now
	g.started = true

	switch ev.Type {
	case EventRateLimit:
		return g.handleRateLimit(ev, now)
	case EventPermission:
		return g.handlePermission(ev)
	default:
		// Non-rate-limit events may reset the backoff if enough time has passed.
		g.maybeResetRateLimit(now)
		return nil
	}
}

// ProcessCrash evaluates an agent crash (unexpected exit) and returns the
// appropriate remediation action. Call this when the agent process exits
// with a non-zero code or unexpectedly.
func (g *Guard) ProcessCrash(exitCode int, now time.Time) *GuardEvent {
	if !g.config.Crash.Enabled {
		return nil
	}

	g.crashCount++

	if g.config.Crash.MaxRestarts > 0 && g.crashCount <= g.config.Crash.MaxRestarts {
		return &GuardEvent{
			Action: GuardActionRestart,
			Reason: fmt.Sprintf("agent crashed (exit %d), restart %d/%d",
				exitCode, g.crashCount, g.config.Crash.MaxRestarts),
			Details: map[string]string{
				"exitCode":     strconv.Itoa(exitCode),
				"restartCount": strconv.Itoa(g.crashCount),
				"maxRestarts":  strconv.Itoa(g.config.Crash.MaxRestarts),
			},
		}
	}

	return &GuardEvent{
		Action: GuardActionEscalate,
		Reason: fmt.Sprintf("agent crashed (exit %d), max restarts (%d) exceeded",
			exitCode, g.config.Crash.MaxRestarts),
		Details: map[string]string{
			"exitCode":     strconv.Itoa(exitCode),
			"restartCount": strconv.Itoa(g.crashCount),
			"maxRestarts":  strconv.Itoa(g.config.Crash.MaxRestarts),
		},
	}
}

// CheckTimeout checks whether the output timeout has elapsed since the last
// event. Returns a timeout guard event if exceeded, or nil. Call this
// periodically (e.g., in a poll loop). The now parameter allows deterministic
// testing.
func (g *Guard) CheckTimeout(now time.Time) *GuardEvent {
	if !g.config.OutputTimeout.Enabled || !g.started {
		return nil
	}
	if g.config.OutputTimeout.Timeout <= 0 {
		return nil
	}

	elapsed := now.Sub(g.lastEventTime)
	if elapsed >= g.config.OutputTimeout.Timeout {
		return &GuardEvent{
			Action: GuardActionTimeout,
			Reason: fmt.Sprintf("no output for %s (timeout: %s)",
				elapsed.Truncate(time.Second), g.config.OutputTimeout.Timeout),
			Details: map[string]string{
				"elapsed": elapsed.Truncate(time.Millisecond).String(),
				"timeout": g.config.OutputTimeout.Timeout.String(),
			},
		}
	}

	return nil
}

// ResetCrashCount resets the crash counter. Call after a successful restart
// has been confirmed healthy.
func (g *Guard) ResetCrashCount() {
	g.crashCount = 0
}

// GuardState holds observable guard monitor state for debugging and metrics.
type GuardState struct {
	RateLimitCount    int           `json:"rateLimitCount"`
	CurrentDelay      time.Duration `json:"currentDelay"`
	LastRateLimitTime time.Time     `json:"lastRateLimitTime,omitempty"`
	CrashCount        int           `json:"crashCount"`
	LastEventTime     time.Time     `json:"lastEventTime,omitempty"`
	Started           bool          `json:"started"`
}

// State returns the current observable state of the guard for debugging.
func (g *Guard) State() GuardState {
	return GuardState{
		RateLimitCount:    g.rateLimitCount,
		CurrentDelay:      g.currentDelay,
		LastRateLimitTime: g.lastRateLimitTime,
		CrashCount:        g.crashCount,
		LastEventTime:     g.lastEventTime,
		Started:           g.started,
	}
}

// Config returns the guard's configuration.
func (g *Guard) Config() GuardConfig {
	return g.config
}

// handleRateLimit processes a rate-limit event with exponential backoff.
func (g *Guard) handleRateLimit(ev OutputEvent, now time.Time) *GuardEvent {
	if !g.config.RateLimit.Enabled {
		return nil
	}

	g.rateLimitCount++
	g.lastRateLimitTime = now

	// Compute delay using exponential backoff.
	delay := g.computeBackoff()

	// If the event provides a retryAfter hint, use it if it's longer.
	if ev.Fields != nil {
		if retryStr, ok := ev.Fields["retryAfter"]; ok {
			if retrySec, err := strconv.ParseFloat(retryStr, 64); err == nil {
				hint := time.Duration(retrySec * float64(time.Second))
				if hint > delay {
					delay = hint
				}
			}
		}
	}

	g.currentDelay = delay

	return &GuardEvent{
		Action: GuardActionPause,
		Reason: fmt.Sprintf("rate limited (%d occurrences), backoff %s",
			g.rateLimitCount, delay.Truncate(time.Millisecond)),
		Details: map[string]string{
			"delay":          delay.Truncate(time.Millisecond).String(),
			"rateLimitCount": strconv.Itoa(g.rateLimitCount),
		},
	}
}

// computeBackoff calculates the current backoff delay using exponential backoff
// with the configured multiplier and cap.
func (g *Guard) computeBackoff() time.Duration {
	cfg := g.config.RateLimit
	if cfg.InitialDelay <= 0 {
		return 0
	}

	// Exponential: initial * multiplier^(count-1)
	mult := cfg.Multiplier
	if mult <= 0 {
		mult = 2.0
	}
	factor := math.Pow(mult, float64(g.rateLimitCount-1))
	delay := time.Duration(float64(cfg.InitialDelay) * factor)

	// Cap at MaxDelay.
	if cfg.MaxDelay > 0 && delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}

	return delay
}

// maybeResetRateLimit resets the backoff state if enough time has passed
// since the last rate limit event.
func (g *Guard) maybeResetRateLimit(now time.Time) {
	cfg := g.config.RateLimit
	if !cfg.Enabled || cfg.ResetAfter <= 0 || g.rateLimitCount == 0 {
		return
	}
	if now.Sub(g.lastRateLimitTime) >= cfg.ResetAfter {
		g.rateLimitCount = 0
		g.currentDelay = 0
		g.lastRateLimitTime = time.Time{}
	}
}

// handlePermission processes a permission prompt event according to policy.
func (g *Guard) handlePermission(ev OutputEvent) *GuardEvent {
	if !g.config.Permission.Enabled {
		return nil
	}

	switch g.config.Permission.Policy {
	case PermissionPolicyDeny:
		return &GuardEvent{
			Action: GuardActionReject,
			Reason: "permission prompt detected, policy: deny (fail-closed)",
			Details: map[string]string{
				"line":    ev.Line,
				"pattern": ev.Pattern,
			},
		}
	case PermissionPolicyAllow:
		// Allow policy — no guard action, caller handles auto-accept.
		return nil
	default:
		// Unknown policy — fail-closed (safe).
		return &GuardEvent{
			Action: GuardActionReject,
			Reason: "permission prompt detected, unknown policy (fail-closed)",
			Details: map[string]string{
				"line":    ev.Line,
				"pattern": ev.Pattern,
			},
		}
	}
}
