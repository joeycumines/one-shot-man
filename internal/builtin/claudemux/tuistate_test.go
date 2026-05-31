package claudemux

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewTUIStateMachine(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	assert.NotNil(t, sm)
	assert.Equal(t, StateInitializing, sm.State())
	assert.Equal(t, "Initializing", sm.StateName())
}

func TestNewTUIStateMachine_InvalidPattern(t *testing.T) {
	cfg := DefaultClaudeCodeTUIStateConfig()
	cfg.ErrorPatterns = append(cfg.ErrorPatterns, "([invalid")
	_, err := NewTUIStateMachine(cfg)
	assert.Error(t, err)
}

func TestProcessOutput_ReadyDetection(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("❯ ", now)
	assert.Equal(t, StateReady, update.State)
	assert.True(t, update.Changed)
	assert.Equal(t, "Ready", update.StateName)
	assert.NotEmpty(t, update.Pattern)
}

func TestProcessOutput_ProcessingOverridesReady(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("❯ ", now)
	assert.Equal(t, StateReady, update.State)
	assert.True(t, update.Changed)

	update = sm.ProcessOutput("· thinking", now)
	assert.Equal(t, StateProcessing, update.State)
	assert.True(t, update.Changed)
}

func TestProcessOutput_ErrorDetection(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("Error: something went wrong", now)
	assert.Equal(t, StateError, update.State)
	assert.True(t, update.Changed)
	assert.Equal(t, "Error", update.StateName)
}

func TestProcessOutput_RateLimitDetection(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("Rate limit exceeded", now)
	assert.Equal(t, StateRateLimited, update.State)
	assert.True(t, update.Changed)
	assert.Equal(t, "RateLimited", update.StateName)
}

func TestProcessOutput_PermissionDetection(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("Allow Bash", now)
	assert.Equal(t, StatePermissionPrompt, update.State)
	assert.True(t, update.Changed)
	assert.Equal(t, "PermissionPrompt", update.StateName)
}

func TestProcessOutput_ProcessingOverridesReadyBothPresent(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	sm.ProcessOutput("❯ ", now)
	assert.Equal(t, StateReady, sm.State())

	update := sm.ProcessOutput("Running…", now)
	assert.Equal(t, StateProcessing, update.State)
	assert.True(t, update.Changed)

	customConfig := TUIStateMachineConfig{
		ReadyPatterns:      []string{`prompt`},
		ProcessingPatterns: []string{`prompt.*thinking`},
	}
	sm2, err := NewTUIStateMachine(customConfig)
	assert.NoError(t, err)

	update = sm2.ProcessOutput("prompt thinking", now)
	assert.Equal(t, StateProcessing, update.State)
	assert.True(t, update.Changed)
}

func TestProcessOutput_NoChangeOnSameState(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("❯ ", now)
	assert.Equal(t, StateReady, update.State)
	assert.True(t, update.Changed)

	update = sm.ProcessOutput("❯ ", now)
	assert.Equal(t, StateReady, update.State)
	assert.False(t, update.Changed)
}

func TestProcessOutput_RespondingAfterProcessing(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	sm.ProcessOutput("· thinking", now)
	assert.Equal(t, StateProcessing, sm.State())

	update := sm.ProcessOutput("Here is some output text", now)
	assert.Equal(t, StateResponding, update.State)
	assert.True(t, update.Changed)
}

func TestProcessOutput_TransitionFromProcessingToReady(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	sm.ProcessOutput("· thinking", now)
	assert.Equal(t, StateProcessing, sm.State())

	update := sm.ProcessOutput("❯ ", now)
	assert.Equal(t, StateReady, update.State)
	assert.True(t, update.Changed)
}

func TestCheckTimeout_StartupTimeout(t *testing.T) {
	config := DefaultClaudeCodeTUIStateConfig()
	config.StartupTimeout = 5 * time.Millisecond
	sm, err := NewTUIStateMachine(config)
	assert.NoError(t, err)

	sm.ProcessOutput("some random output", time.Now())
	assert.Equal(t, StateInitializing, sm.State())

	time.Sleep(10 * time.Millisecond)

	update := sm.CheckTimeout(time.Now())
	assert.Equal(t, StateError, update.State)
	assert.True(t, update.Changed)
	assert.Equal(t, "startup-timeout", update.Pattern)
}

