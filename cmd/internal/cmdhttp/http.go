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
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
)

const uiAPIProxyTimeout = 30 * time.Second

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
	handler := clientapi.Handler(client, invalidate)
	for _, pattern := range []string{
		"/peer-resources",
		"/peer-resources/",
		"/play/voices/stream",
		"/v1/voices",
		"/webrtc/offer",
	} {
		mux.Handle(pattern, handler)
	}
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
