package gizclaw

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func (s *peerHTTP) GetSideControlInfo(ctx context.Context, _ peerhttp.GetSideControlInfoRequestObject) (peerhttp.GetSideControlInfoResponseObject, error) {
	principal, err := publiclogin.SideControlPrincipal(ctx)
	if err != nil {
		return peerhttp.GetSideControlInfo403JSONResponse{ForbiddenJSONResponse: sideControlForbidden(err)}, nil
	}
	if s == nil || s.Self == nil {
		return peerhttp.GetSideControlInfo500JSONResponse{InternalErrorJSONResponse: sideControlInternal("peer self service is not configured")}, nil
	}
	registration, err := s.Self.GetSelfRegistration(ctx, principal.TargetPublicKey)
	if errors.Is(err, peer.ErrPeerNotFound) {
		return peerhttp.GetSideControlInfo404JSONResponse{NotFoundJSONResponse: sideControlNotFound(err)}, nil
	}
	if err != nil {
		return peerhttp.GetSideControlInfo500JSONResponse{InternalErrorJSONResponse: sideControlInternal(err.Error())}, nil
	}
	if registration.Device == nil {
		return peerhttp.GetSideControlInfo404JSONResponse{NotFoundJSONResponse: sideControlNotFound(errors.New("target device information not found"))}, nil
	}
	return peerhttp.GetSideControlInfo200JSONResponse(*registration.Device), nil
}

func (s *peerHTTP) GetSideControlRuntime(ctx context.Context, _ peerhttp.GetSideControlRuntimeRequestObject) (peerhttp.GetSideControlRuntimeResponseObject, error) {
	principal, err := publiclogin.SideControlPrincipal(ctx)
	if err != nil {
		return peerhttp.GetSideControlRuntime403JSONResponse{ForbiddenJSONResponse: sideControlForbidden(err)}, nil
	}
	if s == nil || s.Self == nil {
		return peerhttp.GetSideControlRuntime500JSONResponse{InternalErrorJSONResponse: sideControlInternal("peer self service is not configured")}, nil
	}
	if _, err := s.Self.GetSelfRegistration(ctx, principal.TargetPublicKey); errors.Is(err, peer.ErrPeerNotFound) {
		return peerhttp.GetSideControlRuntime404JSONResponse{NotFoundJSONResponse: sideControlNotFound(err)}, nil
	} else if err != nil {
		return peerhttp.GetSideControlRuntime500JSONResponse{InternalErrorJSONResponse: sideControlInternal(err.Error())}, nil
	}
	return peerhttp.GetSideControlRuntime200JSONResponse(s.Self.GetSelfRuntime(ctx, principal.TargetPublicKey)), nil
}

func (s *peerHTTP) GetSideControlStatus(ctx context.Context, _ peerhttp.GetSideControlStatusRequestObject) (peerhttp.GetSideControlStatusResponseObject, error) {
	principal, err := publiclogin.SideControlPrincipal(ctx)
	if err != nil {
		return peerhttp.GetSideControlStatus403JSONResponse{ForbiddenJSONResponse: sideControlForbidden(err)}, nil
	}
	if s == nil || s.Self == nil || s.Status == nil {
		return peerhttp.GetSideControlStatus500JSONResponse{InternalErrorJSONResponse: sideControlInternal("peer status service is not configured")}, nil
	}
	if _, err := s.Self.GetSelfRegistration(ctx, principal.TargetPublicKey); errors.Is(err, peer.ErrPeerNotFound) {
		return peerhttp.GetSideControlStatus404JSONResponse{NotFoundJSONResponse: sideControlNotFound(err)}, nil
	} else if err != nil {
		return peerhttp.GetSideControlStatus500JSONResponse{InternalErrorJSONResponse: sideControlInternal(err.Error())}, nil
	}
	status, err := s.Status.GetStatus(ctx, principal.TargetPublicKey)
	if err != nil {
		return peerhttp.GetSideControlStatus500JSONResponse{InternalErrorJSONResponse: sideControlInternal(err.Error())}, nil
	}
	return peerhttp.GetSideControlStatus200JSONResponse(status), nil
}

