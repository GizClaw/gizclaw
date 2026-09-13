package gizclaw

import (
	"bytes"
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestGeneratedOggOpusAggregatesThroughServedGoClient(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	serverListener, err := (&gizwebrtc.ListenConfig{
		CipherMode:     gizwebrtc.CipherModePlaintext,
		SecurityPolicy: testGiznetSecurityPolicy{allowService: func(giznet.PublicKey, uint64) bool { return true }},
		API:            newTestWebRTCAPI(t),
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("gizwebrtc.Listen() error = %v", err)
	}
	t.Cleanup(func() { _ = serverListener.Close() })
	signalingServer := httptest.NewServer(serverListener.SignalingHandler())
	t.Cleanup(signalingServer.Close)
	accepted := make(chan giznet.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, acceptError := serverListener.Accept()
		if acceptError != nil {
			acceptErr <- acceptError
			return
		}
		accepted <- conn
	}()

	client := &gizcli.Client{
		KeyPair: clientKey,
		DialTransport: func(
			key *giznet.KeyPair,
			serverPublicKey giznet.PublicKey,
			serverAddress string,
			policy giznet.SecurityPolicy,
		) (giznet.Listener, giznet.Conn, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return gizwebrtc.Dial(ctx, key, serverPublicKey, gizwebrtc.DialConfig{
				SignalingURL:   serverAddress,
				CipherMode:     gizwebrtc.CipherModePlaintext,
				SecurityPolicy: policy,
				API:            newTestWebRTCAPI(t),
			})
		},
	}
	if err := client.Dial(serverKey.Public, signalingServer.URL+gizwebrtc.SignalingPath); err != nil {
		t.Fatalf("Client.Dial() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	var serverConn giznet.Conn
	select {
	case serverConn = <-accepted:
	case err := <-acceptErr:
		t.Fatalf("server Accept() error = %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server Accept() timeout")
	}
	t.Cleanup(func() { _ = serverConn.Close() })
	eventListener := serverConn.ListenService(EventStreamAgent)
	eventStream, err := eventListener.Accept()
	if err != nil {
		t.Fatalf("Accept(EventStreamAgent) error = %v", err)
	}
	t.Cleanup(func() { _ = eventStream.Close() })
	broker := newPeerStreamEventBroker()
	unsubscribe, err := broker.Subscribe(eventStream)
	if err != nil {
		t.Fatalf("Subscribe(EventStreamAgent) error = %v", err)
	}
	t.Cleanup(unsubscribe)

	serveDone := make(chan error, 1)
	go func() { serveDone <- client.Serve() }()
	stream, err := client.OpenPeerStream(32)
	if err != nil {
		t.Fatalf("OpenPeerStream() error = %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })
	peer := &PeerConn{Conn: serverConn}
	peer.initMixer()
	egressDone := make(chan error, 1)
	go func() {
		_, streamErr := peer.streamMixedAudio(false)
		egressDone <- streamErr
	}()

	inputPackets := generatedAudioDownlinkOpusPackets(t, 3)
	var ogg bytes.Buffer
	if err := codecconv.OpusPacketsToOgg(&ogg, 48000, 1, inputPackets); err != nil {
		t.Fatalf("OpusPacketsToOgg() error = %v", err)
	}
	if ogg.Len() == 0 {
		t.Fatal("generated Ogg/Opus is empty")
	}
	rounds := []struct {
		streamID string
		chunks   []*genx.MessageChunk
	}{
		{
			streamID: "live-a",
			chunks: []*genx.MessageChunk{
				audioDownlinkChunk("live-a", "assistant", ogg.Bytes(), true, false),
				audioDownlinkChunk("effect-b", "effect", ogg.Bytes(), true, false),
				audioDownlinkChunk("live-a", "assistant", nil, false, true),
				audioDownlinkChunk("effect-b", "effect", nil, false, true),
			},
		},
		{
			streamID: "history-replay",
			chunks: []*genx.MessageChunk{
				audioDownlinkChunk("history-replay", "assistant", ogg.Bytes(), true, false),
				audioDownlinkChunk("history-replay", "assistant", nil, false, true),
			},
		},
	}
	for round, spec := range rounds {
		output := &peerStreamSliceStream{chunks: spec.chunks, doneErr: genx.ErrDone}
		outputDone := make(chan error, 1)
		go func() {
			outputDone <- (peerAgentOutput{Events: broker, Tracks: peer}).ConsumeAgentOutput(context.Background(), output)
		}()
		packets := collectServedGoClientAudioRoute(t, stream, spec.streamID)
		if len(packets) == 0 {
			t.Fatalf("round %d delivered zero routed Opus packets", round)
		}
		assertAudioDownlinkDecodableOpus(t, packets)
		select {
		case err := <-outputDone:
			if err != nil {
				t.Fatalf("round %d ConsumeAgentOutput() error = %v", round, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("round %d ConsumeAgentOutput() timeout", round)
		}
	}

	peer.closed.Store(true)
	if err := peer.mixer.Close(); err != nil {
		t.Fatalf("mixer.Close() error = %v", err)
	}
	_ = client.Close()
	select {
	case <-egressDone:
	case <-time.After(3 * time.Second):
		t.Fatal("streamMixedAudio() did not stop")
	}
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Client.Serve() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Client.Serve() did not stop")
	}
}

func audioDownlinkChunk(streamID, label string, data []byte, bos, eos bool) *genx.MessageChunk {
	return &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: append([]byte(nil), data...)},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, BeginOfStream: bos, EndOfStream: eos},
	}
}

func collectServedGoClientAudioRoute(t *testing.T, stream *gizcli.PeerStream, streamID string) [][]byte {
	t.Helper()
	var packets [][]byte
	started := false
	for {
		type result struct {
			chunk *genx.MessageChunk
			err   error
		}
		resultCh := make(chan result, 1)
		go func() {
			chunk, err := stream.Next()
			resultCh <- result{chunk: chunk, err: err}
		}()
		var chunk *genx.MessageChunk
		select {
		case result := <-resultCh:
			if result.err != nil {
				t.Fatalf("PeerStream.Next() error = %v", result.err)
			}
			chunk = result.chunk
		case <-time.After(5 * time.Second):
			t.Fatalf("PeerStream.Next() timeout for %q", streamID)
		}
		if chunk.IsBeginOfStream() && chunk.Ctrl != nil && chunk.Ctrl.StreamID == streamID {
			started = true
			if chunk.Ctrl.Label != "assistant" {
				t.Fatalf("BOS route = %#v, want %q/assistant", chunk.Ctrl, streamID)
			}
			continue
		}
		if !started {
			continue
		}
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != streamID || chunk.Ctrl.Label != "assistant" {
			t.Fatalf("active SDK chunk route = %#v, want %q/assistant", chunk.Ctrl, streamID)
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
			packets = append(packets, append([]byte(nil), blob.Data...))
		}
		if chunk.IsEndOfStream() {
			return packets
		}
	}
}

func assertAudioDownlinkDecodableOpus(t *testing.T, packets [][]byte) {
	t.Helper()
	decoder, err := opus.NewDecoder(16000, 1)
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	defer decoder.Close()
	decodedSamples := 0
	for index, packet := range packets {
		decoded, err := decoder.Decode(packet, 16000*120/1000, false)
		if err != nil {
			t.Fatalf("Decode(packet %d) error = %v", index, err)
		}
		decodedSamples += len(decoded)
	}
	if decodedSamples == 0 {
		t.Fatal("Opus packets decoded to zero PCM samples")
	}
}

func generatedAudioDownlinkOpusPackets(t *testing.T, count int) [][]byte {
	t.Helper()
	encoder, err := opus.NewEncoder(48000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder() error = %v", err)
	}
	defer encoder.Close()
	packets := make([][]byte, count)
	for index := range packets {
		pcm := make([]int16, 960)
		for sample := range pcm {
			pcm[sample] = int16((sample + index*100) % 16000)
		}
		packets[index], err = encoder.Encode(pcm, len(pcm))
		if err != nil {
			t.Fatalf("Encode(packet %d) error = %v", index, err)
		}
	}
	return packets
}
