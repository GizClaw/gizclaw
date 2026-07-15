package rpcapi

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWorkflowI18nJSONUsesFlatHTTPShape(t *testing.T) {
	name := "Workflow"
	description := "Description"
	want := WorkflowI18n{
		DefaultLocale: "en",
		Value: map[string]WorkflowI18nCatalog{
			"en": {
				Name:        &name,
				Description: &description,
			},
			"zh-CN": {},
		},
	}

	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if bytes.Contains(raw, []byte(`"value"`)) {
		t.Fatalf("json.Marshal() included RPC-only value wrapper: %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"default_locale":"en"`)) || !bytes.Contains(raw, []byte(`"zh-CN":{}`)) {
		t.Fatalf("json.Marshal() = %s", raw)
	}

	var got WorkflowI18n
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.DefaultLocale != want.DefaultLocale || len(got.Value) != len(want.Value) {
		t.Fatalf("json.Unmarshal() = %+v", got)
	}
	if got.Value["en"].Name == nil || *got.Value["en"].Name != name {
		t.Fatalf("json.Unmarshal() en catalog = %+v", got.Value["en"])
	}
}

func TestWorkflowI18nJSONRequiresDefaultLocale(t *testing.T) {
	var got WorkflowI18n
	err := json.Unmarshal([]byte(`{"en":{"name":"Workflow"}}`), &got)
	if err == nil || err.Error() != "default_locale is required" {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
}
