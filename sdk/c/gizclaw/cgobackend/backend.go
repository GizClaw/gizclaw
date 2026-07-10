package cgobackend

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/stampedopus"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	SignalingPath   = "/webrtc/v1/offer"
	MediaStreamOpus = "audio/opus"

	HTTPMethodGet     = 1
	HTTPMethodPost    = 2
	HTTPMethodPut     = 3
	HTTPMethodPatch   = 4
	HTTPMethodDelete  = 5
	HTTPMethodHead    = 6
	HTTPMethodOptions = 7

	CipherChaCha20Poly1305 = 1
	CipherAES256GCM        = 2
	CipherPlaintext        = 3

	RTCChannelOpen   = 1
	RTCChannelClosed = 2
)

type HTTPHeader struct {
	Name  string
	Value string
}

type HTTPResponse struct {
	StatusCode int
	Body       []byte
}

type EventSink interface {
	RemoteChannel(channelID int, label string, ordered, reliable bool)
	ChannelState(channelID int, state int)
	ChannelMessage(channelID int, data []byte, isText bool)
}

type Backend struct {
	mu         sync.Mutex
	dispatchMu sync.Mutex
	pc         *webrtc.PeerConnection
	dcs        map[int]*dataChannelState
	nextDCID   int
	sink       EventSink
	events     []backendEvent
	eventReady chan struct{}
	closed     bool
}

type dataChannelState struct {
	dc        *webrtc.DataChannel
	label     string
	ordered   bool
	reliable  bool
	openCh    chan struct{}
	closeCh   chan struct{}
	openOnce  sync.Once
	closeOnce sync.Once
}

type backendEventKind uint8

const (
	backendEventRemoteChannel backendEventKind = iota
	backendEventChannelState
	backendEventChannelMessage
)

type backendEvent struct {
	kind      backendEventKind
	channelID int
	label     string
	ordered   bool
	reliable  bool
	state     int
	data      []byte
	isText    bool
}

func New() *Backend {
	return &Backend{
		dcs:        make(map[int]*dataChannelState),
		nextDCID:   3,
		eventReady: make(chan struct{}, 1),
	}
}

func Random(out []byte) error {
	_, err := io.ReadFull(rand.Reader, out)
	return err
}

func TimeUnixMs() int64 {
	return time.Now().UnixMilli()
}

func KeyPairFromPrivate(private []byte) (giznet.KeyPair, error) {
	var key giznet.Key
	if len(private) != giznet.KeySize {
		return giznet.KeyPair{}, fmt.Errorf("invalid private key length")
	}
	copy(key[:], private)
	kp, err := giznet.NewKeyPair(key)
	if err != nil {
		return giznet.KeyPair{}, err
	}
	return *kp, nil
}

func DH(private, remotePublic []byte) (giznet.Key, error) {
	var privateKey giznet.Key
	var remote giznet.PublicKey
	if len(private) != giznet.KeySize || len(remotePublic) != giznet.KeySize {
		return giznet.Key{}, fmt.Errorf("invalid key length")
	}
	copy(privateKey[:], private)
	copy(remote[:], remotePublic)
	kp, err := giznet.NewKeyPair(privateKey)
	if err != nil {
		return giznet.Key{}, err
	}
	return kp.DH(remote)
}

func HKDFSHA256(secret, salt []byte, info string, out []byte) error {
	_, err := io.ReadFull(hkdf.New(sha256.New, secret, salt, []byte(info)), out)
	return err
}

func AEADSeal(mode int, key, nonce, plaintext, aad []byte) ([]byte, error) {
	aead, err := newPlatformAEAD(mode, key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("invalid nonce length")
	}
	return aead.Seal(nil, nonce, plaintext, aad), nil
}

func AEADOpen(mode int, key, nonce, ciphertext, aad []byte) ([]byte, error) {
	aead, err := newPlatformAEAD(mode, key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("invalid nonce length")
	}
	return aead.Open(nil, nonce, ciphertext, aad)
}

