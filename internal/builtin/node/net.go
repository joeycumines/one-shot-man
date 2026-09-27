package node

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// netRequire returns the loader for require("net"). The subset is
// net.connect(options): a TCP client socket modeled as a Node EventEmitter
// with "connect", "data", "error" and "close" events, plus write and end.
// Bytes are exposed as strings (UTF-8), which is what the gateway's readiness
// probe and the launcher's socket flows need.
//
// A connected socket holds the event loop alive from net.connect() until the
// socket dispatches "close" (loop.Promisify future released on every exit
// path), so WithAutoExit(true) loops cannot terminate mid-exchange.
func NetRequire(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop) func(*goja.Runtime, *goja.Object) {
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
			socketCtx, cancel := context.WithCancel(ctx)
			queue := newSocketWriteQueue()

			// Keep the event loop alive until the socket worker has joined its
			// reader and writer goroutines.
			socketDone := make(chan struct{})
			if loop != nil {
				_ = loop.Promisify(ctx, func(ctx context.Context) (any, error) {
					select {
					case <-socketDone:
						return nil, nil
					case <-ctx.Done():
						cancel()
						return nil, ctx.Err()
					}
				})
			}

			go func() {
				defer close(socketDone)
				defer cancel()
				var dialer net.Dialer
				conn, err := dialer.DialContext(socketCtx, "tcp", address)
				if err != nil {
					queue.close()
					if !errors.Is(err, context.Canceled) {
						if submitErr := submitSocketEvent(adapter, socket, "error", err); submitErr != nil {
							logSocketDispatchError("error", submitErr)
						}
					}
					if submitErr := submitSocketEvent(adapter, socket, "close", nil); submitErr != nil {
						logSocketDispatchError("close", submitErr)
					}
					return
				}
				if err := queue.setConn(conn); err != nil {
					_ = conn.Close()
					queue.close()
					if submitErr := submitSocketEvent(adapter, socket, "close", nil); submitErr != nil {
						logSocketDispatchError("close", submitErr)
					}
					return
				}
				if err := submitSocketEvent(adapter, socket, "connect", nil); err != nil {
					logSocketDispatchError("connect", err)
					_ = conn.Close()
					queue.close()
					if submitErr := submitSocketEvent(adapter, socket, "close", nil); submitErr != nil {
						logSocketDispatchError("close", submitErr)
					}
					return
				}
				if err := serveSocket(socketCtx, adapter, socket, queue, conn); err != nil {
					if submitErr := submitSocketEvent(adapter, socket, "error", err); submitErr != nil {
						logSocketDispatchError("error", submitErr)
					}
				}
				queue.close()
				_ = conn.Close()
				if err := submitSocketEvent(adapter, socket, "close", nil); err != nil {
					logSocketDispatchError("close", err)
				}
			}()

			_ = socket.obj.Set("write", func(call goja.FunctionCall) goja.Value {
				data := ""
				if len(call.Arguments) > 0 {
					data = call.Argument(0).String()
				}
				if err := queue.write(data); err != nil {
					panic(runtime.NewGoError(err))
				}
				return goja.Undefined()
			})
			_ = socket.obj.Set("end", func(call goja.FunctionCall) goja.Value {
				data := ""
				if len(call.Arguments) > 0 {
					data = call.Argument(0).String()
				}
				if err := queue.end(data); err != nil {
					panic(runtime.NewGoError(err))
				}
				return goja.Undefined()
			})
			_ = socket.obj.Set("destroy", func(call goja.FunctionCall) goja.Value {
				cancel()
				return goja.Undefined()
			})

			return socket.obj
		})
	}
}

func submitSocketEvent(adapter *gojaeventloop.Adapter, socket *jsSocket, event string, value any) error {
	return adapter.Submit(func(rt *goja.Runtime) {
		var arg goja.Value
		switch value := value.(type) {
		case string:
			arg = rt.ToValue(value)
		case error:
			arg = nodeNetError(rt, value)
		}
		if !socket.emit(rt, event, arg) && event == "error" {
			if err, ok := value.(error); ok {
				logSocketUnhandledError(err)
			}
		}
	})
}

