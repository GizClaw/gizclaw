package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giztunnel"
	"golang.org/x/sync/errgroup"
)

var (
	ErrNilPeerConn          = errors.New("gizclaw: nil peer conn")
	ErrNilPeerConnTransport = errors.New("gizclaw: nil peer conn transport")
	ErrNilPeerConnService   = errors.New("gizclaw: nil peer conn service")
	ErrNilPeerConnMixer     = errors.New("gizclaw: nil peer conn mixer")
	ErrPeerConnRetiring     = errors.New("gizclaw: peer conn retiring")
)

const (
	inputRouteReloadedCode    = "INPUT_ROUTE_RELOADED"
	inputRouteReloadedMessage = "input route reloaded"
	peerConnMixerFormat       = pcm.L16Mono16K
	peerConnOpusFrameDuration = 20 * time.Millisecond
	// peerConnOpusComplexity sets the downlink encoder to maximum libopus complexity.
	peerConnOpusComplexity          = 10
	peerConnPacingBufferTarget      = 500 * time.Millisecond
	peerConnPacingMaxRecoveryPerPkt = 5 * time.Millisecond
	peerConnPacingMinimumPeriod     = peerConnOpusFrameDuration - peerConnPacingMaxRecoveryPerPkt
	peerConnPacingSteadyPeriod      = peerConnOpusFrameDuration
	peerConnTelemetryQueueSize      = 32
	peerConnRuntimeStopTimeout      = 2 * time.Second
	peerDeviceInfoRefreshTimeout    = 10 * time.Second
	// peerConnInputAbortTimeout bounds how long a denied-turn abort may wait for
	// input-queue capacity. The abort holds agentInputMu, so an unbounded wait
	// would block teardown and later input transitions behind a flooding peer.
	peerConnInputAbortTimeout    = 2 * time.Second
	peerEventStreamAcceptTimeout = 10 * time.Second
	maxDeniedInputStreams        = 256
)

var peerConnTelemetryShutdownTimeout = 2 * time.Second

// PeerConn is the in-memory runtime for one active peer connection.
// It wraps the existing PeerService bundle and serves one live conn at a time.
type PeerConn struct {
	Conn            giznet.Conn
	Service         *PeerService
	ServerPublicKey giznet.PublicKey

	closeOnce               sync.Once
	agentHost               *agenthost.Service
	agentInput              peerAgentInput
	agentInputMu            sync.Mutex
	events                  *peerStreamEventBroker
	inputAccessMu           sync.Mutex
	deniedInputStreams      map[string]struct{}
	acceptedInputStreams    map[string]eventpb.StreamKind
	deniedAudioInput        bool
	deniedAudioStream       string
	deniedAudioSFU          bool
	acceptedAudioInput      bool
	acceptedAudioStream     string
	acceptedAudioSFU        bool
	acceptedAudioWorkspace  string
	sfuDroppedPackets       atomic.Uint64
	telemetryStatusMu       *sync.Mutex
	serverGenX              *peergenx.Service
	mixer                   *pcm.Mixer
	opusWriteMu             sync.Mutex
	rpc                     *rpcServer
	audioPacing             <-chan time.Time
	runtimeStopTimeout      time.Duration
	eventAcceptTimeout      time.Duration
	closed                  atomic.Bool
	retiring                atomic.Bool
	registration            atomic.Pointer[runtimeprofile.Registration]
	tunnelRouter            *giztunnel.Router
	streamLifecycle         *peerStreamLifecycle
	streamLifecycleDisabled bool
}

type peerAgentInput interface {
	agenthost.StreamSource
	agenthost.InputPusher
	Close() error
}

type peerConnInputPusher struct {
	peer  *PeerConn
	input peerAgentInput
}

