package rpcapi

import (
	"fmt"
	"reflect"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// ClientToolRequestMessage converts validated arguments to the registered
// protobuf request. JSON numbers must be decoded with json.Decoder.UseNumber.
func ClientToolRequestMessage(tool rpcpb.ClientTool, args any) (proto.Message, error) {
	meta, err := ClientToolMetadata(tool)
	if err != nil {
		return nil, err
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(rpcPayloadProtoPackage + meta.Request))
	if err != nil {
		return nil, err
	}
	message := typ.New()
	if err := fillProtoMessageFromGo(message, reflect.ValueOf(args), reflect.Value{}); err != nil {
		return nil, err
	}
	return message.Interface(), nil
}

// ClientToolRequestFromBytes decodes the request selected by the tool registry.
func ClientToolRequestFromBytes(tool rpcpb.ClientTool, data []byte) (proto.Message, error) {
	meta, err := ClientToolMetadata(tool)
	if err != nil {
		return nil, err
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(rpcPayloadProtoPackage + meta.Request))
	if err != nil {
		return nil, err
	}
	message := typ.New().Interface()
	if err := proto.Unmarshal(data, message); err != nil {
		return nil, err
	}
	return message, nil
}

// ClientToolResponseMessage decodes the response selected by the registry.
func ClientToolResponseMessage(tool rpcpb.ClientTool, data []byte) (proto.Message, error) {
	meta, err := ClientToolMetadata(tool)
	if err != nil {
		return nil, err
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(rpcPayloadProtoPackage + meta.Response))
	if err != nil {
		return nil, err
	}
	message := typ.New().Interface()
	if err := proto.Unmarshal(data, message); err != nil {
		return nil, err
	}
	return message, nil
}

// ClientToolResultJSON projects a protobuf result using the SDK JSON mapping.
func ClientToolResultJSON(message proto.Message) (map[string]any, error) {
	value, err := protoMessageGoValue(message.ProtoReflect(), decodeRPCPayloadOptions{emitDefaults: true})
	if err != nil {
		return nil, err
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("rpc: client tool result must be an object")
	}
	return result, nil
}

const (
	RPCMethodClientToolV0Invoke   RPCMethod = "client.tool.v0.invoke"
	RPCMethodClientToolV0List     RPCMethod = "client.tool.v0.list"
	RPCMethodClientRPCMethodsList RPCMethod = "client.rpc.methods.list"
)

// ClientToolMetadata returns the registry binding for a predefined procedure.
func ClientToolMetadata(tool rpcpb.ClientTool) (*rpcpb.ClientToolOptions, error) {
	value := tool.Descriptor().Values().ByNumber(tool.Number())
	if value == nil || !proto.HasExtension(value.Options(), rpcpb.E_ClientTool) {
		return nil, fmt.Errorf("rpc: unknown client tool %d", tool)
	}
	meta, ok := proto.GetExtension(value.Options(), rpcpb.E_ClientTool).(*rpcpb.ClientToolOptions)
	if !ok || meta.GetName() == "" || meta.GetRequest() == "" || meta.GetResponse() == "" {
		return nil, fmt.Errorf("rpc: invalid client tool metadata for %d", tool)
	}
	return proto.Clone(meta).(*rpcpb.ClientToolOptions), nil
}

// ClientToolByName resolves the stable procedure name from the proto registry.
func ClientToolByName(name string) (rpcpb.ClientTool, error) {
	values := rpcpb.ClientTool(0).Descriptor().Values()
	for i := 0; i < values.Len(); i++ {
		tool := rpcpb.ClientTool(values.Get(i).Number())
		meta, err := ClientToolMetadata(tool)
		if err == nil && meta.Name == name {
			return tool, nil
		}
	}
	return 0, fmt.Errorf("rpc: unknown client tool %q", name)
}

// EncodeClientToolRequest wraps a typed procedure payload in tool/v0 invoke.
func EncodeClientToolRequest(tool rpcpb.ClientTool, params *RPCPayload) (*RPCPayload, error) {
	meta, err := ClientToolMetadata(tool)
	if err != nil {
		return nil, err
	}
	var payload []byte
	if params != nil {
		payload, err = params.bytesForMessage(meta.Request)
		if err != nil {
			return nil, err
		}
	}
	var result RPCPayload
	err = result.FromClientToolV0InvokeRequest(&rpcpb.ClientToolV0InvokeRequest{Tool: tool, Payload: payload})
	return &result, err
}

// DecodeClientToolRequest selects the procedure payload using ClientTool.
func DecodeClientToolRequest(request *rpcpb.ClientToolV0InvokeRequest) (*RPCPayload, error) {
	meta, err := ClientToolMetadata(request.GetTool())
	if err != nil {
		return nil, err
	}
	return newRPCPayload(meta.Request, request.GetPayload(), false), nil
}

// EncodeClientToolResponse wraps the procedure response in tool/v0 invoke.
func EncodeClientToolResponse(tool rpcpb.ClientTool, result *RPCPayload) (*RPCPayload, error) {
	meta, err := ClientToolMetadata(tool)
	if err != nil {
		return nil, err
	}
	var payload []byte
	if result != nil {
		payload, err = result.bytesForMessage(meta.Response)
		if err != nil {
			return nil, err
		}
	}
	var wrapped RPCPayload
	err = wrapped.FromClientToolV0InvokeResponse(&rpcpb.ClientToolV0InvokeResponse{Payload: payload})
	return &wrapped, err
}

// DecodeClientToolResponse unwraps the response selected by the invoked tool.
func DecodeClientToolResponse(tool rpcpb.ClientTool, result *RPCPayload) (*RPCPayload, error) {
	meta, err := ClientToolMetadata(tool)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("rpc: missing client tool result")
	}
	response, err := result.AsClientToolV0InvokeResponse()
	if err != nil {
		return nil, err
	}
	return newRPCPayload(meta.Response, response.GetPayload(), true), nil
}

