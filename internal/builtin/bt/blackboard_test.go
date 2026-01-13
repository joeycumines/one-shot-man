package bt

import (
	"fmt"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

func TestBlackboard_BasicOperations(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)

	// Test Set and Get
	bb.Set("key1", "value1")
	require.Equal(t, "value1", bb.Get("key1"))

	// Test non-existent key
	require.Nil(t, bb.Get("nonexistent"))

	// Test Has
	require.True(t, bb.Has("key1"))
	require.False(t, bb.Has("nonexistent"))

	// Test Delete
	bb.Delete("key1")
	require.False(t, bb.Has("key1"))
	require.Nil(t, bb.Get("key1"))

	// Test various types
	bb.Set("int", 42)
	bb.Set("float", 3.14)
	bb.Set("bool", true)
	bb.Set("slice", []int{1, 2, 3})
	bb.Set("map", map[string]int{"a": 1})

	require.Equal(t, 42, bb.Get("int"))
	require.Equal(t, 3.14, bb.Get("float"))
	require.Equal(t, true, bb.Get("bool"))
	require.Equal(t, []int{1, 2, 3}, bb.Get("slice"))
	require.Equal(t, map[string]int{"a": 1}, bb.Get("map"))
}

func TestBlackboard_Keys(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)

	// Empty blackboard
	require.Empty(t, bb.Keys())

	// Add keys
	bb.Set("a", 1)
	bb.Set("b", 2)
	bb.Set("c", 3)

	keys := bb.Keys()
	require.Len(t, keys, 3)
	require.ElementsMatch(t, []string{"a", "b", "c"}, keys)
}

func TestBlackboard_Clear(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)
	bb.Set("a", 1)
	bb.Set("b", 2)

	require.Len(t, bb.Keys(), 2)

	bb.Clear()

	require.Len(t, bb.Keys(), 0)
	require.False(t, bb.Has("a"))
	require.False(t, bb.Has("b"))
}

func TestBlackboard_Snapshot(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)
	bb.Set("a", 1)
	bb.Set("b", "two")

	snapshot := bb.Snapshot()

	require.Equal(t, 1, snapshot["a"])
	require.Equal(t, "two", snapshot["b"])
	require.Len(t, snapshot, 2)

	// Verify snapshot is a copy (modifying it doesn't affect original)
	snapshot["c"] = 3
	require.False(t, bb.Has("c"))
}

func TestBlackboard_Len(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)

	// Empty blackboard
	require.Equal(t, 0, bb.Len())

	// Add one key
	bb.Set("key1", "value1")
	require.Equal(t, 1, bb.Len())

	// Add multiple keys
	bb.Set("key2", "value2")
	bb.Set("key3", "value3")
	bb.Set("key4", "value4")
	require.Equal(t, 4, bb.Len())

	// Len matches Keys() length
	keys := bb.Keys()
	require.Equal(t, len(keys), bb.Len())

	// Len updates after delete
	bb.Delete("key2")
	require.Equal(t, 3, bb.Len())

	// Len updates after clear
	bb.Clear()
	require.Equal(t, 0, bb.Len())
}

