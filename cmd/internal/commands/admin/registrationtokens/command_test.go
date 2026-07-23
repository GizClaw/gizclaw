package registrationtokenscmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestReadUpsertReadsCompleteRegistrationTokenFromStdin(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(bytes.NewBufferString(`{
		"name":"app:com.gizclaw.opensource",
		"token":"desktop-token",
		"runtime_profile_name":"default"
	}`))
	item, err := readUpsert(cmd, "-")
	if err != nil {
		t.Fatalf("readUpsert() error = %v", err)
	}
	if item.Name != "app:com.gizclaw.opensource" || item.Token != "desktop-token" || item.RuntimeProfileName != "default" {
		t.Fatalf("readUpsert() = %#v", item)
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
