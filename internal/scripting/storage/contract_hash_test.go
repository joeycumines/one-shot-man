package storage

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestComputeContractSchemaHash(t *testing.T) {
	testCases := []struct {
		name      string
		contract  ContractDefinition
		expectErr bool
		// Used for comparing two different contracts that should yield the same hash
		compareContract *ContractDefinition
	}{
		{
			name:     "empty contract",
			contract: ContractDefinition{},
		},
		{
			name: "keys only",
			contract: ContractDefinition{
				Keys: map[string]any{"key1": "default1", "key2": 123},
			},
		},
		{
			name: "keys and schemas",
			contract: ContractDefinition{
				Keys:    map[string]any{"key1": "default1", "key2": 123},
				Schemas: map[string]any{"key1": "string", "key2": "number"},
			},
		},
		{
			name: "key order does not affect hash",
			contract: ContractDefinition{
				Keys:    map[string]any{"b": 2, "a": 1},
				Schemas: map[string]any{"a": "number", "b": "number"},
			},
			compareContract: &ContractDefinition{
				Keys:    map[string]any{"a": 1, "b": 2},
				Schemas: map[string]any{"b": "number", "a": "number"},
			},
		},
		{
			name: "default values do not affect hash",
			contract: ContractDefinition{
				Keys:    map[string]any{"key": "default_value_1"},
				Schemas: map[string]any{"key": "string"},
			},
			compareContract: &ContractDefinition{
				Keys:    map[string]any{"key": "default_value_2"},
				Schemas: map[string]any{"key": "string"},
			},
		},
		{
			name: "schema presence affects hash",
			contract: ContractDefinition{
				Keys:    map[string]any{"key": "v"},
				Schemas: map[string]any{"key": "string"},
			},
			// This contract differs only by the absence of a schema, should produce a different hash
			compareContract: &ContractDefinition{
				Keys: map[string]any{"key": "v"},
			},
		},
		{
			name:      "unmarshallable schema returns error",
			contract:  ContractDefinition{Keys: map[string]any{"key": 1}, Schemas: map[string]any{"key": make(chan int)}},
			expectErr: true,
		},
		{
			name: "schema with map is deterministic",
			contract: ContractDefinition{
				Keys: map[string]any{"config": nil},
				Schemas: map[string]any{
					"config": map[string]any{
						"b_prop": "string",
						"a_prop": "number",
						"c_prop": true,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			hash1, err := ComputeContractSchemaHash(tc.contract)

			// Assert
			if tc.expectErr {
				if err == nil {
					t.Fatal("expected an error but got none")
				}
				// Check for specific JSON error if possible
				var unsupportedErr *json.UnsupportedTypeError
				if !errors.As(err, &unsupportedErr) && tc.name == "unmarshallable schema returns error" {
					t.Errorf("expected json.UnsupportedTypeError, got %T", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("ComputeContractSchemaHash failed: %v", err)
			}
			if len(hash1) != 64 { // SHA256 is 32 bytes, 64 hex characters
				t.Errorf("expected hash length 64, got %d", len(hash1))
			}

			// For the determinism test, run multiple times to verify stability
			if tc.name == "schema with map is deterministic" {
				for i := 0; i < 10; i++ {
					hash2, err := ComputeContractSchemaHash(tc.contract)
					if err != nil {
						t.Fatalf("Hash computation %d failed: %v", i+2, err)
					}
					if hash1 != hash2 {
						t.Errorf("Hash was not deterministic on iteration %d. Got %q and %q", i+2, hash1, hash2)
					}
				}
			}

			if tc.compareContract != nil {
				hash2, err := ComputeContractSchemaHash(*tc.compareContract)
				if err != nil {
					t.Fatalf("ComputeContractSchemaHash for comparison failed: %v", err)
				}

				// The logic for this test is inverted for the "schema presence affects hash" case
				if tc.name == "schema presence affects hash" {
					if hash1 == hash2 {
						t.Error("expected hashes to be different, but they were identical")
					}
				} else {
					if hash1 != hash2 {
						t.Errorf("expected hashes to be identical, but they were different: %q vs %q", hash1, hash2)
					}
				}
			}
		})
	}
}
