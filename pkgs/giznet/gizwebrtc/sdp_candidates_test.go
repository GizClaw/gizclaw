package gizwebrtc

import (
	"strings"
	"testing"
)

func TestRewriteSDPHostCandidatesUsesPublicEndpoints(t *testing.T) {
	raw := "v=0\r\n" +
		"o=- 0 0 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n" +
		"a=candidate:0 1 UDP 2130706431 10.0.0.2 9820 typ host generation 0\r\n" +
		"m=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\n" +
		"a=candidate:1 1 UDP 2130706431 172.18.0.2 9820 typ host generation 0\r\n" +
		"a=candidate:2 1 TCP 1671430143 172.18.0.2 9820 typ host tcptype passive\r\n" +
		"a=candidate:3 1 UDP 1694498815 198.51.100.1 50000 typ srflx raddr 172.18.0.2 rport 9820\r\n" +
		"a=candidate:4 1 UDP 16777215 203.0.113.10 50001 typ relay raddr 172.18.0.2 rport 9820\r\n" +
		"a=candidate:5 1 TCP 16777214 203.0.113.11 50002 typ prflx raddr 172.18.0.2 rport 9820 tcptype passive\r\n"

	got, err := rewriteSDPHostCandidates(raw, "192.168.1.20:19820", "192.168.1.20:19820")
	if err != nil {
		t.Fatalf("rewriteSDPHostCandidates error = %v", err)
	}

	for _, want := range []string{
		"a=candidate:0 1 UDP 2130706431 192.168.1.20 19820 typ host generation 0",
		"a=candidate:1 1 UDP 2130706431 192.168.1.20 19820 typ host generation 0",
		"a=candidate:2 1 TCP 1671430143 192.168.1.20 19820 typ host tcptype passive",
		"a=candidate:3 1 UDP 1694498815 198.51.100.1 50000 typ srflx raddr 172.18.0.2 rport 9820",
		"a=candidate:4 1 UDP 16777215 203.0.113.10 50001 typ relay raddr 172.18.0.2 rport 9820",
		"a=candidate:5 1 TCP 16777214 203.0.113.11 50002 typ prflx raddr 172.18.0.2 rport 9820 tcptype passive",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rewritten SDP missing %q:\n%s", want, got)
		}
	}
}

func TestRewriteSDPHostCandidatesSkipsNonConcretePublicEndpoint(t *testing.T) {
	raw := "v=0\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n" +
		"m=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\n" +
		"a=candidate:0 1 UDP 2130706431 10.0.0.2 9820 typ host generation 0\r\n" +
		"a=candidate:1 1 TCP 1671430143 172.18.0.2 9820 typ host tcptype passive\r\n"
	got, err := rewriteSDPHostCandidates(raw, "example.com:9820", "0.0.0.0:9820")
	if err != nil {
		t.Fatalf("rewriteSDPHostCandidates error = %v", err)
	}
	if got != raw {
		t.Fatalf("rewriteSDPHostCandidates changed SDP without concrete public endpoint")
	}
}
