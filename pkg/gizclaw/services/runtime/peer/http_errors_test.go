package peer

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

func TestHTTPErrorHelpersAndVisitors(t *testing.T) {
	if apitypes.NewErrorResponse("G", "boom").Error.Code != "G" {
		t.Fatal("NewErrorResponse code mismatch")
	}

	app := fiber.New()
	t.Cleanup(func() {
		_ = app.Shutdown()
	})

	checkStatus := func(name string, visit func(*fiber.Ctx) error) {
		t.Helper()
		var reqCtx fasthttp.RequestCtx
		ctx := app.AcquireCtx(&reqCtx)
		defer app.ReleaseCtx(ctx)
		if err := visit(ctx); err != nil {
			t.Fatalf("%s error: %v", name, err)
		}
		if ctx.Response().StatusCode() != 500 {
			t.Fatalf("%s status = %d", name, ctx.Response().StatusCode())
		}
	}

	checkStatus("get-peer-config", func(c *fiber.Ctx) error {
		return getPeerConfig500JSONResponse(apitypes.NewErrorResponse("ERR", "boom")).VisitGetPeerConfigResponse(c)
	})
	checkStatus("put-peer-config", func(c *fiber.Ctx) error {
		return putPeerConfig500JSONResponse(apitypes.NewErrorResponse("ERR", "boom")).VisitPutPeerConfigResponse(c)
	})
	checkStatus("get-peer-info", func(c *fiber.Ctx) error {
		return getPeerInfo500JSONResponse(apitypes.NewErrorResponse("ERR", "boom")).VisitGetPeerInfoResponse(c)
	})
	checkStatus("refresh-peer", func(c *fiber.Ctx) error {
		return refreshPeer500JSONResponse(apitypes.NewErrorResponse("ERR", "boom")).VisitRefreshPeerResponse(c)
	})
	var (
		_ apitypes.ErrorResponse
		_ apitypes.ErrorResponse
		_ apitypes.ErrorResponse
	)
}
