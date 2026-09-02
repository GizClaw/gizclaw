package gizedge

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/pion/webrtc/v4"
)

func TestPrepareWorkspaceConfigLoadsUpstreamRelayPool(t *testing.T) {
	edgeKey := testKeyPair(t, 0x23)
	upstreamKey := testKeyPair(t, 0x24)
	dir := t.TempDir()
	writeConfig(t, dir, `
identity:
  private-key: `+edgeKey.Private.String()+`
webrtc:
  listen: 0.0.0.0:9821
  endpoint: 0.0.0.0:9821
upstreams:
  - endpoint: server-a.example.com:9820
    public-key: `+upstreamKey.Public.String()+`
    ice-transport-policy: relay
    ice-servers:
      - urls: [turn:192.0.2.10:3478?transport=udp]
        username: relay-a
        credential: shared-secret
        credential-mode: turn-rest
      - urls: ['turn:[2001:db8::20]:3478?transport=udp']
        username: relay-b
        credential: static-password
        credential-mode: static
http:
  listeners:
    - listen: 0.0.0.0:9821
`)

	cfg, err := PrepareWorkspaceConfig(dir)
	if err != nil {
		t.Fatalf("PrepareWorkspaceConfig error = %v", err)
	}
	if len(cfg.Upstreams) != 1 || !cfg.Upstreams[0].relayEnabled() {
		t.Fatal("upstream relay mode is disabled")
	}
	if len(cfg.Upstreams[0].ICEServers) != 2 {
		t.Fatalf("upstream ICE servers = %d, want 2", len(cfg.Upstreams[0].ICEServers))
	}
	if got := cfg.Upstreams[0].ICEServers[1].URLs[0]; got != "turn:[2001:db8::20]:3478?transport=udp" {
		t.Fatalf("second relay URL = %q", got)
	}
}

