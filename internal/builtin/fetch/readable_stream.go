package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/one-shot-man/internal/builtin/async"
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
	ctx       context.Context
	source    io.ReadCloser
	chunkSize int
	mu        sync.Mutex
	locked    bool
	started   bool
	closed    bool
	chunks chan readResult
	done   chan struct{}
}

func NewReadableStream(ctx context.Context, src io.ReadCloser, _ any) *ReadableStream {
	return &ReadableStream{
		ctx:       ctx,
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
		go rs.pump()
	}
	return &ReadableStreamDefaultReader{stream: rs}, nil
}

func (rs *ReadableStream) Cancel() error {
	rs.mu.Lock()
	if rs.closed {
		rs.mu.Unlock()
		return nil
	}
	rs.closed = true
	started := rs.started
	rs.mu.Unlock()
	err := rs.source.Close()
	if started {
		go func() {
			for range rs.chunks {
			}
		}()
	}
	return err
}

func (rs *ReadableStream) pump() {
	defer close(rs.done)
	defer close(rs.chunks)
	buf := make([]byte, rs.chunkSize)
	for {
		n, err := rs.source.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			rs.chunks <- readResult{data: chunk}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				rs.chunks <- readResult{err: err}
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
	result, ok := <-r.stream.chunks
	if !ok {
		return nil, true, nil
	}
	if result.err != nil {
		return nil, false, result.err
	}
	return result.data, false, nil
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

func wrapReadableStreamJS(ctx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, rs *ReadableStream, _ any) *goja.Object {
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
		return wrapReaderJS(ctx, rt, adapter, reader)
	})
	_ = obj.Set("cancel", func(call goja.FunctionCall) goja.Value {
		if err := rs.Cancel(); err != nil {
			panic(rt.NewGoError(err))
		}
		return goja.Undefined()
	})
	return obj
}

func wrapReaderJS(ctx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, reader *ReadableStreamDefaultReader) *goja.Object {
	obj := rt.NewObject()
	_ = obj.Set("read", func(call goja.FunctionCall) goja.Value {
		return async.Promise(adapter, ctx, func(ctx context.Context) (any, error) {
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
