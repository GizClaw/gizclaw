package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// mhsInstance is a physical instance scripted by a document. Each provider owns
// its own decoded values; read and write share that state within one client.
type mhsInstance struct {
	Hwd         string         `json:"hwd"`
	Value       map[string]any `json:"value"`
	WriteFields []string       `json:"write_fields"`
}

func statefulMhs(response any) bool {
	object, ok := response.(map[string]any)
	return ok && object["instances"] != nil
}

func decodeMhsInstances(response any) (map[string]mhsInstance, error) {
	data, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	var config struct {
		Instances map[string]mhsInstance `json:"instances"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	for id, instance := range config.Instances {
		if !rpcapi.ValidMhsKey(id) {
			return nil, fmt.Errorf("invalid fixture instance %q", id)
		}
		hwd, err := rpcapi.ClientHwdByName(instance.Hwd)
		if err != nil {
			return nil, err
		}
		meta, err := rpcapi.ClientHwdMetadata(hwd)
		if err != nil {
			return nil, err
		}
		if _, err := encodePayload(meta.ReadResponse, instance.Value); err != nil {
			return nil, fmt.Errorf("instance %s: %w", id, err)
		}
		if instance.Value == nil {
			instance.Value = map[string]any{}
			config.Instances[id] = instance
		}
	}
	return config.Instances, nil
}

// mhsValue checks identity and write permissions before atomically updating one
// instance. Encoding happens after releasing the lock using the copied state.
func (p *clientRPCProvider) mhsValue(id, hwd string, args map[string]any) (mhsInstance, int32, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	instance, ok := p.instances[id]
	if !ok || instance.Hwd != hwd {
		return mhsInstance{}, int32(rpcpb.StatusCode_STATUS_CODE_NOT_FOUND), "instance not installed"
	}
	for field := range args {
		if !slices.Contains(instance.WriteFields, field) {
			return mhsInstance{}, int32(rpcpb.StatusCode_STATUS_CODE_UNIMPLEMENTED), "write field not implemented"
		}
	}
	maps.Copy(instance.Value, args)
	instance.Value = maps.Clone(instance.Value)
	instance.WriteFields = slices.Clone(instance.WriteFields)
	return instance, 0, ""
}

func (p *clientRPCProvider) answerMhs(method rpcpb.RpcMethod, requestPayload []byte) ([]byte, int32, string, error) {
	var id string
	var hwd rpcpb.ClientHwd
	var args map[string]any
	if method == rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_READ {
		request := new(rpcpb.ClientMhsV0ReadRequest)
		if err := proto.Unmarshal(requestPayload, request); err != nil || rpcapi.ValidateMhsHwdReadRequest(request) != nil {
			return nil, int32(rpcpb.StatusCode_STATUS_CODE_INVALID_ARGUMENT), "invalid params", nil
		}
		id, hwd = request.Id, request.Hwd
	} else {
		request := new(rpcpb.ClientMhsV0WriteRequest)
		if err := proto.Unmarshal(requestPayload, request); err != nil || rpcapi.ValidateMhsHwdWriteRequest(request) != nil {
			return nil, int32(rpcpb.StatusCode_STATUS_CODE_INVALID_ARGUMENT), "invalid params", nil
		}
		id, hwd = request.Id, request.Hwd
		message, err := rpcapi.ClientHwdWriteRequestFromBytes(hwd, request.Payload)
		if err != nil {
			return nil, 0, "", err
		}
		data, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(message)
		if err != nil {
			return nil, 0, "", err
		}
		if err := json.Unmarshal(data, &args); err != nil {
			return nil, 0, "", err
		}
	}
	meta, err := rpcapi.ClientHwdMetadata(hwd)
	if err != nil {
		return nil, 0, "", err
	}
	instance, code, message := p.mhsValue(id, meta.Name, args)
	if code != 0 {
		return nil, code, message, nil
	}
	var response proto.Message
	if method == rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_READ {
		payload, err := encodePayload(meta.ReadResponse, instance.Value)
		if err != nil {
			return nil, 0, "", err
		}
		response = &rpcpb.ClientMhsV0ReadResponse{Payload: payload, WriteCapabilities: &rpcpb.MhsV0WriteCapabilities{Fields: instance.WriteFields}}
	} else {
		payload, err := encodePayload(meta.WriteResponse, map[string]any{"applied": instance.Value})
		if err != nil {
			return nil, 0, "", err
		}
		response = &rpcpb.ClientMhsV0WriteResponse{Payload: payload}
	}
	payload, err := proto.Marshal(response)
	return payload, 0, "", err
}
