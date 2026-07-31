package cgobackend

import (
	"bytes"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

type recordingSampleWriter struct {
	samples []media.Sample
}

func (w *recordingSampleWriter) WriteSample(sample media.Sample) error {
	sample.Data = append([]byte(nil), sample.Data...)
	w.samples = append(w.samples, sample)
	return nil
}

type recordingEventSink struct {
	opus       [][]byte
	peerStates []int
}

func (s *recordingEventSink) PeerState(state int) {
	s.peerStates = append(s.peerStates, state)
}
func (*recordingEventSink) RemoteChannel(int, string, bool, bool) {}
func (*recordingEventSink) ChannelState(int, int)                 {}
func (*recordingEventSink) ChannelMessage(int, []byte, bool)      {}
func (*recordingEventSink) BufferedAmountLow(int)                 {}
func (s *recordingEventSink) OpusFrame(opus []byte) {
	s.opus = append(s.opus, append([]byte(nil), opus...))
}

func TestSendOpusUsesPacketDuration(t *testing.T) {
	writer := &recordingSampleWriter{}
	backend := New()
	backend.opusTrack = writer

	for _, packet := range [][]byte{
		{0x80},
		{0x98},
		{0x03, 0x0c},
	} {
		if err := backend.SendOpus(packet); err != nil {
			t.Fatalf("SendOpus(%x): %v", packet, err)
		}
	}
	want := []time.Duration{
		2500 * time.Microsecond,
		20 * time.Millisecond,
		120 * time.Millisecond,
	}
	if len(writer.samples) != len(want) {
		t.Fatalf("samples = %d, want %d", len(writer.samples), len(want))
	}
	for i := range want {
		if writer.samples[i].Duration != want[i] {
			t.Errorf("sample %d duration = %v, want %v", i, writer.samples[i].Duration, want[i])
		}
	}
	packetCalls, opusCalls := backend.TransportSendCounts()
	if packetCalls != 0 || opusCalls != uint64(len(want)) {
		t.Fatalf(
			"transport counts = packet:%d opus:%d, want packet:0 opus:%d",
			packetCalls, opusCalls, len(want),
		)
	}
}

func TestOpusEventQueueDropsOldestOnly(t *testing.T) {
	backend := New()
	sink := &recordingEventSink{}
	backend.SetEventSink(sink)
	backend.enqueue(backendEvent{
		kind: backendEventChannelMessage, channelID: 7, data: []byte("packet"),
	})
	for i := range 10 {
		backend.enqueue(backendEvent{kind: backendEventOpusFrame, data: []byte{byte(i)}})
	}

	backend.Poll(0)
	if len(sink.opus) != 8 {
		t.Fatalf("Opus frames = %d, want 8", len(sink.opus))
	}
	for i := range sink.opus {
		if !bytes.Equal(sink.opus[i], []byte{byte(i + 2)}) {
			t.Errorf("Opus frame %d = %v, want %d", i, sink.opus[i], i+2)
		}
	}
}

func TestAnswerHasBidirectionalOpus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer string
		want   bool
	}{
		{
			name:   "explicit sendrecv",
			answer: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=sendrecv\r\na=rtpmap:111 opus/48000/2\r\n",
			want:   true,
		},
		{
			name:   "default sendrecv",
			answer: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=rtpmap:111 opus/48000/2\r\n",
			want:   true,
		},
		{
			name:   "recvonly",
			answer: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=recvonly\r\na=rtpmap:111 opus/48000/2\r\n",
		},
		{
			name:   "rejected audio section",
			answer: "v=0\r\nm=audio 0 UDP/TLS/RTP/SAVPF 111\r\na=sendrecv\r\na=rtpmap:111 opus/48000/2\r\n",
		},
		{
			name:   "session recvonly",
			answer: "v=0\r\na=recvonly\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=rtpmap:111 opus/48000/2\r\n",
		},
		{
			name:   "wrong codec",
			answer: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 0\r\na=sendrecv\r\na=rtpmap:0 PCMU/8000\r\n",
		},
		{
			name:   "Opus mapping outside audio payload list",
			answer: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 0\r\na=sendrecv\r\na=rtpmap:111 opus/48000/2\r\n",
		},
		{
			name:   "invalid Opus channels",
			answer: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=sendrecv\r\na=rtpmap:111 opus/48000/99\r\n",
		},
		{
			name:   "Opus mapping in later active audio section",
			answer: "v=0\r\nm=audio 0 UDP/TLS/RTP/SAVPF 111\r\na=rtpmap:111 opus/48000/2\r\nm=audio 9 UDP/TLS/RTP/SAVPF 112\r\na=sendrecv\r\na=rtpmap:112 opus/48000/2\r\n",
			want:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := answerHasBidirectionalOpus(tc.answer); got != tc.want {
				t.Fatalf("answerHasBidirectionalOpus() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestBackendAcceptsOnlyOneRemoteOpusTrack(t *testing.T) {
	backend := New()
	if !backend.claimRemoteOpus() {
		t.Fatal("first remote Opus track was rejected")
	}
	if backend.claimRemoteOpus() {
		t.Fatal("duplicate remote Opus track was accepted")
	}
	backend.Close()
	if backend.claimRemoteOpus() {
		t.Fatal("remote Opus track was accepted after close")
	}
}

func TestBackendReportsAcceptedRemoteOpusTrackEndAsPeerFailure(t *testing.T) {
	backend := New()
	sink := &recordingEventSink{}
	backend.SetEventSink(sink)

	backend.failPeer()
	backend.Poll(0)

	if len(sink.peerStates) != 1 || sink.peerStates[0] != RTCPeerFailed {
		t.Fatalf("peer states = %v, want [%d]", sink.peerStates, RTCPeerFailed)
	}
}

func TestBackendKeepsIndependentSameServiceDataChannels(t *testing.T) {
	backend := New()
	if err := backend.CreatePeer(); err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	for _, channel := range []struct {
		id    int
		label string
	}{
		{id: -1, label: "giznet/v1/service/49"},
		{id: -2, label: "giznet/v1/service/49"},
		{id: -3, label: "giznet/v1/service/48"},
	} {
		if err := backend.CreateDataChannel(channel.label, channel.id, true, true); err != nil {
			t.Fatalf("CreateDataChannel(%q, %d): %v", channel.label, channel.id, err)
		}
	}
	backend.mu.Lock()
	if len(backend.dcs) != 3 {
		backend.mu.Unlock()
		t.Fatalf("live data channels = %d, want 3", len(backend.dcs))
	}
	first := backend.dcs[-1]
	second := backend.dcs[-2]
	different := backend.dcs[-3]
	backend.mu.Unlock()
	if first == nil || second == nil || different == nil ||
		first == second || first == different || second == different {
		t.Fatal("same-ID and different-ID services did not receive distinct channel state")
	}

	backend.CloseDataChannel(-1)
	backend.mu.Lock()
	_, firstOpen := backend.dcs[-1]
	_, secondOpen := backend.dcs[-2]
	_, differentOpen := backend.dcs[-3]
	backend.mu.Unlock()
	if firstOpen || !secondOpen || !differentOpen {
		t.Fatalf(
			"channel close state = first:%t second:%t different:%t",
			firstOpen,
			secondOpen,
			differentOpen,
		)
	}
}

func TestBackendBidirectionalOpusRTP(t *testing.T) {
	backend := New()
	sink := &recordingEventSink{}
	backend.SetEventSink(sink)
	if err := backend.CreatePeer(); err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	offer, err := backend.StartOffer()
	if err != nil {
		t.Fatal(err)
	}

	remote, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	remoteTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType: MediaStreamOpus, ClockRate: 48000, Channels: 2,
	}, "remote-opus", "remote")
	if err != nil {
		t.Fatal(err)
	}
	remoteTransceiver, err := remote.AddTransceiverFromTrack(
		remoteTrack,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendrecv},
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			if _, _, err := remoteTransceiver.Sender().ReadRTCP(); err != nil {
				return
			}
		}
	}()
	downlink := make(chan []byte, 1)
	remote.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		packet, _, err := track.ReadRTP()
		if err == nil {
			downlink <- append([]byte(nil), packet.Payload...)
		}
	})
	if err := remote.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer, SDP: offer,
	}); err != nil {
		t.Fatal(err)
	}
	gathered := webrtc.GatheringCompletePromise(remote)
	answer, err := remote.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	<-gathered
	if err := backend.SetRemoteSDP(remote.LocalDescription().SDP); err != nil {
		t.Fatal(err)
	}
	connectedDeadline := time.Now().Add(3 * time.Second)
	for (backend.pc.ConnectionState() != webrtc.PeerConnectionStateConnected ||
		remote.ConnectionState() != webrtc.PeerConnectionStateConnected) &&
		time.Now().Before(connectedDeadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if backend.pc.ConnectionState() != webrtc.PeerConnectionStateConnected ||
		remote.ConnectionState() != webrtc.PeerConnectionStateConnected {
		t.Fatalf(
			"peer states = %s/%s, want connected",
			backend.pc.ConnectionState(),
			remote.ConnectionState(),
		)
	}

	serverPacket := []byte{0xf8, 0x51}
	if err := backend.SendOpus(serverPacket); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-downlink:
		if !bytes.Equal(got, serverPacket) {
			t.Fatalf("downlink Opus = %x, want %x", got, serverPacket)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for downlink Opus RTP")
	}

	clientPacket := []byte{0xf8, 0x52}
	if err := remoteTrack.WriteSample(media.Sample{
		Data: clientPacket, Duration: 20 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(sink.opus) == 0 && time.Now().Before(deadline) {
		backend.Poll(20)
	}
	if len(sink.opus) != 1 || !bytes.Equal(sink.opus[0], clientPacket) {
		t.Fatalf("uplink Opus = %x, want %x", sink.opus, clientPacket)
	}
}
