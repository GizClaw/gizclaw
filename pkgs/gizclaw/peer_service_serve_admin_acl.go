package gizclaw

import (
	"context"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
)

func (s *adminService) ListACLRoles(ctx context.Context, request adminhttp.ListACLRolesRequestObject) (adminhttp.ListACLRolesResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.ListACLRoles500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	cursor, limit := aclListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := server.ListRoles(ctx, acl.ListRolesRequest{Cursor: cursor, Limit: limit})
	if err != nil {
		return adminhttp.ListACLRoles500JSONResponse(apitypes.NewErrorResponse("ACL_LIST_ROLES_FAILED", err.Error())), nil
	}
	return adminhttp.ListACLRoles200JSONResponse{
		Items:      items,
		HasNext:    hasNext,
		NextCursor: nextCursor,
	}, nil
}

func (s *adminService) ListACLViews(ctx context.Context, request adminhttp.ListACLViewsRequestObject) (adminhttp.ListACLViewsResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.ListACLViews500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	cursor, limit := aclListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := server.ListViews(ctx, acl.ListViewsRequest{Cursor: cursor, Limit: limit})
	if err != nil {
		return adminhttp.ListACLViews500JSONResponse(apitypes.NewErrorResponse("ACL_LIST_VIEWS_FAILED", err.Error())), nil
	}
	return adminhttp.ListACLViews200JSONResponse{
		Items:      items,
		HasNext:    hasNext,
		NextCursor: nextCursor,
	}, nil
}

func (s *adminService) CreateACLView(ctx context.Context, request adminhttp.CreateACLViewRequestObject) (adminhttp.CreateACLViewResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.CreateACLView500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateACLView400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_VIEW", "request body is required")), nil
	}
	view, err := server.CreateView(ctx, request.Body.Name, aclViewSpec(*request.Body))
	if err != nil {
		return createACLViewError(err), nil
	}
	return adminhttp.CreateACLView200JSONResponse(view), nil
}

