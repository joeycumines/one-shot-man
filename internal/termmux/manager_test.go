package termmux

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// ---------------------------------------------------------------------------
// SessionState transition tests
// ---------------------------------------------------------------------------

func TestSessionState_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		state SessionState
		want  string
	}{
		{SessionCreated, "created"},
		{SessionRunning, "running"},
		{SessionExited, "exited"},
		{SessionClosed, "closed"},
		{SessionState(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("SessionState(%d).String() = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestSessionState_ValidTransitions(t *testing.T) {
	t.Parallel()
	type transition struct {
		from SessionState
		to   SessionState
		ok   bool
	}
	// Exhaustive transition matrix:
	//   Created  → Running ✓, Closed ✓ (all others ✗)
	//   Running  → Exited ✓ (all others ✗)
	//   Exited   → Closed ✓ (all others ✗)
	//   Closed   → nothing ✓ (all ✗)
	transitions := []transition{
		// From Created
		{SessionCreated, SessionCreated, false},
		{SessionCreated, SessionRunning, true},
		{SessionCreated, SessionExited, false},
		{SessionCreated, SessionClosed, true},

		// From Running
		{SessionRunning, SessionCreated, false},
		{SessionRunning, SessionRunning, false},
		{SessionRunning, SessionExited, true},
		{SessionRunning, SessionClosed, false},

		// From Exited
		{SessionExited, SessionCreated, false},
		{SessionExited, SessionRunning, false},
		{SessionExited, SessionExited, false},
		{SessionExited, SessionClosed, true},

		// From Closed (terminal — nothing valid)
		{SessionClosed, SessionCreated, false},
		{SessionClosed, SessionRunning, false},
		{SessionClosed, SessionExited, false},
		{SessionClosed, SessionClosed, false},
	}
	for _, tc := range transitions {
		got := tc.from.validTransition(tc.to)
		if got != tc.ok {
			t.Errorf("%s → %s: validTransition = %v, want %v",
				tc.from, tc.to, got, tc.ok)
		}
	}
}

// ---------------------------------------------------------------------------
// ScreenSnapshot immutability under concurrent access
// ---------------------------------------------------------------------------

func TestScreenSnapshot_ConcurrentReadSafe(t *testing.T) {
	t.Parallel()

	ms := &managedSession{}

	// Simulate the worker publishing snapshots while readers consume them.
	const writers = 1
	const readers = 10
	const iterations = 1000

	var wg sync.WaitGroup

	// Writer goroutine (simulates the worker).
	wg.Go(func() {
		for i := range iterations {
			snap := &ScreenSnapshot{
				Gen:             uint64(i),
				plainTextCache:  "hello",
				ansiCache:       "\x1b[32mhello\x1b[0m",
				fullScreenCache: "\x1b[1;1Hhello",
				Rows:            24,
				Cols:            80,
				Timestamp:       time.Now(),
			}
			ms.snapshot.Store(snap)
		}
	})

	// Reader goroutines (simulate TUI, JS shim, etc.).
	wg.Add(readers)
	for range readers {
		go func() {
			defer wg.Done()
			for range iterations {
				snap := ms.snapshot.Load()
				if snap == nil {
					continue
				}
				// Access all fields — the race detector would catch sharing.
				_ = snap.Gen
				_ = snap.GetPlainText()
				_ = snap.GetANSI()
				_ = snap.GetFullScreen()
				_ = snap.Rows
				_ = snap.Cols
				_ = snap.Timestamp
			}
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// SessionInfo construction
// ---------------------------------------------------------------------------

func TestSessionInfo_Construction(t *testing.T) {
	t.Parallel()

	target := SessionTarget{
		ID:   "claude-1",
		Name: "Claude",
		Kind: SessionKindCapture,
	}

	info := SessionInfo{
		ID:       SessionID(42),
		Target:   target,
		State:    SessionRunning,
		IsActive: true,
	}

	if info.ID != 42 {
		t.Errorf("ID = %d, want 42", info.ID)
	}
	if info.Target.Name != "Claude" {
		t.Errorf("Target.Name = %q, want %q", info.Target.Name, "Claude")
	}
	if info.Target.Kind != SessionKindCapture {
		t.Errorf("Target.Kind = %q, want %q", info.Target.Kind, SessionKindCapture)
	}
	if info.State != SessionRunning {
		t.Errorf("State = %s, want %s", info.State, SessionRunning)
	}
	if !info.IsActive {
		t.Error("IsActive = false, want true")
	}
}

func TestSessionInfo_IsValueCopy(t *testing.T) {
	t.Parallel()

	original := SessionInfo{
		ID:       SessionID(1),
		Target:   SessionTarget{Name: "shell"},
		State:    SessionRunning,
		IsActive: true,
	}

	// Copy and mutate — original must be unaffected.
	copyInfo := original
	copyInfo.State = SessionClosed
	copyInfo.IsActive = false
	copyInfo.Target.Name = "changed"

	if original.State != SessionRunning {
		t.Errorf("original.State mutated to %s", original.State)
	}
	if !original.IsActive {
		t.Error("original.IsActive mutated to false")
	}
	if original.Target.Name != "shell" {
		t.Errorf("original.Target.Name mutated to %q", original.Target.Name)
	}
}

// ---------------------------------------------------------------------------
// Request/response protocol types
// ---------------------------------------------------------------------------

func TestRequest_RoundTrip(t *testing.T) {
	t.Parallel()

	// Simulate a caller creating a request and blocking on the reply.
	replyCh := make(chan response, 1)
	req := request{
		kind: reqRegister,
		payload: &registerPayload{
			session: mockSession{},
			target:  SessionTarget{Name: "test"},
		},
		reply: replyCh,
	}

	// Simulate the worker processing the request.
	if req.kind != reqRegister {
		t.Fatalf("kind = %d, want reqRegister (%d)", req.kind, reqRegister)
	}
	p, ok := req.payload.(*registerPayload)
	if !ok {
		t.Fatalf("payload type = %T, want *registerPayload", req.payload)
	}
	if p.session == nil {
		t.Fatal("session is nil")
	}
	if p.target.Name != "test" {
		t.Fatalf("target.Name = %q, want %q", p.target.Name, "test")
	}

	// Worker sends response.
	req.reply <- response{value: SessionID(1)}

	// Caller receives response.
	resp := <-replyCh
	id, ok := resp.value.(SessionID)
	if !ok {
		t.Fatalf("response value type = %T, want SessionID", resp.value)
	}
	if id != 1 {
		t.Fatalf("SessionID = %d, want 1", id)
	}
	if resp.err != nil {
		t.Fatalf("unexpected error: %v", resp.err)
	}
}

func TestRequest_ErrorResponse(t *testing.T) {
	t.Parallel()

	replyCh := make(chan response, 1)
	req := request{
		kind:    reqActivate,
		payload: SessionID(999),
		reply:   replyCh,
	}

	// Worker dispatches error.
	req.reply <- response{err: ErrSessionNotFound}

	resp := <-replyCh
	if resp.err != ErrSessionNotFound {
		t.Fatalf("err = %v, want ErrSessionNotFound", resp.err)
	}
	if resp.value != nil {
		t.Fatalf("value = %v, want nil on error", resp.value)
	}
}

func TestRequestKind_AllValues(t *testing.T) {
	t.Parallel()

	// Verify all request kinds are distinct and documented.
	kinds := []requestKind{
		reqRegister,
		reqUnregister,
		reqActivate,
		reqInput,
		reqResize,
		reqSnapshot,
		reqActiveID,
		reqSessions,
		reqClose,
		reqActiveWriter,
		reqEnablePassthroughTee,
		reqDisablePassthroughTee,
		reqResizeSession,
	}

	seen := make(map[requestKind]bool)
	for _, k := range kinds {
		if seen[k] {
			t.Errorf("duplicate requestKind value: %d", k)
		}
		seen[k] = true
	}

	if len(kinds) != 13 {
		t.Errorf("expected 13 request kinds, got %d", len(kinds))
	}
}

// ---------------------------------------------------------------------------
// sessionOutput sentinel
// ---------------------------------------------------------------------------

func TestSessionOutput_NilDataIsEOF(t *testing.T) {
	t.Parallel()

	eof := sessionOutput{id: SessionID(1), data: nil}
	normal := sessionOutput{id: SessionID(1), data: []byte("hello")}

	if eof.data != nil {
		t.Error("EOF sentinel should have nil data")
	}
	if normal.data == nil {
		t.Error("normal output should have non-nil data")
	}
}

// ---------------------------------------------------------------------------
// resizePayload
// ---------------------------------------------------------------------------

func TestResizePayload(t *testing.T) {
	t.Parallel()

	p := resizePayload{rows: 50, cols: 132}
	if p.rows != 50 || p.cols != 132 {
		t.Errorf("resize = %dx%d, want 50x132", p.rows, p.cols)
	}
}

// ---------------------------------------------------------------------------
// NewSessionManager
// ---------------------------------------------------------------------------

func TestNewSessionManager_Defaults(t *testing.T) {
	t.Parallel()

	m := NewSessionManager()
	if m.termRows != 24 {
		t.Errorf("termRows = %d, want 24", m.termRows)
	}
	if m.termCols != 80 {
		t.Errorf("termCols = %d, want 80", m.termCols)
	}
	if m.nextID != 1 {
		t.Errorf("nextID = %d, want 1", m.nextID)
	}
	if m.reqChan == nil {
		t.Error("reqChan is nil")
	}
	if m.mergedOutput == nil {
		t.Error("mergedOutput is nil")
	}
	if m.eventBus == nil {
		t.Error("eventBus is nil")
	}
	if m.done == nil {
		t.Error("done is nil")
	}
	if m.sessions == nil {
		t.Error("sessions map is nil")
	}
	if cap(m.reqChan) != 64 {
		t.Errorf("reqChan cap = %d, want 64", cap(m.reqChan))
	}
	if cap(m.mergedOutput) != 64 {
		t.Errorf("mergedOutput cap = %d, want 64", cap(m.mergedOutput))
	}
}

func TestNewSessionManager_WithOptions(t *testing.T) {
	t.Parallel()

	m := NewSessionManager(
		WithTermSize(50, 132),
		WithRequestBuffer(128),
		WithMergedOutputBuffer(32),
	)
	if m.termRows != 50 {
		t.Errorf("termRows = %d, want 50", m.termRows)
	}
	if m.termCols != 132 {
		t.Errorf("termCols = %d, want 132", m.termCols)
	}
	if cap(m.reqChan) != 128 {
		t.Errorf("reqChan cap = %d, want 128", cap(m.reqChan))
	}
	if cap(m.mergedOutput) != 32 {
		t.Errorf("mergedOutput cap = %d, want 32", cap(m.mergedOutput))
	}
}

// ---------------------------------------------------------------------------
// Event construction
// ---------------------------------------------------------------------------

func TestEvent_Construction(t *testing.T) {
	t.Parallel()

	now := time.Now()
	evt := Event{
		Kind:      EventSessionOutput,
		SessionID: SessionID(3),
		Data:      []byte("output data"),
		Time:      now,
	}

	if evt.Kind != EventSessionOutput {
		t.Errorf("Kind = %s, want session-output", evt.Kind)
	}
	if evt.SessionID != 3 {
		t.Errorf("SessionID = %d, want 3", evt.SessionID)
	}
	data, ok := evt.DataAsBytes()
	if !ok || string(data) != "output data" {
		t.Errorf("Data = %v, want []byte(\"output data\")", evt.Data)
	}
	if evt.Time != now {
		t.Errorf("Time mismatch")
	}
}

// ---------------------------------------------------------------------------
// managedSession snapshot publishing
// ---------------------------------------------------------------------------

func TestManagedSession_SnapshotLoadStore(t *testing.T) {
	t.Parallel()

	ms := &managedSession{
		state:  SessionCreated,
		target: SessionTarget{Name: "test"},
	}

	// Initial: no snapshot.
	if snap := ms.snapshot.Load(); snap != nil {
		t.Fatalf("initial snapshot should be nil, got %+v", snap)
	}

	// Publish a snapshot.
	snap1 := &ScreenSnapshot{
		Gen:             1,
		plainTextCache:  "gen1",
		Rows:            24,
		Cols:            80,
		Timestamp:       time.Now(),
	}
	ms.snapshot.Store(snap1)

	loaded := ms.snapshot.Load()
	if loaded == nil {
		t.Fatal("loaded snapshot is nil after Store")
	}
	if loaded.Gen != 1 || loaded.GetPlainText() != "gen1" {
		t.Errorf("snapshot = {Gen: %d, PlainText: %q}, want {1, gen1}",
			loaded.Gen, loaded.GetPlainText())
	}

	// Overwrite with a new generation.
	snap2 := &ScreenSnapshot{Gen: 2, plainTextCache: "gen2"}
	ms.snapshot.Store(snap2)

	loaded = ms.snapshot.Load()
	if loaded.Gen != 2 {
		t.Errorf("Gen = %d after overwrite, want 2", loaded.Gen)
	}

	// Original snap1 is unaffected (immutability).
	if snap1.Gen != 1 || snap1.GetPlainText() != "gen1" {
		t.Error("snap1 was mutated after publishing snap2")
	}
}

// TestScreenSnapshot_CursorFields verifies cursor position is preserved in snapshots.
func TestScreenSnapshot_CursorFields(t *testing.T) {
	t.Parallel()

	snap := &ScreenSnapshot{
		Gen:            1,
		plainTextCache: "hello",
		Rows:           24,
		Cols:           80,
		CursorRow:      5,
		CursorCol:      10,
		Timestamp:      time.Now(),
	}

	if snap.CursorRow != 5 {
		t.Errorf("CursorRow = %d; want 5", snap.CursorRow)
	}
	if snap.CursorCol != 10 {
		t.Errorf("CursorCol = %d; want 10", snap.CursorCol)
	}

	// Verify it's stored/loaded correctly via atomic pointer.
	ms := &managedSession{}
	ms.snapshot.Store(snap)
	loaded := ms.snapshot.Load()
	if loaded.CursorRow != 5 || loaded.CursorCol != 10 {
		t.Errorf("loaded cursor = (%d,%d); want (5,10)", loaded.CursorRow, loaded.CursorCol)
	}
}

// ---------------------------------------------------------------------------
// managedSession field coverage
// ---------------------------------------------------------------------------

func TestManagedSession_AllFields(t *testing.T) {
	t.Parallel()

	v := vt.NewVTerm(24, 80)

	ms := &managedSession{
		session:    mockSession{},
		vterm:      v,
		state:      SessionCreated,
		target:     SessionTarget{Name: "shell", Kind: SessionKindPTY},
		lastActive: time.Now(),
	}

	// Verify session field.
	if ms.session == nil {
		t.Error("session is nil")
	}

	// Verify vterm field.
	if ms.vterm == nil {
		t.Error("vterm is nil")
	}
	_, err := ms.vterm.Write([]byte("hello"))
	if err != nil {
		t.Errorf("vterm.Write error: %v", err)
	}
	if s := ms.vterm.String(); s != "hello" {
		t.Errorf("vterm.String() = %q, want %q", s, "hello")
	}

	// Verify state field.
	if ms.state != SessionCreated {
		t.Errorf("state = %s, want created", ms.state)
	}

	// Verify target field.
	if ms.target.Name != "shell" {
		t.Errorf("target.Name = %q, want %q", ms.target.Name, "shell")
	}

	// Verify lastActive field.
	if ms.lastActive.IsZero() {
		t.Error("lastActive is zero")
	}

	// Verify passthroughWriter field (atomic.Pointer[io.Writer]).
	if w := ms.passthroughWriter.Load(); w != nil {
		t.Error("initial passthroughWriter should be nil")
	}
	var writer io.Writer = io.Discard
	ms.passthroughWriter.Store(&writer)
	if w := ms.passthroughWriter.Load(); w == nil || *w != io.Discard {
		t.Error("passthroughWriter store/load failed")
	}
}

// ---------------------------------------------------------------------------
// SessionManager field coverage (worker-owned fields)
// ---------------------------------------------------------------------------

func TestNewSessionManager_WorkerFields(t *testing.T) {
	t.Parallel()

	m := NewSessionManager()

	// Verify activeID default.
	if m.activeID != 0 {
		t.Errorf("activeID = %d, want 0", m.activeID)
	}

	// Verify snapshotGen default.
	if m.snapshotGen != 0 {
		t.Errorf("snapshotGen = %d, want 0", m.snapshotGen)
	}

	// Verify sessions map is initialized.
	if len(m.sessions) != 0 {
		t.Errorf("sessions len = %d, want 0", len(m.sessions))
	}
}

// ---------------------------------------------------------------------------
// Run / Close / dispatch integration
// ---------------------------------------------------------------------------

func TestSessionManager_RegisterViaWorker(t *testing.T) {
	t.Parallel()

	m := NewSessionManager(WithTermSize(30, 120))

	ctx := t.Context()

	// Start the worker in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(ctx)
	}()

	// Send a register request through the channel.
	replyCh := make(chan response, 1)
	m.reqChan <- request{
		kind: reqRegister,
		payload: &registerPayload{
			session: mockSession{},
			target:  SessionTarget{Name: "test-shell", Kind: SessionKindPTY},
		},
		reply: replyCh,
	}

	// Wait for the worker's response.
	resp := <-replyCh
	if resp.err != nil {
		t.Fatalf("register error: %v", resp.err)
	}
	id, ok := resp.value.(SessionID)
	if !ok {
		t.Fatalf("response value type = %T, want SessionID", resp.value)
	}
	if id != 1 {
		t.Errorf("SessionID = %d, want 1", id)
	}

	// Close the manager and verify clean shutdown.
	m.Close()

	if err := <-errCh; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestSessionManager_ContextCancellation(t *testing.T) {
	t.Parallel()

	m := NewSessionManager()

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(ctx)
	}()

	// Cancel the context — worker should exit.
	cancel()

	if err := <-errCh; err != context.Canceled {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
}

// ---------------------------------------------------------------------------
// Public API: Register
// ---------------------------------------------------------------------------

func TestSessionManager_Register(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "shell", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if id != 1 {
		t.Errorf("id = %d, want 1", id)
	}

	// Register a second session — should get id 2.
	id2, err := m.Register(newControllableSession(), SessionTarget{Name: "claude"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if id2 != 2 {
		t.Errorf("id2 = %d, want 2", id2)
	}
}

func TestSessionManager_Register_FirstBecomesActive(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	_, err := m.Register(newControllableSession(), SessionTarget{Name: "first"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	activeID := m.ActiveID()
	if activeID != 1 {
		t.Errorf("ActiveID = %d, want 1 (first registered)", activeID)
	}
}

// ---------------------------------------------------------------------------
// Public API: Activate
// ---------------------------------------------------------------------------

func TestSessionManager_Activate(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	_, _ = m.Register(newControllableSession(), SessionTarget{Name: "first"})
	id2, _ := m.Register(newControllableSession(), SessionTarget{Name: "second"})

	// Activate second session.
	if err := m.Activate(id2); err != nil {
		t.Fatalf("Activate error: %v", err)
	}
	if got := m.ActiveID(); got != id2 {
		t.Errorf("ActiveID = %d, want %d", got, id2)
	}
}

func TestSessionManager_Activate_InvalidID(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	err := m.Activate(999)
	if err == nil {
		t.Fatal("expected error for invalid session ID")
	}
}

// ---------------------------------------------------------------------------
// Public API: Input
// ---------------------------------------------------------------------------

func TestSessionManager_Input(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	_, _ = m.Register(session, SessionTarget{Name: "shell"})

	if err := m.Input([]byte("hello")); err != nil {
		t.Fatalf("Input error: %v", err)
	}

	// Give the mock time to record the write.
	time.Sleep(10 * time.Millisecond)

	if got := string(session.Written()); got != "hello" {
		t.Errorf("written = %q, want %q", got, "hello")
	}
}

func TestSessionManager_Input_NoActiveSession(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	err := m.Input([]byte("data"))
	if err == nil {
		t.Fatal("expected error when no active session")
	}
}

// ---------------------------------------------------------------------------
// Public API: Resize
// ---------------------------------------------------------------------------

func TestSessionManager_Resize(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	s1 := newControllableSession()
	s2 := newControllableSession()
	_, _ = m.Register(s1, SessionTarget{Name: "a"})
	_, _ = m.Register(s2, SessionTarget{Name: "b"})

	if err := m.Resize(50, 120); err != nil {
		t.Fatalf("Resize error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	for _, s := range []*controllableSession{s1, s2} {
		resizes := s.Resizes()
		if len(resizes) != 1 {
			t.Fatalf("resize calls = %d, want 1", len(resizes))
		}
		if resizes[0].rows != 50 || resizes[0].cols != 120 {
			t.Errorf("resize = %dx%d, want 50x120", resizes[0].rows, resizes[0].cols)
		}
	}
}

func TestSessionManager_ResizeSession(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	s2 := newControllableSession()
	id1, _ := m.Register(s1, SessionTarget{Name: "a"})
	_, _ = m.Register(s2, SessionTarget{Name: "b"})

	if err := m.ResizeSession(id1, 50, 120); err != nil {
		t.Fatalf("ResizeSession error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	resizes1 := s1.Resizes()
	if len(resizes1) != 1 {
		t.Fatalf("session 1 resize calls = %d, want 1", len(resizes1))
	}
	if resizes1[0].rows != 50 || resizes1[0].cols != 120 {
		t.Errorf("session 1 resize = %dx%d, want 50x120", resizes1[0].rows, resizes1[0].cols)
	}

	resizes2 := s2.Resizes()
	if len(resizes2) != 0 {
		t.Fatalf("session 2 resize calls = %d, want 0 (should not be affected)", len(resizes2))
	}
}

func TestSessionManager_ResizeSession_InvalidID(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	err := m.ResizeSession(999, 50, 120)
	if err == nil {
		t.Fatal("expected error for non-existent session ID")
	}
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestSessionManager_TermSize_Default(t *testing.T) {
	t.Parallel()
	m, cleanup := startManager(t)
	defer cleanup()

	rows, cols := m.TermSize()
	if rows != 24 || cols != 80 {
		t.Errorf("TermSize = (%d, %d); want (24, 80)", rows, cols)
	}
}

func TestSessionManager_TermSize_Custom(t *testing.T) {
	t.Parallel()
	m, cleanup := startManager(t, WithTermSize(50, 120))
	defer cleanup()

	rows, cols := m.TermSize()
	if rows != 50 || cols != 120 {
		t.Errorf("TermSize = (%d, %d); want (50, 120)", rows, cols)
	}
}

func TestSessionManager_TermSize_AfterResize(t *testing.T) {
	t.Parallel()
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	if err := m.Resize(40, 160); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	rows, cols := m.TermSize()
	if rows != 40 || cols != 160 {
		t.Errorf("TermSize after Resize = (%d, %d); want (40, 160)", rows, cols)
	}
}

// ---------------------------------------------------------------------------
// Public API: Snapshot
// ---------------------------------------------------------------------------

func TestSessionManager_Snapshot(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	_, _ = m.Register(newControllableSession(), SessionTarget{Name: "test"})

	snap := m.Snapshot(1)
	if snap == nil {
		t.Fatal("Snapshot is nil for registered session")
	}
	if snap.Rows != 24 || snap.Cols != 80 {
		t.Errorf("snap dimensions = %dx%d, want 24x80", snap.Rows, snap.Cols)
	}
}

func TestSessionManager_Snapshot_Unknown(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	if snap := m.Snapshot(999); snap != nil {
		t.Errorf("Snapshot for unknown ID should be nil, got %+v", snap)
	}
}

// ---------------------------------------------------------------------------
// Public API: Sessions
// ---------------------------------------------------------------------------

func TestSessionManager_Sessions(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	_, _ = m.Register(newControllableSession(), SessionTarget{Name: "shell", Kind: SessionKindPTY})
	_, _ = m.Register(newControllableSession(), SessionTarget{Name: "claude", Kind: SessionKindCapture})

	infos := m.Sessions()
	if len(infos) != 2 {
		t.Fatalf("Sessions count = %d, want 2", len(infos))
	}

	// At least one should be active.
	var foundActive bool
	for _, info := range infos {
		if info.IsActive {
			foundActive = true
		}
	}
	if !foundActive {
		t.Error("no active session found in Sessions()")
	}
}

// ---------------------------------------------------------------------------
// Public API: Unregister
// ---------------------------------------------------------------------------

func TestSessionManager_Unregister(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "shell"})

	if err := m.Unregister(id); err != nil {
		t.Fatalf("Unregister error: %v", err)
	}

	// Session should have been closed.
	if !session.closeCalled.Load() {
		t.Error("session.Close() was not called")
	}

	// Snapshot should return nil for removed session.
	if snap := m.Snapshot(id); snap != nil {
		t.Error("Snapshot should return nil for unregistered session")
	}
}

func TestSessionManager_Unregister_ClearsActive(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	id, _ := m.Register(newControllableSession(), SessionTarget{Name: "sole"})

	_ = m.Unregister(id)

	if got := m.ActiveID(); got != 0 {
		t.Errorf("ActiveID = %d, want 0 after unregistering active session", got)
	}
}

func TestSessionManager_Unregister_NotFound(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	err := m.Unregister(999)
	if err == nil {
		t.Fatal("expected error for non-existent session ID")
	}
}

// ---------------------------------------------------------------------------
// Public API: Close (graceful shutdown)
// ---------------------------------------------------------------------------

func TestSessionManager_Close_ClosesAllSessions(t *testing.T) {
	t.Parallel()

	m := NewSessionManager()
	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(ctx)
	}()
	<-m.Started()

	s1 := newControllableSession()
	s2 := newControllableSession()
	s3 := newControllableSession()
	_, _ = m.Register(s1, SessionTarget{Name: "a"})
	_, _ = m.Register(s2, SessionTarget{Name: "b"})
	_, _ = m.Register(s3, SessionTarget{Name: "c"})

	m.Close()

	if err := <-errCh; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	for i, s := range []*controllableSession{s1, s2, s3} {
		if !s.closeCalled.Load() {
			t.Errorf("session %d: Close() was not called during shutdown", i+1)
		}
	}
}

// ---------------------------------------------------------------------------
// mergedOutput: session output processing
// ---------------------------------------------------------------------------

func TestSessionManager_MergedOutput_VTerm(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	_, err := m.Register(newControllableSession(), SessionTarget{Name: "test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// Simulate PTY output by sending directly to mergedOutput.
	m.mergedOutput <- sessionOutput{id: 1, data: []byte("hello world")}

	// Wait for the worker to process.
	time.Sleep(50 * time.Millisecond)

	snap := m.Snapshot(1)
	if snap == nil {
		t.Fatal("Snapshot is nil after output")
	}
	if snap.GetPlainText() != "hello world" {
		t.Errorf("PlainText = %q, want %q", snap.GetPlainText(), "hello world")
	}
	if snap.GetANSI() == "" {
		t.Error("ANSI is empty, want non-empty")
	}
	if snap.GetFullScreen() == "" {
		t.Error("FullScreen should not be empty after output")
	}
	if snap.Gen < 2 {
		t.Errorf("Gen = %d, should be >= 2 (initial + output)", snap.Gen)
	}
}

func TestSessionManager_MergedOutput_CreatedToRunning(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	_, _ = m.Register(newControllableSession(), SessionTarget{Name: "test"})

	// Before output — should be Created.
	infos := m.Sessions()
	for _, info := range infos {
		if info.State != SessionCreated {
			t.Errorf("initial state = %s, want created", info.State)
		}
	}

	// Send output — should transition to Running.
	m.mergedOutput <- sessionOutput{id: 1, data: []byte("x")}
	time.Sleep(50 * time.Millisecond)

	infos = m.Sessions()
	for _, info := range infos {
		if info.ID == 1 && info.State != SessionRunning {
			t.Errorf("state after output = %s, want running", info.State)
		}
	}
}

func TestSessionManager_MergedOutput_EOF(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	subID, evtCh := m.Subscribe(64)
	defer m.Unsubscribe(subID)

	_, _ = m.Register(newControllableSession(), SessionTarget{Name: "test"})

	// Transition to Running first (EOF only valid from Running).
	m.mergedOutput <- sessionOutput{id: 1, data: []byte("x")}
	time.Sleep(50 * time.Millisecond)

	// Send EOF sentinel.
	m.mergedOutput <- sessionOutput{id: 1, data: nil}
	time.Sleep(50 * time.Millisecond)

	infos := m.Sessions()
	for _, info := range infos {
		if info.ID == 1 && info.State != SessionExited {
			t.Errorf("state after EOF = %s, want exited", info.State)
		}
	}

	// Verify EventSessionExited was published.
	var foundExited bool
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventSessionExited && evt.SessionID == 1 {
				foundExited = true
			}
		default:
			goto done
		}
	}
done:
	if !foundExited {
		t.Error("EventSessionExited not received")
	}
}

// ---------------------------------------------------------------------------
// Event delivery via public API
// ---------------------------------------------------------------------------

func TestSessionManager_Events_Register(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	subID, evtCh := m.Subscribe(64)
	defer m.Unsubscribe(subID)

	_, _ = m.Register(newControllableSession(), SessionTarget{Name: "test"})

	select {
	case evt := <-evtCh:
		if evt.Kind != EventSessionRegistered {
			t.Errorf("Kind = %s, want session-registered", evt.Kind)
		}
		if evt.SessionID != 1 {
			t.Errorf("SessionID = %d, want 1", evt.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for register event")
	}
}

func TestSessionManager_Events_Activate(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	subID, evtCh := m.Subscribe(64)
	defer m.Unsubscribe(subID)

	_, _ = m.Register(newControllableSession(), SessionTarget{Name: "a"})
	id2, _ := m.Register(newControllableSession(), SessionTarget{Name: "b"})

	// Drain register events.
	<-evtCh
	<-evtCh

	_ = m.Activate(id2)

	select {
	case evt := <-evtCh:
		if evt.Kind != EventSessionActivated {
			t.Errorf("Kind = %s, want session-activated", evt.Kind)
		}
		if evt.SessionID != id2 {
			t.Errorf("SessionID = %d, want %d", evt.SessionID, id2)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for activate event")
	}
}

// ---------------------------------------------------------------------------
// sortSessionIDs
// ---------------------------------------------------------------------------

func TestSortSessionIDs(t *testing.T) {
	t.Parallel()

	ids := []SessionID{3, 1, 5, 2, 4}
	sortSessionIDs(ids)

	want := []SessionID{5, 4, 3, 2, 1}
	for i, id := range ids {
		if id != want[i] {
			t.Errorf("ids[%d] = %d, want %d", i, id, want[i])
		}
	}
}

func TestSortSessionIDs_Empty(t *testing.T) {
	t.Parallel()
	sortSessionIDs(nil) // Must not panic.
}

func TestSortSessionIDs_Single(t *testing.T) {
	t.Parallel()
	ids := []SessionID{42}
	sortSessionIDs(ids)
	if ids[0] != 42 {
		t.Errorf("ids[0] = %d, want 42", ids[0])
	}
}

// ---------------------------------------------------------------------------
// Concurrent access under race detector
// ---------------------------------------------------------------------------

func TestSessionManager_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	const goroutines = 10
	const iterations = 100

	var wg sync.WaitGroup

	// Concurrent registrations.
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				_, _ = m.Register(newControllableSession(), SessionTarget{Name: "test"})
			}
		}()
	}

	// Concurrent queries.
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				_ = m.ActiveID()
				_ = m.Sessions()
				_ = m.Snapshot(SessionID(1))
			}
		}()
	}

	// Concurrent input.
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				_ = m.Input([]byte("x"))
			}
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Register→Activate→Input→Snapshot round-trip
// ---------------------------------------------------------------------------

func TestSessionManager_RoundTrip(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "shell", Kind: SessionKindPTY})

	// Activate (already active as first session, but verify explicit activate works).
	if err := m.Activate(id); err != nil {
		t.Fatalf("Activate error: %v", err)
	}

	// Send input.
	if err := m.Input([]byte("ls -la\n")); err != nil {
		t.Fatalf("Input error: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	if got := string(session.Written()); got != "ls -la\n" {
		t.Errorf("written = %q, want %q", got, "ls -la\n")
	}

	// Simulate output.
	m.mergedOutput <- sessionOutput{id: id, data: []byte("total 42\n")}
	time.Sleep(50 * time.Millisecond)

	// Get snapshot.
	snap := m.Snapshot(id)
	if snap == nil {
		t.Fatal("Snapshot is nil after output")
	}
	if snap.GetPlainText() != "total 42" {
		t.Errorf("PlainText = %q, want %q", snap.GetPlainText(), "total 42")
	}
}

// ---------------------------------------------------------------------------
// Post-shutdown: ErrManagerNotRunning
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Methods before Run / after Close / after context cancel
// ---------------------------------------------------------------------------

func TestSessionManager_MethodsBeforeRun(t *testing.T) {
	t.Parallel()

	m := NewSessionManager()
	// Do NOT call Run — all methods should immediately return ErrManagerNotRunning.

	if _, err := m.Register(newControllableSession(), SessionTarget{Name: "pre-run"}); err != ErrManagerNotRunning {
		t.Errorf("Register before Run: err = %v, want ErrManagerNotRunning", err)
	}
	if err := m.Unregister(1); err != ErrManagerNotRunning {
		t.Errorf("Unregister before Run: err = %v, want ErrManagerNotRunning", err)
	}
	if err := m.Activate(1); err != ErrManagerNotRunning {
		t.Errorf("Activate before Run: err = %v, want ErrManagerNotRunning", err)
	}
	if err := m.Input([]byte("data")); err != ErrManagerNotRunning {
		t.Errorf("Input before Run: err = %v, want ErrManagerNotRunning", err)
	}
	if err := m.Resize(50, 120); err != ErrManagerNotRunning {
		t.Errorf("Resize before Run: err = %v, want ErrManagerNotRunning", err)
	}
	if got := m.ActiveID(); got != 0 {
		t.Errorf("ActiveID before Run = %d, want 0", got)
	}
	if got := m.Sessions(); got != nil {
		t.Errorf("Sessions before Run = %v, want nil", got)
	}
	if got := m.Snapshot(1); got != nil {
		t.Errorf("Snapshot before Run = %v, want nil", got)
	}
}

func TestSessionManager_MethodsAfterClose(t *testing.T) {
	t.Parallel()

	m := NewSessionManager()
	ctx := t.Context()
	errCh := make(chan error, 1)
	go func() { errCh <- m.Run(ctx) }()
	<-m.Started()

	// Register one session, then close.
	_, _ = m.Register(newControllableSession(), SessionTarget{Name: "test"})
	m.Close()
	<-errCh

	// All mutation methods should return ErrManagerNotRunning.
	if _, err := m.Register(newControllableSession(), SessionTarget{Name: "post-close"}); err != ErrManagerNotRunning {
		t.Errorf("Register after Close: err = %v, want ErrManagerNotRunning", err)
	}
	if err := m.Unregister(1); err != ErrManagerNotRunning {
		t.Errorf("Unregister after Close: err = %v, want ErrManagerNotRunning", err)
	}
	if err := m.Activate(1); err != ErrManagerNotRunning {
		t.Errorf("Activate after Close: err = %v, want ErrManagerNotRunning", err)
	}
	if err := m.Input([]byte("data")); err != ErrManagerNotRunning {
		t.Errorf("Input after Close: err = %v, want ErrManagerNotRunning", err)
	}
	if err := m.Resize(50, 120); err != ErrManagerNotRunning {
		t.Errorf("Resize after Close: err = %v, want ErrManagerNotRunning", err)
	}
	if got := m.ActiveID(); got != 0 {
		t.Errorf("ActiveID after Close = %d, want 0", got)
	}
	if got := m.Sessions(); got != nil {
		t.Errorf("Sessions after Close = %v, want nil", got)
	}
	if got := m.Snapshot(1); got != nil {
		t.Errorf("Snapshot after Close = %v, want nil", got)
	}
}

func TestSessionManager_MethodsAfterContextCancel(t *testing.T) {
	t.Parallel()

	m := NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- m.Run(ctx) }()
	<-m.Started()

	_, _ = m.Register(newControllableSession(), SessionTarget{Name: "test"})

	// Cancel context instead of calling Close.
	cancel()
	<-errCh

	// API should return ErrManagerNotRunning (reqChan is closed by Run on ctx cancel).
	if _, err := m.Register(newControllableSession(), SessionTarget{Name: "post-cancel"}); err != ErrManagerNotRunning {
		t.Errorf("Register after cancel: err = %v, want ErrManagerNotRunning", err)
	}
}

func TestSessionManager_CloseIdempotent(t *testing.T) {
	t.Parallel()

	m := NewSessionManager()
	ctx := t.Context()
	errCh := make(chan error, 1)
	go func() { errCh <- m.Run(ctx) }()
	<-m.Started()

	m.Close()
	<-errCh

	// Second close should not panic.
	m.Close()
}

// ---------------------------------------------------------------------------
// EOF on Created-state session
// ---------------------------------------------------------------------------

func TestSessionManager_MergedOutput_EOF_Created(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	subID, evtCh := m.Subscribe(64)
	defer m.Unsubscribe(subID)

	session := newControllableSession()
	_, _ = m.Register(session, SessionTarget{Name: "quick-exit"})

	// Drain register event.
	<-evtCh

	// Send EOF without any data first (process exits immediately).
	m.mergedOutput <- sessionOutput{id: 1, data: nil}
	time.Sleep(50 * time.Millisecond)

	// Session should be removed (Closed via Created→Closed shortcut).
	if snap := m.Snapshot(1); snap != nil {
		t.Error("Snapshot should be nil for closed session")
	}

	// Session should have been closed.
	if !session.closeCalled.Load() {
		t.Error("session.Close() was not called")
	}

	// Verify EventSessionClosed was published.
	var foundClosed bool
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventSessionClosed && evt.SessionID == 1 {
				foundClosed = true
			}
		default:
			goto done
		}
	}
done:
	if !foundClosed {
		t.Error("EventSessionClosed not received for Created→Closed")
	}
}

// ---------------------------------------------------------------------------
// Merged output pipeline: session.Reader() → reader goroutine → worker
// ---------------------------------------------------------------------------

func TestSessionManager_Pipeline_OutputFlowsToSnapshot(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "pipeline-test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// Send output through the session's Reader channel.
	session.readerCh <- []byte("pipeline output")

	// Wait for the worker to process.
	deadline := time.After(2 * time.Second)
	for {
		snap := m.Snapshot(id)
		if snap != nil && snap.GetPlainText() == "pipeline output" {
			break
		}
		select {
		case <-deadline:
			snap := m.Snapshot(id)
			t.Fatalf("timed out waiting for snapshot; PlainText = %q", snap.GetPlainText())
		case <-time.After(10 * time.Millisecond):
		}
	}

	snap := m.Snapshot(id)
	if snap.GetPlainText() != "pipeline output" {
		t.Errorf("PlainText = %q, want %q", snap.GetPlainText(), "pipeline output")
	}
	if snap.GetANSI() == "" {
		t.Error("ANSI should not be empty")
	}
	if snap.GetFullScreen() == "" {
		t.Error("FullScreen should not be empty")
	}
}

func TestSessionManager_Pipeline_EOFTransition(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	subID, evtCh := m.Subscribe(64)
	defer m.Unsubscribe(subID)

	session := newControllableSession()
	_, err := m.Register(session, SessionTarget{Name: "eof-test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	// Drain register event.
	<-evtCh

	// Send output to transition to Running, then close the channel for EOF.
	session.readerCh <- []byte("data")
	time.Sleep(50 * time.Millisecond)

	close(session.readerCh)
	time.Sleep(100 * time.Millisecond)

	// Verify state transitioned to Exited.
	infos := m.Sessions()
	for _, info := range infos {
		if info.ID == 1 && info.State != SessionExited {
			t.Errorf("state = %s, want exited", info.State)
		}
	}

	// Verify EventSessionExited.
	var foundExited bool
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventSessionExited && evt.SessionID == 1 {
				foundExited = true
			}
		default:
			goto done
		}
	}
done:
	if !foundExited {
		t.Error("EventSessionExited not received")
	}
}

func TestSessionManager_Pipeline_CreatedToRunningTransition(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	_, err := m.Register(session, SessionTarget{Name: "transition-test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// Before output — should be Created.
	infos := m.Sessions()
	for _, info := range infos {
		if info.ID == 1 && info.State != SessionCreated {
			t.Errorf("initial state = %s, want created", info.State)
		}
	}

	// Send output through Reader channel.
	session.readerCh <- []byte("first output")
	time.Sleep(100 * time.Millisecond)

	// Should transition to Running.
	infos = m.Sessions()
	for _, info := range infos {
		if info.ID == 1 && info.State != SessionRunning {
			t.Errorf("state after output = %s, want running", info.State)
		}
	}
}

func TestSessionManager_Pipeline_SnapshotGenIncreases(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "gen-test"})

	initialSnap := m.Snapshot(id)
	initialGen := initialSnap.Gen

	// Send multiple outputs.
	for range 5 {
		session.readerCh <- []byte("x")
	}
	time.Sleep(200 * time.Millisecond)

	snap := m.Snapshot(id)
	if snap.Gen <= initialGen {
		t.Errorf("Gen = %d, should be > %d after 5 outputs", snap.Gen, initialGen)
	}
}

func TestSessionManager_Pipeline_BellEvent(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	subID, evtCh := m.Subscribe(64)
	defer m.Unsubscribe(subID)

	session := newControllableSession()
	_, err := m.Register(session, SessionTarget{Name: "bell-test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	// Drain register event.
	<-evtCh

	// Send BEL character through the Reader channel.
	session.readerCh <- []byte("\x07")
	time.Sleep(100 * time.Millisecond)

	// Verify EventBell was published.
	var foundBell bool
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventBell && evt.SessionID == 1 {
				foundBell = true
			}
		default:
			goto done
		}
	}
done:
	if !foundBell {
		t.Error("EventBell not received after BEL character")
	}
}

func TestSessionManager_Pipeline_OSCTitleEvent(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	subID, evtCh := m.Subscribe(64)
	defer m.Unsubscribe(subID)

	session := newControllableSession()
	_, err := m.Register(session, SessionTarget{Name: "osc-test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	// Drain register event.
	<-evtCh

	// Send OSC 0 (set window title) through the Reader channel.
	session.readerCh <- []byte("\x1b]0;My Title\x07")
	time.Sleep(100 * time.Millisecond)

	// Verify EventTitle was published with correct data.
	var foundTitle bool
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventTitle && evt.SessionID == 1 {
				foundTitle = true
				if data, ok := evt.Data.(string); !ok || data != "My Title" {
					t.Errorf("EventTitle data = %q; want %q", data, "My Title")
				}
			}
		default:
			goto doneTitle
		}
	}
doneTitle:
	if !foundTitle {
		t.Error("EventTitle not received after OSC 0 sequence")
	}
}

func TestSessionManager_Pipeline_OSCWorkingDirectoryEvent(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	subID, evtCh := m.Subscribe(64)
	defer m.Unsubscribe(subID)

	session := newControllableSession()
	_, err := m.Register(session, SessionTarget{Name: "osc-cwd-test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	// Drain register event.
	<-evtCh

	// Send OSC 7 (set working directory) through the Reader channel.
	session.readerCh <- []byte("\x1b]7;file:///home/user\x07")
	time.Sleep(100 * time.Millisecond)

	// Verify EventWorkingDirectory was published.
	var foundCwd bool
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventWorkingDirectory && evt.SessionID == 1 {
				foundCwd = true
				if data, ok := evt.Data.(string); !ok || data != "file:///home/user" {
					t.Errorf("EventWorkingDirectory data = %q; want %q", data, "file:///home/user")
				}
			}
		default:
			goto doneCwd
		}
	}
doneCwd:
	if !foundCwd {
		t.Error("EventWorkingDirectory not received after OSC 7 sequence")
	}
}

func TestSessionManager_Pipeline_OSCClipboardEvent(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	subID, evtCh := m.Subscribe(64)
	defer m.Unsubscribe(subID)

	session := newControllableSession()
	_, err := m.Register(session, SessionTarget{Name: "osc-clip-test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	// Drain register event.
	<-evtCh

	// Send OSC 52 (clipboard) through the Reader channel.
	session.readerCh <- []byte("\x1b]52;c;SGVsbG8=\x07")
	time.Sleep(100 * time.Millisecond)

	// Verify EventClipboard was published.
	var foundClip bool
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventClipboard && evt.SessionID == 1 {
				foundClip = true
				if data, ok := evt.Data.(string); !ok || data != "c;SGVsbG8=" {
					t.Errorf("EventClipboard data = %q; want %q", data, "c;SGVsbG8=")
				}
			}
		default:
			goto doneClip
		}
	}
doneClip:
	if !foundClip {
		t.Error("EventClipboard not received after OSC 52 sequence")
	}
}

func TestSessionManager_Pipeline_OSC2TitleEvent(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	subID, evtCh := m.Subscribe(64)
	defer m.Unsubscribe(subID)

	session := newControllableSession()
	_, err := m.Register(session, SessionTarget{Name: "osc2-test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	// Drain register event.
	<-evtCh

	// Send OSC 2 (set window title) through the Reader channel.
	session.readerCh <- []byte("\x1b]2;XTerm Title\x07")
	time.Sleep(100 * time.Millisecond)

	// Verify EventTitle was published.
	var foundTitle bool
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventTitle && evt.SessionID == 1 {
				foundTitle = true
				if data, ok := evt.Data.(string); !ok || data != "XTerm Title" {
					t.Errorf("EventTitle data = %q; want %q", data, "XTerm Title")
				}
			}
		default:
			goto doneOsc2
		}
	}
doneOsc2:
	if !foundTitle {
		t.Error("EventTitle not received after OSC 2 sequence")
	}
}

func TestSessionManager_Pipeline_OSCUnrecognizedNoEvent(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	subID, evtCh := m.Subscribe(64)
	defer m.Unsubscribe(subID)

	session := newControllableSession()
	_, err := m.Register(session, SessionTarget{Name: "osc-unknown-test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	// Drain register event.
	<-evtCh

	// Send OSC 4 (set color palette — not one we handle) through the Reader channel.
	session.readerCh <- []byte("\x1b]4;0;#ff0000\x07")
	time.Sleep(100 * time.Millisecond)

	// Verify no EventTitle/EventWorkingDirectory/EventClipboard was published.
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventTitle || evt.Kind == EventWorkingDirectory || evt.Kind == EventClipboard {
				t.Errorf("unexpected event kind %v for unrecognized OSC code", evt.Kind)
			}
		default:
			goto doneUnrecognized
		}
	}
doneUnrecognized:
	// Test passes if no unrecognized events were emitted.
}

func TestSessionManager_Pipeline_MultipleSessionsIndependent(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	s2 := newControllableSession()
	id1, _ := m.Register(s1, SessionTarget{Name: "session-a"})
	id2, _ := m.Register(s2, SessionTarget{Name: "session-b"})

	// Send different output to each session.
	s1.readerCh <- []byte("output-a")
	s2.readerCh <- []byte("output-b")
	time.Sleep(200 * time.Millisecond)

	snap1 := m.Snapshot(id1)
	snap2 := m.Snapshot(id2)
	if snap1 == nil || snap1.GetPlainText() != "output-a" {
		t.Errorf("session 1 PlainText = %q, want %q", snap1.GetPlainText(), "output-a")
	}
	if snap2 == nil || snap2.GetPlainText() != "output-b" {
		t.Errorf("session 2 PlainText = %q, want %q", snap2.GetPlainText(), "output-b")
	}
}

func TestSessionManager_Pipeline_DelayedStart(t *testing.T) {
	t.Parallel()

	// Create a session whose Reader() returns nil until "started".
	session := &delayedSession{
		doneCh: make(chan struct{}),
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	id, err := m.Register(session, SessionTarget{Name: "delayed"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// Reader is nil initially — goroutine should be polling.
	time.Sleep(50 * time.Millisecond)

	// "Start" the session — Reader() now returns a channel.
	ch := make(chan []byte, 16)
	session.start(ch)

	// Send output.
	ch <- []byte("delayed output")
	time.Sleep(200 * time.Millisecond)

	snap := m.Snapshot(id)
	if snap == nil || snap.GetPlainText() != "delayed output" {
		plain := ""
		if snap != nil {
			plain = snap.GetPlainText()
		}
		t.Errorf("PlainText = %q, want %q", plain, "delayed output")
	}
}

func TestSessionManager_Pipeline_ShutdownStopsReaders(t *testing.T) {
	t.Parallel()

	m := NewSessionManager()
	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() { errCh <- m.Run(ctx) }()
	<-m.Started()

	session := newControllableSession()
	_, _ = m.Register(session, SessionTarget{Name: "shutdown-test"})

	// Send some output.
	session.readerCh <- []byte("data")
	time.Sleep(50 * time.Millisecond)

	// Shutdown — reader goroutine should exit cleanly via canceled context.
	m.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Session should have been closed during shutdown.
	if !session.closeCalled.Load() {
		t.Error("session.Close() was not called during shutdown")
	}
}

// delayedSession is a mock InteractiveSession whose Reader() returns nil
// until start() is called. This simulates sessions that are registered
// before their PTY is initialized.
type delayedSession struct {
	mu     sync.Mutex
	ch     <-chan []byte
	doneCh chan struct{}
}

func (d *delayedSession) Write(data []byte) (int, error) { return len(data), nil }
func (d *delayedSession) Resize(int, int) error          { return nil }
func (d *delayedSession) Close() error {
	select {
	case <-d.doneCh:
	default:
		close(d.doneCh)
	}
	return nil
}
func (d *delayedSession) Done() <-chan struct{} { return d.doneCh }
func (d *delayedSession) Reader() <-chan []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ch
}

func (d *delayedSession) start(ch chan []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ch = ch
}

func TestSessionManager_EventsDropped(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	// Subscribe with buffer size 1 — guaranteed to overflow quickly.
	_, _ = m.Subscribe(1)

	session := newControllableSession()
	_, err := m.Register(session, SessionTarget{Name: "drop-test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// Pump output rapidly to generate many EventSessionOutput events that
	// overflow the subscriber's buffer-1 channel.
	for range 20 {
		session.readerCh <- []byte("x")
	}

	// Give the worker goroutine time to process output and publish events.
	time.Sleep(300 * time.Millisecond)

	dropped := m.EventsDropped()
	if dropped == 0 {
		t.Error("EventsDropped() = 0, want > 0 with buffer-1 subscriber and rapid output")
	}
}

// ---------------------------------------------------------------------------
// Fuzz testing
// ---------------------------------------------------------------------------

// fuzzSession is a minimal InteractiveSession for fuzz testing.
// It accepts writes, ignores resizes, and produces initial output
// to exercise the merged output pipeline.
type fuzzSession struct {
	doneCh   chan struct{}
	readerCh chan []byte
	closed   atomic.Bool
}

func newFuzzSession() *fuzzSession {
	s := &fuzzSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, 4),
	}
	// Pre-enqueue output to exercise the merged output pipeline.
	s.readerCh <- []byte("fuzz output\r\n")
	return s
}

func (s *fuzzSession) Write(data []byte) (int, error) {
	if s.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	return len(data), nil
}

func (s *fuzzSession) Resize(int, int) error { return nil }

func (s *fuzzSession) Close() error {
	if s.closed.CompareAndSwap(false, true) {
		close(s.readerCh)
		close(s.doneCh)
	}
	return nil
}

func (s *fuzzSession) Done() <-chan struct{} { return s.doneCh }
func (s *fuzzSession) Reader() <-chan []byte { return s.readerCh }

// FuzzSessionRouter bombards the SessionManager with parallel, random
// operation sequences (Register, Activate, Input, Unregister, Resize,
// Snapshot, ActiveID, Sessions, Subscribe, Unsubscribe) from multiple
// goroutines. The goal is to discover race conditions, panics, or
// deadlocks in the worker goroutine under chaotic interleaving.
//
// Run with: go test -fuzz=FuzzSessionRouter -fuzztime=30s ./internal/termmux/
func FuzzSessionRouter(f *testing.F) {
	// Seed corpus: representative operation sequences.
	// Each byte encodes an operation (byte % 10).
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})             // one of each op
	f.Add([]byte{0, 0, 0, 1, 2, 2, 3, 3, 3})                // register-heavy then unregister
	f.Add([]byte{0, 1, 3, 0, 1, 3, 0, 1, 3})                // register-activate-unregister cycles
	f.Add([]byte{0, 2, 2, 2, 2, 2, 2, 2, 2, 2})             // register then input flood
	f.Add([]byte{0, 4, 4, 4, 4, 4, 4, 4, 4, 4})             // register then resize flood
	f.Add([]byte{3, 3, 3, 1, 1, 5, 5})                      // unregister/activate/snapshot on empty
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 3, 3, 3}) // register many, then unregister
	f.Add([]byte{8, 0, 2, 9, 8, 0, 2, 4, 9, 3})             // subscribe/unsubscribe interleaved

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 2 {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		mgr := NewSessionManager(WithTermSize(24, 80))
		errCh := make(chan error, 1)
		go func() { errCh <- mgr.Run(ctx) }()
		<-mgr.Started()

		defer func() {
			mgr.Close()
			<-errCh
		}()

		const numWorkers = 4
		chunkSize := max(len(data)/numWorkers, 1)

		// Shared state: session IDs and subscriber IDs protected by mutex.
		var mu sync.Mutex
		var sessionIDs []SessionID
		var subIDs []int

		var wg sync.WaitGroup
		for w := range numWorkers {
			start := w * chunkSize
			end := start + chunkSize
			if w == numWorkers-1 {
				end = len(data)
			}
			if start >= len(data) {
				break
			}
			chunk := data[start:end]

			wg.Go(func() {
				for _, b := range chunk {
					op := b % 10
					switch op {
					case 0: // Register a new session
						s := newFuzzSession()
						id, err := mgr.Register(s, SessionTarget{
							Name: "fuzz",
							Kind: SessionKindCapture,
						})
						if err == nil {
							mu.Lock()
							sessionIDs = append(sessionIDs, id)
							mu.Unlock()
						}

					case 1: // Activate a random registered session
						mu.Lock()
						ids := append([]SessionID(nil), sessionIDs...)
						mu.Unlock()
						if len(ids) > 0 {
							idx := int(b/10) % len(ids)
							_ = mgr.Activate(ids[idx])
						}

					case 2: // Input to active session
						_ = mgr.Input([]byte{b, b ^ 0xFF})

					case 3: // Unregister a random session
						mu.Lock()
						ids := append([]SessionID(nil), sessionIDs...)
						mu.Unlock()
						if len(ids) > 0 {
							idx := int(b/10) % len(ids)
							_ = mgr.Unregister(ids[idx])
						}

					case 4: // Resize with fuzzer-derived dimensions
						rows := int(b/10)%50 + 1
						cols := int(b/20)%200 + 1
						_ = mgr.Resize(rows, cols)

					case 5: // Snapshot a random session
						mu.Lock()
						ids := append([]SessionID(nil), sessionIDs...)
						mu.Unlock()
						if len(ids) > 0 {
							idx := int(b/10) % len(ids)
							_ = mgr.Snapshot(ids[idx])
						}

					case 6: // ActiveID query
						_ = mgr.ActiveID()

					case 7: // Sessions list query
						_ = mgr.Sessions()

					case 8: // Subscribe to events
						subID, ch := mgr.Subscribe(8)
						mu.Lock()
						subIDs = append(subIDs, subID)
						mu.Unlock()
						// Drain events in background to prevent blocking.
						go func() {
							for range ch {
							}
						}()

					case 9: // Unsubscribe
						mu.Lock()
						ids := append([]int(nil), subIDs...)
						mu.Unlock()
						if len(ids) > 0 {
							idx := int(b/10) % len(ids)
							mgr.Unsubscribe(ids[idx])
						}
					}
				}
			})
		}

		wg.Wait()
	})
}

