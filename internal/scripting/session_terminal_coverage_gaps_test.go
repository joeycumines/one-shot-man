package scripting

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/storage"
	"github.com/joeycumines/one-shot-man/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// logging.go coverage gaps
// ============================================================================

// --- WithAttrs / WithGroup (0% → 100%) ---

func TestTUILogHandler_WithAttrs_ReturnsNewHandler(t *testing.T) {
	t.Parallel()
	handler := &tuiLogHandler{shared: &tuiLogHandlerShared{
		maxSize: 100,
		level:   slog.LevelDebug,
	}}

	attrs := []slog.Attr{
		slog.String("key1", "val1"),
		slog.Int("key2", 42),
	}
	got := handler.WithAttrs(attrs)
	assert.NotSame(t, handler, got, "WithAttrs should return a new handler")
	// But they should share the same state
	newH := got.(*tuiLogHandler)
	assert.Same(t, handler.shared, newH.shared, "new handler should share state")
	assert.Len(t, newH.preAttrs, 2)
}

func TestTUILogHandler_WithGroup_ReturnsNewHandler(t *testing.T) {
	t.Parallel()
	handler := &tuiLogHandler{shared: &tuiLogHandlerShared{
		maxSize: 100,
		level:   slog.LevelDebug,
	}}

	got := handler.WithGroup("test-group")
	assert.NotSame(t, handler, got, "WithGroup should return a new handler")
	newH := got.(*tuiLogHandler)
	assert.Same(t, handler.shared, newH.shared, "new handler should share state")
	assert.Equal(t, "test-group", newH.groupPrefix)
}

// --- Handle with non-zero PC (source extraction) ---

func TestTUILogHandler_Handle_WithPC(t *testing.T) {
	t.Parallel()
	handler := &tuiLogHandler{shared: &tuiLogHandlerShared{
		maxSize: 100,
		level:   slog.LevelDebug,
	}}

	// Get a real PC from the current call site
	var pcs [1]uintptr
	runtime.Callers(1, pcs[:])
	pc := pcs[0]
	require.NotZero(t, pc, "sanity: runtime.Callers should return non-zero PC")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test with PC", pc)
	err := handler.Handle(context.Background(), record)
	require.NoError(t, err)

	require.Len(t, handler.shared.entries, 1)
	assert.Equal(t, "scripting", handler.shared.entries[0].Source, "non-zero PC should populate Source")
}

func TestTUILogHandler_Handle_WithZeroPC(t *testing.T) {
	t.Parallel()
	handler := &tuiLogHandler{shared: &tuiLogHandlerShared{
		maxSize: 100,
		level:   slog.LevelDebug,
	}}

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test no PC", 0)
	err := handler.Handle(context.Background(), record)
	require.NoError(t, err)

	require.Len(t, handler.shared.entries, 1)
	assert.Empty(t, handler.shared.entries[0].Source, "zero PC should leave Source empty")
}

// --- Handle with file handler ---

func TestTUILogHandler_Handle_ForwardsToFileHandler(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fileHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	handler := &tuiLogHandler{
		shared: &tuiLogHandlerShared{
			maxSize: 100,
			level:   slog.LevelDebug,
		},
		fileHandler: fileHandler,
	}

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "forwarded msg", 0)
	record.AddAttrs(slog.String("k", "v"))
	err := handler.Handle(context.Background(), record)
	require.NoError(t, err)

	// Handler should store entry AND forward to file handler
	require.Len(t, handler.shared.entries, 1)
	assert.Contains(t, buf.String(), "forwarded msg")
}

// --- Handle overflow (entry eviction) ---

func TestTUILogHandler_Handle_EvictsOldest(t *testing.T) {
	t.Parallel()
	handler := &tuiLogHandler{shared: &tuiLogHandlerShared{
		entries: make([]logEntry, 0, 2),
		maxSize: 2,
		level:   slog.LevelDebug,
	}}

	for i := range 3 {
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "", 0)
		record.AddAttrs(slog.Int("i", i))
		require.NoError(t, handler.Handle(context.Background(), record))
	}

	// Should have evicted entry 0, keeping entries 1 and 2
	require.Len(t, handler.shared.entries, 2)
	assert.Equal(t, "1", handler.shared.entries[0].Attrs["i"])
	assert.Equal(t, "2", handler.shared.entries[1].Attrs["i"])
}

// --- Handle with attrs ---

