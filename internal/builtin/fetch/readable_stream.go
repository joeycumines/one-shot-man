// ReadableStream provides chunked, back-pressure-aware reading from an
// io.ReadCloser source.  It models the essential subset of the WHATWG
// ReadableStream API needed for fetch Response.body.
//
// The pump goroutine starts lazily on the first GetReader call and pushes
// fixed-size chunks into a bounded channel, providing natural back-pressure
// when the consumer falls behind.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/dop251/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

const (
	defaultChunkSize  = 65536 // 64 KiB per chunk
	defaultBufferSize = 4     // bounded channel capacity
)

// readResult pairs a chunk of data with an optional error.
type readResult struct {
	data []byte
	err  error
}

// ReadableStream wraps an io.ReadCloser with a bounded-channel pump
// goroutine, exposing a locking reader interface analogous to the
// browser ReadableStream.
type ReadableStream struct {
	source    io.ReadCloser
	chunkSize int

	mu        sync.Mutex
	locked    bool
	started   bool
	closed    bool
	promisify PromisifyFunc

	chunks chan readResult
	done   chan struct{} // closed when pump finishes
}

// NewReadableStream wraps src in a ReadableStream with 64 KiB chunks
// and a 4-slot bounded channel.
func NewReadableStream(src io.ReadCloser, promisify PromisifyFunc) *ReadableStream {
	return &ReadableStream{
		source:    src,
		chunkSize: defaultChunkSize,
		chunks:    make(chan readResult, defaultBufferSize),
		done:      make(chan struct{}),
		promisify: promisify,
	}
}

// Locked reports whether a reader currently holds the lock.
func (rs *ReadableStream) Locked() bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.locked
}

// GetReader acquires a ReadableStreamDefaultReader.  The stream must
// not already be locked or closed.  The pump goroutine starts on
// the first call.
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
		if rs.promisify != nil {
			rs.promisify(context.Background(), func(ctx context.Context) (any, error) {
				rs.pump()
				return nil, nil
			})
		} else {
			go rs.pump()
		}
	}
	return &ReadableStreamDefaultReader{stream: rs}, nil
}

// Cancel closes the stream and the underlying source.
// Safe to call more than once.
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

	// Drain remaining chunks so the pump goroutine can exit.
	if started {
		drain := func() {
			//nolint:revive // intentional drain
			for range rs.chunks {
			}
		}
		if rs.promisify != nil {
			rs.promisify(context.Background(), func(ctx context.Context) (any, error) {
				drain()
				return nil, nil
			})
		} else {
			go drain()
		}
	}
	return err
}

// pump reads from the source and sends chunks into the bounded channel.
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

// ReadableStreamDefaultReader provides sequential read access to a
// ReadableStream's chunks.
type ReadableStreamDefaultReader struct {
	stream   *ReadableStream
	released bool
}

// Read returns the next chunk.  When the stream is exhausted, done is
// true and data is nil.  Read blocks until a chunk is available.
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

// ReleaseLock releases the reader's lock on the stream, allowing a
// new reader to be acquired.  Safe to call more than once.
func (r *ReadableStreamDefaultReader) ReleaseLock() {
	if r.released {
		return
	}
	r.released = true
	r.stream.mu.Lock()
	r.stream.locked = false
	r.stream.mu.Unlock()
}

// ---------------------------------------------------------------------------
// JS wrappers — expose ReadableStream as a goja object on Response.body
// ---------------------------------------------------------------------------

// wrapReadableStreamJS returns a goja.Object exposing the ReadableStream
// to JavaScript with the standard locked/getReader()/cancel() surface.
func wrapReadableStreamJS(rt *goja.Runtime, adapter *gojaeventloop.Adapter, rs *ReadableStream, promisify PromisifyFunc) *goja.Object {
	obj := rt.NewObject()

	// Stash the Go ReadableStream for internal access (e.g., sseReader).
	_ = obj.Set("_goStream", rs)

	// locked — accessor property (dynamic getter, no setter).
	getter := rt.ToValue(func(goja.FunctionCall) goja.Value {
		return rt.ToValue(rs.Locked())
	})
	_ = obj.DefineAccessorProperty("locked", getter, goja.Undefined(),
		goja.FLAG_FALSE, goja.FLAG_TRUE)

	// getReader() — returns a ReadableStreamDefaultReader JS wrapper.
	_ = obj.Set("getReader", func(call goja.FunctionCall) goja.Value {
		reader, err := rs.GetReader()
		if err != nil {
			panic(rt.NewGoError(err))
		}
		return wrapReaderJS(rt, adapter, reader, promisify)
	})

	// cancel() — cancels the stream.
	_ = obj.Set("cancel", func(call goja.FunctionCall) goja.Value {
		if err := rs.Cancel(); err != nil {
			panic(rt.NewGoError(err))
		}
		return goja.Undefined()
	})

	return obj
}

// wrapReaderJS returns a goja.Object exposing the
// ReadableStreamDefaultReader to JavaScript.
//
// read() returns Promise<{value: string, done: boolean}>.  The blocking
// Read() call runs in a goroutine; the Promise resolves on the event loop.
func wrapReaderJS(rt *goja.Runtime, adapter *gojaeventloop.Adapter, reader *ReadableStreamDefaultReader, promisify PromisifyFunc) *goja.Object {
	obj := rt.NewObject()

	_ = obj.Set("read", func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := adapter.JS().NewChainedPromise()

		promisify(context.Background(), func(ctx context.Context) (any, error) {
			data, done, err := reader.Read()
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
					_ = result.Set("value", string(data))
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

	_ = obj.Set("releaseLock", func(call goja.FunctionCall) goja.Value {
		reader.ReleaseLock()
		return goja.Undefined()
	})

	return obj
}
