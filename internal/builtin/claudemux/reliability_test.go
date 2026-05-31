package claudemux

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/joeycumines/one-shot-man/internal/builtin/claudemux/testutil"
)

// TestReliablePrompter_SimplePrompt tests the basic prompt flow with a
// ChannelHandle simulating protocol-mode output.
func TestReliablePrompter_SimplePrompt(t *testing.T) {

	h := testutil.NewChannelHandle()

	// Simulate ready + processing + response.
	go func() {
		// Ready signal (simulating system/init).
		time.Sleep(50 * time.Millisecond)
		h.WriteOutput(`{"type":"system","subtype":"init","session_id":"test"}`)
		// Wait for input.
		if input, ok := h.ReadInput(); ok && strings.Contains(input, "hello") {
			// Processing indicator.
			h.WriteOutput(`{"type":"assistant","subtype":"text","content":"· thinking..."}`)
			// Response.
			h.WriteOutput(`{"type":"assistant","subtype":"text","content":"Response to: hello"}`)
			// Completion.
			h.WriteOutput(`{"type":"result","subtype":"success","cost_usd":0,"duration_ms":100}`)
		}
	}()

	p := NewReliablePrompter(h, nil, PromptOpts{
		ReadyTimeout:    5 * time.Second,
		AcceptTimeout:   5 * time.Second,
		ResponseTimeout: 10 * time.Second,
		MaxRetries:      1,
	})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := p.SendPrompt(ctx, "hello")
	if err != nil {
		t.Fatalf("SendPrompt() error = %v", err)
	}
	if result.ResponseText == "" {
		t.Error("ResponseText is empty")
	}
	if len(result.Events) == 0 {
		t.Error("Events is empty")
	}
}

// TestReliablePrompter_RateLimitRecovery tests that the prompter retries
// after a rate limit event.
func TestReliablePrompter_RateLimitRecovery(t *testing.T) {

	h := testutil.NewChannelHandle()
	attempt := 0

	go func() {
		time.Sleep(50 * time.Millisecond)
		h.WriteOutput(`{"type":"system","subtype":"init","session_id":"test"}`)

		for {
			input, ok := h.ReadInput()
			if !ok {
				return
			}
			attempt++
			if attempt == 1 {
				// First attempt: rate limit.
				h.WriteOutput(`{"type":"rate_limit_event","subtype":"warning","message":"Rate limit exceeded","retryAfterMs":100}`)
			} else {
				// Second attempt: success.
				h.WriteOutput(`{"type":"assistant","subtype":"text","content":"Response to: ` + input + `"}`)
				h.WriteOutput(`{"type":"result","subtype":"success","cost_usd":0,"duration_ms":100}`)
			}
		}
	}()

	p := NewReliablePrompter(h, nil, PromptOpts{
		ReadyTimeout:    5 * time.Second,
		AcceptTimeout:   5 * time.Second,
		ResponseTimeout: 10 * time.Second,
		MaxRetries:      3,
	})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := p.SendPrompt(ctx, "hello")
	if err != nil {
		t.Fatalf("SendPrompt() error = %v", err)
	}
	if result.ResponseText == "" {
		t.Error("ResponseText is empty after rate limit recovery")
	}
}

// TestReliablePrompter_PermissionDenied tests that the prompter returns an
// error when a permission prompt is encountered with deny policy.
func TestReliablePrompter_PermissionDenied(t *testing.T) {

	h := testutil.NewChannelHandle()

	go func() {
		time.Sleep(50 * time.Millisecond)
		h.WriteOutput(`{"type":"system","subtype":"init","session_id":"test"}`)

		if _, ok := h.ReadInput(); ok {
			h.WriteOutput(`{"type":"control_request","subtype":"tool_call","tool":"Bash","args":{"command":"rm -rf /"},"id":"perm-001"}`)
		}
	}()

	p := NewReliablePrompter(h, nil, PromptOpts{
		ReadyTimeout:     5 * time.Second,
		AcceptTimeout:    5 * time.Second,
		ResponseTimeout:  10 * time.Second,
		MaxRetries:       1,
		PermissionPolicy: PermissionPolicyDeny,
	})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := p.SendPrompt(ctx, "do something dangerous")
	if err == nil {
		t.Fatal("expected error for permission denied, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %v, want error containing 'permission denied'", err)
	}
}

