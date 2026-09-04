// Command multiserver-seed provisions one GizClaw Server of the multi-server
// E2E stack with the local catalog that SFU scenarios need: a Pet Workflow, a
// RuntimeProfile and a RegistrationToken with fixed IDs. Every upsert is
// idempotent so the command may run again against a Server that already holds
// the catalog. When the Volc/Doubao provider credentials are present in the
// environment it also seeds the `asr` Model alias and the `narrator` Voice
// alias used by the TTS/ASR scenarios; otherwise those aliases are omitted and
// the provider-gated scenarios must be skipped by the caller.
//
// The registration token is printed on stdout as the only output line.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

const (
	credentialID = "volc-main-credential"
	tenantID     = "volc-main"
	asrModelID   = "volc-bigasr-sauc"
	asrResource  = "volc.bigasr.sauc.duration"
	ttsResource  = "seed-tts-2.0"
	narratorID   = "volc-tenant:volc-main:zh_female_xiaohe_uranus_bigtts"
	narratorVID  = "zh_female_xiaohe_uranus_bigtts"
)

var providerEnv = []string{
	"GIZCLAW_E2E_DOUBAO_APP_ID",
	"GIZCLAW_E2E_DOUBAO_API_KEY",
	"GIZCLAW_E2E_DOUBAO_SEARCH_API_KEY",
	"GIZCLAW_E2E_VOLC_ARK_API_KEY",
	"GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY_ID",
	"GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "multiserver-seed:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		server     = flag.String("server", "", "Server endpoint, e.g. server-a:9820")
		profileID  = flag.String("profile-id", "", "RuntimeProfile ID to upsert")
		workflowID = flag.String("workflow-id", "", "Pet Workflow ID to upsert (default <profile-id>-pet)")
		tokenID    = flag.String("token-id", "", "RegistrationToken ID to upsert (default <profile-id>-token)")
		token      = flag.String("token", "", "RegistrationToken value (default the token ID)")
		adminEnv   = flag.String("admin-key-env", "GIZCLAW_E2E_ADMIN_PRIVATE_KEY", "environment variable holding the Server's admin private key")
		timeout    = flag.Duration("timeout", 60*time.Second, "overall deadline")
	)
	flag.Parse()
	if *server == "" || *profileID == "" {
		return errors.New("-server and -profile-id are required")
	}
	if *workflowID == "" {
		*workflowID = *profileID + "-pet"
	}
	if *tokenID == "" {
		*tokenID = *profileID + "-token"
	}
	if *token == "" {
		*token = *tokenID
	}
	var private giznet.Key
	if err := private.UnmarshalText([]byte(os.Getenv(*adminEnv))); err != nil {
		return fmt.Errorf("parse %s: %w", *adminEnv, err)
	}
	adminKey, err := giznet.NewKeyPair(private)
	if err != nil {
		return fmt.Errorf("derive admin key: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	info, err := gizcli.FetchServerInfo(ctx, *server)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", *server, err)
	}
	client := &gizcli.Client{
		KeyPair: adminKey,
		DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			dialCtx, dialCancel := context.WithTimeout(ctx, 20*time.Second)
			defer dialCancel()
			return gizwebrtc.Dial(dialCtx, key, info.TransportPublicKey, gizwebrtc.DialConfig{
				SignalingURL:   info.SignalingURL,
				ICEServers:     info.ICEServers,
				SecurityPolicy: policy,
			})
		},
	}
	if err := client.Dial(info.PublicKey, info.PublicKey.String()); err != nil {
		return fmt.Errorf("dial %s: %w", *server, err)
	}
	defer client.Close()
	go func() { _ = client.Serve() }()
	if _, err := client.Ping(ctx, "multiserver-seed"); err != nil {
		return fmt.Errorf("ping %s: %w", *server, err)
	}
	api, err := client.ServerAdminClient()
	if err != nil {
		return fmt.Errorf("admin client: %w", err)
	}

	provider, providerErr := providerCredentials()
	if providerErr != nil {
		fmt.Fprintln(os.Stderr, "multiserver-seed: provider aliases omitted:", providerErr)
	} else if err := seedProvider(ctx, api, provider); err != nil {
		return err
	}
	if err := upsertWorkflow(ctx, api, adminhttp.WorkflowUpsert{Id: *workflowID, Spec: petWorkflowSpec()}); err != nil {
		return err
	}
	if err := upsertRuntimeProfile(ctx, api, adminhttp.RuntimeProfileUpsert{
		Id:   *profileID,
		Spec: runtimeProfileSpec(*workflowID, providerErr == nil),
	}); err != nil {
		return err
	}
	registration, err := upsertRegistrationToken(ctx, api, adminhttp.RegistrationTokenUpsert{
		Id:               *tokenID,
		Token:            *token,
		RuntimeProfileId: *profileID,
	})
	if err != nil {
		return err
	}
	fmt.Println(registration.Token)
	return nil
}

