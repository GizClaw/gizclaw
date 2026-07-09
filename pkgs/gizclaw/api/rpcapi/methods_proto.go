package rpcapi

import (
	"fmt"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

var (
	rpcMethodToProto           map[RPCMethod]rpcpb.RpcMethod
	rpcMethodFromProto         map[rpcpb.RpcMethod]RPCMethod
	rpcRequestPayloadMessages  map[RPCMethod]string
	rpcResponsePayloadMessages map[RPCMethod]string
)

func init() {
	if err := loadProtoMethodRegistry(); err != nil {
		panic(err)
	}
}

func loadProtoMethodRegistry() error {
	toProto := map[RPCMethod]rpcpb.RpcMethod{}
	fromProto := map[rpcpb.RpcMethod]RPCMethod{}
	requestMessages := map[RPCMethod]string{}
	responseMessages := map[RPCMethod]string{}

	values := rpcpb.RpcMethod(0).Descriptor().Values()
	for i := 0; i < values.Len(); i++ {
		value := values.Get(i)
		protoMethod := rpcpb.RpcMethod(value.Number())
		if protoMethod == rpcpb.RpcMethod_RPC_METHOD_UNSPECIFIED {
			continue
		}

		opts, ok := value.Options().(*descriptorpb.EnumValueOptions)
		if !ok || opts == nil || !proto.HasExtension(opts, rpcpb.E_RpcMethod) {
			return fmt.Errorf("rpc: method id %d is missing rpc_method metadata", protoMethod)
		}
		ext := proto.GetExtension(opts, rpcpb.E_RpcMethod)
		meta, ok := ext.(*rpcpb.RpcMethodOptions)
		if !ok || meta == nil {
			return fmt.Errorf("rpc: method id %d has invalid rpc_method metadata", protoMethod)
		}

		method := RPCMethod(meta.GetName())
		if method == "" {
			return fmt.Errorf("rpc: method id %d has empty name", protoMethod)
		}
		if !method.Valid() {
			return fmt.Errorf("rpc: method id %d references unknown method %q", protoMethod, method)
		}
		if meta.GetRequest() == "" {
			return fmt.Errorf("rpc: method %q has empty request payload", method)
		}
		if meta.GetResponse() == "" {
			return fmt.Errorf("rpc: method %q has empty response payload", method)
		}
		if prev, exists := toProto[method]; exists {
			return fmt.Errorf("rpc: method %q has duplicate ids %d and %d", method, prev, protoMethod)
		}
		if prev, exists := fromProto[protoMethod]; exists {
			return fmt.Errorf("rpc: methods %q and %q share id %d", prev, method, protoMethod)
		}

		toProto[method] = protoMethod
		fromProto[protoMethod] = method
		requestMessages[method] = meta.GetRequest()
		responseMessages[method] = meta.GetResponse()
	}
	if len(toProto) == 0 {
		return fmt.Errorf("rpc: no protobuf RPC methods registered")
	}

	rpcMethodToProto = toProto
	rpcMethodFromProto = fromProto
	rpcRequestPayloadMessages = requestMessages
	rpcResponsePayloadMessages = responseMessages
	return nil
}

func ProtoMethod(method RPCMethod) (rpcpb.RpcMethod, error) {
	protoMethod, ok := rpcMethodToProto[method]
	if !ok {
		return rpcpb.RpcMethod_RPC_METHOD_UNSPECIFIED, fmt.Errorf("rpc: unknown method %q", method)
	}
	return protoMethod, nil
}

func MethodFromProto(protoMethod rpcpb.RpcMethod) (RPCMethod, error) {
	method, ok := rpcMethodFromProto[protoMethod]
	if !ok {
		return "", fmt.Errorf("rpc: unknown method id %d", protoMethod)
	}
	return method, nil
}

func ValidateProtoMethodRegistry() error {
	return loadProtoMethodRegistry()
}