func (p peerConnInputPusher) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if p.peer == nil || p.input == nil {
		return agenthost.ErrNoActiveInput
	}
	p.peer.agentInputMu.Lock()
	defer p.peer.agentInputMu.Unlock()
	if p.peer.isRetiring() {
		return ErrPeerConnRetiring
	}
	return p.input.Push(ctx, chunk)
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
	if h.streamLifecycle == nil && !h.streamLifecycleDisabled {
		h.streamLifecycle = newPeerStreamLifecycle(slog.Default(), "", h.Conn.PublicKey().String())
		h.streamLifecycleDisabled = h.streamLifecycle == nil
	}
	if h.Service.manager.allowActivePeerRole(
		context.Background(),
		h.Conn.PublicKey(),
		apitypes.PeerRoleEdgeNode,
	) {
		return h.serveEdgeNode()
	}
	h.initEvents()
	eventListener := h.Conn.ListenService(EventStreamAgent)
	if eventListener == nil {
		_ = h.close()
		return errPeerEventStreamClosed
	}
	defer func() { _ = eventListener.Close() }()
	eventStream, err := h.acceptMandatoryEventStream(eventListener)
	if err != nil {
		_ = h.close()
		return err
	}
	h.streamLifecycle.eventStreamAccepted()
	unsubscribeEvent, err := h.events.Subscribe(eventStream)
	if err != nil {
		_ = eventStream.Close()
		_ = h.close()
		return err
	}
	oldConn, err := h.Service.activateConn(context.Background(), h.Conn)
	if err != nil {
		unsubscribeEvent()
		_ = eventStream.Close()
		_ = h.close()
		return err
	}
	defer h.Service.manager.SetPeerDown(h.Conn.PublicKey(), h.Conn)
	if oldConn != nil {
		_ = oldConn.Close()
	}
	h.init()
	if !h.Service.manager.RegisterPeerRetirer(h.Conn.PublicKey(), h.Conn, func() {
		if h.retiring.CompareAndSwap(false, true) {
			h.registration.Store(nil)
		}
	}) {
		_ = h.close()
		return ErrPeerConnRetiring
	}
	if h.events != nil {
		_ = h.Service.manager.SetPeerEventBroker(
			h.Conn.PublicKey(),
			h.Conn,
			h.events,
		)
	}

	var g errgroup.Group
	g.Go(h.serveService)
	g.Go(h.servePackets)
	g.Go(h.serveRPC)
	g.Go(h.serveEdgeRPC)
	g.Go(h.serveOpenAI)
	g.Go(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), peerDeviceInfoRefreshTimeout)
		defer cancel()
		if _, _, err := h.Service.manager.RefreshPeer(ctx, h.Conn.PublicKey()); err != nil {
			slog.Debug("peer device info refresh failed", "peer_public_key", h.Conn.PublicKey().String(), "error", err)
		}
		return nil
	})
	g.Go(func() error {
		defer func() { _ = h.close() }()
		defer unsubscribeEvent()
		defer func() { _ = eventStream.Close() }()
		return h.readEventStream(eventStream)
	})
	g.Go(func() error { return h.rejectDuplicateEventStreams(eventListener) })
	err = g.Wait()
	if err != nil {
		_ = h.close()
	}
	return err
}

func (h *PeerConn) acceptMandatoryEventStream(listener giznet.ServiceListener) (net.Conn, error) {
	type acceptResult struct {
		stream net.Conn
		err    error
	}
	result := make(chan acceptResult, 1)
	go func() {
		stream, err := listener.Accept()
		result <- acceptResult{stream: stream, err: err}
	}()
	timeout := h.eventAcceptTimeout
	if timeout <= 0 {
		timeout = peerEventStreamAcceptTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case accepted := <-result:
		return accepted.stream, accepted.err
	case <-timer.C:
		_ = listener.Close()
		_ = h.close()
		return nil, fmt.Errorf(
			"%w: timed out after %s",
			errPeerEventStreamClosed,
			timeout,
		)
	}
}

func (h *PeerConn) serveEdgeNode() error {
	if err := h.Service.manager.activateEdgeTransport(context.Background(), h.Conn); err != nil {
		_ = h.close()
		return err
	}
	defer h.Service.manager.setEdgeTransportDown(h.Conn.PublicKey(), h.Conn)
	if _, err := h.initEdgeTunnelRouter(); err != nil {
		_ = h.close()
		return err
	}
	var g errgroup.Group
	g.Go(func() error {
		defer func() { _ = h.close() }()
		return h.Service.serveEdgePublicWithRetiring(h.Conn, h.isRetiring)
	})
	g.Go(h.serveEdgeRPC)
	g.Go(h.serveEdgeTunnel)
	g.Go(h.serveEdgePackets)
	return g.Wait()
}

