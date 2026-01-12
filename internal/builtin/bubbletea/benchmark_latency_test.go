package bubbletea

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
		type: 'Mouse',
		x: 40,
		y: 12,
		button: 'left',
		action: 'press',
		alt: false,
		ctrl: false,
		shift: false
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

	// Create a realistic tea.KeyMsg
	keyMsg := tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'w'},
	}

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

	m := map[string]interface{}{
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
		msg := tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{'w'},
		}
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
