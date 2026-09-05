// Package monitor serves authenticated, process-local monitoring snapshots.
package monitor

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	monitorapi "github.com/GizClaw/gizclaw-go/pkgs/monitor/api"
	monitorweb "github.com/GizClaw/gizclaw-go/web/monitor"
)

// Config grants read-only access to this node; an empty token disables node data.
type Config struct {
	Token string `yaml:"token" json:"-"`
}

// Validate expands environment variables and requires a distinct monitor token.
func (c *Config) Validate() error {
	c.Token = os.ExpandEnv(c.Token)
	if c.Token != "" && (!strings.HasPrefix(c.Token, "gizclaw_mk_") || len(strings.TrimPrefix(c.Token, "gizclaw_mk_")) < 32) {
		return &configError{}
	}
	return nil
}

type configError struct{}

func (*configError) Error() string {
	return "monitor.token must begin with gizclaw_mk_ and contain at least 32 additional characters"
}

type nodeServer struct {
	role, publicKey string
	started         time.Time
}

func (s *nodeServer) GetNodeMonitor(_ context.Context, _ monitorapi.GetNodeMonitorRequestObject) (monitorapi.GetNodeMonitorResponseObject, error) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	snapshot := monitorapi.NodeSnapshot{PublicKey: s.publicKey, Role: s.role, Time: time.Now().UTC(), UptimeSeconds: time.Since(s.started).Seconds(), Goroutines: runtime.NumGoroutine(), HeapBytes: mem.HeapAlloc, Logs: []monitorapi.MonitorLog{}}
	transport := gizwebrtc.ReadMonitorSnapshot()
	snapshot.Transport.Connections = transport.Connections
	snapshot.Transport.Services = transport.Services
	snapshot.Transport.RxBytes = transport.RXBytes
	snapshot.Transport.TxBytes = transport.TXBytes
	for _, entry := range gizlog.ReadMonitorLogs("") {
		log := monitorapi.MonitorLog{Id: entry.ID, Time: entry.Time, Level: entry.Level, Message: entry.Message}
		if entry.Error != "" {
			log.Error = &entry.Error
		}
		if entry.PeerPublicKey != "" {
			log.PeerPublicKey = &entry.PeerPublicKey
		}
		snapshot.Logs = append(snapshot.Logs, log)
	}
	return monitorapi.GetNodeMonitor200JSONResponse(snapshot), nil
}

// Handler shares the embedded UI with a token-protected node snapshot API.
func Handler(cfg Config, role, publicKey string, next http.Handler) http.Handler {
	endpoint := monitorapi.Handler(monitorapi.NewStrictHandler(&nodeServer{role: role, publicKey: publicKey, started: time.Now()}, nil))
	tokenHash := sha256.Sum256([]byte(cfg.Token))
	assets := monitorweb.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/monitor/api/node" {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				w.WriteHeader(405)
				return
			}
			if cfg.Token == "" {
				w.WriteHeader(503)
				_ = json.NewEncoder(w).Encode(monitorapi.MonitorError{Error: monitorapi.MONITORDISABLED})
				return
			}
			scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
			candidate := sha256.Sum256([]byte(token))
			if !ok || !strings.EqualFold(scheme, "Bearer") || subtle.ConstantTimeCompare(tokenHash[:], candidate[:]) != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				w.WriteHeader(401)
				_ = json.NewEncoder(w).Encode(monitorapi.MonitorError{Error: monitorapi.INVALIDMONITORTOKEN})
				return
			}
			endpoint.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/monitor" || strings.HasPrefix(r.URL.Path, "/monitor/") {
			assets.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
