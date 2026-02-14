package fetch

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	_ = runtime.Set("fetchMod", module.Get("exports"))
	return runtime
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

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url);
		if (resp.status !== 200) throw new Error("expected 200, got " + resp.status);
		if (resp.ok !== true) throw new Error("expected ok=true");
		if (resp.text() !== "hello world") throw new Error("unexpected body: " + resp.text());
	`)
	if err != nil {
		t.Fatal(err)
	}
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

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: '{"key":"value"}'
		});
		if (resp.status !== 200) throw new Error("expected 200");
		if (resp.text() !== "ok") throw new Error("unexpected body");
	`)
	if err != nil {
		t.Fatal(err)
	}

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

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url);
		var data = resp.json();
		if (data.name !== "test") throw new Error("bad name: " + data.name);
		if (data.count !== 42) throw new Error("bad count: " + data.count);
		if (data.active !== true) throw new Error("bad active");
		if (!Array.isArray(data.tags)) throw new Error("tags not array");
		if (data.tags.length !== 2) throw new Error("bad tags length");
		if (data.tags[0] !== "a") throw new Error("bad tags[0]");
		if (data.nested.key !== "val") throw new Error("bad nested.key");
		if (data.nothing !== null) throw new Error("bad nothing: " + data.nothing);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBadURL(t *testing.T) {
	t.Parallel()
	runtime := setup(t)

	_, err := runtime.RunString(`
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
}

func TestCustomTimeout(t *testing.T) {
	t.Parallel()
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
			fetchMod.fetch(url, { timeout: 0.1 });
			throw new Error("expected timeout");
		} catch(e) {
			if (e.message === "expected timeout") throw e;
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStatusCode404(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url);
		if (resp.status !== 404) throw new Error("expected 404, got " + resp.status);
		if (resp.ok !== false) throw new Error("expected ok=false for 404");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStatusCode500(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url);
		if (resp.status !== 500) throw new Error("expected 500, got " + resp.status);
		if (resp.ok !== false) throw new Error("expected ok=false for 500");
		if (resp.text() !== "internal server error") throw new Error("unexpected body");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestResponseHeaders(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.Header().Set("X-Another", "another-value")
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url);
		if (resp.headers["x-custom-header"] !== "custom-value") {
			throw new Error("bad x-custom-header: " + resp.headers["x-custom-header"]);
		}
		if (resp.headers["x-another"] !== "another-value") {
			throw new Error("bad x-another: " + resp.headers["x-another"]);
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
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

	runtime := setup(t)
	_ = runtime.Set("url", server.URL+"/redirect")
	_ = runtime.Set("expectedUrl", server.URL+"/final")
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url);
		if (resp.url !== expectedUrl) {
			throw new Error("expected url " + expectedUrl + ", got " + resp.url);
		}
		if (resp.text() !== "final") throw new Error("unexpected body after redirect");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDefaultMethod(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		fetchMod.fetch(url);
	`)
	if err != nil {
		t.Fatal(err)
	}
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

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url);
		if (resp.status !== 201) throw new Error("expected 201");
		if (resp.statusText.indexOf("201") === -1) {
			throw new Error("statusText should contain 201: " + resp.statusText);
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestJsonParseError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetch(url);
		if (resp.text() !== "not json") throw new Error("unexpected text");
		try {
			resp.json();
			throw new Error("expected json parse error");
		} catch(e) {
			if (e.message === "expected json parse error") throw e;
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMethodCaseInsensitive(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(200)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		fetchMod.fetch(url, { method: 'put' });
	`)
	if err != nil {
		t.Fatal(err)
	}
	if receivedMethod != "PUT" {
		t.Errorf("expected PUT (uppercased), got %s", receivedMethod)
	}
}

func TestConnectionRefused(t *testing.T) {
	t.Parallel()
	runtime := setup(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	_ = runtime.Set("url", url)
	_, err := runtime.RunString(`
		try {
			fetchMod.fetch(url);
			throw new Error("expected connection error");
		} catch(e) {
			if (e.message === "expected connection error") throw e;
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamBasicReadLine(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("line one\nline two\nline three\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url);
		var lines = [];
		while (true) {
			var line = resp.readLine();
			if (line === null) break;
			lines.push(line);
		}
		resp.close();
		if (lines.length !== 3) throw new Error("expected 3 lines, got " + lines.length);
		if (lines[0] !== "line one") throw new Error("bad line 0: " + lines[0]);
		if (lines[1] !== "line two") throw new Error("bad line 1: " + lines[1]);
		if (lines[2] !== "line three") throw new Error("bad line 2: " + lines[2]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamEOF(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("only line\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url);
		var first = resp.readLine();
		if (first !== "only line") throw new Error("bad first: " + first);
		var second = resp.readLine();
		if (second !== null) throw new Error("expected null at EOF, got: " + second);
		// Multiple calls after EOF should keep returning null
		var third = resp.readLine();
		if (third !== null) throw new Error("expected null on repeated EOF, got: " + third);
		resp.close();
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamClose(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("data\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)

	// Verify close can be called without reading the body; no panic
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url);
		if (resp.status !== 200) throw new Error("expected status 200");
		resp.close();
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamReadAll(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("first\nsecond\nthird\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url);
		// Read one line first
		var first = resp.readLine();
		if (first !== "first") throw new Error("bad first: " + first);
		// readAll gets the remaining body
		var rest = resp.readAll();
		if (rest !== "second\nthird\n") throw new Error("bad readAll: " + JSON.stringify(rest));
		resp.close();
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamHeaders(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Stream-Id", "abc123")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("data: hello\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url);
		if (resp.status !== 200) throw new Error("expected 200");
		if (resp.ok !== true) throw new Error("expected ok=true");
		if (resp.headers["x-stream-id"] !== "abc123") {
			throw new Error("bad x-stream-id: " + resp.headers["x-stream-id"]);
		}
		if (resp.headers["content-type"] !== "text/event-stream") {
			throw new Error("bad content-type: " + resp.headers["content-type"]);
		}
		var line = resp.readLine();
		if (line !== "data: hello") throw new Error("bad line: " + line);
		resp.close();
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamChunked(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "flushing not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(200)
		// Simulate SSE-style chunked data
		_, _ = w.Write([]byte("event: start\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: chunk1\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: chunk2\n"))
		flusher.Flush()
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url);
		if (resp.status !== 200) throw new Error("expected 200");
		var lines = [];
		while (true) {
			var line = resp.readLine();
			if (line === null) break;
			lines.push(line);
		}
		resp.close();
		if (lines.length !== 3) throw new Error("expected 3 lines, got " + lines.length + ": " + JSON.stringify(lines));
		if (lines[0] !== "event: start") throw new Error("bad line 0: " + lines[0]);
		if (lines[1] !== "data: chunk1") throw new Error("bad line 1: " + lines[1]);
		if (lines[2] !== "data: chunk2") throw new Error("bad line 2: " + lines[2]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamPostWithOptions(t *testing.T) {
	t.Parallel()
	var receivedMethod string
	var receivedBody string
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(200)
		_, _ = w.Write([]byte("response line 1\nresponse line 2\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: '{"prompt":"hello"}'
		});
		var lines = [];
		while (true) {
			var line = resp.readLine();
			if (line === null) break;
			lines.push(line);
		}
		resp.close();
		if (lines.length !== 2) throw new Error("expected 2 lines, got " + lines.length);
		if (lines[0] !== "response line 1") throw new Error("bad line 0");
		if (lines[1] !== "response line 2") throw new Error("bad line 1");
	`)
	if err != nil {
		t.Fatal(err)
	}

	if receivedMethod != "POST" {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected application/json, got %s", receivedContentType)
	}
	if receivedBody != `{"prompt":"hello"}` {
		t.Errorf("unexpected body: %q", receivedBody)
	}
}

func TestStreamNoTrailingNewline(t *testing.T) {
	t.Parallel()
	// Test that a final line without a trailing newline is still returned
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("line one\nline two without newline"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url);
		var lines = [];
		while (true) {
			var line = resp.readLine();
			if (line === null) break;
			lines.push(line);
		}
		resp.close();
		if (lines.length !== 2) throw new Error("expected 2 lines, got " + lines.length);
		if (lines[0] !== "line one") throw new Error("bad line 0: " + lines[0]);
		if (lines[1] !== "line two without newline") throw new Error("bad line 1: " + lines[1]);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamConnectionError(t *testing.T) {
	t.Parallel()
	runtime := setup(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	_ = runtime.Set("url", url)
	_, err := runtime.RunString(`
		try {
			fetchMod.fetchStream(url);
			throw new Error("expected connection error");
		} catch(e) {
			if (e.message === "expected connection error") throw e;
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamErrorStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("error details\n"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url);
		if (resp.status !== 500) throw new Error("expected 500, got " + resp.status);
		if (resp.ok !== false) throw new Error("expected ok=false");
		var line = resp.readLine();
		if (line !== "error details") throw new Error("expected error body, got: " + line);
		resp.close();
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamEmptyBody(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url);
		if (resp.status !== 204) throw new Error("expected 204");
		var line = resp.readLine();
		if (line !== null) throw new Error("expected null for empty body, got: " + line);
		resp.close();
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamReadAllWithoutReadLine(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("full body content"))
	}))
	defer server.Close()

	runtime := setup(t)
	_ = runtime.Set("url", server.URL)
	_, err := runtime.RunString(`
		var resp = fetchMod.fetchStream(url);
		var body = resp.readAll();
		if (body !== "full body content") throw new Error("bad readAll: " + JSON.stringify(body));
		resp.close();
	`)
	if err != nil {
		t.Fatal(err)
	}
}
