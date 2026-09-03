package sfu

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeTalkMessage(t *testing.T) {
	t.Parallel()
	valid, err := talkMessage{V: talkProtocolVersion, Type: talkTypeEOS, Utterance: "u-1", Seq: 7}.encode()
	if err != nil {
		t.Fatalf("encode() error = %v", err)
	}
	message, err := decodeTalkMessage(valid)
	if err != nil || message.Type != talkTypeEOS || message.Utterance != "u-1" || message.Seq != 7 {
		t.Fatalf("decodeTalkMessage(valid) = %+v, %v", message, err)
	}
	for name, payload := range map[string][]byte{
		"empty":           nil,
		"oversized":       []byte("{" + strings.Repeat(" ", maxTalkPayloadBytes) + "}"),
		"not json":        []byte("bos"),
		"wrong version":   []byte(`{"v":0,"type":"bos","utterance":"u","seq":1}`),
		"future version":  []byte(`{"v":2,"type":"bos","utterance":"u","seq":1}`),
		"unknown type":    []byte(`{"v":1,"type":"mid","utterance":"u","seq":1}`),
		"blank utterance": []byte(`{"v":1,"type":"bos","utterance":" ","seq":1}`),
		"zero seq":        []byte(`{"v":1,"type":"bos","utterance":"u"}`),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := decodeTalkMessage(payload); !errors.Is(err, errTalkMessage) {
				t.Fatalf("decodeTalkMessage(%q) error = %v, want errTalkMessage", payload, err)
			}
		})
	}
	if a, b := newUtteranceID(), newUtteranceID(); a == "" || a == b {
		t.Fatalf("newUtteranceID() = %q, %q", a, b)
	}
}
