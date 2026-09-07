package rpcapi

import (
	"strings"
	"testing"
)

func TestAppConfigPayloadRoundTripAndMethodRegistry(t *testing.T) {
	t.Parallel()
	for _, method := range []RPCMethod{RPCMethodServerAppConfigList, RPCMethodServerAppConfigGet} {
		protoMethod, err := ProtoMethod(method)
		if err != nil {
			t.Fatalf("ProtoMethod(%s) error = %v", method, err)
		}
		if got, err := MethodFromProto(protoMethod); err != nil || got != method {
			t.Fatalf("MethodFromProto(%s) = %q, %v", method, got, err)
		}
	}

	cursor := "cmV2aXNpb24AY2hhdA"
	list := AppConfigListResponse{
		Keys:                   []string{"ui.theme", "voice.wakeword"},
		HasNext:                true,
		NextCursor:             &cursor,
		RuntimeProfileName:     "default",
		RuntimeProfileRevision: "revision",
	}
	var payload RPCPayload
	if err := payload.FromAppConfigListResponse(list); err != nil {
		t.Fatalf("FromAppConfigListResponse() error = %v", err)
	}
	decodedList, err := payload.AsAppConfigListResponse()
	if err != nil {
		t.Fatalf("AsAppConfigListResponse() error = %v", err)
	}
	if len(decodedList.Keys) != 2 || decodedList.Keys[0] != "ui.theme" || !decodedList.HasNext {
		t.Fatalf("AppConfigListResponse round trip = %#v", decodedList)
	}
	if decodedList.NextCursor == nil || *decodedList.NextCursor != cursor {
		t.Fatalf("next_cursor round trip = %v", decodedList.NextCursor)
	}
	if decodedList.RuntimeProfileName != "default" || decodedList.RuntimeProfileRevision != "revision" {
		t.Fatalf("profile identity round trip = %#v", decodedList)
	}

	// The value is opaque: newlines, whitespace and non-ASCII survive verbatim.
	value := "{\n  \"theme\": \"深色\",\n  \"rows\": [1, 2]\n}\n" + strings.Repeat("x", 64)
	if err := payload.FromAppConfigGetResponse(AppConfigGetResponse{
		Value: value, RuntimeProfileName: "default", RuntimeProfileRevision: "revision",
	}); err != nil {
		t.Fatalf("FromAppConfigGetResponse() error = %v", err)
	}
	decodedGet, err := payload.AsAppConfigGetResponse()
	if err != nil {
		t.Fatalf("AsAppConfigGetResponse() error = %v", err)
	}
	if decodedGet.Value != value {
		t.Fatalf("value round trip = %q, want %q", decodedGet.Value, value)
	}

	if err := payload.FromAppConfigGetRequest(AppConfigGetRequest{Key: "ui.theme"}); err != nil {
		t.Fatalf("FromAppConfigGetRequest() error = %v", err)
	}
	decodedRequest, err := payload.AsAppConfigGetRequest()
	if err != nil || decodedRequest.Key != "ui.theme" {
		t.Fatalf("AsAppConfigGetRequest() = %#v, %v", decodedRequest, err)
	}
}
