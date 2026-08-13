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
	paths := workspaceConfigCatalogPaths(t)
	specialized := map[string]struct{}{
		"dashscope-realtime.json":          {},
		"doubao-realtime-quality.json":     {},
		"doubao-realtime-duplex.json":      {},
		"eino-memory.json":                 {},
		"flowcraft-configured-memory.json": {},
		"flowcraft-realtime-chat.json":     {},
	}
	defaults := make([]string, 0, len(paths)-len(specialized))
	for _, path := range paths {
		if _, ok := specialized[filepath.Base(path)]; !ok {
			defaults = append(defaults, path)
		}
	}
	return defaults
}

func workspaceConfigCatalogPaths(t testing.TB) []string {
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
	return selectedWorkspaceConfigPaths(t, "ast-translate.json", "doubao-realtime.json", "flowcraft-basic.json")
}

func flowcraftRealtimeChatWorkspaceConfigPaths(t testing.TB) []string {
	t.Helper()
	return selectedWorkspaceConfigPaths(t, "flowcraft-realtime-chat.json")
}

func historyReplayWorkspaceConfigPaths(t testing.TB) []string {
	t.Helper()
	return selectedWorkspaceConfigPaths(t, "flowcraft-basic.json")
}

func selectedWorkspaceConfigPaths(t testing.TB, names ...string) []string {
	t.Helper()
	available := make(map[string]string)
	for _, path := range workspaceConfigCatalogPaths(t) {
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
		t.Fatalf("required e2e setup server is not available: %v", err)
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
				t.Fatalf("required e2e setup server became unavailable: %v", err)
			}
			t.Fatalf("%s %s: %v", selected, path, err)
		})
	}
}

func runRequiredLiveWorkspaceCase(t *testing.T, selected workspaceCase, paths []string) {
	t.Helper()
	if err := probeLiveWorkspaceSetup(); err != nil {
		t.Fatalf("required e2e setup server is not available: %v", err)
	}
	t.Setenv("GIZCLAW_E2E_CHAT_REGISTRATION_TOKEN", createChatRegistrationToken(t, selected))
	for _, path := range paths {
		path := path
		t.Run(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), func(t *testing.T) {
			if err := runConfigWithLiveRetry(path, clientContextConfigPath(), selected); err != nil {
				t.Fatalf("%s %s: %v", selected, path, err)
			}
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

	workflowResources := map[string]string{
		"dashscope-realtime-conversation":     "dashscope-realtime-conversation",
		"volc-ast-translate":                  "volc-ast-translate",
		"volc-ast-translate-tts":              "volc-ast-translate-tts",
		"volc-ast-translate-zh-en":            "volc-ast-translate-zh-en",
		"volc-ast-translate-zh-jp":            "volc-ast-translate-zh-jp",
		"doubao-realtime-conversation":        "doubao-realtime-conversation",
		"doubao-realtime-quality":             "doubao-realtime-quality",
		"doubao-realtime-duplex-conversation": "doubao-realtime-duplex-conversation",
		"eino-memory-assistant":               "eino-memory-assistant",
		"flowcraft-configured-memory":         "flowcraft-configured-memory",
		"flowcraft-realtime-chat":             "flowcraft-realtime-chat",
		"flowcraft-voice-assistant":           "flowcraft-voice-assistant",
		"flowcraft-chat-assistant":            "flowcraft-chat-assistant",
		"flowcraft-journey-guide":             "flowcraft-journey-guide",
		"flowcraft-multi-role-storyteller":    "flowcraft-multi-role-storyteller",
		"flowcraft-murder-mystery":            "flowcraft-murder-mystery",
		"flowcraft-poetry-adventure-li-bai":   "flowcraft-poetry-adventure-li-bai",
		"flowcraft-werewolf-game":             "flowcraft-werewolf-game",
	}
	modelResources := map[string]string{
		"llm":             "doubao-lite-chat",
		"tts":             "volc-bigtts",
		"asr":             "volc-bigasr-sauc",
		"realtime":        "doubao-realtime-dialog",
		"dash-realtime":   "qwen-realtime-dialog",
		"realtime-duplex": "doubao-realtime-duplex-dialog",
		"translation":     "volc-ast-translate",
	}
	voiceResources := map[string]string{
		"assistant-voice":  "volc-tenant:volc-main:zh_female_vv_mars_bigtts",
		"japanese-voice":   "volc-tenant:volc-main:multi_female_shuangkuaisisi_moon_bigtts",
		"doubao-assistant": "volc-tenant:volc-main:zh_female_vv_jupiter_bigtts",
		"narrator":         "volc-tenant:volc-main:zh_female_shaoergushi_mars_bigtts",
		"cute-pet":         "volc-tenant:volc-main:zh_male_naiqimengwa_mars_bigtts",
		"monster":          "volc-tenant:volc-main:ICL_zh_female_bingjiao3_tob",
		"game-master":      "volc-tenant:volc-main:zh_male_changtianyi_mars_bigtts",
		"detective":        "volc-tenant:volc-main:ICL_zh_male_lengjungaozhi_tob",
		"police-officer":   "volc-tenant:volc-main:ICL_zh_male_zhengzhiqingnian_tob",
		"sun-wukong":       "volc-tenant:volc-main:zh_male_sunwukong_mars_bigtts",
		"tang-sanzang":     "volc-tenant:volc-main:zh_male_tangseng_mars_bigtts",
		"zhu-bajie":        "volc-tenant:volc-main:zh_male_zhubajie_mars_bigtts",
	}
	const profileName = "e2e-chat"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	profile, err := clitest.UpsertRuntimeProfile(ctx, api, adminhttp.RuntimeProfileUpsert{
		Id: profileName,
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{
			Memories: ptr(runtimeMemoryBindings(t)),
			Models:   ptr(runtimeBindings(modelResources)),
			Voices:   ptr(runtimeBindings(voiceResources)),
		}, Workflows: apitypes.RuntimeProfileWorkflows{
			System: apitypes.RuntimeProfileSystemWorkflows{
				FriendChatroom: "chatroom-direct",
				GroupChatroom:  "chatroom-direct",
				Pet:            "pet-chatroom",
			},
			Collections: apitypes.RuntimeProfileWorkflowCollections{
				"assistants": runtimeBindings(workflowResources),
			},
		}},
	})
	if err != nil {
		t.Fatalf("put chat RuntimeProfile: %v", err)
	}
	tokenName := "e2e-chat-" + string(selected)
	if err := clitest.DeleteRegistrationTokenByID(ctx, api, tokenName); err != nil {
		t.Fatalf("retire chat RegistrationToken: %v", err)
	}
	tokenResp, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{
		Id:               tokenName,
		Token:            tokenName,
		RuntimeProfileId: profile.Id,
	})
	if err != nil {
		t.Fatalf("create chat RegistrationToken: %v", err)
	}
	if tokenResp.JSON200 == nil || tokenResp.JSON200.Token == "" {
		t.Fatalf("create chat RegistrationToken status %d: %s", tokenResp.StatusCode(), strings.TrimSpace(string(tokenResp.Body)))
	}
	return tokenResp.JSON200.Token
}

