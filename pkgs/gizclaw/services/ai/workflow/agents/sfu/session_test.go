package sfu

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media"
)

const (
	testWorkspaceID = "ws-test"
	testPeer        = "peer-a"
	testRemote      = "peer-b"
)

type fakeResolver struct {
	mu      sync.Mutex
	binding socialutil.SFUWorkspaceBinding
	err     error
	calls   int
}

func (r *fakeResolver) ResolveSFUWorkspaceBinding(_ context.Context, workspaceID, peer string) (socialutil.SFUWorkspaceBinding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return socialutil.SFUWorkspaceBinding{}, r.err
	}
	if workspaceID != r.binding.WorkspaceID {
		return socialutil.SFUWorkspaceBinding{}, ErrNotBound
	}
	if !slices.Contains(r.binding.Members, peer) {
		return socialutil.SFUWorkspaceBinding{}, ErrNotMember
	}
	return r.binding, nil
}

func (r *fakeResolver) set(mutate func(*fakeResolver)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	mutate(r)
}

type fakeClient struct {
	mu           sync.Mutex
	samples      []media.Sample
	disconnected int
	events       roomEvents
	params       connectParams
}

func (c *fakeClient) WriteAudio(sample media.Sample) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.samples = append(c.samples, sample)
	return nil
}

func (c *fakeClient) Disconnect() {
	c.mu.Lock()
	c.disconnected++
	c.mu.Unlock()
	c.events.onDisconnected(disconnectLeave)
}

func (c *fakeClient) sampleCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.samples)
}

func (c *fakeClient) disconnects() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disconnected
}

type fakeConnector struct {
	mu      sync.Mutex
	clients []*fakeClient
	fail    func(attempt int) error
}

func (c *fakeConnector) connect(ctx context.Context, params connectParams, events roomEvents) (roomClient, error) {
	c.mu.Lock()
	attempt := len(c.clients) + 1
	fail := c.fail
	c.mu.Unlock()
	if fail != nil {
		if err := fail(attempt); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client := &fakeClient{events: events, params: params}
	c.mu.Lock()
	c.clients = append(c.clients, client)
	c.mu.Unlock()
	return client, nil
}

func (c *fakeConnector) client(index int) *fakeClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.clients) {
		return nil
	}
	return c.clients[index]
}

func (c *fakeConnector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.clients)
}

// fakeReader feeds RTP packets to a remote route until closed.
type fakeReader struct {
	packets chan *rtp.Packet
	closed  chan struct{}
	once    sync.Once
}

func newFakeReader() *fakeReader {
	return &fakeReader{packets: make(chan *rtp.Packet, 64), closed: make(chan struct{})}
}

func (r *fakeReader) ReadRTP() (*rtp.Packet, interceptor.Attributes, error) {
	select {
	case packet := <-r.packets:
		return packet, nil, nil
	case <-r.closed:
		return nil, nil, io.EOF
	}
}

func (r *fakeReader) send(seq uint16, ts uint32, payload ...byte) {
	r.packets <- &rtp.Packet{Header: rtp.Header{SequenceNumber: seq, Timestamp: ts}, Payload: payload}
}

func (r *fakeReader) close() {
	r.once.Do(func() { close(r.closed) })
}

type harness struct {
	t         *testing.T
	agent     *Agent
	resolver  *fakeResolver
	connector *fakeConnector
}

func newHarness(t *testing.T, config Config) *harness {
	t.Helper()
	resolver := &fakeResolver{binding: socialutil.SFUWorkspaceBinding{
		WorkspaceID: testWorkspaceID,
		Kind:        socialutil.SFUWorkspaceKindFriend,
		Members:     []string{testPeer, testRemote},
		SFU:         socialutil.SFUBinding{URL: "wss://sfu.test", RoomToken: "room-1", Generation: 1},
	}}
	connector := &fakeConnector{}
	if config.RecheckInterval == 0 {
		config.RecheckInterval = 20 * time.Millisecond
	}
	if config.ReconnectTimeout == 0 {
		config.ReconnectTimeout = 500 * time.Millisecond
	}
	factory := Factory{Config: config, Bindings: resolver, connector: connector}
	agent, err := factory.NewAgent(context.Background(), agenthost.Spec{Workspace: apitypes.Workspace{Id: testWorkspaceID, Name: "ws"}})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	return &harness{t: t, agent: agent.(*Agent), resolver: resolver, connector: connector}
}

