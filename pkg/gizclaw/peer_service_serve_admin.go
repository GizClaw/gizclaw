package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/providertenants"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/gameplay/badge"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/gameplay/petspecies"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/resourcemanager"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
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
	PetSpecies      *petspecies.Server
	Badges          *badge.Server
	Contacts        *contact.Server
	Friends         *friend.Server
	FriendGroups    *friendgroup.Server
	ACL             *acl.Server
	ResourceManager *resourcemanager.Manager
}

var _ adminservice.StrictServerInterface = (*adminService)(nil)

func (s *PeerService) serveAdmin(conn *giznet.Conn) error {
	app := fiber.New(fiber.Config{DisableStartupMessage: true, StreamRequestBody: true})
	app.Use(func(ctx *fiber.Ctx) error {
		return ctx.Next()
	})
	handler := adminservice.NewStrictHandler(s.admin, nil)
	adminservice.RegisterHandlers(app, handler)

	server := gizhttp.NewServer(conn, ServiceAdmin, fiberHTTPHandler(app))
	defer func() {
		_ = server.Shutdown(context.Background())
	}()
	defer func() {
		_ = conn.Close()
	}()
	return server.Serve()
}

func (s *adminService) UploadPetSpeciesPixa(ctx context.Context, request adminservice.UploadPetSpeciesPixaRequestObject) (adminservice.UploadPetSpeciesPixaResponseObject, error) {
	if s == nil || s.PetSpecies == nil {
		return adminservice.UploadPetSpeciesPixa500JSONResponse(apitypes.NewErrorResponse("PET_SPECIES_SERVICE_NOT_CONFIGURED", "pet species service is not configured")), nil
	}
	item, err := s.PetSpecies.UploadPixa(ctx, request.Id, request.Body)
	if err != nil {
		status, body := assetError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.UploadPetSpeciesPixa404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.UploadPetSpeciesPixa500JSONResponse(body), nil
		default:
			return adminservice.UploadPetSpeciesPixa400JSONResponse(body), nil
		}
	}
	return adminservice.UploadPetSpeciesPixa200JSONResponse(item), nil
}

func (s *adminService) DownloadPetSpeciesPixa(ctx context.Context, request adminservice.DownloadPetSpeciesPixaRequestObject) (adminservice.DownloadPetSpeciesPixaResponseObject, error) {
	if s == nil || s.PetSpecies == nil {
		return adminservice.DownloadPetSpeciesPixa500JSONResponse(apitypes.NewErrorResponse("PET_SPECIES_SERVICE_NOT_CONFIGURED", "pet species service is not configured")), nil
	}
	r, err := s.PetSpecies.DownloadPixa(ctx, request.Id)
	if err != nil {
		status, body := assetError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.DownloadPetSpeciesPixa404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.DownloadPetSpeciesPixa500JSONResponse(body), nil
		default:
			return adminservice.DownloadPetSpeciesPixa400JSONResponse(body), nil
		}
	}
	return adminservice.DownloadPetSpeciesPixa200ApplicationoctetStreamResponse{Body: r}, nil
}

func (s *adminService) ListPetSpecies(ctx context.Context, request adminservice.ListPetSpeciesRequestObject) (adminservice.ListPetSpeciesResponseObject, error) {
	if s == nil || s.PetSpecies == nil {
		return adminservice.ListPetSpecies500JSONResponse(apitypes.NewErrorResponse("PET_SPECIES_SERVICE_NOT_CONFIGURED", "pet species service is not configured")), nil
	}
	cursor := ""
	if request.Params.Cursor != nil {
		cursor = *request.Params.Cursor
	}
	limit := 0
	if request.Params.Limit != nil {
		limit = int(*request.Params.Limit)
	}
	items, hasNext, nextCursor, err := s.PetSpecies.List(ctx, cursor, limit)
	if err != nil {
		return adminservice.ListPetSpecies500JSONResponse(apitypes.NewErrorResponse("PET_SPECIES_LIST_FAILED", err.Error())), nil
	}
	return adminservice.ListPetSpecies200JSONResponse(adminservice.PetSpeciesList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *adminService) UploadBadgeIcon(ctx context.Context, request adminservice.UploadBadgeIconRequestObject) (adminservice.UploadBadgeIconResponseObject, error) {
	if s == nil || s.Badges == nil {
		return adminservice.UploadBadgeIcon500JSONResponse(apitypes.NewErrorResponse("BADGE_SERVICE_NOT_CONFIGURED", "badge service is not configured")), nil
	}
	item, err := s.Badges.UploadIcon(ctx, request.Id, request.Body)
	if err != nil {
		status, body := assetError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.UploadBadgeIcon404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.UploadBadgeIcon500JSONResponse(body), nil
		default:
			return adminservice.UploadBadgeIcon400JSONResponse(body), nil
		}
	}
	return adminservice.UploadBadgeIcon200JSONResponse(item), nil
}

