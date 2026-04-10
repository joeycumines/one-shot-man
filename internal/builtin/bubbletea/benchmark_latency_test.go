package bubbletea

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dop251/goja"
)

// ============================================================================
// INPUT LATENCY BENCHMARKS
// ============================================================================
//
// These benchmarks measure the performance of critical path operations
// in the BubbleTea/JS bridge to identify potential bottlenecks causing
// input latency in scripts like example-04-bt-shooter.js.
//
// Run with: go test -bench=. -benchmem ./internal/builtin/bubbletea/
//
// Key metrics:
//   - ns/op: Nanoseconds per operation (latency)
//   - B/op: Bytes allocated per operation (GC pressure)
//   - allocs/op: Number of allocations per operation
// ============================================================================

// BenchmarkJsToTeaMsg_KeyMsg measures the time to convert a JS key event
// object to a Go tea.KeyMsg. This is called for EVERY key press.
func BenchmarkJsToTeaMsg_KeyMsg(b *testing.B) {
	runtime := goja.New()

	// Create a realistic key event object (simulating what JS would send)
	keyEventJS := `({
		type: 'Key',
		key: 'w',
		runes: ['w'],
		alt: false,
		ctrl: false,
		paste: false
	})`

	val, err := runtime.RunString(keyEventJS)
	if err != nil {
		b.Fatalf("Failed to create key event: %v", err)
	}
	obj := val.ToObject(runtime)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msg := JsToTeaMsg(runtime, obj)
		if msg == nil {
			b.Fatal("JsToTeaMsg returned nil")
		}
		// Prevent compiler optimization
		_ = msg
	}
}

// BenchmarkJsToTeaMsg_MouseMsg measures mouse event conversion time.
func BenchmarkJsToTeaMsg_MouseMsg(b *testing.B) {
	runtime := goja.New()

	mouseEventJS := `({
		type: 'MouseClick',
		x: 40,
		y: 12,
		button: 'left',
		mod: []
	})`

	val, err := runtime.RunString(mouseEventJS)
	if err != nil {
		b.Fatalf("Failed to create mouse event: %v", err)
	}
	obj := val.ToObject(runtime)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msg := JsToTeaMsg(runtime, obj)
		_ = msg
	}
}

// BenchmarkJsToTeaMsg_WindowSizeMsg measures window size event conversion.
func BenchmarkJsToTeaMsg_WindowSizeMsg(b *testing.B) {
	runtime := goja.New()

	sizeEventJS := `({
		type: 'WindowSize',
		width: 80,
		height: 24
	})`

	val, err := runtime.RunString(sizeEventJS)
	if err != nil {
		b.Fatalf("Failed to create size event: %v", err)
	}
	obj := val.ToObject(runtime)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msg := JsToTeaMsg(runtime, obj)
		_ = msg
	}
}

// BenchmarkMsgToJS_KeyMsg measures the time to convert a Go KeyMsg to JS.
// This is called in jsModel.Update for every message.
func BenchmarkMsgToJS_KeyMsg(b *testing.B) {
	runtime := goja.New()

	model := &jsModel{
		runtime: runtime,
	}

	// Create a realistic tea.KeyPressMsg
	keyMsg := tea.KeyPressMsg{Text: "w"}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		jsMsg := model.msgToJS(keyMsg)
		if jsMsg == nil {
			b.Fatal("msgToJS returned nil")
		}
		_ = jsMsg
	}
}

// BenchmarkMsgToJS_TickMsg measures tick message conversion.
// Tick messages are sent every 16ms in the shooter game.
func BenchmarkMsgToJS_TickMsg(b *testing.B) {
	runtime := goja.New()

	model := &jsModel{
		runtime: runtime,
	}

	tick := tickMsg{
		id:   "gameTick",
		time: time.Now(),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		jsMsg := model.msgToJS(tick)
		_ = jsMsg
	}
}

