package giztestcmd

import (
	"crypto/rand"
	"encoding/hex"
)

// newStreamID returns a fresh identifier for one Peer stream or relay turn.
// The Server only requires it to be unique within a connection.
func newStreamID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "g" + hex.EncodeToString(raw), nil
}
