package fetch

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url, { timeout: 5 });
		if (resp.status !== 200) throw new Error("expected 200, got " + resp.status);
		if (resp.text() !== "ok") throw new Error("unexpected body");
	`)
	require.NoError(t, err)
	assert.True(t, received, "server should have received the request")
}

func TestFetch_MultiValueHeaders(t *testing.T) {
	t.Parallel()
	// When a response has multiple values for the same header,
	// they should be joined with ", ".
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("X-Multi", "val1")
		w.Header().Add("X-Multi", "val2")
		w.Header().Add("X-Multi", "val3")
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	v, err := runtime.RunString(`
		var resp = fetchMod.fetch(url);
		resp.headers["x-multi"];
	`)
	require.NoError(t, err)
	assert.Equal(t, "val1, val2, val3", v.String())
}

func TestFetch_NoOptions(t *testing.T) {
	t.Parallel()
	// fetch(url) with no second argument at all.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url);
		if (resp.status !== 200) throw new Error("expected 200");
	`)
	require.NoError(t, err)
}

func TestFetch_OptionsNull(t *testing.T) {
	t.Parallel()
	// fetch(url, null) — should be treated as no options.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url, null);
		if (resp.status !== 200) throw new Error("expected 200");
	`)
	require.NoError(t, err)
}

func TestFetch_OptionsUndefined(t *testing.T) {
	t.Parallel()
	// fetch(url, undefined) — should be treated as no options.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url, undefined);
		if (resp.status !== 200) throw new Error("expected 200");
	`)
	require.NoError(t, err)
}

func TestFetch_NonStringMethod(t *testing.T) {
	t.Parallel()
	// If method is not a string (e.g., a number), it should be silently ignored
	// and default to GET.
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		fetchMod.fetch(url, { method: 123 });
	`)
	require.NoError(t, err)
	assert.Equal(t, "GET", receivedMethod, "non-string method should be ignored, defaulting to GET")
}

func TestFetch_NonStringBody(t *testing.T) {
	t.Parallel()
	// If body is not a string (e.g., a number), it should be silently ignored.
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 100)
		n, _ := r.Body.Read(b)
		receivedBody = string(b[:n])
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		fetchMod.fetch(url, { body: 42 });
	`)
	require.NoError(t, err)
	assert.Empty(t, receivedBody, "non-string body should be ignored")
}

func TestFetch_NonObjectHeaders(t *testing.T) {
	t.Parallel()
	// If headers is not an object (e.g., a string), it should be silently ignored.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("X-Custom"), "non-object headers should be ignored")
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		fetchMod.fetch(url, { headers: "not-an-object" });
	`)
	require.NoError(t, err)
}

func TestFetch_HeaderWithNonStringValue(t *testing.T) {
	t.Parallel()
	// If a header value is not a string (e.g., a number), it should be silently skipped.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("X-Numeric"), "non-string header value should be skipped")
		assert.Equal(t, "valid", r.Header.Get("X-Valid"), "string header value should be set")
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		fetchMod.fetch(url, { headers: { "X-Numeric": 42, "X-Valid": "valid" } });
	`)
	require.NoError(t, err)
}

func TestFetch_OptionsNonObject(t *testing.T) {
	t.Parallel()
	// fetch(url, "string") — non-object second arg should be treated as no options.
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		fetchMod.fetch(url, "not-an-object");
	`)
	require.NoError(t, err)
	assert.Equal(t, "GET", receivedMethod, "non-object options should be ignored")
}

// --- fetchStream() coverage gaps ---

func TestStream_TimeoutInteger(t *testing.T) {
	t.Parallel()
	// Tests the int64 branch of timeout handling in fetchStream.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url, { timeout: 5 });
		if (resp.status !== 200) throw new Error("expected 200");
		var line = resp.readLine();
		if (line !== "ok") throw new Error("unexpected: " + line);
		resp.close();
	`)
	require.NoError(t, err)
}

func TestStream_MultiValueHeaders(t *testing.T) {
	t.Parallel()
	// Test multi-value headers in fetchStream response.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("X-Multi", "a")
		w.Header().Add("X-Multi", "b")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	v, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url);
		var h = resp.headers["x-multi"];
		resp.close();
		h;
	`)
	require.NoError(t, err)
	assert.Equal(t, "a, b", v.String())
}

func TestStream_BadURL(t *testing.T) {
	t.Parallel()
	// fetchStream with an invalid URL should panic with a Go error.
	runtime := setup(t)
	_, err := runtime.RunString(`
		try {
			fetchMod.fetchStream("://invalid-url");
			throw new Error("expected error");
		} catch(e) {
			if (e.message === "expected error") throw e;
		}
	`)
	require.NoError(t, err)
}

func TestStream_NonStringMethod(t *testing.T) {
	t.Parallel()
	// Non-string method should be ignored, defaulting to GET.
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url, { method: 123 });
		resp.close();
	`)
	require.NoError(t, err)
	assert.Equal(t, "GET", receivedMethod)
}

