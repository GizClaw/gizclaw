package ui_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	adminui "github.com/GizClaw/gizclaw-go/cmd/ui/admin"
	playui "github.com/GizClaw/gizclaw-go/cmd/ui/play"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
	"github.com/pion/webrtc/v4"
	"github.com/playwright-community/playwright-go"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
	itest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/testutil"
)

const (
	SeedCredentialName      = itest.SeedCredentialName
	SeedOpenAITenantName    = itest.SeedOpenAITenantName
	SeedGeminiTenantName    = itest.SeedGeminiTenantName
	SeedDashScopeTenantName = itest.SeedDashScopeTenantName
	SeedModelID             = itest.SeedModelID
	SeedFirmwareName        = itest.SeedFirmwareName
	SeedACLViewName         = itest.SeedACLViewName
	SeedMiniMaxTenantName   = itest.SeedMiniMaxTenantName
	SeedVoiceID             = itest.SeedVoiceID
	SeedVolcCredentialName  = itest.SeedVolcCredentialName
	SeedVolcTenantName      = itest.SeedVolcTenantName
	SeedVolcVoiceID         = itest.SeedVolcVoiceID
	SeedWorkflowName        = itest.SeedWorkflowName
	SeedWorkspaceName       = itest.SeedWorkspaceName
	SeedAltWorkspaceName    = "ui-alt-workspace"
)

type Story struct {
	Name string
	Run  func(testing.TB, *Page)
}

type Seed struct {
	AdminURL              string
	PlayURL               string
	ErrorPlayURL          string
	DevicePublicKey       string
	ActionDevicePublicKey string
	DeleteDevicePublicKey string
}

type Page struct {
	t    testing.TB
	page playwright.Page
	Seed Seed
}

type Suite struct {
	t       testing.TB
	seed    Seed
	runner  *browserRunner
	context playwright.BrowserContext
}

type browserRunner struct {
	browser playwright.Browser
	pw      *playwright.Playwright
}

func RunStories(t *testing.T, stories []Story) {
	t.Helper()

	suite := NewSuite(t)
	defer suite.Close()

	for _, story := range stories {
		story := story
		t.Run(story.Name, func(t *testing.T) {
			suite.RunStory(t, story.Run)
		})
	}
}

func NewSuite(t testing.TB) *Suite {
	t.Helper()

	seed := startSeededUI(t)
	runner := newBrowserRunner(t)
	ctx, err := runner.browser.NewContext()
	if err != nil {
		runner.close(t)
		t.Fatalf("create browser context: %v", err)
	}
	return &Suite{t: t, seed: seed, runner: runner, context: ctx}
}

func (s *Suite) RunStory(t testing.TB, run func(testing.TB, *Page)) {
	t.Helper()

	page, err := s.context.NewPage()
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	defer page.Close()
	page.OnConsole(func(message playwright.ConsoleMessage) {
		if message.Type() == "error" {
			t.Logf("browser console error: %s", message.Text())
		}
	})
	page.OnPageError(func(err error) {
		t.Logf("browser page error: %v", err)
	})

	run(t, &Page{t: t, page: page, Seed: s.seed})
}

func (s *Suite) Close() {
	s.t.Helper()
	if err := s.context.Close(); err != nil {
		s.t.Fatalf("close browser context: %v", err)
	}
	s.runner.close(s.t)
}

func (p *Page) GotoAdmin(routePath string) {
	p.gotoURL(p.Seed.AdminURL, routePath)
}

func (p *Page) GotoPlay(routePath string) {
	p.gotoURL(p.Seed.PlayURL, routePath)
}

func (p *Page) GotoErrorPlay(routePath string) {
	p.gotoURL(p.Seed.ErrorPlayURL, routePath)
}

func (p *Page) ExpectURLSuffix(suffix string) {
	p.t.Helper()
	if err := itest.WaitUntil(10*time.Second, func() error {
		current := p.page.URL()
		if strings.HasSuffix(current, suffix) {
			return nil
		}
		return fmt.Errorf("url %q does not end with %q", current, suffix)
	}); err != nil {
		p.t.Fatal(err)
	}
}