func TestUpstreamRelayConfigValidation(t *testing.T) {
	valid := func() UpstreamConfig {
		return UpstreamConfig{
			ICETransportPolicy: "relay",
			ICEServers: []gizwebrtc.ICEServer{
				{
					URLs:           []string{"turn:192.0.2.10:3478?transport=udp"},
					Username:       "relay-a",
					Credential:     "shared-secret-a",
					CredentialMode: gizwebrtc.ICECredentialModeTURNREST,
				},
				{
					URLs:           []string{"turn:[2001:db8::20]:3478?transport=udp"},
					Username:       "relay-b",
					Credential:     "static-secret-b",
					CredentialMode: gizwebrtc.ICECredentialModeStatic,
				},
			},
		}
	}
	if err := (UpstreamConfig{}).validate(); err != nil {
		t.Fatalf("direct upstream validation error = %v", err)
	}
	if err := valid().validate(); err != nil {
		t.Fatalf("valid relay upstream validation error = %v", err)
	}
	turnRESTWithoutKeyID := valid()
	turnRESTWithoutKeyID.ICEServers[0].Username = ""
	if err := turnRESTWithoutKeyID.validate(); err != nil {
		t.Fatalf("TURN REST relay without key ID validation error = %v", err)
	}
	defaultStaticMode := valid()
	defaultStaticMode.ICEServers[1].CredentialMode = ""
	if err := defaultStaticMode.validate(); err != nil {
		t.Fatalf("default static relay validation error = %v", err)
	}

	tests := []struct {
		name   string
		change func(*UpstreamConfig)
		want   string
	}{
		{name: "policy without members", change: func(cfg *UpstreamConfig) { cfg.ICEServers = nil }, want: "at least two"},
		{name: "members without policy", change: func(cfg *UpstreamConfig) { cfg.ICETransportPolicy = "" }, want: "must be relay"},
		{name: "unsupported policy", change: func(cfg *UpstreamConfig) { cfg.ICETransportPolicy = "all" }, want: "must be relay"},
		{name: "policy with whitespace", change: func(cfg *UpstreamConfig) { cfg.ICETransportPolicy = " relay " }, want: "must be relay"},
		{name: "one member", change: func(cfg *UpstreamConfig) { cfg.ICEServers = cfg.ICEServers[:1] }, want: "at least two"},
		{name: "multiple URLs", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs = append(cfg.ICEServers[0].URLs, "turn:192.0.2.11:3478?transport=udp")
		}, want: "exactly one"},
		{name: "secure TURN", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs[0] = "turns:192.0.2.10:5349?transport=udp"
		}, want: "turn URI"},
		{name: "uppercase scheme", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs[0] = "TURN:192.0.2.10:3478?transport=udp"
		}, want: "turn URI"},
		{name: "URL with whitespace", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs[0] = " turn:192.0.2.10:3478?transport=udp"
		}, want: "surrounding whitespace"},
		{name: "userinfo", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs[0] = "turn:user@192.0.2.10:3478?transport=udp"
		}, want: "literal IP"},
		{name: "hostname", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs[0] = "turn:relay.example.com:3478?transport=udp"
		}, want: "literal IP"},
		{name: "missing port", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs[0] = "turn:192.0.2.10?transport=udp"
		}, want: "explicit port"},
		{name: "TCP transport", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs[0] = "turn:192.0.2.10:3478?transport=tcp"
		}, want: "transport=udp"},
		{name: "missing transport", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs[0] = "turn:192.0.2.10:3478"
		}, want: "transport=udp"},
		{name: "zero port", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs[0] = "turn:192.0.2.10:0?transport=udp"
		}, want: "between 1 and 65535"},
		{name: "extra query", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs[0] = "turn:192.0.2.10:3478?transport=udp&region=hk"
		}, want: "transport=udp"},
		{name: "fragment", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs[0] = "turn:192.0.2.10:3478?transport=udp#candidate"
		}, want: "fragment"},
		{name: "duplicate normalized endpoint", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[1].URLs[0] = "turn:192.0.2.10:3478?transport=udp"
		}, want: "duplicates member 0"},
		{name: "duplicate normalized IPv6 endpoint", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].URLs[0] = "turn:[2001:db8::20]:3478?transport=udp"
			cfg.ICEServers[1].URLs[0] = "turn:[2001:0db8:0:0:0:0:0:20]:3478?transport=udp"
		}, want: "duplicates member 0"},
		{name: "static username", change: func(cfg *UpstreamConfig) { cfg.ICEServers[1].Username = "" }, want: "requires username"},
		{name: "static credential", change: func(cfg *UpstreamConfig) { cfg.ICEServers[1].Credential = "" }, want: "requires username"},
		{name: "TURN REST credential", change: func(cfg *UpstreamConfig) { cfg.ICEServers[0].Credential = "" }, want: "requires credential"},
		{name: "unsupported credential mode", change: func(cfg *UpstreamConfig) {
			cfg.ICEServers[0].CredentialMode = "opaque-mode"
		}, want: "unsupported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid()
			tt.change(&cfg)
			err := cfg.validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validation error = %v, want %q", err, tt.want)
			}
			for _, secret := range []string{"relay-a", "shared-secret-a", "relay-b", "static-secret-b"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("validation error contains sensitive value %q: %v", secret, err)
				}
			}
		})
	}
}

func TestUpstreamRelaySelectorDialsOneMemberInStableOrder(t *testing.T) {
	cfg := testUpstreamRelayConfig(t)
	selector, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		t.Fatalf("newUpstreamRelaySelector error = %v", err)
	}
	initial := selector.next
	upstreamURL, err := cfg.selectedUpstreamURL()
	if err != nil {
		t.Fatalf("UpstreamURL error = %v", err)
	}
	var gotURLs []string
	selector.dial = func(
		_ context.Context,
		key *giznet.KeyPair,
		serverKey giznet.PublicKey,
		dialCfg gizwebrtc.DialConfig,
	) (giznet.Listener, giznet.Conn, error) {
		if key != cfg.KeyPair {
			t.Fatal("dial key pair does not preserve Edge identity")
		}
		if !serverKey.Equal(cfg.selectedUpstream.PublicKey) {
			t.Fatalf("dial server key = %v, want %v", serverKey, cfg.selectedUpstream.PublicKey)
		}
		if dialCfg.SignalingURL != "https://server.example.com:9820/webrtc/v1/offer" {
			t.Fatalf("signaling URL = %q", dialCfg.SignalingURL)
		}
		if dialCfg.ICETransportPolicy != webrtc.ICETransportPolicyRelay {
			t.Fatalf("ICE transport policy = %v, want relay", dialCfg.ICETransportPolicy)
		}
		if dialCfg.SCTPReceiveBufferSize != gizwebrtc.GatewaySCTPReceiveBufferSize {
			t.Fatalf("SCTP receive buffer = %d, want gateway profile", dialCfg.SCTPReceiveBufferSize)
		}
		if len(dialCfg.ICEServers) != 1 || len(dialCfg.ICEServers[0].URLs) != 1 {
			t.Fatalf("dial ICE servers = %+v, want exactly one member", dialCfg.ICEServers)
		}
		gotURLs = append(gotURLs, dialCfg.ICEServers[0].URLs[0])
		return nil, &failingGiznetConn{state: giznet.PeerStateEstablished}, nil
	}

	for range 2 {
		conn, _, attempt, _, err := selector.dialUpstream(t.Context(), cfg, upstreamURL)
		if err != nil {
			t.Fatalf("dialUpstream error = %v", err)
		}
		if conn == nil || attempt == nil {
			t.Fatalf("dial result conn=%v attempt=%v", conn, attempt)
		}
	}
	want := []string{
		cfg.selectedUpstream.ICEServers[initial].URLs[0],
		cfg.selectedUpstream.ICEServers[(initial+1)%len(cfg.selectedUpstream.ICEServers)].URLs[0],
	}
	if !slices.Equal(gotURLs, want) {
		t.Fatalf("relay order = %v, want %v", gotURLs, want)
	}

	second, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		t.Fatalf("second newUpstreamRelaySelector error = %v", err)
	}
	if second.next != initial {
		t.Fatalf("stable initial index = %d, want %d", second.next, initial)
	}
}

