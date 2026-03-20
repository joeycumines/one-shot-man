package command

import (
	"encoding/json"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  Fuzz targets for pr-split parsing and validation logic.
//
//  Each fuzz test creates a JS engine once (in the outer *testing.F scope)
//  and reuses it across iterations. This is safe because all target functions
//  are pure and synchronous — they do not mutate prSplit state.
//
//  Run individual fuzz tests:
//    go test -fuzz=FuzzClassificationParsing ./internal/command/...
//    go test -fuzz=FuzzPlanValidation         ./internal/command/...
//    go test -fuzz=FuzzValidateClassification ./internal/command/...
//    go test -fuzz=FuzzValidateSplitPlan      ./internal/command/...
//    go test -fuzz=FuzzValidateResolution     ./internal/command/...
//    go test -fuzz=FuzzIsTransientError       ./internal/command/...
// ---------------------------------------------------------------------------

// FuzzClassificationParsing fuzzes classificationToGroups (pr_split_10a)
// with arbitrary JSON input. The function accepts both array-of-categories
// and legacy {filePath: categoryName} map formats.
func FuzzClassificationParsing(f *testing.F) {
	skipSlow(f)
	evalJS := prsplittest.NewChunkEngine(f, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning",
		"04_validation", "05_execution", "06_verification",
		"07_prcreation", "08_conflict", "09_claude", "10a_pipeline_config",
	)

	// Seed: valid array format.
	f.Add(`[{"name":"api","description":"API layer","files":["api.go","handler.go"]},{"name":"ui","description":"UI layer","files":["app.js"]}]`)
	// Seed: valid legacy map format.
	f.Add(`{"api.go":"core","handler.go":"core","app.js":"ui"}`)
	// Seed: null/empty.
	f.Add(`null`)
	f.Add(`[]`)
	f.Add(`{}`)
	// Seed: missing name (skipped in array format).
	f.Add(`[{"description":"no-name","files":["a.go"]}]`)
	// Seed: not JSON.
	f.Add(`not json at all`)
	// Seed: number.
	f.Add(`42`)
	// Seed: nested object.
	f.Add(`{"a":{"b":"c"}}`)
	// Seed: binary-ish.
	f.Add("\x00\x01\xff")
	// Seed: empty string.
	f.Add(``)

	f.Fuzz(func(t *testing.T, data string) {
		// Feed raw data through JSON.parse so the JS function receives an
		// actual JS value (object/array/null/etc). If data is not valid JSON,
		// JSON.parse will throw — the function still must not panic.
		script := `(function() {
			try {
				var input = JSON.parse(` + jsonStringLiteral(data) + `);
				var result = prSplit.classificationToGroups(input);
				return JSON.stringify(result);
			} catch(e) {
				return JSON.stringify({__error: e.message});
			}
		})()`
		raw, err := evalJS(script)
		if err != nil {
			// Goja runtime errors (e.g., stack overflow) are acceptable.
			return
		}

		s, ok := raw.(string)
		if !ok {
			return
		}

		// If we got a valid result (not an error), verify structural invariants.
		var parsed map[string]json.RawMessage
		if err := json.Unmarshal([]byte(s), &parsed); err != nil {
			t.Fatalf("classificationToGroups returned non-object JSON: %s", s)
		}

		if _, isErr := parsed["__error"]; isErr {
			return // JSON.parse threw — acceptable
		}

		// Each value must have files (array) and description (any JSON type).
		// The function doesn't enforce description type — non-string values
		// like [] are passed through when present in input.
		for groupName, raw := range parsed {
			var group struct {
				Files       []json.RawMessage `json:"files"`
				Description json.RawMessage   `json:"description"`
			}
			if err := json.Unmarshal(raw, &group); err != nil {
				t.Fatalf("group %q is not {files, description}: %s", groupName, string(raw))
			}
			if group.Files == nil {
				t.Fatalf("group %q missing files field: %s", groupName, string(raw))
			}
		}
	})
}

// FuzzPlanValidation fuzzes validatePlan (pr_split_04) with arbitrary JSON.
func FuzzPlanValidation(f *testing.F) {
	skipSlow(f)
	evalJS := prsplittest.NewChunkEngine(f, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation",
	)

	// Seed: valid plan.
	f.Add(`{"splits":[{"name":"core","files":["a.go","b.go"]},{"name":"ui","files":["c.js"]}]}`)
	// Seed: no splits.
	f.Add(`{}`)
	f.Add(`null`)
	// Seed: empty splits array.
	f.Add(`{"splits":[]}`)
	// Seed: missing name.
	f.Add(`{"splits":[{"files":["a.go"]}]}`)
	// Seed: duplicate files.
	f.Add(`{"splits":[{"name":"a","files":["x.go"]},{"name":"b","files":["x.go"]}]}`)
	// Seed: invalid branch chars.
	f.Add(`{"splits":[{"name":"feat ure","files":["a.go"]}]}`)
	f.Add(`{"splits":[{"name":"a..b","files":["a.go"]}]}`)
	f.Add(`{"splits":[{"name":"ref.lock","files":["a.go"]}]}`)
	// Seed: not JSON.
	f.Add(`not json`)
	// Seed: empty.
	f.Add(``)
	// Seed: binary.
	f.Add("\x00\xff")

	f.Fuzz(func(t *testing.T, data string) {
		script := `(function() {
			try {
				var input = JSON.parse(` + jsonStringLiteral(data) + `);
				var result = JSON.stringify(prSplit.validatePlan(input));
				return result;
			} catch(e) {
				return JSON.stringify({__error: e.message});
			}
		})()`
		raw, err := evalJS(script)
		if err != nil {
			return
		}

		s, ok := raw.(string)
		if !ok {
			return
		}

		assertValidationResultInvariants(t, s)
	})
}

// FuzzValidateClassification fuzzes validateClassification (pr_split_04).
func FuzzValidateClassification(f *testing.F) {
	skipSlow(f)
	evalJS := prsplittest.NewChunkEngine(f, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation",
	)

	// Seed: valid.
	f.Add(`[{"name":"api","description":"API","files":["a.go"]}]`)
	// Seed: empty.
	f.Add(`[]`)
	f.Add(`null`)
	// Seed: missing name.
	f.Add(`[{"description":"d","files":["a.go"]}]`)
	// Seed: duplicate files across categories.
	f.Add(`[{"name":"a","description":"d","files":["x.go"]},{"name":"b","description":"d","files":["x.go"]}]`)
	// Seed: not an array.
	f.Add(`"string"`)
	f.Add(`42`)
	// Seed: not JSON.
	f.Add(`not json`)
	f.Add(``)
	f.Add("\x00")

	f.Fuzz(func(t *testing.T, data string) {
		script := `(function() {
			try {
				var input = JSON.parse(` + jsonStringLiteral(data) + `);
				return JSON.stringify(prSplit.validateClassification(input));
			} catch(e) {
				return JSON.stringify({__error: e.message});
			}
		})()`
		raw, err := evalJS(script)
		if err != nil {
			return
		}

		s, ok := raw.(string)
		if !ok {
			return
		}

		assertValidationResultInvariants(t, s)
	})
}

// FuzzValidateSplitPlan fuzzes validateSplitPlan (pr_split_04).
func FuzzValidateSplitPlan(f *testing.F) {
	skipSlow(f)
	evalJS := prsplittest.NewChunkEngine(f, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation",
	)

	// Seed: valid stages.
	f.Add(`[{"name":"stage1","files":["a.go","b.go"]},{"name":"stage2","files":["c.go"]}]`)
	// Seed: empty.
	f.Add(`[]`)
	f.Add(`null`)
	// Seed: invalid branch name.
	f.Add(`[{"name":"a b","files":["a.go"]}]`)
	// Seed: duplicate files.
	f.Add(`[{"name":"s1","files":["x"]},{"name":"s2","files":["x"]}]`)
	// Seed: not JSON.
	f.Add(`not json`)
	f.Add(``)

	f.Fuzz(func(t *testing.T, data string) {
		script := `(function() {
			try {
				var input = JSON.parse(` + jsonStringLiteral(data) + `);
				return JSON.stringify(prSplit.validateSplitPlan(input));
			} catch(e) {
				return JSON.stringify({__error: e.message});
			}
		})()`
		raw, err := evalJS(script)
		if err != nil {
			return
		}

		s, ok := raw.(string)
		if !ok {
			return
		}

		assertValidationResultInvariants(t, s)
	})
}

// FuzzValidateResolution fuzzes validateResolution (pr_split_04).
func FuzzValidateResolution(f *testing.F) {
	skipSlow(f)
	evalJS := prsplittest.NewChunkEngine(f, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation",
	)

	// Seed: valid with patches.
	f.Add(`{"patches":[{"file":"a.go","content":"package a"}]}`)
	// Seed: valid with commands.
	f.Add(`{"commands":[{"command":"go mod tidy"}]}`)
	// Seed: valid preExistingFailure.
	f.Add(`{"preExistingFailure":true,"reason":"Build was already broken on main"}`)
	// Seed: preExistingFailure without reason (invalid).
	f.Add(`{"preExistingFailure":true}`)
	// Seed: empty object.
	f.Add(`{}`)
	// Seed: null.
	f.Add(`null`)
	// Seed: not JSON.
	f.Add(`not json`)
	f.Add(``)
	// Seed: invalid patch (missing file).
	f.Add(`{"patches":[{"content":"x"}]}`)
	// Seed: invalid command (empty).
	f.Add(`{"commands":[{"command":""}]}`)

	f.Fuzz(func(t *testing.T, data string) {
		script := `(function() {
			try {
				var input = JSON.parse(` + jsonStringLiteral(data) + `);
				return JSON.stringify(prSplit.validateResolution(input));
			} catch(e) {
				return JSON.stringify({__error: e.message});
			}
		})()`
		raw, err := evalJS(script)
		if err != nil {
			return
		}

		s, ok := raw.(string)
		if !ok {
			return
		}

		assertValidationResultInvariants(t, s)
	})
}

// FuzzIsTransientError fuzzes isTransientError (pr_split_10a) which classifies
// error messages for retry decisions in the conflict resolution pipeline.
func FuzzIsTransientError(f *testing.F) {
	skipSlow(f)
	evalJS := prsplittest.NewChunkEngine(f, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning",
		"04_validation", "05_execution", "06_verification",
		"07_prcreation", "08_conflict", "09_claude", "10a_pipeline_config",
	)

	// Seed: transient errors.
	f.Add("rate limit exceeded")
	f.Add("429 Too Many Requests")
	f.Add("connection timeout")
	f.Add("ECONNRESET")
	f.Add("503 Service Unavailable")
	f.Add("server is overloaded, please try again")
	// Seed: permanent errors.
	f.Add("invalid tool use")
	f.Add("malformed JSON in request body")
	f.Add("unknown tool: foo_bar")
	// Seed: empty/null.
	f.Add("")
	// Seed: binary.
	f.Add("\x00\x01\xff")
	// Seed: long string.
	f.Add("a]very~long^error:message*with?special[chars that should not crash the regex engine")

	f.Fuzz(func(t *testing.T, msg string) {
		script := `(function() {
			var result = prSplit._isTransientError(` + jsonStringLiteral(msg) + `);
			return JSON.stringify({result: result, type: typeof result});
		})()`
		raw, err := evalJS(script)
		if err != nil {
			return
		}

		s, ok := raw.(string)
		if !ok {
			return
		}

		var parsed struct {
			Result bool   `json:"result"`
			Type   string `json:"type"`
		}
		if err := json.Unmarshal([]byte(s), &parsed); err != nil {
			t.Fatalf("isTransientError returned invalid JSON: %s", s)
		}

		// Invariant: must always return a boolean.
		if parsed.Type != "boolean" {
			t.Fatalf("isTransientError returned type %q, want boolean", parsed.Type)
		}
	})
}

// ---------------------------------------------------------------------------
//  Helpers
// ---------------------------------------------------------------------------

// jsonStringLiteral produces a JSON-encoded string literal (with surrounding
// quotes) suitable for embedding in a JavaScript expression. This safely
// escapes all special characters including newlines, tabs, quotes, and
// control characters, preventing injection via fuzzed input.
func jsonStringLiteral(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// assertValidationResultInvariants checks that a JSON string from a
// validate* function obeys the {valid, errors} contract.
func assertValidationResultInvariants(tb testing.TB, s string) {
	tb.Helper()

	var result struct {
		Valid  *bool    `json:"valid"`
		Errors []string `json:"errors"`
		Error  *string  `json:"__error"`
	}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		tb.Fatalf("validation result is not valid JSON: %s", s)
	}

	// If JSON.parse threw, this is acceptable.
	if result.Error != nil {
		return
	}

	if result.Valid == nil {
		tb.Fatalf("validation result missing 'valid' field: %s", s)
	}

	// valid=true ⟹ errors must be empty.
	if *result.Valid && len(result.Errors) > 0 {
		tb.Fatalf("valid=true but errors=%v", result.Errors)
	}

	// valid=false ⟹ errors must be non-empty.
	if !*result.Valid && len(result.Errors) == 0 {
		tb.Fatalf("valid=false but no errors: %s", s)
	}
}
