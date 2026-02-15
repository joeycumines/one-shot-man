package tview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/gdamore/tcell/v2"
	"github.com/joeycumines/one-shot-man/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"
)

// --- TcellAdapter coverage ---

// errTerm is a terminal mock that returns configurable errors.
type errTerm struct {
	makeRawErr  error
	restoreErr  error
	getSizeErr  error
	getWidth    int
	getHeight   int
	readData    []byte
	readErr     error
	writeErr    error
	closeErr    error
	readCalled  bool
	writeCalled bool
	closeCalled bool
}

func (t *errTerm) Read(p []byte) (int, error) {
	t.readCalled = true
	if t.readData != nil {
		n := copy(p, t.readData)
		t.readData = nil
		return n, t.readErr
	}
	if t.readErr != nil {
		return 0, t.readErr
	}
	return 0, io.EOF
}

func (t *errTerm) Write(p []byte) (int, error) {
	t.writeCalled = true
	if t.writeErr != nil {
		return 0, t.writeErr
	}
	return len(p), nil
}

func (t *errTerm) Close() error {
	t.closeCalled = true
	return t.closeErr
}

func (t *errTerm) Fd() uintptr                   { return 0 }
func (t *errTerm) IsTerminal() bool              { return true }
func (t *errTerm) MakeRaw() (*term.State, error) { return &term.State{}, t.makeRawErr }
func (t *errTerm) Restore(_ *term.State) error   { return t.restoreErr }
func (t *errTerm) GetSize() (int, int, error)    { return t.getWidth, t.getHeight, t.getSizeErr }

func TestTcellAdapter_Start_NilTerminal(t *testing.T) {
	adapter := NewTcellAdapter(nil)
	err := adapter.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "terminal not initialized")
}

func TestTcellAdapter_Start_MakeRawError(t *testing.T) {
	ft := &errTerm{makeRawErr: errors.New("raw mode failed")}
	adapter := NewTcellAdapter(ft)
	err := adapter.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to set raw mode")
}

func TestTcellAdapter_Stop_NilSavedState(t *testing.T) {
	ft := &testFakeTerm{}
	adapter := NewTcellAdapter(ft)
	// Stop without Start — savedState is nil
	err := adapter.Stop()
	assert.NoError(t, err)
	assert.Equal(t, 0, ft.restoreCount)
}

func TestTcellAdapter_Stop_RestoreError(t *testing.T) {
	ft := &errTerm{restoreErr: errors.New("restore failed")}
	adapter := NewTcellAdapter(ft)
	// Start to set savedState
	require.NoError(t, adapter.Start())
	err := adapter.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "restore failed")
	// savedState should be cleared even on error
	assert.Nil(t, adapter.savedState)
}

func TestTcellAdapter_NotifyResize(t *testing.T) {
	ft := &testFakeTerm{}
	adapter := NewTcellAdapter(ft)

	called := false
	cb := func() { called = true }
	adapter.NotifyResize(cb)

	// Verify callback was stored
	adapter.resizeMu.Lock()
	storedCb := adapter.resizeCb
	adapter.resizeMu.Unlock()
	assert.NotNil(t, storedCb)

	// Call it to prove it's the right one
	storedCb()
	assert.True(t, called)
}

func TestTcellAdapter_WindowSize(t *testing.T) {
	ft := &testFakeTerm{}
	adapter := NewTcellAdapter(ft)

	ws, err := adapter.WindowSize()
	require.NoError(t, err)
	assert.Equal(t, 80, ws.Width)
	assert.Equal(t, 24, ws.Height)
	assert.Equal(t, 0, ws.PixelWidth)
	assert.Equal(t, 0, ws.PixelHeight)
}

func TestTcellAdapter_WindowSize_Error(t *testing.T) {
	ft := &errTerm{getSizeErr: errors.New("size failed")}
	adapter := NewTcellAdapter(ft)

	_, err := adapter.WindowSize()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "size failed")
}

func TestTcellAdapter_Read(t *testing.T) {
	ft := &errTerm{readData: []byte("hello"), readErr: nil}
	adapter := NewTcellAdapter(ft)

	buf := make([]byte, 10)
	n, err := adapter.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(buf[:n]))
	assert.True(t, ft.readCalled)
}

