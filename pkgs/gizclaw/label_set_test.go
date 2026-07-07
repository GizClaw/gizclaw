package gizclaw

import (
	"context"
	"log/slog"
	"testing"
)

func TestHTTPLabelSetTagMergeAndLogAttr(t *testing.T) {
	ctx, err := Tag(context.Background(), &HTTPLabelSet{Method: "GET", Path: "/v1"})
	if err != nil {
		t.Fatalf("Tag initial error = %v", err)
	}
	ctx, err = Tag(ctx, &HTTPLabelSet{StatusCode: "200", TraceID: "trace-1"})
	if err != nil {
		t.Fatalf("Tag merge error = %v", err)
	}
	labels, ok := labelSet(ctx, "http")
	if !ok {
		t.Fatal("http labels missing")
	}
	httpLabels := labels.(*HTTPLabelSet)
	if httpLabels.Method != "GET" || httpLabels.Path != "/v1" || httpLabels.StatusCode != "200" || httpLabels.TraceID != "trace-1" {
		t.Fatalf("merged http labels = %+v", httpLabels)
	}
	got := labelValues(httpLabels)
	if got["method"] != "GET" || got["path"] != "/v1" || got["status_code"] != "200" || got["trace_id"] != "trace-1" {
		t.Fatalf("http key values = %+v", got)
	}
	attr := LogAttr(httpLabels)
	if attr.Key != "http" || attr.Value.Kind() != slog.KindGroup {
		t.Fatalf("LogAttr = %+v", attr)
	}
}

func TestGenxLabelSetMergeIncludesHTTPLabels(t *testing.T) {
	labels := &GenxLabelSet{
		HTTP:     HTTPLabelSet{Method: "POST"},
		Provider: "openai",
		Model:    "gpt",
	}
	if err := labels.MergeWith(&GenxLabelSet{
		HTTP:      HTTPLabelSet{Path: "/chat"},
		Method:    "chat",
		Status:    "ok",
		TokenType: "output",
	}); err != nil {
		t.Fatalf("MergeWith error = %v", err)
	}
	got := labelValues(labels)
	if got["method"] != "POST" || got["path"] != "/chat" || got["provider"] != "openai" ||
		got["genx_method"] != "chat" || got["model"] != "gpt" || got["status"] != "ok" || got["token_type"] != "output" {
		t.Fatalf("genx key values = %+v", got)
	}
}

func TestLabelSetMergeRejectsWrongNamespaceType(t *testing.T) {
	if err := (&HTTPLabelSet{}).MergeWith(&GenxLabelSet{}); err == nil {
		t.Fatal("HTTPLabelSet.MergeWith accepted GenxLabelSet")
	}
	if err := (&GenxLabelSet{}).MergeWith(&HTTPLabelSet{}); err == nil {
		t.Fatal("GenxLabelSet.MergeWith accepted HTTPLabelSet")
	}
}

func labelValues(labels LabelSet) map[string]string {
	out := map[string]string{}
	for key, value := range labels.KeyValues() {
		out[key] = value
	}
	return out
}
