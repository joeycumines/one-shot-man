package nextintegerid

import (
	"testing"

	"github.com/dop251/goja"
)

// --- Edge cases for Require function ---

func TestNextID_NullArgument(t *testing.T) {
	t.Parallel()
	_, nextFn := setupModule(t)

	result, err := nextFn(goja.Undefined(), goja.Null())
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if got := result.ToInteger(); got != 1 {
		t.Fatalf("expected 1 for null arg, got %d", got)
	}
}

func TestNextID_UndefinedArgument(t *testing.T) {
	t.Parallel()
	_, nextFn := setupModule(t)

	result, err := nextFn(goja.Undefined(), goja.Undefined())
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if got := result.ToInteger(); got != 1 {
		t.Fatalf("expected 1 for undefined arg, got %d", got)
	}
}

func TestNextID_ObjectWithoutLength(t *testing.T) {
	t.Parallel()
	runtime, nextFn := setupModule(t)

	// Create via JS to ensure it's a proper JS object (not Go struct wrapper)
	val := mustRunValue(t, runtime, `({foo: "bar"})`)

	result, err := nextFn(goja.Undefined(), val)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if got := result.ToInteger(); got != 1 {
		t.Fatalf("expected 1 for object without length, got %d", got)
	}
}

func TestNextID_ArrayWithNilItems(t *testing.T) {
	t.Parallel()
	runtime, nextFn := setupModule(t)

	// Create array with null elements: [null, {id: 5}, null]
	val := mustRunValue(t, runtime, `[null, {id: 5}, null]`)

	result, err := nextFn(goja.Undefined(), val)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if got := result.ToInteger(); got != 6 {
		t.Fatalf("expected 6, got %d", got)
	}
}

func TestNextID_ItemsWithNoIDField(t *testing.T) {
	t.Parallel()
	runtime, nextFn := setupModule(t)

	val := mustRunValue(t, runtime, `[{name: "a"}, {name: "b"}, {id: 3}]`)

	result, err := nextFn(goja.Undefined(), val)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if got := result.ToInteger(); got != 4 {
		t.Fatalf("expected 4, got %d", got)
	}
}

func TestNextID_ItemsWithUndefinedID(t *testing.T) {
	t.Parallel()
	runtime, nextFn := setupModule(t)

	val := mustRunValue(t, runtime, `[{id: undefined}, {id: 10}]`)

	result, err := nextFn(goja.Undefined(), val)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if got := result.ToInteger(); got != 11 {
		t.Fatalf("expected 11, got %d", got)
	}
}

func TestNextID_ItemsWithNullID(t *testing.T) {
	t.Parallel()
	runtime, nextFn := setupModule(t)

	val := mustRunValue(t, runtime, `[{id: null}, {id: 2}]`)

	result, err := nextFn(goja.Undefined(), val)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if got := result.ToInteger(); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestNextID_SingleItem(t *testing.T) {
	t.Parallel()
	runtime, nextFn := setupModule(t)

	val := mustRunValue(t, runtime, `[{id: 1}]`)

	result, err := nextFn(goja.Undefined(), val)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if got := result.ToInteger(); got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
}

func TestNextID_AllZeroIDs(t *testing.T) {
	t.Parallel()
	runtime, nextFn := setupModule(t)

	val := mustRunValue(t, runtime, `[{id: 0}, {id: 0}]`)

	result, err := nextFn(goja.Undefined(), val)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if got := result.ToInteger(); got != 1 {
		t.Fatalf("expected 1 (0 + 1), got %d", got)
	}
}

func TestNextID_NegativeIDs(t *testing.T) {
	t.Parallel()
	runtime, nextFn := setupModule(t)

	val := mustRunValue(t, runtime, `[{id: -5}, {id: -1}]`)

	result, err := nextFn(goja.Undefined(), val)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	// maxVal starts at 0, negative ids < 0, so maxVal stays 0, result = 1
	if got := result.ToInteger(); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

func TestNextID_LargeIDs(t *testing.T) {
	t.Parallel()
	runtime, nextFn := setupModule(t)

	val := mustRunValue(t, runtime, `[{id: 999999}]`)

	result, err := nextFn(goja.Undefined(), val)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if got := result.ToInteger(); got != 1000000 {
		t.Fatalf("expected 1000000, got %d", got)
	}
}