// TestReliablePrompter_PermissionAllowed tests that the prompter sends a
// control_response when a permission prompt is encountered with allow policy.
func TestReliablePrompter_PermissionAllowed(t *testing.T) {

	h := testutil.NewChannelHandle()

	go func() {
		time.Sleep(50 * time.Millisecond)
		h.WriteOutput(`{"type":"system","subtype":"init","session_id":"test"}`)

		if _, ok := h.ReadInput(); ok {
			h.WriteOutput(`{"type":"control_request","subtype":"tool_call","tool":"Bash","args":{"command":"ls"},"id":"perm-001"}`)
			// After receiving control_response, respond.
			if input, ok := h.ReadInput(); ok {
				if strings.Contains(input, "control_response") {
					h.WriteOutput(`{"type":"assistant","subtype":"text","content":"Bash output here"}`)
					h.WriteOutput(`{"type":"result","subtype":"success","cost_usd":0,"duration_ms":50}`)
				}
			}
		}
	}()

	p := NewReliablePrompter(h, nil, PromptOpts{
		ReadyTimeout:     5 * time.Second,
		AcceptTimeout:    5 * time.Second,
		ResponseTimeout:  10 * time.Second,
		MaxRetries:       1,
		PermissionPolicy: PermissionPolicyAllow,
	})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := p.SendPrompt(ctx, "list files")
	if err != nil {
		t.Fatalf("SendPrompt() error = %v", err)
	}
	if result.ResponseText == "" {
		t.Error("ResponseText is empty after permission allowed")
	}
}

// TestReliablePrompter_ContextCancellation tests that the prompter respects
// context cancellation.
func TestReliablePrompter_ContextCancellation(t *testing.T) {

	h := testutil.NewChannelHandle()

	// Never write any output — the prompter will time out on WaitReady.
	p := NewReliablePrompter(h, nil, PromptOpts{
		ReadyTimeout:    100 * time.Millisecond,
		AcceptTimeout:   100 * time.Millisecond,
		ResponseTimeout: 100 * time.Millisecond,
		MaxRetries:      0,
	})
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := p.SendPrompt(ctx, "hello")
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "wait ready") {
		t.Errorf("error = %v, want context canceled or wait ready error", err)
	}
}

// TestReliablePrompter_AgentError tests that the prompter returns an error
// when the agent reports an error event.
func TestReliablePrompter_AgentError(t *testing.T) {

	h := testutil.NewChannelHandle()

	go func() {
		time.Sleep(50 * time.Millisecond)
		h.WriteOutput(`{"type":"system","subtype":"init","session_id":"test"}`)

		if _, ok := h.ReadInput(); ok {
			h.WriteOutput(`{"type":"assistant","subtype":"text","content":"Error: something went wrong"}`)
			h.WriteOutput(`{"type":"result","subtype":"error","error":"something went wrong"}`)
		}
	}()

	p := NewReliablePrompter(h, nil, PromptOpts{
		ReadyTimeout:    5 * time.Second,
		AcceptTimeout:   5 * time.Second,
		ResponseTimeout: 10 * time.Second,
		MaxRetries:      0,
	})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := p.SendPrompt(ctx, "do something")
	if err == nil {
		t.Fatal("expected error from agent error event, got nil")
	}
	if !strings.Contains(err.Error(), "agent error") {
		t.Errorf("error = %v, want error containing 'agent error'", err)
	}
}

// TestReliablePrompter_TUIMode tests the prompter with a VTStateDetector
// for TUI-mode state detection using a ChannelHandle.
func TestReliablePrompter_TUIMode(t *testing.T) {

	h := testutil.NewChannelHandle()
	detector, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}

	go func() {
		h.WriteOutput("Claude Code v1.0.0\n❯ ")

		if _, ok := h.ReadInput(); ok {
			h.WriteOutput("· thinking\n")
			h.WriteOutput("Response text\n❯ ")
		}
	}()

	p := NewReliablePrompter(h, nil, PromptOpts{
		Detector:        detector,
		ReadyTimeout:    5 * time.Second,
		AcceptTimeout:   5 * time.Second,
		ResponseTimeout: 10 * time.Second,
		MaxRetries:      1,
	})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := p.SendPrompt(ctx, "hello")
	if err != nil {
		t.Fatalf("SendPrompt() error = %v", err)
	}
	if result.ResponseText == "" {
		t.Error("ResponseText is empty")
	}
	if len(result.StateTransitions) == 0 {
		t.Error("StateTransitions is empty")
	}

	hasInitToReady := false
	hasReadyToProcessing := false
	hasCompletionToReady := false
	for _, tr := range result.StateTransitions {
		if tr.From == StateInitializing && tr.To == StateReady {
			hasInitToReady = true
		}
		if tr.From == StateReady && tr.To == StateProcessing {
			hasReadyToProcessing = true
		}
		if tr.To == StateReady && (tr.From == StateProcessing || tr.From == StateResponding) {
			hasCompletionToReady = true
		}
	}
	if !hasInitToReady {
		t.Error("expected Initializing→Ready transition")
	}
	if !hasReadyToProcessing {
		t.Error("expected Ready→Processing transition")
	}
	if !hasCompletionToReady {
		t.Error("expected Processing/Responding→Ready transition (completion signal)")
	}
}

