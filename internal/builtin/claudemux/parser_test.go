package claudemux

import (
	"testing"
)

func TestNewParser(t *testing.T) {
	t.Parallel()
	p := NewParser()
	if p == nil {
		t.Fatal("NewParser returned nil")
	}
	if len(p.patterns) == 0 {
		t.Fatal("NewParser should pre-load builtin patterns")
	}
}

func TestParse_RateLimit(t *testing.T) {
	t.Parallel()
	p := NewParser()

	tests := []struct {
		name           string
		line           string
		wantPattern    string
		wantRetryAfter string
	}{
		{"rate limit keyword", "You have been rate limited", "rate-limit-keyword", ""},
		{"too many requests", "Too many requests, slow down", "rate-limit-too-many", ""},
		{"429 status", "HTTP 429 received from API", "rate-limit-429", ""},
		{"try again with seconds", "Rate limited. Please try again in 30 seconds.", "rate-limit-try-again", "30"},
		{"please wait", "Please wait before sending more requests", "rate-limit-please-wait", ""},
		{"quota exceeded", "API quota exceeded for this billing period", "rate-limit-quota", ""},
		{"try again in 120", "try again in 120 seconds", "rate-limit-try-again", "120"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := p.Parse(tc.line)
			if ev.Type != EventRateLimit {
				t.Errorf("expected EventRateLimit, got %s", EventTypeName(ev.Type))
			}
			if ev.Line != tc.line {
				t.Errorf("expected line %q, got %q", tc.line, ev.Line)
			}
			if ev.Pattern != tc.wantPattern {
				t.Errorf("expected pattern %q, got %q", tc.wantPattern, ev.Pattern)
			}
			if tc.wantRetryAfter != "" {
				if ev.Fields == nil {
					t.Fatal("expected Fields to be non-nil for retryAfter extraction")
				}
				if got := ev.Fields["retryAfter"]; got != tc.wantRetryAfter {
					t.Errorf("expected retryAfter=%q, got %q", tc.wantRetryAfter, got)
				}
			}
		})
	}
}

func TestParse_Permission(t *testing.T) {
	t.Parallel()
	p := NewParser()

	tests := []struct {
		name string
		line string
	}{
		{"allow yn", "Do you want to allow write access to /etc/passwd? [Y/n]"},
		{"permit yn", "permit? [Y/n]"},
		{"approve yn", "approve? Y/n"},
		{"do you want to allow", "Do you want to allow this operation?"},
		{"do you want to proceed", "Do you want to proceed with the changes?"},
		{"do you want to continue", "Do you want to continue with deletion?"},
		{"permission required", "permission required to access this resource"},
		{"permission needed", "Permission needed for write access"},
		{"permission denied", "Permission denied: /etc/shadow"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := p.Parse(tc.line)
			if ev.Type != EventPermission {
				t.Errorf("line %q: expected EventPermission, got %s (pattern=%s)",
					tc.line, EventTypeName(ev.Type), ev.Pattern)
			}
		})
	}
}

func TestParse_ModelSelect(t *testing.T) {
	t.Parallel()
	p := NewParser()

	tests := []struct {
		name string
		line string
	}{
		{"select a model", "Select a model: claude-sonnet-4-20250514, opus"},
		{"select model", "Please select model from the list"},
		{"choose a model", "Choose a model to use"},
		{"choose model", "Choose model for this task"},
		{"available models", "Available models: sonnet, opus, haiku"},
		{"available model colon", "Available model: sonnet"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := p.Parse(tc.line)
			if ev.Type != EventModelSelect {
				t.Errorf("line %q: expected EventModelSelect, got %s (pattern=%s)",
					tc.line, EventTypeName(ev.Type), ev.Pattern)
			}
		})
	}
}

func TestParse_ModelItemSelected(t *testing.T) {
	t.Parallel()
	p := NewParser()

	tests := []struct {
		name          string
		line          string
		wantModelName string
	}{
		{"arrow selected", "❯ claude-sonnet-4-20250514", "claude-sonnet-4-20250514"},
		{"gt selected", "> llama3.2", "llama3.2"},
		{"indented arrow", "  ❯ model-name", "model-name"},
		{"indented gt", "  > codellama:7b", "codellama:7b"},
		{"trailing space", "> model-with-trailing   ", "model-with-trailing"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := p.Parse(tc.line)
			if ev.Type != EventModelSelect {
				t.Errorf("line %q: expected EventModelSelect, got %s (pattern=%s)",
					tc.line, EventTypeName(ev.Type), ev.Pattern)
			}
			if ev.Pattern != "model-item-selected" {
				t.Errorf("expected pattern 'model-item-selected', got %q", ev.Pattern)
			}
			if ev.Fields == nil {
				t.Fatalf("expected non-nil Fields for model-item-selected")
			}
			if got := ev.Fields["modelName"]; got != tc.wantModelName {
				t.Errorf("expected modelName=%q, got %q", tc.wantModelName, got)
			}
			if got := ev.Fields["selected"]; got != "true" {
				t.Errorf("expected selected='true', got %q", got)
			}
		})
	}
}