func (p *Page) ExpectText(text string) {
	p.t.Helper()
	if err := itest.WaitUntil(10*time.Second, func() error {
		body, err := p.page.TextContent("body")
		if err != nil {
			return err
		}
		if strings.Contains(body, text) {
			return nil
		}
		return fmt.Errorf("page body does not contain %q; body=%q", text, body)
	}); err != nil {
		p.t.Fatal(err)
	}
}

func (p *Page) ExpectNoText(text string) {
	p.t.Helper()
	body, err := p.page.TextContent("body")
	if err != nil {
		p.t.Fatalf("read page body: %v", err)
	}
	if strings.Contains(body, text) {
		p.t.Fatalf("page body contains %q; body=%q", text, body)
	}
}

func (p *Page) Fill(selector, value string) {
	p.t.Helper()
	if err := p.page.Locator(selector).Fill(value); err != nil {
		p.t.Fatalf("fill %q: %v", selector, err)
	}
}

func (p *Page) FillNth(selector string, index int, value string) {
	p.t.Helper()
	if err := p.page.Locator(selector).Nth(index).Fill(value); err != nil {
		p.t.Fatalf("fill %q nth=%d: %v", selector, index, err)
	}
}

func (p *Page) ClickRole(role, name string) {
	p.t.Helper()
	if err := p.page.GetByRole(playwright.AriaRole(role), playwright.PageGetByRoleOptions{
		Name:  name,
		Exact: playwright.Bool(true),
	}).Click(); err != nil {
		p.t.Fatalf("click role=%s name=%q: %v", role, name, err)
	}
}

func (p *Page) ClickRoleLike(role, name string) {
	p.t.Helper()
	if err := p.page.GetByRole(playwright.AriaRole(role), playwright.PageGetByRoleOptions{
		Name: name,
	}).Click(); err != nil {
		p.t.Fatalf("click role=%s name~=%q: %v", role, name, err)
	}
}

func (p *Page) ClickSelector(selector string) {
	p.t.Helper()
	if err := p.page.Locator(selector).Click(); err != nil {
		p.t.Fatalf("click selector=%q: %v", selector, err)
	}
}

func (p *Page) ClickNavigationLink(name string) {
	p.t.Helper()
	err := p.page.GetByRole(playwright.AriaRole("navigation")).GetByRole(playwright.AriaRole("link"), playwright.LocatorGetByRoleOptions{
		Name:  name,
		Exact: playwright.Bool(true),
	}).Click()
	if err != nil {
		p.t.Fatalf("click navigation link %q: %v", name, err)
	}
}

func (p *Page) SetInputFiles(index int, name, mimeType string, data []byte) {
	p.t.Helper()
	err := p.page.Locator(`input[type="file"]`).Nth(index).SetInputFiles([]playwright.InputFile{{
		Name:     name,
		MimeType: mimeType,
		Buffer:   data,
	}})
	if err != nil {
		p.t.Fatalf("set input file %d: %v", index, err)
	}
}

func (p *Page) gotoURL(baseURL, routePath string) {
	p.t.Helper()
	target := joinURL(p.t, baseURL, routePath)
	if _, err := p.page.Goto(target); err != nil {
		p.t.Fatalf("goto %s: %v", target, err)
	}
}

