package ctxutil_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/builtin"
	"github.com/joeycumines/one-shot-man/internal/builtin/ctxutil"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func setupContextManager(t *testing.T) *goja.Runtime {
	t.Helper()

	runtime := goja.New()

	// Create test event loop provider (REQUIRED for builtin.Register)
	eventLoopProvider := testutil.NewTestEventLoopProvider()
	t.Cleanup(eventLoopProvider.Stop)

	registry := require.NewRegistry()

	builtin.Register(context.Background(),
		func(s string) {
			t.Logf("TUI: %s", s)
		},
		registry,
		nil, nil, eventLoopProvider)

	registry.Enable(runtime)

	_, err := runtime.RunString(`const exports = require("osm:ctxutil");`)
	if err != nil {
		t.Fatalf("failed to load ctxutil module: %v", err)
	}

	return runtime
}

func TestContextManagerBasicUsage(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: () => "test prompt"
		});

		// Test that manager methods are available
		globalThis.__hasGetItems = typeof ctxmgr.getItems === 'function';
		globalThis.__hasSetItems = typeof ctxmgr.setItems === 'function';
		globalThis.__hasAddItem = typeof ctxmgr.addItem === 'function';
		globalThis.__hasCommands = typeof ctxmgr.commands === 'object';
		globalThis.__hasBuildPrompt = typeof ctxmgr.buildPrompt === 'function';

		// Test buildPrompt
		globalThis.__promptResult = ctxmgr.buildPrompt();

		// Test that commands exist
		globalThis.__hasAddCommand = typeof ctxmgr.commands.add === 'object';
		globalThis.__hasDiffCommand = typeof ctxmgr.commands.diff === 'object';
		globalThis.__hasNoteCommand = typeof ctxmgr.commands.note === 'object';
		globalThis.__hasListCommand = typeof ctxmgr.commands.list === 'object';
		globalThis.__hasEditCommand = typeof ctxmgr.commands.edit === 'object';
		globalThis.__hasRemoveCommand = typeof ctxmgr.commands.remove === 'object';
		globalThis.__hasShowCommand = typeof ctxmgr.commands.show === 'object';
		globalThis.__hasCopyCommand = typeof ctxmgr.commands.copy === 'object';
		globalThis.__hasExecCommand = typeof ctxmgr.commands.exec === 'object';
		globalThis.__hasSnippetsCommand = typeof ctxmgr.commands.snippets === 'object';
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	checks := map[string]string{
		"__hasGetItems":        "getItems method",
		"__hasSetItems":        "setItems method",
		"__hasAddItem":         "addItem method",
		"__hasCommands":        "commands object",
		"__hasBuildPrompt":     "buildPrompt method",
		"__hasAddCommand":      "add command",
		"__hasDiffCommand":     "diff command",
		"__hasNoteCommand":     "note command",
		"__hasListCommand":     "list command",
		"__hasEditCommand":     "edit command",
		"__hasRemoveCommand":   "remove command",
		"__hasShowCommand":     "show command",
		"__hasCopyCommand":     "copy command",
		"__hasExecCommand":     "exec command",
		"__hasSnippetsCommand": "snippets command",
	}

	for varName, desc := range checks {
		if !runtime.Get(varName).ToBoolean() {
			t.Errorf("expected %s to be available", desc)
		}
	}

	if got := runtime.Get("__promptResult").String(); got != "test prompt" {
		t.Errorf("expected buildPrompt to return 'test prompt', got %q", got)
	}
}

func TestContextManagerAddItem(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; }
		});

		const id1 = ctxmgr.addItem("file", "test.txt", "content");
		const id2 = ctxmgr.addItem("note", "note", "note content");

		globalThis.__id1 = id1;
		globalThis.__id2 = id2;
		globalThis.__items = items;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	id1 := runtime.Get("__id1").ToInteger()
	id2 := runtime.Get("__id2").ToInteger()

	if id1 != 1 {
		t.Errorf("expected first id to be 1, got %d", id1)
	}
	if id2 != 2 {
		t.Errorf("expected second id to be 2, got %d", id2)
	}

	itemsVal := runtime.Get("__items")
	items := itemsVal.Export()
	itemsSlice, ok := items.([]interface{})
	if !ok {
		t.Fatalf("expected items to be a slice, got %T", items)
	}

	if len(itemsSlice) != 2 {
		t.Fatalf("expected 2 items, got %d", len(itemsSlice))
	}
}

func TestContextManagerCommandExtension(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: () => "base prompt",
			openEditor: () => "test note content"
		});

		// Test extending a command
		const commands = {
			...ctxmgr.commands,
			note: {
				...ctxmgr.commands.note,
				handler: function(args) {
					if (args.length === 1 && args[0] === "--special") {
						globalThis.__specialHandled = true;
						return;
					}
					return ctxmgr.commands.note.handler(args);
				}
			}
		};

		// Test that the base handler still works
		globalThis.__output = [];
		globalThis.output = {
			print: (msg) => { globalThis.__output.push(msg); }
		};

		// Call the extended handler with special arg
		commands.note.handler(["--special"]);
		globalThis.__specialResult = globalThis.__specialHandled === true;

		// Call with regular args (should delegate)
		commands.note.handler([]);
		globalThis.__regularResult = items.length === 1;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	if !runtime.Get("__specialResult").ToBoolean() {
		t.Error("expected special handler to be invoked")
	}

	if !runtime.Get("__regularResult").ToBoolean() {
		t.Error("expected regular handler to add item")
	}
}

