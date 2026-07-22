package streamkit

import (
	"context"
	"errors"
	"io"
	"strings"
	"unicode"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const (
	defaultTTSSegmentMaxRunes      = 80
	defaultTTSFirstSegmentMinRunes = 8
)

// TTSMeta is immutable input-route metadata supplied to a TTS synthesizer.
type TTSMeta struct {
	Role     genx.Role
	Name     string
	Label    string
	StreamID string
}

type ttsStreamState struct {
	meta      TTSMeta
	segmenter *ttsSentenceSegmenter
	response  *Response
}

// TTSSynthesizer streams one text segment to a provider. emit accepts already
// normalized container bytes and attaches canonical route metadata.
type TTSSynthesizer func(context.Context, string, TTSMeta, string, func([]byte) error) error

// NewTTSStream starts the shared non-provider-specific TTS pipeline. It
// returns immediately; each call owns its invocation, output, segmentation,
// cancellation, and response state.
func NewTTSStream(ctx context.Context, input genx.Stream, config OutputConfig, mimeType string, synthesize TTSSynthesizer) *Output {
	invocation := NewInvocation(ctx, config)
	go runTTS(invocation, input, mimeType, synthesize)
	return invocation.Output()
}

func runTTS(invocation *Invocation, input genx.Stream, mimeType string, synthesize TTSSynthesizer) {
	ctx := invocation.Context()
	states := make(map[string]*ttsStreamState)

	stateFor := func(chunk *genx.MessageChunk) (*ttsStreamState, error) {
		streamID := inputStreamID(chunk)
		state := states[streamID]
		if state == nil {
			meta := TTSMeta{StreamID: streamID}
			updateTTSMeta(&meta, chunk)
			response, err := invocation.StartResponse(ResponseConfig{
				StreamID: meta.StreamID,
				Role:     meta.Role,
				Name:     meta.Name,
				Label:    meta.Label,
			}, mimeType)
			if err != nil {
				return nil, err
			}
			meta.StreamID = response.StreamID()
			state = &ttsStreamState{
				meta:      meta,
				segmenter: newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes),
				response:  response,
			}
			states[streamID] = state
		} else {
			updateTTSMeta(&state.meta, chunk)
		}
		return state, nil
	}

	flushState := func(state *ttsStreamState, all bool) error {
		for _, segment := range state.segmenter.Segments(all) {
			if !hasReadableTTSSpokenText(segment) {
				continue
			}
			debugTTSSegment(state.meta, segment, all)
			emit := func(data []byte) error {
				if len(data) == 0 {
					return nil
				}
				return invocation.Emit(state.response, &genx.MessageChunk{
					Part: &genx.Blob{MIMEType: mimeType, Data: data},
				})
			}
			if err := synthesize(ctx, segment, state.meta, mimeType, emit); err != nil {
				return err
			}
		}
		return nil
	}

	closeState := func(key string, state *ttsStreamState, errorText string) error {
		if errorText == "" {
			if err := flushState(state, true); err != nil {
				errorText = err.Error()
			}
		}
		if err := invocation.FinishResponse(state.response, errorText); err != nil {
			return err
		}
		delete(states, key)
		if errorText != "" {
			return errors.New(errorText)
		}
		return nil
	}

	closeAll := func() error {
		for key, state := range states {
			if err := closeState(key, state, ""); err != nil {
				return err
			}
		}
		return nil
	}

	for {
		chunk, err := input.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, genx.ErrDone) {
				_ = invocation.Fail(err)
				return
			}
			if err := closeAll(); err != nil {
				_ = invocation.Fail(err)
				return
			}
			_ = invocation.Close()
			return
		}
		if chunk == nil {
			continue
		}

		key := inputStreamID(chunk)
		if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			if state := states[key]; state != nil {
				updateTTSMeta(&state.meta, chunk)
				_ = invocation.Interrupt(state.response, chunk.Ctrl.Error)
				delete(states, key)
			} else if err := invocation.Output().Push(chunk); err != nil {
				return
			}
			continue
		}

		text, isText := chunk.Part.(genx.Text)
		if isText {
			state, err := stateFor(chunk)
			if err != nil {
				_ = invocation.Fail(err)
				return
			}
			if text != "" {
				state.segmenter.WriteString(string(text))
				if err := flushState(state, false); err != nil {
					_ = closeState(key, state, err.Error())
					_ = invocation.Fail(err)
					return
				}
			}
			if chunk.IsEndOfStream() {
				if err := closeState(key, state, ""); err != nil {
					_ = invocation.Fail(err)
					return
				}
			}
			continue
		}

		if chunk.IsEndOfStream() {
			if state := states[key]; state != nil {
				updateTTSMeta(&state.meta, chunk)
				if err := closeState(key, state, ""); err != nil {
					_ = invocation.Fail(err)
					return
				}
			}
		}
		if err := invocation.Output().Push(chunk); err != nil {
			return
		}
	}
}

func inputStreamID(chunk *genx.MessageChunk) string {
	if chunk != nil && chunk.Ctrl != nil {
		return strings.TrimSpace(chunk.Ctrl.StreamID)
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

func updateTTSMeta(meta *TTSMeta, chunk *genx.MessageChunk) {
	meta.Role = chunk.Role
	meta.Name = chunk.Name
	if chunk.Ctrl != nil {
		if streamID := strings.TrimSpace(chunk.Ctrl.StreamID); streamID != "" {
			meta.StreamID = streamID
		}
		if chunk.Ctrl.Label != "" {
			meta.Label = chunk.Ctrl.Label
		}
	}
}
