package fetch_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dop251/goja"
	fetchmod "github.com/joeycumines/one-shot-man/internal/builtin/fetch"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// runOnLoop submits fn to the event loop and waits for it to complete.
func runOnLoop(t *testing.T, provider *testutil.TestEventLoopProvider, fn func()) {
	t.Helper()
	done := make(chan struct{})
	err := provider.Loop().Submit(func() {
		defer close(done)
		fn()
	})
	if err != nil {
		t.Fatalf("failed to submit to event loop: %v", err)
	}
	<-done
}

// loadModule creates the fetch module exports on the event loop and sets fetchMod global.
func loadModule(t *testing.T, provider *testutil.TestEventLoopProvider) {
	t.Helper()
	runOnLoop(t, provider, func() {
		vm := provider.Runtime()
		adapter := provider.Adapter()
		loader := fetchmod.Require(adapter)
		module := vm.NewObject()
		exports := vm.NewObject()
		_ = module.Set("exports", exports)
		loader(vm, module)
		_ = vm.Set("fetchMod", exports)
	})
}

// runAsync wraps JS code in an async IIFE, executes it on the event loop,
// and waits for the Promise to settle. Fails the test on rejection or timeout.
func runAsync(t *testing.T, provider *testutil.TestEventLoopProvider, js string) {
	t.Helper()
	done := make(chan error, 1)
	err := provider.Loop().Submit(func() {
		vm := provider.Runtime()
		_ = vm.Set("__asyncDone", func() {
			done <- nil
		})
		_ = vm.Set("__asyncFail", func(msg string) {
			done <- fmt.Errorf("%s", msg)
		})
		wrapped := `(async function() { ` + js + ` })()
			.then(function() { __asyncDone(); })
			.catch(function(e) { __asyncFail(e.message || String(e)); });`
		if _, runErr := vm.RunString(wrapped); runErr != nil {
			done <- runErr
		}
	})
	if err != nil {
		t.Fatalf("failed to submit to event loop: %v", err)
	}
	select {
	case result := <-done:
		if result != nil {
			t.Fatal(result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("async test timed out")
	}
}

func TestBasicGet(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		if (resp.status !== 200) throw new Error("expected 200, got " + resp.status);
		if (resp.ok !== true) throw new Error("expected ok=true");
		var body = await resp.text();
		if (body !== "hello world") throw new Error("unexpected body: " + body);
	`)
}

func TestPostWithBodyAndHeaders(t *testing.T) {
	t.Parallel()
	var receivedBody string
	var receivedContentType string
	var receivedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: '{"key":"value"}'
		});
		if (resp.status !== 200) throw new Error("expected 200");
		var body = await resp.text();
		if (body !== "ok") throw new Error("unexpected body");
	`)

	if receivedMethod != "POST" {
		t.Errorf("expected POST method, got %s", receivedMethod)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected application/json, got %s", receivedContentType)
	}
	if receivedBody != `{"key":"value"}` {
		t.Errorf("unexpected body: %q", receivedBody)
	}
}

func TestJsonResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"name":    "test",
			"count":   42.0,
			"active":  true,
			"tags":    []string{"a", "b"},
			"nested":  map[string]interface{}{"key": "val"},
			"nothing": nil,
		})
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		var data = await resp.json();
		if (data.name !== "test") throw new Error("bad name: " + data.name);
		if (data.count !== 42) throw new Error("bad count: " + data.count);
		if (data.active !== true) throw new Error("bad active");
		if (!Array.isArray(data.tags)) throw new Error("tags not array");
		if (data.tags.length !== 2) throw new Error("bad tags length");
		if (data.tags[0] !== "a") throw new Error("bad tags[0]");
		if (data.nested.key !== "val") throw new Error("bad nested.key");
		if (data.nothing !== null) throw new Error("bad nothing: " + data.nothing);
	`)
}

func TestBadURL(t *testing.T) {
	t.Parallel()
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	// http.NewRequest rejects URLs like "://invalid-url" synchronously,
	// which causes a panic caught by goja as a GoError.
	runOnLoop(t, provider, func() {
		vm := provider.Runtime()
		_, err := vm.RunString(`
			try {
				fetchMod.fetch("://invalid-url");
				throw new Error("expected error");
			} catch(e) {
				if (e.message === "expected error") throw e;
			}
		`)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestCustomTimeout(t *testing.T) {
	t.Parallel()
	blocker := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocker
	}))
	defer server.Close()
	defer close(blocker)

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		try {
			await fetchMod.fetch(url, { timeout: 0.1 });
			throw new Error("expected timeout");
		} catch(e) {
			if (e.message === "expected timeout") throw e;
		}
	`)
}

func TestStatusCode404(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		if (resp.status !== 404) throw new Error("expected 404, got " + resp.status);
		if (resp.ok !== false) throw new Error("expected ok=false for 404");
	`)
}

func TestStatusCode500(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		if (resp.status !== 500) throw new Error("expected 500, got " + resp.status);
		if (resp.ok !== false) throw new Error("expected ok=false for 500");
		var body = await resp.text();
		if (body !== "internal server error") throw new Error("unexpected body");
	`)
}