func TestParse_SSOLogin(t *testing.T) {
	t.Parallel()
	p := NewParser()

	tests := []struct {
		name string
		line string
	}{
		{"opening browser", "Opening your browser for SSO authentication..."},
		{"open browser", "Open your browser to complete login"},
		{"open browser no your", "Open browser to authenticate"},
		{"sso flow", "SSO flow required for this organization"},
		{"oauth required", "OAuth required for API access"},
		{"login required", "Login required to continue"},
		{"sign in needed", "Sign in needed for this workspace"},
		{"signin needed", "Signin needed"},
		{"authentication required", "Authentication required to access this resource"},
		{"visit url", "Visit https://auth.example.com/login to authenticate"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := p.Parse(tc.line)
			if ev.Type != EventSSOLogin {
				t.Errorf("line %q: expected EventSSOLogin, got %s (pattern=%s)",
					tc.line, EventTypeName(ev.Type), ev.Pattern)
			}
		})
	}
}

func TestParse_Completion(t *testing.T) {
	t.Parallel()
	p := NewParser()

	tests := []struct {
		name string
		line string
	}{
		{"task completed", "Task completed successfully"},
		{"task complete", "Task complete"},
		{"task finished", "Task finished with no errors"},
		{"task done", "Task done"},
		{"operation completed", "Operation completed in 3.2s"},
		{"operation finished", "Operation finished successfully"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := p.Parse(tc.line)
			if ev.Type != EventCompletion {
				t.Errorf("line %q: expected EventCompletion, got %s (pattern=%s)",
					tc.line, EventTypeName(ev.Type), ev.Pattern)
			}
		})
	}
}

func TestParse_ToolUse(t *testing.T) {
	t.Parallel()
	p := NewParser()

	tests := []struct {
		name      string
		line      string
		wantField string
		wantValue string
	}{
		{"calling tool colon", "Calling tool: readFile", "toolName", "readFile"},
		{"calling tool no colon", "calling tool writeFile", "toolName", "writeFile"},
		{"calling tool mcp", "Calling tool: mcp_github_search_code", "toolName", "mcp_github_search_code"},
		{"tool result", "Tool result: success", "result", "success"},
		{"tool result colon", "tool result: file contents here", "result", "file contents here"},
		{"tool result empty", "Tool result:", "result", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := p.Parse(tc.line)
			if ev.Type != EventToolUse {
				t.Errorf("line %q: expected EventToolUse, got %s (pattern=%s)",
					tc.line, EventTypeName(ev.Type), ev.Pattern)
			}
			if ev.Fields == nil {
				t.Fatalf("expected Fields to be non-nil for %s", tc.name)
			}
			if got := ev.Fields[tc.wantField]; got != tc.wantValue {
				t.Errorf("expected %s=%q, got %q", tc.wantField, tc.wantValue, got)
			}
		})
	}
}

func TestParse_Error(t *testing.T) {
	t.Parallel()
	p := NewParser()

	tests := []struct {
		name        string
		line        string
		wantMessage string
	}{
		{"error prefix", "Error: file not found", "file not found"},
		{"error colon", "error: connection refused", "connection refused"},
		{"fatal prefix", "Fatal: out of memory", "out of memory"},
		{"fatal colon", "fatal: not a git repository", "not a git repository"},
		{"panic prefix", "panic: runtime error: index out of range", "runtime error: index out of range"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := p.Parse(tc.line)
			if ev.Type != EventError {
				t.Errorf("line %q: expected EventError, got %s (pattern=%s)",
					tc.line, EventTypeName(ev.Type), ev.Pattern)
			}
			if ev.Fields == nil {
				t.Fatalf("expected Fields to be non-nil for %s", tc.name)
			}
			if got := ev.Fields["message"]; got != tc.wantMessage {
				t.Errorf("expected message=%q, got %q", tc.wantMessage, got)
			}
		})
	}
}

