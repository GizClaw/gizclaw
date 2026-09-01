package redis8

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/flowcraft/memory/retrieval"

	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
)

func TestParseRedisVersion(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		info    string
		want    redisVersion
		wantErr bool
	}{
		{name: "redis 8.4", info: "# Server\r\nredis_version:8.4.6\r\n", want: redisVersion{major: 8, minor: 4, patch: 6}},
		{name: "redis 10", info: "redis_version:10.0.0\n", want: redisVersion{major: 10}},
		{name: "no patch", info: "redis_version:8.4\n", want: redisVersion{major: 8, minor: 4}},
		{name: "missing", info: "redis_mode:standalone\r\n", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseRedisVersion(test.info)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseRedisVersion error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("parseRedisVersion = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRedisVersionLessThan(t *testing.T) {
	t.Parallel()
	if !(redisVersion{major: 8, minor: 2, patch: 1}).lessThan(redisVersion{major: 8, minor: 4}) {
		t.Fatal("Redis 8.2 must be rejected")
	}
	if (redisVersion{major: 8, minor: 4}).lessThan(redisVersion{major: 8, minor: 4}) {
		t.Fatal("Redis 8.4 must be accepted")
	}
}

func TestEscapeText(t *testing.T) {
	t.Parallel()
	if got, want := escapeText(`hello @world | test`), `hello \@world \| test`; got != want {
		t.Fatalf("escapeText = %q, want %q", got, want)
	}
}

func TestNewRejectsNonDurableGraph(t *testing.T) {
	t.Parallel()
	_, err := New(context.Background(), Config{Flowcraft: memoryflowcraft.Config{GraphEnabled: true}})
	if err == nil || !strings.Contains(err.Error(), "graph") {
		t.Fatalf("New error = %v, want graph injection error", err)
	}
}

func TestCapabilitiesAdvertiseHybridSearch(t *testing.T) {
	t.Parallel()
	capabilities := (&Index{}).Capabilities()
	if !capabilities.BM25 || !capabilities.Vector || !capabilities.Hybrid {
		t.Fatalf("Capabilities() = %#v, want text, vector, and hybrid retrieval", capabilities)
	}
	if !retrieval.Supports(&Index{}, retrieval.CapabilityHybrid) {
		t.Fatal("capability-driven selection did not enable hybrid retrieval")
	}
	if !capabilities.FilterPushdown || !(&Index{}).SupportsFilter(retrieval.Filter{
		And:   []retrieval.Filter{{Eq: map[string]any{"kind": "fact"}}},
		Range: map[string]retrieval.Range{"confidence": {Gte: 0.5}},
	}) {
		t.Fatalf("Capabilities() = %#v, want native structured filter pushdown", capabilities)
	}
	if (&Index{}).SupportsFilter(retrieval.Filter{Match: map[string]string{"_content": "fact"}}) {
		t.Fatal("substring Match must remain a client-side filter")
	}
}

func TestCompileFilterBooleanConstants(t *testing.T) {
	t.Parallel()
	index := &Index{}
	matchNone := retrieval.Filter{In: map[string][]any{"kind": {}}}
	compiled, err := index.compileFilter(context.Background(), matchNone)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.query != impossibleFilterQuery {
		t.Fatalf("compileFilter(empty In) = %q, want %q", compiled.query, impossibleFilterQuery)
	}
	compiled, err = index.compileFilter(context.Background(), retrieval.Filter{Not: &matchNone})
	if err != nil {
		t.Fatal(err)
	}
	if compiled.query != "*" {
		t.Fatalf("compileFilter(Not empty In) = %q, want match-all", compiled.query)
	}
}
