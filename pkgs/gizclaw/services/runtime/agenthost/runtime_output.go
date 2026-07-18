package agenthost

import (
	"context"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type AudioTrackCreator interface {
	CreateAudioTrack(...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error)
}

// OutputObservationStream lets an output producer track when chunks have
// reached their final consumer. MixerOutput defers audio observation until the
// corresponding mixer track has drained.
type OutputObservationStream interface {
	genx.Stream
	DeferOutputObservation()
	ObserveOutput(*genx.MessageChunk)
}

func deferOutputObservation(stream genx.Stream) OutputObservationStream {
	observer, _ := stream.(OutputObservationStream)
	if observer != nil {
		observer.DeferOutputObservation()
	}
	return observer
}

type MixerOutput struct {
	Tracks            AudioTrackCreator
	Observe           func(*genx.MessageChunk) error
	WaitForAudioDrain bool
}

func (o MixerOutput) ConsumeAgentOutput(ctx context.Context, output genx.Stream) (retErr error) {
	if output == nil {
		return fmt.Errorf("agenthost: output stream is required")
	}
	outputObserver := deferOutputObservation(output)
	tracks := newAudioOutputTracks(o.Tracks)
	type outputResult struct {
		chunk *genx.MessageChunk
		err   error
	}
	results := make(chan outputResult)
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()
	go func() {
		for {
			chunk, err := output.Next()
			select {
			case results <- outputResult{chunk: chunk, err: err}:
			case <-readCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	var pendingObserve []*genx.MessageChunk
	observe := func(chunks []*genx.MessageChunk) error {
		for _, chunk := range chunks {
			if o.Observe != nil {
				if err := o.Observe(chunk); err != nil {
					return err
				}
			}
			if outputObserver != nil {
				outputObserver.ObserveOutput(chunk)
			}
		}
		return nil
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, output.CloseWithError(retErr), tracks.closeWithError(retErr))
			return
		}
		retErr = tracks.closeWrite()
	}()
	for {
		var pendingDone <-chan struct{}
		if o.WaitForAudioDrain && len(pendingObserve) > 0 {
			pendingDone = tracks.nextPendingDone()
		}
		var result outputResult
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pendingDone:
			tracks.removeDrainedPending()
			if !tracks.hasPending() {
				if err := observe(pendingObserve); err != nil {
					return err
				}
				pendingObserve = nil
			}
			continue
		case result = <-results:
		}
		chunk, err := result.chunk, result.err
		if err != nil {
			if IsStreamDone(err) {
				if o.WaitForAudioDrain {
					if err := tracks.waitPending(ctx); err != nil {
						return err
					}
				}
				if err := observe(pendingObserve); err != nil {
					return err
				}
				return nil
			}
			return err
		}
		if chunk == nil {
			continue
		}
		if err := tracks.consume(chunk); err != nil {
			return err
		}
		pendingObserve = removeSupersededAudioEOS(pendingObserve, chunk)
		tracks.removeDrainedPending()
		if o.WaitForAudioDrain && (len(pendingObserve) > 0 || tracks.hasPending()) {
			pendingObserve = append(pendingObserve, chunk)
			if !tracks.hasPending() {
				if err := observe(pendingObserve); err != nil {
					return err
				}
				pendingObserve = nil
			}
		} else {
			if err := observe([]*genx.MessageChunk{chunk}); err != nil {
				return err
			}
		}
	}
}

func removeSupersededAudioEOS(pending []*genx.MessageChunk, interrupt *genx.MessageChunk) []*genx.MessageChunk {
	if interrupt == nil || interrupt.Ctrl == nil || !interrupt.IsEndOfStream() || interrupt.Ctrl.Error == "" {
		return pending
	}
	mimeType, ok := interrupt.MIMEType()
	if !ok || !isMixerAudioMIME(mimeType) {
		return pending
	}
	kept := pending[:0]
	for _, chunk := range pending {
		if chunk == nil {
			kept = append(kept, chunk)
			continue
		}
		queuedMIME, queuedMIMEOK := chunk.MIMEType()
		if chunk.Ctrl != nil && chunk.IsEndOfStream() && chunk.Ctrl.Error == "" &&
			chunk.Ctrl.StreamID == interrupt.Ctrl.StreamID && queuedMIMEOK && queuedMIME == mimeType {
			continue
		}
		kept = append(kept, chunk)
	}
	return kept
}
