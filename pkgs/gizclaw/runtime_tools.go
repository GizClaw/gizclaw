package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"reflect"
	"slices"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/mhs"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolcatalog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// runtimeDevices borrows the same serialized controller used by device control.
// It never issues HTTP requests or changes the Peer selected by owner identity.
type runtimeDevices struct{ public *peerHTTP }

func (d *runtimeDevices) inspectFailure(failure *deviceControlError) toolcatalog.Availability {
	return toolcatalog.Availability{Online: failure.Code != deviceOfflineCode, Reason: failure.Code}
}

// Snapshot holds one resolver operation's observed capability registries. It
// batches introspection by protocol family and performs no work at construction.
func (d *runtimeDevices) Snapshot(ctx context.Context, owner string, profile apitypes.RuntimeProfile) (toolcatalog.Devices, error) {
	var key giznet.PublicKey
	if err := key.UnmarshalText([]byte(owner)); err != nil || key.IsZero() {
		return nil, errors.New("invalid Tool owner")
	}
	return &runtimeDeviceSnapshot{devices: d, owner: key}, nil
}

func (d *runtimeDevices) Inspect(ctx context.Context, owner string, profile apitypes.RuntimeProfile, tool toolcatalog.Tool) (toolcatalog.Availability, error) {
	snapshot, err := d.Snapshot(ctx, owner, profile)
	if err != nil {
		return toolcatalog.Availability{}, err
	}
	return snapshot.Inspect(ctx, owner, profile, tool)
}

type runtimeDeviceSnapshot struct {
	devices          *runtimeDevices
	owner            giznet.PublicKey
	methods          *rpcpb.ClientRpcMethodsListResponse
	procedures       *rpcpb.ClientToolV0ListResponse
	methodsFailure   *deviceControlError
	procedureFailure *deviceControlError
	methodsRead      bool
	proceduresRead   bool
}

func (s *runtimeDeviceSnapshot) Invoke(ctx context.Context, owner string, profile apitypes.RuntimeProfile, tool toolcatalog.Tool, args json.RawMessage) (json.RawMessage, error) {
	return s.devices.Invoke(ctx, owner, profile, tool, args)
}

func (s *runtimeDeviceSnapshot) Inspect(ctx context.Context, owner string, profile apitypes.RuntimeProfile, tool toolcatalog.Tool) (toolcatalog.Availability, error) {
	if s.owner.String() != owner {
		return toolcatalog.Availability{}, errors.New("capability owner mismatch")
	}
	if s.devices == nil || s.devices.public == nil {
		return toolcatalog.Availability{Reason: "capability_unknown"}, nil
	}
	control := s.devices.public.DeviceControl
	if tool.Source == "client_tool" {
		if !s.proceduresRead {
			s.proceduresRead = true
			s.procedures, s.procedureFailure = callDeviceControl(ctx, control, s.owner, deviceControlOptions{}, func(ctx context.Context, _ *rpcClient, conn net.Conn) (*rpcpb.ClientToolV0ListResponse, error) {
				params, err := newRPCRequestParams(&rpcpb.ClientToolV0ListRequest{}, (*rpcapi.RPCPayload).FromClientToolV0ListRequest)
				if err != nil {
					return nil, err
				}
				result, err := callRPCResult(ctx, conn, newRPCRequest("runtime.catalog.tools", rpcapi.RPCMethodClientToolV0List, params), rpcapi.RPCPayload.AsClientToolV0ListResponse)
				if err != nil {
					return nil, err
				}
				return *result, nil
			}, nil)
		}
		if s.procedureFailure != nil {
			return s.devices.inspectFailure(s.procedureFailure), nil
		}
		selected, err := rpcapi.ClientToolByName(tool.Binding.ClientTool.Name)
		if err != nil {
			return toolcatalog.Availability{}, err
		}
		if !slices.Contains(s.procedures.Tools, selected) {
			return toolcatalog.Availability{Online: true, Reason: "procedure_unsupported"}, nil
		}
		return toolcatalog.Availability{Online: true, Supported: true}, nil
	}
	if !s.methodsRead {
		s.methodsRead = true
		s.methods, s.methodsFailure = callDeviceControl(ctx, control, s.owner, deviceControlOptions{}, func(ctx context.Context, _ *rpcClient, conn net.Conn) (*rpcpb.ClientRpcMethodsListResponse, error) {
			params, err := newRPCRequestParams(&rpcpb.ClientRpcMethodsListRequest{}, (*rpcapi.RPCPayload).FromClientRpcMethodsListRequest)
			if err != nil {
				return nil, err
			}
			result, err := callRPCResult(ctx, conn, newRPCRequest("runtime.catalog.methods", rpcapi.RPCMethodClientRPCMethodsList, params), rpcapi.RPCPayload.AsClientRpcMethodsListResponse)
			if err != nil {
				return nil, err
			}
			return *result, nil
		}, nil)
	}
	if s.methodsFailure != nil {
		return s.devices.inspectFailure(s.methodsFailure), nil
	}
	method := rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_READ
	if tool.Binding.Mhs.Operation == "write" {
		method = rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_WRITE
	}
	if !slices.Contains(s.methods.Methods, method) {
		return toolcatalog.Availability{Online: true, Reason: "method_unsupported"}, nil
	}
	id, _ := tool.Target["id"].(string)
	hwd, _ := tool.Target["hwd"].(string)
	var capability *rpcpb.MhsV0InstanceCapability
	for _, instance := range s.methods.MhsV0 {
		if instance == nil || instance.Id != id {
			continue
		}
		if capability != nil {
			return toolcatalog.Availability{Online: true, Reason: "invalid_device_capabilities"}, nil
		}
		capability = instance
	}
	if capability == nil {
		return toolcatalog.Availability{Online: true, Reason: "instance_capability_unknown"}, nil
	}
	meta, err := rpcapi.ClientHwdMetadata(capability.Hwd)
	if err != nil || meta.Name != hwd {
		return toolcatalog.Availability{Online: true, Reason: "instance_unsupported"}, nil
	}
	if tool.Binding.Mhs.Operation == "write" {
		for _, field := range *tool.Binding.Mhs.Fields {
			if !slices.Contains(capability.WriteFields, field) {
				return toolcatalog.Availability{Online: true, Reason: "write_field_unsupported"}, nil
			}
		}
	}
	return toolcatalog.Availability{Online: true, Supported: true}, nil
}

