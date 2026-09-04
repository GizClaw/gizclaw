package gizclaw

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const (
	publicHTTPInvalidRequestCode = "INVALID_REQUEST"
	publicHTTPInternalErrorCode  = "INTERNAL_ERROR"
	publicHTTPContactNotFound    = "CONTACT_NOT_FOUND"
	publicHTTPContactExists      = "CONTACT_ALREADY_EXISTS"
	publicHTTPFirmwareNotFound   = "FIRMWARE_NOT_FOUND"

	maxPublicContactPageSize = 200
)

var errPublicHTTPOwnerMissing = errors.New("API key owner is missing from the request context")

func unauthorizedPublicHTTP() apitypes.ErrorResponse {
	return apiError("INVALID_API_KEY", "missing or invalid bearer API key")
}

func internalPublicHTTP() apitypes.ErrorResponse {
	return apiError(publicHTTPInternalErrorCode, http.StatusText(http.StatusInternalServerError))
}

// publicHTTPOwner returns the owner Peer bound to the authenticated API key.
// Handlers never accept a Peer selector from the request.
func publicHTTPOwner(ctx context.Context) (giznet.PublicKey, error) {
	owner := peerhttp.CallerPublicKey(ctx)
	if owner.IsZero() {
		return giznet.PublicKey{}, errPublicHTTPOwnerMissing
	}
	return owner, nil
}

func (s *peerHTTP) deviceReads(owner giznet.PublicKey) (peerresource.DeviceReads, bool) {
	if s == nil || s.DeviceReads == nil {
		return peerresource.DeviceReads{}, false
	}
	return s.DeviceReads(owner), true
}