func (h *harness) attach(ctx context.Context, peer string) (genx.Stream, *outputStream) {
	h.t.Helper()
	input := newOutputStream()
	output, err := h.agent.Transform(gizlog.WithPeerPublicKey(ctx, peer), input)
	if err != nil {
		h.t.Fatalf("Transform() error = %v", err)
	}
	return output, input
}

func (h *harness) session(peer string) *session {
	h.agent.mu.Lock()
	defer h.agent.mu.Unlock()
	return h.agent.sessions[peer]
}

// queued reports how many chunks wait in the peer's output stream.
func (h *harness) queued(peer string) int {
	s := h.session(peer)
	if s == nil {
		return -1
	}
	s.out.mu.Lock()
	defer s.out.mu.Unlock()
	return len(s.out.queue)
}

func (h *harness) routeCount(peer string) int {
	s := h.session(peer)
	if s == nil {
		return -1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.routes)
}

// collect drains the output until it ends and returns the chunks plus the
// terminal error.
func collect(t *testing.T, output genx.Stream) ([]*genx.MessageChunk, error) {
	t.Helper()
	var chunks []*genx.MessageChunk
	done := make(chan error, 1)
	go func() {
		for {
			chunk, err := output.Next()
			if err != nil {
				done <- err
				return
			}
			chunks = append(chunks, chunk)
		}
	}()
	select {
	case err := <-done:
		return chunks, err
	case <-time.After(5 * time.Second):
		t.Fatal("output stream did not end")
		return nil, nil
	}
}

func waitFor(t *testing.T, condition func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func opusChunk(payload ...byte) *genx.MessageChunk {
	return &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: payload}}
}

func TestTransformFailsClosedOnBindingErrors(t *testing.T) {
	h := newHarness(t, Config{})
	ctx := context.Background()
	if _, err := h.agent.Transform(ctx, newOutputStream()); err == nil {
		t.Fatal("Transform() without peer identity succeeded")
	}
	if _, err := h.agent.Transform(gizlog.WithPeerPublicKey(ctx, "stranger"), newOutputStream()); !errors.Is(err, ErrNotMember) {
		t.Fatalf("Transform(stranger) error = %v, want ErrNotMember", err)
	}
	h.resolver.set(func(r *fakeResolver) { r.err = ErrRevoked })
	if _, err := h.agent.Transform(gizlog.WithPeerPublicKey(ctx, testPeer), newOutputStream()); !errors.Is(err, ErrRevoked) {
		t.Fatalf("Transform(revoked) error = %v, want ErrRevoked", err)
	}
	h.resolver.set(func(r *fakeResolver) { r.err = nil; r.binding.SFU.RoomToken = "" })
	if _, err := h.agent.Transform(gizlog.WithPeerPublicKey(ctx, testPeer), newOutputStream()); !errors.Is(err, ErrNotBound) {
		t.Fatalf("Transform(unbound) error = %v, want ErrNotBound", err)
	}
	if h.connector.count() != 0 {
		t.Fatalf("connector joined %d rooms, want 0", h.connector.count())
	}
}

