package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/idy/ai-server-shell/backend"
	shellopenai "github.com/idy/ai-server-shell/openai"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/openaiapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
)

func (h *PeerConn) serveOpenAI() error {
	handler := rejectRetiringHTTP(h.isRetiring, h.openAIHTTPHandler())
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
		adapter := openAIWorkspaceAdapter{caller: publicKey, manager: s.manager, resources: resources}
		svc.Workspaces = adapter
		svc.Executor = adapter
		svc.Responses = s.openAIResponseRuntime()
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
	if s == nil {
		return newOpenAIHTTPHandler(&svc)
	}
	protocol, err := s.openAIProtocolHandler()
	if err != nil {
		return openAIUnavailableHandler()
	}
	return bindOpenAIHTTPHandler(protocol, &svc)
}

func (s *PeerService) openAIResponseRuntime() *openaiapi.ResponseRuntime {
	if s == nil {
		return nil
	}
	s.openAIResponseOnce.Do(func() { s.openAIResponses = openaiapi.NewResponseRuntime() })
	return s.openAIResponses
}

func (s *PeerService) closeOpenAIResponses(ctx context.Context) error {
	if s == nil || s.openAIResponses == nil {
		return nil
	}
	return s.openAIResponses.Close(ctx)
}

func (s *PeerService) peerResources(publicKey giznet.PublicKey) *peerresource.Server {
	return s.peerResourcesWithRegistration(publicKey, true)
}

func (s *PeerService) peerResourcesForAPIKey(publicKey giznet.PublicKey) *peerresource.Server {
	return s.peerResourcesWithRegistration(publicKey, false)
}

func (s *PeerService) peerResourcesWithRegistration(publicKey giznet.PublicKey, inheritActiveConnection bool) *peerresource.Server {
	if s == nil || s.manager == nil {
		return nil
	}
	manager := s.manager
	return &peerresource.Server{
		Caller:       publicKey,
		Peers:        manager.Peers,
		Firmwares:    manager.Firmwares,
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
			if inheritActiveConnection {
				if _, ok := manager.PeerRegistration(publicKey); !ok {
					return nil
				}
			}
			if manager.RuntimeProfiles == nil {
				return nil
			}
			profile, err := manager.RuntimeProfiles.ResolveOwnerProfile(context.Background(), publicKey.String())
			if err != nil {
				return nil
			}
			return &profile
		},
	}
}

// deviceReadsForAPIKey builds the owner-scoped device projection used by the
// Public HTTP device extension. Reads never contact the device.
func (s *PeerService) deviceReadsForAPIKey(publicKey giznet.PublicKey) peerresource.DeviceReads {
	if s == nil || s.manager == nil {
		return peerresource.DeviceReads{Caller: publicKey}
	}
	reads := peerresource.DeviceReads{Caller: publicKey}
	if s.manager.Peers != nil {
		reads.Info = s.manager.Peers
	}
	if s.manager.PeerRun != nil {
		reads.Status = s.manager.PeerRun
	}
	if s.manager.Peers != nil {
		reads.Peers = s.manager.Peers
	}
	if s.manager.Firmwares != nil {
		reads.Firmwares = s.manager.Firmwares
	}
	if s.manager.Metrics != nil {
		reads.Telemetry = &peertelemetry.AdminService{Metrics: s.manager.Metrics}
	}
	return reads
}

type peerPublicKey giznet.PublicKey

func (p peerPublicKey) PublicKey() giznet.PublicKey {
	return giznet.PublicKey(p)
}

func newOpenAIHTTPHandler(svc *openaiapi.Server) http.Handler {
	protocol, err := newOpenAIProtocolHandler()
	if err != nil {
		return openAIUnavailableHandler()
	}
	return bindOpenAIHTTPHandler(protocol, svc)
}

func (s *PeerService) openAIProtocolHandler() (http.Handler, error) {
	if s == nil {
		return nil, errors.New("gizclaw: nil PeerService")
	}
	s.openAIOnce.Do(func() {
		s.openAIProtocol, s.openAIProtocolErr = newOpenAIProtocolHandler()
	})
	return s.openAIProtocol, s.openAIProtocolErr
}

const openAIMaxBodyBytes int64 = 32 << 20

type openAIRequestBinding struct {
	caller  string
	service *openaiapi.Server
}

type openAIRequestBindingKey struct{}

type openAIProtocolHandler struct {
	shell http.Handler
}

type openAIRoute struct {
	method    string
	path      string
	operation string
	voices    bool
	queries   map[string]bool
}

var openAIRoutes = [...]openAIRoute{
	{method: http.MethodGet, path: "/v1/models", operation: "listModels"},
	{method: http.MethodPost, path: "/v1/chat/completions", operation: "createChatCompletion"},
	{method: http.MethodPost, path: "/v1/audio/speech", operation: "createSpeech"},
	{method: http.MethodPost, path: "/v1/audio/transcriptions", operation: "createTranscription"},
	{method: http.MethodGet, path: "/v1/voices", operation: "listVoices", voices: true, queries: map[string]bool{"cursor": true, "limit": true}},
	{method: http.MethodPost, path: "/v1/conversations", operation: "createConversation"},
	{method: http.MethodGet, path: "/v1/conversations/{conversation_id}", operation: "getConversation"},
	{method: http.MethodGet, path: "/v1/conversations/{conversation_id}/items", operation: "listConversationItems", queries: map[string]bool{"after": true, "limit": true, "order": true}},
	{method: http.MethodGet, path: "/v1/conversations/{conversation_id}/items/{item_id}", operation: "getConversationItem"},
	{method: http.MethodPost, path: "/v1/responses", operation: "createResponse"},
	{method: http.MethodGet, path: "/v1/responses/{response_id}", operation: "getResponse"},
	{method: http.MethodGet, path: "/v1/responses/{response_id}/input_items", operation: "listInputItems", queries: map[string]bool{"after": true, "limit": true, "order": true}},
	{method: http.MethodPost, path: "/v1/responses/{response_id}/cancel", operation: "cancelResponse"},
}

