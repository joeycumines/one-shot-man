package claudemux

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
	"unicode/utf8"
)

// PromptOpts configures the behavior of a ReliablePrompter.
type PromptOpts struct {
	// ReadyTimeout is the maximum duration to wait for the agent to become
	// ready before sending a prompt. Default: 30s.
	ReadyTimeout time.Duration
	// AcceptTimeout is the maximum duration to wait for the prompt to be
	// accepted after sending. Default: 5s.
	AcceptTimeout time.Duration
	// ResponseTimeout is the maximum duration to wait for a full response
	// after the prompt is accepted. Default: 120s.
	ResponseTimeout time.Duration
	// MaxRetries is the maximum number of retries on rate limit or crash
	// before giving up. Default: 3.
	MaxRetries int
	// PermissionPolicy determines how permission prompts are handled.
	// Uses the canonical PermissionPolicy from guard.go.
	PermissionPolicy PermissionPolicy
	// Detector is an optional VTStateDetector for TUI/PTY mode state detection.
	// When set, readLoop feeds output through the detector for state tracking.
	// When nil (protocol mode), state transitions are derived from classifyLine events.
	Detector *VTStateDetector
	// ChunkSize is the maximum byte size of each prompt chunk when sending
	// to a PTY-based agent. Default: 4096. Only used when Detector is non-nil.
	ChunkSize int
	// ChunkDelay is the delay between prompt chunks when sending to a PTY-based
	// agent. Default: 10ms. Only used when Detector is non-nil.
	ChunkDelay time.Duration
}

// defaults applies zero-value defaults.
func (o PromptOpts) defaults() PromptOpts {
	if o.ReadyTimeout == 0 {
		o.ReadyTimeout = 30 * time.Second
	}
	if o.AcceptTimeout == 0 {
		o.AcceptTimeout = 5 * time.Second
	}
	if o.ResponseTimeout == 0 {
		o.ResponseTimeout = 120 * time.Second
	}
	if o.MaxRetries == 0 {
		o.MaxRetries = 3
	}
	if o.ChunkSize == 0 {
		o.ChunkSize = 4096
	}
	if o.ChunkDelay == 0 {
		o.ChunkDelay = 10 * time.Millisecond
	}
	return o
}

// PromptResult holds the outcome of a successful SendPrompt call.
type PromptResult struct {
	// ResponseText is the accumulated text content from the agent's response.
	ResponseText string
	// StateTransitions tracks TUI state transitions observed during the prompt.
	StateTransitions []TUIStateUpdate
	// Events contains all classified output events received during the prompt.
	Events []OutputEvent
	// Duration is the total wall-clock time spent in SendPrompt.
	Duration time.Duration
}

// ReliablePrompter wraps an AgentHandle with wait-for-ready → send →
// verify → retry semantics. It owns the read loop on the handle during a
// prompt cycle, classifying NDJSON events and handling rate limits,
// permission prompts, errors, and crash recovery.
type ReliablePrompter struct {
	handle   AgentHandle
	provider Provider // For crash recovery (re-spawn). May be nil.
	opts     PromptOpts
	mu       sync.Mutex
}

// NewReliablePrompter creates a prompter that wraps the given handle.
// If provider is non-nil, the prompter can re-spawn the agent on crash.
func NewReliablePrompter(handle AgentHandle, provider Provider, opts PromptOpts) *ReliablePrompter {
	return &ReliablePrompter{
		handle:   handle,
		provider: provider,
		opts:     opts.defaults(),
	}
}

