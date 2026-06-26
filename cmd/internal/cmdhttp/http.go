package cmdhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/clientapi"
	"github.com/GizClaw/gizclaw-go/cmd/internal/connection"
	"github.com/GizClaw/gizclaw-go/cmd/internal/publicapi"
	adminui "github.com/GizClaw/gizclaw-go/cmd/ui/admin"
	playui "github.com/GizClaw/gizclaw-go/cmd/ui/play"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
)

const (
	uiAPIProxyTimeout       = 5 * time.Second
	playWorkspaceRPCTimeout = 30 * time.Second
)

var playOpenAIHTTPClient = func(c *gizcli.Client) *http.Client {
	return c.HTTPClient(gizcli.ServiceOpenAI)
}

func ListenAndServeAdminUI(ctxName, addr string, out io.Writer) error {
	return listenAndServeUI(ctxName, addr, "GizClaw Admin UI", adminui.Handler(), out, nil, registerAdminUIAPIProxyRoutes, func(mux *http.ServeMux, _ clientapi.ClientProvider, _ clientapi.ClientInvalidator) {
		registerAdminUIRoutes(mux)
	})
}

func ListenAndServePlayUI(ctxName, addr string, out io.Writer) error {
	return listenAndServeUI(ctxName, addr, "GizClaw Play UI", playui.Handler(), out, ensurePlayReady, registerPlayUIAPIProxyRoutes, registerPlayUIRoutes)
}

func listenAndServeUI(
	ctxName, addr, title string,
	uiHandler http.Handler,
	out io.Writer,
	beforeServe func(context.Context, *gizcli.Client) error,
	registerProxyRoutes func(*http.ServeMux, http.Handler),
	registerRoutes func(*http.ServeMux, clientapi.ClientProvider, clientapi.ClientInvalidator),
) error {
	if strings.TrimSpace(addr) == "" {
		return fmt.Errorf("gizclaw: empty listen addr")
	}
	listener, err := net.Listen("tcp", normalizeListenAddr(addr))
	if err != nil {
		return fmt.Errorf("gizclaw: listen ui: %w", err)
	}

	c, err := connection.ConnectFromContext(ctxName)
	if err != nil {
		_ = listener.Close()
		return err
	}
	apiProxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		return connection.ConnectFromContext(ctxName)
	}, uiAPIProxyTimeout)
	apiProxy.set(c)
	defer apiProxy.Close()

	if beforeServe != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := beforeServe(ctx, c); err != nil {
			_ = listener.Close()
			return err
		}
	}

	mux := http.NewServeMux()
	registerProxyRoutes(mux, apiProxy)
	if registerRoutes != nil {
		registerRoutes(mux, apiProxy.gizCLIClient, apiProxy.invalidateGizCLIClient)
	}
	mux.Handle("/", uiHandler)

	server := &http.Server{
		Handler: mux,
		BaseContext: func(net.Listener) context.Context {
			return context.Background()
		},
	}

	if out != nil {
		_, _ = fmt.Fprintf(out, "%s listening on %s\n", title, displayURL(listener.Addr()))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()

	err = server.Serve(listener)
	if err == nil || err == http.ErrServerClosed {
		return nil
	}
	return err
}

func registerAdminUIAPIProxyRoutes(mux *http.ServeMux, apiProxy http.Handler) {
	mux.Handle("/api/", apiProxy)
	mux.Handle("/api", apiProxy)
	registerOpenAIAPIProxyRoutes(mux, apiProxy)
}

func registerPlayUIAPIProxyRoutes(mux *http.ServeMux, apiProxy http.Handler) {
	mux.HandleFunc("/api/", http.NotFound)
	mux.HandleFunc("/api", http.NotFound)
	registerOpenAIAPIProxyRoutes(mux, apiProxy)
}

func registerOpenAIAPIProxyRoutes(mux *http.ServeMux, apiProxy http.Handler) {
	mux.Handle("/v1/", apiProxy)
	mux.Handle("/v1", apiProxy)
}

type uiAPIProxyClient interface {
	Close() error
	ProxyHandler() http.Handler
}

type uiAPIProxy struct {
	connect func() (uiAPIProxyClient, error)
	timeout time.Duration

	mu     sync.Mutex
	client uiAPIProxyClient
}

