package eino

import (
	"github.com/cloudwego/eino/schema"
)

// transcriptExtraKey marks the stream message a ChatModel component returns
// to report the transcript of the audio user turn it received.
const transcriptExtraKey = "genx.input_transcript"

// TranscriptMessage returns the stream message a ChatModel component sends,
// ahead of its reply, to report the transcript of the audio in its latest user
// message. The AudioTranscript node publishes it as the turn's user text.
func TranscriptMessage(text string) *schema.Message {
	return &schema.Message{Role: schema.User, Content: text, Extra: map[string]any{transcriptExtraKey: true}}
}

func transcriptOf(message *schema.Message) (string, bool) {
	if message == nil || message.Role != schema.User {
		return "", false
	}
	marked, _ := message.Extra[transcriptExtraKey].(bool)
	return message.Content, marked
}

// transcriptPublisher receives the transcript of the active audio turn.
type transcriptPublisher interface {
	audioTurn() bool
	PublishTranscript(string) error
}

// transcriptPublisher returns the root turn publisher when the run is an audio
// turn.
func (state *runState) transcriptPublisher() (transcriptPublisher, bool) {
	publisher, ok := state.emitter.(transcriptPublisher)
	if !ok || !publisher.audioTurn() {
		return nil, false
	}
	return publisher, true
}

func messagesContainAudio(messages []*schema.Message) bool {
	for _, message := range messages {
		if message == nil || message.Role != schema.User {
			continue
		}
		for _, part := range message.UserInputMultiContent {
			if part.Type == schema.ChatMessagePartTypeAudioURL && part.Audio != nil {
				return true
			}
		}
	}
	return false
}

// audioTranscript tracks the transcript a ChatModel component reports during
// one audio turn. Tool rounds resend the audio, so only the first report is
// published.
type audioTranscript struct {
	publisher transcriptPublisher
	published bool
	text      string
}

func (transcript *audioTranscript) observe(text string) error {
	if transcript == nil || transcript.published {
		return nil
	}
	transcript.published = true
	transcript.text = text
	return transcript.publisher.PublishTranscript(text)
}
