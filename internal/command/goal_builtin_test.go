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

// using strings.Contains to check substring presence
