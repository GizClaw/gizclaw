package adminfirmwares_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestAdminFirmwaresUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "511-admin-firmwares")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-sn").MustSucceed(t)
	h.CreateContext("device-a").MustSucceed(t)
	h.RegisterContext("device-a", "--sn", "device-sn").MustSucceed(t)

	firmwarePath := filepath.Join(h.SandboxDir, "firmware.json")
	if err := os.WriteFile(firmwarePath, []byte(`{
			"name": "devkit",
			"description": "Devkit firmware line",
			"slots": {
				"stable": {"version": "1.0.0", "artifacts": [{"name": "main", "kind": "app"}]},
				"beta": {"version": "1.1.0"},
				"develop": {"version": "1.2.0"},
				"pending": {"version": "1.3.0", "artifacts": [{"name": "assets", "kind": "data"}]}
			}
	}`), 0o644); err != nil {
		t.Fatalf("write firmware file: %v", err)
	}
	appBinPath := filepath.Join(h.SandboxDir, "app.bin")
	if err := os.WriteFile(appBinPath, []byte("app firmware payload"), 0o644); err != nil {
		t.Fatalf("write app bin: %v", err)
	}
	dataBinPath := filepath.Join(h.SandboxDir, "data.bin")
	if err := os.WriteFile(dataBinPath, []byte("data firmware payload"), 0o644); err != nil {
		t.Fatalf("write data bin: %v", err)
	}

	put := h.RunCLI("admin", "firmwares", "put", "devkit", "-f", firmwarePath, "--context", "admin-a")
	put.MustSucceed(t)
	assertContains(t, put.Stdout, `"name":"devkit"`, `"version":"1.0.0"`)

	list := h.RunCLI("admin", "firmwares", "list", "--context", "admin-a")
	list.MustSucceed(t)
	assertContains(t, list.Stdout, `"name":"devkit"`, `"description":"Devkit firmware line"`)

	get := h.RunCLI("admin", "firmwares", "get", "devkit", "--context", "admin-a")
	get.MustSucceed(t)
	assertContains(t, get.Stdout, `"kind":"app"`, `"kind":"data"`)

	uploadApp := h.RunCLI("admin", "firmwares", "upload-bin", "devkit", "--channel", "stable", "--bin", "main", "-f", appBinPath, "--context", "admin-a")
	uploadApp.MustSucceed(t)
	assertContains(t, uploadApp.Stdout, `"path":"devkit/stable/main/`, `"size":20`, `"sha256":`)

	uploadData := h.RunCLI("admin", "firmwares", "upload-bin", "devkit", "--channel", "pending", "--bin", "assets", "-f", dataBinPath, "--context", "admin-a")
	uploadData.MustSucceed(t)
	assertContains(t, uploadData.Stdout, `"path":"devkit/pending/assets/`, `"size":21`, `"sha256":`)

	configPath := filepath.Join(h.SandboxDir, "device-firmware-config.json")
	if err := os.WriteFile(configPath, []byte(`{"firmware":{"id":"devkit","channel":"stable"}}`), 0o644); err != nil {
		t.Fatalf("write peer config: %v", err)
	}
	putConfig := h.RunCLI("admin", "peers", "put-config", h.ContextPublicKey("device-a"), "--file", configPath, "--context", "admin-a")
	putConfig.MustSucceed(t)
	assertContains(t, putConfig.Stdout, `"firmware":{`, `"id":"devkit"`, `"channel":"stable"`)
	grantFirmwareRead(t, h, "device-a", "devkit")
	assertDeviceFirmwareRPC(t, h, "device-a", filepath.Join(h.SandboxDir, "downloaded-app.bin"))

	release := h.RunCLI("admin", "firmwares", "release", "devkit", "--context", "admin-a")
	release.MustSucceed(t)
	assertContains(t, release.Stdout, `"stable":{"artifacts":[{`, `"kind":"data"`, `"name":"assets"`, `"path":"devkit/pending/assets/`, `"version":"1.3.0"`, `"beta":{"artifacts":[{`, `"kind":"app"`, `"name":"main"`, `"path":"devkit/stable/main/`, `"version":"1.0.0"`)

	rollback := h.RunCLI("admin", "firmwares", "rollback", "devkit", "--context", "admin-a")
	rollback.MustSucceed(t)
	assertContains(t, rollback.Stdout, `"stable":{"artifacts":[{`, `"kind":"app"`, `"name":"main"`, `"path":"devkit/stable/main/`, `"version":"1.0.0"`)

	resource := h.RunCLI("admin", "show", "Firmware", "devkit", "--context", "admin-a")
	resource.MustSucceed(t)
	assertContains(t, resource.Stdout, `"kind":"Firmware"`, `"name":"devkit"`)

	delete := h.RunCLI("admin", "firmwares", "delete", "devkit", "--context", "admin-a")
	delete.MustSucceed(t)
	assertContains(t, delete.Stdout, `"name":"devkit"`)
}

