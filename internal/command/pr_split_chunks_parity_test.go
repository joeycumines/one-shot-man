package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// TestPrSplitChunksMatchesDiscoverChunks verifies that the manifest chunk IDs
// match the chunk files discovered on the filesystem by prsplittest.
// A mismatch means production and tests load different code — a silent
// correctness hazard.
func TestPrSplitChunksMatchesDiscoverChunks(t *testing.T) {
	manifestNames := make([]string, len(prSplitManifestData.Chunks))
	for i, entry := range prSplitManifestData.Chunks {
		manifestNames[i] = entry.ID
	}

	fsNames := prsplittest.AllChunkNames()

	if len(manifestNames) != len(fsNames) {
		t.Fatalf("chunk count mismatch: manifest has %d, filesystem has %d\n  manifest: %v\n  filesystem: %v",
			len(manifestNames), len(fsNames), manifestNames, fsNames)
	}

	for i := range manifestNames {
		if manifestNames[i] != fsNames[i] {
			t.Errorf("chunk order mismatch at index %d: manifest=%q, filesystem=%q",
				i, manifestNames[i], fsNames[i])
		}
	}

	for _, entry := range prSplitManifestData.Chunks {
		data, err := chunkFS.ReadFile(entry.File)
		if err != nil {
			t.Errorf("chunk %q (%s): not found in embedded FS: %v", entry.ID, entry.File, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("chunk %q (%s): empty source in embedded FS", entry.ID, entry.File)
		}
	}
}

// TestPrSplitChunksNoDuplicates verifies that no chunk ID appears
// twice in the manifest — a typo could silently drop a chunk.
func TestPrSplitChunksNoDuplicates(t *testing.T) {
	seen := make(map[string]int, len(prSplitManifestData.Chunks))
	for i, entry := range prSplitManifestData.Chunks {
		if prev, ok := seen[entry.ID]; ok {
			t.Errorf("duplicate chunk ID %q at indices %d and %d", entry.ID, prev, i)
		}
		seen[entry.ID] = i
	}
}

// TestPrSplitChunksFileSystemNoOrphans verifies that every .js file on disk
// is accounted for in the manifest — no orphan chunks that are loaded by
// tests but forgotten by production.
func TestPrSplitChunksFileSystemNoOrphans(t *testing.T) {
	fsNames := prsplittest.AllChunkNames()
	manifestSet := make(map[string]struct{}, len(prSplitManifestData.Chunks))
	for _, entry := range prSplitManifestData.Chunks {
		manifestSet[entry.ID] = struct{}{}
	}

	for _, name := range fsNames {
		if _, ok := manifestSet[name]; !ok {
			t.Errorf("orphan chunk on filesystem not in manifest: %q (loaded by tests but NOT by production)", name)
		}
	}
}