func TestTUILogHandler_Handle_CapturesAttrs(t *testing.T) {
	t.Parallel()
	handler := &tuiLogHandler{shared: &tuiLogHandlerShared{
		maxSize: 100,
		level:   slog.LevelDebug,
	}}

	record := slog.NewRecord(time.Now(), slog.LevelWarn, "with attrs", 0)
	record.AddAttrs(
		slog.String("str", "hello"),
		slog.Int("num", 42),
		slog.Bool("flag", true),
	)
	require.NoError(t, handler.Handle(context.Background(), record))

	require.Len(t, handler.shared.entries, 1)
	assert.Equal(t, "hello", handler.shared.entries[0].Attrs["str"])
	assert.Equal(t, "42", handler.shared.entries[0].Attrs["num"])
	assert.Equal(t, "true", handler.shared.entries[0].Attrs["flag"])
}

// --- Enabled ---

func TestTUILogHandler_Enabled(t *testing.T) {
	t.Parallel()
	handler := &tuiLogHandler{shared: &tuiLogHandlerShared{level: slog.LevelWarn}}

	assert.False(t, handler.Enabled(context.Background(), slog.LevelDebug))
	assert.False(t, handler.Enabled(context.Background(), slog.LevelInfo))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelWarn))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelError))
}

// --- GetRecentLogs edge cases ---

func TestGetRecentLogs_ZeroCount(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Info("msg1")
	logger.Info("msg2")
	logger.Info("msg3")

	// count=0 should return all entries
	logs := logger.GetRecentLogs(0)
	assert.Len(t, logs, 3)
}

func TestGetRecentLogs_NegativeCount(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Info("msg1")
	logger.Info("msg2")

	// count < 0 should return all entries
	logs := logger.GetRecentLogs(-5)
	assert.Len(t, logs, 2)
}

func TestGetRecentLogs_ExceedsTotal(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Info("only-one")

	// count > total entries should return all
	logs := logger.GetRecentLogs(100)
	assert.Len(t, logs, 1)
	assert.Equal(t, "only-one", logs[0].Message)
}

func TestGetRecentLogs_ExactCount(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Info("first")
	logger.Info("second")
	logger.Info("third")

	// count=2 should return last 2
	logs := logger.GetRecentLogs(2)
	require.Len(t, logs, 2)
	assert.Equal(t, "second", logs[0].Message)
	assert.Equal(t, "third", logs[1].Message)
}

// --- SearchLogs attribute matching ---

func TestSearchLogs_MatchesAttributeKey(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Info("no match here", slog.String("secret-key", "irrelevant-value"))
	logger.Info("also no match")

	// Search for text in attribute KEY (not message, not value)
	matches := logger.SearchLogs("secret-key")
	require.Len(t, matches, 1)
	assert.Equal(t, "no match here", matches[0].Message)
}

func TestSearchLogs_MatchesAttributeValue(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Info("generic message", slog.String("k", "unique-attr-value-xyz"))
	logger.Info("another message")

	// Search for text in attribute VALUE
	matches := logger.SearchLogs("unique-attr-value-xyz")
	require.Len(t, matches, 1)
	assert.Equal(t, "generic message", matches[0].Message)
}

func TestSearchLogs_CaseInsensitive(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Info("Hello World")
	logger.Info("goodbye world")

	matches := logger.SearchLogs("HELLO")
	require.Len(t, matches, 1)
	assert.Equal(t, "Hello World", matches[0].Message)
}

func TestSearchLogs_NoMatches(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Info("something")

	matches := logger.SearchLogs("nonexistent")
	assert.Empty(t, matches)
}

func TestSearchLogs_MatchesMessageBeforeAttributes(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	// Entry where both message AND attribute value contain the search term
	logger.Info("findme in message", slog.String("key", "findme in attr"))

	matches := logger.SearchLogs("findme")
	// Should match exactly once (message match short-circuits attribute search)
	require.Len(t, matches, 1)
}

// --- PrintToTUI nil writer nil sink ---

func TestPrintToTUI_NilWriterNilSink(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	// Should not panic when both writer and sink are nil
	logger.PrintToTUI("safe message")
}

// --- NewTUILogger maxEntries edge cases ---

func TestNewTUILogger_ZeroMaxEntries(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 0, slog.LevelDebug)
	assert.NotNil(t, logger)

	// Should default to 1000
	assert.Equal(t, 1000, logger.handler.shared.maxSize)
}

func TestNewTUILogger_NegativeMaxEntries(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, -5, slog.LevelDebug)
	assert.NotNil(t, logger)

	assert.Equal(t, 1000, logger.handler.shared.maxSize)
}

