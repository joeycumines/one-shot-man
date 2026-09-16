package freezeterm

import (
	"errors"
	"fmt"
	"log/slog"
	"math"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/freezeterm"
)

// errAdapterRequired reports a missing event-loop adapter synchronously,
// matching the other I/O bindings.
func errAdapterRequired(name string) error {
	return fmt.Errorf("%s: event loop adapter is required", name)
}

// settle tolerates settlement failures caused by loop shutdown, mirroring the
// documented tolerance used by the other tracked bindings. Any other
// settlement error is logged (a hung promise would otherwise be silent).
func settle(settleFn gojaeventloop.TrackedSettlement, reject bool, build func(*goja.Runtime) any) {
	err := settleFn.Settle(reject, build)
	if err == nil {
		return
	}
	if errors.Is(err, goeventloop.ErrLoopTerminated) || errors.Is(err, gojaeventloop.ErrAdapterInvalid) || errors.Is(err, gojaeventloop.ErrPromiseSettled) {
		return
	}
	slog.Debug("freezeterm promise settlement failed", "error", err, "reject", reject)
}

// parseInfoOptions accepts an optional executable path, either as a bare
// string or as {executable}.
func parseInfoOptions(runtime *goja.Runtime, value goja.Value) string {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return ""
	}
	if _, ok := value.Export().(string); ok {
		return value.String()
	}
	if _, ok := value.Export().(map[string]any); !ok {
		panic(runtime.NewTypeError("info: options must be an object or executable string"))
	}
	obj := value.ToObject(runtime)
	keys := obj.Keys()
	for _, key := range keys {
		if key != "executable" {
			panic(runtime.NewTypeError(fmt.Sprintf("info: unknown option %q", key)))
		}
	}
	if len(keys) == 0 {
		return ""
	}
	executable, ok := obj.Get("executable").Export().(string)
	if !ok {
		panic(runtime.NewTypeError("info: executable must be a string"))
	}
	return executable
}

// renderOptions mirrors the JS option object for render and renderText.
type renderOptions struct {
	executable      string
	output          string
	format          string
	language        string
	theme           string
	background      string
	window          bool
	width           float64
	height          float64
	margin          []float64
	padding         []float64
	wrap            int
	lineHeight      float64
	lines           []int
	showLineNumbers bool
	fontFamily      string
	fontFile        string
	fontSize        float64
	fontLigatures   *bool
	borderRadius    float64
	borderWidth     float64
	borderColor     string
	shadowBlur      float64
	shadowX         float64
	shadowY         float64
	args            []string
	dir             string
}

// withInput builds the Go options for a captured-text render.
func (o renderOptions) withInput(text string) parent.Options {
	return parent.Options{
		Executable:      o.executable,
		Input:           []byte(text),
		Output:          o.output,
		Format:          parent.Format(o.format),
		Language:        o.language,
		Theme:           o.theme,
		Background:      o.background,
		Window:          o.window,
		Width:           o.width,
		Height:          o.height,
		Margin:          o.margin,
		Padding:         o.padding,
		Wrap:            o.wrap,
		LineHeight:      o.lineHeight,
		Lines:           o.lines,
		ShowLineNumbers: o.showLineNumbers,
		Font: parent.FontOptions{
			Family:    o.fontFamily,
			File:      o.fontFile,
			Size:      o.fontSize,
			Ligatures: o.fontLigatures,
		},
		Border: parent.BorderOptions{
			Radius: o.borderRadius,
			Width:  o.borderWidth,
			Color:  o.borderColor,
		},
		Shadow: parent.ShadowOptions{
			Blur: o.shadowBlur,
			X:    o.shadowX,
			Y:    o.shadowY,
		},
		Args: o.args,
		Dir:  o.dir,
	}
}