func TestContextManagerNextIntegerId(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		const list = [
			{ id: 1, type: "file" },
			{ id: 5, type: "note" },
			{ id: 3, type: "diff" }
		];

		const ctxmgr = contextManager({
			getItems: () => [],
			setItems: () => {}
		});

		globalThis.__nextId = ctxmgr.nextIntegerId(list);
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	nextId := runtime.Get("__nextId").ToInteger()
	if nextId != 6 {
		t.Errorf("expected next id to be 6, got %d", nextId)
	}
}

func TestContextManagerCustomNextIntegerId(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		let customCalled = false;
		const ctxmgr = contextManager({
			getItems: () => [],
			setItems: () => {},
			nextIntegerId: (list) => {
				customCalled = true;
				return 42;
			}
		});

		const items = [];
		ctxmgr.setItems = (v) => { items.push(...v); };
		ctxmgr.getItems = () => items;

		const id = ctxmgr.addItem("file", "test", "");
		globalThis.__customCalled = customCalled;
		globalThis.__customId = id;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	if !runtime.Get("__customCalled").ToBoolean() {
		t.Error("expected custom nextIntegerId to be called")
	}

	if got := runtime.Get("__customId").ToInteger(); got != 42 {
		t.Errorf("expected custom id to be 42, got %d", got)
	}
}

func TestContextManagerHelperOverrides(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		let openEditorCalled = false;
		let clipboardCopyCalled = false;
		let fileExistsCalled = false;
		let formatArgvCalled = false;
		let parseArgvCalled = false;

		const ctxmgr = contextManager({
			getItems: () => [],
			setItems: () => {},
			buildPrompt: () => "test",
			openEditor: (title, initial) => {
				openEditorCalled = true;
				return "edited content";
			},
			clipboardCopy: (text) => {
				clipboardCopyCalled = true;
			},
			fileExists: (path) => {
				fileExistsCalled = true;
				return true;
			},
			formatArgv: (argv) => {
				formatArgvCalled = true;
				return argv.join(" ");
			},
			parseArgv: (str) => {
				parseArgvCalled = true;
				return str.split(" ");
			}
		});

		// Test that overrides are used
		ctxmgr.openEditor("test", "");
		ctxmgr.clipboardCopy("test");
		ctxmgr.fileExists("test.txt");
		ctxmgr.formatArgv(["a", "b"]);
		ctxmgr.parseArgv("a b");

		globalThis.__openEditorCalled = openEditorCalled;
		globalThis.__clipboardCopyCalled = clipboardCopyCalled;
		globalThis.__fileExistsCalled = fileExistsCalled;
		globalThis.__formatArgvCalled = formatArgvCalled;
		globalThis.__parseArgvCalled = parseArgvCalled;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	checks := map[string]string{
		"__openEditorCalled":    "openEditor",
		"__clipboardCopyCalled": "clipboardCopy",
		"__fileExistsCalled":    "fileExists",
		"__formatArgvCalled":    "formatArgv",
		"__parseArgvCalled":     "parseArgv",
	}

	for varName, desc := range checks {
		if !runtime.Get(varName).ToBoolean() {
			t.Errorf("expected custom %s to be called", desc)
		}
	}
}

func TestContextManagerCommandDescriptions(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		const ctxmgr = contextManager({
			getItems: () => [],
			setItems: () => {},
			buildPrompt: () => "test"
		});

		const descriptions = {};
		for (const [name, cmd] of Object.entries(ctxmgr.commands)) {
			descriptions[name] = cmd.description || "";
		}

		globalThis.__descriptions = descriptions;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	descriptionsVal := runtime.Get("__descriptions")
	descriptions := descriptionsVal.Export()
	descriptionsMap, ok := descriptions.(map[string]interface{})
	if !ok {
		t.Fatalf("expected descriptions to be a map, got %T", descriptions)
	}

	requiredCommands := []string{"add", "diff", "note", "list", "edit", "remove", "show", "copy", "exec", "snippets"}
	for _, cmd := range requiredCommands {
		desc, ok := descriptionsMap[cmd]
		if !ok {
			t.Errorf("expected command %q to have a description", cmd)
			continue
		}
		if descStr, ok := desc.(string); !ok || descStr == "" {
			t.Errorf("expected command %q to have a non-empty description, got %v", cmd, desc)
		}
	}
}