func startSeededUI(t testing.TB) Seed {
	t.Helper()

	h := clitest.NewHarness(t, "200-server-config-boot")
	h.StartServerFromFixture("server_config.yaml")

	h.CreateContext("admin").MustSucceed(t)
	adminClient := h.ConnectClientFromContext("admin")
	t.Cleanup(func() { _ = adminClient.Close() })

	adminSeed, err := itest.LoadRegistrationSeed("admin")
	if err != nil {
		t.Fatalf("load admin registration seed: %v", err)
	}
	putPeerInfo(t, adminClient, h.ContextPublicKey("admin"), adminSeed.Device)

	adminAPI, err := adminClient.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin API client for seeded UI service: %v", err)
	}

	h.CreateContext("device-a").MustSucceed(t)
	h.CreateContext("device-actions-a").MustSucceed(t)
	h.CreateContext("device-delete-a").MustSucceed(t)
	deviceClient := h.ConnectClientFromContext("device-a")
	t.Cleanup(func() { _ = deviceClient.Close() })
	actionDeviceClient := h.ConnectClientFromContext("device-actions-a")
	t.Cleanup(func() { _ = actionDeviceClient.Close() })
	deleteDeviceClient := h.ConnectClientFromContext("device-delete-a")
	t.Cleanup(func() { _ = deleteDeviceClient.Close() })

	deviceSeed, err := itest.LoadRegistrationSeed("device")
	if err != nil {
		t.Fatalf("load device registration seed: %v", err)
	}
	putPeerInfo(t, deviceClient, h.ContextPublicKey("device-a"), deviceSeed.Device)
	putPeerInfo(t, actionDeviceClient, h.ContextPublicKey("device-actions-a"), deviceSeed.Device)
	putPeerInfo(t, deleteDeviceClient, h.ContextPublicKey("device-delete-a"), deviceSeed.Device)

	seedCtx, cancel := context.WithTimeout(context.Background(), itest.ReadyTimeout)
	defer cancel()
	if err := itest.ApplyAdminCatalogSeed(seedCtx, adminAPI); err != nil {
		t.Fatalf("apply admin catalog seed: %v", err)
	}
	if err := itest.ApplyWorkspaceSeed(seedCtx, adminAPI); err != nil {
		t.Fatalf("apply workspace seed: %v", err)
	}
	for _, publicKey := range []string{
		h.ContextPublicKey("device-a"),
		h.ContextPublicKey("device-delete-a"),
	} {
		approvePeerSeed(t, seedCtx, adminAPI, publicKey)
	}
	for _, publicKey := range []string{
		h.ContextPublicKey("device-a"),
		h.ContextPublicKey("device-actions-a"),
		h.ContextPublicKey("device-delete-a"),
	} {
		if err := itest.ApplyDeviceConfigSeed(seedCtx, adminAPI, publicKey); err != nil {
			t.Fatalf("apply device config seed for %q: %v", publicKey, err)
		}
	}
	seedPlayResourceACL(t, seedCtx, adminAPI, h.ContextPublicKey("device-a"))

	return Seed{
		AdminURL:              startTestUI(t, "admin", adminClient, adminui.FS()),
		PlayURL:               startTestUI(t, "play", deviceClient, playui.FS()),
		ErrorPlayURL:          startErrorTestUI(t, "play-error", playui.FS()),
		DevicePublicKey:       h.ContextPublicKey("device-a"),
		ActionDevicePublicKey: h.ContextPublicKey("device-actions-a"),
		DeleteDevicePublicKey: h.ContextPublicKey("device-delete-a"),
	}
}