// BenchmarkValueToCmd_Quit measures quit command extraction time.
func BenchmarkValueToCmd_Quit(b *testing.B) {
	runtime := goja.New()

	model := &jsModel{
		runtime:     runtime,
		validCmdIDs: make(map[uint64]bool),
	}

	cmdJS := `({ _cmdType: 'quit', _cmdID: 1 })`
	val, err := runtime.RunString(cmdJS)
	if err != nil {
		b.Fatalf("Failed to create command: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cmd := model.valueToCmd(val)
		_ = cmd
	}
}

// BenchmarkValueToCmd_Tick measures tick command extraction time.
// Tick commands are returned frequently by update().
func BenchmarkValueToCmd_Tick(b *testing.B) {
	runtime := goja.New()

	model := &jsModel{
		runtime:     runtime,
		validCmdIDs: make(map[uint64]bool),
	}

	cmdJS := `({ _cmdType: 'tick', _cmdID: 1, duration: 16, id: 'gameTick' })`
	val, err := runtime.RunString(cmdJS)
	if err != nil {
		b.Fatalf("Failed to create command: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cmd := model.valueToCmd(val)
		_ = cmd
	}
}

// BenchmarkValueToCmd_Batch measures batch command extraction time.
// Batch commands are used when multiple commands are returned.
func BenchmarkValueToCmd_Batch(b *testing.B) {
	runtime := goja.New()

	model := &jsModel{
		runtime:     runtime,
		validCmdIDs: make(map[uint64]bool),
	}

	// Batch with 3 commands (common case)
	cmdJS := `({
		_cmdType: 'batch',
		_cmdID: 1,
		cmds: [
			{ _cmdType: 'tick', _cmdID: 2, duration: 16, id: 'tick1' },
			{ _cmdType: 'tick', _cmdID: 3, duration: 100, id: 'tick2' },
			{ _cmdType: 'clearScreen', _cmdID: 4 }
		]
	})`
	val, err := runtime.RunString(cmdJS)
	if err != nil {
		b.Fatalf("Failed to create command: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cmd := model.valueToCmd(val)
		_ = cmd
	}
}

// BenchmarkParseKey measures ParseKey performance for common keys.
// ParseKey is called during JsToTeaMsg for every key event.
func BenchmarkParseKey_Rune(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		key, ok := ParseKey("w")
		_ = key
		_ = ok
	}
}

func BenchmarkParseKey_Special(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		key, ok := ParseKey("up")
		_ = key
		_ = ok
	}
}

func BenchmarkParseKey_Modifier(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		key, ok := ParseKey("ctrl+c")
		_ = key
		_ = ok
	}
}

// ============================================================================
// Full Pipeline Benchmarks
// ============================================================================

// BenchmarkFullKeyPipeline simulates the full path of a key event:
// 1. Create JS key event object
// 2. Convert to Go tea.KeyMsg (JsToTeaMsg)
// 3. Convert back to JS (msgToJS)
// 4. Extract command from response (valueToCmd with tick)
func BenchmarkFullKeyPipeline(b *testing.B) {
	runtime := goja.New()

	model := &jsModel{
		runtime:     runtime,
		validCmdIDs: make(map[uint64]bool),
	}

	// Pre-create the key event and response command
	keyEventJS := `({ type: 'Key', key: 'w', runes: ['w'], alt: false, ctrl: false, paste: false })`
	keyVal, _ := runtime.RunString(keyEventJS)
	keyObj := keyVal.ToObject(runtime)

	tickCmdJS := `({ _cmdType: 'tick', _cmdID: 1, duration: 16, id: 'gameTick' })`
	tickVal, _ := runtime.RunString(tickCmdJS)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Step 1: JS object → Go tea.Msg (input direction)
		msg := JsToTeaMsg(runtime, keyObj)

		// Step 2: Go tea.Msg → JS object (for update callback)
		jsMsg := model.msgToJS(msg)
		_ = jsMsg

		// Step 3: Extract command from JS response
		cmd := model.valueToCmd(tickVal)
		_ = cmd
	}
}

// BenchmarkFullTickPipeline simulates the tick message path:
// 1. Create tickMsg
// 2. Convert to JS (msgToJS)
// 3. Extract tick command from response (valueToCmd)
func BenchmarkFullTickPipeline(b *testing.B) {
	runtime := goja.New()

	model := &jsModel{
		runtime:     runtime,
		validCmdIDs: make(map[uint64]bool),
	}

	tick := tickMsg{
		id:   "gameTick",
		time: time.Now(),
	}

	tickCmdJS := `({ _cmdType: 'tick', _cmdID: 1, duration: 16, id: 'gameTick' })`
	tickVal, _ := runtime.RunString(tickCmdJS)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Step 1: Convert tick to JS
		jsMsg := model.msgToJS(tick)
		_ = jsMsg

		// Step 2: Extract command from response
		cmd := model.valueToCmd(tickVal)
		_ = cmd
	}
}

