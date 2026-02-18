package fetch_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// newProvider creates a TestEventLoopProvider and registers cleanup.
func newProvider(t *testing.T) *testutil.TestEventLoopProvider {
	t.Helper()
	p := testutil.NewTestEventLoopProvider()
	t.Cleanup(p.Stop)
	return p
}

// setVar sets a JS variable on the event loop.
func setVar(t *testing.T, provider *testutil.TestEventLoopProvider, name string, value interface{}) {
	t.Helper()
	runOnLoop(t, provider, func() {
		_ = provider.Runtime().Set(name, value)
	})
}

// --- fetch() coverage gaps ---

func TestFetch_TimeoutInteger(t *testing.T) {
	t.Parallel()
	// JS integer values export as int64 from Goja.
	// This tests the int64 branch of timeout handling.
	var received bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url, { timeout: 5 });
		if (resp.status !== 200) throw new Error("expected 200, got " + resp.status);
		var body = await resp.text();
		if (body !== "ok") throw new Error("unexpected body");
	`)
	if !received {
		t.Error("server should have received the request")
	}
}

func TestFetch_MultiValueHeaders(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("X-Multi", "val1")
		w.Header().Add("X-Multi", "val2")
		w.Header().Add("X-Multi", "val3")
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		var v = resp.headers.get("x-multi");
		if (v !== "val1, val2, val3") throw new Error("bad x-multi: " + v);
	`)
}

func TestFetch_NoOptions(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		if (resp.status !== 200) throw new Error("expected 200");
	`)
}

func TestFetch_OptionsNull(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url, null);
		if (resp.status !== 200) throw new Error("expected 200");
	`)
}

func TestFetch_OptionsUndefined(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url, undefined);
		if (resp.status !== 200) throw new Error("expected 200");
	`)
}

func TestFetch_NonStringMethod(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		await fetchMod.fetch(url, { method: 123 });
	`)
	if receivedMethod != "GET" {
		t.Errorf("non-string method should default to GET, got %s", receivedMethod)
	}
}

func TestFetch_NonStringBody(t *testing.T) {
	t.Parallel()
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 100)
		n, _ := r.Body.Read(b)
		receivedBody = string(b[:n])
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		await fetchMod.fetch(url, { body: 42 });
	`)
	if receivedBody != "" {
		t.Errorf("non-string body should be ignored, got %q", receivedBody)
	}
}

func TestFetch_NonObjectHeaders(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("X-Custom"); v != "" {
			t.Errorf("non-object headers should be ignored, got X-Custom=%q", v)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		await fetchMod.fetch(url, { headers: "not-an-object" });
	`)
}

func TestFetch_HeaderWithNonStringValue(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("X-Numeric"); v != "" {
			t.Errorf("non-string header value should be skipped, got X-Numeric=%q", v)
		}
		if v := r.Header.Get("X-Valid"); v != "valid" {
			t.Errorf("expected X-Valid=valid, got %q", v)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		await fetchMod.fetch(url, { headers: { "X-Numeric": 42, "X-Valid": "valid" } });
	`)
}

func TestFetch_OptionsNonObject(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		await fetchMod.fetch(url, "not-an-object");
	`)
	if receivedMethod != "GET" {
		t.Errorf("non-object options should default to GET, got %s", receivedMethod)
	}
}

func TestFetch_EmptyBody(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		var body = await resp.text();
		if (body !== "") throw new Error("expected empty body, got: " + JSON.stringify(body));
	`)
}

func TestFetch_StatusTextContainsCode(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		var resp = await fetchMod.fetch(url);
		if (resp.statusText.indexOf("403") === -1) {
			throw new Error("statusText should contain 403: " + resp.statusText);
		}
	`)
}

func TestFetch_DeleteMethod(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `await fetchMod.fetch(url, { method: 'delete' });`)
	if receivedMethod != "DELETE" {
		t.Errorf("expected DELETE, got %s", receivedMethod)
	}
}

func TestFetch_PatchMethod(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `await fetchMod.fetch(url, { method: 'patch' });`)
	if receivedMethod != "PATCH" {
		t.Errorf("expected PATCH, got %s", receivedMethod)
	}
}

func TestFetch_TimeoutFloat(t *testing.T) {
	t.Parallel()
	blocker := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocker
	}))
	defer server.Close()
	defer close(blocker)

	provider := newProvider(t)
	loadModule(t, provider)
	setVar(t, provider, "url", server.URL)

	runAsync(t, provider, `
		try {
			await fetchMod.fetch(url, { timeout: 0.1 });
			throw new Error("expected timeout");
		} catch(e) {
			if (e.message === "expected timeout") throw e;
		}
	`)
}