func (s *peerHTTP) GetSideControlTelemetryLatest(ctx context.Context, request peerhttp.GetSideControlTelemetryLatestRequestObject) (peerhttp.GetSideControlTelemetryLatestResponseObject, error) {
	principal, err := publiclogin.SideControlPrincipal(ctx)
	if err != nil {
		return peerhttp.GetSideControlTelemetryLatest403JSONResponse{ForbiddenJSONResponse: sideControlForbidden(err)}, nil
	}
	fields, err := parsePeerTelemetryFields(request.Params.Fields)
	if err != nil {
		return peerhttp.GetSideControlTelemetryLatest400JSONResponse{BadRequestJSONResponse: sideControlBadRequest(err)}, nil
	}
	if s == nil || s.Telemetry == nil {
		return peerhttp.GetSideControlTelemetryLatest500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(telemetryNotConfiguredResponse())}, nil
	}
	result, err := s.Telemetry.Latest(ctx, principal.TargetPublicKey, fields)
	if err != nil {
		status, body := peerTelemetryAdminError(err)
		if status == http.StatusBadRequest {
			return peerhttp.GetSideControlTelemetryLatest400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		}
		return peerhttp.GetSideControlTelemetryLatest500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}, nil
	}
	return peerhttp.GetSideControlTelemetryLatest200JSONResponse(result), nil
}

func (s *peerHTTP) QuerySideControlTelemetry(ctx context.Context, request peerhttp.QuerySideControlTelemetryRequestObject) (peerhttp.QuerySideControlTelemetryResponseObject, error) {
	principal, err := publiclogin.SideControlPrincipal(ctx)
	if err != nil {
		return peerhttp.QuerySideControlTelemetry403JSONResponse{ForbiddenJSONResponse: sideControlForbidden(err)}, nil
	}
	if s == nil || s.Telemetry == nil {
		return peerhttp.QuerySideControlTelemetry500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(telemetryNotConfiguredResponse())}, nil
	}
	step := time.Duration(0)
	if request.Params.StepMs != nil {
		step = time.Duration(*request.Params.StepMs) * time.Millisecond
	}
	limit := 0
	if request.Params.Limit != nil {
		limit = int(*request.Params.Limit)
	}
	order := apitypes.PeerTelemetryOrderAsc
	if request.Params.Order != nil {
		order = *request.Params.Order
	}
	result, err := s.Telemetry.QueryRange(ctx, principal.TargetPublicKey, request.Params.Field, time.UnixMilli(request.Params.StartTimeMs), time.UnixMilli(request.Params.EndTimeMs), step, limit, order)
	if err != nil {
		status, body := peerTelemetryAdminError(err)
		if status == http.StatusBadRequest {
			return peerhttp.QuerySideControlTelemetry400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		}
		return peerhttp.QuerySideControlTelemetry500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}, nil
	}
	return peerhttp.QuerySideControlTelemetry200JSONResponse(result), nil
}

func (s *peerHTTP) AggregateSideControlTelemetry(ctx context.Context, request peerhttp.AggregateSideControlTelemetryRequestObject) (peerhttp.AggregateSideControlTelemetryResponseObject, error) {
	principal, err := publiclogin.SideControlPrincipal(ctx)
	if err != nil {
		return peerhttp.AggregateSideControlTelemetry403JSONResponse{ForbiddenJSONResponse: sideControlForbidden(err)}, nil
	}
	if s == nil || s.Telemetry == nil {
		return peerhttp.AggregateSideControlTelemetry500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(telemetryNotConfiguredResponse())}, nil
	}
	result, err := s.Telemetry.Aggregate(ctx, principal.TargetPublicKey, request.Params.Field, time.UnixMilli(request.Params.StartTimeMs), time.UnixMilli(request.Params.EndTimeMs), time.Duration(request.Params.BucketMs)*time.Millisecond, request.Params.Aggregate)
	if err != nil {
		status, body := peerTelemetryAdminError(err)
		if status == http.StatusBadRequest {
			return peerhttp.AggregateSideControlTelemetry400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		}
		return peerhttp.AggregateSideControlTelemetry500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}, nil
	}
	return peerhttp.AggregateSideControlTelemetry200JSONResponse(result), nil
}

