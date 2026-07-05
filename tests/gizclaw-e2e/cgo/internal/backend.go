//go:build gizclaw_e2e

package internal

/*
#cgo CFLAGS: -I. -I../../../../c/gizclaw/include -I../../../../c/gizclaw/generated
#include "bridge.h"
#include "sdk_driver.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/stampedopus"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
)

const signalingPath = "/webrtc/v1/offer"
const protocolStampedOpus = 0x10

type backend struct {
	mu       sync.Mutex
	key      *giznet.KeyPair
	serverPK giznet.PublicKey
	endpoint string

	pc       *webrtc.PeerConnection
	dcs      map[int]*dataChannelState
	cBackend unsafe.Pointer
}

type dataChannelState struct {
	dc        *webrtc.DataChannel
	openCh    chan struct{}
	closeCh   chan struct{}
	openOnce  sync.Once
	closeOnce sync.Once
}

func newBackend(identityDir string) (*backend, error) {
	cfg, err := os.ReadFile(filepath.Join(identityDir, "config.yaml"))
	if err != nil {
		return nil, err
	}
	var private giznet.Key
	if err := private.UnmarshalText([]byte(matchConfig(string(cfg), `private-key:\s*"?([^"\s]+)"?`))); err != nil {
		return nil, fmt.Errorf("identity.private-key: %w", err)
	}
	key, err := giznet.NewKeyPair(private)
	if err != nil {
		return nil, err
	}
	var serverPK giznet.PublicKey
	if err := serverPK.UnmarshalText([]byte(matchConfig(string(cfg), `public-key:\s*"?([^"\s]+)"?`))); err != nil {
		return nil, err
	}
	return &backend{
		key:      key,
		serverPK: serverPK,
		endpoint: matchConfig(string(cfg), `endpoint:\s*([^\s]+)`),
		dcs:      make(map[int]*dataChannelState),
	}, nil
}

func matchConfig(config, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(config)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func (b *backend) createPeer() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pc != nil {
		return fmt.Errorf("peer already exists")
	}
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return err
	}
	if _, err := pc.AddTransceiverFromKind(
		webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
	); err != nil {
		_ = pc.Close()
		return err
	}
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if strings.EqualFold(track.Codec().MimeType, webrtc.MimeTypeOpus) {
			go b.forwardRemoteOpus(track)
		}
	})
	b.pc = pc
	return nil
}

func (b *backend) createDataChannel(label string, channelID int, ordered, reliable bool) error {
	b.mu.Lock()
	pc := b.pc
	if b.dcs == nil {
		b.dcs = make(map[int]*dataChannelState)
	}
	if _, exists := b.dcs[channelID]; exists {
		b.mu.Unlock()
		return fmt.Errorf("data channel %d already exists", channelID)
	}
	state := &dataChannelState{openCh: make(chan struct{}), closeCh: make(chan struct{})}
	b.dcs[channelID] = state
	b.mu.Unlock()
	if pc == nil {
		return fmt.Errorf("nil peer connection")
	}
	init := &webrtc.DataChannelInit{}
	init.Ordered = &ordered
	if !reliable {
		maxRetransmits := uint16(0)
		init.MaxRetransmits = &maxRetransmits
	}
	dc, err := pc.CreateDataChannel(label, init)
	if err != nil {
		b.mu.Lock()
		delete(b.dcs, channelID)
		b.mu.Unlock()
		return err
	}
	dc.OnOpen(func() {
		state.openOnce.Do(func() {
			close(state.openCh)
			b.emitChannelState(channelID, C.GZC_RTC_CHANNEL_OPEN)
		})
	})
	dc.OnClose(func() {
		state.closeOnce.Do(func() {
			close(state.closeCh)
			b.emitChannelState(channelID, C.GZC_RTC_CHANNEL_CLOSED)
		})
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		b.mu.Lock()
		cBackend := b.cBackend
		b.mu.Unlock()
		if cBackend == nil {
			return
		}
		data := C.CBytes(msg.Data)
		defer C.free(data)
		C.gzc_cgo_emit_channel_message(
			(*C.gzc_cgo_backend_t)(cBackend),
			C.int(channelID),
			(*C.uint8_t)(data),
			C.size_t(len(msg.Data)),
			C.bool(msg.IsString),
		)
	})
	b.mu.Lock()
	state.dc = dc
	b.mu.Unlock()
	return nil
}

func (b *backend) startOffer() (string, error) {
	b.mu.Lock()
	pc := b.pc
	b.mu.Unlock()
	if pc == nil {
		return "", fmt.Errorf("nil peer connection")
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return "", err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return "", err
	}
	<-gatherComplete
	if pc.LocalDescription() == nil {
		return "", fmt.Errorf("missing local description")
	}
	return pc.LocalDescription().SDP, nil
}

func (b *backend) setRemoteSDP(answer string) error {
	b.mu.Lock()
	pc := b.pc
	states := make([]*dataChannelState, 0, len(b.dcs))
	for _, state := range b.dcs {
		states = append(states, state)
	}
	b.mu.Unlock()
	if pc == nil {
		return fmt.Errorf("nil peer connection")
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer}); err != nil {
		return err
	}
	deadline := time.After(10 * time.Second)
	for _, state := range states {
		select {
		case <-state.openCh:
		case <-deadline:
			return fmt.Errorf("timeout waiting for data channel open")
		}
	}
	return nil
}

func (b *backend) send(channelID int, data []byte, isText bool) error {
	b.mu.Lock()
	state := b.dcs[channelID]
	b.mu.Unlock()
	if state == nil || state.dc == nil {
		return fmt.Errorf("nil data channel %d", channelID)
	}
	if isText {
		return state.dc.SendText(string(data))
	}
	return state.dc.Send(data)
}

func (b *backend) forwardRemoteOpus(track *webrtc.TrackRemote) {
	if track == nil {
		return
	}
	for {
		packet, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		if len(packet.Payload) == 0 {
			continue
		}
		payload := stampedopus.Pack(uint64(time.Now().UnixMilli()), packet.Payload)
		message := make([]byte, 1+len(payload))
		message[0] = protocolStampedOpus
		copy(message[1:], payload)

		b.mu.Lock()
		cBackend := b.cBackend
		b.mu.Unlock()
		if cBackend == nil {
			return
		}
		data := C.CBytes(message)
		C.gzc_cgo_emit_channel_message(
			(*C.gzc_cgo_backend_t)(cBackend),
			C.int(0),
			(*C.uint8_t)(data),
			C.size_t(len(message)),
			C.bool(false),
		)
		C.free(data)
	}
}

func (b *backend) close() {
	b.mu.Lock()
	states := make([]*dataChannelState, 0, len(b.dcs))
	for _, state := range b.dcs {
		states = append(states, state)
	}
	pc := b.pc
	b.dcs = nil
	b.pc = nil
	b.mu.Unlock()
	for _, state := range states {
		if state != nil && state.dc != nil {
			_ = state.dc.Close()
		}
	}
	if pc != nil {
		_ = pc.Close()
	}
}

func (b *backend) closeDataChannel(channelID int) {
	b.mu.Lock()
	state := b.dcs[channelID]
	b.mu.Unlock()
	if state != nil && state.dc != nil {
		_ = state.dc.Close()
		select {
		case <-state.closeCh:
		case <-time.After(time.Second):
		}
	}
	b.mu.Lock()
	delete(b.dcs, channelID)
	b.mu.Unlock()
}

func (b *backend) emitChannelState(channelID int, state C.gzc_rtc_channel_state_t) {
	b.mu.Lock()
	cBackend := b.cBackend
	b.mu.Unlock()
	if cBackend == nil {
		return
	}
	C.gzc_cgo_emit_channel_state(
		(*C.gzc_cgo_backend_t)(cBackend),
		C.int(channelID),
		state,
	)
}

func (b *backend) clearCBackend() {
	b.mu.Lock()
	b.cBackend = nil
	b.mu.Unlock()
}

func (b *backend) setCBackend(cBackend unsafe.Pointer) {
	b.mu.Lock()
	b.cBackend = cBackend
	b.mu.Unlock()
}
