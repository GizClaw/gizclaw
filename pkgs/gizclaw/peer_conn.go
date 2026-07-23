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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/chatroom"
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

const (
	peerConnMixerFormat        = pcm.L16Mono16K
	peerConnOpusFrameDuration  = 20 * time.Millisecond
	peerConnTelemetryQueueSize = 32
	peerConnRuntimeStopTimeout = 2 * time.Second
	maxDeniedInputStreams      = 256
)

var peerConnTelemetryShutdownTimeout = 2 * time.Second

// PeerConn is the in-memory runtime for one active peer connection.
// It wraps the existing PeerService bundle and serves one live conn at a time.
type PeerConn struct {
	Conn    giznet.Conn
	Service *PeerService

	closeOnce              sync.Once
	agentHost              *agenthost.Service
	agentInput             peerAgentInput
	agentInputMu           sync.Mutex
	events                 *peerStreamEventBroker
	chatroomAccessMu       sync.Mutex
	deniedInputStreams     map[string]struct{}
	acceptedInputStreams   map[string]eventpb.StreamKind
	deniedAudioInput       bool
	deniedAudioStream      string
	acceptedAudioInput     bool
	acceptedAudioStream    string
	acceptedAudioChatroom  bool
	acceptedAudioWorkspace string
	telemetryStatusMu      *sync.Mutex
	serverGenX             *peergenx.Service
	mixer                  *pcm.Mixer
	rpc                    *rpcServer
	audioPacing            <-chan time.Time
	runtimeStopTimeout     time.Duration
	closed                 atomic.Bool
	retiring               atomic.Bool
	registration           atomic.Pointer[runtimeprofile.Registration]
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
	oldConn, err := h.Service.activateConn(context.Background(), h.Conn)
	if err != nil {
		_ = h.close()
		return err
	}
	defer h.Service.manager.SetPeerDown(h.Conn.PublicKey(), h.Conn)
	if oldConn != nil {
		_ = oldConn.Close()
	}
	h.init()
	if h.events != nil {
		_ = h.Service.manager.SetPeerEventBroker(
			h.Conn.PublicKey(),
			h.Conn,
			h.events,
			h.observePeerEvent,
		)
	}

	var g errgroup.Group
	g.Go(h.serveService)
	g.Go(h.servePackets)
	g.Go(h.serveRPC)
	g.Go(h.serveEdgeRPC)
	g.Go(h.serveOpenAI)
	g.Go(h.serveEvents)
	err = g.Wait()
	if err != nil {
		_ = h.close()
	}
	return err
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
	h.agentInput = newPeerRealtimeSource()
	h.events = newPeerStreamEventBroker()
	host := newPeerAgentHost(manager.AgentHost, h.serverGenX, h.ownerGenX, manager.Gameplay, manager.FlowcraftHistory, manager.FlowcraftState, manager.FlowcraftMemoryObjects)
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
		AllowRestrictedReload: manager.isChatroomWorkspace,
		Source:                h.agentInput,
		Consumer: peerAgentOutput{
			Events: h.events,
			Tracks: h,
		},
		OnConsumerError:           h.broadcastAgentOutputError,
		OnWorkspaceHistoryUpdated: manager.broadcastWorkspaceHistoryUpdated,
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
	profile, err := h.ownerRuntimeProfile(ctx, owner)
	if err != nil {
		return nil, err
	}
	manager := h.Service.manager
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(owner)); err != nil {
		return nil, fmt.Errorf("gizclaw: invalid workspace owner public key %q: %w", owner, err)
	}
	resources := &peerresource.Server{
		Caller:         publicKey,
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
		RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile },
	}
	return peergenx.New(peergenx.Service{
		Models: resources, Voices: resources, Credentials: manager.Credentials, ProviderTenants: manager.ProviderTenants,
	}), nil
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
	unsubscribe, err := h.events.Subscribe(stream)
	if err != nil {
		return err
	}
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
		authorized, err := h.authorizeChatroomEvent(context.Background(), event)
		if err != nil {
			return err
		}
		if !authorized {
			continue
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

func (h *PeerConn) authorizeChatroomEvent(ctx context.Context, event *eventpb.PeerEvent) (bool, error) {
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
	workspaceName, workspaceErr := h.currentInputWorkspace(ctx)
	if workspaceErr != nil {
		return h.rejectChatroomEvent(
			event,
			streamID,
			chatroom.AccessCheckFailedError(),
		)
	}
	if workspaceName == "" {
		h.acceptInputEvent(event, streamID, "", false)
		return true, nil
	}
	isChatroom, denial := h.Service.manager.chatroomAccessState(
		ctx,
		h.Conn.PublicKey(),
		workspaceName,
	)
	if denial == nil {
		h.acceptInputEvent(event, streamID, workspaceName, isChatroom)
		return true, nil
	}
	return h.rejectChatroomEvent(event, streamID, denial)
}

func (h *PeerConn) rejectChatroomEvent(
	event *eventpb.PeerEvent,
	streamID string,
	denial *chatroom.AccessError,
) (bool, error) {
	abortCurrentTurn := h.markDeniedInputStream(streamID, event.StreamKindValue())
	terminal := event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_EOS ||
		event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE
	if terminal {
		h.clearDeniedInputStream(streamID, event.StreamKindValue())
	}
	var abortErr error
	if abortCurrentTurn {
		abortErr = h.abortAgentInputTurn()
	}
	broadcastErr := h.events.Broadcast(&eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
			StreamId: streamID,
			Kind:     event.StreamKindValue(),
			Label:    "assistant",
			Error:    chatroomEventError(denial),
		}},
	})
	if err := errors.Join(abortErr, broadcastErr); err != nil {
		return false, err
	}
	return false, nil
}

