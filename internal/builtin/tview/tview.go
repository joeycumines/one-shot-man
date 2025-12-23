package tview

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.org/x/term"
)

// TerminalOps defines the interface for terminal operations.
// This matches the interface in internal/scripting/tui_io.go.
type TerminalOps interface {
	io.Reader
	io.Writer
	io.Closer

	// Fd returns the file descriptor of the underlying terminal.
	Fd() uintptr

	// MakeRaw puts the terminal into raw mode and returns the previous state.
	MakeRaw() (*term.State, error)

	// Restore restores the terminal to a previous state.
	Restore(state *term.State) error

	// GetSize returns the current terminal size (width, height).
	GetSize() (width, height int, err error)

	// IsTerminal returns true if the underlying resource is a terminal.
	IsTerminal() bool
}

// TcellAdapter implements tcell.Tty by wrapping a TerminalOps.
// This allows tcell/tview to use the shared terminal I/O from the engine.
type TcellAdapter struct {
	terminal   TerminalOps
	savedState *term.State
	resizeCb   func()
	resizeMu   sync.Mutex
	stopResize chan struct{}
	closeOnce  sync.Once
}

// NewTcellAdapter creates a new tcell.Tty adapter wrapping the given terminal.
func NewTcellAdapter(terminal TerminalOps) *TcellAdapter {
	return &TcellAdapter{
		terminal:   terminal,
		stopResize: make(chan struct{}),
	}
}

// Start puts the terminal into raw mode.
func (a *TcellAdapter) Start() error {
	if a.terminal == nil {
		return fmt.Errorf("terminal not initialized")
	}
	// If we've already put the terminal into raw mode, do nothing.
	// This prevents overwriting the original saved state on repeated Start() calls.
	if a.savedState != nil {
		return nil
	}
	state, err := a.terminal.MakeRaw()
	if err != nil {
		return fmt.Errorf("failed to set raw mode: %w", err)
	}
	a.savedState = state
	return nil
}

// Stop restores the terminal to its previous state.
func (a *TcellAdapter) Stop() error {
	if a.savedState == nil {
		return nil
	}
	err := a.terminal.Restore(a.savedState)
	a.savedState = nil
	return err
}

// Drain satisfies the tcell.Tty interface.
// We implement it as a no-op because the adapter bypass mechanism
// handles state transitions, and attempting to drain via FCNTL/IOCTL
// manually here is dangerous and platform-dependent.
func (a *TcellAdapter) Drain() error {
	return nil
}

// NotifyResize registers a callback for terminal resize events.
func (a *TcellAdapter) NotifyResize(cb func()) {
	a.resizeMu.Lock()
	defer a.resizeMu.Unlock()
	a.resizeCb = cb
}

// WindowSize returns the current terminal size.
func (a *TcellAdapter) WindowSize() (tcell.WindowSize, error) {
	width, height, err := a.terminal.GetSize()
	if err != nil {
		return tcell.WindowSize{}, err
	}
	return tcell.WindowSize{
		Width:  width,
		Height: height,
		// Pixel dimensions not available from term.GetSize
		PixelWidth:  0,
		PixelHeight: 0,
	}, nil
}

// Read implements io.Reader.
func (a *TcellAdapter) Read(p []byte) (n int, err error) {
	return a.terminal.Read(p)
}

// Write implements io.Writer.
func (a *TcellAdapter) Write(p []byte) (n int, err error) {
	return a.terminal.Write(p)
}

// Close implements io.Closer.
func (a *TcellAdapter) Close() error {
	var err error
	a.closeOnce.Do(func() {
		// Stop resize handler
		a.resizeMu.Lock()
		a.resizeCb = nil
		a.resizeMu.Unlock()
		select {
		case a.stopResize <- struct{}{}:
		default:
		}

		// Restore terminal state if needed
		if a.savedState != nil {
			err = a.Stop()
		}
		// Don't close the underlying terminal - it's shared
	})
	return err
}

