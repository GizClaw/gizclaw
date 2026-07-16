package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"

	"github.com/GizClaw/gizclaw-go/cmd/internal/logging/volclog"
	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	petagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/pet"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/pion/webrtc/v4"
)

var BuildCommit = "dev"

// CmdServer owns the command-layer store registry for a gizclaw server.
type CmdServer struct {
	*gizclaw.Server
	AdminPublicKey  giznet.PublicKey
	ServeToClients  bool
	stores          *stores.Stores
	metricsShutdown func(context.Context) error
}

func (s *CmdServer) Close() error {
	if s == nil {
		return nil
	}
	var errs []error
	if s.Server != nil {
		errs = append(errs, s.Server.Close())
		s.Server = nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), gizmetrics.DefaultAppendTimeout)
	errs = append(errs, s.shutdownMetrics(shutdownCtx))
	cancel()
	if s.stores != nil {
		errs = append(errs, s.stores.Close())
		s.stores = nil
	}
	return errors.Join(errs...)
}

func (s *CmdServer) shutdownMetrics(ctx context.Context) error {
	if s == nil || s.metricsShutdown == nil {
		return nil
	}
	shutdown := s.metricsShutdown
	s.metricsShutdown = nil
	return shutdown(ctx)
}

func (s *CmdServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.Server == nil {
		http.NotFound(w, r)
		return
	}
	if !s.ServeToClients && isPublicHTTPRoute(r.URL.Path) {
		if !isPublicHTTPLoginRoute(r.Method, r.URL.Path) && !s.authorizePrivateHTTPIngress(w, r) {
			return
		}
	}
	s.Server.ServeHTTP(w, r)
}

