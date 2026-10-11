// Package peergenx supplies providers only to the opt-in local build overlay.
package peergenx

import (
	"context"
	"errors"
	"io"
	"math"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type runStateFixtureBuilder struct{}

func (runStateFixtureBuilder) BuildGenerator(context.Context, GeneratorConfig) (genx.Generator, error) {
	return nil, errors.New("run-state fixture uses a deterministic Eino graph")
}

func (runStateFixtureBuilder) BuildTransformer(_ context.Context, cfg TransformerConfig) (genx.Transformer, error) {
	return runStateTransformer{asr: cfg.Model != nil && cfg.Model.Kind == apitypes.ModelKindAsr}, nil
}

type runStateTransformer struct{ asr bool }

func (t runStateTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	out := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 32)
	stop := context.AfterFunc(ctx, func() { _ = input.Close() })
	go func() {
		defer stop()
		defer input.Close()
		defer out.Done(genx.Usage{})
		seen := false
		for {
			chunk, err := input.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = out.Stream().CloseWithError(err)
				return
			}
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
				seen = true
			}
			if t.asr && seen && chunk.IsEndOfStream() {
				if err := out.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("local run state reply"), Ctrl: &genx.StreamCtrl{StreamID: chunk.Ctrl.StreamID, BeginOfStream: true, EndOfStream: true}}); err != nil {
					return
				}
				seen = false
			}
		}
		if t.asr {
			return
		}
		encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
		if err != nil {
			_ = out.Stream().CloseWithError(err)
			return
		}
		defer encoder.Close()
		id := genx.NewStreamID()
		for frame := range 20 {
			timer := time.NewTimer(20 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			pcm := make([]int16, 320)
			for i := range pcm {
				pcm[i] = int16(12000 * math.Sin(2*math.Pi*440*float64(frame*320+i)/16000))
			}
			packet, err := encoder.Encode(pcm, len(pcm))
			if err != nil {
				_ = out.Stream().CloseWithError(err)
				return
			}
			if err := out.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: frame == 0}}); err != nil {
				return
			}
		}
		_ = out.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}})
	}()
	return out.Stream(), nil
}
