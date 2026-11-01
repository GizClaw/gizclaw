package volctlsfixture_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/volctlsfixture"
	"github.com/volcengine/volc-sdk-golang/service/tls"
)

func TestMissingLegacyIndexRejectsTheWholeExpression(t *testing.T) {
	server := httptest.NewServer(volctlsfixture.NewHandlerWithMissingIndexes("error_code"))
	defer server.Close()
	store, err := logstore.NewVolcStore(logstore.VolcConfig{
		Endpoint: server.URL, Region: "fixture", TopicID: "fixture",
		AccessKeyID: "fixture-key", AccessKeySecret: "fixture-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	if _, err := store.Append(t.Context(), []logstore.Record{{
		ID: "fixture-record", Time: now, Stream: "system", Kind: "log", Severity: "WARN",
		Message: "literal error_code: inside a message", Attributes: map[string]string{"error_code": "duplicate"},
	}}); err != nil {
		t.Fatal(err)
	}
	client := tls.NewClient(server.URL, "fixture-key", "fixture-secret", "", "fixture")
	search := func(query string) (*tls.SearchLogsResponse, error) {
		return client.SearchLogsV2(&tls.SearchLogsRequest{
			TopicID: "fixture", Query: query, Limit: 10, Sort: "desc",
			StartTime: now.Add(-time.Second).UnixMilli(), EndTime: now.Add(time.Second).UnixMilli(),
		})
	}
	for _, query := range []string{`level:"WARN"`, `attributes.error_code:"duplicate"`, `msg:#"literal error_code:"`} {
		page, err := search(query)
		if err != nil || len(page.Logs) != 1 {
			t.Fatalf("valid query %q: page=%+v, err=%v", query, page, err)
		}
	}
	_, err = search(`attributes.error_code:"duplicate" OR error_code:"duplicate"`)
	if err == nil || !strings.Contains(err.Error(), "key-value index for field (error_code) is not configured") {
		t.Fatalf("unindexed OR branch must reject the query: %v", err)
	}
}
