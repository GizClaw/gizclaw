package gizclaw

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/credential"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/firmware"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/model"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peer"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peerrun"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/providertenants"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/resourcemanager"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/voice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workflow"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workspace"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

// Server holds peer transport configuration. Per-stream protocol handling can be
// extended later.
//
// Set peer storage config on the struct, then call ListenAndServe.
// Internal runtime state is built automatically on first ListenAndServe.
type Server struct {
	LocalStatic giznet.KeyPair
	ListenAddr  string
	CipherMode  giznet.CipherMode

	SecurityPolicy giznet.SecurityPolicy

	PeerStore              kv.Store
	PeerRunStore           kv.Store
	CredentialStore        kv.Store
	FirmwareStore          kv.Store
	MiniMaxCredentialStore kv.Store
	MiniMaxTenantStore     kv.Store
	VolcTenantStore        kv.Store
	ModelStore             kv.Store
	VoiceStore             kv.Store
	WorkspaceStore         kv.Store
	WorkflowStore          kv.Store
	PublicLoginStore       kv.Store
	BuildCommit            string
	ACLDB                  *sql.DB

	manager     *Manager
	peerService *PeerService
	sessions    *publiclogin.SessionManager
	listener    *giznet.Listener
	httpHandler http.Handler
}

// ServeHTTP exposes server-public APIs over ordinary HTTP.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.httpHandler.ServeHTTP(w, r)
}

// Listen initializes the server runtime and binds the UDP peer listener.
func (s *Server) Listen() error {
	if s == nil {
		return errors.New("gizclaw: nil server")
	}
	if s.listener != nil {
		return nil
	}
	if err := s.init(); err != nil {
		return err
	}
	cfg := giznet.ListenConfig{
		Addr:             s.ListenAddr,
		CipherMode:       s.CipherMode,
		SecurityPolicy:   (*ServerSecurityPolicy)(s),
		PeerEventHandler: (*serverPeerEventHandler)(s),
	}
	l, err := (&cfg).Listen(&s.LocalStatic)
	if err != nil {
		return err
	}
	s.listener = l
	return nil
}

// Serve blocks serving accepted peer connections from a listener created by Listen.
func (s *Server) Serve() error {
	l := s.listener
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
		host := &GearConn{
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
	switch ev.State {
	case giznet.PeerStateOffline:
		(*Server)(h).manager.SetPeerDown(ev.PublicKey)
	}
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
	listener := s.listener
	var errs []error
	if listener != nil {
		errs = append(errs, listener.Close())
	}
	return errors.Join(errs...)
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
		s.MiniMaxTenantStore == nil &&
		s.VolcTenantStore == nil &&
		s.ModelStore == nil &&
		s.VoiceStore == nil &&
		s.MiniMaxCredentialStore == nil &&
		s.WorkspaceStore == nil &&
		s.WorkflowStore == nil &&
		s.PeerRunStore == nil &&
		s.PublicLoginStore == nil
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

	publicLoginServer := publiclogin.NewServer(&s.LocalStatic, publicLoginStore)
	sessions := publicLoginServer.SessionManager()
	peersServer := &peer.Server{
		Store:           peerStore,
		BuildCommit:     s.BuildCommit,
		ServerPublicKey: s.LocalStatic.Public,
	}
	manager := NewManager(peersServer)
	manager.PeerRun = &peerrun.Server{Store: peerRunStore}
	peersServer.PeerManager = manager

	workflowServer := &workflow.Server{Store: workflowStore}
	workspaceServer := &workspace.Server{Store: workspaceStore, WorkflowStore: workflowStore}
	credentialServer := &credential.Server{Store: credentialStore}
	firmwareServer := &firmware.Server{Store: firmwareStore}
	modelServer := &model.Server{Store: modelStore}
	voiceServer := &voice.Server{Store: voiceStore}
	providerTenantsServer := &providertenants.Server{
		ModelStore:      modelStore,
		TenantStore:     miniMaxTenantStore,
		VolcTenantStore: volcTenantStore,
		VoiceStore:      voiceStore,
		CredentialStore: miniMaxCredentialStore,
	}
	var aclServer *acl.Server
	if s.ACLDB != nil {
		aclServer = &acl.Server{DB: s.ACLDB}
	}
	resourceManager := resourcemanager.New(resourcemanager.Services{
		ACL:             aclServer,
		Credentials:     credentialServer,
		Firmwares:       firmwareServer,
		Peers:           peersServer,
		Models:          modelServer,
		ProviderTenants: providerTenantsServer,
		Voices:          voiceServer,
		Workspaces:      workspaceServer,
		Workflows:       workflowServer,
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
			ACL:                         aclServer,
			ResourceManager:             resourceManager,
		},
		public: &serverPublic{
			ServerPublicService: peersServer,
			ServerPublic:        publicLoginServer,
		},
	}
	s.sessions = sessions
	mux := http.NewServeMux()
	mux.Handle("/api/public/", http.StripPrefix("/api/public", s.peerService.publicHTTPHandler(sessions)))
	mux.HandleFunc("/api/public", redirectProxyPrefix("/api/public/"))
	s.httpHandler = httpLabelSetHandler(mux)
	return nil
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
