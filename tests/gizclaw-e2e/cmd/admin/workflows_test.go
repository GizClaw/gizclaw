//go:build gizclaw_e2e

package admin_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminWorkflowsUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "507-admin-workflows")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)

	list := h.RunCLI("admin", "workflows", "list", "--context", "admin-a")
	list.MustSucceed(t)
	if !strings.Contains(list.Stdout, `"name":"flowcraft-assistant"`) {
		t.Fatalf("workflows list missing flowcraft-assistant:\n%s", list.Stdout)
	}
	for _, want := range []string{`"name":"flowcraft-support"`, `"name":"flowcraft-scenario-119"`} {
		if !strings.Contains(list.Stdout, want) {
			t.Fatalf("workflows list missing %q:\n%s", want, list.Stdout)
		}
	}

	get := h.RunCLI("admin", "workflows", "get", "flowcraft-assistant", "--context", "admin-a")
	get.MustSucceed(t)
	if !strings.Contains(get.Stdout, `"driver":"flowcraft"`) {
		t.Fatalf("workflows get missing driver:\n%s", get.Stdout)
	}

	rpcGet := h.RunCLI("admin", "workflows", "get", "flowcraft-support", "--context", "admin-a")
	rpcGet.MustSucceed(t)
	if !strings.Contains(rpcGet.Stdout, `"name":"flowcraft-support"`) || !strings.Contains(rpcGet.Stdout, `"driver":"flowcraft"`) {
		t.Fatalf("workflows get missing resource fields:\n%s", rpcGet.Stdout)
	}

	asset := filepath.Join("..", "..", "testdata", "assets", "workflows", "flowcraft-support", "icon.png")
	for range 2 {
		upload := h.RunCLI("admin", "workflows", "upload-icon", "flowcraft-support", "--format", "png", "-f", asset, "--context", "admin-a")
		upload.MustSucceed(t)
		if !strings.Contains(upload.Stdout, `"png":"flowcraft-support/icon.png"`) {
			t.Fatalf("workflow icon upload missing stable slot:\n%s", upload.Stdout)
		}
	}
	output := filepath.Join(t.TempDir(), "flowcraft-support.png")
	download := h.RunCLI("admin", "workflows", "download-icon", "flowcraft-support", "--format", "png", "-o", output, "--context", "admin-a")
	download.MustSucceed(t)
	want, err := os.ReadFile(asset)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("workflow CLI download differs from committed fixture")
	}
}
