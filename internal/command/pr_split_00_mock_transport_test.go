package command

import (
	"encoding/json"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// The shared mock must preserve the same independent process facts as the
// native bindings. Otherwise async tests can pass by silently converting an
// exit-0 capture or tree-cleanup failure into a complete successful command.
func TestGitMockSpawnPreservesIndependentProcessFacts(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the JavaScript engine")
	}
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, "00_core")
	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatal(err)
	}
	raw, err := evalJS(`(async function() {
		var results = [];
		for (var code of [0, 7]) {
			for (var truncated of [false, true]) {
				for (var treeFailure of [false, true]) {
					var fixture = {
						stdout: 'captured prefix\n', stderr: 'diagnostic\n', code: code,
						error: code !== 0 || truncated || treeFailure,
						truncated: truncated, message: ''
					};
					if (treeFailure) fixture.treeCleanupError = 'tree close denied';
					_gitResponses['status'] = fixture;
					var child = await require('osm:exec').spawn('git', ['status']);
					var observed = await prSplit._collectSpawnOutput(child, null);
					results.push({fixture: fixture, observed: observed,
						commandFailed: prSplit._commandFailed(observed)});
				}
			}
		}
		return JSON.stringify(results);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var results []struct {
		Fixture struct {
			Code             int    `json:"code"`
			Error            bool   `json:"error"`
			Truncated        bool   `json:"truncated"`
			TreeCleanupError string `json:"treeCleanupError"`
		} `json:"fixture"`
		Observed struct {
			Code             int    `json:"code"`
			Error            bool   `json:"error"`
			Truncated        bool   `json:"truncated"`
			TreeCleanupError string `json:"treeCleanupError"`
			Stdout           string `json:"stdout"`
			Stderr           string `json:"stderr"`
		} `json:"observed"`
		CommandFailed bool `json:"commandFailed"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 8 {
		t.Fatalf("received %d cases, want 8", len(results))
	}
	for _, result := range results {
		want, got := result.Fixture, result.Observed
		if got.Code != want.Code || got.Error != want.Error || got.Truncated != want.Truncated ||
			got.TreeCleanupError != want.TreeCleanupError || result.CommandFailed != want.Error {
			t.Errorf("mock lost process facts: fixture=%+v observed=%+v commandFailed=%v", want, got, result.CommandFailed)
		}
		if got.Stdout != "captured prefix\n" || got.Stderr != "diagnostic\n" {
			t.Errorf("mock lost captured output: %+v", got)
		}
	}
}
