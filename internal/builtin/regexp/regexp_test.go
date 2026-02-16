package regexpmod

import (
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
	_ = vm.Set("re", module.Get("exports"))
	return vm
}

// --- match ---

func TestMatch_Simple(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.match("^hello", "hello world")`)
	if err != nil {
		t.Fatal(err)
	}
	if !v.ToBoolean() {
		t.Fatal("expected true")
	}
}

func TestMatch_NoMatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.match("^world", "hello world")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.ToBoolean() {
		t.Fatal("expected false")
	}
}

func TestMatch_EmptyPattern(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.match("", "anything")`)
	if err != nil {
		t.Fatal(err)
	}
	if !v.ToBoolean() {
		t.Fatal("empty pattern should match anything")
	}
}

func TestMatch_EmptyString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.match("^$", "")`)
	if err != nil {
		t.Fatal(err)
	}
	if !v.ToBoolean() {
		t.Fatal("^$ should match empty string")
	}
}

func TestMatch_InvalidPattern(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`re.match("[invalid", "test")`)
	if err == nil {
		t.Fatal("expected error for invalid pattern")
	}
}

func TestMatch_NullPattern(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`re.match(null, "test")`)
	if err == nil {
		t.Fatal("expected error for null pattern")
	}
}

func TestMatch_UndefinedPattern(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`re.match(undefined, "test")`)
	if err == nil {
		t.Fatal("expected error for undefined pattern")
	}
}

// --- find ---

func TestFind_Simple(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.find("\\d+", "abc 123 def 456")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "123" {
		t.Fatalf("expected '123', got %q", v.String())
	}
}

func TestFind_NoMatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.find("\\d+", "no numbers here")`)
	if err != nil {
		t.Fatal(err)
	}
	if !goja.IsNull(v) {
		t.Fatalf("expected null, got %v", v)
	}
}

func TestFind_EmptyMatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.find("x*", "yyy")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected empty string, got %q", v.String())
	}
}

// --- findAll ---