// ============================================================================
// Object Creation Benchmarks (GC Pressure Analysis)
// ============================================================================

// BenchmarkNewGojaObject measures the cost of creating new goja objects.
// This happens frequently during message conversion.
func BenchmarkNewGojaObject(b *testing.B) {
	runtime := goja.New()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		obj := runtime.NewObject()
		_ = obj
	}
}

// BenchmarkGojaToValue measures the cost of ToValue conversions.
func BenchmarkGojaToValue_Map(b *testing.B) {
	runtime := goja.New()

	m := map[string]any{
		"type":  "Key",
		"key":   "w",
		"runes": []string{"w"},
		"alt":   false,
		"ctrl":  false,
		"paste": false,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		val := runtime.ToValue(m)
		_ = val
	}
}

// ============================================================================
// Comparative Benchmarks (Native Go vs Bridge)
// ============================================================================

// BenchmarkNativeTeaKeyMsg shows the baseline cost of creating a KeyMsg.
func BenchmarkNativeTeaKeyMsg(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		msg := tea.KeyPressMsg{Text: "w"}
		_ = msg
	}
}

// BenchmarkNativeTeaTick shows the baseline cost of tea.Tick.
func BenchmarkNativeTeaTick(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cmd := tea.Tick(16*time.Millisecond, func(t time.Time) tea.Msg {
			return tickMsg{id: "tick", time: t}
		})
		_ = cmd
	}
}

// ============================================================================
// Context/Concurrency Benchmarks
// ============================================================================

// BenchmarkContextCheck measures the cost of context cancellation checks.
// This is done on every update cycle.
func BenchmarkContextCheck(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		select {
		case <-ctx.Done():
		default:
		}
	}
}

// BenchmarkContextWithCancel measures context creation cost.
func BenchmarkContextWithCancel(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, cancel := context.WithCancel(ctx)
		cancel()
	}
}

// ============================================================================
// Render Pipeline Benchmarks
// ============================================================================
//
// Profiling notes (T059, 2026-02-15):
//
// These benchmarks measure the View() rendering path through the JS bridge.
//
// Key findings:
//   - FullKeyPipeline: ~1.0µs/op (1584 B, 29 allocs) — msg→JS→cmd total
//   - App code accounts for 3.7% of CPU (pprof, 2s profile)
//   - All hotspots reside in goja internals (Object.Get, Export, stringKeys)
//   - msgToJS: 0.59%, ParseKey: 0.39%, valueToCmd: 0.2% — negligible
//   - No app-level optimization targets exist; costs are inherent to JS interop
//
// For context against other subsystems:
//   - Engine startup (T056): ~134µs total, <0.3% app code
//   - Session I/O (T057): ~5.4ms write (fsync-dominated), 1.54% app code
//   - BT bridge (T058): ~2.6µs update cycle, <1% app code
//
// Conclusion: the bubbletea render pipeline is dominated by goja VM costs.
// All per-interaction operations are sub-microsecond for app code.

// BenchmarkViewDirect measures the cost of calling View() via SyncJSRunner.
// This is the full render path including JS function call, state passing, and
// string extraction.
func BenchmarkViewDirect(b *testing.B) {
	runtime := goja.New()

	state := runtime.NewObject()
	_ = state.Set("count", 42)
	_ = state.Set("label", "hello")

	model := &jsModel{
		runtime: runtime,
		viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			// Simulate a realistic view function that reads state
			s := args[0].ToObject(runtime)
			count := s.Get("count").ToInteger()
			label := s.Get("label").String()
			_ = count
			_ = label
			return runtime.ToValue("Count: 42 | Label: hello\nPress q to quit"), nil
		},
		state:    state,
		jsRunner: &SyncJSRunner{Runtime: runtime},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		output := model.View()
		if output.Content == "" {
			b.Fatal("View returned empty string")
		}
		_ = output
	}
}