func TestParse_Thinking(t *testing.T) {
	t.Parallel()
	p := NewParser()

	tests := []struct {
		name string
		line string
	}{
		{"thinking dots", "Thinking..."},
		{"analyzing dots", "Analyzing..."},
		{"processing dots", "Processing...."},
		{"thinking many dots", "thinking......."},
		{"spinner braille 1", "\u280b Processing your request"},
		{"spinner braille 2", "\u2819 Loading files"},
		{"spinner braille 3", "\u2839 Analyzing code"},
		{"spinner braille 4", "\u2838"},
		{"spinner braille 5", "\u283c"},
		{"spinner braille 6", "\u2834"},
		{"spinner braille 7", "\u2826"},
		{"spinner braille 8", "\u2827"},
		{"spinner braille 9", "\u2807"},
		{"spinner braille 10", "\u280f"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := p.Parse(tc.line)
			if ev.Type != EventThinking {
				t.Errorf("line %q: expected EventThinking, got %s (pattern=%s)",
					tc.line, EventTypeName(ev.Type), ev.Pattern)
			}
		})
	}
}

func TestParse_NoMatch_ReturnsEventText(t *testing.T) {
	t.Parallel()
	p := NewParser()

	tests := []struct {
		name string
		line string
	}{
		{"regular output", "Hello, world!"},
		{"code output", "func main() {"},
		{"file path", "/usr/local/bin/osm"},
		{"json output", `{"key": "value"}`},
		{"blank line", ""},
		{"whitespace only", "   "},
		{"tab only", "\t"},
		{"number only", "42"},
		{"thinking one dot", "Thinking."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := p.Parse(tc.line)
			if ev.Type != EventText {
				t.Errorf("line %q: expected EventText, got %s (pattern=%s)",
					tc.line, EventTypeName(ev.Type), ev.Pattern)
			}
			if ev.Pattern != "" {
				t.Errorf("expected empty pattern for EventText, got %q", ev.Pattern)
			}
			if ev.Line != tc.line {
				t.Errorf("expected line %q, got %q", tc.line, ev.Line)
			}
		})
	}
}

func TestParse_EmptyAndWhitespace(t *testing.T) {
	t.Parallel()
	p := NewParser()

	ev := p.Parse("")
	if ev.Type != EventText {
		t.Errorf("empty string: expected EventText, got %s", EventTypeName(ev.Type))
	}

	ev = p.Parse("   ")
	if ev.Type != EventText {
		t.Errorf("whitespace: expected EventText, got %s", EventTypeName(ev.Type))
	}
}

func TestAddPattern_Valid(t *testing.T) {
	t.Parallel()
	p := NewParser()

	err := p.AddPattern("custom-warn", `(?i)^warning:`, EventError)
	if err != nil {
		t.Fatalf("AddPattern returned unexpected error: %v", err)
	}

	ev := p.Parse("Warning: deprecated feature used")
	if ev.Type != EventError {
		t.Errorf("expected EventError from custom pattern, got %s", EventTypeName(ev.Type))
	}
	if ev.Pattern != "custom-warn" {
		t.Errorf("expected pattern name 'custom-warn', got %q", ev.Pattern)
	}
}

func TestAddPattern_InvalidRegex(t *testing.T) {
	t.Parallel()
	p := NewParser()

	err := p.AddPattern("bad-pattern", `(?i)unclosed(`, EventError)
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
}

func TestAddPattern_AppendedAfterBuiltin(t *testing.T) {
	t.Parallel()
	p := NewParser()

	builtinCount := len(p.patterns)
	err := p.AddPattern("custom", `custom-marker`, EventCompletion)
	if err != nil {
		t.Fatalf("AddPattern returned unexpected error: %v", err)
	}
	if len(p.patterns) != builtinCount+1 {
		t.Errorf("expected %d patterns after add, got %d", builtinCount+1, len(p.patterns))
	}
}

func TestPatterns_BuiltinReturned(t *testing.T) {
	t.Parallel()
	p := NewParser()

	infos := p.Patterns()
	if len(infos) == 0 {
		t.Fatal("Patterns() returned empty slice for fresh parser")
	}
	// All builtins should have non-empty names and patterns.
	for i, info := range infos {
		if info.Name == "" {
			t.Errorf("Patterns()[%d]: empty name", i)
		}
		if info.Pattern == "" {
			t.Errorf("Patterns()[%d] %q: empty pattern", i, info.Name)
		}
	}
}

func TestPatterns_IncludesCustom(t *testing.T) {
	t.Parallel()
	p := NewParser()

	builtinCount := len(p.Patterns())

	err := p.AddPattern("custom-xyz", `xyz-marker`, EventCompletion)
	if err != nil {
		t.Fatalf("AddPattern: %v", err)
	}

	infos := p.Patterns()
	if len(infos) != builtinCount+1 {
		t.Fatalf("expected %d patterns, got %d", builtinCount+1, len(infos))
	}

	last := infos[len(infos)-1]
	if last.Name != "custom-xyz" {
		t.Errorf("last pattern name = %q, want %q", last.Name, "custom-xyz")
	}
	if last.Pattern != "xyz-marker" {
		t.Errorf("last pattern regex = %q, want %q", last.Pattern, "xyz-marker")
	}
	if last.EventType != EventCompletion {
		t.Errorf("last pattern type = %d, want EventCompletion(%d)", last.EventType, EventCompletion)
	}
}