func TestFindAll_Simple(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var result = re.findAll("\\d+", "abc 123 def 456 ghi 789");
		if (result.length !== 3) throw new Error("expected 3 matches, got " + result.length);
		if (result[0] !== "123") throw new Error("expected '123', got " + result[0]);
		if (result[1] !== "456") throw new Error("expected '456', got " + result[1]);
		if (result[2] !== "789") throw new Error("expected '789', got " + result[2]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFindAll_NoMatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var result = re.findAll("\\d+", "no numbers");
		if (result.length !== 0) throw new Error("expected 0 matches, got " + result.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFindAll_WithLimit(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var result = re.findAll("\\d+", "1 2 3 4 5", 3);
		if (result.length !== 3) throw new Error("expected 3 matches, got " + result.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// --- findSubmatch ---

func TestFindSubmatch_Groups(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var m = re.findSubmatch("(\\w+)@(\\w+)", "user@host.com");
		if (m === null) throw new Error("expected match");
		if (m.length !== 3) throw new Error("expected 3 elements, got " + m.length);
		if (m[0] !== "user@host") throw new Error("full match: " + m[0]);
		if (m[1] !== "user") throw new Error("group 1: " + m[1]);
		if (m[2] !== "host") throw new Error("group 2: " + m[2]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFindSubmatch_NoMatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.findSubmatch("(\\d+)-(\\d+)", "no match here")`)
	if err != nil {
		t.Fatal(err)
	}
	if !goja.IsNull(v) {
		t.Fatalf("expected null, got %v", v)
	}
}

// --- findAllSubmatch ---

func TestFindAllSubmatch_Multiple(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var m = re.findAllSubmatch("(\\w+)=(\\w+)", "a=1 b=2 c=3");
		if (m.length !== 3) throw new Error("expected 3 matches, got " + m.length);
		if (m[0][1] !== "a") throw new Error("m[0][1]: " + m[0][1]);
		if (m[0][2] !== "1") throw new Error("m[0][2]: " + m[0][2]);
		if (m[2][1] !== "c") throw new Error("m[2][1]: " + m[2][1]);
		if (m[2][2] !== "3") throw new Error("m[2][2]: " + m[2][2]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFindAllSubmatch_NoMatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var m = re.findAllSubmatch("(\\d+)-(\\d+)", "no match");
		if (m.length !== 0) throw new Error("expected 0 matches, got " + m.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFindAllSubmatch_WithLimit(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var m = re.findAllSubmatch("(\\w+)=(\\w+)", "a=1 b=2 c=3", 2);
		if (m.length !== 2) throw new Error("expected 2 matches, got " + m.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// --- replace ---

func TestReplace_FirstOnly(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.replace("\\d+", "a1b2c3", "X")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "aXb2c3" {
		t.Fatalf("expected 'aXb2c3', got %q", v.String())
	}
}

func TestReplace_WithBackref(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.replace("(\\w+)@(\\w+)", "user@host rest", "$2@$1")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "host@user rest" {
		t.Fatalf("expected 'host@user rest', got %q", v.String())
	}
}

func TestReplace_NoMatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.replace("\\d+", "no numbers", "X")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "no numbers" {
		t.Fatalf("expected 'no numbers', got %q", v.String())
	}
}

// --- replaceAll ---

func TestReplaceAll_Simple(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.replaceAll("\\d+", "a1b2c3", "X")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "aXbXcX" {
		t.Fatalf("expected 'aXbXcX', got %q", v.String())
	}
}

func TestReplaceAll_WithBackref(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.replaceAll("(\\w+)=(\\w+)", "a=1 b=2", "$1:$2")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "a:1 b:2" {
		t.Fatalf("expected 'a:1 b:2', got %q", v.String())
	}
}

func TestReplaceAll_NoMatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.replaceAll("\\d+", "no numbers", "X")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "no numbers" {
		t.Fatalf("expected 'no numbers', got %q", v.String())
	}
}

// --- split ---

func TestSplit_Simple(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var parts = re.split("[,;]+", "a,b;;c,d");
		if (parts.length !== 4) throw new Error("expected 4 parts, got " + parts.length);
		if (parts[0] !== "a") throw new Error("parts[0]: " + parts[0]);
		if (parts[1] !== "b") throw new Error("parts[1]: " + parts[1]);
		if (parts[2] !== "c") throw new Error("parts[2]: " + parts[2]);
		if (parts[3] !== "d") throw new Error("parts[3]: " + parts[3]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSplit_WithLimit(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var parts = re.split("\\s+", "a b c d e", 3);
		if (parts.length !== 3) throw new Error("expected 3 parts, got " + parts.length);
		if (parts[0] !== "a") throw new Error("parts[0]: " + parts[0]);
		if (parts[1] !== "b") throw new Error("parts[1]: " + parts[1]);
		if (parts[2] !== "c d e") throw new Error("parts[2]: " + parts[2]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSplit_NoMatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var parts = re.split(",", "no commas");
		if (parts.length !== 1) throw new Error("expected 1 part, got " + parts.length);
		if (parts[0] !== "no commas") throw new Error("parts[0]: " + parts[0]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// --- compile ---

func TestCompile_MethodsWork(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = re.compile("\\d+");
		if (r.pattern !== "\\d+") throw new Error("pattern: " + r.pattern);
		if (!r.match("abc123")) throw new Error("match should be true");
		if (r.find("abc 123 def") !== "123") throw new Error("find: " + r.find("abc 123 def"));
		var all = r.findAll("1 2 3");
		if (all.length !== 3) throw new Error("findAll length: " + all.length);
		if (r.replace("a1b2", "X") !== "aXb2") throw new Error("replace: " + r.replace("a1b2", "X"));
		if (r.replaceAll("a1b2", "X") !== "aXbX") throw new Error("replaceAll: " + r.replaceAll("a1b2", "X"));
		var parts = r.split("a1b2c3");
		if (parts.length !== 4) throw new Error("split length: " + parts.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCompile_Submatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = re.compile("(\\w+)=(\\w+)");
		var m = r.findSubmatch("key=value extra");
		if (m[0] !== "key=value") throw new Error("full: " + m[0]);
		if (m[1] !== "key") throw new Error("group 1: " + m[1]);
		if (m[2] !== "value") throw new Error("group 2: " + m[2]);
		var all = r.findAllSubmatch("a=1 b=2");
		if (all.length !== 2) throw new Error("findAllSubmatch length: " + all.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCompile_InvalidPattern(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`re.compile("[invalid")`)
	if err == nil {
		t.Fatal("expected error for invalid pattern")
	}
}

// --- Edge cases ---

func TestUnicodeMatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString("re.match(\"\u65e5\u672c\", \"\u65e5\u672c\u8a9e\u30c6\u30b9\u30c8\")")
	if err != nil {
		t.Fatal(err)
	}
	if !v.ToBoolean() {
		t.Fatal("expected true for Unicode match")
	}
}

func TestUnicodeFind(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString("re.find(\"[\\\\p{Han}]+\", \"hello \u4e16\u754c test\")")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "\u4e16\u754c" {
		t.Fatalf("expected '\u4e16\u754c', got %q", v.String())
	}
}

func TestFindAll_EmptyString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var result = re.findAll("\\d+", "");
		if (result.length !== 0) throw new Error("expected 0 matches, got " + result.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSplit_EmptyString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var parts = re.split(",", "");
		if (parts.length !== 1) throw new Error("expected 1 part, got " + parts.length);
		if (parts[0] !== "") throw new Error("parts[0] should be empty, got " + parts[0]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestReplace_EmptyReplacement(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.replace("\\d+", "abc123def", "")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "abcdef" {
		t.Fatalf("expected 'abcdef', got %q", v.String())
	}
}

func TestReplaceAll_EmptyReplacement(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.replaceAll("\\d", "a1b2c3", "")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "abc" {
		t.Fatalf("expected 'abc', got %q", v.String())
	}
}

func TestMatch_NullString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`re.match("^$", null)`)
	if err != nil {
		t.Fatal(err)
	}
	if !v.ToBoolean() {
		t.Fatal("expected true for ^$ matching null (empty string)")
	}
}
