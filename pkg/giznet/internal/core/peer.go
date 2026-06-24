package core

import "github.com/GizClaw/gizclaw-go/pkg/giznet/internal/noise"

// Peer represents a UDP peer handle plus optional snapshot/connection fields
// used by peer iterators.
type Peer struct {
	udp   *UDP
	state *peerState

	Info *PeerInfo
	Conn *Conn
}

func (p *Peer) PublicKey() noise.PublicKey {
	if p == nil || p.state == nil {
		return noise.PublicKey{}
	}
	return p.state.pk
}

// State returns the current peer state snapshot.
func (p *Peer) State() PeerState {
	if p == nil || p.state == nil {
		return PeerStateFailed
	}
	p.state.mu.RLock()
	defer p.state.mu.RUnlock()
	return p.state.state
}

// PeerInfo returns the current peer state snapshot.
func (p *Peer) PeerInfo() *PeerInfo {
	if p == nil || p.state == nil {
		return nil
	}
	p.state.mu.RLock()
	defer p.state.mu.RUnlock()
	return &PeerInfo{
		PublicKey: p.state.pk,
		Endpoint:  p.state.endpoint,
		State:     p.state.state,
		RxBytes:   p.state.rxBytes,
		TxBytes:   p.state.txBytes,
		LastSeen:  p.state.lastSeen,
	}
}

// IsClosed reports whether the owning UDP transport is closing or closed.
func (p *Peer) IsClosed() bool {
	return p == nil || p.udp == nil || p.udp.closed.Load() || p.udp.closing.Load()
}

// ServiceMux returns this peer's active service mux, creating one if the peer has
// an established Noise session but no active service generation yet.
func (p *Peer) ServiceMux() (*ServiceMux, error) {
	if p == nil || p.udp == nil || p.state == nil {
		return nil, ErrPeerNotFound
	}
	if p.IsClosed() {
		return nil, ErrClosed
	}
	return p.udp.ensureServiceMux(p.state)
}

func (p *Peer) Connect() error {
	if p == nil || p.udp == nil || p.state == nil {
		return ErrPeerNotFound
	}
	if p.IsClosed() {
		return ErrClosed
	}
	return p.udp.initiateHandshake(p.state)
}

// OpenServiceMux starts a new service mux generation for this peer and closes
// the previous generation, while preserving the peer and Noise session.
func (p *Peer) OpenServiceMux() (*ServiceMux, error) {
	if p == nil || p.udp == nil || p.state == nil {
		return nil, ErrPeerNotFound
	}
	if p.IsClosed() {
		return nil, ErrClosed
	}

	p.state.mu.Lock()
	if p.state.session == nil {
		p.state.mu.Unlock()
		return nil, ErrNoSession
	}
	oldMux := p.state.serviceMux
	smux := p.udp.createServiceMux(p.state)
	p.state.serviceMux = smux
	p.state.state = PeerStateEstablished
	p.state.mu.Unlock()

	if oldMux != nil {
		_ = oldMux.Close()
	}
	p.udp.emitPeerEvent(p.PublicKey(), PeerStateEstablished)
	return smux, nil
}

// CloseServiceMux closes this peer's active service mux. When ifMatch is
// non-nil, it is used only as a generation token: the active mux is closed only
// if it matches ifMatch.
func (p *Peer) CloseServiceMux(ifMatch *ServiceMux) {
	if p == nil || p.udp == nil || p.state == nil {
		return
	}

	p.state.mu.Lock()
	smux := p.state.serviceMux
	if ifMatch != nil && smux != ifMatch {
		p.state.mu.Unlock()
		return
	}
	wasOffline := p.state.state == PeerStateOffline
	p.state.state = PeerStateOffline
	p.state.serviceMux = nil
	p.state.mu.Unlock()

	if smux != nil {
		_ = smux.Close()
	}
	if !wasOffline {
		p.udp.emitPeerEvent(p.PublicKey(), PeerStateOffline)
	}
}
