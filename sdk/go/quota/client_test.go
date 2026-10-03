package quota

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestDecisionNullableExpiryAndRequiredValidity(t *testing.T) {
	for _, body := range []string{`{"valid_until":"2030-01-01T00:00:00Z"}`, `{"expires_at":null,"valid_until":"2030-01-01T00:00:00Z"}`} {
		var value QuotaResponse
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			t.Fatal(err)
		}
		if value.ExpiresAt != nil || value.ValidUntil.IsZero() {
			t.Fatalf("decision=%+v", value)
		}
	}
	var value QuotaResponse
	if err := json.Unmarshal([]byte(`{"expires_at":"2000-01-01T00:00:00Z"}`), &value); err != nil {
		t.Fatal(err)
	}
	if !value.ValidUntil.IsZero() || value.ExpiresAt == nil {
		t.Fatalf("decision=%+v", value)
	}
}
func TestRequestPreservesSharedIdentifiersAndHourlySnapshot(t *testing.T) {
	request := QuotaRequest{PeerPublicKey: "peer", Identifiers: &apitypes.DeviceIdentifiers{Sn: new("sn"), Imeis: new([]apitypes.PeerIMEI{{Tac: "12345678", Serial: "123456"}}), Labels: new([]apitypes.PeerLabel{{Key: "board", Value: "test"}})}, Usage: []QuotaUsage{{ModelId: "billing", Hour: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), Quantity: 37}}}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var value QuotaRequest
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value.Identifiers == nil || *value.Identifiers.Sn != "sn" || len(*value.Identifiers.Imeis) != 1 || len(*value.Identifiers.Labels) != 1 || len(value.Usage) != 1 || value.Usage[0].Quantity != 37 {
		t.Fatalf("request=%+v", value)
	}
}
