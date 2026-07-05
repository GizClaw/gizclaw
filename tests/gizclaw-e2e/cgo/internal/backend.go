//go:build gizclaw_e2e

package internal

/*
#cgo CFLAGS: -I. -I../../../../c/gizwebrtc/include -I../../../../c/gizwebrtc/generated
#include "bridge.h"
#include "sdk_driver.h"
#include <stdlib.h>
*/
import "C"

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/stampedopus"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
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

func (b *backend) postOffer(ctx context.Context, offer []byte) ([]byte, error) {
	var nonceRaw [16]byte
	if _, err := rand.Read(nonceRaw[:]); err != nil {
		return nil, err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceRaw[:])
	ts := time.Now().Unix()
	reqAEAD, reqNonce, respAEAD, respNonce, err := deriveSignaling(b.key, b.serverPK, nonce, ts)
	if err != nil {
		return nil, err
	}
	body := reqAEAD.Seal(nil, reqNonce, offer, requestAAD(b.key.Public, ts, nonce))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+b.endpoint+signalingPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Giznet-Public-Key", b.key.Public.String())
	req.Header.Set("X-Giznet-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Giznet-Nonce", nonce)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("signaling failed: %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	return respAEAD.Open(nil, respNonce, respBody, responseAAD(b.key.Public, ts, nonce))
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

func deriveSignaling(local *giznet.KeyPair, remote giznet.PublicKey, clientNonce string, ts int64) (cipher.AEAD, []byte, cipher.AEAD, []byte, error) {
	shared, err := local.DH(remote)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	nonceRaw, err := base64.RawURLEncoding.DecodeString(clientNonce)
	if err != nil || len(nonceRaw) != 16 {
		return nil, nil, nil, nil, fmt.Errorf("invalid signaling nonce")
	}
	salt := append([]byte{}, nonceRaw...)
	salt = strconv.AppendInt(salt, ts, 10)
	reqKey, err := hkdfBytes(shared[:], salt, "giznet/gizwebrtc/http-signaling/v1 c2s", chacha20poly1305.KeySize)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	respKey, err := hkdfBytes(shared[:], salt, "giznet/gizwebrtc/http-signaling/v1 s2c", chacha20poly1305.KeySize)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	reqNonce, err := hkdfBytes(shared[:], salt, "giznet/gizwebrtc/http-signaling/v1 c2s nonce", 12)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	respNonce, err := hkdfBytes(shared[:], salt, "giznet/gizwebrtc/http-signaling/v1 s2c nonce", 12)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	reqAEAD, err := chacha20poly1305.New(reqKey)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	respAEAD, err := chacha20poly1305.New(respKey)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return reqAEAD, reqNonce, respAEAD, respNonce, nil
}

func hkdfBytes(secret, salt []byte, info string, n int) ([]byte, error) {
	out := make([]byte, n)
	if _, err := io.ReadFull(hkdf.New(sha256.New, secret, salt, []byte(info)), out); err != nil {
		return nil, err
	}
	return out, nil
}

func requestAAD(client giznet.PublicKey, ts int64, nonce string) []byte {
	return []byte(strings.Join([]string{"POST", signalingPath, client.String(), strconv.FormatInt(ts, 10), nonce}, "\n"))
}

func responseAAD(client giznet.PublicKey, ts int64, nonce string) []byte {
	return []byte(strings.Join([]string{"POST", signalingPath, client.String(), strconv.FormatInt(ts, 10), nonce, "answer"}, "\n"))
}