// ---------------------------------------------------------------------------
// activeWriter tests
// ---------------------------------------------------------------------------

func TestSessionManager_ActiveWriter(t *testing.T) {
	t.Parallel()
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "aw-test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Pump output to transition to Running.
	session.readerCh <- []byte("hello")
	waitForSnapshotContains(t, m, id, "hello", 2*time.Second)

	w, err := m.activeWriter()
	if err != nil {
		t.Fatalf("activeWriter: %v", err)
	}
	if w == nil {
		t.Fatal("activeWriter returned nil writer")
	}

	// Writing to the active writer should send data to the session.
	n, err := w.Write([]byte("test-input"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len("test-input") {
		t.Errorf("Write returned %d, want %d", n, len("test-input"))
	}
	got := string(session.Written())
	if !strings.Contains(got, "test-input") {
		t.Errorf("session received %q, want it to contain %q", got, "test-input")
	}
}

func TestSessionManager_ActiveWriter_NoActiveSession(t *testing.T) {
	t.Parallel()
	m, cleanup := startManager(t)
	defer cleanup()

	_, err := m.activeWriter()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// passthroughTee tests
// ---------------------------------------------------------------------------

func TestSessionManager_EnablePassthroughTee(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "tee-test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Pump output to transition to Running.
	session.readerCh <- []byte("ready")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)

	// Enable tee with a buffer to capture output.
	var teeBuf syncBuffer
	if err := m.enablePassthroughTee(id, &teeBuf); err != nil {
		t.Fatalf("enablePassthroughTee: %v", err)
	}

	// Send output and verify tee captures it.
	session.readerCh <- []byte("tee-data")

	// Wait for the output to be processed and teed.
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(teeBuf.String(), "tee-data") {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for tee data; teeBuf=%q", teeBuf.String())
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Disable tee.
	if err := m.disablePassthroughTee(); err != nil {
		t.Fatalf("disablePassthroughTee: %v", err)
	}

	// Output after disable should not appear in tee.
	session.readerCh <- []byte("after-disable")
	// Wait for the snapshot to update.
	waitForSnapshotContains(t, m, id, "after-disable", 2*time.Second)

	// The tee buffer should not contain "after-disable" because the tee was disabled.
	if strings.Contains(teeBuf.String(), "after-disable") {
		t.Error("tee captured data after being disabled")
	}
}

func TestSessionManager_EnablePassthroughTee_AlreadyActive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "tee-dup", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("ready")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)

	var buf bytes.Buffer
	if err := m.enablePassthroughTee(id, &buf); err != nil {
		t.Fatalf("first enablePassthroughTee: %v", err)
	}

	// Second enable should fail with ErrPassthroughActive.
	err = m.enablePassthroughTee(id, &buf)
	if err == nil {
		t.Error("expected error on duplicate enable, got nil")
	}
	if !errors.Is(err, ErrPassthroughActive) {
		t.Errorf("error = %v, want ErrPassthroughActive", err)
	}

	// Clean up.
	_ = m.disablePassthroughTee()
}

func TestSessionManager_EnablePassthroughTee_InvalidSession(t *testing.T) {
	t.Parallel()
	m, cleanup := startManager(t)
	defer cleanup()

	var buf bytes.Buffer
	err := m.enablePassthroughTee(99999, &buf)
	if err == nil {
		t.Error("expected error for invalid session, got nil")
	}
}

func TestSessionManager_DisablePassthroughTee_Idempotent(t *testing.T) {
	t.Parallel()
	m, cleanup := startManager(t)
	defer cleanup()

	// Disable when nothing is active should be a no-op.
	if err := m.disablePassthroughTee(); err != nil {
		t.Errorf("disablePassthroughTee no-op: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CaptureSession.Rows/Cols tests
// ---------------------------------------------------------------------------

func TestCaptureSession_RowsCols(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()
	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"test"},
		Rows:    30,
		Cols:    120,
	})

	if got := cs.Rows(); got != 30 {
		t.Errorf("Rows before start = %d, want 30", got)
	}
	if got := cs.Cols(); got != 120 {
		t.Errorf("Cols before start = %d, want 120", got)
	}

	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	if got := cs.Rows(); got != 30 {
		t.Errorf("Rows after start = %d, want 30", got)
	}
	if got := cs.Cols(); got != 120 {
		t.Errorf("Cols after start = %d, want 120", got)
	}

	if err := cs.Resize(40, 160); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if got := cs.Rows(); got != 40 {
		t.Errorf("Rows after resize = %d, want 40", got)
	}
	if got := cs.Cols(); got != 160 {
		t.Errorf("Cols after resize = %d, want 160", got)
	}
}

// ---------------------------------------------------------------------------
// Session lifecycle edge case tests
// ---------------------------------------------------------------------------

func TestSessionManager_Pipeline_CreatedToClosedOnEOF(t *testing.T) {
	t.Parallel()

	// When a process exits immediately without producing any output,
	// the session transitions from Created directly to Closed (skipping
	// Exited) and is removed from the sessions map.
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	subID, evtCh := m.Subscribe(64)
	defer m.Unsubscribe(subID)

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "immediate-exit"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	// Drain register event.
	<-evtCh

	// Close the reader channel immediately — simulates process that exits
	// before producing any output.
	close(session.readerCh)
	time.Sleep(200 * time.Millisecond)

	// Session should have been removed from the map (Created -> Closed
	// path deletes it). Sessions() should not list it.
	for _, info := range m.Sessions() {
		if info.ID == id {
			t.Errorf("session %d still in Sessions() after immediate exit, state=%s", id, info.State)
		}
	}

	// Verify EventSessionClosed was emitted (not EventSessionExited,
	// since the Created -> Closed path skips Exited).
	var foundClosed bool
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventSessionClosed && evt.SessionID == id {
				foundClosed = true
			}
		default:
			goto doneCreatedClosed
		}
	}
