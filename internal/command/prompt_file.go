package command

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// maxPromptFileSize is the maximum size of a .prompt.md file (1 MiB).
const maxPromptFileSize = 1 << 20

// PromptFile represents a parsed VS Code .prompt.md file.
type PromptFile struct {
	// Frontmatter fields (from YAML header).
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Model       string   `json:"model,omitempty"`
	Tools       []string `json:"tools,omitempty"`

	// Body is the Markdown prompt text (everything after the frontmatter).
	Body string `json:"body"`

	// SourcePath is the path from which the prompt file was loaded.
	SourcePath string `json:"-"`
}

// ParsePromptFile parses a .prompt.md file from raw bytes.
// It extracts optional YAML frontmatter delimited by --- lines
// and the Markdown body that follows.
func ParsePromptFile(data []byte) (*PromptFile, error) {
	pf := &PromptFile{}

	content := string(data)

	// Check for YAML frontmatter.
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		// Find the closing --- delimiter.
		var afterOpener string
		if strings.HasPrefix(content, "---\r\n") {
			afterOpener = content[5:]
		} else {
			afterOpener = content[4:]
		}

		// The closing --- can be the very first line (empty frontmatter)
		// or after a newline.
		var closeIdx int
		var frontmatter string
		if strings.HasPrefix(afterOpener, "---\n") || strings.HasPrefix(afterOpener, "---\r\n") {
			// Empty frontmatter.
			closeIdx = 0
			frontmatter = ""
		} else {
			nlIdx := strings.Index(afterOpener, "\n---")
			if nlIdx < 0 {
				return nil, fmt.Errorf("unclosed YAML frontmatter: missing closing ---")
			}
			closeIdx = nlIdx + 1 // position of the closing ---
			frontmatter = afterOpener[:nlIdx]
		}

		// Skip past the closing --- and the newline after it.
		rest := afterOpener[closeIdx+3:]
		if strings.HasPrefix(rest, "\n") {
			rest = rest[1:]
		} else if strings.HasPrefix(rest, "\r\n") {
			rest = rest[2:]
		}
		pf.Body = rest

		// Parse frontmatter as simple key: value pairs.
		// We do NOT pull in a YAML dependency — the frontmatter format used by
		// VS Code prompt files is simple enough for line-by-line parsing.
		if err := parseSimpleYAML(frontmatter, pf); err != nil {
			return nil, fmt.Errorf("invalid frontmatter: %w", err)
		}
	} else {
		pf.Body = content
	}

	return pf, nil
}

// parseSimpleYAML parses a lightweight subset of YAML frontmatter into a
// PromptFile. Only scalar string values and simple lists (inline [...] and
// multi-line "- item" syntax) are supported — this covers the VS Code prompt
// file schema without requiring a full YAML parser.
func parseSimpleYAML(raw string, pf *PromptFile) error {
	lines := strings.Split(raw, "\n")

	// currentListKey tracks which key is being populated by multi-line list
	// items (lines starting with "- ").
	var currentListKey string

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		// Skip empty lines and comments.
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			currentListKey = ""
			continue
		}

		// Multi-line list item: "  - value"
		if strings.HasPrefix(trimmed, "- ") && currentListKey != "" {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			item = unquoteYAMLString(item)
			if currentListKey == "tools" {
				pf.Tools = append(pf.Tools, item)
			}
			continue
		}

		// Key: value pair.
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			currentListKey = ""
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])

		// Reset list state for each new key.
		currentListKey = ""

		switch key {
		case "name":
			pf.Name = unquoteYAMLString(value)
		case "description":
			pf.Description = unquoteYAMLString(value)
		case "model":
			pf.Model = unquoteYAMLString(value)
		case "tools":
			if value == "" {
				// Multi-line list follows: subsequent "- item" lines.
				currentListKey = "tools"
			} else {
				// Inline list: [tool1, tool2, ...]
				pf.Tools = parseInlineYAMLList(value)
			}
		// agent, argument-hint: acknowledged but not mapped to Goal fields.
		default:
			// Unknown keys are silently ignored for forward compatibility.
		}
	}

	return nil
}

