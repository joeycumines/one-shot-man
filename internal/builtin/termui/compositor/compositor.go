// Package compositor provides JavaScript bindings for
// [github.com/joeycumines/one-shot-man/internal/termui/compositor].
//
// The module is exposed as "osm:termui/compositor" and provides a
// Compositor factory with chainable pane and chrome management API.
//
// # JavaScript API
//
//	const comp = require('osm:termui/compositor');
//
//	// Create a compositor with canvas dimensions
//	const c = comp.compositor({ width: 80, height: 24 });
//
//	// Chainable pane management
//	c.addPane({ id: 'main', content: 'hello', bounds: { x: 0, y: 0, width: 40, height: 12 } })
//	 .addPane({ id: 'side', content: 'world', bounds: { x: 40, y: 0, width: 40, height: 12 } })
//	 .updatePane({ id: 'main', content: 'updated' })
//	 .updatePaneIfNew({ id: 'main', content: 'v2', gen: 2 })
//	 .removePane('side');
//
//	// Chainable chrome management
//	c.addChrome({ id: 'border', content: '═══', bounds: { x: 0, y: 0, width: 80, height: 1 }, z: 10 })
//	 .updateChrome({ id: 'border', content: '───' })
//	 .removeChrome('border');
//
//	// Resize
//	c.resize(120, 40);
//
//	// Non-chainable methods
//	const rendered = c.render();          // string
//	const hit = c.hit(5, 5);             // { id: string, hit: bool }
//	const ids = c.paneIds();             // string[]
package compositor

import (
	"github.com/dop251/goja"

	"github.com/joeycumines/one-shot-man/internal/termui/compositor"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// Require returns a CommonJS module under "osm:termui/compositor".
func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)

	// compositor({width, height}) — creates a Compositor with canvas
	_ = exports.Set("compositor", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("compositor requires a config object {width, height}"))
		}

		config := call.Argument(0).ToObject(runtime)
		if config == nil {
			panic(runtime.NewTypeError("compositor: config must be an object"))
		}

		widthVal := config.Get("width")
		heightVal := config.Get("height")

		var width, height int
		if widthVal != nil && !goja.IsUndefined(widthVal) && !goja.IsNull(widthVal) {
			width = int(widthVal.ToInteger())
		}
		if heightVal != nil && !goja.IsUndefined(heightVal) && !goja.IsNull(heightVal) {
			height = int(heightVal.ToInteger())
		}

		c := compositor.NewCompositor(width, height)

		return createCompositorObject(runtime, c)
	})
}

