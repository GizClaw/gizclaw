package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerquota"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerusage"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/quota"
	"github.com/jmoiron/sqlx"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type quotaReportProbe struct{ calls atomic.Int32 }

func (p *quotaReportProbe) QuotaReport(context.Context, giznet.PublicKey) (quota.QuotaRequest, error) {
	p.calls.Add(1)
	return quota.QuotaRequest{}, errors.New("reporting dependencies are unavailable")
}

func managerQuotaBinding(t *testing.T, data string) *apitypes.RuntimeProfileQuota {
	t.Helper()
	var binding *apitypes.RuntimeProfileQuota
	if err := json.Unmarshal([]byte(data), &binding); err != nil {
		t.Fatal(err)
	}
	return binding
}

func TestUnlimitedQuotaAllowsProviderWithIndependentUsage(t *testing.T) {
	for _, policy := range []string{"null", `{"type":"unlimited"}`} {
		for _, persist := range []bool{false, true} {
			t.Run(policy+map[bool]string{false: "/no-usage", true: "/sql-usage"}[persist], func(t *testing.T) {
				var providerCalls atomic.Int32
				provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					providerCalls.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, `data: {"id":"fixture","object":"chat.completion.chunk","created":1,"model":"billing-chat","choices":[{"index":0,"delta":{"content":"allowed"},"finish_reason":""}]}`+"\n\n"+`data: {"id":"fixture","object":"chat.completion.chunk","created":1,"model":"billing-chat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n"+`data: {"id":"fixture","object":"chat.completion.chunk","created":1,"model":"billing-chat","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":4,"total_tokens":9}}`+"\n\n"+"data: [DONE]\n\n")
				}))
				defer provider.Close()
				probe := &quotaReportProbe{}
				controller := peerquota.New(probe)
				defer func() {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					if err := controller.Close(ctx); err != nil {
						t.Error(err)
					}
				}()
				manager := &Manager{}
				if persist {
					manager.PeerQuota = controller
				}
				peer := giznet.PublicKey{42}
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				if persist {
					db := sqlx.MustOpen("sqlite", ":memory:")
					db.SetMaxOpenConns(1)
					defer db.Close()
					store, err := peerusage.NewStore(ctx, db)
					if err != nil {
						t.Fatal(err)
					}
					manager.PeerUsage = peerusage.NewRecorder(store)
					ctx = genx.WithUsageRecorder(ctx, manager.PeerUsage.Handler(peer))
				}
				profile := apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{Quota: managerQuotaBinding(t, policy)}}
				callCtx, release, err := manager.quotaAuthorizer(peer, func(context.Context) (apitypes.RuntimeProfile, error) { return profile, nil })(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				client := openai.NewClient(option.WithBaseURL(provider.URL+"/"), option.WithAPIKey("fixture"))
				generator := &genx.OpenAIGenerator{Client: &client, Model: "billing-chat"}
				var input genx.ModelContextBuilder
				input.UserText("user", "hello")
				stream, err := generator.GenerateStream(callCtx, "", input.Build())
				if err != nil {
					t.Fatal(err)
				}
				defer stream.Close()
				var text string
				for {
					chunk, err := stream.Next()
					if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					if chunk != nil {
						if part, ok := chunk.Part.(genx.Text); ok {
							text += string(part)
						}
					}
				}
				if text != "allowed" || providerCalls.Load() != 1 || probe.calls.Load() != 0 {
					t.Fatalf("text=%q provider calls=%d quota reports=%d", text, providerCalls.Load(), probe.calls.Load())
				}
				if persist {
					rows, err := manager.PeerUsage.Hourly(ctx, peer)
					if err != nil || len(rows) != 1 || rows[0].ModelID != "billing-chat" || rows[0].Quantity != 9 {
						t.Fatalf("independent SQL usage=%v error=%v", rows, err)
					}
				}
			})
		}
	}
}

func TestConfiguredOrMalformedQuotaCannotBypassMissingController(t *testing.T) {
	for _, policy := range []string{`{"type":"custom","endpoint":"http://quota.example.test/v1/quota","api_key":"test"}`, `{}`, `{"type":"custom"}`, `{"type":"unknown"}`, `{"type":"unlimited","api_key":"test"}`} {
		profile := apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{Quota: managerQuotaBinding(t, policy)}}
		_, _, err := (&Manager{}).quotaAuthorizer(giznet.PublicKey{43}, func(context.Context) (apitypes.RuntimeProfile, error) { return profile, nil })(t.Context())
		if !errors.Is(err, peergenx.ErrDenied) {
			t.Fatalf("configured or malformed policy bypassed controller: %v", err)
		}
	}
}

func TestQuotaAuthorizerPreservesUnavailableCauseAndResourceFailures(t *testing.T) {
	profile := apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{Quota: managerQuotaBinding(t, `{"type":"custom","endpoint":"http://quota.example.test/v1/quota","api_key":"fixture"}`)}}
	controller := peerquota.New(&quotaReportProbe{})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = controller.Close(ctx)
	}()
	manager := &Manager{PeerQuota: controller}
	_, _, err := manager.quotaAuthorizer(giznet.PublicKey{1}, func(context.Context) (apitypes.RuntimeProfile, error) { return profile, nil })(t.Context())
	if !errors.Is(err, peerquota.ErrUnavailable) || !errors.Is(err, peergenx.ErrDenied) {
		t.Fatalf("cause was lost: %v", err)
	}
	_, _, err = manager.quotaAuthorizer(giznet.PublicKey{1}, func(context.Context) (apitypes.RuntimeProfile, error) {
		return apitypes.RuntimeProfile{}, errors.New("missing resource")
	})(t.Context())
	if _, _, _, quota := peerquota.ErrorDetails(err); quota {
		t.Fatalf("resource failure mislabeled as quota: %v", err)
	}
}
