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
	_, err := loadDocument(writeTestDocument(t, strings.Replace(validDocument, "name: ping-connectivity", "name: ping-connectivity\nunknown: true", 1)))
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error = %v", err)
	}
}

func TestEmbeddedSchemaValidatesPersistentPeerStreamFields(t *testing.T) {
	valid := persistentPeerStreamDocument("      session: microphone\n      keep_open: true\n")
	if _, err := loadDocument(writeTestDocument(t, valid)); err != nil {
		t.Fatalf("persistent peer_stream schema rejected valid document: %v", err)
	}
	for name, fields := range map[string]string{
		"keep_open type":   "      session: microphone\n      keep_open: retained\n",
		"await_rearm type": "      session: microphone\n      await_rearm: 42\n",
		"unknown field":    "      session: microphone\n      keep_open: true\n      retain_forever: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadDocument(writeTestDocument(t, persistentPeerStreamDocument(fields))); err == nil || !strings.Contains(err.Error(), "schema") {
				t.Fatalf("schema error = %v", err)
			}
		})
	}
}

func persistentPeerStreamDocument(fields string) string {
	return validDocument + "  - id: turn\n    client: peer\n    peer_stream:\n      mode: realtime\n      input: hello\n" + fields
}