// createCompositorObject wraps a *compositor.Compositor as a Goja Object
// with chainable pane and chrome management methods.
func createCompositorObject(runtime *goja.Runtime, c *compositor.Compositor) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/compositor")

	// Store the Go compositor as a non-enumerable data property so it
	// doesn't appear in Object.keys() but is accessible via Export().
	_ = obj.DefineDataProperty("_goComp", runtime.ToValue(c),
		goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)

	// extractBounds parses {x, y, width, height} into coordinate.Rect.
	extractBounds := func(val goja.Value) coordinate.Rect {
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			return coordinate.Rect{}
		}
		o := val.ToObject(runtime)
		if o == nil {
			return coordinate.Rect{}
		}
		return coordinate.Rect{
			Position: coordinate.Position{
				X: int(o.Get("x").ToInteger()),
				Y: int(o.Get("y").ToInteger()),
			},
			Size: coordinate.Size{
				Width:  int(o.Get("width").ToInteger()),
				Height: int(o.Get("height").ToInteger()),
			},
		}
	}

	// addPane({id, content, bounds, z}) — chainable
	_ = obj.Set("addPane", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("addPane requires an object argument {id, content, bounds, z}"))
		}
		cfg := call.Argument(0).ToObject(runtime)
		id := cfg.Get("id").String()
		content := cfg.Get("content").String()
		bounds := extractBounds(cfg.Get("bounds"))
		z := 0
		if zVal := cfg.Get("z"); zVal != nil && !goja.IsUndefined(zVal) && !goja.IsNull(zVal) {
			z = int(zVal.ToInteger())
		}
		c.AddPane(id, content, bounds, z)
		return obj
	})

	// updatePane({id, content}) — chainable
	_ = obj.Set("updatePane", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("updatePane requires an object argument {id, content}"))
		}
		cfg := call.Argument(0).ToObject(runtime)
		id := cfg.Get("id").String()
		content := cfg.Get("content").String()
		c.UpdatePane(id, content)
		return obj
	})

	// updatePaneIfNew({id, content, gen}) — chainable
	_ = obj.Set("updatePaneIfNew", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("updatePaneIfNew requires an object argument {id, content, gen}"))
		}
		cfg := call.Argument(0).ToObject(runtime)
		id := cfg.Get("id").String()
		content := cfg.Get("content").String()
		gen := uint64(0)
		if genVal := cfg.Get("gen"); genVal != nil && !goja.IsUndefined(genVal) && !goja.IsNull(genVal) {
			gen = uint64(genVal.ToInteger())
		}
		c.UpdatePaneIfNew(id, content, gen)
		return obj
	})

	// removePane(id) — chainable
	_ = obj.Set("removePane", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("removePane requires an id argument"))
		}
		id := call.Argument(0).String()
		c.RemovePane(id)
		return obj
	})

	// addChrome({id, content, bounds, z}) — chainable
	_ = obj.Set("addChrome", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("addChrome requires an object argument {id, content, bounds, z}"))
		}
		cfg := call.Argument(0).ToObject(runtime)
		id := cfg.Get("id").String()
		content := cfg.Get("content").String()
		bounds := extractBounds(cfg.Get("bounds"))
		z := 0
		if zVal := cfg.Get("z"); zVal != nil && !goja.IsUndefined(zVal) && !goja.IsNull(zVal) {
			z = int(zVal.ToInteger())
		}
		c.AddChrome(id, content, bounds, z)
		return obj
	})

	// updateChrome({id, content}) — chainable
	_ = obj.Set("updateChrome", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("updateChrome requires an object argument {id, content}"))
		}
		cfg := call.Argument(0).ToObject(runtime)
		id := cfg.Get("id").String()
		content := cfg.Get("content").String()
		c.UpdateChrome(id, content)
		return obj
	})

	// removeChrome(id) — chainable
	_ = obj.Set("removeChrome", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewTypeError("removeChrome requires an id argument"))
		}
		id := call.Argument(0).String()
		c.RemoveChrome(id)
		return obj
	})

	// resize(width, height) — chainable
	_ = obj.Set("resize", func(call goja.FunctionCall) goja.Value {
		width := 0
		height := 0
		if len(call.Arguments) >= 1 {
			width = int(call.Argument(0).ToInteger())
		}
		if len(call.Arguments) >= 2 {
			height = int(call.Argument(1).ToInteger())
		}
		c.Resize(width, height)
		return obj
	})

	// render() — returns string (non-chainable)
	_ = obj.Set("render", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(c.Render())
	})

	// hit(x, y) — returns {id, hit} (non-chainable)
	_ = obj.Set("hit", func(call goja.FunctionCall) goja.Value {
		x := 0
		y := 0
		if len(call.Arguments) >= 1 {
			x = int(call.Argument(0).ToInteger())
		}
		if len(call.Arguments) >= 2 {
			y = int(call.Argument(1).ToInteger())
		}
		id, ok := c.Hit(x, y)
		result := runtime.NewObject()
		_ = result.Set("id", id)
		_ = result.Set("hit", ok)
		return runtime.ToValue(result)
	})

	// paneIds() — returns string array (non-chainable)
	_ = obj.Set("paneIds", func(call goja.FunctionCall) goja.Value {
		ids := c.PaneIDs()
		values := make([]any, len(ids))
		for i, id := range ids {
			values[i] = runtime.ToValue(id)
		}
		return runtime.NewArray(values...)
	})

	return obj
}
