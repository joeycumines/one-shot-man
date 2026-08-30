package termmux

import (
	"golang.org/x/crypto/bcrypt"
)

// SessionLock manages the locked state of a session.
type SessionLock struct {
	hashedPassword []byte
	locked         bool
}

// LockSession locks the session with the given plaintext password.
// The password is hashed with bcrypt. Returns an error if hashing fails.
func (l *SessionLock) Lock(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	l.hashedPassword = hash
	l.locked = true
	return nil
}

// Unlock attempts to unlock the session with the given plaintext password.
// Returns true if the password matches, false otherwise.
// A failed attempt does not change the locked state.
func (l *SessionLock) Unlock(password string) bool {
	if !l.locked {
		return true
	}
	if err := bcrypt.CompareHashAndPassword(l.hashedPassword, []byte(password)); err != nil {
		return false
	}
	l.locked = false
	l.hashedPassword = nil
	return true
}

// IsLocked returns whether the session is currently locked.
func (l *SessionLock) IsLocked() bool {
	return l.locked
}
