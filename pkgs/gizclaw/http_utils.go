package gizclaw

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func authenticateFiberSession(ctx *fiber.Ctx, sessions *publiclogin.SessionManager) (giznet.PublicKey, bool) {
	publicKey, err := sessions.AuthenticateHeaders(ctx.Get("Authorization"), ctx.Get(publiclogin.PublicKeyHeader))
	if err != nil {
		if errors.Is(err, publiclogin.ErrPublicKeyMismatch) {
			ctx.Status(http.StatusUnauthorized)
			_ = ctx.JSON(map[string]any{
				"error": map[string]string{
					"code":    "PUBLIC_KEY_MISMATCH",
					"message": "x-public-key does not match bearer session",
				},
			})
			return giznet.PublicKey{}, false
		}
		ctx.Status(http.StatusUnauthorized)
		_ = ctx.JSON(map[string]any{
			"error": map[string]string{
				"code":    "INVALID_SESSION",
				"message": "missing or invalid bearer session",
			},
		})
		return giznet.PublicKey{}, false
	}
	return publicKey, true
}

func authenticateHTTPSession(w http.ResponseWriter, r *http.Request, sessions *publiclogin.SessionManager) (giznet.PublicKey, bool) {
	publicKey, err := sessions.AuthenticateHeaders(r.Header.Get("Authorization"), r.Header.Get(publiclogin.PublicKeyHeader))
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
	return publicKey, true
}

func setPublicHTTPCORSHeaders(header http.Header) {
	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
	header.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Public-Key,X-Giznet-Nonce,X-Giznet-Public-Key,X-Giznet-Timestamp")
	header.Set("Access-Control-Expose-Headers", "Content-Length,Content-Type")
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