func (s *peerHTTP) ListSideControlContacts(ctx context.Context, request peerhttp.ListSideControlContactsRequestObject) (peerhttp.ListSideControlContactsResponseObject, error) {
	principal, err := publiclogin.SideControlPrincipal(ctx)
	if err != nil {
		return peerhttp.ListSideControlContacts403JSONResponse{ForbiddenJSONResponse: sideControlForbidden(err)}, nil
	}
	if s == nil || s.Contacts == nil {
		return peerhttp.ListSideControlContacts500JSONResponse{InternalErrorJSONResponse: sideControlInternal("contact service is not configured")}, nil
	}
	var limit *int
	if request.Params.Limit != nil {
		value := int(*request.Params.Limit)
		limit = &value
	}
	result, err := s.Contacts.ListContacts(ctx, principal.TargetPublicKey.String(), rpcapi.ContactListRequest{Cursor: request.Params.Cursor, Limit: limit})
	if err != nil {
		status, body := adminSocialError(err)
		if status != http.StatusInternalServerError {
			return peerhttp.ListSideControlContacts400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		}
		return peerhttp.ListSideControlContacts500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}, nil
	}
	return peerhttp.ListSideControlContacts200JSONResponse(result), nil
}

func (s *peerHTTP) CreateSideControlContact(ctx context.Context, request peerhttp.CreateSideControlContactRequestObject) (peerhttp.CreateSideControlContactResponseObject, error) {
	principal, err := publiclogin.SideControlPrincipal(ctx)
	if err != nil {
		return peerhttp.CreateSideControlContact403JSONResponse{ForbiddenJSONResponse: sideControlForbidden(err)}, nil
	}
	if request.Body == nil {
		return peerhttp.CreateSideControlContact400JSONResponse{BadRequestJSONResponse: sideControlBadRequest(errors.New("request body is required"))}, nil
	}
	if s == nil || s.Contacts == nil {
		return peerhttp.CreateSideControlContact500JSONResponse{InternalErrorJSONResponse: sideControlInternal("contact service is not configured")}, nil
	}
	item, err := s.Contacts.CreateContact(ctx, principal.TargetPublicKey.String(), *request.Body)
	if err != nil {
		status, body := adminSocialError(err)
		if status == http.StatusInternalServerError {
			return peerhttp.CreateSideControlContact500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}, nil
		}
		return peerhttp.CreateSideControlContact400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	return peerhttp.CreateSideControlContact201JSONResponse(item), nil
}

func (s *peerHTTP) GetSideControlContact(ctx context.Context, request peerhttp.GetSideControlContactRequestObject) (peerhttp.GetSideControlContactResponseObject, error) {
	principal, err := publiclogin.SideControlPrincipal(ctx)
	if err != nil {
		return peerhttp.GetSideControlContact403JSONResponse{ForbiddenJSONResponse: sideControlForbidden(err)}, nil
	}
	if s == nil || s.Contacts == nil {
		return peerhttp.GetSideControlContact500JSONResponse{InternalErrorJSONResponse: sideControlInternal("contact service is not configured")}, nil
	}
	item, err := s.Contacts.GetContact(ctx, principal.TargetPublicKey.String(), rpcapi.ContactGetRequest{Id: request.ContactId})
	if errors.Is(err, kv.ErrNotFound) {
		return peerhttp.GetSideControlContact404JSONResponse{NotFoundJSONResponse: sideControlNotFound(err)}, nil
	}
	if err != nil {
		return peerhttp.GetSideControlContact500JSONResponse{InternalErrorJSONResponse: sideControlInternal(err.Error())}, nil
	}
	return peerhttp.GetSideControlContact200JSONResponse(item), nil
}

