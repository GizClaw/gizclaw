package gizcli

import (
	"bytes"
	"context"
	"net"
	"strings"
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
		done:           make(chan struct{}),
		resourceEvents: make(chan *eventpb.PeerEvent, 3),
	}
	defer stream.Close()
	go stream.readEvents()
	go stream.readPackets()

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