// TestReliablePrompter_StateTransitions verifies that state transitions
// are correctly recorded during the prompt lifecycle.
func TestReliablePrompter_StateTransitions(t *testing.T) {

	h := testutil.NewChannelHandle()

	go func() {
		time.Sleep(50 * time.Millisecond)
		h.WriteOutput(`{"type":"system","subtype":"init","session_id":"test"}`)

		if _, ok := h.ReadInput(); ok {
			h.WriteOutput(`{"type":"assistant","subtype":"text","thinking":true,"content":"thinking..."}`)
			h.WriteOutput(`{"type":"assistant","subtype":"text","content":"The answer is 42"}`)
			h.WriteOutput(`{"type":"result","subtype":"success","cost_usd":0,"duration_ms":500}`)
		}
	}()

	p := NewReliablePrompter(h, nil, PromptOpts{
		ReadyTimeout:    5 * time.Second,
		AcceptTimeout:   5 * time.Second,
		ResponseTimeout: 10 * time.Second,
		MaxRetries:      1,
	})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := p.SendPrompt(ctx, "what is the answer")
	if err != nil {
		t.Fatalf("SendPrompt() error = %v", err)
	}

	if len(result.StateTransitions) == 0 {
		t.Error("StateTransitions is empty")
	}

	// Should have at least one transition: Ready → Processing or Processing → Ready.
	hasProcessing := false
	for _, tr := range result.StateTransitions {
		if tr.To == StateProcessing || tr.From == StateProcessing {
			hasProcessing = true
		}
	}
	if !hasProcessing {
		t.Errorf("expected at least one StateProcessing transition, got %v", result.StateTransitions)
	}
}

// TestReliablePrompter_MaxRetriesExceeded tests that the prompter returns an
// error after exceeding MaxRetries on recoverable errors.
func TestReliablePrompter_MaxRetriesExceeded(t *testing.T) {

	h := testutil.NewChannelHandle()
	callCount := 0

	go func() {
		time.Sleep(50 * time.Millisecond)
		h.WriteOutput(`{"type":"system","subtype":"init","session_id":"test"}`)

		for {
			if _, ok := h.ReadInput(); !ok {
				return
			}
			callCount++
			// Always rate limit — never succeed.
			h.WriteOutput(`{"type":"rate_limit_event","subtype":"warning","message":"Rate limit exceeded","retryAfterMs":50}`)
		}
	}()

	p := NewReliablePrompter(h, nil, PromptOpts{
		ReadyTimeout:    5 * time.Second,
		AcceptTimeout:   2 * time.Second,
		ResponseTimeout: 2 * time.Second,
		MaxRetries:      2,
	})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := p.SendPrompt(ctx, "hello")
	if err == nil {
		t.Fatal("expected error after max retries, got nil")
	}
	if !strings.Contains(err.Error(), "retries") {
		t.Errorf("error = %v, want error mentioning retries", err)
	}
}

// chunkTrackingWriter records each chunk written for per-chunk validation.
type chunkTrackingWriter struct {
	chunks []string
}

func (w *chunkTrackingWriter) Write(p []byte) (int, error) {
	w.chunks = append(w.chunks, string(p))
	return len(p), nil
}

func TestChunkPrompt_SmallInput(t *testing.T) {
	var w chunkTrackingWriter
	input := "hello world"

	err := chunkPrompt(&w, input, 4096, 0)
	if err != nil {
		t.Fatalf("chunkPrompt: %v", err)
	}
	if got := strings.Join(w.chunks, ""); got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestChunkPrompt_LargeInput(t *testing.T) {
	var w chunkTrackingWriter
	input := strings.Repeat("a", 10000)

	err := chunkPrompt(&w, input, 4096, 0)
	if err != nil {
		t.Fatalf("chunkPrompt: %v", err)
	}
	if got := strings.Join(w.chunks, ""); got != input {
		t.Errorf("output length = %d, want %d", len(got), len(input))
	}
}

func TestChunkPrompt_UTF8Boundary(t *testing.T) {
	var w chunkTrackingWriter

	// Each '日' is 3 bytes. 1365 * 3 = 4095 bytes, so byte 4095 is the
	// first byte of the 1366th '日' — the 4096 boundary falls mid-rune.
	segment := strings.Repeat("日", 1366)
	input := segment + "tail"

	err := chunkPrompt(&w, input, 4096, 0)
	if err != nil {
		t.Fatalf("chunkPrompt: %v", err)
	}

	got := strings.Join(w.chunks, "")
	if got != input {
		t.Errorf("output does not match input")
	}
	if !utf8.ValidString(got) {
		t.Error("reassembled output is not valid UTF-8")
	}
	for i, chunk := range w.chunks {
		if !utf8.ValidString(chunk) {
			t.Errorf("chunk %d (len=%d) is not valid UTF-8", i, len(chunk))
		}
	}
}

func TestChunkPrompt_ExactBoundary(t *testing.T) {
	var w chunkTrackingWriter
	input := strings.Repeat("x", 4096)

	err := chunkPrompt(&w, input, 4096, 0)
	if err != nil {
		t.Fatalf("chunkPrompt: %v", err)
	}
	if got := strings.Join(w.chunks, ""); got != input {
		t.Errorf("output length = %d, want %d", len(got), len(input))
	}
	if len(w.chunks) != 1 {
		t.Errorf("got %d chunks, want 1", len(w.chunks))
	}
}
