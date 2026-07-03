package jscompliance

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSlow_Gitops_RepoContract pins the osm:gitops SYNC surface against a real
// temp repo (built with the git CLI): isRepo, open, headBranchName.
// gitops is pr-split's backbone; this is its first JS-layer behavioral test.
func TestSlow_Gitops_RepoContract(t *testing.T) {
	skipSlow(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	t.Parallel()

	repoDir := t.TempDir()
	// git init + identity config (gpgsign off so commit doesn't need a key).
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "jscompliance@test"},
		{"config", "user.name", "jscompliance"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// One committed file (HEAD must exist for branch lookups).
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	emptyDir := t.TempDir() // not a repo

	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)

	// isRepo: true for the repo, false for the empty dir.
	if v, err := evalJS(t, engine, `require('osm:gitops').isRepo(`+jsStringLit(repoDir)+`)`, defaultEvalTimeout); err != nil {
		t.Fatalf("isRepo(repo): %v", err)
	} else if b, _ := v.(bool); !b {
		t.Errorf("gitops.isRepo(repoDir) = %v, want true", v)
	}
	if v, _ := evalJS(t, engine, `require('osm:gitops').isRepo(`+jsStringLit(emptyDir)+`)`, defaultEvalTimeout); v != nil {
		if b, _ := v.(bool); b {
			t.Errorf("gitops.isRepo(emptyDir) = %v, want false", v)
		}
	}

	// headBranchName: a non-empty branch name.
	if v, err := evalJS(t, engine, `require('osm:gitops').headBranchName(`+jsStringLit(repoDir)+`)`, defaultEvalTimeout); err != nil {
		t.Fatalf("headBranchName: %v", err)
	} else if s, _ := v.(string); strings.TrimSpace(s) == "" {
		t.Errorf("gitops.headBranchName(repoDir) = %q, want a non-empty branch", s)
	}

	// open returns a Repo object exposing the sync methods.
	if v, err := evalJS(t, engine, `(function(){ var r = require('osm:gitops').open(`+jsStringLit(repoDir)+`); return typeof r + ':' + typeof r.headBranchName; })()`, defaultEvalTimeout); err != nil {
		t.Fatalf("open: %v", err)
	} else if s, _ := v.(string); !strings.HasPrefix(s, "object:function") {
		t.Errorf("gitops.open(repoDir) = %q, want an object with a headBranchName function", s)
	}

	// Async value round-trip: write a new file, addAll (async), hasStagedChanges
	// (async) → true, commit (async) → a commit hash string. Pins the documented
	// async Repo methods with VALUE assertions (not just Promise-shape).
	newFile := filepath.Join(repoDir, "second.txt")
	if err := os.WriteFile(newFile, []byte("second\n"), 0o600); err != nil {
		t.Fatalf("write second file: %v", err)
	}
	v, err := evalJS(t, engine, `(async function () {
		var r = require('osm:gitops').open(`+jsStringLit(repoDir)+`);
		await r.addAll();
		var staged = await r.hasStagedChanges();
		var hash = await r.commit('second commit');
		return JSON.stringify({ staged: staged, hashType: typeof hash, hashLen: String(hash).length });
	})()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("gitops async addAll/hasStagedChanges/commit: %v", err)
	}
	s, _ := v.(string)
	if !strings.Contains(s, `"staged":true`) {
		t.Errorf("gitops hasStagedChanges after addAll should be true; got %s", s)
	}
	if !strings.Contains(s, `"hashType":"string"`) {
		t.Errorf("gitops commit should resolve to a string hash; got %s", s)
	}
}