func (s *CmdServer) authorizePrivateHTTPIngress(w http.ResponseWriter, r *http.Request) bool {
	publicKey, err := s.Server.AuthenticateHTTPSessionHeaders(r.Header.Get("Authorization"), r.Header.Get(publiclogin.PublicKeyHeader))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		if errors.Is(err, publiclogin.ErrPublicKeyMismatch) {
			_, _ = w.Write([]byte(`{"error":{"code":"PUBLIC_KEY_MISMATCH","message":"x-public-key does not match bearer session"}}`))
			return false
		}
		_, _ = w.Write([]byte(`{"error":{"code":"INVALID_SESSION","message":"missing or invalid bearer session"}}`))
		return false
	}
	if err := s.Server.AuthorizePrivateHTTPIngress(r.Context(), publicKey); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"PRIVATE_INGRESS_DENIED","message":"session peer is not authorized for private server ingress"}}`))
		return false
	}
	return true
}

func isPublicHTTPRoute(path string) bool {
	switch path {
	case "/server-info", "/login", gizwebrtc.SignalingPath, "/me", "/me/status", "/me/runtime":
		return true
	default:
		return strings.HasPrefix(path, "/openai/v1/")
	}
}

func isPublicHTTPLoginRoute(method, path string) bool {
	return (method == http.MethodPost && path == "/login") ||
		((method == http.MethodPost || method == http.MethodOptions) && path == gizwebrtc.SignalingPath)
}

// New wires an already prepared in-memory config into a command server.
type newServerOptions struct {
	ICETCPListener net.Listener
}

func New(cfg Config) (srv *CmdServer, err error) {
	return newWithOptions(cfg, newServerOptions{})
}

func newWithOptions(cfg Config, newOpts newServerOptions) (srv *CmdServer, err error) {
	cfg, err = prepareConfig(cfg)
	if err != nil {
		return nil, err
	}
	ss, err := newStoreRegistry(cfg)
	if err != nil {
		return nil, fmt.Errorf("server: stores: %w", err)
	}
	openedStores := ss
	var metricsShutdown func(context.Context) error
	defer func() {
		if err != nil {
			if metricsShutdown != nil {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), gizmetrics.DefaultAppendTimeout)
				err = errors.Join(err, metricsShutdown(shutdownCtx))
				cancel()
			}
			err = errors.Join(err, openedStores.Close())
		}
	}()

	peersKV, err := ss.KV(defaultPeersStore)
	if err != nil {
		return nil, fmt.Errorf("server: peers store: %w", err)
	}

	cmdSrv := &CmdServer{stores: ss, AdminPublicKey: cfg.AdminPublicKey, ServeToClients: cfg.ServeToClients}
	var gizServer *gizclaw.Server
	gizServer = &gizclaw.Server{
		LocalStatic:     *cfg.KeyPair,
		PeerStore:       peersKV,
		BuildCommit:     BuildCommit,
		PublicEndpoint:  cfg.Endpoint,
		PublicICETCP:    newOpts.ICETCPListener != nil,
		DefaultPeerView: cfg.DefaultPeerView,
		EdgeNodes:       cfg.EdgeNodes,
		ICEServers:      cfg.ICEServers,
		PetWorkflow: petagent.Config{
			GenerateModel:  cfg.SystemTasks.PetFlowcraftWorkflow.GenerateModel,
			ExtractModel:   cfg.SystemTasks.PetFlowcraftWorkflow.ExtractModel,
			EmbeddingModel: cfg.SystemTasks.PetFlowcraftWorkflow.EmbeddingModel,
			ASRModel:       cfg.SystemTasks.PetFlowcraftWorkflow.ASRModel,
		},
		PeerListenerFactories: []gizclaw.PeerListenerFactory{
			func(opts gizclaw.PeerListenerOptions) (giznet.Listener, error) {
				listenConfig := webRTCListenConfig(cfg, opts, newOpts.ICETCPListener)
				l, err := (&listenConfig).Listen(opts.KeyPair)
				if err != nil {
					return nil, err
				}
				gizServer.WebRTCSignalingHandler = l.SignalingHandler()
				return l, nil
			},
		},
	}
	if cfg.Log.Volc.Enabled {
		logQuery, err := volclog.NewQueryService(volclog.Config{
			Endpoint:        cfg.Log.Volc.Endpoint,
			Region:          cfg.Log.Volc.Region,
			TopicID:         cfg.Log.Volc.TopicID,
			AccessKeyID:     cfg.Log.Volc.AccessKeyID,
			AccessKeySecret: cfg.Log.Volc.AccessKeySecret,
		})
		if err != nil {
			return nil, fmt.Errorf("server: log query: %w", err)
		}
		gizServer.ServerLogQuery = logQuery
	}
	cmdSrv.Server = gizServer
	if cfg.FriendGroups.MessageDefaultTTL != "" {
		ttl, err := parseConfigDuration(cfg.FriendGroups.MessageDefaultTTL)
		if err != nil {
			return nil, fmt.Errorf("server: friend_groups.message_default_ttl: %w", err)
		}
		gizServer.FriendGroupMessageDefaultTTL = ttl
	}
	if cfg.FriendGroups.MessageMaxTTL != "" {
		ttl, err := parseConfigDuration(cfg.FriendGroups.MessageMaxTTL)
		if err != nil {
			return nil, fmt.Errorf("server: friend_groups.message_max_ttl: %w", err)
		}
		gizServer.FriendGroupMessageMaxTTL = ttl
	}
	if cfg.FriendGroups.MessageCleanupInterval != "" {
		interval, err := parseConfigDuration(cfg.FriendGroups.MessageCleanupInterval)
		if err != nil {
			return nil, fmt.Errorf("server: friend_groups.message_cleanup_interval: %w", err)
		}
		gizServer.FriendGroupMessageCleanup = interval
	}
	gizServer.FriendGroupMessageMaxBytes = cfg.FriendGroups.MessageMaxAudioBytes
	if !cfg.AdminPublicKey.IsZero() {
		gizServer.SecurityPolicy = adminPublicKeySecurityPolicy{
			PublicKey: cfg.AdminPublicKey,
		}
	}
	if !cfg.ServeToClients {
		gizServer.PublicLoginAuthorizer = gizclaw.PrivateHTTPIngressLoginAuthorizer(gizServer)
	}
	if len(cfg.Storage) > 0 {
		assetMetadataConfigured := storeExists(cfg, defaultAssetMetadataStore)
		assetObjectsConfigured := storeExists(cfg, defaultAssetObjectsStore)
		if assetMetadataConfigured != assetObjectsConfigured {
			return nil, errors.New("server: asset metadata and object stores must be configured together")
		}
		if assetMetadataConfigured {
			if gizServer.AssetMetadataStore, err = ss.KV(defaultAssetMetadataStore); err != nil {
				return nil, fmt.Errorf("server: asset metadata store: %w", err)
			}
			if gizServer.AssetObjects, err = ss.ObjectStore(defaultAssetObjectsStore); err != nil {
				return nil, fmt.Errorf("server: asset object store: %w", err)
			}
		}
		if gizServer.CredentialStore, err = ss.KV(defaultCredentialsStore); err != nil {
			return nil, fmt.Errorf("server: credentials store: %w", err)
		}
		if gizServer.FirmwareStore, err = ss.KV(defaultFirmwaresStore); err != nil {
			return nil, fmt.Errorf("server: firmwares store: %w", err)
		}
		if storeExists(cfg, defaultFirmwareAssetsStore) {
			if gizServer.FirmwareAssets, err = ss.ObjectStore(defaultFirmwareAssetsStore); err != nil {
				return nil, fmt.Errorf("server: firmwares assets store: %w", err)
			}
		}
		if storeExists(cfg, defaultAgentHostStore) {
			if gizServer.AgentHostStore, err = ss.ObjectStore(defaultAgentHostStore); err != nil {
				return nil, fmt.Errorf("server: agenthost store: %w", err)
			}
		}
		if gizServer.MiniMaxCredentialStore, err = ss.KV(defaultCredentialsStore); err != nil {
			return nil, fmt.Errorf("server: minimax credentials store: %w", err)
		}
		if gizServer.MiniMaxTenantStore, err = ss.KV(defaultMiniMaxTenantsStore); err != nil {
			return nil, fmt.Errorf("server: minimax tenants store: %w", err)
		}
		if gizServer.VoiceStore, err = ss.KV(defaultVoicesStore); err != nil {
			return nil, fmt.Errorf("server: voices store: %w", err)
		}
		if gizServer.WorkspaceStore, err = ss.KV(defaultWorkspacesStore); err != nil {
			return nil, fmt.Errorf("server: workspaces store: %w", err)
		}
		if gizServer.WorkflowStore, err = ss.KV(defaultWorkflowsStore); err != nil {
			return nil, fmt.Errorf("server: workflows store: %w", err)
		}
		if gizServer.ACLDB, err = ss.SQL(defaultACLStore); err != nil {
			return nil, fmt.Errorf("server: acl store: %w", err)
		}
		if storeExists(cfg, defaultContactsStore) {
			if gizServer.ContactStore, err = ss.KV(defaultContactsStore); err != nil {
				return nil, fmt.Errorf("server: contacts store: %w", err)
			}
		}
		if storeExists(cfg, defaultFriendInviteTokensStore) {
			if gizServer.FriendInviteTokenStore, err = ss.KV(defaultFriendInviteTokensStore); err != nil {
				return nil, fmt.Errorf("server: friend invite tokens store: %w", err)
			}
		}
		if storeExists(cfg, defaultFriendsStore) {
			if gizServer.FriendStore, err = ss.KV(defaultFriendsStore); err != nil {
				return nil, fmt.Errorf("server: friends store: %w", err)
			}
		}
		if storeExists(cfg, defaultFriendGroupsStore) {
			if gizServer.FriendGroupStore, err = ss.KV(defaultFriendGroupsStore); err != nil {
				return nil, fmt.Errorf("server: friend_groups store: %w", err)
			}
		}
		if storeExists(cfg, defaultFriendGroupInviteTokensStore) {
			if gizServer.FriendGroupInviteTokenStore, err = ss.KV(defaultFriendGroupInviteTokensStore); err != nil {
				return nil, fmt.Errorf("server: friend group invite tokens store: %w", err)
			}
		}
		if storeExists(cfg, defaultFriendGroupMembersStore) {
			if gizServer.FriendGroupMemberStore, err = ss.KV(defaultFriendGroupMembersStore); err != nil {
				return nil, fmt.Errorf("server: friend group members store: %w", err)
			}
		}
		if storeExists(cfg, defaultFriendGroupBelongsStore) {
			if gizServer.FriendGroupBelongStore, err = ss.KV(defaultFriendGroupBelongsStore); err != nil {
				return nil, fmt.Errorf("server: friend group belongs store: %w", err)
			}
		}
		if storeExists(cfg, defaultFriendGroupMessagesStore) {
			if gizServer.FriendGroupMessageStore, err = ss.KV(defaultFriendGroupMessagesStore); err != nil {
				return nil, fmt.Errorf("server: friend group messages store: %w", err)
			}
		}
		if storeExists(cfg, defaultFriendGroupMessageAssetsStore) {
			if gizServer.FriendGroupMessageAssets, err = ss.ObjectStore(defaultFriendGroupMessageAssetsStore); err != nil {
				return nil, fmt.Errorf("server: friend group message assets store: %w", err)
			}
		}
		if storeExists(cfg, defaultGameRulesetsStore) {
			if gizServer.GameRulesetStore, err = ss.KV(defaultGameRulesetsStore); err != nil {
				return nil, fmt.Errorf("server: game rulesets store: %w", err)
			}
		}
		if storeExists(cfg, defaultPetDefsStore) {
			if gizServer.PetDefStore, err = ss.KV(defaultPetDefsStore); err != nil {
				return nil, fmt.Errorf("server: pet defs store: %w", err)
			}
		}
		if storeExists(cfg, defaultBadgeDefsStore) {
			if gizServer.BadgeDefStore, err = ss.KV(defaultBadgeDefsStore); err != nil {
				return nil, fmt.Errorf("server: badge defs store: %w", err)
			}
		}
		if storeExists(cfg, defaultGameDefsStore) {
			if gizServer.GameDefStore, err = ss.KV(defaultGameDefsStore); err != nil {
				return nil, fmt.Errorf("server: game defs store: %w", err)
			}
		}
		if storeExists(cfg, defaultGameplayAssetsStore) {
			if gizServer.GameplayAssets, err = ss.ObjectStore(defaultGameplayAssetsStore); err != nil {
				return nil, fmt.Errorf("server: gameplay assets store: %w", err)
			}
		}
		if storeExists(cfg, defaultGameplayDBStore) {
			if gizServer.GameplayDB, err = ss.SQL(defaultGameplayDBStore); err != nil {
				return nil, fmt.Errorf("server: gameplay db store: %w", err)
			}
		}
	}
	if storeExists(cfg, defaultMetricsStore) {
		if gizServer.MetricsStore, err = ss.Metrics(defaultMetricsStore); err != nil {
			return nil, fmt.Errorf("server: metrics store: %w", err)
		}
		metricsShutdown, err = gizmetrics.InstallStore(gizServer.MetricsStore)
		if err != nil {
			return nil, fmt.Errorf("server: install metrics recorder: %w", err)
		}
		cmdSrv.metricsShutdown = metricsShutdown
	}
	if err := bootstrapEdgeNodes(context.Background(), &runtimepeer.Server{Store: effectivePeerStore(gizServer)}, cfg.EdgeNodes); err != nil {
		return nil, err
	}
	return cmdSrv, nil
}

func effectivePeerStore(s *gizclaw.Server) kv.Store {
	if usesLegacySharedStore(s) {
		return kv.Prefixed(s.PeerStore, kv.Key{"peers"})
	}
	return s.PeerStore
}

func usesLegacySharedStore(s *gizclaw.Server) bool {
	return s.CredentialStore == nil &&
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
		s.ContactStore == nil &&
		s.FriendInviteTokenStore == nil &&
		s.FriendStore == nil &&
		s.FriendGroupStore == nil &&
		s.FriendGroupInviteTokenStore == nil &&
		s.FriendGroupMemberStore == nil &&
		s.FriendGroupBelongStore == nil &&
		s.FriendGroupMessageStore == nil &&
		s.FriendGroupMessageAssets == nil &&
		s.GameRulesetStore == nil &&
		s.PetDefStore == nil &&
		s.BadgeDefStore == nil &&
		s.GameDefStore == nil &&
		s.GameplayAssets == nil &&
		s.GameplayDB == nil &&
		s.FriendGroupMessageDefaultTTL == 0 &&
		s.FriendGroupMessageMaxTTL == 0 &&
		s.FriendGroupMessageCleanup == 0 &&
		s.FriendGroupMessageMaxBytes == 0
}

func bootstrapEdgeNodes(ctx context.Context, peers *runtimepeer.Server, publicKeys []giznet.PublicKey) error {
	if len(publicKeys) == 0 {
		return nil
	}
	approvedAt := time.Now()
	for _, publicKey := range publicKeys {
		if publicKey.IsZero() {
			return fmt.Errorf("server: bootstrap edge-node: zero public key")
		}
		peer, err := peers.LoadPeer(ctx, publicKey)
		if errors.Is(err, runtimepeer.ErrPeerNotFound) {
			peer = apitypes.Peer{PublicKey: publicKey.String()}
		} else if err != nil {
			return fmt.Errorf("server: load bootstrap edge-node %s: %w", publicKey, err)
		}
		peer.Role = apitypes.PeerRoleEdgeNode
		peer.Status = apitypes.PeerRegistrationStatusActive
		if peer.ApprovedAt == nil {
			peer.ApprovedAt = &approvedAt
		}
		if _, err := peers.SavePeer(ctx, peer); err != nil {
			return fmt.Errorf("server: bootstrap edge-node %s: %w", publicKey, err)
		}
	}
	return nil
}

func webRTCListenConfig(cfg Config, opts gizclaw.PeerListenerOptions, iceTCPListener net.Listener) gizwebrtc.ListenConfig {
	publicAddr := publicICEAddr(cfg)
	listenConfig := gizwebrtc.ListenConfig{
		ICEUDPAddr:       cfg.ICEListenAddr(),
		ICETCPListener:   iceTCPListener,
		PublicICEUDPAddr: publicAddr,
		PublicICETCPAddr: publicAddr,
		ICEServers:       cfg.ICEServers,
		SecurityPolicy:   opts.SecurityPolicy,
		PeerEventHandler: opts.PeerEventHandler,
	}
	if gizwebrtc.HasTURNServer(cfg.ICEServers) {
		listenConfig.ICETransportPolicy = webrtc.ICETransportPolicyRelay
	}
	return listenConfig
}

func publicICEAddr(cfg Config) string {
	if gizwebrtc.HasTURNServer(cfg.ICEServers) {
		return ""
	}
	host, _, err := net.SplitHostPort(cfg.Endpoint)
	if err != nil {
		return ""
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsUnspecified() {
		return ""
	}
	return cfg.Endpoint
}

func storeExists(cfg Config, name string) bool {
	_, ok := cfg.Stores[name]
	return ok
}

type adminPublicKeySecurityPolicy struct {
	PublicKey giznet.PublicKey
}

func (p adminPublicKeySecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (p adminPublicKeySecurityPolicy) AllowService(publicKey giznet.PublicKey, service uint64) bool {
	return service == gizclaw.ServiceAdminHTTP && publicKey == p.PublicKey
}

func newStoreRegistry(cfg Config) (*stores.Stores, error) {
	if len(cfg.Storage) == 0 {
		return stores.New(cfg.Stores)
	}
	physical, err := storage.New(cfg.Storage)
	if err != nil {
		return nil, err
	}
	ss, err := stores.NewWithOwnedStorage(physical, cfg.Stores)
	if err != nil {
		return nil, err
	}
	return ss, nil
}
