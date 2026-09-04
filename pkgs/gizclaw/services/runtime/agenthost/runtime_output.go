package agenthost

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"strings"

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

// OutputProductionObserver lets the final producer boundary report a chunk
// before it becomes available to an independently scheduled consumer.
type OutputProductionObserver interface {
	SetOutputProductionObserver(func(*genx.MessageChunk))
}

type outputObservationAbandoner interface {
	AbandonOutputObservation(*genx.MessageChunk)
}

func deferOutputObservation(stream genx.Stream) OutputObservationStream {
	observer, _ := stream.(OutputObservationStream)
	if observer != nil {
		observer.DeferOutputObservation()
	}
	return observer
}

// OpusPassthroughMIME marks a raw Opus packet that must reach the Peer's
// Opus track unchanged. MixerOutput never decodes or mixes chunks carrying
// it; the peer-side consumer claims them through MixerOutput.Passthrough and
// writes each payload straight to the device. The SFU driver uses it so the
// downlink costs no decode: the Opus RTP clock is always 48 kHz while the
// payload keeps the sending device's internal bandwidth, so the packet the
// remote device encoded is exactly the packet this device can decode.
const OpusPassthroughMIME = "audio/opus; passthrough=1"

// IsOpusPassthroughMIME reports whether mimeType carries the passthrough
// marker: base type audio/opus with the passthrough=1 parameter.
func IsOpusPassthroughMIME(mimeType string) bool {
	base, params, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return false
	}
	return strings.EqualFold(base, "audio/opus") && params["passthrough"] == "1"
}

// IsOpusPassthroughChunk reports whether chunk is a passthrough audio chunk:
// a Blob part whose MIME carries the passthrough marker. BOS and EOS chunks
// of a passthrough route qualify as well as its packet chunks.
func IsOpusPassthroughChunk(chunk *genx.MessageChunk) bool {
	if chunk == nil {
		return false
	}
	blob, ok := chunk.Part.(*genx.Blob)
	return ok && blob != nil && IsOpusPassthroughMIME(blob.MIMEType)
}

type MixerOutput struct {
	Tracks  AudioTrackCreator
	Observe func(*genx.MessageChunk) error
	// Passthrough claims chunks that bypass decoding and the mixer entirely.
	// A claimed chunk is never handed to the audio tracks, so no decoder or
	// mixer track is created for it, but it is still observed in order so
	// BOS/EOS route bookkeeping and Peer Events fire for it. The consumer
	// that owns the device transport installs it; without it passthrough
	// chunks are rejected rather than decoded.
	Passthrough func(*genx.MessageChunk) error
	// OnAudioCutover runs after the superseded audio track drains and before
	// the replacement BOS is observed.
	OnAudioCutover    func(*genx.MessageChunk) error
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
				if err := tracks.closeWrite(); err != nil {
					return err
				}
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
		if IsOpusPassthroughChunk(chunk) {
			if o.Passthrough == nil {
				return fmt.Errorf("agenthost: passthrough audio stream_id=%q has no packet writer", chunkStreamID(chunk))
			}
			if err := o.Passthrough(chunk); err != nil {
				return err
			}
		} else if err := tracks.consume(chunk); err != nil {
			return err
		}
		interruptedDrained := false
		if o.WaitForAudioDrain {
			var err error
			interruptedDrained, err = tracks.waitInterrupted(ctx, chunk)
			if err != nil {
				return err
			}
		}
		if o.WaitForAudioDrain && tracks.takeCutoverPending() {
			if err := tracks.waitPending(ctx); err != nil {
				return err
			}
			if o.OnAudioCutover != nil {
				if err := o.OnAudioCutover(chunk); err != nil {
					return err
				}
			}
		}
		var superseded []*genx.MessageChunk
		pendingObserve, superseded = removeSupersededAudioEOS(pendingObserve, chunk)
		if abandoner, ok := output.(outputObservationAbandoner); ok {
			for _, replaced := range superseded {
				abandoner.AbandonOutputObservation(replaced)
			}
		}
		tracks.removeDrainedPending()
		if o.WaitForAudioDrain && !interruptedDrained && shouldWaitForAudioDrain(chunk) &&
			(len(pendingObserve) > 0 || tracks.hasPending()) {
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

func chunkStreamID(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return ""
	}
	return chunk.Ctrl.StreamID
}

func shouldWaitForAudioDrain(chunk *genx.MessageChunk) bool {
	if chunk == nil || chunk.IsBeginOfStream() {
		return false
	}
	if chunk.Part == nil {
		return chunk.IsEndOfStream()
	}
	mimeType, ok := chunk.MIMEType()
	return ok && isMixerAudioMIME(mimeType) && chunk.IsEndOfStream()
}

func removeSupersededAudioEOS(pending []*genx.MessageChunk, interrupt *genx.MessageChunk) ([]*genx.MessageChunk, []*genx.MessageChunk) {
	if interrupt == nil || interrupt.Ctrl == nil || !interrupt.IsEndOfStream() || interrupt.Ctrl.Error == "" {
		return pending, nil
	}
	interruptMIME, hasInterruptMIME := interrupt.MIMEType()
	routeInterrupt := interrupt.Part == nil
	if !routeInterrupt && (!hasInterruptMIME || !isMixerAudioMIME(interruptMIME)) {
		return pending, nil
	}
	kept := pending[:0]
	var superseded []*genx.MessageChunk
	for _, chunk := range pending {
		if chunk == nil {
			kept = append(kept, chunk)
			continue
		}
		queuedMIME, queuedMIMEOK := chunk.MIMEType()
		if chunk.Ctrl != nil && chunk.IsEndOfStream() && chunk.Ctrl.Error == "" &&
			chunk.Ctrl.StreamID == interrupt.Ctrl.StreamID {
			if routeInterrupt && chunk.Part == nil {
				superseded = append(superseded, chunk)
				continue
			}
			if queuedMIMEOK && isMixerAudioMIME(queuedMIME) && (routeInterrupt || queuedMIME == interruptMIME) {
				superseded = append(superseded, chunk)
				continue
			}
		}
		kept = append(kept, chunk)
	}
	return kept, superseded
}