// unquoteYAMLString removes leading/trailing quotes from a YAML string value.
func unquoteYAMLString(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseInlineYAMLList parses an inline YAML list like [a, b, c].
func parseInlineYAMLList(s string) []string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		// Treat as single-element list.
		v := unquoteYAMLString(s)
		if v == "" {
			return nil
		}
		return []string{v}
	}
	inner := s[1 : len(s)-1]
	if strings.TrimSpace(inner) == "" {
		return nil
	}
	parts := strings.Split(inner, ",")
	var result []string
	for _, p := range parts {
		v := unquoteYAMLString(strings.TrimSpace(p))
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

// LoadPromptFile reads and parses a .prompt.md file from disk.
func LoadPromptFile(path string) (*PromptFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat prompt file %q: %w", path, err)
	}
	if info.Size() > maxPromptFileSize {
		return nil, fmt.Errorf("prompt file %q is too large (%d bytes, max %d)", path, info.Size(), maxPromptFileSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompt file %q: %w", path, err)
	}

	pf, err := ParsePromptFile(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse prompt file %q: %w", path, err)
	}

	pf.SourcePath = path
	return pf, nil
}

// PromptFileToGoal converts a parsed PromptFile into an osm Goal.
//
// The mapping is:
//   - PromptFile.Name      → Goal.Name (falls back to filename stem)
//   - PromptFile.Description → Goal.Description
//   - PromptFile.Body      → Goal.PromptInstructions
//   - PromptFile.Tools     → stored in Goal.PromptOptions["tools"]
//   - PromptFile.Model     → stored in Goal.PromptOptions["model"]
//
// The goal uses the standard embedded goalScript interpreter and the default
// set of contextManager commands.
func PromptFileToGoal(pf *PromptFile) *Goal {
	name := pf.Name
	if name == "" && pf.SourcePath != "" {
		name = promptFileNameFromPath(pf.SourcePath)
	}
	if name == "" {
		name = "unnamed-prompt"
	}

	description := pf.Description
	if description == "" {
		description = "Imported from " + filepath.Base(pf.SourcePath)
	}

	// Expand file references in the body.
	body := pf.Body
	if pf.SourcePath != "" {
		body = expandPromptFileReferences(body, filepath.Dir(pf.SourcePath))
	}

	goal := &Goal{
		Name:               name,
		Description:        description,
		Category:           "prompt-file",
		Script:             goalScript,
		FileName:           filepath.Base(pf.SourcePath),
		TUITitle:           cases.Title(language.Und).String(strings.ReplaceAll(name, "-", " ")),
		TUIPrompt:          "(" + name + ") > ",
		PromptInstructions: body,
		ContextHeader:      "CONTEXT",
		Commands: []CommandConfig{
			{Name: "add", Type: "contextManager"},
			{Name: "note", Type: "contextManager"},
			{Name: "list", Type: "contextManager"},
			{Name: "edit", Type: "contextManager"},
			{Name: "remove", Type: "contextManager"},
			{Name: "show", Type: "contextManager"},
			{Name: "copy", Type: "contextManager"},
			{Name: "help", Type: "help"},
		},
		PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,
	}

	// Carry over prompt options.
	if pf.Model != "" || len(pf.Tools) > 0 {
		opts := make(map[string]interface{})
		if pf.Model != "" {
			opts["model"] = pf.Model
		}
		if len(pf.Tools) > 0 {
			opts["tools"] = pf.Tools
		}
		goal.PromptOptions = opts
	}

	return goal
}

// promptFileNameFromPath derives a goal name from a .prompt.md file path.
// It strips the .prompt.md extension and sanitizes the name to be a valid
// goal identifier (alphanumeric + hyphens).
func promptFileNameFromPath(path string) string {
	base := filepath.Base(path)
	// Strip .prompt.md suffix (case-insensitive).
	lower := strings.ToLower(base)
	var name string
	if strings.HasSuffix(lower, ".prompt.md") {
		name = base[:len(base)-len(".prompt.md")]
	} else if strings.HasSuffix(lower, ".md") {
		name = base[:len(base)-len(".md")]
	} else {
		name = base
	}
	// Sanitize: replace non-alphanumeric chars with hyphens.
	var buf bytes.Buffer
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			buf.WriteRune(r)
		} else {
			buf.WriteByte('-')
		}
	}
	result := buf.String()
	// Collapse consecutive hyphens.
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	// Trim leading/trailing hyphens.
	result = strings.Trim(result, "-")
	if result == "" {
		return "unnamed-prompt"
	}
	return result
}

// expandPromptFileReferences resolves Markdown link file references in the
// prompt body. Links of the form [text](relative/path.ext) where the target
// file exists on disk are replaced with inline file content blocks.
func expandPromptFileReferences(body string, baseDir string) string {
	// We use a simple scan rather than a regex for clarity and robustness.
	var result strings.Builder
	i := 0
	for i < len(body) {
		// Look for Markdown link pattern: [text](path)
		if body[i] == '[' {
			closeText := strings.Index(body[i:], "](")
			if closeText >= 0 {
				closeTextAbs := i + closeText
				closeParen := strings.Index(body[closeTextAbs+2:], ")")
				if closeParen >= 0 {
					closeParenAbs := closeTextAbs + 2 + closeParen
					linkPath := body[closeTextAbs+2 : closeParenAbs]
					linkText := body[i+1 : closeTextAbs]

					// Only expand local file references (no URLs).
					if !strings.Contains(linkPath, "://") && linkPath != "" {
						absPath := filepath.Join(baseDir, linkPath)
						if data, err := os.ReadFile(absPath); err == nil {
							result.WriteString("**")
							result.WriteString(linkText)
							result.WriteString("** (`")
							result.WriteString(linkPath)
							result.WriteString("`):\n```\n")
							result.Write(data)
							if len(data) > 0 && data[len(data)-1] != '\n' {
								result.WriteByte('\n')
							}
							result.WriteString("```\n")
							i = closeParenAbs + 1
							continue
						}
					}
				}
			}
		}
		result.WriteByte(body[i])
		i++
	}
	return result.String()
}

// FindPromptFiles scans a directory for .prompt.md files.
// Permission errors on individual entries are skipped.
func FindPromptFiles(dir string) ([]GoalFileCandidate, error) {
	var candidates []GoalFileCandidate

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		if os.IsPermission(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read prompt directory %q: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Resolve symlinks.
		if entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			if info.IsDir() {
				continue
			}
		}

		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".prompt.md") {
			continue
		}

		path := filepath.Join(dir, name)
		goalName := promptFileNameFromPath(name)
		candidates = append(candidates, GoalFileCandidate{
			Path: path,
			Name: goalName,
		})
	}

	return candidates, nil
}
