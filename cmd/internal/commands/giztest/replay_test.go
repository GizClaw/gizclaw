package giztestcmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func readReplay(t *testing.T, directory string) replayData {
	t.Helper()
	path := filepath.Join(directory, "replay.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		paths, err := filepath.Glob(filepath.Join(directory, "*", "replay.json"))
		if err != nil || len(paths) != 1 {
			t.Fatalf("expected one test replay, paths=%v error=%v", paths, err)
		}
		path = paths[0]
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var replay replayData
	if err := json.Unmarshal(data, &replay); err != nil {
		t.Fatal(err)
	}
	return replay
}

func TestReplayPacketTimesAndTerminalDataArePreserved(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "out")
	record, err := newReplayRecord(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer record.directory.abort()
	started := time.Now()
	capture := record.capture("scene.yaml", "scene-0000", giztest.StepRequest{Step: giztest.Step{ID: "turn"}})
	for i, offset := range []time.Duration{200 * time.Millisecond, 200 * time.Millisecond, 800 * time.Millisecond} {
		ctrl := &genx.StreamCtrl{StreamID: "private-route", Label: "assistant", BeginOfStream: i == 0}
		if i == 2 {
			ctrl.EndOfStream, ctrl.Error = true, "interrupted"
		}
		capture.observe("peer", "received", &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}, Ctrl: ctrl}, started.Add(offset), 7)
	}
	if err := record.commit(giztest.Report{StartedAt: started, DurationMS: 900, Status: "passed"}); err != nil {
		t.Fatal(err)
	}
	replay := readReplay(t, directory)
	if len(replay.Events) != 3 {
		t.Fatalf("media, BOS and terminal events = %#v", replay.Events)
	}
	audio, begin, terminal := replay.Events[0], replay.Events[1], replay.Events[2]
	if audio.DurationMS != 60 || audio.EOS || audio.Error != "" || audio.Connection != 7 {
		t.Fatalf("audio = %#v", audio)
	}
	if !begin.BOS || begin.OffsetMS != 200 || !terminal.EOS || terminal.OffsetMS != 800 || !strings.Contains(terminal.Error, "interrupted") || terminal.Stream != audio.Stream {
		t.Fatalf("begin/terminal = %#v / %#v", begin, terminal)
	}
	want := []float64{200, 200, 800}
	if len(audio.AudioTiming) != len(want) {
		t.Fatalf("audio timing = %#v", audio.AudioTiming)
	}
	for i, timing := range audio.AudioTiming {
		if timing.OffsetMS != want[i] || timing.DurationMS != 20 {
			t.Fatalf("chunk %d timing = %#v", i, timing)
		}
	}
}

func TestReplayCloseAndRouteIdentitySurviveCaptureRebinding(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "out")
	record, err := newReplayRecord(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer record.directory.abort()
	started := time.Now()
	first := record.capture("scene.yaml", "scene-0000", giztest.StepRequest{Step: giztest.Step{ID: "listen"}})
	raw := newFakeRelayStream()
	stream := first.wrap("peer", raw)
	raw.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}, Ctrl: &genx.StreamCtrl{StreamID: "private-route", Label: "assistant"}}
	if _, err := stream.Next(); err != nil {
		t.Fatal(err)
	}
	second := record.capture("scene.yaml", "scene-0000", giztest.StepRequest{Step: giztest.Step{ID: "interrupt"}})
	bindReplayCapture(stream, second)
	raw.in <- &genx.MessageChunk{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "private-route", Label: "assistant", EndOfStream: true, Error: "interrupted"}}
	if _, err := stream.Next(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := stream.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := record.commit(giztest.Report{StartedAt: started, Status: "passed"}); err != nil {
		t.Fatal(err)
	}
	events := readReplay(t, directory).Events
	if len(events) != 3 || events[0].Step != "listen" || events[1].Step != "interrupt" || events[0].Stream != events[1].Stream {
		t.Fatalf("route identity across steps = %#v", events)
	}
	if events[2].Action != "close" || events[2].Connection == 0 || events[2].Connection != events[0].Connection || events[2].OffsetMS < events[1].OffsetMS {
		t.Fatalf("close event = %#v", events[2])
	}
}