func newUIAPIProxy(connect func() (uiAPIProxyClient, error), timeout time.Duration) *uiAPIProxy {
	if timeout <= 0 {
		timeout = uiAPIProxyTimeout
	}
	return &uiAPIProxy{
		connect: connect,
		timeout: timeout,
	}
}

func (p *uiAPIProxy) set(client uiAPIProxyClient) {
	p.mu.Lock()
	p.client = client
	p.mu.Unlock()
}

func (p *uiAPIProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isUIAPIProxyBufferedAPIRequest(r) {
		if err := p.serveBuffered(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		}
		return
	}
	if !isUIAPIProxyRetryable(r) {
		if err := p.serveDirect(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		}
		return
	}
	response, client, err := p.serveOnce(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if isUIAPIProxyRetryable(r) && isUIAPIProxyFailure(response.statusCode()) {
		p.invalidate(client)
		response, _, err = p.serveOnce(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
	}
	response.writeTo(w)
}

func (p *uiAPIProxy) serveBuffered(w http.ResponseWriter, r *http.Request) error {
	body, err := readReplayBody(r)
	if err != nil {
		return err
	}
	var response *bufferedHTTPResponse
	for attempt, maxAttempts := 0, uiAPIProxyMaxAttempts(r); attempt < maxAttempts; attempt++ {
		var client uiAPIProxyClient
		response, client, err = p.serveOnce(requestWithReplayBody(r, body))
		if err != nil {
			return err
		}
		if !isUIAPIProxyFailure(response.statusCode()) {
			break
		}
		p.invalidate(client)
		if !isUIAPIProxyReplaySafeRequest(r) {
			break
		}
	}
	response.writeTo(w)
	return nil
}

func (p *uiAPIProxy) serveDirect(w http.ResponseWriter, r *http.Request) error {
	client, err := p.get()
	if err != nil {
		return err
	}

	client.ProxyHandler().ServeHTTP(w, r)
	if r.Context().Err() != nil {
		p.invalidate(client)
	}
	return nil
}

func readReplayBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(r.Body)
	if closeErr := r.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	return body, nil
}

func requestWithReplayBody(r *http.Request, body []byte) *http.Request {
	clone := r.Clone(r.Context())
	if body == nil {
		clone.Body = nil
		clone.GetBody = nil
		clone.ContentLength = 0
		return clone
	}
	clone.Body = io.NopCloser(bytes.NewReader(body))
	clone.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	clone.ContentLength = int64(len(body))
	return clone
}

func (p *uiAPIProxy) serveOnce(r *http.Request) (*bufferedHTTPResponse, uiAPIProxyClient, error) {
	client, err := p.get()
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(r.Context(), p.timeout)
	defer cancel()

	response := newBufferedHTTPResponse()
	client.ProxyHandler().ServeHTTP(response, r.WithContext(ctx))
	if ctx.Err() != nil {
		p.invalidate(client)
	}
	return response, client, nil
}

func (p *uiAPIProxy) Close() error {
	p.mu.Lock()
	client := p.client
	p.client = nil
	p.mu.Unlock()
	if client != nil {
		return client.Close()
	}
	return nil
}

func (p *uiAPIProxy) get() (uiAPIProxyClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil {
		return p.client, nil
	}
	client, err := p.connect()
	if err != nil {
		return nil, err
	}
	p.client = client
	return client, nil
}

func (p *uiAPIProxy) gizCLIClient() (*gizcli.Client, error) {
	client, err := p.get()
	if err != nil {
		return nil, err
	}
	c, ok := client.(*gizcli.Client)
	if !ok {
		return nil, fmt.Errorf("gizclaw: unexpected ui client %T", client)
	}
	return c, nil
}

func (p *uiAPIProxy) invalidateGizCLIClient(c *gizcli.Client) {
	if c == nil {
		return
	}
	p.invalidate(c)
}

func (p *uiAPIProxy) invalidate(stale uiAPIProxyClient) {
	p.mu.Lock()
	if p.client != stale {
		p.mu.Unlock()
		return
	}
	p.client = nil
	p.mu.Unlock()
	_ = stale.Close()
}

func isUIAPIProxyBufferedAPIRequest(r *http.Request) bool {
	return r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/")
}

func isUIAPIProxyReplaySafeRequest(r *http.Request) bool {
	if isUIAPIProxyRetryable(r) {
		return true
	}
	return r.Method == http.MethodPost && r.URL.Path == "/api/admin/social/friends"
}

func uiAPIProxyMaxAttempts(r *http.Request) int {
	if r.Method == http.MethodPost && r.URL.Path == "/api/admin/social/friends" {
		return 3
	}
	if isUIAPIProxyReplaySafeRequest(r) {
		return 2
	}
	return 1
}

func isUIAPIProxyRetryable(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func isUIAPIProxyFailure(statusCode int) bool {
	switch statusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

type bufferedHTTPResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newBufferedHTTPResponse() *bufferedHTTPResponse {
	return &bufferedHTTPResponse{header: make(http.Header)}
}

func (r *bufferedHTTPResponse) Header() http.Header {
	return r.header
}

func (r *bufferedHTTPResponse) WriteHeader(statusCode int) {
	if r.status != 0 {
		return
	}
	r.status = statusCode
}

func (r *bufferedHTTPResponse) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(data)
}

func (r *bufferedHTTPResponse) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func (r *bufferedHTTPResponse) writeTo(w http.ResponseWriter) {
	for key, values := range r.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(r.statusCode())
	_, _ = r.body.WriteTo(w)
}

func ensurePlayReady(ctx context.Context, c *gizcli.Client) error {
	_, err := publicapi.GetServerInfo(ctx, c)
	return err
}

func registerPlayUIRoutes(mux *http.ServeMux, client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator) {
	mux.HandleFunc("/v1/audio/speech", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		playOpenAIBufferedProxy(client, invalidate, w, r, "/v1/audio/speech")
	})
	mux.HandleFunc("/peer-run/workspace", handlePlayWorkspaceState(client, invalidate))
	mux.HandleFunc("/peer-run/workspace/details", handlePlayWorkspaceDetails(client, invalidate))
	mux.HandleFunc("/peer-run/workspace/reload", handlePlayWorkspaceReload(client, invalidate))
	mux.HandleFunc("/peer-run/workspace/mode", handlePlayWorkspaceMode(client, invalidate))
	mux.HandleFunc("/peer-run/workspace/history", handlePlayWorkspaceHistory(client, invalidate))
	mux.HandleFunc("/peer-run/workspace/history/play", handlePlayWorkspaceHistoryPlay(client, invalidate))
	mux.HandleFunc("/peer-run/workspace/memory/stats", handlePlayWorkspaceMemoryStats(client, invalidate))
	mux.HandleFunc("/peer-run/workspace/recall", handlePlayWorkspaceRecall(client, invalidate))
	handler := clientapi.Handler(client, invalidate)
	retryingHandler := retryingPlayClientAPIHandler(client, invalidate, handler)
	for _, pattern := range []string{
		"/peer-resources",
		"/peer-resources/",
		"/v1/voices",
	} {
		mux.Handle(pattern, retryingHandler)
	}
	for _, pattern := range []string{
		"/play/voices/stream",
		"/webrtc/offer",
	} {
		mux.Handle(pattern, handler)
	}
}

func retryingPlayClientAPIHandler(client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Body != nil {
			var err error
			body, err = io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read request body: "+err.Error(), http.StatusBadRequest)
				return
			}
		}
		response := newBufferedHTTPResponse()
		next.ServeHTTP(response, requestWithReplayBody(r, body))
		if !isPlayClientAPIStaleResponse(response) {
			response.writeTo(w)
			return
		}

		invalidatePlayClient(client, invalidate)
		if !isPlayClientAPIReplaySafe(r) {
			response.writeTo(w)
			return
		}

		retryResponse := newBufferedHTTPResponse()
		next.ServeHTTP(retryResponse, requestWithReplayBody(r, body))
		retryResponse.writeTo(w)
	})
}

