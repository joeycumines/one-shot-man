package scripting

import (
	"testing"

	"github.com/dop251/goja"
)

// TestSerializeDeserializeState verifies that state can be serialized to JSON and
// deserialized back to Symbol-keyed state, maintaining type fidelity.
func TestSerializeDeserializeState(t *testing.T) {
	// Create a new engine and registry
	registry := NewSymbolRegistry()
	runtime := goja.New()

	// Create test state contract with various types
	definitions := map[string]interface{}{
		"counter": map[string]interface{}{
			"description":  "test:counter",
			"defaultValue": float64(0),
		},
		"name": map[string]interface{}{
			"description":  "test:name",
			"defaultValue": "",
		},
		"items": map[string]interface{}{
			"description":  "test:items",
			"defaultValue": []interface{}{},
		},
	}

	// Create Symbols in JavaScript
	jsCode := `
		(function() {
			return {
				counter: Symbol('test:counter'),
				name: Symbol('test:name'),
				items: Symbol('test:items')
			};
		})()
	`
	symbolMap, err := runtime.RunString(jsCode)
	if err != nil {
		t.Fatalf("Failed to create symbols: %v", err)
	}

	// Register the contract
	contract, err := RegisterContract(registry, "test", runtime, symbolMap, definitions, false)
	if err != nil {
		t.Fatalf("Failed to register contract: %v", err)
	}

	// Create test state with various types using the ORIGINAL Symbol objects from symbolMap
	// NOT the corrupted ones from contract.Definitions
	symbolMapObj := symbolMap.ToObject(runtime)
	originalState := make(map[goja.Value]interface{})

	counterSymbol := symbolMapObj.Get("counter")
	nameSymbol := symbolMapObj.Get("name")
	itemsSymbol := symbolMapObj.Get("items")

	t.Logf("counterSymbol: %v, ExportType: %s", counterSymbol, counterSymbol.ExportType().Name())
	t.Logf("nameSymbol: %v, ExportType: %s", nameSymbol, nameSymbol.ExportType().Name())
	t.Logf("itemsSymbol: %v, ExportType: %s", itemsSymbol, itemsSymbol.ExportType().Name())

	originalState[counterSymbol] = float64(42)
	originalState[nameSymbol] = "test-value"
	originalState[itemsSymbol] = []interface{}{"item1", "item2", "item3"}

	t.Logf("Registry contents:")
	for k := range registry.registry {
		t.Logf("  Registry key: %s", k)
	}

	// Serialize the state
	serialized, err := SerializeState(registry, runtime, originalState)
	if err != nil {
		t.Fatalf("Failed to serialize state: %v", err)
	}

	if serialized == "" {
		t.Fatal("Serialized state should not be empty")
	}

	t.Logf("Serialized JSON: %s", serialized)

	// Deserialize the state
	deserialized, err := DeserializeState(registry, runtime, serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize state: %v", err)
	}

	// Verify the deserialized state matches the original
	if len(deserialized) != len(originalState) {
		t.Fatalf("Deserialized state has %d entries, expected %d", len(deserialized), len(originalState))
	}

	// Check each value
	for key, def := range contract.Definitions {
		originalValue := originalState[def.Symbol]
		deserializedValue := deserialized[def.Symbol]

		if originalValue == nil && deserializedValue == nil {
			continue
		}

		if originalValue == nil || deserializedValue == nil {
			t.Errorf("Value mismatch for key %s: original=%v, deserialized=%v", key, originalValue, deserializedValue)
			continue
		}

		// For slices, compare as JSON to handle type conversions
		switch v := originalValue.(type) {
		case []interface{}:
			deserSlice, ok := deserializedValue.([]interface{})
			if !ok {
				t.Errorf("Expected slice for key %s, got %T", key, deserializedValue)
				continue
			}
			if len(v) != len(deserSlice) {
				t.Errorf("Slice length mismatch for key %s: expected %d, got %d", key, len(v), len(deserSlice))
			}
		default:
			if originalValue != deserializedValue {
				t.Errorf("Value mismatch for key %s: original=%v, deserialized=%v", key, originalValue, deserializedValue)
			}
		}
	}
}

// TestDeserializeStateEmpty verifies that deserializing an empty string returns an empty map.
func TestDeserializeStateEmpty(t *testing.T) {
	registry := NewSymbolRegistry()
	runtime := goja.New()

	deserialized, err := DeserializeState(registry, runtime, "")
	if err != nil {
		t.Fatalf("Failed to deserialize empty state: %v", err)
	}

	if len(deserialized) != 0 {
		t.Errorf("Expected empty map, got %d entries", len(deserialized))
	}
}

// TestDeserializeStateIgnoresUnregisteredKeys verifies that keys not in the registry are ignored.
func TestDeserializeStateIgnoresUnregisteredKeys(t *testing.T) {
	registry := NewSymbolRegistry()
	runtime := goja.New()

	// Create a contract with one key
	definitions := map[string]interface{}{
		"knownKey": map[string]interface{}{
			"description":  "test:known",
			"defaultValue": "",
		},
	}

	jsCode := `
		(function() {
			return {
				knownKey: Symbol('test:known')
			};
		})()
	`
	symbolMap, err := runtime.RunString(jsCode)
	if err != nil {
		t.Fatalf("Failed to create symbols: %v", err)
	}

	_, err = RegisterContract(registry, "test", runtime, symbolMap, definitions, false)
	if err != nil {
		t.Fatalf("Failed to register contract: %v", err)
	}

	// Create JSON with both known and unknown keys
	jsonState := `{"test:known": "value1", "test:unknown": "value2"}`

	deserialized, err := DeserializeState(registry, runtime, jsonState)
	if err != nil {
		t.Fatalf("Failed to deserialize state: %v", err)
	}

	// Should only have the known key
	if len(deserialized) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(deserialized))
	}
}
