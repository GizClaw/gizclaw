package sfu

import (
	"context"
	"encoding/json"
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
	testRemoteC     = "peer-c"
	// testTalkHangover and testFloorIdle keep the timer-driven cases fast
	// while staying far above the polling granularity of waitFor.
	testTalkHangover = 80 * time.Millisecond
	testFloorIdle    = 60 * time.Millisecond
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
	published    []talkMessage
	topics       []string
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

func (c *fakeClient) PublishData(topic string, payload []byte) error {
	message, err := decodeTalkMessage(payload)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.published = append(c.published, message)
	c.topics = append(c.topics, topic)
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

func (c *fakeClient) talk() []talkMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.published)
}

func (c *fakeClient) talkCount() int {
	return len(c.talk())
}

func (c *fakeClient) disconnects() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disconnected
}

// remoteTalk injects a talk message as if participant identity published it.
func (c *fakeClient) remoteTalk(identity, kind, utterance string, seq uint64) {
	payload, err := talkMessage{V: talkProtocolVersion, Type: kind, Utterance: utterance, Seq: seq}.encode()
	if err != nil {
		panic(err)
	}
	c.events.onDataPacket(identity, talkTopic, payload)
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

// fakeReader feeds RTP packets to a remote track until closed.
type fakeReader struct {
	packets chan *rtp.Packet
	closed  chan struct{}
	once    sync.Once
	seq     uint16
}

func newFakeReader() *fakeReader {
	return &fakeReader{packets: make(chan *rtp.Packet, 256), closed: make(chan struct{})}
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

// voiced sends the next in-order voiced packet tagged with marker.
func (r *fakeReader) voiced(marker byte) {
	r.seq++
	r.send(r.seq, uint32(r.seq)*960, 0x78, marker, 0x00, 0x00)
}

// silence sends the next in-order Opus silence packet.
func (r *fakeReader) silence() {
	r.seq++
	r.send(r.seq, uint32(r.seq)*960, 0xf8, 0xff, 0xfe)
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
	if config.TalkHangover == 0 {
		config.TalkHangover = testTalkHangover
	}
	if config.FloorIdle == 0 {
		config.FloorIdle = testFloorIdle
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

func (h *harness) trackCount(peer string) int {
	s := h.session(peer)
	if s == nil {
		return -1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tracks)
}

func (h *harness) floorHolder(peer string) string {
	s := h.session(peer)
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.floor == nil {
		return ""
	}
	return s.floor.identity
}

func (h *harness) talking(peer string) bool {
	s := h.session(peer)
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.talk.open
}

func (h *harness) dropped(peer string) uint64 {
	status, _ := h.agent.SessionStatus(peer)
	return status.DroppedPackets
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

func voicedChunk(marker byte) *genx.MessageChunk {
	return opusChunk(0x78, marker)
}

func silenceChunk() *genx.MessageChunk {
	return opusChunk(0xf8, 0xff, 0xfe)
}

func inputBOS(streamID string) *genx.MessageChunk {
	return &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true}}
}

func inputEOS(streamID string) *genx.MessageChunk {
	return &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}}
}

// streams groups downlink chunks by stream_id in first-seen order.
type streamRecord struct {
	id      string
	label   string
	name    string
	bos     bool
	eos     bool
	payload [][]byte
}

func groupStreams(t *testing.T, chunks []*genx.MessageChunk) []*streamRecord {
	t.Helper()
	var order []*streamRecord
	byID := map[string]*streamRecord{}
	for _, chunk := range chunks {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID == "" {
			t.Fatalf("chunk without stream id: %+v", chunk)
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || blob.MIMEType != agenthost.OpusPassthroughMIME {
			t.Fatalf("chunk part = %#v, want passthrough Opus blob", chunk.Part)
		}
		record := byID[chunk.Ctrl.StreamID]
		if record == nil {
			if !chunk.Ctrl.BeginOfStream {
				t.Fatalf("stream %s did not start with BOS", chunk.Ctrl.StreamID)
			}
			record = &streamRecord{id: chunk.Ctrl.StreamID, label: chunk.Ctrl.Label, name: chunk.Name}
			byID[chunk.Ctrl.StreamID] = record
			order = append(order, record)
		}
		if chunk.Ctrl.Label != record.label || chunk.Name != record.name {
			t.Fatalf("stream %s changed label or name", chunk.Ctrl.StreamID)
		}
		switch {
		case chunk.Ctrl.BeginOfStream:
			if record.bos {
				t.Fatalf("stream %s has two BOS", record.id)
			}
			record.bos = true
		case chunk.Ctrl.EndOfStream:
			if record.eos {
				t.Fatalf("stream %s has two EOS", record.id)
			}
			record.eos = true
		default:
			if record.eos {
				t.Fatalf("stream %s carried payload after EOS", record.id)
			}
			record.payload = append(record.payload, blob.Data)
		}
	}
	return order
}

func markers(payload [][]byte) []byte {
	out := make([]byte, 0, len(payload))
	for _, packet := range payload {
		out = append(out, packet[1])
	}
	return out
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
	input.push(inputBOS("in"))
	input.push(opusChunk(0x78, 0x01))
	input.push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("ignored")})
	input.push(&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}})
	input.push(opusChunk(0xFB, 0x03, 0xAA))
	input.push(inputEOS("in"))
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

