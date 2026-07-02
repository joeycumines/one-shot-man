package jscompliance

import (
	"context"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// moduleContract is one row of the contract authority table. Adding a module
// or export = adding a row/field here; TestModuleContract covers it.
//
//   - asyncExports: module-level exports documented to return a Promise
//     (the binding contract). Checked for typeof === 'function' here; their
//     Promise-shape is asserted in the behavioral specs + binding-contract
//     test (calling them is I/O, so not in the FAST tier).
//   - mustExist: headline documented exports whose PRESENCE is asserted
//     (catches a removed/renamed export).
//   - smoke/smokeWant: an optional JS expression that INVOKES a safe export
//     and the expression for its expected value. Closes the "typeof passes
//     but the function throws" trap for pure modules in the FAST tier.
type moduleContract struct {
	name         string // without the osm: prefix
	unixOnly     bool
	asyncExports []string
	mustExist    []string
	smoke        string // optional; empty disables
	smokeWant    string
}

// moduleContracts enumerates all 47 production osm: modules (45 in
// register.go + osm:sharedStateSymbols + osm:bt) cross-checked against
// docs/scripting.md AND each module's module.go exports. nextIntegerID and
// nextIntegerId are aliases (deprecated) of one module.
//
// Drift between this table and the dynamic surface (TestModuleSurface) is a
// finding: missing documented exports FAIL; undocumented actual exports are
// logged for triage into docs (DRIFT-7/8/10).
var moduleContracts = []moduleContract{
	// --- Core utilities (pure) ---
	{name: "crypto", mustExist: []string{"sha256", "sha1", "md5", "hmacSHA256", "hmacSHA1"}, smoke: `require('osm:crypto').sha256('')`, smokeWant: `"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`},
	{name: "encoding", mustExist: []string{"base64Encode", "base64Decode", "base64URLEncode", "base64URLDecode", "hexEncode", "hexDecode"}, smoke: `require('osm:encoding').hexEncode('A')`, smokeWant: `"41"`},
	{name: "json", mustExist: []string{"parse", "stringify", "query", "mergePatch", "diff", "flatten", "unflatten"}, smoke: `require('osm:json').query({a:{b:1}}, 'a.b')`, smokeWant: `1`},
	{name: "regexp", mustExist: []string{"match", "find", "findAll", "findSubmatch", "findAllSubmatch", "replace", "replaceAll", "split", "compile"}, smoke: `require('osm:regexp').match('^a+$', 'aaa')`, smokeWant: `true`},
	{name: "format", mustExist: []string{"formatNum", "formatBytes"}, smoke: `require('osm:format').formatBytes(2048)`, smokeWant: `"2.0 kB"`},
	{name: "argv", mustExist: []string{"parseArgv", "formatArgv"}, smoke: `require('osm:argv').parseArgv('echo hi').length`, smokeWant: `2`},
	{name: "flag", mustExist: []string{"newFlagSet"}, smoke: `typeof require('osm:flag').newFlagSet('x')`, smokeWant: `"object"`},
	{name: "nextIntegerID", mustExist: nil, smoke: `require('osm:nextIntegerID')([{id:1},{id:3}])`, smokeWant: `4`},
	{name: "nextIntegerId", mustExist: nil, smoke: `require('osm:nextIntegerId')([{id:1},{id:3}])`, smokeWant: `4`},
	{name: "path", asyncExports: []string{"glob"}, mustExist: []string{"join", "dir", "base", "ext", "abs", "rel", "clean", "isAbs", "match", "glob", "separator", "listSeparator"}},
	{name: "unicodetext", mustExist: []string{"width", "truncate"}, smoke: `require('osm:unicodetext').width('abc')`, smokeWant: `3`},
	{name: "text/template", mustExist: []string{"new", "execute"}, smoke: `require('osm:text/template').execute('{{.V}}', {V:'hi'})`, smokeWant: `"hi"`},
	{name: "tokenizer", asyncExports: []string{"loadFile"}, mustExist: []string{"tokenize", "count", "loadFile", "loadJSON", "loadBPE", "loadWordPiece", "loadWordLevel"}, smoke: `require('osm:tokenizer').count('hello')`, smokeWant: `5`},
	{name: "ctxutil", asyncExports: []string{"buildContext"}, mustExist: []string{"buildContext", "contextManager"}},
	{name: "protobuf", mustExist: []string{"loadDescriptorSet"}},

	// --- I/O / process modules (async exports checked for typeof; behavior in SLOW specs) ---
	{name: "os", asyncExports: []string{"readFile", "writeFile", "appendFile", "openEditor", "clipboardCopy", "clipboardPaste"}, mustExist: []string{"readFile", "writeFile", "appendFile", "fileExists", "openEditor", "clipboardCopy", "clipboardPaste", "getenv", "isAbsolute", "join", "platform"}},
	{name: "exec", asyncExports: []string{"execv"}, mustExist: []string{"execv", "spawn"}},
	{name: "fetch", asyncExports: []string{"fetch", "sseReader"}, mustExist: []string{"fetch"}},
	{name: "gitops", mustExist: []string{"isRepo", "open", "openDetect", "defaultBranch", "branchExists", "isWorkTree", "headBranchName", "ERR_NOT_REPO", "ERR_NOTHING_TO_COMMIT", "ERR_CONFLICT", "ERR_DETACHED_HEAD"}},
	{name: "grpc", mustExist: []string{"createClient", "createServer", "dial", "status", "metadata", "enableReflection", "createReflectionClient"}},
	{name: "mcp", mustExist: []string{"createServer"}},
	{name: "mcpcallback", mustExist: []string{"MCPCallback"}},
	{name: "aimux", mustExist: []string{"processProvider", "newRegistry", "newParser", "EVENT_TEXT", "EVENT_RATE_LIMIT", "EVENT_PERMISSION", "EVENT_MODEL_SELECT", "EVENT_SSO_LOGIN", "EVENT_COMPLETION", "EVENT_TOOL_USE", "EVENT_ERROR", "EVENT_THINKING"}},
	{name: "termmux", unixOnly: true, mustExist: []string{"newSessionManager", "newCaptureSession", "EXIT_TOGGLE", "EXIT_CHILD_EXIT", "EXIT_CONTEXT", "EXIT_ERROR", "SIDE_OSM", "SIDE_AGENT", "DEFAULT_TOGGLE_KEY"}},

	// --- Workflow & state ---
	{name: "sharedStateSymbols", mustExist: []string{"contextItems"}},
	{name: "bt", mustExist: []string{"success", "failure", "running", "node", "createLeafNode", "sequence", "fallback", "selector", "tick", "Blackboard"}},
	{name: "pabt", mustExist: []string{"newState", "newAction", "newPlan", "newExprCondition"}},
	{name: "lipgloss", mustExist: []string{"newStyle", "joinHorizontal", "joinVertical", "place", "width", "Left", "Center", "Right"}, smoke: `require('osm:lipgloss').width('hi')`, smokeWant: `2`},
	{name: "bubblezone", mustExist: []string{"mark", "scan", "inBounds", "get", "newPrefix", "close"}},
	{name: "bubbletea", mustExist: []string{"newModel", "run", "isTTY", "quit", "clearScreen", "batch", "sequence", "tick", "requestWindowSize", "keys", "keysByName", "mouseButtons"}},
	{name: "bubbles/textarea", mustExist: []string{"new"}, smoke: `typeof require('osm:bubbles/textarea').new()`, smokeWant: `"object"`},
	{name: "bubbles/viewport", mustExist: []string{"new"}, smoke: `typeof require('osm:bubbles/viewport').new()`, smokeWant: `"object"`},

	// --- termui/* (data-driven surface; construct + view() shape in specs) ---
	// NOTE (DRIFT-10): docs/scripting.md documents these as `.new()`, but the
	// actual constructor is the lowercase module-name function (e.g.
	// `require('osm:termui/box').box()`). The table pins CODE reality; the doc
	// drift is resolved in the docs/scripting.md update task.
	{name: "termui/scrollbar", mustExist: []string{"new"}, smoke: `typeof require('osm:termui/scrollbar').new()`, smokeWant: `"object"`},
	{name: "termui/coordinate", mustExist: []string{"position", "size", "rect", "layer", "fromLayer", "fromPaneGeometry"}},
	{name: "termui/layout", mustExist: []string{"grid", "split", "stack", "Direction"}},
	{name: "termui/termpane", mustExist: []string{"termpane"}},
	{name: "termui/label", mustExist: []string{"label"}, smoke: `typeof require('osm:termui/label').label('x')`, smokeWant: `"object"`},
	{name: "termui/divider", mustExist: []string{"divider", "Direction"}, smoke: `typeof require('osm:termui/divider').divider('horizontal')`, smokeWant: `"object"`},
	{name: "termui/box", mustExist: []string{"box"}, smoke: `typeof require('osm:termui/box').box()`, smokeWant: `"object"`},
	{name: "termui/panel", mustExist: []string{"panel"}, smoke: `typeof require('osm:termui/panel').panel()`, smokeWant: `"object"`},
	{name: "termui/list", mustExist: []string{"list"}, smoke: `typeof require('osm:termui/list').list()`, smokeWant: `"object"`},
	{name: "termui/table", mustExist: []string{"table"}, smoke: `typeof require('osm:termui/table').table()`, smokeWant: `"object"`},
	{name: "termui/splitview", mustExist: []string{"splitView", "Direction"}, smoke: `typeof require('osm:termui/splitview').splitView()`, smokeWant: `"object"`},
	{name: "termui/modal", mustExist: []string{"modal"}, smoke: `typeof require('osm:termui/modal').modal()`, smokeWant: `"object"`},
	{name: "termui/toast", mustExist: []string{"toast"}, smoke: `typeof require('osm:termui/toast').toast()`, smokeWant: `"object"`},
	{name: "termui/compositor", mustExist: []string{"compositor", "normalBorder", "renderBordered", "roundedBorder"}, smoke: `typeof require('osm:termui/compositor').compositor({width:10,height:5})`, smokeWant: `"object"`},
	{name: "termui/splitlayout", mustExist: []string{"splitLayout"}},
}

// TestModuleContract is the FAST-tier centerpiece: every documented module
// loads; headline exports exist; documented async exports are functions; pure
// modules are INVOKED with a value-smoke (closing the typeof-throws trap).
func TestModuleContract(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	for _, mc := range moduleContracts {
		t.Run(mc.name, func(t *testing.T) {
			t.Parallel()
			if mc.unixOnly && runtime.GOOS == "windows" {
				t.Skip("module requires Unix PTY")
			}
			engine, _, _ := newComplianceEngine(t, ctx)

			// 1. The module loads (require does not throw) and returns an object.
			modAny, err := evalJS(t, engine, `(function(){ var m = require('osm:`+mc.name+`'); return m; })()`, defaultEvalTimeout)
			if err != nil {
				t.Fatalf("require('osm:%s') failed: %v", mc.name, err)
				return
			}
			mod, ok := modAny.(map[string]any)
			if !ok {
				// Some modules export a bare function/value (e.g. nextIntegerID
				// default export). For those, mustExist is empty by design.
				if len(mc.mustExist) == 0 && len(mc.asyncExports) == 0 {
					return
				}
				t.Fatalf("require('osm:%s') returned %T, not an exports object", mc.name, modAny)
				return
			}

			// 2. Headline documented exports exist.
			for _, exp := range mc.mustExist {
				if _, present := mod[exp]; !present {
					t.Errorf("osm:%s: documented export %q is MISSING (drift: removed or renamed)", mc.name, exp)
				}
			}

			// 3. Documented async exports are functions (Promise-shape asserted
			//    in the behavioral specs + binding-contract test, since calling
			//    is I/O).
			for _, exp := range mc.asyncExports {
				if _, present := mod[exp]; !present {
					t.Errorf("osm:%s: async export %q is MISSING", mc.name, exp)
				}
			}
			if len(mc.asyncExports) > 0 {
				// Reliable typeof check in JS.
				exprs := make([]string, 0, len(mc.asyncExports))
				for _, e := range mc.asyncExports {
					exprs = append(exprs, e+": typeof require('osm:"+mc.name+"')."+e)
				}
				got, err := evalJS(t, engine, `(function(){ return {`+strings.Join(exprs, ",")+`}; })()`, defaultEvalTimeout)
				if err != nil {
					t.Fatalf("async typeof probe failed: %v", err)
				}
				if m, ok := got.(map[string]any); ok {
					for _, e := range mc.asyncExports {
						if typ, _ := m[e].(string); typ != "function" {
							t.Errorf("osm:%s: async export %q typeof=%q, want function", mc.name, e, typ)
						}
					}
				}
			}

			// 4. Pure-module value smoke (INVOKES a safe export — closes the
			//    typeof-passes-but-throws trap).
			if mc.smoke != "" {
				got, err := evalJS(t, engine, mc.smoke, defaultEvalTimeout)
				if err != nil {
					t.Errorf("osm:%s smoke %q threw: %v", mc.name, mc.smoke, err)
				} else if mc.smokeWant != "" {
					want, werr := evalJS(t, engine, mc.smokeWant, defaultEvalTimeout)
					if werr != nil {
						t.Fatalf("smokeWant %q failed: %v", mc.smokeWant, werr)
					}
					if !equalJS(got, want) {
						t.Errorf("osm:%s smoke %q = %v, want %v", mc.name, mc.smoke, got, want)
					}
				}
			}
		})
	}
}

// TestModuleSurface dynamically captures every module's actual export keys and
// reports drift vs the documented mustExist set: missing documented exports
// FAIL (already covered by TestModuleContract, restated here for clarity);
// undocumented actual exports are LOGGED for triage into scripting.md
// (DRIFT-7/8/10). This is the anti-circularity check: the surface is read
// from the runtime, not the doc.
func TestModuleSurface(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	for _, mc := range moduleContracts {
		t.Run(mc.name, func(t *testing.T) {
			t.Parallel()
			if mc.unixOnly && runtime.GOOS == "windows" {
				t.Skip("module requires Unix PTY")
			}
			engine, _, _ := newComplianceEngine(t, ctx)
			got, err := evalJS(t, engine, `(function(){ var m = require('osm:`+mc.name+`'); if (m && typeof m === 'object') return Object.keys(m).sort(); return null; })()`, defaultEvalTimeout)
			if err != nil {
				t.Fatalf("surface probe failed: %v", err)
			}
			keysAny, ok := got.([]any)
			if !ok {
				return // bare export (e.g. nextIntegerID function)
			}
			keys := make([]string, 0, len(keysAny))
			for _, k := range keysAny {
				if s, ok := k.(string); ok {
					keys = append(keys, s)
				}
			}
			sort.Strings(keys)
			documented := map[string]bool{}
			for _, e := range mc.mustExist {
				documented[e] = true
			}
			for _, e := range mc.asyncExports {
				documented[e] = true
			}
			var undocumented []string
			for _, k := range keys {
				if !documented[k] {
					undocumented = append(undocumented, k)
				}
			}
			if len(undocumented) > 0 {
				// Log, don't fail: undocumented extras are triaged into the doc
				// (some are intentional internals). This output feeds DRIFT
				// closure (T43) — update scripting.md or mark internal.
				t.Logf("osm:%s has undocumented exports (candidate drift for scripting.md): %v", mc.name, undocumented)
			}
		})
	}
}
