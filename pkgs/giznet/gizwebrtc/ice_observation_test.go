package gizwebrtc

import (
	"reflect"
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestSanitizeICECandidateOmitsEndpointAndCredentialFields(t *testing.T) {
	candidate := &webrtc.ICECandidate{
		Foundation:     "secret-foundation",
		Priority:       123,
		Address:        "192.0.2.10",
		Protocol:       webrtc.ICEProtocolUDP,
		Port:           49160,
		Typ:            webrtc.ICECandidateTypeRelay,
		Component:      1,
		RelatedAddress: "10.0.0.1",
		RelatedPort:    3478,
	}
	got := sanitizeICECandidate(candidate)
	want := ICECandidateObservation{
		Type: "relay", Protocol: "udp", AddressFamily: "ipv4", Component: 1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sanitizeICECandidate() = %+v, want %+v", got, want)
	}
	value := reflect.ValueOf(got)
	for _, forbidden := range []string{"Address", "Port", "Foundation", "Priority", "URL", "Credential"} {
		if value.FieldByName(forbidden).IsValid() {
			t.Fatalf("sanitized observation exposes forbidden field %q", forbidden)
		}
	}
}

func TestICEAddressFamily(t *testing.T) {
	for _, test := range []struct {
		address string
		want    string
	}{
		{address: "192.0.2.1", want: "ipv4"},
		{address: "2001:db8::1", want: "ipv6"},
		{address: "turn.example.invalid", want: "unknown"},
		{address: "", want: "unknown"},
	} {
		if got := iceAddressFamily(test.address); got != test.want {
			t.Errorf("iceAddressFamily(%q) = %q, want %q", test.address, got, test.want)
		}
	}
}

func TestSelectedICECandidatePairStatsMatchesExactPair(t *testing.T) {
	selected := &webrtc.ICECandidatePair{
		Local:  &webrtc.ICECandidate{Address: "192.0.2.10", Port: 1000, Protocol: webrtc.ICEProtocolUDP, Typ: webrtc.ICECandidateTypeRelay},
		Remote: &webrtc.ICECandidate{Address: "192.0.2.20", Port: 2000, Protocol: webrtc.ICEProtocolUDP, Typ: webrtc.ICECandidateTypeHost},
	}
	report := webrtc.StatsReport{
		"wanted":        webrtc.ICECandidatePairStats{ID: "wanted", LocalCandidateID: "local-wanted", RemoteCandidateID: "remote-wanted", BytesSent: 1},
		"local-wanted":  webrtc.ICECandidateStats{ID: "local-wanted", IP: "192.0.2.10", Port: 1000, Protocol: "udp", CandidateType: webrtc.ICECandidateTypeRelay},
		"remote-wanted": webrtc.ICECandidateStats{ID: "remote-wanted", IP: "192.0.2.20", Port: 2000, Protocol: "udp", CandidateType: webrtc.ICECandidateTypeHost},
		"other":         webrtc.ICECandidatePairStats{ID: "other", LocalCandidateID: "local-other", RemoteCandidateID: "remote-wanted", BytesSent: 1000},
		"local-other":   webrtc.ICECandidateStats{ID: "local-other", IP: "192.0.2.11", Port: 1001, Protocol: "udp", CandidateType: webrtc.ICECandidateTypeRelay},
	}
	pair, ok := selectedICECandidatePairStats(report, selected)
	if !ok || pair.ID != "wanted" {
		t.Fatalf("selected stats = %+v, %t, want wanted", pair, ok)
	}
}
