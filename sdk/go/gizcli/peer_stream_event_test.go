package gizcli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"google.golang.org/protobuf/proto"
)

func TestDialPeerEventStreamValidation(t *testing.T) {
	var nilClient *Client
	if _, err := nilClient.DialPeerEventStream(); err == nil || !strings.Contains(err.Error(), "nil client") {
		t.Fatalf("nil DialPeerEventStream() error = %v", err)
	}
	if _, err := (&Client{}).DialPeerEventStream(); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("unconnected DialPeerEventStream() error = %v", err)
	}
}

func TestClientPeerEventSessionSharesOnePhysicalStream(t *testing.T) {
	serverSide, physical := net.Pipe()
	failed := make(chan struct{}, 1)
	session := newPeerEventSession(physical, func() { failed <- struct{}{} })
	client := &Client{events: session}
	session.start()
	t.Cleanup(func() { _ = session.close() })

	first, err := client.DialPeerEventStream()
	if err != nil {
		t.Fatalf("DialPeerEventStream(first) error = %v", err)
	}
	second, err := client.DialPeerEventStream()
	if err != nil {
		t.Fatalf("DialPeerEventStream(second) error = %v", err)
	}
	defer second.Close()

	incoming := textEvent("shared-1", "assistant", "one")
	if err := WritePeerStreamEvent(serverSide, incoming); err != nil {
		t.Fatalf("WritePeerStreamEvent(server) error = %v", err)
	}
	for name, stream := range map[string]net.Conn{"first": first, "second": second} {
		got, err := ReadPeerStreamEvent(stream)
		if err != nil {
			t.Fatalf("ReadPeerStreamEvent(%s) error = %v", name, err)
		}
		if got.Text() != "one" {
			t.Fatalf("ReadPeerStreamEvent(%s) text = %q", name, got.Text())
		}
	}

	if err := first.Close(); err != nil {
		t.Fatalf("Close(first) error = %v", err)
	}
	if err := WritePeerStreamEvent(serverSide, textEvent("shared-2", "assistant", "two")); err != nil {
		t.Fatalf("WritePeerStreamEvent(after first close) error = %v", err)
	}
	got, err := ReadPeerStreamEvent(second)
	if err != nil {
		t.Fatalf("ReadPeerStreamEvent(second after first close) error = %v", err)
	}
	if got.Text() != "two" {
		t.Fatalf("second event text = %q", got.Text())
	}

	outgoing := textEvent("shared-3", "user", "three")
	readBack := make(chan *eventpb.PeerEvent, 1)
	go func() {
		event, _ := ReadPeerStreamEvent(serverSide)
		readBack <- event
	}()
	if err := WritePeerStreamEvent(second, outgoing); err != nil {
		t.Fatalf("WritePeerStreamEvent(second) error = %v", err)
	}
	if got := <-readBack; got == nil || got.Text() != "three" {
		t.Fatalf("physical outgoing event = %+v", got)
	}
	if err := serverSide.Close(); err != nil {
		t.Fatalf("Close(physical server Event stream) error = %v", err)
	}
	if _, err := second.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("logical subscriber after physical close error = %v, want EOF", err)
	}
	select {
	case <-failed:
	case <-time.After(time.Second):
		t.Fatal("physical Event stream close did not fail the owning connection")
	}
}

func TestClientPeerEventWriteFailureClosesOwningConnection(t *testing.T) {
	writeErr := errors.New("event write failed")
	physical := &failingPeerEventConn{
		closed:   make(chan struct{}),
		writeErr: writeErr,
	}
	failed := make(chan struct{}, 1)
	session := newPeerEventSession(physical, func() { failed <- struct{}{} })
	session.start()
	t.Cleanup(func() { _ = session.close() })
	logical, err := session.subscribe()
	if err != nil {
		t.Fatalf("subscribe() error = %v", err)
	}

	if _, err := logical.Write([]byte("event")); !errors.Is(err, writeErr) {
		t.Fatalf("logical Event write error = %v, want %v", err, writeErr)
	}
	select {
	case <-failed:
	case <-time.After(time.Second):
		t.Fatal("physical Event write failure did not fail the owning connection")
	}
	if _, err := logical.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("logical subscriber after write failure error = %v, want EOF", err)
	}
}

