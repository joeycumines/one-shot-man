package command

import (
	"slices"
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
		{
			name:          "pii-scrubber",
			category:      "data-privacy",
			contextHeader: "CONTENT TO SCRUB",
			wantCommands:  []string{"add", "diff", "note", "level", "list", "edit", "remove", "show", "copy", "help"},
		},
		{
			name:          "prose-polisher",
			category:      "writing",
			contextHeader: "PROSE TO POLISH",
			wantCommands:  []string{"add", "diff", "note", "style", "list", "edit", "remove", "show", "copy", "help"},
		},
		{
			name:          "data-to-json",
			category:      "data-transformation",
			contextHeader: "RAW DATA / UNSTRUCTURED INPUT",
			wantCommands:  []string{"add", "diff", "note", "mode", "list", "edit", "remove", "show", "copy", "help"},
		},
		{
			name:          "cite-sources",
			category:      "research",
			contextHeader: "SOURCE MATERIAL",
			wantCommands:  []string{"add", "diff", "note", "format", "list", "edit", "remove", "show", "copy", "help"},
		},
		{
			name:          "sql-generator",
			category:      "data-engineering",
			contextHeader: "SCHEMA & QUERY REQUEST",
			wantCommands:  []string{"add", "diff", "note", "dialect", "list", "edit", "remove", "show", "copy", "help"},
		},
		{
			name:          "report-analyzer",
			category:      "business-analysis",
			contextHeader: "REPORT / DOCUMENT",
			wantCommands:  []string{"add", "diff", "note", "focus", "list", "edit", "remove", "show", "copy", "help"},
		},
		{
			name:          "review-classifier",
			category:      "product-analysis",
			contextHeader: "FEEDBACK / REVIEWS",
			wantCommands:  []string{"add", "note", "output-format", "list", "edit", "remove", "show", "copy", "help"},
		},
		{
			name:          "adaptive-editor",
			category:      "writing",
			contextHeader: "TEXT TO REWRITE",
			wantCommands:  []string{"add", "diff", "note", "instruct", "list", "edit", "remove", "show", "copy", "help"},
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
	foundNotable := slices.Contains(found.NotableVariables, "depth")
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

func TestBuiltInGoal_PIIScrubberLevelState(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "pii-scrubber" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("pii-scrubber goal not found")
	}

	// Must have level in StateVars with default "strict"
	v, ok := found.StateVars["level"]
	if !ok {
		t.Fatalf("expected stateVars to contain 'level'")
	}
	if sv, ok := v.(string); !ok || sv != "strict" {
		t.Fatalf("expected default stateVars['level'] == 'strict', got: %#v", v)
	}

	// Must have NotableVariables containing "level"
	foundNotable := slices.Contains(found.NotableVariables, "level")
	if !foundNotable {
		t.Fatalf("expected NotableVariables to contain 'level', got: %v", found.NotableVariables)
	}

	// Must have PromptOptions with levelInstructions
	if found.PromptOptions == nil {
		t.Fatalf("expected non-nil PromptOptions")
	}
	li, ok := found.PromptOptions["levelInstructions"]
	if !ok {
		t.Fatalf("expected PromptOptions to contain 'levelInstructions'")
	}
	levelMap, ok := li.(map[string]string)
	if !ok {
		t.Fatalf("expected levelInstructions to be map[string]string, got: %T", li)
	}
	for _, level := range []string{"strict", "moderate", "minimal"} {
		if _, ok := levelMap[level]; !ok {
			t.Errorf("expected levelInstructions to contain key %q", level)
		}
	}

	// Must have level custom command with handler
	var levelCmd *CommandConfig
	for i := range found.Commands {
		if found.Commands[i].Name == "level" {
			levelCmd = &found.Commands[i]
			break
		}
	}
	if levelCmd == nil {
		t.Fatalf("expected 'level' command in pii-scrubber")
	}
	if levelCmd.Type != "custom" {
		t.Errorf("expected level command type 'custom', got %q", levelCmd.Type)
	}
	if levelCmd.Handler == "" {
		t.Error("expected non-empty handler for level command")
	}
}

func TestBuiltInGoal_ProsePolisherStyleState(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "prose-polisher" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("prose-polisher goal not found")
	}

	// Must have style in StateVars with default "technical"
	v, ok := found.StateVars["style"]
	if !ok {
		t.Fatalf("expected stateVars to contain 'style'")
	}
	if sv, ok := v.(string); !ok || sv != "technical" {
		t.Fatalf("expected default stateVars['style'] == 'technical', got: %#v", v)
	}

	// Must have NotableVariables containing "style"
	foundNotable := slices.Contains(found.NotableVariables, "style")
	if !foundNotable {
		t.Fatalf("expected NotableVariables to contain 'style', got: %v", found.NotableVariables)
	}

	// Must have PromptOptions with styleInstructions
	if found.PromptOptions == nil {
		t.Fatalf("expected non-nil PromptOptions")
	}
	si, ok := found.PromptOptions["styleInstructions"]
	if !ok {
		t.Fatalf("expected PromptOptions to contain 'styleInstructions'")
	}
	styleMap, ok := si.(map[string]string)
	if !ok {
		t.Fatalf("expected styleInstructions to be map[string]string, got: %T", si)
	}
	for _, style := range []string{"technical", "casual", "academic", "marketing"} {
		if _, ok := styleMap[style]; !ok {
			t.Errorf("expected styleInstructions to contain key %q", style)
		}
	}

	// Must have style custom command with handler
	var styleCmd *CommandConfig
	for i := range found.Commands {
		if found.Commands[i].Name == "style" {
			styleCmd = &found.Commands[i]
			break
		}
	}
	if styleCmd == nil {
		t.Fatalf("expected 'style' command in prose-polisher")
	}
	if styleCmd.Type != "custom" {
		t.Errorf("expected style command type 'custom', got %q", styleCmd.Type)
	}
	if styleCmd.Handler == "" {
		t.Error("expected non-empty handler for style command")
	}
}

