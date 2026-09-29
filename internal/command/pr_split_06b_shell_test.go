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
//   - Always kill sessions in JS finally blocks
// ---------------------------------------------------------------------------

func TestCanSpawnInteractiveShell_Unix(t *testing.T) {
	if testing.Short() {
		t.Skip("PTY test requires spawning real processes")
	}
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
	evalJS := prsplittest.NewTUIEngine(t)

	dir := t.TempDir()

	raw, err := evalJS(fmt.Sprintf(`(async function() {
		var errors = [];
		var session;
		try {
			session = globalThis.prSplit.spawnShellSession(%q, {rows: 24, cols: 80});
			await session._startPromise;

			await session.write('echo hello_test_marker\n');

			// Poll for marker in accumulated output.
			var output = '';
			var deadline = Date.now() + 5000;
			var found = false;
			while (Date.now() < deadline) {
				var chunk = session.readAvailable();
				if (chunk !== null) output += chunk;
				if (output.indexOf('hello_test_marker') >= 0) {
					found = true;
					break;
				}
				await new Promise(function(resolve) { setTimeout(resolve, 10); });
			}
			if (!found) errors.push('did not find hello_test_marker in output');
		} catch (e) {
			errors.push('spawn error: ' + e.message);
		} finally {
			if (session) {
				await session.kill();
				await session.wait();
				await session.close();
			}
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
	evalJS := prsplittest.NewTUIEngine(t)

	dir := t.TempDir()

	raw, err := evalJS(fmt.Sprintf(`(async function() {
		var errors = [];
		var session;
		var exited = false;
		try {
			session = globalThis.prSplit.spawnShellSession(%q, {rows: 24, cols: 80});
			await session._startPromise;

			await session.write('exit\n');
			await session.wait();
			exited = true;
		} catch (e) {
			errors.push('error: ' + e.message);
		} finally {
			if (session) {
				if (!exited) await session.kill();
				await session.close();
			}
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
	evalJS := prsplittest.NewTUIEngine(t)

	dir := t.TempDir()
	// Resolve symlinks so the path matches what pwd reports.
	// macOS maps /var → /private/var.
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", dir, err)
	}

	raw, err := evalJS(fmt.Sprintf(`(async function() {
		var errors = [];
		var session;
		var dir = %q;
		try {
			session = globalThis.prSplit.spawnShellSession(dir, {rows: 24, cols: 200});
			await session._startPromise;
			await session.write('pwd\n');

			// Poll for the temp dir path in output.
			var output = '';
			var deadline = Date.now() + 5000;
			var found = false;
			while (Date.now() < deadline) {
				var chunk = session.readAvailable();
				if (chunk !== null) output += chunk;
				if (output.indexOf(dir) >= 0) {
					found = true;
					break;
				}
				await new Promise(function(resolve) { setTimeout(resolve, 10); });
			}
			if (!found) {
				errors.push('pwd output did not contain ' + dir + '; output: ' + output.substring(0, 200));
			}
		} catch (e) {
			errors.push('error: ' + e.message);
		} finally {
			if (session) {
				await session.kill();
				await session.wait();
				await session.close();
			}
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
	evalJS := prsplittest.NewTUIEngine(t)

	dir := t.TempDir()

	raw, err := evalJS(fmt.Sprintf(`(async function() {
		var errors = [];
		var session;
		try {
			session = globalThis.prSplit.spawnShellSession(%q, {rows: 24, cols: 80});
			await session._startPromise;

			// Resize — should not throw.
			await session.resize(40, 100);
		} catch (e) {
			errors.push('error: ' + e.message);
		} finally {
			if (session) {
				await session.kill();
				await session.wait();
				await session.close();
			}
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
	evalJS := prsplittest.NewTUIEngine(t)

	dir := t.TempDir()

	raw, err := evalJS(fmt.Sprintf(`(async function() {
		var errors = [];
		var session;
		try {
			session = globalThis.prSplit.spawnShellSession(%q, {rows: 30, cols: 100});
			await session._startPromise;
			if (session.pid() <= 0) errors.push('shell did not start with custom rows/cols');
		} catch (e) {
			errors.push('error: ' + e.message);
		} finally {
			if (session) {
				await session.kill();
				await session.wait();
				await session.close();
			}
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
