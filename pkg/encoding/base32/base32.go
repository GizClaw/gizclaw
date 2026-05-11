// Package base32 implements unpadded Crockford Base32 encoding.
package base32

import (
	"errors"
	"fmt"
)

const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ErrInvalidEncoding is returned when Crockford Base32 input is malformed.
var ErrInvalidEncoding = errors.New("base32: invalid encoding")

// EncodeToString encodes data using Crockford Base32 without padding.
func EncodeToString(data []byte) string {
	out := make([]byte, 0, (len(data)*8+4)/5)
	buffer := 0
	bits := 0
	for _, b := range data {
		buffer = (buffer << 8) | int(b)
		bits += 8
		for bits >= 5 {
			out = append(out, crockfordAlphabet[(buffer>>(bits-5))&31])
			bits -= 5
			if bits == 0 {
				buffer = 0
			} else {
				buffer &= (1 << bits) - 1
			}
		}
	}
	if bits > 0 {
		out = append(out, crockfordAlphabet[(buffer<<(5-bits))&31])
	}
	return string(out)
}

// DecodeString decodes unpadded Crockford Base32.
//
// Hyphens are ignored. Lowercase letters are accepted. The common Crockford
// aliases O/o for 0 and I/i/L/l for 1 are accepted.
func DecodeString(value string) ([]byte, error) {
	out := make([]byte, 0, len(value)*5/8)
	buffer := 0
	bits := 0
	for i := 0; i < len(value); i++ {
		v, ok, skip := decodeValue(value[i])
		if skip {
			continue
		}
		if !ok {
			return nil, fmt.Errorf("%w: invalid byte %q at offset %d", ErrInvalidEncoding, value[i], i)
		}
		buffer = (buffer << 5) | v
		bits += 5
		for bits >= 8 {
			out = append(out, byte(buffer>>(bits-8)))
			bits -= 8
			if bits == 0 {
				buffer = 0
			} else {
				buffer &= (1 << bits) - 1
			}
		}
	}
	if bits >= 5 || buffer != 0 {
		return nil, ErrInvalidEncoding
	}
	return out, nil
}

func decodeValue(ch byte) (value int, ok bool, skip bool) {
	switch {
	case ch == '-':
		return 0, false, true
	case ch == 'O' || ch == 'o':
		return 0, true, false
	case ch == 'I' || ch == 'i' || ch == 'L' || ch == 'l':
		return 1, true, false
	case ch >= '0' && ch <= '9':
		return int(ch - '0'), true, false
	case ch >= 'a' && ch <= 'z':
		ch -= 'a' - 'A'
	}
	for i := range len(crockfordAlphabet) {
		if crockfordAlphabet[i] == ch {
			return i, true, false
		}
	}
	return 0, false, false
}
