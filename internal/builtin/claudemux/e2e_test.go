package claudemux

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/claudemux/testutil"
)

// e2eProvider returns a MockClaudeProvider configured for fast E2E tests.
func e2eProvider() *MockClaudeProvider {
	return &MockClaudeProvider{ProcessingMs: 50}
}

// e2ePromptOpts returns reasonable PromptOpts for E2E tests.
func e2ePromptOpts() PromptOpts {
	return PromptOpts{
		ReadyTimeout:    10 * time.Second,
		AcceptTimeout:   5 * time.Second,
		ResponseTimeout: 15 * time.Second,
		MaxRetries:      3,
	}
}

// spawnE2E spawns a mockclaude agent via MockClaudeProvider and returns
// the handle and provider. The caller must defer handle.Close().
func spawnE2E(t *testing.T) (AgentHandle, *MockClaudeProvider) {
	t.Helper()
	prov := e2eProvider()
	handle, err := prov.Spawn(context.Background(), SpawnOpts{Mode: ModeProtocol})
	if err != nil {
		t.Fatalf("MockClaudeProvider.Spawn: %v", err)
	}
	return handle, prov
}

func TestE2E_SimplePrompt(t *testing.T) {
	handle, _ := spawnE2E(t)
	defer handle.Close()

	p := NewReliablePrompter(handle, nil, e2ePromptOpts())
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := p.SendPrompt(ctx, "hello")
	if err != nil {
		t.Fatalf("SendPrompt() error = %v", err)
	}

	if !strings.Contains(result.ResponseText, "hello") {
		t.Errorf("ResponseText = %q, want containing 'hello'", result.ResponseText)
	}

	// Verify Events include system-init. The result-success event may not
	// appear in Events because the Processing→Ready state transition
	// (derived from result-success) triggers the return before that
	// promptEvent is consumed from the channel.
	hasInit := false
	for _, e := range result.Events {
		if e.Type == EventCompletion && e.Pattern == "system-init" {
			hasInit = true
		}
	}
	if !hasInit {
		t.Error("expected system-init event in Events")
	}

	// Verify completion: must see Processing→Ready state transition
	// (result-success triggers this). Single-channel delivery guarantees
	// the state transition is always observed before SendPrompt returns.
	hasProcessingToReady := false
	hasInitToReady := false
	for _, tr := range result.StateTransitions {
		if tr.From == StateProcessing && tr.To == StateReady {
			hasProcessingToReady = true
		}
		if tr.From == StateInitializing && tr.To == StateReady {
			hasInitToReady = true
		}
	}
	if !hasProcessingToReady {
		t.Errorf("expected Processing→Ready transition (completion), got %v", result.StateTransitions)
	}
	if !hasInitToReady {
		t.Errorf("expected Initializing→Ready transition, got %v", result.StateTransitions)
	}
}

func TestE2E_MultiTurnConversation(t *testing.T) {
	handle, _ := spawnE2E(t)
	defer handle.Close()

	p := NewReliablePrompter(handle, nil, e2ePromptOpts())
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result1, err := p.SendPrompt(ctx, "first question")
	if err != nil {
		t.Fatalf("SendPrompt turn 1 error = %v", err)
	}
	if !strings.Contains(result1.ResponseText, "first question") {
		t.Errorf("turn 1 ResponseText = %q, want containing 'first question'", result1.ResponseText)
	}

	result2, err := p.SendPrompt(ctx, "second question")
	if err != nil {
		t.Fatalf("SendPrompt turn 2 error = %v", err)
	}
	if !strings.Contains(result2.ResponseText, "second question") {
		t.Errorf("turn 2 ResponseText = %q, want containing 'second question'", result2.ResponseText)
	}
}

func TestE2E_RateLimitRecovery(t *testing.T) {
	handle, _ := spawnE2E(t)
	defer handle.Close()

	opts := e2ePromptOpts()
	opts.MaxRetries = 2

	p := NewReliablePrompter(handle, nil, opts)
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// MOCK_RATE_LIMIT causes the mock to emit a rate_limit_event on every
	// attempt. Since the ReliablePrompter resends the same prompt on retry,
	// each retry also hits the rate limit. Verify that the retry mechanism
	// is invoked and eventually reports max retries exceeded.
	_, err := p.SendPrompt(ctx, "MOCK_RATE_LIMIT:test")
	if err == nil {
		t.Fatal("expected error after rate limit retries exhausted, got nil")
	}
	if !strings.Contains(err.Error(), "retries") {
		t.Errorf("error = %v, want error mentioning retries", err)
	}
}

