// The test proxy interrupts center-to-Learner Raft streams while keeping the
// Learner reachable by the E2E client. It is never part of a production binary.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type proxy struct {
	mu          sync.Mutex
	frozen      bool
	blocked     int
	leaderIPs   map[string]bool
	connections map[net.Conn]bool
	target      string
}

func (p *proxy) serve(ctx context.Context, source net.Conn) {
	stopClose := context.AfterFunc(ctx, func() { _ = source.Close() })
	defer stopClose()
	host, _, _ := net.SplitHostPort(source.RemoteAddr().String())
	fromLeader := p.leaderIPs[host]
	p.mu.Lock()
	blocked := fromLeader && p.frozen
	if blocked {
		p.blocked++
	} else {
		p.connections[source] = fromLeader
	}
	p.mu.Unlock()
	defer source.Close()
	defer func() { p.mu.Lock(); delete(p.connections, source); p.mu.Unlock() }()
	if blocked {
		return
	}
	target, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", p.target)
	if err != nil {
		return
	}
	defer target.Close()
	var copies sync.WaitGroup
	copies.Go(func() { _, _ = io.Copy(target, source); _ = target.Close(); _ = source.Close() })
	_, _ = io.Copy(source, target)
	_ = source.Close()
	_ = target.Close()
	copies.Wait()
}

func (p *proxy) control(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/status" && r.URL.Path != "/freeze" && r.URL.Path != "/resume" {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path != "/status" && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var closeConnections []net.Conn
	p.mu.Lock()
	if r.URL.Path == "/freeze" {
		p.frozen = true
		for conn, leader := range p.connections {
			if leader {
				closeConnections = append(closeConnections, conn)
				p.blocked++
			}
		}
	} else if r.URL.Path == "/resume" {
		p.frozen = false
	}
	status := struct {
		Frozen  bool `json:"frozen"`
		Blocked int  `json:"blocked"`
	}{p.frozen, p.blocked}
	p.mu.Unlock()
	for _, conn := range closeConnections {
		_ = conn.Close()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func main() {
	leader := flag.String("leader", "leader", "center TiKV hostname")
	target := flag.String("target", "learner:20160", "Learner listener")
	flag.Parse()
	ips, err := net.LookupIP(*leader)
	if err != nil {
		log.Fatal(err)
	}
	p := &proxy{leaderIPs: make(map[string]bool), connections: make(map[net.Conn]bool), target: *target}
	for _, ip := range ips {
		p.leaderIPs[ip.String()] = true
	}
	listener, err := net.Listen("tcp", ":20160")
	if err != nil {
		log.Fatal(err)
	}
	control := &http.Server{Addr: ":8080", Handler: http.HandlerFunc(p.control), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := control.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Print(err)
			stop()
		}
	}()
	go func() { <-ctx.Done(); _ = listener.Close() }()
	var handlers sync.WaitGroup
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() == nil {
				log.Print(err)
			}
			break
		}
		handlers.Go(func() { p.serve(ctx, conn) })
	}
	handlers.Wait()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = control.Shutdown(shutdown)
}