// --- ClearLogs ---

func TestClearLogs(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Info("entry1")
	logger.Info("entry2")
	require.Len(t, logger.GetLogs(), 2)

	logger.ClearLogs()
	assert.Empty(t, logger.GetLogs())
}

// --- Logger accessor ---

func TestLogger_ReturnsUnderlyingSlogLogger(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	slogLogger := logger.Logger()
	assert.NotNil(t, slogLogger)

	// Using the slog logger should still go through our handler
	slogLogger.Info("via slog")
	logs := logger.GetLogs()
	require.Len(t, logs, 1)
	assert.Equal(t, "via slog", logs[0].Message)
}

// --- Printf ---

func TestPrintf(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Printf("formatted %s %d", "test", 42)

	logs := logger.GetLogs()
	require.Len(t, logs, 1)
	assert.Equal(t, "formatted test 42", logs[0].Message)
}

// --- PrintfToTUI ---

func TestPrintfToTUI(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewTUILogger(&buf, nil, 10, slog.LevelDebug)

	logger.PrintfToTUI("hello %s", "world")

	assert.Equal(t, "hello world\n", buf.String())
}

// --- All log levels ---

func TestAllLogLevels(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 100, slog.LevelDebug)

	logger.Debug("debug msg")
	logger.Info("info msg")
	logger.Warn("warn msg")
	logger.Error("error msg")

	logs := logger.GetLogs()
	require.Len(t, logs, 4)
	assert.Equal(t, slog.LevelDebug, logs[0].Level)
	assert.Equal(t, slog.LevelInfo, logs[1].Level)
	assert.Equal(t, slog.LevelWarn, logs[2].Level)
	assert.Equal(t, slog.LevelError, logs[3].Level)
}

// --- Log level filtering ---

func TestLogLevel_Filtering(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 100, slog.LevelWarn)

	logger.Debug("debug msg")
	logger.Info("info msg")
	logger.Warn("warn msg")
	logger.Error("error msg")

	logs := logger.GetLogs()
	// Only Warn and Error should be captured
	require.Len(t, logs, 2)
	assert.Equal(t, slog.LevelWarn, logs[0].Level)
	assert.Equal(t, slog.LevelError, logs[1].Level)
}

// --- SetTUISink ---

func TestSetTUISink_EnableDisable(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewTUILogger(&buf, nil, 10, slog.LevelDebug)

	var sinkMessages []string
	logger.SetTUISink(func(s string) {
		sinkMessages = append(sinkMessages, s)
	})

	logger.PrintToTUI("to sink")
	assert.Len(t, sinkMessages, 1)
	assert.Empty(t, buf.String(), "writer should not receive when sink is set")

	// Disable sink
	logger.SetTUISink(nil)
	logger.PrintToTUI("to writer")
	assert.Len(t, sinkMessages, 1, "sink should not receive after disable")
	assert.Equal(t, "to writer\n", buf.String())
}

// ============================================================================
// session_id_common.go coverage gaps
// ============================================================================

// --- GetSessionID exported function (0% → 100%) ---

func TestGetSessionID_Empty(t *testing.T) {
	isolateEnv(t)

	id := GetSessionID("")
	assert.NotEmpty(t, id, "GetSessionID should return a non-empty ID even with no override")
	assert.Contains(t, id, "--", "ID should have namespace delimiter")
}

func TestGetSessionID_WithOverride(t *testing.T) {
	isolateEnv(t)

	id := GetSessionID("my-override")
	assert.True(t, strings.HasPrefix(id, "ex--my-override_"),
		"expected ex-- prefix with override payload, got %q", id)
}

// --- initializeStateManager error paths ---

