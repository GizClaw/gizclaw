package rpcapi

import "testing"

func TestAPIKeyCreateMethodAndPayloadRoundTrip(t *testing.T) {
	protoMethod, err := ProtoMethod(RPCMethodServerAPIKeyCreate)
	if err != nil {
		t.Fatalf("ProtoMethod() error = %v", err)
	}
	if protoMethod != 96 {
		t.Fatalf("ProtoMethod() = %d, want 96", protoMethod)
	}
	method, err := MethodFromProto(protoMethod)
	if err != nil || method != RPCMethodServerAPIKeyCreate {
		t.Fatalf("MethodFromProto() = %q, %v", method, err)
	}

	request := APIKeyCreateRequest{DisplayName: "phone", ManageAPIKeys: true}
	var requestPayload RPCPayload
	if err := requestPayload.FromAPIKeyCreateRequest(request); err != nil {
		t.Fatalf("FromAPIKeyCreateRequest() error = %v", err)
	}
	gotRequest, err := requestPayload.AsAPIKeyCreateRequest()
	if err != nil {
		t.Fatalf("AsAPIKeyCreateRequest() error = %v", err)
	}
	if gotRequest != request {
		t.Fatalf("AsAPIKeyCreateRequest() = %#v, want %#v", gotRequest, request)
	}

	response := APIKeyCreateResponse{
		Value: &APIKey{
			Name:          "key_0123456789012345678901",
			DisplayName:   "phone",
			Prefix:        "gizclaw_sk_v1_01234567…",
			ManageAPIKeys: true,
			CreatedAt:     "2026-08-19T00:00:00Z",
		},
		APIKey: "gizclaw_sk_v1_0123456789012345678901234567890123456789012",
	}
	var responsePayload RPCPayload
	if err := responsePayload.FromAPIKeyCreateResponse(response); err != nil {
		t.Fatalf("FromAPIKeyCreateResponse() error = %v", err)
	}
	gotResponse, err := responsePayload.AsAPIKeyCreateResponse()
	if err != nil {
		t.Fatalf("AsAPIKeyCreateResponse() error = %v", err)
	}
	if gotResponse.APIKey != response.APIKey || gotResponse.Value == nil || gotResponse.Value.Name != response.Value.Name || !gotResponse.Value.ManageAPIKeys {
		t.Fatal("AsAPIKeyCreateResponse() did not preserve metadata and secret")
	}
}
