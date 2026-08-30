package builtin

import (
	"encoding/json"
	"testing"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/storage"
)

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
	loader(rt, module)

	exports := module.Get("exports")
	if exports == nil {
		t.Fatal("expected 'exports' to be set on module")
	}
	exportsObj := exports.ToObject(rt)

	contextItems := exportsObj.Get("contextItems")
	if contextItems == nil || goja.IsUndefined(contextItems) {
		t.Fatal("expected 'contextItems' key in exports")
	}
	if _, ok := contextItems.(*goja.Symbol); !ok {
		t.Fatalf("expected *goja.Symbol for contextItems, got %T", contextItems)
	}

	str := contextItems.String()
	if str == "" || str == "undefined" {
		t.Fatalf("expected non-empty Symbol string, got: %q", str)
	}

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

	loader := GetSharedSymbolsLoader(&mockStateManagerProvider{sm: nil})
	rt := goja.New()
	module := rt.NewObject()

	loader(rt, module)

	exports := module.Get("exports")
	if exports == nil {
		t.Fatal("expected 'exports' to be set even with nil StateManager")
	}
}

func TestGetSharedSymbolsLoader_NilProvider(t *testing.T) {
	t.Parallel()

	loader := GetSharedSymbolsLoader(nil)
	rt := goja.New()
	module := rt.NewObject()

	loader(rt, module)

	exports := module.Get("exports")
	if exports == nil {
		t.Fatal("expected 'exports' to be set even with nil provider")
	}
}
