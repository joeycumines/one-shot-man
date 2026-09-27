package bubbletea

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"
)

// jsModel wraps a JavaScript model definition for bubbletea.
type jsModel struct {
	runtime   *goja.Runtime
	initFn    goja.Callable
	updateFn  goja.Callable
	viewFn    goja.Callable
	state     goja.Value
	initError string   // Store init error for debugging
	jsRunner  JSRunner // Optional: thread-safe JS execution via event loop

	// Render throttling state (optional, opt-in feature)
	throttleEnabled    bool            // Whether throttling is enabled (default: false)
	throttleIntervalMs int64           // Minimum ms between renders (default: 16)
	alwaysRenderTypes  map[string]bool // Message types that force immediate render
	cachedView         string          // Cached view output
	lastRenderTime     time.Time       // Time of last actual render
	forceNextRender    bool            // Force next render (e.g., for Tick messages)
	throttleMu         sync.Mutex      // Protects throttle timer state
	throttleTimerSet   bool            // True if a delayed render is already scheduled
	program            *tea.Program    // Reference for sending delayed render messages

	// Throttle timer cancellation - prevents goroutine leak on program exit
	// Set when program starts, cancelled when program exits
	throttleCtx    context.Context
	throttleCancel context.CancelFunc

	// Cached declarative view fields for throttled renders.
	// When throttling, we must preserve the parsed tea.View fields
	// so that cursedRenderer sees consistent mode fields on each render.
	cachedViewAltScreen             bool
	cachedViewMouseMode             tea.MouseMode
	cachedViewReportFocus           bool
	cachedViewWindowTitle           string
	cachedViewCursor                *tea.Cursor
	cachedViewForegroundColor       string
	cachedViewBackgroundColor       string
	cachedViewKeyboardEnhancements  tea.KeyboardEnhancements
	cachedViewDisableBracketedPaste bool
	cachedViewProgressBar           *tea.ProgressBar
}

// runJSSync executes fn on the JS event loop and waits for completion.
// If the runner supports TryRunSync, recursion on the event-loop
// goroutine is executed directly to avoid self-deadlock.
func (m *jsModel) runJSSync(fn func(*goja.Runtime) error) error {
	if m.jsRunner == nil {
		return fmt.Errorf("bubbletea: js runner is nil")
	}
	ctx := m.throttleCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if tr, ok := m.jsRunner.(TrySyncJSRunner); ok {
		return tr.TryRunSync(ctx, m.runtime, fn)
	}
	return m.jsRunner.RunSync(ctx, fn)
}

// Init implements tea.Model.
// CRITICAL: This is called from BubbleTea's goroutine, NOT the event loop goroutine.
// JSRunner MUST be set to safely marshal JS execution to the event loop.
func (m *jsModel) Init() tea.Cmd {
	if m == nil || m.initFn == nil || m.runtime == nil {
		return nil
	}

	// JSRunner is MANDATORY - panic if not set.
	// This ensures thread-safe JS execution from BubbleTea's goroutine.
	if m.jsRunner == nil {
		panic("bubbletea: jsModel.Init called without JSRunner - this is a programming error; SetJSRunner must be called before running any BubbleTea program")
	}

	var cmd tea.Cmd
	err := m.runJSSync(func(vm *goja.Runtime) error {
		cmd = m.initDirect()
		return nil
	})
	if err != nil {
		m.initError = fmt.Sprintf("Init error (event loop): %v", err)
		return nil
	}
	return cmd
}

// initDirect performs the actual init call. MUST be called from event loop goroutine.
func (m *jsModel) initDirect() tea.Cmd {
	// Call JS init function to get initial state
	result, err := m.initFn(goja.Undefined())
	if err != nil {
		// Store the error so View can display it
		m.initError = fmt.Sprintf("Init error: %v", err)
		return nil
	}
	// Ensure we have a valid state (not nil)
	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		m.state = m.runtime.NewObject()
		m.initError = "Init returned nil/undefined"
		return nil
	}

	// Check if result is an array [state, cmd] (like update returns)
	resultObj := result.ToObject(m.runtime)
	if resultObj != nil && resultObj.ClassName() == "Array" {
		// Extract state from index 0
		if newState := resultObj.Get("0"); !goja.IsUndefined(newState) && !goja.IsNull(newState) {
			m.state = newState
		} else {
			m.state = m.runtime.NewObject()
		}
		// Extract command from index 1
		cmdVal := resultObj.Get("1")
		return m.valueToCmd(cmdVal)
	}

	// Otherwise, result is just the state object (no initial command)
	m.state = result
	return nil
}

