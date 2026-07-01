package builtin

import (
	"encoding/json"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/storage"
)

// StateListener is invoked when shared state changes.
type StateListener func(key string)

// StateManagerProvider exposes the application's StateManager.
type StateManagerProvider interface {
	GetStateManager() StateManager
}

// StateManager handles shared-state symbols and persistent session state.
type StateManager interface {
	SetSharedSymbols(symbolToString map[goja.Value]string, stringToSymbol map[string]goja.Value)
	IsSharedSymbol(symbol goja.Value) (string, bool)
	GetState(persistentKey string) (any, bool)
	SetState(persistentKey string, value any)
	CaptureSnapshot(modeID, command string, stateJSON json.RawMessage) error
	PersistSession() error
	GetSessionHistory() []storage.HistoryEntry
	SerializeCompleteState() (json.RawMessage, error)
	ArchiveAndReset() (string, error)
	Close() error
	ClearAllState()
	AddListener(fn StateListener) int
	RemoveListener(id int)
}

// sharedStateKeys lists the shared-state symbol names exposed to JavaScript.
var sharedStateKeys = []string{
	"contextItems",
}

// GetSharedSymbolsLoader returns a CommonJS loader for osm:sharedStateSymbols.
func GetSharedSymbolsLoader(stateManagerProvider StateManagerProvider) func(*goja.Runtime, *goja.Object) {
	return func(rt *goja.Runtime, module *goja.Object) {
		symbolToString := make(map[goja.Value]string)
		stringToSymbol := make(map[string]goja.Value)

		exports := rt.NewObject()
		for _, keyName := range sharedStateKeys {
			desc := "osm:shared/" + keyName
			symbolVal := goja.NewSymbol(desc)

			_ = exports.Set(keyName, symbolVal)
			symbolToString[symbolVal] = keyName
			stringToSymbol[keyName] = symbolVal
		}

		if stateManagerProvider != nil {
			if sm := stateManagerProvider.GetStateManager(); sm != nil {
				sm.SetSharedSymbols(symbolToString, stringToSymbol)
			}
		}

		_ = module.Set("exports", exports)
	}
}
