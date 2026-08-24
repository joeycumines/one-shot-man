package aimux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/builtin/async"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	"github.com/joeycumines/one-shot-man/internal/aimuxcore"
)

// promisifyFn matches adapter.Loop().Promisify. It runs blocking work in a
// background goroutine, keeps the Goja event loop alive until completion, and
// resolves/rejects the returned promise on the loop goroutine.
// registerProviderBindings wires the generic provider/registry/handle JavaScript API.
//
// The surface is intentionally provider-agnostic. Consumers (e.g. pr-split) that
// need provider-specific defaults or model-menu utilities must implement those
// helpers in JavaScript on top of processProvider and the parser/registry APIs.
func registerProviderBindings(ctx context.Context, adapter *gojaeventloop.Adapter, runtime *goja.Runtime, exports *goja.Object) {
	_ = exports.Set("newRegistry", func() *goja.Object {
		return newRegistryObject(ctx, runtime, adapter, aimuxcore.NewRegistry())
	})

	_ = exports.Set("processProvider", func(opts goja.Value) *goja.Object {
		cfg := providerConfigFromJS(runtime, opts)
		return newProviderObject(ctx, runtime, adapter, aimuxcore.NewProcessProvider(cfg.name, cfg.command, cfg.defaultArgs, cfg.caps))
	})
}

type providerConfig struct {
	name        string
	command     string
	defaultArgs []string
	caps        aimuxcore.ProviderCapabilities
}

func providerConfigFromJS(runtime *goja.Runtime, v goja.Value) providerConfig {
	cfg := providerConfig{name: "process"}
	if isAbsent(v) {
		return cfg
	}
	return providerConfigFromObject(runtime, v.(*goja.Object))
}

func providerConfigFromObject(runtime *goja.Runtime, obj *goja.Object) providerConfig {
	cfg := providerConfig{name: gojaString(obj, "name")}
	if cfg.name == "" {
		cfg.name = "process"
	}
	cfg.command = gojaString(obj, "command")
	cfg.defaultArgs = gojaStringSlice(runtime, obj, "defaultArgs")
	cfg.caps = capabilitiesFromJS(runtime, obj)
	return cfg
}

func capabilitiesFromJS(runtime *goja.Runtime, obj *goja.Object) aimuxcore.ProviderCapabilities {
	var caps aimuxcore.ProviderCapabilities
	if v := obj.Get("capabilities"); !isAbsent(v) {
		_ = runtime.ExportTo(v, &caps)
	}
	caps.MCP = gojaBoolDefault(obj, "mcp", caps.MCP)
	caps.Streaming = gojaBoolDefault(obj, "streaming", caps.Streaming)
	caps.MultiTurn = gojaBoolDefault(obj, "multiTurn", caps.MultiTurn)
	caps.Resizable = gojaBoolDefault(obj, "resizable", caps.Resizable)
	return caps
}

func newProviderObject(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter, p *aimuxcore.ProcessProvider) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("_provider", runtime.ToValue(p))
	_ = obj.Set("name", func() string { return p.Name() })
	_ = obj.Set("capabilities", func() *goja.Object {
		capsObj := runtime.NewObject()
		c := p.Capabilities()
		_ = capsObj.Set("mcp", c.MCP)
		_ = capsObj.Set("streaming", c.Streaming)
		_ = capsObj.Set("multiTurn", c.MultiTurn)
		_ = capsObj.Set("resizable", c.Resizable)
		return capsObj
	})
	_ = obj.Set("spawn", func(opts goja.Value) *goja.Object {
		return spawnProvider(ctx, runtime, adapter, p, opts)
	})
	return obj
}

func newRegistryObject(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter, r *aimuxcore.Registry) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("register", func(p *goja.Object) error {
		if p == nil {
			return fmt.Errorf("register: provider is nil")
		}
		prov, err := providerFromJS(runtime, p)
		if err != nil {
			return err
		}
		return r.Register(prov)
	})
	_ = obj.Set("get", func(name string) goja.Value {
		p, err := r.Get(name)
		if err != nil {
			return goja.Null()
		}
		return newProviderObject(ctx, runtime, adapter, p.(*aimuxcore.ProcessProvider))
	})
	_ = obj.Set("list", func() []string {
		return r.List()
	})
	_ = obj.Set("spawn", func(providerName string, opts goja.Value) *goja.Object {
		p, err := r.Get(providerName)
		if err != nil {
			panic(runtime.NewTypeError(err.Error()))
		}
		pp := p.(*aimuxcore.ProcessProvider)
		if pp == nil {
			panic(runtime.NewTypeError("provider not found"))
		}
		return spawnProvider(ctx, runtime, adapter, pp, opts)
	})
	return obj
}