func TestPeerEventSessionDropsQueuedFramesOnShutdown(t *testing.T) {
	session := newPeerEventSession(nil, nil)
	logical, err := session.subscribe()
	if err != nil {
		t.Fatalf("subscribe() error = %v", err)
	}
	session.publish([]byte("stale"))
	session.shutdown()

	if _, err := logical.Read(make([]byte, 8)); !errors.Is(err, io.EOF) {
		t.Fatalf("Read() after shutdown error = %v, want EOF", err)
	}
}

type failingPeerEventConn struct {
	closed   chan struct{}
	writeErr error
	once     sync.Once
}

func (c *failingPeerEventConn) Read([]byte) (int, error) {
	<-c.closed
	return 0, io.EOF
}

func (c *failingPeerEventConn) Write([]byte) (int, error) {
	return 0, c.writeErr
}

func (c *failingPeerEventConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (*failingPeerEventConn) LocalAddr() net.Addr              { return nil }
func (*failingPeerEventConn) RemoteAddr() net.Addr             { return nil }
func (*failingPeerEventConn) SetDeadline(time.Time) error      { return nil }
func (*failingPeerEventConn) SetReadDeadline(time.Time) error  { return nil }
func (*failingPeerEventConn) SetWriteDeadline(time.Time) error { return nil }

func TestPeerStreamEventHelpersUseOnlyBinaryProtobuf(t *testing.T) {
	event := textEvent("s1", "assistant", "hello")
	var buf bytes.Buffer
	if err := WritePeerStreamEvent(&buf, event); err != nil {
		t.Fatalf("WritePeerStreamEvent() error = %v", err)
	}
	got, err := ReadPeerStreamEvent(&buf)
	if err != nil {
		t.Fatalf("ReadPeerStreamEvent() error = %v", err)
	}
	if got.Version != eventpb.Version || got.Type != event.Type || got.Text() != "hello" {
		t.Fatalf("event = %+v", got)
	}

	buf.Reset()
	if err := rpcapi.WriteFrame(&buf, rpcapi.Frame{Type: rpcapi.FrameTypeJSON, Payload: []byte(`{"v":1}`)}); err != nil {
		t.Fatalf("WriteFrame(JSON) error = %v", err)
	}
	if _, err := ReadPeerStreamEvent(&buf); err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("ReadPeerStreamEvent(JSON) error = %v", err)
	}
	if _, err := ReadPeerStreamEvent(bytes.NewBufferString("bad")); err == nil {
		t.Fatal("ReadPeerStreamEvent() succeeded for bad frame")
	}
}

