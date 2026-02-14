package flag

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Edge-case tests for osm:flag module ────────────────────────────────────
//
// Starting coverage: 99.2% (Require 100%, newFlagSetWrapper 99.2%).
// The sole uncovered block is the `default:` case in get() (flag.go:143-144),
// a defensive dead-code path — kind is always "string"/"int"/"bool"/"float64"
// as set by the corresponding definition methods, so the default case is
// unreachable through the JS API.
//
// These tests exercise edge-case inputs (minimal args, undefined/null values,
// multiple parses) to improve test robustness without moving the coverage
// needle.

func setupRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	runtime := goja.New()
	module := runtime.NewObject()
	exports := runtime.NewObject()
	err := module.Set("exports", exports)
	require.NoError(t, err)
	Require(runtime, module)
	err = runtime.Set("flag", module.Get("exports"))
	require.NoError(t, err)
	return runtime
}

// TestStringFlag_MinimalArgs — call string() with name only (no default, no usage).
func TestStringFlag_MinimalArgs(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name");
		fs.parse([]);
		fs.get("name");
	`)
	require.NoError(t, err)
	assert.Equal(t, "", v.Export())
}

// TestIntFlag_MinimalArgs — call int() with name only.
func TestIntFlag_MinimalArgs(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.int("count");
		fs.parse([]);
		fs.get("count");
	`)
	require.NoError(t, err)
	assert.Equal(t, int64(0), v.Export())
}

// TestBoolFlag_MinimalArgs — call bool() with name only.
func TestBoolFlag_MinimalArgs(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.bool("verbose");
		fs.parse([]);
		fs.get("verbose");
	`)
	require.NoError(t, err)
	assert.Equal(t, false, v.Export())
}

// TestFloat64Flag_MinimalArgs — call float64() with name only.
func TestFloat64Flag_MinimalArgs(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.float64("rate");
		fs.parse([]);
		fs.get("rate");
	`)
	require.NoError(t, err)
	assert.InDelta(t, 0.0, v.ToFloat(), 0.001)
}

// TestStringFlag_UndefinedDefault — pass undefined as default value.
func TestStringFlag_UndefinedDefault(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", undefined, undefined);
		fs.parse([]);
		fs.get("name");
	`)
	require.NoError(t, err)
	assert.Equal(t, "", v.Export())
}

// TestIntFlag_UndefinedDefault — pass undefined as default value.
func TestIntFlag_UndefinedDefault(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.int("count", undefined, undefined);
		fs.parse([]);
		fs.get("count");
	`)
	require.NoError(t, err)
	assert.Equal(t, int64(0), v.Export())
}

// TestBoolFlag_UndefinedDefault — pass undefined as default value.
func TestBoolFlag_UndefinedDefault(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.bool("verbose", undefined, undefined);
		fs.parse([]);
		fs.get("verbose");
	`)
	require.NoError(t, err)
	assert.Equal(t, false, v.Export())
}

// TestFloat64Flag_UndefinedDefault — pass undefined as default value.
func TestFloat64Flag_UndefinedDefault(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.float64("rate", undefined, undefined);
		fs.parse([]);
		fs.get("rate");
	`)
	require.NoError(t, err)
	assert.InDelta(t, 0.0, v.ToFloat(), 0.001)
}

// TestNewFlagSet_NullArg — null is coerced to empty string, should not panic.
func TestNewFlagSet_NullArg(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet(null);
		if (typeof fs !== "object" || fs === null) throw new Error("expected object");
	`)
	require.NoError(t, err)
}

// TestVisit_Properties — verify each flag property in visitor callback.
func TestVisit_Properties(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", "def_val", "the usage text");
		fs.parse(["--name", "actual_val"]);
		let found = false;
		fs.visit(function(f) {
			if (f.name !== "name") throw new Error("bad name: " + f.name);
			if (f.usage !== "the usage text") throw new Error("bad usage: " + f.usage);
			if (f.defValue !== "def_val") throw new Error("bad defValue: " + f.defValue);
			if (f.value !== "actual_val") throw new Error("bad value: " + f.value);
			found = true;
		});
		if (!found) throw new Error("visit callback was not called");
	`)
	require.NoError(t, err)
}

