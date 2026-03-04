package claudemux

import (
	"context"
	"sync"
	"testing"
)

// --- Name helpers ---

func TestSupervisorStateName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state SupervisorState
		want  string
	}{
		{SupervisorIdle, "Idle"},
		{SupervisorRunning, "Running"},
		{SupervisorRecovering, "Recovering"},
		{SupervisorDraining, "Draining"},
		{SupervisorStopped, "Stopped"},
		{SupervisorState(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := SupervisorStateName(tt.state); got != tt.want {
			t.Errorf("SupervisorStateName(%d) = %q, want %q", int(tt.state), got, tt.want)
		}
	}
}

func TestErrorClassName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		class ErrorClass
		want  string
	}{
		{ErrorClassNone, "None"},
		{ErrorClassPTYEOF, "PTY-EOF"},
		{ErrorClassPTYCrash, "PTY-Crash"},
		{ErrorClassPTYError, "PTY-Error"},
		{ErrorClassMCPTimeout, "MCP-Timeout"},
		{ErrorClassMCPMalformed, "MCP-Malformed"},
		{ErrorClassCancelled, "Cancelled"},
		{ErrorClass(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := ErrorClassName(tt.class); got != tt.want {
			t.Errorf("ErrorClassName(%d) = %q, want %q", int(tt.class), got, tt.want)
		}
	}
}

func TestRecoveryActionName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		action RecoveryAction
		want   string
	}{
		{RecoveryNone, "None"},
		{RecoveryRetry, "Retry"},
		{RecoveryRestart, "Restart"},
		{RecoveryForceKill, "ForceKill"},
		{RecoveryEscalate, "Escalate"},
		{RecoveryAbort, "Abort"},
		{RecoveryDrain, "Drain"},
		{RecoveryAction(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := RecoveryActionName(tt.action); got != tt.want {
			t.Errorf("RecoveryActionName(%d) = %q, want %q", int(tt.action), got, tt.want)
		}
	}
}

// --- DefaultSupervisorConfig ---

func TestDefaultSupervisorConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultSupervisorConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.MaxForceKills != 1 {
		t.Errorf("MaxForceKills = %d, want 1", cfg.MaxForceKills)
	}
}

// --- NewSupervisor + Start ---

func TestSupervisor_NewAndStart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := NewSupervisor(ctx, DefaultSupervisorConfig())

	snap := s.Snapshot()
	if snap.State != SupervisorIdle {
		t.Errorf("initial state = %s, want Idle", snap.StateName)
	}

	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	snap = s.Snapshot()
	if snap.State != SupervisorRunning {
		t.Errorf("after Start: state = %s, want Running", snap.StateName)
	}
}

func TestSupervisor_StartFromNonIdle(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()

	// Starting from Running should fail.
	if err := s.Start(); err == nil {
		t.Error("expected error starting from Running")
	}
}

// --- PTY EOF: restart then escalate ---

func TestSupervisor_PTYEOF_Restart(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), SupervisorConfig{MaxRetries: 2})
	_ = s.Start()

	d := s.HandleError("EOF", ErrorClassPTYEOF)
	if d.Action != RecoveryRestart {
		t.Errorf("first EOF: Action = %s, want Restart", RecoveryActionName(d.Action))
	}
	if d.NewState != SupervisorRecovering {
		t.Errorf("first EOF: state = %s, want Recovering", SupervisorStateName(d.NewState))
	}
}

func TestSupervisor_PTYEOF_EscalateAfterMax(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), SupervisorConfig{MaxRetries: 2})
	_ = s.Start()

	// Use up retries.
	s.HandleError("EOF", ErrorClassPTYEOF)
	s.HandleError("EOF", ErrorClassPTYEOF)

	// Third: escalate.
	d := s.HandleError("EOF", ErrorClassPTYEOF)
	if d.Action != RecoveryEscalate {
		t.Errorf("third EOF: Action = %s, want Escalate", RecoveryActionName(d.Action))
	}
	if d.NewState != SupervisorStopped {
		t.Errorf("third EOF: state = %s, want Stopped", SupervisorStateName(d.NewState))
	}
}

