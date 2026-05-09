package gizclaw

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
	"golang.org/x/sync/errgroup"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/gearservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/serverpublic"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/firmware"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gear"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizhttp"
)

const (
	ServiceRPC          uint64 = 0x00
	ServiceServerPublic uint64 = 0x01
	ServicePeerPublic   uint64 = 0x02
	ServiceAdmin        uint64 = 0x10
	ServiceGear         uint64 = 0x11

	ProtocolEvent       byte = 0x03
	ProtocolStampedOpus byte = 0x10
)

type gearAPIBundle struct {
	firmware.FirmwareGearService
	gear.GearsGearService
}

type serverPublic struct {
	gear.GearsServerPublic
	publiclogin.ServerPublic
}

// PeerService serves one peer connection.
type PeerService struct {
	admin   *adminService
	gear    *gearAPIBundle
	public  *serverPublic
	manager *Manager
}

var _ gearservice.StrictServerInterface = (*gearAPIBundle)(nil)
var _ serverpublic.StrictServerInterface = (*serverPublic)(nil)

func (s *PeerService) ServeConn(conn *giznet.Conn) error {
	if s == nil {
		return errors.New("gizclaw: nil peer service")
	}
	if conn == nil {
		return errors.New("gizclaw: nil conn")
	}
	defer func() {
		_ = conn.Close()
	}()
	if err := s.validateServices(); err != nil {
		return err
	}
	if err := s.ensurePeerGear(context.Background(), conn); err != nil {
		return err
	}
	publicKey := conn.PublicKey()
	s.manager.SetPeerUp(publicKey, conn)
	defer s.manager.SetPeerDown(publicKey)

	var g errgroup.Group
	g.Go(func() error { return s.serveAdmin(conn) })
	g.Go(func() error { return s.serveGear(conn) })
	g.Go(func() error { return s.servePublic(conn) })

	return g.Wait()
}

func (s *PeerService) ensurePeerGear(ctx context.Context, conn *giznet.Conn) error {
	if s == nil || s.manager == nil {
		return nil
	}
	_, err := s.manager.EnsureGear(ctx, conn.PublicKey())
	return err
}

func (s *PeerService) validateServices() error {
	switch {
	case s.admin == nil:
		return errors.New("gizclaw: nil admin service")
	case s.gear == nil:
		return errors.New("gizclaw: nil gear service")
	case s.public == nil:
		return errors.New("gizclaw: nil public service")
	default:
		return nil
	}
}

func (s *PeerService) servePublic(conn *giznet.Conn) error {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(ctx *fiber.Ctx) error {
		base := ctx.UserContext()
		if base == nil {
			base = context.Background()
		}
		ctx.SetUserContext(serverpublic.WithCallerPublicKey(base, conn.PublicKey()))
		return ctx.Next()
	})
	serverpublic.RegisterHandlers(app, serverpublic.NewStrictHandler(s.public, nil))

	server := gizhttp.NewServer(conn, ServiceServerPublic, fiberHTTPHandler(app))
	defer func() {
		_ = server.Shutdown(context.Background())
	}()
	defer func() {
		_ = conn.Close()
	}()
	return server.Serve()
}

func (s *PeerService) serveGear(conn *giznet.Conn) error {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(ctx *fiber.Ctx) error {
		base := ctx.UserContext()
		if base == nil {
			base = context.Background()
		}
		ctx.SetUserContext(gearservice.WithCallerPublicKey(base, conn.PublicKey()))
		return ctx.Next()
	})
	registerGearDownloadHandler(app, s.gear)

	server := gizhttp.NewServer(conn, ServiceGear, fiberHTTPHandler(app))
	defer func() {
		_ = server.Shutdown(context.Background())
	}()
	defer func() {
		_ = conn.Close()
	}()
	return server.Serve()
}

func registerGearDownloadHandler(app *fiber.App, service gearservice.StrictServerInterface) {
	app.Get("/download/firmware/:path", func(ctx *fiber.Ctx) error {
		resp, err := service.DownloadFirmware(ctx.UserContext(), gearservice.DownloadFirmwareRequestObject{
			Path: ctx.Params("path"),
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if resp == nil {
			return nil
		}
		return resp.VisitDownloadFirmwareResponse(ctx)
	})
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

		responseBody := fctx.Response.Body()
		fctx.Response.Header.VisitAll(func(k, v []byte) {
			w.Header().Add(string(k), string(v))
		})
		if len(responseBody) > 0 && w.Header().Get("Content-Length") == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(responseBody)))
		}
		statusCode := fctx.Response.StatusCode()
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write(responseBody)
	})
}
