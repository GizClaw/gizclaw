package doubaorealtime

import (
	"slices"
	"strings"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
)

// realtimeVADResponses is owned by the provider event reader. A VAD interruption
// must keep audio already accepted by that provider while excluding the old reply.
type realtimeVADResponses struct {
	question string
	retired  []string
	tagged   bool
	reply    doubaoRealtimePTTResponseIdentity
	audio    doubaoRealtimePTTResponseIdentity
	audioSet bool
	audioOK  bool
}

func (v *realtimeVADResponses) questionStarted(id string) (accepted, changed bool) {
	id = strings.TrimSpace(id)
	if id == "" || id == v.question {
		return true, false
	}
	if slices.Contains(v.retired, id) {
		return false, false
	}
	if v.question != "" {
		v.retired = append(v.retired, v.question)
		if len(v.retired) > 64 {
			v.retired = slices.Delete(v.retired, 0, len(v.retired)-64)
		}
	}
	v.question = id
	v.reply = doubaoRealtimePTTResponseIdentity{}
	return true, true
}

func (v *realtimeVADResponses) acceptsIdentity(id doubaoRealtimePTTResponseIdentity) bool {
	id = id.normalized()
	if v.question == "" {
		return true
	}
	if id.questionID != "" && id.questionID != v.question {
		return false
	}
	if !v.tagged && id.questionID == "" {
		return true
	}
	if v.reply.empty() {
		if id.questionID != v.question {
			return false
		}
		v.reply = id
	} else if v.reply.conflicts(id) {
		return false
	} else {
		v.reply.merge(id)
	}
	v.tagged = true
	return true
}

func (v *realtimeVADResponses) acceptsOutput(event *doubaospeech.RealtimeEvent) bool {
	id := doubaoRealtimeEventResponseIdentity(event)
	switch event.Type {
	case doubaospeech.EventTTSStarted, doubaospeech.EventTTSSegmentEnd,
		doubaospeech.EventTTSAudioData, doubaospeech.EventTTSFinished,
		doubaospeech.EventChatResponse, doubaospeech.EventChatEnded:
	default:
		return true
	}
	if event.Type == doubaospeech.EventTTSStarted {
		accepted := v.acceptsIdentity(id)
		v.audio, v.audioSet = id, true
		v.audioOK = accepted
		if accepted {
			v.audio.questionID = v.question
		}
		return accepted
	}
	if id.empty() && v.audioSet && (event.Type == doubaospeech.EventTTSAudioData ||
		event.Type == doubaospeech.EventTTSSegmentEnd || event.Type == doubaospeech.EventTTSFinished) {
		if !v.audioOK {
			return false
		}
		id = v.audio
	}
	return v.acceptsIdentity(id)
}
