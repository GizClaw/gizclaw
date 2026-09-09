//go:build gizclaw_genx_e2e

package transformer

import (
	"context"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaotts"
)

// The next PTT recording starts on the first response's audio, without an
// explicit interrupt command. The transformer owns the provider handoff.
func TestDoubaoRealtimePushToTalkLiveOverlappingInput(t *testing.T) {
	requireLiveDoubaoCredentials(t)
	speech, err := doubaotts.NewSeedV2(doubaotts.SeedV2Config{
		Client: liveDoubaoClient(t), Speaker: "zh_female_xiaohe_uranus_bigtts",
	})
	if err != nil {
		t.Fatal(err)
	}
	packets := opusPacketsFromOgg(t, collectTTSAudioE2E(t, speech, "overlap-prompt", "请用中文从一数到三十", "audio/ogg"))
	transcode := false
	transformer, err := doubaorealtime.New(doubaorealtime.Config{
		Client: liveDoubaoClient(t), Model: string(doubaospeech.RealtimeModelO20),
		Mode: doubaorealtime.ModePushToTalk, InputTranscode: &transcode,
		Instructions: "每次收到用户的语音后，请用中文从一数到三十。",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	defer input.CloseWithError(context.Canceled)
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.CloseWithError(context.Canceled)
	type received struct {
		chunk *genx.MessageChunk
		at    time.Time
		err   error
	}
	events := make(chan received, 512)
	receiverDone := make(chan struct{})
	go func() {
		defer close(receiverDone)
		for {
			c, err := output.Next()
			select {
			case events <- received{c, time.Now(), err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	defer func() { cancel(); _ = output.CloseWithError(context.Canceled); <-receiverDone }()
	feedDone := make(chan error, 1)
	go func() { feedDone <- pushDuplexTurn(ctx, input, "overlap-1", packets) }()
	defer func() {
		cancel()
		if feedDone != nil {
			<-feedDone
		}
	}()
	var firstAudio, firstEnd, secondInput time.Time
	var result duplexRoundResult
	firstInputDone, secondStarted, secondInputDone := false, false, false
	for {
		if secondInputDone && len(result.assistantStreams) > 0 && result.lifecycles != nil && result.lifecycles.allComplete() {
			if err := result.terminalError(); err != nil {
				t.Fatal(err)
			}
			if firstEnd.IsZero() || !firstAudio.Before(secondInput) || !secondInput.Before(firstEnd) {
				t.Fatalf("missing input overlap: first_audio=%v second_input=%v first_end=%v", firstAudio, secondInput, firstEnd)
			}
			assertDuplexRound(t, 2, result)
			t.Logf("overlap confirmed; first EOS after second input=%s; second audio bytes=%d", firstEnd.Sub(secondInput), result.assistantAudioBytes)
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("overlapping response timed out: second audio bytes=%d: %v", result.assistantAudioBytes, ctx.Err())
		case err := <-feedDone:
			feedDone = nil
			if err != nil {
				t.Fatal(err)
			}
			if secondStarted {
				secondInputDone = true
			} else {
				firstInputDone = true
			}
		case event := <-events:
			if event.err != nil {
				t.Fatal(event.err)
			}
			c := event.chunk
			if c == nil || c.Ctrl == nil {
				continue
			}
			sourceID := c.Ctrl.SourceStreamID
			if sourceID == "" {
				sourceID = c.Ctrl.StreamID
			}
			if sourceID == "overlap-2" {
				if err := result.observe("overlap-2", c); err != nil {
					t.Fatal(err)
				}
			}
			if sourceID != "overlap-1" || c.Ctrl.Label != duplexAssistantLabel {
				continue
			}
			if b, ok := c.Part.(*genx.Blob); ok {
				if len(b.Data) > 0 && firstAudio.IsZero() {
					firstAudio = event.at
				}
				if c.IsEndOfStream() {
					firstEnd = event.at
				}
			}
		}
		if firstInputDone && !firstAudio.IsZero() && !secondStarted {
			if !firstEnd.IsZero() {
				t.Fatal("first response completed before the next input")
			}
			// Push the second route and its first packet before resuming output reads.
			// Receipt timestamps are captured independently by the receiver above.
			chunks := duplexTurnInputChunks("overlap-2", packets)
			firstPacket := chunks[2].Clone()
			firstPacket.Ctrl.BeginOfStream = true
			if err := input.Push(ctx, firstPacket); err != nil {
				t.Fatal(err)
			}
			secondInput = time.Now()
			secondStarted = true
			feedDone = make(chan error, 1)
			go func() {
				for _, c := range chunks[3:] {
					select {
					case <-ctx.Done():
						feedDone <- ctx.Err()
						return
					case <-time.After(20 * time.Millisecond):
					}
					if err := input.Push(ctx, c); err != nil {
						feedDone <- err
						return
					}
				}
				feedDone <- nil
			}()
		}
	}
}
