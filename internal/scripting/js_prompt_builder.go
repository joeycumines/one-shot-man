package scripting

import (
	"fmt"
	"time"
)

// jsCreatePromptBuilder creates a new prompt builder for JavaScript.
func (e *Engine) jsCreatePromptBuilder(title, description string) map[string]interface{} {
	pb := NewPromptBuilder(title, description)

	return map[string]interface{}{
		// Core methods
		"setTemplate": pb.SetTemplate,
		"setVariable": pb.SetVariable,
		"getVariable": pb.GetVariable,
		"build":       pb.Build,

		// Version management
		"saveVersion":    pb.SaveVersion,
		"getVersion":     pb.GetVersion,
		"restoreVersion": pb.RestoreVersion,
		"listVersions":   pb.ListVersions,

		// Export/Import
		"export": pb.Export,

		// Properties (read-only)
		"getTitle":       func() string { return pb.Title },
		"getDescription": func() string { return pb.Description },
		"getTemplate":    func() string { return pb.Template },
		"getVariables":   func() map[string]interface{} { return pb.Variables },

		// Utility methods
		"preview": func() string {
			return fmt.Sprintf("Title: %s\nDescription: %s\n\nCurrent Prompt:\n%s",
				pb.Title, pb.Description, pb.Build())
		},

		"stats": func() map[string]interface{} {
			return map[string]interface{}{
				"title":       pb.Title,
				"description": pb.Description,
				"versions":    len(pb.History),
				"variables":   len(pb.Variables),
				"hasTemplate": pb.Template != "",
				"lastUpdated": time.Now().Format(time.RFC3339),
			}
		},
	}
}