doneCreatedClosed:
	if !foundClosed {
		t.Error("EventSessionClosed not received for immediate-exit session")
	}

	// Verify session.Close() was called.
	if !session.closeCalled.Load() {
		t.Error("session.Close() was not called on immediate exit")
	}
}

func TestSessionManager_Pipeline_SessionSwitchingOutputRouted(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	s2 := newControllableSession()
	id1, _ := m.Register(s1, SessionTarget{Name: "session-a"})
	id2, _ := m.Register(s2, SessionTarget{Name: "session-b"})

	// Send output to session A.
	s1.readerCh <- []byte("output-a")
	waitForSnapshotContains(t, m, id1, "output-a", 2*time.Second)

	// Activate session B (first session is active by default).
	if err := m.Activate(id2); err != nil {
		t.Fatalf("Activate(%d): %v", id2, err)
	}

	// Send output to session B.
	s2.readerCh <- []byte("output-b")
	waitForSnapshotContains(t, m, id2, "output-b", 2*time.Second)

	// Verify both sessions have independent content.
	snap1 := m.Snapshot(id1)
	snap2 := m.Snapshot(id2)
	if snap1 == nil || !strings.Contains(snap1.GetPlainText(), "output-a") {
		t.Errorf("session 1 PlainText = %q, want containing %q", snap1.GetPlainText(), "output-a")
	}
	if snap2 == nil || !strings.Contains(snap2.GetPlainText(), "output-b") {
		t.Errorf("session 2 PlainText = %q, want containing %q", snap2.GetPlainText(), "output-b")
	}

	// Verify session B is now active.
	infos := m.Sessions()
	for _, info := range infos {
		if info.ID == id2 && !info.IsActive {
			t.Error("session B should be active after Activate")
		}
		if info.ID == id1 && info.IsActive {
			t.Error("session A should not be active after Activate(B)")
		}
	}
}

