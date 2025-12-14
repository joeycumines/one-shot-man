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