// SendPrompt sends a prompt to the agent and waits for a complete response.
// It implements the reliability loop:
//  1. Wait for the agent to be ready
//  2. Send the prompt
//  3. Read and classify events from the agent
//  4. Handle rate limits (retry with backoff), permission prompts (deny/allow),
//     errors (return), and completion (return result)
//  5. On crash, re-spawn via Provider if available and retry
func (p *ReliablePrompter) SendPrompt(ctx context.Context, prompt string) (*PromptResult, error) {
	start := time.Now()
	opts := p.opts

	var result PromptResult
	retries := 0
	backoff := time.Second

	for {
		// Step 1: Wait for ready.
		readyCtx, readyCancel := context.WithTimeout(ctx, opts.ReadyTimeout)
		err := p.handle.WaitReady(readyCtx)
		readyCancel()
		if err != nil {
			if !p.handle.IsAlive() && p.provider != nil {
				if retries >= opts.MaxRetries {
					return nil, fmt.Errorf("claudemux: agent not ready after %d retries: %w", retries, err)
				}
				if recoverErr := p.recoverHandle(ctx); recoverErr != nil {
					return nil, fmt.Errorf("claudemux: agent not ready and recovery failed: %w (original: %w)", recoverErr, err)
				}
				retries++
				continue
			}
			return nil, fmt.Errorf("claudemux: agent not ready: %w", err)
		}

		// Step 2: Send the prompt.
		if opts.Detector != nil {
			if err := chunkPrompt(writerFunc(p.handle.Send), prompt, opts.ChunkSize, opts.ChunkDelay); err != nil {
				if !p.handle.IsAlive() && p.provider != nil {
					if retries >= opts.MaxRetries {
						return nil, fmt.Errorf("claudemux: send failed after %d retries: %w", retries, err)
					}
					if recoverErr := p.recoverHandle(ctx); recoverErr != nil {
						return nil, fmt.Errorf("claudemux: send failed and recovery failed: %w (original: %w)", recoverErr, err)
					}
					retries++
					continue
				}
				return nil, fmt.Errorf("claudemux: send failed: %w", err)
			}
		} else {
			if err := p.handle.Send(prompt); err != nil {
				if !p.handle.IsAlive() && p.provider != nil {
					if retries >= opts.MaxRetries {
						return nil, fmt.Errorf("claudemux: send failed after %d retries: %w", retries, err)
					}
					if recoverErr := p.recoverHandle(ctx); recoverErr != nil {
						return nil, fmt.Errorf("claudemux: send failed and recovery failed: %w (original: %w)", recoverErr, err)
					}
					retries++
					continue
				}
				return nil, fmt.Errorf("claudemux: send failed: %w", err)
			}
		}

		// Step 3: Read loop — start a goroutine that feeds events and state
		// updates into a single channel (promptEvent) for deterministic ordering.
		peCh := make(chan promptEvent, 64)
		readCtx, readCancel := context.WithCancel(ctx)
		go p.readLoop(readCtx, peCh)

		// Step 4: Process events and state updates.
		// Rate limits are handled by resending the prompt within the same
		// readLoop (avoiding a race where the old readLoop consumes messages
		// meant for a new one). Crash recovery breaks out to the outer loop.
		responseTimeout := opts.ResponseTimeout
		timer := time.NewTimer(responseTimeout)

		sawProcessing := false
		needRetry := false

	eventLoop:
		for {
			select {
			case pe, ok := <-peCh:
				if !ok {
					break eventLoop
				}

				if pe.HasState {
					su := pe.State
					if su.Changed {
						result.StateTransitions = append(result.StateTransitions, su)
					}
					if su.To == StateProcessing {
						sawProcessing = true
					}
					if sawProcessing && su.To == StateReady {
						readCancel()
						timer.Stop()
						result.Duration = time.Since(start)
						return &result, nil
					}
				}

				oe := pe.Event

				result.Events = append(result.Events, oe)

				switch oe.Type {
				case EventRateLimit:
					timer.Stop()
					if retries >= opts.MaxRetries {
						readCancel()
						return nil, fmt.Errorf("claudemux: rate limit exceeded, max retries (%d) reached", opts.MaxRetries)
					}
					backoffDur := backoff
					if ms := oe.Fields["retryAfterMs"]; ms != "" {
						if customBackoff := parseDurationMs(ms); customBackoff > 0 && customBackoff < 60*time.Second {
							backoffDur = customBackoff
						}
					}
					select {
					case <-time.After(backoffDur):
					case <-ctx.Done():
						readCancel()
						return nil, ctx.Err()
					}
					backoff *= 2
					if backoff > 30*time.Second {
						backoff = 30 * time.Second
					}
					retries++
					if err := p.handle.Send(prompt); err != nil {
						readCancel()
						if !p.handle.IsAlive() && p.provider != nil {
							if retries >= opts.MaxRetries {
								return nil, fmt.Errorf("claudemux: send failed after %d retries: %w", retries, err)
							}
							if recoverErr := p.recoverHandle(ctx); recoverErr != nil {
								return nil, fmt.Errorf("claudemux: send failed and recovery failed: %w", recoverErr)
							}
							retries++
							needRetry = true
							break eventLoop
						}
						return nil, fmt.Errorf("claudemux: send failed: %w", err)
					}
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(responseTimeout)

				case EventPermission:
					switch opts.PermissionPolicy {
					case PermissionPolicyDeny:
						readCancel()
						timer.Stop()
						return nil, fmt.Errorf("claudemux: permission denied: tool=%s id=%s",
							oe.Fields["tool"], oe.Fields["id"])
					case PermissionPolicyAllow:
						permID := oe.Fields["id"]
						if permID == "" {
							readCancel()
							timer.Stop()
							return nil, fmt.Errorf("claudemux: permission prompt without id")
						}
						controlResp, err := json.Marshal(map[string]string{
							"type":     "control_response",
							"id":       permID,
							"response": "allow",
						})
						if err != nil {
							readCancel()
							timer.Stop()
							return nil, fmt.Errorf("claudemux: marshal control_response: %w", err)
						}
						if err := p.handle.Send(string(controlResp)); err != nil {
							readCancel()
							timer.Stop()
							return nil, fmt.Errorf("claudemux: send control_response: %w", err)
						}
						if !timer.Stop() {
							select {
							case <-timer.C:
							default:
							}
						}
						timer.Reset(responseTimeout)
					default:
						readCancel()
						timer.Stop()
						return nil, fmt.Errorf("claudemux: unknown permission policy %d", opts.PermissionPolicy)
					}

				case EventError:
					readCancel()
					timer.Stop()
					msg := oe.Fields["message"]
					if msg == "" {
						msg = oe.Line
					}
					return nil, fmt.Errorf("claudemux: agent error: %s", msg)

				case EventCompletion:
					if oe.Pattern == "result-success" {
						readCancel()
						timer.Stop()
						result.Duration = time.Since(start)
						return &result, nil
					}

				case EventText:
					if content, ok := oe.Fields["content"]; ok && content != "" {
						if result.ResponseText != "" {
							result.ResponseText += "\n"
						}
						result.ResponseText += content
					} else if opts.Detector != nil {
						if result.ResponseText != "" {
							result.ResponseText += "\n"
						}
						result.ResponseText += oe.Line
					}

				case EventThinking, EventToolUse:
				}

				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(responseTimeout)

			case <-timer.C:
				readCancel()
				return nil, fmt.Errorf("claudemux: response timeout after %v", responseTimeout)

			case <-ctx.Done():
				readCancel()
				timer.Stop()
				return nil, ctx.Err()
			}
		}

		// Channels closed — readLoop exited. Check if handle died.
		readCancel()
		timer.Stop()
		if needRetry {
			continue
		}
		if !p.handle.IsAlive() && p.provider != nil {
			if retries < opts.MaxRetries {
				if recoverErr := p.recoverHandle(ctx); recoverErr != nil {
					return nil, fmt.Errorf("claudemux: agent crashed and recovery failed: %w", recoverErr)
				}
				retries++
				continue
			}
			return nil, fmt.Errorf("claudemux: agent crashed after %d retries", retries)
		} else if !p.handle.IsAlive() {
			return nil, fmt.Errorf("claudemux: agent crashed, no provider for recovery")
		}
		return nil, fmt.Errorf("claudemux: event stream closed unexpectedly")
	}
}