func TestSessionManager_Pipeline_OutputAfterUnregisterDiscarded(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "unregister-test"})

	// Send initial output to transition to Running.
	session.readerCh <- []byte("before-unregister")
	waitForSnapshotContains(t, m, id, "before-unregister", 2*time.Second)

	// Unregister the session.
	if err := m.Unregister(id); err != nil {
		t.Fatalf("Unregister(%d): %v", id, err)
	}
	time.Sleep(100 * time.Millisecond)

	// Send more output — should be silently discarded (no panic, no error).
	session.readerCh <- []byte("after-unregister")
	time.Sleep(100 * time.Millisecond)

	// Snapshot for the unregistered session should be nil.
	if snap := m.Snapshot(id); snap != nil {
		t.Errorf("Snapshot(%d) = %v, want nil after unregister", id, snap)
	}
}

func TestSessionManager_Pipeline_MultiParamDECSET(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "multi-param"})

	// Send a multi-param DECSET: enable both bracketed paste (2004) and
	// application cursor mode (1) in a single escape sequence.
	session.readerCh <- []byte("\x1b[?2004;1h")
	time.Sleep(200 * time.Millisecond)

	snap := m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if !snap.BracketedPaste {
		t.Error("BracketedPaste = false, want true after CSI ?2004;1h")
	}
	if !snap.ApplicationCursor {
		t.Error("ApplicationCursor = false, want true after CSI ?2004;1h")
	}

	// Now disable both with multi-param DECRST.
	session.readerCh <- []byte("\x1b[?2004;1l")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil after DECRST")
	}
	if snap.BracketedPaste {
		t.Error("BracketedPaste = true, want false after CSI ?2004;1l")
	}
	if snap.ApplicationCursor {
		t.Error("ApplicationCursor = true, want false after CSI ?2004;1l")
	}
}

