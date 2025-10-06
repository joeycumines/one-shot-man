package scripting

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/dop251/goja"
)

// jsCreateStateContract registers a JavaScript function that enables the
// declarative definition of state keys using Symbols.
// This function performs two tasks:
// 1. Creates a Map of { keyName: Symbol } for runtime usage in JS.
// 2. Registers the Symbol <-> String mapping with the Go-side registry.
//
// JS signature: createStateContract(modeName: string, definitions: object): object (Symbol-keyed map)
// OR (legacy): createStateContract(definitions: object): object (Symbol-keyed map) - extracts mode name from first description
func (e *Engine) jsCreateStateContract(call goja.FunctionCall) goja.Value {
	runtime := e.vm
	if len(call.Arguments) == 0 {
		panic(runtime.NewTypeError("createStateContract requires at least a definitions object"))
	}

	var modeName string
	var definitionsMap map[string]interface{}

	// Check if first argument is a string (new signature with explicit mode name)
	if len(call.Arguments) >= 2 {
		firstArg := call.Argument(0)
		if firstArg.ExportType().Kind() == reflect.String {
			modeName = firstArg.String()
			// Second argument is the definitions
			jsDefinitions := call.Argument(1).Export()
			var ok bool
			definitionsMap, ok = jsDefinitions.(map[string]interface{})
			if !ok {
				panic(runtime.NewTypeError("createStateContract second argument must be a definitions object"))
			}
		}
	}

	// Legacy mode: single argument (definitions object), extract mode name from description
	if modeName == "" {
		arg := call.Argument(0)
		jsDefinitions := arg.Export()
		var ok bool
		definitionsMap, ok = jsDefinitions.(map[string]interface{})
		if !ok {
			panic(runtime.NewTypeError("createStateContract argument must be an object"))
		}

		// Extract mode name from first definition's description
		for _, defRaw := range definitionsMap {
			if defMap, ok := defRaw.(map[string]interface{}); ok {
				if desc, ok := defMap["description"].(string); ok {
					parts := strings.SplitN(desc, ":", 2)
					if len(parts) > 0 {
						modeName = parts[0]
						break
					}
				}
			}
		}

		if modeName == "" {
			panic(runtime.NewTypeError("could not determine mode name from state contract definitions"))
		}
	}

	// Create a new object to hold the Symbol keys
	symbolsObj := runtime.NewObject()

	// Iterate over each definition and create a Symbol
	for keyName, defValue := range definitionsMap {
		defMap, ok := defValue.(map[string]interface{})
		if !ok {
			panic(runtime.NewTypeError(fmt.Sprintf("definition for '%s' must be an object", keyName)))
		}

		desc, ok := defMap["description"].(string)
		if !ok || desc == "" {
			panic(runtime.NewTypeError(fmt.Sprintf("definition for '%s' missing 'description' string", keyName)))
		}

		// Create the Symbol in JavaScript
		// We need to execute Symbol(description) in the JS context
		symbolValue, err := runtime.RunString(fmt.Sprintf("Symbol(%q)", desc))
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("failed to create Symbol for '%s': %w", keyName, err)))
		}

		// Store the Symbol in the return object
		if err := symbolsObj.Set(keyName, symbolValue); err != nil {
			panic(runtime.NewGoError(fmt.Errorf("failed to set Symbol for '%s': %w", keyName, err)))
		}
	}

	// Now register the contract with the TUIManager (not shared)
	// We pass both the symbols object and the original definitions for metadata
	if err := e.tuiManager.jsCreateStateContractInternal(modeName, symbolsObj, definitionsMap, false); err != nil {
		panic(runtime.NewGoError(err))
	}

	// Return the Symbol-keyed object for JS consumption
	return symbolsObj
}

// jsCreateSharedStateContract creates a shared state contract accessible across all modes.
// JS signature: createSharedStateContract(modeName: string, definitions: object): object (Symbol-keyed map)
// OR (legacy): createSharedStateContract(definitions: object): object (Symbol-keyed map) - extracts mode name from first description
func (e *Engine) jsCreateSharedStateContract(call goja.FunctionCall) goja.Value {
	runtime := e.vm
	if len(call.Arguments) == 0 {
		panic(runtime.NewTypeError("createSharedStateContract requires at least a definitions object"))
	}

	var modeName string
	var definitionsMap map[string]interface{}

	// Check if first argument is a string (new signature with explicit mode name)
	if len(call.Arguments) >= 2 {
		firstArg := call.Argument(0)
		if firstArg.ExportType().Kind() == reflect.String {
			modeName = firstArg.String()
			// Second argument is the definitions
			jsDefinitions := call.Argument(1).Export()
			var ok bool
			definitionsMap, ok = jsDefinitions.(map[string]interface{})
			if !ok {
				panic(runtime.NewTypeError("createSharedStateContract second argument must be a definitions object"))
			}
		}
	}

	// Legacy mode: single argument (definitions object), extract mode name from description
	if modeName == "" {
		arg := call.Argument(0)
		jsDefinitions := arg.Export()
		var ok bool
		definitionsMap, ok = jsDefinitions.(map[string]interface{})
		if !ok {
			panic(runtime.NewTypeError("createSharedStateContract argument must be an object"))
		}

		// Extract mode name from first definition's description
		for _, defRaw := range definitionsMap {
			if defMap, ok := defRaw.(map[string]interface{}); ok {
				if desc, ok := defMap["description"].(string); ok {
					parts := strings.SplitN(desc, ":", 2)
					if len(parts) > 0 {
						modeName = parts[0]
						break
					}
				}
			}
		}

		if modeName == "" {
			panic(runtime.NewTypeError("could not determine mode name from state contract definitions"))
		}
	}

	// Create a new object to hold the Symbol keys
	symbolsObj := runtime.NewObject()

	// Iterate over each definition and create a Symbol
	for keyName, defValue := range definitionsMap {
		defMap, ok := defValue.(map[string]interface{})
		if !ok {
			panic(runtime.NewTypeError(fmt.Sprintf("definition for '%s' must be an object", keyName)))
		}

		desc, ok := defMap["description"].(string)
		if !ok || desc == "" {
			panic(runtime.NewTypeError(fmt.Sprintf("definition for '%s' missing 'description' string", keyName)))
		}

		// Create the Symbol in JavaScript
		symbolValue, err := runtime.RunString(fmt.Sprintf("Symbol(%q)", desc))
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("failed to create Symbol for '%s': %w", keyName, err)))
		}

		// Store the Symbol in the return object
		if err := symbolsObj.Set(keyName, symbolValue); err != nil {
			panic(runtime.NewGoError(fmt.Errorf("failed to set Symbol for '%s': %w", keyName, err)))
		}
	}

	// Register the contract as shared
	if err := e.tuiManager.jsCreateStateContractInternal(modeName, symbolsObj, definitionsMap, true); err != nil {
		panic(runtime.NewGoError(err))
	}

	// Return the Symbol-keyed object for JS consumption
	return symbolsObj
}
