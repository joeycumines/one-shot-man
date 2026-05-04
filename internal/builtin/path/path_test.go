package pathmod

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dop251/goja"
)

func setup(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	module := vm.NewObject()
	exports := vm.NewObject()
	_ = module.Set("exports", exports)
	Require(vm, module)
	_ = vm.Set("path", module.Get("exports"))
	return vm
}

// --- join ---

func TestJoinBasic(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.join("a", "b", "c")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join("a", "b", "c")
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestJoinSingleArg(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.join("a")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "a" {
		t.Fatalf("expected 'a', got %q", v.String())
	}
}

func TestJoinNoArgs(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.join()`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join()
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestJoinAbsolutePath(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	var script string
	if runtime.GOOS == "windows" {
		script = `path.join("C:\\Users", "test", "file.txt")`
	} else {
		script = `path.join("/usr", "local", "bin")`
	}
	v, err := vm.RunString(script)
	if err != nil {
		t.Fatal(err)
	}
	var expected string
	if runtime.GOOS == "windows" {
		expected = filepath.Join("C:\\Users", "test", "file.txt")
	} else {
		expected = filepath.Join("/usr", "local", "bin")
	}
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestJoinWithDotDot(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.join("a", "b", "..", "c")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join("a", "b", "..", "c")
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestJoinEmptyStrings(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.join("", "a", "", "b")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join("", "a", "", "b")
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- dir ---

func TestDirBasic(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.dir("a/b/c.txt")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Dir("a/b/c.txt")
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestDirRootPath(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	var script string
	if runtime.GOOS == "windows" {
		script = `path.dir("C:\\")`
	} else {
		script = `path.dir("/")`
	}
	v, err := vm.RunString(script)
	if err != nil {
		t.Fatal(err)
	}
	var expected string
	if runtime.GOOS == "windows" {
		expected = filepath.Dir("C:\\")
	} else {
		expected = filepath.Dir("/")
	}
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestDirEmpty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.dir("")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Dir("")
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestDirNoArgs(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.dir()`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Dir("")
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- base ---

func TestBaseBasic(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.base("a/b/file.txt")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Base("a/b/file.txt")
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestBaseNoExt(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.base("a/b/Makefile")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "Makefile" {
		t.Fatalf("expected 'Makefile', got %q", v.String())
	}
}

func TestBaseEmpty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.base("")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Base("")
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- ext ---

func TestExtBasic(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.ext("file.txt")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != ".txt" {
		t.Fatalf("expected '.txt', got %q", v.String())
	}
}

func TestExtMultipleDots(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.ext("archive.tar.gz")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != ".gz" {
		t.Fatalf("expected '.gz', got %q", v.String())
	}
}

func TestExtNoExtension(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.ext("Makefile")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected '', got %q", v.String())
	}
}

func TestExtEmpty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.ext("")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected '', got %q", v.String())
	}
}

