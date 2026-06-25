//go:build gizclaw_e2e

package harness

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/playwright-community/playwright-go"
)

const (
	SeedCredentialName      = "minimax-main-credential"
	SeedOpenAITenantName    = "openai-lab"
	SeedGeminiTenantName    = "gemini-main"
	SeedDashScopeTenantName = "dashscope-main"
	SeedModelID             = "openai-lab-chat"
	SeedFirmwareName        = "devkit-voice-firmware"
	SeedACLViewName         = "under-12"
	SeedMiniMaxTenantName   = "minimax-main"
	SeedVoiceID             = "minimax-cloned-narrator"
	SeedVolcCredentialName  = "volc-lab-credential"
	SeedVolcTenantName      = "volc-lab"
	SeedVolcVoiceID         = "volc-tenant:volc-lab:ICL_demo_voice"
	SeedWorkflowName        = "flowcraft-assistant"
	SeedWorkspaceName       = "workspace-flowcraft-assistant"
	SeedAltWorkspaceName    = "workspace-flowcraft-alt"
	pollInterval            = 20 * time.Millisecond
)

const (
	defaultClientConfigHome = "test/gizclaw-e2e/testdata/gizclaw-config-home"
	defaultClientContext    = "e2e-client"
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

func RunAdminStories(t *testing.T, stories []Story) {
	t.Helper()
	RunStories(t, requireAdminSeed(t), stories)
}

func RunPlayStories(t *testing.T, stories []Story) {
	t.Helper()
	RunStories(t, requirePlaySeed(t), stories)
}

func RunSmokeStories(t *testing.T, stories []Story) {
	t.Helper()
	RunStories(t, requireSmokeSeed(t), stories)
}

func RunStories(t *testing.T, seed Seed, stories []Story) {
	t.Helper()

	suite := NewSuite(t, seed)
	defer suite.Close()

	for _, story := range stories {
		story := story
		t.Run(story.Name, func(t *testing.T) {
			suite.RunStory(t, story.Run)
		})
	}
}

func NewSuite(t testing.TB, seed Seed) *Suite {
	t.Helper()

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
	defer captureStoryFailure(t, page)
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

func captureStoryFailure(t testing.TB, page playwright.Page) {
	t.Helper()
	if !t.Failed() {
		return
	}
	currentURL := page.URL()
	body, err := page.TextContent("body")
	if err != nil {
		t.Logf("browser failure url=%s body_read_error=%v", currentURL, err)
	} else {
		t.Logf("browser failure url=%s body=%q", currentURL, truncateForLog(body, 4000))
	}

	artifactDir := getenvDefault("GIZCLAW_E2E_UI_ARTIFACT_DIR", filepath.Join(os.TempDir(), "gizclaw-e2e-ui-artifacts"))
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Logf("browser failure screenshot mkdir %s: %v", artifactDir, err)
		return
	}
	screenshotPath := filepath.Join(artifactDir, safeArtifactName(t.Name())+".png")
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(screenshotPath),
		FullPage: playwright.Bool(true),
	}); err != nil {
		t.Logf("browser failure screenshot %s: %v", screenshotPath, err)
		return
	}
	t.Logf("browser failure screenshot=%s", screenshotPath)
}

func (s *Suite) Close() {
	s.t.Helper()
	if err := s.context.Close(); err != nil {
		s.t.Fatalf("close browser context: %v", err)
	}
	s.runner.close(s.t)
}

func (p *Page) Raw() playwright.Page {
	return p.page
}

func (p *Page) GotoAdmin(routePath string) {
	p.gotoURL(p.Seed.AdminURL, routePath)
}

func (p *Page) GotoPlay(routePath string) {
	p.gotoURL(p.Seed.PlayURL, routePath)
}

func (p *Page) GotoErrorPlay(routePath string) {
	if strings.TrimSpace(p.Seed.ErrorPlayURL) == "" {
		p.t.Skip("set GIZCLAW_E2E_ERROR_PLAY_URL to run Play UI error-state stories")
	}
	p.gotoURL(p.Seed.ErrorPlayURL, routePath)
}