func TestUpstreamRelaySelectorReturnsICEObservation(t *testing.T) {
	cfg := testUpstreamRelayConfig(t)
	selector, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		t.Fatalf("newUpstreamRelaySelector error = %v", err)
	}
	upstreamURL, err := cfg.selectedUpstreamURL()
	if err != nil {
		t.Fatalf("UpstreamURL error = %v", err)
	}
	want := &gizwebrtc.ICECandidatePairObservation{
		Local: gizwebrtc.ICECandidateObservation{
			Type: "relay", Protocol: "udp", AddressFamily: "ipv4", Component: 1,
		},
		Remote: gizwebrtc.ICECandidateObservation{
			Type: "host", Protocol: "udp", AddressFamily: "ipv4", Component: 1,
		},
		State: "succeeded", Nominated: true,
	}
	selector.dial = func(
		_ context.Context,
		_ *giznet.KeyPair,
		_ giznet.PublicKey,
		dialCfg gizwebrtc.DialConfig,
	) (giznet.Listener, giznet.Conn, error) {
		dialCfg.OnTiming(gizwebrtc.DialTiming{SelectedCandidatePair: want})
		return nil, &failingGiznetConn{state: giznet.PeerStateEstablished}, nil
	}

	_, _, attempt, got, err := selector.dialUpstream(t.Context(), cfg, upstreamURL)
	if err != nil {
		t.Fatalf("dialUpstream error = %v", err)
	}
	if attempt == nil {
		t.Fatal("relay attempt is nil")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ICE observation = %+v, want %+v", got, want)
	}
}

func TestUpstreamRelaySelectorRetriesOtherMembersAndSanitizesFailure(t *testing.T) {
	cfg := testUpstreamRelayConfig(t)
	selector, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		t.Fatalf("newUpstreamRelaySelector error = %v", err)
	}
	now := time.Unix(1000, 0)
	selector.now = func() time.Time { return now }
	upstreamURL, err := cfg.selectedUpstreamURL()
	if err != nil {
		t.Fatalf("UpstreamURL error = %v", err)
	}
	var attempted []string
	selector.dial = func(
		_ context.Context,
		_ *giznet.KeyPair,
		_ giznet.PublicKey,
		dialCfg gizwebrtc.DialConfig,
	) (giznet.Listener, giznet.Conn, error) {
		attempted = append(attempted, dialCfg.ICEServers[0].URLs[0])
		return nil, nil, errors.New("credential=must-not-leak sdp=must-not-leak")
	}

	_, _, _, _, err = selector.dialUpstream(t.Context(), cfg, upstreamURL)
	if !errors.Is(err, errUpstreamRelaysUnavailable) {
		t.Fatalf("dialUpstream error = %v, want relay unavailable", err)
	}
	if len(attempted) != len(cfg.selectedUpstream.ICEServers) {
		t.Fatalf("dial attempts = %d, want %d", len(attempted), len(cfg.selectedUpstream.ICEServers))
	}
	if attempted[0] == attempted[1] {
		t.Fatalf("one operation retried the same relay: %v", attempted)
	}
	if strings.Contains(err.Error(), "must-not-leak") {
		t.Fatalf("sanitized dial error contains underlying details: %v", err)
	}
	var unavailable *upstreamRelaysUnavailableError
	if !errors.As(err, &unavailable) || unavailable.retryAfter < upstreamRelayBackoffInitial ||
		unavailable.retryAfter > upstreamRelayBackoffInitial+upstreamRelayBackoffInitial/5 {
		t.Fatalf("retry delay = %v, want initial jittered backoff", unavailable.retryAfter)
	}
}