func TestSessionManager_Pipeline_SecondRegisterDoesNotChangeActive(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	s2 := newControllableSession()
	id1, _ := m.Register(s1, SessionTarget{Name: "first"})
	id2, _ := m.Register(s2, SessionTarget{Name: "second"})

	// First registered session should be active.
	infos := m.Sessions()
	for _, info := range infos {
		if info.ID == id1 && !info.IsActive {
			t.Error("first registered session should be active by default")
		}
		if info.ID == id2 && info.IsActive {
			t.Error("second registered session should NOT be active by default")
		}
	}

	// Verify the first session is still the active one via Sessions().
	activeID := SessionID(0)
	for _, info := range infos {
		if info.IsActive {
			activeID = info.ID
		}
	}
	if activeID != id1 {
		t.Errorf("active session = %d, want %d", activeID, id1)
	}
}

// ---------------------------------------------------------------------------
// VT mode flag integration tests
// ---------------------------------------------------------------------------

func TestSessionManager_Pipeline_BracketedPasteMode(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "bp-test"})

	// Default: BracketedPaste should be false.
	snap := m.Snapshot(id)
	if snap != nil && snap.BracketedPaste {
		t.Error("BracketedPaste should be false by default")
	}

	// Enable bracketed paste.
	session.readerCh <- []byte("\x1b[?2004h")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if !snap.BracketedPaste {
		t.Error("BracketedPaste = false, want true after CSI ?2004h")
	}

	// Disable bracketed paste.
	session.readerCh <- []byte("\x1b[?2004l")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil after disable")
	}
	if snap.BracketedPaste {
		t.Error("BracketedPaste = true, want false after CSI ?2004l")
	}
}

