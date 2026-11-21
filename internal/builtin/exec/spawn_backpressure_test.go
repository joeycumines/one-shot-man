package exec

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// retainedBytes reports the queue's currently retained payload bytes.
func (q *pipeQueue) retainedBytes() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, c := range q.items {
		n += len(c.data)
	}
	return n
}

func (q *pipeQueue) chunkCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// highVolumeWriterEnv marks the re-executed test binary as the helper.
const highVolumeWriterEnv = "OSM_EXEC_HIGHVOLUME_HELPER"

// highVolumeWriterCommand returns a command that writes to stdout as fast as
// the pipe accepts, indefinitely (until killed). The writer must be an
// out-of-process program: on Windows, cmd.exe's `for /L` loop only ever runs
// in-process (the parser substitutes 0 as the end bound, so the loop is
// finite and the child exits before the pump can retain anything), which made
// the bounded-queue assertion vacuous there. The re-executed test binary
// (OSM_EXEC_HIGHVOLUME_HELPER=1) is a genuine infinite writer on every
// platform and is what SpawnChild runs.
func highVolumeWriterCommand() (string, []string) {
	return os.Args[0], []string{"-test.run=TestZZHighVolumeWriterHelper"}
}

// A sustained writer must not grow the retained output queue past its cap.
func TestSpawnChild_UnreadOutputIsBounded(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	const capBytes = DefaultQueueCapBytes
	command, args := highVolumeWriterCommand()
	child, err := SpawnChild(context.Background(), SpawnConfig{
		Command:    command,
		Args:       args,
		Env:        map[string]string{highVolumeWriterEnv: "1"},
		EnvReplace: false,
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}
	t.Cleanup(func() {
		if err := child.Kill(); err != nil {
			t.Errorf("cleanup kill: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := child.WaitContext(ctx); errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("cleanup wait: %v", err)
		}
	})

	// Let the pump run with zero reads for a fixed window.
	time.Sleep(500 * time.Millisecond)

	retained := child.stdout.retainedBytes()
	chunks := child.stdout.chunkCount()
	t.Logf("retained=%d bytes in %d chunks after 500ms with zero reads (cap=%d)", retained, chunks, capBytes)
	if retained > capBytes {
		t.Fatalf("DEFECT: retained %d bytes in %d chunks exceeds cap %d with zero reads — queue is unbounded", retained, chunks, capBytes)
	}

	// wait() must still resolve promptly even with a full queue and zero reads
	// (reaping stays decoupled), then kill to stop the writer.
	waitDone := make(chan int, 1)
	go func() {
		code, _ := child.WaitContext(context.Background())
		waitDone <- code
	}()
	if err := child.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	select {
	case code := <-waitDone:
		// On Windows the helper can observe the closed stdout reader and
		// take its explicit exit-0 path before the process-tree signal. Its
		// recorded status is valid either way; this test checks bounded
		// output and completion, not which exit mechanism won that race.
		t.Logf("writer exit code after abort: %d", code)
	case <-time.After(5 * time.Second):
		t.Fatal("wait() did not resolve after kill with a full unread queue")
	}
	// Kill must settle the stream. EOF can be clean if every produced byte
	// was retained; otherwise it must report the incomplete capture.
	readCtx, cancelRead := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRead()
	for {
		_, done, err := child.ReadStdoutContext(readCtx)
		if err != nil {
			if !errors.Is(err, errOutputStreamCutShort) {
				t.Fatalf("terminal read err = %v, want errOutputStreamCutShort after kill", err)
			}
			break
		}
		if done {
			break
		}
		if retained := child.stdout.retainedBytes(); retained > capBytes {
			t.Fatalf("retained %d grew past cap while draining a killed child", retained)
		}
	}
}

// A full queue must durably block without holding its mutex, then resume
// when a reader frees room. synctest.Wait requires actual blocking, not a
// scheduler-dependent observation after a sleep.
func TestPipeQueuePushBlocksWhenFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := newPipeQueue(16)
		if !q.push(readChunk{data: []byte("0123456789abcdef")}, nil) {
			t.Fatal("initial push failed")
		}
		result := make(chan bool, 1)
		go func() { result <- q.push(readChunk{data: []byte("next")}, nil) }()
		synctest.Wait()
		select {
		case <-result:
			t.Fatal("push did not block at the capacity limit")
		default:
		}
		if _, ok, _, _ := q.pop(); !ok {
			t.Fatal("full queue was empty")
		}
		synctest.Wait()
		if !<-result {
			t.Fatal("push did not retain the chunk after room was freed")
		}
		if got := q.retainedBytes(); got != 4 {
			t.Fatalf("retained=%d, want 4", got)
		}
	})
}

func TestPipeQueuePushUnblocksOnClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := newPipeQueue(4)
		q.push(readChunk{data: []byte("abcd")}, nil)
		result := make(chan bool, 1)
		go func() { result <- q.push(readChunk{data: []byte("efgh")}, nil) }()
		synctest.Wait()
		q.closeTruncated()
		synctest.Wait()
		if <-result {
			t.Fatal("push admitted output after queue closure")
		}
		if got := q.retainedBytes(); got != 4 {
			t.Fatalf("retained=%d, want the original 4-byte prefix", got)
		}
	})
}

func TestPipeQueuePushUnblocksOnAbort(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := newPipeQueue(4)
		abort := make(chan struct{})
		q.push(readChunk{data: []byte("abcd")}, abort)
		result := make(chan bool, 1)
		go func() { result <- q.push(readChunk{data: []byte("efgh")}, abort) }()
		synctest.Wait()
		close(abort)
		synctest.Wait()
		if <-result {
			t.Fatal("aborted capacity wait admitted output")
		}
		if got := q.retainedBytes(); got != 4 {
			t.Fatalf("retained=%d, want the original 4-byte prefix", got)
		}
	})
}

// A full queue parks the pump in push, stopping further pipe reads. The child
// then blocks once the kernel pipe also fills.
func TestPumpPipeBlocksInReadWhenQueueFull(t *testing.T) {
	t.Parallel()

	const cap = 4 * DefaultBufSize
	q := newPipeQueue(cap)
	done := make(chan struct{})
	abort := make(chan struct{})
	child := &ChildProcess{done: done, abort: abort}

	// A reader that yields an endless supply of full-size chunks.
	r := newInfiniteReader()
	pumpDone := make(chan struct{})
	go func() {
		child.pumpPipe(r, &q)
		close(pumpDone)
	}()

	// Let the pump fill the queue.
	deadline := time.Now().Add(2 * time.Second)
	for q.retainedBytes() < cap && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := q.retainedBytes(); got > cap {
		t.Fatalf("retained %d exceeds cap %d", got, cap)
	}
	// With the queue full and no pop, retained must stay bounded: sample twice.
	time.Sleep(100 * time.Millisecond)
	if got := q.retainedBytes(); got > cap {
		t.Fatalf("retained grew past cap while pump should be blocked: %d > %d", got, cap)
	}

	// Unblock the pump via abort; it must close the stream as incomplete.
	close(abort)
	_ = r.Close()
	select {
	case <-pumpDone:
	case <-time.After(2 * time.Second):
		t.Fatal("pump did not finish after abort while blocked on a full queue")
	}
}

// infiniteReader serves DefaultBufSize bytes per Read, forever, until Closed.
type infiniteReader struct {
	closed chan struct{}
	once   sync.Once
	buf    []byte
}

func newInfiniteReader() *infiniteReader {
	return &infiniteReader{closed: make(chan struct{})}
}

func (r *infiniteReader) Read(p []byte) (int, error) {
	if r.buf == nil {
		r.buf = make([]byte, DefaultBufSize)
		for i := range r.buf {
			r.buf[i] = 'x'
		}
	}
	select {
	case <-r.closed:
		return 0, errFakeClosed
	default:
	}
	return copy(p, r.buf), nil
}

func (r *infiniteReader) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