func (p *Page) ExpectURLSuffix(suffix string) {
	p.t.Helper()
	if err := WaitUntil(10*time.Second, func() error {
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
	if err := WaitUntil(10*time.Second, func() error {
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

func requireAdminSeed(t *testing.T) Seed {
	t.Helper()
	seed := setupSeed(t)
	requireURL(t, "Admin UI", seed.AdminURL)
	return seed
}

func requirePlaySeed(t *testing.T) Seed {
	t.Helper()
	seed := setupSeed(t)
	requireURL(t, "Play UI", seed.PlayURL)
	return seed
}

func requireSmokeSeed(t *testing.T) Seed {
	t.Helper()
	seed := setupSeed(t)
	requireURL(t, "Admin UI", seed.AdminURL)
	requireURL(t, "Play UI", seed.PlayURL)
	return seed
}

func setupSeed(t testing.TB) Seed {
	t.Helper()
	return Seed{
		AdminURL:              getenvDefault("GIZCLAW_E2E_ADMIN_URL", "http://127.0.0.1:8080"),
		PlayURL:               getenvDefault("GIZCLAW_E2E_PLAY_URL", "http://127.0.0.1:8081"),
		ErrorPlayURL:          os.Getenv("GIZCLAW_E2E_ERROR_PLAY_URL"),
		DevicePublicKey:       setupClientPublicKey(t),
		ActionDevicePublicKey: setupClientPublicKey(t),
		DeleteDevicePublicKey: setupClientPublicKey(t),
	}
}

func setupClientPublicKey(t testing.TB) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_CLIENT_IDENTITY_KEY"))
	if path == "" {
		configHome := getenvDefault("GIZCLAW_E2E_CLIENT_CONFIG_HOME", defaultClientConfigHome)
		contextName := getenvDefault("GIZCLAW_E2E_CLIENT_CONTEXT", defaultClientContext)
		path = filepath.Join(configHome, "gizclaw", contextName, "identity.key")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) != giznet.KeySize {
		t.Fatalf("invalid client identity key %s: got %d bytes, want %d", path, len(data), giznet.KeySize)
	}
	var private giznet.Key
	copy(private[:], data)
	kp, err := giznet.NewKeyPair(private)
	if err != nil {
		t.Fatalf("derive client public key from %s: %v", path, err)
	}
	return kp.Public.String()
}

func requireURL(t *testing.T, name, rawURL string) {
	t.Helper()
	if err := probeURL(rawURL, "GizClaw "+name); err != nil {
		t.Skipf("%s not reachable at %s; run setup/start-%s-ui.sh first: %v", name, rawURL, strings.ToLower(strings.TrimSuffix(name, " UI")), err)
	}
}

func probeURL(rawURL, marker string) error {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("read GET body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 500 {
		return fmt.Errorf("GET status %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), marker) {
		return fmt.Errorf("GET body missing %q", marker)
	}
	return nil
}

func WaitUntil(timeout time.Duration, check func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := check(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(pollInterval)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("condition not satisfied before timeout")
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func truncateForLog(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...<truncated>"
}

func safeArtifactName(name string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', ' ', '\t', '\n', '\r':
			return '_'
		default:
			return r
		}
	}, name)
}

func newBrowserRunner(t testing.TB) *browserRunner {
	t.Helper()

	pw, err := playwright.Run()
	if err != nil {
		t.Skipf("start Playwright: %v", err)
	}
	browser, err := pw.Chromium.Launch()
	if err != nil {
		_ = pw.Stop()
		t.Skipf("launch Chromium: %v", err)
	}
	return &browserRunner{browser: browser, pw: pw}
}

func (r *browserRunner) close(t testing.TB) {
	t.Helper()
	if r == nil {
		return
	}
	if r.browser != nil {
		if err := r.browser.Close(); err != nil {
			t.Fatalf("close browser: %v", err)
		}
	}
	if r.pw != nil {
		if err := r.pw.Stop(); err != nil {
			t.Fatalf("stop Playwright: %v", err)
		}
	}
}

func joinURL(t testing.TB, baseURL, routePath string) string {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse base URL %q: %v", baseURL, err)
	}
	if routePath == "" {
		return parsed.String()
	}
	if strings.HasPrefix(routePath, "#") {
		parsed.Fragment = strings.TrimPrefix(routePath, "#")
		return parsed.String()
	}
	parsed.Path = path.Join(parsed.Path, routePath)
	if strings.HasSuffix(routePath, "/") && !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	return parsed.String()
}
