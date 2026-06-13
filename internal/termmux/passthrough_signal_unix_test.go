//go:build !windows

package termmux

import (
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestWatchSignals_SIGTSTP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	var sigtstpReceived atomic.Int32
	signalChild := func(sig string) error {
		if sig == "SIGTSTP" {
			sigtstpReceived.Add(1)
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan signalResult, 1)
	sigCancel := watchSignals(ctx, resultCh, signalChild)
	defer sigCancel()

	time.Sleep(100 * time.Millisecond)

	syscall.Kill(syscall.Getpid(), syscall.SIGTSTP)

	select {
	case sr := <-resultCh:
		if sr.reason != ExitSuspended {
			t.Errorf("reason = %v, want ExitSuspended", sr.reason)
		}
		if sr.err != nil {
			t.Errorf("err = %v, want nil", sr.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SIGTSTP result")
	}

	if sigtstpReceived.Load() < 1 {
		t.Errorf("SIGTSTP forwarded %d times, want >= 1", sigtstpReceived.Load())
	}
}

func TestWatchSignals_SIGINT(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	var sigintReceived atomic.Int32
	signalChild := func(sig string) error {
		if sig == "SIGINT" {
			sigintReceived.Add(1)
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan signalResult, 1)
	sigCancel := watchSignals(ctx, resultCh, signalChild)
	defer sigCancel()

	time.Sleep(100 * time.Millisecond)

	syscall.Kill(syscall.Getpid(), syscall.SIGINT)

	time.Sleep(300 * time.Millisecond)

	if sigintReceived.Load() < 1 {
		t.Errorf("SIGINT forwarded %d times, want >= 1", sigintReceived.Load())
	}

	select {
	case sr := <-resultCh:
		t.Errorf("unexpected result from SIGINT: %v", sr)
	default:
	}
}

func TestWatchSignals_SIGQUIT(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	var sigquitReceived atomic.Int32
	signalChild := func(sig string) error {
		if sig == "SIGQUIT" {
			sigquitReceived.Add(1)
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan signalResult, 1)
	sigCancel := watchSignals(ctx, resultCh, signalChild)
	defer sigCancel()

	time.Sleep(100 * time.Millisecond)

	syscall.Kill(syscall.Getpid(), syscall.SIGQUIT)

	time.Sleep(300 * time.Millisecond)

	if sigquitReceived.Load() < 1 {
		t.Errorf("SIGQUIT forwarded %d times, want >= 1", sigquitReceived.Load())
	}

	select {
	case sr := <-resultCh:
		t.Errorf("unexpected result from SIGQUIT: %v", sr)
	default:
	}
}

func TestWatchSignals_Cancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	var sigtstpReceived atomic.Int32
	signalChild := func(sig string) error {
		if sig == "SIGTSTP" {
			sigtstpReceived.Add(1)
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan signalResult, 1)
	sigCancel := watchSignals(ctx, resultCh, signalChild)

	time.Sleep(100 * time.Millisecond)

	sigCancel()
	cancel()

	select {
	case sr := <-resultCh:
		t.Errorf("unexpected result after cancel: %v", sr)
	default:
	}
}

func TestWatchSignals_NilSignalChild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan signalResult, 1)
	sigCancel := watchSignals(ctx, resultCh, nil)
	defer sigCancel()

	time.Sleep(100 * time.Millisecond)

	syscall.Kill(syscall.Getpid(), syscall.SIGTSTP)

	select {
	case sr := <-resultCh:
		if sr.reason != ExitSuspended {
			t.Errorf("reason = %v, want ExitSuspended", sr.reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SIGTSTP result with nil SignalChild")
	}
}

func TestSignalName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sig  os.Signal
		want string
	}{
		{syscall.SIGINT, "SIGINT"},
		{syscall.SIGQUIT, "SIGQUIT"},
		{syscall.SIGTSTP, "SIGTSTP"},
		{syscall.SIGTERM, "terminated"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := signalName(tt.sig)
			if got != tt.want {
				t.Errorf("signalName(%v) = %q, want %q", tt.sig, got, tt.want)
			}
		})
	}
}