// TestVisitAll_Ordering — verify visitAll iterates in definition order.
func TestVisitAll_Ordering(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.int("alpha", 10, "");
		fs.bool("beta", false, "");
		fs.string("gamma", "g", "");
		fs.parse([]);
		const names = [];
		fs.visitAll(function(f) { names.push(f.name); });
		// Go flag package iterates in lexicographic order
		const expected = ["alpha", "beta", "gamma"];
		if (JSON.stringify(names) !== JSON.stringify(expected)) {
			throw new Error("expected " + JSON.stringify(expected) + ", got " + JSON.stringify(names));
		}
	`)
	require.NoError(t, err)
}

// TestVisit_NoSetFlags — visit with no flags set should not call callback.
func TestVisit_NoSetFlags(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", "", "");
		fs.parse([]);
		let called = false;
		fs.visit(function(f) { called = true; });
		if (called) throw new Error("visit should not call callback when no flags are set");
	`)
	require.NoError(t, err)
}

// TestLookup_AllTypes — lookup returns correct defValue for all flag types.
func TestLookup_AllTypes(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("s", "hello", "");
		fs.int("i", 42, "");
		fs.bool("b", true, "");
		fs.float64("f", 3.14, "");
		fs.parse([]);

		let info;

		info = fs.lookup("s");
		if (info.defValue !== "hello") throw new Error("string defValue: " + info.defValue);

		info = fs.lookup("i");
		if (info.defValue !== "42") throw new Error("int defValue: " + info.defValue);

		info = fs.lookup("b");
		if (info.defValue !== "true") throw new Error("bool defValue: " + info.defValue);

		info = fs.lookup("f");
		if (info.defValue !== "3.14") throw new Error("float64 defValue: " + info.defValue);
	`)
	require.NoError(t, err)
}

// TestDefaults_Empty — defaults with no flags defined should return empty string.
func TestDefaults_Empty(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.defaults();
	`)
	require.NoError(t, err)
	assert.Equal(t, "", v.Export())
}

// TestNArg_BeforeParse — nArg before parse should return 0.
func TestNArg_BeforeParse(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.nArg();
	`)
	require.NoError(t, err)
	assert.Equal(t, int64(0), v.Export())
}

// TestNFlag_BeforeParse — nFlag before parse should return 0.
func TestNFlag_BeforeParse(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.nFlag();
	`)
	require.NoError(t, err)
	assert.Equal(t, int64(0), v.Export())
}

// TestArgs_BeforeParse — args before parse should return empty array.
func TestArgs_BeforeParse(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		const a = fs.args();
		if (!Array.isArray(a) || a.length !== 0) throw new Error("expected empty array, got " + JSON.stringify(a));
	`)
	require.NoError(t, err)
}

// TestParse_EmptyArray — parse with empty array should succeed.
func TestParse_EmptyArray(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", "def", "");
		const result = fs.parse([]);
		if (result.error !== null) throw new Error("expected no error");
	`)
	require.NoError(t, err)
}

// TestFloat64Flag_Parsed — verify float64 after parse with explicit value.
func TestFloat64Flag_Parsed(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.float64("rate", 1.5, "a rate");
		fs.parse(["--rate", "99.9"]);
		fs.get("rate");
	`)
	require.NoError(t, err)
	f, ok := v.Export().(float64)
	require.True(t, ok)
	assert.InDelta(t, 99.9, f, 0.001)
}

// TestMultipleFlagSets — independent flag sets don't share state.
func TestMultipleFlagSets(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs1 = flag.newFlagSet("set1");
		const fs2 = flag.newFlagSet("set2");
		fs1.string("name", "one", "");
		fs2.string("name", "two", "");
		fs1.parse(["--name", "override1"]);
		fs2.parse([]);
		if (fs1.get("name") !== "override1") throw new Error("fs1 wrong: " + fs1.get("name"));
		if (fs2.get("name") !== "two") throw new Error("fs2 wrong: " + fs2.get("name"));
	`)
	require.NoError(t, err)
}