func TestSessionManager_Pipeline_ApplicationCursorMode(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "appcursor-test"})

	// Enable application cursor mode.
	session.readerCh <- []byte("\x1b[?1h")
	time.Sleep(200 * time.Millisecond)

	snap := m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if !snap.ApplicationCursor {
		t.Error("ApplicationCursor = false, want true after CSI ?1h")
	}

	// Disable.
	session.readerCh <- []byte("\x1b[?1l")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil after disable")
	}
	if snap.ApplicationCursor {
		t.Error("ApplicationCursor = true, want false after CSI ?1l")
	}
}

func TestSessionManager_Pipeline_CursorShape(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "cursorshape-test"})

	// Default cursor shape should be 0.
	snap := m.Snapshot(id)
	if snap != nil && snap.CursorShape != 0 {
		t.Errorf("CursorShape = %d, want 0 by default", snap.CursorShape)
	}

	// Set cursor to blink-bar (5).
	session.readerCh <- []byte("\x1b[5 q")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if snap.CursorShape != 5 {
		t.Errorf("CursorShape = %d, want 5 after CSI 5 SP q", snap.CursorShape)
	}

	// Reset to default (0).
	session.readerCh <- []byte("\x1b[0 q")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil after reset")
	}
	if snap.CursorShape != 0 {
		t.Errorf("CursorShape = %d, want 0 after CSI 0 SP q", snap.CursorShape)
	}
}