func invalidatePlayClient(client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator) {
	if client == nil || invalidate == nil {
		return
	}
	c, err := client()
	if err != nil {
		return
	}
	invalidate(c)
}

func isPlayClientAPIReplaySafe(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func isPlayClientAPIStaleResponse(response *bufferedHTTPResponse) bool {
	if response == nil {
		return false
	}
	switch response.statusCode() {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
	default:
		return false
	}
	message := strings.ToLower(response.body.String())
	for _, marker := range []string{
		"kcp: stream aborted by peer",
		"kcp: stream closed by peer",
		"kcp: stream closed as invalid",
		"kcp: conn closed",
		"kcp: service mux closed",
		"gizclaw: client is not connected",
		"giznet: conn closed",
		"net: connection closed",
		"use of closed network connection",
		"io: read/write on closed pipe",
		"connection reset by peer",
		"broken pipe",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

type playWorkspaceSetRequest struct {
	WorkspaceName string `json:"workspace_name"`
}

type playWorkspaceModeRequest struct {
	Mode          string `json:"mode"`
	WorkspaceName string `json:"workspace_name,omitempty"`
}

type playWorkspaceDetailsRequest struct {
	Parameters    *rpcapi.WorkspaceParameters `json:"parameters,omitempty"`
	WorkspaceName string                      `json:"workspace_name,omitempty"`
	WorkflowName  string                      `json:"workflow_name,omitempty"`
}

type playWorkspaceState struct {
	ActiveWorkspaceName  string `json:"active_workspace_name,omitempty"`
	AgentType            string `json:"agent_type,omitempty"`
	Message              string `json:"message,omitempty"`
	PendingWorkspaceName string `json:"pending_workspace_name,omitempty"`
	RuntimeState         string `json:"runtime_state"`
	WorkspaceMode        string `json:"workspace_mode,omitempty"`
	WorkspaceName        string `json:"workspace_name"`
	WorkflowName         string `json:"workflow_name,omitempty"`
}

func handlePlayWorkspaceDetails(client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			workspaceName := strings.TrimSpace(r.URL.Query().Get("workspace_name"))
			workspace, ok := playWorkspaceGetByName(w, r, client, invalidate, workspaceName)
			if !ok {
				return
			}
			writePlayWorkspaceJSON(w, http.StatusOK, workspace)
		case http.MethodPut:
			var req playWorkspaceDetailsRequest
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
				playWorkspaceHTTPError(w, http.StatusBadRequest, "invalid workspace details json")
				return
			}
			workspace, ok := playWorkspaceGetByName(w, r, client, invalidate, req.WorkspaceName)
			if !ok {
				return
			}
			if strings.TrimSpace(req.WorkflowName) != "" {
				workspace.WorkflowName = strings.TrimSpace(req.WorkflowName)
			}
			if req.Parameters != nil {
				workspace.Parameters = req.Parameters
			}
			c, ok := playWorkspaceClient(w, client)
			if !ok {
				return
			}
			ctx, cancel := playWorkspaceRPCContext(r)
			defer cancel()
			updated, err := c.PutWorkspace(ctx, playWorkspaceRPCID(), rpcapi.WorkspacePutRequest{Name: workspace.Name, Body: *workspace})
			if err != nil {
				if invalidate != nil {
					invalidate(c)
				}
				playWorkspaceHTTPError(w, http.StatusBadGateway, err.Error())
				return
			}
			writePlayWorkspaceJSON(w, http.StatusOK, updated)
		default:
			w.Header().Set("Allow", "GET, PUT")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

func handlePlayWorkspaceState(client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			c, ok := playWorkspaceClient(w, client)
			if !ok {
				return
			}
			ctx, cancel := playWorkspaceRPCContext(r)
			defer cancel()
			state, err := c.GetServerRunWorkspace(ctx, playWorkspaceRPCID())
			writePlayWorkspaceState(w, r, c, invalidate, state, err)
		case http.MethodPut:
			var req playWorkspaceSetRequest
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
				playWorkspaceHTTPError(w, http.StatusBadRequest, "invalid workspace set json")
				return
			}
			workspaceName := strings.TrimSpace(req.WorkspaceName)
			if workspaceName == "" {
				playWorkspaceHTTPError(w, http.StatusBadRequest, "workspace_name is required")
				return
			}
			c, ok := playWorkspaceClient(w, client)
			if !ok {
				return
			}
			ctx, cancel := playWorkspaceRPCContext(r)
			defer cancel()
			state, err := c.SetServerRunWorkspace(ctx, playWorkspaceRPCID(), rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: workspaceName})
			writePlayWorkspaceState(w, r, c, invalidate, state, err)
		default:
			w.Header().Set("Allow", "GET, PUT")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

func handlePlayWorkspaceReload(client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		c, ok := playWorkspaceClient(w, client)
		if !ok {
			return
		}
		ctx, cancel := playWorkspaceRPCContext(r)
		defer cancel()
		state, err := c.ReloadServerRunWorkspace(ctx, playWorkspaceRPCID())
		writePlayWorkspaceState(w, r, c, invalidate, state, err)
	}
}

func handlePlayWorkspaceMode(client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.Header().Set("Allow", http.MethodPut)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		var req playWorkspaceModeRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			playWorkspaceHTTPError(w, http.StatusBadRequest, "invalid workspace mode json")
			return
		}
		mode, err := normalizePlayWorkspaceMode(req.Mode)
		if err != nil {
			playWorkspaceHTTPError(w, http.StatusBadRequest, err.Error())
			return
		}
		c, ok := playWorkspaceClient(w, client)
		if !ok {
			return
		}
		ctx, cancel := playWorkspaceRPCContext(r)
		defer cancel()
		workspaceName := strings.TrimSpace(req.WorkspaceName)
		if workspaceName == "" {
			state, err := c.GetServerRunWorkspace(ctx, playWorkspaceRPCID())
			if err != nil {
				if invalidate != nil {
					invalidate(c)
				}
				playWorkspaceHTTPError(w, http.StatusBadGateway, err.Error())
				return
			}
			workspaceName = selectedPlayWorkspaceName(state)
		}
		if workspaceName == "" {
			playWorkspaceHTTPError(w, http.StatusBadRequest, "workspace_name is required")
			return
		}
		workspace, err := c.GetWorkspace(ctx, playWorkspaceRPCID(), rpcapi.WorkspaceGetRequest{Name: workspaceName})
		if err != nil {
			if invalidate != nil {
				invalidate(c)
			}
			playWorkspaceHTTPError(w, http.StatusBadGateway, err.Error())
			return
		}
		if workspace == nil {
			playWorkspaceHTTPError(w, http.StatusBadGateway, "empty workspace response")
			return
		}
		if workspace.Parameters == nil {
			playWorkspaceHTTPError(w, http.StatusBadRequest, "workspace parameters are required")
			return
		}
		params, err := playWorkspaceParametersWithMode(workspace.Parameters, mode)
		if err != nil {
			playWorkspaceHTTPError(w, http.StatusBadRequest, err.Error())
			return
		}
		workspace.Parameters = params
		if _, err := c.PutWorkspace(ctx, playWorkspaceRPCID(), rpcapi.WorkspacePutRequest{Name: workspaceName, Body: *workspace}); err != nil {
			if invalidate != nil {
				invalidate(c)
			}
			playWorkspaceHTTPError(w, http.StatusBadGateway, err.Error())
			return
		}
		state, err := c.ReloadServerRunWorkspace(ctx, playWorkspaceRPCID())
		writePlayWorkspaceState(w, r, c, invalidate, state, err)
	}
}

