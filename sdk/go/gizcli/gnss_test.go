package gizcli

import (
	"context"
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestGNSSReportingProviders(t *testing.T) {
	for _, generic := range []bool{false, true} {
		t.Run(map[bool]string{false: "device-control", true: "generic-tool"}[generic], func(t *testing.T) {
			device := &Client{}
			enabled := true // The provider supplies the initial value.
			sets := 0
			get := func(context.Context, *rpcpb.ClientGnssReportingGetRequest) (*rpcpb.ClientGnssReportingGetResponse, error) {
				return &rpcpb.ClientGnssReportingGetResponse{Enabled: new(enabled)}, nil
			}
			set := func(_ context.Context, request *rpcpb.ClientGnssReportingSetRequest) (*rpcpb.ClientGnssReportingSetResponse, error) {
				sets++
				enabled = request.GetEnabled()
				return &rpcpb.ClientGnssReportingSetResponse{Enabled: new(enabled)}, nil
			}
			if generic {
				if err := device.HandleClientTool(rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_GET, func(ctx context.Context, request proto.Message) (proto.Message, error) {
					return get(ctx, request.(*rpcpb.ClientGnssReportingGetRequest))
				}); err != nil {
					t.Fatal(err)
				}
				if err := device.HandleClientTool(rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_SET, func(ctx context.Context, request proto.Message) (proto.Message, error) {
					return set(ctx, request.(*rpcpb.ClientGnssReportingSetRequest))
				}); err != nil {
					t.Fatal(err)
				}
			} else if err := device.HandleDeviceControl(DeviceControlHandlers{GNSSReportingGet: get, GNSSReportingSet: set}); err != nil {
				t.Fatal(err)
			}
			list := deviceControlDispatch(t, device, rpcapi.RPCMethodClientToolV0List, nil)
			tools, err := list.Result.AsClientToolV0ListResponse()
			if err != nil || !slices.Contains(tools.Tools, rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_GET) || !slices.Contains(tools.Tools, rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_SET) {
				t.Fatalf("discovery = %v, %v", tools, err)
			}
			for _, value := range []bool{false, false, true} {
				response := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_SET, func(p *rpcapi.RPCPayload) error {
					return p.FromClientGnssReportingSetRequest(&rpcpb.ClientGnssReportingSetRequest{Enabled: new(value)})
				})
				if response.Error != nil {
					t.Fatal(response.Error)
				}
				applied, err := response.Result.AsClientGnssReportingSetResponse()
				if err != nil || applied.Enabled == nil || applied.GetEnabled() != value {
					t.Fatalf("set = %v, %v", applied, err)
				}
				response = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_GET, nil)
				if response.Error != nil {
					t.Fatal(response.Error)
				}
				current, err := response.Result.AsClientGnssReportingGetResponse()
				if err != nil || current.Enabled == nil || current.GetEnabled() != value {
					t.Fatalf("get = %v, %v", current, err)
				}
			}
			invalid := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_SET, nil)
			if invalid.Error == nil || invalid.Error.Code != rpcapi.StatusCodeInvalidArgument || sets != 3 {
				t.Fatalf("missing enabled = %v, sets=%d", invalid, sets)
			}
		})
	}
}

func TestGNSSReportingMissingAndInvalidProvider(t *testing.T) {
	device := &Client{}
	response := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_GET, nil)
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatalf("missing provider = %v", response)
	}
	if err := device.HandleDeviceControl(DeviceControlHandlers{GNSSReportingGet: func(context.Context, *rpcpb.ClientGnssReportingGetRequest) (*rpcpb.ClientGnssReportingGetResponse, error) {
		return &rpcpb.ClientGnssReportingGetResponse{}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	response = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_GET, nil)
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInternal {
		t.Fatalf("empty provider result = %v", response)
	}
}