// protocolStateTracker derives TUI state transitions from classifyLine
// OutputEvents in protocol mode (no Detector). It maps NDJSON event types
// to TUI states and emits transitions on the stateCh channel.
type protocolStateTracker struct {
	current TUIState
}

func newProtocolStateTracker() *protocolStateTracker {
	return &protocolStateTracker{current: StateInitializing}
}

func (t *protocolStateTracker) update(oe OutputEvent, now time.Time) (TUIStateUpdate, bool) {
	var next TUIState
	switch oe.Type {
	case EventCompletion:
		switch oe.Pattern {
		case "system-init":
			next = StateReady
		case "result-success":
			next = StateReady
		default:
			return TUIStateUpdate{}, false
		}
	case EventThinking:
		next = StateProcessing
	case EventError:
		next = StateError
	case EventRateLimit:
		next = StateRateLimited
	case EventPermission:
		next = StatePermissionPrompt
	default:
		return TUIStateUpdate{}, false
	}
	if t.current == next {
		return TUIStateUpdate{}, false
	}
	from := t.current
	t.current = next
	return TUIStateUpdate{
		From:      from,
		To:        next,
		State:     next,
		StateName: tuiStateName(next),
		Pattern:   oe.Pattern,
		Changed:   true,
		Timestamp: now,
	}, true
}

