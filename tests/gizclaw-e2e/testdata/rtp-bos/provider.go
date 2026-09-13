// Package peergenx supplies the Docker-only provider overlay. The runner adds
// this file to peergenx and selects this Builder; release builds never include it.
package peergenx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type latencyFixtureBuilder struct{}

func (latencyFixtureBuilder) BuildGenerator(context.Context, GeneratorConfig) (genx.Generator, error) {
	return nil, errors.New("rtp-bos fixture uses deterministic workflow script nodes")
}
func (latencyFixtureBuilder) BuildTransformer(_ context.Context, cfg TransformerConfig) (genx.Transformer, error) {
	if cfg.Model != nil && cfg.Model.Kind == apitypes.ModelKindAsr {
		return latencyASR{realtime: cfg.Params["emit_interim"] == true || cfg.Params["emit_interim"] == "true"}, nil
	}
	if cfg.Voice == nil {
		return nil, errors.New("rtp-bos fixture expected ASR or voice")
	}
	startup, err := time.ParseDuration(os.Getenv("LATENCY_TTS_STARTUP"))
	if err != nil || startup < 3*time.Second {
		return nil, errors.New("LATENCY_TTS_STARTUP must be at least 3s")
	}
	synthesis, err := time.ParseDuration(os.Getenv("LATENCY_TTS_SYNTHESIS"))
	if err != nil || synthesis < 0 {
		return nil, errors.New("invalid LATENCY_TTS_SYNTHESIS")
	}
	return latencyTTS{startup: startup, synthesis: synthesis}, nil
}

type latencyASR struct{ realtime bool }

func (a latencyASR) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	out := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 16)
	stop := context.AfterFunc(ctx, func() { _ = input.Close() })
	go func() {
		defer stop()
		defer input.Close()
		defer out.Done(genx.Usage{})
		var id string
		seen, sent := false, false
		for {
			c, err := input.Next()
			if err != nil {
				return
			}
			if c.IsBeginOfStream() {
				id = c.Ctrl.StreamID
				seen = false
				sent = false
			}
			if b, ok := c.Part.(*genx.Blob); ok && len(b.Data) > 0 {
				seen = true
			}
			if seen && !sent && (a.realtime || c.IsEndOfStream()) {
				sent = true
				if err := out.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("Latency fixture reply"), Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true, EndOfStream: true}}); err != nil {
					return
				}
			}
		}
	}()
	return out.Stream(), nil
}

type latencyTTS struct{ startup, synthesis time.Duration }

func latencyWait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (t latencyTTS) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	fmt.Fprintln(os.Stderr, "rtp-bos: startup", t.startup)
	if err := latencyWait(ctx, t.startup); err != nil {
		return nil, err
	}
	out := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
	stop := context.AfterFunc(ctx, func() { _ = input.Close() })
	go func() {
		defer stop()
		defer input.Close()
		fail := func(err error) { _ = out.Stream().CloseWithError(err) }
		for {
			_, err := input.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				fail(err)
				return
			}
		}
		id := genx.NewStreamID()
		if err := latencyWait(ctx, t.synthesis); err != nil {
			fail(err)
			return
		}
		enc, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
		if err != nil {
			fail(err)
			return
		}
		defer enc.Close()
		for i := 0; i < 80; i++ {
			if err := latencyWait(ctx, 20*time.Millisecond); err != nil {
				fail(err)
				return
			}
			pcm := make([]int16, 320)
			for j := range pcm {
				pcm[j] = int16(12000 * math.Sin(2*math.Pi*440*float64(i*320+j)/16000))
			}
			packet, err := enc.Encode(pcm, len(pcm))
			if err != nil {
				fail(err)
				return
			}
			if err := out.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: i == 0}}); err != nil {
				return
			}
		}
		_ = out.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}})
		_ = out.Done(genx.Usage{})
	}()
	return out.Stream(), nil
}
