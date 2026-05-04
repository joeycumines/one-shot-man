// Package encodingmod provides a Goja module wrapping Go's encoding/base64 and
// encoding/hex packages for JS scripts. It is registered as "osm:encoding" and
// exposes standard and URL-safe base64 encoding/decoding plus hex encoding/decoding
// as synchronous functions. Decode errors throw JavaScript errors.
package encodingmod

import (
	"encoding/base64"
	"encoding/hex"

	"github.com/dop251/goja"
)

// Require is the Goja module loader for osm:encoding.
func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)

	// base64Encode(input: string|[]byte): string — standard base64 encoding
	_ = exports.Set("base64Encode", func(call goja.FunctionCall) goja.Value {
		data := toBytes(runtime, call.Argument(0))
		return runtime.ToValue(base64.StdEncoding.EncodeToString(data))
	})

	// base64Decode(encoded: string): string — standard base64 decoding
	_ = exports.Set("base64Decode", func(call goja.FunctionCall) goja.Value {
		encoded := argString(call, 0)
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			panic(runtime.NewGoError(newEncodingError("base64Decode: " + err.Error())))
		}
		return runtime.ToValue(string(decoded))
	})

	// base64URLEncode(input: string|[]byte): string — URL-safe base64 encoding (no padding)
	_ = exports.Set("base64URLEncode", func(call goja.FunctionCall) goja.Value {
		data := toBytes(runtime, call.Argument(0))
		return runtime.ToValue(base64.RawURLEncoding.EncodeToString(data))
	})

	// base64URLDecode(encoded: string): string — URL-safe base64 decoding (no padding)
	_ = exports.Set("base64URLDecode", func(call goja.FunctionCall) goja.Value {
		encoded := argString(call, 0)
		decoded, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			panic(runtime.NewGoError(newEncodingError("base64URLDecode: " + err.Error())))
		}
		return runtime.ToValue(string(decoded))
	})

	// hexEncode(input: string|[]byte): string — hex encoding (lowercase)
	_ = exports.Set("hexEncode", func(call goja.FunctionCall) goja.Value {
		data := toBytes(runtime, call.Argument(0))
		return runtime.ToValue(hex.EncodeToString(data))
	})

	// hexDecode(encoded: string): string — hex decoding
	_ = exports.Set("hexDecode", func(call goja.FunctionCall) goja.Value {
		encoded := argString(call, 0)
		decoded, err := hex.DecodeString(encoded)
		if err != nil {
			panic(runtime.NewGoError(newEncodingError("hexDecode: " + err.Error())))
		}
		return runtime.ToValue(string(decoded))
	})
}

// encodingError is a distinct error type for encoding/decoding failures.
type encodingError struct {
	msg string
}

func newEncodingError(msg string) *encodingError {
	return &encodingError{msg: msg}
}

func (e *encodingError) Error() string {
	return e.msg
}

// toBytes converts a Goja value to a byte slice.
// Accepts strings (converted via UTF-8) and byte arrays/slices.
// For undefined/null, returns an empty slice.
func toBytes(runtime *goja.Runtime, v goja.Value) []byte {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return []byte{}
	}

	// Try to export as []byte first (handles Uint8Array / byte arrays from JS)
	var bs []byte
	if err := runtime.ExportTo(v, &bs); err == nil {
		return bs
	}

	// Fall back to string conversion
	return []byte(v.String())
}

// argString extracts the i-th argument as a string. Returns "" for missing/undefined/null.
func argString(call goja.FunctionCall, i int) string {
	if i >= len(call.Arguments) {
		return ""
	}
	v := call.Arguments[i]
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	return v.String()
}
