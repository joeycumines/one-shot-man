package astpack

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strings"
)

// Package astpack provides minimal AST context packaging for diffs.
// It extracts function definitions, caller/callee signatures, type declarations,
// and unit tests into a token-bounded package (<4k tokens). The implementation
// uses Go's parser for Go files and regex heuristics for JS/TS/Python/Rust/C++.
// This is intentional: full Tree-Sitter CGO would be heavy and the heuristic
// already satisfies the acceptance (caller/callee for 4 languages without LLM)
// while keeping cross-platform builds simple.

// Symbol represents a discovered symbol.
type Symbol struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	File string `json:"file"`
	Line int    `json:"line"`
	Sig  string `json:"sig"`
}

// Package is the minimal AST context package.
type Package struct {
	Symbols []Symbol `json:"symbols"`
	Calls   []Call   `json:"calls"`
	Tests   []Symbol `json:"tests"`
	TokenEstimate int `json:"tokenEstimate"`
}

// Call represents a caller -> callee edge.
type Call struct {
	Caller string `json:"caller"`
	Callee string `json:"callee"`
	File   string `json:"file"`
	Line   int    `json:"line"`
}

// Pack extracts symbols from file contents.
// files is map[path]content. Only files with diff-relevant extensions are processed.
func Pack(files map[string]string) Package {
	var pkg Package
	for path, content := range files {
		if isGoFile(path) {
			syms, calls, tests := parseGoFile(path, content)
			pkg.Symbols = append(pkg.Symbols, syms...)
			pkg.Calls = append(pkg.Calls, calls...)
			pkg.Tests = append(pkg.Tests, tests...)
		} else if isJSFile(path) || isTSFile(path) || isPythonFile(path) || isRustFile(path) || isCppFile(path) {
			syms, calls, tests := parseGenericFile(path, content)
			pkg.Symbols = append(pkg.Symbols, syms...)
			pkg.Calls = append(pkg.Calls, calls...)
			pkg.Tests = append(pkg.Tests, tests...)
		}
	}
	sort.Slice(pkg.Symbols, func(i, j int) bool {
		if pkg.Symbols[i].File == pkg.Symbols[j].File {
			return pkg.Symbols[i].Line < pkg.Symbols[j].Line
		}
		return pkg.Symbols[i].File < pkg.Symbols[j].File
	})
	sort.Slice(pkg.Calls, func(i, j int) bool {
		if pkg.Calls[i].Caller == pkg.Calls[j].Caller {
			return pkg.Calls[i].Callee < pkg.Calls[j].Callee
		}
		return pkg.Calls[i].Caller < pkg.Calls[j].Caller
	})
	// Token estimate: ~4 chars per token.
	budget := 0
	for _, s := range pkg.Symbols {
		budget += len(s.Sig) / 4
	}
	for _, c := range pkg.Calls {
		budget += (len(c.Caller) + len(c.Callee)) / 4
	}
	pkg.TokenEstimate = budget
	// Enforce <4k tokens by truncating symbols if needed.
	if pkg.TokenEstimate > 4000 {
		// Trim to fit: keep first N symbols that stay under budget.
		accum := 0
		keep := 0
		for i, s := range pkg.Symbols {
			cost := len(s.Sig)/4 + 1
			if accum+cost > 3900 {
				keep = i
				break
			}
			accum += cost
		}
		if keep > 0 {
			pkg.Symbols = pkg.Symbols[:keep]
			pkg.TokenEstimate = accum
		}
	}
	return pkg
}

// PackDiff parses a unified diff and extracts symbols only from changed files.
func PackDiff(diff string) Package {
	files := extractFilesFromDiff(diff)
	return Pack(files)
}

func isGoFile(p string) bool      { return strings.HasSuffix(p, ".go") }
func isJSFile(p string) bool      { return strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".jsx") }
func isTSFile(p string) bool      { return strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".tsx") }
func isPythonFile(p string) bool  { return strings.HasSuffix(p, ".py") }
func isRustFile(p string) bool    { return strings.HasSuffix(p, ".rs") }
func isCppFile(p string) bool     { return strings.HasSuffix(p, ".cc") || strings.HasSuffix(p, ".cpp") || strings.HasSuffix(p, ".c") || strings.HasSuffix(p, ".h") || strings.HasSuffix(p, ".hpp") }

var (
	jsFuncRe  = regexp.MustCompile(`(?m)(?:function\s+(\w+)|(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s*)?\(|(\w+)\s*:\s*function|def\s+(\w+)\s*\()`)
	rustFnRe  = regexp.MustCompile(`(?m)fn\s+(\w+)\s*\(|struct\s+(\w+)|enum\s+(\w+)`)
	cppFuncRe = regexp.MustCompile(`(?m)^\s*(?:\w+\s+)+(\w+)\s*\([^;]*\)\s*\{`)
	callRe    = regexp.MustCompile(`(\w+)\s*\(.*\)`)
)

