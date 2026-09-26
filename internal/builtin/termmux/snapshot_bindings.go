package termmux

import (
	"errors"
	"strings"
	"time"

	"github.com/joeycumines/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// registerSnapshotMethods registers query and capture methods. Every method
// that consults the SessionManager worker returns a tracked Promise.
func registerSnapshotMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("termSize", func() goja.Value {
		return s.runtime.ToValue(map[string]any{
			"rows": int(s.termRowsCached.Load()),
			"cols": int(s.termColsCached.Load()),
		})
	})

	// CaptureScreen reads the manager's immutable snapshot index directly;
	// it does not send a request to the worker or block the JS event loop.
	_ = obj.Set("capture", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(s.runtime.NewTypeError("capture: session ID argument is required"))
		}
		id := parent.SessionID(call.Argument(0).ToInteger())
		opts := s.captureOptions(call.Argument(1))
		capture, err := s.mgr.CaptureScreen(id, opts)
		if err != nil {
			if errors.Is(err, parent.ErrSessionNotFound) || errors.Is(err, parent.ErrSnapshotUnavailable) {
				return goja.Null()
			}
			panic(s.runtime.NewGoError(err))
		}
		var plain, ansi, fullScreen string
		var needPlain, needANSI, needFull bool
		switch opts.Kind {
		case parent.CaptureANSI:
			ansi = capture.Text
			needPlain, needFull = true, true
		case parent.CaptureFullScreen:
			fullScreen = capture.Text
			needPlain, needANSI = true, true
		default:
			plain = capture.Text
			needANSI, needFull = true, true
		}
		if needPlain {
			var b strings.Builder
			if err := capture.Snapshot.WriteCapture(&b, parent.CapturePlain); err != nil {
				panic(s.runtime.NewGoError(err))
			}
			plain = b.String()
		}
		if needANSI {
			var b strings.Builder
			if err := capture.Snapshot.WriteCapture(&b, parent.CaptureANSI); err != nil {
				panic(s.runtime.NewGoError(err))
			}
			ansi = b.String()
		}
		if needFull {
			var b strings.Builder
			if err := capture.Snapshot.WriteCapture(&b, parent.CaptureFullScreen); err != nil {
				panic(s.runtime.NewGoError(err))
			}
			fullScreen = b.String()
		}
		result := s.runtime.NewObject()
		_ = result.Set("plain", plain)
		_ = result.Set("ansi", ansi)
		_ = result.Set("fullScreen", fullScreen)
		_ = result.Set("gen", capture.Snapshot.Gen)
		_ = result.Set("rows", capture.Snapshot.Rows)
		_ = result.Set("cols", capture.Snapshot.Cols)
		_ = result.Set("cursorRow", capture.Snapshot.CursorRow)
		_ = result.Set("cursorCol", capture.Snapshot.CursorCol)
		_ = result.Set("cursorVisible", capture.Snapshot.CursorVisible)
		_ = result.Set("mouseTracking", capture.Snapshot.MouseTracking)
		_ = result.Set("mouseSGR", capture.Snapshot.MouseSGR)
		_ = result.Set("locked", capture.Snapshot.Locked)
		_ = result.Set("message", capture.Snapshot.Message)
		_ = result.Set("timestamp", capture.Snapshot.Timestamp.UnixMilli())
		return result
	})

	_ = obj.Set("activeID", func() uint64 {
		return s.cachedActiveID()
	})

	_ = obj.Set("isDone", func(id uint64) bool {
		return s.cachedSessionDone(id)
	})

	_ = obj.Set("sessions", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			infos := s.mgr.Sessions()
			result := make([]map[string]any, len(infos))
			for i, info := range infos {
				result[i] = map[string]any{
					"id": uint64(info.ID),
					"target": map[string]any{
						"name": info.Target.Name,
						"kind": string(info.Target.Kind),
						"id":   info.Target.ID,
					},
					"state":    info.State.String(),
					"isActive": info.IsActive,
				}
			}
			return result, nil
		})
	})

	_ = obj.Set("eventsDropped", func() int64 {
		return s.mgr.EventsDropped()
	})

	_ = obj.Set("lastActivityMs", func(call goja.FunctionCall) goja.Value {
		hasID := len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0))
		id := s.cachedActiveID()
		if hasID {
			id = uint64(call.Argument(0).ToInteger())
		}
		if id == 0 {
			return s.runtime.ToValue(int64(-1))
		}
		capture, err := s.mgr.CaptureScreen(parent.SessionID(id), parent.CaptureOptions{Kind: parent.CapturePlain})
		if err != nil || capture == nil || capture.Snapshot == nil || capture.Snapshot.Timestamp.IsZero() {
			return s.runtime.ToValue(int64(-1))
		}
		return s.runtime.ToValue(time.Since(capture.Snapshot.Timestamp).Milliseconds())
	})
}