func playWorkspaceParametersWithMode(parameters *rpcapi.WorkspaceParameters, mode string) (*rpcapi.WorkspaceParameters, error) {
	discriminator, err := parameters.Discriminator()
	if err != nil {
		return nil, fmt.Errorf("decode workspace parameters: %w", err)
	}
	input := rpcapi.WorkspaceInputMode(mode)
	var params rpcapi.WorkspaceParameters
	switch strings.TrimSpace(discriminator) {
	case string(rpcapi.FlowcraftWorkspaceParametersAgentTypeFlowcraft):
		typed, err := parameters.AsFlowcraftWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		typed.Input = &input
		if err := params.FromFlowcraftWorkspaceParameters(typed); err != nil {
			return nil, err
		}
	case string(rpcapi.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime):
		typed, err := parameters.AsDoubaoRealtimeWorkspaceParameters()
		if err != nil {
			return nil, err
		}
		typed.Input = &input
		if err := params.FromDoubaoRealtimeWorkspaceParameters(typed); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported workspace parameter agent_type %q", discriminator)
	}
	return &params, nil
}

func playWorkspaceGetByName(w http.ResponseWriter, r *http.Request, client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator, workspaceName string) (*rpcapi.WorkspaceGetResponse, bool) {
	c, ok := playWorkspaceClient(w, client)
	if !ok {
		return nil, false
	}
	ctx, cancel := playWorkspaceRPCContext(r)
	defer cancel()
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" {
		state, err := c.GetServerRunWorkspace(ctx, playWorkspaceRPCID())
		if err != nil {
			if invalidate != nil {
				invalidate(c)
			}
			playWorkspaceHTTPError(w, http.StatusBadGateway, err.Error())
			return nil, false
		}
		workspaceName = selectedPlayWorkspaceName(state)
	}
	if workspaceName == "" {
		playWorkspaceHTTPError(w, http.StatusBadRequest, "workspace_name is required")
		return nil, false
	}
	workspace, err := c.GetWorkspace(ctx, playWorkspaceRPCID(), rpcapi.WorkspaceGetRequest{Name: workspaceName})
	if err != nil {
		if invalidate != nil {
			invalidate(c)
		}
		playWorkspaceHTTPError(w, http.StatusBadGateway, err.Error())
		return nil, false
	}
	if workspace == nil {
		playWorkspaceHTTPError(w, http.StatusBadGateway, "empty workspace response")
		return nil, false
	}
	return workspace, true
}

