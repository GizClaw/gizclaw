//go:build gizclaw_e2e

package chat

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestPushToTalkRoundtrip(t *testing.T) {
	runLiveWorkspaceCase(t, workspaceCasePushToTalkRoundtrip, allWorkspaceConfigPaths(t))
}

func allWorkspaceConfigPaths(t testing.TB) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "workspaces", "*.json"))
	if err != nil {
		t.Fatalf("glob workspace configs: %v", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatal("no workspace configs found under testdata/workspaces")
	}
	return paths
}

func interruptWorkspaceConfigPaths(t testing.TB) []string {
	t.Helper()
	return selectedWorkspaceConfigPaths(t, "ast-translate-tts.json", "flowcraft-basic.json")
}

func realtimeInterruptWorkspaceConfigPaths(t testing.TB) []string {
	t.Helper()
	return selectedWorkspaceConfigPaths(t, "ast-translate.json", "doubao-realtime.json", "flowcraft-basic.json")
}

func continuousWorkspaceConfigPaths(t testing.TB) []string {
	t.Helper()
	return selectedWorkspaceConfigPaths(t, "ast-translate.json", "doubao-realtime.json", "flowcraft-basic.json")
}

func realtimeAutoSplitWorkspaceConfigPaths(t testing.TB) []string {
	t.Helper()
	return selectedWorkspaceConfigPaths(t, "ast-translate.json", "doubao-realtime.json")
}

func historyReplayWorkspaceConfigPaths(t testing.TB) []string {
	t.Helper()
	return selectedWorkspaceConfigPaths(t, "flowcraft-basic.json")
}

func selectedWorkspaceConfigPaths(t testing.TB, names ...string) []string {
	t.Helper()
	available := make(map[string]string)
	for _, path := range allWorkspaceConfigPaths(t) {
		available[filepath.Base(path)] = path
	}
	paths := make([]string, 0, len(names))
	for _, name := range names {
		path, ok := available[name]
		if !ok {
			t.Fatalf("workspace config %q is not committed", name)
		}
		paths = append(paths, path)
	}
	return paths
}

func runLiveWorkspaceCase(t *testing.T, selected workspaceCase, paths []string) {
	t.Helper()
	if err := probeLiveWorkspaceSetup(); err != nil {
		if os.Getenv("GIZCLAW_E2E_REQUIRE_LIVE") == "1" {
			t.Fatalf("required e2e setup server is not available: %v", err)
		}
		t.Skipf("e2e setup server is not available: %v", err)
	}
	t.Setenv("GIZCLAW_E2E_CHAT_REGISTRATION_TOKEN", createChatRegistrationToken(t, selected))
	for _, path := range paths {
		path := path
		t.Run(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), func(t *testing.T) {
			err := runConfigWithLiveRetry(path, clientContextConfigPath(), selected)
			if err == nil {
				return
			}
			if shouldSkipUnavailableSetup(err) {
				if os.Getenv("GIZCLAW_E2E_REQUIRE_LIVE") == "1" {
					t.Fatalf("required e2e setup server became unavailable: %v", err)
				}
				t.Skipf("e2e setup server is not available: %v", err)
			}
			t.Fatalf("%s %s: %v", selected, path, err)
		})
	}
}