func TestCheckTimeout_ProcessingTimeout(t *testing.T) {
	config := DefaultClaudeCodeTUIStateConfig()
	config.ProcessingTimeout = 10 * time.Second
	sm, err := NewTUIStateMachine(config)
	assert.NoError(t, err)

	startTime := time.Now()
	sm.ProcessOutput("· thinking", startTime)
	assert.Equal(t, StateProcessing, sm.State())

	update := sm.CheckTimeout(startTime.Add(5 * time.Second))
	assert.Equal(t, StateProcessing, update.State)
	assert.False(t, update.Changed)

	update = sm.CheckTimeout(startTime.Add(11 * time.Second))
	assert.Equal(t, StateReady, update.State)
	assert.True(t, update.Changed)
	assert.Equal(t, "processing-timeout", update.Pattern)
}

func TestCheckTimeout_NoTimeoutWhenReady(t *testing.T) {
	config := DefaultClaudeCodeTUIStateConfig()
	sm, err := NewTUIStateMachine(config)
	assert.NoError(t, err)

	startTime := time.Now()
	sm.ProcessOutput("❯ ", startTime)
	assert.Equal(t, StateReady, sm.State())

	update := sm.CheckTimeout(startTime.Add(300 * time.Second))
	assert.Equal(t, StateReady, update.State)
	assert.False(t, update.Changed)
}

func TestCheckTimeout_NoTimeoutWhenNoOutput(t *testing.T) {
	config := DefaultClaudeCodeTUIStateConfig()
	config.StartupTimeout = 1 * time.Hour
	sm, err := NewTUIStateMachine(config)
	assert.NoError(t, err)

	update := sm.CheckTimeout(time.Now())
	assert.Equal(t, StateInitializing, update.State)
	assert.False(t, update.Changed)
}