func spawnProvider(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter, p *aimuxcore.ProcessProvider, opts goja.Value) *goja.Object {
	ctx, cancel := context.WithCancel(ctx)
	h, err := p.Spawn(ctx, spawnOptsFromJS(runtime, opts))
	if err != nil {
		cancel()
		panic(runtime.NewTypeError(err.Error()))
	}
	return newHandleObject(ctx, runtime, adapter, h, cancel)
}

func providerFromJS(runtime *goja.Runtime, p *goja.Object) (aimuxcore.Provider, error) {
	if v := p.Get("_provider"); !isAbsent(v) {
		var pp *aimuxcore.ProcessProvider
		if err := runtime.ExportTo(v, &pp); err == nil && pp != nil {
			return pp, nil
		}
	}
	name := gojaString(p, "name")
	command := gojaString(p, "command")
	if command == "" {
		command = name
	}
	if command == "" {
		return nil, fmt.Errorf("register: provider has no command")
	}
	defaultArgs := gojaStringSlice(runtime, p, "defaultArgs")
	caps := capabilitiesFromJS(runtime, p)
	return aimuxcore.NewProcessProvider(name, command, defaultArgs, caps), nil
}

func newHandleObject(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter, h aimuxcore.AgentHandle, cancel context.CancelFunc) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("_handle", runtime.ToValue(h))

	// send and resize are generally fast PTY/syscall operations and may stay synchronous.
	_ = obj.Set("send", func(input string) error {
		return h.Send(input)
	})
	_ = obj.Set("resize", func(rows, cols int) error {
		return h.Resize(rows, cols)
	})

	// isAlive is a non-blocking query.
	_ = obj.Set("isAlive", func() bool {
		return h.IsAlive()
	})

	// close may block waiting for the child to exit.
	_ = obj.Set("close", func() goja.Value {
		cancel()
		return asyncHandleVoid(ctx, runtime, adapter, func() error { return h.Close() })
	})

	// receive blocks until output is available or the handle closes.
	_ = obj.Set("receive", func() goja.Value {
		out, err := h.Receive()
		if err != nil || out == "" {
			return goja.Null()
		}
		return runtime.ToValue(out)
	})
	_ = obj.Set("receiveAsync", func() goja.Value {
		return asyncHandleValue(ctx, runtime, adapter, func() (any, error) {
			out, err := h.Receive()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return goja.Null(), nil
				}
				return nil, err
			}
			if out == "" {
				return goja.Null(), nil
			}
			return out, nil
		})
	})

	// receiveEventAsync blocks until a line event is available or the handle
	// closes. Resolves to the line string, or null on EOF / unsupported.
	_ = obj.Set("receiveEventAsync", func() goja.Value {
		return asyncHandleValue(ctx, runtime, adapter, func() (any, error) {
			eventsCh := h.Events()
			if eventsCh == nil {
				return goja.Null(), nil
			}
			le, ok := <-eventsCh
			if !ok || le.Err != nil {
				return goja.Null(), nil
			}
			return le.Line, nil
		})
	})

	// health returns a snapshot of the handle's current health state.
	_ = obj.Set("health", func() map[string]any {
		snap := h.Health()
		var lastEventMs, lastSendMs int64
		if !snap.LastEvent.IsZero() {
			lastEventMs = snap.LastEvent.UnixMilli()
		}
		if !snap.LastSend.IsZero() {
			lastSendMs = snap.LastSend.UnixMilli()
		}
		return map[string]any{
			"alive":       snap.Alive,
			"lastEventMs": lastEventMs,
			"lastSendMs":  lastSendMs,
		}
	})

	// drainOutput blocks until the handle reaches EOF.
	_ = obj.Set("drainOutput", func() string {
		return drainHandleOutput(h)
	})
	_ = obj.Set("drainOutputAsync", func() goja.Value {
		return asyncHandleValue(ctx, runtime, adapter, func() (any, error) {
			return drainHandleOutput(h), nil
		})
	})

	// wait blocks until process exit.
	_ = obj.Set("wait", func() map[string]any {
		code, err := h.Wait()
		result := map[string]any{"code": code}
		if err != nil {
			result["error"] = err.Error()
		}
		return result
	})
	_ = obj.Set("waitAsync", func() goja.Value {
		return asyncHandleValue(ctx, runtime, adapter, func() (any, error) {
			code, err := h.Wait()
			result := map[string]any{"code": code}
			if err != nil {
				result["error"] = err.Error()
			}
			return result, nil
		})
	})

	// waitReady blocks until the provider signals readiness or the timeout expires.
	_ = obj.Set("waitReady", func(timeoutMs int) error {
		ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
		return h.WaitReady(ctx)
	})
	_ = obj.Set("waitReadyAsync", func(timeoutMs int) goja.Value {
		return asyncHandleVoid(ctx, runtime, adapter, func() error {
			ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
			defer cancel()
			return h.WaitReady(ctx)
		})
	})

	return obj
}

