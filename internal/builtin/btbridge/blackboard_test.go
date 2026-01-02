package btbridge

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

func TestBlackboard_BasicOperations(t *testing.T) {
	t.Parallel()

	bb := NewBlackboard()

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

	bb := NewBlackboard()

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

	bb := NewBlackboard()
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

	bb := NewBlackboard()
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

func TestBlackboard_ThreadSafety(t *testing.T) {
	t.Parallel()

	bb := NewBlackboard()
	const goroutines = 10
	const iterations = 100

	done := make(chan struct{})

	// Concurrent writers
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			for i := 0; i < iterations; i++ {
				key := "key"
				bb.Set(key, i*id)
				bb.Get(key)
				bb.Has(key)
			}
			done <- struct{}{}
		}(g)
	}

	// Wait for all goroutines
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Should complete without race conditions
	require.True(t, bb.Has("key"))
}

func TestBlackboard_ExposeToJS(t *testing.T) {
	t.Parallel()

	bb := NewBlackboard()
	bb.Set("initial", "value")

	vm := goja.New()
	jsObj := bb.ExposeToJS(vm)
	require.NoError(t, vm.Set("blackboard", jsObj))

	// Test get from JS
	result, err := vm.RunString(`blackboard.get("initial")`)
	require.NoError(t, err)
	require.Equal(t, "value", result.Export())

	// Test set from JS
	_, err = vm.RunString(`blackboard.set("fromJS", 123)`)
	require.NoError(t, err)
	require.Equal(t, int64(123), bb.Get("fromJS"))

	// Test has from JS
	result, err = vm.RunString(`blackboard.has("initial")`)
	require.NoError(t, err)
	require.True(t, result.ToBoolean())

	result, err = vm.RunString(`blackboard.has("nonexistent")`)
	require.NoError(t, err)
	require.False(t, result.ToBoolean())

	// Test delete from JS
	_, err = vm.RunString(`blackboard.delete("initial")`)
	require.NoError(t, err)
	require.False(t, bb.Has("initial"))

	// Test keys from JS
	bb.Set("key1", 1)
	bb.Set("key2", 2)
	result, err = vm.RunString(`blackboard.keys()`)
	require.NoError(t, err)
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
	require.Len(t, keys, 3) // fromJS, key1, key2

	// Test clear from JS
	_, err = vm.RunString(`blackboard.clear()`)
	require.NoError(t, err)
	require.Empty(t, bb.Keys())
}
