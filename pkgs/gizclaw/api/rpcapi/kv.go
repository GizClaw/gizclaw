package rpcapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func fillKVProto(msg protoreflect.Message, value reflect.Value) error {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(value.Interface())
	data := encoded.Bytes()
	if err != nil {
		return fmt.Errorf("rpc: invalid KV entry")
	}
	var entry struct {
		Value      json.RawMessage `json:"value"`
		JSONValue  string          `json:"json_value"`
		ObservedAt time.Time       `json:"observed_at"`
	}
	if json.Unmarshal(data, &entry) != nil || !json.Valid(entry.Value) || entry.ObservedAt.IsZero() {
		return fmt.Errorf("rpc: invalid KV entry")
	}
	if entry.JSONValue != "" {
		entry.Value = json.RawMessage(entry.JSONValue)
	}
	if !json.Valid(entry.Value) {
		return fmt.Errorf("rpc: invalid KV entry")
	}
	fields := msg.Descriptor().Fields()
	msg.Set(fields.ByName("json_value"), protoreflect.ValueOfString(string(entry.Value)))
	msg.Set(fields.ByName("observed_at"), protoreflect.ValueOfString(entry.ObservedAt.Format(time.RFC3339Nano)))
	return nil
}

func kvProtoJSON(msg protoreflect.Message) (any, error) {
	fields := msg.Descriptor().Fields()
	value := json.RawMessage(msg.Get(fields.ByName("json_value")).String())
	observedAt := msg.Get(fields.ByName("observed_at")).String()
	if !json.Valid(value) {
		return nil, fmt.Errorf("rpc: invalid KV JSON value")
	}
	if _, err := time.Parse(time.RFC3339Nano, observedAt); err != nil {
		return nil, fmt.Errorf("rpc: invalid KV observation time")
	}
	return map[string]any{"value": value, "observed_at": observedAt, "json_value": string(value)}, nil
}
