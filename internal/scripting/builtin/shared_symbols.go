// Package builtin provides osm:sharedStateSymbols module.
package builtin

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/scripting/storage"
)

// StateManagerProvider provides access to a StateManager instance.
type StateManagerProvider interface {
	GetStateManager() StateManager
}

// StateManager is the interface for managing shared symbol registration.
type StateManager interface {
	SetSharedSymbols(symbolToString map[goja.Value]string, stringToSymbol map[string]goja.Value)
	IsSharedSymbol(symbol goja.Value) (string, bool)
	GetState(persistentKey string) (interface{}, bool)
	SetState(persistentKey string, value interface{})
	CaptureSnapshot(modeID, command string, stateJSON string) error
	PersistSession() error
	GetSessionHistory() []storage.HistoryEntry
	SerializeCompleteState() (string, error)
	// ArchiveAndReset archives the current session and resets state. Returns
	// the archive path if successful. Implementations may return an error
	// indicating the destination already exists (useful for retries).
	ArchiveAndReset() (string, error)
	Close() error
	ClearAllState()
}

// sharedStateKeys defines the canonical string keys for all shared state.
var sharedStateKeys = []string{
	"contextItems",
	// Add other future shared keys here.
}

// GetSharedSymbolsLoader returns a loader function compatible with require.RegisterNativeModule.
func GetSharedSymbolsLoader(stateManagerProvider StateManagerProvider) func(*goja.Runtime, *goja.Object) {
	return func(rt *goja.Runtime, module *goja.Object) {
		// These maps are for Go-side identity checks.
		symbolToString := make(map[goja.Value]string)
		stringToSymbol := make(map[string]goja.Value)

		// This object is exported to JS.
		exports := rt.NewObject()

		for _, keyName := range sharedStateKeys {
			desc := fmt.Sprintf("osm:shared/%s", keyName)
			symbolVal, _ := rt.RunString(fmt.Sprintf("Symbol(%q)", desc))

			_ = exports.Set(keyName, symbolVal)
			symbolToString[symbolVal] = keyName
			stringToSymbol[keyName] = symbolVal
		}

		// Register symbols with StateManager for identity checks.
		stateManager := stateManagerProvider.GetStateManager()
		if stateManager != nil {
			stateManager.SetSharedSymbols(symbolToString, stringToSymbol)
		}

		// Export the {contextItems: Symbol(...)} object.
		_ = module.Set("exports", exports)
	}
}
