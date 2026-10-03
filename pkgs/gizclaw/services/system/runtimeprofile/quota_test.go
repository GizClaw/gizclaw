package runtimeprofile

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestQuotaBindingIsRequiredAndValidatesSecretsAndEndpoint(t *testing.T) {
	for _, policy := range []apitypes.RuntimeProfileQuota{{}, {Endpoint: "http://quota.example.test/v1/quota"}, {Endpoint: "ftp://quota.example.test", ApiKey: "test"}, {Endpoint: "http://user:pass@quota.example.test", ApiKey: "test"}, {Endpoint: "http://quota.example.test?secret=x", ApiKey: "test"}, {Endpoint: "http://quota.example.test/v1/quota", ApiKey: "a\nb"}} {
		if _, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{Id: "test", Spec: apitypes.RuntimeProfileSpec{Quota: policy}}, ""); err == nil {
			t.Fatalf("accepted invalid quota binding")
		}
	}
	value, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{Id: "test", Spec: apitypes.RuntimeProfileSpec{Quota: apitypes.RuntimeProfileQuota{Endpoint: " http://quota.example.test/v1/quota ", ApiKey: " test "}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if value.Spec.Quota.Endpoint != "http://quota.example.test/v1/quota" || value.Spec.Quota.ApiKey != "test" {
		t.Fatalf("binding=%+v", value.Spec.Quota)
	}
}
