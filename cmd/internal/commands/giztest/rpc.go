package giztest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

type methodInfo struct {
	method            rpcpb.RpcMethod
	request, response string
}

type rpcFailure struct {
	method  string
	code    int32
	message string
}

func (e *rpcFailure) Error() string {
	return fmt.Sprintf("rpc %s failed (code %d): %s", e.method, e.code, e.message)
}

func lookupMethod(name string) (methodInfo, error) {
	values := rpcpb.RpcMethod(0).Descriptor().Values()
	for i := 0; i < values.Len(); i++ {
		value := values.Get(i)
		opts, _ := value.Options().(*descriptorpb.EnumValueOptions)
		if opts == nil || !proto.HasExtension(opts, rpcpb.E_RpcMethod) {
			continue
		}
		meta, _ := proto.GetExtension(opts, rpcpb.E_RpcMethod).(*rpcpb.RpcMethodOptions)
		if meta != nil && meta.GetName() == name {
			return methodInfo{rpcpb.RpcMethod(value.Number()), meta.GetRequest(), meta.GetResponse()}, nil
		}
	}
	return methodInfo{}, fmt.Errorf("unsupported RPC method %q", name)
}

func dynamicMessage(name string) (*dynamicpb.Message, error) {
	mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName("gizclaw.rpc.v1." + name))
	if err != nil {
		return nil, err
	}
	return dynamicpb.NewMessage(mt.Descriptor()), nil
}

func invokeUnary(ctx context.Context, client *gizcli.Client, step Step, params any) (map[string]any, error) {
	if step.RPC == nil {
		return nil, fmt.Errorf("rpc operation is required")
	}
	method := step.RPC.Method
	info, err := lookupMethod(method)
	if err != nil {
		return nil, err
	}
	reqMsg, err := dynamicMessage(info.request)
	if err != nil {
		return nil, err
	}
	params = wrapValueMessage(reqMsg.Descriptor(), params)
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if string(data) == "null" {
		data = []byte("{}")
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(data, reqMsg); err != nil {
		return nil, fmt.Errorf("encode %s request: %w", method, err)
	}
	payload, err := proto.Marshal(reqMsg)
	if err != nil {
		return nil, err
	}
	conn := client.PeerConn()
	if conn == nil {
		return nil, fmt.Errorf("client disconnected")
	}
	stream, err := conn.Dial(gizcli.ServicePeerRPC)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	if err := stream.SetDeadline(deadline); err != nil {
		return nil, err
	}
	frame, err := rpcapi.NewProtobufFrame(&rpcpb.RpcRequest{Id: step.ID, Method: info.method, Payload: payload})
	if err != nil {
		return nil, err
	}
	if err := rpcapi.WriteFrame(stream, frame); err != nil {
		return nil, err
	}
	if err := rpcapi.WriteEOS(stream); err != nil {
		return nil, err
	}
	responseFrame, err := rpcapi.ReadFrame(stream)
	if err != nil {
		return nil, err
	}
	var response rpcpb.RpcResponse
	if err := rpcapi.DecodeProtobufFrame(responseFrame, &response); err != nil {
		return nil, err
	}
	if response.GetId() != step.ID {
		return nil, fmt.Errorf("rpc %s response id %q does not match request %q", method, response.GetId(), step.ID)
	}
	if err := rpcapi.ReadEOS(stream); err != nil {
		return nil, err
	}
	if rpcErr := response.GetError(); rpcErr != nil {
		return nil, &rpcFailure{method: method, code: int32(rpcErr.GetCode()), message: rpcErr.GetMessage()}
	}
	respMsg, err := dynamicMessage(info.response)
	if err != nil {
		return nil, err
	}
	if err := proto.Unmarshal(response.GetPayload(), respMsg); err != nil {
		return nil, err
	}
	jsonData, err := (protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}).Marshal(respMsg)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return nil, err
	}
	if unwrapped, ok := unwrapValueMessage(respMsg.Descriptor(), result); ok {
		return unwrapped, nil
	}
	return result, nil
}

func validateRPCRequestShape(method string, request any, specs map[string]VariableSpec) error {
	info, err := lookupMethod(method)
	if err != nil {
		return err
	}
	message, err := dynamicMessage(info.request)
	if err != nil {
		return err
	}
	normalized, err := validationValue(request, specs)
	if err != nil {
		return err
	}
	normalized = wrapValueMessage(message.Descriptor(), normalized)
	data, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	if string(data) == "null" {
		data = []byte("{}")
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(data, message); err != nil {
		return fmt.Errorf("invalid %s request: %w", method, err)
	}
	return nil
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

func validationValue(input any, specs map[string]VariableSpec) (any, error) {
	switch value := input.(type) {
	case string:
		if match := referencePattern.FindStringSubmatch(value); match != nil {
			spec, ok := specs[match[1]]
			if !ok {
				return nil, fmt.Errorf("unknown variable %q", match[1])
			}
			switch spec.Type {
			case "string":
				return "validation", nil
			case "integer":
				return 1, nil
			case "number":
				return 1.0, nil
			case "boolean":
				return true, nil
			case "object":
				return map[string]any{}, nil
			default:
				return nil, fmt.Errorf("variable %q type %s cannot be used in an RPC JSON request", match[1], spec.Type)
			}
		}
		return regexpReferenceAll.ReplaceAllString(value, "validation"), nil
	case []any:
		result := make([]any, len(value))
		for i := range value {
			item, err := validationValue(value[i], specs)
			if err != nil {
				return nil, err
			}
			result[i] = item
		}
		return result, nil
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, raw := range value {
			item, err := validationValue(raw, specs)
			if err != nil {
				return nil, err
			}
			result[key] = item
		}
		return result, nil
	default:
		return input, nil
	}
}