// Compile-time check that TcellAdapter implements tcell.Tty
var _ tcell.Tty = (*TcellAdapter)(nil)

// Manager holds the tview application and related state per engine instance.
type Manager struct {
	ctx          context.Context
	mu           sync.Mutex
	screen       tcell.Screen // optional
	terminal     TerminalOps  // optional - for creating screen via adapter
	signalNotify func(c chan<- os.Signal, sig ...os.Signal)
	signalStop   func(c chan<- os.Signal)
}

// TableRow represents a row of data for the interactive table.
type TableRow struct {
	Cells []string
}

// TableConfig holds configuration for an interactive table.
type TableConfig struct {
	Title    string
	Headers  []string
	Rows     []TableRow
	Footer   string
	OnSelect func(rowIndex int) // Optional callback when a row is selected with Enter
}

// NewManager creates a new tview manager for an engine instance.
// The provided screen is optional and mainly for testing purposes.
// Similarly, custom signal handling functions can be provided for testing.
// The terminal parameter is optional - if provided and screen is nil,
// it will be used to create a screen via TcellAdapter.
func NewManager(
	ctx context.Context,
	screen tcell.Screen,
	terminal TerminalOps,
	signalNotify func(c chan<- os.Signal, sig ...os.Signal),
	signalStop func(c chan<- os.Signal),
) *Manager {
	return NewManagerWithTerminal(ctx, screen, terminal, signalNotify, signalStop)
}

// NewManagerWithTerminal creates a new tview manager with terminal ops support.
// If screen is nil and terminal is provided, a screen will be created using
// the TcellAdapter when needed.
func NewManagerWithTerminal(
	ctx context.Context,
	screen tcell.Screen,
	terminal TerminalOps,
	signalNotify func(c chan<- os.Signal, sig ...os.Signal),
	signalStop func(c chan<- os.Signal),
) *Manager {
	if signalNotify == nil {
		signalNotify = signal.Notify
	}
	if signalStop == nil {
		signalStop = signal.Stop
	}
	return &Manager{
		ctx:          ctx,
		screen:       screen,
		terminal:     terminal,
		signalNotify: signalNotify,
		signalStop:   signalStop,
	}
}

// getOrCreateScreen returns the configured screen or creates one using the terminal adapter.
// If no screen or terminal is configured, returns (nil, nil) to let tview create its own
// screen via tcell.NewScreen() internally. This is the correct production behavior.
func (m *Manager) getOrCreateScreen() (tcell.Screen, error) {
	if m.screen != nil {
		return m.screen, nil
	}
	if m.terminal != nil {
		adapter := NewTcellAdapter(m.terminal)
		screen, err := tcell.NewTerminfoScreenFromTty(adapter)
		if err != nil {
			// Attempt to restore terminal state if screen creation fails
			_ = adapter.Stop()
			return nil, fmt.Errorf("failed to create screen from terminal: %w", err)
		}
		if err := screen.Init(); err != nil {
			// Ensure we restore terminal state if initialization fails
			_ = adapter.Stop()
			return nil, fmt.Errorf("failed to initialize screen: %w", err)
		}

		return screen, nil
	}

	// No screen or terminal configured - return nil to let tview handle screen creation.
	// tview.Application.Run() will call tcell.NewScreen() internally, which:
	// - On Unix: Uses NewTerminfoScreen() -> opens /dev/tty internally
	// - On Windows: Falls back to NewConsoleScreen()
	// This is the behavior that works correctly with go-prompt.
	return nil, nil
}