func TestTUIStateName(t *testing.T) {
	tests := []struct {
		state    TUIState
		expected string
	}{
		{StateInitializing, "Initializing"},
		{StateReady, "Ready"},
		{StateProcessing, "Processing"},
		{StateResponding, "Responding"},
		{StateError, "Error"},
		{StateRateLimited, "RateLimited"},
		{StatePermissionPrompt, "PermissionPrompt"},
		{TUIState(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tuiStateName(tt.state))
		assert.Equal(t, tt.expected, TUIStateName(tt.state))
	}
}

func TestDefaultClaudeCodeTUIStateConfig(t *testing.T) {
	config := DefaultClaudeCodeTUIStateConfig()

	assert.Len(t, config.ReadyPatterns, 2)
	assert.Len(t, config.ProcessingPatterns, 6)
	assert.Len(t, config.ErrorPatterns, 2)
	assert.Len(t, config.RateLimitPatterns, 2)
	assert.Len(t, config.PermissionPatterns, 2)
	assert.Equal(t, 30*time.Second, config.StartupTimeout)
	assert.Equal(t, 120*time.Second, config.ProcessingTimeout)

	assert.Contains(t, config.ReadyPatterns, `^❯\s*$`)
	assert.Contains(t, config.ReadyPatterns, `^>\s*$`)
	assert.Contains(t, config.ProcessingPatterns, `·\s*thinking`)
	assert.Contains(t, config.ErrorPatterns, `(?i)^error:`)
	assert.Contains(t, config.RateLimitPatterns, `(?i)429`)
	assert.Contains(t, config.PermissionPatterns, `(?i)allow\s+`)
}

func TestProcessOutput_ErrorOverridesAll(t *testing.T) {
	config := TUIStateMachineConfig{
		ErrorPatterns:      []string{`(?i)error:`},
		RateLimitPatterns:  []string{`rate`},
		PermissionPatterns: []string{`allow`},
		ProcessingPatterns: []string{`thinking`},
		ReadyPatterns:      []string{`prompt`},
	}
	sm, err := NewTUIStateMachine(config)
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("error: rate allow thinking prompt", now)
	assert.Equal(t, StateError, update.State)
	assert.True(t, update.Changed)
}

func TestProcessOutput_RateLimitOverridesPermissionAndLower(t *testing.T) {
	config := TUIStateMachineConfig{
		ErrorPatterns:      []string{`nevermatch`},
		RateLimitPatterns:  []string{`rate`},
		PermissionPatterns: []string{`allow`},
		ProcessingPatterns: []string{`thinking`},
		ReadyPatterns:      []string{`prompt`},
	}
	sm, err := NewTUIStateMachine(config)
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("rate allow thinking prompt", now)
	assert.Equal(t, StateRateLimited, update.State)
}

func TestProcessOutput_PermissionOverridesProcessingAndReady(t *testing.T) {
	config := TUIStateMachineConfig{
		ErrorPatterns:      []string{`nevermatch`},
		RateLimitPatterns:  []string{`nevermatch`},
		PermissionPatterns: []string{`allow`},
		ProcessingPatterns: []string{`thinking`},
		ReadyPatterns:      []string{`prompt`},
	}
	sm, err := NewTUIStateMachine(config)
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("allow thinking prompt", now)
	assert.Equal(t, StatePermissionPrompt, update.State)
}

func TestProcessOutput_FatalError(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("Fatal: crash", now)
	assert.Equal(t, StateError, update.State)
	assert.True(t, update.Changed)
}

func TestProcessOutput_429RateLimit(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("Got 429 response", now)
	assert.Equal(t, StateRateLimited, update.State)
}

func TestProcessOutput_PermitPattern(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("Permit this action", now)
	assert.Equal(t, StatePermissionPrompt, update.State)
}

func TestProcessOutput_ProcessingPatterns(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"thinking", "· thinking"},
		{"ellipsis-duration", "… (30s)"},
		{"running", "Running…"},
		{"bash", "Bash(some command)"},
		{"edit", "Edit(file.go)"},
		{"read", "Read(file.go)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
			assert.NoError(t, err)
			now := time.Now()
			update := sm.ProcessOutput(tt.input, now)
			assert.Equal(t, StateProcessing, update.State, "input: %q", tt.input)
			assert.True(t, update.Changed)
		})
	}
}

func TestProcessOutput_RespondingNotFromProcessing(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	sm.ProcessOutput("❯ ", now)
	assert.Equal(t, StateReady, sm.State())

	update := sm.ProcessOutput("some random text", now)
	assert.Equal(t, StateReady, update.State)
	assert.False(t, update.Changed)
}

func TestProcessOutput_NamedGroupExtraction(t *testing.T) {
	config := TUIStateMachineConfig{
		ProcessingPatterns: []string{`(?P<tool>\w+)\(.*?\)`},
	}
	sm, err := NewTUIStateMachine(config)
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("Bash(ls -la)", now)
	assert.Equal(t, StateProcessing, update.State)
	assert.NotNil(t, update.Fields)
	assert.Equal(t, "Bash", update.Fields["tool"])
}

func TestProcessOutput_Timestamp(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("❯ ", now)
	assert.Equal(t, now, update.Timestamp)
}

func TestProcessOutput_NoMatch(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("some random output", now)
	assert.Equal(t, StateInitializing, update.State)
	assert.False(t, update.Changed)
	assert.Empty(t, update.Pattern)
}

func TestProcessOutput_ReadyWithAngleBracket(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)
	now := time.Now()

	update := sm.ProcessOutput("> ", now)
	assert.Equal(t, StateReady, update.State)
	assert.True(t, update.Changed)
}

func TestReset(t *testing.T) {
	sm, err := NewTUIStateMachine(DefaultClaudeCodeTUIStateConfig())
	assert.NoError(t, err)

	sm.ProcessOutput("❯ ", time.Now())
	assert.Equal(t, StateReady, sm.State())

	sm.Reset()
	assert.Equal(t, StateInitializing, sm.State())
	assert.Equal(t, "Initializing", sm.StateName())
}
