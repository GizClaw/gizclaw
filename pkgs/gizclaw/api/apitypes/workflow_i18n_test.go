package apitypes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkflowI18nCatalogRejectsUnknownFields(t *testing.T) {
	var workflow Workflow
	err := json.Unmarshal([]byte(`{
		"name":"workflow",
		"i18n":{"default_locale":"en","en":{"descripton":"typo"}},
		"spec":{"driver":"flowcraft","flowcraft":{}}
	}`), &workflow)
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
	if i18n.En == nil || i18n.ZhCN == nil {
		t.Fatalf("catalogs = %#v", i18n)
	}
}

func TestWorkflowI18nRejectsUnsupportedLocale(t *testing.T) {
	var i18n WorkflowI18n
	err := json.Unmarshal([]byte(`{
		"default_locale":"en",
		"en":{},
		"fr":{}
	}`), &i18n)
	if err == nil || !strings.Contains(err.Error(), `unknown field "fr"`) {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
}