// Require returns a CommonJS native module under "osm:tview".
// It exposes tview functionality for creating rich terminal UIs.
//
// The key design principle is that TUI components are:
// - Explicitly invoked by JavaScript code
// - Accept simple, trivially-wirable data structures
// - Implemented in Go for performance and testability
// - Extensible through configuration objects
//
// API (JS):
//
//	const tview = require('osm:tview');
//
//	// Interactive table - blocks until user exits (Escape/q) or selects (Enter)
//	tview.interactiveTable({
//	    title: "Context Items",
//	    headers: ["ID", "Type", "Label", "Status"],
//	    rows: [
//	        ["1", "file", "main.go", "ok"],
//	        ["2", "note", "Important note", "ok"],
//	    ],
//	    footer: "Press Escape or 'q' to close | Enter to edit | Arrow keys to navigate",
//	    onSelect: function(rowIndex) {
//	        // Called when user presses Enter on a row
//	        // rowIndex is 0-based (excluding header)
//	        output.print("Selected row: " + rowIndex);
//	    }
//	});
func Require(baseCtx context.Context, manager *Manager) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		// Get or create exports object
		exportsVal := module.Get("exports")
		var exports *goja.Object
		if goja.IsUndefined(exportsVal) || goja.IsNull(exportsVal) {
			exports = runtime.NewObject()
			_ = module.Set("exports", exports)
		} else {
			exports = exportsVal.ToObject(runtime)
		}

		// interactiveTable displays an interactive table with optional selection callback
		_ = exports.Set("interactiveTable", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue(fmt.Errorf("interactiveTable requires a config object"))
			}

			configVal := call.Argument(0)
			if goja.IsUndefined(configVal) || goja.IsNull(configVal) {
				return runtime.ToValue(fmt.Errorf("config cannot be null or undefined"))
			}

			config := configVal.ToObject(runtime)

			// Extract configuration
			title := getStringProp(config, "title", "Table")
			footer := getStringProp(config, "footer", "Press Escape or 'q' to close")

			// Extract headers
			var headers []string
			if headersVal := config.Get("headers"); !goja.IsUndefined(headersVal) && !goja.IsNull(headersVal) {
				if headersObj := headersVal.ToObject(runtime); headersObj != nil && headersObj.ClassName() == "Array" {
					length := int(headersObj.Get("length").ToInteger())
					headers = make([]string, 0, length)
					for i := 0; i < length; i++ {
						if val := headersObj.Get(fmt.Sprintf("%d", i)); !goja.IsUndefined(val) && !goja.IsNull(val) {
							headers = append(headers, val.String())
						}
					}
				}
			}

			// Extract rows
			var rows []TableRow
			if rowsVal := config.Get("rows"); !goja.IsUndefined(rowsVal) && !goja.IsNull(rowsVal) {
				if rowsObj := rowsVal.ToObject(runtime); rowsObj != nil && rowsObj.ClassName() == "Array" {
					length := int(rowsObj.Get("length").ToInteger())
					rows = make([]TableRow, 0, length)
					for i := 0; i < length; i++ {
						rowVal := rowsObj.Get(fmt.Sprintf("%d", i))
						if goja.IsUndefined(rowVal) || goja.IsNull(rowVal) {
							continue
						}
						rowObj := rowVal.ToObject(runtime)
						if rowObj == nil || rowObj.ClassName() != "Array" {
							continue
						}
						rowLength := int(rowObj.Get("length").ToInteger())
						cells := make([]string, 0, rowLength)
						for j := 0; j < rowLength; j++ {
							cellVal := rowObj.Get(fmt.Sprintf("%d", j))
							if !goja.IsUndefined(cellVal) && !goja.IsNull(cellVal) {
								cells = append(cells, cellVal.String())
							}
						}
						rows = append(rows, TableRow{Cells: cells})
					}
				}
			}

			// Extract optional onSelect callback
			var onSelect func(int)
			if onSelectVal := config.Get("onSelect"); !goja.IsUndefined(onSelectVal) && !goja.IsNull(onSelectVal) {
				if callable, ok := goja.AssertFunction(onSelectVal); ok {
					onSelect = func(rowIndex int) {
						// Call the JavaScript function with the row index
						_, _ = callable(goja.Undefined(), runtime.ToValue(rowIndex))
					}
				}
			}

			// Show the interactive table
			tableConfig := TableConfig{
				Title:    title,
				Headers:  headers,
				Rows:     rows,
				Footer:   footer,
				OnSelect: onSelect,
			}

			if err := manager.ShowInteractiveTable(tableConfig); err != nil {
				return runtime.ToValue(err.Error())
			}

			return goja.Undefined()
		})
	}
}

