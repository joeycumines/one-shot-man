package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// TestPrSplitChunksMatchesDiscoverChunks verifies that the hardcoded
// prSplitChunks array in pr_split.go matches the chunk files discovered
// on the filesystem by prsplittest.discoverChunks(). A mismatch means
// production and tests load different code — a silent correctness hazard.
func TestPrSplitChunksMatchesDiscoverChunks(t *testing.T) {
	// Extract names from the production prSplitChunks array.
	prodNames := make([]string, len(prSplitChunks))
	for i, chunk := range prSplitChunks {
		prodNames[i] = chunk.name
	}

	// Get names from filesystem discovery (lexicographic sort).
	fsNames := prsplittest.AllChunkNames()

	// Verify same count.
	if len(prodNames) != len(fsNames) {
		t.Fatalf("chunk count mismatch: production has %d, filesystem has %d\n  production: %v\n  filesystem: %v",
			len(prodNames), len(fsNames), prodNames, fsNames)
	}

	// Verify same names in same order.
	for i := range prodNames {
		if prodNames[i] != fsNames[i] {
			t.Errorf("chunk order mismatch at index %d: production=%q, filesystem=%q",
				i, prodNames[i], fsNames[i])
		}
	}

	// Verify every production chunk has a non-nil source pointer.
	for i, chunk := range prSplitChunks {
		if chunk.source == nil {
			t.Errorf("prSplitChunks[%d] (%q) has nil source pointer — embedded JS will be empty", i, chunk.name)
		} else if *chunk.source == "" {
			t.Errorf("prSplitChunks[%d] (%q) has empty source — //go:embed directive may be missing", i, chunk.name)
		}
	}
}

// TestPrSplitChunksNoDuplicates verifies that no chunk name appears
// twice in the prSplitChunks array — a typo could silently drop a chunk.
func TestPrSplitChunksNoDuplicates(t *testing.T) {
	seen := make(map[string]int, len(prSplitChunks))
	for i, chunk := range prSplitChunks {
		if prev, ok := seen[chunk.name]; ok {
			t.Errorf("duplicate chunk name %q at indices %d and %d", chunk.name, prev, i)
		}
		seen[chunk.name] = i
	}
}

// TestPrSplitChunksFileSystemNoOrphans verifies that every .js file on disk
// is accounted for in the prSplitChunks array — no orphan chunks that are
// loaded by tests but forgotten by production.
func TestPrSplitChunksFileSystemNoOrphans(t *testing.T) {
	fsNames := prsplittest.AllChunkNames()
	prodSet := make(map[string]struct{}, len(prSplitChunks))
	for _, chunk := range prSplitChunks {
		prodSet[chunk.name] = struct{}{}
	}

	for _, name := range fsNames {
		if _, ok := prodSet[name]; !ok {
			t.Errorf("orphan chunk on filesystem not in prSplitChunks: %q (loaded by tests but NOT by production)", name)
		}
	}
}