func TestBuiltInGoal_ProsePolisherHotSnippets(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "prose-polisher" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("prose-polisher goal not found")
	}

	if len(found.HotSnippets) == 0 {
		t.Fatal("expected at least one hot-snippet for prose-polisher")
	}

	var expandSnippet *GoalHotSnippet
	for i := range found.HotSnippets {
		if found.HotSnippets[i].Name == "expand-section" {
			expandSnippet = &found.HotSnippets[i]
			break
		}
	}
	if expandSnippet == nil {
		t.Fatal("expected hot-snippet 'expand-section' in prose-polisher")
	}
	if expandSnippet.Text == "" {
		t.Error("expected non-empty Text for expand-section snippet")
	}
	if expandSnippet.Description == "" {
		t.Error("expected non-empty Description for expand-section snippet")
	}
}

func TestBuiltInGoal_DataToJsonModeState(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "data-to-json" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("data-to-json goal not found")
	}

	// Must have mode in StateVars with default "auto"
	v, ok := found.StateVars["mode"]
	if !ok {
		t.Fatalf("expected stateVars to contain 'mode'")
	}
	if sv, ok := v.(string); !ok || sv != "auto" {
		t.Fatalf("expected default stateVars['mode'] == 'auto', got: %#v", v)
	}

	// Must have NotableVariables containing "mode"
	foundNotable := slices.Contains(found.NotableVariables, "mode")
	if !foundNotable {
		t.Fatalf("expected NotableVariables to contain 'mode', got: %v", found.NotableVariables)
	}

	// Must have PromptOptions with modeInstructions
	if found.PromptOptions == nil {
		t.Fatalf("expected non-nil PromptOptions")
	}
	mi, ok := found.PromptOptions["modeInstructions"]
	if !ok {
		t.Fatalf("expected PromptOptions to contain 'modeInstructions'")
	}
	modeMap, ok := mi.(map[string]string)
	if !ok {
		t.Fatalf("expected modeInstructions to be map[string]string, got: %T", mi)
	}
	for _, mode := range []string{"auto", "tabular", "log", "document"} {
		if _, ok := modeMap[mode]; !ok {
			t.Errorf("expected modeInstructions to contain key %q", mode)
		}
	}

	// Must have mode custom command with handler
	var modeCmd *CommandConfig
	for i := range found.Commands {
		if found.Commands[i].Name == "mode" {
			modeCmd = &found.Commands[i]
			break
		}
	}
	if modeCmd == nil {
		t.Fatalf("expected 'mode' command in data-to-json")
	}
	if modeCmd.Type != "custom" {
		t.Errorf("expected mode command type 'custom', got %q", modeCmd.Type)
	}
	if modeCmd.Handler == "" {
		t.Error("expected non-empty handler for mode command")
	}
}