func TestExtDotFile(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.ext(".gitignore")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Ext(".gitignore")
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- abs ---

func TestAbsRelative(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = path.abs("relative/path");
		if (r.error !== null) throw new Error("expected no error, got: " + r.error);
		if (r.result === "") throw new Error("expected non-empty result");
		if (!path.isAbs(r.result)) throw new Error("expected absolute path, got: " + r.result);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAbsAlreadyAbsolute(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	var script string
	if runtime.GOOS == "windows" {
		script = `
			var r = path.abs("C:\\Windows\\System32");
			if (r.error !== null) throw new Error("expected no error");
			if (r.result !== "C:\\Windows\\System32") throw new Error("expected same path back, got: " + r.result);
		`
	} else {
		script = `
			var r = path.abs("/usr/local/bin");
			if (r.error !== null) throw new Error("expected no error");
			if (r.result !== "/usr/local/bin") throw new Error("expected same path back, got: " + r.result);
		`
	}
	_, err := vm.RunString(script)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAbsEmpty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = path.abs("");
		if (r.error !== null) throw new Error("expected no error for empty string, got: " + r.error);
		if (r.result === "") throw new Error("expected non-empty result");
	`)
	if err != nil {
		t.Fatal(err)
	}
	// Verify it matches Go's behavior
	expected, _ := filepath.Abs("")
	v, _ := vm.RunString(`path.abs("").result`)
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- rel ---

func TestRelBasic(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	var script string
	if runtime.GOOS == "windows" {
		script = `
			var r = path.rel("C:\\a\\b", "C:\\a\\b\\c\\d");
			if (r.error !== null) throw new Error("expected no error, got: " + r.error);
			if (r.result !== "c\\d") throw new Error("expected 'c\\d', got: " + r.result);
		`
	} else {
		script = `
			var r = path.rel("/a/b", "/a/b/c/d");
			if (r.error !== null) throw new Error("expected no error, got: " + r.error);
			if (r.result !== "c/d") throw new Error("expected 'c/d', got: " + r.result);
		`
	}
	_, err := vm.RunString(script)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRelSamePath(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	var script string
	if runtime.GOOS == "windows" {
		script = `
			var r = path.rel("C:\\a\\b", "C:\\a\\b");
			if (r.error !== null) throw new Error("expected no error");
			if (r.result !== ".") throw new Error("expected '.', got: " + r.result);
		`
	} else {
		script = `
			var r = path.rel("/a/b", "/a/b");
			if (r.error !== null) throw new Error("expected no error");
			if (r.result !== ".") throw new Error("expected '.', got: " + r.result);
		`
	}
	_, err := vm.RunString(script)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRelParentTraversal(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	var script string
	if runtime.GOOS == "windows" {
		script = `
			var r = path.rel("C:\\a\\b\\c", "C:\\a\\x\\y");
			if (r.error !== null) throw new Error("expected no error, got: " + r.error);
			if (r.result !== "..\\..\\x\\y") throw new Error("expected '..\\..\\x\\y', got: " + r.result);
		`
	} else {
		script = `
			var r = path.rel("/a/b/c", "/a/x/y");
			if (r.error !== null) throw new Error("expected no error, got: " + r.error);
			if (r.result !== "../../x/y") throw new Error("expected '../../x/y', got: " + r.result);
		`
	}
	_, err := vm.RunString(script)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRelMixedAbsRel(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = path.rel("a/b", "/c/d");
		if (typeof r.result !== "string") throw new Error("expected result to be string");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// --- clean ---

func TestCleanBasic(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.clean("a//b/../c/./d")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Clean("a//b/../c/./d")
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestCleanDot(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.clean(".")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "." {
		t.Fatalf("expected '.', got %q", v.String())
	}
}

func TestCleanEmpty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.clean("")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Clean("")
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestCleanTrailingSlash(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.clean("a/b/c/")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Clean("a/b/c/")
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- isAbs ---

func TestIsAbsTrue(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	var script string
	if runtime.GOOS == "windows" {
		script = `path.isAbs("C:\\Windows")`
	} else {
		script = `path.isAbs("/usr/bin")`
	}
	v, err := vm.RunString(script)
	if err != nil {
		t.Fatal(err)
	}
	if !v.ToBoolean() {
		t.Fatal("expected true")
	}
}

func TestIsAbsFalse(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.isAbs("relative/path")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.ToBoolean() {
		t.Fatal("expected false")
	}
}

func TestIsAbsEmpty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.isAbs("")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.ToBoolean() {
		t.Fatal("expected false for empty string")
	}
}

// --- separator and listSeparator ---

func TestSeparator(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.separator`)
	if err != nil {
		t.Fatal(err)
	}
	expected := string(filepath.Separator)
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestListSeparator(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`path.listSeparator`)
	if err != nil {
		t.Fatal(err)
	}
	expected := string(filepath.ListSeparator)
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- match ---

func TestMatchBasic(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = path.match("*.txt", "file.txt");
		if (r.error !== null) throw new Error("expected no error");
		if (r.matched !== true) throw new Error("expected matched=true");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMatchNoMatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = path.match("*.go", "file.txt");
		if (r.error !== null) throw new Error("expected no error");
		if (r.matched !== false) throw new Error("expected matched=false");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMatchWildcard(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = path.match("file?", "file1");
		if (r.error !== null) throw new Error("expected no error");
		if (r.matched !== true) throw new Error("expected matched=true");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMatchBadPattern(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = path.match("[", "anything");
		if (r.error === null) throw new Error("expected error for bad pattern");
		if (typeof r.error !== "string") throw new Error("expected error string");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMatchExact(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = path.match("exact.txt", "exact.txt");
		if (r.error !== null) throw new Error("expected no error");
		if (r.matched !== true) throw new Error("expected matched=true");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMatchCharClass(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = path.match("[abc]", "b");
		if (r.error !== null) throw new Error("expected no error");
		if (r.matched !== true) throw new Error("expected matched=true for char class");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMatchEmpty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = path.match("", "");
		if (r.error !== null) throw new Error("expected no error");
		if (r.matched !== true) throw new Error("expected matched=true for empty/empty");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// --- glob ---

func TestGlobFindsFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	vm := setup(t)
	_ = vm.Set("testDir", dir)
	_, err := vm.RunString(`
		var pattern = path.join(testDir, "*.txt");
		var r = path.glob(pattern);
		if (r.error !== null) throw new Error("expected no error, got: " + r.error);
		if (r.matches === null) throw new Error("expected matches array");
		if (r.matches.length !== 2) throw new Error("expected 2 matches, got " + r.matches.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGlobNoMatches(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	vm := setup(t)
	_ = vm.Set("testDir", dir)
	_, err := vm.RunString(`
		var pattern = path.join(testDir, "*.nonexistent");
		var r = path.glob(pattern);
		if (r.error !== null) throw new Error("expected no error");
		if (r.matches !== null) throw new Error("expected null for no matches, got: " + JSON.stringify(r.matches));
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGlobBadPattern(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = path.glob("[");
		if (r.error === null) throw new Error("expected error for bad pattern");
		if (typeof r.error !== "string") throw new Error("expected error string");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGlobAllFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"x.js", "y.js"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	vm := setup(t)
	_ = vm.Set("testDir", dir)
	_, err := vm.RunString(`
		var pattern = path.join(testDir, "*");
		var r = path.glob(pattern);
		if (r.error !== null) throw new Error("expected no error");
		if (r.matches === null) throw new Error("expected matches");
		if (r.matches.length < 2) throw new Error("expected at least 2 matches, got " + r.matches.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// --- Integration / combined usage ---

func TestJoinThenBase(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var full = path.join("a", "b", "file.txt");
		var b = path.base(full);
		if (b !== "file.txt") throw new Error("expected 'file.txt', got: " + b);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestJoinThenDirThenBase(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var full = path.join("root", "sub", "file.go");
		var d = path.dir(full);
		var b = path.base(full);
		var e = path.ext(full);
		if (b !== "file.go") throw new Error("bad base: " + b);
		if (e !== ".go") throw new Error("bad ext: " + e);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAbsThenRel(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var a = path.abs("foo/bar");
		if (a.error !== null) throw new Error("abs failed: " + a.error);
		var cwd = path.abs(".");
		if (cwd.error !== null) throw new Error("abs cwd failed: " + cwd.error);
		var r = path.rel(cwd.result, a.result);
		if (r.error !== null) throw new Error("rel failed: " + r.error);
		var cleaned = path.clean(r.result);
		var expected = path.clean("foo/bar");
		if (cleaned !== expected) throw new Error("expected " + expected + ", got: " + cleaned);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSeparatorAndListSeparatorTypes(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		if (typeof path.separator !== "string") throw new Error("separator not string");
		if (path.separator.length !== 1) throw new Error("separator wrong length: " + path.separator.length);
		if (typeof path.listSeparator !== "string") throw new Error("listSeparator not string");
		if (path.listSeparator.length !== 1) throw new Error("listSeparator wrong length: " + path.listSeparator.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCleanIdempotent(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var p = "a//b/../c/./d";
		var c1 = path.clean(p);
		var c2 = path.clean(c1);
		if (c1 !== c2) throw new Error("clean is not idempotent: " + c1 + " vs " + c2);
	`)
	if err != nil {
		t.Fatal(err)
	}
}