// The pump limits each read to the configured cap, so a cap smaller than
// DefaultBufSize neither overfills an empty queue nor deadlocks the first push.
func TestPumpPipe_SmallCapRespectsLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const capacity = 4
		const payload = "a small cap still preserves every output byte"
		q := newPipeQueue(capacity)
		child := &ChildProcess{}
		done := make(chan struct{})
		go func() { child.pumpPipe(io.NopCloser(strings.NewReader(payload)), &q); close(done) }()
		var got strings.Builder
		for {
			synctest.Wait()
			if n := q.retainedBytes(); n > capacity {
				t.Fatalf("retained=%d exceeds capacity=%d", n, capacity)
			}
			chunk, ok, closed, _ := q.pop()
			if ok {
				got.Write(chunk.data)
				continue
			}
			if closed {
				break
			}
			t.Fatal("pump stopped without queuing output or closing")
		}
		<-done
		if got.String() != payload || q.cutShort() {
			t.Fatalf("capture=(%q,truncated=%v), want (%q,false)", got.String(), q.cutShort(), payload)
		}
	})
}

// TestSpawnChild_CapBelowChunkSizeStillDrains checks that maxBufferedOutputBytes below DefaultBufSize must not
// wedge the pump; the full output must still drain.
func TestSpawnChild_CapBelowChunkSizeStillDrains(t *testing.T) {
	skipSlow(t)
	skipIfWindows(t)
	t.Parallel()

	child, err := SpawnChild(context.Background(), SpawnConfig{
		Command:                "sh",
		Args:                   []string{"-c", `i=0; while [ $i -lt 500 ]; do printf '%064d\n' $i; i=$((i+1)); done`},
		MaxBufferedOutputBytes: 1024, // below one 8 KiB chunk
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}
	total := 0
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, done, err := child.ReadStdoutContext(context.Background())
		if err != nil {
			t.Fatalf("read err = %v", err)
		}
		if done {
			break
		}
		total += len(data)
		if time.Now().After(deadline) {
			t.Fatalf("drain wedged with sub-chunk cap (got %d bytes)", total)
		}
	}
	if total != 500*65 {
		t.Fatalf("drained %d bytes, want exactly %d (500 lines x 65 bytes: 64 digits + newline) — backpressure must not drop bytes", total, 500*65)
	}
	if code, err := child.WaitContext(context.Background()); err != nil || code != 0 {
		t.Fatalf("wait = (%d, %v), want (0, nil)", code, err)
	}
}

var errFakeClosed = errors.New("fake pipe closed")

// Delaying consumption must keep the queue bounded while allowing the entire
// finite output to be delivered once the consumer starts reading.
func TestSpawnChild_ChildBlockedPastCap(t *testing.T) {
	skipSlow(t)
	skipIfWindows(t)
	t.Parallel()

	// Writer emits 2000 lines of 64 bytes (~128 KiB) then exits. Cap = 8 KiB.
	const capBytes = 8 * 1024
	child, err := SpawnChild(context.Background(), SpawnConfig{
		Command:                "sh",
		Args:                   []string{"-c", `i=0; while [ $i -lt 2000 ]; do printf '%064d\n' $i; i=$((i+1)); done`},
		MaxBufferedOutputBytes: capBytes,
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}

	// With zero reads, the queued payload must stay within its cap.
	time.Sleep(300 * time.Millisecond)
	if got := child.stdout.retainedBytes(); got > capBytes {
		t.Fatalf("retained %d exceeds cap %d", got, capBytes)
	}
	// Drain the full numbered output and verify that bounding the queue did
	// not lose any payload.
	var total int
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, done, err := child.ReadStdoutContext(context.Background())
		if err != nil {
			t.Fatalf("read err = %v", err)
		}
		if done {
			break
		}
		total += len(data)
		if child.stdout.retainedBytes() > capBytes {
			t.Fatalf("retained grew past cap while draining: %d", child.stdout.retainedBytes())
		}
		if time.Now().After(deadline) {
			t.Fatal("drain did not finish in 10s")
		}
	}
	if total != 2000*65 {
		t.Fatalf("drained %d bytes, want exactly 130000 (2000 lines x 65 bytes)", total)
	}
	code, werr := child.WaitContext(context.Background())
	if werr != nil || code != 0 {
		t.Fatalf("wait = (%d, %v), want (0, nil)", code, werr)
	}
}

