package flowcraft

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/GizClaw/flowcraft/memory/recall"
)

const locatorPrefix = "flowcraft:v1:"

func encodeLocator(scope recall.Scope, nativeID string) string {
	return locatorPrefix + base64.RawURLEncoding.EncodeToString([]byte(scope.UserID)) + ":" + base64.RawURLEncoding.EncodeToString([]byte(nativeID))
}

func decodeLocator(value string) (recall.Scope, string, error) {
	parts := strings.Split(strings.TrimPrefix(value, locatorPrefix), ":")
	if !strings.HasPrefix(value, locatorPrefix) || len(parts) != 2 {
		return recall.Scope{}, "", fmt.Errorf("%w: invalid flowcraft locator", errInvalidInput)
	}
	scopeValue, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(scopeValue) == 0 {
		return recall.Scope{}, "", fmt.Errorf("%w: invalid flowcraft locator scope", errInvalidInput)
	}
	nativeID, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(nativeID) == 0 {
		return recall.Scope{}, "", fmt.Errorf("%w: invalid flowcraft locator id", errInvalidInput)
	}
	return recall.Scope{RuntimeID: "gizclaw", UserID: string(scopeValue)}, string(nativeID), nil
}

func sameScope(left, right recall.Scope) bool {
	return left.RuntimeID == right.RuntimeID && left.UserID == right.UserID && left.AgentID == right.AgentID
}