func (h *PeerConn) serveEdgePackets() error {
	buf := make([]byte, 64*1024)
	for {
		protocol, n, err := h.Conn.Read(buf)
		if err != nil {
			if isPeerServiceClosed(err) || errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if protocol != giznet.ProtocolTunnelPacket {
			continue
		}
		if err := h.tunnelRouter.HandlePacket(buf[:n]); err != nil &&
			!errors.Is(err, giztunnel.ErrSessionNotFound) {
			slog.Warn("gizclaw: edge tunnel packet ignored", "error", err)
		}
	}
}

func (h *PeerConn) serveService() error {
	defer func() {
		_ = h.close()
	}()
	return h.Service.serveActiveConn(h.Conn, h.isRetiring)
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
	g.Go(func() error {
		defer func() { _ = h.close() }()
		return h.serveDirectPackets()
	})
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
		go handleRPCStream(stream, server.Handle)
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
	server := &edgeRPCServer{routes: h.Service.manager.PeerRoutes, apiKeys: h.Service.apiKeys, isPeerRetiring: h.isRetiring}
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
		go handleRPCStream(stream, server.Handle)
	}
}

func handleRPCStream(stream net.Conn, handle func(net.Conn) error) {
	defer func() {
		_ = stream.Close()
	}()
	_ = handle(stream)
}

func (h *PeerConn) init() {
	h.initEvents()
	h.initMixer()
	h.initPeerGenX()
	h.initAgentHost()
	h.initRPC()
}

func (h *PeerConn) initEvents() {
	if h != nil && h.events == nil {
		h.events = newPeerStreamEventBroker()
	}
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
		h.rpc.apiKeys = h.Service.apiKeys
		h.rpc.validateAPIKeyOwner = h.Service.validateAPIKeyOwner
		h.rpc.deletePeerSelf = func(ctx context.Context) error {
			return h.Service.manager.deleteActivePeer(ctx, h.Conn.PublicKey(), h.Conn, h.beginRetiring)
		}
		h.rpc.onPeerRetiring = nil
		h.rpc.onRegistration = func(registration runtimeprofile.Registration) {
			if h.Conn == nil {
				return
			}
			accepted := h.Service.manager.setPeerRegistrationIfActive(h.Conn.PublicKey(), h.Conn, registration, func() bool {
				if h.isRetiring() {
					return false
				}
				h.registration.Store(&registration)
				return true
			})
			if !accepted {
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
	h.agentInput = newPeerRealtimeSourceWithRouteReplacement(h.streamLifecycle, h.replaceAudioInputRoute)
	host := newPeerAgentHost(
		manager.AgentHost,
		resources,
		h.serverGenX,
		h.ownerGenX,
		manager.Gameplay,
		manager.FlowcraftHistory,
		manager.FlowcraftState,
		manager.MemoryRoot,
		manager.MemoryStores,
		sfu.Factory{Config: manager.SFU, Bindings: manager.sfuBindings()},
	)
	h.agentHost = &agenthost.Service{
		Host:           host,
		PeerRun:        manager.PeerRun,
		PublicKey:      h.Conn.PublicKey(),
		RuntimeProfile: h.currentRuntimeProfile,
		ClientTools:    peerClientToolInvoker{conn: h.Conn},
		ValidateWorkspaceSelection: func(ctx context.Context, name string) (string, error) {
			canonicalName, rpcErr := resources.ValidateRunWorkspaceSelection(ctx, name)
			if rpcErr != nil {
				return "", errors.New(rpcErr.Message)
			}
			return canonicalName, nil
		},
		AllowRestrictedReload: func(ctx context.Context, workspaceName string) bool {
			return manager.allowSFURestrictedReload(ctx, h.Conn.PublicKey(), workspaceName)
		},
		Source: h.agentInput,
		Consumer: peerAgentOutput{
			Events:        h.events,
			Tracks:        h,
			Packets:       h.writeOpusPacket,
			Logger:        slog.Default(),
			PeerPublicKey: h.Conn.PublicKey().String(),
			WorkspaceName: func(ctx context.Context) string {
				workspaceName, _ := h.currentInputWorkspace(ctx)
				return workspaceName
			},
			Lifecycle:         h.streamLifecycle,
			LifecycleDisabled: h.streamLifecycleDisabled,
		},
		OnConsumerError: h.broadcastAgentOutputError,
		OnWorkspaceActivated: func(ctx context.Context, workspaceName string) {
			// The Server-local name index is scoped by owner, so the record
			// is resolved through the same access path that activated it.
			item, rpcErr := resources.ResolveAccessibleWorkspace(ctx, workspaceName)
			var err error
			if rpcErr != nil {
				err = errors.New(rpcErr.Message)
			} else {
				err = manager.handleWorkspaceActivated(ctx, item)
			}
			if err != nil {
				slog.Error("activate Workspace reward",
					"workspace", workspaceName,
					"error_class", "activation",
					"error", err,
				)
			}
		},
		OnWorkspaceHistoryUpdated: manager.handleWorkspaceHistoryUpdated,
	}
	if h.rpc != nil {
		h.rpc.peerRunRuntime = h.agentHost
	}
}

func (h *PeerConn) replaceAudioInputRoute(_ context.Context, route peerAudioInputRoute) error {
	if h == nil || route.streamID == "" {
		return nil
	}
	h.inputAccessMu.Lock()
	delete(h.acceptedInputStreams, route.streamID)
	if h.acceptedAudioStream == route.streamID {
		h.acceptedAudioInput = false
		h.acceptedAudioStream = ""
		h.acceptedAudioSFU = false
		h.acceptedAudioWorkspace = ""
	}
	h.inputAccessMu.Unlock()
	if h.events == nil {
		return errPeerEventStreamClosed
	}
	if err := h.events.Broadcast(peerAudioInputRouteReloadedEvent(route)); err != nil {
		// The Event stream is mandatory. Close the underlying Peer transport
		// immediately, but let serve() perform the full PeerConn cleanup: this
		// callback runs inside an AgentHost transition, so calling h.close()
		// synchronously here would recursively wait for that same transition.
		h.closed.Store(true)
		if h.Conn != nil {
			_ = h.Conn.Close()
		}
		return err
	}
	return nil
}

func peerAudioInputRouteReloadedEvent(route peerAudioInputRoute) *eventpb.PeerEvent {
	return &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
			StreamId: route.streamID,
			Kind:     eventpb.StreamKind_STREAM_KIND_AUDIO,
			Label:    "user",
			MimeType: canonicalAudioMIMEType(route.mimeType),
			Error: &eventpb.EventError{
				Code:      inputRouteReloadedCode,
				Message:   inputRouteReloadedMessage,
				Retryable: true,
			},
		}},
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
	profile, err := h.Service.manager.RuntimeProfiles.ResolveOwnerProfile(context.Background(), h.Conn.PublicKey().String())
	if err != nil {
		return nil
	}
	return &profile
}

