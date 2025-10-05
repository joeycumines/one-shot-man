package ctxutil_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/scripting/builtin"
	"github.com/joeycumines/one-shot-man/internal/scripting/builtin/ctxutil"
)

func setupContextManager(t *testing.T) *goja.Runtime {
	t.Helper()

	runtime := goja.New()

	registry := require.NewRegistry()

	builtin.Register(context.Background(),
		func(s string) {
			t.Logf("TUI: %s", s)
		},
		registry)

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
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	checks := map[string]string{
		"__hasGetItems":      "getItems method",
		"__hasSetItems":      "setItems method",
		"__hasAddItem":       "addItem method",
		"__hasCommands":      "commands object",
		"__hasBuildPrompt":   "buildPrompt method",
		"__hasAddCommand":    "add command",
		"__hasDiffCommand":   "diff command",
		"__hasNoteCommand":   "note command",
		"__hasListCommand":   "list command",
		"__hasEditCommand":   "edit command",
		"__hasRemoveCommand": "remove command",
		"__hasShowCommand":   "show command",
		"__hasCopyCommand":   "copy command",
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

	requiredCommands := []string{"add", "diff", "note", "list", "edit", "remove", "show", "copy"}
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
		ctxmgr.addItem("lazy-diff", "git diff HEAD~1", ["HEAD~1"]);

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
