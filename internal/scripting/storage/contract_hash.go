package storage

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gowebpki/jcs"
)

// ComputeContractSchemaHash generates a deterministic SHA256 hash of a contract definition.
// The hash is computed from the alphabetically sorted list of the contract's persistent
// key names (Symbol descriptions) and their associated schemas (if provided).
// Default values are excluded from the hash to allow for non-breaking changes.
func ComputeContractSchemaHash(contract ContractDefinition) (string, error) {
	// Extract and sort keys
	keys := make([]string, 0, len(contract.Keys))
	for key := range contract.Keys {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Build canonical representation
	type canonicalEntry struct {
		Key    string `json:"key"`
		Schema any    `json:"schema,omitempty"`
	}

	canonical := make([]canonicalEntry, len(keys))
	for i, key := range keys {
		entry := canonicalEntry{Key: key}
		// Include schema if present
		if contract.Schemas != nil {
			if schema, ok := contract.Schemas[key]; ok {
				entry.Schema = schema
			}
		}
		canonical[i] = entry
	}

	// Serialize to JSON first
	tempData, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("failed to marshal contract for hashing: %w", err)
	}

	// Canonicalize to ensure deterministic output
	data, err := jcs.Transform(tempData)
	if err != nil {
		return "", fmt.Errorf("failed to canonicalize contract for hashing: %w", err)
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}
