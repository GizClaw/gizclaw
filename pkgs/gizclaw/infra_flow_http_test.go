package gizclaw

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/infraflow"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestEdgeHTTPInfraFeedUsesReadTokenWithoutDeviceAPIKey(t *testing.T) {
	feed, err := infraflow.New(kv.NewMemory(nil))
	if err != nil {
		t.Fatal(err)
	}
	token := "gizclaw_mk_" + strings.Repeat("x", 32)
	service := &PeerService{public: &peerHTTP{PeerHTTPService: &peer.Server{}, APIKeys: apikey.NewServer(kv.NewMemory(nil))}, infraFlowHandler: infraflow.Handler(infraflow.Config{Store: "flows", ReadToken: token}, feed, nil)}
	handler := service.edgeHTTPHandlerForPeer(service.public.APIKeys, "")
	for _, test := range []struct {
		token string
		want  int
	}{{token, 200}, {"gizclaw_sk_v1_device", 401}, {"", 401}} {
		request := httptest.NewRequest(http.MethodGet, infraflow.Path+"?environment=prod", nil)
		request.Header.Set("Authorization", "Bearer "+test.token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("edge feed status %d, want %d: %s", response.Code, test.want, response.Body.String())
		}
	}
}
