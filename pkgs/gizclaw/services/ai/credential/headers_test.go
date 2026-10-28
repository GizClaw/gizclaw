package credential

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
)

func TestCredentialFixedHeadersValidation(t *testing.T) {
	for _, row := range []struct {
		name    string
		headers map[string]string
		valid   bool
	}{
		{"proxy authentication", map[string]string{"X-GizProxy-Authorization": "Bearer protected-header-fixture"}, true},
		{"empty map", map[string]string{}, true},
		{"duplicate name", map[string]string{"X-Proxy": "one", "x-proxy": "two"}, false},
		{"upstream auth", map[string]string{"Authorization": "Bearer protected-header-fixture"}, false},
		{"host", map[string]string{"Host": "vendor.example"}, false},
		{"SDK header", map[string]string{"X-Stainless-Test": "value"}, false},
		{"newline", map[string]string{"X-Proxy": "protected-header-fixture\r\nInjected: true"}, false},
		{"empty value", map[string]string{"X-Proxy": ""}, false},
		{"surrounding spaces", map[string]string{"X-Proxy": " padded"}, false},
		{"Unicode", map[string]string{"X-Proxy": "秘密"}, false},
		{"oversized", map[string]string{"X-Proxy": strings.Repeat("v", 1025)}, false},
		{"too many", fixedHeaderSet(17, 1), false},
		{"byte budget", fixedHeaderSet(9, 1000), false},
	} {
		t.Run(row.name, func(t *testing.T) {
			server := newTestServer(t)
			raw, err := json.Marshal(map[string]any{"id": "fixed", "provider": "openai", "body": map[string]any{"api_key": "local-key-fixture", "headers": row.headers}})
			if err != nil {
				t.Fatal(err)
			}
			body := mustCredentialUpsert(t, string(raw))
			response, err := server.CreateCredential(t.Context(), adminhttp.CreateCredentialRequestObject{Body: &body})
			if err != nil {
				t.Fatal(err)
			}
			if row.valid {
				created, ok := response.(adminhttp.CreateCredential200JSONResponse)
				if !ok {
					t.Fatalf("valid headers rejected: %T", response)
				}
				data, err := created.Body.AsOpenAICredentialBody()
				if err != nil || data.Headers == nil || len(*data.Headers) != len(row.headers) {
					t.Fatal("protected fixed headers were not retained")
				}
			} else {
				if _, ok := response.(adminhttp.CreateCredential400JSONResponse); !ok {
					t.Fatalf("invalid headers accepted: %T", response)
				}
				if strings.Contains(fmt.Sprint(response), "protected-header-fixture") {
					t.Fatal("validation error exposed a credential header")
				}
			}
		})
	}
}

func fixedHeaderSet(count, size int) map[string]string {
	values := make(map[string]string, count)
	for i := range count {
		values[fmt.Sprintf("X-Fixed-%d", i)] = strings.Repeat("v", size)
	}
	return values
}