func playWorkspaceClient(w http.ResponseWriter, client clientapi.ClientProvider) (*gizcli.Client, bool) {
	c, err := client()
	if err != nil {
		playWorkspaceHTTPError(w, http.StatusServiceUnavailable, err.Error())
		return nil, false
	}
	return c, true
}

func writePlayWorkspaceState(w http.ResponseWriter, r *http.Request, c *gizcli.Client, invalidate clientapi.ClientInvalidator, state *rpcapi.ServerGetRunWorkspaceResponse, err error) {
	if err != nil {
		if invalidate != nil {
			invalidate(c)
		}
		playWorkspaceHTTPError(w, http.StatusBadGateway, err.Error())
		return
	}
	if state == nil {
		playWorkspaceHTTPError(w, http.StatusBadGateway, "empty workspace state response")
		return
	}
	payload := playWorkspaceState{
		RuntimeState:         string(state.RuntimeState),
		WorkspaceName:        strings.TrimSpace(state.WorkspaceName),
		ActiveWorkspaceName:  stringPtrTrimmed(state.ActiveWorkspaceName),
		AgentType:            stringPtrTrimmed(state.AgentType),
		Message:              stringPtrTrimmed(state.Message),
		PendingWorkspaceName: stringPtrTrimmed(state.PendingWorkspaceName),
		WorkflowName:         stringPtrTrimmed(state.WorkflowName),
	}
	if payload.WorkspaceName == "" && state.SelectedWorkspaceName != nil {
		payload.WorkspaceName = strings.TrimSpace(*state.SelectedWorkspaceName)
	}
	if payload.WorkspaceName != "" {
		enrichPlayWorkspaceState(r.Context(), c, &payload)
	}
	writePlayWorkspaceJSON(w, http.StatusOK, payload)
}

