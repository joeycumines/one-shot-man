package claudemux

import (
	"fmt"
	"strconv"
	"time"
)

// MCPGuardConfig holds configuration for MCP tool-call monitoring.
// Zero values produce a guard with all monitors disabled.
type MCPGuardConfig struct {
	NoCallTimeout   MCPNoCallTimeoutConfig
	FrequencyLimit  MCPFrequencyLimitConfig
	RepeatDetection MCPRepeatDetectionConfig
	ToolAllowlist   MCPToolAllowlistConfig
}

// DefaultMCPGuardConfig returns a production-ready MCP guard configuration.
func DefaultMCPGuardConfig() MCPGuardConfig {
	return MCPGuardConfig{
		NoCallTimeout: MCPNoCallTimeoutConfig{
			Enabled: true,
			Timeout: 10 * time.Minute,
		},
		FrequencyLimit: MCPFrequencyLimitConfig{
			Enabled:  true,
			Window:   10 * time.Second,
			MaxCalls: 50,
		},
		RepeatDetection: MCPRepeatDetectionConfig{
			Enabled:      true,
			MaxRepeats:   5,
			WindowSize:   20,
			MatchTool:    true,
			MatchArgHash: true,
		},
		ToolAllowlist: MCPToolAllowlistConfig{
			Enabled: false, // disabled by default — all tools allowed
		},
	}
}

// MCPNoCallTimeoutConfig controls detection of agents that register
// a session but never call any MCP tools.
type MCPNoCallTimeoutConfig struct {
	Enabled bool          // Enable no-call timeout monitoring
	Timeout time.Duration // How long to wait for the first/next call
}

// MCPFrequencyLimitConfig controls detection of excessive MCP tool call
// frequency (e.g., runaway loops).
type MCPFrequencyLimitConfig struct {
	Enabled  bool          // Enable frequency monitoring
	Window   time.Duration // Time window for counting calls
	MaxCalls int           // Max calls allowed within the window
}

// MCPRepeatDetectionConfig controls detection of repeated identical
// tool calls (stuck-loop detection).
type MCPRepeatDetectionConfig struct {
	Enabled      bool // Enable repeat detection
	MaxRepeats   int  // Max identical consecutive calls before triggering
	WindowSize   int  // Size of the recent-call window to check
	MatchTool    bool // Match on tool name
	MatchArgHash bool // Match on argument content (hash)
}

// MCPToolAllowlistConfig controls validation of tool names against
// a known allowlist.
type MCPToolAllowlistConfig struct {
	Enabled      bool     // Enable allowlist checking
	AllowedTools []string // List of permitted tool names
}

// MCPToolCall represents a single MCP tool call for guard analysis.
type MCPToolCall struct {
	ToolName  string
	Arguments string // Raw JSON arguments string
	Timestamp time.Time
}

// MCPGuard monitors MCP tool call patterns and detects anomalies.
// It tracks call history for frequency, repetition, and timeout analysis.
// Time is passed explicitly for deterministic testing.
//
// MCPGuard is NOT safe for concurrent use from multiple goroutines.
type MCPGuard struct {
	config MCPGuardConfig

	// Recent call history (ring buffer semantics with append+trim).
	recentCalls []MCPToolCall

	// Tracking state.
	lastCallTime time.Time
	totalCalls   int
	started      bool // true after first ProcessToolCall

	// Allowlist lookup cache (built once from config).
	allowedSet map[string]struct{}
}

// NewMCPGuard creates an MCP guard monitor with the given configuration.
func NewMCPGuard(config MCPGuardConfig) *MCPGuard {
	g := &MCPGuard{
		config: config,
	}

	// Build allowlist lookup set.
	if config.ToolAllowlist.Enabled && len(config.ToolAllowlist.AllowedTools) > 0 {
		g.allowedSet = make(map[string]struct{}, len(config.ToolAllowlist.AllowedTools))
		for _, tool := range config.ToolAllowlist.AllowedTools {
			g.allowedSet[tool] = struct{}{}
		}
	}

	return g
}

// ProcessToolCall evaluates a tool call and returns a guard event if
// a violation is detected, or nil if the call is normal. Checks are
// applied in order: allowlist, frequency, repetition. The first violation
// wins.
func (g *MCPGuard) ProcessToolCall(call MCPToolCall) *GuardEvent {
	g.started = true
	g.lastCallTime = call.Timestamp
	g.totalCalls++

	// Append to recent history.
	g.recentCalls = append(g.recentCalls, call)

	// Trim history to a reasonable size (max of window sizes + buffer).
	maxHistory := 100
	if g.config.RepeatDetection.WindowSize > maxHistory {
		maxHistory = g.config.RepeatDetection.WindowSize + 10
	}
	if len(g.recentCalls) > maxHistory {
		g.recentCalls = g.recentCalls[len(g.recentCalls)-maxHistory:]
	}

	// Check 1: Allowlist.
	if ge := g.checkAllowlist(call); ge != nil {
		return ge
	}

	// Check 2: Frequency.
	if ge := g.checkFrequency(call); ge != nil {
		return ge
	}

	// Check 3: Repetition.
	if ge := g.checkRepetition(); ge != nil {
		return ge
	}

	return nil
}