func (b *Backend) SetEventSink(sink EventSink) {
	b.mu.Lock()
	b.sink = sink
	b.mu.Unlock()
}

func (b *Backend) HTTPRequest(method int, url string, headers []HTTPHeader, body []byte) (HTTPResponse, error) {
	methodText, ok := httpMethod(method)
	if !ok {
		return HTTPResponse{}, fmt.Errorf("unsupported HTTP method %d", method)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, methodText, url, bytes.NewReader(body))
	if err != nil {
		return HTTPResponse{}, err
	}
	for _, header := range headers {
		req.Header.Set(header.Name, header.Value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return HTTPResponse{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return HTTPResponse{}, err
	}
	return HTTPResponse{StatusCode: resp.StatusCode, Body: respBody}, nil
}

func (b *Backend) CreatePeer() error {
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
		if strings.EqualFold(track.Codec().MimeType, MediaStreamOpus) {
			go b.forwardRemoteOpus(track)
		}
	})
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		b.acceptRemoteDataChannel(dc)
	})
	b.pc = pc
	b.closed = false
	return nil
}

func (b *Backend) acceptRemoteDataChannel(dc *webrtc.DataChannel) {
	if dc == nil {
		return
	}
	ordered := dc.Ordered()
	reliable := dc.MaxRetransmits() == nil && dc.MaxPacketLifeTime() == nil
	state := &dataChannelState{
		dc: dc, label: dc.Label(), ordered: ordered, reliable: reliable,
		openCh: make(chan struct{}), closeCh: make(chan struct{}),
	}
	b.mu.Lock()
	if b.closed || b.dcs == nil {
		b.mu.Unlock()
		_ = dc.Close()
		return
	}
	channelID := b.nextDCID
	b.nextDCID++
	b.dcs[channelID] = state
	b.mu.Unlock()
	b.enqueue(backendEvent{
		kind: backendEventRemoteChannel, channelID: channelID,
		label: state.label, ordered: state.ordered, reliable: state.reliable,
	})
	dc.OnOpen(func() {
		state.openOnce.Do(func() {
			close(state.openCh)
			b.emitChannelState(channelID, RTCChannelOpen)
		})
	})
	dc.OnClose(func() {
		state.closeOnce.Do(func() {
			close(state.closeCh)
			b.mu.Lock()
			if b.dcs[channelID] == state {
				delete(b.dcs, channelID)
			}
			b.mu.Unlock()
			b.emitChannelState(channelID, RTCChannelClosed)
		})
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		b.emitChannelMessage(channelID, msg.Data, msg.IsString)
	})
}

func (b *Backend) CreateDataChannel(label string, channelID int, ordered, reliable bool) error {
	b.mu.Lock()
	pc := b.pc
	if b.dcs == nil {
		b.dcs = make(map[int]*dataChannelState)
	}
	if _, exists := b.dcs[channelID]; exists {
		b.mu.Unlock()
		return fmt.Errorf("data channel %d already exists", channelID)
	}
	state := &dataChannelState{
		label: label, ordered: ordered, reliable: reliable,
		openCh: make(chan struct{}), closeCh: make(chan struct{}),
	}
	b.dcs[channelID] = state
	b.mu.Unlock()
	if pc == nil {
		b.mu.Lock()
		delete(b.dcs, channelID)
		b.mu.Unlock()
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
			b.emitChannelState(channelID, RTCChannelOpen)
		})
	})
	dc.OnClose(func() {
		state.closeOnce.Do(func() {
			close(state.closeCh)
			b.mu.Lock()
			if b.dcs[channelID] == state {
				delete(b.dcs, channelID)
			}
			b.mu.Unlock()
			b.emitChannelState(channelID, RTCChannelClosed)
		})
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		b.emitChannelMessage(channelID, msg.Data, msg.IsString)
	})
	b.mu.Lock()
	state.dc = dc
	b.mu.Unlock()
	return nil
}

