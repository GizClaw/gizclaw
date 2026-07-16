package genx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRealtimeStreamOrdersChunksByTimestamp(t *testing.T) {
	stream := NewRealtimeStream(WithRealtimeStreamDelay(0))
	if err := stream.Push(context.Background(), testRealtimeEOSChunk(300)); err != nil {
		t.Fatalf("Push(EOS) error = %v", err)
	}
	if err := stream.Push(context.Background(), testRealtimeAudioChunk(100)); err != nil {
		t.Fatalf("Push(100) error = %v", err)
	}
	if err := stream.Push(context.Background(), testRealtimeAudioChunk(200)); err != nil {
		t.Fatalf("Push(200) error = %v", err)
	}

	assertRealtimeNextTimestamp(t, stream, 100, false)
	assertRealtimeNextTimestamp(t, stream, 200, false)
	assertRealtimeNextTimestamp(t, stream, 300, true)
}

func TestRealtimeStreamDropsLateChunkBeforeLastEmittedTimestamp(t *testing.T) {
	stream := NewRealtimeStream(WithRealtimeStreamDelay(0))
	if err := stream.Push(context.Background(), testRealtimeAudioChunk(200)); err != nil {
		t.Fatalf("Push(200) error = %v", err)
	}
	assertRealtimeNextTimestamp(t, stream, 200, false)

	if err := stream.Push(context.Background(), testRealtimeAudioChunk(100)); err != nil {
		t.Fatalf("Push(late) error = %v", err)
	}
	stream.mu.Lock()
	heapLen := stream.heap.Len()
	stream.mu.Unlock()
	if heapLen != 0 {
		t.Fatalf("late chunk stayed in heap, len=%d", heapLen)
	}
}

func TestRealtimeStreamWaitsForEarlierPush(t *testing.T) {
	var now atomic.Int64
	now.Store(1_000)
	stream := NewRealtimeStream(
		WithRealtimeStreamDelay(40*time.Millisecond),
		WithRealtimeStreamNow(func() time.Time { return time.UnixMilli(now.Load()) }),
	)
	if err := stream.Push(context.Background(), testRealtimeAudioChunk(1_100)); err != nil {
		t.Fatalf("Push(1100) error = %v", err)
	}
	done := make(chan *MessageChunk, 1)
	go func() {
		chunk, _ := stream.Next()
		done <- chunk
	}()
	select {
	case chunk := <-done:
		t.Fatalf("Next returned early: %#v", chunk)
	case <-time.After(20 * time.Millisecond):
	}
	now.Store(1_040)
	if err := stream.Push(context.Background(), testRealtimeAudioChunk(1_000)); err != nil {
		t.Fatalf("Push(1000) error = %v", err)
	}
	select {
	case chunk := <-done:
		if chunk.Ctrl == nil || chunk.Ctrl.Timestamp != 1_000 {
			t.Fatalf("chunk ctrl = %#v, want timestamp 1000", chunk.Ctrl)
		}
	case <-time.After(time.Second):
		t.Fatal("Next did not wake after earlier push")
	}
}

func TestRealtimeStreamCloseFlushesThenReturnsError(t *testing.T) {
	stream := NewRealtimeStream(WithRealtimeStreamDelay(time.Hour))
	if err := stream.Push(context.Background(), testRealtimeAudioChunk(100)); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	want := errors.New("closed")
	if err := stream.CloseWithError(want); err != nil {
		t.Fatalf("CloseWithError() error = %v", err)
	}
	assertRealtimeNextTimestamp(t, stream, 100, false)
	if _, err := stream.Next(); !errors.Is(err, want) {
		t.Fatalf("Next after flush error = %v, want %v", err, want)
	}
}

func TestRealtimeStreamEvictsByDuration(t *testing.T) {
	stream := NewRealtimeStream(
		WithRealtimeStreamDelay(0),
		WithRealtimeStreamMaxDuration(100*time.Millisecond),
	)
	for _, ts := range []int64{100, 150, 250} {
		if err := stream.Push(context.Background(), testRealtimeAudioChunk(ts)); err != nil {
			t.Fatalf("Push(%d) error = %v", ts, err)
		}
	}
	assertRealtimeNextTimestamp(t, stream, 150, false)
	assertRealtimeNextTimestamp(t, stream, 250, false)
}