func (d *runtimeDevices) Invoke(ctx context.Context, owner string, profile apitypes.RuntimeProfile, tool toolcatalog.Tool, args json.RawMessage) (json.RawMessage, error) {
	var key giznet.PublicKey
	if err := key.UnmarshalText([]byte(owner)); err != nil || key.IsZero() {
		return nil, errors.New("invalid Tool owner")
	}
	if d == nil || d.public == nil || d.public.DeviceControl == nil {
		return nil, errors.New("device controller unavailable")
	}
	authorize := func(ctx context.Context) *deviceControlError {
		current, err := d.public.DeviceControl.manager.runtimeProfileForOwner(ctx, owner)
		if err != nil || current.Spec.Resources.Tools == nil {
			return &deviceControlError{Status: http.StatusForbidden, Code: "TOOL_UNAVAILABLE", Message: "Tool authorization changed"}
		}
		binding, ok := (*current.Spec.Resources.Tools)[tool.Alias]
		if !ok || current.Revision != profile.Revision || !reflect.DeepEqual(binding, tool.Binding) {
			return &deviceControlError{Status: http.StatusForbidden, Code: "TOOL_UNAVAILABLE", Message: "Tool binding changed"}
		}
		if tool.Authorize != nil {
			if err := tool.Authorize(ctx); err != nil {
				return &deviceControlError{Status: http.StatusForbidden, Code: "TOOL_UNAVAILABLE", Message: "Tool permissions changed"}
			}
		}
		return nil
	}
	if tool.Source == "client_tool" {
		var values map[string]any
		decoder := json.NewDecoder(bytes.NewReader(args))
		decoder.UseNumber()
		if err := decoder.Decode(&values); err != nil {
			return nil, err
		}
		result, failure := d.public.invokeClientToolForOwner(ctx, key, tool.Binding.ClientTool.Name, values, authorize)
		if failure != nil {
			return nil, &toolcatalog.InvocationError{Code: failure.Code}
		}
		return json.Marshal(result)
	}
	id, _ := tool.Target["id"].(string)
	hwd, _ := tool.Target["hwd"].(string)
	if tool.Binding.Mhs.Operation == "read" {
		params, err := mhs.ReadRequest(*profile.Spec.Mhs.V0, apitypes.MhsV0ReadRequest{Id: id, Hwd: apitypes.MhsV0ReadRequestHwd(hwd)})
		if err != nil {
			return nil, err
		}
		result, failure := callDeviceControl(ctx, d.public.DeviceControl, key, deviceControlOptions{notFoundCode: mhsHwdNotFoundCode, authorize: authorize}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientMhsV0ReadResponse, error) {
			return client.ReadMhsHwd(ctx, conn, "runtime.mhs.read", params)
		}, nil)
		if failure != nil {
			return nil, &toolcatalog.InvocationError{Code: failure.Code}
		}
		value, err := mhs.ReadResponse(params, result)
		if err != nil {
			return nil, err
		}
		return value.MarshalJSON()
	}
	data, err := json.Marshal(map[string]any{"id": id, "hwd": hwd, "value": json.RawMessage(args)})
	if err != nil {
		return nil, err
	}
	var request apitypes.MhsV0WriteRequest
	if err := request.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	params, err := mhs.WriteRequest(*profile.Spec.Mhs.V0, request)
	if err != nil {
		return nil, err
	}
	result, failure := callDeviceControl(ctx, d.public.DeviceControl, key, deviceControlOptions{notFoundCode: mhsHwdNotFoundCode, authorize: authorize}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientMhsV0WriteResponse, error) {
		return client.WriteMhsHwd(ctx, conn, "runtime.mhs.write", params)
	}, nil)
	if failure != nil {
		return nil, &toolcatalog.InvocationError{Code: failure.Code}
	}
	value, err := mhs.WriteResponse(params, result)
	if err != nil {
		return nil, err
	}
	return value.MarshalJSON()
}