// --- PTY Crash: force-kill then restart then escalate ---

func TestSupervisor_PTYCrash_ForceKill(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), SupervisorConfig{
		MaxRetries:    3,
		MaxForceKills: 1,
	})
	_ = s.Start()

	d := s.HandleError("exit 137", ErrorClassPTYCrash)
	if d.Action != RecoveryForceKill {
		t.Errorf("first crash: Action = %s, want ForceKill", RecoveryActionName(d.Action))
	}
}

func TestSupervisor_PTYCrash_ThenRestart(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), SupervisorConfig{
		MaxRetries:    3,
		MaxForceKills: 1,
	})
	_ = s.Start()

	// First crash: force-kill.
	d := s.HandleError("exit 137", ErrorClassPTYCrash)
	if d.Action != RecoveryForceKill {
		t.Fatalf("Action = %s, want ForceKill", RecoveryActionName(d.Action))
	}

	// Second crash: force-kill limit exceeded, falls to restart.
	d = s.HandleError("exit 137", ErrorClassPTYCrash)
	if d.Action != RecoveryRestart {
		t.Errorf("second crash: Action = %s, want Restart", RecoveryActionName(d.Action))
	}
}

// --- PTY Error: retry then restart then escalate ---

func TestSupervisor_PTYError_RetryThenRestart(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), SupervisorConfig{MaxRetries: 4})
	_ = s.Start()

	// retryThreshold = 4/2 = 2, so first 2 are retries.
	d := s.HandleError("read error", ErrorClassPTYError)
	if d.Action != RecoveryRetry {
		t.Errorf("first: Action = %s, want Retry", RecoveryActionName(d.Action))
	}

	d = s.HandleError("read error", ErrorClassPTYError)
	if d.Action != RecoveryRetry {
		t.Errorf("second: Action = %s, want Retry", RecoveryActionName(d.Action))
	}

	// 3rd and 4th: restart.
	d = s.HandleError("read error", ErrorClassPTYError)
	if d.Action != RecoveryRestart {
		t.Errorf("third: Action = %s, want Restart", RecoveryActionName(d.Action))
	}

	d = s.HandleError("read error", ErrorClassPTYError)
	if d.Action != RecoveryRestart {
		t.Errorf("fourth: Action = %s, want Restart", RecoveryActionName(d.Action))
	}

	// 5th: escalate.
	d = s.HandleError("read error", ErrorClassPTYError)
	if d.Action != RecoveryEscalate {
		t.Errorf("fifth: Action = %s, want Escalate", RecoveryActionName(d.Action))
	}
}

// --- MCP Timeout: retry then restart then escalate ---

func TestSupervisor_MCPTimeout_RetryThenRestart(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), SupervisorConfig{MaxRetries: 3})
	_ = s.Start()

	// retryThreshold = 3/2 = 1, so first is retry.
	d := s.HandleError("timeout", ErrorClassMCPTimeout)
	if d.Action != RecoveryRetry {
		t.Errorf("first: Action = %s, want Retry", RecoveryActionName(d.Action))
	}

	// 2nd-3rd: restart.
	d = s.HandleError("timeout", ErrorClassMCPTimeout)
	if d.Action != RecoveryRestart {
		t.Errorf("second: Action = %s, want Restart", RecoveryActionName(d.Action))
	}

	d = s.HandleError("timeout", ErrorClassMCPTimeout)
	if d.Action != RecoveryRestart {
		t.Errorf("third: Action = %s, want Restart", RecoveryActionName(d.Action))
	}

	// 4th: escalate.
	d = s.HandleError("timeout", ErrorClassMCPTimeout)
	if d.Action != RecoveryEscalate {
		t.Errorf("fourth: Action = %s, want Escalate", RecoveryActionName(d.Action))
	}
}

// --- MCP Malformed: retry once then escalate ---

