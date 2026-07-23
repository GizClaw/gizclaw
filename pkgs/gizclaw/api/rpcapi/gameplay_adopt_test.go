package rpcapi

import "testing"

func TestRuntimeAdoptRequestPreservesCallerID(t *testing.T) {
	id := "device-pet-01"
	displayName := "Miso"
	var payload RPCPayload
	if err := payload.FromRuntimeAdoptRequest(RuntimeAdoptRequest{Id: &id, DisplayName: displayName}); err != nil {
		t.Fatalf("FromRuntimeAdoptRequest() error = %v", err)
	}
	got, err := payload.AsRuntimeAdoptRequest()
	if err != nil {
		t.Fatalf("AsRuntimeAdoptRequest() error = %v", err)
	}
	if got.Id == nil || *got.Id != id || got.DisplayName != displayName {
		t.Fatalf("AsRuntimeAdoptRequest() = %#v", got)
	}
}
