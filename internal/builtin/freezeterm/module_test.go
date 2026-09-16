package freezeterm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// moduleEnv builds a runtime with a running event loop, loads osm:freezeterm
// with the supplied base context, and returns a runJS helper that executes a
// script on the loop and waits for __collect or __collectErr.
func moduleEnv(t *testing.T, baseCtx context.Context) (*goja.Runtime, func(string) (goja.Value, error)) {
	t.Helper()
	runtime := goja.New()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)
	Require(baseCtx, adapter)(runtime, module)
	_ = runtime.Set("freezeterm", exports)

	resultCh := make(chan goja.Value, 1)
	errCh := make(chan error, 1)
	_ = runtime.Set("__collect", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0)
		return goja.Undefined()
	})
	_ = runtime.Set("__collectErr", func(call goja.FunctionCall) goja.Value {
		errCh <- fmt.Errorf("%s", call.Argument(0).String())
		return goja.Undefined()
	})

	loopCtx, loopCancel := context.WithCancel(context.Background())
	go loop.Run(loopCtx)
	t.Cleanup(func() {
		loopCancel()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = loop.Shutdown(shutdownCtx)
	})

	runJS := func(script string) (goja.Value, error) {
		t.Helper()
		submitErr := loop.Submit(func() {
			wrapped := "(async function() {\n" + script + "\n})();"
			_, runErr := runtime.RunString(wrapped)
			if runErr != nil {
				errCh <- runErr
			}
		})
		if submitErr != nil {
			return goja.Undefined(), submitErr
		}
		select {
		case val := <-resultCh:
			return val, nil
		case err := <-errCh:
			return goja.Undefined(), err
		case <-time.After(10 * time.Second):
			return goja.Undefined(), fmt.Errorf("timeout waiting for async result")
		}
	}
	return runtime, runJS
}

// stubExecutable builds the shared freeze stub from the freezeterm package's
// testdata.
func stubExecutable(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("slow: builds and runs a stub executable")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "freezestub")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, filepath.Join("..", "..", "freezeterm", "testdata", "stub"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stub: %v: %s", err, out)
	}
	return exe
}

func TestRequireNilAdapter(t *testing.T) {
	t.Parallel()
	runtime := goja.New()
	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)
	Require(context.Background(), nil)(runtime, module)
	_ = runtime.Set("freezeterm", exports)

	_, err := runtime.RunString(`freezeterm.info()`)
	if err == nil {
		t.Fatal("expected nil adapter call to fail")
	}
	if !strings.Contains(err.Error(), "event loop adapter is required") {
		t.Fatalf("nil adapter error = %v", err)
	}
}

