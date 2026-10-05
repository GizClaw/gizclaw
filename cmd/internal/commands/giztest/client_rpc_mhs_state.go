package giztestcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// hwdFixture implements physical instance behavior over the real SDK protocol.
// Its state is local to one Giztest Peer and cannot affect another document.
type hwdFixture struct {
	Hwd         string         `json:"hwd"`
	Value       map[string]any `json:"value"`
	WriteFields []string       `json:"write_fields"`
}

func installStatefulMhs(handlers *gizcli.DeviceControlHandlers, response any) error {
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	var config struct {
		Instances map[string]hwdFixture `json:"instances"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	for id, instance := range config.Instances {
		if !rpcapi.ValidMhsKey(id) {
			return fmt.Errorf("invalid fixture instance %q", id)
		}
		hwd, err := rpcapi.ClientHwdByName(instance.Hwd)
		if err != nil {
			return err
		}
		message, err := rpcapi.ClientHwdReadResponseMessage(hwd, nil)
		if err != nil {
			return err
		}
		value, err := json.Marshal(instance.Value)
		if err != nil {
			return err
		}
		if err := protojson.Unmarshal(value, message); err != nil {
			return err
		}
	}
	var mu sync.Mutex
	handlers.MhsCapabilities = func(ctx context.Context) ([]*rpcpb.MhsV0InstanceCapability, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		mu.Lock()
		instances := maps.Clone(config.Instances)
		mu.Unlock()
		ids := []string{}
		for id := range instances {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		result := make([]*rpcpb.MhsV0InstanceCapability, 0, len(ids))
		for _, id := range ids {
			instance := instances[id]
			hwd, err := rpcapi.ClientHwdByName(instance.Hwd)
			if err != nil {
				return nil, err
			}
			result = append(result, &rpcpb.MhsV0InstanceCapability{Id: id, Hwd: hwd, WriteFields: slices.Clone(instance.WriteFields)})
		}
		return result, nil
	}
	get := func(id string, hwd rpcpb.ClientHwd) (hwdFixture, error) {
		instance, ok := config.Instances[id]
		meta, err := rpcapi.ClientHwdMetadata(hwd)
		if err != nil || !ok || meta.Name != instance.Hwd {
			return hwdFixture{}, rpcapi.Error{Code: rpcapi.StatusCodeNotFound, Message: "instance not installed"}
		}
		return instance, nil
	}
	handlers.ReadMhsHwd = func(ctx context.Context, request *rpcpb.ClientMhsV0ReadRequest) (*rpcpb.ClientMhsV0ReadResponse, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		mu.Lock()
		instance, err := get(request.Id, request.Hwd)
		value := maps.Clone(instance.Value)
		fields := slices.Clone(instance.WriteFields)
		mu.Unlock()
		if err != nil {
			return nil, err
		}
		message, err := rpcapi.ClientHwdReadResponseMessage(request.Hwd, nil)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		if err := protojson.Unmarshal(data, message); err != nil {
			return nil, err
		}
		payload, err := proto.Marshal(message)
		if err != nil {
			return nil, err
		}
		return &rpcpb.ClientMhsV0ReadResponse{Payload: payload, WriteCapabilities: &rpcpb.MhsV0WriteCapabilities{Fields: fields}}, nil
	}
	handlers.WriteMhsHwd = func(ctx context.Context, request *rpcpb.ClientMhsV0WriteRequest) (*rpcpb.ClientMhsV0WriteResponse, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		message, err := rpcapi.ClientHwdWriteRequestFromBytes(request.Hwd, request.Payload)
		if err != nil {
			return nil, err
		}
		data, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(message)
		if err != nil {
			return nil, err
		}
		var args map[string]any
		if err := json.Unmarshal(data, &args); err != nil {
			return nil, err
		}
		mu.Lock()
		instance, err := get(request.Id, request.Hwd)
		if err != nil {
			mu.Unlock()
			return nil, err
		}
		for name := range args {
			if !slices.Contains(instance.WriteFields, name) {
				mu.Unlock()
				return nil, rpcapi.Error{Code: rpcapi.StatusCodeUnimplemented, Message: "write field not implemented"}
			}
		}
		maps.Copy(instance.Value, args)
		config.Instances[request.Id] = instance
		applied := maps.Clone(instance.Value)
		mu.Unlock()
		response, err := rpcapi.ClientHwdWriteResponseMessage(request.Hwd, nil)
		if err != nil {
			return nil, err
		}
		data, err = json.Marshal(map[string]any{"applied": applied})
		if err != nil {
			return nil, err
		}
		if err := protojson.Unmarshal(data, response); err != nil {
			return nil, err
		}
		payload, err := proto.Marshal(response)
		if err != nil {
			return nil, err
		}
		return &rpcpb.ClientMhsV0WriteResponse{Payload: payload}, nil
	}
	return nil
}