func TestBuiltInGoal_CiteSourcesFormatState(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "cite-sources" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("cite-sources goal not found")
	}

	// Must have format in StateVars with default "numbered"
	v, ok := found.StateVars["format"]
	if !ok {
		t.Fatalf("expected stateVars to contain 'format'")
	}
	if sv, ok := v.(string); !ok || sv != "numbered" {
		t.Fatalf("expected default stateVars['format'] == 'numbered', got: %#v", v)
	}

	// Must have NotableVariables containing "format"
	foundNotable := slices.Contains(found.NotableVariables, "format")
	if !foundNotable {
		t.Fatalf("expected NotableVariables to contain 'format', got: %v", found.NotableVariables)
	}

	// Must have PromptOptions with formatInstructions
	if found.PromptOptions == nil {
		t.Fatalf("expected non-nil PromptOptions")
	}
	fi, ok := found.PromptOptions["formatInstructions"]
	if !ok {
		t.Fatalf("expected PromptOptions to contain 'formatInstructions'")
	}
	formatMap, ok := fi.(map[string]string)
	if !ok {
		t.Fatalf("expected formatInstructions to be map[string]string, got: %T", fi)
	}
	for _, format := range []string{"numbered", "author-date", "footnote"} {
		if _, ok := formatMap[format]; !ok {
			t.Errorf("expected formatInstructions to contain key %q", format)
		}
	}

	// Must have format custom command with handler
	var formatCmd *CommandConfig
	for i := range found.Commands {
		if found.Commands[i].Name == "format" {
			formatCmd = &found.Commands[i]
			break
		}
	}
	if formatCmd == nil {
		t.Fatalf("expected 'format' command in cite-sources")
	}
	if formatCmd.Type != "custom" {
		t.Errorf("expected format command type 'custom', got %q", formatCmd.Type)
	}
	if formatCmd.Handler == "" {
		t.Error("expected non-empty handler for format command")
	}
}

func TestBuiltInGoal_CiteSourcesHotSnippets(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "cite-sources" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("cite-sources goal not found")
	}

	if len(found.HotSnippets) == 0 {
		t.Fatal("expected at least one hot-snippet for cite-sources")
	}

	var challengeSnippet *GoalHotSnippet
	for i := range found.HotSnippets {
		if found.HotSnippets[i].Name == "challenge-claims" {
			challengeSnippet = &found.HotSnippets[i]
			break
		}
	}
	if challengeSnippet == nil {
		t.Fatal("expected hot-snippet 'challenge-claims' in cite-sources")
	}
	if challengeSnippet.Text == "" {
		t.Error("expected non-empty Text for challenge-claims snippet")
	}
	if challengeSnippet.Description == "" {
		t.Error("expected non-empty Description for challenge-claims snippet")
	}
}

func TestBuiltInGoal_SQLGeneratorDialectState(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "sql-generator" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("sql-generator goal not found")
	}

	// Must have dialect in StateVars with default "auto"
	v, ok := found.StateVars["dialect"]
	if !ok {
		t.Fatalf("expected stateVars to contain 'dialect'")
	}
	if sv, ok := v.(string); !ok || sv != "auto" {
		t.Fatalf("expected default stateVars['dialect'] == 'auto', got: %#v", v)
	}

	// Must have NotableVariables containing "dialect"
	foundNotable := slices.Contains(found.NotableVariables, "dialect")
	if !foundNotable {
		t.Fatalf("expected NotableVariables to contain 'dialect', got: %v", found.NotableVariables)
	}

	// Must have PromptOptions with dialectInstructions
	if found.PromptOptions == nil {
		t.Fatalf("expected non-nil PromptOptions")
	}
	di, ok := found.PromptOptions["dialectInstructions"]
	if !ok {
		t.Fatalf("expected PromptOptions to contain 'dialectInstructions'")
	}
	dialectMap, ok := di.(map[string]string)
	if !ok {
		t.Fatalf("expected dialectInstructions to be map[string]string, got: %T", di)
	}
	for _, dialect := range []string{"auto", "postgresql", "mysql", "sqlite", "mssql"} {
		if _, ok := dialectMap[dialect]; !ok {
			t.Errorf("expected dialectInstructions to contain key %q", dialect)
		}
	}

	// Must have dialect custom command with handler
	var dialectCmd *CommandConfig
	for i := range found.Commands {
		if found.Commands[i].Name == "dialect" {
			dialectCmd = &found.Commands[i]
			break
		}
	}
	if dialectCmd == nil {
		t.Fatalf("expected 'dialect' command in sql-generator")
	}
	if dialectCmd.Type != "custom" {
		t.Errorf("expected dialect command type 'custom', got %q", dialectCmd.Type)
	}
	if dialectCmd.Handler == "" {
		t.Error("expected non-empty handler for dialect command")
	}
}

