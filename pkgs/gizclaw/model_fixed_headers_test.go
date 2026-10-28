package gizclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func addFixedModelHeader(t *testing.T, f *modelBindingFixture, name, value string) {
	t.Helper()
	bindings := f.bindings(t)
	if bindings == nil {
		bindings = new(apitypes.ModelRequestBindings)
	}
	var binding apitypes.ModelRequestBinding
	data, err := json.Marshal(map[string]any{"target": map[string]any{"kind": "http_header", "name": name}, "source": map[string]any{"constant": value}})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &binding); err != nil {
		t.Fatal(err)
	}
	*bindings = append(*bindings, binding)
	f.setBindings(t, bindings)
	f.update(t)
}

func putFixedCredentialHeaders(t *testing.T, f *modelBindingFixture, headers map[string]string) {
	t.Helper()
	if err := f.putFixedCredentialHeaders(t.Context(), headers); err != nil {
		t.Fatal(err)
	}
}

func (f *modelBindingFixture) putFixedCredentialHeaders(ctx context.Context, headers map[string]string) error {
	var body adminhttp.CredentialUpsert
	raw, err := json.Marshal(map[string]any{"id": "provider-key", "provider": "openai", "body": map[string]any{"api_key": "binding-test-key", "headers": headers}})
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	response, err := f.credentials.PutCredential(ctx, adminhttp.PutCredentialRequestObject{Id: "provider-key", Body: &body})
	if err != nil {
		return err
	}
	if _, ok := response.(adminhttp.PutCredential200JSONResponse); !ok {
		return fmt.Errorf("credential update returned %T", response)
	}
	return nil
}

func TestModelFixedHeadersWithoutRuntime(t *testing.T) {
	f := newModelBindingFixture(t)
	f.setBindings(t, nil)
	addFixedModelHeader(t, f, "X-GizProxy-Upstream-Host", "vendor.example:30100")
	putFixedCredentialHeaders(t, f, map[string]string{"X-GizProxy-Authorization": "Bearer local-proxy-fixture"})
	service := f.service(giznet.PublicKey{1})
	service.ReportedKV = nil
	if err := consumeModelBindingStream(service.Generator().GenerateStream(t.Context(), "model/chat", modelBindingInput())); err != nil {
		t.Fatal(err)
	}
	r := f.nextRequest(t)
	if r.headers.Get("X-GizProxy-Upstream-Host") != "vendor.example:30100" || r.headers.Get("X-GizProxy-Authorization") != "Bearer local-proxy-fixture" || r.headers.Get("Authorization") != "Bearer binding-test-key" {
		t.Fatal("fixed headers or upstream authorization were not preserved")
	}
	_, call, err := service.Generator().Invoke(t.Context(), "model/chat", modelBindingInput(), genx.MustNewFuncTool[struct {
		OK bool `json:"ok"`
	}]("answer", "answer"))
	if err != nil || call == nil || f.nextRequest(t).headers.Get("X-GizProxy-Authorization") != "Bearer local-proxy-fixture" {
		t.Fatalf("fixed Invoke headers: %v", err)
	}
	response, err := f.resources(giznet.PublicKey{1}).GetModel(t.Context(), adminhttp.GetModelRequestObject{Id: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.GetModel200JSONResponse); !ok {
		t.Fatalf("Peer Model descriptor returned %T", response)
	}
	encoded, err := json.Marshal(response)
	if err != nil || strings.Contains(string(encoded), "local-proxy-fixture") {
		t.Fatal("private credential header leaked into a Peer Model descriptor")
	}
}

func TestCredentialOnlyHeadersProtectErrorsAndRedirects(t *testing.T) {
	for _, mode := range []string{"error", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			f := newModelBindingFixture(t)
			f.setBindings(t, nil)
			f.update(t)
			putFixedCredentialHeaders(t, f, map[string]string{"X-GizProxy-Authorization": "Bearer private-proxy-fixture"})
			var redirected atomic.Int32
			destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
			defer destination.Close()
			f.onRequest = func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
				if mode == "redirect" {
					http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
				} else {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": r.Header.Get("X-GizProxy-Authorization")}})
				}
				return true
			}
			service := f.service(giznet.PublicKey{1})
			service.ReportedKV = nil
			err := consumeModelBindingStream(service.Generator().GenerateStream(t.Context(), "model/chat", modelBindingInput()))
			if err == nil || strings.Contains(err.Error(), "private-proxy-fixture") || redirected.Load() != 0 {
				t.Fatalf("unsafe fixed-header failure: %v", err)
			}
		})
	}
}

func TestFixedHeaderConflictAndCombinedBudgets(t *testing.T) {
	for _, mode := range []string{"overlap", "bytes", "count"} {
		t.Run(mode, func(t *testing.T) {
			f := newModelBindingFixture(t)
			f.setBindings(t, nil)
			private := map[string]string{}
			switch mode {
			case "overlap":
				addFixedModelHeader(t, f, "X-Proxy-Token", "model-value")
				private["x-proxy-token"] = "credential-value"
			case "bytes":
				for i := range 5 {
					addFixedModelHeader(t, f, "X-Public-"+string(rune('A'+i)), strings.Repeat("x", 1000))
					private["X-Private-"+string(rune('A'+i))] = strings.Repeat("y", 1000)
				}
			case "count":
				for i := range 16 {
					addFixedModelHeader(t, f, "X-Public-"+string(rune('A'+i)), "value")
				}
				private["X-Private"] = "value"
			}
			putFixedCredentialHeaders(t, f, private)
			if err := consumeModelBindingStream(f.service(giznet.PublicKey{1}).Generator().GenerateStream(context.Background(), "model/chat", modelBindingInput())); err == nil || len(f.requests) != 0 {
				t.Fatal("overlapping or over-budget headers reached the provider")
			}
		})
	}
}

func TestFixedHeadersAndRuntimeValues(t *testing.T) {
	f := newModelBindingFixture(t)
	peer := giznet.PublicKey{1}
	addFixedModelHeader(t, f, "X-GizProxy-Upstream-Host", "vendor.example:30100")
	putFixedCredentialHeaders(t, f, map[string]string{"X-GizProxy-Authorization": "Bearer mixed-proxy-fixture"})
	if err := f.report(t.Context(), peer, `{"vid":"product-value-a","uid":"device-value-a","ip":"192.0.2.1"}`); err != nil {
		t.Fatal(err)
	}
	if err := consumeModelBindingStream(f.service(peer).Generator().GenerateStream(ownership.WithCaller(t.Context(), peer), "model/chat", modelBindingInput())); err != nil {
		t.Fatal(err)
	}
	r := f.nextRequest(t)
	if r.headers.Get("X-AI-UID") != "device-value-a" || r.headers.Get("X-GizProxy-Upstream-Host") != "vendor.example:30100" || r.headers.Get("X-GizProxy-Authorization") != "Bearer mixed-proxy-fixture" {
		t.Fatal("fixed and Runtime header sources did not compose")
	}
}