func seedPlayResourceACL(t testing.TB, ctx context.Context, api *adminservice.ClientWithResponses, publicKey string) {
	t.Helper()

	const roleName = "ui-play-openai-user"
	roleResp, err := api.PutACLRoleWithResponse(ctx, roleName, adminservice.ACLRoleUpsert{
		Name: roleName,
		Permissions: apitypes.ACLPermissionList{
			apitypes.ACLPermissionModelRead,
			apitypes.ACLPermissionModelUse,
			apitypes.ACLPermissionCredentialRead,
			apitypes.ACLPermissionCredentialUse,
			apitypes.ACLPermissionVoiceRead,
			apitypes.ACLPermissionVoiceUse,
			apitypes.ACLPermissionWorkflowRead,
			apitypes.ACLPermissionWorkflowUse,
			apitypes.ACLPermissionWorkspaceRead,
			apitypes.ACLPermissionWorkspaceUse,
		},
	})
	if err != nil {
		t.Fatalf("put play ACL role: %v", err)
	}
	if roleResp.JSON200 == nil {
		t.Fatalf("put play ACL role status %d: %s", roleResp.StatusCode(), strings.TrimSpace(string(roleResp.Body)))
	}

	subject := apitypes.ACLSubject{Kind: apitypes.ACLSubjectKindPk, Id: publicKey}
	for _, resource := range []apitypes.ACLResource{
		{Kind: apitypes.ACLResourceKindModel, Id: SeedModelID},
		{Kind: apitypes.ACLResourceKindCredential, Id: "ui-seed-openai-credential"},
		{Kind: apitypes.ACLResourceKindVoice, Id: SeedVoiceID},
		{Kind: apitypes.ACLResourceKindWorkflow, Id: SeedWorkflowName},
		{Kind: apitypes.ACLResourceKindWorkspace, Id: SeedWorkspaceName},
	} {
		id := fmt.Sprintf("ui-play-%s-%s", resource.Kind, strings.NewReplacer(":", "-", "/", "-").Replace(resource.Id))
		resp, err := api.PutACLPolicyBindingWithResponse(ctx, id, adminservice.ACLPolicyBindingUpsert{
			Id: &id,
			Policy: apitypes.ACLPolicy{
				Subject:  subject,
				Resource: resource,
				Role:     roleName,
			},
		})
		if err != nil {
			t.Fatalf("put play ACL policy binding %s: %v", id, err)
		}
		if resp.JSON200 == nil {
			t.Fatalf("put play ACL policy binding %s status %d: %s", id, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
		}
	}
}

func putPeerInfo(t testing.TB, client *gizcli.Client, publicKey string, info apitypes.DeviceInfo) {
	t.Helper()

	req, err := convertUIAPIType[rpcapi.ServerPutInfoRequest](info)
	if err != nil {
		t.Fatalf("convert peer info %q: %v", publicKey, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), itest.ReadyTimeout)
	defer cancel()
	resp, err := client.PutServerInfo(ctx, "server.info.put", req)
	if err != nil {
		t.Fatalf("put info %q: %v", publicKey, err)
	}
	if resp != nil {
		return
	}
	t.Fatalf("put info %q got nil response", publicKey)
}

func convertUIAPIType[T any](value any) (T, error) {
	var out T
	data, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func approvePeerSeed(t testing.TB, ctx context.Context, api *adminservice.ClientWithResponses, publicKey string) {
	t.Helper()

	resp, err := api.ApprovePeerWithResponse(ctx, publicKey, adminservice.ApproveRequest{Role: apitypes.PeerRoleClient})
	if err != nil {
		t.Fatalf("approve %q: %v", publicKey, err)
	}
	if resp.JSON200 != nil {
		return
	}
	t.Fatalf("approve %q got status %d: %s", publicKey, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
}

func startTestUI(t testing.TB, name string, client *gizcli.Client, uiFS fs.FS) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("/api/", client.ProxyHandler())
	mux.Handle("/api", client.ProxyHandler())
	mux.Handle("/v1/", client.ProxyHandler())
	mux.Handle("/v1", client.ProxyHandler())
	if strings.HasPrefix(name, "play") {
		registerTestPlayRoutes(mux, client)
	}
	mux.Handle("/", staticWithSPAFallback(uiFS))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Logf("%s test UI listening on %s", name, server.URL)
	return server.URL
}

func startErrorTestUI(t testing.TB, name string, uiFS fs.FS) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no gizclaw client configured for error scenario", http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no gizclaw client configured for error scenario", http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/v1/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no gizclaw client configured for error scenario", http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/v1", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no gizclaw client configured for error scenario", http.StatusServiceUnavailable)
	})
	registerTestPlayErrorRoutes(mux)
	mux.Handle("/", staticWithSPAFallback(uiFS))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Logf("%s test UI listening on %s", name, server.URL)
	return server.URL
}

