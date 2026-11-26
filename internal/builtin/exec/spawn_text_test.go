package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func execTestOwner(t *testing.T, loop *goeventloop.Loop, fn func() error) {
	t.Helper()
	completed := make(chan error, 1)
	if err := loop.Submit(func() { completed <- fn() }); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runtime owner callback did not finish")
	}
}

func TestReadableStream_PreservesUTF8AcrossByteChunks(t *testing.T) {
	skipSlow(t)
	vm, adapter, loop := newJSTestRuntime(t)
	const payload = "🧪 café 世界\n"
	raw := []byte(payload)
	read := func(context.Context) (string, bool, error) {
		if len(raw) == 0 {
			return "", true, nil
		}
		value := string(raw[:1])
		raw = raw[1:]
		return value, false, nil
	}
	execTestOwner(t, loop, func() error {
		return vm.Set("stream", wrapReadableStream(context.Background(), vm, adapter, loop, read))
	})
	got := evalOnLoop(t, vm, loop, `(async function () {
		var parts = [];
		for (;;) {
			var chunk = await stream.read();
			if (chunk.done) {
				__got({text: parts.join(""), truncated: !!chunk.truncated});
				return;
			}
			parts.push(chunk.value);
		}
	})().catch(function (e) { __got({error: String(e)}); });`)
	if got["text"] != payload || got["truncated"] != false {
		t.Fatalf("stream=%v, want exact UTF-8 payload %q and complete EOF", got, payload)
	}
}

func TestReadableStream_UTF8MatchesWholeString(t *testing.T) {
	skipSlow(t)
	vm, adapter, loop := newJSTestRuntime(t)
	payloads := []string{
		"", "é", "界", "🧪", "ASCII é界🧪 tail",
		"\xff\x80\xc0\xaf", "\xe2(\xa1", "\xed\xa0\x80",
		"A\xe2", "A\xe2\x82", "A\xf0\x9f\xa7",
		"\xf0\x9f\xa7\xaa\xe2\x82",
	}
	for _, payload := range payloads {
		for split := 0; split <= len(payload); split++ {
			for _, truncated := range []bool{false, true} {
				name := fmt.Sprintf("%x/split%d/truncated%v", payload, split, truncated)
				t.Run(name, func(t *testing.T) {
					chunks := []string{payload[:split], payload[split:]}
					read := func(context.Context) (string, bool, error) {
						if len(chunks) != 0 {
							chunk := chunks[0]
							chunks = chunks[1:]
							return chunk, false, nil
						}
						if truncated {
							return "", true, errOutputStreamCutShort
						}
						return "", true, nil
					}
					execTestOwner(t, loop, func() error {
						if err := vm.Set("expected", payload); err != nil {
							return err
						}
						return vm.Set("stream", wrapReadableStream(context.Background(), vm, adapter, loop, read))
					})
					got := evalOnLoop(t, vm, loop, `(async function () {
						var parts = [];
						for (;;) {
							var chunk = await stream.read();
							if (chunk.done) {
								var repeated = await stream.read();
								__got({
									matches: JSON.stringify(parts.join("")) === JSON.stringify(expected),
									truncated: !!chunk.truncated,
									repeated: repeated.done && !!repeated.truncated === !!chunk.truncated
								});
								return;
							}
							parts.push(chunk.value);
						}
					})().catch(function (e) { __got({error: String(e)}); });`)
					if got["matches"] != true || got["truncated"] != truncated || got["repeated"] != true {
						t.Fatalf("stream=%v, want whole-string conversion and truncated=%v", got, truncated)
					}
				})
			}
		}
	}
}

func TestStreamTextReader_CancellationRetainsSuffix(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	step := 0
	reader := streamTextReader{read: func(ctx context.Context) (string, bool, error) {
		step++
		switch step {
		case 1:
			cancel()
			return "\xf0\x9f", false, nil
		case 2:
			return "", false, ctx.Err()
		case 3:
			return "\xa7\xaa", false, nil
		default:
			return "", true, nil
		}
	}}
	if data, done, err := reader.readText(ctx); data != "" || done || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read=(%q, %v, %v)", data, done, err)
	}
	if reader.suffixLen != 2 {
		t.Fatalf("cancellation retained %d suffix bytes, want 2", reader.suffixLen)
	}
	if data, done, err := reader.readText(context.Background()); data != "🧪" || done || err != nil {
		t.Fatalf("retry=(%q, %v, %v), want the completed code point", data, done, err)
	}
	if data, done, err := reader.readText(context.Background()); data != "" || !done || err != nil {
		t.Fatalf("terminal read=(%q, %v, %v)", data, done, err)
	}
}