func TestContextManagerIntegrationWithBuildContext(t *testing.T) {
	runtime := setupContextManager(t)

	// Setup git diff mock
	restoreRunGitDiff := ctxutil.SetRunGitDiffFn(func(ctx context.Context, args []string) (string, string, bool) {
		return "+added line", "", false
	})
	restoreDefaultGitDiff := ctxutil.SetGetDefaultGitDiffArgsFn(func(ctx context.Context) []string {
		return []string{"HEAD~1"}
	})
	t.Cleanup(func() {
		restoreRunGitDiff()
		restoreDefaultGitDiff()
	})

	// Mock the context and output globals
	script := `
		const { contextManager, buildContext } = exports;

		// Track calls to context methods
		const contextCalls = {
			addPath: [],
			removePath: []
		};

		globalThis.context = {
			addPath: (path) => {
				contextCalls.addPath.push(path);
				return null;
			},
			removePath: (path) => {
				contextCalls.removePath.push(path);
				return null;
			},
			toTxtar: () => "txtar content"
		};

		// Track output.print calls
		const outputCalls = [];
		globalThis.output = {
			print: (msg) => {
				outputCalls.push(msg);
			}
		};

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: function() {
				return buildContext(this.getItems(), { toTxtar: () => globalThis.context.toTxtar() });
			}
		});

		// Test add command handler
		ctxmgr.commands.add.handler(["test.txt"]);
		globalThis.__addPathCalls = contextCalls.addPath.length;
		globalThis.__addPathArg = contextCalls.addPath[0];
		globalThis.__itemsAfterAdd = items.length;

		// Test that note was added via addItem
		ctxmgr.addItem("note", "test note", "This is a test note");

		// Test that lazy-diff was added
		ctxmgr.addItem("lazy-diff", "git diff HEAD", ["HEAD"]);

		globalThis.__itemsAfterNoteAndDiff = items.length;

		// Test remove command handler
		const fileItemId = items[0].id;
		ctxmgr.commands.remove.handler([String(fileItemId)]);
		globalThis.__removePathCalls = contextCalls.removePath.length;
		globalThis.__removePathArg = contextCalls.removePath[0];
		globalThis.__itemsAfterRemove = items.length;

		// Build the prompt
		const prompt = ctxmgr.buildPrompt();
		globalThis.__prompt = prompt;
		globalThis.__outputCalls = outputCalls.length;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	// Verify add command called context.addPath
	if addPathCalls := runtime.Get("__addPathCalls").ToInteger(); addPathCalls != 1 {
		t.Errorf("expected add command to call context.addPath once, got %d calls", addPathCalls)
	}
	if addPathArg := runtime.Get("__addPathArg").String(); addPathArg != "test.txt" {
		t.Errorf("expected add command to call context.addPath with 'test.txt', got %q", addPathArg)
	}
	if itemsAfterAdd := runtime.Get("__itemsAfterAdd").ToInteger(); itemsAfterAdd != 1 {
		t.Errorf("expected 1 item after add, got %d", itemsAfterAdd)
	}

	// Verify items were added
	if itemsAfterNoteAndDiff := runtime.Get("__itemsAfterNoteAndDiff").ToInteger(); itemsAfterNoteAndDiff != 3 {
		t.Errorf("expected 3 items after adding note and diff, got %d", itemsAfterNoteAndDiff)
	}

	// Verify remove command called context.removePath
	if removePathCalls := runtime.Get("__removePathCalls").ToInteger(); removePathCalls != 1 {
		t.Errorf("expected remove command to call context.removePath once, got %d calls", removePathCalls)
	}
	if removePathArg := runtime.Get("__removePathArg").String(); removePathArg != "test.txt" {
		t.Errorf("expected remove command to call context.removePath with 'test.txt', got %q", removePathArg)
	}
	if itemsAfterRemove := runtime.Get("__itemsAfterRemove").ToInteger(); itemsAfterRemove != 2 {
		t.Errorf("expected 2 items after remove, got %d", itemsAfterRemove)
	}

	// Verify output.print was called
	if outputCalls := runtime.Get("__outputCalls").ToInteger(); outputCalls < 1 {
		t.Errorf("expected output.print to be called at least once, got %d calls", outputCalls)
	}

	// Verify buildContext resolved lazy-diff and built proper prompt
	prompt := runtime.Get("__prompt").String()

	if !strings.Contains(prompt, "This is a test note") {
		t.Error("expected prompt to contain note content")
	}

	if !strings.Contains(prompt, "+added line") {
		t.Error("expected prompt to contain diff content")
	}

	if !strings.Contains(prompt, "txtar content") {
		t.Error("expected prompt to contain txtar content")
	}
}

func TestContextManagerDiffHandlerPayload(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;
		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; }
		});

		globalThis.output = { print: (msg) => {} };

		// Default invocation: no args -> payload should be empty array
		ctxmgr.commands.diff.handler([]);
		globalThis.__itemsAfterDefault = items.length;
		globalThis.__defaultPayload = items[items.length - 1].payload;

		// Custom args: should be persisted verbatim
		ctxmgr.commands.diff.handler(["HEAD~2", "--name-only"]);
		globalThis.__itemsAfterCustom = items.length;
		globalThis.__customPayload = items[items.length - 1].payload;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	if got := runtime.Get("__itemsAfterDefault").ToInteger(); got != 1 {
		t.Fatalf("expected 1 item after default diff, got %d", got)
	}

	defaultPayload := runtime.Get("__defaultPayload").Export()
	defaultSlice, ok := defaultPayload.([]interface{})
	if !ok {
		t.Fatalf("expected default payload to be a slice, got %T", defaultPayload)
	}
	if len(defaultSlice) != 0 {
		t.Fatalf("expected default payload to be empty slice, got %v", defaultSlice)
	}

	if got := runtime.Get("__itemsAfterCustom").ToInteger(); got != 2 {
		t.Fatalf("expected 2 items after custom diff, got %d", got)
	}

	customPayload := runtime.Get("__customPayload").Export()
	customSlice, ok := customPayload.([]interface{})
	if !ok {
		t.Fatalf("expected custom payload to be a slice, got %T", customPayload)
	}
	if len(customSlice) != 2 || customSlice[0].(string) != "HEAD~2" || customSlice[1].(string) != "--name-only" {
		t.Fatalf("expected custom payload [\"HEAD~2\",\"--name-only\"], got %v", customSlice)
	}
}

func TestContextManagerRemoveMissingFile(t *testing.T) {
	runtime := setupContextManager(t)

	// Mock the context and output globals where removePath returns an error indicating missing path
	script := `
		const { contextManager } = exports;

		const contextCalls = { addPath: [], removePath: [] };

		globalThis.context = {
			addPath: (path) => { contextCalls.addPath.push(path); return null; },
			removePath: (path) => { contextCalls.removePath.push(path); return new Error('path not found: ' + path); },
			toTxtar: () => ''
		};

		globalThis.output = { print: (msg) => { /* ignore */ } };

		let items = [];
		const ctxmgr = contextManager({ getItems: () => items, setItems: (v) => { items = v; } });

		// Add a file item
		ctxmgr.addItem('file', 'test.txt', '');
		const id = items[0].id;

		// Remove it - context.removePath will return a 'path not found' error,
		// but the handler should still remove the item from the list.
		ctxmgr.commands.remove.handler([String(id)]);

		globalThis.__itemsLen = items.length;
		globalThis.__removeCalls = contextCalls.removePath.length;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	if removeCalls := runtime.Get("__removeCalls").ToInteger(); removeCalls != 1 {
		t.Fatalf("expected context.removePath to be called once, got %d", removeCalls)
	}

	if itemsLen := runtime.Get("__itemsLen").ToInteger(); itemsLen != 0 {
		t.Fatalf("expected item to be removed despite missing file, got %d items", itemsLen)
	}
}

