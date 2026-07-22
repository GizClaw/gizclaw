package gizclaw

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"golang.org/x/sync/errgroup"
)

var (
	ErrNilPeerConn          = errors.New("gizclaw: nil peer conn")
	ErrNilPeerConnTransport = errors.New("gizclaw: nil peer conn transport")
	ErrNilPeerConnService   = errors.New("gizclaw: nil peer conn service")
	ErrNilPeerConnMixer     = errors.New("gizclaw: nil peer conn mixer")
	ErrPeerConnRetiring     = errors.New("gizclaw: peer conn retiring")
)

const peerConnMixerFormat = pcm.L16Mono16K

const peerConnOpusFrameDuration = 20 * time.Millisecond
const peerConnTelemetryQueueSize = 32

var peerConnTelemetryShutdownTimeout = 2 * time.Second

// PeerConn is the in-memory runtime for one active peer connection.
// It wraps the existing PeerService bundle and serves one live conn at a time.
type PeerConn struct {
	Conn    giznet.Conn
	Service *PeerService

	closeOnce         sync.Once
	agentHost         *agenthost.Service
	agentInput        *peerRealtimeSource
	agentInputMu      sync.Mutex
	events            *peerStreamEventBroker
	telemetryStatusMu *sync.Mutex
	serverGenX        *peergenx.Service
	mixer             *pcm.Mixer
	rpc               *rpcServer
	audioPacing       <-chan time.Time
	closed            atomic.Bool
	retiring          atomic.Bool
	registration      atomic.Pointer[runtimeprofile.Registration]
}

// CreateAudioTrack creates a writable audio track on the peer mixer.
// The mixer itself is intentionally kept private to PeerConn.
func (h *PeerConn) CreateAudioTrack(opts ...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error) {
	if h.isRetiring() {
		return nil, nil, ErrPeerConnRetiring
	}
	mx, err := h.audioMixer()
	if err != nil {
		return nil, nil, err
	}
	return mx.CreateTrack(opts...)
}

// serve proxies to the existing PeerService implementation for one live conn.
func (h *PeerConn) serve() error {
	if h == nil {
		return ErrNilPeerConn
	}
	if h.Conn == nil {
		return ErrNilPeerConnTransport
	}
	if h.Service == nil {
		return ErrNilPeerConnService
	}
	if err := h.Service.validateServices(); err != nil {
		return err
	}
	h.init()

	var g errgroup.Group
	g.Go(h.serveService)
	g.Go(h.servePackets)
	g.Go(h.serveRPC)
	g.Go(h.serveEdgeRPC)
	g.Go(h.serveOpenAI)
	g.Go(h.serveEvents)
	err := g.Wait()
	if err != nil {
		_ = h.close()
	}
	return err
}

func (h *PeerConn) serveService() error {
	defer func() {
		_ = h.close()
	}()
	return h.Service.serveConn(h.Conn, h.isRetiring)
}

func (h *PeerConn) servePackets() error {
	if _, err := h.audioMixer(); err != nil {
		return err
	}
	var g errgroup.Group
	g.Go(func() error {
		h.streamMixedAudioLoop()
		return nil
	})
	g.Go(h.serveDirectPackets)
	return g.Wait()
}

func (h *PeerConn) serveRPC() error {
	listener := h.Conn.ListenService(ServicePeerRPC)
	defer func() {
		_ = listener.Close()
	}()
	server := h.rpcServer()
	for {
		stream, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		if h.isRetiring() {
			_ = stream.Close()
			continue
		}
		go func(stream net.Conn) {
			if err := server.Handle(stream); err != nil {
				_ = stream.Close()
			}
		}(stream)
	}
}

func (h *PeerConn) serveEdgeRPC() error {
	if h == nil || h.Service == nil || h.Service.manager == nil || h.Service.manager.PeerRoutes == nil {
		return nil
	}
	listener := h.Conn.ListenService(ServiceEdgeRPC)
	defer func() {
		_ = listener.Close()
	}()
	server := &edgeRPCServer{routes: h.Service.manager.PeerRoutes, isPeerRetiring: h.isRetiring}
	for {
		stream, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		if h.isRetiring() {
			_ = stream.Close()
			continue
		}
		go func(stream net.Conn) {
			if err := server.Handle(stream); err != nil {
				_ = stream.Close()
			}
		}(stream)
	}
}