func TestBuiltInGoal_ReportAnalyzerFocusState(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "report-analyzer" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("report-analyzer goal not found")
	}

	// Must have focus in StateVars with default "general"
	v, ok := found.StateVars["focus"]
	if !ok {
		t.Fatalf("expected stateVars to contain 'focus'")
	}
	if sv, ok := v.(string); !ok || sv != "general" {
		t.Fatalf("expected default stateVars['focus'] == 'general', got: %#v", v)
	}

	// Must have NotableVariables containing "focus"
	foundNotable := slices.Contains(found.NotableVariables, "focus")
	if !foundNotable {
		t.Fatalf("expected NotableVariables to contain 'focus', got: %v", found.NotableVariables)
	}

	// Must have PromptOptions with focusInstructions
	if found.PromptOptions == nil {
		t.Fatalf("expected non-nil PromptOptions")
	}
	fi, ok := found.PromptOptions["focusInstructions"]
	if !ok {
		t.Fatalf("expected PromptOptions to contain 'focusInstructions'")
	}
	focusMap, ok := fi.(map[string]string)
	if !ok {
		t.Fatalf("expected focusInstructions to be map[string]string, got: %T", fi)
	}
	for _, focus := range []string{"general", "financial", "risk", "strategic", "technical"} {
		if _, ok := focusMap[focus]; !ok {
			t.Errorf("expected focusInstructions to contain key %q", focus)
		}
	}

	// Must have focus custom command with handler
	var focusCmd *CommandConfig
	for i := range found.Commands {
		if found.Commands[i].Name == "focus" {
			focusCmd = &found.Commands[i]
			break
		}
	}
	if focusCmd == nil {
		t.Fatalf("expected 'focus' command in report-analyzer")
	}
	if focusCmd.Type != "custom" {
		t.Errorf("expected focus command type 'custom', got %q", focusCmd.Type)
	}
	if focusCmd.Handler == "" {
		t.Error("expected non-empty handler for focus command")
	}
}

func TestBuiltInGoal_ReviewClassifierOutputFormatState(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "review-classifier" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("review-classifier goal not found")
	}

	// Must have outputFormat in StateVars with default "detailed"
	v, ok := found.StateVars["outputFormat"]
	if !ok {
		t.Fatalf("expected stateVars to contain 'outputFormat'")
	}
	if sv, ok := v.(string); !ok || sv != "detailed" {
		t.Fatalf("expected default stateVars['outputFormat'] == 'detailed', got: %#v", v)
	}

	// Must have NotableVariables containing "outputFormat"
	foundNotable := slices.Contains(found.NotableVariables, "outputFormat")
	if !foundNotable {
		t.Fatalf("expected NotableVariables to contain 'outputFormat', got: %v", found.NotableVariables)
	}

	// Must have PromptOptions with outputFormatInstructions
	if found.PromptOptions == nil {
		t.Fatalf("expected non-nil PromptOptions")
	}
	ofi, ok := found.PromptOptions["outputFormatInstructions"]
	if !ok {
		t.Fatalf("expected PromptOptions to contain 'outputFormatInstructions'")
	}
	formatMap, ok := ofi.(map[string]string)
	if !ok {
		t.Fatalf("expected outputFormatInstructions to be map[string]string, got: %T", ofi)
	}
	for _, format := range []string{"detailed", "summary", "json"} {
		if _, ok := formatMap[format]; !ok {
			t.Errorf("expected outputFormatInstructions to contain key %q", format)
		}
	}

	// Must have output-format custom command with handler
	var fmtCmd *CommandConfig
	for i := range found.Commands {
		if found.Commands[i].Name == "output-format" {
			fmtCmd = &found.Commands[i]
			break
		}
	}
	if fmtCmd == nil {
		t.Fatalf("expected 'output-format' command in review-classifier")
	}
	if fmtCmd.Type != "custom" {
		t.Errorf("expected output-format command type 'custom', got %q", fmtCmd.Type)
	}
	if fmtCmd.Handler == "" {
		t.Error("expected non-empty handler for output-format command")
	}
}