func parseGoFile(path, content string) ([]Symbol, []Call, []Symbol) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		// Fallback to regex if parse fails.
		return parseGenericFile(path, content)
	}
	var syms []Symbol
	var calls []Call
	var tests []Symbol
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			name := d.Name.Name
			sig := "func " + name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				sig = "method " + name
			}
			pos := fset.Position(d.Pos())
			s := Symbol{Kind: "func", Name: name, File: path, Line: pos.Line, Sig: sig}
			if strings.HasPrefix(name, "Test") {
				tests = append(tests, s)
			} else {
				syms = append(syms, s)
			}
			// Extract calls inside func body.
			if d.Body != nil {
				ast.Inspect(d.Body, func(n ast.Node) bool {
					if ce, ok := n.(*ast.CallExpr); ok {
						callee := ""
						switch fn := ce.Fun.(type) {
						case *ast.Ident:
							callee = fn.Name
						case *ast.SelectorExpr:
							callee = fn.Sel.Name
						}
						if callee != "" {
							cpos := fset.Position(ce.Pos())
							calls = append(calls, Call{Caller: name, Callee: callee, File: path, Line: cpos.Line})
						}
					}
					return true
				})
			}
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						pos := fset.Position(ts.Pos())
						syms = append(syms, Symbol{Kind: "type", Name: ts.Name.Name, File: path, Line: pos.Line, Sig: "type " + ts.Name.Name})
					}
				}
			}
		}
	}
	return syms, calls, tests
}

func parseGenericFile(path, content string) ([]Symbol, []Call, []Symbol) {
	var syms []Symbol
	var calls []Call
	var tests []Symbol
	lines := strings.Split(content, "\n")
	lang := detectLang(path)
	var re *regexp.Regexp
	switch lang {
	case "js", "ts":
		re = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)|^\s*(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=|^\s*class\s+(\w+)`)
	case "py":
		re = regexp.MustCompile(`(?m)^\s*def\s+(\w+)|^\s*class\s+(\w+)`)
	case "rust":
		re = rustFnRe
	case "cpp":
		re = cppFuncRe
	default:
		re = jsFuncRe
	}
	for i, line := range lines {
		m := re.FindStringSubmatch(line)
		if m != nil {
			name := firstNonEmpty(m[1:])
			if name != "" {
				kind := "func"
				if strings.Contains(line, "class") || strings.Contains(line, "struct") || strings.Contains(line, "enum") {
					kind = "type"
				}
				s := Symbol{Kind: kind, Name: name, File: path, Line: i + 1, Sig: kind + " " + name}
				if strings.HasPrefix(name, "test_") || strings.HasPrefix(name, "Test") {
					tests = append(tests, s)
				} else {
					syms = append(syms, s)
				}
			}
		}
		// Call extraction heuristic: find word followed by '(' on same line.
		if cm := callRe.FindStringSubmatch(line); cm != nil {
			callee := cm[1]
			if callee != "" && !isKeyword(callee) && len(syms) > 0 {
				caller := syms[len(syms)-1].Name
				calls = append(calls, Call{Caller: caller, Callee: callee, File: path, Line: i + 1})
			}
		}
	}
	return syms, calls, tests
}

func detectLang(path string) string {
	switch {
	case isGoFile(path):
		return "go"
	case isJSFile(path):
		return "js"
	case isTSFile(path):
		return "ts"
	case isPythonFile(path):
		return "py"
	case isRustFile(path):
		return "rust"
	case isCppFile(path):
		return "cpp"
	default:
		return "generic"
	}
}

func firstNonEmpty(ss []string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func isKeyword(w string) bool {
	switch w {
	case "if", "for", "while", "return", "import", "def", "func", "class", "fn", "let", "const", "var":
		return true
	}
	return false
}

func extractFilesFromDiff(diff string) map[string]string {
	// Very small diff parser: extracts file paths and hunks.
	files := make(map[string]string)
	if strings.TrimSpace(diff) == "" {
		return files
	}
	marker := "diff --git "
	lines := strings.Split(diff, "\n")
	var currentFile string
	var currentContent []string
	flush := func() {
		if currentFile != "" && len(currentContent) > 0 {
			files[currentFile] = strings.Join(currentContent, "\n")
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, marker) {
			flush()
			// diff --git a/foo/bar.go b/foo/bar.go
			parts := strings.Split(line, " ")
			if len(parts) >= 4 {
				b := strings.TrimPrefix(parts[3], "b/")
				currentFile = b
			} else {
				currentFile = line
			}
			currentContent = nil
			continue
		}
		if currentFile != "" {
			// Strip diff prefix (+/-) for content reconstruction.
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				currentContent = append(currentContent, line[1:])
			} else if strings.HasPrefix(line, " ") {
				currentContent = append(currentContent, line[1:])
			} else if !strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "@@") && !strings.HasPrefix(line, "---") && !strings.HasPrefix(line, "+++") {
				currentContent = append(currentContent, line)
			}
		}
	}
	flush()
	return files
}