func (h *PeerConn) abortAgentInputTurn() error {
	if h == nil || h.agentInput == nil {
		return nil
	}
	h.agentInputMu.Lock()
	defer h.agentInputMu.Unlock()
	return h.agentInput.Close()
}

func chatroomEventError(err *chatroom.AccessError) *eventpb.EventError {
	if err == nil {
		return nil
	}
	return &eventpb.EventError{
		Code:      err.Code,
		Message:   err.Message,
		Retryable: err.Retryable,
	}
}

func (h *PeerConn) currentInputWorkspace(ctx context.Context) (string, error) {
	if h == nil || h.Service == nil || h.Service.manager == nil || h.Service.manager.PeerRun == nil || h.Conn == nil {
		return "", errors.New("gizclaw: Peer run state is unavailable")
	}
	run, err := h.Service.manager.PeerRun.GetRunAgent(ctx, h.Conn.PublicKey())
	if err != nil {
		return "", err
	}
	if run.Active != nil {
		return strings.TrimSpace(run.Active.WorkspaceName), nil
	}
	if run.Pending != nil {
		return strings.TrimSpace(run.Pending.WorkspaceName), nil
	}
	return "", nil
}

func (h *PeerConn) inputStreamDenied(streamID string) bool {
	h.chatroomAccessMu.Lock()
	defer h.chatroomAccessMu.Unlock()
	if streamID == "audio" && h.deniedAudioInput {
		return true
	}
	_, denied := h.deniedInputStreams[streamID]
	return denied
}

func (h *PeerConn) markDeniedInputStream(streamID string, kind eventpb.StreamKind) bool {
	h.chatroomAccessMu.Lock()
	defer h.chatroomAccessMu.Unlock()
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
		h.acceptedAudioInput = false
		h.acceptedAudioStream = ""
		h.acceptedAudioChatroom = false
		h.acceptedAudioWorkspace = ""
	}
	return accepted
}

func (h *PeerConn) clearDeniedInputStream(streamID string, kind eventpb.StreamKind) {
	h.chatroomAccessMu.Lock()
	defer h.chatroomAccessMu.Unlock()
	delete(h.deniedInputStreams, streamID)
	if kind == eventpb.StreamKind_STREAM_KIND_AUDIO || streamID == h.deniedAudioStream {
		h.deniedAudioInput = false
		h.deniedAudioStream = ""
	}
}

func (h *PeerConn) acceptInputEvent(
	event *eventpb.PeerEvent,
	streamID string,
	workspaceName string,
	isChatroom bool,
) {
	if event == nil {
		return
	}
	h.chatroomAccessMu.Lock()
	defer h.chatroomAccessMu.Unlock()
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
			h.acceptedAudioChatroom = isChatroom
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
			h.acceptedAudioChatroom = false
			h.acceptedAudioWorkspace = ""
		}
	}
}

