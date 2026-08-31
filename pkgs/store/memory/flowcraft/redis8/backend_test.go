package redis8

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/flowcraft/memory/retrieval"

	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
)

func TestRedisMajorVersion(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		info    string
		want    int
		wantErr bool
	}{
		{name: "redis 8", info: "# Server\r\nredis_version:8.2.1\r\n", want: 8},
		{name: "redis 10", info: "redis_version:10.0.0\n", want: 10},
		{name: "missing", info: "redis_mode:standalone\r\n", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := redisMajorVersion(test.info)
			if (err != nil) != test.wantErr {
				t.Fatalf("redisMajorVersion error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("redisMajorVersion = %d, want %d", got, test.want)
			}
		})
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
}