func TestContextManagerCopyRefreshesFileItems(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		const refreshedPaths = [];

		globalThis.context = {
			addPath: (path) => null,
			removePath: (path) => null,
			refreshPath: (path) => { refreshedPaths.push(path); },
			toTxtar: () => 'txtar content'
		};

		let clipboardContent = '';
		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: () => 'refreshed prompt',
			clipboardCopy: (text) => { clipboardContent = text; }
		});

		// Add file items
		items.push({id: 1, type: 'file', label: 'mydir', payload: ''});
		items.push({id: 2, type: 'file', label: 'other.txt', payload: ''});
		items.push({id: 3, type: 'note', label: 'note', payload: 'some note'});

		// Invoke copy
		ctxmgr.commands.copy.handler();

		globalThis.__refreshedPaths = refreshedPaths;
		globalThis.__clipboardContent = clipboardContent;
		globalThis.__outputCalls = outputCalls;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	// Verify refreshPath was called for each file-type item (but NOT notes)
	refreshedPaths := runtime.Get("__refreshedPaths").Export()
	paths, ok := refreshedPaths.([]interface{})
	if !ok {
		t.Fatalf("expected refreshedPaths to be a slice, got %T", refreshedPaths)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 refreshPath calls (for 2 file items), got %d", len(paths))
	}
	if paths[0].(string) != "mydir" {
		t.Errorf("expected first refresh path to be 'mydir', got %q", paths[0])
	}
	if paths[1].(string) != "other.txt" {
		t.Errorf("expected second refresh path to be 'other.txt', got %q", paths[1])
	}

	// Verify clipboard got the prompt
	if got := runtime.Get("__clipboardContent").String(); got != "refreshed prompt" {
		t.Errorf("expected clipboard content to be 'refreshed prompt', got %q", got)
	}
}

func TestContextManagerShowRefreshesFileItems(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		const refreshedPaths = [];

		globalThis.context = {
			addPath: (path) => null,
			removePath: (path) => null,
			refreshPath: (path) => { refreshedPaths.push(path); },
			toTxtar: () => ''
		};

		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: () => 'the prompt'
		});

		items.push({id: 1, type: 'file', label: 'somedir', payload: ''});

		// Invoke show
		ctxmgr.commands.show.handler();

		globalThis.__refreshedPaths = refreshedPaths;
		globalThis.__outputCalls = outputCalls;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	refreshedPaths := runtime.Get("__refreshedPaths").Export()
	paths, ok := refreshedPaths.([]interface{})
	if !ok {
		t.Fatalf("expected refreshedPaths to be a slice, got %T", refreshedPaths)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 refreshPath call, got %d", len(paths))
	}
	if paths[0].(string) != "somedir" {
		t.Errorf("expected refresh path to be 'somedir', got %q", paths[0])
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}
	if len(outputs) != 1 || outputs[0].(string) != "the prompt" {
		t.Errorf("expected show to output 'the prompt', got %v", outputs)
	}
}

func TestContextManagerRefreshIgnoresErrors(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		globalThis.context = {
			addPath: (path) => null,
			removePath: (path) => null,
			refreshPath: (path) => { throw new Error('file deleted: ' + path); },
			toTxtar: () => ''
		};

		let clipboardContent = '';
		globalThis.output = { print: (msg) => {} };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: () => 'still works',
			clipboardCopy: (text) => { clipboardContent = text; }
		});

		items.push({id: 1, type: 'file', label: 'deleted-dir', payload: ''});

		// Copy should NOT throw even though refreshPath throws
		ctxmgr.commands.copy.handler();
		globalThis.__clipboardContent = clipboardContent;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	if got := runtime.Get("__clipboardContent").String(); got != "still works" {
		t.Errorf("expected clipboard content 'still works' despite refresh error, got %q", got)
	}
}

