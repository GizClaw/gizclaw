package gizwebrtc

import (
	"bytes"
	"context"
	"io"
	"maps"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestMeteredDataChannelCountsMessagesAcrossPartialReadsAndChunkedWrites(t *testing.T) {
	counts := &channelMetrics{}
	counts.active.Store(1)
	payload := bytes.Repeat([]byte{1}, streamChunkSize+7)
	raw := &meteredDataChannel{ReadWriteCloserDeadliner: &fakeStreamRaw{reads: [][]byte{payload}}, counts: counts}
	stream := newDataChannelConn(raw, nil, nil, nil)
	buf := make([]byte, 13)
	for remaining := len(payload); remaining > 0; {
		n, err := stream.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		remaining -= n
	}
	if got := counts.rx.Load(); got != 1 {
		t.Fatalf("partial reads counted %d messages, want 1", got)
	}
	payload = bytes.Repeat([]byte{1}, 2*streamChunkSize+7)
	if n, err := stream.Write(payload); err != nil || n != len(payload) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if got := counts.tx.Load(); got != 3 {
		t.Fatalf("chunked write counted %d messages, want 3", got)
	}
	if _, _, err := raw.ReadDataChannel(buf); err != io.EOF {
		t.Fatalf("Read after payload = %v, want EOF", err)
	}
	_ = stream.Close()
	_ = raw.Close()
	if got := counts.active.Load(); got != 0 {
		t.Fatalf("EOF plus duplicate close active = %d", got)
	}
}

func TestTransportMetricsMeasureRealPacketServiceNativeAndRTP(t *testing.T) {
	base := readTrafficGauges()
	client, server := nativeChannelTestPair(t)
	clientCounts, serverCounts := client.trafficMetrics(), server.trafficMetrics()
	if clientCounts.rtp[0].active.Load() == 0 || serverCounts.rtp[0].active.Load() == 0 {
		t.Fatal("silent negotiated RTP uplink was not counted")
	}
	packetTx, packetRx := clientCounts.channels[channelPacket].tx.Load(), serverCounts.channels[channelPacket].rx.Load()
	if _, err := client.Write(0x42, []byte("packet")); err != nil {
		t.Fatal(err)
	}
	if protocol, _ := readDirectPacketWithTimeout(t, server); protocol != 0x42 {
		t.Fatalf("packet protocol = %d", protocol)
	}
	if clientCounts.channels[channelPacket].tx.Load() != packetTx+1 || serverCounts.channels[channelPacket].rx.Load() != packetRx+1 {
		t.Fatal("packet channel message not measured at each endpoint")
	}
	listener := server.ListenService(77)
	send, err := client.Dial(77)
	if err != nil {
		t.Fatal(err)
	}
	recv, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = send.Close(); _ = recv.Close() })
	serviceTx, serviceRx := clientCounts.channels[channelService].tx.Load(), serverCounts.channels[channelService].rx.Load()
	if _, err := send.Write([]byte("service")); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(recv, make([]byte, 7)); err != nil {
		t.Fatal(err)
	}
	if clientCounts.channels[channelService].tx.Load() != serviceTx+1 || serverCounts.channels[channelService].rx.Load() != serviceRx+1 {
		t.Fatal("service channel message not measured")
	}
	incoming := make(chan *NativeChannel, 1)
	unregister, err := server.RegisterNativeChannelHandler("metrics/", func(channel *NativeChannel) { incoming <- channel })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unregister)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	native, err := client.OpenNativeChannel(ctx, "metrics/test", NativeChannelOptions{Ordered: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = native.Close() })
	var remote *NativeChannel
	select {
	case remote = <-incoming:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	nativeTx, nativeRx := clientCounts.channels[channelNative].tx.Load(), serverCounts.channels[channelNative].rx.Load()
	if _, err := native.WriteMessage([]byte("native")); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.ReadMessage(make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if clientCounts.channels[channelNative].tx.Load() != nativeTx+1 || serverCounts.channels[channelNative].rx.Load() != nativeRx+1 {
		t.Fatal("native message bypassed instrumentation")
	}
	rtpTx, rtpRx := clientCounts.rtp[1].packets.Load(), serverCounts.rtp[0].packets.Load()
	if _, err := client.Write(giznet.ProtocolOpusPacket, []byte{0x00, 0xaa, 0xbb}); err != nil {
		t.Fatal(err)
	}
	if protocol, _ := readDirectPacketWithTimeout(t, server); protocol != giznet.ProtocolOpusPacket {
		t.Fatalf("RTP packet protocol = %d", protocol)
	}
	if clientCounts.rtp[1].packets.Load() != rtpTx+1 || serverCounts.rtp[0].packets.Load() != rtpRx+1 {
		t.Fatal("actual RTP packet not measured")
	}
	if serverCounts.rtp[0].active.Load() == 0 || clientCounts.rtp[1].active.Load() == 0 {
		t.Fatal("negotiated send or observed receive track not counted")
	}
	serviceActive, nativeActive := serverCounts.channels[channelService].active.Load(), serverCounts.channels[channelNative].active.Load()
	_ = send.Close()
	_ = native.Close()
	// Detached channels observe remote closure when their pending messages have
	// drained. A terminal read releases the gauge even before application Close.
	if _, err := recv.Read(make([]byte, 16)); err != io.EOF {
		t.Fatalf("service remote close = %v, want EOF", err)
	}
	if _, err := remote.ReadMessage(make([]byte, 16)); err != io.EOF {
		t.Fatalf("native remote close = %v, want EOF", err)
	}
	waitTrafficMetric(t, func() bool {
		return serverCounts.channels[channelService].active.Load() == serviceActive-1 && serverCounts.channels[channelNative].active.Load() == nativeActive-1
	})
	_ = client.Close()
	_ = server.Close()
	waitTrafficMetric(t, func() bool { return maps.Equal(readTrafficGauges(), base) })
}

func waitTrafficMetric(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for !ready() {
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatalf("transport metrics did not settle: %v", readTrafficGauges())
		}
	}
}

func readTrafficGauges() map[string]int64 {
	values := make(map[string]int64)
	for _, node := range []string{"application", "edge"} {
		for _, sample := range MetricsSnapshot(time.Now(), node) {
			if sample.Name != metricDataChannelsActive && sample.Name != metricRTPTracksActive {
				continue
			}
			if sample.Value != 0 {
				key := node + sample.Name + sample.Labels["role"] + sample.Labels["peer_kind"] + sample.Labels["channel_kind"] + sample.Labels["direction"]
				values[key] = int64(sample.Value)
			}
		}
	}
	return values
}

func BenchmarkMeteredDataChannelMessage(b *testing.B) {
	for _, metered := range []bool{false, true} {
		name := "raw"
		if metered {
			name = "metered"
		}
		b.Run(name, func(b *testing.B) {
			raw := &repeatingStreamRaw{message: []byte("message")}
			measured := &meteredDataChannel{ReadWriteCloserDeadliner: raw, counts: &channelMetrics{}}
			buf := make([]byte, 64)
			b.ReportAllocs()
			for b.Loop() {
				if metered {
					_, _, _ = measured.ReadDataChannel(buf)
				} else {
					_, _, _ = raw.ReadDataChannel(buf)
				}
			}
		})
	}
}
