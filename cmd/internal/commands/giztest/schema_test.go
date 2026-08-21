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
