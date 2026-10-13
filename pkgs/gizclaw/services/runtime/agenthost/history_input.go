package agenthost

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// historyInputStream commits complete raw text before the Agent receives its
// terminal. ASR and native audio transcripts still arrive through output.
type historyInputStream struct {
	genx.Stream
	ctx      context.Context
	recorder *historyRecorder
	streamID string
	text     strings.Builder
	audio    bool
}

func (s *historyInputStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.Stream.Next()
	if chunk == nil || s.recorder.inputRecorded {
		if err != nil {
			s.text.Reset()
		}
		return chunk, err
	}
	id := ""
	if chunk.Ctrl != nil {
		id = strings.TrimSpace(chunk.Ctrl.StreamID)
	}
	if chunk.IsBeginOfStream() || (id != "" && id != s.streamID) {
		s.streamID = id
		s.text.Reset()
		s.audio = false
	}
	if mime, ok := chunk.MIMEType(); ok && strings.HasPrefix(mime, "audio/") {
		if !s.audio {
			s.recorder.forgetTextInput(s.streamID)
		}
		s.audio = true
	}
	if text, ok := chunk.Part.(genx.Text); ok && chunk.Role == genx.RoleUser && !s.audio {
		if s.streamID == "" {
			s.streamID = genx.NewStreamID()
		}
		s.recorder.rememberTextInput(s.streamID)
		if chunk.Ctrl == nil || !chunk.Ctrl.TextInterim {
			s.text.WriteString(string(text))
		}
	}
	if id == "" && s.streamID != "" && !s.audio {
		chunk = chunk.Clone()
		if chunk.Ctrl == nil {
			chunk.Ctrl = &genx.StreamCtrl{}
		}
		chunk.Ctrl.StreamID = s.streamID
	}
	if chunk.IsEndOfStream() && !s.audio {
		text := s.text.String()
		if strings.TrimSpace(text) != "" && genx.StreamError(chunk.Ctrl) == nil {
			entry := &genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: s.streamID, Label: "transcript", BeginOfStream: true, EndOfStream: true}}
			if recordErr := s.recorder.observe(s.ctx, entry, historyEntryTypeGear, s.recorder.gearID); recordErr != nil {
				return nil, fmt.Errorf("agenthost: record input History: %w", recordErr)
			}
		}
		s.text.Reset()
		s.streamID = ""
	}
	return chunk, err
}

// Recent raw input identities suppress an Agent's transcript echo. Playback
// ownership rejects stale turns; keep this cache bounded for long sessions.
const historyTextInputCacheSize = 128

func (r *historyRecorder) recordedTextInput(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return id != "" && slices.Contains(r.textInputs, id)
}

func (r *historyRecorder) rememberTextInput(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if slices.Contains(r.textInputs, id) {
		return
	}
	if len(r.textInputs) == historyTextInputCacheSize {
		r.textInputs = slices.Delete(r.textInputs, 0, 1)
	}
	r.textInputs = append(r.textInputs, id)
}

func (r *historyRecorder) forgetTextInput(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.textInputs = slices.DeleteFunc(r.textInputs, func(value string) bool { return value == id })
}
