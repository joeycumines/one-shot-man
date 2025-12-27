package tview

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"
)

type testFakeTerm struct {
	makeRawCount int
	restoreCount int
}

func (f *testFakeTerm) Read(p []byte) (int, error)              { return 0, io.EOF }
func (f *testFakeTerm) Write(p []byte) (int, error)             { return len(p), nil }
func (f *testFakeTerm) Close() error                            { return nil }
func (f *testFakeTerm) Fd() uintptr                             { return uintptr(0) }
func (f *testFakeTerm) MakeRaw() (*term.State, error)           { f.makeRawCount++; return &term.State{}, nil }
func (f *testFakeTerm) Restore(state *term.State) error         { f.restoreCount++; return nil }
func (f *testFakeTerm) GetSize() (width, height int, err error) { return 80, 24, nil }
func (f *testFakeTerm) IsTerminal() bool                        { return true }

func TestRequire_ExportsCorrectAPI(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	// Call the require function
	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	// Verify exports
	exports := module.Get("exports").ToObject(vm)
	require.NotNil(t, exports)

	// Check that interactiveTable function is exported
	val := exports.Get("interactiveTable")
	assert.False(t, goja.IsUndefined(val), "Function interactiveTable should be exported")
	assert.False(t, goja.IsNull(val), "Function interactiveTable should not be null")
	_, ok := goja.AssertFunction(val)
	assert.True(t, ok, "Export interactiveTable should be a function")
}

func TestInteractiveTable_RequiresConfig(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	// Try to call interactiveTable without arguments
	exports := module.Get("exports").ToObject(vm)
	showTableFn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := showTableFn(goja.Undefined())
	require.NoError(t, err)

	// Should return an error message
	assert.Contains(t, result.String(), "requires a config object")
}

func TestInteractiveTable_HandlesNullConfig(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	exports := module.Get("exports").ToObject(vm)
	showTableFn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := showTableFn(goja.Undefined(), goja.Null())
	require.NoError(t, err)

	// Should return an error message
	assert.Contains(t, result.String(), "cannot be null or undefined")
}

