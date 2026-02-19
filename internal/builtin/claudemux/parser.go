// Package claudemux provides a PTY output parser for classifying Claude Code
// terminal output into typed events. It detects rate limits, permission prompts,
// model selection menus, SSO login flows, tool invocations, errors, and more.
// Configurable custom patterns allow extension beyond the built-in set.
package claudemux

import (
	"fmt"
	"regexp"
	"strings"
)

// EventType classifies a parsed output event.
type EventType int

const (
	EventText        EventType = iota // Normal text output (no pattern matched)
	EventRateLimit                    // Rate limit / 429 / backoff
	EventPermission                   // Permission prompt requiring Y/N
	EventModelSelect                  // Model selection menu
	EventSSOLogin                     // SSO / OAuth login flow
	EventCompletion                   // Agent signaled completion
	EventToolUse                      // MCP tool invocation
	EventError                        // Error message
	EventThinking                     // Agent thinking/processing indicator
)

// OutputEvent represents a parsed, classified line of agent output.
type OutputEvent struct {
	Type    EventType
	Line    string            // Raw line text
	Fields  map[string]string // Extracted fields (e.g., "retryAfter": "30")
	Pattern string            // Name of the matched pattern (empty for EventText)
}

// Parser transforms raw PTY output lines into classified events.
// It is NOT safe for concurrent use from multiple goroutines.
type Parser struct {
	patterns []patternEntry
}

type patternEntry struct {
	name    string
	re      *regexp.Regexp
	typ     EventType
	extract func([]string) map[string]string // submatch -> fields
}

// NewParser creates a parser pre-loaded with Claude Code output patterns.
func NewParser() *Parser {
	p := &Parser{}
	p.loadBuiltinPatterns()
	return p
}

// Parse classifies a single line of output.
// Returns an OutputEvent with EventText if no pattern matches.
func (p *Parser) Parse(line string) OutputEvent {
	for i := range p.patterns {
		pe := &p.patterns[i]
		matches := pe.re.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		var fields map[string]string
		if pe.extract != nil {
			fields = pe.extract(matches)
		}
		return OutputEvent{
			Type:    pe.typ,
			Line:    line,
			Fields:  fields,
			Pattern: pe.name,
		}
	}
	return OutputEvent{
		Type: EventText,
		Line: line,
	}
}

// AddPattern registers a custom detection pattern.
// Returns an error if the pattern is invalid regex.
func (p *Parser) AddPattern(name string, pattern string, eventType EventType) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("claudemux: invalid pattern %q: %w", name, err)
	}
	p.patterns = append(p.patterns, patternEntry{
		name: name,
		re:   re,
		typ:  eventType,
	})
	return nil
}

// EventTypeName returns the string name for an EventType constant.
func EventTypeName(t EventType) string {
	switch t {
	case EventText:
		return "Text"
	case EventRateLimit:
		return "RateLimit"
	case EventPermission:
		return "Permission"
	case EventModelSelect:
		return "ModelSelect"
	case EventSSOLogin:
		return "SSOLogin"
	case EventCompletion:
		return "Completion"
	case EventToolUse:
		return "ToolUse"
	case EventError:
		return "Error"
	case EventThinking:
		return "Thinking"
	default:
		return fmt.Sprintf("Unknown(%d)", int(t))
	}
}

