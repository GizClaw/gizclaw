package gizedge

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestUpstreamRequestsCancelWhileAnotherDialIsPending(t *testing.T) {
	for _, operation := range []string{"direct HTTP", "ordered HTTP", "API key route", "Peer route"} {
		t.Run(operation, func(t *testing.T) {
			lifetime, stop := context.WithCancel(t.Context())
			defer stop()
			pending := make(chan struct{})
			entry := &upstreamTransport{ctx: lifetime, connecting: pending}
			ordered := &orderedUpstreamTransport{entries: []*upstreamTransport{entry}}
			requestContext, cancel := context.WithCancel(t.Context())
			cancel()
			done := make(chan error, 1)
			go func() {
				if operation == "API key route" {
					_, err := ordered.resolveAPIKeyAssignment(requestContext, "fixture-key")
					done <- err
					return
				}
				if operation == "Peer route" {
					_, err := ordered.resolvePeerAssignment(requestContext, giznet.PublicKey{})
					done <- err
					return
				}
				request, err := http.NewRequestWithContext(requestContext, http.MethodGet, "http://gizclaw/server-info", nil)
				if err != nil {
					done <- err
					return
				}
				var transport http.RoundTripper = entry
				if operation == "ordered HTTP" {
					transport = ordered
				}
				response, err := transport.RoundTrip(request)
				if response != nil {
					_ = response.Body.Close()
				}
				done <- err
			}()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("request error = %v, want context.Canceled", err)
				}
			case <-time.After(500 * time.Millisecond):
				stop()
				<-done
				t.Fatal("canceled request remained blocked on another upstream dial")
			}
			if lifetime.Err() != nil || entry.connecting != pending {
				t.Fatal("request cancellation changed the shared dial or transport lifetime")
			}
		})
	}
}
