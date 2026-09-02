package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"time"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

// driver runs Giztest steps with the two C SDKs: sdk/c/gizclaw dials the
// device Peer and answers server-initiated client.* RPCs, and
// sdk/c/gizclaw_control serves every `http` step against `/gizclaw/v1`.
//
// It deliberately supports fewer operations than the Go runner. Streaming,
// speech, and Workspace relay steps have no C client, so they are absent from
// Operations() and validate rejects a document that uses them instead of
// skipping the step.
type driver struct{}

func (driver) Operations() []string { return []string{"rpc", "client_rpc", "http"} }

func (driver) ValidateStep(doc *giztest.Document, step giztest.Step) error {
	switch step.Operation() {
	case "rpc":
		if _, err := lookupMethod(step.RPC.Method); err != nil {
			return err
		}
	case "client_rpc":
		if _, err := lookupMethod(step.ClientRPC.Method); err != nil {
			return err
		}
	case "http":
		return validateControlRoute(step)
	}
	return nil
}

// validateControlRoute rejects a step the controller SDK cannot route, so an
// unsupported contract surfaces during validate rather than at run time.
func validateControlRoute(step giztest.Step) error {
	switch step.HTTP.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		return fmt.Errorf("the C controller SDK does not send %s requests", step.HTTP.Method)
	}
	path := step.HTTP.Path
	if index := strings.IndexByte(path, '?'); index >= 0 {
		path = path[:index]
	}
	if !strings.HasPrefix(path, giztest.ControlPathPrefix) {
		return fmt.Errorf("path %q is outside the %s controller contract", step.HTTP.Path, giztest.ControlPathPrefix)
	}
	return nil
}

// FailureCode reports the structured RPC error an rpc step observed.
func (driver) FailureCode(err error) (int32, string, bool) {
	var failure *rpcError
	if errors.As(err, &failure) {
		return failure.code, failure.message, true
	}
	return 0, "", false
}

// rpcError is a structured RPC error the Server returned.
type rpcError struct {
	method  string
	code    int32
	message string
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("rpc %s failed (code %d): %s", e.method, e.code, e.message)
}

func (driver) Open(ctx context.Context, doc *giztest.Document, vars *giztest.Variables) (giztest.Session, error) {
	s := &session{clients: map[string]*deviceClient{}}
	names := make([]string, 0, len(doc.Clients))
	for name := range doc.Clients {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		endpoint, err := resolveString(vars, doc.Clients[name].AccessPoint)
		if err != nil {
			return s, fmt.Errorf("client %s access_point: %w", name, err)
		}
		client, err := newDeviceClient(ctx, name, endpoint, doc.Steps)
		if err != nil {
			return s, fmt.Errorf("client %s: %w", name, err)
		}
		s.clients[name] = client
	}
	control, err := openControl()
	if err != nil {
		return s, err
	}
	s.control = control
	return s, nil
}

func resolveString(vars *giztest.Variables, input string) (string, error) {
	value, err := vars.Resolve(input)
	if err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("must resolve to a string")
	}
	return text, nil
}

// session is one task's C clients.
type session struct {
	clients map[string]*deviceClient
	control *cControl
}

func (s *session) Fingerprints() map[string]string {
	result := make(map[string]string, len(s.clients))
	for name, client := range s.clients {
		result[name] = client.fingerprint
	}
	return result
}

// CloseStreams has nothing to do: the C runner supports no operation that
// holds a stream open across steps.
func (s *session) CloseStreams() error { return nil }

func (s *session) Close() {
	for _, client := range s.clients {
		client.Close()
	}
	s.control.Close()
}

func (s *session) client(name string) (*deviceClient, error) {
	client, ok := s.clients[name]
	if !ok {
		return nil, fmt.Errorf("unknown client %q", name)
	}
	return client, nil
}

func (s *session) Execute(ctx context.Context, req giztest.StepRequest) (giztest.StepResult, error) {
	client, err := s.client(req.Step.Client)
	if err != nil {
		return giztest.StepResult{}, err
	}
	switch req.Step.Operation() {
	case "rpc":
		return s.executeRPC(ctx, client, req)
	case "client_rpc":
		return s.executeClientRPC(ctx, client, req)
	case "http":
		return s.executeHTTP(ctx, client, req)
	}
	return giztest.StepResult{}, fmt.Errorf("unsupported operation %q", req.Step.Operation())
}

