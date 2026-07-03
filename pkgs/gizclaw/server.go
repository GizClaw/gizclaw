package gizclaw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/providertenants"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay/badge"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay/pet"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay/petspecies"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay/reward"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay/wallet"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/resourcemanager"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
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

	PeerStore                    kv.Store
	PeerRunStore                 kv.Store
	CredentialStore              kv.Store
	FirmwareStore                kv.Store
	FirmwareAssets               objectstore.ObjectStore
	AgentHostStore               objectstore.ObjectStore
	MiniMaxCredentialStore       kv.Store
	MiniMaxTenantStore           kv.Store
	VolcTenantStore              kv.Store
	ModelStore                   kv.Store
	VoiceStore                   kv.Store
	WorkspaceStore               kv.Store
	WorkflowStore                kv.Store
	PublicLoginStore             kv.Store
	PetStore                     kv.Store
	PetSpeciesStore              kv.Store
	PetSpeciesAssets             objectstore.ObjectStore
	BadgeStore                   kv.Store
	BadgeAssets                  objectstore.ObjectStore
	WalletDB                     *sql.DB
	RewardStore                  kv.Store
	ContactStore                 kv.Store
	FriendInviteTokenStore       kv.Store
	FriendStore                  kv.Store
	FriendGroupStore             kv.Store
	FriendGroupInviteTokenStore  kv.Store
	FriendGroupMemberStore       kv.Store
	FriendGroupBelongStore       kv.Store
	FriendGroupMessageStore      kv.Store
	FriendGroupMessageAssets     objectstore.ObjectStore
	FriendGroupMessageDefaultTTL time.Duration
	FriendGroupMessageMaxTTL     time.Duration
	FriendGroupMessageCleanup    time.Duration
	FriendGroupMessageMaxBytes   int64
	RewardClaimGenerator         string
	RewardClaimCooldown          time.Duration
	PetActionGenerator           string
	PetAdoptPointCost            int64
	BuildCommit                  string
	PublicEndpoint               string
	ACLDB                        *sql.DB
	WebRTCSignalingHandler       http.Handler

	manager     *Manager
	peerService *PeerService
	sessions    *publiclogin.SessionManager
	listenerMu  sync.RWMutex
	listeners   []giznet.Listener
	closed      bool
	httpHandler http.Handler
	cleanupStop context.CancelFunc
	cleanupDone <-chan struct{}
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
	s.startCleanup()
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
			Conn:    conn,
			Service: svc,
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
	if s.cleanupStop != nil {
		s.cleanupStop()
		s.cleanupStop = nil
	}
	if s.cleanupDone != nil {
		<-s.cleanupDone
		s.cleanupDone = nil
	}
	return errors.Join(errs...)
}

