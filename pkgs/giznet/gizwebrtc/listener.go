package gizwebrtc

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
)

const signalingReplayTTL = int64((5 * time.Minute) / time.Second)

type Listener struct {
	key            *giznet.KeyPair
	cfg            ListenConfig
	api            *webrtc.API
	gatewaySCTPAPI *webrtc.API

	closers []func() error

	acceptCh chan giznet.Conn
	closeCh  chan struct{}
	once     sync.Once
	closed   atomic.Bool

	replayMu        sync.Mutex
	replaySeen      map[string]int64
	replayNextPrune int64
}

func (l *Listener) Accept() (giznet.Conn, error) {
	if l == nil {
		return nil, giznet.ErrNilListener
	}
	select {
	case c := <-l.acceptCh:
		return c, nil
	case <-l.closeCh:
		return nil, giznet.ErrClosed
	}
}

func (l *Listener) Close() error {
	if l == nil {
		return giznet.ErrNilListener
	}
	var err error
	l.once.Do(func() {
		l.closed.Store(true)
		close(l.closeCh)
		for _, closeFn := range l.closers {
			err = errors.Join(err, closeFn())
		}
	})
	return err
}

func (l *Listener) enqueueConn(conn *Conn) {
	if l == nil || conn == nil {
		return
	}
	if l.cfg.PeerEventHandler != nil {
		l.cfg.PeerEventHandler.HandlePeerEvent(giznet.PeerEvent{PublicKey: conn.PublicKey(), State: giznet.PeerStateEstablished})
	}
	select {
	case l.acceptCh <- conn:
	case <-l.closeCh:
		_ = conn.Close()
	}
}

func (l *Listener) checkReplay(pk giznet.PublicKey, nonce string, now int64) error {
	key := pk.String() + ":" + nonce
	l.replayMu.Lock()
	defer l.replayMu.Unlock()
	if seen, ok := l.replaySeen[key]; ok {
		if now-seen <= signalingReplayTTL {
			return ErrSignalingReplay
		}
		delete(l.replaySeen, key)
	}
	if l.replayNextPrune == 0 || now >= l.replayNextPrune {
		for k, seen := range l.replaySeen {
			if now-seen > signalingReplayTTL {
				delete(l.replaySeen, k)
			}
		}
		l.replayNextPrune = now + signalingReplayTTL
	}
	if l.replaySeen == nil {
		l.replaySeen = make(map[string]int64)
	}
	l.replaySeen[key] = now
	return nil
}