func TestTransformUplinkForwardsOpusFrames(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, input := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	if client == nil {
		t.Fatal("connector did not join")
	}
	if client.params.Identity != testPeer || client.params.Room != "room-1" || client.params.URL != "wss://sfu.test" {
		t.Fatalf("connect params = %+v", client.params)
	}
	input.push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "in", BeginOfStream: true}})
	input.push(opusChunk(0x78, 0x01))
	input.push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("ignored")})
	input.push(&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}})
	input.push(opusChunk(0xFB, 0x03, 0xAA))
	input.push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "in", EndOfStream: true}})
	waitFor(t, func() bool { return client.sampleCount() == 2 }, "uplink samples")
	client.mu.Lock()
	first, second := client.samples[0], client.samples[1]
	client.mu.Unlock()
	if first.Duration != 20*time.Millisecond || second.Duration != 60*time.Millisecond {
		t.Fatalf("sample durations = %s, %s", first.Duration, second.Duration)
	}
	if client.disconnects() != 0 {
		t.Fatal("BOS/EOS touched the connection")
	}
	cancel()
	chunks, err := collect(t, output)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("output ended with %v, want EOF", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("output chunks = %d, want 0", len(chunks))
	}
	waitFor(t, func() bool { return client.disconnects() == 1 }, "disconnect")
	if _, attached := h.agent.SessionStatus(testPeer); attached {
		t.Fatal("session still registered after cancel")
	}
}

func TestTransformDownlinkRoutesPerParticipant(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	client := h.connector.client(0)

	readerB := newFakeReader()
	readerC := newFakeReader()
	client.events.onTrackSubscribed(testRemote, "TR_b", readerB)
	client.events.onTrackSubscribed("peer-c", "TR_c", readerC)
	// The local participant's own track never becomes a route.
	client.events.onTrackSubscribed(testPeer, "TR_self", newFakeReader())

	readerB.send(1, 0, 0x78, 0xB1, 0x00, 0x00)
	readerB.send(3, 1920, 0x78, 0xB3, 0x00, 0x00)
	readerB.send(2, 960, 0x78, 0xB2, 0x00, 0x00)
	readerC.send(7, 0, 0x78, 0xC1, 0x00, 0x00)
	// B: BOS + 3 payloads, C: BOS + 1 payload.
	waitFor(t, func() bool { return h.queued(testPeer) == 6 }, "downlink chunks")
	readerB.close()
	client.events.onTrackUnsubscribed("TR_b")
	waitFor(t, func() bool { return h.routeCount(testPeer) == 1 }, "route B to close")
	client.events.onParticipantDisconnected("peer-c")
	waitFor(t, func() bool { return h.routeCount(testPeer) == 0 }, "route C to close")
	cancel()
	chunks, err := collect(t, output)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("output ended with %v, want EOF", err)
	}

	type route struct {
		label   string
		bos     bool
		eos     bool
		payload [][]byte
	}
	routes := map[string]*route{}
	order := []string{}
	for _, chunk := range chunks {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID == "" {
			t.Fatalf("chunk without stream id: %+v", chunk)
		}
		if chunk.Name != chunk.Ctrl.Label {
			t.Fatalf("chunk name %q != label %q", chunk.Name, chunk.Ctrl.Label)
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || blob.MIMEType != "audio/opus" {
			t.Fatalf("chunk part = %#v", chunk.Part)
		}
		r := routes[chunk.Ctrl.StreamID]
		if r == nil {
			if !chunk.Ctrl.BeginOfStream {
				t.Fatalf("stream %s did not start with BOS", chunk.Ctrl.StreamID)
			}
			r = &route{label: chunk.Ctrl.Label}
			routes[chunk.Ctrl.StreamID] = r
			order = append(order, chunk.Ctrl.StreamID)
		}
		if chunk.Ctrl.Label != r.label {
			t.Fatalf("stream %s changed label", chunk.Ctrl.StreamID)
		}
		switch {
		case chunk.Ctrl.BeginOfStream:
			r.bos = true
		case chunk.Ctrl.EndOfStream:
			r.eos = true
		default:
			r.payload = append(r.payload, blob.Data)
		}
	}
	if len(routes) != 2 {
		t.Fatalf("routes = %d, want 2 (%v)", len(routes), order)
	}
	labels := map[string]int{}
	for id, r := range routes {
		labels[r.label]++
		if !r.bos || !r.eos {
			t.Fatalf("stream %s bos=%v eos=%v", id, r.bos, r.eos)
		}
		switch r.label {
		case testRemote:
			if len(r.payload) != 3 || r.payload[0][1] != 0xB1 || r.payload[1][1] != 0xB2 || r.payload[2][1] != 0xB3 {
				t.Fatalf("route B payloads = %x", r.payload)
			}
		case "peer-c":
			if len(r.payload) != 1 || r.payload[0][1] != 0xC1 {
				t.Fatalf("route C payloads = %x", r.payload)
			}
		default:
			t.Fatalf("unexpected label %q", r.label)
		}
	}
	if labels[testRemote] != 1 || labels["peer-c"] != 1 {
		t.Fatalf("labels = %v", labels)
	}
}