func TestTcellAdapter_Read_EOF(t *testing.T) {
	ft := &errTerm{readErr: io.EOF}
	adapter := NewTcellAdapter(ft)

	buf := make([]byte, 10)
	_, err := adapter.Read(buf)
	assert.ErrorIs(t, err, io.EOF)
}

func TestTcellAdapter_Write(t *testing.T) {
	ft := &errTerm{}
	adapter := NewTcellAdapter(ft)

	n, err := adapter.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.True(t, ft.writeCalled)
}

func TestTcellAdapter_Write_Error(t *testing.T) {
	ft := &errTerm{writeErr: errors.New("write failed")}
	adapter := NewTcellAdapter(ft)

	_, err := adapter.Write([]byte("data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

func TestTcellAdapter_Close(t *testing.T) {
	ft := &testFakeTerm{}
	adapter := NewTcellAdapter(ft)

	// Close without Start — no savedState to restore
	err := adapter.Close()
	assert.NoError(t, err)
	assert.Equal(t, 0, ft.restoreCount)

	// Verify idempotent
	err = adapter.Close()
	assert.NoError(t, err)
}

func TestTcellAdapter_Close_WithSavedState(t *testing.T) {
	ft := &testFakeTerm{}
	adapter := NewTcellAdapter(ft)

	// Start to create savedState
	require.NoError(t, adapter.Start())
	assert.Equal(t, 1, ft.makeRawCount)

	// Close should restore
	err := adapter.Close()
	assert.NoError(t, err)
	assert.Equal(t, 1, ft.restoreCount)
}

func TestTcellAdapter_Close_ClearsResizeCallback(t *testing.T) {
	ft := &testFakeTerm{}
	adapter := NewTcellAdapter(ft)

	adapter.NotifyResize(func() {})

	err := adapter.Close()
	assert.NoError(t, err)

	adapter.resizeMu.Lock()
	assert.Nil(t, adapter.resizeCb)
	adapter.resizeMu.Unlock()
}

func TestTcellAdapter_Close_RestoreError(t *testing.T) {
	ft := &errTerm{restoreErr: errors.New("restore err")}
	adapter := NewTcellAdapter(ft)

	// Start to set savedState
	require.NoError(t, adapter.Start())

	err := adapter.Close()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "restore err")
}

// --- Manager.getOrCreateScreen coverage ---

func TestManager_GetOrCreateScreen_NoScreenNoTerminal(t *testing.T) {
	manager := NewManagerWithTerminal(t.Context(), nil, nil, nil, nil)
	screen, err := manager.getOrCreateScreen()
	assert.NoError(t, err)
	assert.Nil(t, screen) // Returns nil to let tview handle it
}

func TestManager_GetOrCreateScreen_WithScreen(t *testing.T) {
	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	defer sim.Fini()

	manager := NewManagerWithTerminal(t.Context(), sim, nil, nil, nil)
	screen, err := manager.getOrCreateScreen()
	assert.NoError(t, err)
	assert.Equal(t, sim, screen)
}

// --- Require edge cases ---

func TestRequire_UndefinedExports(t *testing.T) {
	// Test when module.exports is undefined — should create new exports
	ctx := context.Background()
	manager := NewManagerWithTerminal(ctx, nil, nil, nil, nil)

	vm := goja.New()
	module := vm.NewObject()
	// Don't set exports — leave it undefined

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	exports := module.Get("exports").ToObject(vm)
	require.NotNil(t, exports)

	// interactiveTable should still be exported
	val := exports.Get("interactiveTable")
	assert.False(t, goja.IsUndefined(val))
}

func TestInteractiveTable_UndefinedConfig(t *testing.T) {
	ctx := context.Background()
	manager := NewManagerWithTerminal(ctx, nil, nil, nil, nil)

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	exports := module.Get("exports").ToObject(vm)
	fn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := fn(goja.Undefined(), goja.Undefined())
	require.NoError(t, err)
	assert.Contains(t, result.String(), "cannot be null or undefined")
}

func TestInteractiveTable_EmptyConfig(t *testing.T) {
	// Config with no headers, no rows — uses defaults
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	go func() {
		time.Sleep(200 * time.Millisecond)
		sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	}()

	config := TableConfig{} // All zero values — empty table

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
}

func TestInteractiveTable_RowWithNullElements(t *testing.T) {
	// Test rows containing null/undefined values and non-array rows using Go API
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	go func() {
		time.Sleep(200 * time.Millisecond)
		sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	}()

	// Use Go API directly — the null-cell parsing is tested via the
	// existing Goja tests. This test covers the empty/sparse row path.
	config := TableConfig{
		Title:   "Test",
		Headers: []string{"Col"},
		Rows: []TableRow{
			{Cells: []string{"valid"}},
			{Cells: nil},          // nil cells
			{Cells: []string{""}}, // empty cell
		},
	}

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
}

func TestInteractiveTable_NoRows(t *testing.T) {
	// Table with headers but no rows — tests the skip-select path when rows is empty
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	go func() {
		time.Sleep(200 * time.Millisecond)
		sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	}()

	config := TableConfig{
		Title:   "Empty Table",
		Headers: []string{"Header"},
		Rows:    nil, // no rows — skip table.Select
	}

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
}

func TestInteractiveTable_EscapeKey(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(200 * time.Millisecond)
		sim.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
	}()

	config := TableConfig{
		Title:   "Esc Test",
		Headers: []string{"A"},
		Rows:    []TableRow{{Cells: []string{"val"}}},
	}

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
	wg.Wait()
}

func TestInteractiveTable_EnterKeyWithOnSelect(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	var selectedRow int = -1

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Wait for content then press Enter
		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				sim.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
				return
			case <-ticker.C:
				cells, w, h := sim.GetContents()
				if w == 0 || h == 0 || len(cells) == 0 {
					continue
				}
				var content strings.Builder
				for _, c := range cells {
					if len(c.Runes) > 0 {
						content.WriteRune(c.Runes[0])
					}
				}
				if strings.Contains(content.String(), "Val1") {
					sim.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
					return
				}
			}
		}
	}()

	config := TableConfig{
		Title:   "Enter Test",
		Headers: []string{"Col"},
		Rows:    []TableRow{{Cells: []string{"Val1"}}},
		OnSelect: func(rowIndex int) {
			selectedRow = rowIndex
		},
	}

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
	wg.Wait()

	// Row 0 (0-based data row) should be selected
	assert.Equal(t, 0, selectedRow)
}

