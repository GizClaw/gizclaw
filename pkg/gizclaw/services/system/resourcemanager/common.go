package resourcemanager

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func marshalResource(in interface{}) (apitypes.Resource, error) {
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

func validateResourceHeader(apiVersion apitypes.ResourceAPIVersion, name string) error {
	if apiVersion != apitypes.ResourceAPIVersionGizclawAdminv1alpha1 {
		return applyError(400, "UNSUPPORTED_RESOURCE_VERSION", fmt.Sprintf("unsupported resource apiVersion %q", apiVersion))
	}
	if name == "" {
		return applyError(400, "INVALID_RESOURCE", "metadata.name is required")
	}
	return nil
}

func semanticEqual(left, right interface{}) (bool, error) {
	var leftValue interface{}
	if err := normalizeJSON(left, &leftValue); err != nil {
		return false, err
	}
	var rightValue interface{}
	if err := normalizeJSON(right, &rightValue); err != nil {
		return false, err
	}
	return reflect.DeepEqual(leftValue, rightValue), nil
}

func normalizeJSON(in interface{}, out *interface{}) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func applyResult(action apitypes.ApplyAction, kind apitypes.ResourceKind, name string) apitypes.ApplyResult {
	return apitypes.ApplyResult{
		Action:     action,
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       kind,
		Name:       name,
	}
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

func responseError(statusCode int, fallbackCode, fallbackMessage string, response interface{}) *Error {
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

func unexpectedResponse(operation string, response interface{}) *Error {
	return applyError(500, "UNEXPECTED_SERVICE_RESPONSE", fmt.Sprintf("%s returned unexpected response %T", operation, response))
}

func pathParam(value string) string {
	return url.PathEscape(value)
}
