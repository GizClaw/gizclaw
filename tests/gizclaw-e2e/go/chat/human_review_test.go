//go:build gizclaw_e2e

package chat

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestHumanReview(t *testing.T) {
	runLiveWorkspaceCase(t, workspaceCaseHumanReview, allWorkspaceConfigPaths(t))
}

type fakeOpusPacketDecoder struct {
	samples []int16
	err     error
	closed  bool
}

func (d *fakeOpusPacketDecoder) Decode([]byte, int, bool) ([]int16, error) {
	if d.err != nil {
		return nil, d.err
	}
	return append([]int16(nil), d.samples...), nil
}

func (d *fakeOpusPacketDecoder) Close() error {
	d.closed = true
	return nil
}

type fakePlaybackStream struct {
	writes [][]byte
	errs   []error
	closed bool
}

func (s *fakePlaybackStream) Write(data []byte) (int, error) {
	if len(s.errs) > 0 {
		err := s.errs[0]
		s.errs = s.errs[1:]
		return 0, err
	}
	s.writes = append(s.writes, append([]byte(nil), data...))
	return len(data), nil
}

func (s *fakePlaybackStream) Close() error {
	s.closed = true
	return nil
}

func newTestHumanReviewPlayback(stream *fakePlaybackStream) (*humanReviewPlayback, *fakeOpusPacketDecoder, *fakeOpusPacketDecoder) {
	inputDec := &fakeOpusPacketDecoder{samples: []int16{1, -2}}
	outputDec := &fakeOpusPacketDecoder{samples: []int16{3, -4}}
	return &humanReviewPlayback{
		format:     pcm.L16Mono48K,
		channels:   2,
		frameSize:  960,
		stream:     stream,
		inputDec:   inputDec,
		outputDec:  outputDec,
		playBuffer: buffer.RingN[humanReviewPCMFrame](4),
		playDone:   make(chan error, 1),
	}, inputDec, outputDec
}

func TestHumanReviewPCMConversions(t *testing.T) {
	mono := int16SamplesToPCM16LE([]int16{1, -2})
	if len(mono) != 4 || mono[0] != 1 || mono[1] != 0 || mono[2] != 254 || mono[3] != 255 {
		t.Fatalf("mono pcm = %v", mono)
	}
	stereo := monoPCM16LEToStereo(mono)
	want := []byte{1, 0, 1, 0, 254, 255, 254, 255}
	if string(stereo) != string(want) {
		t.Fatalf("stereo pcm = %v, want %v", stereo, want)
	}
}

func TestHumanReviewTapPacketQueuesStereoFrame(t *testing.T) {
	playback, _, _ := newTestHumanReviewPlayback(&fakePlaybackStream{})
	if err := playback.TapInputPacket(context.Background(), "input", []byte{1}); err != nil {
		t.Fatalf("TapInputPacket() error = %v", err)
	}
	frame, ok, err := playback.nextPlaybackFrame()
	if err != nil || !ok {
		t.Fatalf("nextPlaybackFrame() = (%+v, %t, %v)", frame, ok, err)
	}
	if frame.label != "input" || len(frame.data) != 8 {
		t.Fatalf("frame = %+v", frame)
	}
	if err := playback.TapOutputPacket(context.Background(), "output", nil); err != nil {
		t.Fatalf("TapOutputPacket(nil) error = %v", err)
	}
	if playback.playBuffer.Len() != 0 {
		t.Fatalf("empty packet queued frames = %d", playback.playBuffer.Len())
	}
}

