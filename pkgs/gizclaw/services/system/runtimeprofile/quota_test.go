package runtimeprofile

import (
	"encoding/json"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"testing"
)

func testQuotaBinding(t *testing.T, data string) *apitypes.RuntimeProfileQuota {
	t.Helper()
	var binding *apitypes.RuntimeProfileQuota
	if err := json.Unmarshal([]byte(data), &binding); err != nil {
		t.Fatal(err)
	}
	return binding
}

func TestQuotaBindingDefaultsAndCustomValidation(t *testing.T) {
	for _, data := range []string{"null", `{"type":"unlimited"}`} {
		profile, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{Id: "test", Spec: apitypes.RuntimeProfileSpec{Quota: testQuotaBinding(t, data)}}, "")
		if err != nil {
			t.Fatalf("default policy: %v", err)
		}
		policy, err := CustomQuotaPolicy(profile.Spec.Quota)
		if err != nil || policy != nil {
			t.Fatalf("default policy unexpectedly requires HTTP: %v", err)
		}
	}
	for _, data := range []string{
		`{}`, `{"type":"rate_limit"}`, `{"type":"unlimited","endpoint":"http://quota.example.test"}`,
		`{"endpoint":"http://quota.example.test","api_key":"test"}`, `{"type":"custom"}`,
		`{"type":"custom","endpoint":"http://quota.example.test"}`,
		`{"type":"custom","endpoint":"ftp://quota.example.test","api_key":"test"}`,
		`{"type":"custom","endpoint":"http://user:pass@quota.example.test","api_key":"test"}`,
		`{"type":"custom","endpoint":"http://quota.example.test?secret=x","api_key":"test"}`,
		`{"type":"custom","endpoint":"http://quota.example.test","api_key":"a\nb"}`,
		`{"type":"custom","endpoint":"http://quota.example.test","api_key":""}`,
		`{"type":"custom","endpoint":"http://quota.example.test","api_key":"test","extra":true}`,
	} {
		if _, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{Id: "test", Spec: apitypes.RuntimeProfileSpec{Quota: testQuotaBinding(t, data)}}, ""); err == nil {
			t.Fatal("accepted invalid quota binding")
		}
		if _, err := CustomQuotaPolicy(testQuotaBinding(t, data)); err == nil {
			t.Fatal("invalid policy could bypass authorization")
		}
	}
	input := testQuotaBinding(t, `{"type":"custom","endpoint":" http://quota.example.test/v1/quota ","api_key":" test "}`)
	value, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{Id: "test", Spec: apitypes.RuntimeProfileSpec{Quota: input}}, "")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := CustomQuotaPolicy(value.Spec.Quota)
	if err != nil || policy.Endpoint != "http://quota.example.test/v1/quota" || policy.ApiKey != "test" {
		t.Fatalf("custom policy normalization: %v", err)
	}
	original, err := input.AsRuntimeProfileQuotaCustom()
	if err != nil || original.ApiKey != " test " {
		t.Fatal("normalization mutated the caller's binding")
	}
}

func TestStoredQuotaLegacyCompatibility(t *testing.T) {
	for _, data := range []string{`{}`, `null`} {
		policy, err := decodeRuntimeProfileQuota([]byte(data))
		if err != nil || policy != nil {
			t.Fatalf("unconfigured legacy row: %v", err)
		}
	}
	binding, err := decodeRuntimeProfileQuota([]byte(`{"endpoint":"http://quota.example.test/v1/quota","api_key":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	policy, err := CustomQuotaPolicy(binding)
	if err != nil || policy == nil || policy.Endpoint != "http://quota.example.test/v1/quota" {
		t.Fatalf("lost legacy custom enforcement: %v", err)
	}
	for _, data := range []string{`{"endpoint":"http://quota.example.test"}`, `{"type":"unknown"}`, `[]`, `{"type":"custom","endpoint":"http://quota.example.test","api_key":""}`} {
		if _, err := decodeRuntimeProfileQuota([]byte(data)); err == nil {
			t.Fatal("invalid stored policy became unlimited")
		}
	}
}