func createChatRegistrationToken(t *testing.T, selected workspaceCase) string {
	t.Helper()
	h := clitest.NewSetupHarness(t, "go-chat-"+string(selected))
	identitiesHome := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_IDENTITIES_HOME"))
	if identitiesHome == "" {
		identitiesHome = filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "identities")
	}
	adminContext := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_ADMIN_IDENTITY"))
	if adminContext == "" {
		adminContext = "admin"
	}
	h.SetContextDirAlias("admin-a", filepath.Join(identitiesHome, adminContext))
	admin := h.ConnectClientFromContextEventually("admin-a", 30*time.Second)
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create chat admin client: %v", err)
	}

	workflows := map[string]string{
		"ast-translate":                    "volc-ast-translate",
		"ast-translate-tts":                "volc-ast-translate-tts",
		"ast-translate-zh-en":              "volc-ast-translate-zh-en",
		"ast-translate-zh-jp":              "volc-ast-translate-zh-jp",
		"realtime":                         "doubao-realtime-conversation",
		"flowcraft-basic":                  "flowcraft-voice-assistant",
		"flowcraft-chat":                   "flowcraft-chat-assistant",
		"flowcraft-func-chat":              "flowcraft-tool-chat",
		"flowcraft-journey":                "flowcraft-journey-guide",
		"flowcraft-match-route":            "flowcraft-route-matcher",
		"flowcraft-multi-role-storyteller": "flowcraft-multi-role-storyteller",
		"flowcraft-murder-mystery":         "flowcraft-murder-mystery",
		"flowcraft-poetry-adventure":       "flowcraft-poetry-adventure-li-bai",
		"flowcraft-werewolf":               "flowcraft-werewolf-game",
	}
	models := map[string]string{
		"llm":         "doubao-lite-chat",
		"tts":         "volc-bigtts",
		"asr":         "volc-bigasr-sauc",
		"realtime":    "doubao-realtime-dialog",
		"translation": "volc-ast-translate",
	}
	voices := map[string]string{
		"assistant":   "volc-tenant:volc-main:zh_female_vv_mars_bigtts",
		"storyteller": "volc-tenant:volc-main:zh_female_shaoergushi_mars_bigtts",
		"monster":     "volc-tenant:volc-main:ICL_zh_female_bingjiao3_tob",
		"wukong":      "volc-tenant:volc-main:zh_male_sunwukong_mars_bigtts",
		"tangseng":    "volc-tenant:volc-main:zh_male_tangseng_mars_bigtts",
		"bajie":       "volc-tenant:volc-main:zh_male_zhubajie_mars_bigtts",
	}
	const profileName = "e2e-chat"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	profileResp, err := api.PutRuntimeProfileWithResponse(ctx, profileName, adminhttp.RuntimeProfileUpsert{
		Name: profileName,
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{
			Workflows: &workflows,
			Models:    &models,
			Voices:    &voices,
		}},
	})
	if err != nil {
		t.Fatalf("put chat RuntimeProfile: %v", err)
	}
	if profileResp.JSON200 == nil {
		t.Fatalf("put chat RuntimeProfile status %d: %s", profileResp.StatusCode(), strings.TrimSpace(string(profileResp.Body)))
	}

	tokenName := "e2e-chat-" + string(selected)
	_, _ = api.DeleteRegistrationTokenWithResponse(ctx, tokenName)
	tokenResp, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{
		Name:               tokenName,
		FirmwareName:       "devkit-firmware-main",
		RuntimeProfileName: profileName,
	})
	if err != nil {
		t.Fatalf("create chat RegistrationToken: %v", err)
	}
	if tokenResp.JSON200 == nil || tokenResp.JSON200.Token == "" {
		t.Fatalf("create chat RegistrationToken status %d: %s", tokenResp.StatusCode(), strings.TrimSpace(string(tokenResp.Body)))
	}
	return tokenResp.JSON200.Token
}

func runConfigWithLiveRetry(path, contextConfigPath string, selected workspaceCase) error {
	var err error
	for attempt := 1; attempt <= 5; attempt++ {
		started := time.Now()
		fmt.Printf("workspace_case_attempt case=%s config=%s attempt=%d\n", selected, filepath.Base(path), attempt)
		err = runConfig(path, contextConfigPath, selected)
		retryable := isRetryableLiveWorkspaceError(err)
		result := "pass"
		if err != nil {
			result = "fail"
		}
		fmt.Printf("workspace_case_attempt_done case=%s config=%s attempt=%d result=%s retryable=%t elapsed=%s\n", selected, filepath.Base(path), attempt, result, retryable, time.Since(started).Truncate(time.Millisecond))
		if err == nil || !retryable {
			return err
		}
		if attempt < 5 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return err
}

func isRetryableLiveWorkspaceError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "Bad Gateway") ||
		strings.Contains(text, "websocket read: unexpected EOF") ||
		strings.Contains(text, "websocket: close 1006 (abnormal closure): unexpected EOF") ||
		strings.Contains(text, "transport: timeout") ||
		strings.Contains(text, "response incomplete: length") ||
		strings.Contains(text, "doubaospeech: [Server processing timeout] node execution timeout") ||
		strings.Contains(text, "doubaospeech: [Server-side generic error]") && strings.Contains(text, "big asr recv err") ||
		strings.Contains(text, "send tts stream request:") && strings.Contains(text, "Client.Timeout exceeded while awaiting headers") ||
		strings.Contains(text, "assistant audio asr") && (strings.Contains(text, "400 Bad Request") || strings.Contains(text, "status code 400")) ||
		strings.Contains(text, "self-start missing assistant text") ||
		strings.Contains(text, "interrupt second stream started before interrupted assistant EOS") ||
		strings.Contains(text, "transcript mismatch: similarity")
}

func probeLiveWorkspaceSetup() error {
	contextPath := clientContextConfigPath()
	if contextPath == "" {
		contextPath = defaultClientContextConfigPath()
	}
	contextCfg, err := readSetupContextConfig(contextPath)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("tcp", contextCfg.Server.Addr, 200*time.Millisecond)
	if err != nil {
		return err
	}
	return conn.Close()
}

func shouldSkipUnavailableSetup(err error) bool {
	text := err.Error()
	return strings.Contains(text, "connection refused") ||
		strings.Contains(text, "no such file or directory") ||
		strings.Contains(text, "read context config")
}