func (s *Server) startCleanup() {
	if s == nil || s.cleanupStop != nil || s.manager == nil || s.manager.FriendGroups == nil || s.manager.FriendGroups.Messages == nil {
		return
	}
	interval := s.FriendGroupMessageCleanup
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.cleanupStop = cancel
	s.cleanupDone = done
	friendGroups := s.manager.FriendGroups
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = friendGroups.CleanupExpiredFriendGroupMessages(context.WithoutCancel(ctx))
			}
		}
	}()
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

	legacySharedStore := s.CredentialStore == nil &&
		s.FirmwareStore == nil &&
		s.AgentHostStore == nil &&
		s.MiniMaxTenantStore == nil &&
		s.VolcTenantStore == nil &&
		s.ModelStore == nil &&
		s.VoiceStore == nil &&
		s.MiniMaxCredentialStore == nil &&
		s.WorkspaceStore == nil &&
		s.WorkflowStore == nil &&
		s.PeerRunStore == nil &&
		s.PublicLoginStore == nil &&
		s.PetStore == nil &&
		s.PetSpeciesStore == nil &&
		s.PetSpeciesAssets == nil &&
		s.BadgeStore == nil &&
		s.BadgeAssets == nil &&
		s.WalletDB == nil &&
		s.RewardStore == nil &&
		s.ContactStore == nil &&
		s.FriendInviteTokenStore == nil &&
		s.FriendStore == nil &&
		s.FriendGroupStore == nil &&
		s.FriendGroupInviteTokenStore == nil &&
		s.FriendGroupMemberStore == nil &&
		s.FriendGroupBelongStore == nil &&
		s.FriendGroupMessageStore == nil &&
		s.FriendGroupMessageAssets == nil &&
		s.FriendGroupMessageDefaultTTL == 0 &&
		s.FriendGroupMessageMaxTTL == 0 &&
		s.FriendGroupMessageCleanup == 0 &&
		s.FriendGroupMessageMaxBytes == 0 &&
		s.RewardClaimGenerator == "" &&
		s.RewardClaimCooldown == 0 &&
		s.PetActionGenerator == ""
	peerStore := s.PeerStore
	if legacySharedStore {
		peerStore = kv.Prefixed(s.PeerStore, kv.Key{"peers"})
	}
	credentialStore := moduleStore(s.CredentialStore, s.PeerStore, "credentials")
	firmwareStore := moduleStore(s.FirmwareStore, s.PeerStore, "firmwares")
	miniMaxCredentialStore := moduleStore(s.MiniMaxCredentialStore, credentialStore, "")
	miniMaxTenantStore := moduleStore(s.MiniMaxTenantStore, s.PeerStore, "minimax-tenants")
	volcTenantStore := moduleStore(s.VolcTenantStore, miniMaxTenantStore, "volc-tenants")
	modelStore := moduleStore(s.ModelStore, s.PeerStore, "models")
	voiceStore := moduleStore(s.VoiceStore, s.PeerStore, "voices")
	workspaceStore := moduleStore(s.WorkspaceStore, s.PeerStore, "workspaces")
	workflowStore := moduleStore(s.WorkflowStore, s.PeerStore, "workflows")
	peerRunStore := moduleStore(s.PeerRunStore, s.PeerStore, "peer-run")
	publicLoginStore := moduleStore(s.PublicLoginStore, s.PeerStore, "public-login")
	petStore := moduleStore(s.PetStore, s.PeerStore, "pets")
	petSpeciesStore := moduleStore(s.PetSpeciesStore, s.PeerStore, "pet-species")
	badgeStore := moduleStore(s.BadgeStore, s.PeerStore, "badges")
	rewardStore := moduleStore(s.RewardStore, s.PeerStore, "rewards")
	contactStore := moduleStore(s.ContactStore, s.PeerStore, "contacts")
	friendInviteTokenStore := moduleStore(s.FriendInviteTokenStore, s.PeerStore, "friend-invite-tokens")
	friendStore := moduleStore(s.FriendStore, s.PeerStore, "friends")
	friendGroupStore := moduleStore(s.FriendGroupStore, s.PeerStore, "friend-groups")
	friendGroupInviteTokenStore := moduleStore(s.FriendGroupInviteTokenStore, s.PeerStore, "friend-group-invite-tokens")
	friendGroupMemberStore := moduleStore(s.FriendGroupMemberStore, s.PeerStore, "friend-group-members")
	friendGroupBelongStore := moduleStore(s.FriendGroupBelongStore, s.PeerStore, "friend-group-belongs")
	friendGroupMessageStore := moduleStore(s.FriendGroupMessageStore, s.PeerStore, "friend-group-messages")

	publicLoginServer := publiclogin.NewServer(&s.LocalStatic, publicLoginStore)
	sessions := publicLoginServer.SessionManager()
	peersServer := &peer.Server{
		Store:           peerStore,
		BuildCommit:     s.BuildCommit,
		Endpoint:        s.PublicEndpoint,
		ServerPublicKey: s.LocalStatic.Public,
		SignalingPath:   gizwebrtc.SignalingPath,
	}
	manager := NewManager(peersServer)
	manager.PeerRun = &peerrun.Server{Store: peerRunStore}
	peersServer.PeerManager = manager

	workflowServer := &workflow.Server{Store: workflowStore}
	workspaceServer := &workspace.Server{Store: workspaceStore, WorkflowStore: workflowStore}
	if s.AgentHostStore != nil {
		workspaceServer.RuntimeStore = workspace.NewObjectRuntimeStore(s.AgentHostStore)
	}
	credentialServer := &credential.Server{Store: credentialStore}
	firmwareServer := &firmware.Server{Store: firmwareStore, Assets: s.FirmwareAssets}
	modelServer := &model.Server{Store: modelStore}
	voiceServer := &voice.Server{Store: voiceStore}
	var aclServer *acl.Server
	if s.ACLDB != nil {
		aclServer = &acl.Server{DB: s.ACLDB}
	}
	workspaceServer.Authorizer = aclServer
	var walletServer *wallet.Server
	if s.WalletDB != nil {
		walletServer = &wallet.Server{DB: s.WalletDB}
	}
	petSpeciesServer := &petspecies.Server{Store: petSpeciesStore, Assets: s.PetSpeciesAssets}
	badgeServer := &badge.Server{Store: badgeStore, Assets: s.BadgeAssets}
	rewardServer := &reward.Server{
		Store:         rewardStore,
		Wallet:        walletServer,
		BadgeResolver: badgeGrantResolver{Badges: badgeServer, ACL: aclServer},
		Cooldown:      s.RewardClaimCooldown,
	}
	petServer := &pet.Server{
		Store:           petStore,
		Wallet:          walletServer,
		SpeciesSelector: firstPetSpeciesSelector{Service: petSpeciesServer, ACL: aclServer, Peers: peersServer},
		VoiceSelector:   firstVoiceSelector{Service: voiceServer},
		AdoptPointCost:  s.PetAdoptPointCost,
	}
	contactServer := &contact.Server{
		Store: contactStore,
	}
	friendServer := &friend.Server{
		InviteTokens: friendInviteTokenStore,
		Friends:      friendStore,
		Workspaces:   workspaceServer,
		ACL:          aclServer,
	}
	friendGroupServer := &friendgroup.Server{
		Groups:               friendGroupStore,
		InviteTokens:         friendGroupInviteTokenStore,
		Members:              friendGroupMemberStore,
		Belongs:              friendGroupBelongStore,
		Messages:             friendGroupMessageStore,
		MessageAssets:        s.FriendGroupMessageAssets,
		ACL:                  aclServer,
		Workspaces:           workspaceServer,
		MessageDefaultTTL:    s.FriendGroupMessageDefaultTTL,
		MessageMaxTTL:        s.FriendGroupMessageMaxTTL,
		MessageMaxAudioBytes: s.FriendGroupMessageMaxBytes,
	}
	providerTenantsServer := &providertenants.Server{
		ModelStore:      modelStore,
		TenantStore:     miniMaxTenantStore,
		VolcTenantStore: volcTenantStore,
		VoiceStore:      voiceStore,
		CredentialStore: miniMaxCredentialStore,
	}
	systemGenerator := model.NewGenerator(peergenx.Service{
		Peer:            systemTaskPeer(s.LocalStatic.Public),
		Authorizer:      systemTaskAuthorizer{},
		Models:          modelServer,
		Voices:          voiceServer,
		Credentials:     credentialServer,
		ProviderTenants: providerTenantsServer,
	})
	if strings.TrimSpace(s.RewardClaimGenerator) != "" {
		rewardServer.Decider = genxRewardDecider{
			Generator: systemGenerator,
			Pattern:   s.RewardClaimGenerator,
		}
	}
	if strings.TrimSpace(s.PetActionGenerator) != "" {
		petServer.ActionDecider = genxPetActionDecider{
			Generator: systemGenerator,
			Pattern:   s.PetActionGenerator,
		}
	}
	manager.ACL = aclServer
	manager.AgentHost = agenthost.New(agenthost.ServiceResolver{
		Workspaces: workspaceServer,
		Workflows:  workflowServer,
	})
	manager.Workspaces = workspaceServer
	manager.Workflows = workflowServer
	manager.Firmwares = firmwareServer
	manager.Models = modelServer
	manager.Credentials = credentialServer
	manager.Voices = voiceServer
	manager.Pets = petServer
	manager.Wallets = walletServer
	manager.Rewards = rewardServer
	manager.Contacts = contactServer
	manager.Friends = friendServer
	manager.FriendGroups = friendGroupServer
	manager.ProviderTenants = providerTenantsServer
	resourceManager := resourcemanager.New(resourcemanager.Services{
		ACL:             aclServer,
		Credentials:     credentialServer,
		Firmwares:       firmwareServer,
		Badges:          badgeServer,
		Peers:           peersServer,
		Models:          modelServer,
		PetSpecies:      petSpeciesServer,
		ProviderTenants: providerTenantsServer,
		Voices:          voiceServer,
		Workspaces:      workspaceServer,
		Workflows:       workflowServer,
		Contacts:        contactServer,
		Friends:         friendServer,
		FriendGroups:    friendGroupServer,
	})

	s.manager = manager
	s.peerService = &PeerService{
		manager: manager,
		admin: &adminService{
			CredentialAdminService:      credentialServer,
			FirmwareAdminService:        firmwareServer,
			PeerAdminService:            peersServer,
			ModelAdminService:           modelServer,
			VoiceAdminService:           voiceServer,
			ProviderTenantsAdminService: providerTenantsServer,
			WorkspaceAdminService:       workspaceServer,
			WorkflowAdminService:        workflowServer,
			PetSpecies:                  petSpeciesServer,
			Badges:                      badgeServer,
			Contacts:                    contactServer,
			Friends:                     friendServer,
			FriendGroups:                friendGroupServer,
			ACL:                         aclServer,
			ResourceManager:             resourceManager,
		},
		public: &serverPublic{
			ServerPublicService: peersServer,
			ServerPublic:        publicLoginServer,
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
	s.httpHandler = httpLabelSetHandler(mux)
	return nil
}

type firstPetSpeciesSelector struct {
	Service *petspecies.Server
	ACL     aclAuthorizer
	Peers   peerConfigGetter
}

func (s firstPetSpeciesSelector) SelectSpecies(ctx context.Context, owner string) (string, error) {
	if s.Service == nil {
		return "", errors.New("pet species service not configured")
	}
	if s.ACL == nil {
		return "", errors.New("acl service not configured")
	}
	cursor := ""
	authorizer := s.authorizer(owner)
	for {
		items, hasNext, next, err := s.Service.List(ctx, cursor, 50)
		if err != nil {
			return "", err
		}
		for _, item := range items {
			if err := authorizer.Authorize(ctx, acl.AuthorizeRequest{
				Subject:    acl.PublicKeySubject(owner),
				Resource:   acl.PetSpeciesResource(item.Id),
				Permission: apitypes.ACLPermissionPetSpeciesUse,
			}); err == nil {
				return item.Id, nil
			} else if !errors.Is(err, acl.ErrDenied) {
				return "", err
			}
		}
		if !hasNext || next == nil {
			break
		}
		cursor = *next
	}
	return "", errors.New("no usable pet species available")
}

func (s firstPetSpeciesSelector) authorizer(owner string) aclAuthorizer {
	if s.Peers == nil {
		return s.ACL
	}
	var pk giznet.PublicKey
	if err := pk.UnmarshalText([]byte(owner)); err != nil {
		return s.ACL
	}
	return peerAuthorizer{
		ACL:       s.ACL,
		Peers:     s.Peers,
		PublicKey: pk,
	}
}

type firstVoiceSelector struct {
	Service voice.VoiceAdminService
}

func (s firstVoiceSelector) SelectVoice(ctx context.Context, owner string) (string, error) {
	if s.Service == nil {
		return "", errors.New("voice service not configured")
	}
	limit := int32(1)
	resp, err := s.Service.ListVoices(ctx, adminservice.ListVoicesRequestObject{
		Params: adminservice.ListVoicesParams{Limit: &limit},
	})
	if err != nil {
		return "", err
	}
	list, ok := resp.(adminservice.ListVoices200JSONResponse)
	if !ok {
		return "", fmt.Errorf("voice list returned %T", resp)
	}
	if len(list.Items) == 0 {
		return "", errors.New("no voices available")
	}
	return list.Items[0].Id, nil
}

func moduleStore(configured, fallback kv.Store, defaultPrefix string) kv.Store {
	if configured != nil {
		return configured
	}
	if fallback == nil {
		return nil
	}
	if defaultPrefix == "" {
		return fallback
	}
	return kv.Prefixed(fallback, kv.Key{defaultPrefix})
}
