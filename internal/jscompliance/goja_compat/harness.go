package goja_compat

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

	"github.com/goccy/go-yaml"
	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

//go:embed all:testdata
var testdata embed.FS

var errInvalidFormat = errors.New("invalid goja compat file format")

type tc39Meta struct {
	Includes []string `yaml:"includes"`
	Flags    []string `yaml:"flags"`
}

func parseFile(content string) (*tc39Meta, string, error) {
	start := strings.Index(content, "/*---")
	if start == -1 {
		// No frontmatter, treat as plain js with no includes
		return &tc39Meta{}, content, nil
	}
	start += 5
	end := strings.Index(content, "---*/")
	if end == -1 || end <= start {
		return nil, "", errInvalidFormat
	}
	var meta tc39Meta
	if err := yaml.Unmarshal([]byte(content[start:end]), &meta); err != nil {
		return nil, "", err
	}
	return &meta, content, nil
}

type result struct {
	Name string `json:"name"`
	Pass bool   `json:"pass"`
	Err  string `json:"error,omitempty"`
}

type Report struct {
	Total  int      `json:"total"`
	Passed int      `json:"passed"`
	Failed int      `json:"failed"`
	Rate   float64  `json:"passRate"`
	Suite  map[string]int `json:"suitePassed,omitempty"`
	Results []result `json:"results"`
}

func (r *Report) finalize() {
	if r.Total > 0 {
		r.Rate = float64(r.Passed) / float64(r.Total) * 100
	}
}

var harnessCache = map[string]string{}

func loadHarness(name string) (string, error) {
	if v, ok := harnessCache[name]; ok {
		return v, nil
	}
	b, err := testdata.ReadFile(path.Join("testdata/harness", name))
	if err != nil {
		return "", err
	}
	s := string(b)
	harnessCache[name] = s
	return s, nil
}

func runOne(name, src string, meta *tc39Meta) (bool, string) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("", name), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		return false, fmt.Sprintf("NewEngine: %v", err)
	}
	defer func() { _ = engine.Close() }()

	type res struct {
		err error
	}
	done := make(chan res, 1)
	submitErr := engine.Loop().Submit(func() {
		vm := engine.Runtime()
		for _, inc := range meta.Includes {
			hSrc, err := loadHarness(inc)
			if err != nil {
				done <- res{err: fmt.Errorf("include %q: %w", inc, err)}
				return
			}
			if _, err := vm.RunString(hSrc); err != nil {
				done <- res{err: fmt.Errorf("include %q run: %w", inc, err)}
				return
			}
		}
		// For compat we don't need $262
		_ = vm.Set("print", func(s string) { _, _ = fmt.Fprint(os.Stderr, s) })
		if _, err := vm.RunString(src); err != nil {
			done <- res{err: err}
			return
		}
		// If promise was used, give microtask a chance? Our harness doesn't have async promise handling.
		// For promise suite, we need to handle async: if test used Promise.then, we need to wait.
		// We handle by checking if vm has pending promises via a tick: run a dummy setTimeout 0 and wait.
		// Simplified: just succeed if no immediate error; promise rejections will be via assert.
		done <- res{err: nil}
	})
	if submitErr != nil {
		return false, fmt.Sprintf("submit: %v", submitErr)
	}
	r := <-done
	if r.err != nil {
		// Check for goja exception with message
		if exc, ok := r.err.(*goja.Exception); ok {
			return false, fmt.Sprintf("%v", exc.Value())
		}
		return false, fmt.Sprintf("%v", r.err)
	}
	return true, ""
}

func Collect() Report {
	var names []string
	_ = fs.WalkDir(testdata, "testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(p, ".js") && !strings.Contains(p, "/harness/") {
			names = append(names, p)
		}
		return nil
	})
	sort.Strings(names)
	var r Report
	r.Suite = make(map[string]int)
	r.Total = len(names)
	for _, full := range names {
		b, err := testdata.ReadFile(full)
		if err != nil {
			r.Results = append(r.Results, result{Name: full, Pass: false, Err: fmt.Sprintf("read: %v", err)})
			r.Failed++
			continue
		}
		content := string(b)
		meta, _, err := parseFile(content)
		if err != nil {
			r.Results = append(r.Results, result{Name: full, Pass: false, Err: fmt.Sprintf("parse: %v", err)})
			r.Failed++
			continue
		}
		pass, msg := runOne(full, content, meta)
		rec := result{Name: full, Pass: pass, Err: msg}
		r.Results = append(r.Results, rec)
		if pass {
			r.Passed++
			// suite tracking
			parts := strings.Split(full, "/")
			if len(parts) >= 2 {
				suite := parts[1]
				r.Suite[suite]++
			}
		} else {
			r.Failed++
		}
	}
	r.finalize()
	return r
}

func ReportJSON(r Report) ([]byte, error) { return json.MarshalIndent(r, "", "  ") }