func TestUpstreamRelaySelectorHonorsCancellationWithoutBackoff(t *testing.T) {
	cfg := testUpstreamRelayConfig(t)
	selector, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		t.Fatalf("newUpstreamRelaySelector error = %v", err)
	}
	upstreamURL, err := cfg.selectedUpstreamURL()
	if err != nil {
		t.Fatalf("UpstreamURL error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	selector.dial = func(
		_ context.Context,
		_ *giznet.KeyPair,
		_ giznet.PublicKey,
		_ gizwebrtc.DialConfig,
	) (giznet.Listener, giznet.Conn, error) {
		cancel()
		return nil, nil, context.Canceled
	}

	_, _, _, _, err = selector.dialUpstream(ctx, cfg, upstreamURL)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("dialUpstream error = %v, want canceled", err)
	}
	for i, member := range selector.members {
		if member.failures != 0 || !member.unavailableUntil.IsZero() {
			t.Fatalf("member %d penalized after cancellation: %+v", i, member)
		}
	}
}

func TestUpstreamRelaySelectorAttemptTimeoutUsesBackoff(t *testing.T) {
	cfg := testUpstreamRelayConfig(t)
	selector, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		t.Fatalf("newUpstreamRelaySelector error = %v", err)
	}
	selector.attemptTimeout = time.Millisecond
	upstreamURL, err := cfg.selectedUpstreamURL()
	if err != nil {
		t.Fatalf("UpstreamURL error = %v", err)
	}
	selector.dial = func(
		ctx context.Context,
		_ *giznet.KeyPair,
		_ giznet.PublicKey,
		_ gizwebrtc.DialConfig,
	) (giznet.Listener, giznet.Conn, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}

	_, _, _, _, err = selector.dialUpstream(t.Context(), cfg, upstreamURL)
	if !errors.Is(err, errUpstreamRelaysUnavailable) {
		t.Fatalf("dialUpstream error = %v, want relay unavailable", err)
	}
	for i, member := range selector.members {
		if member.failures != 1 || member.unavailableUntil.IsZero() {
			t.Fatalf("member %d timeout state = %+v, want one failure", i, member)
		}
	}
}

func TestUpstreamRelayAttemptIgnoresDuplicateAndStaleFailure(t *testing.T) {
	cfg := testUpstreamRelayConfig(t)
	selector, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		t.Fatalf("newUpstreamRelaySelector error = %v", err)
	}
	now := time.Unix(2000, 0)
	selector.now = func() time.Time { return now }
	stale := selector.markSuccess(0)
	current := selector.markSuccess(0)

	stale.reportFailure()
	if got := selector.members[0].failures; got != 0 {
		t.Fatalf("stale failure count = %d, want 0", got)
	}
	current.reportFailure()
	current.reportFailure()
	if got := selector.members[0].failures; got != 1 {
		t.Fatalf("duplicate failure count = %d, want 1", got)
	}
	if selector.members[0].unavailableUntil.Before(now.Add(upstreamRelayBackoffInitial)) {
		t.Fatalf("backoff deadline = %v, want at least %v", selector.members[0].unavailableUntil, now.Add(upstreamRelayBackoffInitial))
	}

	recovered := selector.markSuccess(0)
	if selector.members[0].failures != 0 || !selector.members[0].unavailableUntil.IsZero() {
		t.Fatalf("successful recovery left failure state: %+v", selector.members[0])
	}
	current.reportFailure()
	if selector.members[0].failures != 0 {
		t.Fatal("older connection failure poisoned a newer success")
	}
	recovered.reportFailure()
	if selector.members[0].failures != 1 {
		t.Fatal("current connection failure was not recorded")
	}
}

