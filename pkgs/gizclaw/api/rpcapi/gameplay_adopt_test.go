package rpcapi

import "testing"

func TestRuntimeAdoptRequestPreservesPeerScopedName(t *testing.T) {
	name := "device-pet-01"
	displayName := "Miso"
	var payload RPCPayload
	if err := payload.FromRuntimeAdoptRequest(RuntimeAdoptRequest{Name: name, DisplayName: displayName}); err != nil {
		t.Fatalf("FromRuntimeAdoptRequest() error = %v", err)
	}
	got, err := payload.AsRuntimeAdoptRequest()
	if err != nil {
		t.Fatalf("AsRuntimeAdoptRequest() error = %v", err)
	}
	if got.Name != name || got.DisplayName != displayName {
		t.Fatalf("AsRuntimeAdoptRequest() = %#v", got)
	}
}