type volcCredentials struct {
	appID, apiKey, searchKey, arkKey, accessKeyID, accessKey string
}

func providerCredentials() (volcCredentials, error) {
	values := make(map[string]string, len(providerEnv))
	var missing []string
	for _, name := range providerEnv {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			missing = append(missing, name)
		}
		values[name] = value
	}
	if len(missing) != 0 {
		return volcCredentials{}, fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}
	return volcCredentials{
		appID:       values["GIZCLAW_E2E_DOUBAO_APP_ID"],
		apiKey:      values["GIZCLAW_E2E_DOUBAO_API_KEY"],
		searchKey:   values["GIZCLAW_E2E_DOUBAO_SEARCH_API_KEY"],
		arkKey:      values["GIZCLAW_E2E_VOLC_ARK_API_KEY"],
		accessKeyID: values["GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY_ID"],
		accessKey:   values["GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY"],
	}, nil
}

// seedProvider mirrors tests/gizclaw-e2e/testdata/resources for the Volc
// speech tenant: Credential, VolcTenant, the BigASR Model and the narrator
// Voice that the giztest RuntimeProfile binds as `asr` and `narrator`.
func seedProvider(ctx context.Context, api *adminhttp.ClientWithResponses, creds volcCredentials) error {
	var body apitypes.CredentialBody
	if err := body.FromVolcCredentialBody(apitypes.VolcCredentialBody{
		SpeechAppId:        new(creds.appID),
		SpeechApiKey:       new(creds.apiKey),
		ArkApiKey:          new(creds.arkKey),
		SearchApiKey:       new(creds.searchKey),
		OpenapiAccessKeyId: new(creds.accessKeyID),
		OpenapiAccessKey:   new(creds.accessKey),
	}); err != nil {
		return err
	}
	if err := upsertCredential(ctx, api, adminhttp.CredentialUpsert{
		Id:          credentialID,
		Provider:    "volc",
		Description: new("Volc service credential"),
		Body:        body,
	}); err != nil {
		return err
	}
	if err := upsertVolcTenant(ctx, api, adminhttp.VolcTenantUpsert{
		Id:           tenantID,
		CredentialId: credentialID,
		Description:  new("Volc/Doubao speech tenant"),
		Region:       new("cn-beijing"),
		ResourceIds:  &[]string{ttsResource, asrResource},
	}); err != nil {
		return err
	}
	var modelData apitypes.ModelProviderData
	if err := modelData.FromVolcTenantModelProviderData(apitypes.VolcTenantModelProviderData{
		ApiMode:    apitypes.VolcTenantModelProviderDataApiModeAsr,
		ResourceId: new(asrResource),
	}); err != nil {
		return err
	}
	if err := upsertModel(ctx, api, adminhttp.ModelUpsert{
		Id:           asrModelID,
		Kind:         apitypes.ModelKindAsr,
		Source:       apitypes.ModelSourceManual,
		DisplayName:  new("Volc BigASR SAUC"),
		Provider:     apitypes.ModelProvider{Kind: apitypes.ModelProviderKindVolcTenant, Id: tenantID},
		ProviderData: modelData,
	}); err != nil {
		return err
	}
	var voiceData apitypes.VoiceProviderData
	if err := voiceData.FromVolcTenantVoiceProviderData(apitypes.VolcTenantVoiceProviderData{
		ResourceId: new(ttsResource),
		VoiceId:    new(narratorVID),
	}); err != nil {
		return err
	}
	return upsertVoice(ctx, api, adminhttp.VoiceUpsert{
		Id:           narratorID,
		Source:       apitypes.VoiceSourceManual,
		DisplayName:  new("Narrator 2.0"),
		Provider:     apitypes.VoiceProvider{Kind: apitypes.VoiceProviderKindVolcTenant, Id: tenantID},
		ProviderData: &voiceData,
	})
}