// TestSpawnChild_WaitResolvesWithFullUnreadQueue pins decoupling: wait() must
// resolve when the child is reaped even with zero reads and a full queue. The
// writer here blocks once the kernel pipe + capped queue fill, so it runs
// until killed; wait() resolving after the kill proves reaping is independent
// of stream consumption.
func TestSpawnChild_WaitResolvesWithFullUnreadQueue(t *testing.T) {
	skipSlow(t)
	skipIfWindows(t)
	t.Parallel()

	// Infinite writer; cap 8 KiB. The queue fills and stays full with zero reads.
	child, err := SpawnChild(context.Background(), SpawnConfig{
		Command:                "sh",
		Args:                   []string{"-c", `while true; do printf '%064d\n' 1; done`},
		MaxBufferedOutputBytes: 8 * 1024,
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}
	// Let the queue fill.
	time.Sleep(300 * time.Millisecond)
	if got := child.stdout.retainedBytes(); got > 8*1024 {
		t.Fatalf("retained %d exceeds cap before wait", got)
	}
	// Kill the child: the reaper must record the exit and wait() must resolve
	// with the killed status even though the queue is still full and unread.
	if err := child.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	type res struct {
		code int
		err  error
	}
	done := make(chan res, 1)
	go func() {
		code, err := child.WaitContext(context.Background())
		done <- res{code, err}
	}()
	select {
	case r := <-done:
		// A SIGKILLed child records code -1 with waitErr "signal: killed" —
		// that IS the recorded reap. What matters is that wait() returned at
		// all with a full unread queue, not that err is nil.
		if r.code == 0 {
			t.Fatalf("killed child reported code 0 with a full unread queue")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("wait() did not resolve with a full unread queue — reaping re-coupled to consumption")
	}
	// Queue is still bounded after reap.
	if got := child.stdout.retainedBytes(); got > 8*1024 {
		t.Fatalf("retained %d after reap exceeds cap", got)
	}
	// Aborted admission ends the pump with a truncated prefix. A pump that
	// retained every byte and reached EOF during grace may finish cleanly.
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, done, err := child.ReadStdoutContext(context.Background())
		if err != nil || done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stream did not terminate within 5s of reap+kill (retained=%d chunks=%d)", child.stdout.retainedBytes(), child.stdout.chunkCount())
		}
	}
}

// TestSpawnChild_KillUnblocksStuckPump: a child writing past the cap with zero
// reads leaves the pump blocked in push; kill() must promptly unblock it and
// end the incomplete capture.
func TestSpawnChild_KillUnblocksStuckPump(t *testing.T) {
	skipSlow(t)
	skipIfWindows(t)
	t.Parallel()

	child, err := SpawnChild(context.Background(), SpawnConfig{
		Command:                "sh",
		Args:                   []string{"-c", `while true; do printf 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n'; done`},
		MaxBufferedOutputBytes: 8 * 1024,
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}
	// Let the queue fill; pump is now blocked in push.
	time.Sleep(300 * time.Millisecond)
	if got := child.stdout.retainedBytes(); got > 8*1024 {
		t.Fatalf("retained %d exceeds cap before kill", got)
	}
	start := time.Now()
	if err := child.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	settled := make(chan struct{})
	go func() { <-child.settled; close(settled) }()
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatalf("settlePumps did not unblock a pump stuck on a full queue within 5s of kill (took >%s)", time.Since(start))
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("kill-to-settle took %s, want prompt (<2s)", elapsed)
	}
	// wait resolves too.
	waitDone := make(chan struct{})
	go func() { _, _ = child.WaitContext(context.Background()); close(waitDone) }()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("wait() did not resolve after kill unblocked the pump")
	}
}

// TestSpawnChild_TimeoutUnblocksStuckPump: timeoutMs must abort the drain and
// unblock a pump stuck on a full queue through the spawn context.
func TestSpawnChild_TimeoutUnblocksStuckPump(t *testing.T) {
	skipSlow(t)
	_, runJS := asyncTestEnv(t)
	value, err := runJS(`
		const child = await exec.spawn("sh", ["-c", "while true; do printf 'yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy\\n'; done"], {timeoutMs: 500, maxBufferedOutputBytes: 8192});
		const w = await child.wait();
		// after timeout+kill, drain to terminal
		let truncated = false;
		while (true) {
			const chunk = await child.stdout.read();
			if (chunk.done) { truncated = !!chunk.truncated; break; }
		}
		__collect(JSON.stringify({code: w.code, truncated}));
	`)
	if err != nil {
		t.Fatalf("runJS: %v", err)
	}
	got := value.String()
	t.Logf("result: %s", got)
	if !strings.Contains(got, `"code":-1`) {
		t.Fatalf("wait code = %s, want -1 (killed)", got)
	}
}