func TestHumanReviewTapPacketReportsDecodeAndContextErrors(t *testing.T) {
	playback, inputDec, _ := newTestHumanReviewPlayback(&fakePlaybackStream{})
	inputDec.err = errors.New("decode failed")
	if err := playback.TapInputPacket(context.Background(), "input", []byte{1}); err == nil || !strings.Contains(err.Error(), "decode failed") {
		t.Fatalf("decode error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inputDec.err = nil
	if err := playback.TapInputPacket(ctx, "input", []byte{1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("context error = %v", err)
	}
}

func TestHumanReviewWritePCMHandlesUnderflowAndShortWrite(t *testing.T) {
	stream := &fakePlaybackStream{errs: []error{
		errors.New("PortAudio: Output underflowed"),
		errors.New("PortAudio: Output underflowed"),
		errors.New("PortAudio: Output underflowed"),
		errors.New("PortAudio: Output underflowed"),
	}}
	playback, _, _ := newTestHumanReviewPlayback(stream)
	if err := playback.writePCM(context.Background(), "underflow", []byte{1, 2}); err != nil {
		t.Fatalf("writePCM underflow error = %v", err)
	}
	if playback.underflowWarnings != 4 {
		t.Fatalf("underflow warnings = %d", playback.underflowWarnings)
	}

	short := &shortPlaybackStream{}
	playback, _, _ = newTestHumanReviewPlayback(nil)
	playback.stream = short
	if err := playback.writePCM(context.Background(), "short", []byte{1, 2}); err == nil || !strings.Contains(err.Error(), "short write") {
		t.Fatalf("short write error = %v", err)
	}
}

type shortPlaybackStream struct{}

func (shortPlaybackStream) Write(data []byte) (int, error) { return len(data) - 1, nil }
func (shortPlaybackStream) Close() error                   { return nil }

func TestHumanReviewPlayBeepAndDrainPlayback(t *testing.T) {
	stream := &fakePlaybackStream{}
	playback, _, _ := newTestHumanReviewPlayback(stream)
	if err := playback.PlayBeep(context.Background()); err != nil {
		t.Fatalf("PlayBeep() error = %v", err)
	}
	if len(stream.writes) != 1 || len(stream.writes[0]) == 0 {
		t.Fatalf("beep writes = %d", len(stream.writes))
	}

	stream.writes = nil
	if err := playback.playBuffer.Add(humanReviewPCMFrame{label: "first", data: []byte{1, 2}}); err != nil {
		t.Fatalf("Add(first) error = %v", err)
	}
	if err := playback.playBuffer.Add(humanReviewPCMFrame{label: "second", data: []byte{3, 4}}); err != nil {
		t.Fatalf("Add(second) error = %v", err)
	}
	playback.markPlaybackClosed()
	if err := playback.drainPlayback(); err != nil {
		t.Fatalf("drainPlayback() error = %v", err)
	}
	if playback.playFrames != 2 || len(stream.writes) != 2 {
		t.Fatalf("drain frames/writes = %d/%d", playback.playFrames, len(stream.writes))
	}
}

func TestHumanReviewPlaybackBurstFlushesPendingWhenClosed(t *testing.T) {
	stream := &fakePlaybackStream{}
	playback, _, _ := newTestHumanReviewPlayback(stream)
	playback.markPlaybackClosed()
	if err := playback.playbackBurst([]humanReviewPCMFrame{
		{label: "first", data: []byte{1, 2}},
		{label: "second", data: []byte{3, 4}},
	}); err != nil {
		t.Fatalf("playbackBurst() error = %v", err)
	}
	if playback.playFrames != 2 || len(stream.writes) != 2 {
		t.Fatalf("burst frames/writes = %d/%d", playback.playFrames, len(stream.writes))
	}
}

func TestHumanReviewPlaybackFrameStateAndClose(t *testing.T) {
	stream := &fakePlaybackStream{}
	playback, inputDec, outputDec := newTestHumanReviewPlayback(stream)
	playback.playDone <- nil

	if _, ok, closed, err := playback.tryNextPlaybackFrame(); ok || closed || err != nil {
		t.Fatalf("empty tryNextPlaybackFrame() = %t/%t/%v", ok, closed, err)
	}
	playback.markPlaybackClosed()
	if _, ok, closed, err := playback.tryNextPlaybackFrame(); ok || !closed || err != nil {
		t.Fatalf("closed tryNextPlaybackFrame() = %t/%t/%v", ok, closed, err)
	}
	if frame, ok, closed, err := playback.waitPlaybackContinuation("idle", 0); frame.data != nil || ok || closed || err != nil {
		t.Fatalf("zero waitPlaybackContinuation() = %+v/%t/%t/%v", frame, ok, closed, err)
	}
	if err := playback.flushPlaybackPending([]humanReviewPCMFrame{{label: "pending", data: []byte{1, 2}}}); err != nil {
		t.Fatalf("flushPlaybackPending() error = %v", err)
	}
	if err := playback.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := playback.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if !inputDec.closed || !outputDec.closed || !stream.closed {
		t.Fatalf("closed input/output/stream = %t/%t/%t", inputDec.closed, outputDec.closed, stream.closed)
	}
}

func TestHumanReviewNextPlaybackFrameClosedPipe(t *testing.T) {
	playback, _, _ := newTestHumanReviewPlayback(&fakePlaybackStream{})
	if err := playback.playBuffer.CloseWithError(io.ErrClosedPipe); err != nil {
		t.Fatalf("CloseWithError() error = %v", err)
	}
	if _, ok, err := playback.nextPlaybackFrame(); ok || err != nil {
		t.Fatalf("nextPlaybackFrame closed pipe = %t/%v", ok, err)
	}
}

func TestRunHumanReviewRejectsNilDriver(t *testing.T) {
	if _, err := runHumanReview(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "persona driver is nil") {
		t.Fatalf("runHumanReview(nil) error = %v", err)
	}
}

func TestRunHumanReviewRejectsMissingTransportFactory(t *testing.T) {
	if _, err := runHumanReview(context.Background(), &personaDriver{}); err == nil || !strings.Contains(err.Error(), "transport factory") {
		t.Fatalf("runHumanReview(no factory) error = %v", err)
	}
}

func TestDetachHumanReviewPlaybackOnlyClearsMatchingTap(t *testing.T) {
	playback, _, _ := newTestHumanReviewPlayback(&fakePlaybackStream{})
	other, _, _ := newTestHumanReviewPlayback(&fakePlaybackStream{})
	transport := &chatTransport{audioTap: playback}

	detachHumanReviewPlayback(transport, other)
	if transport.audioTap != playback {
		t.Fatalf("non-matching playback detached")
	}

	detachHumanReviewPlayback(transport, playback)
	if transport.audioTap != nil {
		t.Fatalf("matching playback not detached")
	}
}

func TestHumanReviewHistoryReplayTarget(t *testing.T) {
	tests := []struct {
		rounds int
		max    int
		want   int
	}{
		{rounds: 3, max: 2, want: 2},
		{rounds: 1, max: 2, want: 1},
		{rounds: 0, max: 2, want: 2},
		{rounds: 3, max: 0, want: 1},
	}
	for _, tt := range tests {
		if got := humanReviewHistoryReplayTarget(tt.rounds, tt.max); got != tt.want {
			t.Fatalf("humanReviewHistoryReplayTarget(%d, %d) = %d, want %d", tt.rounds, tt.max, got, tt.want)
		}
	}
}

func TestHumanReviewReplayItemsSelectsOldestReplayableAgentEntries(t *testing.T) {
	base := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	items := []rpcapi.PeerRunHistoryEntry{
		{Id: "newer", CreatedAt: base.Add(2 * time.Second), Text: "新回复", ReplayAvailable: true, Type: rpcapi.PeerRunHistoryEntryTypeAgent},
		{Id: "gear", CreatedAt: base.Add(-time.Second), Text: "用户输入", ReplayAvailable: true, Type: rpcapi.PeerRunHistoryEntryTypeGear},
		{Id: "empty", CreatedAt: base, Text: "  ", ReplayAvailable: true, Type: rpcapi.PeerRunHistoryEntryTypeAgent},
		{Id: "oldest", CreatedAt: base.Add(time.Second), Text: "旧回复", ReplayAvailable: true, Type: rpcapi.PeerRunHistoryEntryTypeAgent},
		{Id: "disabled", CreatedAt: base.Add(3 * time.Second), Text: "不可播放", ReplayAvailable: false, Type: rpcapi.PeerRunHistoryEntryTypeAgent},
	}
	got := humanReviewReplayItems(items, 2)
	if len(got) != 2 {
		t.Fatalf("len(humanReviewReplayItems) = %d, want 2", len(got))
	}
	if got[0].Id != "oldest" || got[1].Id != "newer" {
		t.Fatalf("humanReviewReplayItems ids = %q, %q; want oldest, newer", got[0].Id, got[1].Id)
	}
}
