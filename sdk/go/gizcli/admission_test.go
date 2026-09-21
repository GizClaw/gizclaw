package gizcli

import (
	"encoding/hex"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestRegistrationTokenCredentialWireVector(t *testing.T) {
	credential, err := RegistrationTokenCredential("token")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := proto.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Type != RegistrationTokenCredentialType || hex.EncodeToString(encoded) != "0801121e67697a636c61772e636f6d2f726567697374726174696f6e5f746f6b656e1a05746f6b656e" {
		t.Fatal("registration-token helper differs from JS, Dart and C wire vector")
	}
}

func TestRegistrationTokenCredentialValueBoundary(t *testing.T) {
	for _, value := range []string{strings.Repeat("x", 512), strings.Repeat("é", 256)} {
		credential, err := RegistrationTokenCredential(value)
		if err != nil || credential.Value != value {
			t.Fatalf("512-byte value rejected: %v", err)
		}
		if credential, err := RegistrationTokenCredential(value + "x"); err == nil || credential != nil {
			t.Fatal("513-byte value accepted")
		}
	}
	if _, err := RegistrationTokenCredential(string([]byte{255})); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}
