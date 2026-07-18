package gizclaw

import (
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
)

func (s *Server) peerOpenAIHTTPHandler(sessions *publiclogin.SessionManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setPublicHTTPCORSHeaders(w.Header())
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		publicKey, ok := authenticatePrimaryHTTPSession(w, r, sessions)
		if !ok {
			return
		}
		observability.SetPeer(r.Context(), publicKey.String(), "")
		if s == nil || s.peerService == nil {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		http.StripPrefix("/openai", s.peerService.openAIHTTPHandlerForPeer(publicKey, nil)).ServeHTTP(w, r)
	})
}