func (s *session) executeRPC(ctx context.Context, client *deviceClient, req giztest.StepRequest) (giztest.StepResult, error) {
	info, err := lookupMethod(req.Step.RPC.Method)
	if err != nil {
		return giztest.StepResult{}, err
	}
	request := req.Step.RPC.Request
	if request == nil {
		request = map[string]any{}
	}
	resolved, err := req.Vars.Resolve(request)
	if err != nil {
		return giztest.StepResult{}, err
	}
	payload, err := encodePayload(info.request, resolved)
	if err != nil {
		return giztest.StepResult{}, err
	}
	result, err := client.callRPC(ctx, uint32(info.id), payload)
	if err != nil {
		return giztest.StepResult{}, err
	}
	if result.errorCode != 0 {
		return giztest.StepResult{}, &rpcError{
			method: req.Step.RPC.Method, code: result.errorCode, message: result.message,
		}
	}
	decoded, err := decodePayload(info.response, result.payload)
	if err != nil {
		return giztest.StepResult{}, err
	}
	return giztest.StepResult{Value: decoded, Saved: decoded}, nil
}

// executeClientRPC waits for the Server to invoke the scripted provider the
// document installed at connect time.
func (s *session) executeClientRPC(ctx context.Context, client *deviceClient, req giztest.StepRequest) (giztest.StepResult, error) {
	method := req.Step.ClientRPC.Method
	if !client.provider.installed(method) {
		return giztest.StepResult{}, fmt.Errorf("client RPC %s was not installed", method)
	}
	want := int64(req.Step.ClientRPC.ExpectCalls)
	if want == 0 {
		want = 1
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		calls := client.provider.callCount(method)
		if calls >= want {
			evidence := map[string]any{"method": method, "calls": calls}
			return giztest.StepResult{Value: evidence, Saved: evidence, Evidence: evidence}, nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return giztest.StepResult{}, fmt.Errorf(
				"client RPC %s calls = %d, want at least %d: %w", method, calls, want, context.Cause(ctx))
		}
	}
}

// executeHTTP sends one `/gizclaw/v1` request through the controller SDK and
// hands the runner the raw response body so expect and capture see the wire
// JSON, while the SDK itself performed the typed call.
func (s *session) executeHTTP(_ context.Context, client *deviceClient, req giztest.StepRequest) (giztest.StepResult, error) {
	step := req.Step
	pathValue, err := resolveString(req.Vars, step.HTTP.Path)
	if err != nil {
		return giztest.StepResult{}, fmt.Errorf("http path: %w", err)
	}
	apiKey, err := bearerToken(req.Vars, step.HTTP.Headers)
	if err != nil {
		return giztest.StepResult{}, err
	}
	body := ""
	if step.HTTP.Body != nil {
		resolved, err := req.Vars.Resolve(step.HTTP.Body)
		if err != nil {
			return giztest.StepResult{}, err
		}
		encoded, err := json.Marshal(resolved)
		if err != nil {
			return giztest.StepResult{}, err
		}
		body = string(encoded)
	}
	result, err := s.control.Request(client.baseURL, apiKey, step.HTTP.Method, pathValue, body)
	if err != nil {
		return giztest.StepResult{}, err
	}
	evidence := map[string]any{"method": step.HTTP.Method, "path": pathValue, "status": result.status}
	if result.kind != 0 {
		evidence["error_kind"] = result.kind
	}
	outcome := giztest.StepResult{Evidence: evidence}
	if len(strings.TrimSpace(string(result.body))) > 0 {
		var decoded any
		if json.Unmarshal(result.body, &decoded) == nil {
			outcome.Value = decoded
		} else {
			outcome.Value = string(result.body)
		}
		outcome.Saved = outcome.Value
	}
	if step.HTTP.Status != 0 && result.status != step.HTTP.Status {
		return outcome, giztest.NewAssertionError(
			fmt.Errorf("http status = %d, want %d", result.status, step.HTTP.Status))
	}
	if step.HTTP.Status == 0 && result.status >= http.StatusBadRequest {
		return outcome, giztest.NewAssertionError(fmt.Errorf("http status = %d", result.status))
	}
	return outcome, nil
}

