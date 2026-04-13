package main

import (
	"strings"
	"testing"
)

// ── generateKeysOutput ─────────────────────────────────────────────

func TestGenerateKeysOutput_EmptySlice(t *testing.T) {
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
	entries := []keyEntry{
		{Name: "KeySpace", StringVal: "space", Code: ' '},
	}
	out, err := generateKeysOutput(entries, entries)
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
	entries := []keyEntry{
		{Name: "KeyEnter", StringVal: "enter", Code: '\r'},
	}
	out, err := generateKeysOutput(entries, entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"enter"`) {
		t.Error("missing 'enter' in output")
	}
	if !strings.Contains(s, "KeyEnter") {
		t.Error("missing KeyEnter in output")
	}
	lines := strings.SplitSeq(s, "\n")
	for line := range lines {
		if strings.Contains(line, `"enter":`) && strings.Contains(line, "KeyDef") {
			if !strings.Contains(line, `Name: "KeyEnter"`) {
				t.Errorf("enter should use canonical KeyEnter, got: %s", line)
			}
		}
	}
}

func TestGenerateKeysOutput_ExportedName(t *testing.T) {
	entries := []keyEntry{
		{Name: "KeyRunes", StringVal: "runes", Code: 0},
	}
	out, err := generateKeysOutput(entries, entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "KeyRunes") {
		t.Error("missing KeyRunes")
	}
}

func TestGenerateKeysOutput_Deterministic(t *testing.T) {
	entries := []keyEntry{
		{Name: "KeyA", StringVal: "aaa", Code: 'a'},
		{Name: "KeyB", StringVal: "bbb", Code: 'b'},
		{Name: "KeyC", StringVal: "ccc", Code: 'c'},
	}
	out1, err := generateKeysOutput(entries, entries)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	out2, err := generateKeysOutput(entries, entries)
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
	// MouseActionDefs no longer exists in v2 - mouse actions are removed
	if strings.Contains(s, "MouseActionDefs") {
		t.Error("MouseActionDefs should not exist in v2")
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
	// MouseActionDefs no longer exists in v2
	if strings.Contains(s, "MouseActionPress") {
		t.Error("MouseActionPress should not exist in v2")
	}
	if strings.Contains(s, `"press"`) {
		t.Error("'press' string value should not exist in v2")
	}
}

func TestGenerateMouseOutput_Deterministic(t *testing.T) {
	buttons := map[string]string{
		"MouseButtonLeft":  "left",
		"MouseButtonRight": "right",
	}
	// actions is always nil in v2 since mouse actions are removed
	out1, err := generateMouseOutput(buttons, nil)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	out2, err := generateMouseOutput(buttons, nil)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if string(out1) != string(out2) {
		t.Error("mouse output is not deterministic")
	}
}
