package apitypes

import "testing"

func TestRuntimeProfileQuotaOptionalUnionSchema(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spec  string
		valid bool
	}{
		{name: "omitted", spec: `{}`, valid: true},
		{name: "null", spec: `{"quota":null}`, valid: true},
		{name: "unlimited", spec: `{"quota":{"type":"unlimited"}}`, valid: true},
		{name: "custom", spec: `{"quota":{"type":"custom","endpoint":"http://quota.example.test/v1/quota","api_key":"test"}}`, valid: true},
		{name: "empty", spec: `{"quota":{}}`},
		{name: "missing type", spec: `{"quota":{"endpoint":"http://quota.example.test/v1/quota","api_key":"test"}}`},
		{name: "unknown type", spec: `{"quota":{"type":"rate_limit"}}`},
		{name: "unlimited extras", spec: `{"quota":{"type":"unlimited","api_key":"test"}}`},
		{name: "custom missing endpoint", spec: `{"quota":{"type":"custom","api_key":"test"}}`},
		{name: "custom missing key", spec: `{"quota":{"type":"custom","endpoint":"http://quota.example.test/v1/quota"}}`},
		{name: "custom empty key", spec: `{"quota":{"type":"custom","endpoint":"http://quota.example.test/v1/quota","api_key":""}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"RuntimeProfile","metadata":{"id":"test"},"spec":` + tc.spec + `}`
			if err := ValidateResourceJSON([]byte(data)); (err == nil) != tc.valid {
				t.Fatalf("schema accepted=%v, want %v: %v", err == nil, tc.valid, err)
			}
		})
	}
}