func TestPeerStreamPushWritesEventsAndOpus(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer serverSide.Close()
	writer := &recordingPeerPacketWriter{ch: make(chan []byte, 1)}
	stream := &PeerStream{
		events: clientSide,
		conn:   writer,
		out:    make(chan *genx.MessageChunk, 1),
		done:   make(chan struct{}),
	}
	defer stream.Close()

	pushErr := make(chan error, 1)
	go func() {
		pushErr <- stream.Push(context.Background(), &genx.MessageChunk{
			Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}},
			Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "mic", BeginOfStream: true},
		})
	}()
	event, err := ReadPeerStreamEvent(serverSide)
	if err != nil {
		t.Fatalf("ReadPeerStreamEvent() error = %v", err)
	}
	if err := <-pushErr; err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if event.Type != eventpb.PeerEventType_PEER_EVENT_TYPE_BOS || event.StreamID() != "s1" || event.Label() != "mic" {
		t.Fatalf("event = %+v, want BOS", event)
	}
	select {
	case payload := <-writer.ch:
		if !bytes.Equal(payload, []byte{1, 2, 3}) {
			t.Fatalf("packet = %x", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Opus packet")
	}
}

func TestPeerStreamEventToChunkPreservesTypedEOS(t *testing.T) {
	event := eosEvent("s1", "assistant", "audio/opus", &eventpb.EventError{
		Code:      "CHATROOM_MEMBER_REMOVED",
		Message:   "removed",
		Retryable: false,
	})
	chunk, err := peerStreamEventToChunk(event)
	if err != nil {
		t.Fatalf("peerStreamEventToChunk() error = %v", err)
	}
	if chunk.Ctrl == nil || !chunk.Ctrl.EndOfStream || chunk.Ctrl.StreamID != "s1" ||
		chunk.Ctrl.ErrorCode != "CHATROOM_MEMBER_REMOVED" || chunk.Ctrl.Error != "removed" {
		t.Fatalf("chunk ctrl = %#v", chunk.Ctrl)
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob.MIMEType != "audio/opus" || len(blob.Data) != 0 {
		t.Fatalf("chunk part = %#v", chunk.Part)
	}
}

func TestPeerStreamEventToChunkPreservesTextEOSKind(t *testing.T) {
	event := &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
			StreamId: "s1", Label: "assistant", Kind: eventpb.StreamKind_STREAM_KIND_TEXT,
			Error: &eventpb.EventError{Code: "STREAM_INTERRUPTED", Message: "interrupted"},
		}},
	}
	chunk, err := peerStreamEventToChunk(event)
	if err != nil {
		t.Fatal(err)
	}
	text, ok := chunk.Part.(genx.Text)
	if !ok || text != "" || !chunk.IsEndOfStream() || chunk.Ctrl.Error != "interrupted" {
		t.Fatalf("text EOS chunk = %#v", chunk)
	}
}

func TestPeerStreamEventToChunkAcceptsWorkspaceHistoryUpdated(t *testing.T) {
	lastUpdated := time.Date(2026, 6, 22, 12, 0, 0, 123000000, time.UTC)
	event := &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED,
		Payload: &eventpb.PeerEvent_WorkspaceHistoryUpdated{
			WorkspaceHistoryUpdated: &eventpb.WorkspaceHistoryUpdated{
				WorkspaceName:       "workspace-a",
				WorkspaceKind:       eventpb.WorkspaceKind_WORKSPACE_KIND_WORKFLOW,
				LastUpdatedAtUnixMs: lastUpdated.UnixMilli(),
			},
		},
	}
	chunk, err := peerStreamEventToChunk(event)
	if err != nil {
		t.Fatalf("peerStreamEventToChunk() error = %v", err)
	}
	if chunk.Ctrl == nil || chunk.Ctrl.Label != "workspace.history.updated" || chunk.Ctrl.Timestamp != lastUpdated.UnixMilli() {
		t.Fatalf("chunk ctrl = %#v", chunk.Ctrl)
	}
}

