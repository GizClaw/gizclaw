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
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, tracks.closeWithError(retErr))
			return
		}
		retErr = tracks.closeWrite()
	}()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk, err := output.Next()
		if err != nil {
			if IsStreamDone(err) {
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
		if o.WaitForAudioDrain {
			if err := tracks.waitPending(ctx); err != nil {
				return err
			}
		}
		if o.Observe != nil {
			if err := o.Observe(chunk); err != nil {
				return err
			}
		}
	}
}