func TestInitializeStateManager_UnknownBackend(t *testing.T) {
	t.Parallel()
	sessionID := testutil.NewTestSessionID("test", t.Name())

	sm, err := initializeStateManager(sessionID, "nonexistent-backend")
	assert.Nil(t, sm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create storage backend")
	assert.Contains(t, err.Error(), "nonexistent-backend")
}

func TestInitializeStateManager_EmptySessionID(t *testing.T) {
	t.Parallel()

	// The memory backend itself validates sessionID, so GetBackend fails first
	sm, err := initializeStateManager("", "memory")
	assert.Nil(t, sm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create storage backend")
}

func TestInitializeStateManager_Success_Memory(t *testing.T) {
	t.Parallel()
	sessionID := testutil.NewTestSessionID("test", t.Name())

	sm, err := initializeStateManager(sessionID, "memory")
	require.NoError(t, err)
	require.NotNil(t, sm)
	t.Cleanup(func() { _ = sm.Close() })

	assert.Equal(t, sessionID, sm.sessionID)
}

func TestInitializeStateManager_EnvOverride(t *testing.T) {
	sessionID := testutil.NewTestSessionID("test", t.Name())

	// Set OSM_STORE env to "memory" — should use memory backend
	t.Setenv("OSM_STORE", "memory")

	sm, err := initializeStateManager(sessionID, "") // empty override → reads env
	require.NoError(t, err)
	require.NotNil(t, sm)
	t.Cleanup(func() { _ = sm.Close() })
}

func TestInitializeStateManager_OverrideBeatsEnv(t *testing.T) {
	sessionID := testutil.NewTestSessionID("test", t.Name())

	// Set OSM_STORE to an invalid backend, but override with "memory"
	t.Setenv("OSM_STORE", "nonexistent")

	sm, err := initializeStateManager(sessionID, "memory")
	require.NoError(t, err, "explicit override should beat env var")
	require.NotNil(t, sm)
	t.Cleanup(func() { _ = sm.Close() })
}

func TestInitializeStateManager_EnvInvalid(t *testing.T) {
	sessionID := testutil.NewTestSessionID("test", t.Name())

	t.Setenv("OSM_STORE", "bogus-backend-xyz")

	sm, err := initializeStateManager(sessionID, "")
	assert.Nil(t, sm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus-backend-xyz")
}

func TestInitializeStateManager_BackendErrorCleansUp(t *testing.T) {
	// Register a custom backend that creates successfully but whose
	// LoadSession always errors. This tests that the deferred cleanup
	// in initializeStateManager properly closes the backend.
	backendName := "test-cleanup-" + testutil.NewTestSessionID("test", t.Name())

	closed := false
	storage.BackendRegistry[backendName] = func(sessionID string) (storage.StorageBackend, error) {
		return &cleanupTrackingBackend{onClose: func() { closed = true }}, nil
	}
	t.Cleanup(func() {
		delete(storage.BackendRegistry, backendName)
	})

	sm, err := initializeStateManager("valid-session", backendName)
	assert.Nil(t, sm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create state manager")
	assert.True(t, closed, "backend should be closed when NewStateManager fails")
}

func TestInitializeStateManager_LoadSessionError(t *testing.T) {
	// Register a custom backend factory that returns a backend whose
	// LoadSession always errors. This tests the NewStateManager error path
	// specifically when LoadSession fails.
	backendName := "test-error-backend-" + testutil.NewTestSessionID("test", t.Name())

	storage.BackendRegistry[backendName] = func(sessionID string) (storage.StorageBackend, error) {
		return &loadErrorBackend{}, nil
	}
	t.Cleanup(func() {
		delete(storage.BackendRegistry, backendName)
	})

	sm, err := initializeStateManager("valid-session-id", backendName)
	assert.Nil(t, sm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create state manager")
}

// loadErrorBackend is a storage backend whose LoadSession always errors.
type loadErrorBackend struct{}

func (b *loadErrorBackend) LoadSession(sessionID string) (*storage.Session, error) {
	return nil, assert.AnError
}

func (b *loadErrorBackend) SaveSession(session *storage.Session) error {
	return nil
}

func (b *loadErrorBackend) ArchiveSession(sessionID string, destPath string) error {
	return nil
}

func (b *loadErrorBackend) Close() error {
	return nil
}

// cleanupTrackingBackend is a storage backend that tracks Close() calls
// and always fails LoadSession so NewStateManager returns an error.
type cleanupTrackingBackend struct {
	onClose func()
}

func (b *cleanupTrackingBackend) LoadSession(sessionID string) (*storage.Session, error) {
	return nil, assert.AnError // force NewStateManager to fail
}

func (b *cleanupTrackingBackend) SaveSession(session *storage.Session) error {
	return nil
}

func (b *cleanupTrackingBackend) ArchiveSession(sessionID string, destPath string) error {
	return nil
}

func (b *cleanupTrackingBackend) Close() error {
	if b.onClose != nil {
		b.onClose()
	}
	return nil
}

// ============================================================================
// terminal.go coverage gaps
// ============================================================================

// NewTerminal is straightforward struct initialization.
// Run() is tested by swapping the TUIManager's reader with an in-memory
// reader that returns EOF immediately. go-prompt interprets EOF as a
// synthetic Ctrl-D, which causes an immediate clean exit when the input
// buffer is empty. This avoids requiring a real terminal or PTY while
// still exercising the full shutdown sequence (signal handler registration,
// session persistence, TUIManager close, terminal state restore guard).

func TestNewTerminal_StructInitialization(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	ctx := t.Context()

	sessionID := testutil.NewTestSessionID("test", t.Name())
	engine, err := NewEngineDeprecated(ctx, &buf, &buf, sessionID, "memory")
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close() })

	terminal := NewTerminal(ctx, engine)
	require.NotNil(t, terminal)
	assert.Equal(t, engine, terminal.engine)
	assert.NotNil(t, terminal.tuiManager)
	assert.Equal(t, ctx, terminal.ctx)
}

// TestTerminalRun_NormalExit_WithStateManager verifies that Terminal.Run()
// completes cleanly via EOF and persists the session when stateManager exists.
func TestTerminalRun_NormalExit_WithStateManager(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows — go-prompt uses different reader")
	}

	var engineOut bytes.Buffer
	ctx := t.Context()
	sessionID := testutil.NewTestSessionID("test", t.Name())

	engine, err := NewEngineDeprecated(ctx, &engineOut, &engineOut, sessionID, "memory")
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close() })

	// Replace the TUI I/O with in-memory adapters.
	// bytes.NewReader(nil) returns EOF on every Read, causing go-prompt
	// to synthesize Ctrl-D and exit cleanly (empty buffer → shouldExit).
	engine.tuiManager.reader = NewTUIReaderFromIO(bytes.NewReader(nil))
	var tuiOut bytes.Buffer
	engine.tuiManager.writer = NewTUIWriterFromIO(&tuiOut)

	terminal := NewTerminal(ctx, engine)

	done := make(chan struct{})
	go func() {
		defer close(done)
		terminal.Run()
	}()

	select {
	case <-done:
		// Run completed normally.
	case <-time.After(10 * time.Second):
		t.Fatal("Terminal.Run() did not exit within timeout")
	}

	output := tuiOut.String()
	assert.Contains(t, output, "Saving session...")
	assert.Contains(t, output, "Session saved successfully.")
}

