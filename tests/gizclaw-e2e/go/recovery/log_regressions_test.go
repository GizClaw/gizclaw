//go:build gizclaw_e2e

package recovery_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/google/uuid"
	"github.com/volcengine/volc-sdk-golang/service/tls"
)

func logRegressionFixture(t *testing.T) *fixture {
	t.Helper()
	if os.Getenv("RECOVERY_LOG_REGRESSIONS") != "1" {
		t.Skip("requires run_pending_recovery_tests.sh --log-regressions")
	}
	return openFixture(t)
}

func TestLogRegressionsContactGiztests(t *testing.T) {
	f := logRegressionFixture(t)
	token := uuid.NewString()
	response, err := f.regions[0].api.CreateRegistrationTokenWithResponse(t.Context(), adminhttp.RegistrationTokenUpsert{
		Id: token, Token: token, RuntimeProfileId: "recovery-profile",
	})
	must(t, err)
	if response.StatusCode() != http.StatusOK {
		t.Fatalf("create registration token: HTTP %d", response.StatusCode())
	}
	t.Setenv("GIZCLAW_TEST_ENDPOINT", "server-a:9820")
	t.Setenv("GIZCLAW_TEST_REGISTRATION_TOKEN", token)
	reportPath := "/evidence/contact-giztest.json"
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "/binaries/gizclaw", "test", "run",
		"/giztest/server.contact.duplicate-phone.giztest.yaml",
		"/giztest/server.contact.duplicate-phone-update.giztest.yaml",
		"/giztest/server.contact.limit.giztest.yaml",
		"--parallel", "1", "--report", reportPath)
	output, runErr := command.CombinedOutput()
	data, err := os.ReadFile(reportPath)
	must(t, err)
	var report struct {
		Status string `json:"status"`
		Tasks  []struct {
			Name    string `json:"name"`
			Status  string `json:"status"`
			Cleanup []struct {
				Status string `json:"status"`
			} `json:"cleanup"`
		} `json:"tasks"`
	}
	must(t, json.Unmarshal(data, &report))
	if len(report.Tasks) != 3 {
		t.Fatalf("all three Contact Giztests must run: %s", data)
	}
	for _, task := range report.Tasks {
		if len(task.Cleanup) == 0 {
			t.Errorf("%s did not run cleanup", task.Name)
		}
		for _, cleanup := range task.Cleanup {
			if cleanup.Status != "passed" {
				t.Errorf("%s cleanup: %s", task.Name, cleanup.Status)
			}
		}
	}
	if runErr != nil || report.Status != "passed" {
		t.Fatalf("Contact Giztests: %v\n%s\n%s", runErr, output, data)
	}
}

func TestLogRegressionsStructuredFilter(t *testing.T) {
	f := logRegressionFixture(t)
	store, err := logstore.NewVolcStore(logstore.VolcConfig{
		Endpoint: "http://log-sink:9091", Region: "test-region", TopicID: "regressions",
		AccessKeyID: "fixture-key", AccessKeySecret: "fixture-secret",
	})
	must(t, err)
	defer store.Close()
	now := time.Now().UTC()
	message := "structured-filter-" + uuid.NewString()
	_, err = store.Append(t.Context(), []logstore.Record{{
		ID: uuid.NewString(), Time: now, Stream: "system", Kind: "log", Severity: "WARN",
		Message: message, Attributes: map[string]string{"error_code": "regression_duplicate"},
	}})
	must(t, err)
	start, end := now.Add(-time.Minute), now.Add(time.Minute)
	control := queryRegressionLogs(t, f.regions[0], `level:WARN AND text:"`+message+`"`, start, end)
	if entries := regressionLogEntries(t, control); len(entries) != 1 || entries[0].Message != message {
		t.Fatalf("control query did not return the persisted probe: %+v", entries)
	}
	native := tls.NewClient("http://log-sink:9091", "fixture-key", "fixture-secret", "", "test-region")
	modern, err := native.SearchLogsV2(&tls.SearchLogsRequest{
		TopicID:   "regressions",
		Query:     `stream:"system" AND kind:"log" AND attributes.error_code:"regression_duplicate" AND msg:#"` + message + `"`,
		StartTime: start.UnixMilli(), EndTime: end.UnixMilli(), Limit: 10, Sort: "desc",
	})
	must(t, err)
	if len(modern.Logs) != 1 {
		t.Fatalf("modern indexed query returned %d probes, want 1", len(modern.Logs))
	}
	filtered := queryRegressionLogs(t, f.regions[0],
		`level:WARN AND error_code:regression_duplicate AND text:"`+message+`"`, start, end)
	if entries := regressionLogEntries(t, filtered); len(entries) != 1 || entries[0].Message != message {
		t.Fatalf("structured filter did not return the indexed probe: %+v", entries)
	}
}

