package termmux

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// mockSession is a minimal InteractiveSession for type-level tests.
type mockSession struct{}

func (mockSession) Resize(int, int) error     { return nil }
func (mockSession) Write([]byte) (int, error) { return 0, nil }
func (mockSession) Close() error              { return nil }
func (mockSession) Done() <-chan struct{}     { ch := make(chan struct{}); close(ch); return ch }
func (mockSession) Reader() <-chan []byte     { ch := make(chan []byte); close(ch); return ch }

// controllableSession is a richer mock that records calls and allows
// controlling behavior from tests.
type controllableSession struct {
	writtenData []byte
	writeMu     sync.Mutex
	writeErr    error
	resizeCalls []resizePayload
	closeCalled atomic.Bool
	doneCh      chan struct{}
	readerCh    chan []byte
}

func newControllableSession() *controllableSession {
	return &controllableSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, 16),
	}
}

func (s *controllableSession) Done() <-chan struct{} { return s.doneCh }
func (s *controllableSession) Reader() <-chan []byte { return s.readerCh }

func (s *controllableSession) Write(data []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	s.writtenData = append(s.writtenData, data...)
	return len(data), nil
}

func (s *controllableSession) Resize(rows, cols int) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.resizeCalls = append(s.resizeCalls, resizePayload{rows: rows, cols: cols})
	return nil
}

func (s *controllableSession) Close() error {
	s.closeCalled.Store(true)
	select {
	case <-s.doneCh:
	default:
		close(s.doneCh)
	}
	return nil
}

func (s *controllableSession) Written() []byte {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cp := make([]byte, len(s.writtenData))
	copy(cp, s.writtenData)
	return cp
}

func (s *controllableSession) Resizes() []resizePayload {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cp := make([]resizePayload, len(s.resizeCalls))
	copy(cp, s.resizeCalls)
	return cp
}

// startManager creates a SessionManager, starts the worker, and returns
// the manager and a cleanup function. Cleanup cancels the context and
// waits for the worker to stop.
func startManager(t *testing.T, opts ...ManagerOption) (*SessionManager, func()) {
	t.Helper()
	m := NewSessionManager(opts...)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(ctx)
	}()
	// Wait for the worker goroutine to start processing before
	// returning — prevents races where API calls arrive before
	// the worker is ready.
	<-m.Started()
	cleanup := func() {
		cancel()
		<-errCh
	}
	return m, cleanup
}

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
	wg.Add(writers)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			snap := &ScreenSnapshot{
				Gen:        uint64(i),
				PlainText:  "hello",
				ANSI:       "\x1b[32mhello\x1b[0m",
				FullScreen: "\x1b[1;1Hhello",
				Rows:       24,
				Cols:       80,
				Timestamp:  time.Now(),
			}
			ms.snapshot.Store(snap)
		}
	}()

	// Reader goroutines (simulate TUI, JS shim, etc.).
	wg.Add(readers)
	for r := 0; r < readers; r++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				snap := ms.snapshot.Load()
				if snap == nil {
					continue
				}
				// Access all fields — the race detector would catch sharing.
				_ = snap.Gen
				_ = snap.PlainText
				_ = snap.ANSI
				_ = snap.FullScreen
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
	}

	seen := make(map[requestKind]bool)
	for _, k := range kinds {
		if seen[k] {
			t.Errorf("duplicate requestKind value: %d", k)
		}
		seen[k] = true
	}

	if len(kinds) != 12 {
		t.Errorf("expected 12 request kinds, got %d", len(kinds))
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
	data, ok := evt.Data.([]byte)
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
		Gen:       1,
		PlainText: "gen1",
		Rows:      24,
		Cols:      80,
		Timestamp: time.Now(),
	}
	ms.snapshot.Store(snap1)

	loaded := ms.snapshot.Load()
	if loaded == nil {
		t.Fatal("loaded snapshot is nil after Store")
	}
	if loaded.Gen != 1 || loaded.PlainText != "gen1" {
		t.Errorf("snapshot = {Gen: %d, PlainText: %q}, want {1, gen1}",
			loaded.Gen, loaded.PlainText)
	}

	// Overwrite with a new generation.
	snap2 := &ScreenSnapshot{Gen: 2, PlainText: "gen2"}
	ms.snapshot.Store(snap2)

	loaded = ms.snapshot.Load()
	if loaded.Gen != 2 {
		t.Errorf("Gen = %d after overwrite, want 2", loaded.Gen)
	}

	// Original snap1 is unaffected (immutability).
	if snap1.Gen != 1 || snap1.PlainText != "gen1" {
		t.Error("snap1 was mutated after publishing snap2")
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	if snap.PlainText != "hello world" {
		t.Errorf("PlainText = %q, want %q", snap.PlainText, "hello world")
	}
	if snap.ANSI == "" {
		t.Error("ANSI should not be empty after output")
	}
	if snap.FullScreen == "" {
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
	if snap.PlainText != "total 42" {
		t.Errorf("PlainText = %q, want %q", snap.PlainText, "total 42")
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
		if snap != nil && snap.PlainText == "pipeline output" {
			break
		}
		select {
		case <-deadline:
			snap := m.Snapshot(id)
			t.Fatalf("timed out waiting for snapshot; PlainText = %q", snap.PlainText)
		case <-time.After(10 * time.Millisecond):
		}
	}

	snap := m.Snapshot(id)
	if snap.PlainText != "pipeline output" {
		t.Errorf("PlainText = %q, want %q", snap.PlainText, "pipeline output")
	}
	if snap.ANSI == "" {
		t.Error("ANSI should not be empty")
	}
	if snap.FullScreen == "" {
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
	for i := 0; i < 5; i++ {
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
	if snap1 == nil || snap1.PlainText != "output-a" {
		t.Errorf("session 1 PlainText = %q, want %q", snap1.PlainText, "output-a")
	}
	if snap2 == nil || snap2.PlainText != "output-b" {
		t.Errorf("session 2 PlainText = %q, want %q", snap2.PlainText, "output-b")
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
	if snap == nil || snap.PlainText != "delayed output" {
		plain := ""
		if snap != nil {
			plain = snap.PlainText
		}
		t.Errorf("PlainText = %q, want %q", plain, "delayed output")
	}
}

func TestSessionManager_Pipeline_ShutdownStopsReaders(t *testing.T) {
	t.Parallel()

	m := NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

			wg.Add(1)
			go func() {
				defer wg.Done()
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
			}()
		}

		wg.Wait()
	})
}