// TestUplinkPushToTalkSegmentsPerPress covers the push-to-talk input mode:
// the device brackets every press with BOS and EOS. The utterance opens on
// the first voiced frame, closes on the device EOS well before the hangover
// could fire, and every press publishes exactly one bos/eos pair with a new
// utterance id and increasing seq.
func TestUplinkPushToTalkSegmentsPerPress(t *testing.T) {
	h := newHarness(t, Config{TalkHangover: 5 * time.Second})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, input := h.attach(ctx, testPeer)
	client := h.connector.client(0)

	input.push(inputBOS("press-1"))
	input.push(silenceChunk())
	time.Sleep(20 * time.Millisecond)
	if client.talkCount() != 0 || client.sampleCount() != 0 {
		t.Fatalf("BOS plus silence published %d talk messages and wrote %d samples, want 0/0", client.talkCount(), client.sampleCount())
	}
	input.push(voicedChunk(0x01))
	waitFor(t, func() bool { return client.talkCount() == 1 }, "press-1 bos")
	input.push(voicedChunk(0x02))
	input.push(silenceChunk())
	input.push(inputEOS("press-1"))
	waitFor(t, func() bool { return client.talkCount() == 2 }, "press-1 eos")
	if h.talking(testPeer) {
		t.Fatal("utterance still open after the device EOS")
	}
	waitFor(t, func() bool { return client.sampleCount() == 3 }, "press-1 samples")

	input.push(inputBOS("press-2"))
	input.push(voicedChunk(0x03))
	input.push(inputEOS("press-2"))
	waitFor(t, func() bool { return client.talkCount() == 4 }, "press-2 bos/eos")
	time.Sleep(30 * time.Millisecond)
	if client.talkCount() != 4 {
		t.Fatalf("talk messages = %d, want exactly bos/eos per press", client.talkCount())
	}
	talk := client.talk()
	for i, message := range talk {
		if message.Seq != uint64(i+1) {
			t.Fatalf("talk[%d].seq = %d, want %d", i, message.Seq, i+1)
		}
		if message.V != talkProtocolVersion {
			t.Fatalf("talk[%d].v = %d", i, message.V)
		}
	}
	if talk[0].Type != talkTypeBOS || talk[1].Type != talkTypeEOS || talk[2].Type != talkTypeBOS || talk[3].Type != talkTypeEOS {
		t.Fatalf("talk types = %+v", talk)
	}
	if talk[0].Utterance != talk[1].Utterance || talk[2].Utterance != talk[3].Utterance {
		t.Fatalf("utterance ids differ within a press: %+v", talk)
	}
	if talk[0].Utterance == talk[2].Utterance {
		t.Fatal("second press reused the first utterance id")
	}
	for _, topic := range client.topics {
		if topic != talkTopic {
			t.Fatalf("published on topic %q, want %q", topic, talkTopic)
		}
	}
	if client.disconnects() != 0 {
		t.Fatal("device BOS/EOS touched the connection")
	}
}

