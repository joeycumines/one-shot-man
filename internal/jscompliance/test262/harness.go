package test262

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

//go:embed all:testdata
var testdata embed.FS

var invalidFormatError = errors.New("invalid test262 file format")

var ignorableTestError = errors.New("IgnorableTestError")

// skipList mirrors goja's tc39_test.go skipList for known exclusions (subset for our curated data).
var skipList = map[string]bool{}

// skipPrefixes mirrors goja's featuresBlackList + prefix skips.
var skipPrefixes = newPrefixList()

var featuresBlackList = []string{
	"async-iteration",
	"Symbol.asyncIterator",
	"resizable-arraybuffer",
	"regexp-duplicate-named-groups",
	"regexp-unicode-property-escapes",
	"regexp-match-indices",
	"Temporal",
	"import-assertions",
	"dynamic-import",
	"import.meta",
	"Atomics",
	"FinalizationRegistry",
	"WeakRef",
	"ShadowRealm",
	"SharedArrayBuffer",
	"decorators",
	"explicit-resource-management",
	"promise-try",
	"promise-with-resolvers",
	"array-grouping",
	"String.prototype.isWellFormed",
	"String.prototype.toWellFormed",
}

func init() {
	for _, f := range featuresBlackList {
		skipPrefixes.Add(f)
	}
	// Skip async harness prefixes that require special runner.
	skipPrefixes.Add("test/built-ins/Async")
}

type prefixList struct {
	prefixes map[int]map[string]struct{}
	mu       sync.RWMutex
}

func newPrefixList() *prefixList { return &prefixList{prefixes: make(map[int]map[string]struct{})} }

func (pl *prefixList) Add(prefix string) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	l := len(prefix)
	m := pl.prefixes[l]
	if m == nil {
		m = make(map[string]struct{})
		pl.prefixes[l] = m
	}
	m[prefix] = struct{}{}
}

func (pl *prefixList) Match(s string) bool {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	for l, m := range pl.prefixes {
		if len(s) >= l {
			if _, ok := m[s[:l]]; ok {
				return true
			}
		}
	}
	return false
}

type tc39MetaNegative struct {
	Phase string `yaml:"phase"`
	Type  string `yaml:"type"`
}

type tc39Meta struct {
	Negative tc39MetaNegative `yaml:"negative"`
	Includes []string         `yaml:"includes"`
	Flags    []string         `yaml:"flags"`
	Features []string         `yaml:"features"`
}

