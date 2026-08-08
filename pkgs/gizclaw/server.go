package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/memorylayout"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/providertenants"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/memorystore"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/resourcemanager"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/giztools"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/jmoiron/sqlx"
	"golang.org/x/sync/errgroup"
)

// Server holds peer transport configuration. Per-stream protocol handling can be
// extended later.
//
// Set peer storage config on the struct, then call ListenAndServe.
// Internal runtime state is built automatically on first ListenAndServe.
type Server struct {
	LocalStatic giznet.KeyPair

	SecurityPolicy        giznet.SecurityPolicy
	PeerListeners         []giznet.Listener
	PeerListenerFactories []PeerListenerFactory

	PeerStore              kv.Store
	CredentialStore        kv.Store
	FirmwareStore          kv.Store
	RuntimeProfileStore    kv.Store
	AgentHostStore         objectstore.ObjectStore
	ProviderTenantStore    kv.Store
	ModelStore             kv.Store
	VoiceStore             kv.Store
	MemoryLayoutStore      kv.Store
	WorkspaceStore         kv.Store
	WorkflowStore          kv.Store
	ToolStore              kv.Store
	PublicLoginStore       kv.Store
	ContactStore           kv.Store
	FriendStore            kv.Store
	FriendGroupStore       kv.Store
	GameplayStore          kv.Store
	GameplayAssets         objectstore.ObjectStore
	WorkspaceAssets        objectstore.ObjectStore
	GameplayDB             *sqlx.DB
	MetricsStore           metrics.Store
	PendingDeletionConfig  pendingdeletion.Config
	ServerLogQuery         ServerLogQueryService
	FlowcraftHistory       logstore.MutableStore
	FlowcraftState         kv.Store
	MemoryRoot             string
	SpeechLimits           SpeechLimits
	ClientToolTimeout      time.Duration
	ToolHTTPExecutor       giztools.HTTPExecutor
	BuildCommit            string
	PublicEndpoint         string
	PublicICETCP           bool
	PublicLoginAuthorizer  publiclogin.SessionAuthorizer
	ICEServers             []gizwebrtc.ICEServer
	WebRTCSignalingHandler http.Handler
	EdgeNodes              []giznet.PublicKey

	manager                  *Manager
	peerService              *PeerService
	sessions                 *publiclogin.SessionManager
	listenerMu               sync.RWMutex
	listeners                []giznet.Listener
	closed                   bool
	httpHandler              http.Handler
	driveFactStop            context.CancelFunc
	driveFactDone            <-chan struct{}
	workspaceRewardStop      context.CancelFunc
	workspaceRewardDone      <-chan struct{}
	pendingDeletionProcessor *pendingdeletion.Processor
}

type PeerListenerOptions struct {
	KeyPair          *giznet.KeyPair
	SecurityPolicy   giznet.SecurityPolicy
	PeerEventHandler giznet.PeerEventHandler
}

type PeerListenerFactory func(PeerListenerOptions) (giznet.Listener, error)

// ServeHTTP exposes server-public APIs over ordinary HTTP.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.httpHandler.ServeHTTP(w, r)
}

// Listen initializes the server runtime and binds the UDP peer listener.
func (s *Server) Listen() error {
	if s == nil {
		return errors.New("gizclaw: nil server")
	}
	s.listenerMu.RLock()
	if len(s.listeners) > 0 {
		s.listenerMu.RUnlock()
		return nil
	}
	s.listenerMu.RUnlock()
	if err := s.init(); err != nil {
		return err
	}
	listeners := append([]giznet.Listener(nil), s.PeerListeners...)
	opts := PeerListenerOptions{
		KeyPair:          &s.LocalStatic,
		SecurityPolicy:   (*ServerSecurityPolicy)(s),
		PeerEventHandler: (*serverPeerEventHandler)(s),
	}
	for _, factory := range s.PeerListenerFactories {
		if factory == nil {
			continue
		}
		listener, err := factory(opts)
		if err != nil {
			return err
		}
		listeners = append(listeners, listener)
	}
	if len(listeners) == 0 {
		return giznet.ErrNilListener
	}
	s.listenerMu.Lock()
	s.listeners = listeners
	s.closed = false
	s.listenerMu.Unlock()
	s.startDriveFactDispatcher()
	if err := s.startWorkspaceRewardDispatcher(); err != nil {
		_ = s.Close()
		return err
	}
	if s.pendingDeletionProcessor != nil {
		s.pendingDeletionProcessor.Start(context.Background())
	}
	return nil
}

