package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FuzzMCPBacktickFence fuzzes mcpBacktickFence to verify that the returned
// fence string is always safe for wrapping arbitrary content in a Markdown
// code block. Seeds include empty strings, various backtick runs, and mixed
// content.
func FuzzMCPBacktickFence(f *testing.F) {
	seeds := []string{
		"",
		"hello world",
		"`",
		"``",
		"```",
		"````",
		"`````",
		"```code```",
		"some ``` embedded ``` fences",
		"````code````",
		strings.Repeat("`", 100),
		"mixed ` content `` here ``` and ```` more",
		"no backticks at all",
		"\x00\xff`\n`",
		"日本語`テスト",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, content string) {
		fence := mcpBacktickFence(content)

		// Invariant 1: Result contains only backtick characters.
		for i, r := range fence {
			if r != '`' {
				t.Fatalf("fence contains non-backtick rune %q at position %d; fence=%q content=%q",
					string(r), i, fence, content)
			}
		}

		// Invariant 2: Fence length is at least 3.
		if len(fence) < 3 {
			t.Fatalf("fence length %d < 3; fence=%q content=%q", len(fence), fence, content)
		}

		// Invariant 3: Content does NOT contain a consecutive run of backticks
		// of length >= len(fence). This ensures the fence safely wraps the content.
		fenceLen := len(fence)
		run := 0
		for _, r := range content {
			if r == '`' {
				run++
				if run >= fenceLen {
					t.Fatalf("content contains backtick run of length >= %d; fence=%q content=%q",
						fenceLen, fence, content)
				}
			} else {
				run = 0
			}
		}
	})
}

// FuzzMCPSessionTools fuzzes the session-related MCP tools (registerSession,
// reportProgress, reportResult, requestGuidance, heartbeat) with random inputs
// to ensure no panics and consistent error handling.
func FuzzMCPSessionTools(f *testing.F) {
	seeds := []struct {
		tool    string
		payload string
	}{
		{"registerSession", `{"sessionId":"s1","capabilities":[]}`},
		{"registerSession", `{"sessionId":"","capabilities":[]}`},
		{"registerSession", `{"sessionId":"` + strings.Repeat("x", 300) + `","capabilities":[]}`},
		{"reportProgress", `{"sessionId":"s1","status":"working","progress":50,"message":"hi","seq":1}`},
		{"reportProgress", `{"sessionId":"s1","status":"invalid","progress":-1,"message":"","seq":0}`},
		{"reportResult", `{"sessionId":"s1","success":true,"output":"done","seq":1}`},
		{"reportResult", `{"sessionId":"ghost","success":false,"output":"fail","seq":5}`},
		{"requestGuidance", `{"sessionId":"s1","question":"what?","options":["a","b"],"seq":1}`},
		{"requestGuidance", `{"sessionId":"s1","question":"","seq":0}`},
		{"heartbeat", `{"sessionId":"s1"}`},
		{"heartbeat", `{"sessionId":""}`},
		{"heartbeat", `{"sessionId":"ghost"}`},
		{"getSession", `{"sessionId":"s1"}`},
		{"getSession", `{"sessionId":"nonexistent"}`},
	}
	for _, s := range seeds {
		f.Add(s.tool, s.payload)
	}

	f.Fuzz(func(t *testing.T, tool, payload string) {
		// Only fuzz the session tools; skip unknown tool names entirely.
		allowed := map[string]bool{
			"registerSession": true,
			"reportProgress":  true,
			"reportResult":    true,
			"requestGuidance": true,
			"heartbeat":       true,
			"getSession":      true,
			"listSessions":    true,
		}
		if !allowed[tool] {
			return
		}

		cwd := t.TempDir()
		cm, err := scripting.NewContextManager(cwd)
		if err != nil {
			t.Skip("failed to create context manager:", err)
		}
		server := newMCPServer(cm, &mcpTestGoalRegistry{}, "0.0.0-fuzz", "", "")

		// Per-iteration timeout prevents the fuzz engine from blocking
		// on cleanup when the fuzztime expires mid-iteration.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		serverTransport, clientTransport := mcp.NewInMemoryTransports()
		serverDone := make(chan error, 1)
		go func() {
			serverDone <- server.Run(ctx, serverTransport)
		}()

		client := mcp.NewClient(&mcp.Implementation{Name: "fuzz-client", Version: "test"}, nil)
		sess, err := client.Connect(ctx, clientTransport, nil)
		if err != nil {
			cancel()
			t.Skip("failed to connect:", err)
		}
		defer func() {
			_ = sess.Close()
			cancel()
			select {
			case <-serverDone:
			case <-time.After(3 * time.Second):
			}
		}()

		// Ensure there's a session for tools that need one.
		if tool != "registerSession" && tool != "listSessions" {
			_, _ = sess.CallTool(ctx, &mcp.CallToolParams{
				Name: "registerSession",
				Arguments: map[string]any{
					"sessionId":    "fuzz-sess",
					"capabilities": []string{},
				},
			})
		}

		// Parse the fuzz payload as map, falling back to a minimal args map.
		var args map[string]any
		if json.Valid([]byte(payload)) {
			if err := json.Unmarshal([]byte(payload), &args); err != nil {
				args = map[string]any{"sessionId": payload}
			}
		} else {
			args = map[string]any{"sessionId": payload}
		}

		// The key invariant: no panic.
		_, _ = sess.CallTool(ctx, &mcp.CallToolParams{
			Name:      tool,
			Arguments: args,
		})
	})
}

