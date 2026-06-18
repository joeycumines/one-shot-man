package command

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestChunkManifest_Loads(t *testing.T) {
	if len(prSplitManifestData.Chunks) == 0 {
		t.Fatal("manifest has no chunks")
	}
	if prSplitManifestData.Version != "6.0.0" {
		t.Errorf("manifest version = %q, want %q", prSplitManifestData.Version, "6.0.0")
	}
	for i, entry := range prSplitManifestData.Chunks {
		if entry.ID == "" {
			t.Errorf("chunks[%d]: empty ID", i)
		}
		if entry.File == "" {
			t.Errorf("chunks[%d]: empty File", i)
		}
	}
}

func TestChunkManifest_AllFilesExist(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(filename)

	for _, entry := range prSplitManifestData.Chunks {
		path := filepath.Join(dir, entry.File)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("chunk file %q (%s) does not exist: %v", entry.ID, entry.File, err)
		}
	}
}

func TestChunkManifest_AllExportsPresent(t *testing.T) {
	var allExports []string
	for _, entry := range prSplitManifestData.Chunks {
		allExports = append(allExports, entry.Exports...)
	}
	if len(allExports) == 0 {
		t.Skip("manifest declares no exports")
	}

	evalJS := prsplittest.NewChunkEngine(t, nil, prsplittest.AllChunkNames()...)

	for _, export := range allExports {
		raw, err := evalJS(`typeof globalThis.prSplit["` + export + `"]`)
		if err != nil {
			t.Errorf("error checking export %q: %v", export, err)
			continue
		}
		typ, ok := raw.(string)
		if !ok {
			t.Errorf("export %q: typeof returned %T, want string", export, raw)
			continue
		}
		if typ == "undefined" {
			t.Errorf("export %q is undefined on globalThis.prSplit", export)
		}
	}
}