func TestGetStringProp_HandlesDefaults(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(*goja.Runtime) *goja.Object
		propName     string
		defaultValue string
		expected     string
	}{
		{
			name: "nil object returns default",
			setup: func(vm *goja.Runtime) *goja.Object {
				return nil
			},
			propName:     "test",
			defaultValue: "default",
			expected:     "default",
		},
		{
			name: "undefined property returns default",
			setup: func(vm *goja.Runtime) *goja.Object {
				return vm.NewObject()
			},
			propName:     "missing",
			defaultValue: "default",
			expected:     "default",
		},
		{
			name: "null property returns default",
			setup: func(vm *goja.Runtime) *goja.Object {
				obj := vm.NewObject()
				_ = obj.Set("test", goja.Null())
				return obj
			},
			propName:     "test",
			defaultValue: "default",
			expected:     "default",
		},
		{
			name: "empty string returns default",
			setup: func(vm *goja.Runtime) *goja.Object {
				obj := vm.NewObject()
				_ = obj.Set("test", "   ")
				return obj
			},
			propName:     "test",
			defaultValue: "default",
			expected:     "default",
		},
		{
			name: "valid string is returned",
			setup: func(vm *goja.Runtime) *goja.Object {
				obj := vm.NewObject()
				_ = obj.Set("test", "value")
				return obj
			},
			propName:     "test",
			defaultValue: "default",
			expected:     "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := goja.New()
			obj := tt.setup(vm)
			result := getStringProp(obj, tt.propName, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestManager_Creation(t *testing.T) {
	manager := NewManager(t.Context(), nil, nil, nil, nil)
	assert.NotNil(t, manager)
	assert.True(t, t.Context() == manager.ctx)
	assert.True(t, manager.screen == nil)
	assert.NotNil(t, manager.signalNotify)
	assert.NotNil(t, manager.signalStop)
}

func TestTcellAdapter_Start_Idempotent(t *testing.T) {
	ft := &testFakeTerm{}
	adapter := NewTcellAdapter(ft)
	require.NoError(t, adapter.Start())
	require.NoError(t, adapter.Start(), "second Start() should be a no-op and not overwrite saved state")
	assert.Equal(t, 1, ft.makeRawCount)
	require.NoError(t, adapter.Stop())
	assert.Equal(t, 1, ft.restoreCount)
}

func TestTableConfig_Structure(t *testing.T) {
	config := TableConfig{
		Title:   "Test Table",
		Headers: []string{"Col1", "Col2"},
		Rows: []TableRow{
			{Cells: []string{"A", "B"}},
			{Cells: []string{"C", "D"}},
		},
		Footer: "Test Footer",
		OnSelect: func(rowIndex int) {
			// Test callback
		},
	}

	assert.Equal(t, "Test Table", config.Title)
	assert.Equal(t, 2, len(config.Headers))
	assert.Equal(t, 2, len(config.Rows))
	assert.NotNil(t, config.OnSelect)
}

func TestRequire_Integration(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()

	// Create a mock module with exports
	module := vm.NewObject()
	exports := vm.NewObject()
	require.NoError(t, module.Set("exports", exports))

	// Register the module
	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	// Verify the exports were set correctly
	exportsObj := module.Get("exports").ToObject(vm)
	require.NotNil(t, exportsObj)

	// Test that we can access the function
	interactiveTableVal := exportsObj.Get("interactiveTable")
	assert.False(t, goja.IsUndefined(interactiveTableVal))
}

func TestInteractiveTable_ValidConfig_NoActualDisplay(t *testing.T) {
	// This test verifies that a valid config is accepted
	// We cannot actually display the UI in tests, but we can verify the function accepts valid input
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	// Create a simulation screen for testing
	var simScreen tcell.SimulationScreen = &safeSimScreen{}
	err := simScreen.Init()
	require.NoError(t, err)
	t.Cleanup(simScreen.Fini)

	manager := NewManager(ctx, simScreen, nil, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)

	// Inject a 'q' key event after verifying the expected content is visible
	go func() {
		defer wg.Done()

		// Poll for expected content with reasonable timeout
		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()

		expectedContent := []string{"Test Table", "Column 1", "Column 2", "Value 1", "Value 2", "Test Footer"}

		for {
			select {
			case <-timeout:
				t.Error("Timeout waiting for expected content to appear")
				simScreen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
				cancel()
				return
			case <-ticker.C:
				cells, width, height := simScreen.GetContents()
				if width == 0 || height == 0 {
					continue
				}

				// Convert cells to string for content verification
				var content strings.Builder
				for _, cell := range cells {
					if len(cell.Runes) == 0 {
						continue
					}
					content.WriteRune(cell.Runes[0])
				}
				contentStr := content.String()

				// Check if all expected content is visible
				allFound := true
				for _, expected := range expectedContent {
					if !strings.Contains(contentStr, expected) {
						allFound = false
						break
					}
				}

				if allFound {
					t.Logf("Content verified: width=%d, height=%d", width, height)
					simScreen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
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

	// Create a valid config
	config := vm.NewObject()
	_ = config.Set("title", "Test Table")
	_ = config.Set("footer", "Test Footer")

	// Create headers array
	headers := vm.NewArray()
	_ = headers.Set("0", "Column 1")
	_ = headers.Set("1", "Column 2")
	_ = headers.Set("length", 2)
	_ = config.Set("headers", headers)

	// Create rows array
	rows := vm.NewArray()
	row1 := vm.NewArray()
	_ = row1.Set("0", "Value 1")
	_ = row1.Set("1", "Value 2")
	_ = row1.Set("length", 2)
	_ = rows.Set("0", row1)
	_ = rows.Set("length", 1)
	_ = config.Set("rows", rows)

	// Add onSelect callback
	_ = config.Set("onSelect", vm.ToValue(func(call goja.FunctionCall) goja.Value {
		// Callback would be called if table was displayed
		return goja.Undefined()
	}))

	exports := module.Get("exports").ToObject(vm)
	showTableFn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := showTableFn(goja.Undefined(), vm.ToValue(config))
	assert.NoError(t, err)
	assert.True(t, goja.IsUndefined(result))

	if !t.Failed() {
		wg.Wait()
	}
}

func TestInteractiveTable_WithoutOnSelect(t *testing.T) {
	// Test that onSelect is truly optional

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	// Create a simulation screen for testing
	var simScreen tcell.SimulationScreen = &safeSimScreen{}
	err := simScreen.Init()
	require.NoError(t, err)
	t.Cleanup(simScreen.Fini)

	manager := NewManager(ctx, simScreen, nil, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)

	// Inject a 'q' key event after verifying the expected content is visible
	go func() {
		defer wg.Done()

		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()

		expectedContent := []string{"Test Table", "Column 1", "Value 1", "Test Footer"}

		for {
			select {
			case <-timeout:
				t.Error("Timeout waiting for expected content to appear")
				simScreen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
				cancel()
				return
			case <-ticker.C:
				cells, width, height := simScreen.GetContents()
				if width == 0 || height == 0 {
					continue
				}

				var content strings.Builder
				for _, cell := range cells {
					if len(cell.Runes) == 0 {
						continue
					}
					content.WriteRune(cell.Runes[0])
				}
				contentStr := content.String()

				allFound := true
				for _, expected := range expectedContent {
					if !strings.Contains(contentStr, expected) {
						allFound = false
						break
					}
				}

				if allFound {
					t.Logf("Content verified: width=%d, height=%d", width, height)
					simScreen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
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

	// Create a valid config without onSelect
	config := vm.NewObject()
	_ = config.Set("title", "Test Table")
	_ = config.Set("footer", "Test Footer")

	headers := vm.NewArray()
	_ = headers.Set("0", "Column 1")
	_ = headers.Set("length", 1)
	_ = config.Set("headers", headers)

	rows := vm.NewArray()
	row1 := vm.NewArray()
	_ = row1.Set("0", "Value 1")
	_ = row1.Set("length", 1)
	_ = rows.Set("0", row1)
	_ = rows.Set("length", 1)
	_ = config.Set("rows", rows)

	// No onSelect callback

	exports := module.Get("exports").ToObject(vm)
	showTableFn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := showTableFn(goja.Undefined(), vm.ToValue(config))
	assert.NoError(t, err)
	assert.True(t, goja.IsUndefined(result))

	if !t.Failed() {
		wg.Wait()
	}
}

// safeSimScreen is a lightweight, test-only implementation of a tcell simulation
// screen that is fully synchronized and does not start internal goroutines.
type safeSimScreen struct {
	mu       sync.Mutex
	width    int
	height   int
	cells    []tcell.SimCell
	events   chan tcell.Event
	finiOnce sync.Once
	inited   bool
}

func (s *safeSimScreen) Init() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inited {
		return nil
	}
	s.width = 80
	s.height = 25
	s.cells = make([]tcell.SimCell, s.width*s.height)
	s.events = make(chan tcell.Event, 16)
	s.inited = true
	return nil
}

func (s *safeSimScreen) Fini() {
	s.finiOnce.Do(func() {
		s.mu.Lock()
		// signal any waiting PollEvent/ChannelEvents readers to wake up
		if s.events != nil {
			select {
			case s.events <- tcell.NewEventInterrupt(nil):
			default:
			}
		}
		s.inited = false
		s.mu.Unlock()
	})
}

func (s *safeSimScreen) Show() {
	// No-op, tview will call SetContent which we capture.
}

func (s *safeSimScreen) InjectKey(k tcell.Key, ch rune, mod tcell.ModMask) {
	// Avoid holding the mutex while sending to the events channel.
	// Acquire the channel reference under lock, then perform the (possibly
	// blocking) send without the lock held to prevent deadlocks where the
	// event consumer needs the same lock to process events.
	s.mu.Lock()
	chRef := s.events
	s.mu.Unlock()
	if chRef == nil {
		return
	}
	chRef <- tcell.NewEventKey(k, ch, mod)
}

func (s *safeSimScreen) PollEvent() tcell.Event {
	ev, ok := <-s.events
	if !ok {
		return nil
	}
	return ev
}

func (s *safeSimScreen) GetContents() ([]tcell.SimCell, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cells == nil {
		return nil, 0, 0
	}
	copied := make([]tcell.SimCell, len(s.cells))
	for i, c := range s.cells {
		var runesCopy []rune
		if len(c.Runes) > 0 {
			runesCopy = make([]rune, len(c.Runes))
			copy(runesCopy, c.Runes)
		}
		copied[i] = tcell.SimCell{Runes: runesCopy, Style: c.Style}
	}
	return copied, s.width, s.height
}

func (s *safeSimScreen) SetContent(x, y int, mainc rune, combc []rune, style tcell.Style) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x < 0 || y < 0 || x >= s.width || y >= s.height {
		return
	}
	idx := y*s.width + x
	runesCopy := make([]rune, 0)
	if len(combc) > 0 {
		runesCopy = append(runesCopy, combc...)
	}
	if mainc != 0 {
		runesCopy = append([]rune{mainc}, runesCopy...)
	}
	s.cells[idx] = tcell.SimCell{Runes: runesCopy, Style: style}
}

func (s *safeSimScreen) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cells == nil {
		return
	}
	for i := range s.cells {
		s.cells[i] = tcell.SimCell{}
	}
}

func (s *safeSimScreen) Size() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.width, s.height
}

func (s *safeSimScreen) Fill(r rune, style tcell.Style) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cells {
		var runesCopy []rune
		if r != 0 {
			runesCopy = []rune{r}
		}
		s.cells[i] = tcell.SimCell{Runes: runesCopy, Style: style}
	}
}

// Additional methods to fully satisfy tcell.Screen / SimulationScreen interfaces
func (s *safeSimScreen) Put(x int, y int, str string, style tcell.Style) (string, int) {
	if len(str) == 0 {
		return "", 0
	}
	// Put first grapheme only
	r := []rune(str)[0]
	s.SetContent(x, y, r, nil, style)
	return str[1:], 1
}

func (s *safeSimScreen) PutStr(x int, y int, str string) {
	// basic implementation
	r := []rune(str)
	for i, ch := range r {
		s.SetContent(x+i, y, ch, nil, tcell.StyleDefault)
	}
}

func (s *safeSimScreen) PutStrStyled(x int, y int, str string, style tcell.Style) {
	r := []rune(str)
	for i, ch := range r {
		s.SetContent(x+i, y, ch, nil, style)
	}
}

func (s *safeSimScreen) SetCell(x int, y int, style tcell.Style, ch ...rune) {
	var mainc rune
	var comb []rune
	if len(ch) > 0 {
		mainc = ch[0]
		if len(ch) > 1 {
			comb = ch[1:]
		}
	}
	s.SetContent(x, y, mainc, comb, style)
}

func (s *safeSimScreen) Get(x, y int) (string, tcell.Style, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x < 0 || y < 0 || x >= s.width || y >= s.height {
		return "", tcell.StyleDefault, 0
	}
	idx := y*s.width + x
	c := s.cells[idx]
	if len(c.Runes) == 0 {
		return "", c.Style, 0
	}
	return string(c.Runes), c.Style, 1
}

func (s *safeSimScreen) GetContent(x, y int) (rune, []rune, tcell.Style, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x < 0 || y < 0 || x >= s.width || y >= s.height {
		return 0, nil, tcell.StyleDefault, 0
	}
	idx := y*s.width + x
	c := s.cells[idx]
	var main rune
	if len(c.Runes) > 0 {
		main = c.Runes[0]
	}
	// Safely return the tail runes. Slicing c.Runes[1:] when len==0 will
	// panic, so ensure we only slice when there are elements to return.
	var tail []rune
	if len(c.Runes) > 1 {
		tail = make([]rune, len(c.Runes)-1)
		copy(tail, c.Runes[1:])
	} else {
		tail = nil
	}
	return main, tail, c.Style, 1
}

func (s *safeSimScreen) SetStyle(style tcell.Style) {
	// no-op for tests
}

func (s *safeSimScreen) ShowCursor(x int, y int) {
	// no-op
}

func (s *safeSimScreen) HideCursor() {
	// no-op
}

func (s *safeSimScreen) SetCursorStyle(style tcell.CursorStyle, _ ...tcell.Color) {
	// no-op
}

func (s *safeSimScreen) ChannelEvents(ch chan<- tcell.Event, quit <-chan struct{}) {
	for {
		select {
		case <-quit:
			close(ch)
			return
		case ev, ok := <-s.events:
			if !ok {
				close(ch)
				return
			}
			ch <- ev
		}
	}
}

func (s *safeSimScreen) HasPendingEvent() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events) > 0
}

func (s *safeSimScreen) PostEvent(ev tcell.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.events == nil {
		return nil
	}
	select {
	case s.events <- ev:
		return nil
	default:
		return tcell.ErrEventQFull
	}
}

func (s *safeSimScreen) PostEventWait(ev tcell.Event) {
	s.mu.Lock()
	ch := s.events
	s.mu.Unlock()
	if ch == nil {
		return
	}
	ch <- ev
}

func (s *safeSimScreen) EnableMouse(...tcell.MouseFlags) {}

func (s *safeSimScreen) DisableMouse() {}

func (s *safeSimScreen) EnablePaste() {}

func (s *safeSimScreen) DisablePaste() {}

func (s *safeSimScreen) EnableFocus() {}

func (s *safeSimScreen) DisableFocus() {}

func (s *safeSimScreen) HasMouse() bool { return false }

func (s *safeSimScreen) Colors() int { return 256 }

func (s *safeSimScreen) Sync() {}

func (s *safeSimScreen) CharacterSet() string { return "UTF-8" }

func (s *safeSimScreen) RegisterRuneFallback(r rune, subst string) {}

func (s *safeSimScreen) UnregisterRuneFallback(r rune) {}

func (s *safeSimScreen) CanDisplay(r rune, checkFallbacks bool) bool { return true }

func (s *safeSimScreen) Resize(int, int, int, int) {}

func (s *safeSimScreen) HasKey(tcell.Key) bool { return true }

func (s *safeSimScreen) Suspend() error { return nil }

func (s *safeSimScreen) Resume() error { return nil }

func (s *safeSimScreen) Beep() error { return nil }

func (s *safeSimScreen) SetSize(w, h int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w <= 0 || h <= 0 {
		s.width = w
		s.height = h
		s.cells = nil
		return
	}

	// Allocate a new buffer and copy existing contents into the new grid
	// preserving as much data as will fit in the new dimensions.
	oldW, oldH := s.width, s.height
	oldCells := s.cells

	newCells := make([]tcell.SimCell, w*h)
	// If there was no previous content, just assign the new cell buffer.
	if oldCells == nil || oldW == 0 || oldH == 0 {
		s.width = w
		s.height = h
		s.cells = newCells
		return
	}

	copyW := w
	if oldW < copyW {
		copyW = oldW
	}
	copyH := h
	if oldH < copyH {
		copyH = oldH
	}

	for yy := 0; yy < copyH; yy++ {
		for xx := 0; xx < copyW; xx++ {
			oldIdx := yy*oldW + xx
			newIdx := yy*w + xx
			// Make copies of runes to avoid aliasing the old slice.
			cell := oldCells[oldIdx]
			var runesCopy []rune
			if len(cell.Runes) > 0 {
				runesCopy = make([]rune, len(cell.Runes))
				copy(runesCopy, cell.Runes)
			}
			newCells[newIdx] = tcell.SimCell{Runes: runesCopy, Style: cell.Style}
		}
	}

	s.width = w
	s.height = h
	s.cells = newCells
}

func (s *safeSimScreen) LockRegion(x, y, width, height int, lock bool) {}

func (s *safeSimScreen) Tty() (tcell.Tty, bool) { return nil, false }

func (s *safeSimScreen) SetTitle(string) {}

func (s *safeSimScreen) SetClipboard([]byte) {}

func (s *safeSimScreen) GetClipboard() {}

// SimulationScreen-specific additions
func (s *safeSimScreen) InjectKeyBytes(buf []byte) bool { return true }

func (s *safeSimScreen) InjectMouse(x, y int, buttons tcell.ButtonMask, mod tcell.ModMask) {}

func (s *safeSimScreen) GetCursor() (x int, y int, visible bool) { return 0, 0, false }

func (s *safeSimScreen) GetTitle() string { return "" }

func (s *safeSimScreen) GetClipboardData() []byte { return nil }
