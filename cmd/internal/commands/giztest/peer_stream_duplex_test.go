package giztestcmd

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

type duplexReadProbe struct {
	*fakeRelayStream
	nextStarted chan struct{}
	nextOnce    sync.Once
}

func (s *duplexReadProbe) Next() (*genx.MessageChunk, error) {
	s.nextOnce.Do(func() { close(s.nextStarted) })
	return s.fakeRelayStream.Next()
}

func (s *duplexReadProbe) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	// A full-duplex provider may start its response before input is complete.
	// Require the reader to be running before letting the input proceed.
	select {
	case <-s.nextStarted:
	case <-ctx.Done():
		return context.Cause(ctx)
	}
	return s.fakeRelayStream.Push(ctx, chunk)
}

func TestPeerStreamRealtimeReadsDuringInput(t *testing.T) {
	s := &duplexReadProbe{fakeRelayStream: newFakeRelayStream(), nextStarted: make(chan struct{})}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	go func() {
		for {
			select {
			case <-s.pushes:
			case <-s.closed:
				return
			}
		}
	}()
	s.in <- assistantText("reply", "hello", false)
	s.in <- assistantBlob("reply", []byte{0xf8}, false)
	s.in <- assistantText("reply", "", true)
	s.in <- assistantBlob("reply", nil, true)
	_, err := invokePeerStream(ctx, nil, func() (peerStream, error) { return s, nil }, giztest.Step{
		ID: "duplex", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "realtime"},
	}, []byte{0xf8}, 0)
	if err != nil {
		t.Fatalf("realtime input waited for output reader: %v", err)
	}
}

type duplexOutputProbe struct {
	*fakeRelayStream
	observed <-chan struct{}
}

func (s *duplexOutputProbe) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	select {
	case <-s.observed:
	case <-ctx.Done():
		return context.Cause(ctx)
	}
	return s.fakeRelayStream.Push(ctx, chunk)
}

func TestPeerStreamRealtimeProcessesOutputBeyondReadQueueDuringInput(t *testing.T) {
	observed := make(chan struct{})
	s := &duplexOutputProbe{fakeRelayStream: newFakeRelayStream(), observed: observed}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	const packets = 130 // Exceeds both the fake transport and reader queues.
	go func() {
		defer s.Close()
		for {
			select {
			case <-s.pushes:
			case <-ctx.Done():
				return
			case <-s.closed:
				return
			}
		}
	}()
	go func() {
		chunks := []*genx.MessageChunk{assistantText("reply", "hello", false)}
		for range packets {
			chunks = append(chunks, assistantBlob("reply", []byte{0xf8}, false))
		}
		chunks = append(chunks, assistantText("reply", "", true), assistantBlob("reply", nil, true))
		for _, chunk := range chunks {
			select {
			case s.in <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()
	count := 0
	_, err := invokePeerStream(ctx, nil, func() (peerStream, error) { return s, nil }, giztest.Step{
		ID: "duplex", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "realtime"},
	}, []byte{0xf8}, 0, func(_ string, role string, _ []byte, end bool) error {
		if role == "assistant" && !end {
			count++
			if count == packets {
				close(observed)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("output processing waited for input completion: %v", err)
	}
	if count != packets {
		t.Fatalf("observed packets = %d, want %d", count, packets)
	}
}

func TestPeerStreamInputSentDrainsBeyondReaderQueue(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(map[bool]string{false: "step", true: "retained"}[keep], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			observed := make(chan struct{})
			stream := &duplexOutputProbe{fakeRelayStream: newFakeRelayStream(), observed: observed}
			defer stream.Close()
			const packets = 300
			go func() {
				for range packets {
					select {
					case stream.in <- assistantText("reply", "chunk", false):
					case <-ctx.Done():
						return
					}
				}
				close(observed)
			}()
			go func() {
				for {
					select {
					case <-stream.pushes:
					case <-ctx.Done():
						return
					case <-stream.closed:
						return
					}
				}
			}()
			var session *peerStreamSession
			if keep {
				session = newPeerStreamSession("peer", stream)
				defer session.Close()
			}
			result, err := invokePeerStreamOnStream(ctx, nil, func() (peerStream, error) { return stream, nil }, stream, session, "", giztest.Step{ID: "send", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "realtime", Completion: "input_sent", Pacing: "1ms"}}, []byte{0xf8}, 0, nil)
			if err != nil {
				t.Fatal(err)
			}
			if result.evidence["input_sent"] != true {
				t.Fatalf("input not completed: %#v", result.evidence)
			}
			if keep {
				for range packets {
					select {
					case item := <-session.next:
						if item.err != nil || item.chunk == nil {
							t.Fatalf("lost retained output: %#v", item)
						}
					case <-ctx.Done():
						t.Fatal("retained output was consumed by input_sent")
					}
				}
			} else if result.evidence["events"].(int) < packets-128 {
				t.Fatalf("output was not drained: %#v", result.evidence)
			}
		})
	}
}
