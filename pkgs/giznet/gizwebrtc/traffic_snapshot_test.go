package gizwebrtc

import "testing"

func TestTrafficSnapshotCombinesEndpointRolesAndSeparatesPeerKinds(t *testing.T) {
	client := (&Conn{nodeRole: "edge", role: "client", metricsPeerKind: "admin"}).trafficMetrics()
	server := (&Conn{nodeRole: "edge", role: "server", metricsPeerKind: "admin"}).trafficMetrics()
	before := ReadTrafficSnapshot("edge")
	client.channels[channelNative].active.Add(1)
	server.channels[channelNative].active.Add(2)
	client.rtp[1].active.Add(1)
	t.Cleanup(func() {
		client.channels[channelNative].active.Add(-1)
		server.channels[channelNative].active.Add(-2)
		client.rtp[1].active.Add(-1)
	})
	after := ReadTrafficSnapshot("edge")
	if len(after) != 4 {
		t.Fatalf("peer kind count = %d", len(after))
	}
	for i, item := range after {
		if item.PeerKind != before[i].PeerKind {
			t.Fatal("unstable peer kind")
		}
		if item.PeerKind == "admin" {
			if item.NativeChannels != before[i].NativeChannels+3 || item.RTPTXTracks != before[i].RTPTXTracks+1 {
				t.Fatalf("endpoint roles not combined: before=%+v after=%+v", before[i], item)
			}
		} else if item.NativeChannels != before[i].NativeChannels {
			t.Fatal("resource leaked across peer kinds")
		}
	}
}
