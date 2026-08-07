package gizclaw

import (
	"context"
	"errors"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/google/uuid"
)

func (s *adminService) ListPendingDeletions(ctx context.Context, request adminhttp.ListPendingDeletionsRequestObject) (adminhttp.ListPendingDeletionsResponseObject, error) {
	listRequest := pendingdeletion.ListRequest{}
	if request.Params.Source != nil {
		listRequest.Source = *request.Params.Source
	}
	if request.Params.Kind != nil {
		listRequest.Kind = pendingdeletion.Kind(*request.Params.Kind)
	}
	if request.Params.Status != nil {
		listRequest.Status = pendingdeletion.Status(*request.Params.Status)
	}
	if request.Params.StartTimeMs != nil {
		value := time.UnixMilli(*request.Params.StartTimeMs).UTC()
		listRequest.StartTime = &value
	}
	if request.Params.EndTimeMs != nil {
		value := time.UnixMilli(*request.Params.EndTimeMs).UTC()
		listRequest.EndTime = &value
	}
	if request.Params.Limit != nil {
		listRequest.Limit = int(*request.Params.Limit)
	}
	if request.Params.Cursor != nil {
		listRequest.Cursor = *request.Params.Cursor
	}
	result, err := s.PendingDeletions.List(ctx, listRequest)
	if err != nil {
		status, body := pendingDeletionError(err)
		if status == 400 {
			return adminhttp.ListPendingDeletions400JSONResponse(body), nil
		}
		return adminhttp.ListPendingDeletions500JSONResponse(body), nil
	}
	items := make([]apitypes.PendingDeletionTask, 0, len(result.Tasks))
	for _, task := range result.Tasks {
		items = append(items, pendingDeletionProjection(task))
	}
	response := apitypes.PendingDeletionList{Items: items}
	if result.NextCursor != "" {
		response.NextCursor = &result.NextCursor
	}
	return adminhttp.ListPendingDeletions200JSONResponse(response), nil
}

func (s *adminService) GetPendingDeletion(ctx context.Context, request adminhttp.GetPendingDeletionRequestObject) (adminhttp.GetPendingDeletionResponseObject, error) {
	task, err := s.PendingDeletions.Get(ctx, request.Params.Source, request.DeletionId.String())
	if err != nil {
		status, body := pendingDeletionError(err)
		switch status {
		case 400:
			return adminhttp.GetPendingDeletion400JSONResponse(body), nil
		case 404:
			return adminhttp.GetPendingDeletion404JSONResponse(body), nil
		default:
			return adminhttp.GetPendingDeletion500JSONResponse(body), nil
		}
	}
	return adminhttp.GetPendingDeletion200JSONResponse(pendingDeletionProjection(task)), nil
}

func (s *adminService) RetryPendingDeletion(ctx context.Context, request adminhttp.RetryPendingDeletionRequestObject) (adminhttp.RetryPendingDeletionResponseObject, error) {
	task, err := s.PendingDeletions.Retry(ctx, request.Params.Source, request.DeletionId.String())
	if err != nil {
		status, body := pendingDeletionError(err)
		switch status {
		case 400:
			return adminhttp.RetryPendingDeletion400JSONResponse(body), nil
		case 404:
			return adminhttp.RetryPendingDeletion404JSONResponse(body), nil
		case 409:
			return adminhttp.RetryPendingDeletion409JSONResponse(body), nil
		default:
			return adminhttp.RetryPendingDeletion500JSONResponse(body), nil
		}
	}
	return adminhttp.RetryPendingDeletion200JSONResponse(pendingDeletionProjection(task)), nil
}

func pendingDeletionProjection(task pendingdeletion.Task) apitypes.PendingDeletionTask {
	result := apitypes.PendingDeletionTask{
		Source:       task.Source,
		DeletionId:   uuid.MustParse(task.Record.DeletionID),
		Kind:         apitypes.PendingDeletionKind(task.Record.Kind),
		ResourceId:   task.Record.ResourceID,
		Status:       apitypes.PendingDeletionStatus(task.Status),
		Phase:        string(task.Phase),
		FailureCount: int32(task.FailureCount),
		CreatedAt:    task.Record.DeletedAt.UTC(),
		UpdatedAt:    task.UpdatedAt.UTC(),
	}
	if !task.NextAttemptAt.IsZero() {
		value := task.NextAttemptAt.UTC()
		result.NextAttemptAt = &value
	}
	if !task.LeaseDeadline.IsZero() {
		value := task.LeaseDeadline.UTC()
		result.LeaseDeadline = &value
	}
	if task.LastErrorCode != "" {
		value := task.LastErrorCode
		result.LastErrorCode = &value
	}
	if task.LastErrorMessage != "" {
		value := task.LastErrorMessage
		result.LastErrorMessage = &value
	}
	return result
}

func pendingDeletionError(err error) (int, apitypes.ErrorResponse) {
	switch {
	case errors.Is(err, pendingdeletion.ErrInvalid):
		return 400, apitypes.NewErrorResponse("INVALID_PENDING_DELETION_REQUEST", "pending deletion request is invalid")
	case errors.Is(err, pendingdeletion.ErrNotFound):
		return 404, apitypes.NewErrorResponse("PENDING_DELETION_NOT_FOUND", "active pending deletion task not found")
	case errors.Is(err, pendingdeletion.ErrConflict):
		return 409, apitypes.NewErrorResponse("PENDING_DELETION_CONFLICT", "pending deletion task is not retryable")
	default:
		return 500, apitypes.NewErrorResponse("PENDING_DELETION_ERROR", "pending deletion operation failed")
	}
}
