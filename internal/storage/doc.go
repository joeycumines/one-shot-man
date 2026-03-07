// Package storage implements session persistence backends.
// Two backends are provided: a filesystem backend (fs) for
// production use and an in-memory backend (memory) for testing.
// Both implement the StorageBackend interface for reading,
// writing, listing, and deleting session state.
package storage