func drainHandleOutput(h aimuxcore.AgentHandle) string {
	var out []string
	for {
		chunk, err := h.Receive()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}
		if chunk == "" {
			break
		}
		out = append(out, chunk)
	}
	return strings.Join(out, "")
}

func spawnOptsFromJS(runtime *goja.Runtime, v goja.Value) aimuxcore.SpawnOpts {
	if isAbsent(v) {
		return aimuxcore.SpawnOpts{}
	}
	obj := v.(*goja.Object)
	var opts aimuxcore.SpawnOpts
	_ = runtime.ExportTo(obj, &opts)
	if cmd := gojaString(obj, "command"); cmd != "" {
		opts.Command = cmd
	}
	if dir := gojaString(obj, "dir"); dir != "" {
		opts.Dir = dir
	}
	if args := gojaStringSlice(runtime, obj, "args"); len(args) > 0 {
		opts.Args = args
	}
	if env := gojaStringMap(runtime, obj, "env"); len(env) > 0 {
		opts.Env = env
	}
	if rows := gojaInt(obj, "rows"); rows > 0 {
		opts.Rows = uint16(rows)
	}
	if cols := gojaInt(obj, "cols"); cols > 0 {
		opts.Cols = uint16(cols)
	}
	return opts
}

func asyncHandleValue(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter, fn func() (any, error)) goja.Value {
	return async.Promise(adapter, ctx, func(ctx context.Context) (any, error) {
		return fn()
	})
}

func asyncHandleVoid(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter, fn func() error) goja.Value {
	return asyncHandleValue(ctx, runtime, adapter, func() (any, error) {
		return goja.Undefined(), fn()
	})
}

func isAbsent(v goja.Value) bool {
	return v == nil || goja.IsNull(v) || goja.IsUndefined(v)
}

func gojaString(obj *goja.Object, key string) string {
	v := obj.Get(key)
	if isAbsent(v) {
		return ""
	}
	if s, ok := v.Export().(string); ok {
		return s
	}
	return ""
}

func gojaStringSlice(runtime *goja.Runtime, obj *goja.Object, key string) []string {
	v := obj.Get(key)
	if isAbsent(v) {
		return nil
	}
	var out []string
	if err := runtime.ExportTo(v, &out); err != nil {
		return nil
	}
	return out
}

func gojaStringMap(runtime *goja.Runtime, obj *goja.Object, key string) map[string]string {
	v := obj.Get(key)
	if isAbsent(v) {
		return nil
	}
	var out map[string]string
	if err := runtime.ExportTo(v, &out); err != nil {
		return nil
	}
	return out
}

func gojaInt(obj *goja.Object, key string) int {
	v := obj.Get(key)
	if isAbsent(v) {
		return 0
	}
	switch n := v.Export().(type) {
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

func gojaBoolDefault(obj *goja.Object, key string, def bool) bool {
	v := obj.Get(key)
	if isAbsent(v) {
		return def
	}
	if b, ok := v.Export().(bool); ok {
		return b
	}
	return def
}
