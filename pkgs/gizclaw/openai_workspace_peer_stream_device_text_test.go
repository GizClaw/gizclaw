package gizclaw

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
)

func TestDeviceTextWireAfterEmittedAudioTimestamp(t *testing.T) {
	for _, timestamp := range []int64{0, time.Now().UnixMilli()} {
		t.Run(fmt.Sprint(timestamp), func(t *testing.T) {
			input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
			defer input.Close()
			// Establish a nonzero last-emitted timestamp before opening a new route.
			if err := input.Push(t.Context(), &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "previous-audio", Timestamp: time.Now().UnixMilli()}}); err != nil {
				t.Fatal(err)
			}
			if _, err := input.Next(); err != nil {
				t.Fatal(err)
			}
			events := []*eventpb.PeerEvent{
				{Version: 1, Type: eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{StreamId: "demo-2", Label: "demo-home", TimestampUnixMs: timestamp}}},
				{Version: 1, Type: eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE, Payload: &eventpb.PeerEvent_TextDone{TextDone: &eventpb.TextDone{StreamId: "demo-2", Label: "demo-home", TimestampUnixMs: timestamp, Text: "完整文字输入"}}},
			}
			for _, event := range events {
				var wire bytes.Buffer
				if err := writePeerStreamEvent(&wire, event); err != nil {
					t.Fatal(err)
				}
				decoded, err := readPeerStreamEvent(&wire)
				if err != nil {
					t.Fatal(err)
				}
				chunk, err := peerStreamEventToChunk(decoded)
				if err != nil {
					t.Fatal(err)
				}
				if chunk.Ctrl.Timestamp != timestamp {
					t.Fatalf("wire timestamp changed: %d", chunk.Ctrl.Timestamp)
				}
				if err := input.Push(t.Context(), chunk); err != nil {
					t.Fatal(err)
				}
			}
			// Close makes a drop observable as EOF rather than an unbounded wait.
			if err := input.Close(); err != nil {
				t.Fatal(err)
			}
			bos, err := input.Next()
			if err != nil {
				t.Fatalf("BOS dropped: %v", err)
			}
			if bos.Part != nil || !bos.IsBeginOfStream() || bos.Ctrl.StreamID != "demo-2" {
				t.Fatalf("BOS = %#v", bos)
			}
			done, err := input.Next()
			if err != nil {
				t.Fatalf("TEXT_DONE dropped: %v", err)
			}
			if done.Part != genx.Text("完整文字输入") || !done.IsEndOfStream() || done.Role != genx.RoleUser || done.Ctrl.Label != "demo-home" {
				t.Fatalf("TEXT_DONE = %#v", done)
			}
		})
	}
}