func (h *PeerConn) ownerRuntimeProfile(ctx context.Context, owner string) (apitypes.RuntimeProfile, error) {
	if h == nil || h.Service == nil || h.Service.manager == nil {
		return apitypes.RuntimeProfile{}, errors.New("gizclaw: manager is not configured")
	}
	return h.Service.manager.runtimeProfileForOwner(ctx, owner)
}

func (h *PeerConn) ownerGenX(ctx context.Context, owner string) (*peergenx.Service, error) {
	if h == nil || h.Service == nil || h.Service.manager == nil {
		return nil, errors.New("gizclaw: manager is not configured")
	}
	return h.Service.manager.ownerGenX(ctx, owner)
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
		if h.tunnelRouter != nil {
			closeErr = errors.Join(closeErr, h.tunnelRouter.Close())
		}
		if h.Conn != nil {
			if err := h.Conn.Close(); err != nil && !errors.Is(err, giznet.ErrConnClosed) {
				closeErr = errors.Join(closeErr, err)
			}
		}
		if h.agentInput != nil {
			closeErr = errors.Join(closeErr, h.agentInput.Close())
		}
		if h.agentHost != nil {
			timeout := h.runtimeStopTimeout
			if timeout <= 0 {
				timeout = peerConnRuntimeStopTimeout
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			_, err := h.agentHost.Shutdown(ctx)
			closeErr = errors.Join(closeErr, err)
		}
		mx := h.mixer
		if mx != nil {
			closeErr = errors.Join(closeErr, mx.Close())
		}
	})
	return closeErr
}

func (h *PeerConn) retire() {
	if h == nil {
		return
	}
	if h.Conn != nil && h.Service != nil && h.Service.manager != nil {
		h.Service.manager.retirePeer(h.Conn.PublicKey(), h.Conn, func() {
			if h.retiring.CompareAndSwap(false, true) {
				h.registration.Store(nil)
			}
		})
		return
	}
	if h.retiring.CompareAndSwap(false, true) {
		h.registration.Store(nil)
	}
}

func (h *PeerConn) beginRetiring() func() {
	previousRetiring := h.retiring.Swap(true)
	previousRegistration := h.registration.Swap(nil)
	return func() {
		h.registration.Store(previousRegistration)
		h.retiring.Store(previousRetiring)
	}
}

func (h *PeerConn) isRetiring() bool {
	return h != nil && h.retiring.Load()
}

