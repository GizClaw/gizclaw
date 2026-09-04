package gizclaw

import (
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
)

// Stable input access error codes carried by the EOS that ends a denied input
// stream. Clients localize these codes; Message is only a fallback.
const (
	// sfuAccessRevokedCode reports that the Peer is no longer a member of the
	// Social resource bound to the active SFU Workspace.
	sfuAccessRevokedCode = "SFU_ACCESS_REVOKED"
	// sfuAccessCheckFailedCode reports that membership could not be verified;
	// the Server fails closed and the Peer may retry.
	sfuAccessCheckFailedCode = "SFU_ACCESS_CHECK_FAILED"
	// sfuRuntimeNotAttachedCode reports that the SFU runtime for the selected
	// Workspace is not active yet, so media has no Room to enter.
	sfuRuntimeNotAttachedCode = "SFU_RUNTIME_NOT_ATTACHED"
)

// inputAccessError is the transport-independent result of an inbound turn
// authorization check.
type inputAccessError struct {
	Code      string
	Message   string
	Retryable bool
}

func sfuAccessRevokedError() *inputAccessError {
	return &inputAccessError{
		Code:    sfuAccessRevokedCode,
		Message: "SFU workspace access was revoked",
	}
}

func sfuAccessCheckFailedError() *inputAccessError {
	return &inputAccessError{
		Code:      sfuAccessCheckFailedCode,
		Message:   "SFU workspace access could not be verified",
		Retryable: true,
	}
}

func sfuRuntimeNotAttachedError() *inputAccessError {
	return &inputAccessError{
		Code:      sfuRuntimeNotAttachedCode,
		Message:   "SFU workspace runtime is not attached",
		Retryable: true,
	}
}

func inputAccessEventError(err *inputAccessError) *eventpb.EventError {
	if err == nil {
		return nil
	}
	return &eventpb.EventError{
		Code:      err.Code,
		Message:   err.Message,
		Retryable: err.Retryable,
	}
}

// SFUDroppedPackets reports how many inbound Opus packets were dropped because
// their SFU audio stream had already been denied. Dropped packets are never
// cached or forwarded later.
func (h *PeerConn) SFUDroppedPackets() uint64 {
	if h == nil {
		return 0
	}
	return h.sfuDroppedPackets.Load()
}