// TestTerminalRun_NilStateManager verifies that Terminal.Run() skips
// session persistence when stateManager is nil.
func TestTerminalRun_NilStateManager(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows — go-prompt uses different reader")
	}

	var engineOut bytes.Buffer
	ctx := t.Context()
	sessionID := testutil.NewTestSessionID("test", t.Name())

	engine, err := NewEngineDeprecated(ctx, &engineOut, &engineOut, sessionID, "memory")
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close() })

	// Replace TUI I/O with in-memory adapters.
	engine.tuiManager.reader = NewTUIReaderFromIO(bytes.NewReader(nil))
	var tuiOut bytes.Buffer
	engine.tuiManager.writer = NewTUIWriterFromIO(&tuiOut)

	// Nil out stateManager. Terminal.Run() should skip persistence entirely.
	engine.tuiManager.stateManager = nil

	terminal := NewTerminal(ctx, engine)

	done := make(chan struct{})
	go func() {
		defer close(done)
		terminal.Run()
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Terminal.Run() did not exit within timeout")
	}

	output := tuiOut.String()
	assert.NotContains(t, output, "Saving session...")
	assert.NotContains(t, output, "Session saved successfully.")
}

// TestTerminalRun_PersistAndCloseErrors verifies error messages when
// PersistSession and TUIManager.Close both fail.
func TestTerminalRun_PersistAndCloseErrors(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows — go-prompt uses different reader")
	}

	var engineOut bytes.Buffer
	ctx := t.Context()
	sessionID := testutil.NewTestSessionID("test", t.Name())

	engine, err := NewEngineDeprecated(ctx, &engineOut, &engineOut, sessionID, "memory")
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close() })

	// Replace TUI I/O with in-memory adapters.
	engine.tuiManager.reader = NewTUIReaderFromIO(bytes.NewReader(nil))
	var tuiOut bytes.Buffer
	engine.tuiManager.writer = NewTUIWriterFromIO(&tuiOut)

	// Replace the backend with one whose SaveSession always fails.
	// This triggers both the PersistSession error path AND the Close error
	// path (because StateManager.Close also calls persistSessionInternal).
	sm := engine.tuiManager.stateManager.(*StateManager)
	sm.backend = &saveErrorBackend{}

	terminal := NewTerminal(ctx, engine)

	done := make(chan struct{})
	go func() {
		defer close(done)
		terminal.Run()
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Terminal.Run() did not exit within timeout")
	}

	output := tuiOut.String()
	assert.Contains(t, output, "Saving session...")
	assert.Contains(t, output, "Warning: Failed to persist session:")
	assert.Contains(t, output, "Warning: Failed to close TUI manager:")
}

