package template

import (
	"bytes"
	"context"
	"testing"
	"text/template"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

// BenchmarkTemplateExecution benchmarks the osm:text/template module through
// the Goja runtime, measuring the combined cost of JS↔Go bridging and Go
// template execution.
// Expected performance class: ~5-50μs/op (Goja call overhead + Go template).
func BenchmarkTemplateExecution(b *testing.B) {
	b.Run("SimpleExecute", func(b *testing.B) {
		// Benchmark the quick template.execute() helper: parse + execute in one call.
		vm := goja.New()
		registry := require.NewRegistry()
		registry.RegisterNativeModule("osm:text/template", Require(context.Background()))
		registry.Enable(vm)

		_, err := vm.RunString(`var tmpl = require('osm:text/template');`)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := vm.RunString(`tmpl.execute("Hello {{.name}}!", {name: "World"})`)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ParseAndExecute", func(b *testing.B) {
		// Benchmark creating a template, parsing, and executing through JS.
		vm := goja.New()
		registry := require.NewRegistry()
		registry.RegisterNativeModule("osm:text/template", Require(context.Background()))
		registry.Enable(vm)

		_, err := vm.RunString(`var tmpl = require('osm:text/template');`)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := vm.RunString(`
				var t = tmpl.new("bench");
				t.parse("Hello {{.name}}! You have {{.count}} items.");
				t.execute({name: "World", count: 42});
			`)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("PreParsedExecute", func(b *testing.B) {
		// Benchmark executing an already-parsed template (hot path for repeated use).
		vm := goja.New()
		registry := require.NewRegistry()
		registry.RegisterNativeModule("osm:text/template", Require(context.Background()))
		registry.Enable(vm)

		_, err := vm.RunString(`
			var tmpl = require('osm:text/template');
			var t = tmpl.new("bench");
			t.parse("Hello {{.name}}! You have {{.count}} items in {{.category}}.");
		`)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := vm.RunString(`t.execute({name: "World", count: 42, category: "tools"})`)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("WithCustomFunctions", func(b *testing.B) {
		// Benchmark template execution with JS-defined custom functions.
		// This measures the JS→Go→JS round-trip cost for function calls
		// within template pipelines.
		vm := goja.New()
		registry := require.NewRegistry()
		registry.RegisterNativeModule("osm:text/template", Require(context.Background()))
		registry.Enable(vm)

		_, err := vm.RunString(`
			var tmpl = require('osm:text/template');
			var t = tmpl.new("bench");
			t.funcs({
				upper: function(s) { return String(s).toUpperCase(); },
				repeat: function(s, n) { var r = ""; for (var i = 0; i < n; i++) r += s; return r; }
			});
			t.parse("{{.name | upper}} has {{.count}} {{repeat .item .count}}");
		`)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := vm.RunString(`t.execute({name: "World", count: 3, item: "x"})`)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ComplexTemplate", func(b *testing.B) {
		// Benchmark a complex template resembling the prompt template used by
		// goal commands (conditionals, range, pipelines).
		vm := goja.New()
		registry := require.NewRegistry()
		registry.RegisterNativeModule("osm:text/template", Require(context.Background()))
		registry.Enable(vm)

		_, err := vm.RunString(`
			var tmpl = require('osm:text/template');
			var t = tmpl.new("prompt");
			t.funcs({
				upper: function(s) { return String(s).toUpperCase(); }
			});
			t.parse('**{{.description | upper}}**\n\n{{.instructions}}\n\n## CONTEXT\n\n{{.context}}');
		`)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := vm.RunString(`t.execute({
				description: "Code Review Assistant",
				instructions: "Review the following code for bugs, security issues, and style problems.",
				context: "-- file1.go --\npackage main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n-- file2.go --\npackage main\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n"
			})`)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("GoBaselineParseExecute", func(b *testing.B) {
		// Pure Go template baseline for comparison (no Goja overhead).
		tmplText := "Hello {{.Name}}! You have {{.Count}} items in {{.Category}}."
		data := struct {
			Name     string
			Count    int
			Category string
		}{Name: "World", Count: 42, Category: "tools"}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tmpl := template.New("bench")
			if _, err := tmpl.Parse(tmplText); err != nil {
				b.Fatal(err)
			}
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, data); err != nil {
				b.Fatal(err)
			}
		}
	})
}
