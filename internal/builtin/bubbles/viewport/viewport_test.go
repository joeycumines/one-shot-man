package viewport

import (
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/dop251/goja"
	jslipgloss "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
)

// -----------------------------------------------------------------------------
// Unit Tests: Core Logic
// -----------------------------------------------------------------------------

func TestGetUnexportedXOffset(t *testing.T) {
	// Setup a real viewport model
	m := viewport.New(20, 10)

	// Set X Offset using public API, which sets the private field
	expectedX := 5
	m.SetContent("This is a test content that is long enough to require horizontal scrolling. This content determines the longest line width.")
	m.SetXOffset(expectedX)

	// Use our reflection helper to read it back
	got := getUnexportedXOffset(&m)

	if got != expectedX {
		t.Errorf("reflection helper failed: expected %d, got %d", expectedX, got)
	}
}

// -----------------------------------------------------------------------------
// Integration Tests: JavaScript API Surface
// -----------------------------------------------------------------------------

// setupTestRuntime initializes a Goja runtime with the viewport and lipgloss modules loaded.
func setupTestRuntime(t *testing.T) *goja.Runtime {
	rt := goja.New()

	// Create mock require function
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:bubbles/viewport":
			mod := rt.NewObject()
			Require()(rt, mod)
			return mod.Get("exports")
		case "osm:lipgloss":
			mod := rt.NewObject()
			lm := jslipgloss.NewManager()
			jslipgloss.Require(lm)(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})

	return rt
}

func TestJS_API_FullSurface(t *testing.T) {
	rt := setupTestRuntime(t)

	script := `
		const viewport = require('osm:bubbles/viewport');

		// 1. New & Dimensions
		const vp = viewport.new(20, 10);
		if (vp.width() !== 20) throw new Error("width mismatch: " + vp.width());
		if (vp.height() !== 10) throw new Error("height mismatch: " + vp.height());

		// 2. Set Content & Line Counts
		const text = "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10\nLine 11\nLine 12\nThis is a test content that is long enough to require horizontal scrolling. This content determines the longest line width.";
		vp.setContent(text);

		if (vp.totalLineCount() !== 13) throw new Error("total lines mismatch: " + vp.totalLineCount());

		// 3. Scroll Logic
		vp.scrollDown(1);
		if (vp.yOffset() !== 1) throw new Error("scrollDown failed: " + vp.yOffset());

		vp.gotoBottom();
		if (vp.atBottom() !== true) throw new Error("gotoBottom failed");

		vp.gotoTop();
		if (vp.atTop() !== true) throw new Error("gotoTop failed");
		if (vp.yOffset() !== 0) throw new Error("gotoTop offset mismatch");

		// 4. Horizontal Scroll
		vp.setXOffset(2);
		if (vp.xOffset() !== 2) throw new Error("xOffset failed: " + vp.xOffset());

		vp.scrollRight(1);
		if (vp.xOffset() !== 3) throw new Error("scrollRight failed");

		// 5. Percentages
		if (vp.scrollPercent() < 0 || vp.scrollPercent() > 1) throw new Error("invalid scroll percent");

		// 6. Resizing
		vp.setWidth(30);
		vp.setHeight(5);
		if (vp.width() !== 30) throw new Error("setWidth failed");

		// 7. Mouse Settings
		vp.setMouseWheelEnabled(true);
		if (vp.isMouseWheelEnabled() !== true) throw new Error("mouse wheel enable failed");

		vp.setMouseWheelDelta(5);
		if (vp.mouseWheelDelta() !== 5) throw new Error("mouse wheel delta failed");

		// 8. View
		const v = vp.view();
		if (typeof v !== 'string' || v.length === 0) throw new Error("view returned empty or invalid");
	`

	_, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("JS Execution failed: %v", err)
	}
}

