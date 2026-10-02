package peergenx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestVolcArkServiceTierRequests(t *testing.T) {
	for _, tier := range []string{"", "fast", "auto", "default", "flex"} {
		for _, textOnly := range []bool{true, false} {
			t.Run(tier+"/text_only="+strconv.FormatBool(textOnly), func(t *testing.T) {
				requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				requests := make(chan map[string]any, 2)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/api/v3/chat/completions" {
						t.Errorf("request path = %q", r.URL.Path)
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode request: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					requests <- body
					if body["stream"] == true {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"id\":\"chat\",\"object\":\"chat.completion.chunk\",\"model\":\"doubao-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chat\",\"object\":\"chat.completion.chunk\",\"model\":\"doubao-test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"chat","object":"chat.completion","model":"doubao-test","choices":[{"index":0,"message":{"role":"assistant","content":"{\"ok\":true}"},"finish_reason":"stop"}]}`)
				}))
				defer server.Close()
				data := apitypes.VolcTenantModelProviderData{
					ApiMode:           apitypes.VolcTenantModelProviderDataApiModeChatCompletions,
					UpstreamModel:     new("doubao-test"),
					SupportTextOnly:   &textOnly,
					SupportJsonOutput: new(true),
				}
				if textOnly {
					data.ThinkingParam = new("thinking.type")
					data.DefaultThinkingLevel = new("disabled")
				}
				if tier != "" {
					data.ServiceTier = new(apitypes.VolcTenantModelProviderDataServiceTier(tier))
				}
				generator, err := (DefaultBuilder{HTTPClient: server.Client()}).BuildGenerator(requestCtx, GeneratorConfig{
					Model: apitypes.Model{Id: "chat", Kind: apitypes.ModelKindLlm, ProviderData: mustVolcModelProviderData(t, data)},
					Tenant: Tenant{Kind: "volc-tenant", Volc: &apitypes.VolcTenant{
						Id: "main", CredentialId: "volc-key", Endpoint: new(server.URL + "/api/v3"),
					}},
					Credential: apitypes.Credential{Id: "volc-key", Body: testVolcCredentialBodyFromStrings(map[string]string{"ark_api_key": "test"})},
				})
				if err != nil {
					t.Fatalf("BuildGenerator(): %v", err)
				}
				ctx := (&genx.ModelContextBuilder{}).Build()
				stream, err := generator.GenerateStream(requestCtx, "", ctx)
				if err != nil {
					t.Fatalf("GenerateStream(): %v", err)
				}
				defer stream.Close()
				for {
					_, err := stream.Next()
					if errors.Is(err, genx.ErrDone) {
						break
					}
					if err != nil {
						t.Fatalf("stream.Next(): %v", err)
					}
				}
				tool := genx.MustNewFuncTool[struct {
					OK bool `json:"ok"`
				}]("answer", "answer")
				_, call, err := generator.Invoke(requestCtx, "", ctx, tool)
				if err != nil || call == nil || call.Arguments != `{"ok":true}` {
					t.Fatalf("Invoke() = %#v, %v", call, err)
				}
				for range 2 {
					body := <-requests
					value, present := body["service_tier"]
					if tier == "" && present || tier != "" && value != tier {
						t.Errorf("service_tier = %v (present=%v), want %q", value, present, tier)
					}
					thinking, ok := body["thinking"].(map[string]any)
					if textOnly && (!ok || thinking["type"] != "disabled") {
						t.Errorf("thinking = %#v, want disabled", body["thinking"])
					}
					if !textOnly && body["thinking"] != nil {
						t.Errorf("thinking = %#v, want omitted", body["thinking"])
					}
				}
			})
		}
	}
}
