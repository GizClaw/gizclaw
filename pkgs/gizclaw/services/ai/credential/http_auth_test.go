package credential

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
)

func TestHTTPAuthorizerUsesProviderCredentialFields(t *testing.T) {
	server := newTestServer(t)
	credentialIDs := map[string]string{}
	for _, source := range []string{
		`{"name":"volc-tools","provider":"volc","body":{"ark_api_key":"ark-secret","search_api_key":"search-secret","openapi_access_key_id":"ak","openapi_access_key":"sk"}}`,
		`{"name":"aliyun-tools","provider":"aliyun","body":{"app_code":"app-code","access_key_id":"aliyun-ak","access_key_secret":"aliyun-sk"}}`,
	} {
		body := mustCredentialUpsert(t, source)
		response, err := server.CreateCredential(t.Context(), adminhttp.CreateCredentialRequestObject{Body: &body})
		if err != nil {
			t.Fatal(err)
		}
		created, ok := response.(adminhttp.CreateCredential200JSONResponse)
		if !ok {
			t.Fatalf("CreateCredential() = %#v", response)
		}
		credentialIDs[created.Name] = created.Id
	}
	for _, test := range []struct {
		method     string
		credential string
		want       string
	}{
		{method: "volc_ark", credential: "volc-tools", want: "Bearer ark-secret"},
		{method: "volc_search", credential: "volc-tools", want: "Bearer search-secret"},
		{method: "aliyun_app_code", credential: "aliyun-tools", want: "APPCODE app-code"},
	} {
		authorizer, err := server.HTTPAuthorizer(t.Context(), HTTPAuthConfig{
			Method: test.method, Credential: credentialIDs[test.credential],
		})
		if err != nil {
			t.Fatalf("HTTPAuthorizer(%s) error = %v", test.method, err)
		}
		request, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
		if err := authorizer.Authorize(t.Context(), request); err != nil {
			t.Fatal(err)
		}
		if got := request.Header.Get("Authorization"); got != test.want {
			t.Fatalf("%s Authorization = %q, want %q", test.method, got, test.want)
		}
	}
}

func TestHTTPAuthorizerRejectsProviderAliasForVolcMethods(t *testing.T) {
	server := newTestServer(t)
	body := mustCredentialUpsert(t, `{"name":"legacy-volc","provider":"volcengine","body":{"ark_api_key":"secret"}}`)
	response, err := server.CreateCredential(t.Context(), adminhttp.CreateCredentialRequestObject{Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := response.(adminhttp.CreateCredential200JSONResponse)
	if !ok {
		t.Fatalf("CreateCredential() = %#v", response)
	}
	if _, err := server.HTTPAuthorizer(t.Context(), HTTPAuthConfig{
		Method: "volc_ark", Credential: created.Id,
	}); err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("HTTPAuthorizer() error = %v", err)
	}
}

func TestVolcAndAliyunSignersAreDeterministicAndPreserveBody(t *testing.T) {
	when := time.Date(2026, 7, 28, 1, 2, 3, 0, time.UTC)
	tests := []struct {
		name       string
		authorizer interface {
			Authorize(context.Context, *http.Request) error
		}
		prefix string
	}{
		{
			name: "volc",
			authorizer: volcOpenAPIAuthorizer{
				accessKeyID: "ak", secret: "sk", region: "cn-beijing",
				service: "search", now: func() time.Time { return when },
			},
			prefix: "HMAC-SHA256 Credential=ak/20260728/cn-beijing/search/request",
		},
		{
			name: "aliyun",
			authorizer: aliyunV3Authorizer{
				accessKeyID: "ak", secret: "sk", action: "Search",
				version: "2026-01-01", now: func() time.Time { return when },
				nonce: func() (string, error) { return "nonce", nil },
			},
			prefix: "ACS3-HMAC-SHA256 Credential=ak",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			newRequest := func() *http.Request {
				request, _ := http.NewRequest(
					http.MethodPost,
					"https://example.com/v1?q=hello+world&z=%E4%B8%AD",
					strings.NewReader(`{"value":1}`),
				)
				request.Header.Set("Content-Type", "application/json")
				return request
			}
			first := newRequest()
			if err := test.authorizer.Authorize(t.Context(), first); err != nil {
				t.Fatal(err)
			}
			second := newRequest()
			if err := test.authorizer.Authorize(t.Context(), second); err != nil {
				t.Fatal(err)
			}
			if first.Header.Get("Authorization") != second.Header.Get("Authorization") ||
				!strings.HasPrefix(first.Header.Get("Authorization"), test.prefix) {
				t.Fatalf("Authorization = %q / %q", first.Header.Get("Authorization"), second.Header.Get("Authorization"))
			}
			body := make([]byte, len(`{"value":1}`))
			if _, err := first.Body.Read(body); err != nil {
				t.Fatal(err)
			}
			if string(body) != `{"value":1}` {
				t.Fatalf("restored request body = %s", body)
			}
		})
	}
}

func TestCanonicalQueryUsesRFC3986Spaces(t *testing.T) {
	request, _ := http.NewRequest(http.MethodGet, "https://example.com/?q=hello+world&z=%E4%B8%AD", nil)
	if got, want := canonicalQuery(request.URL), "q=hello%20world&z=%E4%B8%AD"; got != want {
		t.Fatalf("canonicalQuery() = %q, want %q", got, want)
	}
}
