package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

type peerHTTPContentTypeContextKey struct{}

func withPeerHTTPContentType(ctx context.Context, contentType string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(contentType) == "" {
		return ctx
	}
	return context.WithValue(ctx, peerHTTPContentTypeContextKey{}, contentType)
}

func peerHTTPContentType(ctx context.Context) string {
	value, _ := ctx.Value(peerHTTPContentTypeContextKey{}).(string)
	return value
}

func (s *peerHTTP) CreateGiznetWebRTCOffer(ctx context.Context, request peerhttp.CreateGiznetWebRTCOfferRequestObject) (peerhttp.CreateGiznetWebRTCOfferResponseObject, error) {
	var handler http.Handler
	if s != nil && s.WebRTCSignalingHandler != nil {
		handler = s.WebRTCSignalingHandler()
	}
	if handler == nil {
		return peerhttp.CreateGiznetWebRTCOffer503JSONResponse{Error: "webrtc_signaling_listener_unavailable"}, nil
	}
	body := request.Body
	if body == nil {
		body = bytes.NewReader(nil)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, gizwebrtc.SignalingPath, body)
	if err != nil {
		return nil, err
	}
	contentType := peerHTTPContentType(ctx)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	httpRequest.Header.Set("Content-Type", contentType)
	httpRequest.Header.Set("X-Giznet-Public-Key", request.Params.XGiznetPublicKey)
	httpRequest.Header.Set("X-Giznet-Timestamp", strconv.FormatInt(request.Params.XGiznetTimestamp, 10))
	httpRequest.Header.Set("X-Giznet-Nonce", request.Params.XGiznetNonce)

	recorder := newSignalingResponseRecorder()
	handler.ServeHTTP(recorder, httpRequest)
	return createGiznetWebRTCOfferResponse(recorder.status(), recorder.body.Bytes())
}

type signalingResponseRecorder struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int
	wrote      bool
}

func newSignalingResponseRecorder() *signalingResponseRecorder {
	return &signalingResponseRecorder{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (r *signalingResponseRecorder) Header() http.Header {
	return r.header
}

func (r *signalingResponseRecorder) WriteHeader(statusCode int) {
	if r.wrote {
		return
	}
	r.statusCode = statusCode
	r.wrote = true
}

func (r *signalingResponseRecorder) Write(p []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	return r.body.Write(p)
}

func (r *signalingResponseRecorder) status() int {
	if r.statusCode == 0 {
		return http.StatusOK
	}
	return r.statusCode
}

func createGiznetWebRTCOfferResponse(status int, body []byte) (peerhttp.CreateGiznetWebRTCOfferResponseObject, error) {
	if status == http.StatusOK {
		return peerhttp.CreateGiznetWebRTCOffer200ApplicationoctetStreamResponse{
			Body:          bytes.NewReader(body),
			ContentLength: int64(len(body)),
		}, nil
	}
	payload := signalingErrorPayload(status, body)
	switch status {
	case http.StatusBadRequest:
		return peerhttp.CreateGiznetWebRTCOffer400JSONResponse(payload), nil
	case http.StatusUnauthorized:
		return peerhttp.CreateGiznetWebRTCOffer401JSONResponse(payload), nil
	case http.StatusForbidden:
		return peerhttp.CreateGiznetWebRTCOffer403JSONResponse(payload), nil
	case http.StatusConflict:
		return peerhttp.CreateGiznetWebRTCOffer409JSONResponse(payload), nil
	case http.StatusRequestEntityTooLarge:
		return peerhttp.CreateGiznetWebRTCOffer413JSONResponse(payload), nil
	case http.StatusUnsupportedMediaType:
		return peerhttp.CreateGiznetWebRTCOffer415JSONResponse(payload), nil
	case http.StatusInternalServerError:
		return peerhttp.CreateGiznetWebRTCOffer500JSONResponse(payload), nil
	case http.StatusServiceUnavailable:
		return peerhttp.CreateGiznetWebRTCOffer503JSONResponse(payload), nil
	default:
		return nil, fmt.Errorf("gizclaw: unsupported webrtc signaling status %d: %s", status, strings.TrimSpace(string(body)))
	}
}

func signalingErrorPayload(status int, body []byte) peerhttp.GiznetWebRTCSignalingError {
	var payload peerhttp.GiznetWebRTCSignalingError
	if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Error) != "" {
		return payload
	}
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(status)
	}
	if message == "" {
		message = "signaling_failed"
	}
	payload.Error = message
	return payload
}
