package scripting

import (
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

// normalizeSymbolDescription converts a goja.Symbol string representation into the
// persistent key format used by the registry.
func normalizeSymbolDescription(symbolDesc string) string {
	if symbolDesc == "" {
		return ""
	}

	const prefix, suffix = "Symbol(", ")"
	if strings.HasPrefix(symbolDesc, prefix) && strings.HasSuffix(symbolDesc, suffix) {
		symbolDesc = symbolDesc[len(prefix) : len(symbolDesc)-len(suffix)]
	}

	if unquoted, err := strconv.Unquote(symbolDesc); err == nil {
		symbolDesc = unquoted
	}

	return symbolDesc
}

// jsCreateState implements tui.createState(commandName, definitions)
// This is the NEW unified API that replaces createStateContract and createSharedStateContract.
func (e *Engine) jsCreateState(call goja.FunctionCall) goja.Value {
	runtime := e.vm

	// Get command name from first argument
	commandName := call.Argument(0).String()
	if commandName == "" {
		panic(runtime.NewTypeError("createState first argument must be a command name"))
	}

	jsDefs := call.Argument(1).ToObject(runtime)
	if jsDefs == nil {
		panic(runtime.NewTypeError("createState second argument must be a definitions object"))
	}

	// Get stateManager from TUIManager
	if e.tuiManager == nil || e.tuiManager.stateManager == nil {
		panic(runtime.NewTypeError("createState called but state manager is not initialized"))
	}
	stateManager := e.tuiManager.stateManager

	// persistentKeyMap maps the runtime Symbol string representation to its persistent string key.
	persistentKeyMap := make(map[string]string)
	// defaultValues maps the runtime Symbol string representation to its default value.
	defaultValues := make(map[string]interface{})

	// 1. Get Symbol keys from the definitions object and access their values
	// Use JavaScript to handle Symbol-keyed properties
	processDefsScript := `(function(defs) {
		const symbols = Object.getOwnPropertySymbols(defs);
		const result = [];
		for (const sym of symbols) {
			result.push({
				symbol: sym,
				definition: defs[sym]
			});
		}
		return result;
	})`

	processDefsFn, err := runtime.RunString(processDefsScript)
	if err != nil {
		panic(runtime.NewTypeError("failed to create processDefs function: " + err.Error()))
	}

	callable, ok := goja.AssertFunction(processDefsFn)
	if !ok {
		panic(runtime.NewTypeError("processDefs did not return a function"))
	}

	result, err := callable(goja.Undefined(), jsDefs)
	if err != nil {
		panic(runtime.NewTypeError("failed to call processDefs: " + err.Error()))
	}

	resultArray := result.ToObject(runtime)
	if resultArray == nil {
		panic(runtime.NewTypeError("processDefs did not return an array"))
	}

	// 2. Iterate over the results
	length := int(resultArray.Get("length").ToInteger())
	for i := 0; i < length; i++ {
		entryVal := resultArray.Get(strconv.Itoa(i))
		if entryVal == nil || goja.IsUndefined(entryVal) || goja.IsNull(entryVal) {
			continue
		}

		entryObj := entryVal.ToObject(runtime)
		if entryObj == nil {
			continue
		}

		symbolKeyValue := entryObj.Get("symbol")
		defVal := entryObj.Get("definition")

		if symbolKeyValue == nil || goja.IsUndefined(symbolKeyValue) || goja.IsNull(symbolKeyValue) {
			continue
		}

		if defVal == nil || goja.IsUndefined(defVal) || goja.IsNull(defVal) {
			continue
		}

		defObj := defVal.ToObject(runtime)
		if defObj == nil {
			continue
		}

		defaultValue := defObj.Get("defaultValue")
		if defaultValue == nil {
			defaultValue = goja.Undefined()
		}
		defaultValueExport := defaultValue.Export()

		var persistentKey string

		// 3. Check if this is a SHARED symbol (from osm:sharedStateSymbols)
		if sharedKey, ok := stateManager.IsSharedSymbol(symbolKeyValue); ok {
			// Shared symbols are stored directly by their canonical name
			persistentKey = sharedKey // e.g., "contextItems"
		} else {
			// 4. COMMAND-SPECIFIC state: namespace with command name
			symbolDesc := normalizeSymbolDescription(symbolKeyValue.String())
			if symbolDesc == "" {
				panic(runtime.NewTypeError("createState symbol must have a description"))
			}
			// Store as "commandName:symbolDescription"
			persistentKey = commandName + ":" + symbolDesc
		}

		persistentKeyMap[symbolKeyValue.String()] = persistentKey
		defaultValues[symbolKeyValue.String()] = defaultValueExport

		// 5. Initialize state with default value if not already set
		if _, ok := stateManager.GetState(persistentKey); !ok {
			stateManager.SetState(persistentKey, defaultValueExport)
		}
	}

	// 5. Create and return the state accessor object
	stateAccessor := runtime.NewObject()

	// --- Implement state.get(symbol) ---
	_ = stateAccessor.Set("get", func(fc goja.FunctionCall) goja.Value {
		symbolKey := fc.Argument(0)
		keyStr := symbolKey.String()

		persistentKey, ok := persistentKeyMap[keyStr]
		if !ok {
			// This is a programmatic error (asking for an unregistered key)
			panic(runtime.NewTypeError("state.get() called with unregistered Symbol"))
		}

		val, ok := stateManager.GetState(persistentKey)
		if !ok {
			// If state is missing (e.g. after reset), return the default value
			if defVal, hasDef := defaultValues[keyStr]; hasDef {
				return runtime.ToValue(defVal)
			}
			return goja.Undefined()
		}
		return runtime.ToValue(val)
	})

	// --- Implement state.set(symbol, value) ---
	_ = stateAccessor.Set("set", func(fc goja.FunctionCall) goja.Value {
		symbolKey := fc.Argument(0)
		value := fc.Argument(1).Export()
		keyStr := symbolKey.String()

		persistentKey, ok := persistentKeyMap[keyStr]
		if !ok {
			panic(runtime.NewTypeError("state.set() called with unregistered Symbol"))
		}

		stateManager.SetState(persistentKey, value)
		return goja.Undefined()
	})

	return stateAccessor
}