// AsClientToolV0InvokeRequest decodes the protobuf payload.
func (t RPCPayload) AsClientToolV0InvokeRequest() (*rpcpb.ClientToolV0InvokeRequest, error) {
	body := new(rpcpb.ClientToolV0InvokeRequest)
	err := t.decode("ClientToolV0InvokeRequest", body)
	return body, err
}

// FromClientToolV0InvokeRequest encodes the protobuf payload.
func (t *RPCPayload) FromClientToolV0InvokeRequest(v *rpcpb.ClientToolV0InvokeRequest) error {
	return t.encode("ClientToolV0InvokeRequest", v)
}

// AsClientToolV0InvokeResponse decodes the protobuf payload.
func (t RPCPayload) AsClientToolV0InvokeResponse() (*rpcpb.ClientToolV0InvokeResponse, error) {
	body := new(rpcpb.ClientToolV0InvokeResponse)
	err := t.decode("ClientToolV0InvokeResponse", body)
	return body, err
}

// FromClientToolV0InvokeResponse encodes the protobuf payload.
func (t *RPCPayload) FromClientToolV0InvokeResponse(v *rpcpb.ClientToolV0InvokeResponse) error {
	return t.encode("ClientToolV0InvokeResponse", v)
}

// AsClientToolV0ListRequest decodes the protobuf payload.
func (t RPCPayload) AsClientToolV0ListRequest() (*rpcpb.ClientToolV0ListRequest, error) {
	body := new(rpcpb.ClientToolV0ListRequest)
	err := t.decode("ClientToolV0ListRequest", body)
	return body, err
}

// FromClientToolV0ListRequest encodes the protobuf payload.
func (t *RPCPayload) FromClientToolV0ListRequest(v *rpcpb.ClientToolV0ListRequest) error {
	return t.encode("ClientToolV0ListRequest", v)
}

// AsClientToolV0ListResponse decodes the protobuf payload.
func (t RPCPayload) AsClientToolV0ListResponse() (*rpcpb.ClientToolV0ListResponse, error) {
	body := new(rpcpb.ClientToolV0ListResponse)
	err := t.decode("ClientToolV0ListResponse", body)
	return body, err
}

// FromClientToolV0ListResponse encodes the protobuf payload.
func (t *RPCPayload) FromClientToolV0ListResponse(v *rpcpb.ClientToolV0ListResponse) error {
	return t.encode("ClientToolV0ListResponse", v)
}

// AsClientRpcMethodsListRequest decodes the protobuf payload.
func (t RPCPayload) AsClientRpcMethodsListRequest() (*rpcpb.ClientRpcMethodsListRequest, error) {
	body := new(rpcpb.ClientRpcMethodsListRequest)
	err := t.decode("ClientRpcMethodsListRequest", body)
	return body, err
}

// FromClientRpcMethodsListRequest encodes the protobuf payload.
func (t *RPCPayload) FromClientRpcMethodsListRequest(v *rpcpb.ClientRpcMethodsListRequest) error {
	return t.encode("ClientRpcMethodsListRequest", v)
}

// AsClientRpcMethodsListResponse decodes the protobuf payload.
func (t RPCPayload) AsClientRpcMethodsListResponse() (*rpcpb.ClientRpcMethodsListResponse, error) {
	body := new(rpcpb.ClientRpcMethodsListResponse)
	err := t.decode("ClientRpcMethodsListResponse", body)
	return body, err
}

// FromClientRpcMethodsListResponse encodes the protobuf payload.
func (t *RPCPayload) FromClientRpcMethodsListResponse(v *rpcpb.ClientRpcMethodsListResponse) error {
	return t.encode("ClientRpcMethodsListResponse", v)
}