func (h *PeerConn) handleEventStream(stream net.Conn) error {
	if stream == nil {
		return nil
	}
	unsubscribe, err := h.events.Subscribe(stream)
	if err != nil {
		return err
	}
	defer unsubscribe()
	defer func() { _ = stream.Close() }()
	return h.readEventStream(stream)
}

func (h *PeerConn) readEventStream(stream net.Conn) (err error) {
	if stream == nil {
		return nil
	}
	var terminalErr error
	defer func() {
		if terminalErr == nil {
			terminalErr = err
		}
		h.streamLifecycle.finish("peer_input", terminalErr)
	}()
	for {
		if h.isRetiring() {
			return ErrPeerConnRetiring
		}
		event, err := readPeerStreamEvent(stream)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				terminalErr = err
				return nil
			}
			return err
		}
		if h.isRetiring() {
			return ErrPeerConnRetiring
		}
		authorized, err := h.authorizeInputEvent(context.Background(), event)
		if err != nil {
			return err
		}
		if !authorized {
			continue
		}
		h.streamLifecycle.observeInput(event)
		chunk, err := peerStreamEventToChunk(event)
		if err != nil {
			return err
		}
		if err := h.pushAgentInputChunk(context.Background(), chunk); err != nil {
			return err
		}
		if event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_BOS &&
			event.StreamKindValue() == eventpb.StreamKind_STREAM_KIND_AUDIO {
			// A successful event write does not order the independent Opus
			// packet channel. Acknowledge only after authorization and routing.
			if err := h.events.Broadcast(&eventpb.PeerEvent{
				Version: eventpb.Version,
				Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_AUDIO_INPUT_READY,
				Payload: &eventpb.PeerEvent_AudioInputReady{AudioInputReady: &eventpb.AudioInputReady{StreamId: event.StreamID()}},
			}); err != nil {
				return err
			}
		}
	}
}

func (h *PeerConn) rejectDuplicateEventStreams(listener giznet.ServiceListener) error {
	for {
		stream, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || isPeerServiceClosed(err) {
				return nil
			}
			return err
		}
		_ = stream.Close()
	}
}

// authorizeInputEvent gates BOS/EOS and text turns on the Peer's current run
// Workspace. Workflow Workspaces admit input as before. SFU Workspaces admit
// input only while the Peer is a current member of the bound Social resource,
// the Workspace has not been revoked on this connection, and the SFU runtime
// is active; anything else is denied and never cached.
func (h *PeerConn) authorizeInputEvent(ctx context.Context, event *eventpb.PeerEvent) (bool, error) {
	if h == nil || event == nil || h.Service == nil || h.Service.manager == nil || h.Conn == nil {
		return true, nil
	}
	switch event.Type {
	case eventpb.PeerEventType_PEER_EVENT_TYPE_BOS,
		eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA,
		eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE:
	default:
		return true, nil
	}
	streamID := event.StreamID()
	if h.inputStreamDenied(streamID) {
		if event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_EOS ||
			event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE {
			h.clearDeniedInputStream(streamID, event.StreamKindValue())
		}
		return false, nil
	}
	run, err := h.currentRunState(ctx)
	if err != nil {
		return h.rejectInputEvent(ctx, event, streamID, sfuAccessCheckFailedError())
	}
	workspaceName := run.workspaceName
	if workspaceName == "" {
		h.acceptInputEvent(event, streamID, "", false)
		return true, nil
	}
	isSFU, denial := h.Service.manager.sfuInputAccess(ctx, h.Conn.PublicKey(), workspaceName)
	if denial == nil && isSFU && !run.active {
		denial = sfuRuntimeNotAttachedError()
	}
	if denial != nil {
		return h.rejectInputEvent(ctx, event, streamID, denial)
	}
	h.acceptInputEvent(event, streamID, workspaceName, isSFU)
	return true, nil
}

func (h *PeerConn) rejectInputEvent(
	ctx context.Context,
	event *eventpb.PeerEvent,
	streamID string,
	denial *inputAccessError,
) (bool, error) {
	abortCurrentTurn := h.markDeniedInputStream(streamID, event.StreamKindValue())
	terminal := event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_EOS ||
		event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE
	if terminal {
		h.clearDeniedInputStream(streamID, event.StreamKindValue())
	}
	var abortErr error
	if abortCurrentTurn {
		abortErr = h.abortAgentInputTurn(ctx)
	}
	broadcastErr := h.events.Broadcast(&eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
			StreamId: streamID,
			Kind:     event.StreamKindValue(),
			Label:    "assistant",
			Error:    inputAccessEventError(denial),
		}},
	})
	if err := errors.Join(abortErr, broadcastErr); err != nil {
		return false, err
	}
	return false, nil
}