func TestStreamTextReader_CanceledContextKeepsQueuedDataAndEOF(t *testing.T) {
	child := &ChildProcess{stdout: newPipeQueue(16)}
	for _, b := range []byte("🧪") {
		child.stdout.push(readChunk{data: []byte{b}}, nil)
	}
	child.stdout.close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := streamTextReader{read: child.ReadStdoutContext}
	if data, done, err := reader.readText(ctx); data != "🧪" || done || err != nil {
		t.Fatalf("queued data=(%q, %v, %v), want the complete queued code point", data, done, err)
	}
	if data, done, err := reader.readText(ctx); data != "" || !done || err != nil {
		t.Fatalf("queued EOF=(%q, %v, %v), want clean EOF", data, done, err)
	}
}

func TestStreamTextReader_TerminalErrorFollowsPartialCodePoint(t *testing.T) {
	endErr := errors.New("terminal read failed")
	step := 0
	reader := streamTextReader{read: func(context.Context) (string, bool, error) {
		step++
		if step == 1 {
			return "\xe2\x82", false, nil
		}
		return "", true, endErr
	}}
	if data, done, err := reader.readText(context.Background()); data != "\xe2\x82" || done || err != nil {
		t.Fatalf("partial final code point=(%q, %v, %v)", data, done, err)
	}
	for range 2 {
		if data, done, err := reader.readText(context.Background()); data != "" || !done || !errors.Is(err, endErr) {
			t.Fatalf("terminal error=(%q, %v, %v)", data, done, err)
		}
	}
}

func TestReadableStream_ConcurrentReadsKeepInvocationOrder(t *testing.T) {
	skipSlow(t)
	vm, adapter, loop := newJSTestRuntime(t)
	raw := []byte("é界🧪X")
	read := func(context.Context) (string, bool, error) {
		if len(raw) == 0 {
			return "", true, nil
		}
		value := string(raw[:1])
		raw = raw[1:]
		return value, false, nil
	}
	execTestOwner(t, loop, func() error {
		return vm.Set("stream", wrapReadableStream(context.Background(), vm, adapter, loop, read))
	})
	got := evalOnLoop(t, vm, loop, `(async function () {
		var reads = [];
		for (var i = 0; i < 5; i++) reads.push(stream.read());
		var results = await Promise.all(reads);
		__got({values: results.map(function (x) { return x.value; }),
			done: results.map(function (x) { return x.done; })});
	})().catch(function (e) { __got({error: String(e)}); });`)
	wantValues := []any{"é", "界", "🧪", "X", nil}
	wantDone := []any{false, false, false, false, true}
	if !reflect.DeepEqual(got["values"], wantValues) || !reflect.DeepEqual(got["done"], wantDone) {
		t.Fatalf("concurrent reads=%v, want ordered values=%v done=%v", got, wantValues, wantDone)
	}
}

func TestWaitStreamReadTurn_CloseReleasesSkippedPredecessor(t *testing.T) {
	skipSlow(t)
	synctest.Test(t, func(t *testing.T) {
		loop, err := goeventloop.New()
		if err != nil {
			t.Fatal(err)
		}
		defer loop.Close()
		// Immediate Close can skip an admitted worker before it runs any
		// code, so its successor's predecessor gate will never close.
		previous := make(chan struct{})
		result := make(chan bool, 1)
		go func() { result <- waitStreamReadTurn(previous, loop.Done()) }()
		synctest.Wait()
		select {
		case <-result:
			t.Fatal("read turn bypassed a live predecessor")
		default:
		}
		if err := loop.Close(); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if <-result {
			t.Fatal("terminal wake authorized decoder access")
		}
		// A successor may see both gates ready after an earlier worker took
		// the terminal path. The terminal precheck must win deterministically.
		close(previous)
		for range 100 {
			if waitStreamReadTurn(previous, loop.Done()) {
				t.Fatal("a released terminal predecessor authorized decoder access")
			}
		}
	})
}

func TestReadableStream_ImmediateCloseSettlesQueuedReads(t *testing.T) {
	skipSlow(t)
	vm, adapter, loop := newJSTestRuntime(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	readFinished := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var calls atomic.Int32
	read := func(context.Context) (string, bool, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
			close(readFinished)
		}
		return "", true, nil
	}
	var promises []*goja.Promise
	execTestOwner(t, loop, func() error {
		if err := vm.Set("stream", wrapReadableStream(context.Background(), vm, adapter, loop, read)); err != nil {
			return err
		}
		if _, err := vm.RunString(`var queued = [stream.read(), stream.read(), stream.read()];
			queued.forEach(function (p) { p.catch(function () {}); });`); err != nil {
			return err
		}
		array := vm.Get("queued").ToObject(vm)
		for _, index := range []string{"0", "1", "2"} {
			promises = append(promises, array.Get(index).Export().(*goja.Promise))
		}
		return nil
	})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first read did not enter")
	}
	closed := make(chan error, 1)
	go func() { closed <- loop.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("immediate close waited for the blocked read")
	}
	<-adapter.Done()
	// Done is the terminal VM barrier, so inspecting these promise states
	// cannot race an owner callback. The first raw read is still blocked.
	for i, promise := range promises {
		if promise.State() != goja.PromiseStateRejected {
			t.Fatalf("read %d state=%v, want terminal rejection", i, promise.State())
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("raw reads=%d, want only the original decoder owner", got)
	}
	select {
	case <-readFinished:
		t.Fatal("first raw read unexpectedly completed before release")
	default:
	}
	releaseOnce.Do(func() { close(release) })
	<-readFinished
}

