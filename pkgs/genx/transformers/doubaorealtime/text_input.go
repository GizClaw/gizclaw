package doubaorealtime

import (
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const doubaoRealtimeTextInputLimit = 1 << 20

// Scoped text is a user message whose fragments end at EOS. SendText submits
// a complete provider turn, so forwarding individual fragments would start
// competing responses. Unscoped text already represents a complete message.
type doubaoRealtimeTextInput struct {
	streamID string
	text     strings.Builder
	ended    bool
}

func (s *doubaoRealtimeTextInput) push(chunk *genx.MessageChunk) (*genx.MessageChunk, error) {
	text, isText := chunk.Part.(genx.Text)
	if chunk.Part != nil && !isText {
		return chunk, nil
	}
	if chunk.Ctrl != nil && chunk.IsEndOfStream() && chunk.Ctrl.Error != "" {
		s.text.Reset()
		s.ended = true
		return nil, fmt.Errorf("doubao realtime text input failed: %s", chunk.Ctrl.Error)
	}
	if chunk.Ctrl == nil || strings.TrimSpace(chunk.Ctrl.StreamID) == "" {
		if len(text) > doubaoRealtimeTextInputLimit {
			return nil, fmt.Errorf("doubao realtime text input exceeds %d bytes", doubaoRealtimeTextInputLimit)
		}
		return chunk, nil
	}
	streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
	if streamID != s.streamID {
		s.streamID = streamID
		s.text.Reset()
		s.ended = false
	}
	if s.ended {
		return nil, nil
	}
	if len(text) > doubaoRealtimeTextInputLimit-s.text.Len() {
		return nil, fmt.Errorf("doubao realtime text input exceeds %d bytes", doubaoRealtimeTextInputLimit)
	}
	s.text.WriteString(string(text))
	if chunk.IsEndOfStream() {
		result := *chunk
		result.Part = genx.Text(s.text.String())
		s.text.Reset()
		s.ended = true
		return &result, nil
	}
	if isText {
		if chunk.IsBeginOfStream() {
			result := *chunk
			result.Part = genx.Text("")
			return &result, nil
		}
		return nil, nil
	}
	return chunk, nil
}
