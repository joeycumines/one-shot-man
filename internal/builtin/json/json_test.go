package jsonmod

import (
	"strings"
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
	_ = vm.Set("jm", module.Get("exports"))
	return vm
}

// expectError runs JS code and asserts it throws.
func expectError(t *testing.T, vm *goja.Runtime, code string) {
	t.Helper()
	_, err := vm.RunString(code)
	if err == nil {
		t.Fatalf("expected error for: %s", code)
	}
}

// ---------------------------------------------------------------------------
// parse
// ---------------------------------------------------------------------------

func TestParseObject(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.parse('{"a":1,"b":"hello"}');
		if (r.a !== 1) throw new Error("a: " + r.a);
		if (r.b !== "hello") throw new Error("b: " + r.b);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseArray(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.parse('[1,2,3]');
		if (r.length !== 3) throw new Error("len: " + r.length);
		if (r[0] !== 1) throw new Error("r[0]: " + r[0]);
		if (r[2] !== 3) throw new Error("r[2]: " + r[2]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.parse('"hello"');
		if (r !== "hello") throw new Error("got: " + r);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseNumber(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.parse('42');
		if (r !== 42) throw new Error("got: " + r);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseBooleanAndNull(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		if (jm.parse('true') !== true) throw new Error("true");
		if (jm.parse('false') !== false) throw new Error("false");
		if (jm.parse('null') !== null) throw new Error("null");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseNested(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.parse('{"a":{"b":[1,{"c":true}]}}');
		if (r.a.b[0] !== 1) throw new Error("b[0]: " + r.a.b[0]);
		if (r.a.b[1].c !== true) throw new Error("c: " + r.a.b[1].c);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseUndefinedArg(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	expectError(t, vm, `jm.parse()`)
}

func TestParseNullArg(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	expectError(t, vm, `jm.parse(null)`)
}

func TestParseInvalidJSON(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	expectError(t, vm, `jm.parse('{invalid}')`)
}

func TestParseEmptyString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	expectError(t, vm, `jm.parse('')`)
}

// ---------------------------------------------------------------------------
// stringify
// ---------------------------------------------------------------------------

func TestStringifyObject(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`jm.stringify({a: 1, b: "hello"})`)
	if err != nil {
		t.Fatal(err)
	}
	s := v.String()
	if !strings.Contains(s, `"a"`) || !strings.Contains(s, `"b"`) {
		t.Fatalf("unexpected: %s", s)
	}
}

func TestStringifyArray(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`jm.stringify([1,2,3])`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "[1,2,3]" {
		t.Fatalf("got: %s", v.String())
	}
}

func TestStringifyNull(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`jm.stringify(null)`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "null" {
		t.Fatalf("got: %s", v.String())
	}
}

func TestStringifyUndefined(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`jm.stringify(undefined)`)
	if err != nil {
		t.Fatal(err)
	}
	if !goja.IsUndefined(v) {
		t.Fatalf("expected undefined, got: %v", v)
	}
}

func TestStringifyNumberIndent(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`jm.stringify({a: 1}, 2)`)
	if err != nil {
		t.Fatal(err)
	}
	s := v.String()
	if !strings.Contains(s, "  ") {
		t.Fatalf("expected 2-space indent, got: %s", s)
	}
	if !strings.Contains(s, "\n") {
		t.Fatalf("expected newlines, got: %s", s)
	}
}

func TestStringifyStringIndent(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`jm.stringify({a: 1}, "\t")`)
	if err != nil {
		t.Fatal(err)
	}
	s := v.String()
	if !strings.Contains(s, "\t") {
		t.Fatalf("expected tab indent, got: %s", s)
	}
}

func TestStringifyPrimitive(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		if (jm.stringify(42) !== "42") throw new Error("num: " + jm.stringify(42));
		if (jm.stringify("hello") !== '"hello"') throw new Error("str");
		if (jm.stringify(true) !== 'true') throw new Error("bool");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStringifyRoundTrip(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var original = {a: {b: [1, 2, null], c: "hello"}, d: true};
		var s = jm.stringify(original);
		var parsed = jm.parse(s);
		if (parsed.a.b[0] !== 1) throw new Error("b[0]");
		if (parsed.a.b[2] !== null) throw new Error("b[2]");
		if (parsed.a.c !== "hello") throw new Error("c");
		if (parsed.d !== true) throw new Error("d");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// query
// ---------------------------------------------------------------------------

func TestQueryDotNotation(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {a: {b: {c: 42}}};
		var r = jm.query(obj, "a.b.c");
		if (r !== 42) throw new Error("got: " + r);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryArrayIndex(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {items: ["a", "b", "c"]};
		if (jm.query(obj, "items[0]") !== "a") throw new Error("[0]");
		if (jm.query(obj, "items[2]") !== "c") throw new Error("[2]");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryNestedArrayObject(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {users: [{name: "Alice"}, {name: "Bob"}]};
		if (jm.query(obj, "users[0].name") !== "Alice") throw new Error("got: " + jm.query(obj, "users[0].name"));
		if (jm.query(obj, "users[1].name") !== "Bob") throw new Error("Bob");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryWildcard(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {users: [{name: "Alice"}, {name: "Bob"}, {name: "Charlie"}]};
		var names = jm.query(obj, "users[*].name");
		if (names.length !== 3) throw new Error("len: " + names.length);
		if (names[0] !== "Alice") throw new Error("0: " + names[0]);
		if (names[1] !== "Bob") throw new Error("1: " + names[1]);
		if (names[2] !== "Charlie") throw new Error("2: " + names[2]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryMissingPath(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {a: 1};
		var r = jm.query(obj, "b");
		if (r !== undefined) throw new Error("expected undefined, got: " + r);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryMissingDeepPath(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {a: {b: 1}};
		var r = jm.query(obj, "a.c.d");
		if (r !== undefined) throw new Error("expected undefined, got: " + r);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryOutOfBoundsIndex(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {arr: [1]};
		var r = jm.query(obj, "arr[5]");
		if (r !== undefined) throw new Error("expected undefined, got: " + r);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryEmptyPath(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {a: 1};
		var r = jm.query(obj, "");
		if (r.a !== 1) throw new Error("got: " + JSON.stringify(r));
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryNullInput(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.query(null, "a");
		if (r !== undefined) throw new Error("expected undefined");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryUndefinedInput(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.query(undefined, "a");
		if (r !== undefined) throw new Error("expected undefined");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryNullPath(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	expectError(t, vm, `jm.query({a:1}, null)`)
}

func TestQueryWildcardEmpty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {items: []};
		var r = jm.query(obj, "items[*].name");
		if (!Array.isArray(r)) throw new Error("not array");
		if (r.length !== 0) throw new Error("len: " + r.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryWildcardOnNonArray(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {items: "notarray"};
		var r = jm.query(obj, "items[*]");
		if (r !== undefined) throw new Error("expected undefined, got: " + r);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// mergePatch (RFC 7386)
// ---------------------------------------------------------------------------

func TestMergePatchBasic(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.mergePatch({a: 1}, {b: 2});
		if (r.a !== 1) throw new Error("a: " + r.a);
		if (r.b !== 2) throw new Error("b: " + r.b);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergePatchNullDeletesKey(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.mergePatch({a: 1, b: 2}, {a: null});
		if (r.a !== undefined) throw new Error("a should be deleted: " + r.a);
		if (r.b !== 2) throw new Error("b: " + r.b);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergePatchNested(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var target = {a: {b: 1, c: 2}};
		var patch = {a: {b: 99, d: 3}};
		var r = jm.mergePatch(target, patch);
		if (r.a.b !== 99) throw new Error("b: " + r.a.b);
		if (r.a.c !== 2) throw new Error("c: " + r.a.c);
		if (r.a.d !== 3) throw new Error("d: " + r.a.d);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergePatchArrayReplaced(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.mergePatch({a: [1,2,3]}, {a: [4,5]});
		if (r.a.length !== 2) throw new Error("len: " + r.a.length);
		if (r.a[0] !== 4) throw new Error("[0]: " + r.a[0]);
		if (r.a[1] !== 5) throw new Error("[1]: " + r.a[1]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergePatchNullPatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.mergePatch({a: 1}, null);
		if (r !== null) throw new Error("expected null, got: " + JSON.stringify(r));
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergePatchNullTarget(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.mergePatch(null, {a: 1});
		if (r.a !== 1) throw new Error("a: " + r.a);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergePatchEmptyPatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.mergePatch({a: 1, b: 2}, {});
		if (r.a !== 1) throw new Error("a: " + r.a);
		if (r.b !== 2) throw new Error("b: " + r.b);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergePatchStringPatch(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.mergePatch({a: 1}, "hello");
		if (r !== "hello") throw new Error("got: " + r);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergePatchDoesNotMutateInput(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var target = {a: 1, b: {c: 2}};
		var patch = {b: {c: 99, d: 3}};
		var r = jm.mergePatch(target, patch);
		if (target.b.c !== 2) throw new Error("target mutated: " + target.b.c);
		if (target.b.d !== undefined) throw new Error("target.b.d appeared");
		if (r.b.c !== 99) throw new Error("result.b.c: " + r.b.c);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergePatchRFC7386Examples(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var original = {"a":"b","c":{"d":"e","f":"g"}};
		var patch = {"a":"z","c":{"f":null}};
		var r = jm.mergePatch(original, patch);
		if (r.a !== "z") throw new Error("a: " + r.a);
		if (r.c.d !== "e") throw new Error("c.d: " + r.c.d);
		if (r.c.f !== undefined) throw new Error("c.f should be deleted: " + r.c.f);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// diff
// ---------------------------------------------------------------------------

func TestDiffEqualObjects(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff({a: 1, b: 2}, {a: 1, b: 2});
		if (d.length !== 0) throw new Error("expected empty, got: " + JSON.stringify(d));
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffAddedKey(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff({a: 1}, {a: 1, b: 2});
		if (d.length !== 1) throw new Error("len: " + d.length);
		if (d[0].op !== "add") throw new Error("op: " + d[0].op);
		if (d[0].path !== "/b") throw new Error("path: " + d[0].path);
		if (d[0].value !== 2) throw new Error("value: " + d[0].value);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffRemovedKey(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff({a: 1, b: 2}, {a: 1});
		if (d.length !== 1) throw new Error("len: " + d.length);
		if (d[0].op !== "remove") throw new Error("op: " + d[0].op);
		if (d[0].path !== "/b") throw new Error("path: " + d[0].path);
		if (d[0].oldValue !== 2) throw new Error("oldValue: " + d[0].oldValue);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffReplacedValue(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff({a: 1}, {a: 2});
		if (d.length !== 1) throw new Error("len: " + d.length);
		if (d[0].op !== "replace") throw new Error("op: " + d[0].op);
		if (d[0].path !== "/a") throw new Error("path: " + d[0].path);
		if (d[0].value !== 2) throw new Error("value: " + d[0].value);
		if (d[0].oldValue !== 1) throw new Error("oldValue: " + d[0].oldValue);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffNestedChanges(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff({a: {b: 1, c: 2}}, {a: {b: 1, c: 99}});
		if (d.length !== 1) throw new Error("len: " + d.length);
		if (d[0].op !== "replace") throw new Error("op: " + d[0].op);
		if (d[0].path !== "/a/c") throw new Error("path: " + d[0].path);
		if (d[0].value !== 99) throw new Error("value: " + d[0].value);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffArrayChanges(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff([1, 2, 3], [1, 99, 3]);
		if (d.length !== 1) throw new Error("len: " + d.length);
		if (d[0].op !== "replace") throw new Error("op: " + d[0].op);
		if (d[0].path !== "/1") throw new Error("path: " + d[0].path);
		if (d[0].value !== 99) throw new Error("value");
		if (d[0].oldValue !== 2) throw new Error("oldValue");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffArrayAdded(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff([1, 2], [1, 2, 3]);
		if (d.length !== 1) throw new Error("len: " + d.length);
		if (d[0].op !== "add") throw new Error("op: " + d[0].op);
		if (d[0].path !== "/2") throw new Error("path: " + d[0].path);
		if (d[0].value !== 3) throw new Error("value: " + d[0].value);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffArrayRemoved(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff([1, 2, 3], [1]);
		if (d.length !== 2) throw new Error("len: " + d.length);
		if (d[0].op !== "remove") throw new Error("op[0]: " + d[0].op);
		if (d[0].path !== "/1") throw new Error("path[0]: " + d[0].path);
		if (d[1].op !== "remove") throw new Error("op[1]: " + d[1].op);
		if (d[1].path !== "/2") throw new Error("path[1]: " + d[1].path);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffTypeChange(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff({a: "hello"}, {a: 42});
		if (d.length !== 1) throw new Error("len: " + d.length);
		if (d[0].op !== "replace") throw new Error("op: " + d[0].op);
		if (d[0].value !== 42) throw new Error("value: " + d[0].value);
		if (d[0].oldValue !== "hello") throw new Error("oldValue: " + d[0].oldValue);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffObjectToArray(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff({a: {b: 1}}, {a: [1, 2]});
		if (d.length !== 1) throw new Error("len: " + d.length);
		if (d[0].op !== "replace") throw new Error("op: " + d[0].op);
		if (d[0].path !== "/a") throw new Error("path: " + d[0].path);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffEmptyObjects(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff({}, {});
		if (d.length !== 0) throw new Error("len: " + d.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffNullValues(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff(null, null);
		if (d.length !== 0) throw new Error("len: " + d.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffNullToObject(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d = jm.diff(null, {a: 1});
		if (d.length !== 1) throw new Error("len: " + d.length);
		if (d[0].op !== "replace") throw new Error("op: " + d[0].op);
		if (d[0].path !== "") throw new Error("path: " + d[0].path);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffJSONPointerEscaping(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var a = {};
		a["a/b"] = 1;
		a["c~d"] = 2;
		var b = {};
		b["a/b"] = 99;
		b["c~d"] = 2;
		var d = jm.diff(a, b);
		if (d.length !== 1) throw new Error("len: " + d.length);
		if (d[0].path !== "/a~1b") throw new Error("path: " + d[0].path);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDiffPrimitiveEquality(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var d1 = jm.diff("hello", "hello");
		if (d1.length !== 0) throw new Error("strings equal: " + d1.length);
		var d2 = jm.diff(42, 42);
		if (d2.length !== 0) throw new Error("numbers equal: " + d2.length);
		var d3 = jm.diff(true, true);
		if (d3.length !== 0) throw new Error("bools equal: " + d3.length);
		var d4 = jm.diff("a", "b");
		if (d4.length !== 1) throw new Error("strings diff: " + d4.length);
		if (d4[0].op !== "replace") throw new Error("op: " + d4[0].op);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// flatten
// ---------------------------------------------------------------------------

func TestFlattenBasic(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.flatten({a: {b: 1, c: 2}});
		if (r["a.b"] !== 1) throw new Error("a.b: " + r["a.b"]);
		if (r["a.c"] !== 2) throw new Error("a.c: " + r["a.c"]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlattenWithArrays(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.flatten({a: {b: 1, c: [2, 3]}});
		if (r["a.b"] !== 1) throw new Error("a.b: " + r["a.b"]);
		if (r["a.c[0]"] !== 2) throw new Error("a.c[0]: " + r["a.c[0]"]);
		if (r["a.c[1]"] !== 3) throw new Error("a.c[1]: " + r["a.c[1]"]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlattenDeepNested(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.flatten({a: {b: {c: {d: 42}}}});
		if (r["a.b.c.d"] !== 42) throw new Error("got: " + r["a.b.c.d"]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlattenCustomSeparator(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.flatten({a: {b: 1}}, "/");
		if (r["a/b"] !== 1) throw new Error("got: " + r["a/b"]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlattenEmptyObject(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.flatten({});
		var keys = Object.keys(r);
		if (keys.length !== 0) throw new Error("expected empty, got: " + JSON.stringify(r));
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlattenNullValue(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.flatten({a: null, b: 1});
		if (r["a"] !== null) throw new Error("a: " + r["a"]);
		if (r["b"] !== 1) throw new Error("b: " + r["b"]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlattenNullInput(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	expectError(t, vm, `jm.flatten(null)`)
}

func TestFlattenNonObject(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	expectError(t, vm, `jm.flatten("string")`)
}

func TestFlattenTopLevel(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.flatten({a: 1, b: "hello", c: true});
		if (r["a"] !== 1) throw new Error("a");
		if (r["b"] !== "hello") throw new Error("b");
		if (r["c"] !== true) throw new Error("c");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlattenEmptyNestedObject(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.flatten({a: {}});
		if (typeof r["a"] !== "object") throw new Error("type: " + typeof r["a"]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlattenEmptyArray(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.flatten({a: []});
		if (!Array.isArray(r["a"])) throw new Error("not array");
		if (r["a"].length !== 0) throw new Error("len: " + r["a"].length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// unflatten
// ---------------------------------------------------------------------------

func TestUnflattenBasic(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.unflatten({"a.b": 1, "a.c": 2});
		if (r.a.b !== 1) throw new Error("a.b: " + r.a.b);
		if (r.a.c !== 2) throw new Error("a.c: " + r.a.c);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnflattenWithArrays(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.unflatten({"a.b": 1, "a.c[0]": 2, "a.c[1]": 3});
		if (r.a.b !== 1) throw new Error("a.b: " + r.a.b);
		if (!Array.isArray(r.a.c)) throw new Error("not array: " + typeof r.a.c);
		if (r.a.c[0] !== 2) throw new Error("c[0]: " + r.a.c[0]);
		if (r.a.c[1] !== 3) throw new Error("c[1]: " + r.a.c[1]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnflattenCustomSeparator(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.unflatten({"a/b": 1, "a/c": 2}, "/");
		if (r.a.b !== 1) throw new Error("a.b: " + r.a.b);
		if (r.a.c !== 2) throw new Error("a.c: " + r.a.c);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnflattenEmptyObject(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.unflatten({});
		var keys = Object.keys(r);
		if (keys.length !== 0) throw new Error("expected empty, got: " + JSON.stringify(r));
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnflattenNullInput(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	expectError(t, vm, `jm.unflatten(null)`)
}

func TestUnflattenNonObject(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	expectError(t, vm, `jm.unflatten("string")`)
}

func TestUnflattenDeep(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.unflatten({"a.b.c.d": 42});
		if (r.a.b.c.d !== 42) throw new Error("got: " + r.a.b.c.d);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// flatten/unflatten round-trip
// ---------------------------------------------------------------------------

func TestFlattenUnflattenRoundTrip(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var original = {a: {b: 1, c: [2, 3]}, d: "hello"};
		var flat = jm.flatten(original);
		var restored = jm.unflatten(flat);
		if (restored.a.b !== 1) throw new Error("a.b: " + restored.a.b);
		if (restored.a.c[0] !== 2) throw new Error("a.c[0]: " + restored.a.c[0]);
		if (restored.a.c[1] !== 3) throw new Error("a.c[1]: " + restored.a.c[1]);
		if (restored.d !== "hello") throw new Error("d: " + restored.d);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlattenUnflattenRoundTripCustomSep(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var original = {x: {y: {z: 99}}};
		var flat = jm.flatten(original, "/");
		var restored = jm.unflatten(flat, "/");
		if (restored.x.y.z !== 99) throw new Error("got: " + restored.x.y.z);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// integration / cross-function tests
// ---------------------------------------------------------------------------

func TestParseQueryCombo(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var data = jm.parse('{"users":[{"name":"Alice","age":30},{"name":"Bob","age":25}]}');
		var names = jm.query(data, "users[*].name");
		if (names.length !== 2) throw new Error("len: " + names.length);
		if (names[0] !== "Alice") throw new Error("0: " + names[0]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergePatchDiffCombo(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var base = {name: "v1", config: {debug: false, retries: 3}};
		var patch = {config: {debug: true, timeout: 30}};
		var merged = jm.mergePatch(base, patch);
		var d = jm.diff(base, merged);
		var debugOp = null, timeoutOp = null;
		for (var i = 0; i < d.length; i++) {
			if (d[i].path === "/config/debug") debugOp = d[i];
			if (d[i].path === "/config/timeout") timeoutOp = d[i];
		}
		if (!debugOp || debugOp.op !== "replace") throw new Error("debug op");
		if (!timeoutOp || timeoutOp.op !== "add") throw new Error("timeout op");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// internal helper tests
// ---------------------------------------------------------------------------

func TestParsePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected int
	}{
		{"foo.bar.baz", 3},
		{"foo[0].bar", 3},
		{"foo[*].name", 3},
		{"a", 1},
		{"", 0},
		{"a[0][1]", 3},
	}
	for _, tt := range tests {
		segments := parsePath(tt.input)
		if len(segments) != tt.expected {
			t.Errorf("parsePath(%q): got %d segments, want %d", tt.input, len(segments), tt.expected)
		}
	}
}

func TestEscapeJSONPointer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input, expected string
	}{
		{"foo", "foo"},
		{"a/b", "a~1b"},
		{"c~d", "c~0d"},
		{"a/b~c", "a~1b~0c"},
		{"", ""},
	}
	for _, tt := range tests {
		got := escapeJSONPointer(tt.input)
		if got != tt.expected {
			t.Errorf("escapeJSONPointer(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestValuesEqual(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b     any
		expected bool
	}{
		{nil, nil, true},
		{nil, "a", false},
		{"a", nil, false},
		{"a", "a", true},
		{"a", "b", false},
		{int64(1), float64(1), true},
		{int64(1), int64(2), false},
		{true, true, true},
		{true, false, false},
		{float64(3.14), float64(3.14), true},
	}
	for _, tt := range tests {
		got := valuesEqual(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("valuesEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestDeepCopy(t *testing.T) {
	t.Parallel()
	original := map[string]any{
		"a": map[string]any{"b": float64(1)},
		"c": []any{float64(2), float64(3)},
	}
	copied := deepCopy(original)
	original["a"].(map[string]any)["b"] = float64(99)
	original["c"].([]any)[0] = float64(99)

	copiedMap := copied.(map[string]any)
	if copiedMap["a"].(map[string]any)["b"] != float64(1) {
		t.Error("deep copy modified")
	}
	if copiedMap["c"].([]any)[0] != float64(2) {
		t.Error("deep copy array modified")
	}
}

func TestMergePatchAlgorithm(t *testing.T) {
	t.Parallel()
	target := map[string]any{"a": "b", "c": map[string]any{"d": "e", "f": "g"}}
	patch := map[string]any{"a": "z", "c": map[string]any{"f": nil}}

	result := mergePatch(target, patch)
	m := result.(map[string]any)
	if m["a"] != "z" {
		t.Errorf("a: %v", m["a"])
	}
	cm := m["c"].(map[string]any)
	if cm["d"] != "e" {
		t.Errorf("c.d: %v", cm["d"])
	}
	if _, ok := cm["f"]; ok {
		t.Error("c.f should be deleted")
	}
}

func TestParseUnflattenKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key      string
		sep      string
		expected int
	}{
		{"a.b", ".", 2},
		{"a.b[0]", ".", 3},
		{"a.b[0].c", ".", 4},
		{"a/b", "/", 2},
		{"x", ".", 1},
	}
	for _, tt := range tests {
		segs := parseUnflattenKey(tt.key, tt.sep)
		if len(segs) != tt.expected {
			t.Errorf("parseUnflattenKey(%q, %q): got %d segments, want %d", tt.key, tt.sep, len(segs), tt.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// coverage gap tests — resolveIndent edge cases
// ---------------------------------------------------------------------------

func TestStringifyNullIndent(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`jm.stringify({a: 1}, null)`)
	if err != nil {
		t.Fatal(err)
	}
	// null indent → resolveIndent returns "" → MarshalIndent with no prefix (has newlines)
	s := v.String()
	if !strings.Contains(s, "\n") {
		t.Fatalf("expected newlines in MarshalIndent output, got: %s", s)
	}
	if !strings.Contains(s, `"a"`) {
		t.Fatalf("expected key \"a\" in output, got: %s", s)
	}
}

func TestStringifyNegativeIntIndent(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`jm.stringify({a: 1}, -3)`)
	if err != nil {
		t.Fatal(err)
	}
	// negative int → clamped to 0 → MarshalIndent with "" (has newlines, no indent)
	s := v.String()
	if !strings.Contains(s, "\n") {
		t.Fatalf("expected newlines, got: %s", s)
	}
	if strings.Contains(s, "   ") {
		t.Fatalf("expected no indentation for clamped-to-0, got: %s", s)
	}
}

func TestStringifyFloatIndent(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// 2.7 → truncated to int 2
	v, err := vm.RunString(`jm.stringify({a: 1}, 2.7)`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.String(), "  ") {
		t.Fatalf("expected 2-space indent, got: %s", v.String())
	}
	// negative float → clamped to 0 → MarshalIndent with "" (newlines, no indent)
	v2, err := vm.RunString(`jm.stringify({a: 1}, -1.5)`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v2.String(), "\n") {
		t.Fatalf("expected newlines for negative float, got: %s", v2.String())
	}
}

// ---------------------------------------------------------------------------
// coverage gap tests — parsePath / queryValue edge cases
// ---------------------------------------------------------------------------

func TestQueryMalformedBracket(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {};
		obj["[malformed"] = 42;
		var r = jm.query(obj, "[malformed");
		if (r !== 42) throw new Error("got: " + r);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryBracketStringKey(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {items: {}};
		obj.items["special-key"] = 99;
		var r = jm.query(obj, "items[special-key]");
		if (r !== 99) throw new Error("got: " + r);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryNegativeIndex(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var obj = {arr: [1, 2, 3]};
		var r = jm.query(obj, "arr[-1]");
		if (r !== undefined) throw new Error("expected undefined, got: " + r);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// coverage gap tests — unflatten edge cases
// ---------------------------------------------------------------------------

func TestUnflattenMalformedBracket(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.unflatten({"a[bad": 42});
		// parseUnflattenKey splits into segments: {key:"a"}, {key:"[bad"}
		if (r.a["[bad"] !== 42) throw new Error("got: " + JSON.stringify(r));
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnflattenNonNumericBracket(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.unflatten({"a[x]": 42});
		// non-numeric bracket → segments: {key:"a"}, {key:"[x]"}
		if (r.a["[x]"] !== 42) throw new Error("got: " + JSON.stringify(r));
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnflattenEmptySeparator(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var r = jm.unflatten({"a.b": 1}, "");
		// empty separator → whole key is one segment → preserved as-is
		if (r["a.b"] !== 1) throw new Error("got: " + JSON.stringify(r));
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// coverage gap tests — normalizeNumeric type coverage
// ---------------------------------------------------------------------------

func TestNormalizeNumericAllTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input any
		want  float64
	}{
		{int(5), 5},
		{int8(5), 5},
		{int16(5), 5},
		{int32(5), 5},
		{float32(2.5), 2.5},
	}
	for _, tt := range tests {
		got, ok := normalizeNumeric(tt.input).(float64)
		if !ok {
			t.Errorf("normalizeNumeric(%T(%v)) did not return float64", tt.input, tt.input)
			continue
		}
		if got != tt.want {
			t.Errorf("normalizeNumeric(%T(%v)) = %v, want %v", tt.input, tt.input, got, tt.want)
		}
	}
	// Non-numeric passthrough
	s := normalizeNumeric("hello")
	if s != "hello" {
		t.Errorf("normalizeNumeric(string) = %v, want \"hello\"", s)
	}
}
