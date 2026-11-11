package tikvfixture

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

var usageCredentialPattern = regexp.MustCompile(`gizclaw_sk_v1_[A-Za-z0-9_-]+|(?i:bearer)[ ]+[^ ]+`)

func usageRedactFailure(text string) string {
	text = usageCredentialPattern.ReplaceAllString(text, "[REDACTED]")
	if len(text) > 2048 {
		text = text[:2048] + " [truncated]"
	}
	return text
}

func usageFailureDetail(body any) string {
	switch value := body.(type) {
	case string:
		return usageRedactFailure(strings.TrimSpace(value))
	case map[string]any:
		if detail, ok := value["error"]; ok {
			return usageRedactFailure(fmt.Sprint(detail))
		}
	}
	return "response body omitted"
}

type usageHTTPBranch struct {
	Endpoint string            `json:"endpoint"`
	Method   string            `json:"method"`
	Path     string            `json:"path"`
	Headers  map[string]string `json:"headers,omitempty"`
	Body     any               `json:"body,omitempty"`
	Status   int               `json:"status,omitempty"`
}

// race is an isolated coordination adapter because Giztest's parallel children
// support peer_stream only. Every branch sends a real HTTP request to an Edge
// or private control listener; status/body and transport failures are preserved.
// It joins both branches before replying and cancels siblings on any failure.
func (g *UsageGate) race(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var input struct {
		Branches map[string]usageHTTPBranch `json:"branches"`
	}
	if err := usageDecode(r, &input); err != nil || len(input.Branches) != 2 {
		http.Error(w, "exactly two HTTP coordination branches required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type completed struct {
		id    string
		value any
		err   error
	}
	done := make(chan completed, len(input.Branches))
	for id, branch := range input.Branches {
		go func() {
			value, err := usageForward(ctx, UsageAction{Operation: "http", Endpoint: branch.Endpoint, Method: branch.Method, Path: branch.Path, Headers: branch.Headers, Body: branch.Body})
			var body any
			if err == nil {
				response := value.(map[string]any)
				status := response["status"].(int)
				body = response["body"]
				if branch.Status != 0 && status != branch.Status || branch.Status == 0 && status >= http.StatusBadRequest {
					err = fmt.Errorf("actual HTTP status %d; expected %d: %s", status, branch.Status, usageFailureDetail(body))
				}
			}
			done <- completed{id, body, err}
		}()
	}
	results := make(map[string]any, len(input.Branches))
	var failure error
	for range input.Branches {
		result := <-done
		if result.err != nil && failure == nil {
			failure = fmt.Errorf("branch %s: %w", result.id, result.err)
			slog.Error("TiKV usage race branch failed", "branch", result.id, "error", usageRedactFailure(result.err.Error()))
			cancel()
		}
		results[result.id] = result.value
	}
	if failure != nil {
		http.Error(w, failure.Error(), http.StatusBadGateway)
		return
	}
	usageJSON(w, results)
}