// loadBuiltinPatterns registers the default Claude Code output patterns.
func (p *Parser) loadBuiltinPatterns() {
	// --- Rate Limit patterns ---
	p.addBuiltin("rate-limit-try-again", `(?i)try\s+again\s+in\s+(\d+)`, EventRateLimit,
		func(m []string) map[string]string {
			if len(m) > 1 {
				return map[string]string{"retryAfter": m[1]}
			}
			return nil
		})
	p.addBuiltin("rate-limit-keyword", `(?i)rate\s*limit`, EventRateLimit, nil)
	p.addBuiltin("rate-limit-too-many", `(?i)too\s+many\s+requests`, EventRateLimit, nil)
	p.addBuiltin("rate-limit-429", `(?i)429`, EventRateLimit, nil)
	p.addBuiltin("rate-limit-please-wait", `(?i)please\s+wait`, EventRateLimit, nil)
	p.addBuiltin("rate-limit-quota", `(?i)quota\s+exceeded`, EventRateLimit, nil)

	// --- Permission patterns ---
	p.addBuiltin("permission-yn", `(?i)(allow|permit|approve)\??\s*\[?[yY]/[nN]\]?`, EventPermission, nil)
	p.addBuiltin("permission-do-you-want", `(?i)do you want to (allow|proceed|continue)`, EventPermission, nil)
	p.addBuiltin("permission-required", `(?i)permission\s+(required|needed|denied)`, EventPermission, nil)

	// --- Model Selection patterns ---
	p.addBuiltin("model-select", `(?i)select\s+(a\s+)?model`, EventModelSelect, nil)
	p.addBuiltin("model-choose", `(?i)choose\s+(a\s+)?model`, EventModelSelect, nil)
	p.addBuiltin("model-available", `(?i)available\s+models?:`, EventModelSelect, nil)
	// Model list item with selection indicator (❯ or > prefix). Extracts
	// the model name and marks it as selected. Used to detect individual
	// lines within an interactive model selection TUI menu.
	p.addBuiltin("model-item-selected", `^\s*[❯>]\s+(\S.+)`, EventModelSelect,
		func(m []string) map[string]string {
			if len(m) > 1 {
				return map[string]string{
					"modelName": strings.TrimSpace(m[1]),
					"selected":  "true",
				}
			}
			return nil
		})

	// --- SSO/Login patterns ---
	p.addBuiltin("sso-browser", `(?i)(open|opening)\s+(your\s+)?browser`, EventSSOLogin, nil)
	p.addBuiltin("sso-flow", `(?i)(SSO|OAuth|login|sign\s*in)\s+(flow|required|needed)`, EventSSOLogin, nil)
	p.addBuiltin("sso-auth-required", `(?i)authentication\s+required`, EventSSOLogin, nil)
	p.addBuiltin("sso-visit-url", `(?i)visit\s+https?://`, EventSSOLogin, nil)

	// --- Completion patterns ---
	p.addBuiltin("completion", `(?i)(task|operation)\s+(complete|completed|finished|done)`, EventCompletion, nil)

	// --- Tool Use patterns ---
	p.addBuiltin("tool-calling", `(?i)calling\s+tool:?\s+(\S+)`, EventToolUse,
		func(m []string) map[string]string {
			if len(m) > 1 {
				return map[string]string{"toolName": m[1]}
			}
			return nil
		})
	p.addBuiltin("tool-result", `(?i)tool\s+result:?\s*(.*)`, EventToolUse,
		func(m []string) map[string]string {
			if len(m) > 1 {
				return map[string]string{"result": m[1]}
			}
			return nil
		})

	// --- Error patterns ---
	p.addBuiltin("error-prefix", `(?i)^error:?\s+(.+)`, EventError,
		func(m []string) map[string]string {
			if len(m) > 1 {
				return map[string]string{"message": m[1]}
			}
			return nil
		})
	p.addBuiltin("fatal-prefix", `(?i)^fatal:?\s+(.+)`, EventError,
		func(m []string) map[string]string {
			if len(m) > 1 {
				return map[string]string{"message": m[1]}
			}
			return nil
		})
	p.addBuiltin("panic-prefix", `(?i)^panic:?\s+(.+)`, EventError,
		func(m []string) map[string]string {
			if len(m) > 1 {
				return map[string]string{"message": m[1]}
			}
			return nil
		})

	// --- Thinking patterns ---
	p.addBuiltin("thinking-dots", `(?i)(thinking|analyzing|processing)\s*\.{2,}`, EventThinking, nil)
	p.addBuiltin("thinking-spinner", `[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]`, EventThinking, nil)
}

// addBuiltin is a helper that adds a pre-compiled pattern entry.
func (p *Parser) addBuiltin(name string, pattern string, typ EventType, extract func([]string) map[string]string) {
	p.patterns = append(p.patterns, patternEntry{
		name:    name,
		re:      regexp.MustCompile(pattern),
		typ:     typ,
		extract: extract,
	})
}
