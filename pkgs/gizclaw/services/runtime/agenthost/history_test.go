package agenthost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestHistoryAgentRecordsOutputText(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agent := wrapHistoryAgent(historyTestAgent{output: historyStreamFromChunks(
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant", EndOfStream: true}},
	)}, history)

	out, err := agent.Transform(withHistoryGearID(context.Background(), "gear-a"), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if _, err := out.Next(); err != nil {
		t.Fatalf("Next text: %v", err)
	}
	if _, err := out.Next(); err != nil {
		t.Fatalf("Next eos: %v", err)
	}
	if _, err := out.Next(); !IsStreamDone(err) {
		t.Fatalf("Next done = %v", err)
	}

	resp, err := agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("history items = %+v", resp.Items)
	}
	item := resp.Items[0]
	if item.Type != apitypes.PeerRunHistoryEntryTypeAgent || item.Name != "assistant" || item.Text != "hello" || !item.ReplayAvailable {
		t.Fatalf("history item = %+v", item)
	}
}

func TestHistoryAgentFinalizesNodeNamedOutputOnRouteEOS(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agentOutput := newBlockingHistoryStream()
	agent := wrapHistoryAgent(historyTestAgent{output: agentOutput}, history)
	out, err := agent.Transform(withHistoryGearID(context.Background(), "gear-a"), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Name: "answer", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		{Role: genx.RoleModel, Name: "answer", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		{Role: genx.RoleModel, Name: "answer", Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant", EndOfStream: true}},
		{Role: genx.RoleModel, Name: "answer", Part: genx.Text("other stream"), Ctrl: &genx.StreamCtrl{StreamID: "s2", Label: "assistant"}},
		{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant", EndOfStream: true}},
	} {
		agentOutput.ch <- chunk
	}

	var resp apitypes.PeerRunHistoryListResponse
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		resp, err = agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
		if err != nil {
			t.Fatalf("ListHistory() error = %v", err)
		}
		if len(resp.Items) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = agentOutput.Close()
	for {
		_, nextErr := out.Next()
		if IsStreamDone(nextErr) {
			break
		}
		if nextErr != nil {
			t.Fatalf("Next() error = %v", nextErr)
		}
	}
	if len(resp.Items) != 1 {
		t.Fatalf("history items = %+v, want node output finalized before transform close", resp.Items)
	}
	item := resp.Items[0]
	if item.Type != apitypes.PeerRunHistoryEntryTypeAgent || item.Name != "answer" || item.Text != "hello" || !item.ReplayAvailable {
		t.Fatalf("history item = %+v", item)
	}
}

func TestHistoryAgentStatusAndUnavailableReplay(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agent := wrapHistoryAgent(historyTestAgent{output: historyStreamFromChunks()}, history)
	state, err := agent.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if state.HistoryAvailable == nil || !*state.HistoryAvailable {
		t.Fatalf("Status().HistoryAvailable = %#v", state.HistoryAvailable)
	}

	entry, err := history.Append(context.Background(), workspace.AppendHistoryRequest{
		Type: "agent",
		Name: "assistant",
		Text: "hello",
	})
	if err != nil {
		t.Fatalf("Append history: %v", err)
	}
	play, err := agent.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: entry.ID})
	if err != nil {
		t.Fatalf("PlayHistory() error = %v", err)
	}
	if play.Accepted || play.State != "unavailable" || play.Message == nil {
		t.Fatalf("PlayHistory() = %+v", play)
	}

	gearEntry, err := history.Append(context.Background(), workspace.AppendHistoryRequest{
		Type:   "gear",
		GearID: "gear-a",
		Name:   "input",
		Text:   "hello",
	})
	if err != nil {
		t.Fatalf("Append gear history: %v", err)
	}
	gearPlay, err := agent.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: gearEntry.ID})
	if err != nil {
		t.Fatalf("PlayHistory(gear) error = %v", err)
	}
	if gearPlay.Accepted || gearPlay.State != "unavailable" || gearPlay.Message == nil {
		t.Fatalf("PlayHistory(gear) = %+v", gearPlay)
	}

	missing, err := agent.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: "missing"})
	if err != nil {
		t.Fatalf("PlayHistory(missing) error = %v", err)
	}
	if missing.Accepted || missing.State != "not_found" {
		t.Fatalf("PlayHistory(missing) = %+v", missing)
	}
}

func TestHistoryAgentUnsupportedState(t *testing.T) {
	agent := &historyAgent{}
	list, err := agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if list.Available || list.Message == nil {
		t.Fatalf("ListHistory() = %+v", list)
	}
	play, err := agent.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: "h1"})
	if err != nil {
		t.Fatalf("PlayHistory() error = %v", err)
	}
	if play.Accepted || play.State != "unsupported" || play.Message == nil {
		t.Fatalf("PlayHistory() = %+v", play)
	}
}

func TestHistoryAgentSkipsGearHistoryWithoutGearID(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agent := wrapHistoryAgent(historyInputDrainingAgent{
		historyTestAgent: historyTestAgent{output: historyStreamFromChunks()},
	}, history)

	out, err := agent.Transform(context.Background(), "demo", historyStreamFromChunks(
		&genx.MessageChunk{Role: genx.RoleUser, Name: "gear", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "transcript"}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "gear", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "transcript", EndOfStream: true}},
	))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if _, err := out.Next(); !IsStreamDone(err) {
		t.Fatalf("Next done = %v", err)
	}
	resp, err := agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if len(resp.Items) != 0 {
		t.Fatalf("history items = %+v", resp.Items)
	}
}

