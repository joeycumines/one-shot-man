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
		
		const template = "Ensure correctness of the PR.\\n\\n## IMPLEMENTATIONS/CONTEXT\\n\\n{{.context_txtar}}";
		
		tmpl.parse(template);
		const result = tmpl.execute({
			context_txtar: "Some context here\\nMore context"
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
		
		const template = "!! Generate a prompt for the following goal. !!\\n\\n!! **GOAL:** !!\\n{{.goal}}\\n\\n!! **IMPLEMENTATIONS/CONTEXT:** !!\\n{{.context_txtar}}";
		
		tmpl.parse(template);
		const result = tmpl.execute({
			goal: "Implement feature X",
			context_txtar: "Context data"
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