// petWorkflowSpec is the minimal Pet Workflow the RuntimeProfile schema
// requires: a nested Flowcraft graph with a single publishing passthrough node
// and no model, voice or memory aliases.
func petWorkflowSpec() apitypes.WorkflowSpec {
	var node apitypes.FlowcraftNode
	if err := node.FromFlowcraftPassthroughNode(apitypes.FlowcraftPassthroughNode{
		Id:      "passthrough",
		Type:    apitypes.FlowcraftPassthroughNodeTypePassthrough,
		Publish: new(true),
	}); err != nil {
		panic(err)
	}
	return apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverPet,
		Pet: &apitypes.PetWorkflowSpec{
			Driver: apitypes.ReusableWorkflowDriverFlowcraft,
			Flowcraft: &apitypes.FlowcraftWorkflowSpec{
				Graph: apitypes.FlowcraftGraph{
					Name:  "multi-server-pet",
					Entry: "passthrough",
					Nodes: []apitypes.FlowcraftNode{node},
					Edges: &[]apitypes.FlowcraftEdge{{From: "passthrough", To: "__end__"}},
				},
			},
		},
	}
}

func runtimeProfileSpec(workflowID string, provider bool) apitypes.RuntimeProfileSpec {
	spec := apitypes.RuntimeProfileSpec{
		Resources: apitypes.RuntimeProfileResources{},
		Workflows: apitypes.RuntimeProfileWorkflows{
			Collections: apitypes.RuntimeProfileWorkflowCollections{},
			System:      apitypes.RuntimeProfileSystemWorkflows{Pet: workflowID},
		},
	}
	if provider {
		spec.Resources.Models = &map[string]apitypes.RuntimeProfileBinding{
			"asr": binding(asrModelID, "Speech Recognition", "语音识别"),
		}
		spec.Resources.Voices = &map[string]apitypes.RuntimeProfileBinding{
			"narrator": binding(narratorID, "Narrator", "旁白"),
		}
	}
	return spec
}

func binding(resourceID, en, zh string) apitypes.RuntimeProfileBinding {
	return apitypes.RuntimeProfileBinding{
		ResourceId: resourceID,
		I18n: map[string]apitypes.RuntimeProfileI18nText{
			"en":    {DisplayName: en},
			"zh-CN": {DisplayName: zh},
		},
	}
}

type statusResponse interface {
	StatusCode() int
}

func expectOK(kind, id string, response statusResponse, body []byte, ok bool, err error) error {
	if err != nil {
		return fmt.Errorf("%s %q: %w", kind, id, err)
	}
	if !ok {
		return fmt.Errorf("%s %q: status %d: %s", kind, id, response.StatusCode(), strings.TrimSpace(string(body)))
	}
	return nil
}

func upsertCredential(ctx context.Context, api *adminhttp.ClientWithResponses, body adminhttp.CredentialUpsert) error {
	existing, err := api.GetCredentialWithResponse(ctx, body.Id)
	if err != nil {
		return fmt.Errorf("get Credential %q: %w", body.Id, err)
	}
	if existing.JSON200 != nil {
		response, err := api.PutCredentialWithResponse(ctx, body.Id, body)
		return expectOK("put Credential", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err)
	}
	response, err := api.CreateCredentialWithResponse(ctx, body)
	return expectOK("create Credential", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err)
}

func upsertVolcTenant(ctx context.Context, api *adminhttp.ClientWithResponses, body adminhttp.VolcTenantUpsert) error {
	existing, err := api.GetVolcTenantWithResponse(ctx, body.Id)
	if err != nil {
		return fmt.Errorf("get VolcTenant %q: %w", body.Id, err)
	}
	if existing.JSON200 != nil {
		response, err := api.PutVolcTenantWithResponse(ctx, body.Id, body)
		return expectOK("put VolcTenant", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err)
	}
	response, err := api.CreateVolcTenantWithResponse(ctx, body)
	return expectOK("create VolcTenant", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err)
}