// Update implements tea.Model.
// CRITICAL: This is called from BubbleTea's goroutine, NOT the event loop goroutine.
// JSRunner MUST be set to safely marshal JS execution to the event loop.
func (m *jsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m == nil || m.updateFn == nil || m.runtime == nil {
		return m, nil
	}

	// Handle renderRefreshMsg specially - it just forces the next view to render
	if _, ok := msg.(renderRefreshMsg); ok {
		m.throttleMu.Lock()
		m.throttleTimerSet = false // Timer has fired
		m.forceNextRender = true   // Force the next View() to actually render
		m.throttleMu.Unlock()
		return m, nil
	}

	jsMsg := m.msgToJS(msg)
	if jsMsg == nil {
		return m, nil
	}

	// Check if this message type should force an immediate render.
	// Paste events (PasteStart, Paste, PasteEnd) should always force render
	// since the textarea needs to update immediately with pasted content.
	if m.throttleEnabled {
		forceRender := false
		if m.alwaysRenderTypes != nil {
			if msgType, ok := jsMsg["type"].(string); ok && m.alwaysRenderTypes[msgType] {
				forceRender = true
			}
		}
		if !forceRender {
			// Always render for paste events
			if msgType, ok := jsMsg["type"].(string); ok {
				switch msgType {
				case "Paste", "PasteStart", "PasteEnd":
					forceRender = true
				}
			}
		}
		if forceRender {
			m.throttleMu.Lock()
			m.forceNextRender = true
			m.throttleMu.Unlock()
		}
	}

	// JSRunner is MANDATORY - panic if not set.
	// This ensures thread-safe JS execution from BubbleTea's goroutine.
	if m.jsRunner == nil {
		panic("bubbletea: jsModel.Update called without JSRunner - this is a programming error; SetJSRunner must be called before running any BubbleTea program")
	}

	var cmd tea.Cmd
	err := m.runJSSync(func(vm *goja.Runtime) error {
		cmd = m.updateDirect(jsMsg)
		return nil
	})
	if err != nil {
		// Event loop error - return current state unchanged
		slog.Error("bubbletea: Update: RunSync error, returning nil cmd (breaks tick loop!)", "error", err, "msgType", jsMsg["type"])
		return m, nil
	}
	return m, cmd
}

// updateDirect performs the actual update call. MUST be called from event loop goroutine.
func (m *jsModel) updateDirect(jsMsg map[string]any) tea.Cmd {
	// Ensure state is not nil before passing to JS
	state := m.state
	if state == nil || goja.IsUndefined(state) || goja.IsNull(state) {
		state = m.runtime.NewObject()
	}

	result, err := m.updateFn(goja.Undefined(), m.runtime.ToValue(jsMsg), state)
	if err != nil {
		slog.Error("bubbletea: updateDirect: JS update function error", "error", err, "msgType", jsMsg["type"])
		return nil
	}
	// Result should be [newState, cmd] array
	resultObj := result.ToObject(m.runtime)
	if resultObj == nil || resultObj.ClassName() != "Array" {
		slog.Error("bubbletea: updateDirect: update did not return [state, cmd] array", "resultObj", resultObj, "className", func() string {
			if resultObj == nil {
				return "nil"
			} else {
				return resultObj.ClassName()
			}
		}())
		return nil
	}

	// Extract new state
	if newState := resultObj.Get("0"); !goja.IsUndefined(newState) && !goja.IsNull(newState) {
		m.state = newState
	}

	// Extract command
	cmdVal := resultObj.Get("1")
	return m.valueToCmd(cmdVal)
}