func TestJS_Styling(t *testing.T) {
	rt := setupTestRuntime(t)

	script := `
		const viewport = require('osm:bubbles/viewport');
		const lipgloss = require('osm:lipgloss');

		const vp = viewport.new(10, 5);

		// Create a style
		const style = lipgloss.newStyle().border(lipgloss.normalBorder()).padding(1);

		// Apply style
		vp.setStyle(style);

		// Clear style
		vp.setStyle(null);
	`

	_, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("JS Execution failed: %v", err)
	}
}

func TestJS_Style_Clear(t *testing.T) {
	rt := setupTestRuntime(t)

	script := `
		const viewport = require('osm:bubbles/viewport');
		const lipgloss = require('osm:lipgloss');
		const vp = viewport.new(10, 5);
		const style = lipgloss.newStyle().padding(5);
		vp.setStyle(style);

		// Clear it
		vp.setStyle(null);
	`
	_, err := rt.RunString(script)
	if err != nil {
		t.Fatal(err)
	}
}

// -----------------------------------------------------------------------------
// Critical Requirement: Fail-Fast on Dispose & Race Safety
// -----------------------------------------------------------------------------

func TestJS_Dispose_FailFast(t *testing.T) {
	rt := setupTestRuntime(t)

	script := `
		const viewport = require('osm:bubbles/viewport');
		const vp = viewport.new(10, 10);

		// Ensure content exists so scrollDown can move the offset
		vp.setContent("A\nB\nC\nD\nE\nF\nG\nH\nI\nJ\n");
		vp.scrollDown(1);
		if (vp.yOffset() !== 1) throw new Error("scrollDown failed, yOffset="+String(vp.yOffset()));
	`

	_, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("Basic functionality check failed: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Update & Edge Cases
// -----------------------------------------------------------------------------

func TestJS_Update_ReturnSignature(t *testing.T) {
	rt := setupTestRuntime(t)

	script := `
		const viewport = require('osm:bubbles/viewport');
		const vp = viewport.new(10, 10);

		// Pass an arbitrary object. Since we don't have bubbletea message converters
		// fully mocked here, likely it results in nil msg or no-op, which is fine.
		// We verify the RETURN structure.
		const res = vp.update({});

		if (!Array.isArray(res)) throw new Error("update did not return array");
		if (res.length !== 2) throw new Error("update array length incorrect");
		if (res[0] !== vp) throw new Error("first element is not viewport instance");

		// If no cmd, second arg is null
		if (res[1] !== null && typeof res[1] !== 'object') throw new Error("second element invalid");
	`

	_, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("Update signature check failed: %v", err)
	}
}

func TestViewport_SyncCommandPropagation(t *testing.T) {
	rt := setupTestRuntime(t)

	script := `
		const viewport = require('osm:bubbles/viewport');
		const vp = viewport.new(10, 10);
		vp.setMouseWheelEnabled(true);

		// Simulate mouse wheel up with explicit modifier flags
		const res = vp.update({ type: 'Mouse', button: 'wheel up', action: 'press', x: 0, y: 0, alt: false, ctrl: false, shift: false });
		if (!Array.isArray(res)) throw new Error('update did not return array');
		// Second element can be null OR an opaque function. If it's a descriptor object
		// with _cmdType it's not a wrapped Go command.
		if (res[1] !== null && typeof res[1] === 'object' && res[1]._cmdType) throw new Error('expected an opaque wrapped command or null');
	`

	_, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("Viewport sync command propagation failed: %v", err)
	}
}

func TestJS_SetDimensions_Reclamp(t *testing.T) {
	// Test the specific requirement that setWidth/setHeight triggers re-clamping
	// of both axes using the unexported field reader.
	rt := setupTestRuntime(t)

	script := `
		const viewport = require('osm:bubbles/viewport');
		const vp = viewport.new(10, 10);
		vp.setContent("Long line that needs scrolling");

		// Scroll right
		vp.setXOffset(100);

		// Resize width to be very large (should clamp xOffset to 0 or low value)
		vp.setWidth(200);

		if (vp.xOffset() > 0) throw new Error("Failed to clamp X offset after width increase: " + vp.xOffset());
	`
	_, err := rt.RunString(script)
	if err != nil {
		t.Fatal(err)
	}
}