func TestHistoryAgentRecordsOutputHistoryPCMAudioAsOggOpus(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	pcmFrame := historyTestPCMFrame(320)
	agent := wrapHistoryAgent(historyTestAgent{output: historyStreamFromChunks(
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: "audio/pcm", Data: pcmFrame[:300]}, Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: genx.HistoryUserAudioLabel}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: "audio/pcm", Data: pcmFrame[300:]}, Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: genx.HistoryUserAudioLabel}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: genx.HistoryUserAudioLabel, EndOfStream: true}},
	)}, history)

	out, err := agent.Transform(withHistoryGearID(context.Background(), "gear-a"), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if _, err := out.Next(); !IsStreamDone(err) {
		t.Fatalf("Next done = %v", err)
	}

	resp, err := agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("history items = %+v", resp.Items)
	}
	item := resp.Items[0]
	if item.Type != apitypes.PeerRunHistoryEntryTypeGear || item.GearId == nil || *item.GearId != "gear-a" || item.Name != "transcript" || !item.ReplayAvailable {
		t.Fatalf("history item = %+v", item)
	}
	entry, err := history.Get(context.Background(), item.Id)
	if err != nil {
		t.Fatalf("Get history: %v", err)
	}
	if len(entry.Assets) != 1 || entry.Assets[0].MIMEType != "audio/ogg; codecs=opus" {
		t.Fatalf("history assets = %+v", entry.Assets)
	}
	r, err := history.ReadAsset(context.Background(), entry.Assets[0].Name)
	if err != nil {
		t.Fatalf("ReadAsset: %v", err)
	}
	defer r.Close()
	packets, err := ogg.ReadAllPackets(r)
	if err != nil {
		t.Fatalf("ReadAllPackets: %v", err)
	}
	if len(packets) != 3 || !codecconv.IsOpusHeadPacket(packets[0].Data) || !codecconv.IsOpusTagsPacket(packets[1].Data) || len(packets[2].Data) == 0 {
		t.Fatalf("ogg packets = %+v", packets)
	}
}

func TestHistoryAgentEmitsHistoryUpdatedNotification(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agentOutput := newBlockingHistoryStream()
	agent := wrapHistoryAgent(historyTestAgent{output: agentOutput}, history)
	before := time.Now().Add(-time.Second).UTC()
	out, err := agent.Transform(withHistoryGearID(context.Background(), "gear-a"), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	agentOutput.ch <- &genx.MessageChunk{Role: genx.RoleUser, Name: "gear", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "transcript"}}
	agentOutput.ch <- &genx.MessageChunk{Role: genx.RoleUser, Name: "gear", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "transcript", EndOfStream: true}}
	chunk := expectHistoryUpdatedChunk(t, out)
	after := time.Now().Add(time.Second).UTC()
	updated := time.UnixMilli(chunk.Ctrl.Timestamp).UTC()
	if updated.Before(before) || updated.After(after) {
		t.Fatalf("history updated timestamp = %s, want between %s and %s", updated, before, after)
	}
	_ = agentOutput.Close()
}

func TestHistoryAgentBroadcastsHistoryUpdatedNotification(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	base := &historyMultiOutputAgent{}
	agent := wrapHistoryAgent(base, history)
	outA, err := agent.Transform(withHistoryGearID(context.Background(), "gear-a"), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform gear-a error = %v", err)
	}
	outB, err := agent.Transform(withHistoryGearID(context.Background(), "gear-b"), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform gear-b error = %v", err)
	}
	defer outA.Close()
	defer outB.Close()
	defer base.closeAll()

	base.mu.Lock()
	outputA := base.outputs[0]
	base.mu.Unlock()
	outputA.ch <- &genx.MessageChunk{Role: genx.RoleUser, Name: "gear", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "transcript"}}
	outputA.ch <- &genx.MessageChunk{Role: genx.RoleUser, Name: "gear", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "transcript", EndOfStream: true}}
	if chunk := expectHistoryUpdatedChunk(t, outA); chunk.Ctrl.Timestamp == 0 {
		t.Fatalf("gear-a history updated chunk = %#v", chunk)
	}
	if chunk := expectHistoryUpdatedChunk(t, outB); chunk.Ctrl.Timestamp == 0 {
		t.Fatalf("gear-b history updated chunk = %#v", chunk)
	}
}

func expectHistoryUpdatedChunk(t *testing.T, stream genx.Stream) *genx.MessageChunk {
	t.Helper()
	ch := make(chan *genx.MessageChunk, 1)
	errCh := make(chan error, 1)
	go func() {
		for {
			chunk, err := stream.Next()
			if err != nil {
				errCh <- err
				return
			}
			if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Label == historyUpdatedLabel {
				ch <- chunk
				return
			}
		}
	}()
	select {
	case err := <-errCh:
		t.Fatalf("Next history updated error = %v", err)
	case chunk := <-ch:
		if chunk.Ctrl.Timestamp == 0 {
			t.Fatalf("history updated chunk = %#v", chunk)
		}
		return chunk
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for history updated chunk")
	}
	return nil
}

func TestHistoryOutputCoalescesHistoryUpdatedNotifications(t *testing.T) {
	builder := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	output := &historyOutput{output: builder}
	first := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	second := first.Add(time.Second)
	output.notifyHistoryUpdated(first)
	output.notifyHistoryUpdated(second)

	stream := builder.Stream()
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next history updated: %v", err)
	}
	if chunk.Ctrl == nil || chunk.Ctrl.Label != historyUpdatedLabel || chunk.Ctrl.Timestamp != second.UnixMilli() {
		t.Fatalf("history updated chunk = %#v, want timestamp %d", chunk, second.UnixMilli())
	}

	result := make(chan error, 1)
	go func() {
		chunk, err := stream.Next()
		if err == nil {
			result <- errors.New("unexpected second history updated chunk: " + chunk.Ctrl.Label)
			return
		}
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("unexpected second notification result = %v", err)
	case <-time.After(historyUpdatedDelay * 2):
	}
	_ = stream.Close()
	if err := <-result; !IsStreamDone(err) && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("stream close error = %v", err)
	}
}