// TestUplinkRealtimeHangoverSegments covers the realtime input mode: one
// BOS, a continuous stream and never an EOS. Silence alone opens nothing,
// the hangover closes the utterance once voiced frames stop, speaking again
// opens a new utterance, and a continuous voiced stream keeps exactly one
// utterance open.
func TestUplinkRealtimeHangoverSegments(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, input := h.attach(ctx, testPeer)
	client := h.connector.client(0)

	input.push(inputBOS("live"))
	for range 3 {
		input.push(silenceChunk())
	}
	time.Sleep(2 * testTalkHangover)
	if client.talkCount() != 0 {
		t.Fatalf("silence frames published %d talk messages", client.talkCount())
	}
	if client.sampleCount() != 0 {
		t.Fatal("silence frames were written to the track while no utterance was open")
	}

	input.push(voicedChunk(0x01))
	input.push(silenceChunk())
	waitFor(t, func() bool { return client.talkCount() == 1 && client.sampleCount() == 2 }, "first utterance bos")
	if !h.talking(testPeer) {
		t.Fatal("utterance not open after a voiced frame")
	}
	waitFor(t, func() bool { return client.talkCount() == 2 }, "hangover eos")
	if h.talking(testPeer) {
		t.Fatal("utterance still open after the hangover")
	}
	input.push(silenceChunk())
	time.Sleep(20 * time.Millisecond)
	if client.sampleCount() != 2 {
		t.Fatal("silence after the hangover was written to the track")
	}

	// A continuous voiced stream (frames well inside the hangover) keeps one
	// utterance open without flapping.
	stop := time.Now().Add(3 * testTalkHangover)
	for time.Now().Before(stop) {
		input.push(voicedChunk(0x02))
		time.Sleep(testTalkHangover / 8)
	}
	waitFor(t, func() bool { return client.talkCount() == 3 }, "second utterance bos")
	if client.talkCount() != 3 || !h.talking(testPeer) {
		t.Fatalf("continuous stream flapped: %d talk messages", client.talkCount())
	}
	waitFor(t, func() bool { return client.talkCount() == 4 }, "second utterance eos")
	talk := client.talk()
	if talk[0].Type != talkTypeBOS || talk[1].Type != talkTypeEOS || talk[2].Type != talkTypeBOS || talk[3].Type != talkTypeEOS {
		t.Fatalf("talk types = %+v", talk)
	}
	if talk[0].Utterance != talk[1].Utterance || talk[2].Utterance != talk[3].Utterance || talk[0].Utterance == talk[2].Utterance {
		t.Fatalf("utterance ids = %+v", talk)
	}
	for i := 1; i < len(talk); i++ {
		if talk[i].Seq <= talk[i-1].Seq {
			t.Fatalf("seq not increasing: %+v", talk)
		}
	}
}

// subscribeRemote attaches a remote participant's track and announces its
// open utterance, then waits for the floor to be granted to it.
func subscribeRemote(t *testing.T, h *harness, client *fakeClient, identity, trackID, utterance string, seq uint64) *fakeReader {
	t.Helper()
	reader := newFakeReader()
	client.events.onTrackSubscribed(identity, trackID, reader)
	client.remoteTalk(identity, talkTypeBOS, utterance, seq)
	return reader
}

// TestHalfDuplexPushToTalkDropsDownlinkWhileTalking: the local press releases
// the floor and drops every remote packet until the device EOS closes the
// utterance, after which the still-open remote utterance retakes the floor
// on a fresh stream.
func TestHalfDuplexPushToTalkDropsDownlinkWhileTalking(t *testing.T) {
	h := newHarness(t, Config{TalkHangover: 5 * time.Second})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, input := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	reader := subscribeRemote(t, h, client, testRemote, "TR_b", "u-b", 1)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor granted")
	reader.voiced(0xB1)
	waitFor(t, func() bool { return h.queued(testPeer) == 2 }, "remote packet forwarded")

	input.push(inputBOS("press"))
	input.push(voicedChunk(0x01))
	waitFor(t, func() bool { return h.floorHolder(testPeer) == "" }, "floor released by local talk")
	waitFor(t, func() bool { return h.queued(testPeer) == 3 }, "floor EOS")
	for i := range maxPrerollPackets + 4 {
		reader.voiced(byte(0xC0 + i))
	}
	waitFor(t, func() bool { return h.dropped(testPeer) >= 4 }, "remote packets dropped while talking")
	// Idle the preroll window so the resume below only carries new packets.
	time.Sleep(2 * testFloorIdle)
	if h.queued(testPeer) != 3 || h.floorHolder(testPeer) != "" {
		t.Fatalf("downlink continued while talking: queued=%d holder=%q", h.queued(testPeer), h.floorHolder(testPeer))
	}

	input.push(inputEOS("press"))
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor retaken after EOS")
	reader.voiced(0xD1)
	waitFor(t, func() bool { return h.queued(testPeer) >= 5 }, "downlink resumed")
	cancel()
	chunks, _ := collect(t, output)
	streams := groupStreams(t, chunks)
	if len(streams) != 2 || streams[0].id == streams[1].id {
		t.Fatalf("streams = %d, want two distinct holds", len(streams))
	}
	if got := markers(streams[0].payload); !slices.Equal(got, []byte{0xB1}) {
		t.Fatalf("first hold payload markers = %x", got)
	}
	if got := markers(streams[1].payload); !slices.Contains(got, 0xD1) || slices.ContainsFunc(got, func(m byte) bool { return m >= 0xC0 && m < 0xD0 }) {
		t.Fatalf("second hold carried packets from the talking window: %x", got)
	}
}

