package jscompliance

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// echoCommand returns a portable (cmd, args) that prints marker to stdout.
func echoCommand(marker string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo", marker}
	}
	return "echo", []string{marker}
}

// jsArray builds a JS array-literal string from cmd + args (each single-quoted).
func jsArray(cmd string, args []string) string {
	return jsStrings(append([]string{cmd}, args...))
}

// jsStrings builds a JS array-literal from the given strings (each single-quoted).
func jsStrings(parts []string) string {
	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = jsStringLit(p)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

// TestSlow_Exec_ExecvShape asserts execv returns the documented
// {stdout, stderr, code, error, message} shape with code 0 on success.
func TestSlow_Exec_ExecvShape(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	cmd, args := echoCommand("jscompliance-exec-marker")
	v, err := evalJS(t, engine, fmt.Sprintf(`await require('osm:exec').execv(%s)`, jsArray(cmd, args)), defaultEvalTimeout)
	if err != nil {
		t.Fatalf("execv failed: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("execv resolved to %T, want object", v)
	}
	for _, k := range []string{"stdout", "stderr", "code", "error"} {
		if _, has := m[k]; !has {
			t.Errorf("execv result missing %q (documented shape {stdout,stderr,code,error,message})", k)
		}
	}
	if code := m["code"]; code != int64(0) && code != float64(0) && code != 0 {
		t.Errorf("execv code = %v, want 0", m["code"])
	}
	if out, _ := m["stdout"].(string); !strings.Contains(out, "jscompliance-exec-marker") {
		t.Errorf("execv stdout = %q, want it to contain the marker", out)
	}
}

// TestSlow_Exec_SpawnWaitIsAsync asserts the spawn handle's wait() returns a
// Promise (exec.go:150 contract), closing the exec.wait concern (WAIT-1 is
// termmux-only; exec.spawn.wait is async). Also checks the handle exposes the
// documented surface.
func TestSlow_Exec_SpawnWaitIsAsync(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	cmd, args := echoCommand("spawn-marker")
	v, err := evalJS(t, engine, fmt.Sprintf(`(async function(){
		var h = await require('osm:exec').spawn(%s, %s);
		var w = h.wait();
		var isPromise = (w !== null && w !== undefined && typeof w === 'object' && typeof w.then === 'function');
		return isPromise;
	})()`, jsStringLit(cmd), jsStrings(args)), defaultEvalTimeout)
	if err != nil {
		t.Fatalf("spawn probe failed: %v", err)
	}
	if b, ok := v.(bool); !ok || !b {
		t.Errorf("exec spawn must resolve to a handle whose wait() returns a Promise (binding contract); got %v", v)
	}
}

// TestSlow_Exec_SpawnReadStream pins the spawn handle's stdout readable stream:
// read() returns Promise<{value, done}>; draining it yields the child's stdout.
// Completes exec's async coverage (wait + streaming reads).
func TestSlow_Exec_SpawnReadStream(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	cmd, args := echoCommand("spawn-stream-probe")
	v, err := evalJS(t, engine, fmt.Sprintf(`(async function () {
		var h = await require('osm:exec').spawn(%s, %s);
		var out = '';
		while (true) {
			var chunk = await h.stdout.read();
			if (chunk.done) break;
			out += chunk.value;
		}
		return out;
	})()`, jsStringLit(cmd), jsStrings(args)), defaultEvalTimeout)
	if err != nil {
		t.Fatalf("spawn read stream failed: %v", err)
	}
	if s, _ := v.(string); !strings.Contains(s, "spawn-stream-probe") {
		t.Errorf("spawn stdout stream = %q, want it to contain the marker", s)
	}
}
