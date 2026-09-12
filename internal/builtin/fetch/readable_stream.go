package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

const (
	defaultChunkSize  = 65536
	defaultBufferSize = 4
)

type readResult struct {
	data []byte
	err  error
}

type ReadableStream struct {
	ctx        context.Context
	cancel     context.CancelFunc
	loop       *goeventloop.Loop
	source     io.ReadCloser
	chunkSize  int
	mu         sync.Mutex
	locked     bool
	started    bool
	closed     bool
	chunks     chan readResult
	done       chan struct{}
	pumpWG     sync.WaitGroup
	sourceOnce sync.Once
	sourceErr  error
}

func NewReadableStream(ctx context.Context, src io.ReadCloser) *ReadableStream {
	if ctx == nil {
		ctx = context.Background()
	}
	streamCtx, cancel := context.WithCancel(ctx)
	return &ReadableStream{
		ctx:       streamCtx,
		cancel:    cancel,
		source:    src,
		chunkSize: defaultChunkSize,
		chunks:    make(chan readResult, defaultBufferSize),
		done:      make(chan struct{}),
	}
}

func (rs *ReadableStream) Locked() bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.locked
}

func (rs *ReadableStream) GetReader() (*ReadableStreamDefaultReader, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.locked {
		return nil, fmt.Errorf("ReadableStream is locked to a reader")
	}
	if rs.closed {
		return nil, fmt.Errorf("ReadableStream is closed")
	}
	rs.locked = true
	if !rs.started {
		rs.started = true
		if rs.loop != nil {
			rs.loop.Promisify(rs.ctx, func(_ context.Context) (any, error) {
				rs.pump()
				return nil, nil
			})
		} else {
			// The standalone Go API has no event-loop lifecycle to join. Own
			// this worker through the stream and join it from Cancel instead
			// of launching an untracked I/O goroutine.
			rs.pumpWG.Go(func() {
				rs.pump()
			})
		}
	}
	return &ReadableStreamDefaultReader{stream: rs}, nil
}

// beginCancel publishes the closed state and returns ownership of source
// cleanup. The state transition is synchronous so JavaScript observes a closed
// stream immediately, while arbitrary source.Close work can run off the event
// loop.
func (rs *ReadableStream) closeSource() error {
	rs.sourceOnce.Do(func() { rs.sourceErr = rs.source.Close() })
	return rs.sourceErr
}

func (rs *ReadableStream) beginCancel() (bool, bool) {
	rs.mu.Lock()
	if rs.closed {
		rs.mu.Unlock()
		return false, true
	}
	rs.closed = true
	cancel := rs.cancel
	started := rs.started
	rs.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if started {
		go func() {
			for range rs.chunks {
			}
		}()
	}
	return true, false
}

func (rs *ReadableStream) Cancel() error {
	owned, alreadyClosed := rs.beginCancel()
	if alreadyClosed || !owned {
		return nil
	}
	// This Go API is synchronous by contract; JavaScript uses cancelJS below
	// so arbitrary source.Close work never blocks the event loop.
	err := rs.closeSource()
	// Standalone streams own their pump goroutine directly. Join it after
	// closing the source so cancellation cannot return with blocking I/O alive.
	if rs.loop == nil && rs.started {
		rs.pumpWG.Wait()
	}
	return err
}

func (rs *ReadableStream) cancelJS(ctx context.Context, adapter *gojaeventloop.Adapter) goja.Value {
	owned, alreadyClosed := rs.beginCancel()
	if alreadyClosed || !owned {
		return adapter.Promisify(ctx, func(context.Context) (any, error) { return nil, nil })
	}
	return adapter.Promisify(ctx, func(context.Context) (any, error) {
		return nil, rs.closeSource()
	})
}

func (rs *ReadableStream) pump() {
	stop := context.AfterFunc(rs.ctx, func() { _ = rs.closeSource() })
	defer stop()
	defer close(rs.done)
	defer close(rs.chunks)
	buf := make([]byte, rs.chunkSize)
	for {
		n, err := rs.source.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			select {
			case rs.chunks <- readResult{data: chunk}:
			case <-rs.ctx.Done():
				return
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				select {
				case rs.chunks <- readResult{err: err}:
				case <-rs.ctx.Done():
				}
			}
			return
		}
	}
}

type ReadableStreamDefaultReader struct {
	stream   *ReadableStream
	released bool
}

func (r *ReadableStreamDefaultReader) Read() (data []byte, done bool, err error) {
	if r.released {
		return nil, false, fmt.Errorf("reader has been released")
	}
	select {
	case result, ok := <-r.stream.chunks:
		if !ok {
			return nil, true, nil
		}
		if result.err != nil {
			return nil, false, result.err
		}
		return result.data, false, nil
	case <-r.stream.ctx.Done():
		return nil, false, r.stream.ctx.Err()
	}
}

func (r *ReadableStreamDefaultReader) ReleaseLock() {
	if r.released {
		return
	}
	r.released = true
	r.stream.mu.Lock()
	r.stream.locked = false
	r.stream.mu.Unlock()
}

func wrapReadableStreamJS(ctx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, rs *ReadableStream, loop *goeventloop.Loop) *goja.Object {
	rs.loop = loop
	obj := rt.NewObject()
	_ = obj.Set("_goStream", rs)
	getter := rt.ToValue(func(goja.FunctionCall) goja.Value {
		return rt.ToValue(rs.Locked())
	})
	_ = obj.DefineAccessorProperty("locked", getter, goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = obj.Set("getReader", func(call goja.FunctionCall) goja.Value {
		reader, err := rs.GetReader()
		if err != nil {
			panic(rt.NewGoError(err))
		}
		return wrapReaderJS(ctx, rt, adapter, reader, loop)
	})
	_ = obj.Set("cancel", func(call goja.FunctionCall) goja.Value {
		if adapter == nil {
			panic(rt.NewGoError(errors.New("fetch: event loop adapter is required")))
		}
		return rs.cancelJS(ctx, adapter)
	})
	return obj
}

func wrapReaderJS(ctx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, reader *ReadableStreamDefaultReader, loop *goeventloop.Loop) *goja.Object {
	if loop == nil {
		panic("fetch: nil event loop")
	}
	obj := rt.NewObject()
	_ = obj.Set("read", func(call goja.FunctionCall) goja.Value {
		if adapter == nil {
			panic(rt.NewGoError(errors.New("fetch: event loop adapter is required")))
		}
		return adapter.Promisify(ctx, func(_ context.Context) (any, error) {
			data, done, err := reader.Read()
			if err != nil {
				return nil, err
			}
			if done {
				return map[string]any{"value": nil, "done": true}, nil
			}
			return map[string]any{"value": string(data), "done": false}, nil
		})
	})
	_ = obj.Set("releaseLock", func(call goja.FunctionCall) goja.Value {
		reader.ReleaseLock()
		return goja.Undefined()
	})
	return obj
}