func TestReplayTimingKeepsMultiPacketChunkDuration(t *testing.T) {
	var ogg bytes.Buffer
	if err := codecconv.OpusPacketsToOgg(&ogg, 16000, 1, [][]byte{{0xf8, 0xff, 0xfe}, {0xf8, 0xff, 0xfe}}); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	packets, timing, err := replayPackets(&replayAudio{chunks: [][]byte{ogg.Bytes()}, times: []time.Time{started.Add(100 * time.Millisecond)}}, started)
	if err != nil || len(packets) != 2 || len(timing) != 1 || timing[0].OffsetMS != 100 || timing[0].DurationMS != 40 {
		t.Fatalf("multi-packet timing = %#v, packets = %d, error = %v", timing, len(packets), err)
	}
}

type replayBlockedPushStream struct {
	peerStream
	entered chan struct{}
	release chan struct{}
}

func (s *replayBlockedPushStream) Push(ctx context.Context, _ *genx.MessageChunk) error {
	close(s.entered)
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestReplaySendTimePrecedesReplyWhilePushIsBlocked(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	directory := filepath.Join(t.TempDir(), "out")
	record, err := newReplayRecord(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer record.directory.abort()
	started := time.Now()
	capture := record.capture("scene.yaml", "scene-0000", giztest.StepRequest{Step: giztest.Step{ID: "turn"}})
	raw := newFakeRelayStream()
	defer raw.Close()
	blocked := &replayBlockedPushStream{peerStream: raw, entered: make(chan struct{}), release: make(chan struct{})}
	stream := capture.wrap("peer", blocked)
	done := make(chan error, 1)
	go func() { done <- stream.Push(ctx, &genx.MessageChunk{Part: genx.Text("input")}) }()
	select {
	case <-blocked.entered:
	case <-ctx.Done():
		t.Fatal("sender did not enter Push")
	}
	raw.in <- &genx.MessageChunk{Part: genx.Text("reply")}
	if _, err := stream.Next(); err != nil {
		t.Fatal(err)
	}
	close(blocked.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := record.commit(giztest.Report{StartedAt: started, Status: "passed"}); err != nil {
		t.Fatal(err)
	}
	events := readReplay(t, directory).Events
	if len(events) != 2 || events[0].Direction != "sent" || events[1].Direction != "received" || events[0].OffsetMS > events[1].OffsetMS {
		t.Fatalf("send/receive causality = %#v", events)
	}
}

func TestReplayRecordOfflineArtifactsAndAudioBoundaries(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native Opus runtime unavailable")
	}
	directory := filepath.Join(t.TempDir(), "out")
	record, err := newReplayRecord(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer record.directory.abort()
	started := time.Now()
	capture := record.capture("scene.giztest.yaml", "scene-0000", giztest.StepRequest{Step: giztest.Step{ID: "turn"}})
	capture.redactions = []string{"fixture-secret"}
	text := "你好 </script><script>window.injected=true</script> fixture-secret"
	capture.observe("peer", "sent", &genx.MessageChunk{Part: genx.Text(text)}, started)
	packets := voiceTonePacketsCount(t, 440, 3)
	packetCopy := bytes.Clone(packets[0])
	for i, packet := range packets {
		capture.observe("peer", "received", &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: "remote-id", Label: "assistant"}}, started.Add(time.Duration(i)*20*time.Millisecond))
	}
	packets[0][0] ^= 1 // retained audio must own its bytes
	capture.observe("peer", "received", &genx.MessageChunk{Part: genx.Text("回答"), Ctrl: &genx.StreamCtrl{StreamID: "remote-id", Label: "assistant", EndOfStream: true}}, started.Add(65*time.Millisecond))
	capture.observe("peer", "received", &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "remote-id", Label: "assistant", EndOfStream: true, Error: "interrupted"}}, started.Add(70*time.Millisecond))
	// Reusing the same stream ID after audio EOS creates another playable clip.
	capture.observe("peer", "received", &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packetCopy}, Ctrl: &genx.StreamCtrl{StreamID: "remote-id", Label: "assistant"}}, started.Add(100*time.Millisecond))
	report := giztest.Report{Version: "v1", Status: "failed", StartedAt: started, DurationMS: 120, Tasks: []giztest.TaskReport{{Path: "scene.giztest.yaml", Name: "scene", TaskID: "scene-0000", Status: "failed"}}}
	if err := record.commit(report); err != nil {
		t.Fatal(err)
	}
	replay := readReplay(t, directory)
	if replay.Report.Status != "failed" || replay.Truncated || len(replay.Errors) != 0 {
		t.Fatalf("replay = %#v", replay)
	}
	var clips []replayEvent
	for _, event := range replay.Events {
		if event.Audio != "" {
			clips = append(clips, event)
		}
	}
	if len(clips) != 2 || clips[0].Packets != 3 || clips[0].DurationMS != 60 || clips[1].Packets != 1 {
		t.Fatalf("audio clips = %#v", clips)
	}
	ogg, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(clips[0].OriginalAudio)))
	if err != nil {
		t.Fatal(err)
	}
	var decoded [][]byte
	for packet, err := range codecconv.OggOpusPackets(bytes.NewReader(ogg)) {
		if err != nil {
			t.Fatal(err)
		}
		decoded = append(decoded, packet)
	}
	if len(decoded) != 3 || !bytes.Equal(decoded[0], packetCopy) {
		t.Fatal("export changed the original Opus packets")
	}
	wav, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(clips[0].Audio)))
	if err != nil {
		t.Fatal(err)
	}
	if string(wav[:4]) != "RIFF" || binary.LittleEndian.Uint32(wav[40:44]) != 3*320*2 || len(wav) != 44+3*320*2 {
		t.Fatalf("invalid WAV header or length: %d", len(wav))
	}
	for _, name := range []string{"report.json", "replay.json", "transcript.txt", "index.html"} {
		path := filepath.Join(directory, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s permissions = %o", name, info.Mode().Perm())
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(content, []byte("fixture-secret")) || bytes.Contains(content, []byte("remote-id")) {
			t.Fatalf("%s exposed a secret or remote stream identity", name)
		}
		if name == "index.html" && (bytes.Contains(content, []byte(text)) || bytes.Contains(content, []byte("__GIZTEST_DATA__")) || bytes.Contains(content, []byte("fetch("))) {
			t.Fatal("HTML needs a server or permits conversation script injection")
		}
	}
	if _, err := os.Stat(filepath.Join(directory, "audio.ogg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("run export invented a combined play-mode recording")
	}
}

func TestReplaySynthesisIncludesPreparedOggAudio(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native Opus runtime unavailable")
	}
	directory := filepath.Join(t.TempDir(), "out")
	record, err := newReplayRecord(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer record.directory.abort()
	step := giztest.Step{ID: "synthesize", Client: "peer", Speech: &giztest.SpeechOperation{Method: "server.speech.synthesize"}}
	capture := record.capture("scene.yaml", "scene-0000", giztest.StepRequest{Step: step})
	metadata, err := jsonObject(rpcapi.SpeechSynthesizeResponse{ContentType: "audio/ogg"})
	if err != nil {
		t.Fatal(err)
	}
	var audio bytes.Buffer
	if err := codecconv.OpusPacketsToOgg(&audio, 16000, 1, voiceTonePacketsCount(t, 440, 3)); err != nil {
		t.Fatal(err)
	}
	capture.observeSpeech(step, map[string]any{"text": "question"}, operationResult{assertion: metadata, saved: audio.Bytes()})
	if err := record.commit(giztest.Report{Status: "passed", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	replay := readReplay(t, directory)
	if len(replay.Events) != 2 || replay.Events[0].Text != "question" {
		t.Fatalf("prepared speech missing: %#v", replay.Events)
	}
	clip := replay.Events[1]
	if clip.Direction != "prepared" || clip.Packets != 3 || !strings.HasPrefix(clip.AudioData, "data:audio/wav;base64,") {
		t.Fatalf("prepared audio missing: %#v", clip)
	}
}

func TestReplayRecordTruncationPreservesReportAndTextOnlyExport(t *testing.T) {
	for _, limited := range []bool{false, true} {
		t.Run(map[bool]string{false: "text-only", true: "bounded"}[limited], func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "out")
			record, err := newReplayRecord(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer record.directory.abort()
			if limited {
				record.maxBytes = 4
			}
			capture := record.capture("text.yaml", "text-0000", giztest.StepRequest{Step: giztest.Step{ID: "turn"}})
			capture.observe("peer", "received", &genx.MessageChunk{Part: genx.Text("hello")}, time.Now())
			report := giztest.Report{Status: "passed", StartedAt: time.Now(), Tasks: []giztest.TaskReport{{Name: "text", Path: "text.yaml", TaskID: "text-0000", Status: "passed"}}}
			err = record.commit(report)
			if (err != nil) != limited {
				t.Fatalf("commit error = %v", err)
			}
			replay := readReplay(t, directory)
			if replay.Truncated != limited || replay.Report.Status != "passed" {
				t.Fatalf("replay=%#v", replay)
			}
			if _, err := os.Stat(filepath.Join(directory, "audio")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("text-only export created fake audio")
			}
			if _, err := os.Stat(filepath.Join(directory, "index.html")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type replayEchoStream struct {
	in   chan *genx.MessageChunk
	done chan struct{}
	once sync.Once
}

func newReplayEchoStream() *replayEchoStream {
	return &replayEchoStream{in: make(chan *genx.MessageChunk, 4), done: make(chan struct{})}
}
func (s *replayEchoStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	text, ok := chunk.Part.(genx.Text)
	if !ok || text == "" {
		return nil
	}
	response := &genx.MessageChunk{Part: genx.Text("reply:" + string(text)), Ctrl: &genx.StreamCtrl{Label: "assistant", StreamID: chunk.Ctrl.StreamID, EndOfStream: true}}
	select {
	case s.in <- response:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *replayEchoStream) Next() (*genx.MessageChunk, error) {
	select {
	case chunk := <-s.in:
		return chunk, nil
	case <-s.done:
		return nil, io.EOF
	}
}
func (s *replayEchoStream) Close() error { s.once.Do(func() { close(s.done) }); return nil }

func TestReplayDriverRepeatParallelAndCleanupIsolation(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "out")
	record, err := newReplayRecord(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer record.directory.abort()
	noAudio := false
	streamStep := func(id, client string) giztest.Step {
		return giztest.Step{ID: id, Client: client, Timeout: "1s", PeerStream: &giztest.PeerStreamOperation{Mode: "text", Input: id, RequireAudio: &noAudio}}
	}
	doc := &giztest.Document{Name: "repeat", Path: "repeat.giztest.yaml", Repeat: 4, Clients: map[string]giztest.ClientSpec{"a": {}, "b": {}}, Variables: map[string]giztest.VariableSpec{"note": {Type: "string", Direction: "input", Value: "cleanup completed"}}, Steps: []giztest.Step{{ID: "both", Parallel: []giztest.Step{streamStep("left", "a"), streamStep("right", "b")}}}, Finally: []giztest.Step{{ID: "cleanup", Output: &giztest.OutputOperation{Variable: "note"}}}}
	testDriver := newDriver(false, nil)
	testDriver.recording = record
	testDriver.connectClients = func(context.Context, map[string]giztest.ClientSpec, []giztest.Step, *giztest.Variables) (*clientSet, error) {
		return &clientSet{clients: map[string]*gizcli.Client{"a": {}, "b": {}}}, nil
	}
	testDriver.openPeerStream = func(*gizcli.Client) peerStreamOpener {
		return func() (peerStream, error) { return newReplayEchoStream(), nil }
	}
	report := giztest.Run(context.Background(), []*giztest.Document{doc}, giztest.Options{Driver: testDriver, Parallel: 4, Out: io.Discard, ObserveOutput: record.observeOutput})
	if report.Status != "passed" {
		t.Fatalf("report=%#v", report)
	}
	if err := record.commit(report); err != nil {
		t.Fatal(err)
	}
	replay := readReplay(t, directory)
	if len(replay.Report.Tasks) != 4 {
		t.Fatal("single-file recording lost repeats")
	}
	counts := map[string]int{}
	cleanup := map[string]bool{}
	for _, event := range replay.Events {
		if event.TaskID == "" {
			t.Fatal("missing repeat task identity")
		}
		if event.Text != "" {
			counts[event.TaskID]++
		}
		if event.Cleanup {
			cleanup[event.TaskID] = event.Text == "cleanup completed"
		}
		if event.Step == "left" || event.Step == "right" {
			if event.Parent != "both" || event.Attempt != 1 {
				t.Fatalf("parallel metadata=%#v", event)
			}
		}
	}

	if len(counts) != 4 {
		t.Fatalf("tasks=%v", counts)
	}
	for task, count := range counts {
		if count != 5 || !cleanup[task] {
			t.Fatalf("%s content count=%d cleanup=%t", task, count, cleanup[task])
		}
	}
}

func TestReplayOutputRefusesOverwriteAndReportCollision(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing")
	if err := os.Mkdir(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := newReplayRecord(existing); err == nil {
		t.Fatal("existing directory accepted")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(existing, link); err != nil {
		t.Fatal(err)
	}
	if _, err := newReplayRecord(link); err == nil {
		t.Fatal("symlink destination accepted")
	}
	for _, name := range []string{"index.html", "replay.json", "transcript.txt", "audio/clip.ogg"} {
		if err := validateReplayReportOutput(existing, filepath.Join(existing, name)); err == nil {
			t.Fatalf("collision accepted: %s", name)
		}
	}
	for _, path := range []string{filepath.Join(existing, "report.json"), filepath.Join(root, "external.json")} {
		if err := validateReplayReportOutput(existing, path); err != nil {
			t.Fatal(err)
		}
	}
	cmd := newRunCmd()
	if cmd.Flags().Lookup("output") == nil || cmd.Flags().Lookup("output").Shorthand != "o" || !strings.Contains(cmd.Flags().Lookup("output").Usage, "HTML") {
		t.Fatal("run does not expose offline export")
	}
}

func TestReplayDriverRetryRetainsFailedAttempt(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "out")
	record, err := newReplayRecord(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer record.directory.abort()
	noAudio := false
	doc := &giztest.Document{Name: "retry", Path: "retry.giztest.yaml", Repeat: 1, Clients: map[string]giztest.ClientSpec{"peer": {}}, Variables: map[string]giztest.VariableSpec{}, Steps: []giztest.Step{{ID: "turn", Client: "peer", Timeout: "1s", Retry: &giztest.RetrySpec{Attempts: 2, On: []string{"operation"}}, PeerStream: &giztest.PeerStreamOperation{Mode: "text", Input: "hello", RequireAudio: &noAudio}}}}
	testDriver := newDriver(false, nil)
	testDriver.recording = record
	testDriver.connectClients = func(context.Context, map[string]giztest.ClientSpec, []giztest.Step, *giztest.Variables) (*clientSet, error) {
		return &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}, nil
	}
	var opens atomic.Int32
	testDriver.openPeerStream = func(*gizcli.Client) peerStreamOpener {
		return func() (peerStream, error) {
			if opens.Add(1) == 1 {
				return nil, errors.New("fixture open error")
			}
			return newReplayEchoStream(), nil
		}
	}
	report := giztest.Run(context.Background(), []*giztest.Document{doc}, giztest.Options{Driver: testDriver, Parallel: 1, Out: io.Discard})
	if report.Status != "passed" || len(report.Tasks[0].Steps[0].Attempts) != 2 || report.Tasks[0].Steps[0].Attempts[0].Status != "failed" {
		t.Fatalf("retry report=%#v", report)
	}
	if err := record.commit(report); err != nil {
		t.Fatal(err)
	}
	replay := readReplay(t, directory)
	for _, event := range replay.Events {
		if event.Attempt != 2 {
			t.Fatalf("retry content attributed to wrong attempt: %#v", event)
		}
	}
}

func TestReplayCommandCanceledRunExportsFailedReport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	directory := filepath.Join(t.TempDir(), "out")
	command := NewCmd()
	command.SetContext(ctx)
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"run", writeTestDocument(t, validDocument), "--output", directory, "--evidence", "full"})
	err := command.Execute()
	if coded, ok := err.(interface{ ExitCode() int }); !ok || coded.ExitCode() != exitExecution {
		t.Fatalf("error=%#v", err)
	}
	replay := readReplay(t, directory)
	if replay.Report.Status != "failed" || len(replay.Report.Tasks) != 1 || replay.Report.Tasks[0].Error != "context canceled" {
		t.Fatalf("canceled report=%#v", replay.Report)
	}
	pages, _ := filepath.Glob(filepath.Join(directory, "index.html"))
	if len(pages) != 1 {
		t.Fatalf("missing per-test HTML: %v", pages)
	}
}

func TestReplayHeldPacketTimingStreamUsesCurrentStep(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "out")
	record, err := newReplayRecord(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer record.directory.abort()
	first := record.capture("held.yaml", "held-0000", giztest.StepRequest{Step: giztest.Step{ID: "first"}})
	underlying := newReplayEchoStream()
	stream := &packetTimedPeerStream{peerStream: first.wrap("peer", underlying)}
	streams := newPeerStreamSessions()
	if err := streams.add("held", newPeerStreamSession("peer", stream)); err != nil {
		t.Fatal(err)
	}
	defer streams.Close()
	session := &session{driver: &driver{recording: record}, clients: &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}, streams: streams, taskID: "held-0000"}
	step := giztest.Step{ID: "next", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "listen", Session: "held", Duration: "10ms"}}
	vars, err := giztest.NewVariables(nil)
	if err != nil {
		t.Fatal(err)
	}
	capture := session.replayCapture(giztest.StepRequest{DocumentPath: "held.yaml", Step: step})
	if _, err := session.preparePeerStream(step, step, vars, capture); err != nil {
		t.Fatal(err)
	}
	underlying.in <- &genx.MessageChunk{Part: genx.Text("next reply")}
	if _, err := stream.Next(); err != nil {
		t.Fatal(err)
	}
	data, _ := record.snapshot()
	if len(data.Events) != 1 || data.Events[0].Step != "next" {
		t.Fatalf("resumed events=%#v", data.Events)
	}
}

func TestReplayAndIndependentReportsSurviveEachOthersExportErrors(t *testing.T) {
	for _, brokenReplay := range []bool{true, false} {
		t.Run(map[bool]string{true: "partial-replay", false: "invalid-report-destination"}[brokenReplay], func(t *testing.T) {
			root := t.TempDir()
			record, err := newReplayRecord(filepath.Join(root, "out"))
			if err != nil {
				t.Fatal(err)
			}
			defer record.directory.abort()
			output := filepath.Join(root, "external.json")
			if brokenReplay {
				record.maxBytes = 1
				capture := record.capture("report.yaml", "report-0000", giztest.StepRequest{Step: giztest.Step{ID: "turn"}})
				capture.observe("peer", "received", &genx.MessageChunk{Part: genx.Text("hello")}, time.Now())
			} else {
				output = filepath.Join(root, "missing", "report.json")
			}
			report := giztest.Report{Status: "passed", StartedAt: time.Now(), Tasks: []giztest.TaskReport{{Path: "report.yaml", Name: "report", TaskID: "report-0000", Status: "passed"}}}
			if err := writeRunReports(output, record, report); err == nil {
				t.Fatal("export error was swallowed")
			}
			if brokenReplay {
				if _, err := os.Stat(output); err != nil {
					t.Fatal("independent report lost after replay export failure:", err)
				}
			}
			if _, err := os.Stat(filepath.Join(record.directory.target, "index.html")); err != nil {
				t.Fatal("replay lost after report export failure:", err)
			}
		})
	}
}

func TestReplayEachTestKeepsItsOwnPlayableAudio(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native Opus runtime unavailable")
	}
	directory := filepath.Join(t.TempDir(), "out")
	record, err := newReplayRecord(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer record.directory.abort()
	started := time.Now()
	report := giztest.Report{Status: "passed", StartedAt: started, DurationMS: 20000}
	for index, name := range []string{"../same/name", "../same/name"} {
		path := "test.giztest.yaml"
		taskID := fmt.Sprintf("test-%04d", index)
		report.Tasks = append(report.Tasks, giztest.TaskReport{Path: path, Name: name, TaskID: taskID, Status: "passed", RepeatIndex: index, DurationMS: 1000})
		capture := record.capture(path, taskID, giztest.StepRequest{Step: giztest.Step{ID: "turn"}})
		capture.observe("peer", "received", &genx.MessageChunk{Part: genx.Text(fmt.Sprintf("reply %d", index)), Ctrl: &genx.StreamCtrl{StreamID: "same-stream-id", Label: "assistant"}}, started.Add(time.Duration(index)*10*time.Second))
		for _, packet := range voiceTonePacketsCount(t, 440+index*220, 20) {
			capture.observe("peer", "received", &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: "same-stream-id", Label: "assistant"}}, started.Add(time.Duration(index)*10*time.Second))
		}
	}
	if err := record.commit(report); err != nil {
		t.Fatal(err)
	}
	if len(record.pages) != 1 || record.pages[0] != "index.html" {
		t.Fatalf("pages=%v", record.pages)
	}
	replay := readReplay(t, directory)
	if len(replay.Report.Tasks) != 2 {
		t.Fatal("recording lost repeated executions")
	}
	var clips [][]byte
	for _, event := range replay.Events {
		if event.Audio == "" {
			continue
		}
		wav, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(event.Audio)))
		if err != nil || len(wav) <= 44 || string(wav[:4]) != "RIFF" {
			t.Fatal("invalid recorded WAV", err)
		}
		if !strings.HasPrefix(event.AudioData, "data:audio/wav;base64,") {
			t.Fatal("audio is not embedded in HTML data")
		}
		clips = append(clips, wav)
	}

	if len(clips) != 2 || bytes.Equal(clips[0], clips[1]) {
		t.Fatal("independent tests reused another test's audio")
	}
}

