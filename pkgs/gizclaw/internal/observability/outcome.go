package observability

import (
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const maxDimensionLength = 128

var safeDimensionRE = regexp.MustCompile(`^[A-Za-z0-9._:/-]+$`)
var safeOperationRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var safeRequestIDRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// Outcome contains the bounded mutable state for one request.
type Outcome struct {
	mu sync.Mutex

	started       time.Time
	transport     Transport
	surface       Surface
	operation     string
	result        Result
	statusClass   StatusClass
	method        string
	route         string
	status        int
	rpcCode       int
	hasRPCCode    bool
	requestID     string
	peerPublicKey string
	peerRole      string
	errorCode     string
	panic         bool
	annotations   map[AnnotationKey]string
}

// NewOutcome creates a request outcome with a monotonic start time.
func NewOutcome(transport Transport, surface Surface, operation string) *Outcome {
	return &Outcome{
		started:     time.Now(),
		transport:   transport,
		surface:     surface,
		operation:   boundedOperation(operation),
		result:      ResultSuccess,
		statusClass: StatusClassUnknown,
		route:       "unknown",
	}
}

// SetHTTP records the final HTTP transport fields.
func (o *Outcome) SetHTTP(method, route string, status int, result Result) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.method = boundedHTTPMethod(method)
	if route != "" {
		o.route = boundedRoute(route)
	}
	o.status = status
	o.statusClass = statusClass(status)
	if o.panic {
		o.result = ResultPanic
	} else {
		o.result = result
	}
}

// SetRPC records the final RPC code and result.
func (o *Outcome) SetRPC(code int, result Result) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.rpcCode = code
	o.hasRPCCode = code != 0
	if code > 0 {
		o.statusClass = statusClass(code)
	} else {
		o.statusClass = StatusClassUnknown
	}
	o.result = result
}

// SetOperation sets a resolved, bounded operation name.
func (o *Outcome) SetOperation(operation string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.operation = boundedOperation(operation)
	o.mu.Unlock()
}

// SetRoute sets a normalized registered HTTP route.
func (o *Outcome) SetRoute(route string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.route = boundedRoute(route)
	o.mu.Unlock()
}

// SetHTTPFallback records a registered route and operation only when a more
// specific router integration has not already resolved them.
func (o *Outcome) SetHTTPFallback(route, operation string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	if o.route == "unknown" {
		o.route = boundedRoute(route)
	}
	if o.operation == "unknown" {
		o.operation = boundedOperation(operation)
	}
	o.mu.Unlock()
}

// SetRequestID sets a validated transport request ID.
func (o *Outcome) SetRequestID(id string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	id = strings.TrimSpace(id)
	if safeRequestIDRE.MatchString(id) {
		o.requestID = id
	} else {
		o.requestID = ""
	}
	o.mu.Unlock()
}

// SetPeer sets already-authenticated peer identity without doing a lookup.
func (o *Outcome) SetPeer(publicKey, role string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.peerPublicKey = boundedDimension(publicKey, "")
	o.peerRole = boundedDimension(role, "")
	o.mu.Unlock()
}

// SetErrorCode stores only a bounded server-owned identifier.
func (o *Outcome) SetErrorCode(code string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	code = strings.TrimSpace(code)
	if safeOperationRE.MatchString(code) {
		o.errorCode = code
	} else {
		o.errorCode = ""
	}
	o.mu.Unlock()
}

// MarkPanic records a panic already recovered by an existing transport boundary.
func (o *Outcome) MarkPanic() {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.panic = true
	o.result = ResultPanic
	o.mu.Unlock()
}

// Annotate adds one allowlisted, bounded domain identifier.
func (o *Outcome) Annotate(key AnnotationKey, value string) {
	if o == nil || !validAnnotationKey(key) {
		return
	}
	value = boundedDimension(value, "")
	if value == "" {
		return
	}
	o.mu.Lock()
	if o.annotations == nil {
		o.annotations = make(map[AnnotationKey]string)
	}
	o.annotations[key] = value
	o.mu.Unlock()
}

func (o *Outcome) logRecord() (slog.Level, []slog.Attr) {
	o.mu.Lock()
	defer o.mu.Unlock()
	attrs := []slog.Attr{
		slog.String("transport", string(o.transport)),
		slog.String("surface", string(o.surface)),
		slog.String("operation", o.operation),
		slog.String("result", string(o.result)),
		slog.String("status_class", string(o.statusClass)),
		slog.Int64("duration_ms", time.Since(o.started).Milliseconds()),
	}
	if o.transport == TransportHTTP {
		attrs = append(attrs, slog.String("method", o.method), slog.String("route", o.route), slog.Int("status", o.status))
	} else if o.transport == TransportRPC && o.hasRPCCode {
		attrs = append(attrs, slog.Int("rpc_code", o.rpcCode))
	}
	for _, item := range []struct{ key, value string }{
		{"request_id", o.requestID},
		{"peer_public_key", o.peerPublicKey},
		{"peer_role", o.peerRole},
		{"error_code", o.errorCode},
	} {
		if item.value != "" {
			attrs = append(attrs, slog.String(item.key, item.value))
		}
	}
	for _, key := range []AnnotationKey{AnnotationWorkspaceName, AnnotationWorkflowName, AnnotationModelID, AnnotationResourceKind, AnnotationResourceName} {
		if value := o.annotations[key]; value != "" {
			attrs = append(attrs, slog.String(string(key), value))
		}
	}
	return levelFor(o.result, o.status, o.rpcCode), attrs
}

func levelFor(result Result, status, rpcCode int) slog.Level {
	if result == ResultCanceled {
		return slog.LevelWarn
	}
	if result == ResultPanic || result == ResultTransportError || result == ResultServerError || status >= http.StatusInternalServerError || rpcCode == -32603 || rpcCode >= 500 && rpcCode <= 599 {
		return slog.LevelError
	}
	if result == ResultClientError || status >= http.StatusBadRequest || rpcCode != 0 {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

func statusClass(status int) StatusClass {
	switch status / 100 {
	case 2:
		return StatusClass2xx
	case 3:
		return StatusClass3xx
	case 4:
		return StatusClass4xx
	case 5:
		return StatusClass5xx
	default:
		return StatusClassUnknown
	}
}

func boundedRoute(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxDimensionLength || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#") || !safeDimensionRE.MatchString(value) {
		return "unknown"
	}
	return value
}

func boundedHTTPMethod(value string) string {
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

func boundedOperation(value string) string {
	value = strings.TrimSpace(value)
	if !safeOperationRE.MatchString(value) {
		return "unknown"
	}
	return value
}

func boundedDimension(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxDimensionLength || !safeDimensionRE.MatchString(value) {
		return fallback
	}
	return value
}

func validAnnotationKey(key AnnotationKey) bool {
	switch key {
	case AnnotationWorkspaceName, AnnotationWorkflowName, AnnotationModelID, AnnotationResourceKind, AnnotationResourceName:
		return true
	default:
		return false
	}
}
