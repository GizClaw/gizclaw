package gizedge

import (
	"errors"
	"net/http"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestOrderedUpstreamTransportRetriesSafeRequestOnNextUpstream(t *testing.T) {
	firstErr := errors.New("first upstream failed")
	secondErr := errors.New("second upstream failed")
	first := &failingGiznetConn{dialErr: firstErr, state: giznet.PeerStateEstablished}
	second := &failingGiznetConn{dialErr: secondErr, state: giznet.PeerStateEstablished}
	transport := &orderedUpstreamTransport{entries: []*upstreamTransport{
		{conn: first, connEpoch: 1},
		{conn: second, connEpoch: 1},
	}}
	request, err := http.NewRequest(http.MethodGet, "http://gizclaw/server-info", nil)
	if err != nil {
		t.Fatalf("http.NewRequest error = %v", err)
	}

	_, err = transport.RoundTrip(request)
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("RoundTrip error = %v, want both upstream errors", err)
	}
}

func TestOrderedUpstreamTransportDoesNotReplayUnsafeRequest(t *testing.T) {
	firstErr := errors.New("first upstream failed")
	secondErr := errors.New("second upstream failed")
	first := &failingGiznetConn{dialErr: firstErr, state: giznet.PeerStateEstablished}
	second := &failingGiznetConn{dialErr: secondErr, state: giznet.PeerStateEstablished}
	transport := &orderedUpstreamTransport{entries: []*upstreamTransport{
		{conn: first, connEpoch: 1},
		{conn: second, connEpoch: 1},
	}}
	request, err := http.NewRequest(http.MethodPost, "http://gizclaw/login", nil)
	if err != nil {
		t.Fatalf("http.NewRequest error = %v", err)
	}

	_, err = transport.RoundTrip(request)
	if !errors.Is(err, firstErr) {
		t.Fatalf("RoundTrip error = %v, want first upstream error", err)
	}
	if errors.Is(err, secondErr) {
		t.Fatalf("RoundTrip error = %v, unsafe request reached second upstream", err)
	}
}