func (s *adminService) DownloadBadgeIcon(ctx context.Context, request adminservice.DownloadBadgeIconRequestObject) (adminservice.DownloadBadgeIconResponseObject, error) {
	if s == nil || s.Badges == nil {
		return adminservice.DownloadBadgeIcon500JSONResponse(apitypes.NewErrorResponse("BADGE_SERVICE_NOT_CONFIGURED", "badge service is not configured")), nil
	}
	r, err := s.Badges.DownloadIcon(ctx, request.Id)
	if err != nil {
		status, body := assetError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.DownloadBadgeIcon404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.DownloadBadgeIcon500JSONResponse(body), nil
		default:
			return adminservice.DownloadBadgeIcon400JSONResponse(body), nil
		}
	}
	return adminservice.DownloadBadgeIcon200ApplicationoctetStreamResponse{Body: r}, nil
}

func (s *adminService) ListBadges(ctx context.Context, request adminservice.ListBadgesRequestObject) (adminservice.ListBadgesResponseObject, error) {
	if s == nil || s.Badges == nil {
		return adminservice.ListBadges500JSONResponse(apitypes.NewErrorResponse("BADGE_SERVICE_NOT_CONFIGURED", "badge service is not configured")), nil
	}
	cursor := ""
	if request.Params.Cursor != nil {
		cursor = *request.Params.Cursor
	}
	limit := 0
	if request.Params.Limit != nil {
		limit = int(*request.Params.Limit)
	}
	items, hasNext, nextCursor, err := s.Badges.List(ctx, cursor, limit)
	if err != nil {
		return adminservice.ListBadges500JSONResponse(apitypes.NewErrorResponse("BADGE_LIST_FAILED", err.Error())), nil
	}
	return adminservice.ListBadges200JSONResponse(adminservice.BadgeList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func assetError(err error) (int, apitypes.ErrorResponse) {
	switch {
	case errors.Is(err, kv.ErrNotFound), errors.Is(err, fs.ErrNotExist):
		return http.StatusNotFound, apitypes.NewErrorResponse("ASSET_NOT_FOUND", err.Error())
	default:
		return http.StatusBadRequest, apitypes.NewErrorResponse("INVALID_ASSET", err.Error())
	}
}

func (s *adminService) ApplyResource(ctx context.Context, request adminservice.ApplyResourceRequestObject) (adminservice.ApplyResourceResponseObject, error) {
	if request.JSONBody == nil {
		return adminservice.ApplyResource400JSONResponse(apitypes.NewErrorResponse("INVALID_RESOURCE", "request body is required")), nil
	}
	result, err := s.ResourceManager.Apply(ctx, *request.JSONBody)
	if err != nil {
		status, body := resourceManagerError(err)
		switch status {
		case http.StatusBadRequest:
			return adminservice.ApplyResource400JSONResponse(body), nil
		case http.StatusConflict:
			return adminservice.ApplyResource409JSONResponse(body), nil
		default:
			return adminservice.ApplyResource500JSONResponse(body), nil
		}
	}
	return adminservice.ApplyResource200JSONResponse(result), nil
}

func (s *adminService) GetResource(ctx context.Context, request adminservice.GetResourceRequestObject) (adminservice.GetResourceResponseObject, error) {
	resource, err := s.ResourceManager.Get(ctx, request.Kind, request.Name)
	if err != nil {
		status, body := resourceManagerError(err)
		switch status {
		case http.StatusBadRequest:
			return adminservice.GetResource400JSONResponse(body), nil
		case http.StatusNotFound:
			return adminservice.GetResource404JSONResponse(body), nil
		default:
			return adminservice.GetResource500JSONResponse(body), nil
		}
	}
	return resource200JSONResponse{Resource: resource}, nil
}

func (s *adminService) PutResource(ctx context.Context, request adminservice.PutResourceRequestObject) (adminservice.PutResourceResponseObject, error) {
	if request.JSONBody == nil {
		return adminservice.PutResource400JSONResponse(apitypes.NewErrorResponse("INVALID_RESOURCE", "request body is required")), nil
	}
	if err := validateResourcePathMatch(*request.JSONBody, request.Kind, request.Name); err != nil {
		return adminservice.PutResource400JSONResponse(apitypes.NewErrorResponse("INVALID_RESOURCE_PATH", err.Error())), nil
	}
	resource, err := s.ResourceManager.Put(ctx, *request.JSONBody)
	if err != nil {
		status, body := resourceManagerError(err)
		switch status {
		case http.StatusBadRequest:
			return adminservice.PutResource400JSONResponse(body), nil
		case http.StatusNotFound:
			return adminservice.PutResource404JSONResponse(body), nil
		case http.StatusConflict:
			return adminservice.PutResource409JSONResponse(body), nil
		default:
			return adminservice.PutResource500JSONResponse(body), nil
		}
	}
	return resource200JSONResponse{Resource: resource}, nil
}

func (s *adminService) DeleteResource(ctx context.Context, request adminservice.DeleteResourceRequestObject) (adminservice.DeleteResourceResponseObject, error) {
	resource, err := s.ResourceManager.Delete(ctx, request.Kind, request.Name)
	if err != nil {
		status, body := resourceManagerError(err)
		switch status {
		case http.StatusBadRequest:
			return adminservice.DeleteResource400JSONResponse(body), nil
		case http.StatusNotFound:
			return adminservice.DeleteResource404JSONResponse(body), nil
		case http.StatusConflict:
			return adminservice.DeleteResource409JSONResponse(body), nil
		default:
			return adminservice.DeleteResource500JSONResponse(body), nil
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