func (s *adminService) GetACLView(ctx context.Context, request adminhttp.GetACLViewRequestObject) (adminhttp.GetACLViewResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.GetACLView500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	view, err := server.GetView(ctx, request.Name)
	if err != nil {
		if errors.Is(err, acl.ErrViewNotFound) {
			return adminhttp.GetACLView404JSONResponse(apitypes.NewErrorResponse("ACL_VIEW_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetACLView500JSONResponse(apitypes.NewErrorResponse("ACL_GET_VIEW_FAILED", err.Error())), nil
	}
	return adminhttp.GetACLView200JSONResponse(view), nil
}

func (s *adminService) PutACLView(ctx context.Context, request adminhttp.PutACLViewRequestObject) (adminhttp.PutACLViewResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.PutACLView500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutACLView400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_VIEW", "request body is required")), nil
	}
	if request.Body.Name != request.Name {
		return adminhttp.PutACLView400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_VIEW", "view name does not match path name")), nil
	}
	view, err := server.PutView(ctx, request.Name, aclViewSpec(*request.Body))
	if err != nil {
		return putACLViewError(err), nil
	}
	return adminhttp.PutACLView200JSONResponse(view), nil
}

func (s *adminService) DeleteACLView(ctx context.Context, request adminhttp.DeleteACLViewRequestObject) (adminhttp.DeleteACLViewResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.DeleteACLView500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	view, err := server.DeleteView(ctx, request.Name)
	if err != nil {
		if errors.Is(err, acl.ErrViewNotFound) {
			return adminhttp.DeleteACLView404JSONResponse(apitypes.NewErrorResponse("ACL_VIEW_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.DeleteACLView500JSONResponse(apitypes.NewErrorResponse("ACL_DELETE_VIEW_FAILED", err.Error())), nil
	}
	return adminhttp.DeleteACLView200JSONResponse(view), nil
}

func (s *adminService) CreateACLRole(ctx context.Context, request adminhttp.CreateACLRoleRequestObject) (adminhttp.CreateACLRoleResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.CreateACLRole500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateACLRole400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_ROLE", "request body is required")), nil
	}
	role, err := server.CreateRole(ctx, request.Body.Name, request.Body.Permissions)
	if err != nil {
		return createACLRoleError(err), nil
	}
	return adminhttp.CreateACLRole200JSONResponse(role), nil
}

func (s *adminService) GetACLRole(ctx context.Context, request adminhttp.GetACLRoleRequestObject) (adminhttp.GetACLRoleResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.GetACLRole500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	role, err := server.GetRole(ctx, request.Name)
	if err != nil {
		if errors.Is(err, acl.ErrRoleNotFound) {
			return adminhttp.GetACLRole404JSONResponse(apitypes.NewErrorResponse("ACL_ROLE_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetACLRole500JSONResponse(apitypes.NewErrorResponse("ACL_GET_ROLE_FAILED", err.Error())), nil
	}
	return adminhttp.GetACLRole200JSONResponse(role), nil
}

func (s *adminService) PutACLRole(ctx context.Context, request adminhttp.PutACLRoleRequestObject) (adminhttp.PutACLRoleResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.PutACLRole500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutACLRole400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_ROLE", "request body is required")), nil
	}
	if request.Body.Name != request.Name {
		return adminhttp.PutACLRole400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_ROLE", "role name does not match path name")), nil
	}
	role, err := server.PutRole(ctx, request.Name, request.Body.Permissions)
	if err != nil {
		return putACLRoleError(err), nil
	}
	return adminhttp.PutACLRole200JSONResponse(role), nil
}

func (s *adminService) DeleteACLRole(ctx context.Context, request adminhttp.DeleteACLRoleRequestObject) (adminhttp.DeleteACLRoleResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.DeleteACLRole500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	role, err := server.DeleteRole(ctx, request.Name)
	if err != nil {
		if errors.Is(err, acl.ErrRoleNotFound) {
			return adminhttp.DeleteACLRole404JSONResponse(apitypes.NewErrorResponse("ACL_ROLE_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.DeleteACLRole500JSONResponse(apitypes.NewErrorResponse("ACL_DELETE_ROLE_FAILED", err.Error())), nil
	}
	return adminhttp.DeleteACLRole200JSONResponse(role), nil
}

func (s *adminService) ListACLPolicyBindings(ctx context.Context, request adminhttp.ListACLPolicyBindingsRequestObject) (adminhttp.ListACLPolicyBindingsResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.ListACLPolicyBindings500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	cursor, limit := aclListParams(request.Params.Cursor, request.Params.Limit)
	var permission apitypes.ACLPermission
	if request.Params.Permission != nil {
		permission = *request.Params.Permission
	}
	items, hasNext, nextCursor, err := server.ListPolicyBindings(ctx, acl.ListPolicyBindingsRequest{
		Cursor:           cursor,
		Limit:            limit,
		OrderBy:          valueOrZero(request.Params.OrderBy),
		SubjectKind:      valueOrZero(request.Params.SubjectKind),
		SubjectID:        valueOrZero(request.Params.SubjectId),
		ResourceKind:     valueOrZero(request.Params.ResourceKind),
		ResourceID:       valueOrZero(request.Params.ResourceId),
		ResourceIDPrefix: valueOrZero(request.Params.ResourceIdPrefix),
		Role:             valueOrZero(request.Params.Role),
		Permission:       permission,
	})
	if err != nil {
		return adminhttp.ListACLPolicyBindings500JSONResponse(apitypes.NewErrorResponse("ACL_LIST_POLICY_BINDINGS_FAILED", err.Error())), nil
	}
	return adminhttp.ListACLPolicyBindings200JSONResponse{
		Items:      items,
		HasNext:    hasNext,
		NextCursor: nextCursor,
	}, nil
}

func (s *adminService) CreateACLPolicyBinding(ctx context.Context, request adminhttp.CreateACLPolicyBindingRequestObject) (adminhttp.CreateACLPolicyBindingResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.CreateACLPolicyBinding500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateACLPolicyBinding400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_POLICY_BINDING", "request body is required")), nil
	}
	binding, err := server.CreatePolicyBinding(ctx, policyBindingID(request.Body.Id), policyBindingDisplayOrder(request.Body.DisplayOrder), request.Body.Policy)
	if err != nil {
		return createACLPolicyBindingError(err), nil
	}
	return adminhttp.CreateACLPolicyBinding200JSONResponse(binding), nil
}

func (s *adminService) GetACLPolicyBinding(ctx context.Context, request adminhttp.GetACLPolicyBindingRequestObject) (adminhttp.GetACLPolicyBindingResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.GetACLPolicyBinding500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	binding, err := server.GetPolicyBinding(ctx, request.Id)
	if err != nil {
		if errors.Is(err, acl.ErrPolicyBindingNotFound) {
			return adminhttp.GetACLPolicyBinding404JSONResponse(apitypes.NewErrorResponse("ACL_POLICY_BINDING_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetACLPolicyBinding500JSONResponse(apitypes.NewErrorResponse("ACL_GET_POLICY_BINDING_FAILED", err.Error())), nil
	}
	return adminhttp.GetACLPolicyBinding200JSONResponse(binding), nil
}

func (s *adminService) PutACLPolicyBinding(ctx context.Context, request adminhttp.PutACLPolicyBindingRequestObject) (adminhttp.PutACLPolicyBindingResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.PutACLPolicyBinding500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutACLPolicyBinding400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_POLICY_BINDING", "request body is required")), nil
	}
	if request.Body.Id != nil && strings.TrimSpace(*request.Body.Id) != "" && strings.TrimSpace(*request.Body.Id) != request.Id {
		return adminhttp.PutACLPolicyBinding400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_POLICY_BINDING", "policy binding id does not match path id")), nil
	}
	binding, err := server.PutPolicyBinding(ctx, request.Id, policyBindingDisplayOrder(request.Body.DisplayOrder), request.Body.Policy)
	if err != nil {
		return putACLPolicyBindingError(err), nil
	}
	return adminhttp.PutACLPolicyBinding200JSONResponse(binding), nil
}

func (s *adminService) DeleteACLPolicyBinding(ctx context.Context, request adminhttp.DeleteACLPolicyBindingRequestObject) (adminhttp.DeleteACLPolicyBindingResponseObject, error) {
	server, err := s.aclServer()
	if err != nil {
		return adminhttp.DeleteACLPolicyBinding500JSONResponse(apitypes.NewErrorResponse("ACL_NOT_CONFIGURED", err.Error())), nil
	}
	binding, err := server.DeletePolicyBinding(ctx, request.Id)
	if err != nil {
		if errors.Is(err, acl.ErrPolicyBindingNotFound) {
			return adminhttp.DeleteACLPolicyBinding404JSONResponse(apitypes.NewErrorResponse("ACL_POLICY_BINDING_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.DeleteACLPolicyBinding500JSONResponse(apitypes.NewErrorResponse("ACL_DELETE_POLICY_BINDING_FAILED", err.Error())), nil
	}
	return adminhttp.DeleteACLPolicyBinding200JSONResponse(binding), nil
}

func (s *adminService) aclServer() (*acl.Server, error) {
	if s == nil || s.ACL == nil {
		return nil, errors.New("acl server is not configured")
	}
	return s.ACL, nil
}

func aclListParams(cursor *string, limit *int32) (string, int) {
	var nextCursor string
	if cursor != nil {
		nextCursor = string(*cursor)
	}
	var nextLimit int
	if limit != nil {
		nextLimit = int(*limit)
	}
	return nextCursor, nextLimit
}

func valueOrZero[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

func policyBindingDisplayOrder(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func policyBindingID(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func aclViewSpec(value adminhttp.ACLViewUpsert) apitypes.ACLViewSpec {
	return apitypes.ACLViewSpec{
		Description: value.Description,
	}
}

func createACLViewError(err error) adminhttp.CreateACLViewResponseObject {
	switch {
	case errors.Is(err, acl.ErrViewAlreadyExists):
		return adminhttp.CreateACLView409JSONResponse(apitypes.NewErrorResponse("ACL_VIEW_ALREADY_EXISTS", err.Error()))
	case isBadACLRequest(err):
		return adminhttp.CreateACLView400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_VIEW", err.Error()))
	default:
		return adminhttp.CreateACLView500JSONResponse(apitypes.NewErrorResponse("ACL_CREATE_VIEW_FAILED", err.Error()))
	}
}

func putACLViewError(err error) adminhttp.PutACLViewResponseObject {
	if isBadACLRequest(err) {
		return adminhttp.PutACLView400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_VIEW", err.Error()))
	}
	return adminhttp.PutACLView500JSONResponse(apitypes.NewErrorResponse("ACL_PUT_VIEW_FAILED", err.Error()))
}

func createACLRoleError(err error) adminhttp.CreateACLRoleResponseObject {
	switch {
	case errors.Is(err, acl.ErrRoleAlreadyExists):
		return adminhttp.CreateACLRole409JSONResponse(apitypes.NewErrorResponse("ACL_ROLE_ALREADY_EXISTS", err.Error()))
	case isBadACLRequest(err):
		return adminhttp.CreateACLRole400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_ROLE", err.Error()))
	default:
		return adminhttp.CreateACLRole500JSONResponse(apitypes.NewErrorResponse("ACL_CREATE_ROLE_FAILED", err.Error()))
	}
}

func putACLRoleError(err error) adminhttp.PutACLRoleResponseObject {
	if isBadACLRequest(err) {
		return adminhttp.PutACLRole400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_ROLE", err.Error()))
	}
	return adminhttp.PutACLRole500JSONResponse(apitypes.NewErrorResponse("ACL_PUT_ROLE_FAILED", err.Error()))
}

func createACLPolicyBindingError(err error) adminhttp.CreateACLPolicyBindingResponseObject {
	switch {
	case errors.Is(err, acl.ErrPolicyBindingAlreadyExists):
		return adminhttp.CreateACLPolicyBinding409JSONResponse(apitypes.NewErrorResponse("ACL_POLICY_BINDING_ALREADY_EXISTS", err.Error()))
	case errors.Is(err, acl.ErrRoleNotFound), isBadACLRequest(err):
		return adminhttp.CreateACLPolicyBinding400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_POLICY_BINDING", err.Error()))
	default:
		return adminhttp.CreateACLPolicyBinding500JSONResponse(apitypes.NewErrorResponse("ACL_CREATE_POLICY_BINDING_FAILED", err.Error()))
	}
}

func putACLPolicyBindingError(err error) adminhttp.PutACLPolicyBindingResponseObject {
	if errors.Is(err, acl.ErrRoleNotFound) || isBadACLRequest(err) {
		return adminhttp.PutACLPolicyBinding400JSONResponse(apitypes.NewErrorResponse("INVALID_ACL_POLICY_BINDING", err.Error()))
	}
	return adminhttp.PutACLPolicyBinding500JSONResponse(apitypes.NewErrorResponse("ACL_PUT_POLICY_BINDING_FAILED", err.Error()))
}

func isBadACLRequest(err error) bool {
	return err != nil &&
		!errors.Is(err, acl.ErrRoleNotFound) &&
		!errors.Is(err, acl.ErrPolicyBindingNotFound) &&
		strings.HasPrefix(err.Error(), "acl:")
}