// Serve blocks serving accepted peer connections from listeners created by Listen.
func (s *Server) Serve() error {
	s.listenerMu.RLock()
	listeners := append([]giznet.Listener(nil), s.listeners...)
	closed := s.closed
	s.listenerMu.RUnlock()
	if len(listeners) == 0 {
		if closed {
			return nil
		}
		return giznet.ErrNilListener
	}
	var g errgroup.Group
	for _, listener := range listeners {
		l := listener
		g.Go(func() error {
			return s.servePeerListener(l)
		})
	}
	return g.Wait()
}

func (s *Server) servePeerListener(l giznet.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, giznet.ErrClosed) {
				return nil
			}
			_ = l.Close()
			return err
		}
		svc := s.peerService
		if svc == nil {
			svc = &PeerService{}
		}
		host := &PeerConn{
			Conn:            conn,
			Service:         svc,
			ServerPublicKey: s.LocalStatic.Public,
		}
		go func() {
			_ = host.serve()
		}()
	}
}

type serverPeerEventHandler Server

var _ giznet.PeerEventHandler = (*serverPeerEventHandler)(nil)

func (h *serverPeerEventHandler) HandlePeerEvent(ev giznet.PeerEvent) {
	// Transport-level events do not identify the specific connection instance.
	// Active peer state is therefore owned by PeerService's identity-aware
	// registration/teardown path.
	_ = ev
}

// PublicKey returns the configured server public key.
func (s *Server) PublicKey() giznet.PublicKey {
	if s == nil {
		return giznet.PublicKey{}
	}
	return s.LocalStatic.Public
}

// PeerService returns the initialized peer service bundle, or nil before runtime initialization.
func (s *Server) PeerService() *PeerService {
	if s == nil {
		return nil
	}
	return s.peerService
}

// Manager returns the initialized peer manager, or nil before runtime initialization.
func (s *Server) Manager() *Manager {
	if s == nil {
		return nil
	}
	return s.manager
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	var errs []error
	s.listenerMu.Lock()
	listeners := append([]giznet.Listener(nil), s.listeners...)
	hadListeners := len(s.listeners) > 0
	s.listeners = nil
	if hadListeners {
		s.closed = true
	}
	s.listenerMu.Unlock()
	for _, listener := range listeners {
		if listener != nil {
			errs = append(errs, listener.Close())
		}
	}
	if s.driveFactStop != nil {
		s.driveFactStop()
		s.driveFactStop = nil
	}
	if s.driveFactDone != nil {
		<-s.driveFactDone
		s.driveFactDone = nil
	}
	if s.workspaceRewardStop != nil {
		s.workspaceRewardStop()
		s.workspaceRewardStop = nil
	}
	if s.workspaceRewardDone != nil {
		<-s.workspaceRewardDone
		s.workspaceRewardDone = nil
	}
	if s.pendingDeletionProcessor != nil {
		s.pendingDeletionProcessor.Close()
		s.pendingDeletionProcessor = nil
	}
	if s.manager != nil && s.manager.MemoryStores != nil {
		errs = append(errs, s.manager.MemoryStores.Close())
	}
	return errors.Join(errs...)
}

func (s *Server) startDriveFactDispatcher() {
	if s == nil || s.driveFactStop != nil || s.manager == nil || s.manager.Gameplay == nil ||
		s.manager.Gameplay.DB == nil || s.manager.Gameplay.DriveFacts == nil {
		return
	}
	stop, done := s.manager.Gameplay.StartDriveFactDispatcher(context.Background())
	s.driveFactStop = stop
	s.driveFactDone = done
}

func (s *Server) startWorkspaceRewardDispatcher() error {
	if s == nil || s.workspaceRewardStop != nil || s.manager == nil ||
		s.manager.Gameplay == nil || s.manager.Gameplay.DB == nil ||
		s.manager.Gameplay.WorkspaceRewards == nil {
		return nil
	}
	stop, done, err := s.manager.Gameplay.StartWorkspaceRewardDispatcher(context.Background())
	if err != nil {
		return fmt.Errorf("gizclaw: start Workspace reward dispatcher: %w", err)
	}
	s.workspaceRewardStop = stop
	s.workspaceRewardDone = done
	return nil
}

