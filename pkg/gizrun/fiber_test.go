package gizrun

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/labelset"
	"github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/metrics"
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
)

type fiberLogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *fiberLogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *fiberLogHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Clone())
	return nil
}

func (h *fiberLogHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *fiberLogHandler) WithGroup(string) slog.Handler {
	return h
}

func (h *fiberLogHandler) hasMessage(message string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, record := range h.records {
		if record.Message == message {
			return true
		}
	}
	return false
}

func TestFiberAppTagsLogsAndRecordsMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics.Reset(reg)
	t.Cleanup(func() { metrics.Reset(prometheus.DefaultRegisterer) })
	if _, err := metrics.RegisterCounter(httpRequestsTotal, httpMethod, httpPath, httpHost, httpStatusCode); err != nil {
		t.Fatal(err)
	}
	if _, err := metrics.RegisterHistogram(httpRequestDurationSeconds, httpMethod, httpPath, httpHost, httpStatusCode); err != nil {
		t.Fatal(err)
	}
	previousApp := runCtx.publicApp
	runCtx.publicApp = newFiberApp(fiber.Config{DisableStartupMessage: true})
	t.Cleanup(func() { runCtx.publicApp = previousApp })

	logs := &fiberLogHandler{}
	previous := slog.Default()
	slog.SetDefault(slog.New(logs))
	t.Cleanup(func() { slog.SetDefault(previous) })

	var labels labelset.LabelSet
	HTTP().Get("/v1/test/:id", func(c *fiber.Ctx) error {
		var ok bool
		labels, ok = httpLabels(c.UserContext())
		if !ok {
			t.Fatal("httpLabels missing")
		}
		return c.SendStatus(fiber.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/test/123", nil)
	req.Host = "gizclaw.test"
	resp, err := runCtx.publicApp.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusCreated)
	}
	for key, want := range map[string]string{
		httpMethod: http.MethodGet,
		httpPath:   "/v1/test/123",
		httpHost:   "gizclaw.test",
	} {
		if got, ok := labels.Value(key); !ok || got != want {
			t.Fatalf("HTTP label %q = (%q, %v), want (%q, true)", key, got, ok, want)
		}
	}
	if !logs.hasMessage("http request") {
		t.Fatal("missing http request log")
	}
	assertGatheredMetric(t, reg, "gizclaw_http_requests_total")
	assertGatheredMetric(t, reg, "gizclaw_http_request_duration_seconds")
}

func assertGatheredMetric(t *testing.T, gatherer prometheus.Gatherer, name string) {
	t.Helper()
	families, err := gatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == name {
			return
		}
	}
	t.Fatalf("metric %q was not gathered", name)
}