func TestInteractiveTable_EnterKeyWithoutOnSelect(t *testing.T) {
	// Enter key should still stop the app even without onSelect
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				sim.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
				return
			case <-ticker.C:
				_, w, h := sim.GetContents()
				if w > 0 && h > 0 {
					sim.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
					return
				}
			}
		}
	}()

	config := TableConfig{
		Title:    "Enter Without Callback",
		Headers:  []string{"Col"},
		Rows:     []TableRow{{Cells: []string{"Val"}}},
		OnSelect: nil,
	}

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
	wg.Wait()
}

func TestInteractiveTable_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	// Cancel context after delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	config := TableConfig{
		Title:   "Cancel Test",
		Headers: []string{"A"},
		Rows:    []TableRow{{Cells: []string{"val"}}},
	}

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
}

func TestInteractiveTable_SignalStop(t *testing.T) {
	ctx := t.Context()
	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	var mu sync.Mutex
	var capturedCh chan<- os.Signal
	mockNotify := func(c chan<- os.Signal, sig ...os.Signal) {
		mu.Lock()
		capturedCh = c
		mu.Unlock()
	}
	mockStop := func(c chan<- os.Signal) {
		// no-op
	}

	manager := NewManagerWithTerminal(ctx, sim, nil, mockNotify, mockStop)

	go func() {
		// Wait for the signal channel to be registered, then send a signal
		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				return
			case <-ticker.C:
				mu.Lock()
				ch := capturedCh
				mu.Unlock()
				if ch != nil {
					ch <- os.Interrupt
					return
				}
			}
		}
	}()

	config := TableConfig{
		Title:   "Signal Test",
		Headers: []string{"A"},
		Rows:    []TableRow{{Cells: []string{"val"}}},
	}

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
}

func TestInteractiveTable_QUpperCase(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	go func() {
		time.Sleep(200 * time.Millisecond)
		sim.InjectKey(tcell.KeyRune, 'Q', tcell.ModNone) // uppercase Q
	}()

	config := TableConfig{
		Title:   "Q Test",
		Headers: []string{"A"},
		Rows:    []TableRow{{Cells: []string{"val"}}},
	}

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
}

