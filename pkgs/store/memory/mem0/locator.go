package mem0

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	factLocatorPrefix         = "mem0:fact:v1:"
	operationLocatorPrefix    = "mem0:operation:v1:"
	selfHostedScopeIdentifier = "gizclaw-scope-v1:"
)

func encodeFactLocator(scope scope, nativeID string) string {
	return encodeLocator(factLocatorPrefix, scope, nativeID)
}

func decodeFactLocator(value string) (scope, string, error) {
	return decodeLocator(factLocatorPrefix, value)
}

func encodeOperationLocator(scope scope, nativeID string) string {
	return encodeLocator(operationLocatorPrefix, scope, nativeID)
}

func decodeOperationLocator(value string) (scope, string, error) {
	return decodeLocator(operationLocatorPrefix, value)
}

func encodeSelfHostedScope(input scope) string {
	parts := []string{input.AppID, input.UserID, input.AgentID, input.RunID}
	for index := range parts {
		parts[index] = base64.RawURLEncoding.EncodeToString([]byte(parts[index]))
	}
	return selfHostedScopeIdentifier + strings.Join(parts, ":")
}

func decodeSelfHostedScope(value string) (scope, error) {
	parts := strings.Split(strings.TrimPrefix(value, selfHostedScopeIdentifier), ":")
	if !strings.HasPrefix(value, selfHostedScopeIdentifier) || len(parts) != 4 {
		return scope{}, fmt.Errorf("%w: invalid self-hosted mem0 scope", errInvalidInput)
	}
	decoded := make([]string, len(parts))
	for index, part := range parts {
		raw, err := base64.RawURLEncoding.DecodeString(part)
		if err != nil {
			return scope{}, fmt.Errorf("%w: invalid self-hosted mem0 scope", errInvalidInput)
		}
		decoded[index] = string(raw)
	}
	result, err := normalizeEntityScope(scope{
		AppID: decoded[0], UserID: decoded[1], AgentID: decoded[2], RunID: decoded[3],
	})
	if err != nil {
		return scope{}, fmt.Errorf("%w: invalid self-hosted mem0 scope", errInvalidInput)
	}
	return result, nil
}

func encodeLocator(prefix string, scope scope, nativeID string) string {
	parts := []string{scope.AppID, scope.UserID, scope.AgentID, scope.RunID, nativeID}
	for index := range parts {
		parts[index] = base64.RawURLEncoding.EncodeToString([]byte(parts[index]))
	}
	return prefix + strings.Join(parts, ":")
}

func decodeLocator(prefix, value string) (scope, string, error) {
	parts := strings.Split(strings.TrimPrefix(value, prefix), ":")
	if !strings.HasPrefix(value, prefix) || len(parts) != 5 {
		return scope{}, "", fmt.Errorf("%w: invalid mem0 locator", errInvalidInput)
	}
	decoded := make([]string, len(parts))
	for index, part := range parts {
		raw, err := base64.RawURLEncoding.DecodeString(part)
		if err != nil {
			return scope{}, "", fmt.Errorf("%w: invalid mem0 locator", errInvalidInput)
		}
		decoded[index] = string(raw)
	}
	if decoded[4] == "" {
		return scope{}, "", fmt.Errorf("%w: invalid mem0 locator id", errInvalidInput)
	}
	result, err := normalizeEntityScope(scope{AppID: decoded[0], UserID: decoded[1], AgentID: decoded[2], RunID: decoded[3]})
	if err != nil {
		return scope{}, "", fmt.Errorf("%w: invalid mem0 locator scope", errInvalidInput)
	}
	return result, decoded[4], nil
}
