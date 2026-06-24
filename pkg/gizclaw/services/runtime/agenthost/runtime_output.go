package agenthost

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

type AudioTrackCreator interface {
	CreateAudioTrack(...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error)
}

type MixerOutput struct {
	Tracks AudioTrackCreator
}

func (o MixerOutput) ConsumeAgentOutput(ctx context.Context, output genx.Stream) error {
	if output == nil {
		return fmt.Errorf("agenthost: output stream is required")
	}
	var track pcm.Track
	var ctrl *pcm.TrackCtrl
	defer func() {
		if ctrl != nil {
			_ = ctrl.Close()
		}
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
		if chunk == nil || chunk.IsEndOfStream() {
			continue
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || !isPCMBlob(blob) || len(blob.Data) == 0 {
			continue
		}
		if track == nil {
			if o.Tracks == nil {
				return fmt.Errorf("agenthost: audio track creator is required")
			}
			track, ctrl, err = o.Tracks.CreateAudioTrack(pcm.WithTrackLabel("agent"))
			if err != nil {
				return err
			}
		}
		if err := track.Write(pcm.L16Mono16K.DataChunk(blob.Data)); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			return fmt.Errorf("agenthost: write audio chunk: %w", err)
		}
	}
}

func isPCMBlob(blob *genx.Blob) bool {
	mimeType := strings.ToLower(strings.TrimSpace(blob.MIMEType))
	return strings.HasPrefix(mimeType, "audio/l16") || mimeType == "audio/pcm" || mimeType == "audio/x-pcm"
}