func (h *PeerConn) init() {
	h.initMixer()
	h.initPeerGenX()
	h.initAgentHost()
	h.initRPC()
}

func (h *PeerConn) initRPC() {
	if h == nil || h.rpc != nil {
		return
	}
	h.rpc = &rpcServer{}
	h.rpc.isPeerRetiring = h.isRetiring
	h.rpc.onPeerRetiring = h.retire
	h.rpc.onPeerDeleted = func() {
		_ = h.close()
	}
	if h.Service != nil && h.Service.manager != nil {
		h.rpc.peer = h.Service.manager.Peers
		h.rpc.peerRun = h.Service.manager.PeerRun
		h.rpc.peerRunRuntime = h.agentHost
		h.rpc.serverGenX = h.serverGenX
		h.rpc.speechLimits = h.Service.manager.SpeechLimits
		h.rpc.serverResources = h.peerResources()
		h.rpc.registrations = h.Service.manager.RuntimeProfiles
		h.rpc.onRegistration = func(registration runtimeprofile.Registration) {
			if h.isRetiring() {
				return
			}
			h.registration.Store(&registration)
			accepted := true
			if h.Conn != nil {
				accepted = h.Service.manager.SetPeerRegistration(h.Conn.PublicKey(), h.Conn, registration)
			}
			if h.isRetiring() || !accepted {
				h.registration.CompareAndSwap(&registration, nil)
			}
		}
	}
	if h.Conn != nil {
		h.rpc.callerPublicKey = h.Conn.PublicKey()
		if info := h.Conn.PeerInfo(); info != nil && info.Endpoint != nil {
			h.rpc.registrationSource = info.Endpoint.String()
		}
	}
}

func (h *PeerConn) rpcServer() *rpcServer {
	h.initMixer()
	h.initPeerGenX()
	h.initAgentHost()
	h.initRPC()
	return h.rpc
}

func (h *PeerConn) initMixer() {
	if h == nil {
		return
	}
	if h.mixer == nil {
		h.mixer = pcm.NewMixer(peerConnMixerFormat)
	}
}

func (h *PeerConn) initAgentHost() {
	if h == nil || h.agentHost != nil || h.Conn == nil || h.Service == nil || h.Service.manager == nil {
		return
	}
	manager := h.Service.manager
	if manager.AgentHost == nil || manager.PeerRun == nil {
		return
	}
	resources := h.peerResources()
	h.agentInput = newPeerRealtimeSource()
	h.events = newPeerStreamEventBroker()
	host := newPeerAgentHost(manager.AgentHost, h.serverGenX, manager.Gameplay, manager.PetWorkflow, manager.FlowcraftHistory)
	h.agentHost = &agenthost.Service{
		Host:           host,
		PeerRun:        manager.PeerRun,
		PublicKey:      h.Conn.PublicKey(),
		RuntimeProfile: h.currentRuntimeProfile,
		ValidateWorkspaceSelection: func(ctx context.Context, name string) (string, error) {
			canonicalName, rpcErr := resources.ValidateRunWorkspaceSelection(ctx, name)
			if rpcErr != nil {
				return "", errors.New(rpcErr.Message)
			}
			return canonicalName, nil
		},
		Source: h.agentInput,
		Consumer: peerAgentOutput{
			Events: h.events,
			Tracks: h,
		},
		OnConsumerError: h.broadcastAgentOutputError,
	}
	if h.rpc != nil {
		h.rpc.peerRunRuntime = h.agentHost
	}
}

func (h *PeerConn) initPeerGenX() {
	if h == nil || h.serverGenX != nil || h.Conn == nil || h.Service == nil || h.Service.manager == nil {
		return
	}
	manager := h.Service.manager
	if manager.Models == nil || manager.Voices == nil || manager.Credentials == nil || manager.ProviderTenants == nil {
		return
	}
	resources := h.peerResources()
	h.serverGenX = peergenx.New(peergenx.Service{
		Peer:            h.Conn,
		Models:          resources,
		Voices:          resources,
		Credentials:     manager.Credentials,
		ProviderTenants: manager.ProviderTenants,
		AudioOutput:     agenthost.MixerOutput{Tracks: h},
	})
	if h.rpc != nil {
		h.rpc.serverGenX = h.serverGenX
	}
}