// BenchmarkViewDirect_Throttled measures View() with render throttling enabled.
// After the first call, subsequent calls should return the cached view.
func BenchmarkViewDirect_Throttled(b *testing.B) {
	runtime := goja.New()

	state := runtime.NewObject()
	_ = state.Set("count", 42)

	model := &jsModel{
		runtime:            runtime,
		throttleEnabled:    true,
		throttleIntervalMs: 1000, // Long interval to ensure caching
		viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return runtime.ToValue("cached view output"), nil
		},
		state:    state,
		jsRunner: &SyncJSRunner{Runtime: runtime},
	}

	// Prime the cache with the first render
	model.View()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		output := model.View()
		_ = output
	}
}

// BenchmarkFullUpdateCycle measures the complete msg → Update → View cycle.
// This simulates what happens every frame in a TUI application.
func BenchmarkFullUpdateCycle(b *testing.B) {
	runtime := goja.New()

	state := runtime.NewObject()
	_ = state.Set("count", 0)

	model := &jsModel{
		runtime: runtime,
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			// Return [state, null] — minimal update
			return runtime.NewArray(args[1], goja.Null()), nil
		},
		viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return runtime.ToValue("view output"), nil
		},
		state:       state,
		validCmdIDs: make(map[uint64]bool),
		jsRunner:    &SyncJSRunner{Runtime: runtime},
	}

	keyMsg := tea.KeyPressMsg{Text: "a"}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Step 1: Update (converts msg to JS, calls updateFn, extracts cmd)
		_, cmd := model.Update(keyMsg)
		_ = cmd

		// Step 2: View (calls viewFn via JSRunner, returns string)
		output := model.View()
		_ = output
	}
}

// BenchmarkWrapCmd measures the cost of wrapping a tea.Cmd for JavaScript.
func BenchmarkWrapCmd(b *testing.B) {
	runtime := goja.New()

	cmd := func() tea.Msg { return nil }

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		val := WrapCmd(runtime, cmd)
		_ = val
	}
}

// BenchmarkWrapCmd_Nil measures the cost of WrapCmd with nil (common case).
func BenchmarkWrapCmd_Nil(b *testing.B) {
	runtime := goja.New()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		val := WrapCmd(runtime, nil)
		_ = val
	}
}

// BenchmarkMsgToJS_MouseMsg measures mouse event conversion to JS.
func BenchmarkMsgToJS_MouseMsg(b *testing.B) {
	model := &jsModel{}

	mouseMsg := tea.MouseClickMsg{
		X:      40,
		Y:      12,
		Button: tea.MouseLeft,
		Mod:    tea.ModCtrl,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		jsMsg := model.msgToJS(mouseMsg)
		_ = jsMsg
	}
}

// BenchmarkMsgToJS_WindowSizeMsg measures window size conversion to JS.
func BenchmarkMsgToJS_WindowSizeMsg(b *testing.B) {
	model := &jsModel{}

	msg := tea.WindowSizeMsg{Width: 80, Height: 24}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		jsMsg := model.msgToJS(msg)
		_ = jsMsg
	}
}

// BenchmarkMsgToJS_FocusBlur measures focus/blur message conversion.
func BenchmarkMsgToJS_FocusBlur(b *testing.B) {
	model := &jsModel{}

	b.Run("Focus", func(b *testing.B) {
		msg := tea.FocusMsg{}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			jsMsg := model.msgToJS(msg)
			_ = jsMsg
		}
	})

	b.Run("Blur", func(b *testing.B) {
		msg := tea.BlurMsg{}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			jsMsg := model.msgToJS(msg)
			_ = jsMsg
		}
	})
}

// ============================================================================
// Input Validation Benchmarks
// ============================================================================

// BenchmarkValidateTextareaInput measures input validation for textarea.
func BenchmarkValidateTextareaInput(b *testing.B) {
	b.Run("PrintableASCII", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := ValidateTextareaInput("a")
			_ = r
		}
	})

	b.Run("NamedKey", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := ValidateTextareaInput("enter")
			_ = r
		}
	})

	b.Run("Rejected", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := ValidateTextareaInput("[<65;33;12M")
			_ = r
		}
	})


}

// BenchmarkValidateLabelInput measures input validation for label fields.
func BenchmarkValidateLabelInput(b *testing.B) {
	b.Run("PrintableASCII", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := ValidateLabelInput("a")
			_ = r
		}
	})

	b.Run("Backspace", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := ValidateLabelInput("backspace")
			_ = r
		}
	})

	b.Run("Rejected", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := ValidateLabelInput("enter")
			_ = r
		}
	})
}