func TestInteractiveTable_NonExitKeyPassthrough(t *testing.T) {
	// Non-exit keys should be passed through (e.g. arrow keys)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	go func() {
		time.Sleep(200 * time.Millisecond)
		// Send a non-exit key, then exit
		sim.InjectKey(tcell.KeyDown, 0, tcell.ModNone)
		time.Sleep(50 * time.Millisecond)
		sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	}()

	config := TableConfig{
		Title:   "Arrow Test",
		Headers: []string{"A"},
		Rows: []TableRow{
			{Cells: []string{"Row1"}},
			{Cells: []string{"Row2"}},
		},
	}

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
}

func TestInteractiveTable_NonExitRune(t *testing.T) {
	// Pressing a rune that is not 'q' or 'Q' should not exit
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	go func() {
		time.Sleep(200 * time.Millisecond)
		sim.InjectKey(tcell.KeyRune, 'x', tcell.ModNone)
		time.Sleep(50 * time.Millisecond)
		sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	}()

	config := TableConfig{
		Title:   "Rune Test",
		Headers: []string{"A"},
		Rows:    []TableRow{{Cells: []string{"val"}}},
	}

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
}

func TestGetOrCreateScreen_TerminalAdapterScreenCreationError(t *testing.T) {
	// Test that getOrCreateScreen handles terminfo screen creation failure
	// We can't easily test screen creation failure without providing a real
	// terminal (fd 0 from a fake won't have valid terminfo), but we verify
	// the error path exists. The terminal adapter wrapping is already tested
	// through the adapter tests above.

	// Test with a terminal that returns invalid FD
	ft := &errTerm{getWidth: 80, getHeight: 24}
	manager := NewManagerWithTerminal(t.Context(), nil, ft, nil, nil)

	// This will attempt to create a screen from the terminal adapter,
	// which may fail because the FD=0 has no valid terminfo
	screen, err := manager.getOrCreateScreen()
	if err != nil {
		// Expected on test environments where FD 0 isn't a real terminal
		assert.Contains(t, err.Error(), "screen")
	} else if screen != nil {
		// If it somehow succeeds (rare), that's fine too
		screen.Fini()
	}
}

func TestShowInteractiveTable_OSMTestReadyEnv(t *testing.T) {
	// Test the OSM_TEST_TVIEW_READY sentinel path
	t.Setenv("OSM_TEST_TVIEW_READY", "TestSentinel")

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
				return
			case <-ticker.C:
				cells, w, h := sim.GetContents()
				if w == 0 || h == 0 {
					continue
				}
				var content strings.Builder
				for _, c := range cells {
					if len(c.Runes) > 0 {
						content.WriteRune(c.Runes[0])
					}
				}
				if strings.Contains(content.String(), "TestSentinel") {
					sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
					return
				}
			}
		}
	}()

	config := TableConfig{
		Title:   "Title",
		Headers: []string{"A"},
		Rows:    []TableRow{{Cells: []string{"val"}}},
	}

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
	wg.Wait()
}

func TestShowInteractiveTable_OSMTestReadyEnv_Default(t *testing.T) {
	t.Setenv("OSM_TEST_TVIEW_READY", "1")

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	go func() {
		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
				return
			case <-ticker.C:
				cells, w, h := sim.GetContents()
				if w == 0 || h == 0 {
					continue
				}
				var content strings.Builder
				for _, c := range cells {
					if len(c.Runes) > 0 {
						content.WriteRune(c.Runes[0])
					}
				}
				if strings.Contains(content.String(), "OSM_TVIEW_READY") {
					sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
					return
				}
			}
		}
	}()

	config := TableConfig{
		Title:   "Title",
		Headers: []string{"A"},
		Rows:    []TableRow{{Cells: []string{"val"}}},
	}

	err := manager.ShowInteractiveTable(config)
	assert.NoError(t, err)
}

