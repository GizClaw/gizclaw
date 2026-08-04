package gizedge

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/pion/webrtc/v4"
)

const (
	upstreamDialTimeout         = 30 * time.Second
	upstreamRelayAttemptTimeout = 5 * time.Second
	upstreamRelayBackoffInitial = 5 * time.Second
	upstreamRelayBackoffMaximum = 2 * time.Minute
	upstreamRelayJitterParts    = 2000
)

var errUpstreamRelaysUnavailable = errors.New("edge: upstream relays unavailable")

type upstreamWebRTCDialFunc func(
	context.Context,
	*giznet.KeyPair,
	giznet.PublicKey,
	gizwebrtc.DialConfig,
) (giznet.Listener, giznet.Conn, error)

type upstreamRelaySelector struct {
	mu             sync.Mutex
	members        []upstreamRelayMember
	next           int
	seed           [sha256.Size]byte
	now            func() time.Time
	dial           upstreamWebRTCDialFunc
	attemptTimeout time.Duration
}

type upstreamRelayMember struct {
	server           gizwebrtc.ICEServer
	endpoint         string
	failures         uint32
	unavailableUntil time.Time
	generation       uint64
}

type upstreamRelayAttempt struct {
	selector   *upstreamRelaySelector
	member     int
	generation uint64
	once       sync.Once
}

type upstreamRelaysUnavailableError struct {
	attempts   int
	retryAfter time.Duration
}

func (e *upstreamRelaysUnavailableError) Error() string {
	if e.retryAfter > 0 {
		return fmt.Sprintf("%s after %d attempts; retry after %s", errUpstreamRelaysUnavailable, e.attempts, e.retryAfter)
	}
	return fmt.Sprintf("%s after %d attempts", errUpstreamRelaysUnavailable, e.attempts)
}

func (e *upstreamRelaysUnavailableError) Unwrap() error {
	return errUpstreamRelaysUnavailable
}

func newUpstreamRelaySelector(cfg Config) (*upstreamRelaySelector, error) {
	if !cfg.Upstream.relayEnabled() {
		return nil, nil
	}
	selector := &upstreamRelaySelector{
		members:        make([]upstreamRelayMember, 0, len(cfg.Upstream.ICEServers)),
		now:            time.Now,
		attemptTimeout: upstreamRelayAttemptTimeout,
		dial: func(ctx context.Context, key *giznet.KeyPair, serverKey giznet.PublicKey, cfg gizwebrtc.DialConfig) (giznet.Listener, giznet.Conn, error) {
			return gizwebrtc.Dial(ctx, key, serverKey, cfg)
		},
	}
	selector.seed = sha256.Sum256([]byte(cfg.KeyPair.Public.String() + "\x00" + cfg.Upstream.PublicKey.String()))
	for _, server := range cfg.Upstream.ICEServers {
		endpoint, err := upstreamRelayEndpoint(server)
		if err != nil {
			return nil, err
		}
		selector.members = append(selector.members, upstreamRelayMember{
			server:   cloneICEServer(server),
			endpoint: endpoint,
		})
	}
	selector.next = int(binary.BigEndian.Uint64(selector.seed[:8]) % uint64(len(selector.members)))
	return selector, nil
}

func cloneICEServer(server gizwebrtc.ICEServer) gizwebrtc.ICEServer {
	server.URLs = append([]string(nil), server.URLs...)
	return server
}