func TestTransformMuteOpensNewStreamID(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	reader := newFakeReader()
	client.events.onTrackSubscribed(testRemote, "TR_b", reader)
	reader.send(1, 0, 0x78, 0x01, 0x00, 0x00)
	waitFor(t, func() bool { return h.queued(testPeer) == 2 }, "first burst")
	client.events.onTrackMuted("TR_b")
	reader.send(2, 960, 0x78, 0x02, 0x00, 0x00)
	waitFor(t, func() bool { return h.queued(testPeer) == 5 }, "second burst")
	cancel()
	chunks, _ := collect(t, output)
	if len(chunks) != 6 {
		t.Fatalf("chunks = %d, want 6", len(chunks))
	}
	first, second := chunks[0].Ctrl.StreamID, chunks[3].Ctrl.StreamID
	if first == second {
		t.Fatalf("mute reused stream id %s", first)
	}
	if !chunks[2].Ctrl.EndOfStream || chunks[2].Ctrl.StreamID != first {
		t.Fatalf("mute did not close first stream: %+v", chunks[2].Ctrl)
	}
	if !chunks[3].Ctrl.BeginOfStream || !chunks[5].Ctrl.EndOfStream || chunks[5].Ctrl.StreamID != second {
		t.Fatalf("second burst boundaries wrong: %+v %+v", chunks[3].Ctrl, chunks[5].Ctrl)
	}
	if chunks[0].Ctrl.Label != testRemote || chunks[3].Ctrl.Label != testRemote {
		t.Fatal("label changed across bursts")
	}
}

// TestTransformSilenceFramesNeverOpenBurstAndIdleClosesIt covers the
// mix-minus-self downlink: an idle publisher only yields Opus silence frames,
// which must not open a device route, and a burst closes on its own once the
// speaker goes quiet so the mixer stops feeding the device.
func TestTransformSilenceFramesNeverOpenBurstAndIdleClosesIt(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	reader := newFakeReader()
	client.events.onTrackSubscribed(testRemote, "TR_b", reader)

	reader.send(1, 0, 0xf8, 0xff, 0xfe)
	reader.send(2, 960, 0xf8)
	// The SFU zero-pads the silence frame for an idle publisher.
	reader.send(2, 1920, append([]byte{0xf8, 0xff, 0xfe}, make([]byte, 77)...)...)
	time.Sleep(2 * routeIdleFlush)
	if got := h.queued(testPeer); got != 0 {
		t.Fatalf("silence frames opened a burst: queued = %d", got)
	}
	reader.send(3, 2880, 0x78, 0x01, 0x00, 0x00)
	reader.send(4, 3840, 0xf8, 0xff, 0xfe)
	waitFor(t, func() bool { return h.queued(testPeer) == 3 }, "voiced burst with trailing silence")
	waitFor(t, func() bool { return h.queued(testPeer) == 4 }, "burst idle EOS")
	reader.send(5, 4800, 0xf8, 0xff, 0xfe)
	time.Sleep(2 * routeIdleFlush)
	if got := h.queued(testPeer); got != 4 {
		t.Fatalf("silence after the burst reopened it: queued = %d", got)
	}
	reader.send(6, 5760, 0x78, 0x02, 0x00, 0x00)
	waitFor(t, func() bool { return h.queued(testPeer) == 6 }, "second burst")
	cancel()
	chunks, _ := collect(t, output)
	if len(chunks) != 7 {
		t.Fatalf("chunks = %d, want 7", len(chunks))
	}
	if !chunks[0].Ctrl.BeginOfStream || !chunks[3].Ctrl.EndOfStream || chunks[3].Ctrl.StreamID != chunks[0].Ctrl.StreamID {
		t.Fatalf("first burst boundaries wrong: %+v %+v", chunks[0].Ctrl, chunks[3].Ctrl)
	}
	if !chunks[4].Ctrl.BeginOfStream || chunks[4].Ctrl.StreamID == chunks[0].Ctrl.StreamID {
		t.Fatalf("second burst did not open a fresh stream: %+v", chunks[4].Ctrl)
	}
	if payload := chunks[2].Part.(*genx.Blob).Data; len(payload) != 3 {
		t.Fatalf("trailing silence inside the burst was not forwarded: %x", payload)
	}
}

