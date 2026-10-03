// quota-fixture is an isolated E2E quota/provider server, never linked into GizClaw.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/GizClaw/gizclaw-go/sdk/go/quota"
	"github.com/coder/websocket"
)

type record struct {
	Report  quota.QuotaRequest `json:"report"`
	Queries int                `json:"queries"`
}
type fixture struct {
	mu      sync.Mutex
	records map[string]record
	first   map[string]time.Time
	forced  map[string]bool
	admin   *adminConnection
}

var modes = []string{"allow", "omitted", "null", "deny", "expire", "renew", "stale", "failure", "malformed", "recover"}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	initDir := flag.String("init", "", "initialize Docker workspace")
	seedFlag := flag.Bool("seed", false, "seed fixture resources through real Admin transport")
	listen := flag.String("listen", ":9825", "fixture listen address")
	flag.Parse()
	if *initDir != "" {
		return initialize(*initDir)
	}
	if *seedFlag {
		return seed()
	}
	f := &fixture{records: map[string]record{}, first: map[string]time.Time{}, forced: map[string]bool{}, admin: &adminConnection{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("POST /v1/quota", f.check)
	mux.HandleFunc("GET /record/{mode}", f.report)
	mux.HandleFunc("POST /deny/{mode}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.forced[r.PathValue("mode")] = true
		f.mu.Unlock()
		time.Sleep(time.Second)
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /prepare", f.prepare)
	mux.HandleFunc("POST /v1/chat/completions", f.chat)
	mux.HandleFunc("GET /ws/v1/t2a_v2", f.speech)
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	closing, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(closing); err != nil {
		return err
	}
	if f.admin != nil {
		return f.admin.Close()
	}
	return nil
}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func (f *fixture) check(w http.ResponseWriter, r *http.Request) {
	mode := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	known := false
	for _, v := range modes {
		if v == mode {
			known = true
		}
	}
	if !known {
		w.WriteHeader(401)
		return
	}
	var report quota.QuotaRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&report); err != nil || report.PeerPublicKey == "" || report.Usage == nil {
		http.Error(w, "invalid report", 400)
		return
	}
	defer r.Body.Close()
	f.mu.Lock()
	key := mode + ":" + report.PeerPublicKey
	item := f.records[mode]
	item.Report = report
	item.Queries++
	f.records[mode] = item
	if f.first[key].IsZero() {
		f.first[key] = time.Now()
	}
	first := f.first[key]
	forced := f.forced[mode]
	count := item.Queries
	f.mu.Unlock()
	now := time.Now()
	body := map[string]any{"valid_until": now.Add(time.Second)}
	switch mode {
	case "allow":
		body["expires_at"] = now.Add(time.Minute)
	case "null":
		body["expires_at"] = nil
	case "deny":
		body["expires_at"] = now.Add(-time.Hour)
	case "expire":
		body["expires_at"] = first.Add(600 * time.Millisecond)
		body["valid_until"] = first.Add(5 * time.Second)
	case "renew":
		body["expires_at"] = now.Add(600 * time.Millisecond)
		body["valid_until"] = now.Add(300 * time.Millisecond)
	case "stale":
		if count > 1 {
			w.WriteHeader(503)
			return
		}
		body["valid_until"] = now.Add(600 * time.Millisecond)
	case "failure":
		w.WriteHeader(503)
		return
	case "malformed":
		delete(body, "valid_until")
	case "recover":
		body["valid_until"] = now.Add(300 * time.Millisecond)
		if count == 1 {
			body["expires_at"] = now.Add(-time.Hour)
		}
	}
	if forced {
		body["expires_at"] = now.Add(-time.Hour)
	}
	writeJSON(w, body)
}
func (f *fixture) report(w http.ResponseWriter, r *http.Request) {
	minUsage, _ := strconv.Atoi(r.URL.Query().Get("usage"))
	minQueries, _ := strconv.Atoi(r.URL.Query().Get("queries"))
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		f.mu.Lock()
		item, ok := f.records[r.PathValue("mode")]
		f.mu.Unlock()
		if ok && item.Queries >= minQueries && len(item.Report.Usage) >= minUsage {
			// Stable evidence ordering without mutating the captured request.
			item.Report.Usage = slices.Clone(item.Report.Usage)
			slices.SortFunc(item.Report.Usage, func(a, b quota.QuotaUsage) int {
				if order := strings.Compare(a.ModelId, b.ModelId); order != 0 {
					return order
				}
				return a.Hour.Compare(b.Hour)
			})
			if minQueries > 0 {
				time.Sleep(50 * time.Millisecond)
			}
			writeJSON(w, item)
			return
		}
		select {
		case <-ctx.Done():
			http.Error(w, "report not observed", 408)
			return
		case <-ticker.C:
		}
	}
}
func (f *fixture) chat(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		w.WriteHeader(400)
		return
	}
	defer r.Body.Close()
	usage := `"usage":{"prompt_tokens":5,"completion_tokens":4,"total_tokens":9,"prompt_tokens_details":{"cached_tokens":2}}`
	if body["stream"] == true {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, `data: {"id":"fixture","object":"chat.completion.chunk","created":1,"model":"billing-chat","choices":[{"index":0,"delta":{"content":"quota fixture answer"},"finish_reason":""}]}`+"\n\n"+`data: {"id":"fixture","object":"chat.completion.chunk","created":1,"model":"billing-chat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n"+`data: {"id":"fixture","object":"chat.completion.chunk","created":1,"model":"billing-chat","choices":[],`+usage+"}\n\ndata: [DONE]\n\n")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, `{"id":"fixture","object":"chat.completion","created":1,"model":"billing-chat","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"quota fixture answer"}}],`+usage+`}`)
}
func (f *fixture) speech(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	write := func(value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return conn.Write(ctx, websocket.MessageText, data)
	}
	read := func() (map[string]any, error) {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return nil, err
		}
		var v map[string]any
		err = json.Unmarshal(data, &v)
		return v, err
	}
	ok := map[string]any{"status_code": 0, "status_msg": "success"}
	if write(map[string]any{"event": "connected_success", "base_resp": ok}) != nil {
		return
	}
	if _, err = read(); err != nil {
		return
	}
	if write(map[string]any{"event": "task_started", "base_resp": ok}) != nil {
		return
	}
	continued, err := read()
	if err != nil {
		return
	}
	if _, err = read(); err != nil {
		return
	}
	frames := 1
	if continued["text"] == "long" {
		frames = 300
	}
	audio := strings.Repeat("0000", 160)
	for i := 0; i < frames; i++ {
		if write(map[string]any{"event": "task_result", "data": map[string]any{"audio": audio}, "base_resp": ok}) != nil {
			return
		}
		if frames > 1 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	_ = write(map[string]any{"event": "task_continued", "is_final": true, "data": map[string]any{"audio": ""}, "extra_info": map[string]any{"usage_characters": 37}, "base_resp": ok})
	_ = write(map[string]any{"event": "task_finished", "base_resp": ok})
}
