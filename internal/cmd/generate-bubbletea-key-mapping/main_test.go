package main

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

// ── generateKeysOutput ─────────────────────────────────────────────

func TestGenerateKeysOutput_EmptyMaps(t *testing.T) {
	out, err := generateKeysOutput(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "DO NOT EDIT") {
		t.Error("missing DO NOT EDIT header")
	}
	if !strings.Contains(s, "KeyDefs") {
		t.Error("missing KeyDefs variable")
	}
}

func TestGenerateKeysOutput_SingleKey(t *testing.T) {
	keyNames := map[string]string{
		"keySpace": "space",
	}
	aliases := map[string]string{
		"KeySpace": "keySpace",
	}
	out, err := generateKeysOutput(keyNames, aliases)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"space"`) {
		t.Error("missing 'space' string value")
	}
	if !strings.Contains(s, "KeySpace") {
		t.Error("missing KeySpace constant name")
	}
}

func TestGenerateKeysOutput_MultipleAliases(t *testing.T) {
	keyNames := map[string]string{
		"keyCR": "enter",
	}
	aliases := map[string]string{
		"KeyEnter": "keyCR",
		"KeyCtrlM": "keyCR",
	}
	out, err := generateKeysOutput(keyNames, aliases)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"KeyEnter"`) {
		t.Error("missing KeyEnter in output")
	}
	if !strings.Contains(s, `"KeyCtrlM"`) {
		t.Error("missing KeyCtrlM in output")
	}
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if strings.Contains(line, `"enter":`) && strings.Contains(line, "KeyDef") {
			if !strings.Contains(line, `Name: "KeyEnter"`) {
				t.Errorf("enter should use canonical KeyEnter, got: %s", line)
			}
		}
	}
}

func TestGenerateKeysOutput_ExportedWithoutAlias(t *testing.T) {
	keyNames := map[string]string{
		"KeyRunes": "runes",
	}
	aliases := map[string]string{}
	out, err := generateKeysOutput(keyNames, aliases)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "KeyRunes") {
		t.Error("missing KeyRunes")
	}
}

func TestGenerateKeysOutput_UnexportedWithoutAlias_Excluded(t *testing.T) {
	keyNames := map[string]string{
		"keyPrivate": "private",
	}
	aliases := map[string]string{}
	out, err := generateKeysOutput(keyNames, aliases)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "keyPrivate") {
		t.Error("unexported key without alias should not appear")
	}
	if strings.Contains(s, `"private"`) {
		t.Error("unexported key string value should not appear")
	}
}

func TestGenerateKeysOutput_Deterministic(t *testing.T) {
	keyNames := map[string]string{
		"keyA": "aaa",
		"keyB": "bbb",
		"keyC": "ccc",
	}
	aliases := map[string]string{
		"KeyA": "keyA",
		"KeyB": "keyB",
		"KeyC": "keyC",
	}
	out1, err := generateKeysOutput(keyNames, aliases)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	out2, err := generateKeysOutput(keyNames, aliases)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if string(out1) != string(out2) {
		t.Error("output is not deterministic across multiple calls")
	}
}

// ── generateMouseOutput ────────────────────────────────────────────

func TestGenerateMouseOutput_EmptyMaps(t *testing.T) {
	out, err := generateMouseOutput(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "DO NOT EDIT") {
		t.Error("missing DO NOT EDIT header")
	}
	if !strings.Contains(s, "MouseButtonDefs") {
		t.Error("missing MouseButtonDefs variable")
	}
	if !strings.Contains(s, "MouseActionDefs") {
		t.Error("missing MouseActionDefs variable")
	}
}

func TestGenerateMouseOutput_ButtonAndAction(t *testing.T) {
	buttons := map[string]string{
		"MouseButtonLeft": "left",
	}
	actions := map[string]string{
		"MouseActionPress": "press",
	}
	out, err := generateMouseOutput(buttons, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "MouseButtonLeft") {
		t.Error("missing MouseButtonLeft")
	}
	if !strings.Contains(s, `"left"`) {
		t.Error("missing 'left' string value")
	}
	if !strings.Contains(s, "MouseActionPress") {
		t.Error("missing MouseActionPress")
	}
	if !strings.Contains(s, `"press"`) {
		t.Error("missing 'press' string value")
	}
}

func TestGenerateMouseOutput_Deterministic(t *testing.T) {
	buttons := map[string]string{
		"MouseButtonLeft":  "left",
		"MouseButtonRight": "right",
	}
	actions := map[string]string{
		"MouseActionPress":   "press",
		"MouseActionRelease": "release",
	}
	out1, err := generateMouseOutput(buttons, actions)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	out2, err := generateMouseOutput(buttons, actions)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if string(out1) != string(out2) {
		t.Error("mouse output is not deterministic")
	}
}

// ── extractMouseMap ────────────────────────────────────────────────

func TestExtractMouseMap_CompositeLit(t *testing.T) {
	expr := &ast.CompositeLit{
		Elts: []ast.Expr{
			&ast.KeyValueExpr{
				Key:   &ast.Ident{Name: "MouseButtonLeft"},
				Value: &ast.BasicLit{Kind: token.STRING, Value: `"left"`},
			},
		},
	}
	result := make(map[string]string)
	extractMouseMap(expr, result, "MouseButton")
	if v, ok := result["MouseButtonLeft"]; !ok || v != "left" {
		t.Errorf("got %v, want MouseButtonLeft -> left", result)
	}
}

func TestExtractMouseMap_NotCompositeLit(t *testing.T) {
	result := make(map[string]string)
	extractMouseMap(&ast.Ident{Name: "foo"}, result, "")
	if len(result) != 0 {
		t.Errorf("expected empty result for non-composite-lit, got %v", result)
	}
}

func TestExtractMouseMap_NonStringValue(t *testing.T) {
	expr := &ast.CompositeLit{
		Elts: []ast.Expr{
			&ast.KeyValueExpr{
				Key:   &ast.Ident{Name: "SomeButton"},
				Value: &ast.BasicLit{Kind: token.INT, Value: "42"},
			},
		},
	}
	result := make(map[string]string)
	extractMouseMap(expr, result, "")
	if len(result) != 0 {
		t.Errorf("expected empty result for non-string value, got %v", result)
	}
}

// ── extractKeyNamesMap ─────────────────────────────────────────────

func TestExtractKeyNamesMap_CompositeLit(t *testing.T) {
	expr := &ast.CompositeLit{
		Elts: []ast.Expr{
			&ast.KeyValueExpr{
				Key:   &ast.Ident{Name: "keyCR"},
				Value: &ast.BasicLit{Kind: token.STRING, Value: `"enter"`},
			},
			&ast.KeyValueExpr{
				Key:   &ast.Ident{Name: "keyBS"},
				Value: &ast.BasicLit{Kind: token.STRING, Value: `"backspace"`},
			},
		},
	}
	result := make(map[string]string)
	extractKeyNamesMap(expr, result)
	if result["keyCR"] != "enter" {
		t.Errorf("keyCR: got %q, want %q", result["keyCR"], "enter")
	}
	if result["keyBS"] != "backspace" {
		t.Errorf("keyBS: got %q, want %q", result["keyBS"], "backspace")
	}
}

func TestExtractKeyNamesMap_NotCompositeLit(t *testing.T) {
	result := make(map[string]string)
	extractKeyNamesMap(&ast.Ident{Name: "x"}, result)
	if len(result) != 0 {
		t.Errorf("expected empty for non-composite-lit, got %v", result)
	}
}
