package giztestcmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

// The first input cannot advance before the reader timestamps old output. A
// current response is then delivered during EOS Push, before that call returns.
type committedReplyStream struct {
	*fakeRelayStream
	oldRead, currentRead chan struct{}
	oldOnce, currentOnce sync.Once
	replyOnce            sync.Once
	packet               []byte
	text, audio          bool
}

func (s *committedReplyStream) observePacketArrival(chunk *genx.MessageChunk, _ time.Time) {
	if chunk == nil || chunk.Ctrl == nil {
		return
	}
	if _, audio := chunk.Part.(*genx.Blob); !audio {
		return
	}
	switch chunk.Ctrl.StreamID {
	case "opening":
		s.oldOnce.Do(func() { close(s.oldRead) })
	case "reply":
		s.currentOnce.Do(func() { close(s.currentRead) })
	}
}

func (s *committedReplyStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	select {
	case <-s.oldRead:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := s.fakeRelayStream.Push(ctx, chunk); err != nil {
		return err
	}
	if !chunk.IsEndOfStream() {
		return nil
	}
	s.replyOnce.Do(func() {
		// Known old IDs remain excluded even when their continuation or error
		// arrives after input commit. Their receipt must not rescue a deadline.
		s.in <- assistantText("opening", "old tail", false)
		s.in <- assistantBlob("opening", s.packet, false)
		oldEOS := assistantText("opening", "", true)
		oldEOS.Ctrl.Error, oldEOS.Ctrl.ErrorCode = "old response failed", "OLD_ROUTE_ERROR"
		s.in <- oldEOS
		if s.text {
			s.in <- assistantText("reply", "current reply", false)
		}
		if s.audio {
			s.in <- assistantBlob("reply", s.packet, false)
		}
	})
	if s.audio {
		select {
		case <-s.currentRead:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func TestPeerStreamFirstResponseRejectsEarlierPTTOutput(t *testing.T) {
	for _, retained := range []bool{false, true} {
		for _, modalities := range []string{"both", "text", "audio", "neither"} {
			t.Run(fmt.Sprintf("retained=%t/%s", retained, modalities), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				s := &committedReplyStream{fakeRelayStream: newFakeRelayStream(), oldRead: make(chan struct{}), currentRead: make(chan struct{}), packet: testAudibleOpus(t), text: modalities == "both" || modalities == "text", audio: modalities == "both" || modalities == "audio"}
				defer s.Close()
				s.in <- assistantText("opening", "old opening", false)
				s.in <- assistantBlob("opening", s.packet, false)
				var session *peerStreamSession
				if retained {
					session = newPeerStreamSession("peer", s)
					defer session.Close()
					session.startReader()
					select {
					case <-s.oldRead:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
				}
				step := giztest.Step{ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "push-to-talk", Completion: "first_response", HoldBeforeAudio: "0ms", Pacing: "0ms", FirstTextTimeout: "50ms", FirstAudioTimeout: "50ms"}}
				audio, _ := testOggOpus(t)
				r, err := invokePeerStreamOnStream(ctx, nil, nil, s, session, "input", step, audio, 0, nil)
				if modalities != "both" {
					want := "first_text_timeout"
					if modalities == "text" {
						want = "first_audio_timeout"
					}
					if !errors.Is(err, context.DeadlineExceeded) || !strings.HasPrefix(fmt.Sprint(r.evidence["deadline"]), "first_") {
						t.Fatalf("old receipt bypassed modality deadline: %v; %v", err, r.evidence)
					}
					if modalities != "neither" && r.evidence["deadline"] != want {
						t.Fatalf("deadline=%v, want %s", r.evidence["deadline"], want)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				obj := r.assertion.(map[string]any)
				if obj["reply"] != "current reply" || obj["audio_bytes"] != len(s.packet) {
					t.Fatalf("earlier output contaminated reply: %#v", obj)
				}
			})
		}
	}
}

type replyAtCommitStream struct {
	*fakeRelayStream
	chunks []*genx.MessageChunk
	once   sync.Once
}

func (s *replyAtCommitStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if err := s.fakeRelayStream.Push(ctx, chunk); err != nil {
		return err
	}
	if chunk.IsEndOfStream() {
		s.once.Do(func() {
			for _, c := range s.chunks {
				s.in <- c
			}
		})
	}
	return nil
}

func TestPeerStreamResponseHistorySurvivesCloseAndReopen(t *testing.T) {
	packet := testAudibleOpus(t)
	sessions := newPeerStreamSessions()
	defer sessions.Close()
	opening := newFakeRelayStream()
	opening.in <- assistantText("opening", "opening", false)
	opening.in <- assistantBlob("opening", packet, false)
	step := giztest.Step{ID: "opening", Client: "peer", PeerStream: &giztest.PeerStreamOperation{Mode: "text", Completion: "first_response", FirstTextTimeout: "1s", FirstAudioTimeout: "1s"}}
	if _, err := invokePeerStreamWithSessions(t.Context(), nil, func() (peerStream, error) { return opening, nil }, sessions, step, "start", 0); err != nil {
		t.Fatal(err)
	}
	for _, client := range []string{"peer", "other"} {
		t.Run(client, func(t *testing.T) {
			s := &replyAtCommitStream{fakeRelayStream: newFakeRelayStream(), chunks: []*genx.MessageChunk{assistantText("opening", "late opening", false), assistantBlob("opening", packet, false), assistantText("current", "current", false), assistantBlob("current", packet, false)}}
			step := giztest.Step{ID: "turn", Client: client, PeerStream: &giztest.PeerStreamOperation{Mode: "push-to-talk", Completion: "first_response", HoldBeforeAudio: "0ms", Pacing: "0ms", FirstTextTimeout: "1s", FirstAudioTimeout: "1s"}}
			audio, _ := testOggOpus(t)
			r, err := invokePeerStreamWithSessions(t.Context(), nil, func() (peerStream, error) { return s, nil }, sessions, step, audio, 0)
			if err != nil {
				t.Fatal(err)
			}
			want := "current"
			if client == "other" {
				want = "late opening"
			}
			if r.assertion.(map[string]any)["reply"] != want {
				t.Fatalf("client %s reply=%v, want %s", client, r.assertion, want)
			}
		})
	}
}

func TestPeerStreamResponseTrackingIsBounded(t *testing.T) {
	history := &peerStreamResponseHistory{}
	for i := range 3 * peerStreamResponseHistoryLimit {
		history.observe(assistantText(fmt.Sprint(i), "text", false), time.Now())
	}
	if len(history.snapshot()) != peerStreamResponseHistoryLimit {
		t.Fatal("response history is not bounded")
	}
	f := &peerStreamResponseFilter{history: history}
	f.begin(history.snapshot())
	for i := range 2*peerStreamResponseHistoryLimit + 1 {
		f.accepts(assistantText(fmt.Sprintf("earlier-%d", i), "text", false), time.Now())
	}
	if f.err() == nil || len(f.earlier) > 2*peerStreamResponseHistoryLimit {
		t.Fatal("response flood did not fail closed within the tracking bound")
	}
}

type rawResponseProbe struct {
	*fakeRelayStream
	recorded chan struct{}
	once     sync.Once
}

func (s *rawResponseProbe) observePacketArrival(*genx.MessageChunk, time.Time) {
	s.once.Do(func() { close(s.recorded) })
}

func TestPeerStreamHistoryPrecedesSlowOutputObservation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	s := &rawResponseProbe{fakeRelayStream: newFakeRelayStream(), recorded: make(chan struct{})}
	defer s.Close()
	history := &peerStreamResponseHistory{}
	release := make(chan struct{})
	defer close(release)
	next := readPeerStreamObserved(ctx, s, func(*genx.MessageChunk, time.Time, peerAudioClass) {
		select {
		case <-release:
		case <-ctx.Done():
		}
	}, history.observe)
	s.in <- assistantText("earlier", "old reply", false)
	select {
	case <-s.recorded:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// The old reader's decoded-output observer is blocked. Another operation
	// still recognizes this route from its raw receipt before its own commit.
	filter := &peerStreamResponseFilter{history: history}
	filter.begin(nil)
	filter.commit(time.Now())
	arrivals := &peerStreamFirstResponseArrivals{history: history, filter: filter, started: time.Now()}
	tail := assistantText("earlier", "late old tail", false)
	arrivals.observe(tail, time.Now(), peerAudioClass{})
	if filter.accepts(tail, time.Now()) || arrivals.firstTextWithin(time.Second) {
		t.Fatal("slow observation hid the earlier raw receipt")
	}
	select {
	case <-next:
		t.Fatal("output observation unexpectedly advanced")
	default:
	}
}
