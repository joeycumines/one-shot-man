package storage

// StorageBackend defines the contract for all persistence mechanisms.
type StorageBackend interface {
	// LoadSession retrieves a session by its unique ID.
	// It MUST return (nil, nil) if the session does not exist.
	LoadSession(sessionID string) (*Session, error)

	// SaveSession atomically persists the entire session state.
	SaveSession(session *Session) error

	// Close performs any necessary cleanup of backend resources, such as releasing file locks.
	Close() error
}
