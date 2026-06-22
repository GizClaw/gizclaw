package agenthost

import (
	"bytes"
	"context"
	"io/fs"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workspace"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
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

func TestHistoryAgentRecordsInputPCMAudioAsOggOpus(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agent := wrapHistoryAgent(historyInputDrainingAgent{
		historyTestAgent: historyTestAgent{output: historyStreamFromChunks()},
	}, history)
	pcmFrame := historyTestPCMFrame(320)

	out, err := agent.Transform(withHistoryGearID(context.Background(), "gear-a"), "demo", historyStreamFromChunks(
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: pcmFrame[:300]}, Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "input"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: pcmFrame[300:]}, Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "input"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "input", EndOfStream: true}},
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

func TestHistoryAgentMergesInputAudioWithTranscript(t *testing.T) {
	history := workspace.NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	agent := wrapHistoryAgent(historyInputDrainingAgent{
		historyTestAgent: historyTestAgent{output: historyStreamFromChunks(
			&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "transcript"}},
			&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "transcript", EndOfStream: true}},
		)},
	}, history)

	out, err := agent.Transform(withHistoryGearID(context.Background(), "gear-a"), "demo", historyStreamFromChunks(
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}, Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "workspacetest"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{4, 5}}, Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "workspacetest"}},
		&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", Label: "workspacetest", EndOfStream: true}},
	))
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

func TestHistoryAgentPlayInterruptsPreviousReplay(t *testing.T) {
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
	if audio, err := out.Next(); err != nil {
		t.Fatalf("Next first audio: %v", err)
	} else if blob, ok := audio.Part.(*genx.Blob); !ok || !bytes.Equal(blob.Data, []byte{1}) {
		t.Fatalf("first audio = %#v", audio)
	}
	if resp, err := agent.PlayHistory(context.Background(), apitypes.PeerRunHistoryPlayRequest{HistoryId: second.ID}); err != nil || !resp.Accepted {
		t.Fatalf("PlayHistory(second) = %+v, %v", resp, err)
	}
	interruptedText, err := out.Next()
	if err != nil {
		t.Fatalf("Next interrupted text eos: %v", err)
	}
	if interruptedText.Ctrl == nil || interruptedText.Ctrl.Error != historyReplayInterrupted || !interruptedText.Ctrl.EndOfStream {
		t.Fatalf("interrupted text eos = %#v", interruptedText)
	}
	interruptedAudio, err := out.Next()
	if err != nil {
		t.Fatalf("Next interrupted audio eos: %v", err)
	}
	if interruptedAudio.Ctrl == nil || interruptedAudio.Ctrl.Error != historyReplayInterrupted || !interruptedAudio.Ctrl.EndOfStream {
		t.Fatalf("interrupted audio eos = %#v", interruptedAudio)
	}
	secondText, err := out.Next()
	if err != nil {
		t.Fatalf("Next second text: %v", err)
	}
	if got, ok := secondText.Part.(genx.Text); !ok || string(got) != "second" {
		t.Fatalf("second text = %#v", secondText)
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
