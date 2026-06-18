package aimuxcore

import (
	"fmt"
	"regexp"
	"sync"
	"time"
)

// TUIState represents the current state of a TUI-based agent.
type TUIState int

const (
	// StateInitializing means no ready indicator has been detected yet.
	StateInitializing TUIState = iota
	// StateReady means a ready/prompt indicator was detected and the agent
	// is not processing.
	StateReady
	// StateProcessing means a processing/thinking indicator was detected.
	// This OVERRIDES StateReady because the prompt character remains visible
	// on screen while the agent is working.
	StateProcessing
	// StateResponding means output was received without a processing indicator.
	StateResponding
	// StateError means an error pattern was detected.
	StateError
	// StateRateLimited means a rate limit pattern was detected.
	StateRateLimited
	// StatePermissionPrompt means a permission prompt was detected.
	StatePermissionPrompt
)

// TUIStateUpdate describes a state transition or confirmation.
type TUIStateUpdate struct {
	From      TUIState
	To        TUIState
	State     TUIState
	StateName string
	Pattern   string
	Fields    map[string]string
	Changed   bool
	Timestamp time.Time
}

// TUIStateMachineConfig holds the regex patterns and timeouts for the state machine.
type TUIStateMachineConfig struct {
	ReadyPatterns      []string
	ProcessingPatterns []string
	ErrorPatterns      []string
	RateLimitPatterns  []string
	PermissionPatterns []string
	StartupTimeout     time.Duration
	ProcessingTimeout  time.Duration
}

// DefaultTUIStateConfig returns the default configuration for
// detecting AI terminal TUI states.
func DefaultTUIStateConfig() TUIStateMachineConfig {
	return TUIStateMachineConfig{
		ReadyPatterns: []string{
			`^>\s*$`,
		},
		ProcessingPatterns: []string{
			`(?i)(thinking|analyzing|processing|working)\.\.\.`,
			`(?i)running\.\.\.`,
		},
		ErrorPatterns: []string{
			`(?i)^error:`,
			`(?i)^fatal:`,
		},
		RateLimitPatterns: []string{
			`(?i)rate\s*limit`,
			`(?i)429`,
		},
		PermissionPatterns: []string{
			`(?i)allow\s+`,
			`(?i)permit\s+`,
		},
		StartupTimeout:    30 * time.Second,
		ProcessingTimeout: 120 * time.Second,
	}
}

// compiledPattern holds a pre-compiled regex pattern with its name.
type compiledPattern struct {
	name string
	re   *regexp.Regexp
}

// TUIStateMachine classifies an agent's current state from its output.
// It is safe for concurrent use from multiple goroutines.
type TUIStateMachine struct {
	mu sync.Mutex

	config         TUIStateMachineConfig
	currentState   TUIState
	createdAt      time.Time
	lastTransition time.Time

	readyPatterns      []compiledPattern
	processingPatterns []compiledPattern
	errorPatterns      []compiledPattern
	rateLimitPatterns  []compiledPattern
	permissionPatterns []compiledPattern
}

// NewTUIStateMachine creates a state machine with the given configuration.
// All regex patterns are compiled at construction time. Returns an error if
// any pattern fails to compile.
// The initial state is StateInitializing.
func NewTUIStateMachine(config TUIStateMachineConfig) (*TUIStateMachine, error) {
	sm := &TUIStateMachine{
		config:       config,
		currentState: StateInitializing,
		createdAt:    time.Now(),
	}

	var err error
	sm.readyPatterns, err = compilePatterns("ready", config.ReadyPatterns)
	if err != nil {
		return nil, fmt.Errorf("ready pattern: %w", err)
	}
	sm.processingPatterns, err = compilePatterns("processing", config.ProcessingPatterns)
	if err != nil {
		return nil, fmt.Errorf("processing pattern: %w", err)
	}
	sm.errorPatterns, err = compilePatterns("error", config.ErrorPatterns)
	if err != nil {
		return nil, fmt.Errorf("error pattern: %w", err)
	}
	sm.rateLimitPatterns, err = compilePatterns("ratelimit", config.RateLimitPatterns)
	if err != nil {
		return nil, fmt.Errorf("ratelimit pattern: %w", err)
	}
	sm.permissionPatterns, err = compilePatterns("permission", config.PermissionPatterns)
	if err != nil {
		return nil, fmt.Errorf("permission pattern: %w", err)
	}
	return sm, nil
}

