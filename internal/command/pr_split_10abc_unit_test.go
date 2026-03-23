package command

// T413: Unit tests for pipeline chunks 10a, 10b, 10c.
// Focused on functions that have zero or fuzz-only coverage.

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
// Chunk 10a — resolveNumber
// ---------------------------------------------------------------------------

func TestChunk10a_ResolveNumber(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	cases := []struct {
		name   string
		expr   string
		expect string
	}{
		{"valid int", `prSplit._resolveNumber(42, 10, 0)`, "42"},
		{"floor float", `prSplit._resolveNumber(42.7, 10, 0)`, "42"},
		{"NaN fallback", `prSplit._resolveNumber(NaN, 10, 0)`, "10"},
		{"Infinity fallback", `prSplit._resolveNumber(Infinity, 10, 0)`, "10"},
		{"-Infinity fallback", `prSplit._resolveNumber(-Infinity, 10, 0)`, "10"},
		{"undefined fallback", `prSplit._resolveNumber(undefined, 10, 0)`, "10"},
		{"null coerces to 0", `prSplit._resolveNumber(null, 10, 0)`, "0"},
		{"string NaN fallback", `prSplit._resolveNumber('not-a-number', 10, 0)`, "10"},
		{"empty string coerces to 0", `prSplit._resolveNumber('', 10, 0)`, "0"},
		{"below min fallback", `prSplit._resolveNumber(5, 10, 20)`, "10"},
		{"above min accepted", `prSplit._resolveNumber(25, 10, 20)`, "25"},
		{"zero no min", `prSplit._resolveNumber(0, 10, undefined)`, "0"},
		{"negative below min=0", `prSplit._resolveNumber(-3, 10, 0)`, "10"},
		{"exactly at min", `prSplit._resolveNumber(20, 10, 20)`, "20"},
		{"bool true (=1)", `prSplit._resolveNumber(true, 10, 0)`, "1"},
		{"bool false (=0)", `prSplit._resolveNumber(false, 10, 0)`, "0"},
		{"numeric string", `prSplit._resolveNumber('42', 10, 0)`, "42"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			val, err := evalJS(`String(` + tc.expr + `)`)
			if err != nil {
				t.Fatalf("eval: %v", err)
			}
			got, _ := val.(string)
			if got != tc.expect {
				t.Errorf("got %q, want %q", got, tc.expect)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Chunk 10a — getCancellationError
// ---------------------------------------------------------------------------

func TestChunk10a_GetCancellationError(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	t.Run("neither cancelled", func(t *testing.T) {
		_, err := evalJS(`
			prSplit.isCancelled = function() { return false; };
			prSplit.isForceCancelled = function() { return false; };
		`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`prSplit._getCancellationError()`)
		if err != nil {
			t.Fatal(err)
		}
		if val != nil {
			t.Errorf("expected null, got %v", val)
		}
	})

	t.Run("normal cancel", func(t *testing.T) {
		_, err := evalJS(`
			prSplit.isCancelled = function() { return true; };
			prSplit.isForceCancelled = function() { return false; };
		`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`prSplit._getCancellationError()`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if !strings.Contains(s, "cancelled") {
			t.Errorf("expected 'cancelled' in %q", s)
		}
		if strings.Contains(s, "force") {
			t.Errorf("normal cancel should not say 'force', got %q", s)
		}
	})

	t.Run("force cancel takes priority", func(t *testing.T) {
		_, err := evalJS(`
			prSplit.isCancelled = function() { return true; };
			prSplit.isForceCancelled = function() { return true; };
		`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`prSplit._getCancellationError()`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if !strings.Contains(s, "force") {
			t.Errorf("expected 'force' in %q", s)
		}
	})

	t.Run("functions not defined", func(t *testing.T) {
		_, err := evalJS(`
			delete prSplit.isCancelled;
			delete prSplit.isForceCancelled;
		`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`prSplit._getCancellationError()`)
		if err != nil {
			t.Fatal(err)
		}
		if val != nil {
			t.Errorf("expected null when functions missing, got %v", val)
		}
	})
}

// ---------------------------------------------------------------------------
// Chunk 10a — isTransientError (deterministic assertions)
// ---------------------------------------------------------------------------

func TestChunk10a_IsTransientError(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	transient := []struct {
		name  string
		input string
	}{
		{"null", "null"},
		{"undefined", "undefined"},
		{"empty string", "''"},
		{"rate limit", "'rate limit exceeded'"},
		{"429", "'429 Too Many Requests'"},
		{"timeout", "'timeout waiting for tool'"},
		{"timed out", "'request timed out'"},
		{"ECONNRESET", "'ECONNRESET'"},
		{"ECONNREFUSED", "'ECONNREFUSED'"},
		{"503", "'503 Service Unavailable'"},
		{"500", "'500 Internal Server Error'"},
		{"overloaded", "'service overloaded'"},
		{"try again", "'please try again later'"},
		{"throttled", "'request was throttled'"},
		{"quota exceeded", "'quota exceeded'"},
		{"unknown error", "'something went wrong'"},
	}

	for _, tc := range transient {
		t.Run("transient/"+tc.name, func(t *testing.T) {
			val, err := evalJS(`prSplit._isTransientError(` + tc.input + `)`)
			if err != nil {
				t.Fatalf("eval: %v", err)
			}
			if val != true {
				t.Errorf("isTransientError(%s) = %v, want true", tc.input, val)
			}
		})
	}

	permanent := []struct {
		name  string
		input string
	}{
		{"invalid tool", "'invalid tool name: foo'"},
		{"malformed schema", "'malformed JSON schema'"},
		{"unknown tool", "'unknown tool requested'"},
		{"argument error", "'argument error: missing field'"},
	}

	for _, tc := range permanent {
		t.Run("permanent/"+tc.name, func(t *testing.T) {
			val, err := evalJS(`prSplit._isTransientError(` + tc.input + `)`)
			if err != nil {
				t.Fatalf("eval: %v", err)
			}
			if val != false {
				t.Errorf("isTransientError(%s) = %v, want false", tc.input, val)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Chunk 10a — AUTOMATED_DEFAULTS new keys (T410/T411)
// ---------------------------------------------------------------------------

func TestChunk10a_AUTOMATED_DEFAULTS_NewKeys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	val, err := evalJS(`JSON.stringify(prSplit.AUTOMATED_DEFAULTS)`)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}

	// T410 keys (orchestrator extraction)
	newKeys := []string{
		`"minPollIntervalMs":50`,
		`"launcherPollMs":200`,
		`"launcherTimeoutMs":10000`,
		`"launcherStableNeed":3`,
		`"launcherPostDismissMs":500`,
		`"planPollTimeoutMs":5000`,
		`"planPollCheckIntervalMs":1000`,
		// T411 keys (09/10c/15b extraction)
		`"spawnHealthCheckDelayMs":300`,
		`"resolveWallClockGraceMs":60000`,
		`"resolveBackoffBaseMs":2000`,
		`"resolveBackoffCapMs":30000`,
	}
	for _, key := range newKeys {
		if !strings.Contains(s, key) {
			t.Errorf("AUTOMATED_DEFAULTS missing %s\ngot: %s", key, s)
		}
	}
}

// ---------------------------------------------------------------------------
// Chunk 10b — sendToHandle multi-chunk text splitting
// ---------------------------------------------------------------------------

func TestChunk10b_SendToHandle_MultiChunkText(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Set small chunk size to force multi-chunk splitting.
	_, err := evalJS(`
		prSplit.SEND_TEXT_CHUNK_BYTES = 100;
		prSplit.SEND_TEXT_CHUNK_DELAY_MS = 0;
		prSplit.SEND_TEXT_NEWLINE_DELAY_MS = 0;
		prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 1;
		prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS = 1;
		prSplit.SEND_PROMPT_READY_TIMEOUT_MS = 1;

		// Mock handle that records sends.
		var __mockSends = [];
		var __mockHandle = {
			send: function(data) { __mockSends.push(data); },
			isAlive: function() { return true; },
			receive: function() { return ''; }
		};
		globalThis.__mockHandle = __mockHandle;
		globalThis.__mockSends = __mockSends;
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Generate a 350-char text → should produce 4 chunks (100+100+100+50).
	_, err = evalJS(`
		var text = '';
		for (var i = 0; i < 350; i++) text += 'x';
		globalThis.__testText = text;
	`)
	if err != nil {
		t.Fatal(err)
	}

	// sendToHandle without tuiMux fallback (no anchor matching).
	_, err = evalJS(`await prSplit.sendToHandle(__mockHandle, __testText)`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify chunk count: 4 text chunks + 1 newline (\r) = 5 sends.
	val, err := evalJS(`__mockSends.length`)
	if err != nil {
		t.Fatal(err)
	}
	count := toInt64(val)
	if count != 5 {
		t.Errorf("expected 5 sends (4 text chunks + 1 \\r), got %d", count)
	}

	// Verify the text chunks concatenate to the original.
	val, err = evalJS(`__mockSends.slice(0, -1).join('')`)
	if err != nil {
		t.Fatal(err)
	}
	joined, _ := val.(string)
	if len(joined) != 350 {
		t.Errorf("concatenated chunks length = %d, want 350", len(joined))
	}

	// Verify the last send is a newline.
	val, err = evalJS(`__mockSends[__mockSends.length - 1]`)
	if err != nil {
		t.Fatal(err)
	}
	last, _ := val.(string)
	if last != "\r" {
		t.Errorf("last send = %q, want '\\r'", last)
	}
}

// ---------------------------------------------------------------------------
// Chunk 10c — exports are functions
// ---------------------------------------------------------------------------

func TestChunk10c_ExportsAreFunctions(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	exports := []string{
		"prSplit.waitForLogged",
		"prSplit.heuristicFallback",
		"prSplit.resolveConflictsWithClaude",
	}
	for _, fn := range exports {
		t.Run(fn, func(t *testing.T) {
			val, err := evalJS(`typeof ` + fn)
			if err != nil {
				t.Fatal(err)
			}
			if val != "function" {
				t.Errorf("typeof %s = %v, want 'function'", fn, val)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Chunk 10c — waitForLogged returns error when MCP callback missing
// ---------------------------------------------------------------------------

func TestChunk10c_WaitForLogged_MissingMCPCallback(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Neither _mcpCallbackObj nor the bare mcpCallbackObj global supports
	// waitForAsync — waitForLogged should return a descriptive error.
	_, err := evalJS(`
		prSplit._mcpCallbackObj = null;
		globalThis.mcpCallbackObj = null;
	`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(await prSplit.waitForLogged('test_tool', 1000, {}))`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	if !strings.Contains(s, "MCP callback missing") {
		t.Errorf("expected MCP callback error, got: %s", s)
	}
}

// ---------------------------------------------------------------------------
// Chunk 10c — exponential backoff cap via AUTOMATED_DEFAULTS
// ---------------------------------------------------------------------------

func TestChunk10c_BackoffUsesAutomatedDefaults(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Verify the backoff constants are accessible.
	val, err := evalJS(`String(prSplit.AUTOMATED_DEFAULTS.resolveBackoffBaseMs)`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "2000" {
		t.Errorf("resolveBackoffBaseMs = %v, want 2000", val)
	}

	val, err = evalJS(`String(prSplit.AUTOMATED_DEFAULTS.resolveBackoffCapMs)`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "30000" {
		t.Errorf("resolveBackoffCapMs = %v, want 30000", val)
	}

	val, err = evalJS(`String(prSplit.AUTOMATED_DEFAULTS.resolveWallClockGraceMs)`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "60000" {
		t.Errorf("resolveWallClockGraceMs = %v, want 60000", val)
	}
}
