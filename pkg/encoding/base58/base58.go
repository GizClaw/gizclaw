// Package base58 implements Bitcoin Base58 encoding.
package base58

import (
	"errors"
	"fmt"
	"math/big"
)

const (
	btcAlphabet     = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	multibasePrefix = 'z'
)

// ErrInvalidEncoding is returned when base58btc input is malformed.
var ErrInvalidEncoding = errors.New("base58: invalid encoding")

var (
	base        = big.NewInt(58)
	zero        = big.NewInt(0)
	decodeTable = buildDecodeTable()
)

// EncodeToString encodes data using the raw base58btc alphabet.
func EncodeToString(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	leadingZeros := 0
	for leadingZeros < len(data) && data[leadingZeros] == 0 {
		leadingZeros++
	}

	value := new(big.Int).SetBytes(data)
	var encoded []byte
	mod := new(big.Int)
	for value.Cmp(zero) > 0 {
		value.DivMod(value, base, mod)
		encoded = append(encoded, btcAlphabet[mod.Int64()])
	}

	out := make([]byte, 0, leadingZeros+len(encoded))
	for i := 0; i < leadingZeros; i++ {
		out = append(out, btcAlphabet[0])
	}
	for i := len(encoded) - 1; i >= 0; i-- {
		out = append(out, encoded[i])
	}
	return string(out)
}

// DecodeString decodes a raw base58btc string.
func DecodeString(value string) ([]byte, error) {
	if value == "" {
		return []byte{}, nil
	}

	leadingZeros := 0
	for leadingZeros < len(value) && value[leadingZeros] == btcAlphabet[0] {
		leadingZeros++
	}

	acc := new(big.Int)
	for i := 0; i < len(value); i++ {
		digit := decodeTable[value[i]]
		if digit < 0 {
			return nil, fmt.Errorf("%w: invalid byte %q at offset %d", ErrInvalidEncoding, value[i], i)
		}
		acc.Mul(acc, base)
		acc.Add(acc, big.NewInt(int64(digit)))
	}

	decoded := acc.Bytes()
	out := make([]byte, leadingZeros+len(decoded))
	copy(out[leadingZeros:], decoded)
	return out, nil
}

// EncodeMultibaseToString encodes data as multibase base58btc with the z prefix.
func EncodeMultibaseToString(data []byte) string {
	return string(multibasePrefix) + EncodeToString(data)
}

// DecodeMultibaseString decodes a multibase base58btc string with the z prefix.
func DecodeMultibaseString(value string) ([]byte, error) {
	if value == "" || value[0] != multibasePrefix {
		return nil, ErrInvalidEncoding
	}
	return DecodeString(value[1:])
}

func buildDecodeTable() [256]int {
	var table [256]int
	for i := range table {
		table[i] = -1
	}
	for i := range len(btcAlphabet) {
		table[btcAlphabet[i]] = i
	}
	return table
}
