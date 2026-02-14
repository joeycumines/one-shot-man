package command

import (
	"strings"
	"testing"
)

// Guard tests to prevent re-introducing legacy template keys / casing mistakes.
func TestBuiltInGoalTemplates_NoLegacyKeys(t *testing.T) {
	// Legacy tokens we don't want present in templates
	badSubstrings := []string{".stateVars", "{{.TypeInstructions"}

	for _, g := range GetBuiltInGoals() {
		fields := map[string]string{
			"PromptInstructions": g.PromptInstructions,
			"PromptTemplate":     g.PromptTemplate,
			"UsageTemplate":      g.UsageTemplate,
		}
		for fname, content := range fields {
			for _, bad := range badSubstrings {
				if content == "" {
					continue
				}
				if strings.Contains(content, bad) {
					t.Fatalf("found legacy template token %q in %s.%s", bad, g.Name, fname)
				}
			}
		}
	}
}

func TestBuiltInGoal_MoraleCommandsExist(t *testing.T) {
	// Ensure morale-improver exposes the state-editing commands
	for _, g := range GetBuiltInGoals() {
		if g.Name != "morale-improver" {
			continue
		}
		found := map[string]bool{}
		for _, c := range g.Commands {
			found[c.Name] = true
		}
		want := []string{"set-original", "set-plan", "set-failures"}
		for _, name := range want {
			if !found[name] {
				t.Fatalf("expected command %q present in goal %q", name, g.Name)
			}
		}
		return
	}
	t.Fatalf("morale-improver goal not found")
}

