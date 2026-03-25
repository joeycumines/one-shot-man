// Package jsonmod provides a Goja module with JSON utilities for JS scripts.
// It is registered as "osm:json" and exposes parse, stringify, query (dot-notation/
// array-indexing/wildcard path queries), mergePatch (RFC 7386), diff (JSON Pointer
// paths), flatten, and unflatten functions.
package jsonmod

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

// Require is the Goja module loader for osm:json.
func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)

	// parse(str: string): any — parse JSON string into a value
	_ = exports.Set("parse", func(call goja.FunctionCall) goja.Value {
		return jsParse(runtime, call)
	})

	// stringify(value: any, indent?: number|string): string — serialize value to JSON
	_ = exports.Set("stringify", func(call goja.FunctionCall) goja.Value {
		return jsStringify(runtime, call)
	})

	// query(obj: any, path: string): any — query a value using dot/bracket path notation
	_ = exports.Set("query", func(call goja.FunctionCall) goja.Value {
		return jsQuery(runtime, call)
	})

	// mergePatch(target: any, patch: any): any — RFC 7386 JSON Merge Patch
	_ = exports.Set("mergePatch", func(call goja.FunctionCall) goja.Value {
		return jsMergePatch(runtime, call)
	})

	// diff(a: any, b: any): Array<{op, path, value?, oldValue?}> — compute JSON diff
	_ = exports.Set("diff", func(call goja.FunctionCall) goja.Value {
		return jsDiff(runtime, call)
	})

	// flatten(obj: object, separator?: string): object — flatten nested object
	_ = exports.Set("flatten", func(call goja.FunctionCall) goja.Value {
		return jsFlatten(runtime, call)
	})

	// unflatten(obj: object, separator?: string): object — reverse of flatten
	_ = exports.Set("unflatten", func(call goja.FunctionCall) goja.Value {
		return jsUnflatten(runtime, call)
	})
}

// exportValue safely exports a goja.Value to a Go any.
// Returns nil for nil, undefined, and null.
func exportValue(v goja.Value) any {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	return v.Export()
}

// ---------------------------------------------------------------------------
// parse
// ---------------------------------------------------------------------------

func jsParse(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	arg := call.Argument(0)
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		panic(runtime.NewTypeError("parse requires a string argument"))
	}
	str := arg.String()
	var result any
	if err := json.Unmarshal([]byte(str), &result); err != nil {
		panic(runtime.NewTypeError(fmt.Sprintf("invalid JSON: %s", err.Error())))
	}
	return runtime.ToValue(result)
}

// ---------------------------------------------------------------------------
// stringify
// ---------------------------------------------------------------------------

func jsStringify(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	arg := call.Argument(0)
	if arg == nil || goja.IsUndefined(arg) {
		return goja.Undefined()
	}

	val := arg.Export()

	indentArg := call.Argument(1)
	var data []byte
	var err error

	if indentArg == nil || goja.IsUndefined(indentArg) {
		data, err = json.Marshal(val)
	} else {
		indent := resolveIndent(indentArg)
		data, err = json.MarshalIndent(val, "", indent)
	}

	if err != nil {
		panic(runtime.NewTypeError(fmt.Sprintf("stringify failed: %s", err.Error())))
	}
	return runtime.ToValue(string(data))
}

// resolveIndent converts the indent argument to a string.
// Numbers produce that many spaces; strings are used directly.
func resolveIndent(v goja.Value) string {
	if goja.IsNull(v) {
		return ""
	}
	exported := v.Export()
	switch n := exported.(type) {
	case int64:
		if n < 0 {
			n = 0
		}
		return strings.Repeat(" ", int(n))
	case float64:
		count := max(int(n), 0)
		return strings.Repeat(" ", count)
	default:
		return v.String()
	}
}

// ---------------------------------------------------------------------------
// query
// ---------------------------------------------------------------------------

type segmentType int

const (
	segKey segmentType = iota
	segIndex
	segWildcard
)

type pathSegment struct {
	typ   segmentType
	key   string
	index int
}