func TestRealtimeStreamUntimestampedEOSFollowsFutureAudio(t *testing.T) {
	now := time.UnixMilli(1_000)
	stream := NewRealtimeStream(
		WithRealtimeStreamDelay(0),
		WithRealtimeStreamNow(func() time.Time { return now }),
	)
	if err := stream.Push(context.Background(), testRealtimeAudioChunk(2_000)); err != nil {
		t.Fatalf("Push(audio) error = %v", err)
	}
	now = time.UnixMilli(2_000)
	assertRealtimeNextTimestamp(t, stream, 2_000, false)

	now = time.UnixMilli(1_000)
	eos := testRealtimeEOSChunk(0)
	eos.Ctrl.Timestamp = 0
	if err := stream.Push(context.Background(), eos); err != nil {
		t.Fatalf("Push(EOS) error = %v", err)
	}
	now = time.UnixMilli(2_001)
	assertRealtimeNextTimestamp(t, stream, 2_001, true)
}

func TestRealtimeStreamNewBOSDropsPendingAudioFromPreviousStream(t *testing.T) {
	stream := NewRealtimeStream(WithRealtimeStreamDelay(0))
	if err := stream.Push(context.Background(), &MessageChunk{
		Part: &Blob{MIMEType: "audio/opus", Data: []byte{0x01}},
		Ctrl: &StreamCtrl{StreamID: "audio-1", Label: "mic", Timestamp: 1_000},
	}); err != nil {
		t.Fatalf("Push(audio-1) error = %v", err)
	}
	if err := stream.Push(context.Background(), &MessageChunk{
		Part: &Blob{MIMEType: "audio/opus"},
		Ctrl: &StreamCtrl{StreamID: "audio-1", Label: "mic", Timestamp: 1_020, EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(audio-1 EOS) error = %v", err)
	}
	if err := stream.Push(context.Background(), &MessageChunk{
		Ctrl: &StreamCtrl{StreamID: "audio-2", Label: "mic", Timestamp: 1_010, BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push(audio-2 BOS) error = %v", err)
	}
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "audio-2" || !chunk.Ctrl.BeginOfStream {
		t.Fatalf("Next() = %#v, want audio-2 BOS", chunk)
	}
	stream.mu.Lock()
	heapLen := stream.heap.Len()
	stream.mu.Unlock()
	if heapLen != 0 {
		t.Fatalf("superseded audio stayed in heap, len=%d", heapLen)
	}
}

func TestRealtimeStreamNewBOSKeepsDifferentRouteAndNonAudio(t *testing.T) {
	stream := NewRealtimeStream(WithRealtimeStreamDelay(0))
	if err := stream.Push(context.Background(), &MessageChunk{
		Part: Text(""),
		Ctrl: &StreamCtrl{StreamID: "audio-1", Label: "mic", Timestamp: 1_000, EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(text EOS) error = %v", err)
	}
	if err := stream.Push(context.Background(), &MessageChunk{
		Part: &Blob{MIMEType: "audio/opus", Data: []byte{0x01}},
		Ctrl: &StreamCtrl{StreamID: "audio-1", Label: "speaker", Timestamp: 1_001},
	}); err != nil {
		t.Fatalf("Push(other label audio) error = %v", err)
	}
	if err := stream.Push(context.Background(), &MessageChunk{
		Ctrl: &StreamCtrl{StreamID: "audio-2", Label: "mic", Timestamp: 1_002, BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push(BOS) error = %v", err)
	}
	assertRealtimeNextTimestamp(t, stream, 1_000, true)
	assertRealtimeNextTimestamp(t, stream, 1_001, false)
	assertRealtimeNextTimestamp(t, stream, 1_002, false)
}

func TestRealtimeAudioChunkClassification(t *testing.T) {
	for _, tc := range []struct {
		name  string
		chunk *MessageChunk
		want  bool
	}{
		{name: "audio data", chunk: &MessageChunk{Part: &Blob{MIMEType: "audio/opus"}}, want: true},
		{name: "parameterized audio eos", chunk: &MessageChunk{Part: &Blob{MIMEType: "audio/ogg; codecs=opus"}, Ctrl: &StreamCtrl{EndOfStream: true}}, want: true},
		{name: "text eos", chunk: NewTextEndOfStream()},
		{name: "non-audio blob eos", chunk: &MessageChunk{Part: &Blob{MIMEType: "application/json"}, Ctrl: &StreamCtrl{EndOfStream: true}}},
		{name: "control eos", chunk: &MessageChunk{Ctrl: &StreamCtrl{EndOfStream: true}}, want: true},
		{name: "control data", chunk: &MessageChunk{Ctrl: &StreamCtrl{BeginOfStream: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRealtimeAudioChunk(tc.chunk); got != tc.want {
				t.Fatalf("isRealtimeAudioChunk() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestRealtimeStreamNewStreamClampsExplicitLateTimestamps(t *testing.T) {
	now := time.UnixMilli(2_000)
	stream := NewRealtimeStream(
		WithRealtimeStreamDelay(0),
		WithRealtimeStreamNow(func() time.Time { return now }),
	)
	if err := stream.Push(context.Background(), &MessageChunk{
		Part: &Blob{MIMEType: "audio/opus", Data: []byte{0x01}},
		Ctrl: &StreamCtrl{StreamID: "audio-1", Label: "mic", Timestamp: 2_000},
	}); err != nil {
		t.Fatalf("Push(audio-1) error = %v", err)
	}
	assertRealtimeNextTimestamp(t, stream, 2_000, false)

	now = time.UnixMilli(1_000)
	if err := stream.Push(context.Background(), &MessageChunk{
		Ctrl: &StreamCtrl{StreamID: "audio-2", Label: "mic", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push(audio-2 BOS) error = %v", err)
	}
	if err := stream.Push(context.Background(), &MessageChunk{
		Part: &Blob{MIMEType: "audio/opus", Data: []byte{0x02}},
		Ctrl: &StreamCtrl{StreamID: "audio-2", Label: "mic", Timestamp: 1_000},
	}); err != nil {
		t.Fatalf("Push(audio-2 late explicit audio) error = %v", err)
	}

	now = time.UnixMilli(2_001)
	assertRealtimeNextTimestamp(t, stream, 2_001, false)
	assertRealtimeNextTimestamp(t, stream, 2_001, false)
}

func TestRealtimeStreamNewBOSDropsFutureAudioFromPreviousStream(t *testing.T) {
	stream := NewRealtimeStream(WithRealtimeStreamDelay(0))
	if err := stream.Push(context.Background(), &MessageChunk{
		Ctrl: &StreamCtrl{StreamID: "audio-2", Label: "mic", Timestamp: 1_000, BeginOfStream: true},
	}); err != nil {
		t.Fatalf("Push(audio-2 BOS) error = %v", err)
	}
	if err := stream.Push(context.Background(), &MessageChunk{
		Part: &Blob{MIMEType: "audio/opus", Data: []byte{0x01}},
		Ctrl: &StreamCtrl{StreamID: "audio-1", Label: "mic", Timestamp: 1_001},
	}); err != nil {
		t.Fatalf("Push(late audio-1 audio) error = %v", err)
	}
	if err := stream.Push(context.Background(), &MessageChunk{
		Part: &Blob{MIMEType: "audio/opus"},
		Ctrl: &StreamCtrl{StreamID: "audio-1", Label: "mic", Timestamp: 1_002, EndOfStream: true},
	}); err != nil {
		t.Fatalf("Push(late audio-1 EOS) error = %v", err)
	}

	assertRealtimeNextTimestamp(t, stream, 1_000, false)
	stream.mu.Lock()
	heapLen := stream.heap.Len()
	stream.mu.Unlock()
	if heapLen != 0 {
		t.Fatalf("superseded future audio stayed in heap, len=%d", heapLen)
	}
}

func testRealtimeAudioChunk(timestamp int64) *MessageChunk {
	return &MessageChunk{
		Part: &Blob{MIMEType: "audio/opus", Data: []byte{0x01}},
		Ctrl: &StreamCtrl{StreamID: "audio", Timestamp: timestamp},
	}
}

func testRealtimeEOSChunk(timestamp int64) *MessageChunk {
	return &MessageChunk{
		Part: &Blob{MIMEType: "audio/opus"},
		Ctrl: &StreamCtrl{StreamID: "audio", Timestamp: timestamp, EndOfStream: true},
	}
}

func assertRealtimeNextTimestamp(t *testing.T, stream *RealtimeStream, want int64, wantEOS bool) {
	t.Helper()
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if chunk.Ctrl == nil || chunk.Ctrl.Timestamp != want || chunk.IsEndOfStream() != wantEOS {
		t.Fatalf("chunk ctrl = %#v, want timestamp=%d eos=%t", chunk.Ctrl, want, wantEOS)
	}
}