// parseRenderCall validates (text, options?) for render and renderText.
// Unknown option keys and type mismatches throw synchronously.
func parseRenderCall(runtime *goja.Runtime, name string, call goja.FunctionCall) (string, renderOptions) {
	if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
		panic(runtime.NewTypeError(name + ": text argument is required"))
	}
	text, ok := call.Argument(0).Export().(string)
	if !ok {
		panic(runtime.NewTypeError(name + ": text must be a string"))
	}
	var opts renderOptions
	value := call.Argument(1)
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return text, opts
	}
	if _, ok := value.Export().(map[string]any); !ok {
		panic(runtime.NewTypeError(name + ": options must be an object"))
	}
	obj := value.ToObject(runtime)
	for _, key := range obj.Keys() {
		option := obj.Get(key)
		switch key {
		case "executable":
			opts.executable = requireString(runtime, name, key, option)
		case "output":
			opts.output = requireString(runtime, name, key, option)
		case "format":
			opts.format = requireString(runtime, name, key, option)
			if opts.format != "svg" && opts.format != "png" {
				panic(runtime.NewTypeError(fmt.Sprintf("%s: format must be %q or %q", name, "svg", "png")))
			}
		case "language":
			opts.language = requireString(runtime, name, key, option)
		case "theme":
			opts.theme = requireString(runtime, name, key, option)
		case "background":
			opts.background = requireString(runtime, name, key, option)
		case "window":
			opts.window = requireBoolean(runtime, name, key, option)
		case "width":
			opts.width = requireNumber(runtime, name, key, option)
		case "height":
			opts.height = requireNumber(runtime, name, key, option)
		case "margin":
			opts.margin = requireNumberList(runtime, name, key, option, "margin")
		case "padding":
			opts.padding = requireNumberList(runtime, name, key, option, "padding")
		case "wrap":
			opts.wrap = int(requireNumber(runtime, name, key, option))
		case "lineHeight":
			opts.lineHeight = requireNumber(runtime, name, key, option)
		case "lines":
			opts.lines = requireIntList(runtime, name, key, option)
			if len(opts.lines) > 2 {
				panic(runtime.NewTypeError(name + ": lines accepts at most two values"))
			}
		case "showLineNumbers":
			opts.showLineNumbers = requireBoolean(runtime, name, key, option)
		case "font":
			parseFontOptions(runtime, name, option, &opts)
		case "border":
			parseBorderOptions(runtime, name, option, &opts)
		case "shadow":
			parseShadowOptions(runtime, name, option, &opts)
		case "args":
			opts.args = requireStringList(runtime, name, key, option)
		case "dir":
			opts.dir = requireString(runtime, name, key, option)
		default:
			panic(runtime.NewTypeError(fmt.Sprintf("%s: unknown option %q", name, key)))
		}
	}
	return text, opts
}

func parseFontOptions(runtime *goja.Runtime, name string, value goja.Value, opts *renderOptions) {
	obj := optionObject(runtime, name, "font", value)
	for _, key := range obj.Keys() {
		switch key {
		case "family":
			opts.fontFamily = requireString(runtime, name, key, obj.Get(key))
		case "file":
			opts.fontFile = requireString(runtime, name, key, obj.Get(key))
		case "size":
			opts.fontSize = requireNumber(runtime, name, key, obj.Get(key))
		case "ligatures":
			ligatures := requireBoolean(runtime, name, key, obj.Get(key))
			opts.fontLigatures = &ligatures
		default:
			panic(runtime.NewTypeError(fmt.Sprintf("%s: unknown font option %q", name, key)))
		}
	}
}

func parseBorderOptions(runtime *goja.Runtime, name string, value goja.Value, opts *renderOptions) {
	obj := optionObject(runtime, name, "border", value)
	for _, key := range obj.Keys() {
		switch key {
		case "radius":
			opts.borderRadius = requireNumber(runtime, name, key, obj.Get(key))
		case "width":
			opts.borderWidth = requireNumber(runtime, name, key, obj.Get(key))
		case "color":
			opts.borderColor = requireString(runtime, name, key, obj.Get(key))
		default:
			panic(runtime.NewTypeError(fmt.Sprintf("%s: unknown border option %q", name, key)))
		}
	}
}