type testPlayVoiceListResponse struct {
	Data       []json.RawMessage `json:"data"`
	HasNext    bool              `json:"has_next"`
	Items      []json.RawMessage `json:"items"`
	NextCursor string            `json:"next_cursor"`
}

type testPlayVoiceStreamEvent struct {
	Done  bool            `json:"done,omitempty"`
	Error string          `json:"error,omitempty"`
	Voice json.RawMessage `json:"voice,omitempty"`
}

type testPlayWorkspaceSetRequest struct {
	WorkspaceName string `json:"workspace_name"`
}

type testPlayHistoryPlayRequest struct {
	HistoryID string `json:"history_id"`
}

func registerTestPlayRoutes(mux *http.ServeMux, client *gizcli.Client) {
	mux.HandleFunc("/peer-resources", handleTestPlayResourceCatalog)
	mux.HandleFunc("/peer-resources/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/peer-resources/"), "/"), "/")
		if len(parts) != 1 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		switch parts[0] {
		case "models":
			handleTestPlayModels(w, r, client)
		case "credentials":
			handleTestPlayCredentials(w, r, client)
		case "voices":
			proxyTestPlayOpenAI(w, r, client, "/v1/voices")
		case "workspaces":
			handleTestPlayWorkspaces(w, r, client)
		case "pets":
			handleTestPlayPets(w, r)
		case "wallet":
			handleTestPlayWallet(w, r)
		case "wallet-transactions":
			handleTestPlayWalletTransactions(w, r)
		case "rewards":
			handleTestPlayRewards(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/play/voices/stream", func(w http.ResponseWriter, r *http.Request) {
		handleTestPlayVoiceStream(w, r, client)
	})
	registerTestPlayWorkspaceRuntimeRoutes(mux)
	registerTestPlayWebRTCRoute(mux, client)
}

func registerTestPlayErrorRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/peer-resources", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no gizclaw client configured for error scenario", http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/peer-resources/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no gizclaw client configured for error scenario", http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/play/voices/stream", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeTestPlayVoiceStreamEvent(w, testPlayVoiceStreamEvent{Error: "no gizclaw client configured for error scenario"})
	})
	mux.HandleFunc("/peer-run/workspace", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no gizclaw client configured for error scenario", http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/peer-run/workspace/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no gizclaw client configured for error scenario", http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/webrtc/offer", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no gizclaw client configured for error scenario", http.StatusServiceUnavailable)
	})
}

func handleTestPlayResourceCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	writeTestPlayJSON(w, map[string][]string{"resources": {"workspaces", "workflows", "models", "credentials", "voices", "pets", "wallet", "wallet-transactions", "rewards"}})
}

func handleTestPlayModels(w http.ResponseWriter, r *http.Request, client *gizcli.Client) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	result, err := client.ListModels(r.Context(), testPlayRPCID(), rpcapi.ModelListRequest{
		Cursor: testQueryStringPtr(r, "cursor"),
		Limit:  testQueryIntPtr(r, "limit", 20, 100),
	})
	writeTestPlayRPCResult(w, result, err)
}

func handleTestPlayCredentials(w http.ResponseWriter, r *http.Request, client *gizcli.Client) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	result, err := client.ListCredentials(r.Context(), testPlayRPCID(), rpcapi.CredentialListRequest{
		Cursor: testQueryStringPtr(r, "cursor"),
		Limit:  testQueryIntPtr(r, "limit", 20, 100),
	})
	if result != nil {
		out := *result
		out.Items = append([]rpcapi.Credential(nil), result.Items...)
		for i := range out.Items {
			out.Items[i].Body = rpcapi.CredentialBody{}
		}
		result = &out
	}
	writeTestPlayRPCResult(w, result, err)
}

