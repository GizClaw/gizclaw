package registrationtokenscmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestReadUpsertReadsCompleteRegistrationTokenFromStdin(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(bytes.NewBufferString(`{
		"id":"app:com.gizclaw.opensource",
		"token":"desktop-token",
		"runtime_profile_id":"default",
		"enabled":false, "expires_at":"2035-01-01T00:00:00Z", "max_activations":2
	}`))
	item, err := readUpsert(cmd, "-")
	if err != nil {
		t.Fatalf("readUpsert() error = %v", err)
	}
	if item.Id != "app:com.gizclaw.opensource" || item.Token != "desktop-token" || item.RuntimeProfileId != "default" {
		t.Fatalf("readUpsert() = %#v", item)
	}
	if item.Enabled == nil || *item.Enabled || item.ExpiresAt == nil || item.ExpiresAt.Year() != 2035 || item.MaxActivations == nil || *item.MaxActivations != 2 {
		t.Fatalf("lifecycle fields lost: %#v", item)
	}
}

func TestRegistrationTokenCommandsIncludePut(t *testing.T) {
	cmd := NewCmd()
	found := map[string]bool{}
	for _, child := range cmd.Commands() {
		found[child.Name()] = true
	}
	for _, name := range []string{"create", "put", "get", "list", "delete"} {
		if !found[name] {
			t.Fatalf("RegistrationToken command %q is missing", name)
		}
	}
}
