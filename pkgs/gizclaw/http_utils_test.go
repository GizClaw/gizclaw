package gizclaw

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

type fiberHTTPHandlerContextKey struct{}

func TestFiberHTTPHandlerHidesPanicDetail(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/panic", func(*fiber.Ctx) error {
		panic("secret-panic-detail")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	fiberHTTPHandler(app).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestFiberHTTPHandlerStreamsResponseBody(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	release := make(chan struct{})
	app.Get("/stream", func(ctx *fiber.Ctx) error {
		pr, pw := io.Pipe()
		go func() {
			_, _ = pw.Write([]byte("first\n"))
			<-release
			_, _ = pw.Write([]byte("second\n"))
			_ = pw.Close()
		}()
		ctx.Response().SetBodyStream(pr, -1)
		return nil
	})

	srv := httptest.NewServer(fiberHTTPHandler(app))
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL + "/stream")
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer resp.Body.Close()

	first := make(chan string, 1)
	go func() {
		buf := make([]byte, len("first\n"))
		_, err := io.ReadFull(resp.Body, buf)
		if err != nil {
			first <- "ERR:" + err.Error()
			return
		}
		first <- string(buf)
	}()

	select {
	case got := <-first:
		if got != "first\n" {
			t.Fatalf("first body chunk = %q, want first", got)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("timed out waiting for first streamed body chunk")
	}
	close(release)
	rest, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read rest: %v", err)
	}
	if string(rest) != "second\n" {
		t.Fatalf("rest body = %q, want second", rest)
	}
}

func TestFiberHTTPHandlerPropagatesRequestContext(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/ctx", func(ctx *fiber.Ctx) error {
		if got := ctx.UserContext().Value(fiberHTTPHandlerContextKey{}); got != "request-value" {
			t.Fatalf("context value = %v", got)
		}
		select {
		case <-ctx.UserContext().Done():
			return ctx.SendStatus(http.StatusNoContent)
		default:
			t.Fatal("request context cancellation was not propagated")
			return nil
		}
	})

	baseCtx, cancel := context.WithCancel(context.WithValue(context.Background(), fiberHTTPHandlerContextKey{}, "request-value"))
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/ctx", nil).WithContext(baseCtx)
	rec := httptest.NewRecorder()
	fiberHTTPHandler(app).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