// TestHalfDuplexRealtimeResumesAfterHangover: in realtime mode the device
// never sends EOS, so the downlink resumes when the talk hangover closes the
// local utterance.
func TestHalfDuplexRealtimeResumesAfterHangover(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, input := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	reader := subscribeRemote(t, h, client, testRemote, "TR_b", "u-b", 1)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor granted")
	input.push(inputBOS("live"))
	input.push(voicedChunk(0x01))
	waitFor(t, func() bool { return h.floorHolder(testPeer) == "" && h.talking(testPeer) }, "floor released by local talk")
	reader.voiced(0xB1)
	time.Sleep(20 * time.Millisecond)
	if got := h.queued(testPeer); got != 2 {
		t.Fatalf("queued while talking = %d, want BOS/EOS only", got)
	}
	waitFor(t, func() bool { return !h.talking(testPeer) }, "hangover closed the utterance")
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor retaken after the hangover")
	reader.voiced(0xB2)
	waitFor(t, func() bool { return h.queued(testPeer) >= 4 }, "downlink resumed")
}

// TestFloorLocksToFirstBOSAndDropsOthers: with two remote utterances open,
// only the earlier one's packets reach the device; the other participant's
// voiced packets are dropped and counted.
func TestFloorLocksToFirstBOSAndDropsOthers(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	readerB := subscribeRemote(t, h, client, testRemote, "TR_b", "u-b", 1)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor to B")
	readerC := subscribeRemote(t, h, client, testRemoteC, "TR_c", "u-c", 7)
	// The local participant's own track never becomes a route.
	client.events.onTrackSubscribed(testPeer, "TR_self", newFakeReader())

	readerB.send(1, 0, 0x78, 0xB1, 0x00, 0x00)
	readerB.send(3, 1920, 0x78, 0xB3, 0x00, 0x00)
	readerB.send(2, 960, 0x78, 0xB2, 0x00, 0x00)
	for i := range maxPrerollPackets + 2 {
		readerC.voiced(byte(0xC0 + i))
	}
	waitFor(t, func() bool { return h.dropped(testPeer) == 2 }, "C packets dropped")
	waitFor(t, func() bool { return h.queued(testPeer) == 4 }, "B packets forwarded")
	if h.floorHolder(testPeer) != testRemote {
		t.Fatalf("floor holder = %q, want B", h.floorHolder(testPeer))
	}
	if got := h.trackCount(testPeer); got != 2 {
		t.Fatalf("tracks = %d, want 2", got)
	}
	cancel()
	chunks, err := collect(t, output)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("output ended with %v, want EOF", err)
	}
	streams := groupStreams(t, chunks)
	if len(streams) != 1 || streams[0].label != testRemote || streams[0].name != testRemote || !streams[0].bos || !streams[0].eos {
		t.Fatalf("streams = %+v", streams)
	}
	if got := markers(streams[0].payload); !slices.Equal(got, []byte{0xB1, 0xB2, 0xB3}) {
		t.Fatalf("reordered payload markers = %x", got)
	}
}