// ProcessOutput classifies a raw output line and returns a state update.
// Priority order (highest to lowest): Error > RateLimit > Permission > Processing > Ready.
// Processing patterns OVERRIDE Ready patterns — if both match, state is Processing.
// If no patterns match, the current state is returned with Changed=false.
func (sm *TUIStateMachine) ProcessOutput(raw string, now time.Time) TUIStateUpdate {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if matched, pat, fields := sm.matchPatterns(raw, sm.errorPatterns); matched {
		return sm.transition(StateError, pat, fields, now)
	}
	if matched, pat, fields := sm.matchPatterns(raw, sm.rateLimitPatterns); matched {
		return sm.transition(StateRateLimited, pat, fields, now)
	}
	if matched, pat, fields := sm.matchPatterns(raw, sm.permissionPatterns); matched {
		return sm.transition(StatePermissionPrompt, pat, fields, now)
	}
	if matched, pat, fields := sm.matchPatterns(raw, sm.processingPatterns); matched {
		return sm.transition(StateProcessing, pat, fields, now)
	}
	if matched, pat, fields := sm.matchPatterns(raw, sm.readyPatterns); matched {
		return sm.transition(StateReady, pat, fields, now)
	}

	// No pattern matched. If we were in StateProcessing and receive
	// unrecognized output, transition to StateResponding.
	if sm.currentState == StateProcessing {
		return sm.transition(StateResponding, "", nil, now)
	}

	return TUIStateUpdate{
		From:      sm.currentState,
		To:        sm.currentState,
		State:     sm.currentState,
		StateName: tuiStateName(sm.currentState),
		Changed:   false,
		Timestamp: now,
	}
}

// CheckTimeout checks for timeout conditions and returns a state update
// if a timeout transition occurs.
//   - StateInitializing + StartupTimeout exceeded → StateError
//   - StateProcessing + ProcessingTimeout exceeded → StateReady
//
// Otherwise returns the current state with Changed=false.
func (sm *TUIStateMachine) CheckTimeout(now time.Time) TUIStateUpdate {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	switch sm.currentState {
	case StateInitializing:
		if now.Sub(sm.createdAt) > sm.config.StartupTimeout {
			return sm.transition(StateError, "startup-timeout", map[string]string{
				"timeout": sm.config.StartupTimeout.String(),
			}, now)
		}
	case StateProcessing:
		if now.Sub(sm.lastTransition) > sm.config.ProcessingTimeout {
			return sm.transition(StateReady, "processing-timeout", map[string]string{
				"timeout": sm.config.ProcessingTimeout.String(),
			}, now)
		}
	}

	return TUIStateUpdate{
		From:      sm.currentState,
		To:        sm.currentState,
		State:     sm.currentState,
		StateName: tuiStateName(sm.currentState),
		Changed:   false,
		Timestamp: now,
	}
}

// State returns the current state (thread-safe).
func (sm *TUIStateMachine) State() TUIState {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.currentState
}

// StateName returns the human-readable name of the current state (thread-safe).
func (sm *TUIStateMachine) StateName() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return tuiStateName(sm.currentState)
}

// Reset returns the state machine to StateInitializing.
func (sm *TUIStateMachine) Reset() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.currentState = StateInitializing
	sm.createdAt = time.Now()
	sm.lastTransition = time.Time{}
}

// TUIStateName returns the human-readable name for a TUIState value.
func TUIStateName(state TUIState) string {
	return tuiStateName(state)
}

// tuiStateName maps a TUIState to its human-readable name.
func tuiStateName(state TUIState) string {
	switch state {
	case StateInitializing:
		return "Initializing"
	case StateReady:
		return "Ready"
	case StateProcessing:
		return "Processing"
	case StateResponding:
		return "Responding"
	case StateError:
		return "Error"
	case StateRateLimited:
		return "RateLimited"
	case StatePermissionPrompt:
		return "PermissionPrompt"
	default:
		return fmt.Sprintf("Unknown(%d)", int(state))
	}
}

// transition updates the state machine to a new state and returns a TUIStateUpdate.
func (sm *TUIStateMachine) transition(newState TUIState, pattern string, fields map[string]string, now time.Time) TUIStateUpdate {
	changed := sm.currentState != newState
	from := sm.currentState
	if changed {
		sm.currentState = newState
		sm.lastTransition = now
	}
	return TUIStateUpdate{
		From:      from,
		To:        newState,
		State:     newState,
		StateName: tuiStateName(newState),
		Pattern:   pattern,
		Fields:    fields,
		Changed:   changed,
		Timestamp: now,
	}
}

// matchPatterns checks a raw string against a slice of compiled patterns.
// Returns (true, patternName, fields) on the first match, or (false, "", nil).
func (sm *TUIStateMachine) matchPatterns(raw string, patterns []compiledPattern) (bool, string, map[string]string) {
	for _, p := range patterns {
		matches := p.re.FindStringSubmatch(raw)
		if matches == nil {
			continue
		}
		fields := extractNamedGroups(p.re, matches)
		return true, p.name, fields
	}
	return false, "", nil
}

// extractNamedGroups builds a map from named capture groups in a regex match.
func extractNamedGroups(re *regexp.Regexp, matches []string) map[string]string {
	if len(matches) == 0 {
		return nil
	}
	names := re.SubexpNames()
	fields := make(map[string]string)
	for i, name := range names {
		if i == 0 || name == "" {
			continue
		}
		if i < len(matches) {
			fields[name] = matches[i]
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// compilePatterns compiles a slice of regex strings into compiledPattern slices.
// Pattern names are formatted as "category-index" (e.g., "ready-0", "processing-2").
func compilePatterns(category string, patterns []string) ([]compiledPattern, error) {
	result := make([]compiledPattern, 0, len(patterns))
	for i, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("%s-%d: %w", category, i, err)
		}
		result = append(result, compiledPattern{
			name: fmt.Sprintf("%s-%d", category, i),
			re:   re,
		})
	}
	return result, nil
}
