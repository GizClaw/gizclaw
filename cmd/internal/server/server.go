package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"

	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/pion/webrtc/v4"
)

var BuildCommit = "dev"

// CmdServer owns the command-layer store registry for a gizclaw server.
type CmdServer struct {
	*gizclaw.Server
	AdminPublicKey  giznet.PublicKey
	ServeToClients  bool
	stores          *stores.Stores
	ownsStores      bool
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
	if s.ownsStores && s.stores != nil {
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
		if isPublicHTTPLoginRoute(r.Method, r.URL.Path) {
			if isSideControlLoginRequest(r) {
				writePrivateHTTPIngressDenied(w)
				return
			}
		} else if !s.authorizePrivateHTTPIngress(w, r) {
			return
		}
	}
	s.Server.ServeHTTP(w, r)
}

func (s *CmdServer) authorizePrivateHTTPIngress(w http.ResponseWriter, r *http.Request) bool {
	principal, err := s.Server.AuthenticateHTTPSessionPrincipalHeaders(r.Header.Get("Authorization"), r.Header.Get(publiclogin.PublicKeyHeader))
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
	if principal.Kind != publiclogin.SessionKindPrimary {
		writePrivateHTTPIngressDenied(w)
		return false
	}
	if err := s.Server.AuthorizePrivateHTTPIngress(r.Context(), principal.PublicKey); err != nil {
		writePrivateHTTPIngressDenied(w)
		return false
	}
	return true
}

func writePrivateHTTPIngressDenied(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":{"code":"PRIVATE_INGRESS_DENIED","message":"session peer is not authorized for private server ingress"}}`))
}

func isSideControlLoginRequest(r *http.Request) bool {
	if r == nil || r.Method != http.MethodPost || r.URL.Path != "/login" || r.Body == nil || r.Body == http.NoBody {
		return false
	}
	return r.ContentLength != 0
}

func isPublicHTTPRoute(path string) bool {
	if strings.HasPrefix(path, "/me/side-control/") || strings.HasPrefix(path, "/side-control/") {
		return true
	}
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
	Stores         *stores.Stores
	StoreOptions   stores.Options
}

func New(cfg Config) (srv *CmdServer, err error) {
	return newWithOptions(cfg, newServerOptions{})
}