func (s *peerHTTP) GetDevice(ctx context.Context, _ peerhttp.GetDeviceRequestObject) (peerhttp.GetDeviceResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetDevice401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	reads, ok := s.deviceReads(owner)
	if !ok {
		return peerhttp.GetDevice500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	info, err := reads.DeviceInfo(ctx)
	if err != nil {
		return peerhttp.GetDevice500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.GetDevice200JSONResponse(info), nil
}

func (s *peerHTTP) GetDeviceRuntime(ctx context.Context, _ peerhttp.GetDeviceRuntimeRequestObject) (peerhttp.GetDeviceRuntimeResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetDeviceRuntime401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	reads, ok := s.deviceReads(owner)
	if !ok {
		return peerhttp.GetDeviceRuntime500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	runtime, err := reads.DeviceRuntime(ctx)
	if err != nil {
		return peerhttp.GetDeviceRuntime500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.GetDeviceRuntime200JSONResponse(runtime), nil
}

func (s *peerHTTP) GetDeviceStatus(ctx context.Context, _ peerhttp.GetDeviceStatusRequestObject) (peerhttp.GetDeviceStatusResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetDeviceStatus401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	reads, ok := s.deviceReads(owner)
	if !ok {
		return peerhttp.GetDeviceStatus500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	status, err := reads.DeviceStatus(ctx)
	if err != nil {
		return peerhttp.GetDeviceStatus500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.GetDeviceStatus200JSONResponse(status), nil
}

func (s *peerHTTP) GetDeviceFirmware(ctx context.Context, _ peerhttp.GetDeviceFirmwareRequestObject) (peerhttp.GetDeviceFirmwareResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetDeviceFirmware401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	reads, ok := s.deviceReads(owner)
	if !ok {
		return peerhttp.GetDeviceFirmware500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	item, err := reads.DeviceFirmware(ctx)
	if err != nil {
		if errors.Is(err, peerresource.ErrDeviceFirmwareNotBound) || errors.Is(err, kv.ErrNotFound) {
			return peerhttp.GetDeviceFirmware404JSONResponse{FirmwareNotFoundJSONResponse: peerhttp.FirmwareNotFoundJSONResponse(apiError(publicHTTPFirmwareNotFound, "the device has no firmware configuration bound"))}, nil
		}
		return peerhttp.GetDeviceFirmware500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.GetDeviceFirmware200JSONResponse{Description: item.Description, Slots: item.Slots}, nil
}

func (s *peerHTTP) GetDeviceTelemetryLatest(ctx context.Context, request peerhttp.GetDeviceTelemetryLatestRequestObject) (peerhttp.GetDeviceTelemetryLatestResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetDeviceTelemetryLatest401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	fields, err := parsePeerTelemetryFields(request.Params.Fields)
	if err != nil {
		return peerhttp.GetDeviceTelemetryLatest400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, err.Error()))}, nil
	}
	reads, ok := s.deviceReads(owner)
	if !ok {
		return peerhttp.GetDeviceTelemetryLatest500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	response, err := reads.DeviceTelemetryLatest(ctx, fields)
	if err != nil {
		if status, body := publicTelemetryError(err); status == http.StatusBadRequest {
			return peerhttp.GetDeviceTelemetryLatest400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		}
		return peerhttp.GetDeviceTelemetryLatest500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.GetDeviceTelemetryLatest200JSONResponse(response), nil
}

func (s *peerHTTP) QueryDeviceTelemetry(ctx context.Context, request peerhttp.QueryDeviceTelemetryRequestObject) (peerhttp.QueryDeviceTelemetryResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.QueryDeviceTelemetry401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	reads, ok := s.deviceReads(owner)
	if !ok {
		return peerhttp.QueryDeviceTelemetry500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
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
	response, err := reads.DeviceTelemetryRange(ctx, request.Params.Field, time.UnixMilli(request.Params.StartTimeMs), time.UnixMilli(request.Params.EndTimeMs), step, limit, order)
	if err != nil {
		if status, body := publicTelemetryError(err); status == http.StatusBadRequest {
			return peerhttp.QueryDeviceTelemetry400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		}
		return peerhttp.QueryDeviceTelemetry500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.QueryDeviceTelemetry200JSONResponse(response), nil
}

func (s *peerHTTP) AggregateDeviceTelemetry(ctx context.Context, request peerhttp.AggregateDeviceTelemetryRequestObject) (peerhttp.AggregateDeviceTelemetryResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.AggregateDeviceTelemetry401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	reads, ok := s.deviceReads(owner)
	if !ok {
		return peerhttp.AggregateDeviceTelemetry500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	response, err := reads.DeviceTelemetryAggregate(ctx, request.Params.Field, time.UnixMilli(request.Params.StartTimeMs), time.UnixMilli(request.Params.EndTimeMs), time.Duration(request.Params.BucketMs)*time.Millisecond, request.Params.Aggregate)
	if err != nil {
		if status, body := publicTelemetryError(err); status == http.StatusBadRequest {
			return peerhttp.AggregateDeviceTelemetry400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		}
		return peerhttp.AggregateDeviceTelemetry500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.AggregateDeviceTelemetry200JSONResponse(response), nil
}

// publicTelemetryError keeps the Admin telemetry query contract but redacts
// store and configuration failures behind the public INTERNAL_ERROR code.
func publicTelemetryError(err error) (int, apitypes.ErrorResponse) {
	if errors.Is(err, peertelemetry.ErrInvalidQuery) || errors.Is(err, peertelemetry.ErrInvalidPeer) {
		return http.StatusBadRequest, apiError(publicHTTPInvalidRequestCode, err.Error())
	}
	return http.StatusInternalServerError, internalPublicHTTP()
}

func (s *peerHTTP) ListContacts(ctx context.Context, request peerhttp.ListContactsRequestObject) (peerhttp.ListContactsResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ListContacts401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.Contacts == nil {
		return peerhttp.ListContacts500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	params := rpcapi.ContactListRequest{}
	if request.Params.Cursor != nil {
		cursor := strings.TrimSpace(*request.Params.Cursor)
		if cursor == "" {
			return peerhttp.ListContacts400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "cursor must not be empty"))}, nil
		}
		params.Cursor = &cursor
	}
	if request.Params.Limit != nil {
		limit := int(*request.Params.Limit)
		if limit < 1 || limit > maxPublicContactPageSize {
			return peerhttp.ListContacts400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "limit must be between 1 and 200"))}, nil
		}
		params.Limit = &limit
	}
	result, err := s.Contacts.ListContacts(ctx, owner.String(), params)
	if err != nil {
		switch status, body := publicContactError(err); status {
		case http.StatusConflict:
			return peerhttp.ListContacts409JSONResponse{ConflictJSONResponse: peerhttp.ConflictJSONResponse(body)}, nil
		default:
			return peerhttp.ListContacts500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	items := make([]peerhttp.Contact, len(result.Items))
	for i := range result.Items {
		items[i] = publicContact(result.Items[i])
	}
	return peerhttp.ListContacts200JSONResponse{Items: items, HasNext: result.HasNext, NextCursor: result.NextCursor}, nil
}

func (s *peerHTTP) CreateContact(ctx context.Context, request peerhttp.CreateContactRequestObject) (peerhttp.CreateContactResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.CreateContact401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.Contacts == nil {
		return peerhttp.CreateContact500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if request.Body == nil {
		return peerhttp.CreateContact400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "request body is required"))}, nil
	}
	item, err := s.Contacts.CreateContact(ctx, owner.String(), rpcapi.ContactCreateRequest{
		Name: request.Body.Name, DisplayName: request.Body.DisplayName, PhoneNumber: request.Body.PhoneNumber,
	})
	if err != nil {
		switch status, body := publicContactError(err); status {
		case http.StatusBadRequest:
			return peerhttp.CreateContact400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		case http.StatusConflict:
			return peerhttp.CreateContact409JSONResponse(body), nil
		default:
			return peerhttp.CreateContact500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.CreateContact201JSONResponse(publicContact(item)), nil
}

func (s *peerHTTP) GetContact(ctx context.Context, request peerhttp.GetContactRequestObject) (peerhttp.GetContactResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetContact401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.Contacts == nil {
		return peerhttp.GetContact500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	item, err := s.Contacts.GetContact(ctx, owner.String(), rpcapi.ContactGetRequest{Name: request.ContactName})
	if err != nil {
		switch status, body := publicContactError(err); status {
		case http.StatusNotFound:
			return peerhttp.GetContact404JSONResponse(body), nil
		case http.StatusConflict:
			return peerhttp.GetContact409JSONResponse{ConflictJSONResponse: peerhttp.ConflictJSONResponse(body)}, nil
		default:
			return peerhttp.GetContact500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.GetContact200JSONResponse(publicContact(item)), nil
}

func (s *peerHTTP) PutContact(ctx context.Context, request peerhttp.PutContactRequestObject) (peerhttp.PutContactResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.PutContact401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.Contacts == nil {
		return peerhttp.PutContact500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if request.Body == nil {
		return peerhttp.PutContact400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "request body is required"))}, nil
	}
	item, err := s.Contacts.PutContact(ctx, owner.String(), rpcapi.ContactPutRequest{
		Name: request.ContactName, DisplayName: request.Body.DisplayName, PhoneNumber: request.Body.PhoneNumber,
	})
	if err != nil {
		switch status, body := publicContactError(err); status {
		case http.StatusBadRequest:
			return peerhttp.PutContact400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		case http.StatusNotFound:
			return peerhttp.PutContact404JSONResponse(body), nil
		case http.StatusConflict:
			return peerhttp.PutContact409JSONResponse(body), nil
		default:
			return peerhttp.PutContact500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.PutContact200JSONResponse(publicContact(item)), nil
}

func (s *peerHTTP) DeleteContact(ctx context.Context, request peerhttp.DeleteContactRequestObject) (peerhttp.DeleteContactResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.DeleteContact401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.Contacts == nil {
		return peerhttp.DeleteContact500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if _, err := s.Contacts.DeleteContact(ctx, owner.String(), rpcapi.ContactDeleteRequest{Name: request.ContactName}); err != nil {
		switch status, body := publicContactError(err); status {
		case http.StatusNotFound:
			return peerhttp.DeleteContact404JSONResponse(body), nil
		case http.StatusConflict:
			return peerhttp.DeleteContact409JSONResponse{ConflictJSONResponse: peerhttp.ConflictJSONResponse(body)}, nil
		default:
			return peerhttp.DeleteContact500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.DeleteContact204Response{}, nil
}

func publicContact(item rpcapi.ContactObject) peerhttp.Contact {
	return peerhttp.Contact{
		Name: item.Name, DisplayName: item.DisplayName, PhoneNumber: item.PhoneNumber,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

// publicContactError maps contact service failures onto the Public HTTP
// contract. Cross-owner and unknown names collapse into one 404; validation
// failures are 400; store failures are redacted as 500.
func publicContactError(err error) (int, apitypes.ErrorResponse) {
	switch {
	case errors.Is(err, kv.ErrNotFound):
		return http.StatusNotFound, apiError(publicHTTPContactNotFound, "contact not found")
	case errors.Is(err, socialutil.ErrResourceAlreadyExists):
		return http.StatusConflict, apiError(publicHTTPContactExists, err.Error())
	case errors.Is(err, contact.ErrPeerPendingDeletion):
		return http.StatusConflict, apiError(contact.PeerPendingDeletionCode, err.Error())
	case errors.Is(err, contact.ErrPeerDeleted):
		return http.StatusConflict, apiError(contact.PeerDeletedCode, err.Error())
	case strings.HasPrefix(err.Error(), "social: contact "):
		return http.StatusBadRequest, apiError(publicHTTPInvalidRequestCode, strings.TrimPrefix(err.Error(), "social: "))
	default:
		return http.StatusInternalServerError, internalPublicHTTP()
	}
}
