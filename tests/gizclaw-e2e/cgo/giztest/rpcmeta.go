package main

import (
	"encoding/json"
	"fmt"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// methodInfo is the wire metadata `api/proto/rpc/rpc.proto` attaches to one
// RPC method: its numeric id and its request and response message names.
type methodInfo struct {
	id       rpcpb.RpcMethod
	request  string
	response string
}

var methodsByName = buildMethodIndex()

func buildMethodIndex() map[string]methodInfo {
	index := map[string]methodInfo{}
	values := rpcpb.RpcMethod(0).Descriptor().Values()
	for i := range values.Len() {
		value := values.Get(i)
		opts, _ := value.Options().(*descriptorpb.EnumValueOptions)
		if opts == nil || !proto.HasExtension(opts, rpcpb.E_RpcMethod) {
			continue
		}
		meta, _ := proto.GetExtension(opts, rpcpb.E_RpcMethod).(*rpcpb.RpcMethodOptions)
		if meta == nil || meta.GetName() == "" {
			continue
		}
		index[meta.GetName()] = methodInfo{
			id:       rpcpb.RpcMethod(value.Number()),
			request:  meta.GetRequest(),
			response: meta.GetResponse(),
		}
	}
	return index
}

func lookupMethod(name string) (methodInfo, error) {
	info, ok := methodsByName[name]
	if !ok {
		return methodInfo{}, fmt.Errorf("unsupported RPC method %q", name)
	}
	return info, nil
}

func lookupMethodByID(id rpcpb.RpcMethod) (string, methodInfo, error) {
	for name, info := range methodsByName {
		if info.id == id {
			return name, info, nil
		}
	}
	return "", methodInfo{}, fmt.Errorf("unknown RPC method id %d", id)
}

func dynamicMessage(name string) (*dynamicpb.Message, error) {
	messageType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName("gizclaw.rpc.v1." + name))
	if err != nil {
		return nil, err
	}
	return dynamicpb.NewMessage(messageType.Descriptor()), nil
}

// encodePayload turns a decoded JSON value into the protobuf payload bytes of
// messageName. A message with a single `value` field is wrapped the way the
// Server expects, matching the Go runner.
func encodePayload(messageName string, value any) ([]byte, error) {
	message, err := dynamicMessage(messageName)
	if err != nil {
		return nil, err
	}
	value = wrapValueMessage(message.Descriptor(), value)
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if string(data) == "null" {
		data = []byte("{}")
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(data, message); err != nil {
		return nil, fmt.Errorf("encode %s: %w", messageName, err)
	}
	return proto.Marshal(message)
}

// decodePayload turns protobuf payload bytes into a decoded JSON object,
// unwrapping a single-field `value` message.
func decodePayload(messageName string, payload []byte) (map[string]any, error) {
	message, err := dynamicMessage(messageName)
	if err != nil {
		return nil, err
	}
	if err := proto.Unmarshal(payload, message); err != nil {
		return nil, err
	}
	data, err := (protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}).Marshal(message)
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	if unwrapped, ok := unwrapValueMessage(message.Descriptor(), decoded); ok {
		return unwrapped, nil
	}
	return decoded, nil
}

func wrapValueMessage(descriptor protoreflect.MessageDescriptor, input any) any {
	fields := descriptor.Fields()
	if fields.Len() != 1 || fields.Get(0).Name() != "value" || fields.Get(0).Kind() != protoreflect.MessageKind {
		return input
	}
	if object, ok := input.(map[string]any); ok {
		if _, wrapped := object["value"]; wrapped {
			return input
		}
	}
	return map[string]any{"value": input}
}

func unwrapValueMessage(descriptor protoreflect.MessageDescriptor, input map[string]any) (map[string]any, bool) {
	fields := descriptor.Fields()
	if fields.Len() != 1 || fields.Get(0).Name() != "value" || fields.Get(0).Kind() != protoreflect.MessageKind {
		return nil, false
	}
	value, ok := input["value"].(map[string]any)
	return value, ok
}
