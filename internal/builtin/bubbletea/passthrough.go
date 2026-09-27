package bubbletea

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"
)

// toggleModel wraps a tea.Model to intercept a toggle key and execute
// terminal lifecycle management around a JS callback. This enables BubbleTea
// programs to integrate with termmux passthrough mode.
//
// When the toggle key is pressed:
//  1. ReleaseTerminal — pauses BubbleTea renderer/input
//  2. Sync write \x1b[?1049l — exit alt-screen (idempotent belt)
//  3. Call JS onToggle via RunSync — typically mux.switchTo() which blocks
//  4. Sync write \x1b[?1049h\x1b[2J\x1b[H — enter alt-screen (idempotent belt)
//  5. RestoreTerminal — resume BubbleTea renderer/input
//
// All other keys/messages pass through to the inner model unchanged.
type toggleModel struct {
	inner     tea.Model
	toggleKey byte          // Raw byte of the toggle key (e.g., 0x1D for Ctrl+])
	onToggle  goja.Callable // JS callback executed between Release/Restore
	jsRunner  JSRunner      // For thread-safe JS execution from BubbleTea goroutine
	ctx       context.Context
	output    io.Writer    // Terminal output for sync escape sequences
	mu        sync.Mutex   // Protects program
	program   *tea.Program // Set by runProgram after NewProgram, before Run
}

func (m *toggleModel) Init() tea.Cmd {
	return m.inner.Init()
}

func (m *toggleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if m.isToggleKey(keyMsg) {
			return m, m.toggleCmd()
		}
	}
	innerModel, cmd := m.inner.Update(msg)
	m.inner = innerModel
	return m, cmd
}

func (m *toggleModel) View() tea.View {
	return m.inner.View()
}

// isToggleKey checks if a BubbleTea key message matches the configured toggle key.
func (m *toggleModel) isToggleKey(msg tea.KeyPressMsg) bool {
	k := msg.Key()
	// Match by rune (handles most key configurations)
	if k.Code == rune(m.toggleKey) {
		return true
	}
	// Match Ctrl+] specifically (0x1D) — v2 parses this with Code = ']' and Mod = ModCtrl
	if k.Code == ']' && k.Mod.Contains(tea.ModCtrl) && m.toggleKey == 0x1D {
		return true
	}
	return false
}

// toggleCmd returns a tea.Cmd that executes the full toggle lifecycle.
// The command runs on BubbleTea's command goroutine and blocks during passthrough.
func (m *toggleModel) toggleCmd() tea.Cmd {
	return func() tea.Msg {
		m.mu.Lock()
		p := m.program
		output := m.output
		m.mu.Unlock()

		// Release BubbleTea's terminal control (renderer, raw mode, input)
		if p != nil {
			p.ReleaseTerminal()
		}
		// Sync exit alt-screen — idempotent belt for async ReleaseTerminal
		if output != nil {
			_, _ = output.Write([]byte("\x1b[?1049l"))
		}

		// Call JS toggle handler (typically mux.switchTo() — blocks during
		// passthrough). A Promise-like return must settle before the terminal
		// is restored; RunSync only covers the synchronous call that registers
		// the Promise callbacks.
		var toggleResult map[string]any
		var toggleErr error
		if m.jsRunner != nil && m.onToggle != nil {
			ctx := m.ctx

			done := make(chan struct{})
			var doneOnce sync.Once
			settle := func(result map[string]any, err error) {
				doneOnce.Do(func() {
					if result != nil {
						toggleResult = result
					}
					if err != nil {
						toggleErr = err
					}
					close(done)
				})
			}

			runErr := m.jsRunner.RunSync(ctx, func(vm *goja.Runtime) error {
				val, err := m.onToggle(goja.Undefined())
				if err != nil {
					settle(nil, err)
					return err
				}
				if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
					settle(nil, nil)
					return nil
				}

				obj := val.ToObject(vm)
				if obj != nil {
					thenProp := obj.Get("then")
					if thenProp != nil && !goja.IsUndefined(thenProp) {
						if thenFn, ok := goja.AssertFunction(thenProp); ok {
							onFulfilled := vm.ToValue(func(call goja.FunctionCall) goja.Value {
								resolved := call.Argument(0)
								var result map[string]any
								if resolved != nil && !goja.IsUndefined(resolved) && !goja.IsNull(resolved) {
									result, _ = resolved.Export().(map[string]any)
								}
								settle(result, nil)
								return goja.Undefined()
							})
							onRejected := vm.ToValue(func(call goja.FunctionCall) goja.Value {
								reason := call.Argument(0)
								settle(nil, fmt.Errorf("toggle promise rejected: %v", reason.Export()))
								return goja.Undefined()
							})
							if _, thenErr := thenFn(val, onFulfilled, onRejected); thenErr != nil {
								settle(nil, thenErr)
								return thenErr
							}
							return nil
						}
					}
				}
				result, _ := val.Export().(map[string]any)
				settle(result, nil)
				return nil
			})
			if runErr != nil {
				settle(nil, runErr)
			} else if ctx == nil {
				<-done
			} else {
				select {
				case <-done:
				case <-ctx.Done():
					settle(nil, ctx.Err())
				}
			}
		}
		if toggleErr != nil {
			slog.Error("bubbletea: onToggle failed", "error", toggleErr)
		}

		// Sync enter alt-screen + clear — idempotent belt for async RestoreTerminal
		if output != nil {
			_, _ = output.Write([]byte("\x1b[?1049h\x1b[2J\x1b[H"))
		}
		// Restore BubbleTea's terminal control
		if p != nil {
			p.RestoreTerminal()
		}

		return toggleReturnMsg{Result: toggleResult}
	}
}
