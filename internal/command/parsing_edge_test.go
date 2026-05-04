package command

import (
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────
// stripBOM — direct unit tests
// ─────────────────────────────────────────────────────────────────────

func TestStripBOM_NoBOM(t *testing.T) {
	t.Parallel()
	input := []byte("hello world")
	got := stripBOM(input)
	if string(got) != "hello world" {
		t.Fatalf("expected %q, got %q", "hello world", string(got))
	}
}

func TestStripBOM_WithBOM(t *testing.T) {
	t.Parallel()
	bom := []byte{0xEF, 0xBB, 0xBF}
	input := append(bom, []byte("content")...)
	got := stripBOM(input)
	if string(got) != "content" {
		t.Fatalf("expected %q, got %q", "content", string(got))
	}
}

func TestStripBOM_BOMOnly(t *testing.T) {
	t.Parallel()
	bom := []byte{0xEF, 0xBB, 0xBF}
	got := stripBOM(bom)
	if len(got) != 0 {
		t.Fatalf("expected empty result for BOM-only input, got %q", string(got))
	}
}

func TestStripBOM_Empty(t *testing.T) {
	t.Parallel()
	got := stripBOM([]byte{})
	if len(got) != 0 {
		t.Fatalf("expected empty result for empty input, got %q", string(got))
	}
}

func TestStripBOM_PartialBOMPrefix(t *testing.T) {
	t.Parallel()
	// Only first two BOM bytes — should NOT be stripped.
	input := []byte{0xEF, 0xBB, 'h', 'i'}
	got := stripBOM(input)
	if len(got) != 4 {
		t.Fatalf("expected 4 bytes (partial BOM not stripped), got %d: %q", len(got), string(got))
	}
}

// ─────────────────────────────────────────────────────────────────────
// unquoteYAMLString — direct unit tests
// ─────────────────────────────────────────────────────────────────────

func TestUnquoteYAMLString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"double_quoted", `"hello"`, "hello"},
		{"single_quoted", "'hello'", "hello"},
		{"unquoted", "hello", "hello"},
		{"empty_string", "", ""},
		{"single_char", "x", "x"},
		{"mismatched_quotes_dq_sq", `"hello'`, `"hello'`},
		{"mismatched_quotes_sq_dq", `'hello"`, `'hello"`},
		{"empty_double_quoted", `""`, ""},
		{"empty_single_quoted", "''", ""},
		{"only_one_quote", `"`, `"`},
		{"nested_quotes", `"it's fine"`, "it's fine"},
		{"spaces_preserved", `" spaced "`, " spaced "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := unquoteYAMLString(tc.input)
			if got != tc.want {
				t.Fatalf("unquoteYAMLString(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────
// parseInlineYAMLList — direct unit tests
// ─────────────────────────────────────────────────────────────────────

func TestParseInlineYAMLList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"bracketed_list", "[a, b, c]", []string{"a", "b", "c"}},
		{"single_item_no_brackets", "codebase", []string{"codebase"}},
		{"empty_brackets", "[]", nil},
		{"quoted_items", `["foo", 'bar']`, []string{"foo", "bar"}},
		{"spaces_in_list", "[ one , two , three ]", []string{"one", "two", "three"}},
		{"empty_string", "", nil},
		{"single_item_quoted_no_brackets", `"terminal"`, []string{"terminal"}},
		{"single_bracket_only_open", "[incomplete", []string{"[incomplete"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseInlineYAMLList(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("parseInlineYAMLList(%q) len = %d, want %d; got %v",
					tc.input, len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("parseInlineYAMLList(%q)[%d] = %q, want %q",
						tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────
// parseSimpleYAML — direct unit tests for edge cases not covered
// by ParsePromptFile integration tests
// ─────────────────────────────────────────────────────────────────────

func TestParseSimpleYAML_BlankLineResetsMultiLineList(t *testing.T) {
	t.Parallel()
	// A blank line between "tools:" header and "- item" should reset
	// currentListKey, so subsequent "- item" lines are NOT appended.
	raw := "tools:\n\n  - orphaned"
	pf := &PromptFile{}
	if err := parseSimpleYAML(raw, pf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pf.Tools) != 0 {
		t.Fatalf("expected empty tools (blank line resets list), got %v", pf.Tools)
	}
}

func TestParseSimpleYAML_LineWithoutColon(t *testing.T) {
	t.Parallel()
	// A line without a colon in frontmatter should be silently skipped.
	raw := "name: test\nthis line has no colon\ndescription: ok"
	pf := &PromptFile{}
	if err := parseSimpleYAML(raw, pf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "test" {
		t.Errorf("expected name %q, got %q", "test", pf.Name)
	}
	if pf.Description != "ok" {
		t.Errorf("expected description %q, got %q", "ok", pf.Description)
	}
	// Verify other fields remain zero.
	if pf.Model != "" || pf.Mode != "" || len(pf.Tools) != 0 {
		t.Errorf("expected untouched fields to remain zero, got model=%q mode=%q tools=%v", pf.Model, pf.Mode, pf.Tools)
	}
}

func TestParseSimpleYAML_UnknownKeySilentlyIgnored(t *testing.T) {
	t.Parallel()
	// The default arm of the switch should silently ignore unknown keys.
	raw := "name: test\nunknown_key: some-value\nfuture-field: data\ndescription: ok"
	pf := &PromptFile{}
	if err := parseSimpleYAML(raw, pf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "test" {
		t.Errorf("expected name %q, got %q", "test", pf.Name)
	}
	if pf.Description != "ok" {
		t.Errorf("expected description %q, got %q", "ok", pf.Description)
	}
	if pf.Model != "" || pf.Mode != "" || len(pf.Tools) != 0 {
		t.Errorf("unknown keys should not set any fields, got model=%q mode=%q tools=%v", pf.Model, pf.Mode, pf.Tools)
	}
}

func TestParseSimpleYAML_InlineToolsList(t *testing.T) {
	t.Parallel()
	// Exercises the inline tools path through the switch (tools with non-empty value).
	raw := "tools: [codebase, terminal, githubRepo]"
	pf := &PromptFile{}
	if err := parseSimpleYAML(raw, pf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"codebase", "terminal", "githubRepo"}
	if len(pf.Tools) != len(want) {
		t.Fatalf("expected %d tools, got %d: %v", len(want), len(pf.Tools), pf.Tools)
	}
	for i, w := range want {
		if pf.Tools[i] != w {
			t.Errorf("tools[%d] = %q, want %q", i, pf.Tools[i], w)
		}
	}
}

func TestParseSimpleYAML_EmptyInput(t *testing.T) {
	t.Parallel()
	// Pre-set a field to verify empty input doesn't clobber existing state.
	pf := &PromptFile{Name: "pre-existing"}
	if err := parseSimpleYAML("", pf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "pre-existing" {
		t.Fatalf("empty parse clobbered existing Name: got %q", pf.Name)
	}
	if pf.Description != "" || pf.Model != "" || pf.Mode != "" || len(pf.Tools) != 0 {
		t.Fatalf("expected untouched fields to remain zero, got %+v", pf)
	}
}

func TestParseSimpleYAML_CommentResetsMultiLineList(t *testing.T) {
	t.Parallel()
	// A comment between "tools:" and "- item" should reset currentListKey.
	raw := "tools:\n# comment\n  - orphaned"
	pf := &PromptFile{}
	if err := parseSimpleYAML(raw, pf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pf.Tools) != 0 {
		t.Fatalf("expected empty tools (comment resets list), got %v", pf.Tools)
	}
}

func TestParseSimpleYAML_CRLFLineEndings(t *testing.T) {
	t.Parallel()
	raw := "name: crlf-test\r\ndescription: windows\r\nmodel: gpt-4\r\n"
	pf := &PromptFile{}
	if err := parseSimpleYAML(raw, pf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "crlf-test" {
		t.Errorf("expected name %q, got %q", "crlf-test", pf.Name)
	}
	if pf.Description != "windows" {
		t.Errorf("expected description %q, got %q", "windows", pf.Description)
	}
	if pf.Model != "gpt-4" {
		t.Errorf("expected model %q, got %q", "gpt-4", pf.Model)
	}
	// Verify unset fields remain zero (CRLF handling must not inject garbage).
	if pf.Mode != "" {
		t.Errorf("expected empty mode, got %q", pf.Mode)
	}
	if len(pf.Tools) != 0 {
		t.Errorf("expected empty tools, got %v", pf.Tools)
	}
}

func TestParseSimpleYAML_MultiLineListWithQuotedItems(t *testing.T) {
	t.Parallel()
	raw := "tools:\n  - \"codebase\"\n  - 'terminal'\n  - plain"
	pf := &PromptFile{}
	if err := parseSimpleYAML(raw, pf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"codebase", "terminal", "plain"}
	if len(pf.Tools) != len(want) {
		t.Fatalf("expected %d tools, got %d: %v", len(want), len(pf.Tools), pf.Tools)
	}
	for i, w := range want {
		if pf.Tools[i] != w {
			t.Errorf("tools[%d] = %q, want %q", i, pf.Tools[i], w)
		}
	}
}

func TestParseSimpleYAML_AllFrontmatterFields(t *testing.T) {
	t.Parallel()
	raw := "name: full\ndescription: all fields\nmodel: claude-3\nmode: agent\ntools: [codebase, terminal]"
	pf := &PromptFile{}
	if err := parseSimpleYAML(raw, pf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "full" {
		t.Errorf("name = %q, want %q", pf.Name, "full")
	}
	if pf.Description != "all fields" {
		t.Errorf("description = %q, want %q", pf.Description, "all fields")
	}
	if pf.Model != "claude-3" {
		t.Errorf("model = %q, want %q", pf.Model, "claude-3")
	}
	if pf.Mode != "agent" {
		t.Errorf("mode = %q, want %q", pf.Mode, "agent")
	}
	if len(pf.Tools) != 2 || pf.Tools[0] != "codebase" || pf.Tools[1] != "terminal" {
		t.Errorf("tools = %v, want [codebase, terminal]", pf.Tools)
	}
}

// ─────────────────────────────────────────────────────────────────────
// validateGoal — direct unit tests
// ─────────────────────────────────────────────────────────────────────

func TestValidateGoal_Valid(t *testing.T) {
	t.Parallel()
	g := &Goal{Name: "code-review", Description: "Review code"}
	if err := validateGoal(g); err != nil {
		t.Fatalf("expected nil error for valid goal, got: %v", err)
	}
}

func TestValidateGoal_EmptyName(t *testing.T) {
	t.Parallel()
	g := &Goal{Name: "", Description: "Has description"}
	err := validateGoal(g)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "Name is required") {
		t.Fatalf("expected 'Name is required' error, got: %v", err)
	}
}

func TestValidateGoal_InvalidName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		goal string
	}{
		{"spaces", "has spaces"},
		{"special_chars", "name@special"},
		{"dots", "name.with.dots"},
		{"underscores", "name_with_underscores"},
		{"leading_hyphen", "-starts-with-dash"},
		{"slash_in_name", "with/slash"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := &Goal{Name: tc.goal, Description: "Test"}
			err := validateGoal(g)
			if err == nil {
				t.Fatalf("expected error for invalid name %q", tc.goal)
			}
			if !strings.Contains(err.Error(), "alphanumeric") {
				t.Fatalf("expected alphanumeric error, got: %v", err)
			}
		})
	}
}

func TestValidateGoal_EmptyDescription(t *testing.T) {
	t.Parallel()
	g := &Goal{Name: "valid-name", Description: ""}
	err := validateGoal(g)
	if err == nil {
		t.Fatal("expected error for empty description")
	}
	if !strings.Contains(err.Error(), "Description is required") {
		t.Fatalf("expected 'Description is required' error, got: %v", err)
	}
}

func TestValidateGoal_ValidNamesAccepted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
	}{
		{"simple"},
		{"code-review"},
		{"my-goal-123"},
		{"a"},
		{"UPPERCASE"},
		{"MiXeD-CaSe"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := &Goal{Name: tc.name, Description: "Test description"}
			if err := validateGoal(g); err != nil {
				t.Fatalf("expected nil error for valid name %q, got: %v", tc.name, err)
			}
		})
	}
}