// FuzzMCPReportClassification fuzzes the reportClassification tool with random
// Category array inputs. Seeds include valid arrays, invalid inputs (empty,
// missing fields, duplicates), and boundary cases.
func FuzzMCPReportClassification(f *testing.F) {
	seeds := []string{
		// Valid: 3 categories
		`[{"name":"api","description":"Add API","files":["a.go","b.go"]},{"name":"cli","description":"Add CLI","files":["cmd/main.go"]},{"name":"docs","description":"Update docs","files":["README.md"]}]`,
		// Valid: 1 category
		`[{"name":"all","description":"Single commit","files":["x.go"]}]`,
		// Invalid: empty array
		`[]`,
		// Invalid: missing name
		`[{"name":"","description":"desc","files":["a.go"]}]`,
		// Invalid: missing description
		`[{"name":"api","description":"","files":["a.go"]}]`,
		// Invalid: missing files
		`[{"name":"api","description":"desc","files":[]}]`,
		// Invalid: duplicate files across categories
		`[{"name":"a","description":"A","files":["x.go"]},{"name":"b","description":"B","files":["x.go"]}]`,
		// Invalid: null values
		`[{"name":null,"description":"desc","files":["a.go"]}]`,
		// Boundary: very long name
		`[{"name":"` + strings.Repeat("x", 1000) + `","description":"desc","files":["a.go"]}]`,
		// Boundary: special characters in description
		`[{"name":"api","description":"fix: handle \"edge\" case\nwith newlines\ttabs","files":["a.go"]}]`,
		// Boundary: many files
		`[{"name":"bulk","description":"Bulk import","files":["a.go","b.go","c.go","d.go","e.go","f.go","g.go","h.go","i.go","j.go"]}]`,
		// Invalid: not an array
		`{"name":"api","description":"desc","files":["a.go"]}`,
		// Invalid: malformed JSON
		`[{"name":"api","descr`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, payload string) {
		cwd := t.TempDir()
		cm, err := scripting.NewContextManager(cwd)
		if err != nil {
			t.Skip("failed to create context manager:", err)
		}
		server := newMCPServer(cm, &mcpTestGoalRegistry{}, "0.0.0-fuzz", cwd, "")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		serverTransport, clientTransport := mcp.NewInMemoryTransports()
		serverDone := make(chan error, 1)
		go func() {
			serverDone <- server.Run(ctx, serverTransport)
		}()

		client := mcp.NewClient(&mcp.Implementation{Name: "fuzz-client", Version: "test"}, nil)
		sess, err := client.Connect(ctx, clientTransport, nil)
		if err != nil {
			cancel()
			t.Skip("failed to connect:", err)
		}
		defer func() {
			_ = sess.Close()
			cancel()
			select {
			case <-serverDone:
			case <-time.After(3 * time.Second):
			}
		}()

		// Register a session first (required by reportClassification).
		_, _ = sess.CallTool(ctx, &mcp.CallToolParams{
			Name: "registerSession",
			Arguments: map[string]any{
				"sessionId":    "fuzz-cls",
				"capabilities": []string{},
			},
		})

		// Build arguments: try to parse payload as categories array.
		args := map[string]any{
			"sessionId": "fuzz-cls",
		}
		if json.Valid([]byte(payload)) {
			var categories any
			if err := json.Unmarshal([]byte(payload), &categories); err == nil {
				args["categories"] = categories
			}
		}

		// The key invariant: no panic.
		_, _ = sess.CallTool(ctx, &mcp.CallToolParams{
			Name:      "reportClassification",
			Arguments: args,
		})
	})
}
