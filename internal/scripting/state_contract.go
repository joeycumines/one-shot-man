package scripting

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/dop251/goja"
)

// StateContract holds the immutable definition for a mode's state keys.
// The in-memory state is keyed by *goja.Symbol.
// The serialized state is keyed by the persistent string.
type StateContract struct {
	// Name is the name of the mode or script the contract belongs to.
	Name string
	// Definitions maps the Symbol description (the persistent string key) to its Symbol and metadata.
	// Map: PersistentStringKey -> Definition
	Definitions map[string]Definition
	// IsShared indicates if this contract defines shared state keys
	IsShared bool
}

// Definition holds the metadata for a single state key.
type Definition struct {
	// Symbol is the unique in-memory key created by JavaScript.
	Symbol goja.Value // Will hold a *goja.Symbol
	// DefaultValue is the value used to initialize state if absent.
	DefaultValue interface{}
	// Schema is an optional type hint for validation (ignored for now, kept for contract).
	Schema interface{}
}

// SymbolRegistry manages the mapping of persistent string keys to their metadata.
// This is the core mechanism that makes Symbol-keyed state persistable.
// Each Engine instance has its own SymbolRegistry to prevent global state issues.
type SymbolRegistry struct {
	sync.RWMutex
	// registry maps the unique Symbol Description (the persistent string key) to the Definition.
	registry map[string]Definition
}

// NewSymbolRegistry creates a new symbol registry instance.
func NewSymbolRegistry() *SymbolRegistry {
	return &SymbolRegistry{
		registry: make(map[string]Definition),
	}
}

// RegisterContract registers all definitions from both the Symbol map and the definitions map.
// jsSymbolMap is the JS object with { KEY_NAME: Symbol(...) } structure
// jsDefinitions is the original definitions map with { KEY_NAME: { description, defaultValue, schema } }
// isShared indicates if this contract defines shared state (accessible across all modes)
func RegisterContract(registry *SymbolRegistry, modeName string, runtime *goja.Runtime, jsSymbolMap goja.Value, jsDefinitions map[string]interface{}, isShared bool) (*StateContract, error) {
	registry.Lock()
	defer registry.Unlock()

	contract := &StateContract{
		Name:        modeName,
		Definitions: make(map[string]Definition),
		IsShared:    isShared,
	}

	// The jsSymbolMap is an object with properties like { COUNTER: Symbol('demo:counter'), ... }
	// We need to iterate over its properties and extract the Symbol VALUES (not descriptions).
	// CRITICAL: We must use a JS-side iteration to preserve Symbol objects, as obj.Get(key)
	// converts Symbols to their string descriptions.
	if obj := jsSymbolMap.ToObject(runtime); obj != nil {
		// Use JavaScript to iterate and extract Symbol values properly
		extractScript := `(function(obj, keys) {
			var symbols = [];
			for (var i = 0; i < keys.length; i++) {
				symbols.push(obj[keys[i]]);
			}
			return symbols;
		})`

		extractFunc, err := runtime.RunString(extractScript)
		if err != nil {
			return nil, fmt.Errorf("failed to create Symbol extraction function: %w", err)
		}

		callable, ok := goja.AssertFunction(extractFunc)
		if !ok {
			return nil, fmt.Errorf("symbol extraction script did not return a function")
		}

		keys := obj.Keys()
		keysValue := runtime.ToValue(keys)

		result, err := callable(goja.Undefined(), obj, keysValue)
		if err != nil {
			return nil, fmt.Errorf("failed to extract Symbol values: %w", err)
		}

		// Access the array elements as goja.Value objects, NOT through Export()
		symbolsArrayObj := result.ToObject(runtime)

		for i, key := range keys {
			// Get the Symbol at index i from the array
			symbolVal := symbolsArrayObj.Get(fmt.Sprintf("%d", i))

			// Extract and normalize the Symbol's description string
			symbolDesc := normalizeSymbolDescription(symbolVal.String())
			if symbolDesc == "" {
				continue
			}

			// Check for collision on the persistent key (description string)
			if _, exists := registry.registry[symbolDesc]; exists {
				// This is a FATAL, non-recoverable error: two contracts attempted to claim the same serialization key.
				return nil, fmt.Errorf("state contract key collision: persistent key '%s' already registered", symbolDesc)
			}

			// Get the corresponding definition from jsDefinitions
			var defaultValue interface{}
			var schema interface{}
			if defRaw, ok := jsDefinitions[key]; ok {
				if defMap, ok := defRaw.(map[string]interface{}); ok {
					defaultValue = defMap["defaultValue"]
					schema = defMap["schema"]
				}
			}

			def := Definition{
				Symbol:       symbolVal,
				DefaultValue: defaultValue,
				Schema:       schema,
			}

			registry.registry[symbolDesc] = def
			contract.Definitions[symbolDesc] = def
		}
	}

	return contract, nil
}

// SerializeState converts the in-memory Symbol-keyed state into a JSON string.
func SerializeState(registry *SymbolRegistry, runtime *goja.Runtime, symbolKeyedState map[goja.Value]interface{}) (string, error) {
	registry.RLock()
	defer registry.RUnlock()

	// Map: PersistentStringKey -> Value
	stringKeyedState := make(map[string]interface{})

	// Iterate over the in-memory Symbol keys
	for symbolKey, value := range symbolKeyedState {
		// Keys can be either Symbol objects (from JS code) or strings (from Go test code that
		// experienced Symbol-to-string conversion). We need to handle both cases.
		exportType := symbolKey.ExportType().Name()

		var symbolDesc string
		if exportType == "Symbol" {
			// Actual Symbol object from JavaScript - extract its description
			symbolDesc = normalizeSymbolDescription(symbolKey.String())
		} else if exportType == "string" {
			// String key (likely a symbol description that was converted by goja)
			// This happens when Go code tries to use Symbols but they get converted
			symbolDesc = normalizeSymbolDescription(symbolKey.String())
		} else {
			// Neither Symbol nor string - skip
			continue
		}

		if symbolDesc == "" {
			continue
		}

		// Lookup the Symbol Definition to ensure it's a registered state key
		if _, exists := registry.registry[symbolDesc]; !exists {
			// This key is not part of a formal contract. Ignore it as requested.
			continue
		}

		// The persistent string key is the symbol's description.
		stringKeyedState[symbolDesc] = value
	}

	// JSON-encode the string-keyed map
	data, err := json.Marshal(stringKeyedState)
	if err != nil {
		return "", fmt.Errorf("failed to marshal state: %w", err)
	}

	return string(data), nil
}

// DeserializeState converts a JSON string into a Symbol-keyed state map.
func DeserializeState(registry *SymbolRegistry, runtime *goja.Runtime, jsonState string) (map[goja.Value]interface{}, error) {
	registry.RLock()
	defer registry.RUnlock()

	if jsonState == "" {
		return make(map[goja.Value]interface{}), nil
	}

	// Map: PersistentStringKey -> Value
	var stringKeyedState map[string]interface{}
	if err := json.Unmarshal([]byte(jsonState), &stringKeyedState); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON state: %w", err)
	}

	// Map: Symbol -> Value
	symbolKeyedState := make(map[goja.Value]interface{})

	for stringKey, value := range stringKeyedState {
		// Look up the registered Symbol object using the persistent string key
		def, ok := registry.registry[stringKey]
		if !ok {
			// Ignore state keys that are no longer in a contract (e.g., deprecated keys).
			continue
		}

		// Use the retrieved goja.Symbol as the in-memory key
		symbolKeyedState[def.Symbol] = value
	}

	return symbolKeyedState, nil
}