func TestInteractiveTable_HeadersAndRowsNull(t *testing.T) {
	// Test null headers/rows/onSelect parsed via Goja — content-polling pattern
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				t.Error("Timeout waiting for content")
				sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
				cancel()
				return
			case <-ticker.C:
				cells, w, h := sim.GetContents()
				if w == 0 || h == 0 {
					continue
				}
				var buf strings.Builder
				for _, c := range cells {
					if len(c.Runes) > 0 {
						buf.WriteRune(c.Runes[0])
					}
				}
				if strings.Contains(buf.String(), "NullTest") {
					sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
					return
				}
			}
		}
	}()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))
	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	config := vm.NewObject()
	_ = config.Set("title", "NullTest")
	_ = config.Set("headers", goja.Null())
	_ = config.Set("rows", goja.Null())
	_ = config.Set("onSelect", goja.Null())

	exports := module.Get("exports").ToObject(vm)
	fn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := fn(goja.Undefined(), vm.ToValue(config))
	assert.NoError(t, err)
	assert.True(t, goja.IsUndefined(result))

	if !t.Failed() {
		wg.Wait()
	}
}

func TestInteractiveTable_OnSelectNotAFunction(t *testing.T) {
	// onSelect is a string, not a function — should be silently ignored
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				t.Error("Timeout waiting for content")
				sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
				cancel()
				return
			case <-ticker.C:
				cells, w, h := sim.GetContents()
				if w == 0 || h == 0 {
					continue
				}
				var buf strings.Builder
				for _, c := range cells {
					if len(c.Runes) > 0 {
						buf.WriteRune(c.Runes[0])
					}
				}
				if strings.Contains(buf.String(), "OnSelectTest") {
					sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
					return
				}
			}
		}
	}()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))
	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	config := vm.NewObject()
	_ = config.Set("title", "OnSelectTest")

	// Need headers + rows so tview has content to render
	headers := vm.NewArray()
	_ = headers.Set("0", "Col")
	_ = headers.Set("length", 1)
	_ = config.Set("headers", headers)

	rows := vm.NewArray()
	row1 := vm.NewArray()
	_ = row1.Set("0", "Val")
	_ = row1.Set("length", 1)
	_ = rows.Set("0", row1)
	_ = rows.Set("length", 1)
	_ = config.Set("rows", rows)

	// onSelect is a STRING, not a function — tests the AssertFunction fallthrough
	_ = config.Set("onSelect", "not_a_function")

	exports := module.Get("exports").ToObject(vm)
	fn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := fn(goja.Undefined(), vm.ToValue(config))
	assert.NoError(t, err)
	assert.True(t, goja.IsUndefined(result))

	if !t.Failed() {
		wg.Wait()
	}
}

func TestInteractiveTable_GojaRowEdgeCases(t *testing.T) {
	// Tests Goja row-parsing edge cases: null rows, non-array rows, null cells
	// Covers lines 324 (null/undefined row → continue) and 328 (non-array → continue)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				t.Error("Timeout waiting for content")
				sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
				cancel()
				return
			case <-ticker.C:
				cells, w, h := sim.GetContents()
				if w == 0 || h == 0 {
					continue
				}
				var buf strings.Builder
				for _, c := range cells {
					if len(c.Runes) > 0 {
						buf.WriteRune(c.Runes[0])
					}
				}
				if strings.Contains(buf.String(), "RowEdge") {
					sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
					return
				}
			}
		}
	}()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))
	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	config := vm.NewObject()
	_ = config.Set("title", "RowEdge")

	headers := vm.NewArray()
	_ = headers.Set("0", "Col")
	_ = headers.Set("length", 1)
	_ = config.Set("headers", headers)

	rows := vm.NewArray()
	// Row 0: valid
	row0 := vm.NewArray()
	_ = row0.Set("0", "valid")
	_ = row0.Set("length", 1)
	_ = rows.Set("0", row0)
	// Row 1: null → line 324 continue
	_ = rows.Set("1", goja.Null())
	// Row 2: undefined → line 324 continue
	_ = rows.Set("2", goja.Undefined())
	// Row 3: non-array object → line 328 continue
	nonArr := vm.NewObject()
	_ = nonArr.Set("x", "y")
	_ = rows.Set("3", nonArr)
	// Row 4: array with null cell → cell skipped
	row4 := vm.NewArray()
	_ = row4.Set("0", goja.Null())
	_ = row4.Set("1", "ok")
	_ = row4.Set("length", 2)
	_ = rows.Set("4", row4)
	_ = rows.Set("length", 5)
	_ = config.Set("rows", rows)

	exports := module.Get("exports").ToObject(vm)
	fn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := fn(goja.Undefined(), vm.ToValue(config))
	assert.NoError(t, err)
	assert.True(t, goja.IsUndefined(result))

	if !t.Failed() {
		wg.Wait()
	}
}