// promptEvent carries a classified event and an optional state transition
// produced from the same input line. Using a single channel guarantees
// deterministic ordering: the consumer never sees the event without the
// corresponding state transition, or vice versa.
type promptEvent struct {
	Event    OutputEvent
	State    TUIStateUpdate // Zero-value if HasState is false
	HasState bool           // Whether State contains a valid transition
}

// readLoop continuously reads from the handle's Receive method and feeds
// classified events into the provided channel. Each promptEvent carries
// both the OutputEvent and any state transition derived from the same
// input line, ensuring atomic delivery to the consumer.
//
// When opts.Detector is non-nil, each raw line is also fed through the
// detector for TUI state tracking. When Detector is nil, state transitions
// are derived from classifyLine events via protocolStateTracker.
func (p *ReliablePrompter) readLoop(ctx context.Context, ch chan<- promptEvent) {
	defer close(ch)

	handle := p.handle
	detector := p.opts.Detector

	var pst *protocolStateTracker
	if detector == nil {
		pst = newProtocolStateTracker()
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line, err := handle.Receive()
		if err != nil {
			return
		}
		if line == "" {
			continue
		}

		oe := classifyLine(line)
		now := time.Now()

		pe := promptEvent{Event: oe}

		if detector != nil {
			updates := detector.ProcessRaw([]byte(line), now)
			su := updates[len(updates)-1]
			if su.Changed {
				pe.State = su
				pe.HasState = true
			}
		} else if pst != nil {
			if su, changed := pst.update(oe, now); changed {
				pe.State = su
				pe.HasState = true
			}
		}

		select {
		case ch <- pe:
		case <-ctx.Done():
			return
		}
	}
}

// recoverHandle attempts to re-spawn the agent via the provider and replace
// the current handle.
func (p *ReliablePrompter) recoverHandle(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_ = p.handle.Close()

	if p.provider == nil {
		return fmt.Errorf("claudemux: no provider for crash recovery")
	}

	newHandle, err := p.provider.Spawn(ctx, SpawnOpts{Mode: ModeProtocol})
	if err != nil {
		return fmt.Errorf("claudemux: re-spawn failed: %w", err)
	}

	p.handle = newHandle
	return nil
}

// Close closes the underlying handle. Must be called to prevent goroutine leaks.
// Safe to call multiple times; no-ops if the handle is nil (e.g. after a failed
// recovery).
func (p *ReliablePrompter) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == nil {
		return nil
	}
	err := p.handle.Close()
	p.handle = nil
	return err
}