// abortAgentInputTurn interrupts the Agent turn fed by a denied input stream
// without closing the runtime input source. A control-only BOS with a fresh
// StreamID supersedes denied audio still buffered in the realtime source, makes
// Audio Dock interrupt the open assistant routes, and resets the text Agent's
// pending input. Closing the source instead ended the whole Agent pipeline with
// a clean EOF, which the runtime then reported as an unexpected output end.
//
// The push runs under a bounded child of ctx because an input source may wait
// for queue capacity while this call holds agentInputMu. Without that bound a
// flooding peer, or a runtime that stopped consuming input, would block peer
// teardown and every later input transition behind the same lock.
func (h *PeerConn) abortAgentInputTurn(ctx context.Context) error {
	if h == nil || h.agentInput == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, peerConnInputAbortTimeout)
	defer cancel()
	h.agentInputMu.Lock()
	defer h.agentInputMu.Unlock()
	err := h.agentInput.Push(ctx, agentInputInterruptChunk())
	if errors.Is(err, agenthost.ErrNoActiveInput) {
		return nil
	}
	if err == nil {
		h.streamLifecycle.observeInterrupt()
	}
	return err
}

// agentInputInterruptChunk is the control-only BOS that aborts the in-flight
// input turn. It carries no MIME type so Audio Dock treats it as a text-turn
// boundary rather than audio, and its fresh StreamID matches no device stream.
func agentInputInterruptChunk() *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: genx.RoleUser,
		Ctrl: &genx.StreamCtrl{StreamID: genx.NewStreamID(), BeginOfStream: true},
	}
}

// peerRunState is the input-facing view of the Peer's run selection: the
// Workspace input belongs to, and whether its runtime is already active.
type peerRunState struct {
	workspaceName string
	active        bool
}

func (h *PeerConn) currentRunState(ctx context.Context) (peerRunState, error) {
	if h == nil || h.Service == nil || h.Service.manager == nil || h.Service.manager.PeerRun == nil || h.Conn == nil {
		return peerRunState{}, errors.New("gizclaw: Peer run state is unavailable")
	}
	run, err := h.Service.manager.PeerRun.GetRunAgent(ctx, h.Conn.PublicKey())
	if err != nil {
		return peerRunState{}, err
	}
	if run.Active != nil {
		return peerRunState{workspaceName: strings.TrimSpace(run.Active.WorkspaceName), active: true}, nil
	}
	if run.Pending != nil {
		return peerRunState{workspaceName: strings.TrimSpace(run.Pending.WorkspaceName)}, nil
	}
	return peerRunState{}, nil
}

func (h *PeerConn) currentInputWorkspace(ctx context.Context) (string, error) {
	run, err := h.currentRunState(ctx)
	if err != nil {
		return "", err
	}
	return run.workspaceName, nil
}

func (h *PeerConn) inputStreamDenied(streamID string) bool {
	h.inputAccessMu.Lock()
	defer h.inputAccessMu.Unlock()
	if streamID == "audio" && h.deniedAudioInput {
		return true
	}
	_, denied := h.deniedInputStreams[streamID]
	return denied
}

func (h *PeerConn) markDeniedInputStream(streamID string, kind eventpb.StreamKind) bool {
	h.inputAccessMu.Lock()
	defer h.inputAccessMu.Unlock()
	_, accepted := h.acceptedInputStreams[streamID]
	delete(h.acceptedInputStreams, streamID)
	if h.deniedInputStreams == nil {
		h.deniedInputStreams = make(map[string]struct{})
	}
	if len(h.deniedInputStreams) >= maxDeniedInputStreams {
		clear(h.deniedInputStreams)
	}
	h.deniedInputStreams[streamID] = struct{}{}
	if kind == eventpb.StreamKind_STREAM_KIND_AUDIO {
		h.deniedAudioInput = true
		h.deniedAudioStream = streamID
		h.deniedAudioSFU = h.acceptedAudioSFU && h.acceptedAudioStream == streamID
		h.acceptedAudioInput = false
		h.acceptedAudioStream = ""
		h.acceptedAudioSFU = false
		h.acceptedAudioWorkspace = ""
	}
	return accepted
}

