package base32

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestEncodeToString(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "empty", data: nil, want: ""},
		{name: "single zero", data: []byte{0}, want: "00"},
		{name: "single byte", data: []byte{0xff}, want: "ZW"},
		{name: "key", data: bytesFromRange(1, 32), want: "041061050R3GG28A1C60T3GF208H44RM2MB1E60S38DHR78Y3WG0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EncodeToString(tt.data); got != tt.want {
				t.Fatalf("EncodeToString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeString(t *testing.T) {
	data := bytesFromRange(1, 32)
	encoded := EncodeToString(data)

	tests := []struct {
		name  string
		input string
		want  []byte
	}{
		{name: "empty", input: "", want: []byte{}},
		{name: "canonical", input: encoded, want: data},
		{name: "lowercase", input: strings.ToLower(encoded), want: data},
		{name: "hyphenated aliases", input: hyphenate(strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(encoded, "0", "O"), "1", "L"))), want: data},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeString(tt.input)
			if err != nil {
				t.Fatalf("DecodeString() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("DecodeString() = %x, want %x", got, tt.want)
			}
		})
	}
}

func TestDecodeStringRejectsInvalid(t *testing.T) {
	for _, input := range []string{"*", "0", "01"} {
		if _, err := DecodeString(input); !errors.Is(err, ErrInvalidEncoding) {
			t.Fatalf("DecodeString(%q) error = %v, want ErrInvalidEncoding", input, err)
		}
	}
}

func bytesFromRange(first, count byte) []byte {
	data := make([]byte, count)
	for i := range data {
		data[i] = first + byte(i)
	}
	return data
}

func hyphenate(value string) string {
	return value[:13] + "-" + value[13:26] + "-" + value[26:39] + "-" + value[39:]
}