func TestSupervisor_MCPMalformed_RetryOnce(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()

	d := s.HandleError("bad json", ErrorClassMCPMalformed)
	if d.Action != RecoveryRetry {
		t.Errorf("first: Action = %s, want Retry", RecoveryActionName(d.Action))
	}

	d = s.HandleError("bad json", ErrorClassMCPMalformed)
	if d.Action != RecoveryEscalate {
		t.Errorf("second: Action = %s, want Escalate", RecoveryActionName(d.Action))
	}
}

// --- Cancellation ---

func TestSupervisor_Cancelled_Error(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()

	d := s.HandleError("cancelled", ErrorClassCancelled)
	if d.Action != RecoveryDrain {
		t.Errorf("Action = %s, want Drain", RecoveryActionName(d.Action))
	}
	if d.NewState != SupervisorDraining {
		t.Errorf("state = %s, want Draining", SupervisorStateName(d.NewState))
	}
}

func TestSupervisor_ContextCancel_DuringError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	s := NewSupervisor(ctx, DefaultSupervisorConfig())
	_ = s.Start()

	// Cancel context before handling error.
	cancel()

	d := s.HandleError("read error", ErrorClassPTYError)
	if d.Action != RecoveryDrain {
		t.Errorf("Action = %s, want Drain (context cancelled)", RecoveryActionName(d.Action))
	}
}

func TestSupervisor_Context_Propagation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	s := NewSupervisor(ctx, DefaultSupervisorConfig())

	// Supervisor context should be derived from parent.
	select {
	case <-s.Context().Done():
		t.Fatal("context should not be done yet")
	default:
		// ok
	}

	cancel()

	select {
	case <-s.Context().Done():
		// ok — propagated
	default:
		t.Fatal("context should be done after parent cancel")
	}
}

// --- Graceful Shutdown ---

func TestSupervisor_Shutdown_Normal(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()

	d := s.Shutdown()
	if d.Action != RecoveryDrain {
		t.Errorf("first shutdown: Action = %s, want Drain", RecoveryActionName(d.Action))
	}
	if d.NewState != SupervisorDraining {
		t.Errorf("state = %s, want Draining", SupervisorStateName(d.NewState))
	}
}

func TestSupervisor_Shutdown_DoubleForceKills(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()

	// First shutdown: drain.
	s.Shutdown()

	// Second shutdown: force-kill.
	d := s.Shutdown()
	if d.Action != RecoveryForceKill {
		t.Errorf("second shutdown: Action = %s, want ForceKill", RecoveryActionName(d.Action))
	}
	if d.NewState != SupervisorStopped {
		t.Errorf("state = %s, want Stopped", SupervisorStateName(d.NewState))
	}
}

func TestSupervisor_Shutdown_AlreadyStopped(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()

	s.ConfirmStopped()

	d := s.Shutdown()
	if d.Action != RecoveryAbort {
		t.Errorf("Action = %s, want Abort (already stopped)", RecoveryActionName(d.Action))
	}
}

// --- ConfirmRecovery ---

func TestSupervisor_ConfirmRecovery(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()

	s.HandleError("EOF", ErrorClassPTYEOF)
	snap := s.Snapshot()
	if snap.State != SupervisorRecovering {
		t.Fatalf("state = %s, want Recovering", snap.StateName)
	}

	s.ConfirmRecovery()
	snap = s.Snapshot()
	if snap.State != SupervisorRunning {
		t.Errorf("after confirm: state = %s, want Running", snap.StateName)
	}
	if snap.LastError != "" {
		t.Errorf("LastError = %q, want empty", snap.LastError)
	}
}

// --- ConfirmStopped ---

func TestSupervisor_ConfirmStopped(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()

	s.ConfirmStopped()
	snap := s.Snapshot()
	if snap.State != SupervisorStopped {
		t.Errorf("state = %s, want Stopped", snap.StateName)
	}

	// Context should be cancelled.
	select {
	case <-s.Context().Done():
		// ok
	default:
		t.Error("context should be done after ConfirmStopped")
	}
}

// --- Reset ---

