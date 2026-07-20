package peerresource

import (
	"context"
	"net/http"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (s *Server) ListModels(ctx context.Context, request adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
	if s.Models == nil {
		return adminhttp.ListModels500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "model service not configured")), nil
	}
	items, err := s.effectiveModels(ctx)
	if err != nil {
		return adminhttp.ListModels500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	requested := 50
	if request.Params.Limit != nil {
		requested = int(*request.Params.Limit)
	}
	page, hasNext, nextCursor := pageModels(items, request.Params.Cursor, &requested)
	return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{Items: page, HasNext: hasNext, NextCursor: nextCursor}), nil
}

func (s *Server) GetModel(ctx context.Context, request adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	if s.Models == nil {
		return adminhttp.GetModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "model service not configured")), nil
	}
	response, err := s.Models.GetModel(ctx, request)
	if err != nil {
		return nil, err
	}
	_, rpcResponse, decodeErr := adminResult[apitypes.Model](response.VisitGetModelResponse)
	if decodeErr != nil || rpcResponse != nil {
		return response, nil
	}
	if !s.profileAllows(profileModels, request.Id) {
		return adminhttp.GetModel404JSONResponse(apitypes.NewErrorResponse("MODEL_NOT_FOUND", "model not found")), nil
	}
	return response, nil
}

func (s *Server) ListVoices(ctx context.Context, request adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error) {
	if s.Voices == nil {
		return adminhttp.ListVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "voice service not configured")), nil
	}
	items := make([]apitypes.Voice, 0, len(s.profileNames(profileVoices)))
	for _, id := range s.profileNames(profileVoices) {
		response, err := s.Voices.GetVoice(ctx, adminhttp.GetVoiceRequestObject{Id: id})
		if err != nil {
			return nil, err
		}
		item, rpcResponse, err := adminResult[apitypes.Voice](response.VisitGetVoiceResponse)
		if err != nil {
			return nil, err
		}
		if isNotFoundResponse(rpcResponse) {
			continue
		}
		if rpcResponse != nil {
			return adminhttp.ListVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", rpcResponse.Error.Message)), nil
		}
		if !voiceMatchesListParams(item, request.Params) {
			continue
		}
		items = append(items, item)
	}
	requested := 50
	if request.Params.Limit != nil {
		requested = int(*request.Params.Limit)
	}
	page, hasNext, nextCursor := pageVoices(items, request.Params.Cursor, &requested)
	return adminhttp.ListVoices200JSONResponse(adminhttp.VoiceList{
		Items:      page,
		HasNext:    hasNext,
		NextCursor: nextCursor,
	}), nil
}

func voiceMatchesListParams(item apitypes.Voice, params adminhttp.ListVoicesParams) bool {
	if params.Source != nil {
		source := strings.TrimSpace(string(*params.Source))
		if source != "" && string(item.Source) != source {
			return false
		}
	}
	if params.ProviderKind != nil {
		kind := strings.TrimSpace(string(*params.ProviderKind))
		if kind != "" && string(item.Provider.Kind) != kind {
			return false
		}
	}
	if params.ProviderName != nil {
		name := strings.TrimSpace(*params.ProviderName)
		if name != "" && item.Provider.Name != name {
			return false
		}
	}
	return true
}

func (s *Server) GetVoice(ctx context.Context, request adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	if s.Voices == nil {
		return adminhttp.GetVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "voice service not configured")), nil
	}
	id := string(request.Id)
	if !s.profileAllows(profileVoices, id) {
		return adminhttp.GetVoice404JSONResponse(apitypes.NewErrorResponse("VOICE_NOT_FOUND", http.StatusText(http.StatusNotFound))), nil
	}
	return s.Voices.GetVoice(ctx, request)
}
