package node

import (
	"bufio"
	"context"
	"errors"
	"net"
	"sync"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// netRequire returns the loader for require("net"). The subset is
// net.connect(options): a TCP client socket modeled as a Node EventEmitter
// with "connect", "data", "error" and "close" events, plus write and end.
// Bytes are exposed as strings (UTF-8), which is what the gateway's readiness
// probe and the launcher's socket flows need.
func NetRequire(ctx context.Context, adapter *gojaeventloop.Adapter) func(*goja.Runtime, *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		_ = exports.Set("connect", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("net.connect: options required"))
			}
			optsObj, ok := call.Argument(0).(*goja.Object)
			if !ok {
				panic(runtime.NewTypeError("net.connect: options must be an object"))
			}
			host := "127.0.0.1"
			port := 0
			if v := optsObj.Get("host"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				host = v.String()
			}
			if v := optsObj.Get("port"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				port = int(v.ToInteger())
			} else {
				panic(runtime.NewTypeError("net.connect: port required"))
			}

			socket := newJSSocket(runtime, adapter)

			address := net.JoinHostPort(host, itoa(port))
			connCh := make(chan net.Conn, 1)
			errCh := make(chan error, 1)
			go func() {
				var dialer net.Dialer
				conn, err := dialer.DialContext(ctx, "tcp", address)
				if err != nil {
					errCh <- err
					return
				}
				connCh <- conn
			}()

			// connHolder carries the established connection between the
			// reader goroutine and the write/end closures with proper
			// synchronization; the connect event is dispatched through the
			// loop while the reader owns the value.
			var mu sync.Mutex
			var conn net.Conn
			setConn := func(c net.Conn) { mu.Lock(); conn = c; mu.Unlock() }
			getConn := func() net.Conn { mu.Lock(); defer mu.Unlock(); return conn }

			// The reader goroutine funnels socket events through the loop via
			// Submit, preserving the single-threaded JS execution contract.
			readerDone := make(chan struct{})
			go func() {
				defer close(readerDone)
				select {
				case c := <-connCh:
					setConn(c)
					if adapter.Submit(func(rt *goja.Runtime) {
						socket.emit(rt, "connect", nil)
					}) != nil {
						c.Close()
						return
					}
					reader := bufio.NewReader(c)
					buf := make([]byte, 4096)
					for {
						n, readErr := reader.Read(buf)
						if n > 0 {
							chunk := string(buf[:n])
							if adapter.Submit(func(rt *goja.Runtime) {
								socket.emit(rt, "data", rt.ToValue(chunk))
							}) != nil {
								c.Close()
								return
							}
						}
						if readErr != nil {
							c.Close()
							finalErr := readErr
							if !errors.Is(finalErr, net.ErrClosed) && finalErr.Error() != "EOF" && !errors.Is(finalErr, errEOFPlaceholder) {
								adapter.Submit(func(rt *goja.Runtime) {
									socket.emit(rt, "error", rt.NewGoError(finalErr))
								})
							}
							adapter.Submit(func(rt *goja.Runtime) {
								socket.emit(rt, "close", nil)
							})
							return
						}
					}
				case err := <-errCh:
					adapter.Submit(func(rt *goja.Runtime) {
						socket.emit(rt, "error", rt.NewGoError(err))
						socket.emit(rt, "close", nil)
					})
				}
			}()

			_ = socket.obj.Set("write", func(call goja.FunctionCall) goja.Value {
				data := ""
				if len(call.Arguments) > 0 {
					data = call.Argument(0).String()
				}
				if c := getConn(); c != nil {
					if _, err := c.Write([]byte(data)); err != nil {
						panic(runtime.NewGoError(err))
					}
				}
				return goja.Undefined()
			})
			_ = socket.obj.Set("end", func(call goja.FunctionCall) goja.Value {
				if c := getConn(); c != nil {
					_ = c.Close()
				}
				return goja.Undefined()
			})
			_ = socket.obj.Set("destroy", func(call goja.FunctionCall) goja.Value {
				if c := getConn(); c != nil {
					_ = c.Close()
				}
				return goja.Undefined()
			})

			return socket.obj
		})
	}
}

// errEOFPlaceholder exists so the reader can compare a plain EOF without
// importing io twice in one expression; io.EOF's Error() is "EOF".
var errEOFPlaceholder = errors.New("EOF")

// jsSocket carries the emitter plumbing shared by the socket object.
type jsSocket struct {
	obj        *goja.Object
	listeners  map[string][]goja.Value
	runtimeRef *goja.Runtime
}

func newJSSocket(runtime *goja.Runtime, adapter *gojaeventloop.Adapter) *jsSocket {
	s := &jsSocket{obj: runtime.NewObject(), listeners: map[string][]goja.Value{}, runtimeRef: runtime}
	_ = s.obj.Set("on", func(call goja.FunctionCall) goja.Value { return s.on(call) })
	_ = s.obj.Set("once", func(call goja.FunctionCall) goja.Value { return s.on(call) })
	return s
}

func (s *jsSocket) on(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		panic(s.runtimeRef.NewTypeError("socket.on: event and listener required"))
	}
	event := call.Argument(0).String()
	listener := call.Argument(1)
	if _, ok := goja.AssertFunction(listener); !ok {
		panic(s.runtimeRef.NewTypeError("socket.on: listener must be a function"))
	}
	s.listeners[event] = append(s.listeners[event], listener)
	return s.obj
}

// emit dispatches to registered listeners on the loop goroutine. It returns
// whether any listener ran, which mirrors Node's process.emit contract.
func (s *jsSocket) emit(rt *goja.Runtime, event string, arg goja.Value) bool {
	handled := false
	for _, listener := range s.listeners[event] {
		if fn, ok := goja.AssertFunction(listener); ok {
			args := []goja.Value{}
			if arg != nil {
				args = append(args, arg)
			}
			if _, err := fn(s.obj, args...); err != nil {
				_ = rt
			}
			handled = true
		}
	}
	return handled
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
