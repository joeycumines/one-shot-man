package jscompliance

import (
	"context"
	"testing"
)

// TestSlow_Mcp_Lifecycle pins the osm:mcp server lifecycle via the REAL engine
// (the production registry path; the mcpmod package tests use a test harness).
// The full JSON-RPC round-trip is covered by the package's own
// TestMCPCallback_E2E_ToolCall; here we pin createServer + addTool + the server
// surface + a clean close (no throw). run() is stdio-blocking and is not
// exercised (it needs a driven transport).
func TestSlow_Mcp_Lifecycle(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)

	v, err := evalJS(t, engine, `(function () {
		var mcp = require('osm:mcp');
		var srv = mcp.createServer('compliance-mcp', '1.0.0');
		// surface
		var surface = {
			isObject: typeof srv === 'object',
			addTool: typeof srv.addTool,
			run: typeof srv.run,
			close: typeof srv.close
		};
		// register a tool handler (must not throw)
		srv.addTool({ name: 'echo', description: 'echo', inputSchema: { type: 'object' } }, function (input) {
			return { text: 'echo:' + JSON.stringify(input) };
		});
		// close must not throw
		var closeThrew = false;
		try { srv.close(); } catch (e) { closeThrew = true; }
		surface.closeThrew = closeThrew;
		return surface;
	})()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("mcp lifecycle: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("mcp lifecycle returned %T, want object", v)
	}
	if b, _ := m["isObject"].(bool); !b {
		t.Errorf("mcp.createServer should return an object")
	}
	for _, meth := range []string{"addTool", "run", "close"} {
		if s, _ := m[meth].(string); s != "function" {
			t.Errorf("mcp server.%s typeof = %v, want function", meth, m[meth])
		}
	}
	if b, _ := m["closeThrew"].(bool); b {
		t.Errorf("mcp server.close() threw")
	}
}