func TestUpstreamConsumersShareOnlyProcessSelectorHealth(t *testing.T) {
	cfg := testUpstreamRelayConfig(t)
	selector, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		t.Fatalf("newUpstreamRelaySelector error = %v", err)
	}
	selector.next = 0
	transport := &upstreamTransport{
		ctx:          context.Background(),
		conn:         &failingGiznetConn{state: giznet.PeerStateEstablished},
		relay:        selector,
		relayAttempt: selector.markSuccess(0),
		connEpoch:    1,
	}
	pool := newGatewayPool(context.Background(), cfg, nil, selector)
	if transport.relay != pool.relay {
		t.Fatal("HTTP transport and gateway pool do not share one selector")
	}
	transport.resetConn(1, true)
	member, _, _, ok := pool.relay.selectMember(nil)
	if !ok || member != 1 {
		t.Fatalf("gateway selected member=%d ok=%t, want healthy member 1", member, ok)
	}

	otherProcess, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		t.Fatalf("second newUpstreamRelaySelector error = %v", err)
	}
	if otherProcess.members[0].failures != 0 || !otherProcess.members[0].unavailableUntil.IsZero() {
		t.Fatalf("separate selector inherited process health: %+v", otherProcess.members[0])
	}
}

func TestUpstreamRelayBackoffIsDeterministicAndCapped(t *testing.T) {
	cfg := testUpstreamRelayConfig(t)
	first, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		t.Fatalf("newUpstreamRelaySelector error = %v", err)
	}
	second, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		t.Fatalf("second newUpstreamRelaySelector error = %v", err)
	}
	for failures := uint32(1); failures <= 10; failures++ {
		got := first.backoff(0, failures)
		if want := second.backoff(0, failures); got != want {
			t.Fatalf("backoff(%d) = %v, want deterministic %v", failures, got, want)
		}
		if got < upstreamRelayBackoffInitial || got > upstreamRelayBackoffMaximum {
			t.Fatalf("backoff(%d) = %v, outside bounds", failures, got)
		}
	}
	if got := first.backoff(0, 10); got != upstreamRelayBackoffMaximum {
		t.Fatalf("capped backoff = %v, want %v", got, upstreamRelayBackoffMaximum)
	}
}

func TestUpstreamRelaySelectorSupportsConcurrentConsumers(t *testing.T) {
	cfg := testUpstreamRelayConfig(t)
	selector, err := newUpstreamRelaySelector(cfg)
	if err != nil {
		t.Fatalf("newUpstreamRelaySelector error = %v", err)
	}
	upstreamURL, err := cfg.selectedUpstreamURL()
	if err != nil {
		t.Fatalf("UpstreamURL error = %v", err)
	}
	var dialCount atomic.Int64
	selector.dial = func(
		_ context.Context,
		_ *giznet.KeyPair,
		_ giznet.PublicKey,
		_ gizwebrtc.DialConfig,
	) (giznet.Listener, giznet.Conn, error) {
		dialCount.Add(1)
		return nil, &failingGiznetConn{state: giznet.PeerStateEstablished}, nil
	}

	const consumers = 64
	errCh := make(chan error, consumers)
	attemptCh := make(chan *upstreamRelayAttempt, consumers)
	var wg sync.WaitGroup
	for range consumers {
		wg.Go(func() {
			_, _, attempt, _, err := selector.dialUpstream(t.Context(), cfg, upstreamURL)
			if err != nil {
				errCh <- err
				return
			}
			attemptCh <- attempt
		})
	}
	wg.Wait()
	close(errCh)
	close(attemptCh)
	for err := range errCh {
		t.Errorf("concurrent dial error = %v", err)
	}
	var failureWG sync.WaitGroup
	index := 0
	for attempt := range attemptCh {
		if index%2 == 0 {
			failureWG.Go(func() {
				attempt.reportFailure()
			})
		}
		index++
	}
	failureWG.Wait()
	if got := dialCount.Load(); got != consumers {
		t.Fatalf("dial count = %d, want %d", got, consumers)
	}
}

func testUpstreamRelayConfig(t *testing.T) Config {
	t.Helper()
	edgeKey := testKeyPair(t, 0x81)
	serverKey := testKeyPair(t, 0x82)
	return Config{
		KeyPair: edgeKey,
		selectedUpstream: UpstreamConfig{
			Endpoint:           "https://server.example.com:9820",
			PublicKey:          serverKey.Public,
			ICETransportPolicy: "relay",
			ICEServers: []gizwebrtc.ICEServer{
				{
					URLs:           []string{"turn:192.0.2.10:3478?transport=udp"},
					Username:       "relay-a",
					Credential:     "shared-secret-a",
					CredentialMode: gizwebrtc.ICECredentialModeTURNREST,
				},
				{
					URLs:           []string{"turn:192.0.2.11:3478?transport=udp"},
					Username:       "relay-b",
					Credential:     "shared-secret-b",
					CredentialMode: gizwebrtc.ICECredentialModeTURNREST,
				},
			},
		},
	}
}
