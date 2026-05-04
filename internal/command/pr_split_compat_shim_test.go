package command

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// TestChunkCompatShim_CoversAllExports verifies that every export listed in
// pr_split_12_exports.js EXPECTED_EXPORTS is accessible as a bare global after
// the ChunkCompatShim is applied. This prevents silent test failures where a
// legacy test uses a bare global name that the shim doesn't proxy — the value
// would be undefined instead of throwing an error.
func TestChunkCompatShim_CoversAllExports(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping JS engine test in short mode")
	}

	evalJS := prsplittest.NewFullEngine(t, nil)

	// Internal-only exports (underscore-prefixed) that were never bare globals
	// in the monolith and should NOT be proxied by the shim.
	internalOnly := map[string]bool{
		"_state": true, "_modules": true, "_style": true, "_cfg": true,
		"_COMMAND_NAME": true, "_MODE_NAME": true,
		"_gitExec": true, "_gitExecAsync": true,
		"_resolveDir": true, "_shellQuote": true,
		"_gitAddChangedFiles": true, "_gitAddChangedFilesAsync": true,
		"_dirname": true, "_fileExtension": true,
		"_sanitizeBranchName": true, "_padIndex": true,
		"_splitsAreIndependent": true,
	}

	// Build JS internalOnly map.
	var jsMapParts []string
	for k := range internalOnly {
		jsMapParts = append(jsMapParts, `"`+k+`":true`)
	}
	jsMapStr := "{" + strings.Join(jsMapParts, ",") + "}"

	// Check each export in EXPECTED_EXPORTS for bare global availability.
	code := `
	(function() {
		var exports = prSplit._EXPECTED_EXPORTS;
		var internalOnly = ` + jsMapStr + `;
		var missing = [];
		for (var i = 0; i < exports.length; i++) {
			var name = exports[i];
			if (internalOnly[name]) continue;
			if (typeof globalThis[name] === 'undefined') {
				missing.push(name);
			}
		}
		return missing.join(',');
	})()
	`

	result, err := evalJS(code)
	if err != nil {
		t.Fatalf("failed to check shim coverage: %v", err)
	}
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("shim check returned %T, want string", result)
	}

	if resultStr != "" {
		missing := strings.Split(resultStr, ",")
		t.Errorf("ChunkCompatShim is missing %d exports that are in EXPECTED_EXPORTS but not accessible as bare globals:", len(missing))
		for _, name := range missing {
			t.Errorf("  - %q", name)
		}
		t.Error("These exports will be undefined when accessed as bare globals in legacy test code. Add them to the ChunkCompatShim funcNames, internalNames, or constants section.")
	}
}