func (h *PeerConn) peerResources() *peerresource.Server {
	if h == nil || h.Conn == nil || h.Service == nil || h.Service.manager == nil {
		return nil
	}
	manager := h.Service.manager
	resources := &peerresource.Server{
		Caller:         h.Conn.PublicKey(),
		Peers:          manager.Peers,
		Firmwares:      manager.Firmwares,
		Workspaces:     manager.Workspaces,
		Workflows:      manager.Workflows,
		Models:         manager.Models,
		Voices:         manager.Voices,
		Contacts:       manager.Contacts,
		Friends:        manager.Friends,
		FriendGroups:   manager.FriendGroups,
		Gameplay:       manager.Gameplay,
		Tools:          manager.Tools,
		RuntimeProfile: h.currentRuntimeProfile,
	}
	if h.serverGenX != nil {
		resources.RewardEvaluator = gameplay.GenXRewardEvaluator{Generator: h.serverGenX.Generator()}
	}
	return resources
}

func (h *PeerConn) currentRuntimeProfile() *apitypes.RuntimeProfile {
	if h == nil || h.Service == nil || h.Service.manager == nil || h.Service.manager.RuntimeProfiles == nil {
		return nil
	}
	registration := h.registration.Load()
	if registration == nil {
		return nil
	}
	profile, err := h.Service.manager.RuntimeProfiles.ResolveProfile(context.Background(), registration.RuntimeProfile.Name)
	if err != nil {
		return nil
	}
	return &profile
}

func (h *PeerConn) audioMixer() (*pcm.Mixer, error) {
	if h == nil {
		return nil, ErrNilPeerConn
	}
	if h.mixer == nil {
		return nil, ErrNilPeerConnMixer
	}
	return h.mixer, nil
}

func (h *PeerConn) close() error {
	if h == nil {
		return nil
	}
	var closeErr error
	h.closeOnce.Do(func() {
		h.closed.Store(true)
		if h.agentHost != nil {
			_, err := h.agentHost.Stop(context.Background())
			closeErr = errors.Join(closeErr, err)
		}
		if h.agentInput != nil {
			closeErr = errors.Join(closeErr, h.agentInput.Close())
		}
		if h.Conn != nil {
			if err := h.Conn.Close(); err != nil && !errors.Is(err, giznet.ErrConnClosed) {
				closeErr = errors.Join(closeErr, err)
			}
		}
		mx := h.mixer
		if mx != nil {
			closeErr = errors.Join(closeErr, mx.Close())
		}
	})
	return closeErr
}

func (h *PeerConn) retire() {
	if h == nil || !h.retiring.CompareAndSwap(false, true) {
		return
	}
	h.registration.Store(nil)
	if h.Conn != nil && h.Service != nil && h.Service.manager != nil {
		h.Service.manager.SetPeerDown(h.Conn.PublicKey(), h.Conn)
	}
}

func (h *PeerConn) isRetiring() bool {
	return h != nil && h.retiring.Load()
}

func (h *PeerConn) serveEvents() error {
	listener := h.Conn.ListenService(EventStreamAgent)
	defer func() {
		_ = listener.Close()
	}()
	for {
		stream, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		if h.isRetiring() {
			_ = stream.Close()
			continue
		}
		go func(stream net.Conn) {
			if err := h.handleEventStream(stream); err != nil {
				_ = stream.Close()
			}
		}(stream)
	}
}

func (h *PeerConn) handleEventStream(stream net.Conn) error {
	if stream == nil {
		return nil
	}
	unsubscribe := h.events.Subscribe(stream)
	defer unsubscribe()
	defer func() { _ = stream.Close() }()
	for {
		if h.isRetiring() {
			return ErrPeerConnRetiring
		}
		event, err := readPeerStreamEvent(stream)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		if h.isRetiring() {
			return ErrPeerConnRetiring
		}
		chunk, err := peerStreamEventToChunk(event)
		if err != nil {
			return err
		}
		if err := h.pushAgentInputChunk(context.Background(), chunk); err != nil {
			return err
		}
	}
}