func enrichPlayWorkspaceState(ctx context.Context, c *gizcli.Client, state *playWorkspaceState) {
	workspace, err := c.GetWorkspace(ctx, playWorkspaceRPCID(), rpcapi.WorkspaceGetRequest{Name: state.WorkspaceName})
	if err != nil || workspace == nil {
		return
	}
	state.WorkflowName = workspace.WorkflowName
	if workspace.Parameters != nil {
		switch discriminator, err := workspace.Parameters.Discriminator(); {
		case err != nil:
		case strings.TrimSpace(discriminator) == string(rpcapi.FlowcraftWorkspaceParametersAgentTypeFlowcraft):
			state.AgentType = strings.TrimSpace(discriminator)
			typed, err := workspace.Parameters.AsFlowcraftWorkspaceParameters()
			if err == nil && typed.Input != nil {
				state.WorkspaceMode = uiWorkspaceMode(string(*typed.Input))
			}
		case strings.TrimSpace(discriminator) == string(rpcapi.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime):
			state.AgentType = strings.TrimSpace(discriminator)
			typed, err := workspace.Parameters.AsDoubaoRealtimeWorkspaceParameters()
			if err == nil && typed.Input != nil {
				state.WorkspaceMode = uiWorkspaceMode(string(*typed.Input))
			}
		default:
			state.AgentType = strings.TrimSpace(discriminator)
		}
	}
}

