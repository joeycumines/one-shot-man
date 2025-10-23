package scripting

import (
	"github.com/joeycumines/one-shot-man/internal/scripting/storage"
)

// convertToStorageContract converts a StateContract to a storage.ContractDefinition.
// This bridges the in-memory Symbol-based contract with the serializable storage format.
func convertToStorageContract(contract *StateContract) storage.ContractDefinition {
	keys := make(map[string]any)
	schemas := make(map[string]any)

	for persistentKey, def := range contract.Definitions {
		keys[persistentKey] = def.DefaultValue
		if def.Schema != nil {
			schemas[persistentKey] = def.Schema
		}
	}

	return storage.ContractDefinition{
		ModeName: contract.Name,
		IsShared: contract.IsShared,
		Keys:     keys,
		Schemas:  schemas,
	}
}
