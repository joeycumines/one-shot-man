package claudemux

import (
	"context"
	"testing"
	"time"
)

// BenchmarkParseOutput measures parser throughput for various line types.
func BenchmarkParseOutput(b *testing.B) {
	parser := NewParser()

	lines := []struct {
		name string
		line string
	}{
		{"text_short", "hello world"},
		{"text_long", "this is a much longer line of text that represents typical console output from a running process that might span many characters on the terminal"},
		{"rate_limit", "Try again in 30 seconds"},
		{"permission", "Allow tool access? [Y/n]"},
		{"model_select", "Select a model"},
		{"error", "Error: file not found in /path/to/file.go"},
		{"thinking", "Thinking..."},
		{"tool_use", "Calling tool: readFile with args"},
		{"no_match", "some random output that matches nothing specific"},
	}

	for _, l := range lines {
		b.Run(l.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = parser.Parse(l.line)
			}
		})
	}
}

// BenchmarkGuardProcessEvent measures guard evaluation throughput.
func BenchmarkGuardProcessEvent(b *testing.B) {
	guard := NewGuard(DefaultGuardConfig())
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	events := []struct {
		name  string
		event OutputEvent
	}{
		{"text", OutputEvent{Type: EventText, Line: "normal output"}},
		{"rate_limit", OutputEvent{Type: EventRateLimit, Line: "Try again in 30 seconds", Fields: map[string]string{"retryAfter": "30"}}},
		{"permission", OutputEvent{Type: EventPermission, Line: "Allow? [y/N]"}},
		{"error", OutputEvent{Type: EventError, Line: "Error: something failed"}},
		{"thinking", OutputEvent{Type: EventThinking, Line: "Thinking..."}},
	}

	for _, e := range events {
		b.Run(e.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				t := now.Add(time.Duration(i) * time.Millisecond)
				_ = guard.ProcessEvent(e.event, t)
			}
		})
	}
}

// BenchmarkMCPGuardProcessToolCall measures MCP guard evaluation throughput.
func BenchmarkMCPGuardProcessToolCall(b *testing.B) {
	guard := NewMCPGuard(DefaultMCPGuardConfig())
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	calls := []struct {
		name string
		call MCPToolCall
	}{
		{"simple", MCPToolCall{ToolName: "readFile", Arguments: `{"path":"/tmp/test.go"}`}},
		{"empty_args", MCPToolCall{ToolName: "listFiles", Arguments: ""}},
		{"large_args", MCPToolCall{ToolName: "writeFile", Arguments: `{"path":"/tmp/out.go","content":"` + string(make([]byte, 1024)) + `"}`}},
	}

	for _, c := range calls {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				call := c.call
				call.Timestamp = now.Add(time.Duration(i) * time.Millisecond)
				_ = guard.ProcessToolCall(call)
			}
		})
	}
}

// BenchmarkSafetyClassify measures safety validator throughput.
func BenchmarkSafetyClassify(b *testing.B) {
	cfg := DefaultSafetyConfig()
	validator := NewSafetyValidator(cfg)

	actions := []struct {
		name   string
		action SafetyAction
	}{
		{"safe_read", SafetyAction{Type: "tool_call", Name: "readFile", Raw: "/tmp/test.go"}},
		{"destructive", SafetyAction{Type: "command", Name: "exec", Raw: "rm -rf /"}},
		{"network", SafetyAction{Type: "tool_call", Name: "fetch", Raw: "https://example.com"}},
		{"credential", SafetyAction{Type: "tool_call", Name: "env_get", Raw: "AWS_SECRET_ACCESS_KEY"}},
		{"unknown", SafetyAction{Type: "tool_call", Name: "customTool", Raw: "arbitrary input"}},
	}

	for _, a := range actions {
		b.Run(a.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = validator.Validate(a.action)
			}
		})
	}
}

// BenchmarkPoolAcquireRelease measures pool acquire/release cycle throughput.
func BenchmarkPoolAcquireRelease(b *testing.B) {
	pool := NewPool(DefaultPoolConfig())
	if err := pool.Start(); err != nil {
		b.Fatal(err)
	}
	defer pool.Close()

	w, err := pool.AddWorker("bench-worker", nil)
	if err != nil {
		b.Fatal(err)
	}
	_ = w

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		worker, _ := pool.TryAcquire()
		if worker != nil {
			pool.Release(worker, nil, now)
		}
	}
}

// BenchmarkManagedSessionProcessLine measures the full pipeline throughput
// (parse → guard → state update → callbacks).
func BenchmarkManagedSessionProcessLine(b *testing.B) {
	ctx := context.Background()
	cfg := DefaultManagedSessionConfig()
	session := NewManagedSession(ctx, "bench", cfg)
	if err := session.Start(); err != nil {
		b.Fatal(err)
	}
	defer session.Close()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	lines := []struct {
		name string
		line string
	}{
		{"text", "normal output line"},
		{"rate_limit", "Try again in 30 seconds"},
		{"error", "Error: something failed"},
	}

	for _, l := range lines {
		b.Run(l.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				t := now.Add(time.Duration(i) * time.Millisecond)
				_ = session.ProcessLine(l.line, t)
			}
		})
	}
}

// BenchmarkPanelAppendOutput measures panel output append throughput.
func BenchmarkPanelAppendOutput(b *testing.B) {
	panel := NewPanel(DefaultPanelConfig())
	if err := panel.Start(); err != nil {
		b.Fatal(err)
	}
	defer panel.Close()

	panel.AddPane("bench", "Bench Pane")

	texts := []struct {
		name string
		text string
	}{
		{"short", "hello"},
		{"medium", "this is a medium length line of output from a running process"},
		{"long", string(make([]byte, 1024))},
	}

	for _, t := range texts {
		b.Run(t.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				panel.AppendOutput("bench", t.text)
			}
		})
	}
}

// BenchmarkChoiceResolverAnalyze measures analysis throughput.
func BenchmarkChoiceResolverAnalyze(b *testing.B) {
	cfg := DefaultChoiceConfig()
	resolver := NewChoiceResolver(cfg)

	candidates := []Candidate{
		{ID: "a", Name: "Alpha", Attributes: map[string]string{"complexity": "0.3", "risk": "0.2", "maintainability": "0.8", "performance": "0.7"}},
		{ID: "b", Name: "Beta", Attributes: map[string]string{"complexity": "0.5", "risk": "0.4", "maintainability": "0.6", "performance": "0.8"}},
		{ID: "c", Name: "Gamma", Attributes: map[string]string{"complexity": "0.7", "risk": "0.6", "maintainability": "0.4", "performance": "0.5"}},
	}

	sizes := []struct {
		name       string
		candidates []Candidate
	}{
		{"2_candidates", candidates[:2]},
		{"3_candidates", candidates},
		{"10_candidates", func() []Candidate {
			var cs []Candidate
			for i := range 10 {
				cs = append(cs, Candidate{
					ID:         string(rune('a' + i)),
					Name:       string(rune('A' + i)),
					Attributes: map[string]string{"complexity": "0.5", "risk": "0.5", "maintainability": "0.5", "performance": "0.5"},
				})
			}
			return cs
		}()},
	}

	for _, s := range sizes {
		b.Run(s.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = resolver.Analyze(s.candidates, nil, nil)
			}
		})
	}
}