// TestFloorHandsOffToEarliestOpenUtterance: the holder's EOS (a push-to-talk
// remote) releases the floor to the earliest utterance still open, not the
// newest one, and replays that participant's recent preroll.
func TestFloorHandsOffToEarliestOpenUtterance(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	readerB := subscribeRemote(t, h, client, testRemote, "TR_b", "u-b", 1)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor to B")
	readerC := subscribeRemote(t, h, client, testRemoteC, "TR_c", "u-c", 1)
	readerD := subscribeRemote(t, h, client, "peer-d", "TR_d", "u-d", 1)
	readerB.voiced(0xB1)
	waitFor(t, func() bool { return h.queued(testPeer) == 2 }, "B forwarded")
	readerC.voiced(0xC1)
	readerD.voiced(0xD1)
	time.Sleep(20 * time.Millisecond)
	client.remoteTalk(testRemote, talkTypeEOS, "u-b", 2)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemoteC }, "floor handed to C")
	readerC.voiced(0xC2)
	waitFor(t, func() bool { return h.queued(testPeer) >= 6 }, "C forwarded with preroll")
	client.remoteTalk(testRemoteC, talkTypeEOS, "u-c", 2)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == "peer-d" }, "floor handed to D")
	cancel()
	chunks, _ := collect(t, output)
	streams := groupStreams(t, chunks)
	if len(streams) != 3 || streams[0].label != testRemote || streams[1].label != testRemoteC || streams[2].label != "peer-d" {
		t.Fatalf("stream labels = %+v", streams)
	}
	if got := markers(streams[1].payload); !slices.Equal(got, []byte{0xC1, 0xC2}) {
		t.Fatalf("C payload markers = %x, want preroll then live packet", got)
	}
	for _, stream := range streams {
		if !stream.bos || !stream.eos {
			t.Fatalf("stream %s bos=%v eos=%v", stream.id, stream.bos, stream.eos)
		}
	}
}

// TestFloorReleasesOnIdleForRealtimeRemote: a realtime remote keeps its
// utterance open while it goes quiet. floor_idle releases the floor, a
// waiting utterance takes it, and the quiet participant is heard again on a
// fresh stream once it speaks with its utterance still open.
func TestFloorReleasesOnIdleForRealtimeRemote(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	readerB := subscribeRemote(t, h, client, testRemote, "TR_b", "u-b", 1)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor to B")
	readerB.voiced(0xB1)
	readerB.silence()
	waitFor(t, func() bool { return h.queued(testPeer) == 3 }, "B forwarded with trailing silence")
	waitFor(t, func() bool { return h.floorHolder(testPeer) == "" }, "floor idle release")
	if got := h.queued(testPeer); got != 4 {
		t.Fatalf("queued after idle release = %d, want BOS/2 packets/EOS", got)
	}
	// Silence alone does not retake the floor for the idle utterance.
	readerB.silence()
	time.Sleep(2 * testFloorIdle)
	if h.floorHolder(testPeer) != "" || h.queued(testPeer) != 4 {
		t.Fatalf("silence retook the floor: holder=%q queued=%d", h.floorHolder(testPeer), h.queued(testPeer))
	}
	readerC := subscribeRemote(t, h, client, testRemoteC, "TR_c", "u-c", 1)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemoteC }, "waiting utterance took the floor")
	readerC.voiced(0xC1)
	waitFor(t, func() bool { return h.queued(testPeer) == 6 }, "C forwarded")
	waitFor(t, func() bool { return h.floorHolder(testPeer) == "" }, "C idle release")
	readerB.voiced(0xB2)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "B heard again")
	cancel()
	chunks, _ := collect(t, output)
	streams := groupStreams(t, chunks)
	if len(streams) != 3 || streams[0].label != testRemote || streams[1].label != testRemoteC || streams[2].label != testRemote {
		t.Fatalf("stream labels = %+v", streams)
	}
	if streams[0].id == streams[2].id {
		t.Fatal("re-granted floor reused the stream id")
	}
	if got := markers(streams[2].payload); !slices.Equal(got, []byte{0xB2}) {
		t.Fatalf("second B hold markers = %x", got)
	}
}

