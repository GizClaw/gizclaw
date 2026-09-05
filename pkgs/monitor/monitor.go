// Package monitor serves authenticated, process-local monitoring snapshots.
package monitor

import (
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

// Snapshot contains process-local values, never another node's statistics.
type Snapshot struct {
	PublicKey     string                    `json:"public_key"`
	Role          string                    `json:"role"`
	Time          time.Time                 `json:"time"`
	UptimeSeconds float64                   `json:"uptime_seconds"`
	Goroutines    int                       `json:"goroutines"`
	HeapBytes     uint64                    `json:"heap_bytes"`
	Transport     gizwebrtc.MonitorSnapshot `json:"transport"`
	Logs          []gizlog.MonitorEntry     `json:"logs"`
}

// Handler shares the embedded UI with a token-protected node snapshot API.
func Handler(cfg Config, role, publicKey string, next http.Handler) http.Handler {
	started := time.Now()
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
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "MONITOR_DISABLED"})
				return
			}
			scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
			candidate := sha256.Sum256([]byte(token))
			if !ok || !strings.EqualFold(scheme, "Bearer") || subtle.ConstantTimeCompare(tokenHash[:], candidate[:]) != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				w.WriteHeader(401)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "INVALID_MONITOR_TOKEN"})
				return
			}
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			_ = json.NewEncoder(w).Encode(Snapshot{PublicKey: publicKey, Role: role, Time: time.Now().UTC(), UptimeSeconds: time.Since(started).Seconds(), Goroutines: runtime.NumGoroutine(), HeapBytes: mem.HeapAlloc, Transport: gizwebrtc.ReadMonitorSnapshot(), Logs: gizlog.ReadMonitorLogs("")})
			return
		}
		if r.URL.Path == "/monitor" || strings.HasPrefix(r.URL.Path, "/monitor/") {
			assets.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
