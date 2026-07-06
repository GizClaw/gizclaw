package gizwebrtc

import (
	"strings"
	"testing"
)

func TestRewriteSDPUDPHostCandidatesUsesPublicEndpoint(t *testing.T) {
	raw := "v=0\r\n" +
		"o=- 0 0 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n" +
		"a=candidate:0 1 UDP 2130706431 10.0.0.2 9820 typ host generation 0\r\n" +
		"m=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\n" +
		"a=candidate:1 1 UDP 2130706431 172.18.0.2 9820 typ host generation 0\r\n" +
		"a=candidate:2 1 TCP 1671430143 172.18.0.2 9 typ host tcptype active\r\n" +
		"a=candidate:3 1 UDP 1694498815 198.51.100.1 50000 typ srflx raddr 172.18.0.2 rport 9820\r\n"

	got, err := rewriteSDPUDPHostCandidates(raw, "192.168.1.20:19820")
	if err != nil {
		t.Fatalf("rewriteSDPUDPHostCandidates error = %v", err)
	}

	for _, want := range []string{
		"a=candidate:0 1 UDP 2130706431 192.168.1.20 19820 typ host generation 0",
		"a=candidate:1 1 UDP 2130706431 192.168.1.20 19820 typ host generation 0",
		"a=candidate:2 1 TCP 1671430143 172.18.0.2 9 typ host tcptype active",
		"a=candidate:3 1 UDP 1694498815 198.51.100.1 50000 typ srflx raddr 172.18.0.2 rport 9820",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rewritten SDP missing %q:\n%s", want, got)
		}
	}
}

func TestRewriteSDPUDPHostCandidatesNoPublicEndpoint(t *testing.T) {
	raw := "v=0\r\ns=-\r\nt=0 0\r\n"
	got, err := rewriteSDPUDPHostCandidates(raw, "")
	if err != nil {
		t.Fatalf("rewriteSDPUDPHostCandidates error = %v", err)
	}
	if got != raw {
		t.Fatalf("rewriteSDPUDPHostCandidates changed SDP without public endpoint")
	}
}
