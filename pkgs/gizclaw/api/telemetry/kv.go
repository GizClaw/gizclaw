package telemetrypb

import (
	"encoding/json"
	"errors"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
)

const (
	MaxKVKeyBytes     = 64
	MaxKVValueBytes   = 1024
	MaxKVFrameEntries = 16
	MaxKVFrameBytes   = 4096
	MaxKVStoredKeys   = 64    // Includes deletion tombstones, retaining replay protection.
	MaxKVStoredBytes  = 16384 // Complete encoded state, including keys and timestamps.
)

var ErrInvalidKV = errors.New("telemetry: invalid or oversized KV observation")

// ValidateKV validates opaque JSON without interpreting its business meaning.
// Error text deliberately excludes keys and values.
func ValidateKV(kv *KVObservation) error {
	if kv == nil || len(kv.Key) == 0 || len(kv.Key) > MaxKVKeyBytes || !utf8.ValidString(kv.Key) {
		return ErrInvalidKV
	}
	if kv.Delete != nil {
		if !*kv.Delete || kv.JsonValue != nil {
			return ErrInvalidKV
		}
		return nil
	}
	if kv.JsonValue == nil || len(*kv.JsonValue) > MaxKVValueBytes || !utf8.ValidString(*kv.JsonValue) || !json.Valid([]byte(*kv.JsonValue)) {
		return ErrInvalidKV
	}
	return nil
}

// ValidateKVFrame is shared by Go senders and the Server. The complete frame
// budget applies when any KV entry is present, including mixed reports.
func ValidateKVFrame(frame *TelemetryFrame) error {
	count := 0
	for _, observation := range frame.GetObservations() {
		if body, ok := observation.GetBody().(*Observation_Kv); ok {
			count++
			if err := ValidateKV(body.Kv); err != nil {
				return err
			}
		}
	}
	if count > MaxKVFrameEntries || count > 0 && proto.Size(frame) > MaxKVFrameBytes {
		return ErrInvalidKV
	}
	return nil
}
