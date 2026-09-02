package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/buildinfo"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
	"github.com/pion/webrtc/v4"
)

// CmdServer owns the command-layer store registry for a gizclaw server.
type CmdServer struct {
	*gizclaw.Server
	AdminPublicKey  giznet.PublicKey
	stores          *stores.Stores
	storage         *storage.Storage
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
	if s.ownsStores && s.storage != nil {
		errs = append(errs, s.storage.Close())
		s.storage = nil
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
	if isProtectedPublicHTTPRoute(r.URL.Path) {
		writePrivateHTTPIngressDenied(w)
		return
	}
	s.Server.ServeHTTP(w, r)
}

func writePrivateHTTPIngressDenied(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":{"code":"PRIVATE_INGRESS_DENIED","message":"public client APIs are disabled"}}`))
}

func isProtectedPublicHTTPRoute(path string) bool {
	return strings.HasPrefix(path, "/gizclaw/v1/") || strings.HasPrefix(path, "/openai/v1/")
}

// New wires an already prepared in-memory config into a command server.
type newServerOptions struct {
	ICETCPListener net.Listener
	Stores         *stores.Stores
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
	var physical *storage.Storage
	ownsStores := false
	if ss == nil {
		ss, physical, err = newStoreRegistry(cfg)
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
				if physical != nil {
					err = errors.Join(err, physical.Close())
				}
			}
		}
	}()

	cmdSrv := &CmdServer{stores: ss, storage: physical, ownsStores: ownsStores, AdminPublicKey: cfg.AdminPublicKey}
	pendingDeletionConfig, err := cfg.PendingDeletion.processorConfig()
	if err != nil {
		return nil, fmt.Errorf("server: pending_deletion: %w", err)
	}
	var gizServer *gizclaw.Server
	gizServer = &gizclaw.Server{
		LocalStatic:           *cfg.KeyPair,
		MemoryRoot:            cfg.WorkspaceRoot,
		BuildVersion:          buildinfo.Version,
		BuildCommit:           buildinfo.Commit,
		PublicEndpoint:        cfg.WebRTC.Endpoint,
		PublicICETCP:          newOpts.ICETCPListener != nil,
		EdgeNodes:             cfg.EdgeNodes,
		PendingDeletionConfig: pendingDeletionConfig,
		ICEServers:            cfg.ICEServers,
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
	if err := configureServiceStores(gizServer, ss, cfg.Services); err != nil {
		return nil, err
	}
	cmdSrv.Server = gizServer
	transcriptionDuration, _ := parsePositiveConfigDuration(cfg.Speech.Transcription.MaxAudioDuration)
	transcriptionTimeout, _ := parsePositiveConfigDuration(cfg.Speech.Transcription.RequestTimeout)
	extractionTimeout, _ := parsePositiveConfigDuration(cfg.Speech.Extraction.RequestTimeout)
	synthesisTimeout, _ := parsePositiveConfigDuration(cfg.Speech.Synthesis.RequestTimeout)
	gizServer.SpeechLimits = gizclaw.SpeechLimits{
		TranscriptionMaxAudioBytes:    cfg.Speech.Transcription.MaxAudioBytes,
		TranscriptionMaxAudioDuration: transcriptionDuration,
		TranscriptionRequestTimeout:   transcriptionTimeout,
		ExtractionMaxSchemaBytes:      int(cfg.Speech.Extraction.MaxSchemaBytes),
		ExtractionMaxSchemaDepth:      int(cfg.Speech.Extraction.MaxSchemaDepth),
		ExtractionMaxSchemaProperties: int(cfg.Speech.Extraction.MaxSchemaProperties),
		ExtractionMaxInstructionBytes: int(cfg.Speech.Extraction.MaxInstructionBytes),
		ExtractionMaxResultBytes:      int(cfg.Speech.Extraction.MaxResultBytes),
		ExtractionRequestTimeout:      extractionTimeout,
		SynthesisMaxTextBytes:         int(cfg.Speech.Synthesis.MaxTextBytes),
		SynthesisMaxOutputBytes:       cfg.Speech.Synthesis.MaxOutputBytes,
		SynthesisRequestTimeout:       synthesisTimeout,
	}
	if !cfg.AdminPublicKey.IsZero() {
		gizServer.SecurityPolicy = adminPublicKeySecurityPolicy{
			PublicKey: cfg.AdminPublicKey,
		}
	}
	if gizServer.MetricsStore != nil {
		metricsShutdown, err = gizmetrics.InstallStore(gizServer.MetricsStore)
		if err != nil {
			return nil, fmt.Errorf("server: install metrics recorder: %w", err)
		}
		cmdSrv.metricsShutdown = metricsShutdown
	}
	peerRecords := kv.Prefixed(gizServer.PeerStore, kv.Key{"records"})
	if err := bootstrapEdgeNodes(context.Background(), &runtimepeer.Server{Store: peerRecords}, cfg.EdgeNodes); err != nil {
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
	if policy, ok := opts.SecurityPolicy.(interface {
		AllowGatewaySCTP(giznet.PublicKey) bool
	}); ok {
		listenConfig.GatewaySCTPPeer = func(_ context.Context, publicKey giznet.PublicKey) bool {
			return policy.AllowGatewaySCTP(publicKey)
		}
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
	host, _, err := net.SplitHostPort(cfg.WebRTC.Endpoint)
	if err != nil {
		return ""
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsUnspecified() {
		return ""
	}
	return cfg.WebRTC.Endpoint
}

func serviceStoreReferenceError(path, name, capability string, err error) error {
	return fmt.Errorf("server: %s %q requires %s: %w", path, name, capability, err)
}

func resolveKVStore(registry *stores.Stores, path, name string) (kv.Store, error) {
	store, err := registry.KV(name)
	if err != nil {
		return nil, serviceStoreReferenceError(path, name, "kv.Store", err)
	}
	return store, nil
}

func resolveObjectStore(registry *stores.Stores, path, name string) (objectstore.ObjectStore, error) {
	store, err := registry.ObjectStore(name)
	if err != nil {
		return nil, serviceStoreReferenceError(path, name, "objectstore.ObjectStore", err)
	}
	return store, nil
}

func resolveProfilingStore(registry *stores.Stores, cfg ProfilingConfig) (objectstore.ObjectStore, error) {
	if cfg.Store == "" {
		return nil, nil
	}
	return resolveObjectStore(registry, "profiling.store", cfg.Store)
}

func resolveSQLStore(registry *stores.Stores, path, name string) (*sqlx.DB, error) {
	store, err := registry.SQL(name)
	if err != nil {
		return nil, serviceStoreReferenceError(path, name, "*sqlx.DB", err)
	}
	return store, nil
}

func resolveMutableLogStore(registry *stores.Stores, path, name string) (logstore.MutableStore, error) {
	store, err := registry.MutableLog(name)
	if err != nil {
		return nil, serviceStoreReferenceError(path, name, "logstore.MutableStore", err)
	}
	return store, nil
}

func resolveMutableRecordStore(registry *stores.Stores, path, name string) (logstore.MutableRecordStore, error) {
	store, err := resolveMutableLogStore(registry, path, name)
	if err != nil {
		return nil, err
	}
	records, ok := store.(logstore.MutableRecordStore)
	if !ok {
		return nil, serviceStoreReferenceError(path, name, "logstore.MutableRecordStore", errors.New("exact record reads are unsupported"))
	}
	return records, nil
}

func resolveMetricsStore(registry *stores.Stores, path, name string) (metrics.Store, error) {
	store, err := registry.Metrics(name)
	if err != nil {
		return nil, serviceStoreReferenceError(path, name, "metrics.Store", err)
	}
	return store, nil
}

func configureServiceStores(server *gizclaw.Server, registry *stores.Stores, cfg *ServicesConfig) error {
	var err error
	peerRoot, err := resolveKVStore(registry, "services.peer.store", cfg.Peer.Store)
	if err != nil {
		return err
	}
	server.PeerStore = peerRoot
	server.PeerRunStore, err = resolveKVStore(registry, "services.peer_run.store", cfg.PeerRun.Store)
	if err != nil {
		return err
	}
	server.APIKeyStore, err = resolveKVStore(registry, "services.api_key.store", cfg.APIKey.Store)
	if err != nil {
		return err
	}
	server.CredentialStore, err = resolveKVStore(registry, "services.credential.store", cfg.Credential.Store)
	if err != nil {
		return err
	}
	server.FirmwareStore, err = resolveKVStore(registry, "services.firmware.store", cfg.Firmware.Store)
	if err != nil {
		return err
	}
	server.RuntimeProfileStore, err = resolveKVStore(registry, "services.runtime_profile.store", cfg.RuntimeProfile.Store)
	if err != nil {
		return err
	}
	server.ModelStore, err = resolveKVStore(registry, "services.model.store", cfg.Model.Store)
	if err != nil {
		return err
	}
	server.VoiceStore, err = resolveKVStore(registry, "services.voice.store", cfg.Voice.Store)
	if err != nil {
		return err
	}
	server.MemoryLayoutStore, err = resolveKVStore(registry, "services.memory_layout.store", cfg.MemoryLayout.Store)
	if err != nil {
		return err
	}
	providerRoot, err := resolveKVStore(registry, "services.provider_tenants.store", cfg.ProviderTenants.Store)
	if err != nil {
		return err
	}
	server.ProviderTenantStore = providerRoot
	server.WorkflowStore, err = resolveKVStore(registry, "services.workflow.store", cfg.Workflow.Store)
	if err != nil {
		return err
	}
	server.WorkspaceStore, err = resolveKVStore(registry, "services.workspace.store", cfg.Workspace.Store)
	if err != nil {
		return err
	}
	server.WorkspaceHistory, err = resolveMutableRecordStore(registry, "services.workspace.history_store", cfg.Workspace.HistoryStore)
	if err != nil {
		return err
	}
	server.WorkspaceHistoryAssets, err = resolveObjectStore(registry, "services.workspace.history_assets_store", cfg.Workspace.HistoryAssetsStore)
	if err != nil {
		return err
	}
	historyTTL, err := registry.TTL(cfg.Workspace.HistoryStore)
	if err != nil {
		return err
	}
	assetTTL, err := registry.TTL(cfg.Workspace.HistoryAssetsStore)
	if err != nil {
		return err
	}
	if historyTTL <= 0 || historyTTL != assetTTL {
		return errors.New("server: services.workspace history Store TTLs must be equal and positive")
	}
	server.WorkspaceAssets, err = resolveObjectStore(registry, "services.workspace.assets_store", cfg.Workspace.AssetsStore)
	if err != nil {
		return err
	}
	server.ToolStore, err = resolveKVStore(registry, "services.toolkit.store", cfg.Toolkit.Store)
	if err != nil {
		return err
	}
	server.ContactStore, err = resolveKVStore(registry, "services.contact.store", cfg.Contact.Store)
	if err != nil {
		return err
	}
	friendRoot, err := resolveKVStore(registry, "services.friend.store", cfg.Friend.Store)
	if err != nil {
		return err
	}
	server.FriendStore = friendRoot
	friendGroupRoot, err := resolveKVStore(registry, "services.friend_group.store", cfg.FriendGroup.Store)
	if err != nil {
		return err
	}
	server.FriendGroupStore = friendGroupRoot
	gameplayRoot, err := resolveKVStore(registry, "services.gameplay.store", cfg.Gameplay.Store)
	if err != nil {
		return err
	}
	server.GameplayStore = gameplayRoot
	server.GameplayAssets, err = resolveObjectStore(registry, "services.gameplay.assets_store", cfg.Gameplay.AssetsStore)
	if err != nil {
		return err
	}
	server.GameplayDB, err = resolveSQLStore(registry, "services.gameplay.database_store", cfg.Gameplay.DatabaseStore)
	if err != nil {
		return err
	}
	if cfg.AgentHost != nil {
		if cfg.AgentHost.RuntimeStore != "" {
			server.AgentHostStore, err = resolveObjectStore(registry, "services.agent_host.runtime_store", cfg.AgentHost.RuntimeStore)
			if err != nil {
				return err
			}
		}
		if flowcraft := cfg.AgentHost.Flowcraft; flowcraft != nil {
			if flowcraft.StateStore != "" {
				server.FlowcraftState, err = resolveKVStore(registry, "services.agent_host.flowcraft.state_store", flowcraft.StateStore)
				if err != nil {
					return err
				}
			}
			if flowcraft.HistoryStore != "" {
				server.FlowcraftHistory, err = resolveMutableLogStore(registry, "services.agent_host.flowcraft.history_store", flowcraft.HistoryStore)
				if err != nil {
					return err
				}
			}
		}
	}
	if cfg.Metrics != nil {
		server.MetricsStore, err = resolveMetricsStore(registry, "services.metrics.store", cfg.Metrics.Store)
		if err != nil {
			return err
		}
	}
	if cfg.SystemLog != nil && cfg.SystemLog.QueryStore != "" {
		queryStore, err := registry.Log(cfg.SystemLog.QueryStore)
		if err != nil {
			return serviceStoreReferenceError("services.system_log.query_store", cfg.SystemLog.QueryStore, "logstore.ImmutableStore", err)
		}
		server.ServerLogQuery, err = gizclaw.NewServerLogQueryService(queryStore)
		if err != nil {
			return fmt.Errorf("server: initialize log query service: %w", err)
		}
	}
	if cfg.SystemLog != nil {
		for index, sink := range cfg.SystemLog.Sinks {
			if sink.Kind != gizlog.SinkStore {
				continue
			}
			if _, err := registry.Log(sink.Store); err != nil {
				path := fmt.Sprintf("services.system_log.sinks[%d].store", index)
				return serviceStoreReferenceError(path, sink.Store, "logstore.ImmutableStore", err)
			}
		}
	}
	return nil
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

func newStoreRegistry(cfg Config) (*stores.Stores, *storage.Storage, error) {
	physical, err := storage.New(cfg.Storage)
	if err != nil {
		return nil, nil, err
	}
	ss, err := stores.New(cfg.Stores, physical)
	if err != nil {
		return nil, nil, errors.Join(err, physical.Close())
	}
	return ss, physical, nil
}
