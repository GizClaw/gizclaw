package gizwebrtc

import (
	"strings"
	"testing"
)

func FuzzRewriteHostCandidate(f *testing.F) {
	for _, seed := range []string{
		"candidate:0 1 UDP 2130706431 10.0.0.2 9820 typ host generation 0",
		"candidate:2 1 TCP 1671430143 172.18.0.2 9820 typ host tcptype passive",
		"candidate:3 1 UDP 1694498815 198.51.100.1 50000 typ srflx raddr 172.18.0.2 rport 9820",
		"candidate:0 1 UDP",
		"",
	} {
		f.Add(seed)
	}

	addrs := publicICECandidateAddrs{
		udpHost: "192.0.2.20",
		udpPort: "19820",
		hasUDP:  true,
		tcpHost: "192.0.2.20",
		tcpPort: "19821",
		hasTCP:  true,
	}
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 1024 {
			return
		}
		rewritten, changed, err := rewriteHostCandidate(value, addrs)
		if err != nil {
			return
		}
		if !changed {
			return
		}
		fields := strings.Fields(rewritten)
		if len(fields) < 6 {
			t.Fatalf("rewritten candidate has too few fields: %q", rewritten)
		}
		if fields[4] != addrs.udpHost && fields[4] != addrs.tcpHost {
			t.Fatalf("rewritten candidate host = %q", fields[4])
		}
		if fields[5] != addrs.udpPort && fields[5] != addrs.tcpPort {
			t.Fatalf("rewritten candidate port = %q", fields[5])
		}
	})
}

func FuzzRewriteSDPHostCandidates(f *testing.F) {
	for _, seed := range []string{
		"v=0\r\ns=-\r\nt=0 0\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\na=candidate:0 1 UDP 2130706431 10.0.0.2 9820 typ host generation 0\r\n",
		"v=0\r\ns=-\r\nt=0 0\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\na=candidate:1 1 TCP 1671430143 172.18.0.2 9820 typ host tcptype passive\r\n",
		"not an sdp",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, rawSDP string) {
		if len(rawSDP) > 4096 {
			return
		}
		_, _ = rewriteSDPHostCandidates(rawSDP, "192.0.2.20:19820", "192.0.2.20:19821")
	})
}