// The requested cap must reach the actual native child launched by JavaScript.
// ASCII keeps JS string length equal to payload bytes, so every returned chunk
// can be checked against the requested bound while completeness is verified.
func TestSpawnOptsMaxBufferedOutputBytesIsHonored(t *testing.T) {
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
		return vm.Set("helperEnv", map[string]string{outputLifecycleHelperEnv: "ascii", "GORACE": "atexit_sleep_ms=0"})
	})
	got := evalOnLoop(t, vm, loop, `(async function () {
		var child = await spawn(command, [], {env: helperEnv, maxBufferedOutputBytes: 4});
		var status = await child.wait();
		async function drain(stream) {
			var text = "", maxChunk = 0;
			for (;;) {
				var chunk = await stream.read();
				if (chunk.done) return {text: text, maxChunk: maxChunk, truncated: !!chunk.truncated};
				text += chunk.value;
				maxChunk = Math.max(maxChunk, chunk.value.length);
			}
		}
		var streams = await Promise.all([drain(child.stdout), drain(child.stderr)]);
		__got({code: status.code, stdout: streams[0].text, stderr: streams[1].text,
			bounded: streams.every(function (s) { return s.maxChunk <= 4 && !s.truncated; })});
	})().catch(function (e) { __got({error: String(e)}); });`)
	if got["code"] != int64(0) || got["stdout"] != "0123456789abcdef" || got["stderr"] != "ready" || got["bounded"] != true {
		t.Fatalf("native JS option result=%v", got)
	}
}

// TestSpawnChild_EngineBaseCtxShutdownUnblocksStuckPumpGo is the
// engine-shutdown proof: it drives trackChildProcessLifetime with a base
// context, lets a real capped queue fill (pump blocked in push with zero
// reads), then cancels the base context and asserts the child settles and
// wait() resolves within deadlines.
func TestSpawnChild_EngineBaseCtxShutdownUnblocksStuckPumpGo(t *testing.T) {
	skipSlow(t)
	skipIfWindows(t)
	t.Parallel()

	child, err := SpawnChild(context.Background(), SpawnConfig{
		Command:                "sh",
		Args:                   []string{"-c", `while true; do printf 'qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq\n'; done`},
		MaxBufferedOutputBytes: 8 * 1024,
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}

	runtime := goja.New()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	baseCtx, baseCancel := context.WithCancel(context.Background())
	loopCtx, loopCancel := context.WithCancel(context.Background())
	if err := loop.Submit(func() { trackChildProcessLifetime(baseCtx, runtime, adapter, child) }); err != nil {
		t.Fatalf("submit: %v", err)
	}
	go loop.Run(loopCtx)
	t.Cleanup(func() {
		baseCancel()
		loopCancel()
		_ = loop.Shutdown(context.Background())
	})

	// Let the queue fill and the pump block in push.
	time.Sleep(400 * time.Millisecond)
	if got := child.stdout.retainedBytes(); got > 8*1024 {
		t.Fatalf("retained %d exceeds cap before shutdown", got)
	}

	// Engine base-context shutdown: the lifetime tracker kills the child, and
	// Kill -> AbortDrain wakes the blocked push, preserving its queued prefix
	// and ending the stream as truncated when admission fails.
	baseCancel()
	settled := make(chan struct{})
	go func() { <-child.settled; close(settled) }()
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("base-ctx shutdown did not settle a child whose pump was blocked on a full queue within 5s")
	}
	waitDone := make(chan struct{})
	go func() { _, _ = child.WaitContext(context.Background()); close(waitDone) }()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("wait() did not resolve after base-ctx shutdown with a full unread queue")
	}
}
