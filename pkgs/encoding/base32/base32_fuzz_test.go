package base32

import (
	"bytes"
	"errors"
	"testing"
)

func FuzzDecodeString(f *testing.F) {
	for _, seed := range []string{
		"",
		"00",
		"ZW",
		"041061050R3GG28A1C60T3GF208H44RM2MB1E60S38DHR78Y3WG0",
		"041061050r3g-g28a1c60t3gf208h44rm2mb1e60s38dhr78y3wg0",
		"*",
		"0",
		"01",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 1024 {
			return
		}
		decoded, err := DecodeString(value)
		if err != nil {
			if !errors.Is(err, ErrInvalidEncoding) {
				t.Fatalf("DecodeString(%q) error = %v, want ErrInvalidEncoding", value, err)
			}
			return
		}
		encoded := EncodeToString(decoded)
		roundTrip, err := DecodeString(encoded)
		if err != nil {
			t.Fatalf("DecodeString(EncodeToString(...)) error = %v", err)
		}
		if !bytes.Equal(roundTrip, decoded) {
			t.Fatalf("round trip = %x, want %x", roundTrip, decoded)
		}
	})
}