// TestTransformShortVoicedPacketsOpenBurst pins the boundary between silence
// and speech: a valid Opus frame can be two or three bytes at a low bitrate,
// so only the identifiable silence encodings may be held back.
func TestTransformShortVoicedPacketsOpenBurst(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	reader := newFakeReader()
	client.events.onTrackSubscribed(testRemote, "TR_b", reader)

	reader.send(1, 0, 0x78, 0x01)
	reader.send(2, 960, 0x78, 0x02, 0x03)
	waitFor(t, func() bool { return h.queued(testPeer) == 3 }, "burst opened by short voiced packets")
	cancel()
	chunks, _ := collect(t, output)
	if len(chunks) < 3 || !chunks[0].Ctrl.BeginOfStream {
		t.Fatalf("short voiced packets did not open a burst: %d chunks", len(chunks))
	}
	if payload := chunks[1].Part.(*genx.Blob).Data; len(payload) != 2 {
		t.Fatalf("first forwarded payload = %x, want the two-byte frame", payload)
	}
	if payload := chunks[2].Part.(*genx.Blob).Data; len(payload) != 3 {
		t.Fatalf("second forwarded payload = %x, want the three-byte frame", payload)
	}
}

func TestIsOpusSilenceFrame(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		payload []byte
		silence bool
	}{
		"empty payload":             {payload: nil, silence: true},
		"bare toc dtx":              {payload: []byte{0xf8}, silence: true},
		"canonical silence":         {payload: []byte{0xf8, 0xff, 0xfe}, silence: true},
		"padded silence":            {payload: append([]byte{0xf8, 0xff, 0xfe}, make([]byte, 77)...), silence: true},
		"two byte speech":           {payload: []byte{0x78, 0x01}, silence: false},
		"three byte speech":         {payload: []byte{0x78, 0x01, 0x02}, silence: false},
		"silence toc with speech":   {payload: []byte{0xf8, 0xff, 0xfe, 0x00, 0x11}, silence: false},
		"two byte silence prefix":   {payload: []byte{0xf8, 0xff}, silence: false},
		"long frame on silence toc": {payload: []byte{0xf8, 0x01, 0x02, 0x03}, silence: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := isOpusSilenceFrame(tc.payload); got != tc.silence {
				t.Fatalf("isOpusSilenceFrame(%x) = %v, want %v", tc.payload, got, tc.silence)
			}
		})
	}
}

func TestTransformRevokesOnGenerationChange(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, input := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	h.resolver.set(func(r *fakeResolver) { r.binding.SFU.Generation = 2 })
	_, err := collect(t, output)
	if !errors.Is(err, ErrRevoked) {
		t.Fatalf("output ended with %v, want ErrRevoked", err)
	}
	waitFor(t, func() bool { return client.disconnects() == 1 }, "disconnect")
	before := client.sampleCount()
	input.push(opusChunk(0x78, 0x01))
	time.Sleep(20 * time.Millisecond)
	if client.sampleCount() != before {
		t.Fatal("audio forwarded after revocation")
	}
	status, attached := h.agent.SessionStatus(testPeer)
	if attached {
		t.Fatalf("session still attached: %+v", status)
	}
}

