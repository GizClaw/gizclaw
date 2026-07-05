package peerresource

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
)

func (s *Server) ListModels(ctx context.Context, request adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error) {
	if s.Models == nil {
		return adminservice.ListModels500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "model service not configured")), nil
	}
	resp, err := s.Models.ListModels(ctx, request)
	if err != nil {
		return nil, err
	}
	list, rpcResp, err := adminResult[adminservice.ModelList](resp.VisitListModelsResponse)
	if err != nil {
		return resp, nil
	}
	if rpcResp != nil {
		return resp, nil
	}
	items := make([]apitypes.Model, 0, len(list.Items))
	for _, item := range list.Items {
		err := s.authorizeErr(ctx, acl.ModelResource(item.Id), apitypes.ACLPermissionRead)
		if errors.Is(err, acl.ErrDenied) {
			continue
		}
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	list.Items = items
	return adminservice.ListModels200JSONResponse(list), nil
}

func (s *Server) GetModel(ctx context.Context, request adminservice.GetModelRequestObject) (adminservice.GetModelResponseObject, error) {
	if s.Models == nil {
		return adminservice.GetModel500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "model service not configured")), nil
	}
	if err := s.authorizeErr(ctx, acl.ModelResource(request.Id), apitypes.ACLPermissionRead); err != nil {
		return adminservice.GetModel500JSONResponse(apitypes.NewErrorResponse("ACL_DENIED", err.Error())), nil
	}
	return s.Models.GetModel(ctx, request)
}

func (s *Server) GetCredential(ctx context.Context, request adminservice.GetCredentialRequestObject) (adminservice.GetCredentialResponseObject, error) {
	if s.Credentials == nil {
		return adminservice.GetCredential500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "credential service not configured")), nil
	}
	if err := s.authorizeErr(ctx, acl.CredentialResource(request.Name), apitypes.ACLPermissionRead); err != nil {
		return adminservice.GetCredential500JSONResponse(apitypes.NewErrorResponse("ACL_DENIED", err.Error())), nil
	}
	return s.Credentials.GetCredential(ctx, request)
}

func (s *Server) ListVoices(ctx context.Context, request adminservice.ListVoicesRequestObject) (adminservice.ListVoicesResponseObject, error) {
	if s.Voices == nil {
		return adminservice.ListVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "voice service not configured")), nil
	}
	cursor := request.Params.Cursor
	limit := int32(50)
	if request.Params.Limit != nil && *request.Params.Limit > 0 {
		limit = *request.Params.Limit
	}
	if limit > 200 {
		limit = 200
	}

	out := adminservice.VoiceList{Items: []apitypes.Voice{}}
	for {
		pageReq := request
		pageReq.Params.Cursor = cursor
		pageReq.Params.Limit = &limit
		resp, err := s.Voices.ListVoices(ctx, pageReq)
		if err != nil {
			return nil, err
		}
		list, rpcResp, err := adminResult[adminservice.VoiceList](resp.VisitListVoicesResponse)
		if err != nil {
			return resp, nil
		}
		if rpcResp != nil {
			return resp, nil
		}
		for _, item := range list.Items {
			err := s.authorizeErr(ctx, acl.VoiceResource(string(item.Id)), apitypes.ACLPermissionRead)
			if errors.Is(err, acl.ErrDenied) {
				continue
			}
			if err != nil {
				return nil, err
			}
			out.Items = append(out.Items, item)
			if int32(len(out.Items)) >= limit {
				out.HasNext = list.HasNext
				out.NextCursor = list.NextCursor
				return adminservice.ListVoices200JSONResponse(out), nil
			}
		}
		if !list.HasNext || list.NextCursor == nil || *list.NextCursor == "" {
			return adminservice.ListVoices200JSONResponse(out), nil
		}
		cursor = list.NextCursor
	}
}

func (s *Server) GetVoice(ctx context.Context, request adminservice.GetVoiceRequestObject) (adminservice.GetVoiceResponseObject, error) {
	if s.Voices == nil {
		return adminservice.GetVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", "voice service not configured")), nil
	}
	id := string(request.Id)
	if err := s.authorizeErr(ctx, acl.VoiceResource(id), apitypes.ACLPermissionRead); err != nil {
		return adminservice.GetVoice500JSONResponse(apitypes.NewErrorResponse("ACL_DENIED", err.Error())), nil
	}
	return s.Voices.GetVoice(ctx, request)
}