func TestReadableStream_PumpTruncationIsStructural(t *testing.T) {
	skipSlow(t)
	for _, cause := range []string{"read error after reap", "aborted queue admission"} {
		t.Run(cause, func(t *testing.T) {
			child := &ChildProcess{stdout: newPipeQueue(4), abort: make(chan struct{}), done: make(chan struct{})}
			child.recordExit(nil, nil)
			close(child.done)
			const prefix = "ok \xe2"
			if cause == "read error after reap" {
				child.pumpPipe(&prefixThenEIOReader{prefix: []byte(prefix)}, &child.stdout)
			} else {
				child.AbortDrain()
				child.pumpPipe(io.NopCloser(strings.NewReader(prefix+"\x82\xac tail")), &child.stdout)
			}
			vm, adapter, loop := newJSTestRuntime(t)
			execTestOwner(t, loop, func() error {
				if err := vm.Set("expected", prefix); err != nil {
					return err
				}
				return vm.Set("stream", wrapReadableStream(context.Background(), vm, adapter, loop, child.ReadStdoutContext))
			})
			got := evalOnLoop(t, vm, loop, `(async function () {
				var text = "";
				for (;;) {
					var part = await stream.read();
					if (part.done) {
						__got({matches: JSON.stringify(text) === JSON.stringify(expected), truncated: !!part.truncated});
						return;
					}
					text += part.value;
				}
			})().catch(function (e) { __got({error: String(e)}); });`)
			if got["matches"] != true || got["truncated"] != true {
				t.Fatalf("pump's JS result=%v, want the full retained prefix then truncated:true", got)
			}
		})
	}
}

func TestSpawnJS_NativeSmallCapPreservesUnicodeAfterWait(t *testing.T) {
	skipSlow(t)
	vm, adapter, loop := newJSTestRuntime(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	execTestOwner(t, loop, func() error {
		if err := vm.Set("spawn", jsSpawn(ctx, vm, adapter, loop, SpawnChild)); err != nil {
			return err
		}
		if err := vm.Set("command", os.Args[0]); err != nil {
			return err
		}
		return vm.Set("helperEnv", map[string]string{outputLifecycleHelperEnv: "unicode", "GORACE": "atexit_sleep_ms=0"})
	})
	got := evalOnLoop(t, vm, loop, `(async function () {
		var child = await spawn(command, [], {env: helperEnv, maxBufferedOutputBytes: 1});
		var status = await child.wait();
		async function drain(stream) {
			var text = "";
			for (;;) {
				var part = await stream.read();
				if (part.done) return {text: text, truncated: !!part.truncated};
				text += part.value;
			}
		}
		var streams = await Promise.all([drain(child.stdout), drain(child.stderr)]);
		__got({code: status.code, stdout: streams[0].text, stderr: streams[1].text,
			truncated: streams[0].truncated || streams[1].truncated});
	})().catch(function (e) { __got({error: String(e)}); });`)
	if got["code"] != int64(0) || got["stdout"] != "🧪 café 世界\n" || got["stderr"] != "ready" || got["truncated"] != false {
		t.Fatalf("native small-cap result=%v", got)
	}
}

func TestStreamTextReader_ValidChunksHaveCompleteCodePoints(t *testing.T) {
	const payload = "aé界🧪z"
	for width := 1; width <= len(payload); width++ {
		raw := payload
		reader := streamTextReader{read: func(context.Context) (string, bool, error) {
			if raw == "" {
				return "", true, nil
			}
			n := min(width, len(raw))
			chunk := raw[:n]
			raw = raw[n:]
			return chunk, false, nil
		}}
		var joined strings.Builder
		for {
			data, done, err := reader.readText(context.Background())
			if err != nil || !utf8.ValidString(data) || reader.suffixLen > utf8.UTFMax-1 {
				t.Fatalf("width=%d data=%q done=%v err=%v suffix=%d", width, data, done, err, reader.suffixLen)
			}
			joined.WriteString(data)
			if done {
				break
			}
		}
		if joined.String() != payload {
			t.Fatalf("width=%d joined=%q, want %q", width, joined.String(), payload)
		}
	}
}