// Membership loss alone ends the session: the Social service pushes nothing,
// so the periodic re-validation is the only mechanism that stops forwarding.
func TestTransformRevokesWhenMembershipIsLost(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, input := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	h.resolver.set(func(r *fakeResolver) {
		r.binding.Members = slices.DeleteFunc(
			slices.Clone(r.binding.Members),
			func(member string) bool { return member == testPeer },
		)
	})
	_, err := collect(t, output)
	if !errors.Is(err, ErrRevoked) {
		t.Fatalf("output ended with %v, want ErrRevoked", err)
	}
	waitFor(t, func() bool { return client.disconnects() == 1 }, "disconnect")
	before := client.sampleCount()
	input.push(opusChunk(0x78, 0x01))
	time.Sleep(20 * time.Millisecond)
	if client.sampleCount() != before {
		t.Fatal("audio forwarded after the membership was revoked")
	}
	if status, attached := h.agent.SessionStatus(testPeer); attached {
		t.Fatalf("session still attached: %+v", status)
	}
}

func TestTransformRevokesOnResolverError(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	h.resolver.set(func(r *fakeResolver) { r.err = errors.New("kv unavailable") })
	_, err := collect(t, output)
	if !errors.Is(err, ErrRevoked) {
		t.Fatalf("output ended with %v, want ErrRevoked", err)
	}
	waitFor(t, func() bool { return h.connector.client(0).disconnects() == 1 }, "disconnect")
}

func TestTransformDuplicateIdentityTerminatesWithoutReconnect(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	client.events.onDisconnected(disconnectDuplicateIdentity)
	_, err := collect(t, output)
	if !errors.Is(err, ErrDuplicateIdentity) {
		t.Fatalf("output ended with %v, want ErrDuplicateIdentity", err)
	}
	waitFor(t, func() bool { return client.disconnects() == 1 }, "disconnect")
	if h.connector.count() != 1 {
		t.Fatalf("connector reconnected: %d joins", h.connector.count())
	}
}

func TestTransformReconnectsAfterNetworkDisconnect(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, input := h.attach(ctx, testPeer)
	first := h.connector.client(0)
	reader := newFakeReader()
	first.events.onTrackSubscribed(testRemote, "TR_b", reader)
	reader.send(1, 0, 0x78, 0x01, 0x00, 0x00)
	waitFor(t, func() bool { return h.queued(testPeer) == 2 }, "downlink before disconnect")

	first.events.onDisconnected(disconnectOther)
	waitFor(t, func() bool { return h.connector.count() == 2 }, "reconnect")
	second := h.connector.client(1)
	waitFor(t, func() bool {
		status, _ := h.agent.SessionStatus(testPeer)
		return status.State == StateConnected
	}, "connected state")
	if first.disconnects() != 1 {
		t.Fatalf("old client disconnects = %d, want 1", first.disconnects())
	}
	input.push(opusChunk(0x78, 0x02))
	waitFor(t, func() bool { return second.sampleCount() == 1 }, "uplink on new client")
	if first.sampleCount() != 0 {
		t.Fatal("uplink written to stale client")
	}
	cancel()
	chunks, err := collect(t, output)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("output ended with %v, want EOF", err)
	}
	// The old route was closed on reconnect: BOS, payload, EOS.
	if len(chunks) != 3 || !chunks[2].Ctrl.EndOfStream {
		t.Fatalf("chunks = %d, want BOS/payload/EOS", len(chunks))
	}
	waitFor(t, func() bool { return second.disconnects() == 1 }, "final disconnect")
}

// The first reconnect attempt must not race the SFU's own teardown: a Room
// that is closing still accepts joins, and two runtimes that re-dial instantly
// can land in different Room instances of the same name.
func TestTransformReconnectWaitsForTheSFUToSettle(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	h.attach(ctx, testPeer)
	first := h.connector.client(0)
	started := time.Now()
	first.events.onDisconnected(disconnectOther)
	if count := h.connector.count(); count != 1 {
		t.Fatalf("connector joins immediately after the disconnect = %d, want 1", count)
	}
	waitFor(t, func() bool { return h.connector.count() == 2 }, "reconnect")
	if elapsed := time.Since(started); elapsed < reconnectMinBackoff {
		t.Fatalf("reconnected after %s, want at least %s", elapsed, reconnectMinBackoff)
	}
}

