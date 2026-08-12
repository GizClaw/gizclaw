package giztunnel

import (
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func testDeclaration(t *testing.T) SessionDeclaration {
	t.Helper()
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	id, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	return SessionDeclaration{SessionID: id, ClientPublicKey: key.Public, RemoteAddr: "[2001:db8::1]:4242"}
}

func TestTunnelLabelsRoundTripCanonically(t *testing.T) {
	declaration := testDeclaration(t)
	control, err := controlLabel(declaration)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseLabel(control)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.kind != labelControl || parsed.session != declaration.SessionID ||
		!parsed.client.Equal(declaration.ClientPublicKey) || parsed.remote != declaration.RemoteAddr {
		t.Fatalf("parsed control = %+v", parsed)
	}
	packet, err := packetLabel(declaration.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parseLabel(packet)
	if err != nil || parsed.kind != labelPacket {
		t.Fatalf("parsed packet = %+v, %v", parsed, err)
	}
	service, err := serviceLabel(declaration.SessionID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parseLabel(service)
	if err != nil || parsed.kind != labelService || parsed.service != 0 || parsed.channelID != 1 {
		t.Fatalf("parsed service = %+v, %v", parsed, err)
	}
}

func TestTunnelLabelsRejectNonCanonicalOrMalformedValues(t *testing.T) {
	declaration := testDeclaration(t)
	base := LabelPrefix + declaration.SessionID.String()
	for _, label := range []string{
		"", "giznet/v1/tunnel/" + declaration.SessionID.String() + "/packet",
		LabelPrefix + strings.ToUpper(declaration.SessionID.String()) + "/packet",
		base + "/service/01/1", base + "/service/1/00", base + "/service/1/0",
		base + "/packet/extra", base + "/control/no-key/-", strings.Repeat("x", maxLabelSize+1),
	} {
		if _, err := parseLabel(label); !errors.Is(err, ErrInvalidFrame) {
			t.Errorf("parseLabel(%q) error = %v", label, err)
		}
	}
}

func TestSessionResultRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		status sessionResultStatus
		reason string
	}{
		{status: sessionAccepted},
		{status: sessionRejected, reason: "service admission denied"},
	} {
		encoded, err := encodeSessionResult(tc.status, tc.reason)
		if err != nil {
			t.Fatal(err)
		}
		status, reason, err := decodeSessionResult(encoded)
		if err != nil || status != tc.status || reason != tc.reason {
			t.Fatalf("decode result = %d %q %v", status, reason, err)
		}
	}
}

func TestOpusPacketRoundTrip(t *testing.T) {
	declaration := testDeclaration(t)
	frame, err := encodeOpusPacket(declaration.SessionID, []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	id, payload, err := decodeOpusPacket(frame)
	if err != nil || id != declaration.SessionID || string(payload) != string([]byte{1, 2, 3}) {
		t.Fatalf("decode opus = %s %v %v", id, payload, err)
	}
}
