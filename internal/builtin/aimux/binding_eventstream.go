package aimux

import (
	"time"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/aimuxcore"
)

// registerEventStreamBindings exposes EventStream and HealthMonitor to JS.
// These provide higher-level abstractions over raw handle receive/poll loops:
//   - newEventStream(handle, parser) wraps a handle+parser into a channel
//     of parsed OutputEvents, eliminating screenshot-polling patterns.
//   - newHealthMonitor(handle, intervalMs) polls handle health periodically
//     and caches the snapshot for non-blocking reads.
func registerEventStreamBindings(runtime *goja.Runtime, exports *goja.Object) {
	_ = exports.Set("newEventStream", func(handleVal goja.Value, parserVal goja.Value) *goja.Object {
		handle := handleFromJS(runtime, handleVal)
		if handle == nil {
			panic(runtime.NewTypeError("newEventStream: handle is required"))
		}
		var parser *aimuxcore.Parser
		if !isAbsent(parserVal) {
			if p, ok := parserVal.Export().(*aimuxcore.Parser); ok {
				parser = p
			}
		}
		es := aimuxcore.NewEventStream(handle, parser)
		return newEventStreamObject(runtime, es)
	})

	_ = exports.Set("newHealthMonitor", func(handleVal goja.Value, intervalMs int) *goja.Object {
		handle := handleFromJS(runtime, handleVal)
		if handle == nil {
			panic(runtime.NewTypeError("newHealthMonitor: handle is required"))
		}
		interval := time.Duration(intervalMs) * time.Millisecond
		if interval <= 0 {
			interval = 5 * time.Second
		}
		hm := aimuxcore.NewHealthMonitor(handle, interval)
		return newHealthMonitorObject(runtime, hm)
	})
}

func newEventStreamObject(runtime *goja.Runtime, es *aimuxcore.EventStream) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("_eventStream", es)
	_ = obj.Set("events", func() <-chan aimuxcore.OutputEvent {
		return es.Events()
	})
	_ = obj.Set("close", func() {
		es.Close()
	})
	return obj
}

func newHealthMonitorObject(runtime *goja.Runtime, hm *aimuxcore.HealthMonitor) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("_healthMonitor", hm)
	_ = obj.Set("snapshot", func() map[string]any {
		s := hm.Snapshot()
		return map[string]any{
			"alive":       s.Alive,
			"lastEventMs": s.LastEvent.UnixMilli(),
			"lastSendMs":  s.LastSend.UnixMilli(),
		}
	})
	_ = obj.Set("close", func() {
		hm.Close()
	})
	return obj
}

// handleFromJS extracts an aimuxcore.AgentHandle from a JS handle object
// created by binding_provider.go's newHandleObject. The handle stores the
// underlying AgentHandle in the "_handle" property.
func handleFromJS(runtime *goja.Runtime, v goja.Value) aimuxcore.AgentHandle {
	if isAbsent(v) {
		return nil
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return nil
	}
	hv := obj.Get("_handle")
	if isAbsent(hv) {
		return nil
	}
	if h, ok := hv.Export().(aimuxcore.AgentHandle); ok {
		return h
	}
	return nil
}
