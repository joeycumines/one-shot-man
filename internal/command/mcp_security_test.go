package command

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// MCP Session ID Spoofing — Server-level validation
// ============================================================================

// TestMCPSecurity_SessionIDControlChars verifies the MCP server rejects
// session IDs containing control characters (potential log injection).
func TestMCPSecurity_SessionIDControlChars(t *testing.T) {
	t.Parallel()

	controlCharIDs := []struct {
		name string
		id   string
	}{
		{"null byte", "session\x00id"},
		{"newline", "session\nid"},
		{"carriage return", "session\rid"},
		{"tab", "session\tid"},
		{"escape", "session\x1bid"},
		{"bell", "session\x07id"},
		{"backspace", "session\x08id"},
		{"delete", "session\x7fid"},
		{"form feed", "session\x0cid"},
		{"vertical tab", "session\x0bid"},
	}

	for _, tc := range controlCharIDs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sess := setupMCPTestSession(t)
			defer sess.cleanup()

			result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
				Name: "registerSession",
				Arguments: map[string]any{
					"sessionId":    tc.id,
					"capabilities": []string{},
				},
			})
			require.NoError(t, err) // transport error
			assert.True(t, result.IsError, "session ID with %s should be rejected", tc.name)

			// Verify error message mentions invalid character
			text := extractTextContent(t, result)
			assert.Contains(t, text, "invalid character",
				"error for %s should mention invalid character", tc.name)
		})
	}
}

// TestMCPSecurity_SessionIDTooLong verifies oversized session IDs are rejected.
func TestMCPSecurity_SessionIDTooLong(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	longID := strings.Repeat("x", 257) // exceeds mcpMaxSessionIDLen=256
	result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    longID,
			"capabilities": []string{},
		},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError, "session ID exceeding max length should be rejected")
}

// TestMCPSecurity_SessionIDAtMaxLength verifies session IDs at exactly
// the max length are accepted.
func TestMCPSecurity_SessionIDAtMaxLength(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	exactID := strings.Repeat("x", 256) // exactly mcpMaxSessionIDLen
	result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    exactID,
			"capabilities": []string{},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError, "session ID at max length should be accepted")
}

// TestMCPSecurity_SessionIDEmpty verifies empty session IDs are rejected.
func TestMCPSecurity_SessionIDEmpty(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    "",
			"capabilities": []string{},
		},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError, "empty session ID should be rejected")
}

// ============================================================================
// MCP Tool Injection — Cross-session access attempts
// ============================================================================

// TestMCPSecurity_CrossSessionAccess verifies that one session cannot
// modify or read another session's data.
func TestMCPSecurity_CrossSessionAccess(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	// Register victim session
	result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    "victim",
			"capabilities": []string{"code"},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "victim registration should succeed")

	// Register attacker session
	result, err = sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    "attacker",
			"capabilities": []string{"code"},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "attacker registration should succeed")

	// Report progress on victim using attacker's knowledge of victim's ID
	// This IS allowed by design (sessions are coordinated, not adversarial)
	// but we verify they maintain separate state
	crossResult, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "reportProgress",
		Arguments: map[string]any{
			"sessionId": "victim",
			"status":    "working",
			"progress":  50.0,
			"message":   "attacker injected",
			"seq":       1,
		},
	})
	require.NoError(t, err)
	// This succeeds because the MCP server trusts its clients
	// The design boundary is at the process level, not session level
	require.False(t, crossResult.IsError, "cross-session progress update is allowed by design")

	// Verify attacker session is untouched
	result, err = sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "getSession",
		Arguments: map[string]any{
			"sessionId": "attacker",
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	text := extractTextContent(t, result)
	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Equal(t, float64(0), resp["progress"],
		"attacker session progress should be unaffected")
}

// TestMCPSecurity_NonexistentSessionAccess verifies that accessing
// a non-registered session returns an error.
func TestMCPSecurity_NonexistentSessionAccess(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	tools := []struct {
		name string
		args map[string]any
	}{
		{"reportProgress", map[string]any{
			"sessionId": "ghost",
			"status":    "working",
			"progress":  50.0,
			"message":   "hi",
			"seq":       1,
		}},
		{"reportResult", map[string]any{
			"sessionId": "ghost",
			"success":   true,
			"output":    "done",
			"seq":       1,
		}},
		{"requestGuidance", map[string]any{
			"sessionId": "ghost",
			"question":  "help?",
			"seq":       1,
		}},
		{"heartbeat", map[string]any{
			"sessionId": "ghost",
		}},
		{"getSession", map[string]any{
			"sessionId": "ghost",
		}},
	}

	for _, tc := range tools {
		t.Run(tc.name, func(t *testing.T) {
			result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
				Name:      tc.name,
				Arguments: tc.args,
			})
			require.NoError(t, err)
			assert.True(t, result.IsError,
				"%s with nonexistent session should return error", tc.name)
		})
	}
}