func nodeNetError(rt *goja.Runtime, err error) *goja.Object {
	obj := rt.NewGoError(err)
	syscallName := "connect"
	if dnsErr, ok := errors.AsType[*net.DNSError](err); ok {
		syscallName = "getaddrinfo"
		_ = obj.Set("hostname", dnsErr.Name)
	} else if opErr, ok := errors.AsType[*net.OpError](err); ok {
		if opErr.Op != "dial" {
			syscallName = opErr.Op
		}
		if address, ok := opErr.Addr.(*net.TCPAddr); ok {
			_ = obj.Set("address", address.IP.String())
			_ = obj.Set("port", address.Port)
		}
	}
	_ = obj.Set("code", nodeNetErrorCode(err))
	_ = obj.Set("syscall", syscallName)
	return obj
}

func nodeNetErrorCode(err error) string {
	if dnsErr, ok := errors.AsType[*net.DNSError](err); ok {
		switch {
		case dnsErr.IsNotFound:
			return "ENOTFOUND"
		case dnsErr.IsTimeout || dnsErr.IsTemporary:
			return "EAI_AGAIN"
		default:
			return "EAI_FAIL"
		}
	}
	return errnoCode(err)
}

func logSocketUnhandledError(err error) {
	slog.Error("node socket error had no listener", "code", nodeNetErrorCode(err), "error", err)
}

func logSocketDispatchError(event string, err error) {
	slog.Warn("node socket event dispatch failed", "event", event, "error", err)
}

func serveSocket(ctx context.Context, adapter *gojaeventloop.Adapter, socket *jsSocket, queue *socketWriteQueue, conn net.Conn) error {
	writerDone := make(chan error, 1)
	readerDone := make(chan error, 1)
	go func() { writerDone <- queue.run(ctx) }()
	go func() { readerDone <- readSocket(adapter, socket, conn) }()

	var writerErr, readerErr error
	writerFinished, readerFinished := false, false
	select {
	case writerErr = <-writerDone:
		writerFinished = true
		if writerErr == nil {
			select {
			case readerErr = <-readerDone:
				readerFinished = true
			case <-ctx.Done():
			}
		}
	case readerErr = <-readerDone:
		readerFinished = true
	case <-ctx.Done():
	}

	_ = conn.Close()
	queue.close()
	if !writerFinished {
		writerErr = <-writerDone
	}
	if !readerFinished {
		readerErr = <-readerDone
	}
	if !isSocketShutdownError(writerErr) {
		return writerErr
	}
	if !isSocketShutdownError(readerErr) {
		return readerErr
	}
	return nil
}

func readSocket(adapter *gojaeventloop.Adapter, socket *jsSocket, conn net.Conn) error {
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			if submitErr := submitSocketEvent(adapter, socket, "data", string(buf[:n])); submitErr != nil {
				return submitErr
			}
		}
		if err != nil {
			return err
		}
	}
}

func isSocketShutdownError(err error) bool {
	return err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled)
}

// socketWriteQueue serializes writes on a dedicated worker. Enqueue never
// waits for network I/O, and writes made before connect remain FIFO-ordered.
type socketWriteQueue struct {
	mu      sync.Mutex
	conn    net.Conn
	pending []socketWrite
	wake    chan struct{}
	ended   bool
	closed  bool
}

type socketWrite struct {
	data       []byte
	closeWrite bool
}

func newSocketWriteQueue() *socketWriteQueue {
	return &socketWriteQueue{wake: make(chan struct{}, 1)}
}

func (q *socketWriteQueue) notify() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *socketWriteQueue) write(data string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return net.ErrClosed
	}
	if q.ended {
		return errors.New("write after socket end")
	}
	if data != "" {
		q.pending = append(q.pending, socketWrite{data: []byte(data)})
		q.notify()
	}
	return nil
}