func TestResponseHeaders(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.Header().Set("X-Another", "another-value")
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		if (resp.headers.get("x-custom-header") !== "custom-value") {
			throw new Error("bad x-custom-header: " + resp.headers.get("x-custom-header"));
		}
		if (resp.headers.get("x-another") !== "another-value") {
			throw new Error("bad x-another: " + resp.headers.get("x-another"));
		}
		if (resp.headers.has("x-custom-header") !== true) {
			throw new Error("has should return true for existing header");
		}
		if (resp.headers.has("x-missing") !== false) {
			throw new Error("has should return false for missing header");
		}
		if (resp.headers.get("x-missing") !== null) {
			throw new Error("get should return null for missing header");
		}
	`)
}

func TestHeadersForEach(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "value1")
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		var found = false;
		resp.headers.forEach(function(value, name) {
			if (name === "x-test" && value === "value1") found = true;
		});
		if (!found) throw new Error("forEach did not iterate over x-test header");
	`)
}

func TestHeadersEntriesKeysValues(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Only", "single")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		var entries = resp.headers.entries();
		if (!Array.isArray(entries)) throw new Error("entries should return array");
		var keys = resp.headers.keys();
		if (!Array.isArray(keys)) throw new Error("keys should return array");
		var values = resp.headers.values();
		if (!Array.isArray(values)) throw new Error("values should return array");

		// Verify x-only is in keys
		var foundKey = false;
		for (var i = 0; i < keys.length; i++) {
			if (keys[i] === "x-only") foundKey = true;
		}
		if (!foundKey) throw new Error("x-only not found in keys");
	`)
}

func TestRedirectUrl(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("final"))
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		vm := provider.Runtime()
		_ = vm.Set("url", server.URL+"/redirect")
		_ = vm.Set("expectedUrl", server.URL+"/final")
	})

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		if (resp.url !== expectedUrl) {
			throw new Error("expected url " + expectedUrl + ", got " + resp.url);
		}
		var body = await resp.text();
		if (body !== "final") throw new Error("unexpected body after redirect");
	`)
}

func TestDefaultMethod(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		await fetchMod.fetch(url);
	`)

	if receivedMethod != "GET" {
		t.Errorf("expected default GET, got %s", receivedMethod)
	}
}

func TestStatusText(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		if (resp.status !== 201) throw new Error("expected 201");
		if (resp.statusText.indexOf("201") === -1) {
			throw new Error("statusText should contain 201: " + resp.statusText);
		}
	`)
}

func TestJsonParseError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		var body = await resp.text();
		if (body !== "not json") throw new Error("unexpected text");
		try {
			await resp.json();
			throw new Error("expected json parse error");
		} catch(e) {
			if (e.message === "expected json parse error") throw e;
		}
	`)
}

func TestMethodCaseInsensitive(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	runAsync(t, provider, `
		await fetchMod.fetch(url, { method: 'put' });
	`)

	if receivedMethod != "PUT" {
		t.Errorf("expected PUT (uppercased), got %s", receivedMethod)
	}
}

func TestConnectionRefused(t *testing.T) {
	t.Parallel()
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", url)
	})

	// Connection refused is an async error caught by the Promise rejection.
	runAsync(t, provider, `
		try {
			await fetchMod.fetch(url);
			throw new Error("expected connection error");
		} catch(e) {
			if (e.message === "expected connection error") throw e;
		}
	`)
}

func TestRequire_ExportsPresent(t *testing.T) {
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		vm := provider.Runtime()
		fetchVal := vm.Get("fetchMod")
		if fetchVal == nil || goja.IsUndefined(fetchVal) {
			t.Fatal("fetchMod should be defined")
		}
		obj := fetchVal.(*goja.Object)

		// fetch should be exported
		v := obj.Get("fetch")
		if v == nil || goja.IsUndefined(v) {
			t.Fatal("fetch should be exported")
		}

		// fetchStream should NOT be exported (removed)
		v = obj.Get("fetchStream")
		if v != nil && !goja.IsUndefined(v) {
			t.Fatal("fetchStream should not be exported")
		}
	})
}

// --- AbortController / AbortSignal tests ---

func TestFetch_AbortSignalCancelsRequest(t *testing.T) {
	t.Parallel()
	// Server that blocks until told to respond.
	// The abort should cancel the request before the server responds.
	blocker := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocker
		w.WriteHeader(200)
	}))
	defer server.Close()
	defer close(blocker)

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	// AbortController.abort() should cause the fetch promise to reject.
	runAsync(t, provider, `
		var ac = new AbortController();
		var p = fetchMod.fetch(url, { signal: ac.signal });
		// Abort after a short delay
		setTimeout(function() { ac.abort(); }, 50);
		try {
			await p;
			throw new Error("expected abort error");
		} catch(e) {
			if (e.message === "expected abort error") throw e;
			// The error should be from the aborted request
		}
	`)
}

func TestFetch_AlreadyAbortedSignal(t *testing.T) {
	t.Parallel()
	var received bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	// If the signal is already aborted before fetch is called,
	// the promise should reject immediately without making a request.
	runAsync(t, provider, `
		var ac = new AbortController();
		ac.abort("cancelled before start");
		try {
			await fetchMod.fetch(url, { signal: ac.signal });
			throw new Error("expected rejection");
		} catch(e) {
			if (e.message === "expected rejection") throw e;
		}
	`)

	if received {
		t.Error("server should NOT have received a request when signal is already aborted")
	}
}

func TestFetch_AbortSignalTimeout(t *testing.T) {
	t.Parallel()
	// Server that never responds.
	blocker := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocker
	}))
	defer server.Close()
	defer close(blocker)

	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)
	loadModule(t, provider)

	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set("url", server.URL)
	})

	// AbortSignal.timeout(ms) should abort after the specified time.
	runAsync(t, provider, `
		try {
			await fetchMod.fetch(url, { signal: AbortSignal.timeout(100) });
			throw new Error("expected timeout abort");
		} catch(e) {
			if (e.message === "expected timeout abort") throw e;
		}
	`)
}
