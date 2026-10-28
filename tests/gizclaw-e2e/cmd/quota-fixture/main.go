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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/GizClaw/gizclaw-go/sdk/go/quota"
)

type record struct {
	Report      quota.QuotaRequest   `json:"report"`
	Queries     int                  `json:"queries"`
	Pending     int                  `json:"pending"`
	Failures    int                  `json:"failures"`
	Canceled    int                  `json:"canceled"`
	Decision    *quota.QuotaResponse `json:"decision,omitempty"`
	Stale       bool                 `json:"stale"`
	Connections []string             `json:"connections"`
}
type fixture struct {
	mu            sync.Mutex
	records       map[string]record
	first         map[string]time.Time
	forced        map[string]bool
	recovered     map[string]bool
	queries       map[string]int
	gates         map[string]chan struct{}
	providerCalls atomic.Int64
	admin         *adminConnection
}

var modes = []string{"allow", "omitted", "null", "deny", "expire", "renew", "stale", "failure", "malformed", "recover", "dialogue-eino", "slow", "timeout", "denied-stale"}

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
	f := &fixture{records: map[string]record{}, first: map[string]time.Time{}, forced: map[string]bool{}, recovered: map[string]bool{}, queries: map[string]int{}, gates: map[string]chan struct{}{}, admin: &adminConnection{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("POST /v1/quota", f.check)
	mux.HandleFunc("GET /record/{mode}", f.report)
	mux.HandleFunc("GET /evidence", f.evidence)
	mux.HandleFunc("POST /release/{mode}/{peer}", f.releaseHook)
	mux.HandleFunc("GET /queries/{peer}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		queries := f.queries[r.PathValue("peer")]
		f.mu.Unlock()
		writeJSON(w, map[string]any{"queries": queries})
	})
	mux.HandleFunc("POST /deny/{mode}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.forced[r.PathValue("mode")] = true
		f.mu.Unlock()
		time.Sleep(time.Second)
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /prepare", f.prepare)
	mux.HandleFunc("POST /wait-denial", f.waitDenial)
	mux.HandleFunc("POST /recover/{mode}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.recovered[r.PathValue("mode")] = true
		f.mu.Unlock()
		time.Sleep(time.Second)
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /dialogue/{driver}", f.dialogue)
	mux.HandleFunc("GET /provider-calls", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"calls": f.providerCalls.Load()})
	})
	mux.HandleFunc("POST /v1/chat/completions", f.chat)
	mux.HandleFunc("POST /v1/t2a_v2", f.speech)
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
	var report quota.QuotaRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&report); err != nil || report.PeerPublicKey == "" || report.Usage == nil {
		http.Error(w, "invalid report", 400)
		return
	}
	defer r.Body.Close()
	f.mu.Lock()
	f.queries[report.PeerPublicKey]++
	if !known {
		f.mu.Unlock()
		w.WriteHeader(401)
		return
	}
	key := mode + ":" + report.PeerPublicKey
	item := f.records[key]
	item.Report = report
	item.Queries++
	item.Pending++
	if !slices.Contains(item.Connections, r.RemoteAddr) {
		item.Connections = append(item.Connections, r.RemoteAddr)
	}
	f.records[mode] = item
	f.records[key] = item
	if f.first[key].IsZero() {
		f.first[key] = time.Now()
	}
	first := f.first[key]
	forced := f.forced[mode]
	recovered := f.recovered[mode]
	count := item.Queries
	var gate chan struct{}
	if mode == "slow" {
		gate = f.gates[key]
		if gate == nil {
			gate = make(chan struct{})
			f.gates[key] = gate
		}
	}
	f.mu.Unlock()
	var decision *quota.QuotaResponse
	var failed bool
	defer func() {
		canceled := r.Context().Err() != nil
		f.mu.Lock()
		item := f.records[key]
		item.Pending--
		if failed || canceled {
			item.Failures++
		}
		if canceled {
			item.Canceled++
		}
		if decision != nil {
			item.Decision = decision
		}
		f.records[key] = item
		f.records[mode] = item
		f.mu.Unlock()
	}()
	now := time.Now()
	body := map[string]any{"valid_until": now.Add(time.Second)}
	switch mode {
	case "allow":
		body["expires_at"] = now.Add(time.Minute)
	case "null":
		body["expires_at"] = nil
	case "deny":
		body["expires_at"] = now.Add(-time.Hour)
	case "dialogue-eino":
		body["expires_at"] = first.Add(2 * time.Second)
		body["valid_until"] = first.Add(10 * time.Second)
	case "expire":
		body["expires_at"] = first.Add(600 * time.Millisecond)
		body["valid_until"] = first.Add(5 * time.Second)
	case "renew":
		body["expires_at"] = now.Add(600 * time.Millisecond)
		body["valid_until"] = now.Add(300 * time.Millisecond)
	case "stale":
		if count > 1 {
			failed = true
			w.WriteHeader(503)
			return
		}
		body["valid_until"] = now.Add(600 * time.Millisecond)
	case "failure":
		failed = true
		w.WriteHeader(503)
		return
	case "malformed":
		failed = true
		delete(body, "valid_until")
	case "slow":
		select {
		case <-gate:
			body["expires_at"] = time.Now().Add(-time.Hour)
			body["valid_until"] = time.Now().Add(time.Minute)
		case <-r.Context().Done():
			return
		}
	case "timeout":
		<-r.Context().Done()
		return
	case "denied-stale":
		if count > 1 {
			failed = true
			w.WriteHeader(503)
			return
		}
		body["expires_at"] = now.Add(-time.Hour)
		body["valid_until"] = now.Add(600 * time.Millisecond)
	case "recover":
		body["valid_until"] = now.Add(300 * time.Millisecond)
		if !recovered {
			body["expires_at"] = now.Add(-time.Hour)
		}
	}
	if forced {
		body["expires_at"] = now.Add(-time.Hour)
	}
	if validUntil, ok := body["valid_until"].(time.Time); ok {
		decision = &quota.QuotaResponse{ValidUntil: validUntil}
		if expiresAt, ok := body["expires_at"].(time.Time); ok {
			decision.ExpiresAt = &expiresAt
		}
	}
	writeJSON(w, body)
}