func TestPeerStreamNextReadsEventsAndRoutesOpus(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer serverSide.Close()
	packets := make(chan []byte, 1)
	stream := &PeerStream{
		events:         clientSide,
		packets:        packets,
		out:            make(chan *genx.MessageChunk, 3),
		eventResults:   make(chan peerStreamEventResult, 3),
		done:           make(chan struct{}),
		resourceEvents: make(chan *eventpb.PeerEvent, 3),
	}
	defer stream.Close()
	go stream.readEvents()
	go stream.mergeOutput()

	if err := WritePeerStreamEvent(serverSide, &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED,
		Payload: &eventpb.PeerEvent_FriendRelationshipUpdated{
			FriendRelationshipUpdated: &eventpb.FriendRelationshipUpdated{
				PeerPublicKey: "peer-b",
				WorkspaceName: "direct-a-b",
				Change:        eventpb.FriendRelationshipChange_FRIEND_RELATIONSHIP_CHANGE_DELETED,
			},
		},
	}); err != nil {
		t.Fatalf("WritePeerStreamEvent(invalidation) error = %v", err)
	}
	select {
	case event := <-stream.ResourceEvents():
		if event.GetFriendRelationshipUpdated().GetPeerPublicKey() != "peer-b" {
			t.Fatalf("resource invalidation = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("OpenPeerStream did not expose the social invalidation")
	}
	rewardEvent := &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_GAMEPLAY_REWARD_UPDATED,
		Payload: &eventpb.PeerEvent_GameplayRewardUpdated{
			GameplayRewardUpdated: &eventpb.GameplayRewardUpdated{
				WorkspaceName:   "workspace-reward",
				RewardGrantName: "grant-a",
			},
		},
	}
	if err := WritePeerStreamEvent(serverSide, rewardEvent); err != nil {
		t.Fatalf("WritePeerStreamEvent(reward invalidation) error = %v", err)
	}
	select {
	case event := <-stream.ResourceEvents():
		if event.GetGameplayRewardUpdated().GetRewardGrantName() != "grant-a" {
			t.Fatalf("reward invalidation = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("OpenPeerStream did not expose the Gameplay reward invalidation")
	}
	historyEvent := &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED,
		Payload: &eventpb.PeerEvent_WorkspaceHistoryUpdated{
			WorkspaceHistoryUpdated: &eventpb.WorkspaceHistoryUpdated{
				WorkspaceName: "workspace-b",
				WorkspaceKind: eventpb.WorkspaceKind_WORKSPACE_KIND_WORKFLOW,
			},
		},
	}
	if err := WritePeerStreamEvent(serverSide, historyEvent); err != nil {
		t.Fatalf("WritePeerStreamEvent(history invalidation) error = %v", err)
	}
	select {
	case event := <-stream.ResourceEvents():
		if event.GetWorkspaceHistoryUpdated().GetWorkspaceName() != "workspace-b" {
			t.Fatalf("history invalidation = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("OpenPeerStream did not expose the history invalidation")
	}
	historyChunk, err := stream.Next()
	if err != nil || historyChunk.Ctrl == nil ||
		historyChunk.Ctrl.Label != "workspace.history.updated" {
		t.Fatalf("Next(history invalidation) = %#v, %v", historyChunk, err)
	}
	futureBytes, err := proto.Marshal(&eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType(99),
	})
	if err != nil {
		t.Fatalf("marshal future event: %v", err)
	}
	if err := rpcapi.WriteFrame(serverSide, rpcapi.Frame{
		Type:    rpcapi.FrameTypeBinary,
		Payload: futureBytes,
	}); err != nil {
		t.Fatalf("write future event: %v", err)
	}
	if err := WritePeerStreamEvent(serverSide, textEvent("s1", "assistant", "hello")); err != nil {
		t.Fatalf("WritePeerStreamEvent() error = %v", err)
	}
	chunk, err := stream.Next()
	if err != nil || string(chunk.Part.(genx.Text)) != "hello" {
		t.Fatalf("Next(text) = %#v, %v", chunk, err)
	}

	if err := WritePeerStreamEvent(serverSide, bosEvent("history-replay-1", "transcript", "audio/opus")); err != nil {
		t.Fatalf("WritePeerStreamEvent(BOS) error = %v", err)
	}
	chunk, err = stream.Next()
	if err != nil || !chunk.IsBeginOfStream() {
		t.Fatalf("Next(BOS) = %#v, %v", chunk, err)
	}
	packets <- []byte{4, 5}
	chunk, err = stream.Next()
	if err != nil {
		t.Fatalf("Next(packet) error = %v", err)
	}
	blob := chunk.Part.(*genx.Blob)
	if !bytes.Equal(blob.Data, []byte{4, 5}) || chunk.Ctrl.StreamID != "history-replay-1" || chunk.Ctrl.Label != "transcript" {
		t.Fatalf("packet chunk = %#v", chunk)
	}
	if err := WritePeerStreamEvent(serverSide, eosEvent("history-replay-1", "transcript", "audio/opus", nil)); err != nil {
		t.Fatalf("WritePeerStreamEvent(EOS) error = %v", err)
	}
	chunk, err = stream.Next()
	if err != nil || !chunk.IsEndOfStream() {
		t.Fatalf("Next(EOS) = %#v, %v", chunk, err)
	}
}

func TestPeerStreamOrdersBufferedOpusBeforeAudioEOS(t *testing.T) {
	packets := make(chan []byte, 2)
	packets <- []byte{1, 2}
	packets <- []byte{3, 4}
	stream := &PeerStream{
		packets: packets,
		out:     make(chan *genx.MessageChunk, 3),
		done:    make(chan struct{}),
		audioRoute: genx.StreamCtrl{
			StreamID: "history-replay",
			Label:    "transcript",
		},
	}
	defer stream.Close()
	eos, err := peerStreamEventToChunk(eosEvent("history-replay", "transcript", "audio/opus", nil))
	if err != nil {
		t.Fatalf("peerStreamEventToChunk(EOS) error = %v", err)
	}
	if err := stream.pushMergedEvent(eos); err != nil {
		t.Fatalf("pushMergedEvent(EOS) error = %v", err)
	}
	for index, want := range [][]byte{{1, 2}, {3, 4}} {
		chunk, err := stream.Next()
		if err != nil {
			t.Fatalf("Next(packet %d) error = %v", index, err)
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || !bytes.Equal(blob.Data, want) || chunk.Ctrl == nil || chunk.Ctrl.StreamID != "history-replay" {
			t.Fatalf("packet %d chunk = %#v, want routed data %x", index, chunk, want)
		}
	}
	chunk, err := stream.Next()
	if err != nil || !chunk.IsEndOfStream() {
		t.Fatalf("Next(EOS) = %#v, %v", chunk, err)
	}
}

func TestPeerStreamRoutesFixedOpusWithEmptyMIMEAcrossEpochs(t *testing.T) {
	stream := &PeerStream{
		out:  make(chan *genx.MessageChunk, 6),
		done: make(chan struct{}),
	}
	defer stream.Close()
	for _, streamID := range []string{"live", "history-replay"} {
		bos, err := peerStreamEventToChunk(bosEvent(streamID, "assistant", ""))
		if err != nil {
			t.Fatalf("peerStreamEventToChunk(%s BOS) error = %v", streamID, err)
		}
		if err := stream.pushMergedEvent(bos); err != nil {
			t.Fatalf("pushMergedEvent(%s BOS) error = %v", streamID, err)
		}
		if err := stream.pushMergedPacket([]byte{1, 2, 3}); err != nil {
			t.Fatalf("pushMergedPacket(%s) error = %v", streamID, err)
		}
		eos, err := peerStreamEventToChunk(eosEvent(streamID, "assistant", "", nil))
		if err != nil {
			t.Fatalf("peerStreamEventToChunk(%s EOS) error = %v", streamID, err)
		}
		if err := stream.pushMergedEvent(eos); err != nil {
			t.Fatalf("pushMergedEvent(%s EOS) error = %v", streamID, err)
		}
		for index := range 3 {
			chunk, err := stream.Next()
			if err != nil {
				t.Fatalf("Next(%s, %d) error = %v", streamID, index, err)
			}
			if chunk.Ctrl == nil || chunk.Ctrl.StreamID != streamID || chunk.Ctrl.Label != "assistant" {
				t.Fatalf("chunk %d route = %#v, want %s/assistant", index, chunk.Ctrl, streamID)
			}
			if index == 0 && !chunk.IsBeginOfStream() {
				t.Fatalf("first chunk = %#v, want BOS", chunk)
			}
			if index == 1 {
				blob, ok := chunk.Part.(*genx.Blob)
				if !ok || blob.MIMEType != "audio/opus" || !bytes.Equal(blob.Data, []byte{1, 2, 3}) {
					t.Fatalf("packet chunk = %#v", chunk)
				}
			}
			if index == 2 && !chunk.IsEndOfStream() {
				t.Fatalf("last chunk = %#v, want EOS", chunk)
			}
		}
	}
}

func TestPeerStreamDeliversQueuedEventBeforeTerminalError(t *testing.T) {
	stream := &PeerStream{
		packets:      make(chan []byte),
		out:          make(chan *genx.MessageChunk, 1),
		eventResults: make(chan peerStreamEventResult, 2),
		done:         make(chan struct{}),
	}
	stream.eventResults <- peerStreamEventResult{
		chunk: &genx.MessageChunk{
			Part: genx.Text("last"),
			Ctrl: &genx.StreamCtrl{StreamID: "answer"},
		},
	}
	stream.eventResults <- peerStreamEventResult{err: io.EOF}
	go stream.mergeOutput()

	chunk, err := stream.Next()
	if err != nil || chunk == nil || chunk.Part != genx.Text("last") {
		t.Fatalf("Next(last event) = %#v, %v", chunk, err)
	}
	if _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("Next(terminal) error = %v, want EOF", err)
	}
}

func TestPeerStreamContinuesEventsAfterPacketSubscriptionCloses(t *testing.T) {
	packets := make(chan []byte)
	close(packets)
	stream := &PeerStream{
		packets:      packets,
		out:          make(chan *genx.MessageChunk, 1),
		eventResults: make(chan peerStreamEventResult),
		done:         make(chan struct{}),
	}
	defer stream.Close()
	go stream.mergeOutput()

	result := peerStreamEventResult{
		chunk: &genx.MessageChunk{
			Part: genx.Text("after packets"),
			Ctrl: &genx.StreamCtrl{StreamID: "answer"},
		},
	}
	select {
	case stream.eventResults <- result:
	case <-time.After(time.Second):
		t.Fatal("mergeOutput stopped after the packet subscription closed")
	}
	chunk, err := stream.Next()
	if err != nil || chunk == nil || chunk.Part != genx.Text("after packets") {
		t.Fatalf("Next(event after packets closed) = %#v, %v", chunk, err)
	}
}

func TestPeerStreamPushSkipsNilAndOggDirectPacket(t *testing.T) {
	writer := &recordingPeerPacketWriter{ch: make(chan []byte, 1)}
	stream := &PeerStream{conn: writer, done: make(chan struct{})}
	if err := stream.Push(context.Background(), nil); err != nil {
		t.Fatalf("Push(nil) error = %v", err)
	}
	if err := stream.Push(context.Background(), &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: []byte("OggS")},
	}); err != nil {
		t.Fatalf("Push(audio/ogg) error = %v", err)
	}
	select {
	case payload := <-writer.ch:
		t.Fatalf("audio/ogg was written as direct Opus: %x", payload)
	default:
	}
}

func bosEvent(streamID, label, mimeType string) *eventpb.PeerEvent {
	return &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_BOS,
		Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{
			StreamId: streamID,
			Kind:     eventpb.StreamKind_STREAM_KIND_AUDIO,
			Label:    label,
			MimeType: mimeType,
		}},
	}
}

func eosEvent(streamID, label, mimeType string, eventErr *eventpb.EventError) *eventpb.PeerEvent {
	return &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
			StreamId: streamID,
			Kind:     eventpb.StreamKind_STREAM_KIND_AUDIO,
			Label:    label,
			MimeType: mimeType,
			Error:    eventErr,
		}},
	}
}

func textEvent(streamID, label, text string) *eventpb.PeerEvent {
	return &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA,
		Payload: &eventpb.PeerEvent_TextDelta{TextDelta: &eventpb.TextDelta{
			StreamId: streamID,
			Label:    label,
			Text:     text,
		}},
	}
}

type recordingPeerPacketWriter struct {
	ch chan []byte
}

func (w *recordingPeerPacketWriter) Write(protocol byte, payload []byte) (int, error) {
	if protocol != giznet.ProtocolOpusPacket {
		return 0, nil
	}
	w.ch <- append([]byte(nil), payload...)
	return len(payload), nil
}
