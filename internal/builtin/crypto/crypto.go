// Package cryptomod provides a Goja module wrapping Go's crypto hash functions for JS scripts.
// It is registered as "osm:crypto" and exposes SHA-256, SHA-1, MD5, HMAC-SHA256, and HMAC-SHA1
// as synchronous functions returning hex-encoded lowercase strings.
package cryptomod

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"

	"github.com/dop251/goja"
)

// Require is the Goja module loader for osm:crypto.
func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)

	// sha256(input: string|[]byte): string — hex-encoded SHA-256
	_ = exports.Set("sha256", func(call goja.FunctionCall) goja.Value {
		data := toBytes(runtime, call.Argument(0))
		sum := sha256.Sum256(data)
		return runtime.ToValue(hex.EncodeToString(sum[:]))
	})

	// sha1(input: string|[]byte): string — hex-encoded SHA-1
	_ = exports.Set("sha1", func(call goja.FunctionCall) goja.Value {
		data := toBytes(runtime, call.Argument(0))
		sum := sha1.Sum(data)
		return runtime.ToValue(hex.EncodeToString(sum[:]))
	})

	// md5(input: string|[]byte): string — hex-encoded MD5
	_ = exports.Set("md5", func(call goja.FunctionCall) goja.Value {
		data := toBytes(runtime, call.Argument(0))
		sum := md5.Sum(data)
		return runtime.ToValue(hex.EncodeToString(sum[:]))
	})

	// hmacSHA256(key: string|[]byte, message: string|[]byte): string — hex-encoded HMAC-SHA256
	_ = exports.Set("hmacSHA256", func(call goja.FunctionCall) goja.Value {
		key := toBytes(runtime, call.Argument(0))
		msg := toBytes(runtime, call.Argument(1))
		mac := hmac.New(sha256.New, key)
		mac.Write(msg)
		return runtime.ToValue(hex.EncodeToString(mac.Sum(nil)))
	})

	// hmacSHA1(key: string|[]byte, message: string|[]byte): string — hex-encoded HMAC-SHA1
	_ = exports.Set("hmacSHA1", func(call goja.FunctionCall) goja.Value {
		key := toBytes(runtime, call.Argument(0))
		msg := toBytes(runtime, call.Argument(1))
		mac := hmac.New(sha1.New, key)
		mac.Write(msg)
		return runtime.ToValue(hex.EncodeToString(mac.Sum(nil)))
	})
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