func (h *PeerConn) clearDeniedInputStream(streamID string, kind eventpb.StreamKind) {
	h.inputAccessMu.Lock()
	defer h.inputAccessMu.Unlock()
	delete(h.deniedInputStreams, streamID)
	if kind == eventpb.StreamKind_STREAM_KIND_AUDIO || streamID == h.deniedAudioStream {
		h.deniedAudioInput = false
		h.deniedAudioStream = ""
		h.deniedAudioSFU = false
	}
}

func (h *PeerConn) acceptInputEvent(
	event *eventpb.PeerEvent,
	streamID string,
	workspaceName string,
	isSFU bool,
) {
	if event == nil {
		return
	}
	h.inputAccessMu.Lock()
	defer h.inputAccessMu.Unlock()
	if h.acceptedInputStreams == nil {
		h.acceptedInputStreams = make(map[string]eventpb.StreamKind)
	}
	kind := event.StreamKindValue()
	switch event.Type {
	case eventpb.PeerEventType_PEER_EVENT_TYPE_BOS:
		if len(h.acceptedInputStreams) >= maxDeniedInputStreams {
			clear(h.acceptedInputStreams)
		}
		h.acceptedInputStreams[streamID] = kind
		if kind == eventpb.StreamKind_STREAM_KIND_AUDIO {
			h.deniedAudioInput = false
			h.deniedAudioStream = ""
			h.acceptedAudioInput = true
			h.acceptedAudioStream = streamID
			h.acceptedAudioSFU = isSFU
			h.acceptedAudioWorkspace = strings.TrimSpace(workspaceName)
		}
	case eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA:
		if len(h.acceptedInputStreams) >= maxDeniedInputStreams {
			clear(h.acceptedInputStreams)
		}
		h.acceptedInputStreams[streamID] = kind
	case eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE:
		delete(h.acceptedInputStreams, streamID)
		if streamID == "" || streamID == h.acceptedAudioStream {
			h.acceptedAudioInput = false
			h.acceptedAudioStream = ""
			h.acceptedAudioSFU = false
			h.acceptedAudioWorkspace = ""
		}
	}
}

func (h *PeerConn) audioInputAccepted() bool {
	h.inputAccessMu.Lock()
	defer h.inputAccessMu.Unlock()
	return h.acceptedAudioInput && !h.deniedAudioInput
}

// authorizeAudioPacket admits a direct Opus packet only for the accepted audio
// stream of the Peer's current Workspace. Packets of a revoked SFU Workspace
// are dropped and counted; nothing is cached for later forwarding.
//
// This runs once per 20 ms packet, so it deliberately re-checks only the
// Workspace selection, which is local state. Membership and the SFU binding
// are resolved when the utterance opens, not per packet: resolving them here
// would put a shared Social KV read on every packet. Membership revoked in the
// middle of an utterance therefore stops the forwarding through the SFU
// session's periodic binding recheck instead, which bounds the exposure by one
// services.sfu.recheck_interval. The next utterance is refused at BOS.
func (h *PeerConn) authorizeAudioPacket(ctx context.Context) (bool, error) {
	if h == nil {
		return false, nil
	}
	h.inputAccessMu.Lock()
	accepted := h.acceptedAudioInput && !h.deniedAudioInput
	countDrop := !accepted && h.deniedAudioInput && h.deniedAudioSFU
	streamID := h.acceptedAudioStream
	workspaceName := h.acceptedAudioWorkspace
	h.inputAccessMu.Unlock()
	if !accepted {
		if countDrop {
			h.sfuDroppedPackets.Add(1)
		}
		return false, nil
	}
	if workspaceName == "" {
		return true, nil
	}
	currentWorkspace, err := h.currentInputWorkspace(ctx)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(currentWorkspace) == workspaceName {
		return true, nil
	}
	h.inputAccessMu.Lock()
	if h.acceptedAudioInput &&
		h.acceptedAudioStream == streamID &&
		h.acceptedAudioWorkspace == workspaceName {
		delete(h.acceptedInputStreams, streamID)
		h.acceptedAudioInput = false
		h.acceptedAudioStream = ""
		h.acceptedAudioSFU = false
		h.acceptedAudioWorkspace = ""
	}
	h.inputAccessMu.Unlock()
	return false, nil
}