// releaseHook lets Giztest hold the first decision until real provider calls
// have completed, then supply an explicit denial through the same HTTP hook.
func (f *fixture) releaseHook(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("mode") + ":" + r.PathValue("peer")
	f.mu.Lock()
	gate := f.gates[key]
	if gate != nil {
		select {
		case <-gate:
		default:
			close(gate)
		}
	}
	f.mu.Unlock()
	if gate == nil {
		http.Error(w, "hook was not started", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"released": true})
}

// evidence exposes only fixture identity and decision counters for the Docker
// runner to correlate real quota warnings with the affected Peer.
func (f *fixture) evidence(w http.ResponseWriter, r *http.Request) {
	var evidence []map[string]any
	f.mu.Lock()
	for key, item := range f.records {
		mode, peer, scoped := strings.Cut(key, ":")
		if scoped {
			evidence = append(evidence, map[string]any{"mode": mode, "peer_public_key": peer, "queries": item.Queries, "failures": item.Failures, "canceled": item.Canceled, "connections": slices.Clone(item.Connections)})
		}
	}
	f.mu.Unlock()
	writeJSON(w, evidence)
}

func (f *fixture) report(w http.ResponseWriter, r *http.Request) {
	minUsage, _ := strconv.Atoi(r.URL.Query().Get("usage"))
	minQueries, _ := strconv.Atoi(r.URL.Query().Get("queries"))
	minFailures, _ := strconv.Atoi(r.URL.Query().Get("failures"))
	minCanceled, _ := strconv.Atoi(r.URL.Query().Get("canceled"))
	minPending, _ := strconv.Atoi(r.URL.Query().Get("pending"))
	stale := r.URL.Query().Get("stale") == "true"
	key := r.PathValue("mode")
	if peer := r.URL.Query().Get("peer"); peer != "" {
		key += ":" + peer
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		f.mu.Lock()
		item, ok := f.records[key]
		f.mu.Unlock()
		item.Stale = item.Decision != nil && !item.Decision.ValidUntil.After(time.Now())
		if ok && item.Queries >= minQueries && item.Failures >= minFailures && item.Canceled >= minCanceled && item.Pending >= minPending && (!stale || item.Stale) && len(item.Report.Usage) >= minUsage {
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
	f.providerCalls.Add(1)
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
	f.providerCalls.Add(1)
	var body struct {
		Text   string `json:"text"`
		Stream bool   `json:"stream"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil || !body.Stream {
		http.Error(w, "invalid streaming speech request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	frames := 1
	if body.Text == "long" {
		frames = 300
	}
	audio := strings.Repeat("0000", 160)
	for range frames {
		if _, err := fmt.Fprintf(w, "data: {\"data\":{\"audio\":%q,\"status\":1}}\n\n", audio); err != nil {
			return
		}
		w.(http.Flusher).Flush()
		if frames > 1 {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	_, _ = io.WriteString(w, "data: {\"data\":{\"status\":2},\"extra_info\":{\"usage_characters\":37}}\n\n")
	w.(http.Flusher).Flush()
}
