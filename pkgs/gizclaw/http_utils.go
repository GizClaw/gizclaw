package gizclaw

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func authenticateFiberSession(ctx *fiber.Ctx, sessions *publiclogin.SessionManager) (giznet.PublicKey, bool) {
	principal, ok := authenticateFiberPrincipal(ctx, sessions)
	if !ok {
		return giznet.PublicKey{}, false
	}
	return principal.PublicKey, true
}

func authenticateFiberPrincipal(ctx *fiber.Ctx, sessions *publiclogin.SessionManager) (publiclogin.Principal, bool) {
	if sessions == nil {
		writeFiberSessionError(ctx, errors.New("session manager not configured"))
		return publiclogin.Principal{}, false
	}
	principal, err := sessions.AuthenticateHeadersPrincipal(ctx.Get("Authorization"), ctx.Get(publiclogin.PublicKeyHeader))
	if err != nil {
		writeFiberSessionError(ctx, err)
		return publiclogin.Principal{}, false
	}
	return principal, true
}

func writeFiberSessionError(ctx *fiber.Ctx, err error) {
	code := "INVALID_SESSION"
	message := "missing or invalid bearer session"
	if errors.Is(err, publiclogin.ErrPublicKeyMismatch) {
		code = "PUBLIC_KEY_MISMATCH"
		message = "x-public-key does not match bearer session"
	}
	ctx.Status(http.StatusUnauthorized)
	_ = ctx.JSON(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func authenticatePrimaryHTTPSession(w http.ResponseWriter, r *http.Request, sessions *publiclogin.SessionManager) (giznet.PublicKey, bool) {
	principal, err := sessions.AuthenticateHeadersPrincipal(r.Header.Get("Authorization"), r.Header.Get(publiclogin.PublicKeyHeader))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		if errors.Is(err, publiclogin.ErrPublicKeyMismatch) {
			_, _ = io.WriteString(w, `{"error":{"code":"PUBLIC_KEY_MISMATCH","message":"x-public-key does not match bearer session"}}`)
			return giznet.PublicKey{}, false
		}
		_, _ = io.WriteString(w, `{"error":{"code":"INVALID_SESSION","message":"missing or invalid bearer session"}}`)
		return giznet.PublicKey{}, false
	}
	if principal.Kind != publiclogin.SessionKindPrimary {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":"PRIMARY_SESSION_REQUIRED","message":"primary session required"}}`)
		return giznet.PublicKey{}, false
	}
	return principal.PublicKey, true
}

func setPublicHTTPCORSHeaders(header http.Header) {
	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
	header.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Public-Key,X-Giznet-Nonce,X-Giznet-Public-Key,X-Giznet-Timestamp,X-Request-ID")
	header.Set("Access-Control-Expose-Headers", "Content-Length,Content-Type,X-Request-ID")
}

const (
	requestIDHeader          = "X-Request-ID"
	maxObservedResponseBytes = 64 << 10
)

var requestIDRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
var observedErrorCodeRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

var requestIDWarningAt atomic.Int64

type httpObservationOptions struct {
	surface       observability.Surface
	peerPublicKey string
	peerRole      string
	entropy       io.Reader
}

func observeHTTPHandler(next http.Handler, opts httpObservationOptions) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		outcome := observability.NewOutcome(observability.TransportHTTP, opts.surface, "unknown")
		outcome.SetPeer(opts.peerPublicKey, opts.peerRole)
		entropy := opts.entropy
		if entropy == nil {
			entropy = rand.Reader
		}
		requestID, requestIDErr := validOrNewRequestID(request.Header.Get(requestIDHeader), entropy)
		if requestIDErr != nil {
			warnRequestIDGeneration(request.Context())
		}
		if requestID != "" {
			request.Header.Set(requestIDHeader, requestID)
			writer.Header().Set(requestIDHeader, requestID)
			outcome.SetRequestID(requestID)
		}
		ctx := observability.WithOutcome(request.Context(), outcome)
		request = request.WithContext(ctx)

		status := http.StatusOK
		wroteHeader := false
		var writeErr error
		body := make([]byte, 0, 1024)
		overflow := false
		captureBody := func(p []byte) {
			if status < http.StatusBadRequest || len(p) == 0 || overflow {
				return
			}
			remaining := maxObservedResponseBytes - len(body)
			if len(p) > remaining {
				overflow = true
				body = nil
				return
			}
			body = append(body, p...)
		}
		wrapped := httpsnoop.Wrap(writer, httpsnoop.Hooks{
			WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
				return func(code int) {
					if requestID != "" {
						writer.Header().Set(requestIDHeader, requestID)
					}
					if !wroteHeader {
						status = code
						wroteHeader = true
					}
					next(code)
				}
			},
			Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
				return func(p []byte) (int, error) {
					if requestID != "" {
						writer.Header().Set(requestIDHeader, requestID)
					}
					if !wroteHeader {
						wroteHeader = true
					}
					n, err := next(p)
					if err != nil && writeErr == nil {
						writeErr = err
					}
					captureBody(p[:n])
					return n, err
				}
			},
			ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
				return func(source io.Reader) (int64, error) {
					if requestID != "" {
						writer.Header().Set(requestIDHeader, requestID)
					}
					n, err := next(io.TeeReader(source, writerFunc(func(p []byte) (int, error) {
						captureBody(p)
						return len(p), nil
					})))
					if err != nil && writeErr == nil {
						writeErr = err
					}
					return n, err
				}
			},
		})

		defer func() {
			if requestID != "" && !wroteHeader {
				writer.Header().Set(requestIDHeader, requestID)
			}
			panicValue := recover()
			if panicValue != nil {
				outcome.MarkPanic()
				if !wroteHeader {
					status = 0
				}
			}
			result := httpObservationResult(request.Context(), status)
			if writeErr != nil {
				result = observability.ResultTransportError
			}
			fallbackRoute, fallbackOperation := registeredHTTPFallback(request)
			outcome.SetHTTPFallback(fallbackRoute, fallbackOperation)
			outcome.SetHTTP(request.Method, "", status, result)
			if status >= http.StatusBadRequest {
				outcome.SetErrorCode(observedHTTPErrorCode(writer.Header().Get("Content-Type"), body, overflow, status))
			}
			observability.Log(request.Context(), outcome)
			if panicValue != nil {
				panic(panicValue)
			}
		}()

		next.ServeHTTP(wrapped, request)
	})
}

