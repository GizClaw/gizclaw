package server

import (
	"errors"
	"fmt"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/storage"
	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
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

// New wires an already prepared in-memory config into a command server.
func New(cfg Config) (srv *CmdServer, err error) {
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

	peersKV, err := ss.KV(cfg.Peers.Store)
	if err != nil {
		return nil, fmt.Errorf("server: peers store: %w", err)
	}

	gizServer := &gizclaw.Server{
		LocalStatic: *cfg.KeyPair,
		ListenAddr:  cfg.ListenAddr,
		CipherMode:  cfg.CipherMode,
		PeerStore:   peersKV,
		BuildCommit: BuildCommit,
	}
	gizServer.RewardClaimGenerator = cfg.SystemTasks.RewardClaim.Generator
	gizServer.PetActionGenerator = cfg.SystemTasks.PetAction.Generator
	if cfg.SystemTasks.RewardClaim.Cooldown != "" {
		cooldown, err := time.ParseDuration(cfg.SystemTasks.RewardClaim.Cooldown)
		if err != nil {
			return nil, fmt.Errorf("server: system_tasks.reward_claim.cooldown: %w", err)
		}
		gizServer.RewardClaimCooldown = cooldown
	}
	if cfg.Friends.FriendOTPTTL != "" {
		ttl, err := parseConfigDuration(cfg.Friends.FriendOTPTTL)
		if err != nil {
			return nil, fmt.Errorf("server: friends.friend_otp_ttl: %w", err)
		}
		gizServer.FriendOTPTTL = ttl
	}
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
		if gizServer.CredentialStore, err = ss.KV(cfg.Credentials.Store); err != nil {
			return nil, fmt.Errorf("server: credentials store: %w", err)
		}
		if gizServer.FirmwareStore, err = ss.KV(cfg.Firmwares.Store); err != nil {
			return nil, fmt.Errorf("server: firmwares store: %w", err)
		}
		if gizServer.MiniMaxCredentialStore, err = ss.KV(cfg.MiniMax.CredentialsStore); err != nil {
			return nil, fmt.Errorf("server: minimax credentials store: %w", err)
		}
		if gizServer.MiniMaxTenantStore, err = ss.KV(cfg.MiniMax.TenantsStore); err != nil {
			return nil, fmt.Errorf("server: minimax tenants store: %w", err)
		}
		if gizServer.VoiceStore, err = ss.KV(cfg.MiniMax.VoicesStore); err != nil {
			return nil, fmt.Errorf("server: voices store: %w", err)
		}
		if gizServer.WorkspaceStore, err = ss.KV(cfg.Workspaces.Store); err != nil {
			return nil, fmt.Errorf("server: workspaces store: %w", err)
		}
		if gizServer.WorkflowStore, err = ss.KV(cfg.Workflows.Store); err != nil {
			return nil, fmt.Errorf("server: workflows store: %w", err)
		}
		if gizServer.ACLDB, err = ss.SQL(cfg.ACL.Store); err != nil {
			return nil, fmt.Errorf("server: acl store: %w", err)
		}
		if cfg.PetSpecies.Store != "" {
			if gizServer.PetSpeciesStore, err = ss.KV(cfg.PetSpecies.Store); err != nil {
				return nil, fmt.Errorf("server: pet_species store: %w", err)
			}
		}
		if cfg.PetSpecies.AssetsStore != "" {
			if gizServer.PetSpeciesAssets, err = ss.ObjectStore(cfg.PetSpecies.AssetsStore); err != nil {
				return nil, fmt.Errorf("server: pet_species assets store: %w", err)
			}
		}
		if cfg.Badges.Store != "" {
			if gizServer.BadgeStore, err = ss.KV(cfg.Badges.Store); err != nil {
				return nil, fmt.Errorf("server: badges store: %w", err)
			}
		}
		if cfg.Badges.AssetsStore != "" {
			if gizServer.BadgeAssets, err = ss.ObjectStore(cfg.Badges.AssetsStore); err != nil {
				return nil, fmt.Errorf("server: badges assets store: %w", err)
			}
		}
		if cfg.Pets.Store != "" {
			if gizServer.PetStore, err = ss.KV(cfg.Pets.Store); err != nil {
				return nil, fmt.Errorf("server: pets store: %w", err)
			}
		}
		if cfg.Rewards.Store != "" {
			if gizServer.RewardStore, err = ss.KV(cfg.Rewards.Store); err != nil {
				return nil, fmt.Errorf("server: rewards store: %w", err)
			}
		}
		if cfg.Wallets.Store != "" {
			if gizServer.WalletDB, err = ss.SQL(cfg.Wallets.Store); err != nil {
				return nil, fmt.Errorf("server: wallets store: %w", err)
			}
		}
		if cfg.Contacts.Store != "" {
			if gizServer.ContactStore, err = ss.KV(cfg.Contacts.Store); err != nil {
				return nil, fmt.Errorf("server: contacts store: %w", err)
			}
		}
		if cfg.Friends.RequestsStore != "" {
			if gizServer.FriendRequestStore, err = ss.KV(cfg.Friends.RequestsStore); err != nil {
				return nil, fmt.Errorf("server: friend requests store: %w", err)
			}
		}
		if cfg.Friends.Store != "" {
			if gizServer.FriendStore, err = ss.KV(cfg.Friends.Store); err != nil {
				return nil, fmt.Errorf("server: friends store: %w", err)
			}
		}
		if cfg.FriendGroups.Store != "" {
			if gizServer.FriendGroupStore, err = ss.KV(cfg.FriendGroups.Store); err != nil {
				return nil, fmt.Errorf("server: friend_groups store: %w", err)
			}
		}
		if cfg.FriendGroups.MembersStore != "" {
			if gizServer.FriendGroupMemberStore, err = ss.KV(cfg.FriendGroups.MembersStore); err != nil {
				return nil, fmt.Errorf("server: friend group members store: %w", err)
			}
		}
		if cfg.FriendGroups.MessagesStore != "" {
			if gizServer.FriendGroupMessageStore, err = ss.KV(cfg.FriendGroups.MessagesStore); err != nil {
				return nil, fmt.Errorf("server: friend group messages store: %w", err)
			}
		}
		if cfg.FriendGroups.MessageAssetsStore != "" {
			if gizServer.FriendGroupMessageAssets, err = ss.ObjectStore(cfg.FriendGroups.MessageAssetsStore); err != nil {
				return nil, fmt.Errorf("server: friend group message assets store: %w", err)
			}
		}
	}
	return &CmdServer{Server: gizServer, AdminPublicKey: cfg.AdminPublicKey, stores: ss}, nil
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
