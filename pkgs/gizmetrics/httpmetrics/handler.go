// Package httpmetrics provides reusable net/http server instrumentation backed
// by the process-wide gizmetrics recorder.
package httpmetrics

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	"github.com/felixge/httpsnoop"
)

const (
	// RequestsTotalMetric counts completed HTTP requests.
	RequestsTotalMetric = "giz_http_server_requests_total"
	// RequestDurationMetric records HTTP request duration in seconds.
	RequestDurationMetric = "giz_http_server_request_duration_seconds"
	// RequestsInFlightMetric records currently active HTTP requests.
	RequestsInFlightMetric = "giz_http_server_requests_in_flight"
	// ResponseBytesMetric counts HTTP response bytes written.
	ResponseBytesMetric = "giz_http_server_response_bytes_total"
)

var (
	durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	dimensionRE     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	inflightMu      sync.Mutex
	processInflight = make(map[inflightKey]int64)
)

// OperationResolver resolves a request to one registered, bounded operation
// name. Returning false records operation=unknown.
type OperationResolver func(*http.Request) (string, bool)

type handler struct {
	next    http.Handler
	surface string
	resolve OperationResolver
}

type inflightKey struct {
	surface   string
	operation string
	method    string
}

// Wrap instruments next with bounded HTTP server metrics. Surface and
// operation values that do not match the bounded dimension grammar become
// unknown; raw request paths and query strings are never used as fallbacks.
func Wrap(next http.Handler, surface string, resolve OperationResolver) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return &handler{
		next:    next,
		surface: boundedDimension(surface),
		resolve: resolve,
	}
}

func (h *handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	operation := "unknown"
	if h.resolve != nil {
		if resolved, ok := h.resolve(request); ok {
			operation = boundedDimension(resolved)
		}
	}
	method := boundedMethod(request.Method)
	baseLabels := []gizmetrics.Label{
		{Name: "surface", Value: h.surface},
		{Name: "operation", Value: operation},
		{Name: "method", Value: method},
	}

	inflight := inflightKey{surface: h.surface, operation: operation, method: method}
	recordInflight(request.Context(), inflight, 1, baseLabels)
	started := time.Now()
	status := http.StatusOK
	var (
		wroteHeader bool
		written     int64
		writeErr    error
	)
	wrapped := httpsnoop.Wrap(writer, httpsnoop.Hooks{
		WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
			return func(code int) {
				if !wroteHeader {
					status = code
					wroteHeader = true
				}
				next(code)
			}
		},
		Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
			return func(body []byte) (int, error) {
				if !wroteHeader {
					wroteHeader = true
				}
				count, err := next(body)
				written += int64(count)
				if err != nil && writeErr == nil {
					writeErr = err
				}
				return count, err
			}
		},
		ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
			return func(source io.Reader) (int64, error) {
				if !wroteHeader {
					wroteHeader = true
				}
				count, err := next(source)
				written += count
				if err != nil && writeErr == nil {
					writeErr = err
				}
				return count, err
			}
		},
	})

	defer func() {
		recordInflight(request.Context(), inflight, -1, baseLabels)
		panicValue := recover()
		result := requestResult(request.Context(), status)
		statusClass := httpStatusClass(status)
		if writeErr != nil {
			result = "transport_error"
		}
		if panicValue != nil {
			result = "panic"
			statusClass = "unknown"
		}
		completionLabels := append([]gizmetrics.Label(nil), baseLabels...)
		completionLabels = append(completionLabels,
			gizmetrics.Label{Name: "status_class", Value: statusClass},
			gizmetrics.Label{Name: "result", Value: result},
		)
		gizmetrics.AddCounter(request.Context(), RequestsTotalMetric, 1, completionLabels...)
		gizmetrics.ObserveHistogram(request.Context(), RequestDurationMetric, time.Since(started).Seconds(), durationBuckets, completionLabels...)
		gizmetrics.AddCounter(request.Context(), ResponseBytesMetric, float64(written), completionLabels...)
		if panicValue != nil {
			panic(panicValue)
		}
	}()

	h.next.ServeHTTP(wrapped, request)
}

func recordInflight(ctx context.Context, key inflightKey, delta int64, labels []gizmetrics.Label) {
	inflightMu.Lock()
	defer inflightMu.Unlock()
	current := processInflight[key] + delta
	if current == 0 {
		delete(processInflight, key)
	} else {
		processInflight[key] = current
	}
	// Keep publication ordered with the process count. SetGauge only updates the
	// in-process recorder, so holding this lock never waits for store I/O.
	gizmetrics.SetGauge(ctx, RequestsInFlightMetric, float64(current), labels...)
}

func boundedDimension(value string) string {
	value = strings.TrimSpace(value)
	if !dimensionRE.MatchString(value) {
		return "unknown"
	}
	return value
}

func boundedMethod(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions,
		http.MethodConnect, http.MethodTrace:
		return value
	default:
		return "OTHER"
	}
}

func requestResult(ctx context.Context, status int) string {
	if ctx != nil && ctx.Err() != nil {
		return "canceled"
	}
	switch {
	case status >= 200 && status < 400:
		return "success"
	case status >= 400 && status < 500:
		return "client_error"
	case status >= 500:
		return "server_error"
	default:
		return "transport_error"
	}
}

func httpStatusClass(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500 && status < 600:
		return "5xx"
	default:
		return "unknown"
	}
}
