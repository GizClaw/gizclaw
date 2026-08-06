//go:build gizclaw_e2e

package admin_test

import (
	"encoding/json"
	"testing"
)

type adminResourceIdentity struct {
	ID string `json:"id"`
}

func adminResourceID(t *testing.T, output, id string) string {
	t.Helper()
	var items []adminResourceIdentity
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("decode admin resource list: %v\n%s", err, output)
	}
	for _, item := range items {
		if item.ID == id {
			if item.ID == "" {
				t.Fatalf("admin resource %q has an empty canonical ID", id)
			}
			return item.ID
		}
	}
	t.Fatalf("admin resource %q not found in list:\n%s", id, output)
	return ""
}

func adminFirstResource(t *testing.T, output string) adminResourceIdentity {
	t.Helper()
	var items []adminResourceIdentity
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("decode admin resource list: %v\n%s", err, output)
	}
	if len(items) == 0 || items[0].ID == "" {
		t.Fatalf("admin resource list has no complete identity:\n%s", output)
	}
	return items[0]
}

func adminCreatedResourceID(t *testing.T, output string) string {
	t.Helper()
	var item adminResourceIdentity
	if err := json.Unmarshal([]byte(output), &item); err != nil {
		t.Fatalf("decode created admin resource: %v\n%s", err, output)
	}
	if item.ID == "" {
		t.Fatalf("created admin resource has an empty canonical ID:\n%s", output)
	}
	return item.ID
}

func adminAppliedResourceID(t *testing.T, output string) string {
	t.Helper()
	var result struct {
		ID *string `json:"id"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode admin apply result: %v\n%s", err, output)
	}
	if result.ID == nil || *result.ID == "" {
		t.Fatalf("admin apply result has an empty canonical ID:\n%s", output)
	}
	return *result.ID
}

func adminAppliedResourceIDs(t *testing.T, output string) map[string]string {
	t.Helper()
	var result struct {
		Items []struct {
			ID *string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode admin ResourceList apply result: %v\n%s", err, output)
	}
	ids := make(map[string]string, len(result.Items))
	for _, item := range result.Items {
		if item.ID == nil || *item.ID == "" {
			t.Fatalf("admin apply result has an empty canonical ID:\n%s", output)
		}
		ids[*item.ID] = *item.ID
	}
	return ids
}
