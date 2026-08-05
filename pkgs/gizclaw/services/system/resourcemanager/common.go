package resourcemanager

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
)

func applyConcreteResource[T any, S any](
	ctx context.Context,
	metadata apitypes.ResourceMetadata,
	kind apitypes.ResourceKind,
	desired S,
	get func(context.Context, string) (T, bool, error),
	create func(context.Context) (string, error),
	put func(context.Context, string) error,
	specOf func(T) S,
) (apitypes.ApplyResult, error) {
	id, err := requireResourceID(metadata)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	transportID := servicePathID(id)
	existing, exists, err := get(ctx, transportID)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if !exists {
		createdID, err := create(ctx)
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		if createdID != id {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_ID_MISMATCH", fmt.Sprintf("%s create returned id %q, expected %q", kind, createdID, id))
		}
		return applyResult(apitypes.ApplyActionCreated, kind, id), nil
	}
	same, err := semanticEqual(specOf(existing), desired)
	if err != nil {
		return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
	}
	if same {
		return applyResult(apitypes.ApplyActionUnchanged, kind, id), nil
	}
	if err := put(ctx, transportID); err != nil {
		return apitypes.ApplyResult{}, err
	}
	return applyResult(apitypes.ApplyActionUpdated, kind, id), nil
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

func validateResourceHeader(apiVersion apitypes.ResourceAPIVersion, metadata apitypes.ResourceMetadata) error {
	if apiVersion != apitypes.ResourceAPIVersionGizclawAdminv1alpha1 {
		return applyError(400, "UNSUPPORTED_RESOURCE_VERSION", fmt.Sprintf("unsupported resource apiVersion %q", apiVersion))
	}
	_, err := requireResourceID(metadata)
	return err
}

func requireResourceID(metadata apitypes.ResourceMetadata) (string, error) {
	id := metadata.Id
	if err := customid.ValidateResourceID(id); err != nil {
		code := "INVALID_RESOURCE_ID"
		if id == "" {
			code = "RESOURCE_ID_REQUIRED"
		}
		return "", applyError(400, code, "metadata."+err.Error())
	}
	return id, nil
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

func applyResult(action apitypes.ApplyAction, kind apitypes.ResourceKind, ids ...string) apitypes.ApplyResult {
	result := apitypes.ApplyResult{
		Action:     action,
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       kind,
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

func servicePathID(value string) string {
	// Strict service request objects contain decoded path values. HTTP escaping
	// belongs to the generated client and server transport boundary.
	return value
}