func TestBuiltInGoal_ReviewClassifierNoDiffCommand(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "review-classifier" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("review-classifier goal not found")
	}

	// review-classifier should NOT have a diff command (not relevant for feedback text)
	for _, c := range found.Commands {
		if c.Name == "diff" {
			t.Fatal("review-classifier should not have a 'diff' command — diffs are not relevant for feedback classification")
		}
	}
}

func TestBuiltInGoal_AdaptiveEditorInstructState(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "adaptive-editor" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("adaptive-editor goal not found")
	}

	// Must have instruction in StateVars with default "" (empty string)
	v, ok := found.StateVars["instruction"]
	if !ok {
		t.Fatalf("expected stateVars to contain 'instruction'")
	}
	if sv, ok := v.(string); !ok || sv != "" {
		t.Fatalf("expected default stateVars['instruction'] == '', got: %#v", v)
	}

	// Must have NotableVariables containing "instruction"
	foundNotable := slices.Contains(found.NotableVariables, "instruction")
	if !foundNotable {
		t.Fatalf("expected NotableVariables to contain 'instruction', got: %v", found.NotableVariables)
	}

	// Must have instruct custom command with handler
	var instructCmd *CommandConfig
	for i := range found.Commands {
		if found.Commands[i].Name == "instruct" {
			instructCmd = &found.Commands[i]
			break
		}
	}
	if instructCmd == nil {
		t.Fatalf("expected 'instruct' command in adaptive-editor")
	}
	if instructCmd.Type != "custom" {
		t.Errorf("expected instruct command type 'custom', got %q", instructCmd.Type)
	}
	if instructCmd.Handler == "" {
		t.Error("expected non-empty handler for instruct command")
	}
}

func TestBuiltInGoal_SQLGeneratorHotSnippets(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "sql-generator" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("sql-generator goal not found")
	}

	if len(found.HotSnippets) == 0 {
		t.Fatal("expected at least one hot-snippet for sql-generator")
	}

	var explainSnippet *GoalHotSnippet
	for i := range found.HotSnippets {
		if found.HotSnippets[i].Name == "explain-plan" {
			explainSnippet = &found.HotSnippets[i]
			break
		}
	}
	if explainSnippet == nil {
		t.Fatal("expected hot-snippet 'explain-plan' in sql-generator")
	}
	if explainSnippet.Text == "" {
		t.Error("expected non-empty Text for explain-plan snippet")
	}
	if explainSnippet.Description == "" {
		t.Error("expected non-empty Description for explain-plan snippet")
	}
}

func TestBuiltInGoal_AdaptiveEditorHotSnippets(t *testing.T) {
	goals := GetBuiltInGoals()

	var found *Goal
	for i := range goals {
		if goals[i].Name == "adaptive-editor" {
			found = &goals[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("adaptive-editor goal not found")
	}

	if len(found.HotSnippets) == 0 {
		t.Fatal("expected at least one hot-snippet for adaptive-editor")
	}

	var compareSnippet *GoalHotSnippet
	for i := range found.HotSnippets {
		if found.HotSnippets[i].Name == "compare-versions" {
			compareSnippet = &found.HotSnippets[i]
			break
		}
	}
	if compareSnippet == nil {
		t.Fatal("expected hot-snippet 'compare-versions' in adaptive-editor")
	}
	if compareSnippet.Text == "" {
		t.Error("expected non-empty Text for compare-versions snippet")
	}
	if compareSnippet.Description == "" {
		t.Error("expected non-empty Description for compare-versions snippet")
	}
}
