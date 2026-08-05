package customid

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	// MaxResourceIDCharacters is the OpenAPI-visible maximum length of a
	// caller-supplied Admin resource ID.
	MaxResourceIDCharacters = 1024
	// MaxResourceIDBytes keeps escaped and composite storage keys comfortably
	// below Badger's 65,000-byte key limit.
	MaxResourceIDBytes = 4096
	// MaxFriendGroupIDCharacters leaves enough room for the deterministic
	// FriendGroupMember ID after worst-case URI escaping of the group ID and
	// appending a canonical 32-byte base58 peer public key.
	MaxFriendGroupIDCharacters = 80
)

// ValidateResourceID checks the transport-level constraints shared by caller-
// supplied Admin resource IDs. IDs remain opaque and are persisted exactly as
// submitted, but the two standalone URI dot segments cannot be represented by
// generated path-parameter clients without URL normalization.
func ValidateResourceID(value string) error {
	if value == "" {
		return errors.New("id is required")
	}
	if strings.TrimSpace(value) != value {
		return errors.New("id must not contain surrounding whitespace")
	}
	if utf8.RuneCountInString(value) > MaxResourceIDCharacters || len(value) > MaxResourceIDBytes {
		return errors.New("id exceeds the 1024-character limit")
	}
	if value == "." || value == ".." {
		return errors.New("id must not be a URI dot segment")
	}
	return nil
}

// ValidateFriendGroupID applies the tighter bound required by the reversible
// FriendGroupMember composite resource ID.
func ValidateFriendGroupID(value string) error {
	if err := ValidateResourceID(value); err != nil {
		return err
	}
	if utf8.RuneCountInString(value) > MaxFriendGroupIDCharacters {
		return errors.New("id exceeds the 80-character FriendGroup limit")
	}
	return nil
}

// EscapeStoreSegment preserves an opaque ID in one default kv key segment.
func EscapeStoreSegment(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	return strings.ReplaceAll(value, ":", "%3A")
}

// UnescapeStoreSegment reverses EscapeStoreSegment.
func UnescapeStoreSegment(value string) string {
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

// OpaquePathSegment maps an unbounded opaque ID to one bounded, portable
// filesystem/object-key segment. The full SHA-256 digest keeps the mapping
// stable and collision-resistant without exposing a path component to NAME_MAX.
func OpaquePathSegment(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "id-sha256-" + hex.EncodeToString(digest[:])
}
