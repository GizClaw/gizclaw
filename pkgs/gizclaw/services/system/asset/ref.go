package asset

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	refPrefix = "asset://"
	idBytes   = 16
	idHexLen  = idBytes * 2
)

// Ref is a stable public reference to an AssetService-owned binary object.
type Ref string

// ParseRef validates and returns a canonical asset reference.
func ParseRef(value string) (Ref, error) {
	if len(value) != len(refPrefix)+idHexLen || !strings.HasPrefix(value, refPrefix) {
		return "", fmt.Errorf("%w: reference must match asset://<32-lowercase-hex>", ErrInvalid)
	}
	id := value[len(refPrefix):]
	if strings.ToLower(id) != id {
		return "", fmt.Errorf("%w: asset id must use lowercase hex", ErrInvalid)
	}
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != idBytes {
		return "", fmt.Errorf("%w: reference must match asset://<32-lowercase-hex>", ErrInvalid)
	}
	return Ref(value), nil
}

// String returns the canonical wire representation.
func (r Ref) String() string {
	return string(r)
}

func refFromID(id string) (Ref, error) {
	return ParseRef(refPrefix + id)
}

func (r Ref) id() (string, error) {
	canonical, err := ParseRef(string(r))
	if err != nil {
		return "", err
	}
	return string(canonical)[len(refPrefix):], nil
}

// IDGenerator produces an opaque 128-bit asset identifier.
type IDGenerator func() ([idBytes]byte, error)

func randomID() ([idBytes]byte, error) {
	var id [idBytes]byte
	_, err := rand.Read(id[:])
	return id, err
}
