package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

type adminConnection struct {
	once   sync.Once
	client *gizcli.Client
	api    *adminhttp.ClientWithResponses
	err    error
	done   chan error
}

func (a *adminConnection) connect(ctx context.Context) error {
	a.once.Do(func() {
		var private giznet.Key
		if a.err = private.UnmarshalText([]byte(os.Getenv("MONITOR_ADMIN_PRIVATE_KEY"))); a.err != nil {
			return
		}
		key, err := giznet.NewKeyPair(private)
		if err != nil {
			a.err = err
			return
		}
		info, err := gizcli.FetchServerInfo(ctx, "server:9820")
		if err != nil {
			a.err = err
			return
		}
		a.client = &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			return gizwebrtc.Dial(ctx, key, info.TransportPublicKey, gizwebrtc.DialConfig{SignalingURL: info.SignalingURL, ICEServers: info.ICEServers, SecurityPolicy: policy})
		}}
		if a.err = a.client.Dial(info.PublicKey, "server:9820"); a.err != nil {
			return
		}
		a.done = make(chan error, 1)
		go func() { a.done <- a.client.Serve() }()
		a.api, a.err = a.client.ServerAdminClient()
	})
	return a.err
}
func (a *adminConnection) Close() error {
	if a.client == nil {
		return nil
	}
	err := a.client.Close()
	if a.done != nil {
		<-a.done
	}
	return err
}
func seed() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a := &adminConnection{}
	if err := a.connect(ctx); err != nil {
		return err
	}
	defer a.Close()
	api := a.api
	check := func(status int, err error) error {
		if err != nil {
			return err
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("seed HTTP %d", status)
		}
		return nil
	}
	var openaiBody, minimaxBody apitypes.CredentialBody
	if err := openaiBody.FromOpenAICredentialBody(apitypes.OpenAICredentialBody{ApiKey: new("provider-fixture")}); err != nil {
		return err
	}
	if err := minimaxBody.FromMiniMaxCredentialBody(apitypes.MiniMaxCredentialBody{ApiKey: new("provider-fixture")}); err != nil {
		return err
	}
	for provider, body := range map[string]apitypes.CredentialBody{"openai": openaiBody, "minimax": minimaxBody} {
		r, err := api.CreateCredentialWithResponse(ctx, adminhttp.CredentialUpsert{Id: provider, Provider: provider, Body: body})
		if err != nil {
			return err
		}
		if err = check(r.StatusCode(), nil); err != nil {
			return err
		}
	}
	openai, err := api.CreateOpenAITenantWithResponse(ctx, adminhttp.OpenAITenantUpsert{Id: "fixture", CredentialId: "openai", BaseUrl: new("http://fixture:9825/v1/")})
	if err != nil {
		return err
	}
	if err = check(openai.StatusCode(), nil); err != nil {
		return err
	}
	minimax, err := api.CreateMiniMaxTenantWithResponse(ctx, adminhttp.MiniMaxTenantUpsert{Id: "fixture", CredentialId: "minimax", BaseUrl: new("http://fixture:9825")})
	if err != nil {
		return err
	}
	if err = check(minimax.StatusCode(), nil); err != nil {
		return err
	}
	var modelData apitypes.ModelProviderData
	if err = modelData.FromOpenAITenantModelProviderData(apitypes.OpenAITenantModelProviderData{UpstreamModel: "billing-chat", SupportJsonOutput: new(true)}); err != nil {
		return err
	}
	model, err := api.CreateModelWithResponse(ctx, adminhttp.ModelUpsert{Id: "quota-chat", Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceManual, Provider: apitypes.ModelProvider{Kind: apitypes.ModelProviderKindOpenaiTenant, Id: "fixture"}, ProviderData: modelData})
	if err != nil {
		return err
	}
	if err = check(model.StatusCode(), nil); err != nil {
		return err
	}
	var voiceData apitypes.VoiceProviderData
	if err = voiceData.FromMiniMaxTenantVoiceProviderData(apitypes.MiniMaxTenantVoiceProviderData{VoiceId: new("fixture-voice"), Model: new("billing-speech"), Format: new("pcm")}); err != nil {
		return err
	}
	voice, err := api.CreateVoiceWithResponse(ctx, adminhttp.VoiceUpsert{Id: "quota-voice", Source: apitypes.VoiceSourceManual, Provider: apitypes.VoiceProvider{Kind: apitypes.VoiceProviderKindMinimaxTenant, Id: "fixture"}, ProviderData: &voiceData})
	if err != nil {
		return err
	}
	if err = check(voice.StatusCode(), nil); err != nil {
		return err
	}
	workflowBodies := map[string]string{
		"eino":      `{"driver":"eino","eino":{"graph":{"name":"Quota chat","state":{"fields":[{"name":"messages","type":"messages","merge":"replace"},{"name":"answer","type":"string","merge":"replace"}]},"nodes":[{"id":"prompt","type":"prompt","format":"f_string","inputs":{"text":{"from":"input.text"}},"outputs":{"messages":"messages"},"messages":[{"role":"user","template":"{text}"}]},{"id":"chat","type":"chat_model","model":"chat","inputs":{"messages":{"from":"messages"}},"outputs":{"text":"answer"}}],"edges":[{"from":"start","to":"prompt"},{"from":"prompt","to":"chat"},{"from":"chat","to":"end"}],"branches":[],"outputs":[{"name":"answer","node":"chat","field":"answer","mime_type":"text/plain","primary":true}],"compile":{"node_trigger_mode":"all_predecessor"}}}}`,
		"flowcraft": `{"driver":"flowcraft","flowcraft":{"graph":{"name":"Quota chat","entry":"chat","nodes":[{"id":"chat","type":"llm","publish":true,"config":{"model":"chat"}}],"edges":[{"from":"chat","to":"__end__"}]}}}`,
	}
	for driver, body := range workflowBodies {
		var spec apitypes.WorkflowSpec
		if err := json.Unmarshal([]byte(body), &spec); err != nil {
			return err
		}
		result, err := api.CreateWorkflowWithResponse(ctx, adminhttp.WorkflowUpsert{Id: "quota-" + driver, Spec: spec})
		if err != nil {
			return err
		}
		if err := check(result.StatusCode(), nil); err != nil {
			return fmt.Errorf("seed %s Workflow: %w: %s", driver, err, result.Body)
		}
	}
	binding := func(id string) apitypes.RuntimeProfileBinding {
		return apitypes.RuntimeProfileBinding{ResourceId: id, I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: id}, "zh-CN": {DisplayName: id}}}
	}
	for _, mode := range append(append([]string{}, modes...), "unconfigured", "unlimited") {
		var policy *apitypes.RuntimeProfileQuota
		if mode != "unconfigured" {
			policy = &apitypes.RuntimeProfileQuota{}
			var err error
			if mode == "unlimited" {
				err = policy.FromRuntimeProfileQuotaUnlimited(apitypes.RuntimeProfileQuotaUnlimited{Type: apitypes.RuntimeProfileQuotaUnlimitedTypeUnlimited})
			} else {
				err = policy.FromRuntimeProfileQuotaCustom(apitypes.RuntimeProfileQuotaCustom{Type: apitypes.RuntimeProfileQuotaCustomTypeCustom, Endpoint: "http://fixture:9825/v1/quota", ApiKey: mode})
			}
			if err != nil {
				return err
			}
		}
		spec := apitypes.RuntimeProfileSpec{Quota: policy, Workflows: apitypes.RuntimeProfileWorkflows{"quota-eino": binding("quota-eino"), "quota-flowcraft": binding("quota-flowcraft")}, Resources: apitypes.RuntimeProfileResources{Models: new(map[string]apitypes.RuntimeProfileBinding{"chat": binding("quota-chat")}), Voices: new(map[string]apitypes.RuntimeProfileBinding{"narrator": binding("quota-voice")})}}
		profile, err := api.CreateRuntimeProfileWithResponse(ctx, adminhttp.RuntimeProfileUpsert{Id: "quota-" + mode, Spec: spec})
		if err != nil {
			return err
		}
		if err = check(profile.StatusCode(), nil); err != nil {
			return err
		}
		token, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{Id: "quota-" + mode, Token: "quota-" + mode, RuntimeProfileId: "quota-" + mode})
		if err != nil {
			return err
		}
		if err = check(token.StatusCode(), nil); err != nil {
			return err
		}
	}
	return nil
}
func (f *fixture) prepare(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name == "" {
		http.Error(w, "name required", 400)
		return
	}
	defer r.Body.Close()
	if err := f.admin.connect(r.Context()); err != nil {
		http.Error(w, "admin unavailable", 503)
		return
	}
	list, err := f.admin.api.ListPeersWithResponse(r.Context(), nil)
	if err != nil || list.JSON200 == nil {
		http.Error(w, "peers unavailable", 503)
		return
	}
	for _, item := range list.JSON200.Items {
		peer, decodeErr := item.AsExternalRef0Registration()
		if decodeErr != nil || peer.Device == nil {
			continue
		}
		if peer.Role == apitypes.PeerRoleClient && peer.Status == apitypes.PeerRegistrationStatusActive {
			result, err := f.admin.api.RefreshPeerWithResponse(r.Context(), peer.PublicKey)
			if err != nil || result.JSON200 == nil {
				continue
			}
			if result.JSON200.Peer.Device.Identifiers == nil || result.JSON200.Peer.Device.Identifiers.Sn == nil || *result.JSON200.Peer.Device.Identifiers.Sn != body.Name {
				continue
			}
			writeJSON(w, map[string]any{"public_key": peer.PublicKey})
			return
		}
	}
	http.Error(w, "Peer not found", 404)
}
func initialize(dir string) error {
	values := map[string]string{}
	for _, role := range []string{"SERVER", "EDGE", "ADMIN"} {
		key, err := giznet.GenerateKeyPair()
		if err != nil {
			return err
		}
		values["MONITOR_"+role+"_PRIVATE_KEY"] = key.Private.String()
		values["MONITOR_"+role+"_PUBLIC_KEY"] = key.Public.String()
	}
	var env strings.Builder
	for key, value := range values {
		fmt.Fprintf(&env, "%s=%s\n", key, value)
	}
	if err := os.MkdirAll(filepath.Join(dir, "server"), 0700); err != nil {
		return err
	}
	data, err := os.ReadFile("tests/gizclaw-e2e/docker/monitor/server.yaml")
	if err != nil {
		return err
	}
	text := os.Expand(string(data), func(key string) string { return values[key] })
	if err = os.WriteFile(filepath.Join(dir, "server", "config.yaml"), []byte(text), 0600); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "edge"), 0700); err != nil {
		return err
	}
	edge, err := os.ReadFile("tests/gizclaw-e2e/docker/monitor/edge.yaml")
	if err != nil {
		return err
	}
	edgeText := os.Expand(string(edge), func(key string) string { return values[key] })
	if err := os.WriteFile(filepath.Join(dir, "edge", "config.yaml"), []byte(edgeText), 0600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "runtime.env"), []byte(env.String()), 0600)
}
