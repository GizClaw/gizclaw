package gizclaw

import (
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
)

func (s *Server) peerOpenAIHTTPHandler(apiKeys *apikey.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setPublicHTTPCORSHeaders(w.Header())
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		principal, ok := authenticateHTTPAPIKey(w, r, apiKeys)
		if !ok {
			return
		}
		publicKey, err := parsePeerPublicKey(principal.Key.Owner)
		if err != nil {
			writeHTTPAPIKeyError(w, err)
			return
		}
		if s == nil || s.peerService == nil {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		if err := s.peerService.validateAPIKeyOwner(r.Context(), publicKey); err != nil {
			writeHTTPAPIKeyOwnerError(w, err)
			return
		}
		observability.SetPeer(r.Context(), publicKey.String(), "")
		resources := s.peerService.peerResourcesForAPIKey(publicKey)
		http.StripPrefix("/openai", s.peerService.openAIHTTPHandlerForPeer(publicKey, nil, resources)).ServeHTTP(w, r)
	})
}