func registeredHTTPFallback(request *http.Request) (string, string) {
	if request == nil {
		return "", ""
	}
	pattern := request.Pattern
	if pattern == "" && request.URL != nil {
		switch request.URL.Path {
		case "/login", "/server-info", "/webrtc/v1/offer", "/me", "/me/runtime", "/me/status":
			pattern = request.URL.Path
		default:
			if strings.HasPrefix(request.URL.Path, "/openai/v1/") {
				pattern = "/openai/v1/"
			}
		}
	}
	return pattern, registeredHTTPOperation(request.Method, pattern)
}

func registeredHTTPOperation(method, pattern string) string {
	if pattern == "" {
		return ""
	}
	if method == http.MethodOptions {
		switch pattern {
		case "/login", "/server-info", "/webrtc/v1/offer", "/me", "/me/runtime", "/me/status", "/openai/v1/":
			return "corsPreflight"
		default:
			return ""
		}
	}
	switch pattern {
	case "/login":
		if method == http.MethodPost {
			return "login"
		}
	case "/server-info":
		if method == http.MethodGet {
			return "getServerInfo"
		}
	case "/webrtc/v1/offer":
		if method == http.MethodPost {
			return "createGiznetWebRTCOffer"
		}
	case "/me":
		if method == http.MethodGet {
			return "getMe"
		}
	case "/me/runtime":
		if method == http.MethodGet {
			return "getMeRuntime"
		}
	case "/me/status":
		switch method {
		case http.MethodGet:
			return "getMeStatus"
		case http.MethodPut:
			return "putMeStatus"
		}
	case "/openai/v1/":
		return "openAIProxy"
	}
	return ""
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func observeFiberRoute(ctx *fiber.Ctx) error {
	middlewareRoute := ctx.Route()
	err := ctx.Next()
	outcome := observability.FromContext(ctx.UserContext())
	if outcome == nil {
		return err
	}
	route := ctx.Route()
	if route == nil || route == middlewareRoute {
		return err
	}
	outcome.SetRoute(route.Path)
	if route.Name != "" {
		outcome.SetOperation(openAPIOperationID(route.Name))
		return err
	}
	if len(route.Handlers) == 0 {
		return err
	}
	fn := runtime.FuncForPC(reflect.ValueOf(route.Handlers[len(route.Handlers)-1]).Pointer())
	if fn == nil {
		return err
	}
	name := fn.Name()
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		name = name[dot+1:]
	}
	name = strings.TrimSuffix(name, "-fm")
	outcome.SetOperation(openAPIOperationID(name))
	return err
}