func (s *peerHTTP) PutSideControlContact(ctx context.Context, request peerhttp.PutSideControlContactRequestObject) (peerhttp.PutSideControlContactResponseObject, error) {
	principal, err := publiclogin.SideControlPrincipal(ctx)
	if err != nil {
		return peerhttp.PutSideControlContact403JSONResponse{ForbiddenJSONResponse: sideControlForbidden(err)}, nil
	}
	if request.Body == nil {
		return peerhttp.PutSideControlContact400JSONResponse{BadRequestJSONResponse: sideControlBadRequest(errors.New("request body is required"))}, nil
	}
	if s == nil || s.Contacts == nil {
		return peerhttp.PutSideControlContact500JSONResponse{InternalErrorJSONResponse: sideControlInternal("contact service is not configured")}, nil
	}
	item, err := s.Contacts.PutContact(ctx, principal.TargetPublicKey.String(), rpcapi.ContactPutRequest{Id: request.ContactId, DisplayName: request.Body.DisplayName, PhoneNumber: request.Body.PhoneNumber})
	if errors.Is(err, kv.ErrNotFound) {
		return peerhttp.PutSideControlContact404JSONResponse{NotFoundJSONResponse: sideControlNotFound(err)}, nil
	}
	if err != nil {
		status, body := adminSocialError(err)
		if status == http.StatusInternalServerError {
			return peerhttp.PutSideControlContact500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}, nil
		}
		return peerhttp.PutSideControlContact400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	return peerhttp.PutSideControlContact200JSONResponse(item), nil
}

func (s *peerHTTP) DeleteSideControlContact(ctx context.Context, request peerhttp.DeleteSideControlContactRequestObject) (peerhttp.DeleteSideControlContactResponseObject, error) {
	principal, err := publiclogin.SideControlPrincipal(ctx)
	if err != nil {
		return peerhttp.DeleteSideControlContact403JSONResponse{ForbiddenJSONResponse: sideControlForbidden(err)}, nil
	}
	if s == nil || s.Contacts == nil {
		return peerhttp.DeleteSideControlContact500JSONResponse{InternalErrorJSONResponse: sideControlInternal("contact service is not configured")}, nil
	}
	item, err := s.Contacts.DeleteContact(ctx, principal.TargetPublicKey.String(), rpcapi.ContactDeleteRequest{Id: request.ContactId})
	if errors.Is(err, kv.ErrNotFound) {
		return peerhttp.DeleteSideControlContact404JSONResponse{NotFoundJSONResponse: sideControlNotFound(err)}, nil
	}
	if err != nil {
		return peerhttp.DeleteSideControlContact500JSONResponse{InternalErrorJSONResponse: sideControlInternal(err.Error())}, nil
	}
	return peerhttp.DeleteSideControlContact200JSONResponse(item), nil
}

func sideControlForbidden(err error) peerhttp.ForbiddenJSONResponse {
	return peerhttp.ForbiddenJSONResponse(apitypes.NewErrorResponse("SIDE_CONTROL_SESSION_REQUIRED", err.Error()))
}

func sideControlBadRequest(err error) peerhttp.BadRequestJSONResponse {
	return peerhttp.BadRequestJSONResponse(apitypes.NewErrorResponse("INVALID_SIDE_CONTROL_REQUEST", err.Error()))
}

func sideControlNotFound(err error) peerhttp.NotFoundJSONResponse {
	return peerhttp.NotFoundJSONResponse(apitypes.NewErrorResponse("SIDE_CONTROL_OBJECT_NOT_FOUND", err.Error()))
}

func sideControlInternal(message string) peerhttp.InternalErrorJSONResponse {
	return peerhttp.InternalErrorJSONResponse(apitypes.NewErrorResponse("SIDE_CONTROL_FAILED", message))
}