func handleTestPlayWorkspaces(w http.ResponseWriter, r *http.Request, client *gizcli.Client) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	result, err := client.ListWorkspaces(r.Context(), testPlayRPCID(), rpcapi.WorkspaceListRequest{
		Cursor: testQueryStringPtr(r, "cursor"),
		Limit:  testQueryIntPtr(r, "limit", 20, 100),
	})
	if result != nil {
		out := *result
		out.Items = append([]rpcapi.Workspace(nil), result.Items...)
		out.Items = append(out.Items, rpcapi.Workspace{
			Name:         SeedAltWorkspaceName,
			WorkflowName: SeedWorkflowName,
			CreatedAt:    time.Date(2026, 6, 18, 2, 2, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2026, 6, 18, 2, 2, 0, 0, time.UTC),
		})
		result = &out
	}
	writeTestPlayRPCResult(w, result, err)
}

func registerTestPlayWorkspaceRuntimeRoutes(mux *http.ServeMux) {
	state := struct {
		sync.Mutex
		active  string
		pending string
	}{active: SeedWorkspaceName}
	mux.HandleFunc("/peer-run/workspace", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			state.Lock()
			writeTestPlayWorkspaceState(w, state.active, state.pending)
			state.Unlock()
		case http.MethodPut:
			var req testPlayWorkspaceSetRequest
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
				http.Error(w, "invalid workspace set json", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(req.WorkspaceName) == "" {
				http.Error(w, "workspace_name is required", http.StatusBadRequest)
				return
			}
			state.Lock()
			state.pending = strings.TrimSpace(req.WorkspaceName)
			writeTestPlayWorkspaceState(w, state.active, state.pending)
			state.Unlock()
		default:
			w.Header().Set("Allow", "GET, PUT")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/peer-run/workspace/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		state.Lock()
		if state.pending != "" {
			state.active = state.pending
			state.pending = ""
		}
		writeTestPlayWorkspaceState(w, state.active, state.pending)
		state.Unlock()
	})
	mux.HandleFunc("/peer-run/workspace/history", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		writeTestPlayJSON(w, map[string]any{
			"items": []map[string]any{{
				"id":               "ui-history-1",
				"type":             "agent",
				"name":             "assistant",
				"text":             "北京今天适合轻松散步，也可以看看天气再决定。",
				"created_at":       "2026-06-18T02:00:00Z",
				"replay_available": true,
			}},
			"has_next": false,
		})
	})
	mux.HandleFunc("/peer-run/workspace/history/play", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		var req testPlayHistoryPlayRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "invalid history play json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.HistoryID) == "" {
			http.Error(w, "history_id is required", http.StatusBadRequest)
			return
		}
		writeTestPlayJSON(w, map[string]any{"accepted": true, "state": "playing", "history_id": req.HistoryID})
	})
	mux.HandleFunc("/peer-run/workspace/memory/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		writeTestPlayJSON(w, map[string]any{
			"available":        true,
			"enabled":          true,
			"item_count":       2416,
			"storage_bytes":    987654,
			"embedding_status": "disabled",
			"index_status":     "ready",
			"updated_at":       "2026-06-18T02:01:00Z",
			"backend":          "flowcraft-memory",
		})
	})
	mux.HandleFunc("/peer-run/workspace/recall", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		writeTestPlayJSON(w, map[string]any{
			"hits": []map[string]any{{
				"id":        "ui-recall-1",
				"score":     0.91,
				"source_id": "ui-history-1",
				"snippet":   "北京出行建议：先看天气，再选择轻松散步。",
				"timestamp": "2026-06-18T02:00:00Z",
				"metadata": map[string]any{
					"lane": "preference",
				},
			}},
		})
	})
}

func writeTestPlayWorkspaceState(w http.ResponseWriter, activeWorkspace, pendingWorkspace string) {
	selectedWorkspace := activeWorkspace
	if pendingWorkspace != "" {
		selectedWorkspace = pendingWorkspace
	}
	writeTestPlayJSON(w, map[string]any{
		"active_workspace_name":  activeWorkspace,
		"pending_workspace_name": pendingWorkspace,
		"workspace_name":         selectedWorkspace,
		"workflow_name":          SeedWorkflowName,
		"agent_type":             "flowcraft",
		"runtime_state":          "active",
	})
}

