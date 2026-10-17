// infra-flow-demo serves the real KV-backed read API with explicitly synthetic
// five-second aggregate input. It never contacts production services.
package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/infraflow"
	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

//go:embed topology.json
var topology []byte

func main() {
	if err := run(); err != nil {
		slog.Error("infra flow demo stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	listen := flag.String("listen", "127.0.0.1:9819", "loopback HTTP address")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || host != "127.0.0.1" {
		return fmt.Errorf("demo listener must use 127.0.0.1")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store := kv.NewMemory(nil)
	defer store.Close()
	feed, err := infraflow.New(store)
	if err != nil {
		return err
	}
	var fixture struct {
		Links []struct {
			Source, Target string
			Kind           infraapi.FlowKind
		}
	}
	if err := json.Unmarshal(topology, &fixture); err != nil {
		return err
	}
	routes := make([]infraapi.Flow, len(fixture.Links))
	for i, link := range fixture.Links {
		routes[i] = infraapi.Flow{Source: link.Source, Target: link.Target, Kind: link.Kind}
	}
	aggregator, err := infraflow.NewAggregator(infraapi.Prod, routes, true)
	if err != nil {
		return err
	}
	var scale atomic.Int64
	scale.Store(1)
	produce := func(end time.Time) error {
		activityScale := scale.Load()
		for i, route := range routes {
			count := int64(math.Round(100+20*math.Sin(float64(end.Unix())/12+float64(i)))) * activityScale
			unit := int64(1200)
			switch route.Kind {
			case infraapi.Audio:
				unit = 320
			case infraapi.Sync:
				unit = 256
			case infraapi.Asset:
				unit = 65536
			}
			latency := 40 + 20*math.Sin(float64(end.Unix())/17+float64(i))
			if err := aggregator.Add(i, count, count*unit, float64(count)*latency, count); err != nil {
				return err
			}
		}
		writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return feed.Append(writeCtx, aggregator.Drain(end))
	}
	if err := produce(time.Now().Truncate(infraflow.Window)); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(time.Until(time.Now().Truncate(infraflow.Window).Add(infraflow.Window)))
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case at := <-timer.C:
				if err := produce(at); err != nil && ctx.Err() == nil {
					slog.ErrorContext(ctx, "demo aggregation failed", "error", err)
				}
				timer.Reset(time.Until(time.Now().Truncate(infraflow.Window).Add(infraflow.Window)))
			}
		}
	}()
	defer func() { stop(); <-done }()
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	token := "gizclaw_mk_" + base64.RawURLEncoding.EncodeToString(random)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /demo/config", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"topology": json.RawMessage(topology), "endpoint": infraflow.Path, "token": token, "simulated": true, "scale": scale.Load()})
	})
	mux.HandleFunc("POST /demo/scale", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Scale int64 `json:"scale"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || (input.Scale != 1 && input.Scale != 1000000) {
			http.Error(w, "choose 1 or 1000000", 400)
			return
		}
		scale.Store(input.Scale)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(input)
	})
	api := infraflow.Handler(infraflow.Config{Store: "local-demo", ReadToken: token}, feed, mux)
	server := &http.Server{Addr: *listen, Handler: api, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	fmt.Println("Local simulated AP API ready at http://" + *listen + infraflow.Path)
	select {
	case err := <-errCh:
		if err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	err = server.Shutdown(shutdown)
	if err != nil {
		return err
	}
	return nil
}