func jsQuery(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	objArg := call.Argument(0)
	if objArg == nil || goja.IsUndefined(objArg) || goja.IsNull(objArg) {
		return goja.Undefined()
	}

	pathArg := call.Argument(1)
	if pathArg == nil || goja.IsUndefined(pathArg) || goja.IsNull(pathArg) {
		panic(runtime.NewTypeError("query requires a path string"))
	}

	path := pathArg.String()
	if path == "" {
		return objArg
	}

	obj := objArg.Export()
	segments := parsePath(path)

	result, found := queryValue(obj, segments)
	if !found {
		return goja.Undefined()
	}
	return runtime.ToValue(result)
}

// parsePath splits a dot/bracket path like "foo.bar[0].baz[*].name" into segments.
func parsePath(path string) []pathSegment {
	var segments []pathSegment
	i := 0
	for i < len(path) {
		if path[i] == '[' {
			// bracket notation
			j := strings.IndexByte(path[i:], ']')
			if j == -1 {
				// malformed: treat rest as key
				segments = append(segments, pathSegment{typ: segKey, key: path[i:]})
				break
			}
			inside := path[i+1 : i+j]
			if inside == "*" {
				segments = append(segments, pathSegment{typ: segWildcard})
			} else if idx, err := strconv.Atoi(inside); err == nil {
				segments = append(segments, pathSegment{typ: segIndex, index: idx})
			} else {
				segments = append(segments, pathSegment{typ: segKey, key: inside})
			}
			i += j + 1
			if i < len(path) && path[i] == '.' {
				i++ // skip dot after ]
			}
		} else {
			// key notation — read until next dot or bracket
			end := len(path)
			for k := i; k < len(path); k++ {
				if path[k] == '.' || path[k] == '[' {
					end = k
					break
				}
			}
			key := path[i:end]
			if key != "" {
				segments = append(segments, pathSegment{typ: segKey, key: key})
			}
			i = end
			if i < len(path) && path[i] == '.' {
				i++ // skip dot separator
			}
		}
	}
	return segments
}

// queryValue navigates val according to the remaining path segments.
func queryValue(val any, segments []pathSegment) (any, bool) {
	if len(segments) == 0 {
		return val, true
	}

	seg := segments[0]
	rest := segments[1:]

	switch seg.typ {
	case segKey:
		m, ok := val.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[seg.key]
		if !ok {
			return nil, false
		}
		return queryValue(v, rest)

	case segIndex:
		arr, ok := val.([]any)
		if !ok {
			return nil, false
		}
		if seg.index < 0 || seg.index >= len(arr) {
			return nil, false
		}
		return queryValue(arr[seg.index], rest)

	case segWildcard:
		arr, ok := val.([]any)
		if !ok {
			return nil, false
		}
		results := make([]any, 0, len(arr))
		for _, item := range arr {
			if r, ok := queryValue(item, rest); ok {
				results = append(results, r)
			}
		}
		return results, true
	}

	return nil, false
}

// ---------------------------------------------------------------------------
// mergePatch (RFC 7386)
// ---------------------------------------------------------------------------

func jsMergePatch(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	target := exportValue(call.Argument(0))
	patch := exportValue(call.Argument(1))

	result := mergePatch(target, patch)
	if result == nil {
		return goja.Null()
	}
	return runtime.ToValue(result)
}

// mergePatch implements RFC 7386 JSON Merge Patch.
// If patch is not an object (map), the patch replaces the target entirely.
// Within objects: null values delete keys; non-null values recurse.
// Arrays are replaced, not merged. The input is not mutated.
func mergePatch(target, patch any) any {
	patchMap, patchIsMap := patch.(map[string]any)
	if !patchIsMap {
		return patch
	}

	targetMap, targetIsMap := target.(map[string]any)
	if !targetIsMap {
		targetMap = make(map[string]any)
	}

	// Deep copy target to avoid mutation.
	result := make(map[string]any, len(targetMap))
	for k, v := range targetMap {
		result[k] = deepCopy(v)
	}

	for k, v := range patchMap {
		if v == nil {
			delete(result, k)
		} else {
			result[k] = mergePatch(result[k], v)
		}
	}

	return result
}

// deepCopy returns a deep clone of maps and slices; primitives are returned as-is.
func deepCopy(v any) any {
	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, child := range val {
			result[k] = deepCopy(child)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, child := range val {
			result[i] = deepCopy(child)
		}
		return result
	default:
		return v
	}
}