func parseShadowOptions(runtime *goja.Runtime, name string, value goja.Value, opts *renderOptions) {
	obj := optionObject(runtime, name, "shadow", value)
	for _, key := range obj.Keys() {
		switch key {
		case "blur":
			opts.shadowBlur = requireNumber(runtime, name, key, obj.Get(key))
		case "x":
			opts.shadowX = requireNumber(runtime, name, key, obj.Get(key))
		case "y":
			opts.shadowY = requireNumber(runtime, name, key, obj.Get(key))
		default:
			panic(runtime.NewTypeError(fmt.Sprintf("%s: unknown shadow option %q", name, key)))
		}
	}
}

func optionObject(runtime *goja.Runtime, name, group string, value goja.Value) *goja.Object {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		panic(runtime.NewTypeError(fmt.Sprintf("%s: %s must be an object", name, group)))
	}
	if _, ok := value.Export().(map[string]any); !ok {
		panic(runtime.NewTypeError(fmt.Sprintf("%s: %s must be an object", name, group)))
	}
	return value.ToObject(runtime)
}

func requireString(runtime *goja.Runtime, name, key string, value goja.Value) string {
	s, ok := value.Export().(string)
	if !ok {
		panic(runtime.NewTypeError(fmt.Sprintf("%s: %s must be a string", name, key)))
	}
	return s
}

func requireBoolean(runtime *goja.Runtime, name, key string, value goja.Value) bool {
	b, ok := value.Export().(bool)
	if !ok {
		panic(runtime.NewTypeError(fmt.Sprintf("%s: %s must be a boolean", name, key)))
	}
	return b
}

func requireNumber(runtime *goja.Runtime, name, key string, value goja.Value) float64 {
	switch value.Export().(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
	default:
		panic(runtime.NewTypeError(fmt.Sprintf("%s: %s must be a finite number", name, key)))
	}
	number := value.ToFloat()
	if math.IsNaN(number) || math.IsInf(number, 0) {
		panic(runtime.NewTypeError(fmt.Sprintf("%s: %s must be a finite number", name, key)))
	}
	return number
}

func requireNumberList(runtime *goja.Runtime, name, key string, value goja.Value, label string) []float64 {
	items := requireArray(runtime, name, key, value)
	out := make([]float64, len(items))
	for i, item := range items {
		out[i] = requireNumber(runtime, name, key, item)
	}
	switch len(out) {
	case 1, 2, 4:
	default:
		panic(runtime.NewTypeError(fmt.Sprintf("%s: %s must have 1, 2 or 4 values", name, label)))
	}
	return out
}

func requireIntList(runtime *goja.Runtime, name, key string, value goja.Value) []int {
	items := requireArray(runtime, name, key, value)
	out := make([]int, len(items))
	for i, item := range items {
		number := requireNumber(runtime, name, key, item)
		if number != math.Trunc(number) {
			panic(runtime.NewTypeError(fmt.Sprintf("%s: %s values must be integers", name, key)))
		}
		out[i] = int(number)
	}
	return out
}

func requireStringList(runtime *goja.Runtime, name, key string, value goja.Value) []string {
	items := requireArray(runtime, name, key, value)
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = requireString(runtime, name, key, item)
	}
	return out
}

func requireArray(runtime *goja.Runtime, name, key string, value goja.Value) []goja.Value {
	exported := value.Export()
	if _, ok := exported.(string); ok {
		panic(runtime.NewTypeError(fmt.Sprintf("%s: %s must be an array", name, key)))
	}
	obj := value.ToObject(runtime)
	lengthVal := obj.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		panic(runtime.NewTypeError(fmt.Sprintf("%s: %s must be an array", name, key)))
	}
	length := int(lengthVal.ToInteger())
	if length < 0 {
		panic(runtime.NewTypeError(fmt.Sprintf("%s: %s must be an array", name, key)))
	}
	items := make([]goja.Value, length)
	for i := range items {
		items[i] = obj.Get(fmt.Sprintf("%d", i))
	}
	return items
}
