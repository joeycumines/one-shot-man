package tokenizermod

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/tokenizer"
)

// Require is the Goja module loader for osm:tokenizer.
// It registers under "osm:tokenizer" and exposes tokenization utilities.
//
// API (JS):
//
//	const tok = require('osm:tokenizer');
//
//	// Tokenize using built-in char-level tokenizer (no JSON needed)
//	const r = tok.tokenize("hello world");
//	// r = { tokens: [{id, value, offsets}, ...], count: N }
//	const n = tok.count("hello world");
//
//	// Load a HuggingFace tokenizer from file (unified entry point)
//	const t = tok.loadFromFile("/path/to/tokenizer.json");
//	// t.encode(text) -> { tokens, count }
//	// t.count(text) -> number
//
//	// Load from JSON string (unified entry point)
//	const t2 = tok.loadFromJSON('{"type":"BPE","vocab":...}');
//
//	// Model-specific loaders
//	const bpe = tok.loadBPEFromFiles(vocabJson, mergesStr);
//	const wp = tok.loadWordPieceFromJSON(jsonStr);
//	const wl = tok.loadWordLevelFromJSON(jsonStr);
func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)

	// ---- tokenize(text: string): { tokens: Array, count: number } ----
	// Uses the built-in char-level tokenizer. Always available.
	_ = exports.Set("tokenize", func(call goja.FunctionCall) goja.Value {
		text := argString(call, 0)
		tok := tokenizer.NewCharTokenizer()
		tokens, count, err := tok.Encode(text)
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("tokenize: %w", err)))
		}
		return tokenResult(runtime, tokens, count)
	})

	// ---- count(text: string): number ----
	// Returns only the token count using the built-in char tokenizer.
	// Returns -1 on error (distinguishing "zero tokens" from "error occurred").
	_ = exports.Set("count", func(call goja.FunctionCall) goja.Value {
		text := argString(call, 0)
		tok := tokenizer.NewCharTokenizer()
		count, err := tok.TokenCount(text)
		if err != nil {
			return runtime.ToValue(-1)
		}
		return runtime.ToValue(count)
	})

	// ---- byteCount(text: string): number ----
	// Returns the UTF-8 byte length of the string.
	_ = exports.Set("byteCount", func(call goja.FunctionCall) goja.Value {
		text := argString(call, 0)
		return runtime.ToValue(len(text))
	})

	// ---- lineCount(text: string): number ----
	// Returns the number of lines (newline count + 1). Empty text returns 0.
	_ = exports.Set("lineCount", func(call goja.FunctionCall) goja.Value {
		text := argString(call, 0)
		if text == "" {
			return runtime.ToValue(0)
		}
		return runtime.ToValue(strings.Count(text, "\n") + 1)
	})

	// ---- loadFromFile(path: string): TokenizerWrapper ----
	// Loads a HuggingFace tokenizer.json from disk.
	_ = exports.Set("loadFromFile", func(call goja.FunctionCall) goja.Value {
		path := argString(call, 0)
		if path == "" {
			panic(runtime.NewGoError(fmt.Errorf("loadFromFile: path is required")))
		}
		tok, err := tokenizer.LoadTokenizerFromFile(path)
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("loadFromFile: %w", err)))
		}
		return newTokenizerWrapper(runtime, tok)
	})

	// ---- loadFromJSON(jsonStr: string): TokenizerWrapper ----
	// Loads a HuggingFace tokenizer from a JSON string.
	_ = exports.Set("loadFromJSON", func(call goja.FunctionCall) goja.Value {
		jsonStr := argString(call, 0)
		if jsonStr == "" {
			panic(runtime.NewGoError(fmt.Errorf("loadFromJSON: json string is required")))
		}
		tok, err := tokenizer.LoadTokenizerFromJSON(strings.NewReader(jsonStr))
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("loadFromJSON: %w", err)))
		}
		return newTokenizerWrapper(runtime, tok)
	})

	// ---- loadBPEFromFiles(vocabJson: string, mergesStr: string): TokenizerWrapper ----
	// Loads a BPE tokenizer from separate vocab JSON and merges text.
	_ = exports.Set("loadBPEFromFiles", func(call goja.FunctionCall) goja.Value {
		vocabStr := argString(call, 0)
		mergesStr := argString(call, 1)
		if vocabStr == "" {
			panic(runtime.NewGoError(fmt.Errorf("loadBPEFromFiles: vocab JSON is required")))
		}
		model, err := tokenizer.LoadBPEFromFiles(
			strings.NewReader(vocabStr),
			strings.NewReader(mergesStr),
		)
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("loadBPEFromFiles: %w", err)))
		}
		return newTokenizerWrapper(runtime, &tokenizer.Tokenizer{Model: model})
	})

	// ---- loadWordPieceFromJSON(jsonStr: string): TokenizerWrapper ----
	// Loads a WordPiece tokenizer from a JSON string.
	_ = exports.Set("loadWordPieceFromJSON", func(call goja.FunctionCall) goja.Value {
		jsonStr := argString(call, 0)
		if jsonStr == "" {
			panic(runtime.NewGoError(fmt.Errorf("loadWordPieceFromJSON: json string is required")))
		}
		model, err := tokenizer.LoadWordPieceFromJSON(strings.NewReader(jsonStr))
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("loadWordPieceFromJSON: %w", err)))
		}
		return newTokenizerWrapper(runtime, &tokenizer.Tokenizer{Model: model})
	})

	// ---- loadWordLevelFromJSON(jsonStr: string): TokenizerWrapper ----
	// Loads a WordLevel tokenizer from a JSON string.
	_ = exports.Set("loadWordLevelFromJSON", func(call goja.FunctionCall) goja.Value {
		jsonStr := argString(call, 0)
		if jsonStr == "" {
			panic(runtime.NewGoError(fmt.Errorf("loadWordLevelFromJSON: json string is required")))
		}
		model, err := tokenizer.LoadWordLevelFromJSON(strings.NewReader(jsonStr))
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("loadWordLevelFromJSON: %w", err)))
		}
		return newTokenizerWrapper(runtime, &tokenizer.Tokenizer{Model: model})
	})
}