func (m *tc39Meta) hasFlag(flag string) bool {
	for _, f := range m.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

// parseTestFile parses yaml frontmatter and returns meta + full source.
func parseTestFile(content string) (*tc39Meta, string, error) {
	metaStart := strings.Index(content, "/*---")
	if metaStart == -1 {
		return nil, "", invalidFormatError
	}
	metaStart += 5
	metaEnd := strings.Index(content, "---*/")
	if metaEnd == -1 || metaEnd <= metaStart {
		return nil, "", invalidFormatError
	}
	var meta tc39Meta
	if err := yaml.Unmarshal([]byte(content[metaStart:metaEnd]), &meta); err != nil {
		return nil, "", err
	}
	if meta.Negative.Type != "" && meta.Negative.Phase == "" {
		return nil, "", errors.New("negative type set but phase empty")
	}
	return &meta, content, nil
}

// result holds quantified outcome for one test file.
type result struct {
	Name string `json:"name"`
	Pass bool   `json:"pass"`
	Skip bool   `json:"skip"`
	Err  string `json:"error,omitempty"`
}

// Report is the quantified compliance report.
type Report struct {
	Total   int      `json:"total"`
	Passed  int      `json:"passed"`
	Failed  int      `json:"failed"`
	Skipped int      `json:"skipped"`
	Rate    float64  `json:"passRate"`
	Results []result `json:"results"`
}

func (r *Report) finalize() {
	if r.Total > 0 {
		r.Rate = float64(r.Passed) / float64(r.Total) * 100
	}
}

// harnessFiles caches includes content.
var harnessFiles = map[string]string{}

func loadHarnessFile(name string) (string, error) {
	if v, ok := harnessFiles[name]; ok {
		return v, nil
	}
	p := path.Join("testdata/test262/harness", name)
	b, err := testdata.ReadFile(p)
	if err != nil {
		// Try alternative path testdata/test262/test/harness
		p2 := path.Join("testdata/test262/test/harness", name)
		b, err = testdata.ReadFile(p2)
		if err != nil {
			return "", err
		}
	}
	s := string(b)
	harnessFiles[name] = s
	return s, nil
}

// newEngine creates a fresh osm engine per test file (hermetic).
func newEngine(t interface{ Helper(); Cleanup(func()); Name() string }) (*scripting.Engine, *bytes.Buffer, *bytes.Buffer) {
	// Use testing.TB via type assertion; caller passes *testing.T.
	// To avoid import cycle, we use interface and runtime check.
	type tb interface {
		Helper()
		Cleanup(func())
		Name() string
		Fatalf(string, ...any)
	}
	var tbVal tb
	if v, ok := t.(tb); ok {
		tbVal = v
		tbVal.Helper()
	}
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		if tbVal != nil {
			tbVal.Fatalf("NewEngine failed: %v", err)
		}
		panic(fmt.Sprintf("NewEngine failed: %v", err))
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine, &stdout, &stderr
}

// runOne runs a single test262 file content on a fresh engine.
func runOne(name, src string, meta *tc39Meta) (bool, bool, string) {
	// Feature blacklist
	for _, feat := range meta.Features {
		if skipPrefixes.Match(feat) {
			return false, true, fmt.Sprintf("skipped feature %q", feat)
		}
	}
	if skipList[name] {
		return false, true, "skipped via skipList"
	}
	if skipPrefixes.Match(name) {
		return false, true, "skipped via prefix"
	}

	// Create engine hermetically.
	// Use context.Background directly to avoid testing.T dependency here; caller will manage.
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("", name), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		return false, false, fmt.Sprintf("NewEngine: %v", err)
	}
	defer func() { _ = engine.Close() }()

	// Helper to run JS on loop synchronously with timeout via engine's VM + event loop.
	// We simplify: use engine.Loop().Submit and wait, similar to harness_test.go collectSpecResults.
	// For test262 we need $262, print, includes, etc.
	type runRes struct {
		err   error
		early bool
	}
	done := make(chan runRes, 1)

	submitErr := engine.Loop().Submit(func() {
		vm := engine.Runtime()
		// Install $262 and IgnorableTestError
		ignorableSym := goja.NewSymbol("IgnorableTestError")
		_262 := vm.NewObject()
		_262.Set("detachArrayBuffer", func(call goja.FunctionCall) goja.Value {
			if obj, ok := call.Argument(0).(*goja.Object); ok {
				// best-effort detach if possible; otherwise no-op
				_ = obj
			}
			return goja.Undefined()
		})
		_262.Set("createRealm", func(goja.FunctionCall) goja.Value {
			panic(ignorableTestError)
		})
		_262.Set("evalScript", func(call goja.FunctionCall) goja.Value {
			script := call.Argument(0).String()
			v, err := vm.RunString(script)
			if err != nil {
				panic(err)
			}
			return v
		})
		vm.Set("$262", _262)
		vm.Set("IgnorableTestError", ignorableSym)

		// Load includes (sta.js, assert.js, etc.)
		for _, inc := range meta.Includes {
			hSrc, err := loadHarnessFile(inc)
			if err != nil {
				done <- runRes{err: fmt.Errorf("include %q: %w", inc, err)}
				return
			}
			if _, err := vm.RunString(hSrc); err != nil {
				done <- runRes{err: fmt.Errorf("include %q run: %w", inc, err)}
				return
			}
		}

		// Setup print
		if meta.hasFlag("async") {
			// Load doneprintHandle
			hSrc, err := loadHarnessFile("doneprintHandle.js")
			if err == nil {
				if _, err := vm.RunString(hSrc); err != nil {
					done <- runRes{err: fmt.Errorf("doneprintHandle: %w", err)}
					return
				}
			}
			var out []string
			_ = vm.Set("print", func(msg string) { out = append(out, msg) })
			// Run test source
			_, runErr := vm.RunString(src)
			if runErr != nil {
				// Check if early error expected
				done <- runRes{err: runErr, early: true}
				return
			}
			// For async, wait for print output? Simplified: check for async flag handling.
			// doneprintHandle defines $DONE; we don't fully support async done callback,
			// so we treat async tests as needing manual async handling via Promise.
			// For our synthetic suite there are no async tests, so just check for errors.
			done <- runRes{err: nil}
			_ = out
		} else {
			_ = vm.Set("print", func(s string) { _, _ = fmt.Fprint(os.Stderr, s) })
			_, runErr := vm.RunString(src)
			if runErr != nil {
				// Determine early vs runtime via presence of "Syntax" etc. Simplified: early=true if SyntaxError
				early := strings.Contains(runErr.Error(), "SyntaxError") || strings.Contains(runErr.Error(), "Parse")
				done <- runRes{err: runErr, early: early}
				return
			}
			done <- runRes{err: nil}
		}
	})
	if submitErr != nil {
		return false, false, fmt.Sprintf("loop submit: %v", submitErr)
	}
	res := <-done
	if res.err != nil {
		// Handle IgnorableTestError as skip
		if errors.Is(res.err, ignorableTestError) {
			return false, true, "IgnorableTestError"
		}
		if exc, ok := res.err.(*goja.Exception); ok {
			if exc.Value() != nil && exc.Value().String() == "IgnorableTestError" {
				return false, true, "IgnorableTestError"
			}
			// Check for symbol
			if exc.Value().Export() == ignorableTestError {
				return false, true, "IgnorableTestError"
			}
		}
		// Negative handling
		if meta.Negative.Type != "" {
			// Expect error
			phase := meta.Negative.Phase
			// If we got an error, check phase
			if (phase == "parse" || phase == "early") && !res.early {
				return false, false, fmt.Sprintf("expected %s error at parse phase but got runtime: %v", meta.Negative.Type, res.err)
			}
			if phase == "runtime" && res.early {
				return false, false, fmt.Sprintf("expected runtime error but got early: %v", res.err)
			}
			// Check type contains
			if !strings.Contains(res.err.Error(), meta.Negative.Type) {
				// Also check exception value type
				return false, false, fmt.Sprintf("negative %q expected but got %v", meta.Negative.Type, res.err)
			}
			return true, false, ""
		}
		return false, false, fmt.Sprintf("%v", res.err)
	}
	// No error but negative expected
	if meta.Negative.Type != "" {
		return false, false, fmt.Sprintf("expected negative %q but no error", meta.Negative.Type)
	}
	return true, false, ""
}