// classifyLine parses a raw NDJSON line and classifies it into an
// OutputEvent. This is the same logic as protocolHandle.translateEvent
// but operates on a raw line string, making it usable outside the
// protocolHandle's internal scan loop.
func classifyLine(line string) OutputEvent {
	var ce ClaudeEvent
	if err := json.Unmarshal([]byte(line), &ce); err != nil {
		return OutputEvent{Type: EventText, Line: line}
	}

	oe := OutputEvent{
		Type:   EventText,
		Line:   line,
		Fields: make(map[string]string),
	}

	switch ce.Type {
	case claudeEventSystem:
		if ce.Subtype == "init" {
			oe.Type = EventCompletion
			oe.Pattern = "system-init"
		}

	case claudeEventAssistant:
		content := ce.Content
		thinking := ce.Thinking
		if content == "" && ce.Message != nil {
			var msg claudeAssistantMessage
			if err := json.Unmarshal(ce.Message, &msg); err == nil {
				content = msg.Content
				if msg.Thinking {
					thinking = true
				}
			}
		}
		if thinking {
			oe.Type = EventThinking
			oe.Pattern = "assistant-thinking"
			if content != "" {
				oe.Fields["content"] = content
			}
		} else if ce.Subtype == "text" || content != "" {
			oe.Type = EventText
			if content != "" {
				oe.Fields["content"] = content
			}
		}

	case claudeEventResult:
		switch ce.Subtype {
		case "success":
			oe.Type = EventCompletion
			oe.Pattern = "result-success"
		case "error":
			oe.Type = EventError
			oe.Pattern = "result-error"
			if ce.Content != "" {
				oe.Fields["message"] = ce.Content
			}
		}

	case claudeEventControlRequest:
		if ce.Subtype == "tool_call" {
			oe.Type = EventPermission
			oe.Pattern = "control-request-tool-call"
			if ce.Tool != "" {
				oe.Fields["tool"] = ce.Tool
			}
			if ce.ID != "" {
				oe.Fields["id"] = ce.ID
			}
		}

	case claudeEventRateLimit:
		oe.Type = EventRateLimit
		oe.Pattern = "rate-limit-event"
		if ce.RetryAfterMs > 0 {
			oe.Fields["retryAfterMs"] = fmt.Sprintf("%d", ce.RetryAfterMs)
		}

	case claudeEventToolUse:
		oe.Type = EventToolUse
		oe.Pattern = "tool-use"
		if ce.Tool != "" {
			oe.Fields["toolName"] = ce.Tool
		}
	}

	return oe
}

// parseDurationMs parses a string as milliseconds and returns a time.Duration.
// Returns 0 on parse failure.
func parseDurationMs(s string) time.Duration {
	var ms int
	if _, err := fmt.Sscanf(s, "%d", &ms); err != nil {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

// writerFunc adapts a func(string) error to io.Writer for chunkPrompt.
type writerFunc func(string) error

func (w writerFunc) Write(p []byte) (int, error) {
	if err := w(string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// chunkPrompt splits a prompt string into chunks of at most maxBytes bytes,
// preserving UTF-8 boundaries. Each chunk is followed by a short delay.
// This prevents PTY buffer overflow when sending long prompts to TUI-mode
// agents. Returns nil on success, error if any chunk write fails.
func chunkPrompt(w io.Writer, prompt string, maxBytes int, delay time.Duration) error {
	if maxBytes <= 0 {
		maxBytes = 4096
	}
	if delay <= 0 {
		delay = 10 * time.Millisecond
	}

	remaining := prompt
	for len(remaining) > 0 {
		chunk := remaining
		if len(chunk) > maxBytes {
			chunk = remaining[:maxBytes]
			// Back up to a valid UTF-8 boundary: ensure the byte after
			// the chunk is a RuneStart (i.e., the chunk ends at a
			// complete rune boundary).
			for len(chunk) < len(remaining) && !utf8.RuneStart(remaining[len(chunk)]) {
				chunk = chunk[:len(chunk)-1]
			}
			if len(chunk) == 0 {
				// Single rune exceeds maxBytes — take the full rune.
				_, size := utf8.DecodeRuneInString(remaining)
				chunk = remaining[:size]
			}
		}

		if _, err := w.Write([]byte(chunk)); err != nil {
			return fmt.Errorf("claudemux: chunk write: %w", err)
		}

		remaining = remaining[len(chunk):]
		if len(remaining) > 0 && delay > 0 {
			time.Sleep(delay)
		}
	}
	return nil
}
