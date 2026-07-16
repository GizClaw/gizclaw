package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/providertenants"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/asset"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/resourcemanager"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
)

type adminService struct {
	credential.CredentialAdminService
	firmware.FirmwareAdminService
	peer.PeerAdminService
	model.ModelAdminService
	voice.VoiceAdminService
	providertenants.ProviderTenantsAdminService
	workspace.WorkspaceAdminService
	workflow.WorkflowAdminService
	gameplay.CatalogAdminService
	Contacts        *contact.Server
	Friends         *friend.Server
	FriendGroups    *friendgroup.Server
	Gameplay        *gameplay.Runtime
	ACL             *acl.Server
	ResourceManager *resourcemanager.Manager
	ServerLogs      ServerLogQueryService
	PeerTelemetry   *peertelemetry.AdminService
	Assets          *asset.Service
	AssetMaxBytes   int64
}

var _ adminhttp.StrictServerInterface = (*adminService)(nil)

func (s *PeerService) serveAdmin(conn giznet.Conn) error {
	app := fiber.New(fiber.Config{DisableStartupMessage: true, StreamRequestBody: true})
	app.Use(observeFiberRoute)
	app.Use(func(ctx *fiber.Ctx) error {
		return ctx.Next()
	})
	handler := adminhttp.NewStrictHandler(s.admin, nil)
	adminhttp.RegisterHandlers(app, handler)

	httpHandler := observeHTTPHandler(fiberHTTPHandler(app), httpObservationOptions{
		surface:       observability.SurfaceAdminHTTP,
		peerPublicKey: conn.PublicKey().String(),
		peerRole:      string(apitypes.PeerRoleAdmin),
	})
	server := gizhttp.NewServer(conn, ServiceAdminHTTP, httpHandler)
	defer func() {
		_ = server.Shutdown(context.Background())
	}()
	return server.Serve()
}

func (s *adminService) ApplyResource(ctx context.Context, request adminhttp.ApplyResourceRequestObject) (adminhttp.ApplyResourceResponseObject, error) {
	if request.JSONBody == nil {
		return adminhttp.ApplyResource400JSONResponse(apitypes.NewErrorResponse("INVALID_RESOURCE", "request body is required")), nil
	}
	result, err := s.ResourceManager.Apply(ctx, *request.JSONBody)
	if err != nil {
		status, body := resourceManagerError(err)
		switch status {
		case http.StatusBadRequest:
			return adminhttp.ApplyResource400JSONResponse(body), nil
		case http.StatusConflict:
			return adminhttp.ApplyResource409JSONResponse(body), nil
		default:
			return adminhttp.ApplyResource500JSONResponse(body), nil
		}
	}
	return adminhttp.ApplyResource200JSONResponse(result), nil
}

func (s *adminService) GetResource(ctx context.Context, request adminhttp.GetResourceRequestObject) (adminhttp.GetResourceResponseObject, error) {
	resource, err := s.ResourceManager.Get(ctx, request.Kind, request.Name)
	if err != nil {
		status, body := resourceManagerError(err)
		switch status {
		case http.StatusBadRequest:
			return adminhttp.GetResource400JSONResponse(body), nil
		case http.StatusNotFound:
			return adminhttp.GetResource404JSONResponse(body), nil
		default:
			return adminhttp.GetResource500JSONResponse(body), nil
		}
	}
	return resource200JSONResponse{Resource: resource}, nil
}

func (s *adminService) PutResource(ctx context.Context, request adminhttp.PutResourceRequestObject) (adminhttp.PutResourceResponseObject, error) {
	if request.JSONBody == nil {
		return adminhttp.PutResource400JSONResponse(apitypes.NewErrorResponse("INVALID_RESOURCE", "request body is required")), nil
	}
	if err := validateResourcePathMatch(*request.JSONBody, request.Kind, request.Name); err != nil {
		return adminhttp.PutResource400JSONResponse(apitypes.NewErrorResponse("INVALID_RESOURCE_PATH", err.Error())), nil
	}
	resource, err := s.ResourceManager.Put(ctx, *request.JSONBody)
	if err != nil {
		status, body := resourceManagerError(err)
		switch status {
		case http.StatusBadRequest:
			return adminhttp.PutResource400JSONResponse(body), nil
		case http.StatusNotFound:
			return adminhttp.PutResource404JSONResponse(body), nil
		case http.StatusConflict:
			return adminhttp.PutResource409JSONResponse(body), nil
		default:
			return adminhttp.PutResource500JSONResponse(body), nil
		}
	}
	return resource200JSONResponse{Resource: resource}, nil
}

func (s *adminService) DeleteResource(ctx context.Context, request adminhttp.DeleteResourceRequestObject) (adminhttp.DeleteResourceResponseObject, error) {
	resource, err := s.ResourceManager.Delete(ctx, request.Kind, request.Name)
	if err != nil {
		status, body := resourceManagerError(err)
		switch status {
		case http.StatusBadRequest:
			return adminhttp.DeleteResource400JSONResponse(body), nil
		case http.StatusNotFound:
			return adminhttp.DeleteResource404JSONResponse(body), nil
		case http.StatusConflict:
			return adminhttp.DeleteResource409JSONResponse(body), nil
		default:
			return adminhttp.DeleteResource500JSONResponse(body), nil
		}
	}
	return resource200JSONResponse{Resource: resource}, nil
}

type resource200JSONResponse struct {
	Resource apitypes.Resource
}

func (response resource200JSONResponse) VisitGetResourceResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(http.StatusOK)
	return ctx.JSON(&response.Resource)
}

func (response resource200JSONResponse) VisitPutResourceResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(http.StatusOK)
	return ctx.JSON(&response.Resource)
}

func (response resource200JSONResponse) VisitDeleteResourceResponse(ctx *fiber.Ctx) error {
	ctx.Response().Header.Set("Content-Type", "application/json")
	ctx.Status(http.StatusOK)
	return ctx.JSON(&response.Resource)
}

func resourceManagerError(err error) (int, apitypes.ErrorResponse) {
	var resourceErr *resourcemanager.Error
	if errors.As(err, &resourceErr) {
		return resourceErr.StatusCode, apitypes.NewErrorResponse(resourceErr.Code, resourceErr.Message)
	}
	return http.StatusInternalServerError, apitypes.NewErrorResponse("RESOURCE_MANAGER_ERROR", err.Error())
}

func validateResourcePathMatch(resource apitypes.Resource, kind apitypes.ResourceKind, name string) error {
	var header struct {
		Kind     apitypes.ResourceKind `json:"kind"`
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	data, err := json.Marshal(resource)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}
	if header.Kind != kind {
		return errors.New("resource kind does not match path kind")
	}
	if header.Metadata.Name != name {
		return errors.New("resource metadata.name does not match path name")
	}
	return nil
}