func historyTestPCMFrame(samples int) []byte {
	data := make([]byte, samples*2)
	for i := range samples {
		v := int16((i%64 - 32) * 512)
		data[i*2] = byte(v)
		data[i*2+1] = byte(uint16(v) >> 8)
	}
	return data
}

func TestHistoryAgentRecordsOutputAudioAsOggOpus(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agent := wrapHistoryAgent(historyTestAgent{output: historyStreamFromChunks(
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant", EndOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{4, 5}}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant", EndOfStream: true}},
	)}, history)

	out, err := agent.Transform(withHistoryGearID(context.Background(), "gear-a"), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for {
		_, err := out.Next()
		if IsStreamDone(err) {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
	}

	resp, err := agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("history items = %+v", resp.Items)
	}
	entry, err := history.Get(context.Background(), resp.Items[0].Id)
	if err != nil {
		t.Fatalf("Get history: %v", err)
	}
	if len(entry.Assets) != 1 || entry.Assets[0].MIMEType != "audio/ogg; codecs=opus" {
		t.Fatalf("history assets = %+v", entry.Assets)
	}
	r, err := history.ReadAsset(context.Background(), entry.Assets[0].Name)
	if err != nil {
		t.Fatalf("ReadAsset: %v", err)
	}
	defer r.Close()
	packets, err := ogg.ReadAllPackets(r)
	if err != nil {
		t.Fatalf("ReadAllPackets: %v", err)
	}
	if len(packets) != 4 || !codecconv.IsOpusHeadPacket(packets[0].Data) || !codecconv.IsOpusTagsPacket(packets[1].Data) {
		t.Fatalf("ogg packets = %+v", packets)
	}
	if !bytes.Equal(packets[2].Data, []byte{1, 2, 3}) || !bytes.Equal(packets[3].Data, []byte{4, 5}) {
		t.Fatalf("audio packets = %#v %#v", packets[2].Data, packets[3].Data)
	}
}

func TestHistoryAgentWaitsForTextAfterAudioEOS(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agent := wrapHistoryAgent(historyTestAgent{output: historyStreamFromChunks(
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("hel"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant", EndOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("lo"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant", EndOfStream: true}},
	)}, history)

	out, err := agent.Transform(context.Background(), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for {
		_, err := out.Next()
		if IsStreamDone(err) {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
	}
	resp, err := agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Text != "hello" || !resp.Items[0].ReplayAvailable {
		t.Fatalf("history items = %+v", resp.Items)
	}
}

func TestHistoryAgentRejectsMultipleAudioMIMEChannels(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agent := wrapHistoryAgent(historyTestAgent{output: historyStreamFromChunks(
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus"}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
	)}, history)

	out, err := agent.Transform(context.Background(), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for {
		_, err = out.Next()
		if err != nil {
			break
		}
	}
	if err == nil || !strings.Contains(err.Error(), "changed audio MIME type") {
		t.Fatalf("Next() error = %v, want audio MIME change error", err)
	}
}

func TestHistoryAgentIgnoresNonRecordableMIMECompletion(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agent := wrapHistoryAgent(historyTestAgent{output: historyStreamFromChunks(
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("hel"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "application/json"}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant", EndOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("lo"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant", EndOfStream: true}},
	)}, history)

	out, err := agent.Transform(context.Background(), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for {
		_, err := out.Next()
		if IsStreamDone(err) {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
	}
	resp, err := agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Text != "hello" {
		t.Fatalf("history items = %+v", resp.Items)
	}
}

func TestHistoryRecorderControlEOSFinalizesRouteOnce(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	recorder := newHistoryRecorder(history, "", nil)
	ctx := context.Background()
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		{Role: genx.RoleModel, Name: "status", Part: genx.Text("working"), Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "status"}},
	} {
		if err := recorder.ObserveOutput(ctx, chunk); err != nil {
			t.Fatalf("ObserveOutput(%s) error = %v", chunk.Name, err)
		}
	}
	routeEOS := &genx.MessageChunk{
		Ctrl: &genx.StreamCtrl{StreamID: "s1", EndOfStream: true},
	}
	if err := recorder.ObserveOutput(ctx, routeEOS); err != nil {
		t.Fatalf("ObserveOutput(route EOS) error = %v", err)
	}
	if err := recorder.ObserveOutput(ctx, routeEOS); err != nil {
		t.Fatalf("ObserveOutput(duplicate route EOS) error = %v", err)
	}
	if len(recorder.pending) != 0 {
		t.Fatalf("pending entries = %d, want 0", len(recorder.pending))
	}
	resp, err := history.List(ctx, apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("history items = %+v", resp.Items)
	}
	got := make(map[string]string, len(resp.Items))
	for _, item := range resp.Items {
		got[item.Name] = item.Text
	}
	if got["assistant"] != "hello" || got["status"] != "working" {
		t.Fatalf("history items = %+v", resp.Items)
	}
}

