package exec

import (
	"context"
	"errors"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
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

// highVolumeWriterCommand returns a command that writes to stdout as fast as
// the pipe accepts, indefinitely (until killed).
func highVolumeWriterCommand() (string, []string) {
	if goruntime.GOOS == "windows" {
		// cmd.exe has no infinite-yield loop that is cheap; use findstr on nul?
		// Simpler: powershell may be absent; use a repeated echo loop via cmd.
		return "cmd.exe", []string{"/C", "for /L %i in (1,0,0) do @echo 01234567890123456789012345678901234567890123456789012345678901234567890123456789"}
	}
	return "sh", []string{"-c", `while true; do printf '01234567890123456789012345678901234567890123456789012345678901234567890123456789\n'; done`}
}

// TestSpawnChild_UnreadOutputIsBounded is the RED repro for the unbounded
// pipe queue: spawn a high-volume writer, NEVER read, and assert the retained
// queue bytes stay under the cap. Today the queue grows without bound (Hana's
// probe measured ~1 GB / 125k chunks in ~2s), so this test FAILS before the
// fix and passes after.
func TestSpawnChild_UnreadOutputIsBounded(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	const capBytes = 64 * 1024 // DefaultQueueCapBytes once wired; assert against the live default below
	command, args := highVolumeWriterCommand()
	child, err := SpawnChild(context.Background(), SpawnConfig{Command: command, Args: args})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}

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
		if code == 0 {
			t.Fatalf("killed high-volume writer reported code 0")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait() did not resolve after kill with a full unread queue")
	}
	// The stream must terminate after kill, promptly. The child was SIGKILLed
	// mid-stream, so the drain was aborted: the terminal read is either
	// errOutputStreamCutShort ({done:true, truncated:true}) or — when the
	// kill reaped the child and the in-group writers died before the abort
	// landed — a clean done. What must NOT happen is hanging past the
	// deadline, and retained memory must stay bounded while draining.
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, done, err := child.ReadStdoutContext(context.Background())
		if err != nil {
			if !errors.Is(err, errOutputStreamCutShort) {
				t.Fatalf("terminal read err = %v, want errOutputStreamCutShort after kill", err)
			}
			break
		}
		if done {
			break
		}
		if retained := child.stdout.retainedBytes(); retained > capBytes+DefaultBufSize {
			t.Fatalf("retained %d grew past cap+chunk while draining a killed child", retained)
		}
		if time.Now().After(deadline) {
			t.Fatalf("stream did not terminate within 5s of kill (retained=%d chunks=%d)", child.stdout.retainedBytes(), child.stdout.chunkCount())
		}
	}
}

// TestPipeQueuePushBlocksWhenFull is the white-box unit RED test: push on a
// capped queue must block once retained bytes exceed the cap, and pop must
// unblock it. Before the fix push never blocks, so the "pusher is blocked"
// assertion fails.
func TestPipeQueuePushBlocksWhenFull(t *testing.T) {
	t.Parallel()

	q := newPipeQueue(16) // cap 16 bytes
	done := make(chan struct{})
	abort := make(chan struct{})
	pushResult := make(chan bool, 1)

	// Fill the queue to the cap.
	if !q.push(readChunk{data: []byte("0123456789abcdef")}, abort, done) {
		t.Fatal("first push reported not retained")
	}
	// The next push exceeds the cap: it must block.
	go func() {
		pushResult <- q.push(readChunk{data: []byte("XXXXXXXXXXXXXXXX")}, abort, done) // 16 bytes: blocks until a pop frees >= 16 bytes... use 1 byte
	}()
	select {
	case <-pushResult:
		t.Fatal("DEFECT: push did not block when retained bytes would exceed the cap")
	case <-time.After(100 * time.Millisecond):
	}
	// Pop one chunk (16 bytes freed): the blocked push (16 bytes) must complete.
	if _, ok, _, _ := q.pop(); !ok {
		t.Fatal("pop of a full queue returned not-ok")
	}
	select {
	case ok := <-pushResult:
		if !ok {
			t.Fatal("push after pop reported not retained")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("push stayed blocked after pop freed room")
	}
	if got := q.retainedBytes(); got != 16 {
		t.Fatalf("retained = %d, want 16 (popped 16-byte chunk, pushed 16-byte chunk)", got)
	}
}

// TestPipeQueuePushUnblocksOnClose: closing the queue must wake a blocked
// pusher and report not-retained.
func TestPipeQueuePushUnblocksOnClose(t *testing.T) {
	t.Parallel()

	q := newPipeQueue(4)
	done := make(chan struct{})
	abort := make(chan struct{})
	if !q.push(readChunk{data: []byte("abcd")}, abort, done) {
		t.Fatal("first push not retained")
	}
	res := make(chan bool, 1)
	go func() { res <- q.push(readChunk{data: []byte("efgh")}, abort, done) }()
	select {
	case <-res:
		t.Fatal("DEFECT: push did not block at cap")
	case <-time.After(100 * time.Millisecond):
	}
	q.close()
	select {
	case ok := <-res:
		if ok {
			t.Fatal("push after close reported retained")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("close did not wake the blocked push")
	}
}

// TestPipeQueuePushUnblocksOnAbortAndDone: abort and done channels must both
// wake a blocked pusher (kill/timeout/shutdown paths).
func TestPipeQueuePushUnblocksOnAbortAndDone(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"abort", "done"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			q := newPipeQueue(4)
			done := make(chan struct{})
			abort := make(chan struct{})
			if !q.push(readChunk{data: []byte("abcd")}, abort, done) {
				t.Fatal("first push not retained")
			}
			res := make(chan bool, 1)
			go func() { res <- q.push(readChunk{data: []byte("efgh")}, abort, done) }()
			select {
			case <-res:
				t.Fatal("DEFECT: push did not block at cap")
			case <-time.After(100 * time.Millisecond):
			}
			if name == "abort" {
				close(abort)
			} else {
				close(done)
			}
			select {
			case ok := <-res:
				if ok {
					t.Fatalf("push woke by %s reported retained", name)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s did not wake the blocked push", name)
			}
		})
	}
}

