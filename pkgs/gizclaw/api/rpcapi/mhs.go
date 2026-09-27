package rpcapi

import (
	"regexp"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

const MaxMhsKeyBytes = 64

var mhsKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*([.-][a-z0-9]+)*$`)

// ValidMhsKey checks the bounded, case-sensitive HWD instance ID syntax.
func ValidMhsKey(key string) bool {
	return len(key) <= MaxMhsKeyBytes && mhsKeyPattern.MatchString(key)
}

// AsClientMhsV0ReadRequest decodes the original protobuf message, preserving oneof presence.
func (t RPCPayload) AsClientMhsV0ReadRequest() (*rpcpb.ClientMhsV0ReadRequest, error) {
	body := new(rpcpb.ClientMhsV0ReadRequest)
	err := t.decode("ClientMhsV0ReadRequest", body)
	return body, err
}

// FromClientMhsV0ReadRequest encodes the original protobuf message.
func (t *RPCPayload) FromClientMhsV0ReadRequest(v *rpcpb.ClientMhsV0ReadRequest) error {
	return t.encode("ClientMhsV0ReadRequest", v)
}

// AsClientMhsV0ReadResponse decodes the original protobuf message, preserving oneof presence.
func (t RPCPayload) AsClientMhsV0ReadResponse() (*rpcpb.ClientMhsV0ReadResponse, error) {
	body := new(rpcpb.ClientMhsV0ReadResponse)
	err := t.decode("ClientMhsV0ReadResponse", body)
	return body, err
}

// FromClientMhsV0ReadResponse encodes the original protobuf message.
func (t *RPCPayload) FromClientMhsV0ReadResponse(v *rpcpb.ClientMhsV0ReadResponse) error {
	return t.encode("ClientMhsV0ReadResponse", v)
}

// AsClientMhsV0WriteRequest decodes the original protobuf message, preserving oneof presence.
func (t RPCPayload) AsClientMhsV0WriteRequest() (*rpcpb.ClientMhsV0WriteRequest, error) {
	body := new(rpcpb.ClientMhsV0WriteRequest)
	err := t.decode("ClientMhsV0WriteRequest", body)
	return body, err
}

// FromClientMhsV0WriteRequest encodes the original protobuf message.
func (t *RPCPayload) FromClientMhsV0WriteRequest(v *rpcpb.ClientMhsV0WriteRequest) error {
	return t.encode("ClientMhsV0WriteRequest", v)
}

// AsClientMhsV0WriteResponse decodes the original protobuf message, preserving oneof presence.
func (t RPCPayload) AsClientMhsV0WriteResponse() (*rpcpb.ClientMhsV0WriteResponse, error) {
	body := new(rpcpb.ClientMhsV0WriteResponse)
	err := t.decode("ClientMhsV0WriteResponse", body)
	return body, err
}

// FromClientMhsV0WriteResponse encodes the original protobuf message.
func (t *RPCPayload) FromClientMhsV0WriteResponse(v *rpcpb.ClientMhsV0WriteResponse) error {
	return t.encode("ClientMhsV0WriteResponse", v)
}
