package node

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// runScript registers the node modules into a fresh test engine and evaluates
// script, returning the value the script passes to report(). The script has
// five seconds to settle its asynchronous work.
func runScript(t *testing.T, script string) string {
	t.Helper()
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)

	registry := provider.Registry()
	ctx := context.Background()
	registry.RegisterNativeModule("fs", FsRequire(ctx, provider.Adapter()))
	registry.RegisterNativeModule("net", NetRequire(ctx, provider.Adapter(), provider.Loop()))
	registry.RegisterNativeModule("crypto", CryptoRequire(ctx, provider.Adapter()))

	resultCh := make(chan string, 1)
	runErrCh := make(chan error, 1)
	if err := provider.Adapter().Submit(func(rt *goja.Runtime) {
		registry.Enable(rt)
		if err := rt.Set("report", func(v string) { resultCh <- v }); err != nil {
			runErrCh <- err
			return
		}
		_, err := rt.RunString(script)
		runErrCh <- err
	}); err != nil {
		t.Fatalf("submit script: %v", err)
	}

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case got := <-resultCh:
			return got
		case err := <-runErrCh:
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			runErrCh = nil
		case <-timer.C:
			t.Fatal("script did not report within 5s")
			return ""
		}
	}
}

// reportScript wraps a script body in an async runner that reports the first
// result or the first rejection (with its Node error code when present).
func reportScript(body string) string {
	return `
		(async () => {` + body + `
		})().catch(e => report("ERROR: " + (e && e.code ? e.code + " " : "") + (e && e.message ? e.message : String(e))));
	`
}

// TestRequireProcessThrows pins the module surface: only fs, net and crypto
// are registered, so require("process") must throw rather than resolve a
// half-built module. (The process globals the sandbox keeps are separate from
// a require("process") module.)
func TestRequireProcessThrows(t *testing.T) {
	got := runScript(t, reportScript(`
			let outcome = "NO-THROW";
			try {
				const proc = require("process");
				outcome = "RESOLVED:" + (typeof proc);
			} catch (err) {
				outcome = "THREW";
			}
			report(outcome);
	`))
	if got != "THREW" {
		t.Fatalf("require(\"process\") = %q, want THREW", got)
	}
}

func itoaLit(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
