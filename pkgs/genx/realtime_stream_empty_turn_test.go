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