// TestVisitAll_WithSetAndUnsetFlags — visitAll includes both set and unset flags.
func TestVisitAll_WithSetAndUnsetFlags(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("set_flag", "", "");
		fs.string("unset_flag", "default", "");
		fs.parse(["--set_flag", "value"]);
		const allNames = [];
		fs.visitAll(function(f) { allNames.push(f.name); });
		if (allNames.length !== 2) throw new Error("expected 2, got " + allNames.length);
		const setNames = [];
		fs.visit(function(f) { setNames.push(f.name); });
		if (setNames.length !== 1) throw new Error("expected 1 set, got " + setNames.length);
		if (setNames[0] !== "set_flag") throw new Error("expected set_flag");
	`)
	require.NoError(t, err)
}

// TestParseError_BadIntValue — parse error for invalid int value.
func TestParseError_BadIntValue(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.int("count", 0, "");
		const result = fs.parse(["--count", "not_a_number"]);
		if (result.error === null) throw new Error("expected parse error for bad int");
		if (typeof result.error !== "string") throw new Error("error should be string");
	`)
	require.NoError(t, err)
}

// TestParseError_BadBoolValue — parse error for invalid bool value.
func TestParseError_BadBoolValue(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.bool("verbose", false, "");
		const result = fs.parse(["--verbose=notbool"]);
		if (result.error === null) throw new Error("expected parse error for bad bool");
	`)
	require.NoError(t, err)
}

// TestParseError_BadFloat64Value — parse error for invalid float64 value.
func TestParseError_BadFloat64Value(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.float64("rate", 0, "");
		const result = fs.parse(["--rate", "not_a_float"]);
		if (result.error === null) throw new Error("expected parse error for bad float");
	`)
	require.NoError(t, err)
}

// TestGet_AfterMultipleParses — second parse should override first.
func TestGet_AfterMultipleParses(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", "", "");
		fs.parse(["--name", "first"]);
		fs.parse(["--name", "second"]);
		fs.get("name");
	`)
	require.NoError(t, err)
	assert.Equal(t, "second", v.Export())
}

// TestLookup_AfterParse_ShowsCurrentValue — lookup.value reflects parsed value.
func TestLookup_AfterParse_ShowsCurrentValue(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.int("count", 10, "a counter");
		fs.parse(["--count", "99"]);
		const info = fs.lookup("count");
		if (info.value !== "99") throw new Error("expected '99', got " + info.value);
		if (info.defValue !== "10") throw new Error("expected '10', got " + info.defValue);
	`)
	require.NoError(t, err)
}

// TestDefaults_MultipleTypes — defaults prints usage for all registered types.
func TestDefaults_MultipleTypes(t *testing.T) {
	rt := setupRuntime(t)
	v, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("s", "sdef", "string usage");
		fs.int("i", 42, "int usage");
		fs.bool("b", false, "bool usage");
		fs.float64("f", 1.5, "float usage");
		fs.defaults();
	`)
	require.NoError(t, err)
	s := v.String()
	assert.Contains(t, s, "-s")
	assert.Contains(t, s, "string usage")
	assert.Contains(t, s, "-i")
	assert.Contains(t, s, "int usage")
	assert.Contains(t, s, "-b")
	assert.Contains(t, s, "bool usage")
	assert.Contains(t, s, "-f")
	assert.Contains(t, s, "float usage")
}

// TestDashDash_AllRemainingAreArgs — everything after -- becomes args.
func TestDashDash_AllRemainingAreArgs(t *testing.T) {
	rt := setupRuntime(t)
	_, err := rt.RunString(`
		const fs = flag.newFlagSet("test");
		fs.bool("v", false, "");
		fs.parse(["--v", "--", "--v", "foo", "bar"]);
		if (fs.get("v") !== true) throw new Error("v should be true");
		const args = fs.args();
		if (args.length !== 3) throw new Error("expected 3 args after --, got " + args.length);
		if (args[0] !== "--v") throw new Error("first arg should be '--v'");
		if (args[1] !== "foo") throw new Error("second arg should be 'foo'");
		if (args[2] !== "bar") throw new Error("third arg should be 'bar'");
	`)
	require.NoError(t, err)
}
