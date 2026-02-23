package command

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// T143: controlAdapter — InterruptCurrent, EnqueueTask, GetStatus, dequeue
// ============================================================================

// TestControlAdapter_InterruptCurrent_NoActiveTask verifies that calling
// InterruptCurrent with no active task returns an error.
func TestControlAdapter_InterruptCurrent_NoActiveTask(t *testing.T) {
	t.Parallel()
	a := &controlAdapter{
		taskCh: make(chan<- string, 1),
	}
	err := a.InterruptCurrent()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no active task")
}

// TestControlAdapter_InterruptCurrent_WithActiveNoFn verifies that when there
// is an active task but no interruptFn, InterruptCurrent returns nil (no-op).
func TestControlAdapter_InterruptCurrent_WithActiveNoFn(t *testing.T) {
	t.Parallel()
	a := &controlAdapter{
		activeTask: "some-task",
		taskCh:     make(chan<- string, 1),
	}
	err := a.InterruptCurrent()
	assert.NoError(t, err, "should return nil when fn is nil")
}

// TestControlAdapter_InterruptCurrent_WithFn verifies that when there is both
// an active task and an interruptFn, the fn is called and its error is returned.
func TestControlAdapter_InterruptCurrent_WithFn(t *testing.T) {
	t.Parallel()
	called := false
	a := &controlAdapter{
		activeTask: "running-task",
		taskCh:     make(chan<- string, 1),
		interruptFn: func() error {
			called = true
			return nil
		},
	}
	err := a.InterruptCurrent()
	assert.NoError(t, err)
	assert.True(t, called, "interruptFn should have been called")
}

// TestControlAdapter_InterruptCurrent_FnError verifies that interruptFn errors
// are propagated.
func TestControlAdapter_InterruptCurrent_FnError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("interrupt failed")
	a := &controlAdapter{
		activeTask: "task",
		taskCh:     make(chan<- string, 1),
		interruptFn: func() error {
			return sentinel
		},
	}
	err := a.InterruptCurrent()
	assert.ErrorIs(t, err, sentinel)
}

// TestControlAdapter_EnqueueTask verifies task enqueuing and channel signaling.
func TestControlAdapter_EnqueueTask(t *testing.T) {
	t.Parallel()
	ch := make(chan string, 10)
	a := &controlAdapter{
		taskCh: ch,
	}

	pos, err := a.EnqueueTask("task-1")
	require.NoError(t, err)
	assert.Equal(t, 0, pos, "first task at position 0")

	pos, err = a.EnqueueTask("task-2")
	require.NoError(t, err)
	assert.Equal(t, 1, pos, "second task at position 1")

	// Verify channel received the signals.
	assert.Equal(t, "task-1", <-ch)
	assert.Equal(t, "task-2", <-ch)
}

// TestControlAdapter_EnqueueTask_ChannelFull verifies that when the channel
// is full, the enqueue still succeeds (non-blocking send).
func TestControlAdapter_EnqueueTask_ChannelFull(t *testing.T) {
	t.Parallel()
	ch := make(chan string) // unbuffered
	a := &controlAdapter{
		taskCh: ch,
	}

	// This should not block even though nobody reads from ch.
	pos, err := a.EnqueueTask("overflow-task")
	require.NoError(t, err)
	assert.Equal(t, 0, pos)
	assert.Len(t, a.queue, 1)
}

// TestControlAdapter_GetStatus verifies status snapshot.
func TestControlAdapter_GetStatus(t *testing.T) {
	t.Parallel()
	ch := make(chan string, 10)
	a := &controlAdapter{
		activeTask: "current",
		queue:      []string{"pending-1", "pending-2"},
		taskCh:     ch,
	}

	status := a.GetStatus()
	assert.Equal(t, "current", status.ActiveTask)
	assert.Equal(t, 2, status.QueueDepth)
	assert.Equal(t, []string{"pending-1", "pending-2"}, status.Queue)

	// Verify the returned queue is a copy (not aliased).
	status.Queue[0] = "modified"
	assert.Equal(t, "pending-1", a.queue[0], "original queue should not be modified")
}

// TestControlAdapter_SetActiveClearActive verifies active task management.
func TestControlAdapter_SetActiveClearActive(t *testing.T) {
	t.Parallel()
	a := &controlAdapter{
		taskCh: make(chan<- string, 1),
	}

	a.setActive("working-on-it")
	status := a.GetStatus()
	assert.Equal(t, "working-on-it", status.ActiveTask)

	a.clearActive()
	status = a.GetStatus()
	assert.Empty(t, status.ActiveTask)
}

// TestControlAdapter_Dequeue verifies dequeue removes the first item.
func TestControlAdapter_Dequeue(t *testing.T) {
	t.Parallel()
	a := &controlAdapter{
		queue:  []string{"first", "second", "third"},
		taskCh: make(chan<- string, 1),
	}

	a.dequeue()
	assert.Equal(t, []string{"second", "third"}, a.queue)

	a.dequeue()
	assert.Equal(t, []string{"third"}, a.queue)

	a.dequeue()
	assert.Empty(t, a.queue)

	// Dequeue on empty queue should be a no-op.
	a.dequeue()
	assert.Empty(t, a.queue)
}
