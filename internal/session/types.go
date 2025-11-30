package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// SessionContext represents the absolute coordinates of a session.
type SessionContext struct {
	BootID      string // Kernel Boot ID (Linux) or MachineGUID (Windows)
	ContainerID string // Linux: namespace ID (e.g. /proc/self/ns/pid); Empty on Windows
	AnchorPID   uint32 // The Process ID of the stable parent (uint32 for Windows DWORD compatibility)
	StartTime   uint64 // Creation time (ticks or filetime)
	TTYName     string // Optional: /dev/pts/X or MinTTY pipe name
}

// GenerateHash produces the final deterministic Session ID.
// Formula: SHA256(BootID : ContainerID/NamespaceID : TTY : PID : StartTime)
func (c *SessionContext) GenerateHash() string {
	// Delimiter ":" is safe: BootID (UUID), ContainerID (hex), TTYName (/dev/pts/X or ptyN)
	// none contain colons in standard configurations.
	raw := fmt.Sprintf("%s:%s:%s:%d:%d",
		c.BootID,
		c.ContainerID,
		c.TTYName,
		c.AnchorPID,
		c.StartTime,
	)

	hasher := sha256.New()
	hasher.Write([]byte(raw))
	return hex.EncodeToString(hasher.Sum(nil))
}