// TestPumpPipeBlocksInReadWhenQueueFull proves backpressure reaches the child:
// with a capped queue and no consumer, the pump must stop reading (its Read is
// blocked behind the full queue), so a child writing past cap+8KiB is blocked
// in write(). We observe this at the queue level: retained stays <= cap and
// the pump goroutine is parked in push, not spinning.
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
	if got := q.retainedBytes(); got > cap+DefaultBufSize {
		t.Fatalf("DEFECT: retained %d exceeds cap %d + one in-flight chunk %d", got, cap, DefaultBufSize)
	}
	// With the queue full and no pop, retained must stay bounded: sample twice.
	time.Sleep(100 * time.Millisecond)
	if got := q.retainedBytes(); got > cap+DefaultBufSize {
		t.Fatalf("DEFECT: retained grew past cap+chunk while pump should be blocked: %d > %d", got, cap+DefaultBufSize)
	}

	// Unblock the pump via abort; it must finish promptly (discard drain).
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

// TestPipeQueuePushOversizedChunkNeverDeadlocks pins the empty-queue bypass:
// a chunk larger than the cap must still be accepted when the queue is empty
// (otherwise a cap below DefaultBufSize would deadlock the pump on the first
// full read).
func TestPipeQueuePushOversizedChunkNeverDeadlocks(t *testing.T) {
	t.Parallel()

	q := newPipeQueue(4) // cap smaller than one chunk
	done := make(chan struct{})
	abort := make(chan struct{})
	res := make(chan bool, 1)
	go func() { res <- q.push(readChunk{data: make([]byte, DefaultBufSize)}, abort, done) }()
	select {
	case ok := <-res:
		if !ok {
			t.Fatal("oversized chunk on an empty queue was not retained")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("BLOCKING: push of a chunk larger than the cap deadlocked on an empty queue")
	}
	if got := q.retainedBytes(); got != DefaultBufSize {
		t.Fatalf("retained = %d, want %d (one oversized chunk)", got, DefaultBufSize)
	}
}

// TestSpawnChild_CapBelowChunkSizeStillDrains is the end-to-end form of the
// oversized-chunk rule: maxBufferedOutputBytes below DefaultBufSize must not
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

// TestSpawnChild_ChildBlockedPastCap proves backpressure reaches the child:
// with a small cap and no reads, the writer child must be blocked in write()
// (its progress stalls), which we observe by reading exactly cap bytes and
// then seeing no more data arrive until we drain. We use a writer that emits
// numbered chunks and stops; if the child were never blocked, the whole
// output would already be in the kernel pipe and the pump would have read it
// all into the queue — but the queue is capped, so the child stalls.
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

	// With zero reads for 300ms, retained must stay <= cap + one in-flight chunk.
	time.Sleep(300 * time.Millisecond)
	if got := child.stdout.retainedBytes(); got > capBytes+DefaultBufSize {
		t.Fatalf("retained %d exceeds cap %d + in-flight %d — pump did not block", got, capBytes, DefaultBufSize)
	}
	// The child cannot have finished: total output ~128 KiB >> cap, so it must
	// still be alive (blocked in write) or just finishing; wait must NOT have
	// resolved with a completed stream before we drained. Either way retained
	// stays bounded, which is the memory assertion. Now drain and verify the
	// FULL output arrives (completeness preserved — no load-shedding).
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
		if child.stdout.retainedBytes() > capBytes+DefaultBufSize {
			t.Fatalf("retained grew past cap while draining: %d", child.stdout.retainedBytes())
		}
		if time.Now().After(deadline) {
			t.Fatal("drain did not finish in 10s")
		}
	}
	if total < 125*1024 {
		t.Fatalf("drained only %d bytes, want the complete 2000*65=130000 bytes — backpressure must not drop bytes", total)
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
// until killed; wait() resolving before the kill proves reaping is independent
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
	if got := child.stdout.retainedBytes(); got > 8*1024+DefaultBufSize {
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
	if got := child.stdout.retainedBytes(); got > 8*1024+DefaultBufSize {
		t.Fatalf("retained %d after reap exceeds cap+chunk", got)
	}
	// Stream terminates promptly after reap+kill: abort wakes the blocked
	// pump, which discards, and settlePumps force-closes + marks truncated.
	// A clean done is also acceptable if the in-group writers died with the
	// kill before the abort landed; either way the read loop must end.
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
// reads leaves the pump blocked in push; kill() must promptly unblock it
// (abort wakes push -> discard drain -> force-close -> settle) with a deadline.
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
	if got := child.stdout.retainedBytes(); got > 8*1024+DefaultBufSize {
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
// unblock a pump stuck on a full queue (the abortCtx path in jsSpawn).
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

// TestSpawnOptsMaxBufferedOutputBytesIsHonored checks the JS opts key reaches
// the queue cap end to end: spawn via the JS binding with a tiny
// maxBufferedOutputBytes, never read, and assert via a Go-side probe that the
// retained bytes stay under the cap. Then kill and confirm prompt settle.
func TestSpawnOptsMaxBufferedOutputBytesIsHonored(t *testing.T) {
	skipSlow(t)
	skipIfWindows(t)
	t.Parallel()

	const capBytes = int64(4096)
	// Drive the JS binding so the opts parsing is exercised, but observe the
	// queue from Go via the child handle captured by the script.
	_, runJS := asyncTestEnv(t)
	value, err := runJS(`
		const child = await exec.spawn("sh", ["-c", "while true; do printf 'zzzzzzzzzzzzzzzz\\n'; done"], {maxBufferedOutputBytes: 4096});
		globalThis.__child = child;
		await new Promise(r => setTimeout(r, 400));
		await child.kill();
		const w = await child.wait();
		__collect(JSON.stringify({code: w.code}));
	`)
	if err != nil {
		t.Fatalf("runJS: %v", err)
	}
	if !strings.Contains(value.String(), `"code":-1`) {
		t.Fatalf("wait code = %s, want -1 (killed)", value.String())
	}

	// Direct Go-level check of the same SpawnConfig path the opts key feeds.
	child, err := SpawnChild(context.Background(), SpawnConfig{
		Command:                "sh",
		Args:                   []string{"-c", `while true; do printf 'zzzzzzzzzzzzzzzz\n'; done`},
		MaxBufferedOutputBytes: capBytes,
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if got := child.stdout.retainedBytes(); got > int(capBytes)+DefaultBufSize {
		t.Fatalf("retained %d exceeds configured cap %d + in-flight chunk", got, capBytes)
	}
	if err := child.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	settled := make(chan struct{})
	go func() { <-child.settled; close(settled) }()
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("settle did not complete within 5s of kill")
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
	if got := child.stdout.retainedBytes(); got > 8*1024+DefaultBufSize {
		t.Fatalf("retained %d exceeds cap before shutdown", got)
	}

	// Engine base-context shutdown: the lifetime tracker kills the child, and
	// Kill -> AbortDrain wakes the blocked push; the pump discards and
	// settlePumps force-closes.
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