func TestHistoryAgentMergesOutputHistoryAudioWithTranscript(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agent := wrapHistoryAgent(historyTestAgent{output: historyStreamFromChunks(
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}, Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: genx.HistoryUserAudioLabel}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{4, 5}}, Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: genx.HistoryUserAudioLabel}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: genx.HistoryUserAudioLabel, EndOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "transcript"}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "transcript", EndOfStream: true}},
	)}, history)

	out, err := agent.Transform(withHistoryGearID(context.Background(), "gear-a"), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	var forwardedAudio bool
	for {
		chunk, err := out.Next()
		if IsStreamDone(err) {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Label == genx.HistoryUserAudioLabel {
			forwardedAudio = true
		}
	}
	if forwardedAudio {
		t.Fatal("history-only user audio was forwarded to peer output")
	}
	resp, err := agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("history items = %+v", resp.Items)
	}
	item := resp.Items[0]
	if item.Type != apitypes.PeerRunHistoryEntryTypeGear || item.GearId == nil || *item.GearId != "gear-a" || item.Name != "transcript" || item.Text != "hello" || !item.ReplayAvailable {
		t.Fatalf("history item = %+v", item)
	}
	entry, err := history.Get(context.Background(), item.Id)
	if err != nil {
		t.Fatalf("Get history: %v", err)
	}
	if len(entry.Assets) != 1 || entry.Assets[0].MIMEType != "audio/ogg; codecs=opus" {
		t.Fatalf("history assets = %+v", entry.Assets)
	}
	r, err := history.ReadAsset(context.Background(), entry.Assets[0].Name)
	if err != nil {
		t.Fatalf("ReadAsset: %v", err)
	}
	defer r.Close()
	packets, err := ogg.ReadAllPackets(r)
	if err != nil {
		t.Fatalf("ReadAllPackets: %v", err)
	}
	if len(packets) != 4 || !bytes.Equal(packets[2].Data, []byte{1, 2, 3}) || !bytes.Equal(packets[3].Data, []byte{4, 5}) {
		t.Fatalf("ogg packets = %+v", packets)
	}
}

func TestHistoryAgentRecordsOutputHistoryAudioByStreamID(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agent := wrapHistoryAgent(historyTestAgent{output: historyStreamFromChunks(
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}, Ctrl: &genx.StreamCtrl{StreamID: "audio:rt:1", Label: genx.HistoryUserAudioLabel}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("first"), Ctrl: &genx.StreamCtrl{StreamID: "audio:rt:1", Label: "transcript"}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio:rt:1", Label: genx.HistoryUserAudioLabel, EndOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "audio:rt:1", Label: "transcript", EndOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{4, 5, 6}}, Ctrl: &genx.StreamCtrl{StreamID: "audio:rt:2", Label: genx.HistoryUserAudioLabel}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("second"), Ctrl: &genx.StreamCtrl{StreamID: "audio:rt:2", Label: "transcript"}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio:rt:2", Label: genx.HistoryUserAudioLabel, EndOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "audio:rt:2", Label: "transcript", EndOfStream: true}},
	)}, history)

	out, err := agent.Transform(withHistoryGearID(context.Background(), "gear-a"), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for {
		_, err := out.Next()
		if IsStreamDone(err) {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
	}
	order := apitypes.PeerRunHistoryListRequestOrderAsc
	resp, err := agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{Order: &order})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("history items = %+v", resp.Items)
	}
	for i, want := range []string{"first", "second"} {
		item := resp.Items[i]
		if item.Type != apitypes.PeerRunHistoryEntryTypeGear || item.GearId == nil || *item.GearId != "gear-a" || item.Name != "transcript" || item.Text != want || !item.ReplayAvailable {
			t.Fatalf("history item[%d] = %+v", i, item)
		}
		entry, err := history.Get(context.Background(), item.Id)
		if err != nil {
			t.Fatalf("Get history[%d]: %v", i, err)
		}
		if len(entry.Assets) != 1 || entry.Assets[0].MIMEType != "audio/ogg; codecs=opus" {
			t.Fatalf("history assets[%d] = %+v", i, entry.Assets)
		}
	}
}