func TestSupervisor_Reset(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()

	// Drive to stopped.
	for i := 0; i < 10; i++ {
		s.HandleError("error", ErrorClassPTYEOF)
	}
	s.ConfirmStopped()

	// Reset.
	s.Reset(context.Background())
	snap := s.Snapshot()
	if snap.State != SupervisorIdle {
		t.Errorf("after Reset: state = %s, want Idle", snap.StateName)
	}
	if snap.RetryCount != 0 {
		t.Errorf("RetryCount = %d, want 0", snap.RetryCount)
	}
	if snap.Cancelled {
		t.Error("should not be cancelled after Reset")
	}

	// Should be able to Start again.
	if err := s.Start(); err != nil {
		t.Fatalf("Start after Reset: %v", err)
	}
}

// --- Error during Stopped/Draining ---

func TestSupervisor_ErrorDuringStopped(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()
	s.ConfirmStopped()

	d := s.HandleError("late error", ErrorClassPTYError)
	if d.Action != RecoveryAbort {
		t.Errorf("Action = %s, want Abort", RecoveryActionName(d.Action))
	}
}

func TestSupervisor_ErrorDuringDraining(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()
	s.Shutdown()

	d := s.HandleError("late error", ErrorClassPTYError)
	if d.Action != RecoveryAbort {
		t.Errorf("Action = %s, want Abort (during draining)", RecoveryActionName(d.Action))
	}
}

// --- Unknown error class ---

func TestSupervisor_UnknownErrorClass(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()

	d := s.HandleError("mystery", ErrorClass(99))
	if d.Action != RecoveryEscalate {
		t.Errorf("Action = %s, want Escalate", RecoveryActionName(d.Action))
	}
	if d.NewState != SupervisorStopped {
		t.Errorf("state = %s, want Stopped", SupervisorStateName(d.NewState))
	}
}

// --- Snapshot ---

func TestSupervisor_Snapshot(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()

	snap := s.Snapshot()
	if snap.StateName != "Running" {
		t.Errorf("StateName = %q, want Running", snap.StateName)
	}
	if snap.Cancelled {
		t.Error("should not be cancelled")
	}

	s.HandleError("crash", ErrorClassPTYCrash)
	snap = s.Snapshot()
	if snap.LastError != "crash" {
		t.Errorf("LastError = %q, want crash", snap.LastError)
	}
	if snap.ForceKillCount != 1 {
		t.Errorf("ForceKillCount = %d, want 1", snap.ForceKillCount)
	}
}

// --- Concurrent ---

func TestSupervisor_ConcurrentShutdown(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), DefaultSupervisorConfig())
	_ = s.Start()

	const n = 20
	var wg sync.WaitGroup
	decisions := make([]RecoveryDecision, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			decisions[idx] = s.Shutdown()
		}(i)
	}
	wg.Wait()

	// Exactly one Drain, at most one ForceKill, rest Abort.
	drains := 0
	forceKills := 0
	aborts := 0
	for _, d := range decisions {
		switch d.Action {
		case RecoveryDrain:
			drains++
		case RecoveryForceKill:
			forceKills++
		case RecoveryAbort:
			aborts++
		default:
			t.Errorf("unexpected action: %s", RecoveryActionName(d.Action))
		}
	}

	if drains != 1 {
		t.Errorf("drains = %d, want exactly 1", drains)
	}
	if forceKills != 1 {
		t.Errorf("forceKills = %d, want exactly 1", forceKills)
	}
	if aborts != n-2 {
		t.Errorf("aborts = %d, want %d", aborts, n-2)
	}
}

func TestSupervisor_ConcurrentErrors(t *testing.T) {
	t.Parallel()
	s := NewSupervisor(context.Background(), SupervisorConfig{MaxRetries: 100})
	_ = s.Start()

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.HandleError("concurrent error", ErrorClassPTYError)
		}()
	}
	wg.Wait()

	snap := s.Snapshot()
	if snap.RetryCount != n {
		t.Errorf("RetryCount = %d, want %d", snap.RetryCount, n)
	}
}
