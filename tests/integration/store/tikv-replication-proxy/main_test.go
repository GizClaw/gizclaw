package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type peerConn struct {
	net.Conn
	remote net.Addr
}

func (c peerConn) RemoteAddr() net.Addr { return c.remote }

func TestFreezeStopsRaftAndPreservesQueries(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var echoes, handlers sync.WaitGroup
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			echoes.Go(func() { defer conn.Close(); _, _ = io.Copy(conn, conn) })
		}
	}()
	p := &proxy{leaderIPs: map[string]bool{"192.0.2.1": true}, connections: make(map[net.Conn]bool), target: listener.Addr().String()}
	t.Cleanup(func() { cancel(); _ = listener.Close(); handlers.Wait(); <-acceptDone; echoes.Wait() })
	connect := func(ip string) net.Conn {
		source, client := net.Pipe()
		handlers.Go(func() { p.serve(ctx, peerConn{Conn: source, remote: &net.TCPAddr{IP: net.ParseIP(ip), Port: 12345}}) })
		_ = client.SetDeadline(time.Now().Add(3 * time.Second))
		t.Cleanup(func() { _ = client.Close() })
		return client
	}
	echo := func(conn net.Conn) {
		t.Helper()
		if _, err := conn.Write([]byte("query")); err != nil {
			t.Fatal(err)
		}
		var data [5]byte
		if _, err := io.ReadFull(conn, data[:]); err != nil || string(data[:]) != "query" {
			t.Fatalf("echo=%q %v", data, err)
		}
	}
	raft, query := connect("192.0.2.1"), connect("192.0.2.2")
	echo(raft)
	echo(query)
	response := httptest.NewRecorder()
	p.control(response, httptest.NewRequest(http.MethodPost, "/freeze", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("freeze status=%d", response.Code)
	}
	var data [1]byte
	if _, err := raft.Read(data[:]); err == nil {
		t.Fatal("existing Raft stream remained open")
	}
	echo(query)
	newRaft := connect("192.0.2.1")
	if _, err := newRaft.Read(data[:]); err == nil {
		t.Fatal("new Raft stream was admitted while frozen")
	}
	// Cancellation must release active query handlers as well as the listener.
	cancel()
	finished := make(chan struct{})
	go func() { handlers.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("proxy handlers did not stop")
	}
}