func newOpenAIProtocolHandler() (http.Handler, error) {
	dispatch := backend.HandlerFunc(func(ctx context.Context, request backend.Request) (backend.Response, error) {
		binding, ok := openAIBindingFromContext(ctx)
		if !ok || binding.service == nil {
			return backend.Response{}, &backend.Error{
				Kind: backend.ErrorUnavailable, Code: "openai_backend_unavailable",
				Message: "The OpenAI backend binding is unavailable.",
			}
		}
		return binding.service.Handle(ctx, request)
	})
	services, err := backend.NewServices(
		backend.WithModels(dispatch),
		backend.WithChat(dispatch),
		backend.WithAudio(dispatch),
		backend.WithConversations(dispatch),
		backend.WithResponses(dispatch),
	)
	if err != nil {
		return nil, err
	}
	handler, err := shellopenai.NewHandler(
		services,
		shellopenai.WithMaxBodyBytes(openAIMaxBodyBytes),
		shellopenai.WithAuthenticator(shellopenai.AuthenticatorFunc(
			func(ctx context.Context, _ shellopenai.Credential) (shellopenai.Principal, error) {
				binding, ok := openAIBindingFromContext(ctx)
				if !ok || binding.service == nil || binding.service.Caller.IsZero() || binding.caller != binding.service.Caller.String() {
					return shellopenai.Principal{}, errors.New("gizclaw: missing verified OpenAI caller binding")
				}
				return shellopenai.Principal{ID: binding.caller}, nil
			},
		)),
	)
	if err != nil {
		return nil, err
	}
	return &openAIProtocolHandler{shell: handler}, nil
}

func (h *openAIProtocolHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	route, ok := supportedOpenAIRoute(request.Method, request.URL.Path)
	if !ok {
		http.NotFound(writer, request)
		return
	}
	for key := range request.URL.Query() {
		if !route.queries[key] {
			http.NotFound(writer, request)
			return
		}
	}
	outcome := observability.FromContext(request.Context())
	outcome.SetRoute(route.path)
	outcome.SetOperation(route.operation)
	if route.voices {
		serveOpenAIVoices(writer, request)
		return
	}
	h.shell.ServeHTTP(writer, request)
}

func supportedOpenAIRoute(method, path string) (openAIRoute, bool) {
	for _, route := range openAIRoutes {
		if route.method == method && openAIRouteMatches(route.path, path) {
			return route, true
		}
	}
	return openAIRoute{}, false
}

func openAIRouteMatches(pattern, requestPath string) bool {
	if !strings.HasPrefix(requestPath, "/") || strings.HasPrefix(requestPath, "//") || (len(requestPath) > 1 && strings.HasSuffix(requestPath, "/")) {
		return false
	}
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	requestParts := strings.Split(strings.Trim(requestPath, "/"), "/")
	if len(patternParts) != len(requestParts) {
		return false
	}
	for index := range patternParts {
		if strings.HasPrefix(patternParts[index], "{") && strings.HasSuffix(patternParts[index], "}") {
			if requestParts[index] == "" {
				return false
			}
			continue
		}
		if patternParts[index] != requestParts[index] {
			return false
		}
	}
	return true
}

func bindOpenAIHTTPHandler(protocol http.Handler, service *openaiapi.Server) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if service == nil || service.Caller.IsZero() {
			protocol.ServeHTTP(writer, request)
			return
		}
		binding := openAIRequestBinding{caller: service.Caller.String(), service: service}
		ctx := context.WithValue(request.Context(), openAIRequestBindingKey{}, binding)
		protocol.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func openAIBindingFromContext(ctx context.Context) (openAIRequestBinding, bool) {
	binding, ok := ctx.Value(openAIRequestBindingKey{}).(openAIRequestBinding)
	return binding, ok && strings.TrimSpace(binding.caller) != ""
}

func serveOpenAIVoices(writer http.ResponseWriter, request *http.Request) {
	binding, ok := openAIBindingFromContext(request.Context())
	if !ok || binding.service == nil || binding.service.Caller.IsZero() || binding.caller != binding.service.Caller.String() {
		writeOpenAIVoiceError(writer, http.StatusServiceUnavailable, "voice service unavailable")
		return
	}
	params := openaiapi.VoiceListParams{}
	if cursor := request.URL.Query().Get("cursor"); cursor != "" {
		params.Cursor = &cursor
	}
	if limitText := request.URL.Query().Get("limit"); limitText != "" {
		limit, err := strconv.ParseInt(limitText, 10, 32)
		if err != nil {
			writeOpenAIVoiceError(writer, http.StatusBadRequest, "invalid limit")
			return
		}
		limit32 := int32(limit)
		params.Limit = &limit32
	}
	list, err := binding.service.ListVoices(request.Context(), params)
	if err != nil {
		writeOpenAIVoiceError(writer, http.StatusInternalServerError, "voice service failed")
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(openAIVoiceListResponse{
		Object: "list", Data: list.Items, HasNext: list.HasNext, NextCursor: list.NextCursor,
	})
}

func writeOpenAIVoiceError(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"error": message})
}

func openAIUnavailableHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
	})
}

type openAIVoiceListResponse struct {
	Object     string           `json:"object"`
	Data       []apitypes.Voice `json:"data"`
	HasNext    bool             `json:"has_next"`
	NextCursor *string          `json:"next_cursor,omitempty"`
}
