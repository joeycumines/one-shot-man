// Package gateway implements the credential-custody gateway: it reads the model
// catalog, resolves the credentials the shaper-fronted providers need, starts
// the transcode shaper with those credentials in its environment only, waits
// for the bind port, advertises itself in a discovery file, and removes that
// file and reaps the child on shutdown.
//
// It lives in Go deliberately: the scripting surface withholds process control
// (the runtime deletes Node's process globals and provides no file removal,
// socket or signal primitives), and those capabilities are exactly what custody
// requires.
package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// DiscoveryVersion is the discovery document format this build writes and
// understands. A reader refuses a document it does not know.
const DiscoveryVersion = 1

// Discovery is the advertisement a running gateway leaves for the launcher.
type Discovery struct {
	Version   int               `json:"version"`
	PID       int               `json:"pid"`
	Host      string            `json:"host"`
	Port      int               `json:"port"`
	Prefixes  map[string]string `json:"prefixes"`
	Token     string            `json:"token"`
	StartedAt string            `json:"started_at"`
}

// ErrAlreadyRunning reports that a discovery file belongs to a live process, so
// a second gateway must not take the advertisement over.
var ErrAlreadyRunning = errors.New("a gateway is already advertised by a live process")

// NewDiscovery builds the advertisement for one start. The token is the
// per-start ephemeral credential class: machine-local, short-lived and not a
// provider secret.
func NewDiscovery(pid, port int, host string, prefixes map[string]string, token string, started time.Time) Discovery {
	copied := make(map[string]string, len(prefixes))
	for access, prefix := range prefixes {
		copied[access] = prefix
	}
	return Discovery{
		Version:   DiscoveryVersion,
		PID:       pid,
		Host:      host,
		Port:      port,
		Prefixes:  copied,
		Token:     token,
		StartedAt: started.UTC().Format(time.RFC3339),
	}
}

// WriteDiscovery writes the advertisement, creating the file exclusively and
// with owner-only permissions, so a reader never sees a partial document and a
// stale file cannot be silently overwritten.
func WriteDiscovery(path string, discovery Discovery, alive func(pid int) bool) error {
	if discovery.Version != DiscoveryVersion {
		return fmt.Errorf("discovery version %d is not the supported version %d", discovery.Version, DiscoveryVersion)
	}
	if discovery.Token == "" {
		return errors.New("discovery requires a token")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating the discovery directory: %w", err)
	}
	if err := removeStale(path, alive); err != nil {
		return err
	}

	encoded, err := json.Marshal(discovery)
	if err != nil {
		return fmt.Errorf("encoding the discovery document: %w", err)
	}
	encoded = append(encoded, '\n')

	// O_EXCL refuses an existing file; O_NOFOLLOW refuses to follow a symlink
	// left where the advertisement belongs.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("creating the discovery file: %w", err)
	}
	if _, err := file.Write(encoded); err != nil {
		file.Close()
		os.Remove(path)
		return fmt.Errorf("writing the discovery file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing the discovery file: %w", err)
	}
	return nil
}

// ReadDiscovery reads the advertisement, refusing a document this build does not
// understand.
func ReadDiscovery(path string) (Discovery, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Discovery{}, err
	}
	var discovery Discovery
	if err := json.Unmarshal(data, &discovery); err != nil {
		return Discovery{}, fmt.Errorf("decoding the discovery document: %w", err)
	}
	if discovery.Version != DiscoveryVersion {
		return Discovery{}, fmt.Errorf("discovery version %d is not the supported version %d", discovery.Version, DiscoveryVersion)
	}
	return discovery, nil
}

// RemoveDiscovery removes the advertisement; a missing file is not an error, so
// shutdown is idempotent.
func RemoveDiscovery(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// removeStale clears an advertisement left by a process that is no longer
// running, and refuses to disturb a live one.
func removeStale(path string, alive func(pid int) bool) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading the existing discovery file: %w", err)
	}
	var existing Discovery
	if err := json.Unmarshal(data, &existing); err != nil {
		// An unreadable document cannot be attributed to a live process.
		return RemoveDiscovery(path)
	}
	if existing.PID > 0 && alive != nil && alive(existing.PID) {
		return fmt.Errorf("%w (pid %d)", ErrAlreadyRunning, existing.PID)
	}
	return RemoveDiscovery(path)
}

// ProcessAlive reports whether a pid is a live process this user can signal.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
