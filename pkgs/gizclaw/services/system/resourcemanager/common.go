package resourcemanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func applyNamedResource[T any, S any](
	ctx context.Context,
	metadata apitypes.ResourceMetadata,
	kind apitypes.ResourceKind,
	desired S,
	get func(context.Context, string) (T, bool, error),
	create func(context.Context) (string, error),
	put func(context.Context, string) error,
	nameOf func(T) string,
	specOf func(T) S,
) (apitypes.ApplyResult, error) {
	id, updating, err := resourceUpdateID(metadata)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !updating {
		createdID, err := create(ctx)
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		return applyResult(apitypes.ApplyActionCreated, kind, metadata.Name, createdID), nil
	}
	existing, exists, err := get(ctx, id)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !exists {
		return apitypes.ApplyResult{}, notFound(kind, id)
	}
	if err := validateImmutableResourceName(kind, id, nameOf(existing), metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	same, err := semanticEqual(specOf(existing), desired)
	if err != nil {
		return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
	}
	if same {
		return applyResult(apitypes.ApplyActionUnchanged, kind, metadata.Name, id), nil
	}
	if err := put(ctx, id); err != nil {
		return apitypes.ApplyResult{}, err
	}
	return applyResult(apitypes.ApplyActionUpdated, kind, metadata.Name, id), nil
}

func marshalResource(in any) (apitypes.Resource, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return apitypes.Resource{}, err
	}
	var resource apitypes.Resource
	if err := json.Unmarshal(data, &resource); err != nil {
		return apitypes.Resource{}, err
	}
	return resource, nil
}

func resourceMetadata(resource apitypes.Resource) (apitypes.ResourceMetadata, error) {
	data, err := json.Marshal(resource)
	if err != nil {
		return apitypes.ResourceMetadata{}, applyError(400, "INVALID_RESOURCE", err.Error())
	}
	var header struct {
		Metadata apitypes.ResourceMetadata `json:"metadata"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return apitypes.ResourceMetadata{}, applyError(400, "INVALID_RESOURCE", err.Error())
	}
	return header.Metadata, nil
}

func resourceKind(resource apitypes.Resource) (apitypes.ResourceKind, error) {
	data, err := json.Marshal(resource)
	if err != nil {
		return "", applyError(400, "INVALID_RESOURCE", err.Error())
	}
	var header struct {
		Kind apitypes.ResourceKind `json:"kind"`
	}
	if err := json.Unmarshal(data, &header); err != nil || header.Kind == "" {
		if err == nil {
			err = fmt.Errorf("kind is required")
		}
		return "", applyError(400, "INVALID_RESOURCE", err.Error())
	}
	return header.Kind, nil
}

func validateResourceHeader(apiVersion apitypes.ResourceAPIVersion, name string) error {
	if apiVersion != apitypes.ResourceAPIVersionGizclawAdminv1alpha1 {
		return applyError(400, "UNSUPPORTED_RESOURCE_VERSION", fmt.Sprintf("unsupported resource apiVersion %q", apiVersion))
	}
	if name == "" {
		return applyError(400, "INVALID_RESOURCE", "metadata.name is required")
	}
	return nil
}

func resourceUpdateID(metadata apitypes.ResourceMetadata) (string, bool, error) {
	if metadata.Id == nil {
		return "", false, nil
	}
	id := *metadata.Id
	if id == "" {
		return "", false, applyError(400, "INVALID_RESOURCE", "metadata.id must not be empty")
	}
	return id, true, nil
}

func requireResourceUpdateID(metadata apitypes.ResourceMetadata) (string, error) {
	id, updating, err := resourceUpdateID(metadata)
	if err != nil {
		return "", err
	}
	if !updating {
		return "", applyError(400, "RESOURCE_ID_REQUIRED", "metadata.id is required for update")
	}
	return id, nil
}

func validateImmutableResourceName(kind apitypes.ResourceKind, id, existingName, desiredName string) error {
	if existingName != desiredName {
		return applyError(409, "IMMUTABLE_RESOURCE_NAME", fmt.Sprintf("%s %q is named %q, not %q", kind, id, existingName, desiredName))
	}
	return nil
}

func semanticEqual(left, right any) (bool, error) {
	var leftValue any
	if err := normalizeJSON(left, &leftValue); err != nil {
		return false, err
	}
	var rightValue any
	if err := normalizeJSON(right, &rightValue); err != nil {
		return false, err
	}
	return reflect.DeepEqual(leftValue, rightValue), nil
}

func normalizeJSON(in any, out *any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func applyResult(action apitypes.ApplyAction, kind apitypes.ResourceKind, name string, ids ...string) apitypes.ApplyResult {
	result := apitypes.ApplyResult{
		Action:     action,
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       kind,
		Name:       name,
	}
	if len(ids) > 0 && ids[0] != "" {
		result.Id = &ids[0]
	}
	return result
}

func applyError(statusCode int, code, message string) *Error {
	return &Error{StatusCode: statusCode, Code: code, Message: message}
}

func missingService(name string) *Error {
	return applyError(500, "RESOURCE_SERVICE_NOT_CONFIGURED", fmt.Sprintf("%s service is not configured", name))
}

func notFound(kind apitypes.ResourceKind, name string) *Error {
	return applyError(404, "RESOURCE_NOT_FOUND", fmt.Sprintf("%s %q not found", kind, name))
}

func responseError(statusCode int, fallbackCode, fallbackMessage string, response any) *Error {
	body := apitypes.ErrorResponse{}
	data, err := json.Marshal(response)
	if err == nil {
		_ = json.Unmarshal(data, &body)
	}
	if body.Error.Code != "" {
		return applyError(statusCode, body.Error.Code, body.Error.Message)
	}
	return applyError(statusCode, fallbackCode, fallbackMessage)
}

func unexpectedResponse(operation string, response any) *Error {
	return applyError(500, "UNEXPECTED_SERVICE_RESPONSE", fmt.Sprintf("%s returned unexpected response %T", operation, response))
}

func pathParam(value string) string {
	return url.PathEscape(value)
}
