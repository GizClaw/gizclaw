package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/sqltest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/providertenants"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"google.golang.org/protobuf/proto"
)

type modelBindingRequest struct {
	headers http.Header
	body    map[string]any
}

type modelBindingFixture struct {
	models      *model.Server
	credentials *credential.Server
	tenants     *providertenants.Server
	runtime     *peerrun.Server
	upstream    *httptest.Server
	requests    chan modelBindingRequest
	profile     apitypes.RuntimeProfile
	upsert      adminhttp.ModelUpsert
	clock       atomic.Int64
	// Set before starting calls. Returning true replaces the normal response.
	onRequest func(http.ResponseWriter, *http.Request, map[string]any) bool
}

func newModelBindingFixture(t *testing.T) *modelBindingFixture {
	t.Helper()
	db, _ := sqltest.New(t)
	f := &modelBindingFixture{
		models: &model.Server{DB: db}, credentials: &credential.Server{DB: db},
		tenants: &providertenants.Server{DB: db}, runtime: &peerrun.Server{DB: db},
		requests: make(chan modelBindingRequest, 128),
	}
	for _, store := range []interface{ Initialize(context.Context) error }{f.models, f.credentials, f.tenants, f.runtime} {
		if err := store.Initialize(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	f.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if r.URL.Path != "/chat/completions" || body["model"] != "upstream-chat" || r.Header.Get("Authorization") != "Bearer binding-test-key" {
			t.Errorf("unexpected provider request: %s %v", r.URL.Path, body["model"])
		}
		encoded, _ := json.Marshal(body)
		if strings.Contains(string(encoded), "device-value-") || strings.Contains(string(encoded), "product-value-") {
			t.Error("Runtime metadata entered the model body")
		}
		f.requests <- modelBindingRequest{headers: r.Header.Clone(), body: body}
		if f.onRequest != nil && f.onRequest(w, r, body) {
			return
		}
		if body["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: "+`{"id":"chat","object":"chat.completion.chunk","model":"upstream-chat","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`+"\n\ndata: "+`{"id":"chat","object":"chat.completion.chunk","model":"upstream-chat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if body["response_format"] != nil {
			_, _ = io.WriteString(w, `{"id":"chat","object":"chat.completion","model":"upstream-chat","choices":[{"index":0,"message":{"role":"assistant","content":"{\"ok\":true}"},"finish_reason":"stop"}]}`)
		} else {
			_, _ = io.WriteString(w, `{"id":"chat","object":"chat.completion","model":"upstream-chat","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"answer","arguments":"{\"ok\":true}"}}]},"finish_reason":"tool_calls"}]}`)
		}
	}))
	t.Cleanup(f.upstream.Close)
	var key adminhttp.CredentialUpsert
	if err := json.Unmarshal([]byte(`{"id":"provider-key","provider":"openai","body":{"api_key":"binding-test-key"}}`), &key); err != nil {
		t.Fatal(err)
	}
	if response, err := f.credentials.CreateCredential(t.Context(), adminhttp.CreateCredentialRequestObject{Body: &key}); err != nil {
		t.Fatal(err)
	} else if _, ok := response.(adminhttp.CreateCredential200JSONResponse); !ok {
		t.Fatalf("create credential: %T", response)
	}
	tenant := adminhttp.OpenAITenantUpsert{Id: "compatible", CredentialId: key.Id, BaseUrl: new(f.upstream.URL + "/")}
	if response, err := f.tenants.CreateOpenAITenant(t.Context(), adminhttp.CreateOpenAITenantRequestObject{Body: &tenant}); err != nil {
		t.Fatal(err)
	} else if _, ok := response.(adminhttp.CreateOpenAITenant200JSONResponse); !ok {
		t.Fatalf("create tenant: %T", response)
	}
	if err := json.Unmarshal([]byte(`{"id":"shared-model","kind":"llm","source":"manual","provider":{"kind":"openai-tenant","id":"compatible"},"provider_data":{"upstream_model":"upstream-chat","support_json_output":true,"support_tool_calls":true,"request_bindings":[{"target":{"kind":"http_header","name":"X-AI-VID"},"source":{"peer_runtime_kv":{"key":"identity","pointer":"/vid"}}},{"target":{"kind":"http_header","name":"X-AI-UID"},"source":{"peer_runtime_kv":{"key":"identity","pointer":"/uid"}}},{"target":{"kind":"http_header","name":"X-AI-IP"},"source":{"peer_runtime_kv":{"key":"identity","pointer":"/ip"}}}]}}`), &f.upsert); err != nil {
		t.Fatal(err)
	}
	if response, err := f.models.CreateModel(t.Context(), adminhttp.CreateModelRequestObject{Body: &f.upsert}); err != nil {
		t.Fatal(err)
	} else if _, ok := response.(adminhttp.CreateModel200JSONResponse); !ok {
		t.Fatalf("create model: %T", response)
	}
	f.profile.Spec.Resources.Models = &map[string]apitypes.RuntimeProfileBinding{"chat": {ResourceId: f.upsert.Id}}
	return f
}

func (f *modelBindingFixture) resources(peer giznet.PublicKey) *peerresource.Server {
	return &peerresource.Server{Caller: peer, Models: f.models, RuntimeProfile: func() *apitypes.RuntimeProfile { return &f.profile }}
}

func (f *modelBindingFixture) service(peer giznet.PublicKey) *peergenx.Service {
	return peergenx.New(peergenx.Service{
		Models: f.resources(peer), Credentials: f.credentials, ProviderTenants: f.tenants,
		ReportedKV: f.runtime, Builder: peergenx.DefaultBuilder{HTTPClient: f.upstream.Client()},
	})
}

func (f *modelBindingFixture) report(ctx context.Context, peer giznet.PublicKey, raw string) error {
	kv := &telemetrypb.KVObservation{Key: "identity"}
	if raw == "" {
		kv.Delete = new(true)
	} else {
		kv.JsonValue = &raw
	}
	frame := &telemetrypb.TelemetryFrame{ObservedAtUnixMs: 10000 + f.clock.Add(1), Observations: []*telemetrypb.Observation{{Body: &telemetrypb.Observation_Kv{Kv: kv}}}}
	packet, err := proto.Marshal(frame)
	if err != nil {
		return err
	}
	telemetry := &peertelemetry.Service{Status: peertelemetry.StatusSync{Store: f.runtime}, Now: func() time.Time { return time.Unix(100, 0) }}
	return telemetry.ReportPacket(ctx, peer, packet)
}

func (f *modelBindingFixture) bindings(t *testing.T) *apitypes.ModelRequestBindings {
	t.Helper()
	data, err := f.upsert.ProviderData.AsOpenAITenantModelProviderData()
	if err != nil {
		t.Fatal(err)
	}
	return data.RequestBindings
}

func (f *modelBindingFixture) setBindings(t *testing.T, bindings *apitypes.ModelRequestBindings) {
	t.Helper()
	data, err := f.upsert.ProviderData.AsOpenAITenantModelProviderData()
	if err != nil {
		t.Fatal(err)
	}
	data.RequestBindings = bindings
	if err := f.upsert.ProviderData.FromOpenAITenantModelProviderData(data); err != nil {
		t.Fatal(err)
	}
}

func (f *modelBindingFixture) update(t *testing.T) {
	t.Helper()
	response, err := f.models.PutModel(t.Context(), adminhttp.PutModelRequestObject{Id: f.upsert.Id, Body: &f.upsert})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.PutModel200JSONResponse); !ok {
		t.Fatalf("update model: %T", response)
	}
}

func (f *modelBindingFixture) nextRequest(t *testing.T) modelBindingRequest {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case request := <-f.requests:
		return request
	case <-timer.C:
		t.Fatal("provider request was not sent")
		return modelBindingRequest{}
	}
}

func modelBindingInput() genx.ModelContext {
	var input genx.ModelContextBuilder
	input.UserText("user", "hello")
	return input.Build()
}

func consumeModelBindingStream(stream genx.Stream, err error) error {
	if err != nil {
		return err
	}
	defer stream.Close()
	for {
		chunk, err := stream.Next()
		if chunk != nil {
			if cause := genx.StreamError(chunk.Ctrl); cause != nil {
				return cause
			}
		}
		if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func TestModelRequestBindingsConcurrentPeerSnapshots(t *testing.T) {
	f := newModelBindingFixture(t)
	a, b, owner := giznet.PublicKey{1}, giznet.PublicKey{2}, giznet.PublicKey{3}
	for peer, value := range map[giznet.PublicKey]string{a: `{"vid":"product-value-a","uid":"device-value-a","ip":"192.0.2.1"}`, b: `{"vid":"product-value-b","uid":"device-value-b","ip":"192.0.2.2"}`, owner: `{"vid":"wrong-owner","uid":"wrong-owner","ip":"192.0.2.3"}`} {
		if err := f.report(t.Context(), peer, value); err != nil {
			t.Fatal(err)
		}
	}
	service := f.service(owner)
	// A single concrete Generator is constructed without a caller and reused.
	cfg, err := service.ResolveGenerator(t.Context(), "model/chat")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(cfg); err != nil {
		t.Fatalf("process-local resolver broke config serialization: %v", err)
	}
	generator, err := (peergenx.DefaultBuilder{HTTPClient: f.upstream.Client()}).BuildGenerator(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 12 {
		for _, peer := range []giznet.PublicKey{a, b} {
			workers.Go(func() {
				ctx := ownership.WithCaller(ownership.WithOwner(t.Context(), owner.String()), peer)
				if err := consumeModelBindingStream(generator.GenerateStream(ctx, "", modelBindingInput())); err != nil {
					t.Error(err)
				}
			})
		}
	}
	workers.Wait()
	counts := map[string]int{}
	for range 24 {
		r := f.nextRequest(t)
		uid := r.headers.Get("X-AI-UID")
		counts[uid]++
		suffix := strings.TrimPrefix(uid, "device-value-")
		if r.headers.Get("X-AI-VID") != "product-value-"+suffix || suffix != "a" && suffix != "b" {
			t.Fatalf("mixed Peer headers: %v", r.headers)
		}
		if suffix == "a" && r.headers.Get("X-AI-IP") != "192.0.2.1" || suffix == "b" && r.headers.Get("X-AI-IP") != "192.0.2.2" {
			t.Fatal("IP came from another Peer")
		}
	}
	if counts["device-value-a"] != 12 || counts["device-value-b"] != 12 {
		t.Fatalf("request distribution: %v", counts)
	}
	if err := f.report(t.Context(), a, `{"vid":"product-value-new","uid":"device-value-new","ip":"192.0.2.4"}`); err != nil {
		t.Fatal(err)
	}
	if err := consumeModelBindingStream(generator.GenerateStream(ownership.WithCaller(t.Context(), a), "", modelBindingInput())); err != nil {
		t.Fatal(err)
	}
	if got := f.nextRequest(t).headers.Get("X-AI-UID"); got != "device-value-new" {
		t.Fatalf("long-lived generator retained old value: %q", got)
	}
	if err := f.report(t.Context(), a, ""); err != nil {
		t.Fatal(err)
	}
	if err := consumeModelBindingStream(generator.GenerateStream(ownership.WithCaller(t.Context(), a), "", modelBindingInput())); err == nil || len(f.requests) != 0 {
		t.Fatal("deleted identity was reused")
	}
}

func TestModelRequestBindingsFreezeRetriesAndFallbacks(t *testing.T) {
	for _, mode := range []string{"stream_fallback", "invoke_fallback", "sdk_retry"} {
		t.Run(mode, func(t *testing.T) {
			f := newModelBindingFixture(t)
			peer := giznet.PublicKey{1}
			if err := f.report(t.Context(), peer, `{"vid":"product-value-before","uid":"device-value-before","ip":"192.0.2.1"}`); err != nil {
				t.Fatal(err)
			}
			var attempts atomic.Int32
			f.onRequest = func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
				if attempts.Add(1) != 1 {
					return false
				}
				if err := f.report(r.Context(), peer, `{"vid":"product-value-after","uid":"device-value-after","ip":"192.0.2.2"}`); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "application/json")
				if mode == "sdk_retry" {
					w.Header().Set("Retry-After-Ms", "1")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = io.WriteString(w, `{"error":{"message":"retry"}}`)
				} else {
					w.WriteHeader(http.StatusBadRequest)
					parameter := "stream_options"
					if mode == "invoke_fallback" {
						parameter = "response_format"
					}
					_, _ = fmt.Fprintf(w, `{"error":{"message":%q,"param":%q}}`, parameter+" unsupported", parameter)
				}
				return true
			}
			g := f.service(peer).Generator()
			ctx := ownership.WithCaller(t.Context(), peer)
			if mode == "invoke_fallback" {
				_, call, err := g.Invoke(ctx, "model/chat", modelBindingInput(), genx.MustNewFuncTool[struct {
					OK bool `json:"ok"`
				}]("answer", "answer"))
				if err != nil || call == nil || call.Arguments != `{"ok":true}` {
					t.Fatalf("Invoke: %v, %v", call, err)
				}
			} else if err := consumeModelBindingStream(g.GenerateStream(ctx, "model/chat", modelBindingInput())); err != nil {
				t.Fatal(err)
			}
			first, retry := f.nextRequest(t), f.nextRequest(t)
			for _, name := range []string{"X-AI-VID", "X-AI-UID", "X-AI-IP"} {
				if first.headers.Get(name) != retry.headers.Get(name) {
					t.Fatalf("retry changed %s", name)
				}
			}
			if mode == "stream_fallback" && (first.body["stream_options"] == nil || retry.body["stream_options"] != nil) {
				t.Fatal("did not exercise stream compatibility fallback")
			}
			if mode == "invoke_fallback" && (first.body["response_format"] == nil || retry.body["response_format"] != nil || retry.body["tools"] == nil) {
				t.Fatal("did not exercise structured output fallback")
			}
			if err := consumeModelBindingStream(g.GenerateStream(ctx, "model/chat", modelBindingInput())); err != nil {
				t.Fatal(err)
			}
			if r := f.nextRequest(t); r.headers.Get("X-AI-UID") != "device-value-after" || r.headers.Get("X-AI-VID") != "product-value-after" {
				t.Fatal("new request did not read the updated snapshot")
			}
		})
	}
}

func TestModelRequestBindingsRejectInvalidValuesAndOmitOptional(t *testing.T) {
	f := newModelBindingFixture(t)
	peer := giznet.PublicKey{1}
	f.setBindings(t, &apitypes.ModelRequestBindings{(*f.bindings(t))[1]})
	f.update(t)
	g := f.service(peer).Generator()
	ctx := ownership.WithCaller(t.Context(), peer)
	for _, value := range []string{`{}`, `{"uid":null}`, `{"uid":0}`, `{"uid":false}`, `{"uid":[]}`, `{"uid":{}}`, `{"uid":""}`, `{"uid":" secret "}`, `{"uid":"secret\r\nAuthorization: injected"}`, `{"uid":"设备"}`, `{"uid":"first","uid":"second"}`} {
		if err := f.report(ctx, peer, value); err != nil {
			t.Fatal(err)
		}
		_, err := g.GenerateStream(ctx, "model/chat", modelBindingInput())
		if err == nil || len(f.requests) != 0 || strings.Contains(err.Error(), "secret") {
			t.Fatalf("invalid value accepted or leaked: %v", err)
		}
	}
	bindings := f.bindings(t)
	(*bindings)[0].Required = new(false)
	f.setBindings(t, bindings)
	f.update(t)
	for _, value := range []string{`{}`, `{"uid":null}`, ""} {
		if err := f.report(ctx, peer, value); err != nil {
			t.Fatal(err)
		}
		if err := consumeModelBindingStream(g.GenerateStream(ctx, "model/chat", modelBindingInput())); err != nil {
			t.Fatal(err)
		}
		if _, present := f.nextRequest(t).headers["X-Ai-Uid"]; present {
			t.Fatal("absent optional header was sent")
		}
	}
	if err := f.report(ctx, peer, `{"uid":false}`); err != nil {
		t.Fatal(err)
	}
	if _, err := g.GenerateStream(ctx, "model/chat", modelBindingInput()); err == nil {
		t.Fatal("optional binding hid an invalid present value")
	}
	if _, err := g.GenerateStream(t.Context(), "model/chat", modelBindingInput()); err == nil {
		t.Fatal("optional bindings accepted a missing caller")
	}
	for _, value := range []string{`{"uid":"0"}`, `{"uid":"false"}`} {
		if err := f.report(ctx, peer, value); err != nil {
			t.Fatal(err)
		}
		if err := consumeModelBindingStream(g.GenerateStream(ctx, "model/chat", modelBindingInput())); err != nil {
			t.Fatal(err)
		}
		f.nextRequest(t)
	}
}

func TestModelRequestBindingsJSONPointerAndHeaderBudget(t *testing.T) {
	f := newModelBindingFixture(t)
	peer := giznet.PublicKey{1}
	f.setBindings(t, &apitypes.ModelRequestBindings{(*f.bindings(t))[1]})
	bindings := f.bindings(t)
	binding := &(*bindings)[0]
	source, err := binding.Source.AsModelRequestPeerRuntimeKVSource()
	if err != nil {
		t.Fatal(err)
	}
	source.PeerRuntimeKv.Pointer = new("/a~1b/~0/0/")
	if err := binding.Source.FromModelRequestPeerRuntimeKVSource(source); err != nil {
		t.Fatal(err)
	}
	f.setBindings(t, bindings)
	f.update(t)
	ctx := ownership.WithCaller(t.Context(), peer)
	if err := f.report(ctx, peer, `{"a/b":{"~":[{"":"device-value-pointer"}]}}`); err != nil {
		t.Fatal(err)
	}
	g := f.service(peer).Generator()
	if err := consumeModelBindingStream(g.GenerateStream(ctx, "model/chat", modelBindingInput())); err != nil {
		t.Fatal(err)
	}
	if f.nextRequest(t).headers.Get("X-AI-UID") != "device-value-pointer" {
		t.Fatal("JSON Pointer was not resolved exactly")
	}
	declarations := apitypes.ModelRequestBindings{}
	for i := range 16 {
		var source apitypes.ModelRequestSource
		if err := source.FromModelRequestPeerRuntimeKVSource(apitypes.ModelRequestPeerRuntimeKVSource{PeerRuntimeKv: apitypes.ModelRequestPeerRuntimeKVSelector{Key: "identity"}}); err != nil {
			t.Fatal(err)
		}
		declarations = append(declarations, apitypes.ModelRequestBinding{Source: source, Target: apitypes.ModelRequestTarget{Kind: apitypes.ModelRequestTargetKindHttpHeader, Name: fmt.Sprintf("X-Metadata-%d", i)}})
	}
	f.setBindings(t, &declarations)
	f.update(t)
	if err := f.report(ctx, peer, `"`+strings.Repeat("x", 600)+`"`); err != nil {
		t.Fatal(err)
	}
	if _, err := g.GenerateStream(ctx, "model/chat", modelBindingInput()); err == nil || len(f.requests) != 0 {
		t.Fatal("combined header budget was not enforced before sending")
	}
}

func TestModelRequestBindingsUnconfiguredAndDeniedModels(t *testing.T) {
	f := newModelBindingFixture(t)
	peer := giznet.PublicKey{1}
	service := f.service(peer)
	service.ReportedKV = nil
	ctx := ownership.WithCaller(t.Context(), peer)
	if _, err := service.Generator().GenerateStream(ctx, "model/chat", modelBindingInput()); err == nil {
		t.Fatal("configured bindings ignored an unavailable Runtime")
	}
	f.setBindings(t, nil)
	f.update(t)
	if err := consumeModelBindingStream(service.Generator().GenerateStream(t.Context(), "model/chat", modelBindingInput())); err != nil {
		t.Fatal(err)
	}
	for name := range f.nextRequest(t).headers {
		if strings.HasPrefix(strings.ToLower(name), "x-ai-") {
			t.Fatal("unconfigured model inherited metadata")
		}
	}
	f.profile.Spec.Resources.Models = nil
	if _, err := service.Generator().GenerateStream(ctx, "model/chat", modelBindingInput()); err == nil || len(f.requests) != 0 {
		t.Fatal("Runtime data granted model access")
	}
}

func TestModelRequestBindingsVerifiedHTTPCaller(t *testing.T) {
	f := newModelBindingFixture(t)
	peer := giznet.PublicKey{1}
	if err := f.report(t.Context(), peer, `{"vid":"product-value-http","uid":"device-value-http","ip":"192.0.2.1"}`); err != nil {
		t.Fatal(err)
	}
	peerService := &PeerService{manager: &Manager{}}
	handler := peerService.openAIHTTPHandlerForPeer(peer, f.service(peer), f.resources(peer))
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"chat","messages":[{"role":"user","content":"hello"}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-AI-UID", "forged-device")
	request = request.WithContext(ownership.WithCaller(request.Context(), giznet.PublicKey{9}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	if got := f.nextRequest(t).headers.Get("X-AI-UID"); got != "device-value-http" {
		t.Fatalf("trusted caller was replaced: %q", got)
	}
}

type modelBindingAgentResolver struct{ spec agenthost.Spec }

func (r modelBindingAgentResolver) Resolve(context.Context, string) (agenthost.Spec, error) {
	return r.spec, nil
}

type modelBindingAgent struct{ generator genx.Generator }

func (a modelBindingAgent) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	_ = input.Close()
	return a.generator.GenerateStream(ctx, "model/chat", modelBindingInput())
}

func TestModelRequestBindingsSharedOwnerScopedWorkspace(t *testing.T) {
	f := newModelBindingFixture(t)
	a, b, owner := giznet.PublicKey{1}, giznet.PublicKey{2}, giznet.PublicKey{3}
	for peer, uid := range map[giznet.PublicKey]string{a: "device-value-a", b: "device-value-b", owner: "wrong-owner"} {
		if err := f.report(t.Context(), peer, fmt.Sprintf(`{"vid":"product","uid":%q,"ip":"192.0.2.1"}`, uid)); err != nil {
			t.Fatal(err)
		}
	}
	manager := &Manager{Models: f.models, Credentials: f.credentials, ProviderTenants: f.tenants, PeerRun: f.runtime}
	service, err := manager.ownerGenXForProfile(t.Context(), owner.String(), f.profile)
	if err != nil {
		t.Fatal(err)
	}
	spec := agenthost.Spec{Workspace: apitypes.Workspace{Id: "shared-workspace", Name: "shared", OwnerPublicKey: new(owner.String())}, AgentType: "bound-chat"}
	host := agenthost.New(modelBindingAgentResolver{spec: spec})
	var builds atomic.Int32
	if err := host.Register(spec.AgentType, agenthost.FactoryFunc(func(context.Context, agenthost.Spec) (genx.Transformer, error) {
		builds.Add(1)
		return modelBindingAgent{generator: service.Generator()}, nil
	})); err != nil {
		t.Fatal(err)
	}
	ctxA := ownership.WithOwner(ownership.WithCaller(t.Context(), a), owner.String())
	ctxB := ownership.WithOwner(ownership.WithCaller(t.Context(), b), owner.String())
	first, releaseA, err := host.OpenAgent(ctxA, "shared")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseA()
	second, releaseB, err := host.OpenAgent(ctxB, "shared")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseB()
	if first != second || builds.Load() != 1 {
		t.Fatal("fixture did not reuse one Workspace Agent")
	}
	for i, ctx := range []context.Context{ctxA, ctxB} {
		if err := consumeModelBindingStream(host.Transform(ctx, "shared", agenthost.NewInputStream(1))); err != nil {
			t.Fatal(err)
		}
		want := []string{"device-value-a", "device-value-b"}[i]
		if got := f.nextRequest(t).headers.Get("X-AI-UID"); got != want {
			t.Fatalf("shared owner-scoped Agent used %q, want %q", got, want)
		}
	}
}

func TestModelRequestBindingsDoNotFollowRedirects(t *testing.T) {
	f := newModelBindingFixture(t)
	peer := giznet.PublicKey{1}
	if err := f.report(t.Context(), peer, `{"vid":"product","uid":"device-value-private","ip":"192.0.2.1"}`); err != nil {
		t.Fatal(err)
	}
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked.Store(true) }))
	defer other.Close()
	f.onRequest = func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
		return true
	}
	shared := f.upstream.Client()
	service := f.service(peer)
	service.Builder = peergenx.DefaultBuilder{HTTPClient: shared}
	err := consumeModelBindingStream(service.Generator().GenerateStream(ownership.WithCaller(t.Context(), peer), "model/chat", modelBindingInput()))
	if err == nil || leaked.Load() || shared.CheckRedirect != nil {
		t.Fatal("redirect leaked metadata or changed the shared HTTP client")
	}
}

type modelBindingSnapshotGetter struct {
	store *peerrun.Server
	reads atomic.Int32
	after func(context.Context) error
}

func (g *modelBindingSnapshotGetter) GetReportedKV(ctx context.Context, peer giznet.PublicKey) (*apitypes.PeerReportedKV, error) {
	snapshot, err := g.store.GetReportedKV(ctx, peer)
	if err == nil && g.reads.Add(1) == 1 && g.after != nil {
		err = g.after(ctx)
	}
	return snapshot, err
}

func TestModelRequestBindingsReadOneSnapshotPerInvocation(t *testing.T) {
	f := newModelBindingFixture(t)
	peer := giznet.PublicKey{1}
	if err := f.report(t.Context(), peer, `{"vid":"before","uid":"before","ip":"192.0.2.1"}`); err != nil {
		t.Fatal(err)
	}
	getter := &modelBindingSnapshotGetter{store: f.runtime, after: func(ctx context.Context) error {
		return f.report(ctx, peer, `{"vid":"after","uid":"after","ip":"192.0.2.2"}`)
	}}
	service := f.service(peer)
	service.ReportedKV = getter
	// Listing and capability resolution do not require a caller or its data.
	if _, err := service.ListAccessibleGeneratorConfigs(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SupportsToolCalls(t.Context(), "model/chat"); err != nil || getter.reads.Load() != 0 {
		t.Fatal("capability resolution read Runtime data")
	}
	ctx := ownership.WithCaller(t.Context(), peer)
	if err := consumeModelBindingStream(service.Generator().GenerateStream(ctx, "model/chat", modelBindingInput())); err != nil {
		t.Fatal(err)
	}
	r := f.nextRequest(t)
	if getter.reads.Load() != 1 || r.headers.Get("X-AI-VID") != "before" || r.headers.Get("X-AI-UID") != "before" || r.headers.Get("X-AI-IP") != "192.0.2.1" {
		t.Fatal("one invocation mixed committed Runtime snapshots")
	}
	if err := consumeModelBindingStream(service.Generator().GenerateStream(ctx, "model/chat", modelBindingInput())); err != nil {
		t.Fatal(err)
	}
	r = f.nextRequest(t)
	if getter.reads.Load() != 2 || r.headers.Get("X-AI-VID") != "after" || r.headers.Get("X-AI-UID") != "after" {
		t.Fatal("next invocation reused the old snapshot")
	}
	getter.after = nil
	getter.reads.Store(0)
	f.profile.Spec.Resources.Models = nil
	if _, err := service.Generator().GenerateStream(ctx, "model/chat", modelBindingInput()); err == nil || getter.reads.Load() != 0 {
		t.Fatal("denied model read Runtime data")
	}
}

func TestModelRequestBindingsHTTPErrorIsSafe(t *testing.T) {
	f := newModelBindingFixture(t)
	peer := giznet.PublicKey{1}
	if err := f.report(t.Context(), peer, `{"uid":"private\r\nheader"}`); err != nil {
		t.Fatal(err)
	}
	service := &PeerService{manager: &Manager{}}
	handler := service.openAIHTTPHandlerForPeer(peer, f.service(peer), f.resources(peer))
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"chat","messages":[{"role":"user","content":"hello"}]}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "model_request_binding_missing") || strings.Contains(w.Body.String(), "private") || len(f.requests) != 0 {
		t.Fatalf("unsafe or unclassified binding error: %d %s", w.Code, w.Body.String())
	}
}

func TestModelRequestBindingsProviderErrorsDoNotExposeMetadata(t *testing.T) {
	for _, mode := range []string{"stream", "invoke"} {
		t.Run(mode, func(t *testing.T) {
			f := newModelBindingFixture(t)
			peer := giznet.PublicKey{1}
			if err := f.report(t.Context(), peer, `{"vid":"product","uid":"device-value-private","ip":"192.0.2.1"}`); err != nil {
				t.Fatal(err)
			}
			f.onRequest = func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "rejected " + r.Header.Get("X-AI-UID") + " " + r.Header.Get("Authorization"), "param": "metadata"}})
				return true
			}
			g := f.service(peer).Generator()
			ctx := ownership.WithCaller(t.Context(), peer)
			var err error
			if mode == "stream" {
				err = consumeModelBindingStream(g.GenerateStream(ctx, "model/chat", modelBindingInput()))
			} else {
				_, _, err = g.Invoke(ctx, "model/chat", modelBindingInput(), genx.MustNewFuncTool[struct {
					OK bool `json:"ok"`
				}]("answer", "answer"))
			}
			if err == nil || strings.Contains(err.Error(), "device-value-private") || strings.Contains(err.Error(), "binding-test-key") {
				t.Fatalf("provider error leaked request metadata: %v", err)
			}
			code, message, _, ok := genx.PublicErrorDetails(err)
			if !ok || code != "MODEL_PROVIDER_ERROR" || strings.Contains(message, "private") {
				t.Fatal("provider failure did not preserve safe typed details")
			}
		})
	}
}

func TestModelRequestBindingsAuthorizeBeforeRuntimeRead(t *testing.T) {
	f := newModelBindingFixture(t)
	peer := giznet.PublicKey{1}
	getter := &modelBindingSnapshotGetter{store: f.runtime}
	service := f.service(peer)
	service.ReportedKV = getter
	denied := errors.New("call not authorized")
	service.Authorize = func(context.Context) (context.Context, func(), error) { return nil, nil, denied }
	_, err := service.Generator().GenerateStream(ownership.WithCaller(t.Context(), peer), "model/chat", modelBindingInput())
	if !errors.Is(err, denied) || getter.reads.Load() != 0 || len(f.requests) != 0 {
		t.Fatal("unauthorized call read Runtime or contacted provider")
	}
}
