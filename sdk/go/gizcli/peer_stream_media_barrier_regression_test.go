package gizcli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestPeerStreamQuickBOSAfterEOSPreservesFirstPCM(t *testing.T) {
	audio := make(chan *genx.Blob, 1)
	s := &PeerStream{audio: audio, pcmAudio: true, out: make(chan *genx.MessageChunk, 4), done: make(chan struct{})}
	defer s.Close()
	begin := func(id string) *genx.MessageChunk {
		return &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: id, Label: "assistant", BeginOfStream: true}}
	}
	first := &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: []byte{1, 0}, OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}}
	var flushes atomic.Int32
	var published atomic.Bool
	s.flushAudio = func(ctx context.Context) error {
		if flushes.Add(1) > 1 {
			// The first B packet is available while a mistaken EOS-style
			// drain runs for BOS(B). It belongs to B, not the completed A.
			select {
			case audio <- first:
				published.Store(true)
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	if err := s.pushMergedEvent(begin("a")); err != nil {
		t.Fatal(err)
	}
	<-s.out
	end := begin("a")
	end.Ctrl.BeginOfStream, end.Ctrl.EndOfStream = false, true
	if err := s.pushMergedEvent(end); err != nil {
		t.Fatal(err)
	}
	<-s.out
	if err := s.pushMergedEvent(begin("b")); err != nil {
		t.Fatal(err)
	}
	if !published.Load() {
		if err := s.pushMergedAudio(first); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.Close()
	bos, err := s.Next()
	if err != nil || !bos.IsBeginOfStream() || bos.Ctrl.StreamID != "b" {
		t.Fatalf("replacement BOS = %+v, %v", bos, err)
	}
	chunk, err := s.Next()
	if err != nil {
		t.Fatalf("first B packet was consumed as post-EOS tail: %v", err)
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || !bytes.Equal(blob.Data, first.Data) || chunk.Ctrl.StreamID != "b" || chunk.Ctrl.Label != "assistant" || chunk.IsEndOfStream() {
		t.Fatalf("first packet did not belong to B: %+v", chunk)
	}
	if flushes.Load() != 1 || s.IgnoredPostEOSAudioPackets() != 0 {
		t.Fatalf("new BOS drained future media: flushes=%d ignored=%d", flushes.Load(), s.IgnoredPostEOSAudioPackets())
	}
}

func TestPeerStreamSlowNextPreservesSuccessfulFlush(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := &Client{}
		audio, unsubscribe := client.subscribePeerAudio(1)
		s := &PeerStream{audio: audio, unsubscribe: unsubscribe, pcmAudio: true, eventResults: make(chan peerStreamEventResult), out: make(chan *genx.MessageChunk, 1), done: make(chan struct{})}
		defer s.Close()
		frames := []*genx.Blob{
			{MIMEType: "audio/pcm;rate=16000;channels=1", Data: []byte{1, 0}, OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}},
			{MIMEType: "audio/pcm;rate=16000;channels=1", Data: []byte{2, 0}, ConcealedAudio: true},
		}
		flushed := false
		s.flushAudio = func(context.Context) error {
			for _, frame := range frames {
				client.dispatchPeerAudio(frame)
			}
			flushed = true
			return nil
		}
		go s.mergeOutput()
		bos := &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "answer", Label: "assistant", BeginOfStream: true}}
		s.eventResults <- peerStreamEventResult{chunk: bos}
		eos := bos.Clone()
		eos.Ctrl.BeginOfStream, eos.Ctrl.EndOfStream = false, true
		s.eventResults <- peerStreamEventResult{chunk: eos}
		synctest.Wait()
		if !flushed {
			t.Fatal("receiver did not finish dispatch before slow Next")
		}
		// The receiver has already succeeded. Hold its output behind BOS for
		// longer than the receiver's timeout before starting to consume it.
		time.Sleep(3 * time.Second)
		synctest.Wait()
		for i := range 4 {
			chunk, err := s.Next()
			if err != nil {
				t.Fatalf("slow Next(%d) lost completed flush: %v", i, err)
			}
			if chunk.Ctrl.StreamID != "answer" || chunk.Ctrl.Label != "assistant" {
				t.Fatalf("slow Next(%d) changed route: %+v", i, chunk)
			}
			switch i {
			case 0:
				if !chunk.IsBeginOfStream() {
					t.Fatal("BOS missing before flushed audio")
				}
			case 3:
				if !chunk.IsEndOfStream() {
					t.Fatal("EOS missing after flushed audio")
				}
			default:
				blob, ok := chunk.Part.(*genx.Blob)
				if !ok || chunk.IsEndOfStream() || !bytes.Equal(blob.Data, frames[i-1].Data) || blob.ConcealedAudio != frames[i-1].ConcealedAudio {
					t.Fatalf("slow Next(%d) lost recovered frame: %+v", i, chunk)
				}
				if blob.ConcealedAudio && blob.OriginalAudio != nil {
					t.Fatal("concealed frame acquired encoded provenance")
				}
			}
		}
		synctest.Wait()
		select {
		case <-s.done:
			t.Fatalf("completed flush closed stream after slow delivery: %v", s.closeErr())
		default:
		}
	})
}