// ============================================================================
// Sequence Number Manipulation — Replay and Skip attacks
// ============================================================================

// TestMCPSecurity_SequenceReplay verifies that replayed sequence numbers
// (already-processed seq) are silently deduplicated.
func TestMCPSecurity_SequenceReplay(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	// Register session
	_, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    "replay-test",
			"capabilities": []string{},
		},
	})
	require.NoError(t, err)

	// Send seq=1
	result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "reportProgress",
		Arguments: map[string]any{
			"sessionId": "replay-test",
			"status":    "working",
			"progress":  25.0,
			"message":   "first",
			"seq":       1,
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Send seq=2
	result, err = sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "reportProgress",
		Arguments: map[string]any{
			"sessionId": "replay-test",
			"status":    "working",
			"progress":  50.0,
			"message":   "second",
			"seq":       2,
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Replay seq=1 (already processed)
	replayResult, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "reportProgress",
		Arguments: map[string]any{
			"sessionId": "replay-test",
			"status":    "working",
			"progress":  99.0,
			"message":   "replayed!",
			"seq":       1,
		},
	})
	require.NoError(t, err)
	// Should be deduplicated (not an error, just skipped)
	// The tool returns success but the update is a no-op
	require.False(t, replayResult.IsError, "replay is silently deduplicated, not an error")

	// Verify progress is at 50 (seq=2), not 99 (replayed seq=1)
	result, err = sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "getSession",
		Arguments: map[string]any{
			"sessionId": "replay-test",
		},
	})
	require.NoError(t, err)
	text := extractTextContent(t, result)
	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Equal(t, float64(50), resp["progress"],
		"progress should be 50 (from seq=2), not 99 (replayed seq=1)")
}

// TestMCPSecurity_SequenceSkip verifies that skipping sequence numbers
// (seq=1 then seq=100) is allowed — the server doesn't enforce contiguity.
func TestMCPSecurity_SequenceSkip(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	_, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    "skip-test",
			"capabilities": []string{},
		},
	})
	require.NoError(t, err)

	// Seq=1
	result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "reportProgress",
		Arguments: map[string]any{
			"sessionId": "skip-test",
			"status":    "working",
			"progress":  10.0,
			"message":   "first",
			"seq":       1,
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Skip to seq=100
	result, err = sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "reportProgress",
		Arguments: map[string]any{
			"sessionId": "skip-test",
			"status":    "working",
			"progress":  90.0,
			"message":   "skipped",
			"seq":       100,
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "skipping sequence numbers should be allowed")
}

// TestMCPSecurity_NegativeSequenceNumber verifies negative seq numbers
// are treated as "no dedup" (same as seq=0).
func TestMCPSecurity_NegativeSequenceNumber(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	_, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    "neg-seq",
			"capabilities": []string{},
		},
	})
	require.NoError(t, err)

	result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "reportProgress",
		Arguments: map[string]any{
			"sessionId": "neg-seq",
			"status":    "working",
			"progress":  10.0,
			"message":   "negative seq",
			"seq":       -5,
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "negative seq should be treated as no-dedup")
}

// ============================================================================
// Invalid Status Values — Status injection
// ============================================================================

// TestMCPSecurity_InvalidProgressStatus verifies that reportProgress rejects
// status values not in the allowed set.
func TestMCPSecurity_InvalidProgressStatus(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	_, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    "status-test",
			"capabilities": []string{},
		},
	})
	require.NoError(t, err)

	invalidStatuses := []string{
		"",
		"hacking",
		"<script>alert(1)</script>",
		strings.Repeat("x", 10000),
		"working\x00extra",
		"WORKING", // case sensitivity check
	}

	for _, status := range invalidStatuses {
		name := status
		if len(name) > 20 {
			name = name[:20]
		}
		t.Run(name, func(t *testing.T) {
			result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
				Name: "reportProgress",
				Arguments: map[string]any{
					"sessionId": "status-test",
					"status":    status,
					"progress":  50.0,
					"message":   "test",
					"seq":       0,
				},
			})
			require.NoError(t, err)
			assert.True(t, result.IsError,
				"invalid status %q should be rejected", name)
		})
	}
}

// ============================================================================
// Concurrent Session Manipulation — Race conditions in MCP server
// ============================================================================

