package workflow

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/gofiber/fiber/v2"
)

type createWorkflow200Response struct {
	doc apitypes.Workflow
}

func (response createWorkflow200Response) VisitCreateWorkflowResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(200)
	return ctx.JSON(response.doc)
}

type getWorkflow200Response struct {
	doc apitypes.Workflow
}

func (response getWorkflow200Response) VisitGetWorkflowResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(200)
	return ctx.JSON(response.doc)
}

type putWorkflow200Response struct {
	doc apitypes.Workflow
}

func (response putWorkflow200Response) VisitPutWorkflowResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(200)
	return ctx.JSON(response.doc)
}

type deleteWorkflow200Response struct {
	doc apitypes.Workflow
}

func (response deleteWorkflow200Response) VisitDeleteWorkflowResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(200)
	return ctx.JSON(response.doc)
}