func (h *PeerConn) audioInputAccepted() bool {
	h.chatroomAccessMu.Lock()
	defer h.chatroomAccessMu.Unlock()
	return h.acceptedAudioInput && !h.deniedAudioInput
}

func (h *PeerConn) authorizeChatroomAudioPacket(ctx context.Context) (bool, error) {
	if h == nil {
		return false, nil
	}
	h.chatroomAccessMu.Lock()
	accepted := h.acceptedAudioInput && !h.deniedAudioInput
	streamID := h.acceptedAudioStream
	workspaceName := h.acceptedAudioWorkspace
	h.chatroomAccessMu.Unlock()
	if !accepted {
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
	h.chatroomAccessMu.Lock()
	if h.acceptedAudioInput &&
		h.acceptedAudioStream == streamID &&
		h.acceptedAudioWorkspace == workspaceName {
		delete(h.acceptedInputStreams, streamID)
		h.acceptedAudioInput = false
		h.acceptedAudioStream = ""
		h.acceptedAudioChatroom = false
		h.acceptedAudioWorkspace = ""
	}
	h.chatroomAccessMu.Unlock()
	return false, nil
}

func (h *PeerConn) observePeerEvent(event *eventpb.PeerEvent) {
	if h == nil || event == nil || h.Conn == nil {
		return
	}
	h.chatroomAccessMu.Lock()
	if !h.acceptedAudioInput || !h.acceptedAudioChatroom || h.deniedAudioInput {
		h.chatroomAccessMu.Unlock()
		return
	}
	streamID := h.acceptedAudioStream
	workspaceName := h.acceptedAudioWorkspace
	h.chatroomAccessMu.Unlock()

	var denial *chatroom.AccessError
	switch event.Type {
	case eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED:
		update := event.GetFriendRelationshipUpdated()
		if update.GetWorkspaceName() == workspaceName &&
			update.GetChange() == eventpb.FriendRelationshipChange_FRIEND_RELATIONSHIP_CHANGE_DELETED {
			denial = chatroom.FriendRemovedError()
		}
	case eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED:
		update := event.GetFriendGroupUpdated()
		if update.GetWorkspaceName() != workspaceName {
			break
		}
		switch update.GetChange() {
		case eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_DELETED:
			denial = chatroom.GroupDeletedError()
		case eventpb.FriendGroupChange_FRIEND_GROUP_CHANGE_MEMBER_REMOVED:
			if update.GetAffectedPeerPublicKey() == h.Conn.PublicKey().String() {
				denial = chatroom.MemberRemovedError()
			}
		}
	}
	if denial == nil {
		return
	}
	h.rejectChatroomAudioFromInvalidation(streamID, denial)
}

func (h *PeerConn) rejectChatroomAudioFromInvalidation(
	streamID string,
	denial *chatroom.AccessError,
) {
	event := audioEndEvent(streamID)
	if !h.markDeniedInputStream(streamID, event.StreamKindValue()) {
		return
	}
	if err := h.abortAgentInputTurn(); err != nil {
		slog.Warn("gizclaw: abort invalid Chatroom audio turn", "error", err)
	}
	if h.events != nil {
		_ = h.events.Notify(&eventpb.PeerEvent{
			Version: eventpb.Version,
			Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
			Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
				StreamId: streamID,
				Kind:     eventpb.StreamKind_STREAM_KIND_AUDIO,
				Label:    "assistant",
				Error:    chatroomEventError(denial),
			}},
		})
	}
}

func audioEndEvent(streamID string) *eventpb.PeerEvent {
	return &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
			StreamId: streamID,
			Kind:     eventpb.StreamKind_STREAM_KIND_AUDIO,
		}},
	}
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
			authorized, err := h.authorizeChatroomAudioPacket(context.Background())
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
		return nil
	}
	if host == nil {
		return peerConnInputPusher{peer: h, input: input}.Push(ctx, chunk)
	}
	inputPusher := peerConnInputPusher{peer: h, input: input}
	revision, pushed, err := host.PushInput(ctx, inputPusher, chunk)
	if !pushed {
		return err
	}
	if !errors.Is(err, agenthost.ErrNoActiveInput) {
		return err
	}
	reloaded, err := host.ReloadAndPushInputIfCurrentRevision(ctx, revision, inputPusher, chunk)
	if !reloaded {
		return err
	}
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