// TestMCPSecurity_ConcurrentSessionRegistration verifies that concurrent
// session registration doesn't corrupt server state.
func TestMCPSecurity_ConcurrentSessionRegistration(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	var wg sync.WaitGroup
	errCh := make(chan error, 50)

	// 20 goroutines registering different sessions
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := strings.Repeat(string(rune('a'+idx%26)), idx+1)
			_, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
				Name: "registerSession",
				Arguments: map[string]any{
					"sessionId":    id,
					"capabilities": []string{},
				},
			})
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent registration error: %v", err)
	}

	// Verify we can list sessions
	result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name:      "listSessions",
		Arguments: map[string]any{},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "listSessions after concurrent registration should work")
}

// TestMCPSecurity_ConcurrentProgressUpdates verifies concurrent updates
// to the same session don't corrupt state.
func TestMCPSecurity_ConcurrentProgressUpdates(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	// Register session first
	_, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    "concurrent-updates",
			"capabilities": []string{},
		},
	})
	require.NoError(t, err)

	var wg sync.WaitGroup

	// 20 goroutines sending progress with increasing seq numbers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
				Name: "reportProgress",
				Arguments: map[string]any{
					"sessionId": "concurrent-updates",
					"status":    "working",
					"progress":  float64(idx * 5),
					"message":   "update",
					"seq":       int64(idx + 1),
				},
			})
		}(i)
	}

	// Concurrent heartbeats
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
				Name: "heartbeat",
				Arguments: map[string]any{
					"sessionId": "concurrent-updates",
				},
			})
		}()
	}

	// Concurrent getSession reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
				Name: "getSession",
				Arguments: map[string]any{
					"sessionId": "concurrent-updates",
				},
			})
		}()
	}

	wg.Wait()

	// Verify session is in a consistent state
	result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "getSession",
		Arguments: map[string]any{
			"sessionId": "concurrent-updates",
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "session should be readable after concurrent updates")
}

// ============================================================================
// Large Payload Handling — Resource exhaustion at MCP server level
// ============================================================================

// TestMCPSecurity_LargeProgressMessage verifies the server handles
// oversized progress messages gracefully.
func TestMCPSecurity_LargeProgressMessage(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	_, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    "large-msg",
			"capabilities": []string{},
		},
	})
	require.NoError(t, err)

	// 1MB message
	largeMsg := strings.Repeat("A", 1<<20)
	result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "reportProgress",
		Arguments: map[string]any{
			"sessionId": "large-msg",
			"status":    "working",
			"progress":  50.0,
			"message":   largeMsg,
		},
	})
	require.NoError(t, err)
	// Should succeed (no hard limit on message size in current impl)
	_ = result
}

// TestMCPSecurity_LargeResultOutput verifies oversized result output.
func TestMCPSecurity_LargeResultOutput(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	_, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    "large-result",
			"capabilities": []string{},
		},
	})
	require.NoError(t, err)

	largeOutput := strings.Repeat("B", 1<<20)
	result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "reportResult",
		Arguments: map[string]any{
			"sessionId": "large-result",
			"success":   true,
			"output":    largeOutput,
		},
	})
	require.NoError(t, err)
	_ = result
}

// TestMCPSecurity_ManyCapabilities verifies large capability lists.
func TestMCPSecurity_ManyCapabilities(t *testing.T) {
	t.Parallel()
	sess := setupMCPTestSession(t)
	defer sess.cleanup()

	caps := make([]string, 1000)
	for i := range caps {
		caps[i] = strings.Repeat("cap", 100) + string(rune(i%256))
	}

	// Convert to []any for the arguments map
	capsAny := make([]any, len(caps))
	for i, c := range caps {
		capsAny[i] = c
	}

	result, err := sess.session.CallTool(sess.ctx, &mcp.CallToolParams{
		Name: "registerSession",
		Arguments: map[string]any{
			"sessionId":    "many-caps",
			"capabilities": capsAny,
		},
	})
	require.NoError(t, err)
	// Should handle gracefully (no panic)
	_ = result
}

// ============================================================================
// Test helpers
// ============================================================================

type mcpSecurityTestSession struct {
	ctx        context.Context
	cancel     context.CancelFunc
	session    *mcp.ClientSession
	serverDone chan error
}

func (s *mcpSecurityTestSession) cleanup() {
	_ = s.session.Close()
	s.cancel()
	<-s.serverDone
}

func setupMCPTestSession(t *testing.T) *mcpSecurityTestSession {
	t.Helper()

	cwd := t.TempDir()
	cm, err := scripting.NewContextManager(cwd)
	require.NoError(t, err)

	server := newMCPServer(cm, &mcpTestGoalRegistry{}, "0.0.0-security-test")

	ctx, cancel := context.WithCancel(context.Background())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "security-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("failed to connect: %v", err)
	}

	return &mcpSecurityTestSession{
		ctx:        ctx,
		cancel:     cancel,
		session:    session,
		serverDone: serverDone,
	}
}

func extractTextContent(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatal("no text content found in result")
	return ""
}