func TestE2E_PermissionAllowed(t *testing.T) {
	handle, _ := spawnE2E(t)
	defer handle.Close()

	opts := e2ePromptOpts()
	opts.PermissionPolicy = PermissionPolicyAllow

	p := NewReliablePrompter(handle, nil, opts)
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := p.SendPrompt(ctx, "MOCK_PERMISSION:test")
	if err != nil {
		t.Fatalf("SendPrompt() error = %v", err)
	}

	if !strings.Contains(result.ResponseText, "Permission granted") {
		t.Errorf("ResponseText = %q, want containing 'Permission granted'", result.ResponseText)
	}

	hasPerm := false
	for _, e := range result.Events {
		if e.Type == EventPermission {
			hasPerm = true
		}
	}
	if !hasPerm {
		t.Error("expected EventPermission in Events")
	}
}

func TestE2E_PermissionDenied(t *testing.T) {
	handle, _ := spawnE2E(t)
	defer handle.Close()

	opts := e2ePromptOpts()
	opts.PermissionPolicy = PermissionPolicyDeny

	p := NewReliablePrompter(handle, nil, opts)
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := p.SendPrompt(ctx, "MOCK_PERMISSION:test")
	if err == nil {
		t.Fatal("expected error for permission denied, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %v, want error containing 'permission denied'", err)
	}
}

func TestE2E_ErrorResult(t *testing.T) {
	handle, _ := spawnE2E(t)
	defer handle.Close()

	p := NewReliablePrompter(handle, nil, e2ePromptOpts())
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := p.SendPrompt(ctx, "MOCK_ERROR:test")
	if err == nil {
		t.Fatal("expected error from MOCK_ERROR, got nil")
	}
	if !strings.Contains(err.Error(), "agent error") {
		t.Errorf("error = %v, want error containing 'agent error'", err)
	}
}

func TestE2E_CrashAndRecovery(t *testing.T) {
	prov := e2eProvider()
	handle, err := prov.Spawn(context.Background(), SpawnOpts{Mode: ModeProtocol})
	if err != nil {
		t.Fatalf("MockClaudeProvider.Spawn: %v", err)
	}

	opts := e2ePromptOpts()
	opts.MaxRetries = 3

	// Pass provider so ReliablePrompter can re-spawn after crash.
	p := NewReliablePrompter(handle, prov, opts)
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// MOCK_CRASH causes the subprocess to exit with code 1.
	_, err = p.SendPrompt(ctx, "MOCK_CRASH:")
	if err == nil {
		t.Fatal("expected error from MOCK_CRASH, got nil")
	}

	// Reap the crashed process so IsAlive() correctly returns false.
	// Without this, ProcessState is nil and IsAlive() incorrectly
	// reports true, preventing the recovery path from triggering.
	handle.Wait()

	// The ReliablePrompter should now detect the dead handle on the
	// next SendPrompt, recover via the provider, and succeed.
	result, err := p.SendPrompt(ctx, "hello after crash")
	if err != nil {
		t.Fatalf("SendPrompt after recovery error = %v", err)
	}
	if !strings.Contains(result.ResponseText, "hello after crash") {
		t.Errorf("post-recovery ResponseText = %q, want containing 'hello after crash'", result.ResponseText)
	}
}

func TestE2E_PrintMode(t *testing.T) {
	testutil.SkipSlow(t)

	prov := e2eProvider()
	handle, err := prov.Spawn(context.Background(), SpawnOpts{Mode: ModePrint})
	if err != nil {
		t.Fatalf("MockClaudeProvider.Spawn ModePrint: %v", err)
	}
	defer handle.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := handle.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	if err := handle.Send("hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	events := collectEvents(t, handle, 10*time.Second, func(events []OutputEvent) bool {
		return hasResultSuccess(events)
	})

	if !hasContentContaining(events, "hello") {
		t.Errorf("expected output containing 'hello', got %d events", len(events))
	}
}
