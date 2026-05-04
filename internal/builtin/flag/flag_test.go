package flag

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func setup(t *testing.T) *goja.Runtime {
	t.Helper()
	runtime := goja.New()
	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)
	Require(runtime, module)
	_ = runtime.Set("flag", module.Get("exports"))
	return runtime
}

func TestNewFlagSet(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		if (typeof fs !== 'object' || fs === null) throw new Error("expected object");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewFlagSetNoArgs(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet();
		if (typeof fs !== 'object' || fs === null) throw new Error("expected object");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStringFlag(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", "default", "a name flag");
		const result = fs.parse(["--name", "hello"]);
		if (result.error !== null) throw new Error("parse failed: " + result.error);
		const val = fs.get("name");
		if (val !== "hello") throw new Error("expected 'hello', got '" + val + "'");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStringFlagDefault(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", "fallback", "a name flag");
		fs.parse([]);
		if (fs.get("name") !== "fallback") throw new Error("expected default 'fallback'");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntFlag(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.int("count", 0, "number");
		fs.parse(["--count", "42"]);
		if (fs.get("count") !== 42) throw new Error("expected 42");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBoolFlag(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.bool("verbose", false, "verbose mode");
		fs.parse(["--verbose"]);
		if (fs.get("verbose") !== true) throw new Error("expected true");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBoolFlagExplicitFalse(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.bool("verbose", true, "verbose mode");
		fs.parse(["--verbose=false"]);
		if (fs.get("verbose") !== false) throw new Error("expected false");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFloat64Flag(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.float64("rate", 1.0, "rate value");
		fs.parse(["--rate", "3.14"]);
		const val = fs.get("rate");
		if (Math.abs(val - 3.14) > 0.001) throw new Error("expected 3.14, got " + val);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseRemainingArgs(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", "", "");
		fs.parse(["--name", "foo", "arg1", "arg2"]);
		const remaining = fs.args();
		if (remaining.length !== 2) throw new Error("expected 2 remaining, got " + remaining.length);
		if (remaining[0] !== "arg1") throw new Error("expected 'arg1'");
		if (remaining[1] !== "arg2") throw new Error("expected 'arg2'");
		if (fs.nArg() !== 2) throw new Error("expected nArg 2");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseDashDashTerminator(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", "", "");
		fs.parse(["--name", "foo", "--", "--other", "value"]);
		if (fs.get("name") !== "foo") throw new Error("expected 'foo'");
		const remaining = fs.args();
		if (remaining.length !== 2) throw new Error("expected 2 remaining after --");
		if (remaining[0] !== "--other") throw new Error("expected '--other'");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseError(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		const result = fs.parse(["--nonexistent", "value"]);
		if (result.error === null) throw new Error("expected parse error");
		if (typeof result.error !== "string") throw new Error("expected error string");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNFlag(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("a", "", "");
		fs.string("b", "", "");
		fs.string("c", "", "");
		fs.parse(["--a", "1", "--c", "3"]);
		if (fs.nFlag() !== 2) throw new Error("expected nFlag=2, got " + fs.nFlag());
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLookup(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", "default_val", "the usage");
		fs.parse(["--name", "actual"]);
		const info = fs.lookup("name");
		if (info === null) throw new Error("expected info");
		if (info.name !== "name") throw new Error("bad name");
		if (info.usage !== "the usage") throw new Error("bad usage");
		if (info.defValue !== "default_val") throw new Error("bad defValue");
		if (info.value !== "actual") throw new Error("bad value");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLookupUndefined(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		if (fs.lookup("nonexistent") !== null) throw new Error("expected null");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetUndefined(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		if (fs.get("nonexistent") !== undefined) throw new Error("expected undefined");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDefaults(t *testing.T) {
	runtime := setup(t)
	v, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", "default", "a name flag");
		fs.int("count", 5, "a count");
		fs.defaults();
	`)
	if err != nil {
		t.Fatal(err)
	}
	defaults := v.String()
	if !strings.Contains(defaults, "-name") {
		t.Error("defaults should contain -name")
	}
	if !strings.Contains(defaults, "-count") {
		t.Error("defaults should contain -count")
	}
}

func TestVisit(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("a", "", "");
		fs.string("b", "", "");
		fs.string("c", "", "");
		fs.parse(["--a", "1", "--c", "3"]);
		const visited = [];
		fs.visit(function(f) { visited.push(f.name); });
		if (visited.length !== 2) throw new Error("expected 2, got " + visited.length);
		if (visited[0] !== "a") throw new Error("expected 'a' first");
		if (visited[1] !== "c") throw new Error("expected 'c' second");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestVisitAll(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("a", "", "");
		fs.string("b", "", "");
		fs.parse(["--a", "1"]);
		const visited = [];
		fs.visitAll(function(f) { visited.push(f.name); });
		if (visited.length !== 2) throw new Error("expected 2, got " + visited.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestVisitNonFunction(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		try {
			fs.visit("not a function");
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("function")) throw new Error("unexpected: " + e.message);
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestVisitAllNonFunction(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		try {
			fs.visitAll(42);
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("function")) throw new Error("unexpected: " + e.message);
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestChaining(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		const ret = fs.string("a", "", "").int("b", 0, "").bool("c", false, "");
		if (ret !== fs) throw new Error("chaining should return same object");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMultipleFlags(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", "", "name");
		fs.int("count", 0, "count");
		fs.bool("verbose", false, "verbose");
		fs.float64("rate", 1.0, "rate");
		fs.parse(["--name", "hello", "--count", "5", "--verbose", "--rate", "2.5", "extra"]);
		if (fs.get("name") !== "hello") throw new Error("bad name");
		if (fs.get("count") !== 5) throw new Error("bad count");
		if (fs.get("verbose") !== true) throw new Error("bad verbose");
		if (Math.abs(fs.get("rate") - 2.5) > 0.001) throw new Error("bad rate");
		if (fs.nArg() !== 1) throw new Error("bad nArg");
		if (fs.args()[0] !== "extra") throw new Error("bad extra arg");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseInvalidArgs(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		try {
			fs.parse("not an array");
			throw new Error("should have thrown");
		} catch(e) {
			if (!e.message.includes("string array")) throw new Error("unexpected: " + e.message);
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHelpFlag(t *testing.T) {
	runtime := setup(t)
	_, err := runtime.RunString(`
		const fs = flag.newFlagSet("test");
		fs.string("name", "", "a name");
		const result = fs.parse(["-h"]);
		if (result.error === null) throw new Error("expected error for -h");
	`)
	if err != nil {
		t.Fatal(err)
	}
}