func runtimeBindings(resources map[string]string) map[string]apitypes.RuntimeProfileBinding {
	bindings := make(map[string]apitypes.RuntimeProfileBinding, len(resources))
	for alias, resourceID := range resources {
		bindings[alias] = apitypes.RuntimeProfileBinding{
			ResourceId: resourceID,
			I18n: map[string]apitypes.RuntimeProfileI18nText{
				"en":    {DisplayName: alias},
				"zh-CN": {DisplayName: alias},
			},
		}
	}
	return bindings
}

func runtimeMemoryBindings(t *testing.T) map[string]apitypes.RuntimeProfileMemoryBinding {
	t.Helper()
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileFlowcraftBBHConnection(apitypes.RuntimeProfileFlowcraftBBHConnection{
		Type: apitypes.RuntimeProfileFlowcraftBBHConnectionTypeFlowcraftBbh,
	}); err != nil {
		t.Fatal(err)
	}
	result := make(map[string]apitypes.RuntimeProfileMemoryBinding)
	for _, alias := range []string{
		"default-memory",
		"voice-assistant-memory",
		"chat-memory",
		"journey-memory",
		"storyteller-memory",
		"murder-mystery-memory",
		"poetry-memory",
		"werewolf-memory",
	} {
		result[alias] = apitypes.RuntimeProfileMemoryBinding{
			LayoutId: alias, Driver: apitypes.RuntimeProfileMemoryDriverFlowcraft, Connection: connection,
		}
	}
	return result
}

func ptr[T any](value T) *T { return &value }

func runConfigWithLiveRetry(path, contextConfigPath string, selected workspaceCase) error {
	var err error
	for attempt := 1; attempt <= 5; attempt++ {
		started := time.Now()
		fmt.Printf("workspace_case_attempt case=%s config=%s attempt=%d\n", selected, filepath.Base(path), attempt)
		cfg, loadErr := loadConfig(path, contextConfigPath)
		if loadErr != nil {
			return loadErr
		}
		cfg.workspaceSuffix = fmt.Sprintf("run-%d", os.Getpid())
		if attempt > 1 {
			cfg.workspaceSuffix += fmt.Sprintf("-retry-%d", attempt)
		}
		err = runLoadedConfig(cfg, selected)
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
		strings.Contains(text, "stream idle timeout") ||
		strings.Contains(text, "audiodock: TTS completion timeout") ||
		strings.Contains(text, "doubao realtime: response idle timeout") ||
		strings.Contains(text, "doubao asr: finalization timeout") ||
		strings.Contains(text, "doubao ast translate: realtime session completion timeout") ||
		strings.Contains(text, "doubaospeech: sami error") ||
		isRetryableSpeechServiceConnectionClosure(text) ||
		strings.Contains(text, "Get \"") && strings.Contains(text, "/server-info\": context deadline exceeded") ||
		strings.Contains(text, "doubaospeech: [Server-side generic error]") && strings.Contains(text, "big asr recv err") ||
		strings.Contains(text, "send tts stream request:") && strings.Contains(text, "Client.Timeout exceeded while awaiting headers") ||
		strings.Contains(text, "assistant audio asr") && (strings.Contains(text, "400 Bad Request") ||
			strings.Contains(text, "status code 400") ||
			strings.Contains(text, "giznet: conn closed") ||
			strings.Contains(text, "User Initiated Abort")) ||
		strings.Contains(text, "transcription ") && strings.Contains(text, " size=") &&
			(strings.Contains(text, "giznet: conn closed") || strings.Contains(text, "User Initiated Abort")) ||
		strings.Contains(text, "self-start missing assistant text") ||
		strings.Contains(text, "interrupt second stream started before interrupted assistant EOS") ||
		strings.Contains(text, "transcript mismatch: similarity")
}

func isRetryableSpeechServiceConnectionClosure(text string) bool {
	return strings.Contains(text, `speech: Post "http://gizclaw/v1/audio/speech"`) &&
		strings.Contains(text, "gizhttp: dial service") &&
		strings.Contains(text, "giznet: conn closed")
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