func (h *PeerConn) broadcastAgentOutputError(_ context.Context, _ string, err error) {
	if h == nil || h.events == nil || err == nil {
		return
	}
	_ = h.events.Broadcast(&eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
			StreamId: "agent-output-error",
			Label:    "agent",
			Error:    &eventpb.EventError{Code: "AGENT_OUTPUT_ERROR", Message: err.Error(), Retryable: true},
		}},
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
			authorized, err := h.authorizeAudioPacket(context.Background())
			if err != nil {
				return err
			}
			if !authorized {
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
	host := h.agentHost
	input := h.agentInput
	if input == nil {
		if chunk.IsBeginOfStream() {
			return agenthost.ErrNoActiveInput
		}
		return nil
	}
	if host == nil {
		return peerConnInputPusher{peer: h, input: input}.Push(ctx, chunk)
	}
	inputPusher := peerConnInputPusher{peer: h, input: input}
	revision, pushed, err := host.PushInput(ctx, inputPusher, chunk)
	if !pushed {
		if err == nil && chunk.IsBeginOfStream() {
			return agenthost.ErrNoActiveInput
		}
		return err
	}
	if !errors.Is(err, agenthost.ErrNoActiveInput) {
		return err
	}
	reloaded, err := host.ReloadAndPushInputIfCurrentRevision(ctx, revision, inputPusher, chunk)
	if !reloaded {
		if err == nil && chunk.IsBeginOfStream() {
			return agenthost.ErrNoActiveInput
		}
		return err
	}
	if errors.Is(err, agenthost.ErrNoActiveInput) && !chunk.IsBeginOfStream() {
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

// newPeerConnOpusEncoder creates the downlink encoder with the complexity the
// mixed peer stream is sent at.
func newPeerConnOpusEncoder() (*opus.Encoder, error) {
	enc, err := opus.NewEncoder(peerConnMixerFormat.SampleRate(), peerConnMixerFormat.Channels(), opus.ApplicationAudio)
	if err != nil {
		return nil, err
	}
	if err := enc.SetComplexity(peerConnOpusComplexity); err != nil {
		_ = enc.Close()
		return nil, err
	}
	return enc, nil
}

func (h *PeerConn) streamMixedAudio(hasWrittenBefore bool) (wrote bool, err error) {
	mx := h.mixer
	enc, err := newPeerConnOpusEncoder()
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
		if !waitForPacing() {
			return wrote, nil
		}
		if !hasWrittenBefore {
			hasWrittenBefore = true
			wrote = true
		}
		if err := h.writeOpusPacket(packet); err != nil {
			return wrote, err
		}
	}
}

// writeOpusPacket writes one Opus packet to the device track. The mixer
// egress and passthrough routes share the track, and the underlying
// packetizer is not safe for concurrent writers, so every write is
// serialized here. The mixer only writes while it owns at least one track
// (pcm.Mixer.Read blocks otherwise), so an SFU passthrough route, which
// creates no mixer track, is never interleaved with mixer silence.
func (h *PeerConn) writeOpusPacket(packet []byte) error {
	if h == nil || h.Conn == nil {
		return ErrNilPeerConnTransport
	}
	h.opusWriteMu.Lock()
	defer h.opusWriteMu.Unlock()
	_, err := h.Conn.Write(giznet.ProtocolOpusPacket, packet)
	return err
}

type peerConnAudioPacer struct {
	started time.Time
	next    time.Time
	packet  int
}

func (p *peerConnAudioPacer) waitDuration(now time.Time) time.Duration {
	if p.next.IsZero() {
		p.started = now
		p.next = now
		p.packet = 1
		return 0
	}
	period := peerConnPacingSteadyPeriod
	mediaSpan := time.Duration(p.packet-1) * peerConnOpusFrameDuration
	surplus := mediaSpan - p.next.Sub(p.started)
	if deficit := peerConnPacingBufferTarget - surplus; deficit > 0 {
		recovery := min(deficit, peerConnPacingMaxRecoveryPerPkt)
		period -= recovery
	}
	p.next = p.next.Add(period)
	delay := p.next.Sub(now)
	if delay < 0 {
		// Send only the current overdue packet immediately. Rebase instead of
		// bursting; later packets replenish the target at the bounded rate above.
		p.next = now
		p.packet++
		return 0
	}
	p.packet++
	return delay
}

func (h *PeerConn) audioPacingWaiter() (func() bool, func()) {
	if h != nil && h.audioPacing != nil {
		return func() bool {
			_, ok := <-h.audioPacing
			return ok
		}, func() {}
	}
	timer := time.NewTimer(peerConnPacingMinimumPeriod)
	if !timer.Stop() {
		<-timer.C
	}
	pacer := peerConnAudioPacer{}
	return func() bool {
		delay := pacer.waitDuration(time.Now())
		if delay > 0 {
			timer.Reset(delay)
			<-timer.C
		}
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
