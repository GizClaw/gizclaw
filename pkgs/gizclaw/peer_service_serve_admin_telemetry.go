package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func (s *adminService) GetPeerTelemetryLatest(ctx context.Context, request adminhttp.GetPeerTelemetryLatestRequestObject) (adminhttp.GetPeerTelemetryLatestResponseObject, error) {
	peer, err := parseAdminTelemetryPublicKey(request.PublicKey)
	if err != nil {
		return adminhttp.GetPeerTelemetryLatest400JSONResponse(apitypes.NewErrorResponse("INVALID_TELEMETRY_QUERY", err.Error())), nil
	}
	fields, err := parsePeerTelemetryFields(request.Params.Fields)
	if err != nil {
		return adminhttp.GetPeerTelemetryLatest400JSONResponse(apitypes.NewErrorResponse("INVALID_TELEMETRY_QUERY", err.Error())), nil
	}
	service := s.PeerTelemetry
	if service == nil {
		return adminhttp.GetPeerTelemetryLatest500JSONResponse(telemetryNotConfiguredResponse()), nil
	}
	response, err := service.Latest(ctx, peer, fields)
	if err != nil {
		status, body := peerTelemetryAdminError(err)
		if status == 400 {
			return adminhttp.GetPeerTelemetryLatest400JSONResponse(body), nil
		}
		return adminhttp.GetPeerTelemetryLatest500JSONResponse(body), nil
	}
	return adminhttp.GetPeerTelemetryLatest200JSONResponse(response), nil
}

func (s *adminService) QueryPeerTelemetry(ctx context.Context, request adminhttp.QueryPeerTelemetryRequestObject) (adminhttp.QueryPeerTelemetryResponseObject, error) {
	peer, err := parseAdminTelemetryPublicKey(request.PublicKey)
	if err != nil {
		return adminhttp.QueryPeerTelemetry400JSONResponse(apitypes.NewErrorResponse("INVALID_TELEMETRY_QUERY", err.Error())), nil
	}
	service := s.PeerTelemetry
	if service == nil {
		return adminhttp.QueryPeerTelemetry500JSONResponse(telemetryNotConfiguredResponse()), nil
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
	response, err := service.QueryRange(
		ctx,
		peer,
		request.Params.Field,
		time.UnixMilli(request.Params.StartTimeMs),
		time.UnixMilli(request.Params.EndTimeMs),
		step,
		limit,
		order,
	)
	if err != nil {
		status, body := peerTelemetryAdminError(err)
		if status == 400 {
			return adminhttp.QueryPeerTelemetry400JSONResponse(body), nil
		}
		return adminhttp.QueryPeerTelemetry500JSONResponse(body), nil
	}
	return adminhttp.QueryPeerTelemetry200JSONResponse(response), nil
}

func (s *adminService) AggregatePeerTelemetry(ctx context.Context, request adminhttp.AggregatePeerTelemetryRequestObject) (adminhttp.AggregatePeerTelemetryResponseObject, error) {
	peer, err := parseAdminTelemetryPublicKey(request.PublicKey)
	if err != nil {
		return adminhttp.AggregatePeerTelemetry400JSONResponse(apitypes.NewErrorResponse("INVALID_TELEMETRY_QUERY", err.Error())), nil
	}
	service := s.PeerTelemetry
	if service == nil {
		return adminhttp.AggregatePeerTelemetry500JSONResponse(telemetryNotConfiguredResponse()), nil
	}
	response, err := service.Aggregate(
		ctx,
		peer,
		request.Params.Field,
		time.UnixMilli(request.Params.StartTimeMs),
		time.UnixMilli(request.Params.EndTimeMs),
		time.Duration(request.Params.BucketMs)*time.Millisecond,
		request.Params.Aggregate,
	)
	if err != nil {
		status, body := peerTelemetryAdminError(err)
		if status == 400 {
			return adminhttp.AggregatePeerTelemetry400JSONResponse(body), nil
		}
		return adminhttp.AggregatePeerTelemetry500JSONResponse(body), nil
	}
	return adminhttp.AggregatePeerTelemetry200JSONResponse(response), nil
}

func parseAdminTelemetryPublicKey(value string) (giznet.PublicKey, error) {
	text, err := url.PathUnescape(value)
	if err != nil {
		return giznet.PublicKey{}, err
	}
	var key giznet.PublicKey
	if err := key.UnmarshalText([]byte(text)); err != nil {
		return giznet.PublicKey{}, fmt.Errorf("invalid public key: %w", err)
	}
	if key.IsZero() {
		return giznet.PublicKey{}, errors.New("public key is empty")
	}
	return key, nil
}

func parsePeerTelemetryFields(value *string) ([]apitypes.PeerTelemetryField, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parts := strings.Split(*value, ",")
	fields := make([]apitypes.PeerTelemetryField, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, errors.New("fields contains an empty field")
		}
		field := apitypes.PeerTelemetryField(part)
		if !field.Valid() {
			return nil, fmt.Errorf("invalid field %q", part)
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func telemetryNotConfiguredResponse() apitypes.ErrorResponse {
	return apitypes.NewErrorResponse("TELEMETRY_QUERY_NOT_CONFIGURED", "telemetry metrics store is not configured")
}

func peerTelemetryAdminError(err error) (int, apitypes.ErrorResponse) {
	switch {
	case errors.Is(err, peertelemetry.ErrInvalidQuery), errors.Is(err, peertelemetry.ErrInvalidPeer):
		return 400, apitypes.NewErrorResponse("INVALID_TELEMETRY_QUERY", err.Error())
	case errors.Is(err, peertelemetry.ErrMetricsStoreNil):
		return 500, telemetryNotConfiguredResponse()
	default:
		return 500, apitypes.NewErrorResponse("TELEMETRY_QUERY_FAILED", err.Error())
	}
}