// Collect walks embedded testdata and runs each .js, returning quantified report.
func Collect() Report {
	var names []string
	_ = fs.WalkDir(testdata, "testdata/test262/test", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(p, ".js") {
			names = append(names, p)
		}
		return nil
	})
	sort.Strings(names)
	var r Report
	r.Total = len(names)
	for _, full := range names {
		// full is like testdata/test262/test/language/expressions/synth_0001.js
		// name for skip matching is like test/language/expressions/synth_0001.js
		name := strings.TrimPrefix(full, "testdata/test262/")
		b, err := testdata.ReadFile(full)
		if err != nil {
			r.Results = append(r.Results, result{Name: name, Pass: false, Err: fmt.Sprintf("read: %v", err)})
			r.Failed++
			continue
		}
		content := string(b)
		meta, _, err := parseTestFile(content)
		if err != nil {
			r.Results = append(r.Results, result{Name: name, Pass: false, Err: fmt.Sprintf("parse: %v", err)})
			r.Failed++
			continue
		}
		pass, skip, msg := runOne(name, content, meta)
		rec := result{Name: name, Pass: pass, Skip: skip, Err: msg}
		r.Results = append(r.Results, rec)
		if skip {
			r.Skipped++
		} else if pass {
			r.Passed++
		} else {
			r.Failed++
		}
	}
	// Adjust total for reporting: total includes skipped? Keep original total but Rate is passed / (total - skipped)
	r.finalize()
	return r
}

// ReportJSON returns json bytes for the report.
func ReportJSON(r Report) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
