package genx

import (
	"io"
	"testing"
	"time"
)

func TestRealtimeStreamKeepsQueuedEmptyTurnBeforeNextBOS(t *testing.T) {
	for _, tc := range []struct {
		name     string
		gap      time.Duration
		explicit bool
	}{
		{name: "same_millisecond"},
		{name: "delayed_consumer", gap: time.Second},
		{name: "explicit_timestamps", explicit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.UnixMilli(1000)
			stream := NewRealtimeStream(WithRealtimeStreamDelay(0), WithRealtimeStreamNow(func() time.Time { return now }))
			chunks := []*MessageChunk{
				{Ctrl: &StreamCtrl{StreamID: "empty", Label: "user", BeginOfStream: true}},
				{Ctrl: &StreamCtrl{StreamID: "empty", Label: "user", EndOfStream: true}},
				{Ctrl: &StreamCtrl{StreamID: "next", Label: "user", BeginOfStream: true}},
			}
			for i, chunk := range chunks {
				if i == 2 {
					now = now.Add(tc.gap)
				}
				if tc.explicit {
					chunk.Ctrl.Timestamp = 1000 + int64(i)
				}
				if err := stream.Push(t.Context(), chunk); err != nil {
					t.Fatal(err)
				}
			}
			if err := stream.Close(); err != nil {
				t.Fatal(err)
			}
			for i, want := range chunks {
				got, err := stream.Next()
				if err != nil || got != want {
					t.Fatalf("boundary %d = %#v, %v; want %#v", i, got, err, want)
				}
			}
			if _, err := stream.Next(); err != io.EOF {
				t.Fatalf("unexpected remaining input: %v", err)
			}
		})
	}
}

func TestRealtimeStreamKeepsEmptyEOSAfterBOSRead(t *testing.T) {
	now := time.UnixMilli(1000)
	stream := NewRealtimeStream(WithRealtimeStreamDelay(0), WithRealtimeStreamNow(func() time.Time { return now }))
	defer stream.Close()
	begin := &MessageChunk{Ctrl: &StreamCtrl{StreamID: "empty", Label: "user", BeginOfStream: true}}
	if err := stream.Push(t.Context(), begin); err != nil {
		t.Fatal(err)
	}
	if got, err := stream.Next(); err != nil || got != begin {
		t.Fatalf("begin=%v err=%v", got, err)
	}
	end := &MessageChunk{Ctrl: &StreamCtrl{StreamID: "empty", Label: "user", EndOfStream: true}}
	next := &MessageChunk{Ctrl: &StreamCtrl{StreamID: "next", Label: "user", BeginOfStream: true}}
	for _, chunk := range []*MessageChunk{end, next} {
		if err := stream.Push(t.Context(), chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := stream.Next(); err != nil || got != end {
		t.Fatalf("prefetched begin lost its queued terminal: got=%v err=%v want=%v", got, err, end)
	}
}

func TestRealtimeStreamKeepsMIMEBeginWithQueuedControlEnd(t *testing.T) {
	for _, endLabel := range []string{"user", ""} {
		t.Run("end_label="+endLabel, func(t *testing.T) {
			stream := NewRealtimeStream(WithRealtimeStreamDelay(0))
			defer stream.Close()
			begin := &MessageChunk{Part: &Blob{MIMEType: "audio/opus"}, Ctrl: &StreamCtrl{StreamID: "old", Label: "user", BeginOfStream: true}}
			end := &MessageChunk{Ctrl: &StreamCtrl{StreamID: "old", Label: endLabel, EndOfStream: true}}
			next := &MessageChunk{Ctrl: &StreamCtrl{StreamID: "next", Label: "user", BeginOfStream: true}}
			for _, chunk := range []*MessageChunk{begin, end, next} {
				if err := stream.Push(t.Context(), chunk); err != nil {
					t.Fatal(err)
				}
			}
			if err := stream.Close(); err != nil {
				t.Fatal(err)
			}
			for index, want := range []*MessageChunk{begin, end, next} {
				if got, err := stream.Next(); err != nil || got != want {
					t.Fatalf("boundary %d=%v err=%v want=%v", index, got, err, want)
				}
			}
		})
	}
}