// CheckNoCallTimeout checks whether the no-call timeout has elapsed.
// Returns a timeout guard event if exceeded, or nil. Call this
// periodically. The now parameter allows deterministic testing.
func (g *MCPGuard) CheckNoCallTimeout(now time.Time) *GuardEvent {
	if !g.config.NoCallTimeout.Enabled || !g.started {
		return nil
	}
	if g.config.NoCallTimeout.Timeout <= 0 {
		return nil
	}

	elapsed := now.Sub(g.lastCallTime)
	if elapsed >= g.config.NoCallTimeout.Timeout {
		return &GuardEvent{
			Action: GuardActionTimeout,
			Reason: fmt.Sprintf("no MCP tool calls for %s (timeout: %s)",
				elapsed.Truncate(time.Second), g.config.NoCallTimeout.Timeout),
			Details: map[string]string{
				"elapsed":    elapsed.Truncate(time.Millisecond).String(),
				"timeout":    g.config.NoCallTimeout.Timeout.String(),
				"totalCalls": strconv.Itoa(g.totalCalls),
			},
		}
	}

	return nil
}

// MCPGuardState holds observable MCP guard state for debugging and metrics.
type MCPGuardState struct {
	TotalCalls   int       `json:"totalCalls"`
	LastCallTime time.Time `json:"lastCallTime,omitempty"`
	RecentCount  int       `json:"recentCount"`
	Started      bool      `json:"started"`
}

// State returns the current observable state of the MCP guard.
func (g *MCPGuard) State() MCPGuardState {
	return MCPGuardState{
		TotalCalls:   g.totalCalls,
		LastCallTime: g.lastCallTime,
		RecentCount:  len(g.recentCalls),
		Started:      g.started,
	}
}

// Config returns the MCP guard's configuration.
func (g *MCPGuard) Config() MCPGuardConfig {
	return g.config
}

// checkAllowlist validates the tool name against the configured allowlist.
func (g *MCPGuard) checkAllowlist(call MCPToolCall) *GuardEvent {
	if !g.config.ToolAllowlist.Enabled || g.allowedSet == nil {
		return nil
	}

	if _, ok := g.allowedSet[call.ToolName]; !ok {
		return &GuardEvent{
			Action: GuardActionReject,
			Reason: fmt.Sprintf("tool %q not in allowlist", call.ToolName),
			Details: map[string]string{
				"toolName":     call.ToolName,
				"allowedCount": strconv.Itoa(len(g.allowedSet)),
			},
		}
	}

	return nil
}

// checkFrequency counts recent calls within the configured window and
// triggers if the limit is exceeded.
func (g *MCPGuard) checkFrequency(call MCPToolCall) *GuardEvent {
	if !g.config.FrequencyLimit.Enabled {
		return nil
	}
	cfg := g.config.FrequencyLimit
	if cfg.Window <= 0 || cfg.MaxCalls <= 0 {
		return nil
	}

	cutoff := call.Timestamp.Add(-cfg.Window)
	count := 0
	for i := len(g.recentCalls) - 1; i >= 0; i-- {
		if g.recentCalls[i].Timestamp.Before(cutoff) {
			break
		}
		count++
	}

	if count > cfg.MaxCalls {
		return &GuardEvent{
			Action: GuardActionPause,
			Reason: fmt.Sprintf("excessive MCP call frequency: %d calls in %s (max: %d)",
				count, cfg.Window, cfg.MaxCalls),
			Details: map[string]string{
				"callCount": strconv.Itoa(count),
				"maxCalls":  strconv.Itoa(cfg.MaxCalls),
				"window":    cfg.Window.String(),
			},
		}
	}

	return nil
}

// checkRepetition looks at the tail of recent calls for consecutive
// identical calls (same tool name and optionally same arguments).
func (g *MCPGuard) checkRepetition() *GuardEvent {
	if !g.config.RepeatDetection.Enabled {
		return nil
	}
	cfg := g.config.RepeatDetection
	if cfg.MaxRepeats <= 0 {
		return nil
	}

	n := len(g.recentCalls)
	if n < 2 {
		return nil
	}

	// Count consecutive identical calls from the tail.
	latest := g.recentCalls[n-1]
	repeats := 1

	windowStart := n - cfg.WindowSize
	if windowStart < 0 {
		windowStart = 0
	}

	for i := n - 2; i >= windowStart; i-- {
		prev := g.recentCalls[i]
		match := true

		if cfg.MatchTool && prev.ToolName != latest.ToolName {
			match = false
		}
		if match && cfg.MatchArgHash && prev.Arguments != latest.Arguments {
			match = false
		}

		if !match {
			break
		}
		repeats++
	}

	if repeats > cfg.MaxRepeats {
		return &GuardEvent{
			Action: GuardActionEscalate,
			Reason: fmt.Sprintf("repeated MCP call detected: %q called %d times consecutively (max: %d)",
				latest.ToolName, repeats, cfg.MaxRepeats),
			Details: map[string]string{
				"toolName":   latest.ToolName,
				"repeats":    strconv.Itoa(repeats),
				"maxRepeats": strconv.Itoa(cfg.MaxRepeats),
			},
		}
	}

	return nil
}
