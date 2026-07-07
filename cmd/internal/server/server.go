package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

var BuildCommit = "dev"

// CmdServer owns the command-layer store registry for a gizclaw server.
type CmdServer struct {
	*gizclaw.Server
	AdminPublicKey giznet.PublicKey
	stores         *stores.Stores
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
	if s.stores != nil {
		errs = append(errs, s.stores.Close())
		s.stores = nil
	}
	return errors.Join(errs...)
}

func (s *CmdServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.Server == nil {
		http.NotFound(w, r)
		return
	}
	s.Server.ServeHTTP(w, r)
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
	defer func() {
		if err != nil {
			err = errors.Join(err, openedStores.Close())
		}
	}()

	peersKV, err := ss.KV(defaultPeersStore)
	if err != nil {
		return nil, fmt.Errorf("server: peers store: %w", err)
	}

	cmdSrv := &CmdServer{stores: ss, AdminPublicKey: cfg.AdminPublicKey}
	var gizServer *gizclaw.Server
	gizServer = &gizclaw.Server{
		LocalStatic:    *cfg.KeyPair,
		PeerStore:      peersKV,
		BuildCommit:    BuildCommit,
		PublicEndpoint: cfg.Endpoint,
		PublicICETCP:   newOpts.ICETCPListener != nil,
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
	if len(cfg.Storage) > 0 {
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
	return cmdSrv, nil
}

func webRTCListenConfig(cfg Config, opts gizclaw.PeerListenerOptions, iceTCPListener net.Listener) gizwebrtc.ListenConfig {
	publicAddr := publicICEAddr(cfg)
	return gizwebrtc.ListenConfig{
		ICEUDPAddr:       cfg.ICEListenAddr(),
		ICETCPListener:   iceTCPListener,
		PublicICEUDPAddr: publicAddr,
		PublicICETCPAddr: publicAddr,
		SecurityPolicy:   opts.SecurityPolicy,
		PeerEventHandler: opts.PeerEventHandler,
	}
}

func publicICEAddr(cfg Config) string {
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
	return service == gizclaw.ServiceAdmin && publicKey == p.PublicKey
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
