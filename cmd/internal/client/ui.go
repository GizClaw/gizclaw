package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw"
	adminui "github.com/GizClaw/gizclaw-go/ui/apps/admin"
	playui "github.com/GizClaw/gizclaw-go/ui/apps/play"
)

const uiAPIProxyTimeout = 30 * time.Second

func ListenAndServeAdminUI(ctxName, addr string, out io.Writer) error {
	return listenAndServeUI(ctxName, addr, "GizClaw Admin UI", adminui.FS(), out, nil)
}

func ListenAndServePlayUI(ctxName, addr string, out io.Writer) error {
	return listenAndServeUI(ctxName, addr, "GizClaw Play UI", playui.FS(), out, ensurePlayRegistration)
}

func listenAndServeUI(ctxName, addr, title string, uiFS fs.FS, out io.Writer, beforeServe func(context.Context, *gizclaw.Client) error) error {
	if strings.TrimSpace(addr) == "" {
		return fmt.Errorf("gizclaw: empty listen addr")
	}
	listener, err := net.Listen("tcp", normalizeListenAddr(addr))
	if err != nil {
		return fmt.Errorf("gizclaw: listen ui: %w", err)
	}

	c, err := ConnectFromContext(ctxName)
	if err != nil {
		_ = listener.Close()
		return err
	}
	apiProxy := newUIAPIProxy(func() (uiAPIProxyClient, error) {
		return ConnectFromContext(ctxName)
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
	mux.Handle("/api/", apiProxy)
	mux.Handle("/api", apiProxy)
	mux.Handle("/", staticWithSPAFallback(uiFS))

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

func ensurePlayRegistration(ctx context.Context, c *gizclaw.Client) error {
	_, err := c.GetGearRegistration(ctx, "gear.registration.get")
	return err
}

// staticWithSPAFallback serves embedded UI assets and falls back to index.html
// for client-side routes (e.g. /peers/...) so BrowserRouter deep links work.
func staticWithSPAFallback(uiFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(uiFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if clean != "" {
			if _, err := fs.Stat(uiFS, clean); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		r2 := r.Clone(r.Context())
		r2.URL = r.URL
		u := *r.URL
		u.Path = "/"
		r2.URL = &u
		fileServer.ServeHTTP(w, r2)
	})
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