func TestHistoryAgentRecordsSplitOggOutput(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	audio, err := historyOggOpusAsset([][]byte{{1, 2, 3}, {4, 5}})
	if err != nil {
		t.Fatalf("historyOggOpusAsset: %v", err)
	}
	mid := len(audio) / 2
	agent := wrapHistoryAgent(historyTestAgent{output: historyStreamFromChunks(
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: audio[:mid]}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: audio[mid:]}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus"}, Ctrl: &genx.StreamCtrl{StreamID: "s1", Label: "assistant", EndOfStream: true}},
	)}, history)

	out, err := agent.Transform(withHistoryGearID(context.Background(), "gear-a"), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for {
		_, err := out.Next()
		if IsStreamDone(err) {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
	}

	resp, err := agent.ListHistory(context.Background(), apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("history items = %+v", resp.Items)
	}
	entry, err := history.Get(context.Background(), resp.Items[0].Id)
	if err != nil {
		t.Fatalf("Get history: %v", err)
	}
	if len(entry.Assets) != 1 || entry.Assets[0].MIMEType != "audio/ogg; codecs=opus" {
		t.Fatalf("history assets = %+v", entry.Assets)
	}
	r, err := history.ReadAsset(context.Background(), entry.Assets[0].Name)
	if err != nil {
		t.Fatalf("ReadAsset: %v", err)
	}
	defer r.Close()
	packets, err := ogg.ReadAllPackets(r)
	if err != nil {
		t.Fatalf("ReadAllPackets: %v", err)
	}
	if len(packets) != 4 || !bytes.Equal(packets[2].Data, []byte{1, 2, 3}) || !bytes.Equal(packets[3].Data, []byte{4, 5}) {
		t.Fatalf("ogg packets = %+v", packets)
	}
}

func TestHistoryAgentPlayInjectsReplayOutput(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	audio, err := historyOggOpusAsset([][]byte{{1, 2, 3}, {4, 5}})
	if err != nil {
		t.Fatalf("historyOggOpusAsset: %v", err)
	}
	entry, err := history.Append(context.Background(), workspace.AppendHistoryRequest{
		Type:      "agent",
		Name:      "assistant",
		Text:      "replay",
		CreatedAt: now,
		Asset:     &workspace.AppendHistoryAsset{MIMEType: "audio/ogg; codecs=opus", Data: audio},
	})
	if err != nil {
		t.Fatalf("Append history: %v", err)
	}
	agentOutput := newBlockingHistoryStream()
	agent := wrapHistoryAgent(historyTestAgent{output: agentOutput}, history)

	out, err := agent.Transform(context.Background(), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	resp, err := agent.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: entry.ID})
	if err != nil {
		t.Fatalf("PlayHistory() error = %v", err)
	}
	if !resp.Accepted || resp.State != "played" {
		t.Fatalf("PlayHistory() = %+v", resp)
	}
	text, err := out.Next()
	if err != nil {
		t.Fatalf("Next replay text: %v", err)
	}
	if got, ok := text.Part.(genx.Text); !ok || string(got) != "replay" {
		t.Fatalf("replay text chunk = %#v", text)
	}
	if _, err := out.Next(); err != nil {
		t.Fatalf("Next replay text eos: %v", err)
	}
	audioBOS, err := out.Next()
	if err != nil {
		t.Fatalf("Next replay audio bos: %v", err)
	}
	if audioBOS.Ctrl == nil || !audioBOS.Ctrl.BeginOfStream || audioBOS.Ctrl.StreamID == "" || audioBOS.Ctrl.Label != "assistant" {
		t.Fatalf("replay audio bos = %#v", audioBOS)
	}
	audioChunk, err := out.Next()
	if err != nil {
		t.Fatalf("Next replay audio: %v", err)
	}
	blob, ok := audioChunk.Part.(*genx.Blob)
	if !ok || blob.MIMEType != "audio/opus" || !bytes.Equal(blob.Data, []byte{1, 2, 3}) {
		t.Fatalf("replay audio chunk = %#v", audioChunk)
	}
	audioChunk, err = out.Next()
	if err != nil {
		t.Fatalf("Next replay audio 2: %v", err)
	}
	blob, ok = audioChunk.Part.(*genx.Blob)
	if !ok || blob.MIMEType != "audio/opus" || !bytes.Equal(blob.Data, []byte{4, 5}) {
		t.Fatalf("replay audio chunk 2 = %#v", audioChunk)
	}
	audioChunk, err = out.Next()
	if err != nil {
		t.Fatalf("Next replay audio eos: %v", err)
	}
	blob, ok = audioChunk.Part.(*genx.Blob)
	if !ok || blob.MIMEType != "audio/opus" || len(blob.Data) != 0 || audioChunk.Ctrl == nil || !audioChunk.Ctrl.EndOfStream {
		t.Fatalf("replay audio eos = %#v", audioChunk)
	}
	_ = agentOutput.Close()
}

func TestHistoryAgentPlayReplaysGearHistory(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	entry, err := history.Append(context.Background(), workspace.AppendHistoryRequest{
		Type:   "gear",
		GearID: "gear-a",
		Name:   "gear-a",
		Text:   "hello from gear",
	})
	if err != nil {
		t.Fatalf("Append history: %v", err)
	}
	agentOutput := newBlockingHistoryStream()
	agent := wrapHistoryAgent(historyTestAgent{output: agentOutput}, history)
	out, err := agent.Transform(context.Background(), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	resp, err := agent.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: entry.ID})
	if err != nil {
		t.Fatalf("PlayHistory() error = %v", err)
	}
	if !resp.Accepted || resp.State != "played" {
		t.Fatalf("PlayHistory() = %+v", resp)
	}
	text, err := out.Next()
	if err != nil {
		t.Fatalf("Next replay text: %v", err)
	}
	if text.Role != genx.RoleUser || text.Ctrl == nil || text.Ctrl.Label != "transcript" {
		t.Fatalf("gear replay route = %#v", text)
	}
	if got, ok := text.Part.(genx.Text); !ok || string(got) != "hello from gear" {
		t.Fatalf("gear replay text = %#v", text)
	}
	if eos, err := out.Next(); err != nil || eos.Ctrl == nil || !eos.Ctrl.EndOfStream {
		t.Fatalf("gear replay eos = %#v, %v", eos, err)
	}
	_ = agentOutput.Close()
}

func TestHistoryAgentPlayRoutesToRequestGearOutput(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	entry, err := history.Append(context.Background(), workspace.AppendHistoryRequest{
		Type: "agent",
		Name: "assistant",
		Text: "hello gear a",
	})
	if err != nil {
		t.Fatalf("Append history: %v", err)
	}
	base := &historyMultiOutputAgent{}
	agent := wrapHistoryAgent(base, history)
	outA, err := agent.Transform(withHistoryGearID(context.Background(), "gear-a"), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform(gear-a) error = %v", err)
	}
	outB, err := agent.Transform(withHistoryGearID(context.Background(), "gear-b"), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform(gear-b) error = %v", err)
	}
	defer outA.Close()
	defer outB.Close()
	defer base.closeAll()

	resp, err := agent.PlayHistory(withHistoryGearID(context.Background(), "gear-a"), apitypes.PeerRunHistoryPlayRequest{HistoryId: entry.ID})
	if err != nil {
		t.Fatalf("PlayHistory() error = %v", err)
	}
	if !resp.Accepted || resp.State != "played" {
		t.Fatalf("PlayHistory() = %+v", resp)
	}
	text, err := outA.Next()
	if err != nil {
		t.Fatalf("gear-a Next replay text: %v", err)
	}
	if got, ok := text.Part.(genx.Text); !ok || string(got) != "hello gear a" {
		t.Fatalf("gear-a replay text = %#v", text)
	}
	result := make(chan error, 1)
	go func() {
		chunk, err := outB.Next()
		if err != nil {
			result <- err
			return
		}
		_ = chunk
		result <- fs.ErrInvalid
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("gear-b received unexpected replay chunk")
		}
		t.Fatalf("gear-b replay result before close = %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	_ = outB.Close()
	if err := <-result; !IsStreamDone(err) && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("gear-b close error = %v, want stream done", err)
	}
}

