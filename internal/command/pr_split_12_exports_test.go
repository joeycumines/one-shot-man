package command

import (
	"encoding/json"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  Chunk 12: Exports — manifest validation and VERSION
// ---------------------------------------------------------------------------

// allChunksThrough12 loads chunks 00-12 for manifest validation tests.
var allChunksThrough12 = []string{
	"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation",
	"05_execution", "06_verification", "07_prcreation", "08_conflict",
	"09_claude",
	"10a_pipeline_config", "10b_pipeline_send", "10c_pipeline_resolve", "10d_pipeline_orchestrator",
	"11_utilities", "12_exports",
}

func TestChunk12_VERSION(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough12...)

	raw, err := evalJS(`globalThis.prSplit.VERSION`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "6.0.0" {
		t.Errorf("VERSION = %v, want '6.0.0'", raw)
	}
}

func TestChunk12_NoMissingExports(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough12...)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit._missingExports)`)
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	if err := json.Unmarshal([]byte(raw.(string)), &missing); err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Errorf("missing exports after loading chunks 00-12: %v", missing)
	}
}

func TestChunk12_ManifestCoversAllExports(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough12...)

	// The manifest should have a reasonable number of entries.
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit._EXPECTED_EXPORTS.length)`)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := json.Unmarshal([]byte(raw.(string)), &count); err != nil {
		t.Fatal(err)
	}
	// We know there are 86+ exports across chunks 00-11.
	if count < 80 {
		t.Errorf("manifest has only %d entries, expected at least 80", count)
	}
}

func TestChunk12_AllManifestEntriesAreExported(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough12...)

	// For each entry in the manifest, verify it exists and is not undefined.
	raw, err := evalJS(`
		var exports = globalThis.prSplit._EXPECTED_EXPORTS;
		var bad = [];
		for (var i = 0; i < exports.length; i++) {
			if (typeof globalThis.prSplit[exports[i]] === 'undefined') {
				bad.push(exports[i]);
			}
		}
		JSON.stringify(bad)
	`)
	if err != nil {
		t.Fatal(err)
	}
	var bad []string
	if err := json.Unmarshal([]byte(raw.(string)), &bad); err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Errorf("exports in manifest but undefined on prSplit: %v", bad)
	}
}
