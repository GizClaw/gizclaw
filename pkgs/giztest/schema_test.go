package giztest

import (
	"strings"
	"testing"
)

func TestEmbeddedSchemaCompiles(t *testing.T) {
	if _, err := schemaOnce(); err != nil {
		t.Fatal(err)
	}
}
func TestSchemaRejectsUnknownField(t *testing.T) {
	_, err := LoadDocument(writeTestDocument(t, strings.Replace(validDocument, "name: ping-connectivity", "name: ping-connectivity\nunknown: true", 1)), nil)
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error = %v", err)
	}
}

func TestEmbeddedSchemaValidatesPersistentPeerStreamFields(t *testing.T) {
	valid := persistentPeerStreamDocument("      session: microphone\n      keep_open: true\n")
	if _, err := LoadDocument(writeTestDocument(t, valid), nil); err != nil {
		t.Fatalf("persistent peer_stream schema rejected valid document: %v", err)
	}
	for name, fields := range map[string]string{
		"keep_open type":   "      session: microphone\n      keep_open: retained\n",
		"await_rearm type": "      session: microphone\n      await_rearm: 42\n",
		"unknown field":    "      session: microphone\n      keep_open: true\n      retain_forever: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadDocument(writeTestDocument(t, persistentPeerStreamDocument(fields)), nil); err == nil || !strings.Contains(err.Error(), "schema") {
				t.Fatalf("schema error = %v", err)
			}
		})
	}
}

func persistentPeerStreamDocument(fields string) string {
	return validDocument + "  - id: turn\n    client: peer\n    peer_stream:\n      mode: realtime\n      input: hello\n" + fields
}

func TestOverlapInputContract(t *testing.T) {
	for _, mode := range []string{"push-to-talk", "realtime"} {
		valid := validDocument + "  - id: overlap\n    client: peer\n    peer_stream:\n      mode: " + mode + "\n      input: audio\n      overlap_input: true\n"
		if _, err := LoadDocument(writeTestDocument(t, valid), nil); err != nil {
			t.Fatal(err)
		}
		for _, extra := range []string{"interrupt_after: 1s", "completion: first_response", "keep_open: true", "require_audio: false", "idle_timeout: 1s", "empty_input: true"} {
			if _, err := LoadDocument(writeTestDocument(t, valid+"      "+extra+"\n"), nil); err == nil {
				t.Fatalf("accepted overlap with %s", extra)
			}
		}
	}
	for _, mode := range []string{"text", "listen"} {
		if err := validatePeerStreamStep(Step{ID: "overlap", PeerStream: &PeerStreamOperation{Mode: mode, Input: "audio", OverlapInput: true}}, false); err == nil {
			t.Fatalf("accepted %s overlap", mode)
		}
	}
}
