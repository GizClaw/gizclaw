package gizrun

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsPushGroupingFlag(t *testing.T) {
	var grouping metricsPushGroupingFlag

	if err := grouping.Set("service=openai-example"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := grouping.Set("env=dev"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if grouping["service"] != "openai-example" || grouping["env"] != "dev" {
		t.Fatalf("grouping = %#v", grouping)
	}
	if grouping.String() == "" {
		t.Fatal("String returned empty value")
	}
}

func TestMetricsPushGroupingFlagRejectsInvalidValue(t *testing.T) {
	var grouping metricsPushGroupingFlag

	if err := grouping.Set("missing-value"); err == nil {
		t.Fatal("Set error = nil, want invalid grouping error")
	}
}

func TestMetricsPushGroupingFlagUnmarshalYAML(t *testing.T) {
	var grouping metricsPushGroupingFlag

	if err := grouping.UnmarshalYAML([]byte("service: openai-example\nenv: dev\n")); err != nil {
		t.Fatalf("UnmarshalYAML failed: %v", err)
	}
	if grouping["service"] != "openai-example" || grouping["env"] != "dev" {
		t.Fatalf("grouping = %#v", grouping)
	}
}

func TestMetricsPushGroupingFlagUnmarshalYAMLRejectsEmptyKey(t *testing.T) {
	var grouping metricsPushGroupingFlag

	err := grouping.UnmarshalYAML([]byte("\"\": value\n"))
	if err == nil || !strings.Contains(err.Error(), "grouping key is empty") {
		t.Fatalf("UnmarshalYAML err = %v, want empty grouping key error", err)
	}
}

func TestMetricsPushConfigNormalize(t *testing.T) {
	t.Setenv("PUSH_URL", "http://push.example.test")
	t.Setenv("PUSH_PASSWORD", "secret")
	t.Setenv("PUSH_TOKEN", "bearer-secret")

	config, err := (metricsPushConfig{
		Enabled:     true,
		URL:         "${PUSH_URL}",
		BearerToken: "${PUSH_TOKEN}",
		Grouping: metricsPushGroupingFlag{
			"service": "openai-example",
		},
	}).normalize()
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if config.URL != "http://push.example.test" {
		t.Fatalf("url = %q", config.URL)
	}
	if config.BearerToken != "bearer-secret" {
		t.Fatalf("bearer token was not expanded")
	}
	if config.Job != "gizrun" {
		t.Fatalf("job = %q, want gizrun", config.Job)
	}
	interval, err := config.pushInterval()
	if err != nil {
		t.Fatalf("pushInterval failed: %v", err)
	}
	if interval != 15*time.Second {
		t.Fatalf("interval = %v, want 15s", interval)
	}
	if config.Grouping["service"] != "openai-example" {
		t.Fatalf("grouping = %#v", config.Grouping)
	}
}

func TestMetricsPushConfigNormalizeRejectsMixedAuth(t *testing.T) {
	_, err := (metricsPushConfig{
		Enabled:     true,
		URL:         "http://push.example.test",
		Username:    "user",
		BearerToken: "token",
	}).normalize()
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("normalize err = %v, want mutually exclusive auth error", err)
	}
}

func TestMetricsPushConfigNormalizeRejectsMissingURL(t *testing.T) {
	_, err := (metricsPushConfig{Enabled: true}).normalize()
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Fatalf("normalize err = %v, want url required", err)
	}
}

func TestMetricsPushConfigNormalizeRejectsBadInterval(t *testing.T) {
	_, err := (metricsPushConfig{
		Enabled:  true,
		URL:      "http://push.example.test",
		Interval: "0s",
	}).normalize()
	if err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("normalize err = %v, want positive interval", err)
	}
}

func TestNewMetricsPusherRejectsInvalidInput(t *testing.T) {
	if _, err := newMetricsPusher(metricsPushConfig{Interval: "0s"}, prometheus.NewRegistry()); err == nil {
		t.Fatal("newMetricsPusher invalid interval err = nil")
	}
	if _, err := newMetricsPusher(metricsPushConfig{}, nil); err == nil {
		t.Fatal("newMetricsPusher nil gatherer err = nil")
	}
}

func TestMetricsPusherPushUsesBearerTokenAndGrouping(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("authorization = %q, want bearer token", got)
		}
		if !strings.Contains(r.URL.Path, "/metrics/job/test-job/instance/dev") {
			t.Errorf("path = %q, want grouping path", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)

	pusher, err := newMetricsPusher(metricsPushConfig{
		URL:         server.URL,
		Job:         "test-job",
		BearerToken: "test-token",
		Interval:    "1h",
		Grouping: metricsPushGroupingFlag{
			"instance": "dev",
		},
	}, prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("newMetricsPusher failed: %v", err)
	}

	pusher.start()
	pusher.stopAndWait()
	if requests.Load() == 0 {
		t.Fatal("metrics pusher did not push")
	}
}
