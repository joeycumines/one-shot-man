package astpack

import (
	"strings"
	"testing"
)

func TestPack_GoCallerCallee(t *testing.T) {
	files := map[string]string{
		"foo.go": `package foo
func Foo() { Bar(); Baz() }
func Bar() {}
func Baz() { Foo() }
type MyType struct{}
`,
	}
	pkg := Pack(files)
	if len(pkg.Symbols) == 0 {
		t.Fatalf("expected symbols")
	}
	// Should have Foo, Bar, Baz, MyType.
	names := make(map[string]bool)
	for _, s := range pkg.Symbols {
		names[s.Name] = true
	}
	for _, want := range []string{"Foo", "Bar", "Baz", "MyType"} {
		if !names[want] {
			t.Errorf("missing symbol %s", want)
		}
	}
	if len(pkg.Calls) == 0 {
		t.Fatalf("expected calls")
	}
	// Foo calls Bar, Baz calls Foo etc.
	found := false
	for _, c := range pkg.Calls {
		if c.Caller == "Foo" && c.Callee == "Bar" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Foo->Bar call, got %+v", pkg.Calls)
	}
}

func TestPack_JSAndPython(t *testing.T) {
	files := map[string]string{
		"app.js": `function hello() { world(); }
function world() {}
`,
		"app.py": `def foo():
    bar()
def bar():
    pass
`,
	}
	pkg := Pack(files)
	if len(pkg.Symbols) < 3 {
		t.Fatalf("expected >=3 symbols, got %d", len(pkg.Symbols))
	}
	if pkg.TokenEstimate <= 0 {
		t.Errorf("expected token estimate >0")
	}
	// Ensure token budget <4k even for large input.
	large := map[string]string{"big.go": strings.Repeat("func FooBarBaz() {}\n", 5000)}
	pkg2 := Pack(large)
	if pkg2.TokenEstimate > 4000 {
		t.Errorf("token estimate should be <4000, got %d", pkg2.TokenEstimate)
	}
}

func TestPackDiff_Extracts(t *testing.T) {
	diff := "diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n@@ -1,3 +1,4 @@\n package foo\n+func Added() {}\n func Foo() {}\n"
	pkg := PackDiff(diff)
	if len(pkg.Symbols) == 0 {
		t.Fatalf("expected symbols from diff")
	}
	found := false
	for _, s := range pkg.Symbols {
		if s.Name == "Added" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Added symbol, got %+v", pkg.Symbols)
	}
}

func TestPack_UnitTests(t *testing.T) {
	files := map[string]string{
		"foo_test.go": `package foo
func TestFoo(t *testing.T) {}
func TestBar(t *testing.T) {}
func Helper() {}
`,
	}
	pkg := Pack(files)
	if len(pkg.Tests) != 2 {
		t.Fatalf("expected 2 tests, got %d", len(pkg.Tests))
	}
}


