package triage

import "testing"

func TestTriageDiff_TrivialWhitespace(t *testing.T) {
	diff := "diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n@@ -1,2 +1,2 @@\n-foo  bar\n+foo   bar\n"
	results := TriageDiff(diff)
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].Kind != Trivial && results[0].Kind != SemanticReview {
		t.Errorf("whitespace diff should be trivial or semantic, got %s", results[0].Kind)
	}
}

func TestTriageDiff_Lockfile(t *testing.T) {
	diff := "diff --git a/package-lock.json b/package-lock.json\n--- a/package-lock.json\n+++ b/package-lock.json\n@@ -1,2 +1,2 @@\n-{\"a\":1}\n+{\"a\":2}\n"
	results := TriageDiff(diff)
	if len(results) != 1 || results[0].Kind != Trivial {
		t.Fatalf("lockfile should be trivial, got %+v", results)
	}
}

func TestTriageDiff_Generated(t *testing.T) {
	diff := "diff --git a/gen.go b/gen.go\n--- a/gen.go\n+++ b/gen.go\n@@ -1,2 +1,2 @@\n-// Code generated DO NOT EDIT\n+// Code generated DO NOT EDIT.\n foo\n"
	results := TriageDiff(diff)
	if len(results) != 1 || results[0].Kind != Trivial {
		t.Fatalf("generated should be trivial, got %+v", results)
	}
}

func TestTriageDiff_Semantic(t *testing.T) {
	diff := "diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n@@ -1,3 +1,4 @@\n package foo\n+func NewFoo() {}\n func Foo() {}\n"
	results := TriageDiff(diff)
	if len(results) != 1 || results[0].Kind != SemanticReview {
		t.Fatalf("semantic should be semantic_review, got %+v", results)
	}
}

func TestTriageDiff_HighRisk(t *testing.T) {
	diff := "diff --git a/auth.go b/auth.go\n--- a/auth.go\n+++ b/auth.go\n@@ -1,2 +1,2 @@\n-foo\n+password := \"secret123\"\n"
	results := TriageDiff(diff)
	if len(results) != 1 || results[0].Kind != HighRiskSecurity {
		t.Fatalf("high-risk should be HIGH_RISK_SECURITY, got %+v", results)
	}
}

func TestTriageDiff_EliminationRate(t *testing.T) {
	var diff string
	diff += "diff --git a/go.mod b/go.mod\n--- a/go.mod\n+++ b/go.mod\n@@ -1,2 +1,2 @@\n-module foo\n+module foo\n"
	diff += "diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n@@ -1,2 +1,2 @@\n-hello world\n+hello  world\n"
	diff += "diff --git a/yarn.lock b/yarn.lock\n--- a/yarn.lock\n+++ b/yarn.lock\n@@ -1,2 +1,2 @@\n-a\n+b\n"
	diff += "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1,2 +1,3 @@\n package main\n+func Foo(){}\n"
	results := TriageDiff(diff)
	trivial := 0
	for _, r := range results {
		if r.Kind == Trivial {
			trivial++
		}
	}
	if trivial < 1 {
		t.Errorf("expected >=1 trivial, got %d in %+v", trivial, results)
	}
}

func TestTriageDiff_CommentsOnly(t *testing.T) {
	diff := "diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n@@ -1,3 +1,3 @@\n package foo\n-// old comment\n+// new comment\n func Foo() {}\n"
	results := TriageDiff(diff)
	if len(results) != 1 {
		t.Fatalf("expected 1")
	}
}