func openAPIOperationID(name string) string {
	if name == "" || name[0] < 'A' || name[0] > 'Z' {
		return name
	}
	return string(name[0]+('a'-'A')) + name[1:]
}

func validOrNewRequestID(value string, entropy io.Reader) (string, error) {
	if requestIDRE.MatchString(value) {
		return value, nil
	}
	if entropy == nil {
		return "", errors.New("request ID entropy reader is nil")
	}
	var raw [16]byte
	if _, err := io.ReadFull(entropy, raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func warnRequestIDGeneration(ctx context.Context) {
	now := time.Now().UnixNano()
	previous := requestIDWarningAt.Load()
	if previous != 0 && time.Duration(now-previous) < time.Minute {
		return
	}
	if !requestIDWarningAt.CompareAndSwap(previous, now) {
		return
	}
	slog.WarnContext(context.WithoutCancel(ctx), "gizclaw: request id generation failed")
}

func observedHTTPErrorCode(contentType string, body []byte, overflow bool, status int) string {
	fallback := "HTTP_SERVER_ERROR"
	if status < http.StatusInternalServerError {
		fallback = "HTTP_CLIENT_ERROR"
	}
	if overflow || len(body) == 0 {
		return fallback
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
		return fallback
	}
	var response apitypes.ErrorResponse
	if err := json.Unmarshal(body, &response); err != nil || !observedErrorCodeRE.MatchString(response.Error.Code) {
		return fallback
	}
	return response.Error.Code
}

func httpObservationResult(ctx context.Context, status int) observability.Result {
	if ctx.Err() != nil {
		return observability.ResultCanceled
	}
	switch {
	case status >= 200 && status < 400:
		return observability.ResultSuccess
	case status >= 400 && status < 500:
		return observability.ResultClientError
	case status >= 500:
		return observability.ResultServerError
	default:
		return observability.ResultTransportError
	}
}

// fiberHTTPHandler adapts a Fiber app to net/http for gizhttp.NewServer.
func fiberHTTPHandler(app *fiber.App) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := fasthttp.AcquireRequest()
		defer fasthttp.ReleaseRequest(req)

		if r.Body != nil {
			n, err := io.Copy(req.BodyWriter(), r.Body)
			req.Header.SetContentLength(int(n))
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}
		req.Header.SetMethod(r.Method)
		requestURI := r.URL.RequestURI()
		if requestURI == "" {
			requestURI = r.RequestURI
		}
		req.SetRequestURI(requestURI)
		req.SetHost(r.Host)
		req.Header.SetHost(r.Host)
		for key, values := range r.Header {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}

		remoteAddr, err := net.ResolveTCPAddr("tcp", r.RemoteAddr)
		if err != nil {
			remoteAddr, _ = net.ResolveTCPAddr("tcp", "127.0.0.1:0")
		}

		var fctx fasthttp.RequestCtx
		fctx.Init(req, remoteAddr, nil)
		fctx.SetUserValue("__local_user_context__", r.Context())
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					observability.MarkPanic(r.Context())
					fctx.Response.Reset()
					fctx.Response.SetStatusCode(http.StatusInternalServerError)
					fctx.Response.SetBodyString(http.StatusText(http.StatusInternalServerError))
				}
			}()
			app.Handler()(&fctx)
		}()

		fctx.Response.Header.VisitAll(func(k, v []byte) {
			w.Header().Add(string(k), string(v))
		})
		statusCode := fctx.Response.StatusCode()
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		if fctx.Response.IsBodyStream() {
			w.WriteHeader(statusCode)
			defer fctx.Response.CloseBodyStream()
			writer := io.Writer(w)
			if flusher, ok := w.(http.Flusher); ok {
				writer = flushWriter{w: w, f: flusher}
			}
			_, _ = io.Copy(writer, fctx.Response.BodyStream())
			return
		}
		responseBody := fctx.Response.Body()
		if len(responseBody) > 0 && w.Header().Get("Content-Length") == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(responseBody)))
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write(responseBody)
	})
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (w flushWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.f.Flush()
	return n, err
}