func TestFloorReleasesOnMuteUnsubscribeAndLeave(t *testing.T) {
	for name, release := range map[string]func(client *fakeClient, reader *fakeReader){
		"mute":        func(client *fakeClient, _ *fakeReader) { client.events.onTrackMuted("TR_b") },
		"unsubscribe": func(client *fakeClient, _ *fakeReader) { client.events.onTrackUnsubscribed("TR_b") },
		"reader end":  func(_ *fakeClient, reader *fakeReader) { reader.close() },
		"leave":       func(client *fakeClient, _ *fakeReader) { client.events.onParticipantDisconnected(testRemote) },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, Config{FloorIdle: 5 * time.Second})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			output, _ := h.attach(ctx, testPeer)
			client := h.connector.client(0)
			readerB := subscribeRemote(t, h, client, testRemote, "TR_b", "u-b", 1)
			waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor to B")
			readerC := subscribeRemote(t, h, client, testRemoteC, "TR_c", "u-c", 1)
			readerB.voiced(0xB1)
			waitFor(t, func() bool { return h.queued(testPeer) == 2 }, "B forwarded")
			release(client, readerB)
			waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemoteC }, "floor handed to C")
			readerC.voiced(0xC1)
			waitFor(t, func() bool { return h.queued(testPeer) == 5 }, "C forwarded")
			if name != "mute" {
				waitFor(t, func() bool { return h.trackCount(testPeer) == 1 }, "B track detached")
			}
			cancel()
			chunks, _ := collect(t, output)
			streams := groupStreams(t, chunks)
			if len(streams) != 2 || streams[0].label != testRemote || streams[1].label != testRemoteC || !streams[0].eos {
				t.Fatalf("streams = %+v", streams)
			}
		})
	}
}

// TestFloorPrerollDeliversPacketsThatRacedTheBOS: media that reaches the
// listener a moment before its sender's BOS on the data channel is still
// delivered once the BOS grants the floor.
func TestFloorPrerollDeliversPacketsThatRacedTheBOS(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	reader := newFakeReader()
	client.events.onTrackSubscribed(testRemote, "TR_b", reader)
	reader.voiced(0xB1)
	reader.voiced(0xB2)
	time.Sleep(20 * time.Millisecond)
	if h.queued(testPeer) != 0 || h.floorHolder(testPeer) != "" {
		t.Fatal("packets without a BOS were forwarded")
	}
	client.remoteTalk(testRemote, talkTypeBOS, "u-b", 1)
	waitFor(t, func() bool { return h.queued(testPeer) == 3 }, "preroll replayed after BOS")
	client.remoteTalk(testRemote, talkTypeEOS, "u-b", 2)
	cancel()
	chunks, _ := collect(t, output)
	streams := groupStreams(t, chunks)
	if len(streams) != 1 || !slices.Equal(markers(streams[0].payload), []byte{0xB1, 0xB2}) {
		t.Fatalf("streams = %+v", streams)
	}
	if h.dropped(testPeer) != 0 {
		t.Fatalf("dropped = %d, want 0", h.dropped(testPeer))
	}
}

func TestMalformedTalkDataIsCountedAndIgnored(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	h.attach(ctx, testPeer)
	client := h.connector.client(0)
	client.events.onTrackSubscribed(testRemote, "TR_b", newFakeReader())
	rejected := func() uint64 {
		status, _ := h.agent.SessionStatus(testPeer)
		return status.RejectedData
	}
	for i, payload := range [][]byte{
		[]byte("not json"),
		[]byte(`{"v":2,"type":"bos","utterance":"u","seq":1}`),
		[]byte(`{"v":1,"type":"hello","utterance":"u","seq":1}`),
		[]byte(`{"v":1,"type":"bos","utterance":"","seq":1}`),
		[]byte(`{"v":1,"type":"bos","utterance":"u","seq":0}`),
		nil,
	} {
		client.events.onDataPacket(testRemote, talkTopic, payload)
		if got := rejected(); got != uint64(i+1) {
			t.Fatalf("rejected after payload %d = %d, want %d", i, got, i+1)
		}
	}
	if h.floorHolder(testPeer) != "" {
		t.Fatal("malformed data granted the floor")
	}
	before := rejected()
	// Other topics, the Peer's own identity and a stale EOS are ignored
	// without counting.
	client.events.onDataPacket(testRemote, "other.topic", []byte("x"))
	client.events.onDataPacket(testPeer, talkTopic, mustEncodeTalk(talkTypeBOS, "self", 1))
	client.events.onDataPacket("", talkTopic, mustEncodeTalk(talkTypeBOS, "anon", 1))
	client.events.onDataPacket(testRemote, talkTopic, mustEncodeTalk(talkTypeEOS, "unknown", 9))
	if got := rejected(); got != before {
		t.Fatalf("ignored data was counted: %d -> %d", before, got)
	}
	if h.floorHolder(testPeer) != "" {
		t.Fatal("ignored data granted the floor")
	}
	// The payload's identity never matters: the sender identity is the SFU's.
	client.events.onDataPacket(testRemote, talkTopic, mustEncodeTalk(talkTypeBOS, "u-b", 1))
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "valid BOS grants the floor")
	// A duplicate BOS for the same utterance changes nothing.
	client.events.onDataPacket(testRemote, talkTopic, mustEncodeTalk(talkTypeBOS, "u-b", 1))
	if got := h.queued(testPeer); got != 1 {
		t.Fatalf("duplicate BOS reopened the stream: queued = %d", got)
	}
}