func newWithOptions(cfg Config, newOpts newServerOptions) (srv *CmdServer, err error) {
	cfg, err = prepareConfig(cfg)
	if err != nil {
		return nil, err
	}
	ss := newOpts.Stores
	ownsStores := false
	if ss == nil {
		ss, err = newStoreRegistryWithOptions(cfg, newOpts.StoreOptions)
		if err != nil {
			return nil, fmt.Errorf("server: stores: %w", err)
		}
		ownsStores = true
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
			if ownsStores {
				err = errors.Join(err, openedStores.Close())
			}
		}
	}()

	peersKV, err := ss.KV(defaultPeersStore)
	if err != nil {
		return nil, fmt.Errorf("server: peers store: %w", err)
	}

	cmdSrv := &CmdServer{stores: ss, ownsStores: ownsStores, AdminPublicKey: cfg.AdminPublicKey, ServeToClients: cfg.ServeToClients}
	var gizServer *gizclaw.Server
	gizServer = &gizclaw.Server{
		LocalStatic:    *cfg.KeyPair,
		PeerStore:      peersKV,
		BuildCommit:    BuildCommit,
		PublicEndpoint: cfg.Endpoint,
		PublicICETCP:   newOpts.ICETCPListener != nil,
		EdgeNodes:      cfg.EdgeNodes,
		ICEServers:     cfg.ICEServers,
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
	if cfg.SystemLog.QueryStore != "" {
		logQuery, err := ss.Log(cfg.SystemLog.QueryStore)
		if err != nil {
			return nil, fmt.Errorf("server: log query store %q: %w", cfg.SystemLog.QueryStore, err)
		}
		gizServer.ServerLogQuery, err = gizclaw.NewServerLogQueryService(logQuery)
		if err != nil {
			return nil, fmt.Errorf("server: initialize log query service: %w", err)
		}
	}
	if storeExists(cfg, defaultFlowcraftHistoryStore) {
		gizServer.FlowcraftHistory, err = ss.MutableLog(defaultFlowcraftHistoryStore)
		if err != nil {
			return nil, fmt.Errorf("server: flowcraft history store %q: %w", defaultFlowcraftHistoryStore, err)
		}
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
	transcriptionDuration, _ := parsePositiveConfigDuration(cfg.Speech.Transcription.MaxAudioDuration)
	transcriptionTimeout, _ := parsePositiveConfigDuration(cfg.Speech.Transcription.RequestTimeout)
	synthesisTimeout, _ := parsePositiveConfigDuration(cfg.Speech.Synthesis.RequestTimeout)
	gizServer.SpeechLimits = gizclaw.SpeechLimits{
		TranscriptionMaxAudioBytes:    cfg.Speech.Transcription.MaxAudioBytes,
		TranscriptionMaxAudioDuration: transcriptionDuration,
		TranscriptionRequestTimeout:   transcriptionTimeout,
		SynthesisMaxTextBytes:         int(cfg.Speech.Synthesis.MaxTextBytes),
		SynthesisMaxOutputBytes:       cfg.Speech.Synthesis.MaxOutputBytes,
		SynthesisRequestTimeout:       synthesisTimeout,
	}
	if !cfg.AdminPublicKey.IsZero() {
		gizServer.SecurityPolicy = adminPublicKeySecurityPolicy{
			PublicKey: cfg.AdminPublicKey,
		}
	}
	if !cfg.ServeToClients {
		gizServer.PublicLoginAuthorizer = gizclaw.PrivateHTTPIngressLoginAuthorizer(gizServer)
	}
	if len(cfg.Storage) > 0 {
		if gizServer.CredentialStore, err = ss.KV(defaultCredentialsStore); err != nil {
			return nil, fmt.Errorf("server: credentials store: %w", err)
		}
		if gizServer.FirmwareStore, err = ss.KV(defaultFirmwaresStore); err != nil {
			return nil, fmt.Errorf("server: firmwares store: %w", err)
		}
		if storeExists(cfg, defaultRuntimeProfilesStore) {
			if gizServer.RuntimeProfileStore, err = ss.KV(defaultRuntimeProfilesStore); err != nil {
				return nil, fmt.Errorf("server: runtime profiles store: %w", err)
			}
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
		if storeExists(cfg, defaultDeepSeekTenantsStore) {
			if gizServer.DeepSeekTenantStore, err = ss.KV(defaultDeepSeekTenantsStore); err != nil {
				return nil, fmt.Errorf("server: deepseek tenants store: %w", err)
			}
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
		if storeExists(cfg, defaultWorkspaceAssetsStore) {
			if gizServer.WorkspaceAssets, err = ss.ObjectStore(defaultWorkspaceAssetsStore); err != nil {
				return nil, fmt.Errorf("server: workspace assets store: %w", err)
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
	if err := bootstrapEdgeNodes(context.Background(), &runtimepeer.Server{Store: gizServer.EffectivePeerStore()}, cfg.EdgeNodes); err != nil {
		return nil, err
	}
	return cmdSrv, nil
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
	return newStoreRegistryContext(context.Background(), cfg)
}

func newStoreRegistryContext(ctx context.Context, cfg Config) (*stores.Stores, error) {
	return newStoreRegistryWithOptionsContext(ctx, cfg, stores.Options{})
}

func newStoreRegistryWithOptions(cfg Config, options stores.Options) (*stores.Stores, error) {
	return newStoreRegistryWithOptionsContext(context.Background(), cfg, options)
}

func newStoreRegistryWithOptionsContext(ctx context.Context, cfg Config, options stores.Options) (*stores.Stores, error) {
	if options.FlowcraftModelLoader == nil {
		for name, storeConfig := range cfg.Stores {
			if storeConfig.Flowcraft == nil {
				continue
			}
			flowcraft := storeConfig.Flowcraft
			if os.ExpandEnv(flowcraft.ExtractionModel) != "" || os.ExpandEnv(flowcraft.EmbeddingModel) != "" || os.ExpandEnv(flowcraft.RerankModel) != "" {
				return nil, fmt.Errorf("memory store %q flowcraft model fields require an injected model loader", name)
			}
		}
	}
	if len(cfg.Storage) == 0 {
		return stores.NewWithOptions(ctx, cfg.Stores, options)
	}
	physical, err := storage.New(cfg.Storage)
	if err != nil {
		return nil, err
	}
	ss, err := stores.NewWithOwnedStorageOptions(ctx, physical, cfg.Stores, options)
	if err != nil {
		return nil, err
	}
	return ss, nil
}
