package flowcraft

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/GizClaw/flowcraft/memory/recall"
)

const locatorPrefix = "flowcraft:v2:"

func encodeLocator(scope recall.Scope, nativeID string) string {
	parts := []string{scope.RuntimeID, scope.UserID, scope.AgentID, nativeID}
	for index := range parts {
		parts[index] = base64.RawURLEncoding.EncodeToString([]byte(parts[index]))
	}
	return locatorPrefix + strings.Join(parts, ":")
}

func decodeLocator(value string) (recall.Scope, string, error) {
	parts := strings.Split(strings.TrimPrefix(value, locatorPrefix), ":")
	if !strings.HasPrefix(value, locatorPrefix) || len(parts) != 4 {
		return recall.Scope{}, "", fmt.Errorf("%w: invalid flowcraft locator", errInvalidInput)
	}
	decoded := make([]string, len(parts))
	for index, part := range parts {
		value, err := base64.RawURLEncoding.DecodeString(part)
		if err != nil {
			return recall.Scope{}, "", fmt.Errorf("%w: invalid flowcraft locator", errInvalidInput)
		}
		decoded[index] = string(value)
	}
	if decoded[0] == "" {
		return recall.Scope{}, "", fmt.Errorf("%w: invalid flowcraft locator scope", errInvalidInput)
	}
	if decoded[3] == "" {
		return recall.Scope{}, "", fmt.Errorf("%w: invalid flowcraft locator id", errInvalidInput)
	}
	return recall.Scope{RuntimeID: decoded[0], UserID: decoded[1], AgentID: decoded[2]}, decoded[3], nil
}

func sameScope(left, right recall.Scope) bool {
	return left.RuntimeID == right.RuntimeID && left.UserID == right.UserID && left.AgentID == right.AgentID
}
