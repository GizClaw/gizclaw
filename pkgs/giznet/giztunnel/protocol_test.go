package giztunnel

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
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
		{status: sessionRedirected, reason: testDeclaration(t).ClientPublicKey.String()},
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

func TestSessionResultRejectsMalformedRedirectsAndUnknownStatus(t *testing.T) {
	key := testDeclaration(t).ClientPublicKey.String()
	for _, tc := range []struct {
		name   string
		status sessionResultStatus
		detail string
	}{
		{"empty key", sessionRedirected, ""},
		{"zero key", sessionRedirected, (giznet.PublicKey{}).String()},
		{"invalid key", sessionRedirected, "not a Server key"},
		{"short key", sessionRedirected, "1"},
		{"noncanonical key", sessionRedirected, " " + key},
		{"trailing whitespace", sessionRedirected, key + "\n"},
		{"invalid UTF8", sessionRedirected, "\xff"},
		{"oversized key", sessionRedirected, strings.Repeat("1", maxRejectReasonSize+1)},
		{"unknown status", 255, key},
		{"accepted with key", sessionAccepted, key},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := encodeSessionResult(tc.status, tc.detail); !errors.Is(err, ErrInvalidFrame) {
				t.Fatalf("encode error = %v", err)
			}
			frame := make([]byte, sessionResultHeaderSize+len(tc.detail))
			copy(frame, sessionResultMagic[:])
			frame[4] = byte(tc.status)
			binary.BigEndian.PutUint16(frame[5:7], uint16(len(tc.detail)))
			copy(frame[7:], tc.detail)
			if _, _, err := decodeSessionResult(frame); !errors.Is(err, ErrInvalidFrame) {
				t.Fatalf("decode error = %v", err)
			}
			if _, _, err := readSessionResult(bytes.NewReader(frame)); !errors.Is(err, ErrInvalidFrame) {
				t.Fatalf("read error = %v", err)
			}
		})
	}
}

func TestSessionRedirectIsDistinctFromLegacyResults(t *testing.T) {
	accepted, err := encodeSessionResult(sessionAccepted, "")
	if err != nil || !bytes.Equal(accepted, []byte{'G', 'Z', 'T', '2', 0, 0, 0}) {
		t.Fatalf("accepted wire = %x, %v", accepted, err)
	}
	rejected, err := encodeSessionResult(sessionRejected, "no")
	if err != nil || !bytes.Equal(rejected, []byte{'G', 'Z', 'T', '2', 1, 0, 2, 'n', 'o'}) {
		t.Fatalf("rejected wire = %x, %v", rejected, err)
	}
	key := testDeclaration(t).ClientPublicKey.String()
	redirected, err := encodeSessionResult(sessionRedirected, key)
	if err != nil {
		t.Fatal(err)
	}
	// The previous decoder only recognizes 0 and 1, so the redirect extension
	// cannot be interpreted as acceptance by an older endpoint.
	if redirected[4] == 0 || redirected[4] == 1 {
		t.Fatalf("redirect uses legacy status %d", redirected[4])
	}
	status, got, err := readSessionResult(io.MultiReader(bytes.NewReader(redirected[:3]), bytes.NewReader(redirected[3:8]), bytes.NewReader(redirected[8:])))
	if err != nil || status != sessionRedirected || got != key {
		t.Fatalf("fragmented result = %d, %q, %v", status, got, err)
	}
	redirected[6]++
	if _, _, err := decodeSessionResult(redirected); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("mismatched redirect size error = %v", err)
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