func TestTransformReconnectGivesUpAfterTimeout(t *testing.T) {
	h := newHarness(t, Config{ReconnectTimeout: 600 * time.Millisecond})
	joinErr := errors.New("sfu down")
	h.connector.fail = func(attempt int) error {
		if attempt > 1 {
			return joinErr
		}
		return nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	h.connector.client(0).events.onDisconnected(disconnectOther)
	waitFor(t, func() bool {
		status, _ := h.agent.SessionStatus(testPeer)
		return status.State == StateReconnecting
	}, "reconnecting state")
	state, err := h.agent.Status(gizlog.WithPeerPublicKey(ctx, testPeer))
	if err != nil || state.RuntimeState != apitypes.PeerRunStatusStateStarting || state.Message == nil {
		t.Fatalf("Status() = %+v, %v", state, err)
	}
	_, err = collect(t, output)
	if !errors.Is(err, joinErr) {
		t.Fatalf("output ended with %v, want reconnect timeout wrapping %v", err, joinErr)
	}
	if h.connector.count() != 1 {
		t.Fatalf("joins = %d, want 1", h.connector.count())
	}
}

func TestTransformReplacesPreviousAttachmentForSamePeer(t *testing.T) {
	h := newHarness(t, Config{})
	ctx1, cancel1 := context.WithCancel(t.Context())
	defer cancel1()
	output1, _ := h.attach(ctx1, testPeer)
	first := h.connector.client(0)

	ctx2, cancel2 := context.WithCancel(t.Context())
	defer cancel2()
	output2, _ := h.attach(ctx2, testPeer)
	if _, err := collect(t, output1); !errors.Is(err, io.EOF) {
		t.Fatalf("first output ended with %v, want EOF", err)
	}
	if first.disconnects() != 1 {
		t.Fatalf("first client disconnects = %d, want 1", first.disconnects())
	}
	if h.connector.count() != 2 {
		t.Fatalf("joins = %d, want 2", h.connector.count())
	}
	status, attached := h.agent.SessionStatus(testPeer)
	if !attached || status.State != StateConnected {
		t.Fatalf("status = %+v attached=%v", status, attached)
	}
	cancel2()
	if _, err := collect(t, output2); !errors.Is(err, io.EOF) {
		t.Fatalf("second output ended with %v, want EOF", err)
	}
}

func TestAgentStatusWithoutAttachment(t *testing.T) {
	h := newHarness(t, Config{})
	state, err := h.agent.Status(context.Background())
	if err != nil || state.RuntimeState != apitypes.PeerRunStatusStateRunning || state.AgentType == nil || *state.AgentType != Type {
		t.Fatalf("Status() = %+v, %v", state, err)
	}
	history, err := h.agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
	if err != nil || history.Available || len(history.Items) != 0 {
		t.Fatalf("ListHistory() = %+v, %v", history, err)
	}
}

func TestNewAgentRequiresConfiguration(t *testing.T) {
	resolver := &fakeResolver{}
	spec := agenthost.Spec{Workspace: apitypes.Workspace{Id: testWorkspaceID}}
	if _, err := (Factory{Bindings: resolver}).NewAgent(context.Background(), spec); err == nil {
		t.Fatal("NewAgent() without services.sfu succeeded")
	}
	if _, err := (Factory{Config: Config{URL: "wss://x", APIKey: "k", APISecret: "s"}}).NewAgent(context.Background(), spec); err == nil {
		t.Fatal("NewAgent() without resolver succeeded")
	}
	agent, err := (Factory{Config: Config{URL: "wss://x", APIKey: "k", APISecret: "s"}, Bindings: resolver}).NewAgent(context.Background(), spec)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if got := agent.(*Agent).config; got.RecheckInterval != DefaultRecheckInterval || got.ReconnectTimeout != DefaultReconnectTimeout {
		t.Fatalf("defaults = %+v", got)
	}
}
