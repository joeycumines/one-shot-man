// Package bubblezone provides JavaScript bindings for github.com/lrstanley/bubblezone.
//
// The module is exposed as "osm:bubblezone" and provides zone-based mouse hit-testing
// for BubbleTea TUI applications. This eliminates the need for hardcoded coordinate
// calculations by allowing components to be wrapped in zones that can be queried
// for mouse event bounds.
//
// # JavaScript API
//
//	const zone = require('osm:bubblezone');
//
//	// Mark a region as a clickable zone
//	const markedContent = zone.mark("button-id", "[ Click Me ]");
//
//	// In your View() function, wrap the final output with scan
//	// This registers zones and strips the invisible markers
//	const output = zone.scan(renderedContent);
//
//	// In your Update() function, check if a mouse event is in bounds
//	// msg is the mouse event from bubbletea
//	if (zone.inBounds("button-id", msg)) {
//	    // Handle click on button
//	}
//
//	// Get zone information
//	const info = zone.get("button-id");
//	// info.startX, info.startY, info.endX, info.endY
//
//	// Generate a unique prefix for child component IDs
//	const prefix = zone.newPrefix();
//	const childId = prefix + "-item-1";
//
// # Implementation Notes
//
// All additions follow these patterns:
//
//  1. No global state - Manager instance per engine
//  2. Zone markers are zero-width to not affect lipgloss.Width()
//  3. Zones must be scanned in the root View() function
//  4. Mouse events can be checked against zones in Update()
//  5. Thread-safe for concurrent access
package bubblezone

import (
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dop251/goja"
	zone "github.com/lrstanley/bubblezone"
)

// Manager holds bubblezone-related state per engine instance.
type Manager struct {
	zone *zone.Manager
	mu   sync.RWMutex
}

// NewManager creates a new bubblezone manager for an engine instance.
func NewManager() *Manager {
	return &Manager{
		zone: zone.New(),
	}
}

// Close cleans up the zone manager.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.zone != nil {
		m.zone.Close()
		m.zone = nil
	}
}

// prefixCounter for generating unique prefixes
var prefixCounter uint64

// Require returns a CommonJS native module under "osm:bubblezone".
// It exposes bubblezone functionality for zone-based mouse hit-testing.
func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// mark wraps content with zone markers
		// Usage: zone.mark("id", "content")
		_ = exports.Set("mark", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return runtime.ToValue("")
			}
			id := call.Argument(0).String()
			content := call.Argument(1).String()

			manager.mu.RLock()
			defer manager.mu.RUnlock()
			if manager.zone == nil {
				return runtime.ToValue(content)
			}

			return runtime.ToValue(manager.zone.Mark(id, content))
		})

		// scan processes the output and registers zone positions
		// Must be called in the root View() function
		// Usage: zone.scan(renderedView)
		_ = exports.Set("scan", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue("")
			}
			content := call.Argument(0).String()

			manager.mu.RLock()
			defer manager.mu.RUnlock()
			if manager.zone == nil {
				return runtime.ToValue(content)
			}

			return runtime.ToValue(manager.zone.Scan(content))
		})

		// inBounds checks if a mouse event is within a zone
		// Usage: zone.inBounds("id", mouseMsg)
		// mouseMsg should have x, y properties (from bubbletea mouse event)
		_ = exports.Set("inBounds", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return runtime.ToValue(false)
			}
			id := call.Argument(0).String()
			msgObj := call.Argument(1).ToObject(runtime)
			if msgObj == nil {
				return runtime.ToValue(false)
			}

			// Extract x, y from mouse message
			xVal := msgObj.Get("x")
			yVal := msgObj.Get("y")
			if goja.IsUndefined(xVal) || goja.IsUndefined(yVal) {
				return runtime.ToValue(false)
			}
			x := int(xVal.ToInteger())
			y := int(yVal.ToInteger())

			manager.mu.RLock()
			defer manager.mu.RUnlock()
			if manager.zone == nil {
				return runtime.ToValue(false)
			}

			zoneInfo := manager.zone.Get(id)
			if zoneInfo == nil || zoneInfo.IsZero() {
				return runtime.ToValue(false)
			}

			// Create a tea.MouseMsg for the InBounds check
			mouseMsg := tea.MouseMsg{X: x, Y: y}
			inBounds := zoneInfo.InBounds(mouseMsg)
			return runtime.ToValue(inBounds)
		})

		// get returns zone information
		// Usage: zone.get("id")
		// Returns: {startX, startY, endX, endY, width, height}
		_ = exports.Set("get", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Null()
			}
			id := call.Argument(0).String()

			manager.mu.RLock()
			defer manager.mu.RUnlock()
			if manager.zone == nil {
				return goja.Null()
			}

			zoneInfo := manager.zone.Get(id)
			if zoneInfo == nil || zoneInfo.IsZero() {
				return goja.Null()
			}

			// ZoneInfo has StartX, StartY, EndX, EndY as direct fields
			return runtime.ToValue(map[string]interface{}{
				"startX": zoneInfo.StartX,
				"startY": zoneInfo.StartY,
				"endX":   zoneInfo.EndX,
				"endY":   zoneInfo.EndY,
				"width":  zoneInfo.EndX - zoneInfo.StartX,
				"height": zoneInfo.EndY - zoneInfo.StartY,
			})
		})

		// newPrefix generates a unique prefix for zone IDs
		// Useful for child components to avoid ID collisions
		// Usage: const prefix = zone.newPrefix();
		_ = exports.Set("newPrefix", func(call goja.FunctionCall) goja.Value {
			manager.mu.RLock()
			defer manager.mu.RUnlock()
			if manager.zone == nil {
				// Fallback to counter-based prefix
				id := atomic.AddUint64(&prefixCounter, 1)
				return runtime.ToValue(string(rune('A'+int(id%26))) + "_")
			}
			return runtime.ToValue(manager.zone.NewPrefix())
		})

		// close cleans up the zone manager
		// Should be called when the TUI is done
		_ = exports.Set("close", func(call goja.FunctionCall) goja.Value {
			manager.Close()
			return goja.Undefined()
		})
	}
}
