package peer

import (
	"context"
	"sort"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
)

func validLookupIdentifier(value string) bool {
	return value != "" && len(value) <= 256 && utf8.ValidString(value)
}

// FindPublicKeysBySN exposes all matching public keys without device metadata.
func (s *Server) FindPublicKeysBySN(ctx context.Context, request peerhttp.FindPublicKeysBySNRequestObject) (peerhttp.FindPublicKeysBySNResponseObject, error) {
	if !validLookupIdentifier(request.Sn) {
		return peerhttp.FindPublicKeysBySN400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apitypes.NewErrorResponse("INVALID_REQUEST", "invalid serial number"))}, nil
	}
	items, err := s.listBySN(ctx, request.Sn)
	if err != nil {
		return peerhttp.FindPublicKeysBySN500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "cannot query device identifiers"))}, nil
	}
	keys := make([]string, 0, len(items))
	for _, item := range items {
		registration, err := item.AsExternalRef0Registration()
		if err != nil {
			return nil, err
		}
		keys = append(keys, registration.PublicKey)
	}
	sort.Strings(keys)
	return peerhttp.FindPublicKeysBySN200JSONResponse{PublicKeys: keys}, nil
}

// FindPublicKeysByIMEI exposes all matching public keys without device metadata.
func (s *Server) FindPublicKeysByIMEI(ctx context.Context, request peerhttp.FindPublicKeysByIMEIRequestObject) (peerhttp.FindPublicKeysByIMEIResponseObject, error) {
	if !validLookupIdentifier(request.Tac) || !validLookupIdentifier(request.Serial) {
		return peerhttp.FindPublicKeysByIMEI400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apitypes.NewErrorResponse("INVALID_REQUEST", "invalid IMEI"))}, nil
	}
	keys, err := s.ListPublicKeysByIMEI(ctx, request.Tac, request.Serial)
	if err != nil {
		return peerhttp.FindPublicKeysByIMEI500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "cannot query device identifiers"))}, nil
	}
	return peerhttp.FindPublicKeysByIMEI200JSONResponse{PublicKeys: keys}, nil
}
