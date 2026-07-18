package webui

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type Context struct {
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Endpoint       string `json:"endpoint"`
	LocalPublicKey string `json:"local_public_key"`
}

type Runtime struct {
	Context           *Context             `json:"context"`
	PrivateKeyBase64  string               `json:"private_key_base64"`
	RegistrationToken string               `json:"registration_token,omitempty"`
	AdminServers      []AdminServerRuntime `json:"admin_servers,omitempty"`
	AdminServerID     string               `json:"admin_server_id,omitempty"`
}

type AdminServerRuntime struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Context          *Context `json:"context"`
	PrivateKeyBase64 string   `json:"private_key_base64"`
}

type Manager struct {
	Assets fs.FS

	mu      sync.Mutex
	servers map[string]*surfaceServer
}

type surfaceServer struct {
	listener net.Listener
	server   *http.Server
	baseURL  string
	entry    string
	done     chan struct{}

	mu       sync.Mutex
	handoffs map[string]Runtime
}

func New(assets fs.FS) *Manager {
	return &Manager{Assets: assets, servers: map[string]*surfaceServer{}}
}

func (m *Manager) LaunchURL(podID, surface string, runtime Runtime) (string, error) {
	if surface != "admin" && surface != "play" {
		return "", fmt.Errorf("webui: unsupported surface %q", surface)
	}
	if runtime.Context == nil || runtime.PrivateKeyBase64 == "" {
		return "", fmt.Errorf("webui: incomplete runtime handoff")
	}
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	key := podID + ":" + surface
	m.mu.Lock()
	server := m.servers[key]
	if server != nil {
		select {
		case <-server.done:
			delete(m.servers, key)
			server = nil
		default:
		}
	}
	if server == nil {
		var err error
		server, err = m.start(key, surface+".html")
		if err != nil {
			m.mu.Unlock()
			return "", err
		}
		m.servers[key] = server
	}
	server.mu.Lock()
	server.handoffs[token] = runtime
	server.mu.Unlock()
	m.mu.Unlock()
	query := url.Values{"token": {token}}
	return server.baseURL + "/?" + query.Encode(), nil
}

func RuntimeFromPrivateKey(name, description, endpoint, privateKey string) (Runtime, error) {
	var key giznet.Key
	if err := key.UnmarshalText([]byte(privateKey)); err != nil {
		return Runtime{}, fmt.Errorf("webui: invalid private key: %w", err)
	}
	kp, err := giznet.NewKeyPair(key)
	if err != nil {
		return Runtime{}, err
	}
	return Runtime{
		Context:          &Context{Name: name, Description: description, Endpoint: endpoint, LocalPublicKey: kp.Public.String()},
		PrivateKeyBase64: base64.StdEncoding.EncodeToString(kp.Private[:]),
	}, nil
}

func (m *Manager) ClosePod(podID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, server := range m.servers {
		if strings.HasPrefix(key, podID+":") {
			_ = server.close()
			delete(m.servers, key)
		}
	}
}

func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, server := range m.servers {
		_ = server.close()
		delete(m.servers, key)
	}
}

func (s *surfaceServer) close() error {
	s.mu.Lock()
	clear(s.handoffs)
	s.mu.Unlock()
	return s.server.Close()
}

func (m *Manager) start(key, entry string) (*surfaceServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("webui: listen: %w", err)
	}
	server := &surfaceServer{
		listener: listener,
		baseURL:  "http://" + listener.Addr().String(),
		entry:    entry,
		done:     make(chan struct{}),
		handoffs: map[string]Runtime{},
	}
	server.server = &http.Server{
		Handler:           server.handler(m.Assets),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       time.Minute,
	}
	go func() {
		_ = server.server.Serve(listener)
		close(server.done)
		m.mu.Lock()
		if m.servers[key] == server {
			delete(m.servers, key)
		}
		m.mu.Unlock()
	}()
	return server, nil
}

func (s *surfaceServer) handler(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self' http: https:; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'")
		if r.URL.Path == "/__gizclaw/runtime" {
			s.serveRuntime(w, r)
			return
		}
		if r.URL.Path == "/" {
			data, err := fs.ReadFile(assets, s.entry)
			if err != nil {
				http.Error(w, "UI entry is unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(data)
			return
		}
		if strings.HasSuffix(strings.ToLower(r.URL.Path), ".html") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func (s *surfaceServer) serveRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.serveHandoff(w, r)
}

func (s *surfaceServer) serveHandoff(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != s.baseURL {
		http.Error(w, "invalid origin", http.StatusForbidden)
		return
	}
	var request struct {
		Token string `json:"token"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	if err := decoder.Decode(&request); err != nil || request.Token == "" {
		http.Error(w, "invalid handoff", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	runtime, ok := s.handoffs[request.Token]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "unknown runtime token", http.StatusUnauthorized)
		return
	}
	writeRuntime(w, runtime)
}

func writeRuntime(w http.ResponseWriter, runtime Runtime) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(runtime)
}

func randomToken() (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("webui: generate handoff: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}