func upsertModel(ctx context.Context, api *adminhttp.ClientWithResponses, body adminhttp.ModelUpsert) error {
	existing, err := api.GetModelWithResponse(ctx, body.Id)
	if err != nil {
		return fmt.Errorf("get Model %q: %w", body.Id, err)
	}
	if existing.JSON200 != nil {
		response, err := api.PutModelWithResponse(ctx, body.Id, body)
		return expectOK("put Model", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err)
	}
	response, err := api.CreateModelWithResponse(ctx, body)
	return expectOK("create Model", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err)
}

func upsertVoice(ctx context.Context, api *adminhttp.ClientWithResponses, body adminhttp.VoiceUpsert) error {
	existing, err := api.GetVoiceWithResponse(ctx, body.Id)
	if err != nil {
		return fmt.Errorf("get Voice %q: %w", body.Id, err)
	}
	if existing.JSON200 != nil {
		response, err := api.PutVoiceWithResponse(ctx, body.Id, body)
		return expectOK("put Voice", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err)
	}
	response, err := api.CreateVoiceWithResponse(ctx, body)
	return expectOK("create Voice", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err)
}

func upsertWorkflow(ctx context.Context, api *adminhttp.ClientWithResponses, body adminhttp.WorkflowUpsert) error {
	existing, err := api.GetWorkflowWithResponse(ctx, body.Id)
	if err != nil {
		return fmt.Errorf("get Workflow %q: %w", body.Id, err)
	}
	if existing.JSON200 != nil {
		response, err := api.PutWorkflowWithResponse(ctx, body.Id, body)
		return expectOK("put Workflow", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err)
	}
	response, err := api.CreateWorkflowWithResponse(ctx, body)
	return expectOK("create Workflow", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err)
}

func upsertRuntimeProfile(ctx context.Context, api *adminhttp.ClientWithResponses, body adminhttp.RuntimeProfileUpsert) error {
	existing, err := api.GetRuntimeProfileWithResponse(ctx, body.Id)
	if err != nil {
		return fmt.Errorf("get RuntimeProfile %q: %w", body.Id, err)
	}
	if existing.JSON200 != nil {
		response, err := api.PutRuntimeProfileWithResponse(ctx, body.Id, body)
		return expectOK("put RuntimeProfile", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err)
	}
	response, err := api.CreateRuntimeProfileWithResponse(ctx, body)
	return expectOK("create RuntimeProfile", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err)
}

func upsertRegistrationToken(ctx context.Context, api *adminhttp.ClientWithResponses, body adminhttp.RegistrationTokenUpsert) (apitypes.RegistrationToken, error) {
	existing, err := api.GetRegistrationTokenWithResponse(ctx, body.Id)
	if err != nil {
		return apitypes.RegistrationToken{}, fmt.Errorf("get RegistrationToken %q: %w", body.Id, err)
	}
	if existing.JSON200 != nil {
		response, err := api.PutRegistrationTokenWithResponse(ctx, body.Id, body)
		if err := expectOK("put RegistrationToken", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err); err != nil {
			return apitypes.RegistrationToken{}, err
		}
		return *response.JSON200, nil
	}
	response, err := api.CreateRegistrationTokenWithResponse(ctx, body)
	if err := expectOK("create RegistrationToken", body.Id, response, responseBody(response), err == nil && response != nil && response.JSON200 != nil, err); err != nil {
		return apitypes.RegistrationToken{}, err
	}
	return *response.JSON200, nil
}

// responseBody extracts the raw body of any generated *XxxResponse value
// for diagnostics, tolerating the nil pointer returned alongside an error.
func responseBody(response any) []byte {
	value := reflect.ValueOf(response)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return nil
	}
	body := value.Elem().FieldByName("Body")
	if body.IsValid() && body.Kind() == reflect.Slice {
		return body.Bytes()
	}
	return nil
}