// saveErrorBackend is a storage backend whose SaveSession always errors.
// Close and other operations succeed. Used to test error paths in
// Terminal.Run()'s shutdown sequence.
type saveErrorBackend struct{}

func (b *saveErrorBackend) LoadSession(string) (*storage.Session, error) {
	return &storage.Session{}, nil
}

func (b *saveErrorBackend) SaveSession(*storage.Session) error {
	return assert.AnError
}

func (b *saveErrorBackend) ArchiveSession(string, string) error {
	return nil
}

func (b *saveErrorBackend) Close() error {
	return nil
}

// ============================================================================
// debug_assertions_stub.go coverage
// ============================================================================

// The stub methods are no-ops compiled when -tags debug is NOT set.
// They are called through any TUIManager creation/usage. The exported
// var assignments and init() function ensure staticcheck doesn't warn.
// We explicitly call them here to ensure coverage.

func TestDebugAssertionStubs_NoOps(t *testing.T) {
	t.Parallel()

	// Create a minimal TUIManager for testing stubs
	tm := &TUIManager{}

	// All of these should be no-ops without panicking
	tm.debugWriteContextEnter()
	tm.debugWriteContextExit()
	tm.debugAssertNotInWriteContext("test message")
	tm.debugAssertInWriteContext("test message")
}

// ============================================================================
// discoverSessionID — all precedence paths
// ============================================================================

// These tests complement the existing session_id_common_test.go tests
// by exercising additional paths.

func TestDiscoverSessionID_EmptyOverride_FallsThrough(t *testing.T) {
	isolateEnv(t)

	id := discoverSessionID("")
	assert.NotEmpty(t, id)
	// Without any env vars, falls through to deep-anchor or UUID
	assert.Contains(t, id, "--")
}

func TestDiscoverSessionID_TMUXPaneWithoutSession(t *testing.T) {
	isolateEnv(t)

	// TMUX_PANE set but TMUX not set — getTmuxSessionID may fail
	// Falls through to next priority
	os.Setenv("TMUX_PANE", "%99")
	// Don't set TMUX

	id := discoverSessionID("")
	assert.NotEmpty(t, id)
	// Should fall through to UUID since no other env is set
}

func TestDiscoverSessionID_TermSessionID_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-only test")
	}
	isolateEnv(t)

	os.Setenv("TERM_SESSION_ID", "A1B2C3D4-E5F6-7890-ABCD-EF1234567890")

	id := discoverSessionID("")
	assert.NotEmpty(t, id)
	// On macOS with TERM_SESSION_ID, should use macos-terminal format
	// The format is term--{hash16} = 22 chars
	if len(id) != 22 {
		t.Logf("Expected term-- prefix (22 chars), got %d chars: %q", len(id), id)
	}
}

// ============================================================================
// Additional logging.go integration tests
// ============================================================================

func TestGetLogs_ReturnsCopy(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Info("entry1")
	logs := logger.GetLogs()
	require.Len(t, logs, 1)

	// Modifying returned slice should not affect internal state
	logs[0].Message = "modified"
	internalLogs := logger.GetLogs()
	assert.Equal(t, "entry1", internalLogs[0].Message)
}

func TestGetRecentLogs_ReturnsCopy(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Info("entry1")
	logs := logger.GetRecentLogs(1)
	require.Len(t, logs, 1)

	logs[0].Message = "modified"
	internalLogs := logger.GetRecentLogs(1)
	assert.Equal(t, "entry1", internalLogs[0].Message)
}

func TestSearchLogs_Empty(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	// No logs at all
	matches := logger.SearchLogs("anything")
	assert.Empty(t, matches)
}

func TestSearchLogs_AttributeKeyAndValueCaseInsensitive(t *testing.T) {
	t.Parallel()
	logger := NewTUILogger(nil, nil, 10, slog.LevelDebug)

	logger.Info("irrelevant", slog.String("MyKey", "MyValue"))

	// Search by key (case insensitive)
	matches := logger.SearchLogs("mykey")
	require.Len(t, matches, 1)

	// Search by value (case insensitive)
	matches = logger.SearchLogs("myvalue")
	require.Len(t, matches, 1)
}
