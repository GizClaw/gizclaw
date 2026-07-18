package peer

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/gofiber/fiber/v2"
)

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