func TestRecordingHTMLIsStandaloneAfterCopy(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native Opus runtime unavailable")
	}
	directory := filepath.Join(t.TempDir(), "output")
	record, err := newReplayRecord(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer record.directory.abort()
	started := time.Now()
	step := giztest.Step{ID: "voice", Client: "peer"}
	capture := record.capture("only.giztest.yaml", "only-0000", giztest.StepRequest{Step: step})
	capture.observe("peer", "received", &genx.MessageChunk{Part: genx.Text("reply </script><script>window.injected=true</script>")}, started)
	for _, packet := range voiceTonePacketsCount(t, 440, 10) {
		capture.observe("peer", "received", &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}}, started)
	}
	report := giztest.Report{StartedAt: started, Status: "passed", DurationMS: 200, Tasks: []giztest.TaskReport{{Path: "only.giztest.yaml", Name: "<script>title</script>", TaskID: "only-0000", Status: "passed", Steps: []giztest.StepReport{{ID: "voice", Operation: "peer_stream", Client: "peer", Status: "passed"}}}}}
	if err := record.commit(report); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(directory, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	// Distribute just the rendered HTML to a directory with no program,
	// source template, JSON file, or media file.
	distributed := filepath.Join(t.TempDir(), "shared.html")
	if err := os.WriteFile(distributed, html, 0o600); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(html, []byte("<a ")) || bytes.Contains(html, []byte("href=")) || bytes.Contains(html, []byte("__GIZTEST_DATA__")) || bytes.Contains(html, []byte("{{ .")) {
		t.Fatal("rendered HTML contains links or unrendered template data")
	}
	match := regexp.MustCompile(`<script id="recording-data" type="application/json">([\s\S]*?)</script>`).FindSubmatch(html)
	if len(match) != 2 {
		t.Fatal("recording data was not rendered into HTML")
	}
	var embedded replayData
	if err := json.Unmarshal(match[1], &embedded); err != nil {
		t.Fatalf("Go template emitted invalid JSON: %v", err)
	}
	if len(embedded.Report.Tasks[0].Steps) != 1 {
		t.Fatal("HTML did not contain the test's steps")
	}
	clips := 0
	for _, event := range embedded.Events {
		if event.AudioData == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(event.AudioData, "data:audio/wav;base64,"))
		if err != nil || len(raw) <= 44 || string(raw[:4]) != "RIFF" {
			t.Fatal("HTML does not contain independently playable audio", err)
		}
		clips++
	}
	if clips != 1 {
		t.Fatalf("embedded clips=%d", clips)
	}
	if _, err := os.Stat(filepath.Join(directory, "audio.ogg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("generated a merged full recording")
	}
}

func TestRecordingOutputRequiresOneGiztestFile(t *testing.T) {
	file := writeTestDocument(t, validDocument)
	for _, args := range [][]string{{filepath.Dir(file)}, {file, file}} {
		command := NewCmd()
		command.SetOut(io.Discard)
		command.SetErr(io.Discard)
		command.SetArgs(append(append([]string{"run"}, args...), "--output", filepath.Join(t.TempDir(), "output")))
		err := command.Execute()
		if coded, ok := err.(interface{ ExitCode() int }); !ok || coded.ExitCode() != exitValidation {
			t.Fatalf("recording accepted non-single-file input: %v", err)
		}
	}
}