func (q *socketWriteQueue) end(data string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return nil
	}
	if q.ended {
		return nil
	}
	if data != "" {
		q.pending = append(q.pending, socketWrite{data: []byte(data)})
	}
	q.pending = append(q.pending, socketWrite{closeWrite: true})
	q.ended = true
	q.notify()
	return nil
}

func (q *socketWriteQueue) setConn(c net.Conn) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return net.ErrClosed
	}
	q.conn = c
	q.notify()
	return nil
}

func (q *socketWriteQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.pending = nil
	q.notify()
	q.mu.Unlock()
}

func (q *socketWriteQueue) run(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return net.ErrClosed
		}
		conn := q.conn
		var next socketWrite
		hasNext := conn != nil && len(q.pending) > 0
		if hasNext {
			next = q.pending[0]
			q.pending[0] = socketWrite{}
			q.pending = q.pending[1:]
		}
		q.mu.Unlock()

		if hasNext {
			if next.closeWrite {
				halfCloser, ok := conn.(interface{ CloseWrite() error })
				if !ok {
					return errors.New("socket connection does not support half-close")
				}
				return halfCloser.CloseWrite()
			}
			for len(next.data) > 0 {
				n, err := conn.Write(next.data)
				if err != nil {
					return err
				}
				if n == 0 {
					return io.ErrShortWrite
				}
				next.data = next.data[n:]
			}
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-q.wake:
		}
	}
}

// socketListener is one registered event listener. A once listener is
// removed after its first invocation; registered listeners are plain
// callables wrapped by the socket's once wrapper.
type socketListener struct {
	fn   goja.Callable
	once bool
}

// jsSocket carries the emitter plumbing shared by the socket object.
type jsSocket struct {
	obj        *goja.Object
	listeners  map[string][]socketListener
	runtimeRef *goja.Runtime
}

func newJSSocket(runtime *goja.Runtime, adapter *gojaeventloop.Adapter) *jsSocket {
	s := &jsSocket{obj: runtime.NewObject(), listeners: map[string][]socketListener{}, runtimeRef: runtime}
	_ = s.obj.Set("on", func(call goja.FunctionCall) goja.Value { return s.on(call, false) })
	_ = s.obj.Set("once", func(call goja.FunctionCall) goja.Value { return s.on(call, true) })
	return s
}

func (s *jsSocket) on(call goja.FunctionCall, once bool) goja.Value {
	if len(call.Arguments) < 2 {
		panic(s.runtimeRef.NewTypeError("socket.on: event and listener required"))
	}
	event := call.Argument(0).String()
	listener := call.Argument(1)
	fn, ok := goja.AssertFunction(listener)
	if !ok {
		panic(s.runtimeRef.NewTypeError("socket.on: listener must be a function"))
	}
	s.listeners[event] = append(s.listeners[event], socketListener{fn: fn, once: once})
	return s.obj
}

// emit dispatches to registered listeners on the loop goroutine. It returns
// whether any listener ran, which mirrors Node's EventEmitter.emit contract.
// once listeners remove themselves after their first invocation. Listener
// errors are logged at Warn level — they must not propagate into the loop's
// uncaught path (that would kill the run) — and never silently swallowed.
func (s *jsSocket) emit(rt *goja.Runtime, event string, arg goja.Value) bool {
	handled := false
	listeners := append([]socketListener(nil), s.listeners[event]...)
	s.listeners[event] = nil
	live := make([]socketListener, 0, len(listeners))
	for _, listener := range listeners {
		args := []goja.Value{}
		if arg != nil {
			args = append(args, arg)
		}
		if _, err := listener.fn(s.obj, args...); err != nil {
			slog.Warn("node socket listener failed", "event", event, "error", err)
		}
		handled = true
		if !listener.once {
			live = append(live, listener)
		}
	}
	s.listeners[event] = append(live, s.listeners[event]...)
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
