package transformers

import (
	"context"
	"errors"
	"io"
	"strings"
	"unicode"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

const (
	defaultTTSSegmentMaxRunes      = 80
	defaultTTSFirstSegmentMinRunes = 8
)

type ttsChunkMeta struct {
	Role     genx.Role
	Name     string
	StreamID string
}

type ttsStreamState struct {
	meta      ttsChunkMeta
	segmenter *ttsSentenceSegmenter
}

type ttsSynthesizer func(context.Context, string, ttsChunkMeta, string, *bufferStream) error

func runTTSTransform(ctx context.Context, input genx.Stream, output *bufferStream, mimeType string, synthesize ttsSynthesizer) {
	defer output.Close()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	states := map[string]*ttsStreamState{}

	stateFor := func(chunk *genx.MessageChunk) *ttsStreamState {
		streamID := chunkStreamID(chunk)
		state := states[streamID]
		if state == nil {
			state = &ttsStreamState{segmenter: newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)}
			states[streamID] = state
		}
		updateTTSMeta(&state.meta, chunk)
		return state
	}

	flushState := func(state *ttsStreamState, all bool) error {
		for _, segment := range state.segmenter.Segments(all) {
			if !hasReadableTTSSpokenText(segment) {
				continue
			}
			debugTTSSegment(state.meta, segment, all)
			if err := synthesize(ctx, segment, state.meta, mimeType, output); err != nil {
				return err
			}
		}
		return nil
	}

	closeState := func(streamID string, state *ttsStreamState, errText string) error {
		if errText == "" {
			if err := flushState(state, true); err != nil {
				return err
			}
		}
		if err := output.Push(newTTSEOSChunk(state.meta, mimeType, errText)); err != nil {
			return err
		}
		delete(states, streamID)
		return nil
	}

	closeAll := func() error {
		for streamID, state := range states {
			if err := closeState(streamID, state, ""); err != nil {
				return err
			}
		}
		return nil
	}

	for {
		chunk, err := input.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, genx.ErrDone) {
				output.CloseWithError(err)
				return
			}
			if err := closeAll(); err != nil {
				output.CloseWithError(err)
			}
			return
		}
		if chunk == nil {
			continue
		}

		streamID := chunkStreamID(chunk)
		if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			if state := states[streamID]; state != nil {
				updateTTSMeta(&state.meta, chunk)
				if err := closeState(streamID, state, chunk.Ctrl.Error); err != nil {
					output.CloseWithError(err)
				}
			} else if err := output.Push(chunk); err != nil {
				return
			}
			continue
		}

		text, isText := chunk.Part.(genx.Text)
		if isText {
			state := stateFor(chunk)
			if text != "" {
				state.segmenter.WriteString(string(text))
				if err := flushState(state, false); err != nil {
					output.CloseWithError(err)
					return
				}
			}
			if chunk.IsEndOfStream() {
				if err := closeState(streamID, state, ""); err != nil {
					output.CloseWithError(err)
					return
				}
			}
			continue
		}

		if chunk.IsEndOfStream() {
			if state := states[streamID]; state != nil {
				updateTTSMeta(&state.meta, chunk)
				if err := closeState(streamID, state, ""); err != nil {
					output.CloseWithError(err)
					return
				}
			}
		}
		if err := output.Push(chunk); err != nil {
			return
		}
	}
}

func chunkStreamID(chunk *genx.MessageChunk) string {
	if chunk != nil && chunk.Ctrl != nil {
		return chunk.Ctrl.StreamID
	}
	return ""
}

func hasReadableTTSSpokenText(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

func updateTTSMeta(meta *ttsChunkMeta, chunk *genx.MessageChunk) {
	meta.Role = chunk.Role
	meta.Name = chunk.Name
	if chunk.Ctrl != nil && chunk.Ctrl.StreamID != "" {
		meta.StreamID = chunk.Ctrl.StreamID
	}
}

func pushTTSAudioChunk(output *bufferStream, meta ttsChunkMeta, mimeType string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	chunk := &genx.MessageChunk{
		Role: meta.Role,
		Name: meta.Name,
		Part: &genx.Blob{
			MIMEType: mimeType,
			Data:     normalizeTTSAudio(mimeType, data),
		},
	}
	if meta.StreamID != "" {
		chunk.Ctrl = &genx.StreamCtrl{StreamID: meta.StreamID}
	}
	return output.Push(chunk)
}

func newTTSEOSChunk(meta ttsChunkMeta, mimeType, errText string) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: meta.Role,
		Name: meta.Name,
		Part: &genx.Blob{MIMEType: mimeType},
		Ctrl: &genx.StreamCtrl{StreamID: meta.StreamID, EndOfStream: true, Error: errText},
	}
}