func (b *Backend) StartOffer() (string, error) {
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

func (b *Backend) SetRemoteSDP(answer string) error {
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

func (b *Backend) Poll(timeoutMS int) {
	b.dispatchMu.Lock()
	defer b.dispatchMu.Unlock()

	b.mu.Lock()
	hasEvents := len(b.events) != 0
	closed := b.closed
	ready := b.eventReady
	b.mu.Unlock()
	if hasEvents {
		select {
		case <-ready:
		default:
		}
	} else if !closed && timeoutMS > 0 {
		timer := time.NewTimer(time.Duration(timeoutMS) * time.Millisecond)
		select {
		case <-ready:
			timer.Stop()
		case <-timer.C:
		}
	}

	b.mu.Lock()
	events := b.events
	b.events = nil
	sink := b.sink
	closed = b.closed
	b.mu.Unlock()
	if sink == nil || closed {
		return
	}
	for _, event := range events {
		switch event.kind {
		case backendEventRemoteChannel:
			sink.RemoteChannel(event.channelID, event.label, event.ordered, event.reliable)
		case backendEventChannelState:
			sink.ChannelState(event.channelID, event.state)
		case backendEventChannelMessage:
			sink.ChannelMessage(event.channelID, event.data, event.isText)
		}
	}
}

func (b *Backend) Send(channelID int, data []byte, isText bool) error {
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

func (b *Backend) CloseDataChannel(channelID int) {
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

func (b *Backend) Close() {
	b.dispatchMu.Lock()
	defer b.dispatchMu.Unlock()
	b.mu.Lock()
	states := make([]*dataChannelState, 0, len(b.dcs))
	for _, state := range b.dcs {
		states = append(states, state)
	}
	pc := b.pc
	b.dcs = nil
	b.pc = nil
	b.sink = nil
	b.events = nil
	b.closed = true
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

func (b *Backend) forwardRemoteOpus(track *webrtc.TrackRemote) {
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
		message[0] = giznet.ProtocolStampedOpusPacket
		copy(message[1:], payload)
		b.emitChannelMessage(0, message, false)
	}
}

func (b *Backend) emitChannelState(channelID int, state int) {
	b.enqueue(backendEvent{kind: backendEventChannelState, channelID: channelID, state: state})
}

func (b *Backend) emitChannelMessage(channelID int, data []byte, isText bool) {
	b.enqueue(backendEvent{
		kind: backendEventChannelMessage, channelID: channelID,
		data: append([]byte(nil), data...), isText: isText,
	})
}

func (b *Backend) enqueue(event backendEvent) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.events = append(b.events, event)
	ready := b.eventReady
	b.mu.Unlock()
	select {
	case ready <- struct{}{}:
	default:
	}
}

func httpMethod(method int) (string, bool) {
	switch method {
	case HTTPMethodGet:
		return http.MethodGet, true
	case HTTPMethodPost:
		return http.MethodPost, true
	case HTTPMethodPut:
		return http.MethodPut, true
	case HTTPMethodPatch:
		return http.MethodPatch, true
	case HTTPMethodDelete:
		return http.MethodDelete, true
	case HTTPMethodHead:
		return http.MethodHead, true
	case HTTPMethodOptions:
		return http.MethodOptions, true
	default:
		return "", false
	}
}

func newPlatformAEAD(mode int, key []byte) (cipher.AEAD, error) {
	switch mode {
	case CipherChaCha20Poly1305:
		return chacha20poly1305.New(key)
	case CipherAES256GCM:
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		return cipher.NewGCM(block)
	case CipherPlaintext:
		return plaintextAEAD{}, nil
	default:
		return nil, fmt.Errorf("unsupported cipher mode %d", mode)
	}
}

type plaintextAEAD struct{}

func (plaintextAEAD) NonceSize() int { return 12 }
func (plaintextAEAD) Overhead() int  { return 0 }
func (plaintextAEAD) Seal(dst, _nonce, plaintext, _aad []byte) []byte {
	return append(dst, plaintext...)
}
func (plaintextAEAD) Open(dst, _nonce, ciphertext, _aad []byte) ([]byte, error) {
	return append(dst, ciphertext...), nil
}
