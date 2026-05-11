package base58

import (
	"bytes"
	"errors"
	"testing"
)

func TestEncodeToString(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "empty", data: nil, want: ""},
		{name: "zero", data: []byte{0}, want: "1"},
		{name: "leading zeros", data: []byte{0, 0, 1}, want: "112"},
		{name: "bitcoin vector", data: []byte("hello world"), want: "StV1DL6CwTryKyV"},
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
	tests := []struct {
		name  string
		input string
		want  []byte
	}{
		{name: "empty", input: "", want: []byte{}},
		{name: "zero", input: "1", want: []byte{0}},
		{name: "leading zeros", input: "112", want: []byte{0, 0, 1}},
		{name: "bitcoin vector", input: "StV1DL6CwTryKyV", want: []byte("hello world")},
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

func TestMultibase(t *testing.T) {
	data := []byte("hello world")
	encoded := EncodeMultibaseToString(data)
	if encoded != "zStV1DL6CwTryKyV" {
		t.Fatalf("EncodeMultibaseToString() = %q", encoded)
	}
	got, err := DecodeMultibaseString(encoded)
	if err != nil {
		t.Fatalf("DecodeMultibaseString() error = %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("DecodeMultibaseString() = %q, want %q", got, data)
	}
}

func TestDecodeRejectsInvalid(t *testing.T) {
	for _, input := range []string{"0", "O", "I", "l", "+"} {
		if _, err := DecodeString(input); !errors.Is(err, ErrInvalidEncoding) {
			t.Fatalf("DecodeString(%q) error = %v, want ErrInvalidEncoding", input, err)
		}
	}
	if _, err := DecodeMultibaseString("xStV1DL6CwTryKyV"); !errors.Is(err, ErrInvalidEncoding) {
		t.Fatalf("DecodeMultibaseString() error = %v, want ErrInvalidEncoding", err)
	}
}
