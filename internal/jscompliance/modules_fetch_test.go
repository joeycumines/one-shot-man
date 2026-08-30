package jscompliance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSlow_Fetch_GetJson asserts the fetch contract against a local httptest
// server: Response.status/ok, await resp.json(), and headers. Cross-platform
// (net/http/httptest works everywhere); no real network.
func TestSlow_Fetch_GetJson(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Marker", "present")
		_ = json.NewEncoder(w).Encode(map[string]any{"value": 42})
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	v, err := evalJS(t, engine, `(async function(){
		var f = require('osm:fetch');
		var resp = await f.fetch(`+jsStringLit(srv.URL)+`);
		return {
			status: resp.status,
			ok: resp.ok,
			marker: resp.headers.get('X-Marker'),
			body: await resp.json()
		};
	})()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("fetch resolved to %T, want object", v)
	}
	if s := asNum(t, m["status"]); s != 200 {
		t.Errorf("fetch status = %v, want 200", m["status"])
	}
	if ok, _ := m["ok"].(bool); !ok {
		t.Errorf("fetch ok = %v, want true", m["ok"])
	}
	if mk, _ := m["marker"].(string); mk != "present" {
		t.Errorf("fetch headers.get('X-Marker') = %q, want 'present'", mk)
	}
	if body, _ := m["body"].(map[string]any); asNum(t, body["value"]) != 42 {
		t.Errorf("fetch json body value = %v, want 42", body)
	}
}

// TestSlow_Fetch_AbortRejects cross-links the AbortController spec: a fetch
// with an aborted signal rejects.
func TestSlow_Fetch_AbortRejects(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	// A server that hangs forever so the abort is what rejects.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	v, err := evalJS(t, engine, `(async function(){
		var f = require('osm:fetch');
		var ac = new AbortController();
		ac.abort();
		try {
			await f.fetch(`+jsStringLit(srv.URL)+`, { signal: ac.signal });
			return 'NOT-ABORTED';
		} catch (e) {
			return 'ABORTED';
		}
	})()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("fetch-abort probe failed: %v", err)
	}
	if s, _ := v.(string); s != "ABORTED" {
		t.Errorf("fetch with an aborted signal should reject; got %v", v)
	}
}

// asNum is a local tolerant numeric converter (avoids importing the smoke-test
// helper's t.Fatalf path).
func asNum(t *testing.T, v any) float64 {
	t.Helper()
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	}
	t.Errorf("expected number, got %T: %v", v, v)
	return 0
}
