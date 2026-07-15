package apitypes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkflowI18nCatalogRejectsUnknownFields(t *testing.T) {
	var doc WorkflowDocument
	err := json.Unmarshal([]byte(`{
		"metadata":{"name":"workflow"},
		"i18n":{"default_locale":"en","en":{"descripton":"typo"}},
		"spec":{"driver":"flowcraft","flowcraft":{}}
	}`), &doc)
	if err == nil || !strings.Contains(err.Error(), `unknown field "descripton"`) {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
}

func TestWorkflowI18nCatalogAcceptsPartialAndEmptyCatalogs(t *testing.T) {
	var i18n WorkflowI18n
	if err := json.Unmarshal([]byte(`{
		"default_locale":"en",
		"en":{"description":"Description"},
		"zh-CN":{}
	}`), &i18n); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(i18n.AdditionalProperties) != 2 {
		t.Fatalf("catalogs = %#v", i18n.AdditionalProperties)
	}
}