func (h *PeerConn) broadcastAgentOutputError(_ context.Context, _ string, err error) {
	if h == nil || h.events == nil || err == nil {
		return
	}
	label := "agent"
	message := err.Error()
	_ = h.events.Broadcast(apitypes.PeerStreamEvent{
		V:     peerStreamEventVersion,
		Type:  apitypes.PeerStreamEventTypeEos,
		Label: &label,
		Error: &message,
	})
}

func (h *PeerConn) serveDirectPackets() error {
	buf := make([]byte, 64*1024)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var peer giznet.PublicKey
	if h != nil && h.Conn != nil {
		peer = h.Conn.PublicKey()
	}
	var manager *Manager
	if h != nil && h.Service != nil {
		manager = h.Service.manager
	}
	if manager != nil && !peer.IsZero() {
		h.telemetryStatusMu = manager.retainTelemetryStatusLock(peer, true)
		defer func() {
			h.telemetryStatusMu = nil
			manager.releaseTelemetryStatusLock(peer)
		}()
	}
	telemetryPackets := make(chan []byte, peerConnTelemetryQueueSize)
	telemetryDone := make(chan struct{})
	go h.processTelemetryPackets(ctx, telemetryPackets, telemetryDone)
	defer func() {
		close(telemetryPackets)
		select {
		case <-telemetryDone:
		case <-time.After(peerConnTelemetryShutdownTimeout):
			cancel()
		}
	}()
	for {
		protocol, n, err := h.Conn.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) ||
				errors.Is(err, net.ErrClosed) ||
				errors.Is(err, giznet.ErrConnClosed) ||
				errors.Is(err, giznet.ErrClosed) ||
				errors.Is(err, giznet.ErrServiceMuxClosed) {
				return nil
			}
			return err
		}
		if h.isRetiring() {
			continue
		}
		switch protocol {
		case giznet.ProtocolOpusPacket:
			chunk, ok := opusPacketChunk(buf[:n])
			if !ok {
				continue
			}
			if err := h.pushAgentInputChunk(context.Background(), chunk); err != nil {
				return err
			}
		case EventStreamTelemetry:
			payload := append([]byte(nil), buf[:n]...)
			select {
			case telemetryPackets <- payload:
			default:
				slog.Warn("gizclaw: peer telemetry packet dropped", "reason", "queue_full")
			}
		default:
			// Unknown direct packets are ignored by the echo slice; service
			// protocols continue to be handled by service streams.
		}
	}
}

func (h *PeerConn) processTelemetryPackets(ctx context.Context, packets <-chan []byte, done chan<- struct{}) {
	defer close(done)
	for payload := range packets {
		if h.isRetiring() {
			continue
		}
		if err := h.handleTelemetryPacket(ctx, payload); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("gizclaw: peer telemetry packet ignored", "error", err)
		}
	}
}

func (h *PeerConn) handleTelemetryPacket(ctx context.Context, payload []byte) error {
	if h == nil || h.Conn == nil || h.Service == nil || h.Service.manager == nil {
		return ErrNilPeerConnService
	}
	manager := h.Service.manager
	peer := h.Conn.PublicKey()
	service := &peertelemetry.Service{
		Metrics: manager.Metrics,
		Status: peerConnTelemetryStatusSync{
			mu:   h.telemetryStatusLock(peer),
			next: peertelemetry.StatusSync{Store: manager.PeerRun},
		},
	}
	return service.ReportPacket(ctx, peer, payload)
}

func (h *PeerConn) telemetryStatusLock(peer giznet.PublicKey) *sync.Mutex {
	if h != nil && h.telemetryStatusMu != nil {
		return h.telemetryStatusMu
	}
	if h == nil || h.Service == nil || h.Service.manager == nil {
		return nil
	}
	return h.Service.manager.telemetryStatusLock(peer)
}

type peerConnTelemetryStatusSync struct {
	mu   *sync.Mutex
	next peertelemetry.StatusService
}

