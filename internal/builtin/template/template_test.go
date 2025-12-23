package template

import (
	"context"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func setupModule(t *testing.T) (*goja.Runtime, *goja.Object) {
	t.Helper()

	runtime := goja.New()
	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)

	loader := Require(context.Background())
	loader(runtime, module)

	// Make exports available in the runtime
	_ = runtime.Set("exports", module.Get("exports"))

	return runtime, exports
}

func TestBasicTemplateExecution(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("test");
        tmpl.parse("Hello {{.name}}!");
        const result = tmpl.execute({name: "World"});
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "Hello World!"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestQuickExecute(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        exports.execute("Hello {{.name}}!", {name: "World"});
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "Hello World!"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestTemplateWithMultipleVariables(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("multi");
        tmpl.parse("{{.greeting}} {{.name}}, you are {{.age}} years old!");
        const result = tmpl.execute({
            greeting: "Hello",
            name: "Alice",
            age: 30
        });
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "Hello Alice, you are 30 years old!"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestJavaScriptFunctionInTemplate(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("jsfunc");
        tmpl.funcs({
            upper: function(s) {
                return s.toUpperCase();
            }
        });
        tmpl.parse("Hello {{.name | upper}}!");
        const result = tmpl.execute({name: "World"});
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "Hello WORLD!"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestMultipleJavaScriptFunctions(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("multifunc");
        tmpl.funcs({
            upper: function(s) {
                return s.toUpperCase();
            },
            lower: function(s) {
                return s.toLowerCase();
            },
            repeat: function(n, s) {
                // Note: In Go templates, piped value comes LAST
                // {{.char | repeat 3}} calls repeat(3, .char)
                n = Number(n);
                let result = "";
                for (let i = 0; i < n; i++) {
                    result += s;
                }
                return result;
            }
        });
        tmpl.parse("{{.text | upper}} and {{.text | lower}} and {{.char | repeat 3}}");
        const result = tmpl.execute({text: "Hello", char: "!"});
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "HELLO and hello and !!!"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestTemplateWithArrays(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("array");
        tmpl.parse("{{range .items}}{{.}} {{end}}");
        const result = tmpl.execute({items: ["one", "two", "three"]});
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "one two three "
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestTemplateWithConditionals(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("cond");
        tmpl.parse("{{if .show}}Visible{{else}}Hidden{{end}}");
        const result1 = tmpl.execute({show: true});
        const result2 = tmpl.execute({show: false});
        [result1, result2];
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	obj := val.ToObject(runtime)
	result1 := obj.Get("0").String()
	result2 := obj.Get("1").String()

	if result1 != "Visible" {
		t.Errorf("expected 'Visible', got %q", result1)
	}
	if result2 != "Hidden" {
		t.Errorf("expected 'Hidden', got %q", result2)
	}
}

func TestTemplateWithNestedObjects(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("nested");
        tmpl.parse("{{.user.name}} lives in {{.user.address.city}}");
        const result = tmpl.execute({
            user: {
                name: "Alice",
                address: {
                    city: "Wonderland"
                }
            }
        });
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "Alice lives in Wonderland"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFunctionWithMultipleArguments(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("multiarg");
        tmpl.funcs({
            add: function(a, b) {
                return a + b;
            },
            concat: function(a, b, c) {
                return a + b + c;
            }
        });
        tmpl.parse("{{add .x .y}} and {{concat .a .b .c}}");
        const result = tmpl.execute({x: 5, y: 3, a: "one", b: "two", c: "three"});
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "8 and onetwothree"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFunctionReturningError(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("error");
        tmpl.funcs({
            fail: function() {
                throw new Error("intentional error");
            }
        });
        tmpl.parse("{{fail}}");
        try {
            tmpl.execute({});
            "no error"; // Should not reach here
        } catch (e) {
            "error caught";
        }
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	if result != "error caught" {
		t.Errorf("expected error to be caught, got %q", result)
	}
}

func TestTemplateWithCustomDelimiters(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("delim");
        tmpl.delims("<<", ">>");
        tmpl.parse("Hello <<.name>>!");
        const result = tmpl.execute({name: "World"});
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "Hello World!"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestTemplateChaining(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const result = exports.new("chain")
            .funcs({upper: function(s) { return s.toUpperCase(); }})
            .parse("{{.text | upper}}")
            .execute({text: "hello"});
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "HELLO"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestEmptyTemplate(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        exports.execute("", {});
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestTemplateWithNoData(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        exports.execute("Static text only", null);
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "Static text only"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestParseError(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        try {
            const tmpl = exports.new("bad");
            tmpl.parse("{{.name");  // Missing closing }}
            "no error";
        } catch (e) {
            "error caught";
        }
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	if result != "error caught" {
		t.Errorf("expected parse error to be caught, got %q", result)
	}
}

func TestExecuteError(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        try {
            const tmpl = exports.new("missing");
            // Use a function that doesn't exist to trigger an error
            tmpl.parse("{{undefined_function}}");
            tmpl.execute({});
            "no error";
        } catch (e) {
            "error caught";
        }
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	if result != "error caught" {
		t.Errorf("expected execute error to be caught, got %q", result)
	}
}

func TestMixedJSAndGoValues(t *testing.T) {
	runtime, _ := setupModule(t)

	// Set up a Go function that will be available
	script := `
        const tmpl = exports.new("mixed");

        // JavaScript function
        tmpl.funcs({
            jsfunc: function(s) {
                return "JS:" + s;
            }
        });

        tmpl.parse("{{jsfunc .value}}");
        const result = tmpl.execute({value: "test"});
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "JS:test"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestComplexTemplateScenario(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("complex");

        // Define multiple helper functions
        tmpl.funcs({
            upper: function(s) { return s.toUpperCase(); },
            title: function(s) { return s.charAt(0).toUpperCase() + s.slice(1); },
            join: function(sep, arr) {
                // Note: In Go templates, piped value comes LAST
                // {{.tags | join ", "}} calls join(", ", .tags)
                // Handle both JS arrays and Go arrays exported to JS
                if (Array.isArray(arr)) {
                    return arr.join(sep);
                }
                // For Go arrays, convert to JS array
                const jsArr = [];
                for (let i = 0; i < arr.length; i++) {
                    jsArr.push(arr[i]);
                }
                return jsArr.join(sep);
            },
            default: function(val, def) { return val || def; }
        });

        // Complex template with range, conditionals, and functions
        const template = ` + "`" + `
# {{.title | upper}}

{{if .description}}{{.description}}{{end}}

## Items:
{{range .items}}
- {{. | title}}
{{end}}

Tags: {{.tags | join ", "}}
Status: {{.status | default "pending"}}
` + "`" + `;

        tmpl.parse(template);

        const result = tmpl.execute({
            title: "my report",
            description: "A sample report",
            items: ["first", "second", "third"],
            tags: ["important", "urgent"],
            status: ""
        });
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()

	// Check key parts of the output
	if !strings.Contains(result, "# MY REPORT") {
		t.Errorf("expected title to be uppercase, got: %s", result)
	}
	if !strings.Contains(result, "A sample report") {
		t.Errorf("expected description, got: %s", result)
	}
	if !strings.Contains(result, "- First") && !strings.Contains(result, "- Second") {
		t.Errorf("expected titled items, got: %s", result)
	}
	if !strings.Contains(result, "important, urgent") {
		t.Errorf("expected joined tags, got: %s", result)
	}
	if !strings.Contains(result, "Status: pending") {
		t.Errorf("expected default status, got: %s", result)
	}
}

func TestTemplateNameMethod(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("testname");
        tmpl.name();
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	expected := "testname"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestRealWorldCodeReviewScenario(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("code-review");

        const template = "Ensure correctness of the PR.\\n\\n## IMPLEMENTATIONS/CONTEXT\\n\\n{{.contextTxtar}}";

        tmpl.parse(template);
        const result = tmpl.execute({
            contextTxtar: "Some context here\\nMore context"
        });
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	if !strings.Contains(result, "Ensure correctness of the PR") {
		t.Errorf("expected header, got: %s", result)
	}
	if !strings.Contains(result, "Some context here") {
		t.Errorf("expected context, got: %s", result)
	}
}

func TestRealWorldPromptFlowScenario(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
        const tmpl = exports.new("prompt-flow");

        const template = "!! Generate a prompt for the following goal. !!\\n\\n!! **GOAL:** !!\\n{{.goal}}\\n\\n!! **IMPLEMENTATIONS/CONTEXT:** !!\\n{{.contextTxtar}}";

        tmpl.parse(template);
        const result = tmpl.execute({
            goal: "Implement feature X",
            contextTxtar: "Context data"
        });
        result;
    `

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	result := val.String()
	if !strings.Contains(result, "Implement feature X") {
		t.Errorf("expected goal, got: %s", result)
	}
	if !strings.Contains(result, "Context data") {
		t.Errorf("expected context, got: %s", result)
	}
}

func TestWidthFunction(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
		const cases = [
			{in: "", want: 0, name: "empty string"},
			{in: "abc", want: 3, name: "basic ascii"},
			{in: "你好", want: 4, name: "CJK characters"}, // 2 chars * 2 width
			{in: "e\u0301", want: 1, name: "combining accent"}, // 'e' + combining acute accent = 1 grapheme
			{in: "a\u200bb", want: 2, name: "zero width space"}, // 'a' + ZWSP + 'b', ZWSP is width 0
			{in: "🏳️‍🌈", want: 2, name: "complex emoji"}, // Rainbow flag usually 2 in uniseg
			{in: "😀", want: 2, name: "simple emoji"},
		];

		const errors = [];
		for (const c of cases) {
			const got = exports.width(c.in);
			if (got !== c.want) {
				errors.push("Case '" + c.name + "': expected " + c.want + ", got " + got);
			}
		}

		errors.join("\n");
	`

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	if result := val.String(); result != "" {
		t.Errorf("exports.width failures:\n%s", result)
	}
}

func TestTruncateFunction(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
		const cases = [
			// Basic ASCII cases
			{s: "hello world", max: 5, tail: "...", want: "he...", name: "standard truncate"},
			{s: "hello", max: 5, tail: "...", want: "hello", name: "exact fit"},
			{s: "hello", max: 10, tail: "...", want: "hello", name: "no truncation needed"},
			{s: "hello", max: 4, tail: ".", want: "hel.", name: "custom tail single char"},
			{s: "hello", max: 4, tail: "", want: "hell", name: "empty tail"},

			// Edge cases: Tail wider than maxWidth
			// If tail (3) > max (2), code returns tail "..."
			{s: "abc", max: 2, tail: "...", want: "...", name: "tail wider than max"},

			// Edge cases: Unicode / CJK
			// "你好世界" -> Each char is width 2.
			// Max 3, tail ".". Target = 3-1 = 2.
			// First char "你" (2). Current=2. Matches target.
			// Next char "好" (2). 2+2 > 2. Break.
			// Result "你" + "." = "你."
			{s: "你好世界", max: 3, tail: ".", want: "你.", name: "CJK truncate split"},

			// "你好世界", max 5, tail ".". Target 4.
			// "你"(2) + "好"(2) = 4. Matches target.
			// Result "你好" + "." = "你好." (Total width 5)
			{s: "你好世界", max: 5, tail: ".", want: "你好.", name: "CJK exact grapheme fit"},

			// Complex Graphemes
			// "e\u0301" is width 1.
			{s: "e\u0301abcd", max: 2, tail: ".", want: "e\u0301.", name: "combining char preservation"},

			// Emoji
			// "😀" is width 2.
			// "😀bc", max 2, tail ".". Target 1.
			// "😀" (2) > 1. Break immediately.
			// Result "" + "." = "."
			{s: "😀bc", max: 2, tail: ".", want: ".", name: "emoji exceeds target"},

			// "abc😀", max 4, tail ".". Target 3.
			// "abc" (3) fits. "😀" (2) -> 5 > 3. Break.
			// Result "abc."
			{s: "abc😀", max: 4, tail: ".", want: "abc.", name: "emoji split at boundary"},
		];

		const errors = [];
		for (const c of cases) {
			// truncate(s, maxWidth, tail)
			const got = exports.truncate(c.s, c.max, c.tail);
			if (got !== c.want) {
				errors.push("Case '" + c.name + "': input='" + c.s + "', max=" + c.max + ", tail='" + c.tail + "'. Expected '" + c.want + "', got '" + got + "'");
			}
		}

		// Verify default tail behavior (optional argument)
		const defaultTail = exports.truncate("hello world", 5);
		if (defaultTail !== "he...") {
			errors.push("Default tail: expected 'he...', got '" + defaultTail + "'");
		}

		errors.join("\n");
	`

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	if result := val.String(); result != "" {
		t.Errorf("exports.truncate failures:\n%s", result)
	}
}
