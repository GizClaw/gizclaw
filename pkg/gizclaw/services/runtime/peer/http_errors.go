package peer

import (
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/gofiber/fiber/v2"
)

type getPeerConfig500JSONResponse apitypes.ErrorResponse

func (response getPeerConfig500JSONResponse) VisitGetPeerConfigResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(500)
	return ctx.JSON(&response)
}

type putPeerConfig500JSONResponse apitypes.ErrorResponse

func (response putPeerConfig500JSONResponse) VisitPutPeerConfigResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(500)
	return ctx.JSON(&response)
}

type getPeerInfo500JSONResponse apitypes.ErrorResponse

func (response getPeerInfo500JSONResponse) VisitGetPeerInfoResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(500)
	return ctx.JSON(&response)
}

type refreshPeer500JSONResponse apitypes.ErrorResponse

func (response refreshPeer500JSONResponse) VisitRefreshPeerResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(500)
	return ctx.JSON(&response)
}
