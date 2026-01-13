package bt

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	bt "github.com/joeycumines/go-behaviortree"
	"github.com/stretchr/testify/require"
)

// TestBlackboard_ConcurrentJSLeaves verifies that multiple concurrent JS leaf adapters
// can access the same exposed Blackboard without causing panics or corruption of Go-level
// invariants. It does not assert application-level atomicity for read-then-set sequences
// (those are inherently racy across separate JS calls), but ensures the Go backing store
// remains consistent and free of data races or panics.
func TestBlackboard_ConcurrentJSLeaves(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)
	bb := new(Blackboard)
	bb.Set("counter", 0)
	require.NoError(t, bridge.ExposeBlackboard("bb", bb))

	// Worker increments counter once
	err := bridge.LoadScript("worker.js", `
		async function worker(ctx, args) {
			var c = bb.get("counter");
			if (!c) { c = 0; }
			bb.set("counter", c + 1);
			return bt.success;
		}
	`)
	require.NoError(t, err)

	fn, err := bridge.GetCallable("worker")
	require.NoError(t, err)

	const N = 32
	var wg sync.WaitGroup
	wg.Add(N)

	// Start N adapters concurrently; each increments counter once
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			node := NewJSLeafAdapter(context.TODO(), bridge, fn, nil)

			// Tick the node and wait for completion
			status, err := node.Tick()
			if err != nil {
				// record error in test
				panic(err)
			}
			if status != bt.Running {
				// still acceptable - we still wait for completion
			}

			// Wait up to 2 seconds for completion
			start := time.Now()
			for time.Since(start) < 2*time.Second {
				s, e := node.Tick()
				if s != bt.Running || e != nil {
					// Node has completed (success/failure or error)
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// Verify the counter is within expected bounds (1..N)
	val := bb.Get("counter")
	// goja exports numbers as int64 when set from JS
	var got int64
	switch v := val.(type) {
	case int:
		got = int64(v)
	case int64:
		got = v
	case int32:
		got = int64(v)
	default:
		require.Failf(t, "unexpected counter type", "type=%T val=%v", val, val)
	}

	require.GreaterOrEqual(t, got, int64(1), "counter should be at least 1")
	require.LessOrEqual(t, got, int64(N), "counter should be at most N")
}

func TestBlackboard_LargeDataset(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)
	const size = 20000

	// Insert many keys
	for i := 0; i < size; i++ {
		bb.Set(fmt.Sprintf("k-%d", i), i)
	}

	require.Equal(t, size, bb.Len())

	snap := bb.Snapshot()
	require.Equal(t, size, len(snap))

	// Enumerate keys - ensure no panics and reasonable performance
	keys := bb.Keys()
	require.Equal(t, size, len(keys))
}