// ---------------------------------------------------------------------------
// diff
// ---------------------------------------------------------------------------

func jsDiff(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	a := exportValue(call.Argument(0))
	b := exportValue(call.Argument(1))

	ops := computeDiff(a, b, "")
	if ops == nil {
		ops = []any{}
	}
	return runtime.ToValue(ops)
}

// computeDiff recursively computes the differences between a and b.
// path is the JSON Pointer prefix for child operations.
func computeDiff(a, b any, path string) []any {
	aMap, aIsMap := a.(map[string]any)
	bMap, bIsMap := b.(map[string]any)
	if aIsMap && bIsMap {
		return diffMaps(aMap, bMap, path)
	}

	aArr, aIsArr := a.([]any)
	bArr, bIsArr := b.([]any)
	if aIsArr && bIsArr {
		return diffArrays(aArr, bArr, path)
	}

	if !valuesEqual(a, b) {
		op := map[string]any{
			"op":       "replace",
			"path":     path,
			"value":    b,
			"oldValue": a,
		}
		return []any{op}
	}
	return nil
}

func diffMaps(a, b map[string]any, path string) []any {
	// Collect and sort keys for deterministic output.
	keySet := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		keySet[k] = struct{}{}
	}
	for k := range b {
		keySet[k] = struct{}{}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var ops []any
	for _, k := range keys {
		childPath := path + "/" + escapeJSONPointer(k)
		av, inA := a[k]
		bv, inB := b[k]

		switch {
		case inA && inB:
			ops = append(ops, computeDiff(av, bv, childPath)...)
		case inA:
			ops = append(ops, map[string]any{
				"op":       "remove",
				"path":     childPath,
				"oldValue": av,
			})
		default:
			ops = append(ops, map[string]any{
				"op":    "add",
				"path":  childPath,
				"value": bv,
			})
		}
	}
	return ops
}

func diffArrays(a, b []any, path string) []any {
	maxLen := max(len(b), len(a))

	var ops []any
	for i := 0; i < maxLen; i++ {
		childPath := path + "/" + strconv.Itoa(i)
		switch {
		case i >= len(a):
			ops = append(ops, map[string]any{
				"op":    "add",
				"path":  childPath,
				"value": b[i],
			})
		case i >= len(b):
			ops = append(ops, map[string]any{
				"op":       "remove",
				"path":     childPath,
				"oldValue": a[i],
			})
		default:
			ops = append(ops, computeDiff(a[i], b[i], childPath)...)
		}
	}
	return ops
}

// escapeJSONPointer escapes a key for use in a JSON Pointer (RFC 6901).
// '~' → '~0', '/' → '~1'. Order matters: escape '~' first.
func escapeJSONPointer(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}

// valuesEqual compares two primitive (non-map, non-slice) values for equality,
// normalizing numeric types so int64(1) == float64(1).
func valuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return normalizeNumeric(a) == normalizeNumeric(b)
}

// normalizeNumeric converts integer types to float64 for consistent comparison.
func normalizeNumeric(v any) any {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int8:
		return float64(n)
	case int16:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case float32:
		return float64(n)
	default:
		return v
	}
}

// ---------------------------------------------------------------------------
// flatten
// ---------------------------------------------------------------------------

func jsFlatten(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	objArg := call.Argument(0)
	if objArg == nil || goja.IsUndefined(objArg) || goja.IsNull(objArg) {
		panic(runtime.NewTypeError("flatten requires an object argument"))
	}

	obj := objArg.Export()
	m, ok := obj.(map[string]any)
	if !ok {
		panic(runtime.NewTypeError("flatten requires an object argument"))
	}

	sep := "."
	sepArg := call.Argument(1)
	if sepArg != nil && !goja.IsUndefined(sepArg) && !goja.IsNull(sepArg) {
		sep = sepArg.String()
	}

	result := make(map[string]any)
	flattenValue(m, "", sep, result)
	return runtime.ToValue(result)
}