func handleTestPlayPets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	writeTestPlayJSON(w, map[string]any{
		"items": []map[string]any{{
			"id":         "ui-pet-1",
			"name":       "Seeded Pet",
			"species_id": "rabbit",
			"voice_id":   SeedVoiceID,
			"life": map[string]int{
				"satiety":     80,
				"cleanliness": 70,
				"mood":        90,
				"energy":      65,
				"health":      95,
			},
			"ability": map[string]int{
				"level":        1,
				"exp":          10,
				"charm":        5,
				"intelligence": 6,
				"stamina":      7,
				"luck":         8,
			},
			"created_at": "2026-06-01T00:00:00Z",
			"updated_at": "2026-06-01T00:01:00Z",
		}},
		"has_next": false,
	})
}

func handleTestPlayWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	writeTestPlayJSON(w, map[string]any{
		"id":            "wallet-ui-seed",
		"token_balance": 12,
		"point_balance": 345,
		"created_at":    "2026-06-01T00:00:00Z",
		"updated_at":    "2026-06-01T00:02:00Z",
	})
}

func handleTestPlayWalletTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	writeTestPlayJSON(w, map[string]any{
		"items": []map[string]any{{
			"id":          "ui-wallet-tx-1",
			"token_delta": -1,
			"point_delta": -10,
			"reason":      "pet_adopt",
			"created_at":  "2026-06-01T00:03:00Z",
		}},
		"has_next": false,
	})
}

func handleTestPlayRewards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	writeTestPlayJSON(w, map[string]any{
		"items": []map[string]any{{
			"id":           "ui-reward-1",
			"prompt":       "Seeded reward",
			"badge_id":     "founder",
			"point_amount": 9,
			"created_at":   "2026-06-01T00:04:00Z",
		}},
		"has_next": false,
	})
}

func handleTestPlayVoiceStream(w http.ResponseWriter, r *http.Request, client *gizcli.Client) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")

	list, err := fetchTestPlayVoicePage(r, client)
	if err != nil {
		writeTestPlayVoiceStreamEvent(w, testPlayVoiceStreamEvent{Error: err.Error()})
		return
	}
	providerKind := strings.TrimSpace(r.URL.Query().Get("provider_kind"))
	providerName := strings.TrimSpace(r.URL.Query().Get("provider_name"))
	for _, raw := range append(list.Data, list.Items...) {
		if !testPlayVoiceMatches(raw, providerKind, providerName) {
			continue
		}
		writeTestPlayVoiceStreamEvent(w, testPlayVoiceStreamEvent{Voice: raw})
	}
	writeTestPlayVoiceStreamEvent(w, testPlayVoiceStreamEvent{Done: true})
}

