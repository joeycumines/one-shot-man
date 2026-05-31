package claudemux

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWaitSettle_ReadyState(t *testing.T) {
	det, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector: %v", err)
	}

	det.ProcessRaw([]byte("MockAgent ready.\r\n❯ \r\n"), time.Now())
	if det.State() != StateReady {
		t.Fatalf("State() = %s, want Ready", tuiStateName(det.State()))
	}

	cfg := SettleConfig{
		StableDuration: 100 * time.Millisecond,
		PollInterval:   20 * time.Millisecond,
		TargetState:    StateReady,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	settled, _, err := WaitSettle(ctx, &idleReader{}, det, cfg)
	if err != nil {
		t.Fatalf("WaitSettle: %v", err)
	}
	if settled != StateReady {
		t.Errorf("settled state = %s, want Ready", tuiStateName(settled))
	}
}

func TestWaitSettle_BlockingReader(t *testing.T) {
	det, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector: %v", err)
	}

	det.ProcessRaw([]byte("MockAgent ready.\r\n❯ \r\n"), time.Now())
	if det.State() != StateReady {
		t.Fatalf("State() = %s, want Ready", tuiStateName(det.State()))
	}

	cfg := SettleConfig{
		StableDuration: 100 * time.Millisecond,
		PollInterval:   20 * time.Millisecond,
		TargetState:    StateReady,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// blockingReader blocks on Read() until Unblock, simulating an idle PTY.
	reader := &blockingReader{}
	t.Cleanup(reader.Unblock)

	settled, _, err := WaitSettle(ctx, reader, det, cfg)
	if err != nil {
		t.Fatalf("WaitSettle: %v", err)
	}
	if settled != StateReady {
		t.Errorf("settled state = %s, want Ready", tuiStateName(settled))
	}
}

func TestWaitSettle_ContextCancellation(t *testing.T) {
	det, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err = WaitSettle(ctx, &idleReader{}, det, DefaultSettleConfig())
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want wrapping context.Canceled", err)
	}
}

type idleReader struct{}

func (r *idleReader) Read() (string, error) { return "", nil }

type blockingReader struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

func (r *blockingReader) Read() (string, error) {
	r.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.mu.Unlock()

	<-ctx.Done()
	return "", ctx.Err()
}

func (r *blockingReader) Unblock() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
}
