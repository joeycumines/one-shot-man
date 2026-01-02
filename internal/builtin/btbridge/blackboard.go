// Package btbridge provides integration between bt.js (JavaScript behavior trees)
// and go-behaviortree using goja runtime.
//
// This package implements the Go-Centric architecture (Variant C.2 - Event-Driven Bridge)
// where go-behaviortree is the canonical BT engine, and JavaScript is used for
// leaf behaviors via an async bridge on goja_nodejs/eventloop.
package btbridge

import (
	"sync"

	"github.com/dop251/goja"
)

// Blackboard provides a thread-safe key-value store for behavior tree state.
// It implements a shared state pattern where Go manages the state and exposes
// it to JavaScript via accessor methods.
type Blackboard struct {
	mu   sync.RWMutex
	data map[string]any
}

// NewBlackboard creates a new empty Blackboard.
func NewBlackboard() *Blackboard {
	return &Blackboard{
		data: make(map[string]any),
	}
}

// Get retrieves a value from the blackboard.
// Returns nil if the key doesn't exist.
func (b *Blackboard) Get(key string) any {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.data[key]
}

// Set stores a value in the blackboard.
func (b *Blackboard) Set(key string, value any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[key] = value
}

// Has returns true if the key exists in the blackboard.
func (b *Blackboard) Has(key string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.data[key]
	return ok
}

// Delete removes a key from the blackboard.
func (b *Blackboard) Delete(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.data, key)
}

// Keys returns all keys in the blackboard.
func (b *Blackboard) Keys() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	keys := make([]string, 0, len(b.data))
	for k := range b.data {
		keys = append(keys, k)
	}
	return keys
}

// Clear removes all entries from the blackboard.
func (b *Blackboard) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = make(map[string]any)
}

// Snapshot returns a shallow copy of the blackboard data.
// This is useful for debugging or serialization.
func (b *Blackboard) Snapshot() map[string]any {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make(map[string]any, len(b.data))
	for k, v := range b.data {
		result[k] = v
	}
	return result
}

// ExposeToJS creates a JavaScript object with accessor methods for this blackboard.
// The returned object can be set on the VM as a global or passed to JS functions.
// Methods are bound so they can be called directly from JavaScript:
//
//	blackboard.get("key")
//	blackboard.set("key", value)
//	blackboard.has("key")
//	blackboard.delete("key")
//	blackboard.keys()
//	blackboard.clear()
func (b *Blackboard) ExposeToJS(vm *goja.Runtime) goja.Value {
	obj := vm.NewObject()
	// Note: Set() cannot fail for these keys as they are valid JavaScript identifiers.
	// Ignoring the errors is intentional and safe.
	_ = obj.Set("get", b.Get)
	_ = obj.Set("set", b.Set)
	_ = obj.Set("has", b.Has)
	_ = obj.Set("delete", b.Delete)
	_ = obj.Set("keys", b.Keys)
	_ = obj.Set("clear", b.Clear)
	return obj
}
