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
			APIKey:        "gizclaw_sk_v1_0123456789012345678901234567890123456789012",
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
	if gotResponse.APIKey != response.APIKey || gotResponse.Value == nil || gotResponse.Value.Name != response.Value.Name || gotResponse.Value.APIKey != response.Value.APIKey || !gotResponse.Value.ManageAPIKeys {
		t.Fatal("AsAPIKeyCreateResponse() did not preserve metadata and secret")
	}
}

func TestAPIKeyManagementMethodsAndPayloadRoundTrip(t *testing.T) {
	for method, want := range map[RPCMethod]int32{
		RPCMethodServerAPIKeyList:   97,
		RPCMethodServerAPIKeyRevoke: 98,
	} {
		got, err := ProtoMethod(method)
		if err != nil || int32(got) != want {
			t.Fatalf("ProtoMethod(%q) = %d, %v; want %d", method, got, err, want)
		}
	}

	listRequest := APIKeyListRequest{Cursor: "key_0123456789012345678901", Limit: 12}
	var listRequestPayload RPCPayload
	if err := listRequestPayload.FromAPIKeyListRequest(listRequest); err != nil {
		t.Fatal(err)
	}
	gotListRequest, err := listRequestPayload.AsAPIKeyListRequest()
	if err != nil || gotListRequest != listRequest {
		t.Fatalf("list request round trip = %#v, %v", gotListRequest, err)
	}

	listResponse := APIKeyListResponse{
		Items:      []APIKey{{Name: "key_0123456789012345678901", DisplayName: "phone", Prefix: "gizclaw_sk_v1_01234567…", APIKey: "gizclaw_sk_v1_0123456789012345678901234567890123456789012"}},
		NextCursor: "key_1234567890123456789012",
	}
	var listResponsePayload RPCPayload
	if err := listResponsePayload.FromAPIKeyListResponse(listResponse); err != nil {
		t.Fatal(err)
	}
	gotListResponse, err := listResponsePayload.AsAPIKeyListResponse()
	if err != nil || len(gotListResponse.Items) != 1 || gotListResponse.Items[0].Name != listResponse.Items[0].Name || gotListResponse.Items[0].APIKey != listResponse.Items[0].APIKey || gotListResponse.NextCursor != listResponse.NextCursor {
		t.Fatalf("list response round trip = %#v, %v", gotListResponse, err)
	}

	revokeRequest := APIKeyRevokeRequest{Name: "key_0123456789012345678901"}
	var revokeRequestPayload RPCPayload
	if err := revokeRequestPayload.FromAPIKeyRevokeRequest(revokeRequest); err != nil {
		t.Fatal(err)
	}
	gotRevokeRequest, err := revokeRequestPayload.AsAPIKeyRevokeRequest()
	if err != nil || gotRevokeRequest != revokeRequest {
		t.Fatalf("revoke request round trip = %#v, %v", gotRevokeRequest, err)
	}
}