func mustEncodeTalk(kind, utterance string, seq uint64) []byte {
	payload, err := json.Marshal(talkMessage{V: talkProtocolVersion, Type: kind, Utterance: utterance, Seq: seq})
	if err != nil {
		panic(err)
	}
	return payload
}

// TestDownlinkChunksArePassthroughOpus pins the chunk shape the peer side
// relies on: passthrough MIME, label and name equal to the participant
// identity, a fresh stream_id per hold, and raw payload bytes unchanged.
func TestDownlinkChunksArePassthroughOpus(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	reader := subscribeRemote(t, h, client, testRemote, "TR_b", "u-1", 1)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor")
	reader.send(1, 0, 0x78, 0x01)
	waitFor(t, func() bool { return h.queued(testPeer) == 2 }, "packet")
	client.remoteTalk(testRemote, talkTypeEOS, "u-1", 2)
	client.remoteTalk(testRemote, talkTypeBOS, "u-2", 3)
	waitFor(t, func() bool { return h.queued(testPeer) == 4 }, "second hold")
	cancel()
	chunks, _ := collect(t, output)
	for _, chunk := range chunks {
		blob := chunk.Part.(*genx.Blob)
		if !agenthost.IsOpusPassthroughChunk(chunk) || blob.MIMEType != agenthost.OpusPassthroughMIME {
			t.Fatalf("chunk MIME = %q", blob.MIMEType)
		}
		if chunk.Role != genx.RoleModel || chunk.Name != testRemote || chunk.Ctrl.Label != testRemote {
			t.Fatalf("chunk identity = %q/%q", chunk.Name, chunk.Ctrl.Label)
		}
	}
	streams := groupStreams(t, chunks)
	if len(streams) != 2 || streams[0].id == streams[1].id {
		t.Fatalf("streams = %+v", streams)
	}
	if got := streams[0].payload; len(got) != 1 || !slices.Equal(got[0], []byte{0x78, 0x01}) {
		t.Fatalf("payload = %x, want the raw two-byte frame", got)
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
	h := newHarness(t, Config{FloorIdle: 5 * time.Second, TalkHangover: 5 * time.Second})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, input := h.attach(ctx, testPeer)
	first := h.connector.client(0)
	reader := subscribeRemote(t, h, first, testRemote, "TR_b", "u-b", 1)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor")
	reader.voiced(0xB1)
	waitFor(t, func() bool { return h.queued(testPeer) == 2 }, "downlink before disconnect")
	input.push(voicedChunk(0x01))
	waitFor(t, func() bool { return first.talkCount() == 1 }, "local bos before disconnect")

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
	if h.floorHolder(testPeer) != "" || h.trackCount(testPeer) != 0 || h.talking(testPeer) {
		t.Fatal("reconnect kept the floor, remote tracks or the open utterance")
	}
	input.push(voicedChunk(0x02))
	waitFor(t, func() bool { return second.sampleCount() == 1 }, "uplink on new client")
	waitFor(t, func() bool { return second.talkCount() == 1 }, "fresh utterance announced on new client")
	if first.sampleCount() != 1 {
		t.Fatalf("stale client samples = %d, want only the pre-disconnect frame", first.sampleCount())
	}
	if first.talk()[0].Utterance == second.talk()[0].Utterance {
		t.Fatal("utterance id survived the reconnect")
	}
	cancel()
	chunks, err := collect(t, output)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("output ended with %v, want EOF", err)
	}
	// The floor was released on reconnect: BOS, payload, EOS.
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
	got := agent.(*Agent).config
	if got.RecheckInterval != DefaultRecheckInterval || got.ReconnectTimeout != DefaultReconnectTimeout ||
		got.TalkHangover != DefaultTalkHangover || got.FloorIdle != DefaultFloorIdle {
		t.Fatalf("defaults = %+v", got)
	}
}
