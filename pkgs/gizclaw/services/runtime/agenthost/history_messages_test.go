package agenthost

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestHistoryMessagesKeepTheirOwnAudioOnOnePlaybackRoute(t *testing.T) {
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	recorder := newHistoryRecorder(history, "", nil)
	ctx := t.Context()
	observe := func(messageID string, part genx.Part, messageEnd, eos bool) {
		t.Helper()
		if err := recorder.ObserveOutput(ctx, &genx.MessageChunk{
			Role: genx.RoleModel, Name: "assistant", Part: part,
			Ctrl: &genx.StreamCtrl{StreamID: "continuous-playback", Label: "assistant", MessageID: messageID, MessageEnd: messageEnd, EndOfStream: eos},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 3 {
		messageID, text := fmt.Sprintf("generation-%d", i), fmt.Sprintf("Topic %d.", i)
		observe(messageID, genx.Text(text), false, false)
		observe(messageID, &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, byte(i), 0}}, false, false)
		observe(messageID, genx.Text(""), true, false)
		page, err := history.List(ctx, apitypes.PeerRunHistoryListRequest{})
		if err != nil || len(page.Items) != i {
			t.Fatalf("History ended before sibling audio: %+v, %v", page, err)
		}
		observe(messageID, &genx.Blob{MIMEType: "audio/opus"}, true, false)
		// Duplicate boundaries do not recreate an empty entry or merge messages.
		observe(messageID, genx.Text(""), true, false)
		page, err = history.List(ctx, apitypes.PeerRunHistoryListRequest{})
		if err != nil || len(page.Items) != i+1 {
			t.Fatalf("History not persisted during playback: %+v, %v", page, err)
		}
	}
	observe("", genx.Text(""), false, true)
	observe("", &genx.Blob{MIMEType: "audio/opus"}, false, true)
	if len(recorder.pending) != 0 {
		t.Fatalf("final EOS retained %d History messages", len(recorder.pending))
	}
	page, err := history.ListPage(ctx, apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 3 {
		t.Fatalf("History=%+v", page.Entries)
	}
	for _, entry := range page.Entries {
		var i int
		if _, err := fmt.Sscanf(entry.Text, "Topic %d.", &i); err != nil || len(entry.Assets) != 1 {
			t.Fatalf("message text/audio not paired: %+v", entry)
		}
		reader, err := history.ReadAsset(ctx, entry.Assets[0].Name)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("read History audio: %v, %v", err, closeErr)
		}
		packets, err := historyOpusFramesFromOgg(data)
		if err != nil || len(packets) != 1 || !bytes.Equal(packets[0], []byte{0xf8, byte(i), 0}) {
			t.Fatalf("message %d inherited another generation's audio: %v, %v", i, packets, err)
		}
	}
}

type cancellingHistoryDelivery struct {
	*historySliceStream
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (*cancellingHistoryDelivery) DeferOutputObservation() {}
func (s *cancellingHistoryDelivery) ObserveOutput(*genx.MessageChunk) {
	close(s.started)
	<-s.release
}
func (*cancellingHistoryDelivery) AbandonOutputObservation(*genx.MessageChunk) {}
func (s *cancellingHistoryDelivery) CloseWithError(error) error {
	s.once.Do(func() { close(s.release) })
	return nil
}

func TestHistoryMessageCloseCancelsBeforeJoiningActiveDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	source := &cancellingHistoryDelivery{
		historySliceStream: historyStreamFromChunks(&genx.MessageChunk{
			Role: genx.RoleModel, Name: "assistant", Part: genx.Text("delivered"),
			Ctrl: &genx.StreamCtrl{StreamID: "playback", Label: "assistant", MessageID: "generation"},
		}), started: make(chan struct{}), release: make(chan struct{}),
	}
	agent := wrapHistoryAgent(historyTestAgent{output: source}, history)
	output, err := agent.Transform(ctx, historyStreamFromChunks())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	stream := output.(*historyOutputStream)
	stream.DeferOutputObservation()
	chunk, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	observed, closed := make(chan struct{}), make(chan struct{})
	go func() { stream.ObserveOutput(chunk); close(observed) }()
	select {
	case <-source.started:
	case <-ctx.Done():
		t.Fatal("delivery observer did not start")
	}
	go func() { _ = stream.CloseWithError(context.Canceled); close(closed) }()
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("close joined delivery before cancelling its source")
	}
	select {
	case <-observed:
	case <-ctx.Done():
		t.Fatal("close left its delivery observer active")
	}
	page, err := history.List(ctx, apitypes.PeerRunHistoryListRequest{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Text != "delivered" {
		t.Fatalf("close lost or duplicated the delivered prefix: %+v, %v", page, err)
	}
}

func TestHistoryMessageReadAheadIsNotRecordedBeforeDelivery(t *testing.T) {
	for _, deliver := range []bool{false, true} {
		t.Run(fmt.Sprintf("deliver-prefix=%v", deliver), func(t *testing.T) {
			history := newTestWorkspaceHistory(t, newTestObjectStore(t))
			source := historyStreamFromChunks(
				&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("delivered"), Ctrl: &genx.StreamCtrl{StreamID: "playback", Label: "assistant", MessageID: "generation", BeginOfStream: true}},
				&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("undelivered"), Ctrl: &genx.StreamCtrl{StreamID: "playback", Label: "assistant", MessageID: "generation"}},
				&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "playback", Label: "assistant", MessageID: "generation", MessageEnd: true}},
				&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "playback", Label: "assistant", EndOfStream: true, ResponseEpochEnd: true}},
			)
			agent := wrapHistoryAgent(historyTestAgent{output: source}, history)
			output, err := agent.Transform(t.Context(), historyStreamFromChunks())
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			stream := output.(*historyOutputStream)
			stream.DeferOutputObservation()
			<-stream.output.forwardingDone
			page, err := history.List(t.Context(), apitypes.PeerRunHistoryListRequest{})
			if err != nil || len(page.Items) != 0 {
				t.Fatalf("read-ahead became History: %+v, %v", page, err)
			}
			prefix, err := stream.Next()
			if err != nil {
				t.Fatal(err)
			}
			if deliver {
				stream.ObserveOutput(prefix)
				stream.ObserveOutput(prefix)
				stream.ObserveOutput(prefix.Clone())
			}
			if err := stream.CloseWithError(context.Canceled); err != nil {
				t.Fatal(err)
			}
			page, err = history.List(t.Context(), apitypes.PeerRunHistoryListRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if deliver {
				if len(page.Items) != 1 || page.Items[0].Text != "delivered" {
					t.Fatalf("History included an unobserved suffix or duplicate acknowledgement: %+v", page.Items)
				}
			} else if len(page.Items) != 0 {
				t.Fatalf("abandoned message was persisted: %+v", page.Items)
			}
		})
	}
}
