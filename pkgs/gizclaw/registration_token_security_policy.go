package gizclaw

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"google.golang.org/protobuf/proto"
)

// RegistrationTokenCredentialType identifies the built-in GizClaw admission credential.
const RegistrationTokenCredentialType = "registration_token"

const (
	admissionFailureLimit  = 64
	admissionInFlightLimit = 8
	admissionCacheLimit    = 1024
	admissionFailureWindow = time.Minute
	admissionNegativeTTL   = 30 * time.Second
	admissionLookupTimeout = 2 * time.Second
)

// RegistrationTokenSecurityPolicy admits existing available, non-blocked Peers
// without a credential, or Peers presenting a valid registration token. It never
// registers a Peer or binds runtime resources. Its service policy grants nothing.
// Create one per Server so negative caching and failure limits span all keys.
type RegistrationTokenSecurityPolicy struct {
	server    *Server
	now       func() time.Time
	mu        sync.Mutex
	windowEnd time.Time
	failures  int
	inFlight  map[[sha256.Size]byte]struct{}
	negative  map[[sha256.Size]byte]time.Time
}

var _ giznet.SecurityPolicy = (*RegistrationTokenSecurityPolicy)(nil)

// NewRegistrationTokenSecurityPolicy creates a policy without I/O. The Server
// must initialize its services before admission requests; otherwise it denies.
func NewRegistrationTokenSecurityPolicy(server *Server) *RegistrationTokenSecurityPolicy {
	return &RegistrationTokenSecurityPolicy{
		server: server, now: time.Now,
		inFlight: make(map[[sha256.Size]byte]struct{}),
		negative: make(map[[sha256.Size]byte]time.Time),
	}
}

// AllowPeer checks admission using read-only services and the request context.
// It does not retain the credential or propagate it into the accepted connection.
func (p *RegistrationTokenSecurityPolicy) AllowPeer(ctx context.Context, admission giznet.PeerAdmission) bool {
	if p == nil || p.server == nil || ctx.Err() != nil || admission.PublicKey.IsZero() {
		return false
	}
	credential := admission.Credential
	// Reject unsupported credentials before any Peer or token store access.
	if credential != nil && (credential.Version != 1 || credential.Type != RegistrationTokenCredentialType ||
		len(credential.Value) > gizwebrtc.MaxCredentialBytes || proto.Size(credential) > gizwebrtc.MaxCredentialBytes) {
		return false
	}
	m := p.server.manager
	if m == nil || m.Peers == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, admissionLookupTimeout)
	defer cancel()
	if credential == nil {
		known, allowed := admissionPeerState(ctx, m.Peers, admission.PublicKey)
		return known && allowed && ctx.Err() == nil
	}
	// ResolveRegistration trims whitespace too. Hash its canonical input so
	// changing keys or padding cannot evade the negative cache. Include the exact
	// accepted type and a separator so the key identifies the (type, value) pair.
	token := strings.TrimSpace(credential.Value)
	digest := sha256.Sum256([]byte(credential.Type + "\x00" + token))
	if !p.beginLookup(digest) {
		return false
	}
	allowed, cacheFailure := false, false
	defer func() { p.finishLookup(digest, allowed, cacheFailure) }()
	_, available := admissionPeerState(ctx, m.Peers, admission.PublicKey)
	if !available || m.RuntimeProfiles == nil {
		return false
	}
	_, err := m.RuntimeProfiles.ResolveRegistration(ctx, token)
	cacheFailure = err != nil
	allowed = err == nil && ctx.Err() == nil
	return allowed
}

func admissionPeerState(ctx context.Context, peers *peer.Server, publicKey giznet.PublicKey) (known, allowed bool) {
	if err := peers.EnsureAvailable(ctx, publicKey); err != nil {
		return false, errors.Is(err, peer.ErrPeerNotFound)
	}
	record, err := peers.LoadPeer(ctx, publicKey)
	return err == nil, err == nil && record.Status != apitypes.PeerRegistrationStatusBlocked
}

// AllowService grants no service authorization; the host composes it separately.
func (*RegistrationTokenSecurityPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return false
}

func (p *RegistrationTokenSecurityPolicy) beginLookup(digest [sha256.Size]byte) bool {
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	if !now.Before(p.windowEnd) {
		p.windowEnd = now.Add(admissionFailureWindow)
		p.failures = 0
	}
	for key, expires := range p.negative {
		if !now.Before(expires) {
			delete(p.negative, key)
		}
	}
	if _, cached := p.negative[digest]; cached {
		return false
	}
	if _, pending := p.inFlight[digest]; pending {
		return false
	}
	// Reserve a possible failure before I/O, so concurrent random tokens cannot
	// overshoot either the process-wide failure budget or in-flight bound.
	if p.failures+len(p.inFlight) >= admissionFailureLimit || len(p.inFlight) >= admissionInFlightLimit {
		return false
	}
	p.inFlight[digest] = struct{}{}
	return true
}

func (p *RegistrationTokenSecurityPolicy) finishLookup(digest [sha256.Size]byte, allowed, cacheFailure bool) {
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.inFlight, digest)
	if allowed {
		return
	}
	p.failures++
	if !cacheFailure {
		return
	}
	if len(p.negative) >= admissionCacheLimit {
		// Eviction cannot remove the global failure budget. Avoid retaining
		// unbounded attacker-controlled state, including after window rollover.
		for key := range p.negative {
			delete(p.negative, key)
			break
		}
	}
	p.negative[digest] = now.Add(admissionNegativeTTL)
}
