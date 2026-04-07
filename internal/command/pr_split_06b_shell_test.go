//go:build !windows

package command

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
// T341: spawnShellSession / canSpawnInteractiveShell unit tests
//
// These tests exercise the real PTY codepath via CaptureSession from chunk
// 06b_verify_shell. They spawn actual shell processes and therefore:
//   - MUST NOT run on Windows (build tag enforced above)
//   - Are skipped in -short mode
//   - Use busy-wait polling (Goja has no setTimeout/sleep)
//   - Always kill sessions in JS finally blocks
// ---------------------------------------------------------------------------

func TestCanSpawnInteractiveShell_Unix(t *testing.T) {
	if testing.Short() {
		t.Skip("PTY test requires spawning real processes")
	}
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var result = globalThis.prSplit.canSpawnInteractiveShell();
		if (result !== true) return 'FAIL: expected true, got ' + result;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("canSpawnInteractiveShell: %v", raw)
	}
}

func TestSpawnShell_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("PTY test requires spawning real processes")
	}
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	dir := t.TempDir()

	raw, err := evalJS(fmt.Sprintf(`(function() {
		var errors = [];
		var session;
		try {
			session = globalThis.prSplit.spawnShellSession(%q, {rows: 24, cols: 80});

			// Wait for shell prompt to appear (any output).
			var output = '';
			var deadline = Date.now() + 5000;
			while (Date.now() < deadline) {
				var chunk = session.readAvailable();
				if (chunk === null) break;
				output += chunk;
				if (output) break;
			}

			session.write('echo hello_test_marker\n');

			// Poll for marker in accumulated output.
			deadline = Date.now() + 5000;
			var found = false;
			while (Date.now() < deadline) {
				var chunk = session.readAvailable();
				if (chunk === null) break;
				output += chunk;
				if (output.indexOf('hello_test_marker') >= 0) {
					found = true;
					break;
				}
			}
			if (!found) errors.push('did not find hello_test_marker in output');
		} catch (e) {
			errors.push('spawn error: ' + e.message);
		} finally {
			if (session) try { session.kill(); } catch(e) {}
		}
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`, dir))
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("spawn shell happy path: %v", raw)
	}
}

func TestSpawnShell_ExitDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("PTY test requires spawning real processes")
	}
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	dir := t.TempDir()

	raw, err := evalJS(fmt.Sprintf(`(function() {
		var errors = [];
		var session;
		try {
			session = globalThis.prSplit.spawnShellSession(%q, {rows: 24, cols: 80});

			// Wait for shell prompt (any output).
			var deadline = Date.now() + 5000;
			while (Date.now() < deadline) {
				var chunk = session.readAvailable();
				if (chunk === null) break;
				if (chunk) break;
			}

			session.write('exit\n');

			// Poll isDone() until true or timeout.
			deadline = Date.now() + 10000;
			var exited = false;
			while (Date.now() < deadline) {
				if (session.isDone()) {
					exited = true;
					break;
				}
			}
			if (!exited) errors.push('shell did not exit within timeout');
		} catch (e) {
			errors.push('error: ' + e.message);
		} finally {
			if (session) try { session.kill(); } catch(e) {}
		}
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`, dir))
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("spawn shell exit detection: %v", raw)
	}
}

func TestSpawnShell_WorktreeDir(t *testing.T) {
	if testing.Short() {
		t.Skip("PTY test requires spawning real processes")
	}
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	dir := t.TempDir()
	// Resolve symlinks so the path matches what pwd reports.
	// macOS maps /var → /private/var.
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", dir, err)
	}

	raw, err := evalJS(fmt.Sprintf(`(function() {
		var errors = [];
		var session;
		var dir = %q;
		try {
			session = globalThis.prSplit.spawnShellSession(dir, {rows: 24, cols: 200});

			// Wait for shell prompt (any output).
			var output = '';
			var deadline = Date.now() + 5000;
			while (Date.now() < deadline) {
				var chunk = session.readAvailable();
				if (chunk === null) break;
				output += chunk;
				if (output) break;
			}

			session.write('pwd\n');

			// Poll for the temp dir path in output.
			deadline = Date.now() + 5000;
			var found = false;
			while (Date.now() < deadline) {
				var chunk = session.readAvailable();
				if (chunk === null) break;
				output += chunk;
				if (output.indexOf(dir) >= 0) {
					found = true;
					break;
				}
			}
			if (!found) {
				errors.push('pwd output did not contain ' + dir + '; output: ' + output.substring(0, 200));
			}
		} catch (e) {
			errors.push('error: ' + e.message);
		} finally {
			if (session) try { session.kill(); } catch(e) {}
		}
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`, resolvedDir))
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("spawn shell worktree dir: %v", raw)
	}
}

func TestSpawnShell_Resize(t *testing.T) {
	if testing.Short() {
		t.Skip("PTY test requires spawning real processes")
	}
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	dir := t.TempDir()

	raw, err := evalJS(fmt.Sprintf(`(function() {
		var errors = [];
		var session;
		try {
			session = globalThis.prSplit.spawnShellSession(%q, {rows: 24, cols: 80});

			// Wait for shell to start (any output).
			var deadline = Date.now() + 5000;
			while (Date.now() < deadline) {
				var chunk = session.readAvailable();
				if (chunk === null) break;
				if (chunk) break;
			}

			// Resize — should not throw.
			session.resize(40, 100);
		} catch (e) {
			errors.push('error: ' + e.message);
		} finally {
			if (session) try { session.kill(); } catch(e) {}
		}
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`, dir))
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("spawn shell resize: %v", raw)
	}
}

func TestSpawnShell_CustomRowsCols(t *testing.T) {
	if testing.Short() {
		t.Skip("PTY test requires spawning real processes")
	}
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	dir := t.TempDir()

	raw, err := evalJS(fmt.Sprintf(`(function() {
		var errors = [];
		var session;
		try {
			session = globalThis.prSplit.spawnShellSession(%q, {rows: 30, cols: 100});

			// Wait for shell to start — success means spawn worked with custom dimensions.
			var deadline = Date.now() + 5000;
			var started = false;
			while (Date.now() < deadline) {
				var chunk = session.readAvailable();
				if (chunk === null) break;
				if (chunk) {
					started = true;
					break;
				}
			}
			if (!started) errors.push('shell did not start with custom rows/cols');
		} catch (e) {
			errors.push('error: ' + e.message);
		} finally {
			if (session) try { session.kill(); } catch(e) {}
		}
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`, dir))
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("spawn shell custom rows/cols: %v", raw)
	}
}