func TestLogRegressionsUnchangedRecoveryWarning(t *testing.T) {
	f := logRegressionFixture(t)
	item := f.seedPeer(t, 0)
	workspaceID := "retained-" + uuid.NewString()
	start := time.Now().UTC()
	task := f.seedFailed(t, item, "peer_replacement_ambiguous", true, func(pendingdeletion.Record) {
		f.seedWorkspace(t, 0, item.PublicKey, workspaceID)
	})
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	select {
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	case <-timer.C:
	}
	after, err := f.source.GetTask(t.Context(), task.Record.DeletionID)
	must(t, err)
	if after.Status != pendingdeletion.StatusFailed || after.FailureCount != task.FailureCount ||
		after.LastErrorCode != task.LastErrorCode || !after.UpdatedAt.Equal(task.UpdatedAt) {
		t.Fatal("automatic rejection changed the failed task")
	}
	var count int
	must(t, f.regions[0].db.GetContext(t.Context(), &count,
		`SELECT count(*) FROM workspaces WHERE id=$1 AND owner_public_key=$2`, workspaceID, item.PublicKey))
	if count != 1 {
		t.Fatal("automatic rejection deleted ambiguous regional data")
	}
	page := queryRegressionLogs(t, f.regions[0], `text:"pending deletion: automatic recovery rejected"`, start, time.Now())
	warnings := 0
	for _, entry := range regressionLogEntries(t, page) {
		if entry.Fields["deletion_id"] == task.Record.DeletionID {
			warnings++
		}
	}
	if warnings != 1 {
		t.Fatalf("unchanged rejected task emitted %d warnings in 4s at a 250ms scan interval, want one bounded warning", warnings)
	}
}

func TestLogRegressionsServerBuildIdentity(t *testing.T) {
	f := logRegressionFixture(t)
	for i, name := range []string{"a", "b"} {
		t.Run(name, func(t *testing.T) {
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://server-"+name+":9820/server-info", nil)
			must(t, err)
			response, err := http.DefaultClient.Do(request)
			must(t, err)
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("server-info: HTTP %d", response.StatusCode)
			}
			var info apitypes.ServerInfo
			must(t, json.NewDecoder(response.Body).Decode(&info))
			if info.Version != "log-regression-version" || info.BuildCommit != "log-regression-commit" {
				t.Fatalf("expected nondefault binary identity, got %q %q", info.Version, info.BuildCommit)
			}
			start := time.Now().Add(-time.Second)
			seed, err := f.regions[i].api.ListPeersWithResponse(t.Context(), nil)
			must(t, err)
			if seed.StatusCode() != http.StatusOK {
				t.Fatalf("seed completion: HTTP %d", seed.StatusCode())
			}
			id := seed.HTTPResponse.Header.Get("X-Request-ID")
			if id == "" {
				t.Fatal("completion has no request ID")
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				page := queryRegressionLogs(t, f.regions[i], "request_id:"+id, start, time.Now().Add(time.Second))
				for _, entry := range regressionLogEntries(t, page) {
					if entry.Fields["request_id"] != id {
						continue
					}
					if entry.Fields["runtime_version"] != info.Version || entry.Fields["build_commit"] != info.BuildCommit {
						t.Fatalf("persisted log identity = %q %q, server-info = %q %q",
							entry.Fields["runtime_version"], entry.Fields["build_commit"], info.Version, info.BuildCommit)
					}
					t.Logf("Server %s persisted runtime_version=%s build_commit=%s", name, info.Version, info.BuildCommit)
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			t.Fatal("completion was not persisted before the deadline")
		})
	}
}

func queryRegressionLogs(t *testing.T, r region, filter string, start, end time.Time) []byte {
	t.Helper()
	response, err := r.api.StreamServerLogsWithResponse(t.Context(), &adminhttp.StreamServerLogsParams{
		Filter: &filter, StartTimeMs: new(start.UnixMilli()), EndTimeMs: new(end.UnixMilli()), Limit: new(int32(1000)),
	})
	must(t, err)
	if response.StatusCode() != http.StatusOK {
		t.Fatalf("Admin filter %q: HTTP %d, want 200: %s", filter, response.StatusCode(), response.Body)
	}
	return response.Body
}

func regressionLogEntries(t *testing.T, body []byte) []apitypes.ServerLogEntry {
	t.Helper()
	var entries []apitypes.ServerLogEntry
	complete := false
	for _, block := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n\n") {
		event, data := "", ""
		for line := range strings.SplitSeq(block, "\n") {
			if value, ok := strings.CutPrefix(line, "event:"); ok {
				event = strings.TrimSpace(value)
			}
			if value, ok := strings.CutPrefix(line, "data:"); ok {
				data += strings.TrimSpace(value)
			}
		}
		switch event {
		case "log":
			var entry apitypes.ServerLogEntry
			must(t, json.Unmarshal([]byte(data), &entry))
			entries = append(entries, entry)
		case "end":
			var end apitypes.ServerLogStreamEnd
			must(t, json.Unmarshal([]byte(data), &end))
			if end.HasNext {
				t.Fatal("regression log query exceeded its bounded page")
			}
			complete = true
		case "error":
			t.Fatalf("Admin log stream error: %s", data)
		}
	}
	if !complete {
		t.Fatal("Admin log stream has no completion event")
	}
	return entries
}
