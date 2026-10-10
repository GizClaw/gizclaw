package rpcapi

import (
	"fmt"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

// ValidateGNSSReportingResponse requires the device to return its switch value,
// including an explicit false rather than an empty acknowledgement.
func ValidateGNSSReportingResponse(message proto.Message) error {
	switch response := message.(type) {
	case *rpcpb.ClientGnssReportingGetResponse:
		if response != nil && response.Enabled != nil {
			return nil
		}
	case *rpcpb.ClientGnssReportingSetResponse:
		if response != nil && response.Enabled != nil {
			return nil
		}
	}
	return fmt.Errorf("rpc: gnss reporting result requires enabled")
}

// AsClientGnssReportingGetRequest decodes the protobuf payload.
func (t RPCPayload) AsClientGnssReportingGetRequest() (*rpcpb.ClientGnssReportingGetRequest, error) {
	body := new(rpcpb.ClientGnssReportingGetRequest)
	err := t.decode("ClientGnssReportingGetRequest", body)
	return body, err
}

// FromClientGnssReportingGetRequest encodes the protobuf payload.
func (t *RPCPayload) FromClientGnssReportingGetRequest(v *rpcpb.ClientGnssReportingGetRequest) error {
	return t.encode("ClientGnssReportingGetRequest", v)
}

// AsClientGnssReportingGetResponse decodes the protobuf payload.
func (t RPCPayload) AsClientGnssReportingGetResponse() (*rpcpb.ClientGnssReportingGetResponse, error) {
	body := new(rpcpb.ClientGnssReportingGetResponse)
	err := t.decode("ClientGnssReportingGetResponse", body)
	return body, err
}

// FromClientGnssReportingGetResponse encodes the protobuf payload.
func (t *RPCPayload) FromClientGnssReportingGetResponse(v *rpcpb.ClientGnssReportingGetResponse) error {
	return t.encode("ClientGnssReportingGetResponse", v)
}

// AsClientGnssReportingSetRequest decodes the protobuf payload.
func (t RPCPayload) AsClientGnssReportingSetRequest() (*rpcpb.ClientGnssReportingSetRequest, error) {
	body := new(rpcpb.ClientGnssReportingSetRequest)
	err := t.decode("ClientGnssReportingSetRequest", body)
	return body, err
}

// FromClientGnssReportingSetRequest encodes the protobuf payload.
func (t *RPCPayload) FromClientGnssReportingSetRequest(v *rpcpb.ClientGnssReportingSetRequest) error {
	return t.encode("ClientGnssReportingSetRequest", v)
}

// AsClientGnssReportingSetResponse decodes the protobuf payload.
func (t RPCPayload) AsClientGnssReportingSetResponse() (*rpcpb.ClientGnssReportingSetResponse, error) {
	body := new(rpcpb.ClientGnssReportingSetResponse)
	err := t.decode("ClientGnssReportingSetResponse", body)
	return body, err
}

// FromClientGnssReportingSetResponse encodes the protobuf payload.
func (t *RPCPayload) FromClientGnssReportingSetResponse(v *rpcpb.ClientGnssReportingSetResponse) error {
	return t.encode("ClientGnssReportingSetResponse", v)
}