func TestStream_NonStringBody(t *testing.T) {
	t.Parallel()
	// Non-string body should be ignored.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 100)
		n, _ := r.Body.Read(b)
		assert.Zero(t, n, "non-string body should produce no request body")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url, { body: 42 });
		resp.close();
	`)
	require.NoError(t, err)
}

func TestStream_NonObjectHeaders(t *testing.T) {
	t.Parallel()
	// Non-object headers should be ignored.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url, { headers: "not-an-object" });
		resp.close();
	`)
	require.NoError(t, err)
}

func TestStream_HeaderWithNonStringValue(t *testing.T) {
	t.Parallel()
	// Non-string header value should be skipped.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("X-Numeric"), "non-string header value should be skipped")
		assert.Equal(t, "valid", r.Header.Get("X-Valid"), "string header value should be set")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url, { headers: { "X-Numeric": 42, "X-Valid": "valid" } });
		resp.close();
	`)
	require.NoError(t, err)
}

func TestStream_OptionsNull(t *testing.T) {
	t.Parallel()
	// fetchStream(url, null) should work like fetchStream(url).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url, null);
		if (resp.status !== 200) throw new Error("expected 200");
		resp.close();
	`)
	require.NoError(t, err)
}

func TestStream_OptionsUndefined(t *testing.T) {
	t.Parallel()
	// fetchStream(url, undefined) should work like fetchStream(url).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url, undefined);
		if (resp.status !== 200) throw new Error("expected 200");
		resp.close();
	`)
	require.NoError(t, err)
}

func TestStream_OptionsNonObject(t *testing.T) {
	t.Parallel()
	// fetchStream(url, "string") — non-object opts should be ignored.
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url, "not-an-object");
		resp.close();
	`)
	require.NoError(t, err)
	assert.Equal(t, "GET", receivedMethod)
}

func TestStream_Redirect(t *testing.T) {
	t.Parallel()
	// fetchStream should follow redirects and report the final URL.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("arrived\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL+"/redirect")
	_ = runtime.Set("expectedUrl", server.URL+"/final")
	v, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url);
		var u = resp.url;
		var line = resp.readLine();
		resp.close();
		if (u !== expectedUrl) throw new Error("bad url: " + u);
		if (line !== "arrived") throw new Error("bad line: " + line);
		u;
	`)
	require.NoError(t, err)
	assert.Contains(t, v.String(), "/final")
}

func TestStream_TimeoutFloat(t *testing.T) {
	t.Parallel()
	// Explicitly test the float64 timeout branch, which should allow fractional seconds.
	blocker := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocker
	}))
	defer server.Close()
	defer close(blocker)

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		try {
			fetchMod.fetchStream(url, { timeout: 0.1 });
			throw new Error("expected timeout");
		} catch(e) {
			if (e.message === "expected timeout") throw e;
		}
	`)
	require.NoError(t, err)
}

func TestFetch_EmptyBody(t *testing.T) {
	t.Parallel()
	// Test that text() returns empty string for empty body.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	v, err := runtime.RunString(`
		var resp = fetchMod.fetch(url);
		resp.text();
	`)
	require.NoError(t, err)
	assert.Equal(t, "", v.String())
}

func TestFetch_StatusTextContainsCode(t *testing.T) {
	t.Parallel()
	// Verify statusText includes the status code text.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	v, err := runtime.RunString(`
		var resp = fetchMod.fetch(url);
		resp.statusText;
	`)
	require.NoError(t, err)
	assert.Contains(t, v.String(), "403")
}

// --- Require function edge case ---

func TestRequire_ExportsAreFunctions(t *testing.T) {
	t.Parallel()
	// Verify that fetch and fetchStream are callable JS functions.
	runtime := setup(t)
	v, err := runtime.RunString(`
		typeof fetchMod.fetch === 'function' && typeof fetchMod.fetchStream === 'function';
	`)
	require.NoError(t, err)
	assert.True(t, v.ToBoolean())
}

func TestFetch_DeleteMethod(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`fetchMod.fetch(url, { method: 'delete' });`)
	require.NoError(t, err)
	assert.Equal(t, "DELETE", receivedMethod)
}

func TestFetch_PatchMethod(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`fetchMod.fetch(url, { method: 'patch' });`)
	require.NoError(t, err)
	assert.Equal(t, "PATCH", receivedMethod)
}