func TestEventTypeName_AllTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		typ  EventType
		want string
	}{
		{EventText, "Text"},
		{EventRateLimit, "RateLimit"},
		{EventPermission, "Permission"},
		{EventModelSelect, "ModelSelect"},
		{EventSSOLogin, "SSOLogin"},
		{EventCompletion, "Completion"},
		{EventToolUse, "ToolUse"},
		{EventError, "Error"},
		{EventThinking, "Thinking"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			got := EventTypeName(tc.typ)
			if got != tc.want {
				t.Errorf("EventTypeName(%d) = %q, want %q", int(tc.typ), got, tc.want)
			}
		})
	}
}

func TestEventTypeName_Unknown(t *testing.T) {
	t.Parallel()
	got := EventTypeName(EventType(999))
	if got != "Unknown(999)" {
		t.Errorf("EventTypeName(999) = %q, want %q", got, "Unknown(999)")
	}
}

func TestParse_PatternPrecedence_FirstMatchWins(t *testing.T) {
	t.Parallel()
	p := NewParser()

	// "Rate limited. Please try again in 30 seconds." should match
	// "rate-limit-try-again" (first in pattern list) rather than
	// "rate-limit-keyword" or "rate-limit-please-wait".
	ev := p.Parse("Rate limited. Please try again in 30 seconds.")
	if ev.Pattern != "rate-limit-try-again" {
		t.Errorf("expected first-match pattern 'rate-limit-try-again', got %q", ev.Pattern)
	}
	if ev.Fields == nil || ev.Fields["retryAfter"] != "30" {
		t.Error("expected retryAfter=30 from first-match extraction")
	}
}

func TestParse_CapturedOutputSamples(t *testing.T) {
	t.Parallel()
	p := NewParser()

	tests := []struct {
		name     string
		line     string
		wantType EventType
	}{
		{"rate limit sample", "Rate limited. Please try again in 30 seconds.", EventRateLimit},
		{"permission sample", "Do you want to allow write access to /etc/passwd? [Y/n]", EventPermission},
		{"model select sample", "Select a model: claude-sonnet-4-20250514, opus", EventModelSelect},
		{"sso sample", "Opening your browser for SSO authentication...", EventSSOLogin},
		{"tool use sample", "Calling tool: readFile", EventToolUse},
		{"error sample", "Error: file not found", EventError},
		{"thinking sample", "Thinking...", EventThinking},
		{"spinner sample", "\u280b Processing your request", EventThinking},
		{"completion sample", "Task completed successfully", EventCompletion},
		{"regular output", "The quick brown fox jumps over the lazy dog", EventText},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := p.Parse(tc.line)
			if ev.Type != tc.wantType {
				t.Errorf("line %q: expected %s, got %s (pattern=%s)",
					tc.line, EventTypeName(tc.wantType), EventTypeName(ev.Type), ev.Pattern)
			}
		})
	}
}

func TestParse_FieldsNilForNoExtract(t *testing.T) {
	t.Parallel()
	p := NewParser()

	// "rate limit" keyword pattern has no extract function.
	ev := p.Parse("rate limited by the API")
	if ev.Type != EventRateLimit {
		t.Fatalf("expected EventRateLimit, got %s", EventTypeName(ev.Type))
	}
	// Fields should be nil when no extract function is defined.
	if ev.Fields != nil {
		t.Errorf("expected nil Fields for pattern without extract, got %v", ev.Fields)
	}
}

func TestParse_ErrorNotMatchedMidLine(t *testing.T) {
	t.Parallel()
	p := NewParser()

	// Error patterns require ^ anchor, so mid-line shouldn't match.
	ev := p.Parse("something Error: not at start")
	if ev.Type == EventError {
		t.Error("Error pattern with ^ anchor should not match mid-line text")
	}
}

func TestParse_CaseInsensitivity(t *testing.T) {
	t.Parallel()
	p := NewParser()

	tests := []struct {
		line     string
		wantType EventType
	}{
		{"RATE LIMIT exceeded", EventRateLimit},
		{"Rate Limit warning", EventRateLimit},
		{"THINKING...", EventThinking},
		{"ERROR: something", EventError},
		{"error: something", EventError},
		{"FATAL: crash", EventError},
		{"TASK COMPLETED", EventCompletion},
	}

	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			t.Parallel()
			ev := p.Parse(tc.line)
			if ev.Type != tc.wantType {
				t.Errorf("line %q: expected %s, got %s",
					tc.line, EventTypeName(tc.wantType), EventTypeName(ev.Type))
			}
		})
	}
}