func grantFirmwareRead(t *testing.T, h *clitest.Harness, peerContext string, firmwareID string) {
	t.Helper()

	admin := h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	role := "firmware-reader"
	roleResp, err := api.CreateACLRoleWithResponse(ctx, adminservice.ACLRoleUpsert{
		Name:        role,
		Permissions: apitypes.ACLPermissionList{apitypes.ACLPermissionFirmwareRead},
	})
	if err != nil {
		t.Fatalf("create firmware ACL role: %v", err)
	}
	if roleResp.JSON200 == nil {
		t.Fatalf("create firmware ACL role status %d: %s", roleResp.StatusCode(), strings.TrimSpace(string(roleResp.Body)))
	}
	bindingID := fmt.Sprintf("firmware-read-%s-%s", peerContext, firmwareID)
	bindingResp, err := api.CreateACLPolicyBindingWithResponse(ctx, adminservice.ACLPolicyBindingUpsert{
		Id: &bindingID,
		Policy: apitypes.ACLPolicy{
			Subject:  apitypes.ACLSubject{Kind: apitypes.ACLSubjectKindPk, Id: h.ContextPublicKey(peerContext)},
			Resource: apitypes.ACLResource{Kind: apitypes.ACLResourceKindFirmware, Id: firmwareID},
			Role:     role,
		},
	})
	if err != nil {
		t.Fatalf("create firmware ACL binding: %v", err)
	}
	if bindingResp.JSON200 == nil {
		t.Fatalf("create firmware ACL binding status %d: %s", bindingResp.StatusCode(), strings.TrimSpace(string(bindingResp.Body)))
	}
}

func assertDeviceFirmwareRPC(t *testing.T, h *clitest.Harness, contextName string, outputPath string) {
	t.Helper()

	list := mustRunCLIJSON[rpcapi.FirmwareListResponse](t, h, "connect", "firmware", "list", "--context", contextName)
	if len(list.Items) != 1 || list.Items[0].Name != "devkit" {
		t.Fatalf("firmware list = %#v", list)
	}
	get := mustRunCLIJSON[rpcapi.FirmwareGetResponse](t, h, "connect", "firmware", "get", "--firmware-id", "devkit", "--context", contextName)
	if get.Slots.Stable.Version == nil || *get.Slots.Stable.Version != "1.0.0" || get.Slots.Stable.Artifacts == nil || len(*get.Slots.Stable.Artifacts) != 1 {
		t.Fatalf("firmware get = %#v", get)
	}
	download := mustRunCLIJSON[firmwareDownloadCLIResponse](t, h, "connect", "firmware", "download", "--firmware-id", "devkit", "--channel", "stable", "--artifact-name", "main", "--output", outputPath, "--context", contextName)
	if download.Bytes != 20 || download.Metadata.Artifact.Name != "main" {
		t.Fatalf("firmware download = %#v", download)
	}
	payload, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read downloaded firmware: %v", err)
	}
	if string(payload) != "app firmware payload" {
		t.Fatalf("downloaded firmware payload = %q", string(payload))
	}
}

type firmwareDownloadCLIResponse struct {
	Metadata rpcapi.FirmwareDownloadResponse `json:"metadata"`
	Bytes    int64                           `json:"bytes"`
	Output   string                          `json:"output"`
}

func mustRunCLIJSON[T any](t *testing.T, h *clitest.Harness, args ...string) T {
	t.Helper()
	result, err := h.RunCLIUntilSuccess(args...)
	if err != nil {
		t.Fatalf("%v failed: %v\nstdout:\n%s\nstderr:\n%s", args, err, result.Stdout, result.Stderr)
	}
	var out T
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		t.Fatalf("decode %v output: %v\n%s", args, err, result.Stdout)
	}
	return out
}

func assertContains(t *testing.T, output string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(output, value) {
			t.Fatalf("output missing %s:\n%s", value, output)
		}
	}
}