func fetchTestPlayVoicePage(r *http.Request, client *gizcli.Client) (testPlayVoiceListResponse, error) {
	rec := httptest.NewRecorder()
	proxyTestPlayOpenAI(rec, r, client, "/v1/voices")
	if rec.Code < 200 || rec.Code >= 300 {
		return testPlayVoiceListResponse{}, fmt.Errorf("list voices failed: HTTP %d %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	var out testPlayVoiceListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		return testPlayVoiceListResponse{}, err
	}
	return out, nil
}

func proxyTestPlayOpenAI(w http.ResponseWriter, r *http.Request, client *gizcli.Client, targetPath string) {
	cloned := r.Clone(r.Context())
	u := *cloned.URL
	u.Path = targetPath
	u.RawPath = ""
	cloned.URL = &u
	client.ProxyHandler().ServeHTTP(w, cloned)
}

func testPlayVoiceMatches(raw json.RawMessage, providerKind, providerName string) bool {
	if providerKind == "" && providerName == "" {
		return true
	}
	var view struct {
		Provider struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(raw, &view); err != nil {
		return false
	}
	if providerKind != "" && view.Provider.Kind != providerKind {
		return false
	}
	if providerName != "" && view.Provider.Name != providerName {
		return false
	}
	return true
}

func writeTestPlayVoiceStreamEvent(w io.Writer, event testPlayVoiceStreamEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		data = []byte(`{"error":"encode voice stream event failed"}`)
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

func writeTestPlayRPCResult(w http.ResponseWriter, result any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeTestPlayJSON(w, result)
}

func writeTestPlayJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func testPlayRPCID() string {
	return "play-ui-" + fmt.Sprint(time.Now().UnixNano())
}

func testQueryStringPtr(r *http.Request, name string) *string {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return nil
	}
	return &value
}

func testQueryIntPtr(r *http.Request, name string, fallback, max int) *int {
	value := fallback
	if raw := strings.TrimSpace(r.URL.Query().Get(name)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			value = parsed
		}
	}
	if value > max {
		value = max
	}
	return &value
}

type testPlayWebRTCOfferRequest struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

type testPlayWebRTCAnswerResponse struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

func registerTestPlayWebRTCRoute(mux *http.ServeMux, client *gizcli.Client) {
	mux.HandleFunc("/webrtc/offer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		handleTestPlayWebRTCOffer(w, r, client)
	})
}

func handleTestPlayWebRTCOffer(w http.ResponseWriter, r *http.Request, client *gizcli.Client) {
	var req testPlayWebRTCOfferRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid offer json", http.StatusBadRequest)
		return
	}
	if req.Type != webrtc.SDPTypeOffer.String() || strings.TrimSpace(req.SDP) == "" {
		http.Error(w, "invalid webrtc offer", http.StatusBadRequest)
		return
	}
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	registration, err := client.RegisterTo(pc)
	if err != nil {
		_ = pc.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateClosed:
			_ = registration.Close()
			_ = pc.Close()
		}
	})
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: req.SDP}); err != nil {
		_ = registration.Close()
		_ = pc.Close()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = registration.Close()
		_ = pc.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = registration.Close()
		_ = pc.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	select {
	case <-gatherComplete:
	case <-r.Context().Done():
		_ = registration.Close()
		_ = pc.Close()
		return
	}
	local := pc.LocalDescription()
	if local == nil {
		_ = registration.Close()
		_ = pc.Close()
		http.Error(w, "missing local description", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(testPlayWebRTCAnswerResponse{SDP: local.SDP, Type: local.Type.String()})
}

func staticWithSPAFallback(uiFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(uiFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if clean != "" {
			info, err := fs.Stat(uiFS, clean)
			if err == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		index, err := uiFS.Open("index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		defer index.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.Copy(w, index)
	})
}

func newBrowserRunner(t testing.TB) *browserRunner {
	t.Helper()

	options := &playwright.RunOptions{
		Browsers:         []string{"chromium"},
		OnlyInstallShell: true,
		Stdout:           io.Discard,
		Stderr:           io.Discard,
	}
	pw, err := playwright.Run(options)
	if err != nil {
		t.Fatalf("start Playwright: %v\nInstall Playwright for Go explicitly before running UI tests.", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		_ = pw.Stop()
		t.Fatalf("launch Chromium: %v", err)
	}
	return &browserRunner{browser: browser, pw: pw}
}

func (r *browserRunner) close(t testing.TB) {
	t.Helper()
	if err := r.browser.Close(); err != nil {
		t.Fatalf("close browser: %v", err)
	}
	if err := r.pw.Stop(); err != nil {
		t.Fatalf("stop Playwright: %v", err)
	}
}

func joinURL(t testing.TB, baseURL, routePath string) string {
	t.Helper()

	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse base URL %q: %v", baseURL, err)
	}
	parsed.Path = routePath
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