func TestHistoryAgentPlayPreservesCompletedBufferedReplay(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	audio1, err := historyOggOpusAsset([][]byte{{1}, {2}, {3}, {4}})
	if err != nil {
		t.Fatalf("historyOggOpusAsset first: %v", err)
	}
	first, err := history.Append(context.Background(), workspace.AppendHistoryRequest{
		Type:  "agent",
		Name:  "assistant",
		Text:  "first",
		Asset: &workspace.AppendHistoryAsset{MIMEType: "audio/ogg; codecs=opus", Data: audio1},
	})
	if err != nil {
		t.Fatalf("Append first: %v", err)
	}
	audio2, err := historyOggOpusAsset([][]byte{{9}})
	if err != nil {
		t.Fatalf("historyOggOpusAsset second: %v", err)
	}
	second, err := history.Append(context.Background(), workspace.AppendHistoryRequest{
		Type:  "agent",
		Name:  "assistant",
		Text:  "second",
		Asset: &workspace.AppendHistoryAsset{MIMEType: "audio/ogg; codecs=opus", Data: audio2},
	})
	if err != nil {
		t.Fatalf("Append second: %v", err)
	}
	agentOutput := newBlockingHistoryStream()
	agent := wrapHistoryAgent(historyTestAgent{output: agentOutput}, history)
	out, err := agent.Transform(context.Background(), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if resp, err := agent.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: first.ID}); err != nil || !resp.Accepted {
		t.Fatalf("PlayHistory(first) = %+v, %v", resp, err)
	}
	if text, err := out.Next(); err != nil {
		t.Fatalf("Next first text: %v", err)
	} else if got, ok := text.Part.(genx.Text); !ok || string(got) != "first" {
		t.Fatalf("first text = %#v", text)
	}
	if _, err := out.Next(); err != nil {
		t.Fatalf("Next first text eos: %v", err)
	}
	if audioBOS, err := out.Next(); err != nil {
		t.Fatalf("Next first audio bos: %v", err)
	} else if audioBOS.Ctrl == nil || !audioBOS.Ctrl.BeginOfStream {
		t.Fatalf("first audio bos = %#v", audioBOS)
	}
	if audio, err := out.Next(); err != nil {
		t.Fatalf("Next first audio: %v", err)
	} else if blob, ok := audio.Part.(*genx.Blob); !ok || !bytes.Equal(blob.Data, []byte{1}) {
		t.Fatalf("first audio = %#v", audio)
	}
	if resp, err := agent.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: second.ID}); err != nil || !resp.Accepted {
		t.Fatalf("PlayHistory(second) = %+v, %v", resp, err)
	}
	var interruptedText, interruptedAudio, foundSecond bool
	for range 12 {
		chunk, err := out.Next()
		if err != nil {
			t.Fatalf("Next after replacement: %v", err)
		}
		if text, ok := chunk.Part.(genx.Text); ok && string(text) == "second" {
			foundSecond = true
			break
		}
		if chunk.Ctrl == nil || !chunk.Ctrl.EndOfStream || chunk.Ctrl.Error != historyReplayInterrupted {
			continue
		}
		switch chunk.Part.(type) {
		case genx.Text:
			interruptedText = true
		case *genx.Blob:
			interruptedAudio = true
		}
	}
	if interruptedText || interruptedAudio || !foundSecond {
		t.Fatalf("replacement result textEOS=%t audioEOS=%t second=%t", interruptedText, interruptedAudio, foundSecond)
	}
	_ = agentOutput.Close()
}

func TestHistoryAgentPlayInterruptsCurrentAgentOutput(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	entry, err := history.Append(context.Background(), workspace.AppendHistoryRequest{
		Type: "agent",
		Name: "assistant",
		Text: "replay",
	})
	if err != nil {
		t.Fatalf("Append history: %v", err)
	}
	agentOutput := newBlockingHistoryStream()
	agent := wrapHistoryAgent(historyTestAgent{output: agentOutput}, history)
	out, err := agent.Transform(context.Background(), "demo", historyStreamFromChunks())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	agentOutput.ch <- &genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("live"), Ctrl: &genx.StreamCtrl{StreamID: "live", Label: "assistant"}}
	if live, err := out.Next(); err != nil {
		t.Fatalf("Next live text: %v", err)
	} else if got, ok := live.Part.(genx.Text); !ok || string(got) != "live" {
		t.Fatalf("live text = %#v", live)
	}
	agentOutput.ch <- &genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{7}}, Ctrl: &genx.StreamCtrl{StreamID: "live", Label: "assistant"}}
	if liveAudio, err := out.Next(); err != nil {
		t.Fatalf("Next live audio: %v", err)
	} else if blob, ok := liveAudio.Part.(*genx.Blob); !ok || !bytes.Equal(blob.Data, []byte{7}) {
		t.Fatalf("live audio = %#v", liveAudio)
	}

	if resp, err := agent.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: entry.ID}); err != nil || !resp.Accepted {
		t.Fatalf("PlayHistory() = %+v, %v", resp, err)
	}
	interruptedText, err := out.Next()
	if err != nil {
		t.Fatalf("Next interrupted text: %v", err)
	}
	if interruptedText.Ctrl == nil || interruptedText.Ctrl.StreamID != "live" || interruptedText.Ctrl.Error != historyReplayInterrupted || !interruptedText.Ctrl.EndOfStream {
		t.Fatalf("interrupted text = %#v", interruptedText)
	}
	interruptedAudio, err := out.Next()
	if err != nil {
		t.Fatalf("Next interrupted audio: %v", err)
	}
	if interruptedAudio.Ctrl == nil || interruptedAudio.Ctrl.StreamID != "live" || interruptedAudio.Ctrl.Error != historyReplayInterrupted || !interruptedAudio.Ctrl.EndOfStream {
		t.Fatalf("interrupted audio = %#v", interruptedAudio)
	}

	agentOutput.ch <- &genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("stale"), Ctrl: &genx.StreamCtrl{StreamID: "live", Label: "assistant"}}
	agentOutput.ch <- &genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{8}}, Ctrl: &genx.StreamCtrl{StreamID: "live", Label: "assistant"}}
	replayText, err := out.Next()
	if err != nil {
		t.Fatalf("Next replay text: %v", err)
	}
	if got, ok := replayText.Part.(genx.Text); !ok || string(got) != "replay" {
		t.Fatalf("replay text = %#v", replayText)
	}
	replayEOS, err := out.Next()
	if err != nil {
		t.Fatalf("Next replay eos: %v", err)
	}
	if replayEOS.Ctrl == nil || replayEOS.Ctrl.StreamID == "live" || !replayEOS.Ctrl.EndOfStream {
		t.Fatalf("replay eos = %#v", replayEOS)
	}
	agentOutput.ch <- &genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "live", Label: "assistant", EndOfStream: true}}
	agentOutput.ch <- &genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "live", Label: "assistant", EndOfStream: true}}
	_ = agentOutput.Close()
	for {
		chunk, err := out.Next()
		if IsStreamDone(err) {
			break
		}
		if err != nil {
			t.Fatalf("Next after close: %v", err)
		}
		if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.StreamID == "live" && chunk.Ctrl.Error == "" {
			t.Fatalf("stale interrupted stream chunk leaked = %#v", chunk)
		}
	}
}