// bearerToken extracts the API key the step passes in its Authorization
// header; the controller SDK owns building the header itself.
func bearerToken(vars *giztest.Variables, headers map[string]string) (string, error) {
	for name, raw := range headers {
		if !strings.EqualFold(name, "Authorization") {
			return "", fmt.Errorf("the C controller SDK sends no %s header", name)
		}
		value, err := resolveString(vars, raw)
		if err != nil {
			return "", fmt.Errorf("header %s: %w", name, err)
		}
		return value, nil
	}
	return "", fmt.Errorf("an http step must supply an Authorization header")
}

// deviceClient owns one connected C SDK client plus the goroutine that polls
// it. The C SDK requires a single serialized poll owner, so every call into
// the session runs on that goroutine while it keeps polling in between.
type deviceClient struct {
	name        string
	baseURL     string
	fingerprint string
	provider    *clientRPCProvider

	commands chan func(*cSession)
	stopped  chan struct{}
	closeOne chan struct{}
}

func newDeviceClient(ctx context.Context, name, endpoint string, steps []giztest.Step) (*deviceClient, error) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	baseURL, err := controlBaseURL(endpoint)
	if err != nil {
		return nil, err
	}
	provider := newClientRPCProvider()
	for _, step := range steps {
		if step.ClientRPC == nil || step.Client != name {
			continue
		}
		if err := provider.install(step.ClientRPC.Method, step.ClientRPC.Response); err != nil {
			return nil, err
		}
	}
	client := &deviceClient{
		name:        name,
		baseURL:     baseURL,
		fingerprint: key.Public.ShortString(),
		provider:    provider,
		commands:    make(chan func(*cSession)),
		stopped:     make(chan struct{}),
		closeOne:    make(chan struct{}),
	}
	ready := make(chan error, 1)
	go client.run(endpoint, key.Private.String(), ready)
	select {
	case err := <-ready:
		if err != nil {
			return nil, err
		}
	case <-ctx.Done():
		close(client.closeOne)
		return nil, context.Cause(ctx)
	}
	return client, nil
}

// run owns the C session for its whole life: it dials, then alternates
// between running queued commands and polling the transport so inbound
// server-initiated RPCs are answered while the runner waits elsewhere.
func (c *deviceClient) run(endpoint, privateKey string, ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(c.stopped)

	cSession, err := openSession(endpoint, privateKey, c.provider)
	ready <- err
	if err != nil {
		return
	}
	defer cSession.Close()
	for {
		select {
		case command := <-c.commands:
			command(cSession)
		case <-c.closeOne:
			return
		default:
			if pollErr := cSession.Poll(10); pollErr != nil {
				return
			}
		}
	}
}

// submit runs fn on the poll-owning goroutine and waits for it.
func (c *deviceClient) submit(ctx context.Context, fn func(*cSession)) error {
	done := make(chan struct{})
	wrapped := func(s *cSession) {
		defer close(done)
		fn(s)
	}
	select {
	case c.commands <- wrapped:
	case <-c.stopped:
		return fmt.Errorf("client %s is closed", c.name)
	case <-ctx.Done():
		return context.Cause(ctx)
	}
	select {
	case <-done:
		return nil
	case <-c.stopped:
		return fmt.Errorf("client %s closed during the call", c.name)
	}
}

func (c *deviceClient) callRPC(ctx context.Context, method uint32, payload []byte) (rpcResult, error) {
	var result rpcResult
	var callErr error
	if err := c.submit(ctx, func(s *cSession) { result, callErr = s.CallRPC(method, payload) }); err != nil {
		return rpcResult{}, err
	}
	return result, callErr
}

func (c *deviceClient) Close() {
	select {
	case <-c.stopped:
		return
	default:
	}
	close(c.closeOne)
	<-c.stopped
}

// controlBaseURL turns a client access point into the origin the controller
// SDK targets.
func controlBaseURL(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("invalid access point %q", endpoint)
	}
	parsed.Path, parsed.RawQuery, parsed.Fragment = "", "", ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

var _ = rpcpb.RpcMethod_RPC_METHOD_UNSPECIFIED