// newTokenizerWrapper creates a JS object wrapping a Go *tokenizer.Tokenizer
// with encode() and count() methods.
func newTokenizerWrapper(runtime *goja.Runtime, tok *tokenizer.Tokenizer) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("encode", func(call goja.FunctionCall) goja.Value {
		text := argString(call, 0)
		tokens, count, err := tok.Encode(text)
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("encode: %w", err)))
		}
		return tokenResult(runtime, tokens, count)
	})

	_ = obj.Set("count", func(call goja.FunctionCall) goja.Value {
		text := argString(call, 0)
		count, err := tok.TokenCount(text)
		if err != nil {
			return runtime.ToValue(-1)
		}
		return runtime.ToValue(count)
	})

	return runtime.ToValue(obj)
}

// tokenResult converts Go Token.Offsets (UTF-8 byte indices) to JS objects.
// JS consumers must not use these offsets to slice JS strings —
// the indices refer to UTF-8 byte positions, not UTF-16 code unit positions.
func tokenResult(runtime *goja.Runtime, tokens tokenizer.Result, count int) goja.Value {
	obj := runtime.NewObject()
	_ = obj.Set("count", count)

	jsTokens := runtime.NewArray()
	idx := 0
	for _, t := range tokens {
		jt := runtime.NewObject()
		_ = jt.Set("id", int64(t.ID))
		_ = jt.Set("value", t.Value)
		offsetsArr := runtime.NewArray()
		_ = offsetsArr.Set("0", int64(t.Offsets[0]))
		_ = offsetsArr.Set("1", int64(t.Offsets[1]))
		_ = jt.Set("offsets", offsetsArr)
		_ = jsTokens.Set(fmt.Sprintf("%d", idx), jt)
		idx++
	}
	_ = obj.Set("tokens", jsTokens)

	return runtime.ToValue(obj)
}

// argString extracts the i-th argument as a string, or returns "".
func argString(call goja.FunctionCall, i int) string {
	if i >= len(call.Arguments) {
		return ""
	}
	v := call.Argument(i)
	if goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	return v.String()
}
