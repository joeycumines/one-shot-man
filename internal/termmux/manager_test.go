package termmux

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// mockSession is a minimal InteractiveSession for type-level tests.
type mockSession struct{}

func (mockSession) Target() SessionTarget          { return SessionTarget{} }
func (mockSession) SetTarget(SessionTarget)        {}
func (mockSession) Output() string                 { return "" }
func (mockSession) Screen() string                 { return "" }
func (mockSession) Resize(int, int) error          { return nil }
func (mockSession) Write([]byte) (int, error)      { return 0, nil }
func (mockSession) Close() error                   { return nil }
func (mockSession) Done() <-chan struct{}           { ch := make(chan struct{}); close(ch); return ch }
func (mockSession) IsRunning() bool                { return false }

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
// EventKind and Event
// ---------------------------------------------------------------------------

func TestEventKind_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind EventKind
		want string
	}{
		{EventSessionRegistered, "session-registered"},
		{EventSessionActivated, "session-activated"},
		{EventSessionOutput, "session-output"},
		{EventSessionExited, "session-exited"},
		{EventSessionClosed, "session-closed"},
		{EventResize, "resize"},
		{EventBell, "bell"},
		{EventKind(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.kind.String(); got != tc.want {
			t.Errorf("EventKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

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

	outputCh := make(chan []byte, 1)
	v := vt.NewVTerm(24, 80)

	ms := &managedSession{
		session:    mockSession{},
		vterm:      v,
		state:      SessionCreated,
		target:     SessionTarget{Name: "shell", Kind: SessionKindPTY},
		lastActive: time.Now(),
		outputCh:   outputCh,
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

	// Verify outputCh field.
	if ms.outputCh == nil {
		t.Error("outputCh is nil")
	}
	outputCh <- []byte("data")
	data := <-ms.outputCh
	if string(data) != "data" {
		t.Errorf("outputCh data = %q, want %q", data, "data")
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
