package builtin

import (
	"encoding/json"
	"testing"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/storage"
)

// mockStateManager records SetSharedSymbols calls for testing.
type mockStateManager struct {
	setSharedSymbolsCalled bool
	symbolToStringLen      int
	stringToSymbolLen      int
}

func (m *mockStateManager) SetSharedSymbols(symbolToString map[goja.Value]string, stringToSymbol map[string]goja.Value) {
	m.setSharedSymbolsCalled = true
	m.symbolToStringLen = len(symbolToString)
	m.stringToSymbolLen = len(stringToSymbol)
}
func (m *mockStateManager) IsSharedSymbol(goja.Value) (string, bool) { return "", false }
func (m *mockStateManager) GetState(string) (any, bool)              { return nil, false }
func (m *mockStateManager) SetState(string, any)                     {}
func (m *mockStateManager) CaptureSnapshot(string, string, json.RawMessage) error {
	return nil
}
func (m *mockStateManager) PersistSession() error                     { return nil }
func (m *mockStateManager) GetSessionHistory() []storage.HistoryEntry { return nil }
func (m *mockStateManager) SerializeCompleteState() (json.RawMessage, error) {
	return nil, nil
}
func (m *mockStateManager) ArchiveAndReset() (string, error) { return "", nil }
func (m *mockStateManager) Close() error                     { return nil }
func (m *mockStateManager) ClearAllState()                   {}
func (m *mockStateManager) AddListener(StateListener) int    { return 0 }
func (m *mockStateManager) RemoveListener(int)               {}

// mockStateManagerProvider returns the mock state manager.
type mockStateManagerProvider struct {
	sm StateManager
}

func (p *mockStateManagerProvider) GetStateManager() StateManager {
	return p.sm
}

func TestGetSharedSymbolsLoader(t *testing.T) {
	t.Parallel()

	sm := &mockStateManager{}
	provider := &mockStateManagerProvider{sm: sm}

	loader := GetSharedSymbolsLoader(provider)
	if loader == nil {
		t.Fatal("expected non-nil loader")
	}

	rt := goja.New()
	module := rt.NewObject()

	// Call the loader (simulates require("osm:sharedStateSymbols")).
	loader(rt, module)

	// Verify exports were set.
	exports := module.Get("exports")
	if exports == nil {
		t.Fatal("expected 'exports' to be set on module")
	}
	exportsObj := exports.ToObject(rt)

	// Verify contextItems key exists and is a Symbol.
	contextItems := exportsObj.Get("contextItems")
	if contextItems == nil || goja.IsUndefined(contextItems) {
		t.Fatal("expected 'contextItems' key in exports")
	}

	// Symbols in goja are represented as *goja.Symbol — verify the value
	// is truthy and of the expected type by checking its string form.
	str := contextItems.String()
	if str == "" || str == "undefined" {
		t.Fatalf("expected Symbol value for contextItems, got: %q", str)
	}

	// Verify SetSharedSymbols was called.
	if !sm.setSharedSymbolsCalled {
		t.Fatal("expected SetSharedSymbols to be called on StateManager")
	}
	if sm.symbolToStringLen != len(sharedStateKeys) {
		t.Errorf("expected symbolToString len=%d, got %d", len(sharedStateKeys), sm.symbolToStringLen)
	}
	if sm.stringToSymbolLen != len(sharedStateKeys) {
		t.Errorf("expected stringToSymbol len=%d, got %d", len(sharedStateKeys), sm.stringToSymbolLen)
	}
}

func TestGetSharedSymbolsLoader_NilStateManager(t *testing.T) {
	t.Parallel()

	// Provider returns nil StateManager — should not panic.
	provider := &mockStateManagerProvider{sm: nil}

	loader := GetSharedSymbolsLoader(provider)
	rt := goja.New()
	module := rt.NewObject()

	// Must not panic.
	loader(rt, module)

	// Exports should still be set.
	exports := module.Get("exports")
	if exports == nil {
		t.Fatal("expected 'exports' to be set even with nil StateManager")
	}
}
