package giztestcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

func TestReplayRelaySentPCMRetainsSourceAndReceivedOriginals(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus")
	}
	directory := filepath.Join(t.TempDir(), "out")
	record, err := newReplayRecord(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer record.directory.abort()
	capture := record.capture("relay.yaml", "relay-0000", giztest.StepRequest{Step: giztest.Step{ID: "relay"}})
	raw := newFakeRelayStream()
	defer raw.Close()
	side := &relaySide{}
	receiver := &relaySide{stream: capture.wrap("candidate", raw)}
	started := time.Now()
	original := []byte{0xf8, 0xff, 0xfe}
	var want []byte
	for i, concealed := range []bool{false, true, false} {
		data := bytes.Repeat([]byte{byte(i + 1), 0}, 320)
		blob := giztestPCM(data, original, false)
		if concealed {
			blob.OriginalAudio, blob.ConcealedAudio = nil, true
		}
		capture.observe("tester", "received", &genx.MessageChunk{Part: blob, Ctrl: &genx.StreamCtrl{StreamID: "answer", Label: "assistant"}}, started.Add(time.Duration(i)*20*time.Millisecond))
		if err := forwardRelayAudio(t.Context(), side, receiver, blob); err != nil {
			t.Fatal(err)
		}
		want = append(want, data...)
		clear(data) // Both replay directions must retain their own source bytes.
	}
	capture.observe("tester", "received", &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", OriginalAudio: &genx.Blob{MIMEType: "audio/opus"}}, Ctrl: &genx.StreamCtrl{StreamID: "answer", Label: "assistant", EndOfStream: true}}, started.Add(60*time.Millisecond))
	if err := forwardRelayTerminal(t.Context(), &giztest.WorkspaceRelayOperation{Media: "audio"}, side, receiver); err != nil {
		t.Fatal(err)
	}
	if len(record.audio) != 0 {
		t.Fatal("PCM EOS did not close the replay track")
	}
	if err := record.commit(giztest.Report{StartedAt: started, Status: "passed"}); err != nil {
		t.Fatal(err)
	}
	var sent, received *replayEvent
	var sentBegins, sentEnds int
	data := readReplay(t, directory)
	for i := range data.Events {
		event := &data.Events[i]
		if event.Direction == "sent" {
			if event.BOS {
				sentBegins++
			}
			if event.EOS {
				sentEnds++
			}
		}
		if event.Audio != "" {
			if event.Direction == "sent" {
				sent = event
			} else {
				received = event
			}
		}
	}
	if sent == nil || sent.Role != "user" || sent.Client != "candidate" || sent.DurationMS != 60 || sent.OriginalAudio != "" || sent.Packets != 0 || !strings.HasPrefix(sent.AudioData, "data:audio/wav;base64,") || sentBegins != 1 || sentEnds != 1 {
		t.Fatalf("sent PCM source or lifecycle missing: %+v", data.Events)
	}
	wav, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(sent.Audio)))
	if err != nil || len(wav) != 44+len(want) || !bytes.Equal(wav[44:], want) {
		t.Fatalf("sent source WAV changed: bytes=%d error=%v", len(wav), err)
	}
	if received == nil || received.MIME != "audio/opus" || received.Packets != 2 || received.DurationMS != 40 || received.OriginalAudio == "" {
		t.Fatalf("received recording includes synthesized media: %+v", received)
	}
}

func TestReplayRecoveredPCMRetainsRealOpusAndLifecycle(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus")
	}
	directory := filepath.Join(t.TempDir(), "out")
	record, err := newReplayRecord(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer record.directory.abort()
	capture := record.capture("scene.yaml", "scene-0000", giztest.StepRequest{Step: giztest.Step{ID: "turn"}})
	started := time.Now()
	original := []byte{0xf8, 0xff, 0xfe}
	for i := range 5 {
		blob := &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1"}
		ctrl := &genx.StreamCtrl{StreamID: "answer", Label: "assistant", BeginOfStream: i == 0, EndOfStream: i == 4}
		if i == 2 {
			blob.Data, blob.ConcealedAudio = make([]byte, 640), true
		} else {
			blob.OriginalAudio = &genx.Blob{MIMEType: "audio/opus"}
			if i != 0 && i != 4 {
				blob.Data = make([]byte, 640)
				blob.OriginalAudio.Data = original
			}
		}
		capture.observe("peer", "received", &genx.MessageChunk{Part: blob, Ctrl: ctrl}, started.Add(time.Duration(i)*20*time.Millisecond), 7)
	}
	if err := record.commit(giztest.Report{StartedAt: started, DurationMS: 100, Status: "passed"}); err != nil {
		t.Fatal(err)
	}
	replay := readReplay(t, directory)
	var clip *replayEvent
	var begins, ends int
	for i := range replay.Events {
		event := &replay.Events[i]
		if event.BOS {
			begins++
		}
		if event.EOS {
			ends++
		}
		if event.MIME != "audio/opus" {
			t.Fatalf("replay stored converted PCM as encoded audio: %q", event.MIME)
		}
		if event.Audio != "" {
			if clip != nil {
				t.Fatal("one audio epoch was split into multiple replay clips")
			}
			clip = event
		}
	}
	if clip == nil || clip.Packets != 2 || clip.DurationMS != 40 || begins != 1 || ends != 1 {
		t.Fatalf("replay lost real originals or lifecycle: %+v", replay.Events)
	}
	for _, event := range replay.Events {
		if event.Stream != clip.Stream {
			t.Fatal("PCM boundaries and recorded audio have different routes")
		}
	}
	ogg, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(clip.OriginalAudio)))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for packet, err := range codecconv.OggOpusPackets(bytes.NewReader(ogg)) {
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(packet, original) {
			t.Fatal("PCM or synthetic PLC was stored as original Opus")
		}
		count++
	}
	if count != 2 {
		t.Fatalf("recorded packets=%d, want only two real originals", count)
	}
	wav, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(clip.Audio)))
	if err != nil || len(wav) != 44+2*640 || string(wav[:4]) != "RIFF" {
		t.Fatalf("replay WAV bytes=%d, error=%v", len(wav), err)
	}
}