// View implements tea.Model.
// CRITICAL: This is called from BubbleTea's goroutine, NOT the event loop goroutine.
// JSRunner MUST be set to safely marshal JS execution to the event loop.
//
// Render Throttling: When throttleEnabled is true, this method may return a cached
// view if the minimum interval has not elapsed. A delayed renderRefreshMsg is scheduled
// to ensure the view is eventually re-rendered.
func (m *jsModel) View() tea.View {
	if m == nil || m.viewFn == nil || m.runtime == nil {
		return tea.NewView("[BT] View: nil model/viewFn/runtime")
	}

	// If init had an error, show it
	if m.initError != "" {
		return tea.NewView("[BT] " + m.initError)
	}

	// Render throttling logic
	if m.throttleEnabled {
		m.throttleMu.Lock()
		now := time.Now()
		elapsed := now.Sub(m.lastRenderTime)
		intervalDur := time.Duration(m.throttleIntervalMs) * time.Millisecond

		// Check if we should throttle this render
		shouldThrottle := !m.forceNextRender && elapsed < intervalDur && m.cachedView != ""

		if shouldThrottle {
			slog.Debug("bubbletea: View throttling", "elapsed", elapsed, "interval", intervalDur, "cachedLen", len(m.cachedView))
			// Schedule a delayed render if not already scheduled
			if !m.throttleTimerSet && m.program != nil && m.throttleCtx != nil {
				m.throttleTimerSet = true
				delay := intervalDur - elapsed
				prog := m.program
				throttleCtx := m.throttleCtx
				go func() {
					timer := time.NewTimer(delay)
					defer timer.Stop()
					select {
					case <-timer.C:
						// Timer fired - send render refresh message
						// prog.Send is documented as safe to call even after program
						// exits (it becomes a no-op), but we check context to avoid
						// unnecessary work
						prog.Send(renderRefreshMsg{})
					case <-throttleCtx.Done():
						// Program is exiting - don't send, just return
						return
					}
				}()
			}
			cached := m.cachedView
			altScreen := m.cachedViewAltScreen
			mouseMode := m.cachedViewMouseMode
			reportFocus := m.cachedViewReportFocus
			windowTitle := m.cachedViewWindowTitle
			cursor := m.cachedViewCursor
			fgColor := m.cachedViewForegroundColor
			bgColor := m.cachedViewBackgroundColor
			kbEnhance := m.cachedViewKeyboardEnhancements
			disablePaste := m.cachedViewDisableBracketedPaste
			progressBar := m.cachedViewProgressBar
			m.throttleMu.Unlock()
			// Wire mode fields into the cached view so cursedRenderer.viewEquals
			// sees a change from the previous view and flushes output.
			// Without this, throttled renders return zero-valued mode fields,
			// so viewEquals compares AltScreen=false vs AltScreen=false (no change)
			// and flush() sends nothing to the PTY.
			v := tea.NewView(cached)
			v.AltScreen = altScreen
			v.MouseMode = mouseMode
			v.ReportFocus = reportFocus
			v.WindowTitle = windowTitle
			v.Cursor = cursor
			v.KeyboardEnhancements = kbEnhance
			v.DisableBracketedPasteMode = disablePaste
			v.ProgressBar = progressBar
			if fgColor != "" {
				v.ForegroundColor = parseColorValue(fgColor)
			}
			if bgColor != "" {
				v.BackgroundColor = parseColorValue(bgColor)
			}
			return v
		}

		// Will do an actual render - update state
		m.forceNextRender = false
		m.lastRenderTime = now
		m.throttleMu.Unlock()
	}

	// JSRunner is MANDATORY - panic if not set.
	// This ensures thread-safe JS execution from BubbleTea's goroutine.
	if m.jsRunner == nil {
		panic("bubbletea: jsModel.View called without JSRunner - this is a programming error; SetJSRunner must be called before running any BubbleTea program")
	}

	var viewStr string
	var viewAltScreen bool
	var viewMouseMode tea.MouseMode
	var viewReportFocus bool
	var viewWindowTitle string
	var viewCursor *tea.Cursor
	var viewForegroundColor string
	var viewBackgroundColor string
	var viewKeyboardEnhancements tea.KeyboardEnhancements
	var viewDisableBracketedPaste bool
	var viewProgressBar *tea.ProgressBar
	var hasViewFields bool // true if JS returned a declarative view object

	err := m.runJSSync(func(vm *goja.Runtime) error {
		result, err := m.viewDirectResult()
		if err != nil {
			viewStr = fmt.Sprintf("[BT] View error: %v", err)
			return nil
		}

		// If result is an object (not a primitive string), parse declarative fields.
		// This is the primary v2 mechanism for controlling terminal features.
		if result != nil && !goja.IsUndefined(result) && !goja.IsNull(result) {
			if obj := result.ToObject(vm); obj != nil && obj.ClassName() == "Object" {
				hasViewFields = true
				viewStr = getJSStringProp(obj, "content")
				if viewStr == "" {
					viewStr = result.String() // fallback
				}
				viewAltScreen = getJSBoolProp(obj, "altScreen")
				viewMouseMode = parseMouseModeProp(obj, "mouseMode")
				viewReportFocus = getJSBoolProp(obj, "reportFocus")
				viewWindowTitle = getJSStringProp(obj, "windowTitle")
				viewCursor = parseCursorProp(vm, obj)
				viewForegroundColor = getJSStringProp(obj, "foregroundColor")
				viewBackgroundColor = getJSStringProp(obj, "backgroundColor")
				viewKeyboardEnhancements = parseKeyboardEnhancementsProp(vm, obj)
				viewDisableBracketedPaste = getJSBoolProp(obj, "disableBracketedPasteMode")
				viewProgressBar = parseProgressBarProp(vm, obj)
				return nil
			}
		}

		// Plain string (or non-object): treat as content only (backward compatible).
		viewStr = result.String()
		if viewStr == "" {
			viewStr = "[BT] View returned empty string"
		}
		return nil
	})
	if err != nil {
		return tea.NewView(fmt.Sprintf("[BT] View error (event loop): %v", err))
	}

	// Cache the view if throttling is enabled.
	// For declarative view objects, we cache the parsed fields so that
	// cursedRenderer.viewEquals sees consistent mode fields on each render.
	if m.throttleEnabled {
		m.throttleMu.Lock()
		m.cachedView = viewStr
		m.cachedViewAltScreen = viewAltScreen
		m.cachedViewMouseMode = viewMouseMode
		m.cachedViewReportFocus = viewReportFocus
		m.cachedViewWindowTitle = viewWindowTitle
		m.cachedViewCursor = viewCursor
		m.cachedViewForegroundColor = viewForegroundColor
		m.cachedViewBackgroundColor = viewBackgroundColor
		m.cachedViewKeyboardEnhancements = viewKeyboardEnhancements
		m.cachedViewDisableBracketedPaste = viewDisableBracketedPaste
		m.cachedViewProgressBar = viewProgressBar
		m.throttleMu.Unlock()
	}

	v := tea.NewView(viewStr)
	if hasViewFields {
		// v2 declarative path: fields came from JS view() return object.
		v.AltScreen = viewAltScreen
		v.MouseMode = viewMouseMode
		v.ReportFocus = viewReportFocus
		v.WindowTitle = viewWindowTitle
		v.Cursor = viewCursor
		v.KeyboardEnhancements = viewKeyboardEnhancements
		v.DisableBracketedPasteMode = viewDisableBracketedPaste
		v.ProgressBar = viewProgressBar
		if viewForegroundColor != "" {
			v.ForegroundColor = parseColorValue(viewForegroundColor)
		}
		if viewBackgroundColor != "" {
			v.BackgroundColor = parseColorValue(viewBackgroundColor)
		}
	}
	return v
}

// viewDirectResult returns the raw JS value from the view function.
// This allows the caller to distinguish between string and object returns.
// MUST be called from event loop goroutine.
func (m *jsModel) viewDirectResult() (goja.Value, error) {
	// Ensure state is not nil before passing to JS
	state := m.state
	if state == nil || goja.IsUndefined(state) || goja.IsNull(state) {
		return goja.Null(), errors.New("state is nil/undefined")
	}

	result, err := m.viewFn(goja.Undefined(), state)
	if err != nil {
		return goja.Null(), err
	}
	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return goja.Null(), errors.New("view returned nil/undefined")
	}
	return result, nil
}
