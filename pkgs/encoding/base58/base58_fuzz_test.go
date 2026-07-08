package base58

import (
	"bytes"
	"errors"
	"testing"
)

func FuzzDecodeString(f *testing.F) {
	for _, seed := range []string{
		"",
		"1",
		"112",
		"StV1DL6CwTryKyV",
		"0",
		"O",
		"I",
		"l",
		"+",
		"zStV1DL6CwTryKyV",
		"xStV1DL6CwTryKyV",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 512 {
			return
		}
		decoded, err := DecodeString(value)
		if err != nil {
			if !errors.Is(err, ErrInvalidEncoding) {
				t.Fatalf("DecodeString(%q) error = %v, want ErrInvalidEncoding", value, err)
			}
		} else {
			encoded := EncodeToString(decoded)
			roundTrip, err := DecodeString(encoded)
			if err != nil {
				t.Fatalf("DecodeString(EncodeToString(...)) error = %v", err)
			}
			if !bytes.Equal(roundTrip, decoded) {
				t.Fatalf("round trip = %x, want %x", roundTrip, decoded)
			}
		}

		multibase, err := DecodeMultibaseString(value)
		if err != nil {
			if !errors.Is(err, ErrInvalidEncoding) {
				t.Fatalf("DecodeMultibaseString(%q) error = %v, want ErrInvalidEncoding", value, err)
			}
			return
		}
		encoded := EncodeMultibaseToString(multibase)
		roundTrip, err := DecodeMultibaseString(encoded)
		if err != nil {
			t.Fatalf("DecodeMultibaseString(EncodeMultibaseToString(...)) error = %v", err)
		}
		if !bytes.Equal(roundTrip, multibase) {
			t.Fatalf("multibase round trip = %x, want %x", roundTrip, multibase)
		}
	})
}
