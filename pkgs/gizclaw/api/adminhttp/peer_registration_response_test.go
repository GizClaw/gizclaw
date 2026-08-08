package adminhttp

import (
	"encoding/json"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestPeerRegistrationResponseMarshalJSON(t *testing.T) {
	var result PeerRegistrationResult
	if err := result.FromExternalRef0RegistrationTombstone(apitypes.RegistrationTombstone{
		PublicKey: "peer-key",
		Status:    apitypes.RegistrationTombstoneStatusDeleted,
	}); err != nil {
		t.Fatalf("FromExternalRef0RegistrationTombstone error = %v", err)
	}

	for _, response := range []any{
		DeletePeer200JSONResponse(result),
		GetPeer200JSONResponse(result),
	} {
		data, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("Marshal(%T) error = %v", response, err)
		}
		var got apitypes.RegistrationTombstone
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal(%T) error = %v", response, err)
		}
		if got.PublicKey != "peer-key" || got.Status != apitypes.RegistrationTombstoneStatusDeleted {
			t.Fatalf("Marshal(%T) = %s", response, data)
		}
	}
}
