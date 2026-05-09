package ui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	adminui "github.com/GizClaw/gizclaw-go/ui/apps/admin"
	playui "github.com/GizClaw/gizclaw-go/ui/apps/play"
	"github.com/playwright-community/playwright-go"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
	itest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/testutil"
)

const (
	SeedDepotName             = itest.SeedDepotName
	SeedCredentialName        = itest.SeedCredentialName
	SeedMiniMaxTenantName     = itest.SeedMiniMaxTenantName
	SeedVoiceID               = itest.SeedVoiceID
	SeedVolcCredentialName    = itest.SeedVolcCredentialName
	SeedVolcTenantName        = itest.SeedVolcTenantName
	SeedVolcVoiceID           = itest.SeedVolcVoiceID
	SeedWorkspaceTemplateName = itest.SeedWorkspaceTemplateName
	SeedWorkspaceName         = itest.SeedWorkspaceName
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

func FirmwareReleaseTar(t testing.TB, channel, firmwareSemver string) []byte {
	t.Helper()

	releaseTar, err := itest.FirmwareReleaseTarSeed(channel, firmwareSemver)
	if err != nil {
		t.Fatalf("build firmware tar seed: %v", err)
	}
	return releaseTar
}

func DepotInfoJSON(t testing.TB) []byte {
	t.Helper()

	info, err := itest.LoadDepotInfoSeed()
	if err != nil {
		t.Fatalf("load depot info seed: %v", err)
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal depot info seed: %v", err)
	}
	return data
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
	putGearInfo(t, adminClient, h.ContextPublicKey("admin"), adminSeed.Device)

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
	putGearInfo(t, deviceClient, h.ContextPublicKey("device-a"), deviceSeed.Device)
	putGearInfo(t, actionDeviceClient, h.ContextPublicKey("device-actions-a"), deviceSeed.Device)
	putGearInfo(t, deleteDeviceClient, h.ContextPublicKey("device-delete-a"), deviceSeed.Device)

	seedCtx, cancel := context.WithTimeout(context.Background(), itest.ReadyTimeout)
	defer cancel()
	if err := itest.ApplyAdminCatalogSeed(seedCtx, adminAPI); err != nil {
		t.Fatalf("apply admin catalog seed: %v", err)
	}
	if err := itest.ApplyWorkspaceSeed(seedCtx, adminAPI); err != nil {
		t.Fatalf("apply workspace seed: %v", err)
	}
	if err := itest.ApplyFirmwareSeed(seedCtx, adminAPI); err != nil {
		t.Fatalf("apply firmware seed: %v", err)
	}
	applyFirmwareReleaseSeed(t, seedCtx, adminAPI, "beta", "1.0.1")
	for _, publicKey := range []string{
		h.ContextPublicKey("device-a"),
		h.ContextPublicKey("device-delete-a"),
	} {
		approveGear(t, seedCtx, adminAPI, publicKey)
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

	return Seed{
		AdminURL:              startTestUI(t, "admin", adminClient, adminui.FS()),
		PlayURL:               startTestUI(t, "play", deviceClient, playui.FS()),
		ErrorPlayURL:          startErrorTestUI(t, "play-error", playui.FS()),
		DevicePublicKey:       h.ContextPublicKey("device-a"),
		ActionDevicePublicKey: h.ContextPublicKey("device-actions-a"),
		DeleteDevicePublicKey: h.ContextPublicKey("device-delete-a"),
	}
}

func putGearInfo(t testing.TB, client *gizclaw.Client, publicKey string, info apitypes.DeviceInfo) {
	t.Helper()

	req, err := convertUIAPIType[rpcapi.GearPutInfoRequest](info)
	if err != nil {
		t.Fatalf("convert gear info %q: %v", publicKey, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), itest.ReadyTimeout)
	defer cancel()
	resp, err := client.PutGearInfo(ctx, "gear.info.put", req)
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

func approveGear(t testing.TB, ctx context.Context, api *adminservice.ClientWithResponses, publicKey string) {
	t.Helper()

	resp, err := api.ApproveGearWithResponse(ctx, publicKey, adminservice.ApproveRequest{Role: apitypes.GearRoleGear})
	if err != nil {
		t.Fatalf("approve %q: %v", publicKey, err)
	}
	if resp.JSON200 != nil {
		return
	}
	t.Fatalf("approve %q got status %d: %s", publicKey, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
}

func applyFirmwareReleaseSeed(t testing.TB, ctx context.Context, api *adminservice.ClientWithResponses, channel, firmwareSemver string) {
	t.Helper()

	releaseTar, err := itest.FirmwareReleaseTarSeed(channel, firmwareSemver)
	if err != nil {
		t.Fatalf("build %s firmware seed: %v", channel, err)
	}
	resp, err := api.PutChannelWithBodyWithResponse(ctx, SeedDepotName, channel, "application/octet-stream", bytes.NewReader(releaseTar))
	if err != nil {
		t.Fatalf("put %s firmware seed: %v", channel, err)
	}
	if resp.JSON200 == nil {
		t.Fatalf("put %s firmware seed got status %d: %s", channel, resp.StatusCode(), strings.TrimSpace(string(resp.Body)))
	}
}

func startTestUI(t testing.TB, name string, client *gizclaw.Client, uiFS fs.FS) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("/api/", client.ProxyHandler())
	mux.Handle("/api", client.ProxyHandler())
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
	mux.Handle("/", staticWithSPAFallback(uiFS))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Logf("%s test UI listening on %s", name, server.URL)
	return server.URL
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