func (s *upstreamRelaySelector) dialUpstream(
	ctx context.Context,
	cfg Config,
	upstreamURL *url.URL,
) (giznet.Conn, giznet.Listener, *upstreamRelayAttempt, *gizwebrtc.ICECandidatePairObservation, error) {
	attempted := make(map[int]struct{}, len(s.members))
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, nil, err
		}
		member, generation, retryAfter, ok := s.selectMember(attempted)
		if !ok {
			return nil, nil, nil, nil, &upstreamRelaysUnavailableError{
				attempts:   len(attempted),
				retryAfter: retryAfter,
			}
		}
		attempted[member] = struct{}{}
		server := cloneICEServer(s.members[member].server)
		attemptCtx, cancel := context.WithTimeout(ctx, s.attemptTimeout)
		var timing gizwebrtc.DialTiming
		listener, conn, err := s.dial(attemptCtx, cfg.KeyPair, cfg.Upstream.PublicKey, gizwebrtc.DialConfig{
			SignalingURL:          upstreamSignalingURL(upstreamURL),
			ICEServers:            []gizwebrtc.ICEServer{server},
			ICETransportPolicy:    webrtc.ICETransportPolicyRelay,
			SecurityPolicy:        edgeSecurityPolicy{},
			SCTPReceiveBufferSize: gizwebrtc.GatewaySCTPReceiveBufferSize,
			OnTiming: func(observation gizwebrtc.DialTiming) {
				timing = observation
			},
		})
		cancel()
		if err == nil {
			attempt := s.markSuccess(member)
			return conn, listener, attempt, timing.SelectedCandidatePair, nil
		}
		if ctx.Err() != nil {
			return nil, nil, nil, nil, ctx.Err()
		}
		s.markDialFailure(member, generation)
	}
}

func (s *upstreamRelaySelector) selectMember(attempted map[int]struct{}) (int, uint64, time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	var earliest time.Time
	for offset := range len(s.members) {
		index := (s.next + offset) % len(s.members)
		member := &s.members[index]
		if now.Before(member.unavailableUntil) {
			if earliest.IsZero() || member.unavailableUntil.Before(earliest) {
				earliest = member.unavailableUntil
			}
		}
		if _, ok := attempted[index]; ok {
			continue
		}
		if now.Before(member.unavailableUntil) {
			continue
		}
		s.next = (index + 1) % len(s.members)
		return index, member.generation, 0, true
	}
	if earliest.IsZero() {
		return 0, 0, 0, false
	}
	return 0, 0, earliest.Sub(now), false
}

func (s *upstreamRelaySelector) markSuccess(member int) *upstreamRelayAttempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := &s.members[member]
	state.failures = 0
	state.unavailableUntil = time.Time{}
	state.generation++
	return &upstreamRelayAttempt{
		selector:   s,
		member:     member,
		generation: state.generation,
	}
}

func (s *upstreamRelaySelector) markDialFailure(member int, generation uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markFailureLocked(member, generation)
}

func (s *upstreamRelaySelector) markFailureLocked(member int, generation uint64) {
	state := &s.members[member]
	if state.generation != generation {
		return
	}
	if state.failures != ^uint32(0) {
		state.failures++
	}
	state.unavailableUntil = s.now().Add(s.backoff(member, state.failures))
}

func (s *upstreamRelaySelector) backoff(member int, failures uint32) time.Duration {
	if failures == 0 {
		return 0
	}
	delay := upstreamRelayBackoffInitial
	for failure := uint32(1); failure < failures; failure++ {
		if delay >= upstreamRelayBackoffMaximum/2 {
			delay = upstreamRelayBackoffMaximum
			break
		}
		delay *= 2
	}
	if delay >= upstreamRelayBackoffMaximum {
		return upstreamRelayBackoffMaximum
	}
	hash := sha256.New()
	_, _ = hash.Write(s.seed[:])
	_, _ = hash.Write([]byte(s.members[member].endpoint))
	_, _ = hash.Write([]byte(strconv.FormatUint(uint64(failures), 10)))
	digest := hash.Sum(nil)
	jitterParts := binary.BigEndian.Uint64(digest[:8]) % upstreamRelayJitterParts
	jitter := time.Duration(uint64(delay) * jitterParts / 10000)
	return min(delay+jitter, upstreamRelayBackoffMaximum)
}

func (a *upstreamRelayAttempt) reportFailure() {
	if a == nil || a.selector == nil {
		return
	}
	a.once.Do(func() {
		a.selector.mu.Lock()
		defer a.selector.mu.Unlock()
		a.selector.markFailureLocked(a.member, a.generation)
	})
}
