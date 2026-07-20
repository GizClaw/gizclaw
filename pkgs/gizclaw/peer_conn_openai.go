package gizclaw

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/openaihttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/openaiapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
)

func (h *PeerConn) serveOpenAI() error {
	handler := h.openAIHTTPHandler()
	server := gizhttp.NewServer(h.Conn, ServicePeerOpenAI, handler)
	defer func() {
		_ = server.Shutdown(context.Background())
	}()
	return server.Serve()
}

func (h *PeerConn) openAIHTTPHandler() http.Handler {
	h.initPeerGenX()

	if h != nil && h.Conn != nil && h.Service != nil {
		return observeHTTPHandler(h.Service.openAIHTTPHandlerForPeer(h.Conn.PublicKey(), h.serverGenX, h.peerResources()), httpObservationOptions{
			surface:       observability.SurfacePeerOpenAI,
			peerPublicKey: h.Conn.PublicKey().String(),
		})
	}
	return observeHTTPHandler(newOpenAIHTTPHandler(&openaiapi.Server{}), httpObservationOptions{surface: observability.SurfacePeerOpenAI})
}

func (s *PeerService) openAIHTTPHandlerForPeer(publicKey giznet.PublicKey, genxSvc *peergenx.Service, resources *peerresource.Server) http.Handler {
	var svc openaiapi.Server
	svc.Caller = publicKey
	if s != nil && s.manager != nil {
		if resources == nil {
			resources = s.peerResources(publicKey)
		}
		svc.Models = resources
		svc.Voices = resources
		if genxSvc == nil && s.manager.Models != nil && s.manager.Voices != nil && s.manager.Credentials != nil && s.manager.ProviderTenants != nil {
			genxSvc = peergenx.New(peergenx.Service{
				Peer:            peerPublicKey(publicKey),
				Models:          resources,
				Voices:          resources,
				Credentials:     s.manager.Credentials,
				ProviderTenants: s.manager.ProviderTenants,
			})
		}
	}
	if genxSvc != nil {
		svc.Generator = genxSvc.Generator()
		svc.Transformer = genxSvc.Transformer()
	}
	return newOpenAIHTTPHandler(&svc)
}

func (s *PeerService) peerResources(publicKey giznet.PublicKey) *peerresource.Server {
	return s.peerResourcesWithRegistration(publicKey, nil, true)
}

func (s *PeerService) peerResourcesForHTTPSession(publicKey giznet.PublicKey, registration *runtimeprofile.Registration) *peerresource.Server {
	return s.peerResourcesWithRegistration(publicKey, registration, false)
}

func (s *PeerService) peerResourcesWithRegistration(publicKey giznet.PublicKey, sessionRegistration *runtimeprofile.Registration, inheritActiveConnection bool) *peerresource.Server {
	if s == nil || s.manager == nil {
		return nil
	}
	manager := s.manager
	var snapshot *runtimeprofile.Registration
	if sessionRegistration != nil {
		value := *sessionRegistration
		snapshot = &value
	}
	registration := func() (runtimeprofile.Registration, bool) {
		if snapshot != nil {
			return *snapshot, true
		}
		if inheritActiveConnection {
			return manager.PeerRegistration(publicKey)
		}
		return runtimeprofile.Registration{}, false
	}
	return &peerresource.Server{
		Caller:       publicKey,
		Workspaces:   manager.Workspaces,
		Workflows:    manager.Workflows,
		Models:       manager.Models,
		Voices:       manager.Voices,
		Contacts:     manager.Contacts,
		Friends:      manager.Friends,
		FriendGroups: manager.FriendGroups,
		Gameplay:     manager.Gameplay,
		Tools:        manager.Tools,
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			registration, ok := registration()
			if !ok {
				return nil
			}
			if manager.RuntimeProfiles == nil {
				return nil
			}
			profile, err := manager.RuntimeProfiles.ResolveProfile(context.Background(), registration.RuntimeProfile.Name)
			if err != nil {
				return nil
			}
			return &profile
		},
	}
}

type peerPublicKey giznet.PublicKey

func (p peerPublicKey) PublicKey() giznet.PublicKey {
	return giznet.PublicKey(p)
}

func newOpenAIHTTPHandler(svc *openaiapi.Server) http.Handler {
	app := fiber.New(fiber.Config{DisableStartupMessage: true, StreamRequestBody: true})
	app.Use(observeFiberRoute)
	openaihttp.RegisterHandlersWithOptions(app, openaihttp.NewStrictHandler(svc, nil), openaihttp.FiberServerOptions{
		BaseURL: "/v1",
	})
	app.Get("/v1/voices", func(c *fiber.Ctx) error {
		params := adminhttp.ListVoicesParams{}
		if source := c.Query("source"); source != "" {
			value := adminhttp.VoiceSource(source)
			params.Source = &value
		}
		if providerKind := c.Query("providerKind", c.Query("provider_kind")); providerKind != "" {
			value := adminhttp.VoiceProviderKind(providerKind)
			params.ProviderKind = &value
		}
		if providerName := c.Query("providerName", c.Query("provider_name")); providerName != "" {
			params.ProviderName = &providerName
		}
		if cursor := c.Query("cursor"); cursor != "" {
			params.Cursor = &cursor
		}
		if limitText := c.Query("limit"); limitText != "" {
			limit, err := strconv.ParseInt(limitText, 10, 32)
			if err != nil {
				return c.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid limit"})
			}
			limit32 := int32(limit)
			params.Limit = &limit32
		}
		list, err := svc.ListVoices(c.UserContext(), params)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(map[string]string{"error": err.Error()})
		}
		return c.JSON(openAIVoiceListResponse{
			Object:     "list",
			Data:       list.Items,
			HasNext:    list.HasNext,
			NextCursor: list.NextCursor,
		})
	}).Name("ListVoices")
	return fiberHTTPHandler(app)
}

type openAIVoiceListResponse struct {
	Object     string           `json:"object"`
	Data       []apitypes.Voice `json:"data"`
	HasNext    bool             `json:"has_next"`
	NextCursor *string          `json:"next_cursor,omitempty"`
}
