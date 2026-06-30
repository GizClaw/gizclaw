package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/serverpublic"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizwebrtc"
)

type serverPublicContentTypeContextKey struct{}

func withServerPublicContentType(ctx context.Context, contentType string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(contentType) == "" {
		return ctx
	}
	return context.WithValue(ctx, serverPublicContentTypeContextKey{}, contentType)
}

func serverPublicContentType(ctx context.Context) string {
	value, _ := ctx.Value(serverPublicContentTypeContextKey{}).(string)
	return value
}

func (s *serverPublic) CreateGiznetWebRTCOffer(ctx context.Context, request serverpublic.CreateGiznetWebRTCOfferRequestObject) (serverpublic.CreateGiznetWebRTCOfferResponseObject, error) {
	var handler http.Handler
	if s != nil && s.WebRTCSignalingHandler != nil {
		handler = s.WebRTCSignalingHandler()
	}
	if handler == nil {
		return serverpublic.CreateGiznetWebRTCOffer503JSONResponse{Error: "webrtc_signaling_listener_unavailable"}, nil
	}
	body := request.Body
	if body == nil {
		body = bytes.NewReader(nil)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, gizwebrtc.SignalingPath, body)
	if err != nil {
		return nil, err
	}
	contentType := serverPublicContentType(ctx)
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

func createGiznetWebRTCOfferResponse(status int, body []byte) (serverpublic.CreateGiznetWebRTCOfferResponseObject, error) {
	if status == http.StatusOK {
		return serverpublic.CreateGiznetWebRTCOffer200ApplicationoctetStreamResponse{
			Body:          bytes.NewReader(body),
			ContentLength: int64(len(body)),
		}, nil
	}
	payload := signalingErrorPayload(status, body)
	switch status {
	case http.StatusBadRequest:
		return serverpublic.CreateGiznetWebRTCOffer400JSONResponse(payload), nil
	case http.StatusUnauthorized:
		return serverpublic.CreateGiznetWebRTCOffer401JSONResponse(payload), nil
	case http.StatusForbidden:
		return serverpublic.CreateGiznetWebRTCOffer403JSONResponse(payload), nil
	case http.StatusConflict:
		return serverpublic.CreateGiznetWebRTCOffer409JSONResponse(payload), nil
	case http.StatusRequestEntityTooLarge:
		return serverpublic.CreateGiznetWebRTCOffer413JSONResponse(payload), nil
	case http.StatusUnsupportedMediaType:
		return serverpublic.CreateGiznetWebRTCOffer415JSONResponse(payload), nil
	case http.StatusInternalServerError:
		return serverpublic.CreateGiznetWebRTCOffer500JSONResponse(payload), nil
	case http.StatusServiceUnavailable:
		return serverpublic.CreateGiznetWebRTCOffer503JSONResponse(payload), nil
	default:
		return nil, fmt.Errorf("gizclaw: unsupported webrtc signaling status %d: %s", status, strings.TrimSpace(string(body)))
	}
}

func signalingErrorPayload(status int, body []byte) serverpublic.GiznetWebRTCSignalingError {
	var payload serverpublic.GiznetWebRTCSignalingError
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