func TestSessionManager_Pipeline_FocusReportingMode(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "focus-test"})

	// Enable focus reporting.
	session.readerCh <- []byte("\x1b[?1004h")
	time.Sleep(200 * time.Millisecond)

	snap := m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if !snap.FocusReporting {
		t.Error("FocusReporting = false, want true after CSI ?1004h")
	}

	// Disable.
	session.readerCh <- []byte("\x1b[?1004l")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil after disable")
	}
	if snap.FocusReporting {
		t.Error("FocusReporting = true, want false after CSI ?1004l")
	}
}

func TestSessionManager_Pipeline_AutoWrapMode(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "autowrap-test"})

	// Send some plain text first to transition to Running and get an
	// initial snapshot. AutoWrap should be true by default.
	session.readerCh <- []byte("initial")
	waitForSnapshotContains(t, m, id, "initial", 2*time.Second)

	snap := m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil after initial output")
	}
	if !snap.AutoWrap {
		t.Error("AutoWrap = false, want true by default")
	}

	// Disable auto-wrap.
	session.readerCh <- []byte("\x1b[?7l")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if snap.AutoWrap {
		t.Error("AutoWrap = true, want false after CSI ?7l")
	}

	// Re-enable.
	session.readerCh <- []byte("\x1b[?7h")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil after enable")
	}
	if !snap.AutoWrap {
		t.Error("AutoWrap = false, want true after CSI ?7h")
	}
}

