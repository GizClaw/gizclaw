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

type MixerOutput struct {
	Tracks            AudioTrackCreator
	Observe           func(*genx.MessageChunk) error
	WaitForAudioDrain bool
}

func (o MixerOutput) ConsumeAgentOutput(ctx context.Context, output genx.Stream) (retErr error) {
	if output == nil {
		return fmt.Errorf("agenthost: output stream is required")
	}
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
		if o.Observe == nil {
			return nil
		}
		for _, chunk := range chunks {
			if err := o.Observe(chunk); err != nil {
				return err
			}
		}
		return nil
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, tracks.closeWithError(retErr))
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
