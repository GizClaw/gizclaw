package genx

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/kaptinlin/jsonrepair"
)

// unmarshalJSON unmarshals JSON data into v, attempting to repair malformed JSON.
func unmarshalJSON(data []byte, v any) error {
	err := json.Unmarshal(data, v)
	if err == nil {
		return nil
	}
	if _, ok := err.(*json.SyntaxError); ok {
		fixed, err := jsonrepair.Repair(string(data))
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(fixed), v)
	}
	return err
}

// hexString generates a random 16-character hexadecimal string.
func hexString() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("genx: failed to read random bytes: %w", err))
	}
	return hex.EncodeToString(b[:])
}