func TestSessionManager_Pipeline_InsertMode(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "insertmode-test"})

	// Default: InsertMode should be false.
	snap := m.Snapshot(id)
	if snap != nil && snap.InsertMode {
		t.Error("InsertMode = true, want false by default")
	}

	// Enable insert mode (non-private SM).
	session.readerCh <- []byte("\x1b[4h")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if !snap.InsertMode {
		t.Error("InsertMode = false, want true after CSI 4h")
	}

	// Disable.
	session.readerCh <- []byte("\x1b[4l")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil after disable")
	}
	if snap.InsertMode {
		t.Error("InsertMode = true, want false after CSI 4l")
	}
}

func TestSessionManager_Pipeline_MouseTrackingMode(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "mouse-test"})

	// Default: no mouse tracking.
	snap := m.Snapshot(id)
	if snap != nil && snap.MouseTracking != 0 {
		t.Errorf("MouseTracking = %d, want 0 by default", snap.MouseTracking)
	}

	// Enable basic mouse tracking (mode 1000).
	session.readerCh <- []byte("\x1b[?1000h")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if snap.MouseTracking != 1 {
		t.Errorf("MouseTracking = %d, want 1 after CSI ?1000h", snap.MouseTracking)
	}

	// Enable button-event tracking (mode 1002) — upgrades from basic.
	session.readerCh <- []byte("\x1b[?1002h")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil after 1002h")
	}
	if snap.MouseTracking != 2 {
		t.Errorf("MouseTracking = %d, want 2 after CSI ?1002h", snap.MouseTracking)
	}

	// Enable SGR mouse encoding (mode 1006).
	session.readerCh <- []byte("\x1b[?1006h")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil after 1006h")
	}
	if !snap.MouseSGR {
		t.Error("MouseSGR = false, want true after CSI ?1006h")
	}
}

func TestSessionManager_Pipeline_SynchronizedOutputMode(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "sync-output-test"})

	// Enable synchronized output.
	session.readerCh <- []byte("\x1b[?2026h")
	time.Sleep(200 * time.Millisecond)

	snap := m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if !snap.SynchronizedOutput {
		t.Error("SynchronizedOutput = false, want true after CSI ?2026h")
	}

	// Disable synchronized output.
	session.readerCh <- []byte("\x1b[?2026l")
	time.Sleep(200 * time.Millisecond)

	snap = m.Snapshot(id)
	if snap == nil {
		t.Fatal("snapshot is nil after disable")
	}
	if snap.SynchronizedOutput {
		t.Error("SynchronizedOutput = true, want false after CSI ?2026l")
	}
}