func (s peerConnTelemetryStatusSync) SyncTelemetryStatus(ctx context.Context, peer giznet.PublicKey, patch peertelemetry.StatusPatch) error {
	if s.next == nil {
		return peertelemetry.ErrStatusServiceNil
	}
	if s.mu == nil {
		return s.next.SyncTelemetryStatus(ctx, peer, patch)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.next.SyncTelemetryStatus(ctx, peer, patch)
}

func (h *PeerConn) pushAgentInputChunk(ctx context.Context, chunk *genx.MessageChunk) error {
	if h == nil || chunk == nil {
		return nil
	}
	if h.isRetiring() {
		return ErrPeerConnRetiring
	}
	h.agentInputMu.Lock()
	defer h.agentInputMu.Unlock()
	if h.isRetiring() {
		return ErrPeerConnRetiring
	}
	if h.agentInput == nil {
		return nil
	}
	err := h.agentInput.Push(ctx, chunk)
	if !errors.Is(err, agenthost.ErrNoActiveInput) {
		return err
	}
	if h.agentHost == nil {
		return nil
	}
	if _, reloadErr := h.agentHost.Reload(ctx); reloadErr != nil {
		return reloadErr
	}
	err = h.agentInput.Push(ctx, chunk)
	if errors.Is(err, agenthost.ErrNoActiveInput) {
		return nil
	}
	return err
}

func (h *PeerConn) streamMixedAudioLoop() {
	hasWrittenBefore := false
	for !h.isClosed() && !h.isRetiring() {
		wrote, err := h.streamMixedAudio(hasWrittenBefore)
		hasWrittenBefore = hasWrittenBefore || wrote
		if err != nil {
			slog.Error("gizclaw: mixed audio stream failed; retrying", "error", err)
		}
	}
}

func (h *PeerConn) streamMixedAudio(hasWrittenBefore bool) (wrote bool, err error) {
	mx := h.mixer
	enc, err := opus.NewEncoder(peerConnMixerFormat.SampleRate(), peerConnMixerFormat.Channels(), opus.ApplicationAudio)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = enc.Close()
	}()
	waitForPacing, stopPacing := h.audioPacingWaiter()
	defer stopPacing()

	frameSize := int(peerConnMixerFormat.SamplesInDuration(peerConnOpusFrameDuration))
	for {
		if h.isRetiring() {
			return wrote, nil
		}
		if !waitForPacing() {
			return wrote, nil
		}
		chunk, err := peerConnMixerFormat.ReadChunk(mx, peerConnOpusFrameDuration)
		if err != nil {
			if h.isClosed() && errors.Is(err, io.ErrClosedPipe) {
				return wrote, nil
			}
			return wrote, err
		}

		packet, err := enc.Encode(peerConnPCMChunkToInt16(chunk), frameSize)
		if err != nil {
			return wrote, err
		}
		if !hasWrittenBefore {
			hasWrittenBefore = true
			wrote = true
		}
		if _, err := h.Conn.Write(giznet.ProtocolOpusPacket, packet); err != nil {
			return wrote, err
		}
	}
}

func (h *PeerConn) audioPacingWaiter() (func() bool, func()) {
	if h != nil && h.audioPacing != nil {
		return func() bool {
			_, ok := <-h.audioPacing
			return ok
		}, func() {}
	}
	timer := time.NewTimer(peerConnOpusFrameDuration)
	if !timer.Stop() {
		<-timer.C
	}
	return func() bool {
		timer.Reset(peerConnOpusFrameDuration)
		<-timer.C
		return true
	}, func() { timer.Stop() }
}

func (h *PeerConn) isClosed() bool {
	if h == nil {
		return true
	}
	return h.closed.Load()
}

func peerConnPCMChunkToInt16(chunk pcm.Chunk) []int16 {
	dataChunk, ok := chunk.(*pcm.DataChunk)
	if !ok || len(dataChunk.Data) == 0 {
		return nil
	}
	data := dataChunk.Data
	out := make([]int16, len(data)/2)
	for i := range out {
		lo := uint16(data[i*2])
		hi := uint16(data[i*2+1]) << 8
		out[i] = int16(lo | hi)
	}
	return out
}