func TestInteractiveTable_GojaOnSelectAsFunction(t *testing.T) {
	// Tests the Goja onSelect-is-a-function path (line 348)
	// Enter key selects a row and calls the onSelect callback
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	sim := &safeSimScreen{}
	require.NoError(t, sim.Init())
	t.Cleanup(sim.Fini)

	manager := NewManagerWithTerminal(ctx, sim, nil, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				t.Error("Timeout waiting for content")
				sim.InjectKey(tcell.KeyEscape, 0, tcell.ModNone)
				cancel()
				return
			case <-ticker.C:
				cells, w, h := sim.GetContents()
				if w == 0 || h == 0 {
					continue
				}
				var buf strings.Builder
				for _, c := range cells {
					if len(c.Runes) > 0 {
						buf.WriteRune(c.Runes[0])
					}
				}
				if strings.Contains(buf.String(), "FuncCB") {
					// Send Enter to select the row and trigger onSelect
					sim.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
					return
				}
			}
		}
	}()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))
	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	config := vm.NewObject()
	_ = config.Set("title", "FuncCB")

	headers := vm.NewArray()
	_ = headers.Set("0", "Col")
	_ = headers.Set("length", 1)
	_ = config.Set("headers", headers)

	rows := vm.NewArray()
	row1 := vm.NewArray()
	_ = row1.Set("0", "RowVal")
	_ = row1.Set("length", 1)
	_ = rows.Set("0", row1)
	_ = rows.Set("length", 1)
	_ = config.Set("rows", rows)

	// onSelect as a proper JS function — tests the goja.AssertFunction ok=true path
	var callbackRowIdx int
	callbackCalled := false
	_ = config.Set("onSelect", vm.ToValue(func(call goja.FunctionCall) goja.Value {
		callbackCalled = true
		callbackRowIdx = int(call.Argument(0).ToInteger())
		return goja.Undefined()
	}))

	exports := module.Get("exports").ToObject(vm)
	fn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := fn(goja.Undefined(), vm.ToValue(config))
	assert.NoError(t, err)
	assert.True(t, goja.IsUndefined(result))
	assert.True(t, callbackCalled, "onSelect callback should have been called")
	assert.Equal(t, 0, callbackRowIdx, "first data row selected should be index 0")

	if !t.Failed() {
		wg.Wait()
	}
}

func TestInteractiveTable_ShowInteractiveTableError(t *testing.T) {
	// Tests the error return path from ShowInteractiveTable (line 387)
	// When getOrCreateScreen fails, ShowInteractiveTable returns an error.
	platform := testutil.DetectPlatform(t)
	testutil.SkipIfWindows(t, platform, "tview uses Windows console API, TERM env var has no effect")
	t.Setenv("TERM", "osm-nonexistent-terminal-type")

	// Create a manager with a terminal (no screen) — getOrCreateScreen will try
	// NewTerminfoScreenFromTty which will fail because the TERM is invalid
	ft := &errTerm{getWidth: 80, getHeight: 24}
	manager := NewManagerWithTerminal(t.Context(), nil, ft, nil, nil)

	config := TableConfig{Title: "ErrTest"}

	err := manager.ShowInteractiveTable(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "screen")
}

func TestDefaultSignals(t *testing.T) {
	// Simply verify defaultSignals is non-empty (platform-dependent)
	assert.NotEmpty(t, defaultSignals)
	for _, sig := range defaultSignals {
		assert.NotNil(t, sig)
		assert.NotEmpty(t, fmt.Sprintf("%v", sig))
	}
}

func TestGetStringProp_GoNilValue(t *testing.T) {
	// Test when obj.Get returns Go nil (not goja.Undefined/Null)
	vm := goja.New()
	obj := vm.NewObject()
	// "missing" key is never set, obj.Get returns Go nil
	result := getStringProp(obj, "missing", "default")
	assert.Equal(t, "default", result)
}