func TestBlackboard_ThreadSafety(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)
	const numWriters = 10
	const numDeleters = 5
	const numReaders = 8
	const numKeysOps = 5
	const numSnapshotters = 5
	const numClearers = 3
	const iterations = 100

	done := make(chan struct{}, numWriters+numDeleters+numReaders+numKeysOps+numSnapshotters+numClearers)

	// Concurrent writers - set random keys with various value types
	for g := 0; g < numWriters; g++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < iterations; i++ {
				key := fmt.Sprintf("key-%d-%d", id, i)
				switch i % 5 {
				case 0:
					bb.Set(key, i*id)
				case 1:
					bb.Set(key, fmt.Sprintf("value-%d", i*id))
				case 2:
					bb.Set(key, i*id > 0)
				case 3:
					bb.Set(key, []int{i, i * id, i * 2})
				case 4:
					bb.Set(key, map[string]int{"a": i, "b": i * id})
				}
				bb.Get(key)
				bb.Has(key)
			}
		}(g)
	}

	// Concurrent deleters - delete random keys
	for g := 0; g < numDeleters; g++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < iterations; i++ {
				// Try to delete keys that might exist from writers
				key := fmt.Sprintf("key-%d-%d", (id+i)%numWriters, i)
				bb.Delete(key)
				// Also try non-existent keys
				bb.Delete(fmt.Sprintf("nonexistent-%d", i))
			}
		}(g)
	}

	// Concurrent readers - get/has on various keys (read operations)
	for g := 0; g < numReaders; g++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < iterations; i++ {
				key := fmt.Sprintf("key-%d-%d", (id+i)%numWriters, i)
				bb.Get(key)
				bb.Has(key)
				bb.Get("nonexistent-key")
				bb.Has("nonexistent-key")
			}
		}(g)
	}

	// Concurrent Keys() calls - enumerate keys during writes/deletes
	for g := 0; g < numKeysOps; g++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < iterations; i++ {
				keys := bb.Keys()
				// Verify keys slice can be safely used (no panic)
				_ = len(keys)
				// Read from some keys we got
				for _, k := range keys {
					if len(k) > 0 {
						bb.Get(k)
					}
				}
			}
		}(g)
	}

	// Concurrent Snapshot() calls - take snapshots during concurrent modifications
	for g := 0; g < numSnapshotters; g++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < iterations; i++ {
				snapshot := bb.Snapshot()
				// Verify snapshot is a valid map (no panic, no nil dereference)
				_ = len(snapshot)
				// Read from snapshot - should be safe since snapshot is a copy
				for k, v := range snapshot {
					if k != "" {
						_ = v
					}
				}
			}
		}(g)
	}

	// Concurrent Clear() calls - periodically clear the blackboard
	for g := 0; g < numClearers; g++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			// Call Clear less frequently (every 10 iterations)
			for i := 0; i < iterations; i += 10 {
				bb.Clear()
			}
		}(g)
	}

	// Wait for all goroutines
	for i := 0; i < numWriters+numDeleters+numReaders+numKeysOps+numSnapshotters+numClearers; i++ {
		<-done
	}

	// Should complete without race conditions
	// Final state doesn't matter due to concurrent Clear() calls,
	// just verify we can still operate on the blackboard
	bb.Set("final-check", "test")
	require.Equal(t, "test", bb.Get("final-check"))
	bb.Delete("final-check")
	require.False(t, bb.Has("final-check"))
}

func TestBlackboard_ExposeToJS(t *testing.T) {
	t.Parallel()

	bb := new(Blackboard)
	bb.Set("initial", "value")

	// Create a bridge to safely run JS on the event loop
	bridge := testBridge(t)

	// All goja.Runtime operations must happen inside RunOnLoopSync
	err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		jsObj := bb.ExposeToJS(vm)
		if err := vm.Set("blackboard", jsObj); err != nil {
			return err
		}

		// Test get from JS
		result, err := vm.RunString(`blackboard.get("initial")`)
		if err != nil {
			return err
		}
		if result.Export() != "value" {
			t.Errorf("expected 'value', got %v", result.Export())
		}

		// Test set from JS
		_, err = vm.RunString(`blackboard.set("fromJS", 123)`)
		if err != nil {
			return err
		}
		if bb.Get("fromJS") != int64(123) {
			t.Errorf("expected 123, got %v", bb.Get("fromJS"))
		}

		// Test has from JS
		result, err = vm.RunString(`blackboard.has("initial")`)
		if err != nil {
			return err
		}
		if !result.ToBoolean() {
			t.Error("expected has('initial') to be true")
		}

		result, err = vm.RunString(`blackboard.has("nonexistent")`)
		if err != nil {
			return err
		}
		if result.ToBoolean() {
			t.Error("expected has('nonexistent') to be false")
		}

		// Test delete from JS
		_, err = vm.RunString(`blackboard.delete("initial")`)
		if err != nil {
			return err
		}
		if bb.Has("initial") {
			t.Error("expected 'initial' to be deleted")
		}

		// Test keys from JS
		bb.Set("key1", 1)
		bb.Set("key2", 2)
		result, err = vm.RunString(`blackboard.keys()`)
		if err != nil {
			return err
		}
		keysExport := result.Export()
		var keys []string
		switch v := keysExport.(type) {
		case []string:
			keys = v
		case []any:
			for _, k := range v {
				keys = append(keys, k.(string))
			}
		default:
			t.Fatalf("unexpected type for keys: %T", keysExport)
		}
		if len(keys) != 3 { // fromJS, key1, key2
			t.Errorf("expected 3 keys, got %d", len(keys))
		}

		// Test clear from JS
		_, err = vm.RunString(`blackboard.clear()`)
		if err != nil {
			return err
		}
		if len(bb.Keys()) != 0 {
			t.Error("expected keys to be empty after clear")
		}

		return nil
	})
	require.NoError(t, err)
}