func (s *Server) init() error {
	if s == nil {
		return errors.New("gizclaw: nil server")
	}
	switch {
	case s.LocalStatic.Private.IsZero():
		return errors.New("gizclaw: empty local static private key")
	case s.PeerStore == nil:
		return errors.New("gizclaw: nil peer store")
	}
	for _, required := range []struct {
		name  string
		store kv.Store
	}{
		{"public login", s.PublicLoginStore}, {"credential", s.CredentialStore},
		{"firmware", s.FirmwareStore}, {"runtime profile", s.RuntimeProfileStore},
		{"model", s.ModelStore}, {"voice", s.VoiceStore}, {"memory layout", s.MemoryLayoutStore},
		{"provider tenant", s.ProviderTenantStore}, {"workflow", s.WorkflowStore},
		{"workspace", s.WorkspaceStore},
		{"tool", s.ToolStore}, {"contact", s.ContactStore},
		{"friend", s.FriendStore}, {"friend group", s.FriendGroupStore}, {"gameplay", s.GameplayStore},
	} {
		if required.store == nil {
			return fmt.Errorf("gizclaw: nil %s store", required.name)
		}
	}
	if s.WorkspaceAssets == nil {
		return errors.New("gizclaw: nil workspace assets store")
	}
	if s.GameplayAssets == nil {
		return errors.New("gizclaw: nil gameplay assets store")
	}
	if s.GameplayDB == nil {
		return errors.New("gizclaw: nil gameplay database store")
	}

	peerStore := kv.Prefixed(s.PeerStore, kv.Key{"records"})
	peerRouteStore := kv.Prefixed(s.PeerStore, kv.Key{"routes"})
	peerRunStore := kv.Prefixed(s.PeerStore, kv.Key{"runs"})
	credentialStore := s.CredentialStore
	firmwareStore := s.FirmwareStore
	runtimeProfileStore := s.RuntimeProfileStore
	modelStore := s.ModelStore
	voiceStore := s.VoiceStore
	memoryLayoutStore := s.MemoryLayoutStore
	workspaceStore := s.WorkspaceStore
	workflowStore := s.WorkflowStore
	toolStore := s.ToolStore
	publicLoginStore := s.PublicLoginStore
	contactStore := s.ContactStore
	friendInviteTokenStore := kv.Prefixed(s.FriendStore, kv.Key{"invite-tokens"})
	friendStore := kv.Prefixed(s.FriendStore, kv.Key{"friends"})
	friendGroupStore := kv.Prefixed(s.FriendGroupStore, kv.Key{"groups"})
	friendGroupInviteTokenStore := kv.Prefixed(s.FriendGroupStore, kv.Key{"invite-tokens"})
	friendGroupMemberStore := kv.Prefixed(s.FriendGroupStore, kv.Key{"members"})
	friendGroupBelongStore := kv.Prefixed(s.FriendGroupStore, kv.Key{"belongs"})
	friendGroupRelationshipStore, friendGroupRelationshipPrefixes, ok := kv.SharedAtomicStore(
		friendGroupStore,
		friendGroupInviteTokenStore,
		friendGroupMemberStore,
		friendGroupBelongStore,
	)
	if !ok {
		return errors.New("gizclaw: friend group relationship stores must share one atomic transaction boundary")
	}
	friendGroupPrefix := friendGroupRelationshipPrefixes[0]
	friendGroupInvitePrefix := friendGroupRelationshipPrefixes[1]
	friendGroupMemberPrefix := friendGroupRelationshipPrefixes[2]
	friendGroupBelongPrefix := friendGroupRelationshipPrefixes[3]
	petDefStore := kv.Prefixed(s.GameplayStore, kv.Key{"pet-defs"})
	badgeDefStore := kv.Prefixed(s.GameplayStore, kv.Key{"badge-defs"})
	gameDefStore := kv.Prefixed(s.GameplayStore, kv.Key{"game-defs"})
	if !kv.SupportsCreateIfAbsent(peerStore) {
		return fmt.Errorf("gizclaw: peer store: %w", kv.ErrCreateIfAbsentUnsupported)
	}
	if !kv.SupportsCompareAndMutate(peerStore) {
		return fmt.Errorf("gizclaw: peer store: %w", kv.ErrCompareAndMutateUnsupported)
	}
	if !kv.SupportsCreateIfAbsent(workspaceStore) {
		return fmt.Errorf("gizclaw: workspace store: %w", kv.ErrCreateIfAbsentUnsupported)
	}
	if !kv.SupportsCompareAndMutate(workspaceStore) {
		return fmt.Errorf("gizclaw: workspace store: %w", kv.ErrCompareAndMutateUnsupported)
	}
	if !kv.SupportsCreateIfAbsent(friendStore) {
		return fmt.Errorf("gizclaw: friend store: %w", kv.ErrCreateIfAbsentUnsupported)
	}
	if !kv.SupportsCompareAndMutate(friendStore) {
		return fmt.Errorf(
			"gizclaw: friend store: %w",
			kv.ErrCompareAndMutateUnsupported,
		)
	}
	if !kv.SupportsCreateIfAbsent(friendGroupRelationshipStore) {
		return fmt.Errorf(
			"gizclaw: friend group relationship store: %w",
			kv.ErrCreateIfAbsentUnsupported,
		)
	}
	if !kv.SupportsCompareAndMutate(friendGroupRelationshipStore) {
		return fmt.Errorf(
			"gizclaw: friend group relationship store: %w",
			kv.ErrCompareAndMutateUnsupported,
		)
	}

	publicLoginServer := publiclogin.NewServer(&s.LocalStatic, publicLoginStore)
	peersServer := &peer.Server{
		Store:           peerStore,
		BuildCommit:     s.BuildCommit,
		Endpoint:        s.PublicEndpoint,
		ServerPublicKey: s.LocalStatic.Public,
		SignalingPath:   gizwebrtc.SignalingPath,
		ICETCP:          s.PublicICETCP,
		ICEServers:      s.ICEServers,
	}
	manager := NewManager(peersServer)
	peerAvailability := func(ctx context.Context, publicKey giznet.PublicKey) error {
		err := peersServer.EnsureAvailable(ctx, publicKey)
		if errors.Is(err, peer.ErrPeerNotFound) {
			return nil
		}
		if errors.Is(err, peer.ErrPeerPendingDeletion) {
			return publiclogin.ErrPeerDeletionPending
		}
		if errors.Is(err, peer.ErrPeerDeleted) {
			return publiclogin.ErrPeerDeleted
		}
		return err
	}
	publicLoginServer.SessionAuthorizer = func(ctx context.Context, publicKey giznet.PublicKey) error {
		if err := peerAvailability(ctx, publicKey); err != nil {
			return err
		}
		if s.PublicLoginAuthorizer != nil {
			return s.PublicLoginAuthorizer(ctx, publicKey)
		}
		return nil
	}
	sessions := publicLoginServer.SessionManager()
	notifyPeer := func(_ context.Context, publicKey string, event *eventpb.PeerEvent) {
		var recipient giznet.PublicKey
		if err := recipient.UnmarshalText([]byte(publicKey)); err != nil || recipient.IsZero() {
			return
		}
		_ = manager.BroadcastPeerEvent(recipient, event)
	}
	manager.FlowcraftHistory = s.FlowcraftHistory
	manager.FlowcraftState = s.FlowcraftState
	manager.MemoryRoot = s.MemoryRoot
	manager.MemoryStores = memorystore.NewRegistry()
	manager.SpeechLimits = s.SpeechLimits
	manager.PeerRoutes = &peerroute.Server{
		Store:           peerRouteStore,
		Peers:           peersServer,
		ServerPublicKey: s.LocalStatic.Public,
		ServerEndpoint:  s.PublicEndpoint,
	}
	manager.PeerRun = &peerrun.Server{Store: peerRunStore}
	peersServer.PeerManager = manager
	if err := peersServer.BootstrapEdgeNodes(context.Background(), s.EdgeNodes); err != nil {
		return err
	}

	modelServer := &model.Server{Store: modelStore}
	voiceServer := &voice.Server{Store: voiceStore}
	memoryLayoutServer := &memorylayout.Server{Store: memoryLayoutStore}
	workflowServer := &workflow.Server{Store: workflowStore}
	workspaceServer := &workspace.Server{
		Store: workspaceStore, Workflows: workflowServer,
		Models: modelServer, Voices: voiceServer, Assets: s.WorkspaceAssets,
		PeerAvailability: func(ctx context.Context, publicKey string) error {
			key, err := parsePeerPublicKey(publicKey)
			if err != nil {
				return err
			}
			err = peersServer.EnsureAvailable(ctx, key)
			switch {
			case errors.Is(err, peer.ErrPeerPendingDeletion):
				return workspace.ErrPeerPendingDeletion
			case errors.Is(err, peer.ErrPeerDeleted):
				return workspace.ErrPeerDeleted
			default:
				return err
			}
		},
	}
	if s.AgentHostStore != nil {
		workspaceServer.RuntimeStore = workspace.NewObjectRuntimeStore(s.AgentHostStore)
	}
	credentialServer := &credential.Server{Store: credentialStore}
	firmwareServer := &firmware.Server{Store: firmwareStore}
	runtimeProfileServer := &runtimeprofile.Server{Store: runtimeProfileStore}
	publicLoginServer.RegistrationResolver = runtimeProfileServer.ResolveRegistration
	publicLoginServer.OwnerProfileBinder = runtimeProfileServer.BindOwnerProfileAndCommit
	toolServer := &toolkit.Server{Store: toolStore}
	contactServer := &contact.Server{
		Store: contactStore,
		PeerAvailability: func(ctx context.Context, publicKey string) error {
			key, err := parsePeerPublicKey(publicKey)
			if err != nil {
				return err
			}
			err = peersServer.EnsureAvailable(ctx, key)
			switch {
			case errors.Is(err, peer.ErrPeerPendingDeletion):
				return contact.ErrPeerPendingDeletion
			case errors.Is(err, peer.ErrPeerDeleted):
				return contact.ErrPeerDeleted
			default:
				return err
			}
		},
	}
	friendServer := &friend.Server{
		InviteTokens:           friendInviteTokenStore,
		Friends:                friendStore,
		Workspaces:             workspaceServer,
		Profiles:               peersServer,
		RuntimeProfileForOwner: manager.runtimeProfileForOwner,
		NotifyPeer:             notifyPeer,
		PeerAvailability: func(ctx context.Context, publicKey string) error {
			key, err := parsePeerPublicKey(publicKey)
			if err != nil {
				return err
			}
			return peersServer.EnsureAvailable(ctx, key)
		},
	}
	friendGroupServer := &friendgroup.Server{
		Groups:                   friendGroupStore,
		InviteTokens:             friendGroupInviteTokenStore,
		Members:                  friendGroupMemberStore,
		Belongs:                  friendGroupBelongStore,
		RelationshipStore:        friendGroupRelationshipStore,
		GroupRelationshipPrefix:  friendGroupPrefix,
		InviteRelationshipPrefix: friendGroupInvitePrefix,
		MemberRelationshipPrefix: friendGroupMemberPrefix,
		BelongRelationshipPrefix: friendGroupBelongPrefix,
		Workspaces:               workspaceServer,
		RuntimeProfileForOwner:   manager.runtimeProfileForOwner,
		NotifyPeer:               notifyPeer,
		PeerAvailability: func(ctx context.Context, publicKey string) error {
			key, err := parsePeerPublicKey(publicKey)
			if err != nil {
				return err
			}
			return peersServer.EnsureAvailable(ctx, key)
		},
	}
	providerTenantsServer := &providertenants.Server{
		Store:       s.ProviderTenantStore,
		Voices:      voiceServer,
		Credentials: credentialServer,
	}
	gameplayCatalog := &gameplay.Catalog{
		PetDefs:   petDefStore,
		BadgeDefs: badgeDefStore,
		GameDefs:  gameDefStore,
		Assets:    s.GameplayAssets,
	}
	gameplayRuntime := &gameplay.Runtime{
		DB:         s.GameplayDB,
		Catalog:    gameplayCatalog,
		Workflows:  workflowServer,
		Workspaces: workspaceServer,
		PeerAvailability: func(ctx context.Context, publicKey string) error {
			key, err := parsePeerPublicKey(publicKey)
			if err != nil {
				return err
			}
			return peersServer.EnsureAvailable(ctx, key)
		},
	}
	if s.GameplayDB != nil {
		if err := gameplayRuntime.Migration(context.Background()); err != nil {
			return err
		}
	}
	pendingDeletionRegistry := pendingdeletion.NewRegistry()
	workspacePendingDeletionSource := workspace.NewPendingDeletionSource(workspaceStore)
	var gameplayWorkspaceCleanup workspace.GameplayWorkspaceCleanup
	if s.GameplayDB != nil {
		gameplayWorkspaceCleanup = gameplayRuntime
	}
	if err := pendingDeletionRegistry.Register(
		workspacePendingDeletionSource,
		workspace.DeletionHandler{
			Server:   workspaceServer,
			Source:   workspacePendingDeletionSource,
			Quiescer: manager,
			Gameplay: gameplayWorkspaceCleanup,
		},
	); err != nil {
		return fmt.Errorf("gizclaw: register Workspace pending deletion: %w", err)
	}
	friendGroupPendingDeletionSource := friendgroup.NewPendingDeletionSource(friendGroupRelationshipStore)
	if err := pendingDeletionRegistry.Register(
		friendGroupPendingDeletionSource,
		friendgroup.DeletionHandler{
			Server: friendGroupServer,
			Source: friendGroupPendingDeletionSource,
		},
	); err != nil {
		return fmt.Errorf("gizclaw: register Friend Group pending deletion: %w", err)
	}
	if s.GameplayDB != nil {
		if err := pendingDeletionRegistry.Register(
			gameplay.PendingDeletionSource{DB: s.GameplayDB},
			gameplay.PetDeletionHandler{DB: s.GameplayDB},
		); err != nil {
			return fmt.Errorf("gizclaw: register Gameplay pending deletion: %w", err)
		}
	}
	peerPendingDeletionSource := peer.PendingDeletionSource(peerStore)
	if err := pendingDeletionRegistry.Register(
		peerPendingDeletionSource,
		peer.DeletionHandler{
			Server: peersServer, Source: peerPendingDeletionSource,
			Social:     social.PeerRetirement{Contacts: contactServer, Friends: friendServer, FriendGroups: friendGroupServer},
			Workspaces: workspaceServer, Gameplay: gameplay.PeerRetirement{Runtime: gameplayRuntime},
			Sessions: sessions, RuntimeProfiles: runtimeProfileServer, Quiescer: manager,
			WorkspaceLookup: workspacePendingDeletionSource, FriendGroupLookup: friendGroupPendingDeletionSource,
		},
	); err != nil {
		return fmt.Errorf("gizclaw: register Peer pending deletion: %w", err)
	}
	pendingDeletionConfig := s.PendingDeletionConfig
	if pendingDeletionConfig == (pendingdeletion.Config{}) {
		pendingDeletionConfig = pendingdeletion.DefaultConfig()
	}
	pendingDeletionProcessor, err := pendingdeletion.NewProcessor(
		pendingDeletionRegistry,
		pendingDeletionConfig,
		s.MetricsStore,
	)
	if err != nil {
		return fmt.Errorf("gizclaw: pending deletion processor: %w", err)
	}
	gameplayRuntime.PendingDeletionWake = pendingDeletionProcessor.Wake
	pendingDeletionAdmin := pendingdeletion.NewAdmin(pendingDeletionRegistry, pendingDeletionProcessor.Wake)
	s.pendingDeletionProcessor = pendingDeletionProcessor
	manager.Tools = toolServer
	manager.ToolBuilder = &toolkit.Builder{Tools: toolServer}
	agentResolver := agenthost.ServiceResolver{
		Workspaces:             workspaceServer,
		Workflows:              workflowServer,
		MemoryLayouts:          memoryLayoutServer,
		RuntimeProfileForOwner: manager.runtimeProfileForOwner,
		ToolBuilder:            manager.ToolBuilder,
		ToolCredentials:        credentialServer,
		ClientToolTimeout:      s.ClientToolTimeout,
		HTTPTools:              s.ToolHTTPExecutor,
	}
	manager.AgentHost = agenthost.New(agentResolver)
	gameplayRuntime.DriveFacts = &driveWorkspaceMemory{
		resolver: agentResolver, stores: manager.MemoryStores,
		serverRoot: s.MemoryRoot, genXForOwner: manager.ownerGenX,
	}
	manager.Workspaces = workspaceServer
	manager.Workflows = workflowServer
	manager.Firmwares = firmwareServer
	manager.RuntimeProfiles = runtimeProfileServer
	manager.Models = modelServer
	manager.Credentials = credentialServer
	manager.Voices = voiceServer
	manager.Contacts = contactServer
	manager.Friends = friendServer
	manager.FriendGroups = friendGroupServer
	if err := friendServer.ReconcileCreationIntents(context.Background()); err != nil {
		return fmt.Errorf("gizclaw: reconcile Friend creation intents: %w", err)
	}
	if err := friendServer.ReconcileRetirementIntents(context.Background()); err != nil {
		return fmt.Errorf("gizclaw: reconcile Friend retirement intents: %w", err)
	}
	if err := friendGroupServer.ReconcileRetirementIntents(context.Background()); err != nil {
		return fmt.Errorf("gizclaw: reconcile Friend Group retirement intents: %w", err)
	}
	manager.ProviderTenants = providerTenantsServer
	manager.Gameplay = gameplayRuntime
	gameplayRuntime.WorkspaceRewards = &workspaceRewardEnvironment{
		manager: manager, workspaces: workspaceServer,
	}
	manager.Metrics = s.MetricsStore
	resourceManager := resourcemanager.New(resourcemanager.Services{
		Credentials:     credentialServer,
		Firmwares:       firmwareServer,
		Peers:           peersServer,
		Models:          modelServer,
		ProviderTenants: providerTenantsServer,
		Voices:          voiceServer,
		Workspaces:      workspaceServer,
		Workflows:       workflowServer,
		MemoryLayouts:   memoryLayoutServer,
		Contacts:        contactServer,
		Friends:         friendServer,
		FriendGroups:    friendGroupServer,
		GameplayCatalog: gameplayCatalog,
		Tools:           toolServer,
		RuntimeProfiles: runtimeProfileServer,
	})
	runtimeProfileServer.ResolveResource = resourceManager.Get

	s.manager = manager
	s.peerService = &PeerService{
		manager:  manager,
		sessions: sessions,
		admin: &adminService{
			Peers:                       peersServer,
			CredentialAdminService:      credentialServer,
			FirmwareAdminService:        firmwareServer,
			AdminService:                runtimeProfileServer,
			PeerAdminService:            peersServer,
			ModelAdminService:           modelServer,
			VoiceAdminService:           voiceServer,
			MemoryLayoutAdminService:    memoryLayoutServer,
			ProviderTenantsAdminService: providerTenantsServer,
			WorkspaceAdminService:       workspaceServer,
			WorkspaceIconAdminService:   workspaceServer,
			WorkflowAdminService:        workflowServer,
			Contacts:                    contactServer,
			Friends:                     friendServer,
			FriendGroups:                friendGroupServer,
			CatalogAdminService:         gameplayCatalog,
			GameDefIconAdminService:     gameplayCatalog,
			Gameplay:                    gameplayRuntime,
			ResourceManager:             resourceManager,
			ServerLogs:                  s.ServerLogQuery,
			PeerTelemetry:               &peertelemetry.AdminService{Metrics: s.MetricsStore},
			PendingDeletions:            pendingDeletionAdmin,
		},
		public: &peerHTTP{
			PeerHTTPService:  peersServer,
			Self:             peersServer,
			Status:           manager.PeerRun,
			Telemetry:        &peertelemetry.AdminService{Metrics: s.MetricsStore},
			Contacts:         contactServer,
			PeerHTTP:         publicLoginServer,
			PeerAvailability: peersServer.EnsureAvailable,
			WebRTCSignalingHandler: func() http.Handler {
				return s.WebRTCSignalingHandler
			},
		},
	}
	s.sessions = sessions
	mux := http.NewServeMux()
	publicHandler := s.peerService.publicHTTPHandler(sessions)
	mux.Handle("/login", publicHandler)
	mux.Handle("/server-info", publicHandler)
	mux.Handle(gizwebrtc.SignalingPath, publicHandler)
	mux.Handle("/me", publicHandler)
	mux.Handle("/me/status", publicHandler)
	mux.Handle("/me/runtime", publicHandler)
	mux.Handle("/me/side-control/", publicHandler)
	mux.Handle("/side-control/", publicHandler)
	mux.Handle("/openai/v1/", s.peerOpenAIHTTPHandler(sessions))
	s.httpHandler = observeHTTPHandler(mux, httpObservationOptions{surface: observability.SurfaceServerPublic})
	return nil
}

func parsePeerPublicKey(value string) (giznet.PublicKey, error) {
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(value)); err != nil || publicKey.IsZero() || publicKey.String() != value {
		return giznet.PublicKey{}, fmt.Errorf("gizclaw: invalid canonical Peer public key")
	}
	return publicKey, nil
}
