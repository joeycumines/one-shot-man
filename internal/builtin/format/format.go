package format

import (
	"fmt"

	"github.com/dop251/goja"
)

// Require is the Goja module loader for osm:format.
// It exposes formatNum(n) and formatBytes(n) for consistent
// number and byte formatting across all JS scripts.
//
// API (JS):
//
//	const fmt = require('osm:format');
//	fmt.formatNum(1234)     // "1,234"
//	fmt.formatNum(50000)    // "50.0k"
//	fmt.formatBytes(2048)   // "2.0 kB"
func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)

	_ = exports.Set("formatNum", func(call goja.FunctionCall) goja.Value {
		n := toInt(call, 0)
		return runtime.ToValue(formatNum(n))
	})

	_ = exports.Set("formatBytes", func(call goja.FunctionCall) goja.Value {
		n := toInt(call, 0)
		return runtime.ToValue(formatBytes(n))
	})
}

// toInt extracts the i-th argument as int64, defaulting to 0.
func toInt(call goja.FunctionCall, i int) int64 {
	if i >= len(call.Arguments) {
		return 0
	}
	v := call.Argument(i)
	if goja.IsUndefined(v) || goja.IsNull(v) {
		return 0
	}
	return v.ToInteger()
}

// formatNum formats a number with comma grouping for values < 10000
// and SI-style notation (k/M/G) for larger values.
// This replicates the JS implementation previously copy-pasted across
// contextManager.js, prompt_flow_script.js, and super_document_script.js.
func formatNum(n int64) string {
	if n < 0 {
		return "0"
	}
	if n < 10000 {
		s := fmt.Sprintf("%d", n)
		if len(s) <= 3 {
			return s
		}
		var result string
		for i := range len(s) {
			if i > 0 && (len(s)-i)%3 == 0 {
				result += ","
			}
			result += string(s[i])
		}
		return result
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	if n < 1_000_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	return fmt.Sprintf("%.1fG", float64(n)/1_000_000_000)
}

// formatBytes formats a byte count using binary (IEC) notation:
// B for < 1024, then kB/MB/GB/TB with one decimal place.
func formatBytes(n int64) string {
	if n < 0 {
		return "0 B"
	}
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"kB", "MB", "GB", "TB"}
	value := float64(n)
	exp := 0
	for value >= 1024 && exp < len(units) {
		value /= 1024
		exp++
	}
	return fmt.Sprintf("%.1f %s", value, units[exp-1])
}
