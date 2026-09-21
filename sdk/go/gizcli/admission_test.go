package gizcli

import (
	"encoding/hex"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestRegistrationTokenCredentialWireVector(t *testing.T) {
	encoded, err := proto.Marshal(RegistrationTokenCredential("token"))
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(encoded) != "08011212726567697374726174696f6e5f746f6b656e1a05746f6b656e" {
		t.Fatal("registration-token helper differs from JS, Dart and C wire vector")
	}
}