// flattenValue recursively flattens val into result using the given prefix and separator.
// Object keys are joined with sep; array indices use [n] notation.
func flattenValue(val any, prefix, sep string, result map[string]any) {
	switch v := val.(type) {
	case map[string]any:
		if len(v) == 0 && prefix != "" {
			result[prefix] = v
			return
		}
		for k, child := range v {
			key := k
			if prefix != "" {
				key = prefix + sep + k
			}
			flattenValue(child, key, sep, result)
		}
	case []any:
		if len(v) == 0 && prefix != "" {
			result[prefix] = v
			return
		}
		for i, child := range v {
			key := prefix + "[" + strconv.Itoa(i) + "]"
			flattenValue(child, key, sep, result)
		}
	default:
		result[prefix] = val
	}
}

// ---------------------------------------------------------------------------
// unflatten
// ---------------------------------------------------------------------------

func jsUnflatten(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	objArg := call.Argument(0)
	if objArg == nil || goja.IsUndefined(objArg) || goja.IsNull(objArg) {
		panic(runtime.NewTypeError("unflatten requires an object argument"))
	}

	obj := objArg.Export()
	m, ok := obj.(map[string]any)
	if !ok {
		panic(runtime.NewTypeError("unflatten requires an object argument"))
	}

	sep := "."
	sepArg := call.Argument(1)
	if sepArg != nil && !goja.IsUndefined(sepArg) && !goja.IsNull(sepArg) {
		sep = sepArg.String()
	}

	result := unflattenMap(m, sep)
	return runtime.ToValue(result)
}

// unflattenSeg represents one element in an unflattened key path.
type unflattenSeg struct {
	key   string
	index int
	isArr bool
}

// unflattenMap rebuilds a nested structure from a flat map.
func unflattenMap(flat map[string]any, sep string) any {
	// Sort keys for deterministic processing (important for array indices).
	keys := make([]string, 0, len(flat))
	for k := range flat {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var root any = make(map[string]any)
	for _, key := range keys {
		segments := parseUnflattenKey(key, sep)
		root = setNestedValue(root, segments, flat[key])
	}
	return root
}

// parseUnflattenKey splits a flat key into path segments.
// "a.b" with sep="." → [{key:"a"}, {key:"b"}]
// "a.c[0]" with sep="." → [{key:"a"}, {key:"c"}, {index:0, isArr:true}]
func parseUnflattenKey(key, sep string) []unflattenSeg {
	var segments []unflattenSeg
	var parts []string
	if sep == "" {
		parts = []string{key}
	} else {
		parts = strings.Split(key, sep)
	}

	for _, part := range parts {
		for len(part) > 0 {
			bracketIdx := strings.IndexByte(part, '[')
			if bracketIdx == -1 {
				if part != "" {
					segments = append(segments, unflattenSeg{key: part})
				}
				break
			}
			if bracketIdx > 0 {
				segments = append(segments, unflattenSeg{key: part[:bracketIdx]})
			}
			closeIdx := strings.IndexByte(part[bracketIdx:], ']')
			if closeIdx == -1 {
				// Malformed bracket — treat remainder as key
				segments = append(segments, unflattenSeg{key: part[bracketIdx:]})
				part = ""
				break
			}
			idxStr := part[bracketIdx+1 : bracketIdx+closeIdx]
			if idx, err := strconv.Atoi(idxStr); err == nil {
				segments = append(segments, unflattenSeg{index: idx, isArr: true})
			} else {
				// Non-numeric bracket content — treat as key
				segments = append(segments, unflattenSeg{key: part[bracketIdx : bracketIdx+closeIdx+1]})
			}
			part = part[bracketIdx+closeIdx+1:]
		}
	}
	return segments
}

// setNestedValue recursively builds the nested structure, returning the
// updated container. Each call handles one segment and recurses for the rest.
func setNestedValue(container any, segments []unflattenSeg, value any) any {
	if len(segments) == 0 {
		return value
	}

	seg := segments[0]
	rest := segments[1:]

	if seg.isArr {
		var arr []any
		if existing, ok := container.([]any); ok {
			arr = existing
		}
		for len(arr) <= seg.index {
			arr = append(arr, nil)
		}
		arr[seg.index] = setNestedValue(arr[seg.index], rest, value)
		return arr
	}

	var m map[string]any
	if existing, ok := container.(map[string]any); ok {
		m = existing
	} else {
		m = make(map[string]any)
	}
	m[seg.key] = setNestedValue(m[seg.key], rest, value)
	return m
}
