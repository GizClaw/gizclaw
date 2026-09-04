package sfu

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Talk segment protocol. Every participant announces the boundaries of its
// own utterances on the Room's reliable data channel so listeners can grant
// the floor without decoding anything. The sender identity comes from the
// SFU (the participant the packet was received from), never from the
// payload.
const (
	// talkTopic is the data channel topic that carries talk messages.
	talkTopic = "gizclaw.sfu.talk"
	// talkProtocolVersion is the only accepted "v" value.
	talkProtocolVersion = 1
	talkTypeBOS         = "bos"
	talkTypeEOS         = "eos"
	// maxTalkPayloadBytes bounds what a remote participant may make the
	// session parse.
	maxTalkPayloadBytes = 256
)

// errTalkMessage reports a data packet on talkTopic that does not conform to
// the protocol.
var errTalkMessage = errors.New("sfu: malformed talk message")

// talkMessage is the wire form of one utterance boundary.
type talkMessage struct {
	V         int    `json:"v"`
	Type      string `json:"type"`
	Utterance string `json:"utterance"`
	Seq       uint64 `json:"seq"`
}

func (m talkMessage) encode() ([]byte, error) {
	return json.Marshal(m)
}

// decodeTalkMessage parses and validates one talk payload.
func decodeTalkMessage(payload []byte) (talkMessage, error) {
	if len(payload) == 0 || len(payload) > maxTalkPayloadBytes {
		return talkMessage{}, fmt.Errorf("%w: %d bytes", errTalkMessage, len(payload))
	}
	var message talkMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		return talkMessage{}, fmt.Errorf("%w: %w", errTalkMessage, err)
	}
	if message.V != talkProtocolVersion {
		return talkMessage{}, fmt.Errorf("%w: unsupported version %d", errTalkMessage, message.V)
	}
	if message.Type != talkTypeBOS && message.Type != talkTypeEOS {
		return talkMessage{}, fmt.Errorf("%w: unknown type %q", errTalkMessage, message.Type)
	}
	if strings.TrimSpace(message.Utterance) == "" {
		return talkMessage{}, fmt.Errorf("%w: missing utterance", errTalkMessage)
	}
	if message.Seq == 0 {
		return talkMessage{}, fmt.Errorf("%w: missing seq", errTalkMessage)
	}
	return message, nil
}

// newUtteranceID mints the random identifier of one talk utterance.
func newUtteranceID() string {
	return rand.Text()
}
