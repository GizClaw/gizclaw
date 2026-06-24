package gizclaw

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/openaiservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/openaiapi"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizhttp"
)

func (h *PeerConn) serveOpenAI() error {
	handler := h.openAIHTTPHandler()
	server := gizhttp.NewServer(h.Conn, ServiceOpenAI, handler)
	defer func() {
		_ = server.Shutdown(context.Background())
	}()
	return server.Serve()
}

func (h *PeerConn) openAIHTTPHandler() http.Handler {
	h.initPeerGenX()

	var svc openaiapi.Server
	if h != nil && h.Conn != nil {
		svc.Caller = h.Conn.PublicKey()
	}
	if h != nil && h.Service != nil && h.Service.manager != nil {
		svc.Authorizer = h.peerAuthorizer()
		resources := h.peerResources()
		svc.Models = resources
		svc.Voices = resources
	}
	if h != nil && h.serverGenX != nil {
		svc.Generator = h.serverGenX.Generator()
		svc.Transformer = h.serverGenX.Transformer()
	}
	return newOpenAIHTTPHandler(&svc)
}

func newOpenAIHTTPHandler(svc *openaiapi.Server) http.Handler {
	app := fiber.New(fiber.Config{DisableStartupMessage: true, StreamRequestBody: true})
	openaiservice.RegisterHandlersWithOptions(app, openaiservice.NewStrictHandler(svc, nil), openaiservice.FiberServerOptions{
		BaseURL: "/v1",
	})
	app.Get("/v1/voices", func(c *fiber.Ctx) error {
		params := adminservice.ListVoicesParams{}
		if source := c.Query("source"); source != "" {
			value := adminservice.VoiceSource(source)
			params.Source = &value
		}
		if providerKind := c.Query("providerKind", c.Query("provider_kind")); providerKind != "" {
			value := adminservice.VoiceProviderKind(providerKind)
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
	})
	return fiberHTTPHandler(app)
}

type openAIVoiceListResponse struct {
	Object     string           `json:"object"`
	Data       []apitypes.Voice `json:"data"`
	HasNext    bool             `json:"has_next"`
	NextCursor *string          `json:"next_cursor,omitempty"`
}