func selectedPlayWorkspaceName(state *rpcapi.ServerGetRunWorkspaceResponse) string {
	if state == nil {
		return ""
	}
	for _, value := range []string{
		strings.TrimSpace(state.WorkspaceName),
		stringPtrTrimmed(state.SelectedWorkspaceName),
		stringPtrTrimmed(state.ActiveWorkspaceName),
		stringPtrTrimmed(state.PendingWorkspaceName),
	} {
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizePlayWorkspaceMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "push", "push_to_talk", "push-to-talk", "ptt":
		return "push-to-talk", nil
	case "realtime", "real_time", "real-time":
		return "realtime", nil
	default:
		return "", fmt.Errorf("workspace mode must be push or realtime")
	}
}

func uiWorkspaceMode(mode string) string {
	switch normalized, err := normalizePlayWorkspaceMode(mode); {
	case err != nil:
		return ""
	case normalized == "push-to-talk":
		return "push"
	default:
		return normalized
	}
}

func handlePlayWorkspaceHistory(client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		c, ok := playWorkspaceClient(w, client)
		if !ok {
			return
		}
		ctx, cancel := playWorkspaceRPCContext(r)
		defer cancel()
		resp, err := c.ListServerRunWorkspaceHistory(ctx, playWorkspaceRPCID(), rpcapi.ServerListRunWorkspaceHistoryRequest{})
		if err != nil {
			if invalidate != nil {
				invalidate(c)
			}
			playWorkspaceHTTPError(w, http.StatusBadGateway, err.Error())
			return
		}
		writePlayWorkspaceJSON(w, http.StatusOK, resp)
	}
}

func handlePlayWorkspaceHistoryPlay(client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		var req rpcapi.ServerPlayRunWorkspaceHistoryRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			playWorkspaceHTTPError(w, http.StatusBadRequest, "invalid workspace history play json")
			return
		}
		c, ok := playWorkspaceClient(w, client)
		if !ok {
			return
		}
		ctx, cancel := playWorkspaceRPCContext(r)
		defer cancel()
		resp, err := c.PlayServerRunWorkspaceHistory(ctx, playWorkspaceRPCID(), req)
		if err != nil {
			if invalidate != nil {
				invalidate(c)
			}
			playWorkspaceHTTPError(w, http.StatusBadGateway, err.Error())
			return
		}
		writePlayWorkspaceJSON(w, http.StatusOK, resp)
	}
}

func handlePlayWorkspaceMemoryStats(client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		c, ok := playWorkspaceClient(w, client)
		if !ok {
			return
		}
		ctx, cancel := playWorkspaceRPCContext(r)
		defer cancel()
		resp, err := c.GetServerRunWorkspaceMemoryStats(ctx, playWorkspaceRPCID(), rpcapi.ServerGetRunWorkspaceMemoryStatsRequest{})
		if err != nil {
			if invalidate != nil {
				invalidate(c)
			}
			playWorkspaceHTTPError(w, http.StatusBadGateway, err.Error())
			return
		}
		writePlayWorkspaceJSON(w, http.StatusOK, resp)
	}
}

func handlePlayWorkspaceRecall(client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		var req rpcapi.ServerRunWorkspaceRecallRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			playWorkspaceHTTPError(w, http.StatusBadRequest, "invalid workspace recall json")
			return
		}
		c, ok := playWorkspaceClient(w, client)
		if !ok {
			return
		}
		ctx, cancel := playWorkspaceRPCContext(r)
		defer cancel()
		resp, err := c.ServerRunWorkspaceRecall(ctx, playWorkspaceRPCID(), req)
		if err != nil {
			if invalidate != nil {
				invalidate(c)
			}
			playWorkspaceHTTPError(w, http.StatusBadGateway, err.Error())
			return
		}
		writePlayWorkspaceJSON(w, http.StatusOK, resp)
	}
}

