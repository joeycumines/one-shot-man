package scripting

import (
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin"
)

// TestStateManager_AddListener tests the AddListener and notification mechanism.
func TestStateManager_AddListener(t *testing.T) {
	sm := &StateManager{
		listeners:  make(map[int]builtin.StateListener),
		nextListID: 1,
	}

	var notifiedKey string
	var wg sync.WaitGroup
	wg.Add(1)

	listenerID := sm.AddListener(func(key string) {
		notifiedKey = key
		wg.Done()
	})

	if listenerID != 1 {
		t.Errorf("expected listenerID 1, got %d", listenerID)
	}

	// Notify listeners
	sm.notifyListeners("test-key")

	// Wait for listener to be called (with timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("listener was not called within timeout")
	}

	if notifiedKey != "test-key" {
		t.Errorf("expected notifiedKey 'test-key', got %q", notifiedKey)
	}
}

// TestStateManager_RemoveListener tests listener removal.
func TestStateManager_RemoveListener(t *testing.T) {
	sm := &StateManager{
		listeners:  make(map[int]builtin.StateListener),
		nextListID: 1,
	}

	callCount := 0
	listenerID := sm.AddListener(func(key string) {
		callCount++
	})

	// Remove the listener
	sm.RemoveListener(listenerID)

	// Notify - should not call the removed listener
	sm.notifyListeners("test-key")

	// Wait a bit to ensure listener wasn't called
	time.Sleep(10 * time.Millisecond)

	if callCount != 0 {
		t.Errorf("expected callCount 0 after removal, got %d", callCount)
	}
}

// TestStateManager_MultipleListeners tests multiple listeners.
func TestStateManager_MultipleListeners(t *testing.T) {
	sm := &StateManager{
		listeners:  make(map[int]builtin.StateListener),
		nextListID: 1,
	}

	var mu sync.Mutex
	callOrder := []int{}

	id1 := sm.AddListener(func(key string) {
		mu.Lock()
		callOrder = append(callOrder, 1)
		mu.Unlock()
	})
	id2 := sm.AddListener(func(key string) {
		mu.Lock()
		callOrder = append(callOrder, 2)
		mu.Unlock()
	})
	id3 := sm.AddListener(func(key string) {
		mu.Lock()
		callOrder = append(callOrder, 3)
		mu.Unlock()
	})

	// Check IDs are unique and sequential
	if id1 != 1 || id2 != 2 || id3 != 3 {
		t.Errorf("expected IDs 1,2,3, got %d,%d,%d", id1, id2, id3)
	}

	// Notify
	sm.notifyListeners("test")

	// Wait for all to be called
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(callOrder) != 3 {
		t.Errorf("expected 3 listeners called, got %d", len(callOrder))
	}

	// Check all listeners were called (order may vary due to map iteration)
	seen := map[int]bool{}
	for _, id := range callOrder {
		seen[id] = true
	}
	if !seen[1] || !seen[2] || !seen[3] {
		t.Errorf("not all listeners were called: %v", callOrder)
	}
}

// TestStateManager_ConcurrentAddRemove tests thread safety of listener operations.
func TestStateManager_ConcurrentAddRemove(t *testing.T) {
	sm := &StateManager{
		listeners:  make(map[int]builtin.StateListener),
		nextListID: 1,
	}

	const numGoroutines = 10
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // Half add/remove, half notify

	// Add/Remove goroutines
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				id := sm.AddListener(func(key string) {})
				// Remove half the time
				if j%2 == 0 {
					sm.RemoveListener(id)
				}
			}
		}()
	}

	// Notify goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				sm.notifyListeners("concurrent-key")
			}
		}(i)
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - no deadlock or panic
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent operations timed out - possible deadlock")
	}
}

// TestStateManager_RemoveInvalidID tests removing an invalid listener ID (no-op).
func TestStateManager_RemoveInvalidID(t *testing.T) {
	sm := &StateManager{
		listeners:  make(map[int]builtin.StateListener),
		nextListID: 1,
	}

	// Should not panic
	sm.RemoveListener(999)
	sm.RemoveListener(0)
	sm.RemoveListener(-1)
}

// TestStateManager_SetStateTriggerNotification tests that SetState triggers notifications.
func TestStateManager_SetStateTriggerNotification(t *testing.T) {
	// Create a minimal StateManager with memory backend
	sm, err := initializeStateManager("test-session", "memory")
	if err != nil {
		t.Fatalf("failed to create state manager: %v", err)
	}

	var notifiedKey string
	var wg sync.WaitGroup
	wg.Add(1)

	sm.AddListener(func(key string) {
		notifiedKey = key
		wg.Done()
	})

	// Set state should trigger notification
	sm.SetState("test-key", "test-value")

	// Wait for listener
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("SetState did not trigger listener notification")
	}

	if notifiedKey != "test-key" {
		t.Errorf("expected notifiedKey 'test-key', got %q", notifiedKey)
	}
}