// ShowInteractiveTable displays an interactive table in the terminal.
func (m *Manager) ShowInteractiveTable(config TableConfig) error {
	ctx, cancel := context.WithCancelCause(m.ctx)
	defer cancel(nil)

	m.mu.Lock()
	defer m.mu.Unlock()

	app := tview.NewApplication()
	var stopOnce sync.Once
	defer stopOnce.Do(app.Stop)

	// Get or create screen using terminal adapter if available
	screen, err := m.getOrCreateScreen()
	if err != nil {
		return err
	}
	if screen != nil {
		app.SetScreen(screen)
	}

	table := tview.NewTable().
		SetBorders(true).
		SetFixed(1, 0).
		SetSelectable(true, false)

	// Set headers
	for col, header := range config.Headers {
		table.SetCell(0, col, tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignCenter).
			SetSelectable(false))
	}

	// Set rows
	for row, rowData := range config.Rows {
		for col, cell := range rowData.Cells {
			table.SetCell(row+1, col, tview.NewTableCell(cell).
				SetTextColor(tcell.ColorWhite).
				SetAlign(tview.AlignLeft))
		}
	}

	// Build title string - embed test sentinel if requested
	titleText := config.Title
	if v := os.Getenv("OSM_TEST_TVIEW_READY"); v != "" {
		sentinel := v
		if sentinel == "1" {
			sentinel = "OSM_TVIEW_READY"
		}
		// Embed sentinel at the START of the title so it's visible even if title is truncated
		titleText = "[" + sentinel + "] " + titleText
	}

	// Create flex layout with title and footer
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().
			SetText(titleText).
			SetTextAlign(tview.AlignCenter).
			SetTextColor(tcell.ColorGreen), 1, 0, false).
		AddItem(table, 0, 1, true).
		AddItem(tview.NewTextView().
			SetText(config.Footer).
			SetTextAlign(tview.AlignCenter).
			SetTextColor(tcell.ColorGray), 1, 0, false)

	// Set up key bindings
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			stopOnce.Do(app.Stop)
			return nil
		case tcell.KeyEnter:
			// Get current row (subtract 1 for header)
			row, _ := table.GetSelection()
			if row > 0 && config.OnSelect != nil {
				// Call onSelect with 0-based row index (excluding header)
				config.OnSelect(row - 1)
			}
			stopOnce.Do(app.Stop)
			return nil
		case tcell.KeyRune:
			if event.Rune() == 'q' || event.Rune() == 'Q' {
				stopOnce.Do(app.Stop)
				return nil
			}
		}
		return event
	})

	// Start on first data row (skip header)
	if len(config.Rows) > 0 {
		table.Select(1, 0)
	}

	// finish setting up the application
	app.SetRoot(flex, true)
	app.SetFocus(table)

	// register signal handler to stop the app
	sigCh := make(chan os.Signal, 1)
	m.signalNotify(sigCh, defaultSignals...)
	defer m.signalStop(sigCh)

	// stop signals
	// TODO: consider propagating TERM and QUIT to the main application
	go func() {
		// N.B. could use cancel w/ error to propagate reason for shutdown
		defer cancel(nil)
		select {
		case <-ctx.Done():
		case <-sigCh:
		}
		stopOnce.Do(app.Stop)
	}()

	// run the application, blocking until it exits
	if err := app.Run(); err != nil {
		return fmt.Errorf("failed to run interactive table UI: %w", err)
	}

	return nil
}

// getStringProp safely extracts a string property from a goja object.
func getStringProp(obj *goja.Object, name, defaultValue string) string {
	if obj == nil {
		return defaultValue
	}
	val := obj.Get(name)
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return defaultValue
	}
	str := val.String()
	if strings.TrimSpace(str) == "" {
		return defaultValue
	}
	return str
}