func stringPtrTrimmed(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func playWorkspaceRPCID() string {
	return "play-workspace-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func playWorkspaceRPCContext(r *http.Request) (context.Context, context.CancelFunc) {
	ctx := r.Context()
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, playWorkspaceRPCTimeout)
}

func playWorkspaceHTTPError(w http.ResponseWriter, status int, message string) {
	writePlayWorkspaceJSON(w, status, map[string]any{"error": map[string]string{"message": message}})
}

func writePlayWorkspaceJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func playOpenAIBufferedProxy(client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator, w http.ResponseWriter, r *http.Request, targetPath string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if isOpenAIStreamingSpeechRequest(body) {
		playOpenAIStreamingProxy(client, invalidate, w, r, targetPath, body)
		return
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		c, err := client()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		respHeader, statusCode, respBody, err := doPlayOpenAIRequest(c, r, targetPath, body)
		if err != nil {
			lastErr = err
			if invalidate != nil {
				invalidate(c)
			}
			continue
		}
		if statusCode == http.StatusBadGateway && attempt == 0 {
			lastErr = fmt.Errorf("bad gateway")
			if invalidate != nil {
				invalidate(c)
			}
			continue
		}
		copyHTTPHeaders(w.Header(), respHeader)
		w.Header().Set("Content-Length", strconv.Itoa(len(respBody)))
		w.WriteHeader(statusCode)
		_, _ = w.Write(respBody)
		return
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("bad gateway")
	}
	http.Error(w, lastErr.Error(), http.StatusBadGateway)
}

func isOpenAIStreamingSpeechRequest(body []byte) bool {
	var payload struct {
		StreamFormat string `json:"stream_format"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(payload.StreamFormat), "sse")
}

func playOpenAIStreamingProxy(client clientapi.ClientProvider, invalidate clientapi.ClientInvalidator, w http.ResponseWriter, r *http.Request, targetPath string, body []byte) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		c, err := client()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		resp, err := newPlayOpenAIRequest(c, r, targetPath, body)
		if err != nil {
			lastErr = err
			if invalidate != nil {
				invalidate(c)
			}
			continue
		}
		if resp.StatusCode == http.StatusBadGateway && attempt == 0 {
			lastErr = fmt.Errorf("bad gateway")
			_ = resp.Body.Close()
			if invalidate != nil {
				invalidate(c)
			}
			continue
		}
		defer resp.Body.Close()
		copyHTTPHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		writer := io.Writer(w)
		if flusher, ok := w.(http.Flusher); ok {
			writer = httpFlushWriter{w: w, f: flusher}
		}
		_, _ = io.Copy(writer, resp.Body)
		return
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("bad gateway")
	}
	http.Error(w, lastErr.Error(), http.StatusBadGateway)
}

func doPlayOpenAIRequest(c *gizcli.Client, r *http.Request, targetPath string, body []byte) (http.Header, int, []byte, error) {
	resp, err := newPlayOpenAIRequest(c, r, targetPath, body)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, nil, err
	}
	return resp.Header.Clone(), resp.StatusCode, respBody, nil
}

func newPlayOpenAIRequest(c *gizcli.Client, r *http.Request, targetPath string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, "http://gizclaw"+targetPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = r.URL.RawQuery
	copyHTTPHeaders(req.Header, r.Header)
	resp, err := playOpenAIHTTPClient(c).Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func copyHTTPHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

type httpFlushWriter struct {
	w io.Writer
	f http.Flusher
}

func (w httpFlushWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.f.Flush()
	return n, err
}

func registerAdminUIRoutes(mux *http.ServeMux) {
	redirectWorkflows := func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ai/workflows", http.StatusFound)
	}
	mux.HandleFunc("/workspace-templates", redirectWorkflows)
	mux.HandleFunc("/workspace-templates/", redirectWorkflows)
	mux.HandleFunc("/ai/workspace-templates", redirectWorkflows)
	mux.HandleFunc("/ai/workspace-templates/", redirectWorkflows)
}

func normalizeListenAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return addr
	}
	if strings.Contains(addr, ":") {
		return addr
	}
	return ":" + addr
}

func displayURL(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "http://" + addr.String()
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
