// SSE (Server-Sent Events) parser for the fetch module.
//
// Consumes a ReadableStream and emits parsed SSEEvent structs following
// the W3C EventSource specification:
//   - Events are delimited by blank lines (\n\n or \r\n\r\n).
//   - Lines starting with ":" are comments (ignored).
//   - Fields: event, data, id, retry.
//   - Multiple data: lines are joined with \n.
//
// Deviation from spec: when the stream ends without a trailing blank
// line, the parser flushes any accumulated data as a final event.  The
// W3C spec (step 3 "If the line is empty") would discard it.
// This is intentional — LLM streaming APIs frequently omit the final
// delimiter, and discarding the last event is worse than emitting it.
package fetch

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/dop251/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// SSEEvent represents a single parsed Server-Sent Event.
type SSEEvent struct {
	Event string // event type (default "message")
	Data  string // event data (joined with \n for multi-line)
	ID    string // last event ID
	Retry int    // reconnection time in ms (0 = not set)
}

// SSEParser reads chunks from a ReadableStreamDefaultReader and
// produces parsed SSEEvent values.  It handles partial lines across
// chunk boundaries, including \r\n sequences that straddle chunks.
type SSEParser struct {
	reader *ReadableStreamDefaultReader

	mu            sync.Mutex
	buf           string // unparsed remainder from previous chunk
	lastID        string
	lastCharWasCR bool // true when previous chunk ended with \r

	// current event being accumulated
	eventType string
	dataLines []string
	hasData   bool
	retryMs   int // reconnection time from most recent retry field
}

// NewSSEParser wraps a ReadableStreamDefaultReader in an SSE parser.
func NewSSEParser(reader *ReadableStreamDefaultReader) *SSEParser {
	return &SSEParser{reader: reader}
}

// Next returns the next SSEEvent from the stream.  It blocks until a
// complete event is available or the stream ends.  When the stream is
// exhausted, done is true.
func (p *SSEParser) Next() (ev SSEEvent, done bool, err error) {
	for {
		// Try to extract an event from the buffer.
		if event, ok := p.extractEvent(); ok {
			return event, false, nil
		}

		// Buffer doesn't contain a complete event; read more.
		data, streamDone, readErr := p.reader.Read()
		if readErr != nil {
			return SSEEvent{}, false, readErr
		}
		if streamDone {
			// Flush any remaining buffered event.
			if event, ok := p.flush(); ok {
				return event, false, nil
			}
			return SSEEvent{}, true, nil
		}
		p.mu.Lock()
		chunk := string(data)
		// Handle \r\n split across chunk boundaries: if the previous
		// chunk ended with \r and this one starts with \n, drop the \n
		// to avoid producing a phantom blank line.
		if p.lastCharWasCR && len(chunk) > 0 && chunk[0] == '\n' {
			chunk = chunk[1:]
		}
		p.lastCharWasCR = len(chunk) > 0 && chunk[len(chunk)-1] == '\r'
		p.buf += chunk
		p.mu.Unlock()
	}
}

// extractEvent tries to parse one complete event from the buffer.
// An event is terminated by a blank line (two consecutive newlines).
func (p *SSEParser) extractEvent() (SSEEvent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for {
		// Find next line boundary.
		idx := strings.IndexAny(p.buf, "\r\n")
		if idx == -1 {
			return SSEEvent{}, false
		}

		// Determine line and advance past the delimiter.
		line := p.buf[:idx]
		rest := p.buf[idx:]
		if strings.HasPrefix(rest, "\r\n") {
			rest = rest[2:]
		} else {
			rest = rest[1:] // \n or \r alone
		}
		p.buf = rest

		if line == "" {
			// Blank line → dispatch event if we have data.
			if p.hasData {
				ev := SSEEvent{
					Event: p.eventType,
					Data:  strings.Join(p.dataLines, "\n"),
					ID:    p.lastID,
					Retry: p.retryMs,
				}
				if ev.Event == "" {
					ev.Event = "message"
				}
				p.eventType = ""
				p.dataLines = nil
				p.hasData = false
				p.retryMs = 0
				return ev, true
			}
			// Reset fields even without data.
			p.eventType = ""
			p.retryMs = 0
			continue
		}

		p.processLine(line)
	}
}

// flush handles any remaining data in the buffer when the stream ends.
func (p *SSEParser) flush() (SSEEvent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Process any remaining partial line.
	if p.buf != "" {
		p.processLine(p.buf)
		p.buf = ""
	}

	if p.hasData {
		ev := SSEEvent{
			Event: p.eventType,
			Data:  strings.Join(p.dataLines, "\n"),
			ID:    p.lastID,
			Retry: p.retryMs,
		}
		if ev.Event == "" {
			ev.Event = "message"
		}
		p.eventType = ""
		p.dataLines = nil
		p.hasData = false
		p.retryMs = 0
		return ev, true
	}
	return SSEEvent{}, false
}

// processLine handles a single SSE line.  Must be called with p.mu held.
func (p *SSEParser) processLine(line string) {
	// Comment lines start with ':'
	if strings.HasPrefix(line, ":") {
		return
	}

	field, value, hasColon := strings.Cut(line, ":")
	if !hasColon {
		// Line with no colon — field name is the entire line, value is empty.
		field = line
		value = ""
	} else {
		// Strip single leading space from value (per spec).
		value = strings.TrimPrefix(value, " ")
	}

	switch field {
	case "event":
		p.eventType = value
	case "data":
		p.dataLines = append(p.dataLines, value)
		p.hasData = true
	case "id":
		if !strings.Contains(value, "\x00") {
			p.lastID = value
		}
	case "retry":
		if n, err := strconv.Atoi(value); err == nil {
			p.retryMs = n
		}
	}
}

// ---------------------------------------------------------------------------
// JS wrapper — expose SSEParser as a goja reader object
// ---------------------------------------------------------------------------

// wrapSSEParserJS returns a goja.Object wrapping the SSEParser for JS use.
// The read() method returns Promise<{value: {event, data, id}, done: boolean}>.
func wrapSSEParserJS(rt *goja.Runtime, adapter *gojaeventloop.Adapter, parser *SSEParser, promisify PromisifyFunc) *goja.Object {
	obj := rt.NewObject()

	_ = obj.Set("read", func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := adapter.JS().NewChainedPromise()

		promisify(context.Background(), func(ctx context.Context) (any, error) {
			ev, done, err := parser.Next()
			if err != nil {
				_ = adapter.Loop().Submit(func() {
					reject(err)
				})
				return nil, err
			}
			if submitErr := adapter.Loop().Submit(func() {
				result := rt.NewObject()
				if done {
					_ = result.Set("value", goja.Undefined())
					_ = result.Set("done", true)
				} else {
					evObj := rt.NewObject()
					_ = evObj.Set("event", ev.Event)
					_ = evObj.Set("data", ev.Data)
					_ = evObj.Set("id", ev.ID)
					_ = result.Set("value", evObj)
					_ = result.Set("done", false)
				}
				resolve(result)
			}); submitErr != nil {
				_ = adapter.Loop().Submit(func() {
					reject(fmt.Errorf("event loop not running"))
				})
			}
			return nil, nil
		})

		return adapter.GojaWrapPromise(promise)
	})

	return obj
}