func TestPeerStreamFlushAudioOutputPreservesFailureAndClose(t *testing.T) {
	for _, mode := range []string{"receiver-error", "receiver-timeout", "close"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := &PeerStream{done: make(chan struct{})}
				defer s.Close()
				want := errors.New("receiver flush failed")
				s.flushAudio = func(ctx context.Context) error {
					if mode == "receiver-error" {
						return want
					}
					<-ctx.Done()
					return ctx.Err()
				}
				finished := make(chan error, 1)
				go func() { finished <- s.flushAudioOutput() }()
				synctest.Wait()
				switch mode {
				case "receiver-timeout":
					want = context.DeadlineExceeded
					time.Sleep(2 * time.Second)
				case "close":
					want = io.EOF
					_ = s.Close()
				}
				if err := <-finished; !errors.Is(err, want) {
					t.Fatalf("flush returned %v, want %v", err, want)
				}
			})
		})
	}
}

func TestPeerStreamBufferOneRetainsTwoFrameFlush(t *testing.T) {
	for _, mode := range []string{"blocking-one-slot", "real-subscriber"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			s := &PeerStream{pcmAudio: true, out: make(chan *genx.MessageChunk, 1), done: make(chan struct{})}
			defer s.Close()
			frames := []*genx.Blob{
				{MIMEType: "audio/pcm;rate=16000;channels=1", Data: []byte{1, 0}, OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}},
				{MIMEType: "audio/pcm;rate=16000;channels=1", Data: []byte{2, 0}, ConcealedAudio: true},
			}
			if mode == "blocking-one-slot" {
				audio := make(chan *genx.Blob, 1)
				s.audio = audio
				s.flushAudio = func(flushCtx context.Context) error {
					// The second send cannot finish until the EOS waiter consumes
					// audio concurrently. A larger internal queue cannot hide this.
					for _, frame := range frames {
						select {
						case audio <- frame:
						case <-flushCtx.Done():
							return flushCtx.Err()
						case <-s.done:
							return s.closeErr()
						}
					}
					return nil
				}
			} else {
				client := &Client{}
				audio, unsubscribe := client.subscribePeerAudio(1)
				s.audio, s.unsubscribe = audio, unsubscribe
				s.flushAudio = func(context.Context) error {
					for _, frame := range frames {
						client.dispatchPeerAudio(frame)
					}
					return nil
				}
			}
			bos := &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "answer", Label: "assistant", BeginOfStream: true}}
			if err := s.pushMergedEvent(bos); err != nil {
				t.Fatal(err)
			}
			<-s.out
			eos := bos.Clone()
			eos.Ctrl.BeginOfStream, eos.Ctrl.EndOfStream = false, true
			finished := make(chan error, 1)
			go func() { finished <- s.pushMergedEvent(eos) }()
			for i := range 3 {
				select {
				case chunk := <-s.out:
					if i == 2 {
						if !chunk.IsEndOfStream() || chunk.Ctrl.StreamID != "answer" {
							t.Fatalf("EOS did not follow both frames: %+v", chunk)
						}
						continue
					}
					blob, ok := chunk.Part.(*genx.Blob)
					if !ok || chunk.IsEndOfStream() || chunk.Ctrl.StreamID != "answer" || chunk.Ctrl.Label != "assistant" || !bytes.Equal(blob.Data, frames[i].Data) || blob.ConcealedAudio != frames[i].ConcealedAudio {
						t.Fatalf("flush frame %d was dropped or overtaken: %+v", i, chunk)
					}
					if i == 1 && blob.OriginalAudio != nil {
						t.Fatal("concealed PCM acquired fake original audio")
					}
				case <-ctx.Done():
					t.Fatal("buffer-one EOS flush stalled before delivering both frames")
				}
			}
			select {
			case err := <-finished:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("EOS flush did not finish after both frames were consumed")
			}
		})
	}
}
