package eventstore

import (
	"crypto/rand"
	"encoding/hex"
)

// generateEventID は 16 random bytes から 32-char hex string を生成する。
// EventEnvelope.EventID の発行に Repository から呼ばれる。実用上 ULID/UUIDv4
// と同等の一意性を持つ。
func generateEventID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