func TestRenderTextStub(t *testing.T) {
	stub := stubExecutable(t)
	_, runJS := moduleEnv(t, context.Background())
	v, err := runJS(`
		try {
			var text = await freezeterm.renderText('rendered text', {executable: ` + jsString(stub) + `});
			__collect(text.indexOf('<svg') >= 0);
		} catch (e) {
			__collectErr('renderText rejected: ' + String(e.message || e));
		}
	`)
	if err != nil {
		t.Fatalf("renderText: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("renderText did not return SVG text")
	}
}

func TestRenderStubPNGPathOnly(t *testing.T) {
	stub := stubExecutable(t)
	_, runJS := moduleEnv(t, context.Background())
	v, err := runJS(`
		try {
			var r = await freezeterm.render('png body', {executable: ` + jsString(stub) + `, format: 'png'});
			__collect(JSON.stringify({format: r.format, path: r.path, text: typeof r.text}));
		} catch (e) {
			__collectErr('render rejected: ' + String(e.message || e));
		}
	`)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var result struct {
		Format string `json:"format"`
		Path   string `json:"path"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal([]byte(v.String()), &result); err != nil {
		t.Fatalf("parse result %q: %v", v.String(), err)
	}
	if result.Format != "png" || !strings.HasSuffix(result.Path, ".png") {
		t.Fatalf("result = %+v, want png path", result)
	}
	if result.Text != "undefined" {
		t.Fatalf("PNG results must not expose text, got %q", result.Text)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if !strings.HasPrefix(string(data), "\x89PNG\r\n\x1a\n") {
		t.Fatalf("artifact is not PNG: %q", data)
	}
}

func TestInfoUnavailable(t *testing.T) {
	t.Parallel()
	_, runJS := moduleEnv(t, context.Background())
	missing := filepath.Join(t.TempDir(), "definitely-not-freeze")
	v, err := runJS(`
		try {
			var info = await freezeterm.info({executable: ` + jsString(missing) + `});
			__collect(JSON.stringify(info));
		} catch (e) {
			__collectErr('info rejected: ' + String(e.message || e));
		}
	`)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	var info struct {
		Available bool   `json:"available"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal([]byte(v.String()), &info); err != nil {
		t.Fatalf("parse info %q: %v", v.String(), err)
	}
	if info.Available {
		t.Fatalf("info = %+v, want unavailable", info)
	}
	if !strings.Contains(info.Error, "unavailable") {
		t.Fatalf("info error = %q, want unavailable diagnostic", info.Error)
	}
}

func TestRenderOptionValidation(t *testing.T) {
	t.Parallel()
	_, runJS := moduleEnv(t, context.Background())
	v, err := runJS(`
		var threw = 0;
		try { freezeterm.render(42); } catch (e) { threw++; }
		try { freezeterm.render('x', {bogus: 1}); } catch (e) { threw++; }
		try { freezeterm.render('x', 42); } catch (e) { threw++; }
		try { freezeterm.render('x', {width: 'wide'}); } catch (e) { threw++; }
		try { freezeterm.render('x', {window: 'yes'}); } catch (e) { threw++; }
		try { freezeterm.render('x', {format: 'webp'}); } catch (e) { threw++; }
		try { freezeterm.render('x', {lines: [1, 2, 3]}); } catch (e) { threw++; }
		try { freezeterm.render('x', {margin: [1, 2, 3]}); } catch (e) { threw++; }
		try { freezeterm.render('x', {font: {bogus: 1}}); } catch (e) { threw++; }
		__collect(threw);
	`)
	if err != nil {
		t.Fatalf("runJS: %v", err)
	}
	if got := v.ToInteger(); got != 9 {
		t.Fatalf("validation threw %d times, want 9", got)
	}
}

func TestRenderRejectsOnExitFailure(t *testing.T) {
	stub := stubExecutable(t)
	record := filepath.Join(t.TempDir(), "record.ndjson")
	t.Setenv("FREEZETERM_STUB_RECORD", record)
	t.Setenv("FREEZETERM_STUB_EXIT", "7")
	t.Setenv("FREEZETERM_STUB_STDERR", "stub failure diagnostic")
	_, runJS := moduleEnv(t, context.Background())
	v, err := runJS(`
		try {
			await freezeterm.renderText('boom', {executable: ` + jsString(stub) + `});
			__collect('resolved');
		} catch (e) {
			__collect(String(e.message || e));
		}
	`)
	if err != nil {
		t.Fatalf("runJS: %v", err)
	}
	got := v.String()
	if !strings.Contains(got, "exited with an error") || !strings.Contains(got, "stub failure diagnostic") {
		t.Fatalf("rejection = %q, want exit failure with diagnostics", got)
	}
}

func TestRenderCancellationRejects(t *testing.T) {
	stub := stubExecutable(t)
	t.Setenv("FREEZETERM_STUB_SLEEP_MS", "3000")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, runJS := moduleEnv(t, ctx)

	done := make(chan struct {
		v   goja.Value
		err error
	}, 1)
	go func() {
		v, err := runJS(`
			try {
				await freezeterm.renderText('slow', {executable: ` + jsString(stub) + `});
				__collect('resolved instead of cancelling');
			} catch (e) {
				__collect(String(e.message || e));
			}
		`)
		done <- struct {
			v   goja.Value
			err error
		}{v, err}
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("runJS: %v", got.err)
		}
		if message := got.v.String(); !strings.Contains(message, "context canceled") {
			t.Fatalf("promise settled with %q, want cancellation rejection", message)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancellation did not settle the promise")
	}
}

// jsString renders s as a JavaScript string literal.
func jsString(s string) string {
	data, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(data)
}

func TestRenderOptionValidation_RejectsStringArray(t *testing.T) {
	t.Parallel()
	_, runJS := moduleEnv(t, context.Background())
	v, err := runJS(`
		var threw = 0;
		try { freezeterm.render('x', {args: '--theme=x'}); } catch (e) { threw++; }
		try { freezeterm.render('x', {margin: 'wide'}); } catch (e) { threw++; }
		__collect(threw);
	`)
	if err != nil {
		t.Fatalf("runJS: %v", err)
	}
	if got := v.ToInteger(); got != 2 {
		t.Fatalf("validation threw %d times, want 2", got)
	}
}
