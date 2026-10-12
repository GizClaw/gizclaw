package sfu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
)

func TestLiveKitConnectHonorsCancellationDuringSignaling(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rtc" {
			entered <- struct{}{}
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}
		http.Error(w, "test signaling unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		connector := livekitConnector{apiKey: "local-test", apiSecret: "local-test-secret-with-at-least-32-characters"}
		client, err := connector.connect(ctx, connectParams{URL: server.URL, Room: "cancel-test", Identity: "cancel-peer"}, &session{disconnects: make(chan disconnectReason, 8)})
		if client != nil {
			client.Disconnect()
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("join did not reach the signaling server")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("connect after cancellation = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled join continued waiting for signaling")
	}
}

type contextRoomConnector func(context.Context, connectParams, roomEvents) (roomClient, error)

func (f contextRoomConnector) connect(ctx context.Context, params connectParams, events roomEvents) (roomClient, error) {
	return f(ctx, params, events)
}

func TestSFURetirementCancelsInitialJoin(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(gizlog.WithPeerPublicKey(t.Context(), testPeer))
	defer cancel()
	entered := make(chan struct{})
	var attempts atomic.Int32
	h.agent.connector = contextRoomConnector(func(ctx context.Context, params connectParams, events roomEvents) (roomClient, error) {
		if attempts.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return &fakeClient{params: params, events: events}, nil
	})
	first, second := make(chan error, 1), make(chan error, 1)
	firstInput, secondInput := newOutputStream(), newOutputStream()
	defer firstInput.Close()
	defer secondInput.Close()
	go func() {
		_, err := h.agent.Transform(ctx, firstInput)
		first <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first attachment did not enter the connector")
	}
	go func() {
		_, err := h.agent.Transform(ctx, secondInput)
		second <- err
	}()
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("replacement attachment: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("retirement did not cancel the previous attachment's initial join")
	}
	select {
	case err := <-first:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("retired initial join = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("retired initial join did not return")
	}
}
