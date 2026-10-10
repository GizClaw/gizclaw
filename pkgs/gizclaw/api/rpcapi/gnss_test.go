package rpcapi

import (
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestGNSSReportingBoolPresence(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		request, err := ClientToolRequestMessage(rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_SET, map[string]any{"enabled": enabled})
		if err != nil || ValidateClientToolRequest(request) != nil {
			t.Fatalf("request enabled=%v: %v", enabled, err)
		}
		wire, err := proto.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := ClientToolRequestFromBytes(rpcpb.ClientTool_CLIENT_TOOL_GNSS_REPORTING_SET, wire)
		if err != nil || !proto.Equal(request, decoded) {
			t.Fatalf("request round trip = %v, %v", decoded, err)
		}
		for _, response := range []proto.Message{
			&rpcpb.ClientGnssReportingGetResponse{Enabled: new(enabled)},
			&rpcpb.ClientGnssReportingSetResponse{Enabled: new(enabled)},
		} {
			if err := ValidateGNSSReportingResponse(response); err != nil {
				t.Fatal(err)
			}
			result, err := ClientToolResultJSON(response)
			if err != nil || result["enabled"] != enabled {
				t.Fatalf("result = %v, %v", result, err)
			}
		}
	}
	if ValidateClientToolRequest(&rpcpb.ClientGnssReportingSetRequest{}) == nil {
		t.Fatal("accepted missing enabled")
	}
	for _, response := range []proto.Message{nil, &rpcpb.ClientGnssReportingGetResponse{}, &rpcpb.ClientGnssReportingSetResponse{}} {
		if ValidateGNSSReportingResponse(response) == nil {
			t.Fatalf("accepted missing enabled in %T", response)
		}
	}
}
