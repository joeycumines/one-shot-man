package scripting

import (
	"fmt"
	"strings"
	"time"
)

// PromptBuilder helps build and refine prompts for LLM services.
type PromptBuilder struct {
	Title       string
	Description string
	Template    string
	Variables   map[string]interface{}
	History     []PromptVersion
	Current     *PromptVersion
}

// PromptVersion represents a version of a prompt with metadata.
type PromptVersion struct {
	Version   int
	Content   string
	Variables map[string]interface{}
	CreatedAt time.Time
	Notes     string
	Tags      []string
}

// NewPromptBuilder creates a new prompt builder.
func NewPromptBuilder(title, description string) *PromptBuilder {
	return &PromptBuilder{
		Title:       title,
		Description: description,
		Variables:   make(map[string]interface{}),
		History:     make([]PromptVersion, 0),
	}
}

// SetTemplate sets the prompt template with variable placeholders.
func (pb *PromptBuilder) SetTemplate(template string) {
	pb.Template = template
}

// SetVariable sets a variable value for the prompt.
func (pb *PromptBuilder) SetVariable(key string, value interface{}) {
	pb.Variables[key] = value
}

// GetVariable gets a variable value.
func (pb *PromptBuilder) GetVariable(key string) interface{} {
	return pb.Variables[key]
}

// Build builds the current prompt by replacing variables in the template.
func (pb *PromptBuilder) Build() string {
	content := pb.Template

	// Replace variables in the template
	for key, value := range pb.Variables {
		placeholder := fmt.Sprintf("{{%s}}", key)
		replacement := fmt.Sprintf("%v", value)
		content = strings.ReplaceAll(content, placeholder, replacement)
	}

	return content
}

// SaveVersion saves the current prompt as a new version.
func (pb *PromptBuilder) SaveVersion(notes string, tags []string) {
	version := PromptVersion{
		Version:   len(pb.History) + 1,
		Content:   pb.Build(),
		Variables: make(map[string]interface{}),
		CreatedAt: time.Now(),
		Notes:     notes,
		Tags:      tags,
	}

	// Copy current variables
	for k, v := range pb.Variables {
		version.Variables[k] = v
	}

	pb.History = append(pb.History, version)
	pb.Current = &version
}

// GetVersion gets a specific version by number.
func (pb *PromptBuilder) GetVersion(versionNum int) *PromptVersion {
	if versionNum < 1 || versionNum > len(pb.History) {
		return nil
	}
	return &pb.History[versionNum-1]
}

// RestoreVersion restores a specific version as the current state.
func (pb *PromptBuilder) RestoreVersion(versionNum int) error {
	version := pb.GetVersion(versionNum)
	if version == nil {
		return fmt.Errorf("version %d not found", versionNum)
	}

	pb.Template = version.Content
	pb.Variables = make(map[string]interface{})
	for k, v := range version.Variables {
		pb.Variables[k] = v
	}

	pb.Current = version
	return nil
}

// ListVersions returns information about all versions.
func (pb *PromptBuilder) ListVersions() []map[string]interface{} {
	versions := make([]map[string]interface{}, len(pb.History))

	for i, version := range pb.History {
		versions[i] = map[string]interface{}{
			"version":   version.Version,
			"createdAt": version.CreatedAt.Format(time.RFC3339),
			"notes":     version.Notes,
			"tags":      version.Tags,
			"content":   version.Content,
		}
	}

	return versions
}

// Export exports the prompt builder data as a JavaScript object.
func (pb *PromptBuilder) Export() map[string]interface{} {
	return map[string]interface{}{
		"title":       pb.Title,
		"description": pb.Description,
		"template":    pb.Template,
		"variables":   pb.Variables,
		"current":     pb.Build(),
		"versions":    pb.ListVersions(),
	}
}