func TestBuiltInGoal_Tier1GoalsExist(t *testing.T) {
	goals := GetBuiltInGoals()

	tier1 := []struct {
		name          string
		category      string
		contextHeader string
		wantCommands  []string
	}{
		{
			name:          "bug-buster",
			category:      "code-quality",
			contextHeader: "CODE TO ANALYZE",
			wantCommands:  []string{"add", "diff", "note", "list", "edit", "remove", "show", "copy", "help"},
		},
		{
			name:          "code-optimizer",
			category:      "code-quality",
			contextHeader: "CODE TO OPTIMIZE",
			wantCommands:  []string{"add", "diff", "note", "list", "edit", "remove", "show", "copy", "help"},
		},
		{
			name:          "code-explainer",
			category:      "code-understanding",
			contextHeader: "CODE TO EXPLAIN",
			wantCommands:  []string{"add", "diff", "note", "depth", "list", "edit", "remove", "show", "copy", "help"},
		},
		{
			name:          "meeting-notes",
			category:      "productivity",
			contextHeader: "MEETING NOTES / TRANSCRIPT",
			wantCommands:  []string{"add", "note", "list", "edit", "remove", "show", "copy", "help"},
		},
	}

	for _, tc := range tier1 {
		t.Run(tc.name, func(t *testing.T) {
			var found *Goal
			for i := range goals {
				if goals[i].Name == tc.name {
					found = &goals[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("goal %q not found in built-ins", tc.name)
			}

			// Verify metadata
			if found.Category != tc.category {
				t.Errorf("expected category %q, got %q", tc.category, found.Category)
			}
			if found.ContextHeader != tc.contextHeader {
				t.Errorf("expected contextHeader %q, got %q", tc.contextHeader, found.ContextHeader)
			}
			if found.Description == "" {
				t.Error("expected non-empty Description")
			}
			if found.Usage == "" {
				t.Error("expected non-empty Usage")
			}
			if found.TUITitle == "" {
				t.Error("expected non-empty TUITitle")
			}
			if found.TUIPrompt == "" {
				t.Error("expected non-empty TUIPrompt")
			}
			if found.Script != goalScript {
				t.Error("expected Script to be goalScript")
			}
			if found.FileName != "goal.js" {
				t.Errorf("expected FileName %q, got %q", "goal.js", found.FileName)
			}
			if found.PromptInstructions == "" {
				t.Error("expected non-empty PromptInstructions")
			}
			if found.PromptTemplate == "" {
				t.Error("expected non-empty PromptTemplate")
			}

			// Verify all expected commands are present
			cmdMap := make(map[string]bool, len(found.Commands))
			for _, c := range found.Commands {
				cmdMap[c.Name] = true
			}
			for _, wantCmd := range tc.wantCommands {
				if !cmdMap[wantCmd] {
					t.Errorf("expected command %q not found in goal %q", wantCmd, tc.name)
				}
			}
		})
	}
}

func TestBuiltInGoal_CodeExplainerDepthState(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "code-explainer" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("code-explainer goal not found")
	}

	// Must have depth in StateVars with default "detailed"
	v, ok := found.StateVars["depth"]
	if !ok {
		t.Fatalf("expected stateVars to contain 'depth'")
	}
	if sv, ok := v.(string); !ok || sv != "detailed" {
		t.Fatalf("expected default stateVars['depth'] == 'detailed', got: %#v", v)
	}

	// Must have NotableVariables containing "depth"
	foundNotable := false
	for _, v := range found.NotableVariables {
		if v == "depth" {
			foundNotable = true
			break
		}
	}
	if !foundNotable {
		t.Fatalf("expected NotableVariables to contain 'depth', got: %v", found.NotableVariables)
	}

	// Must have PromptOptions with depthInstructions
	if found.PromptOptions == nil {
		t.Fatalf("expected non-nil PromptOptions")
	}
	di, ok := found.PromptOptions["depthInstructions"]
	if !ok {
		t.Fatalf("expected PromptOptions to contain 'depthInstructions'")
	}
	depthMap, ok := di.(map[string]string)
	if !ok {
		t.Fatalf("expected depthInstructions to be map[string]string, got: %T", di)
	}
	for _, level := range []string{"brief", "detailed", "comprehensive"} {
		if _, ok := depthMap[level]; !ok {
			t.Errorf("expected depthInstructions to contain key %q", level)
		}
	}

	// Must have depth custom command with handler
	var depthCmd *CommandConfig
	for i := range found.Commands {
		if found.Commands[i].Name == "depth" {
			depthCmd = &found.Commands[i]
			break
		}
	}
	if depthCmd == nil {
		t.Fatalf("expected 'depth' command in code-explainer")
	}
	if depthCmd.Type != "custom" {
		t.Errorf("expected depth command type 'custom', got %q", depthCmd.Type)
	}
	if depthCmd.Handler == "" {
		t.Error("expected non-empty handler for depth command")
	}
}

func TestBuiltInGoal_MeetingNotesNoDiffCommand(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "meeting-notes" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("meeting-notes goal not found")
	}

	// meeting-notes should NOT have a diff command (not relevant for transcripts)
	for _, c := range found.Commands {
		if c.Name == "diff" {
			t.Fatal("meeting-notes should not have a 'diff' command — diffs are not relevant for meeting transcripts")
		}
	}
}

func TestBuiltInGoal_UniqueNames(t *testing.T) {
	goals := GetBuiltInGoals()
	seen := make(map[string]bool, len(goals))
	for _, g := range goals {
		if seen[g.Name] {
			t.Fatalf("duplicate goal name: %q", g.Name)
		}
		seen[g.Name] = true
	}
}

func TestBuiltInGoal_AllGoalsHaveCopyAndHelp(t *testing.T) {
	for _, g := range GetBuiltInGoals() {
		hasCopy := false
		hasHelp := false
		for _, c := range g.Commands {
			if c.Name == "copy" {
				hasCopy = true
			}
			if c.Name == "help" {
				hasHelp = true
			}
		}
		if !hasCopy {
			t.Errorf("goal %q is missing 'copy' command", g.Name)
		}
		if !hasHelp {
			t.Errorf("goal %q is missing 'help' command", g.Name)
		}
	}
}