func TestHistoryOutputTracksForwardMIMEsIndependently(t *testing.T) {
	output := &historyOutput{}
	text := &genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("live"), Ctrl: &genx.StreamCtrl{StreamID: "live", Label: "assistant"}}
	audio := &genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "live", Label: "assistant"}}
	if output.observeForwardChunk(text) || output.observeForwardChunk(audio) {
		t.Fatal("active forward chunks were unexpectedly suppressed")
	}
	textEOS := text.Clone()
	textEOS.Part = genx.Text("")
	textEOS.Ctrl.EndOfStream = true
	if output.observeForwardChunk(textEOS) {
		t.Fatal("text EOS was unexpectedly suppressed")
	}

	interrupt := output.interruptForwardOutput()
	if len(interrupt) != 1 {
		t.Fatalf("interrupt chunks = %#v, want one audio EOS", interrupt)
	}
	mimeType, ok := interrupt[0].MIMEType()
	if !ok || mimeType != "audio/opus" || !interrupt[0].IsEndOfStream() {
		t.Fatalf("interrupt chunk = %#v", interrupt[0])
	}
	if !output.observeForwardChunk(audio) {
		t.Fatal("stale audio was not suppressed after sibling text EOS")
	}
	audioEOS := audio.Clone()
	audioEOS.Part = &genx.Blob{MIMEType: "audio/opus"}
	audioEOS.Ctrl.EndOfStream = true
	if !output.observeForwardChunk(audioEOS) {
		t.Fatal("stale audio EOS was not suppressed")
	}
}

func TestHistoryOutputForwardsControlEOSAndClearsRoute(t *testing.T) {
	output := &historyOutput{}
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("live"), Ctrl: &genx.StreamCtrl{StreamID: "live", Label: "assistant"}},
		{Role: genx.RoleModel, Name: "speaker", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "live", Label: "speaker"}},
	} {
		if output.observeForwardChunk(chunk) {
			t.Fatalf("active chunk was unexpectedly suppressed: %#v", chunk)
		}
	}
	if got := output.interruptForwardOutput(); len(got) != 2 {
		t.Fatalf("interrupt chunks = %#v, want two MIME EOS chunks", got)
	}
	status := &genx.MessageChunk{Role: genx.RoleModel, Name: "status", Part: genx.Text("working"), Ctrl: &genx.StreamCtrl{StreamID: "live", Label: "status"}}
	if output.observeForwardChunk(status) {
		t.Fatal("new label was unexpectedly suppressed")
	}
	routeEOS := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "live", EndOfStream: true}}
	if output.observeForwardChunk(routeEOS) {
		t.Fatal("control-only EOS was suppressed after interruption")
	}
	if len(output.activeForward) != 0 || len(output.interruptedForward) != 0 {
		t.Fatalf("forward state after route EOS = active %d, interrupted %d", len(output.activeForward), len(output.interruptedForward))
	}
	if got := output.interruptForwardOutput(); len(got) != 0 {
		t.Fatalf("completed route emitted interruption chunks: %#v", got)
	}
}

func TestHistoryPCMFormatAndChunkNames(t *testing.T) {
	for _, tc := range []struct {
		mime string
		ok   bool
		rate int
	}{
		{mime: "audio/pcm", ok: true, rate: 16000},
		{mime: "audio/L16; rate=24000; channels=1", ok: true, rate: 24000},
		{mime: "audio/L16; rate=48000; channels=1", ok: true, rate: 48000},
		{mime: "audio/L16; rate=8000; channels=1", ok: false},
		{mime: "audio/L16; rate=16000; channels=2", ok: false},
		{mime: "audio/mpeg", ok: false},
	} {
		format, ok := historyPCMFormat(tc.mime)
		if ok != tc.ok {
			t.Fatalf("historyPCMFormat(%q) ok = %t, want %t", tc.mime, ok, tc.ok)
		}
		if ok && format.SampleRate() != tc.rate {
			t.Fatalf("historyPCMFormat(%q) rate = %d, want %d", tc.mime, format.SampleRate(), tc.rate)
		}
	}
	if got := historyChunkName(&genx.MessageChunk{Name: " named "}, historyEntryTypeAgent); got != "named" {
		t.Fatalf("historyChunkName name = %q", got)
	}
	if got := historyChunkName(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{Label: " label "}}, historyEntryTypeAgent); got != "label" {
		t.Fatalf("historyChunkName label = %q", got)
	}
	if got := historyChunkName(nil, historyEntryTypeGear); got != "gear" {
		t.Fatalf("historyChunkName gear = %q", got)
	}
	if !historyIsNotExist(fs.ErrNotExist) {
		t.Fatal("historyIsNotExist(fs.ErrNotExist) = false")
	}
}