func TestContextManagerErrorHandling(t *testing.T) {
	runtime := setupContextManager(t)

	// Test that contextManager requires certain methods
	script := `
		const { contextManager } = exports;

		let error1, error2;

		// Test missing getItems
		try {
			const ctxmgr = contextManager({
				setItems: () => {}
			});
			ctxmgr.getItems();
		} catch (e) {
			error1 = e.message;
		}

		// Test missing setItems
		try {
			const ctxmgr = contextManager({
				getItems: () => []
			});
			ctxmgr.setItems([]);
		} catch (e) {
			error2 = e.message;
		}

		globalThis.__error1 = error1;
		globalThis.__error2 = error2;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	error1 := runtime.Get("__error1").String()
	error2 := runtime.Get("__error2").String()

	if !strings.Contains(error1, "getItems must be provided") {
		t.Errorf("expected error1 to mention getItems, got %q", error1)
	}

	if !strings.Contains(error2, "setItems must be provided") {
		t.Errorf("expected error2 to mention setItems, got %q", error2)
	}
}

func TestContextManagerAddFromDiffBasic(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		const addPathCalls = [];
		globalThis.context = {
			addPath: (path) => { addPathCalls.push(path); return null; },
			removePath: () => null,
			toTxtar: () => ''
		};

		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			execv: (argv) => ({
				stdout: "file1.go\nfile2.txt\ndir/file3.js\n",
				stderr: "",
				code: 0,
				error: false,
				message: ""
			})
		});

		ctxmgr.commands.add.handler(["--from-diff"]);

		globalThis.__addPathCalls = addPathCalls;
		globalThis.__items = items;
		globalThis.__outputCalls = outputCalls;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	addPathCalls := runtime.Get("__addPathCalls").Export()
	paths, ok := addPathCalls.([]interface{})
	if !ok {
		t.Fatalf("expected addPathCalls to be a slice, got %T", addPathCalls)
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 addPath calls, got %d", len(paths))
	}
	if paths[0].(string) != "file1.go" {
		t.Errorf("expected first path to be 'file1.go', got %q", paths[0])
	}
	if paths[2].(string) != "dir/file3.js" {
		t.Errorf("expected third path to be 'dir/file3.js', got %q", paths[2])
	}

	items := runtime.Get("__items").Export()
	itemsSlice, ok := items.([]interface{})
	if !ok {
		t.Fatalf("expected items to be a slice, got %T", items)
	}
	if len(itemsSlice) != 3 {
		t.Fatalf("expected 3 items added, got %d", len(itemsSlice))
	}
}

func TestContextManagerAddFromDiffWithSpec(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		let receivedArgv;
		globalThis.context = {
			addPath: (path) => null,
			removePath: () => null,
			toTxtar: () => ''
		};
		globalThis.output = { print: (msg) => {} };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			execv: (argv) => {
				receivedArgv = argv;
				return {
					stdout: "changed.txt\n",
					stderr: "",
					code: 0,
					error: false,
					message: ""
				};
			}
		});

		ctxmgr.commands.add.handler(["--from-diff", "HEAD~2"]);

		globalThis.__receivedArgv = receivedArgv;
		globalThis.__itemCount = items.length;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	receivedArgv := runtime.Get("__receivedArgv").Export()
	argv, ok := receivedArgv.([]interface{})
	if !ok {
		t.Fatalf("expected receivedArgv to be a slice, got %T", receivedArgv)
	}
	// Should be ["git", "diff", "--name-only", "HEAD~2"]
	if len(argv) != 4 {
		t.Fatalf("expected 4 argv elements, got %d: %v", len(argv), argv)
	}
	if argv[0].(string) != "git" || argv[1].(string) != "diff" || argv[2].(string) != "--name-only" || argv[3].(string) != "HEAD~2" {
		t.Errorf("unexpected argv: %v", argv)
	}

	if got := runtime.Get("__itemCount").ToInteger(); got != 1 {
		t.Errorf("expected 1 item, got %d", got)
	}
}

func TestContextManagerAddFromDiffNoChanges(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		globalThis.context = {
			addPath: (path) => null,
			removePath: () => null,
			toTxtar: () => ''
		};

		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			execv: (argv) => ({
				stdout: "",
				stderr: "",
				code: 0,
				error: false,
				message: ""
			})
		});

		ctxmgr.commands.add.handler(["--from-diff"]);

		globalThis.__outputCalls = outputCalls;
		globalThis.__itemCount = items.length;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}
	if len(outputs) != 1 || !strings.Contains(outputs[0].(string), "No files found") {
		t.Errorf("expected 'No files found' message, got %v", outputs)
	}
	if got := runtime.Get("__itemCount").ToInteger(); got != 0 {
		t.Errorf("expected 0 items, got %d", got)
	}
}

func TestContextManagerAddFromDiffError(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		globalThis.context = {
			addPath: (path) => null,
			removePath: () => null,
			toTxtar: () => ''
		};

		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			execv: (argv) => ({
				stdout: "",
				stderr: "fatal: not a git repository",
				code: 128,
				error: true,
				message: "fatal: not a git repository"
			})
		});

		ctxmgr.commands.add.handler(["--from-diff"]);

		globalThis.__outputCalls = outputCalls;
		globalThis.__itemCount = items.length;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}
	if len(outputs) != 1 || !strings.Contains(outputs[0].(string), "git diff --name-only failed") {
		t.Errorf("expected error message, got %v", outputs)
	}
	if got := runtime.Get("__itemCount").ToInteger(); got != 0 {
		t.Errorf("expected 0 items, got %d", got)
	}
}

func TestContextManagerExecBasic(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			formatArgv: (argv) => argv.join(" ")
		});

		ctxmgr.commands.exec.handler(["ls", "-la"]);

		globalThis.__items = items;
		globalThis.__outputCalls = outputCalls;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	items := runtime.Get("__items").Export()
	itemsSlice, ok := items.([]interface{})
	if !ok {
		t.Fatalf("expected items to be a slice, got %T", items)
	}
	if len(itemsSlice) != 1 {
		t.Fatalf("expected 1 item, got %d", len(itemsSlice))
	}
	item := itemsSlice[0].(map[string]interface{})
	if item["type"].(string) != "lazy-exec" {
		t.Errorf("expected type 'lazy-exec', got %q", item["type"])
	}
	payload := item["payload"].([]interface{})
	if len(payload) != 2 || payload[0].(string) != "ls" || payload[1].(string) != "-la" {
		t.Errorf("expected payload [\"ls\", \"-la\"], got %v", payload)
	}
	if item["label"].(string) != "ls -la" {
		t.Errorf("expected label 'ls -la', got %q", item["label"])
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output call, got %d", len(outputs))
	}
	if !strings.Contains(outputs[0].(string), "Added exec:") {
		t.Errorf("expected output to contain 'Added exec:', got %q", outputs[0])
	}
	if !strings.Contains(outputs[0].(string), "will be executed when generating prompt") {
		t.Errorf("expected output to mention lazy execution, got %q", outputs[0])
	}
}

func TestContextManagerExecNoArgs(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; }
		});

		ctxmgr.commands.exec.handler([]);

		globalThis.__items = items;
		globalThis.__outputCalls = outputCalls;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	items := runtime.Get("__items").Export()
	itemsSlice, ok := items.([]interface{})
	if !ok {
		t.Fatalf("expected items to be a slice, got %T", items)
	}
	if len(itemsSlice) != 0 {
		t.Fatalf("expected 0 items, got %d", len(itemsSlice))
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output call, got %d", len(outputs))
	}
	if !strings.Contains(outputs[0].(string), "Usage: exec") {
		t.Errorf("expected usage message, got %q", outputs[0])
	}
}

func TestContextManagerExecEditLazyExec(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let editorTitle = null;
		let editorInitial = null;

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			openEditor: (title, initial) => {
				editorTitle = title;
				editorInitial = initial;
				return "cat /etc/hosts";
			},
			formatArgv: (argv) => argv.join(" "),
			parseArgv: (str) => str.split(" ").filter(s => s !== "")
		});

		// Manually add a lazy-exec item
		items.push({id: 1, type: "lazy-exec", label: "ls -la", payload: ["ls", "-la"]});

		// Edit it
		ctxmgr.commands.edit.handler(["1"]);

		globalThis.__editorTitle = editorTitle;
		globalThis.__editorInitial = editorInitial;
		globalThis.__items = items;
		globalThis.__outputCalls = outputCalls;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	// Verify editor was opened with correct title and initial content
	if got := runtime.Get("__editorTitle").String(); got != "exec-spec-1" {
		t.Errorf("expected editor title 'exec-spec-1', got %q", got)
	}
	if got := runtime.Get("__editorInitial").String(); got != "ls -la" {
		t.Errorf("expected editor initial 'ls -la', got %q", got)
	}

	// Verify item was updated
	items := runtime.Get("__items").Export()
	itemsSlice, ok := items.([]interface{})
	if !ok {
		t.Fatalf("expected items to be a slice, got %T", items)
	}
	if len(itemsSlice) != 1 {
		t.Fatalf("expected 1 item, got %d", len(itemsSlice))
	}
	item := itemsSlice[0].(map[string]interface{})
	payload := item["payload"].([]interface{})
	if len(payload) != 2 || payload[0].(string) != "cat" || payload[1].(string) != "/etc/hosts" {
		t.Errorf("expected updated payload [\"cat\", \"/etc/hosts\"], got %v", payload)
	}
	if item["label"].(string) != "cat /etc/hosts" {
		t.Errorf("expected updated label 'cat /etc/hosts', got %q", item["label"])
	}

	// Verify output
	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output call, got %d", len(outputs))
	}
	if !strings.Contains(outputs[0].(string), "Updated exec specification") {
		t.Errorf("expected 'Updated exec specification' message, got %q", outputs[0])
	}
}

func TestContextManagerAddFromDiffPartialFailure(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		globalThis.context = {
			addPath: (path) => {
				if (path === "deleted.txt") return { message: "no such file" };
				return null;
			},
			removePath: () => null,
			toTxtar: () => ''
		};

		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			execv: (argv) => ({
				stdout: "good.txt\ndeleted.txt\nalso-good.txt\n",
				stderr: "",
				code: 0,
				error: false,
				message: ""
			})
		});

		ctxmgr.commands.add.handler(["--from-diff"]);

		globalThis.__items = items;
		globalThis.__outputCalls = outputCalls;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	// 2 successful adds (good.txt and also-good.txt), 1 failed (deleted.txt)
	items := runtime.Get("__items").Export()
	itemsSlice, ok := items.([]interface{})
	if !ok {
		t.Fatalf("expected items to be a slice, got %T", items)
	}
	if len(itemsSlice) != 2 {
		t.Fatalf("expected 2 items (skipping deleted file), got %d", len(itemsSlice))
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}
	// Should have: "Added file: good.txt", "add error: no such file", "Added file: also-good.txt"
	hasError := false
	addedCount := 0
	for _, o := range outputs {
		s := o.(string)
		if strings.Contains(s, "add error") {
			hasError = true
		}
		if strings.Contains(s, "Added file:") {
			addedCount++
		}
	}
	if !hasError {
		t.Error("expected an add error for the deleted file")
	}
	if addedCount != 2 {
		t.Errorf("expected 2 'Added file:' messages, got %d", addedCount)
	}
}

func TestContextManagerPostCopyHint(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		let clipboardContent = '';
		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: () => 'test prompt',
			clipboardCopy: (text) => { clipboardContent = text; },
			postCopyHint: '[Hint] Try a follow-up prompt: "Do something next."'
		});

		ctxmgr.commands.copy.handler();

		globalThis.__clipboardContent = clipboardContent;
		globalThis.__outputCalls = outputCalls;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	if got := runtime.Get("__clipboardContent").String(); got != "test prompt" {
		t.Errorf("expected clipboard content 'test prompt', got %q", got)
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}

	if len(outputs) != 2 {
		t.Fatalf("expected 2 output calls (copy confirmation + hint), got %d: %v", len(outputs), outputs)
	}
	if !strings.Contains(outputs[0].(string), "Prompt copied to clipboard.") {
		t.Errorf("expected copy confirmation, got %q", outputs[0])
	}
	if !strings.Contains(outputs[1].(string), "[Hint]") {
		t.Errorf("expected hint message, got %q", outputs[1])
	}
	if !strings.Contains(outputs[1].(string), "Do something next.") {
		t.Errorf("expected hint text content, got %q", outputs[1])
	}
}

func TestContextManagerPostCopyHintNotShownWhenEmpty(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		let clipboardContent = '';
		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: () => 'test prompt',
			clipboardCopy: (text) => { clipboardContent = text; }
		});

		ctxmgr.commands.copy.handler();
		globalThis.__outputCalls = outputCalls;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}

	if len(outputs) != 1 {
		t.Fatalf("expected 1 output call (copy confirmation only, no hint), got %d: %v", len(outputs), outputs)
	}
	if !strings.Contains(outputs[0].(string), "Prompt copied to clipboard.") {
		t.Errorf("expected copy confirmation, got %q", outputs[0])
	}
}

func TestContextManagerHotSnippetBasic(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		let clipboardContent = '';
		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: () => 'test',
			clipboardCopy: (text) => { clipboardContent = text; },
			hotSnippets: [
				{ name: "followup", text: "Continue with the same context.", description: "Follow-up prompt" },
				{ name: "kickoff", text: "You are an expert software engineer." }
			]
		});

		// Verify snippet commands exist
		globalThis.__hasFollowup = typeof ctxmgr.commands.followup === 'object';
		globalThis.__hasKickoff = typeof ctxmgr.commands.kickoff === 'object';
		globalThis.__followupDesc = ctxmgr.commands.followup.description;
		globalThis.__kickoffDesc = ctxmgr.commands.kickoff.description;

		// Execute the followup snippet
		ctxmgr.commands.followup.handler();
		globalThis.__clipboardContent = clipboardContent;
		globalThis.__outputCalls = outputCalls;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	if !runtime.Get("__hasFollowup").ToBoolean() {
		t.Error("expected followup command to exist")
	}
	if !runtime.Get("__hasKickoff").ToBoolean() {
		t.Error("expected kickoff command to exist")
	}

	if got := runtime.Get("__followupDesc").String(); got != "Follow-up prompt" {
		t.Errorf("expected followup description 'Follow-up prompt', got %q", got)
	}
	if got := runtime.Get("__kickoffDesc").String(); got != "Hot snippet: kickoff" {
		t.Errorf("expected kickoff description 'Hot snippet: kickoff', got %q", got)
	}

	if got := runtime.Get("__clipboardContent").String(); got != "Continue with the same context." {
		t.Errorf("expected clipboard to contain snippet text, got %q", got)
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output call, got %d", len(outputs))
	}
	if !strings.Contains(outputs[0].(string), "Copied snippet 'followup'") {
		t.Errorf("expected copy confirmation, got %q", outputs[0])
	}
}

func TestContextManagerHotSnippetWarning(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		let clipboardContent = '';
		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: () => 'test',
			clipboardCopy: (text) => { clipboardContent = text; },
			hotSnippets: [
				{ name: "builtin1", text: "Builtin snippet text", builtin: true }
			]
		});

		ctxmgr.commands.builtin1.handler();
		globalThis.__outputCalls = outputCalls;
		globalThis.__clipboardContent = clipboardContent;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}

	// Should have warning + copy confirmation = 2 outputs
	if len(outputs) != 2 {
		t.Fatalf("expected 2 output calls (warning + confirmation), got %d: %v", len(outputs), outputs)
	}
	if !strings.Contains(outputs[0].(string), "Note: Using embedded snippet") {
		t.Errorf("expected warning message, got %q", outputs[0])
	}
	if !strings.Contains(outputs[0].(string), "builtin1") {
		t.Errorf("expected warning to mention snippet name, got %q", outputs[0])
	}
	if !strings.Contains(outputs[1].(string), "Copied snippet") {
		t.Errorf("expected copy confirmation, got %q", outputs[1])
	}

	if got := runtime.Get("__clipboardContent").String(); got != "Builtin snippet text" {
		t.Errorf("expected clipboard content 'Builtin snippet text', got %q", got)
	}
}

func TestContextManagerHotSnippetWarningDisabled(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		let clipboardContent = '';
		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: () => 'test',
			clipboardCopy: (text) => { clipboardContent = text; },
			hotSnippets: [
				{ name: "builtin1", text: "Builtin snippet text", builtin: true }
			],
			noSnippetWarning: true
		});

		ctxmgr.commands.builtin1.handler();
		globalThis.__outputCalls = outputCalls;
		globalThis.__clipboardContent = clipboardContent;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}

	// Should have only copy confirmation, NO warning
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output call (no warning), got %d: %v", len(outputs), outputs)
	}
	if !strings.Contains(outputs[0].(string), "Copied snippet") {
		t.Errorf("expected copy confirmation, got %q", outputs[0])
	}
	// Ensure it does NOT contain the warning
	if strings.Contains(outputs[0].(string), "Note: Using embedded snippet") {
		t.Errorf("expected no warning when noSnippetWarning=true, got %q", outputs[0])
	}

	if got := runtime.Get("__clipboardContent").String(); got != "Builtin snippet text" {
		t.Errorf("expected clipboard content 'Builtin snippet text', got %q", got)
	}
}

func TestContextManagerHotSnippetsList(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: () => 'test',
			hotSnippets: [
				{ name: "followup", text: "Continue with the same context.", description: "Follow-up prompt" },
				{ name: "kickoff", text: "You are an expert software engineer.", builtin: true },
				{ name: "review", text: "Review this code for correctness and style." }
			]
		});

		// Verify snippets command exists
		globalThis.__hasSnippetsCmd = typeof ctxmgr.commands.snippets === 'object';

		ctxmgr.commands.snippets.handler();
		globalThis.__outputCalls = outputCalls;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	if !runtime.Get("__hasSnippetsCmd").ToBoolean() {
		t.Error("expected snippets command to exist")
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}

	if len(outputs) != 3 {
		t.Fatalf("expected 3 output lines (one per snippet), got %d: %v", len(outputs), outputs)
	}

	// First snippet: has description
	if !strings.Contains(outputs[0].(string), "followup") {
		t.Errorf("expected first line to contain 'followup', got %q", outputs[0])
	}
	if !strings.Contains(outputs[0].(string), "Follow-up prompt") {
		t.Errorf("expected first line to contain description, got %q", outputs[0])
	}

	// Second snippet: builtin marker
	if !strings.Contains(outputs[1].(string), "kickoff") {
		t.Errorf("expected second line to contain 'kickoff', got %q", outputs[1])
	}
	if !strings.Contains(outputs[1].(string), "[embedded]") {
		t.Errorf("expected second line to contain '[embedded]' marker, got %q", outputs[1])
	}

	// Third snippet: no builtin marker, text preview
	if !strings.Contains(outputs[2].(string), "review") {
		t.Errorf("expected third line to contain 'review', got %q", outputs[2])
	}
	if strings.Contains(outputs[2].(string), "[embedded]") {
		t.Errorf("expected third line NOT to contain '[embedded]' marker, got %q", outputs[2])
	}
}

func TestContextManagerHotSnippetsEmpty(t *testing.T) {
	runtime := setupContextManager(t)

	script := `
		const { contextManager } = exports;

		const outputCalls = [];
		globalThis.output = { print: (msg) => { outputCalls.push(msg); } };

		let items = [];
		const ctxmgr = contextManager({
			getItems: () => items,
			setItems: (v) => { items = v; },
			buildPrompt: () => 'test'
		});

		// Verify snippets command exists even with no snippets
		globalThis.__hasSnippetsCmd = typeof ctxmgr.commands.snippets === 'object';

		ctxmgr.commands.snippets.handler();
		globalThis.__outputCalls = outputCalls;
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	if !runtime.Get("__hasSnippetsCmd").ToBoolean() {
		t.Error("expected snippets command to exist even with no snippets")
	}

	outputCalls := runtime.Get("__outputCalls").Export()
	outputs, ok := outputCalls.([]interface{})
	if !ok {
		t.Fatalf("expected outputCalls to be a slice, got %T", outputCalls)
	}

	if len(outputs) != 1 {
		t.Fatalf("expected 1 output line, got %d: %v", len(outputs), outputs)
	}
	if !strings.Contains(outputs[0].(string), "No hot-snippets configured") {
		t.Errorf("expected 'No hot-snippets configured' message, got %q", outputs[0])
	}
}
