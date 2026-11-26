package ctxutil_test

import (
	"testing"
	"time"
)

// An observed exit 0 does not authorize consuming a partial diff or a command
// whose native tree cleanup failed. Keep the public context-manager consumer
// pinned to execv's error fact before it adds any paths or items.
func TestContextManagerAddFromDiffRejectsExitZeroFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the JavaScript engine")
	}
	runtime, loop := setupContextManager(t)
	done := make(chan string, 1)
	if err := loop.Submit(func() {
		if err := runtime.Set("__consumerResult", func(message string) { done <- message }); err != nil {
			done <- err.Error()
			return
		}
		_, err := runtime.RunString(`(async function() {
			for (var fixture of [
				{code: 0, error: true, truncated: true, message: 'capture incomplete'},
				{code: 0, error: true, truncated: false, treeCleanupError: 'tree close denied', message: 'tree close denied'}
			]) {
				fixture.stdout = 'partial.go\n';
				fixture.stderr = '';
				var paths = [], items = [], messages = [];
				globalThis.context = {
					addPath: function(path) { paths.push(path); },
					removePath: function() {}, toTxtar: function() { return ''; }
				};
				globalThis.output = {print: function(message) { messages.push(message); }};
				var manager = exports.contextManager({
					getItems: function() { return items; },
					setItems: function(value) { items = value; },
					execv: async function() { return fixture; }
				});
				await manager.commands.add.handler(['--from-diff']);
				if (paths.length !== 0 || items.length !== 0) {
					throw new Error('failed diff changed context: ' + JSON.stringify({fixture: fixture, paths: paths, items: items}));
				}
				if (messages.length !== 1 || messages[0] !== 'git diff --name-only failed: ' + fixture.message) {
					throw new Error('failed diff diagnostic lost: ' + JSON.stringify(messages));
				}
			}
		})().then(function() { __consumerResult(''); }, function(error) { __consumerResult(String(error)); });`)
		if err != nil {
			done <- err.Error()
		}
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-done:
		if message != "" {
			t.Fatal(message)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("context-manager diff consumer did not complete")
	}
}
