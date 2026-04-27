package eventsourcing

import (
	"crypto/rand"
	"encoding/hex"
)

// generateEventID produces a 32-char hex string from 16 random bytes.
// Equivalent uniqueness to UUIDv4 / ULID for practical purposes.
func generateEventID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
