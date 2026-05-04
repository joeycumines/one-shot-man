package scripting

import (
	"testing"
	"time"

	"github.com/dop251/goja"
)

// FuzzGojaRunString ensures the Goja JavaScript VM does not panic on
// arbitrary script input. This tests that malformed, random, or adversarial
// JavaScript source code is handled gracefully (returning an error, not panicking).
func FuzzGojaRunString(f *testing.F) {
	// Seed with valid JS
	f.Add("1 + 1")
	f.Add("var x = 'hello';")
	f.Add("function f() { return 42; } f();")
	f.Add("JSON.parse('{\"a\":1}');")
	f.Add("for (var i = 0; i < 10; i++) {}")
	// Seed with invalid JS
	f.Add("{{{")
	f.Add("function(")
	f.Add("var = ;")
	f.Add("throw new Error('boom');")
	f.Add("undefined.toString()")
	f.Add("null.foo")
	// Edge cases
	f.Add("")
	f.Add("\x00\x01\x02")
	f.Add("'\\u{FFFFFF}'")
	f.Add("//comment\n")
	f.Add("/**/")
	// Stack overflow and infinite loops — these should be interrupted, not crash
	f.Add("try { (function f(){f()})() } catch(e) {}")
	f.Add("for(;;){}")
	f.Add("while(true){}")

	f.Fuzz(func(t *testing.T, script string) {
		vm := goja.New()
		// Set a max call stack depth to prevent Go stack overflow on recursive JS
		vm.SetMaxCallStackSize(64)

		// Set a timeout to interrupt infinite loops
		timer := time.AfterFunc(100*time.Millisecond, func() {
			vm.Interrupt("fuzz timeout")
		})
		defer timer.Stop()

		// Must not panic. Errors (including interrupts) are expected.
		_, _ = vm.RunString(script)
	})
}
