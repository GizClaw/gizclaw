//go:build gizclaw_e2e

package admin_test

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

const adminAPIAdminContext = "admin-api-admin"

type adminAPIHarness struct {
	ctx          context.Context
	h            *clitest.Harness
	api          *adminhttp.ClientWithResponses
	admin        *gizcli.Client
	adminContext string
	adminKey     string
	adminSN      string
	peerKey      string
	peerSN       string
}

func newAdminAPIHarness(t *testing.T) *adminAPIHarness {
	t.Helper()

	h := clitest.NewSetupHarness(t, "client-admin")
	h.InstallFixedAdminContext(adminAPIAdminContext).MustSucceed(t)
	h.RequireAdminContextEndpoint(adminAPIAdminContext)
	h.CreateContext("admin-api-peer").MustSucceed(t)
	h.RequireClientContextEndpoint("admin-api-peer")
	adminKey := h.ContextPublicKey(adminAPIAdminContext)
	peerKey := h.ContextPublicKey("admin-api-peer")
	adminSN := "admin"
	peerSN := "client-admin-api-peer-" + peerKey
	h.RegisterContext("admin-api-peer", "--sn", peerSN).MustSucceed(t)

	admin := h.ConnectClientFromContext(adminAPIAdminContext)
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin API client: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = api.DeletePeerWithResponse(ctx, peerKey)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return &adminAPIHarness{
		ctx:          ctx,
		h:            h,
		api:          api,
		admin:        admin,
		adminContext: adminAPIAdminContext,
		adminKey:     adminKey,
		adminSN:      adminSN,
		peerKey:      peerKey,
		peerSN:       peerSN,
	}
}

func (h *adminAPIHarness) reconnectAdminAPI(t *testing.T) {
	t.Helper()
	if h.admin != nil {
		_ = h.admin.Close()
	}
	admin := h.h.ConnectClientFromContext(h.adminContext)
	api, err := admin.ServerAdminClient()
	if err != nil {
		_ = admin.Close()
		t.Fatalf("reconnect admin API client: %v", err)
	}
	h.admin = admin
	h.api = api
}

type statusCoder interface {
	StatusCode() int
}

func requireStatusOK(t *testing.T, resp statusCoder, body []byte) {
	t.Helper()
	if resp.StatusCode() == http.StatusOK {
		return
	}
	t.Fatalf("status = %d, want 200: %s", resp.StatusCode(), strings.TrimSpace(string(body)))
}

func requireName[T any](t *testing.T, items []T, want string, name func(T) string) T {
	t.Helper()
	for _, item := range items {
		if name(item) == want {
			return item
		}
	}
	t.Fatalf("missing %q in %d items", want, len(items))
	var zero T
	return zero
}

func hasAdminName[T any](items []T, want string, name func(T) string) bool {
	for _, item := range items {
		if name(item) == want {
			return true
		}
	}
	return false
}

func requirePrefixCount[T any](t *testing.T, items []T, prefix string, min int, name func(T) string) {
	t.Helper()
	count := 0
	for _, item := range items {
		if strings.HasPrefix(name(item), prefix) {
			count++
		}
	}
	if count < min {
		t.Fatalf("items with prefix %q = %d, want >= %d", prefix, count, min)
	}
}

func collectAdminPages[T any](t *testing.T, limit int32, call func(cursor *string, limit int32) ([]T, bool, *string)) []T {
	t.Helper()
	var out []T
	var cursor *string
	for i := 0; i < 20; i++ {
		items, hasNext, nextCursor := call(cursor, limit)
		out = append(out, items...)
		if !hasNext {
			return out
		}
		if nextCursor == nil || *nextCursor == "" {
			t.Fatalf("page %d has_next without next_cursor", i)
		}
		cursor = nextCursor
	}
	t.Fatalf("pagination did not finish")
	return out
}

func collectAdminPagesInt[T any](t *testing.T, limit int, call func(cursor *string, limit int) ([]T, bool, *string)) []T {
	t.Helper()
	var out []T
	var cursor *string
	for i := 0; i < 20; i++ {
		items, hasNext, nextCursor := call(cursor, limit)
		out = append(out, items...)
		if !hasNext {
			return out
		}
		if nextCursor == nil || *nextCursor == "" {
			t.Fatalf("page %d has_next without next_cursor", i)
		}
		cursor = nextCursor
	}
	t.Fatalf("pagination did not finish")
	return out
}

func ptr[T any](value T) *T {
	return &value
}

func openAICredentialBody(t *testing.T, apiKey string) apitypes.CredentialBody {
	t.Helper()
	var body apitypes.CredentialBody
	if err := body.FromOpenAICredentialBody(apitypes.OpenAICredentialBody{ApiKey: ptr(apiKey)}); err != nil {
		t.Fatalf("build OpenAI credential body: %v", err)
	}
	return body
}

func openAIModelProviderData(t *testing.T, upstream string) *apitypes.ModelProviderData {
	t.Helper()
	var body apitypes.ModelProviderData
	if err := body.FromOpenAITenantModelProviderData(apitypes.OpenAITenantModelProviderData{
		UpstreamModel:     ptr(upstream),
		UseSystemRole:     ptr(true),
		SupportJsonOutput: ptr(true),
	}); err != nil {
		t.Fatalf("build OpenAI model provider data: %v", err)
	}
	return &body
}

func flowcraftWorkspaceParameters(t *testing.T, input apitypes.WorkspaceInputMode) *apitypes.WorkspaceParameters {
	t.Helper()
	var params apitypes.WorkspaceParameters
	if err := params.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{
		AgentType:     apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
		GenerateModel: ptr("fake-openai-chat-000"),
		Input:         &input,
	}); err != nil {
		t.Fatalf("build Flowcraft workspace parameters: %v", err)
	}
	return &params
}

func mutationName(base string) string {
	return fmt.Sprintf("e2e-admin-mut-%s", base)
}

func firmwareSlots(description string) apitypes.FirmwareSlots {
	return apitypes.FirmwareSlots{
		Stable: apitypes.FirmwareSlot{
			Description: ptr(description),
		},
	}
}

func adminFirmwareTarPayload(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	modTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for name, body := range files {
		data := []byte(body)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data)), ModTime: modTime}); err != nil {
			t.Fatalf("WriteHeader(%s): %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("Write(%s): %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("Close tar: %v", err)
	}
	return buf.Bytes()
}