type historyTestAgent struct {
	output genx.Stream
}

func (a historyTestAgent) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return a.output, nil
}

func (a historyTestAgent) Status(context.Context) (apitypes.PeerRunWorkspaceState, error) {
	return apitypes.PeerRunWorkspaceState{RuntimeState: apitypes.PeerRunStatusStateRunning}, nil
}

func (a historyTestAgent) ListHistory(context.Context, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	return apitypes.PeerRunHistoryListResponse{}, nil
}

func (a historyTestAgent) PlayHistory(context.Context, apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	return apitypes.PeerRunHistoryPlayResponse{}, nil
}

func (a historyTestAgent) MemoryStats(context.Context, apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	return apitypes.PeerRunMemoryStatsResponse{}, nil
}

func (a historyTestAgent) Recall(context.Context, apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	return apitypes.PeerRunRecallResponse{}, nil
}

type historyMultiOutputAgent struct {
	mu      sync.Mutex
	outputs []*blockingHistoryStream
}

func (a *historyMultiOutputAgent) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	output := newBlockingHistoryStream()
	a.mu.Lock()
	a.outputs = append(a.outputs, output)
	a.mu.Unlock()
	return output, nil
}

func (a *historyMultiOutputAgent) Status(context.Context) (apitypes.PeerRunWorkspaceState, error) {
	return apitypes.PeerRunWorkspaceState{RuntimeState: apitypes.PeerRunStatusStateRunning}, nil
}

func (a *historyMultiOutputAgent) ListHistory(context.Context, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	return apitypes.PeerRunHistoryListResponse{}, nil
}

func (a *historyMultiOutputAgent) PlayHistory(context.Context, apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	return apitypes.PeerRunHistoryPlayResponse{}, nil
}

func (a *historyMultiOutputAgent) MemoryStats(context.Context, apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	return apitypes.PeerRunMemoryStatsResponse{}, nil
}

func (a *historyMultiOutputAgent) Recall(context.Context, apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	return apitypes.PeerRunRecallResponse{}, nil
}

func (a *historyMultiOutputAgent) closeAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, output := range a.outputs {
		_ = output.Close()
	}
}

type historyAsyncInputDrainingAgent struct {
	mu      sync.Mutex
	outputs []*blockingHistoryStream
}

func (a *historyAsyncInputDrainingAgent) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	output := newBlockingHistoryStream()
	a.mu.Lock()
	a.outputs = append(a.outputs, output)
	a.mu.Unlock()
	go func() {
		defer input.Close()
		for {
			if err := ctx.Err(); err != nil {
				return
			}
			_, err := input.Next()
			if IsStreamDone(err) {
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return output, nil
}

func (a *historyAsyncInputDrainingAgent) Status(context.Context) (apitypes.PeerRunWorkspaceState, error) {
	return apitypes.PeerRunWorkspaceState{RuntimeState: apitypes.PeerRunStatusStateRunning}, nil
}

func (a *historyAsyncInputDrainingAgent) ListHistory(context.Context, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	return apitypes.PeerRunHistoryListResponse{}, nil
}

func (a *historyAsyncInputDrainingAgent) PlayHistory(context.Context, apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	return apitypes.PeerRunHistoryPlayResponse{}, nil
}

func (a *historyAsyncInputDrainingAgent) MemoryStats(context.Context, apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	return apitypes.PeerRunMemoryStatsResponse{}, nil
}

func (a *historyAsyncInputDrainingAgent) Recall(context.Context, apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	return apitypes.PeerRunRecallResponse{}, nil
}

func (a *historyAsyncInputDrainingAgent) closeAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, output := range a.outputs {
		_ = output.Close()
	}
}

type historyInputDrainingAgent struct {
	historyTestAgent
}

func (a historyInputDrainingAgent) Transform(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	for {
		_, err := input.Next()
		if IsStreamDone(err) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return a.output, nil
}

type historySliceStream struct {
	chunks []*genx.MessageChunk
	index  int
}

func historyStreamFromChunks(chunks ...*genx.MessageChunk) *historySliceStream {
	return &historySliceStream{chunks: chunks}
}

func (s *historySliceStream) Next() (*genx.MessageChunk, error) {
	if s.index >= len(s.chunks) {
		return nil, genx.ErrDone
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (s *historySliceStream) Close() error {
	return nil
}

func (s *historySliceStream) CloseWithError(error) error {
	return nil
}

type blockingHistoryStream struct {
	ch   chan *genx.MessageChunk
	once sync.Once
}

func newBlockingHistoryStream() *blockingHistoryStream {
	return &blockingHistoryStream{ch: make(chan *genx.MessageChunk)}
}

func (s *blockingHistoryStream) Next() (*genx.MessageChunk, error) {
	chunk, ok := <-s.ch
	if !ok {
		return nil, genx.ErrDone
	}
	return chunk, nil
}

func (s *blockingHistoryStream) Close() error {
	s.once.Do(func() { close(s.ch) })
	return nil
}

func (s *blockingHistoryStream) CloseWithError(err error) error {
	return s.Close()
}
