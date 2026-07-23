package eventpb

import (
	"errors"
	"fmt"
	"strings"
)

const Version = 1

var (
	ErrInvalidVersion    = errors.New("peer event: invalid version")
	ErrUnknownType       = errors.New("peer event: unknown type")
	ErrPayloadMismatch   = errors.New("peer event: type and payload mismatch")
	ErrMissingIdentifier = errors.New("peer event: missing identifier")
)

// Validate checks the application-level Peer Event contract that Protobuf
// itself cannot express: the explicit type must select the matching oneof arm.
func (e *PeerEvent) Validate() error {
	if e == nil || e.Version != Version {
		return ErrInvalidVersion
	}
	if !payloadMatchesType(e) {
		if e.Type == PeerEventType_PEER_EVENT_TYPE_UNSPECIFIED ||
			e.Type > PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED {
			return ErrUnknownType
		}
		return ErrPayloadMismatch
	}
	switch payload := e.Payload.(type) {
	case *PeerEvent_Bos:
		if strings.TrimSpace(payload.Bos.GetStreamId()) == "" {
			return fmt.Errorf("%w: stream_id", ErrMissingIdentifier)
		}
	case *PeerEvent_Eos:
		if strings.TrimSpace(payload.Eos.GetStreamId()) == "" {
			return fmt.Errorf("%w: stream_id", ErrMissingIdentifier)
		}
	case *PeerEvent_TextDelta:
		if strings.TrimSpace(payload.TextDelta.GetStreamId()) == "" {
			return fmt.Errorf("%w: stream_id", ErrMissingIdentifier)
		}
	case *PeerEvent_TextDone:
		if strings.TrimSpace(payload.TextDone.GetStreamId()) == "" {
			return fmt.Errorf("%w: stream_id", ErrMissingIdentifier)
		}
	case *PeerEvent_WorkspaceHistoryUpdated:
		if strings.TrimSpace(payload.WorkspaceHistoryUpdated.GetWorkspaceName()) == "" {
			return fmt.Errorf("%w: workspace_name", ErrMissingIdentifier)
		}
	case *PeerEvent_FriendRelationshipUpdated:
		if strings.TrimSpace(payload.FriendRelationshipUpdated.GetPeerPublicKey()) == "" ||
			strings.TrimSpace(payload.FriendRelationshipUpdated.GetWorkspaceName()) == "" {
			return fmt.Errorf("%w: friend relationship", ErrMissingIdentifier)
		}
	case *PeerEvent_FriendGroupUpdated:
		if strings.TrimSpace(payload.FriendGroupUpdated.GetFriendGroupId()) == "" ||
			strings.TrimSpace(payload.FriendGroupUpdated.GetWorkspaceName()) == "" {
			return fmt.Errorf("%w: friend group", ErrMissingIdentifier)
		}
	}
	return nil
}

// ValidateReceived validates the current envelope while allowing a future
// event type to pass through an older consumer. Known types still require the
// exact matching oneof arm.
func (e *PeerEvent) ValidateReceived() error {
	if e == nil || e.Version != Version {
		return ErrInvalidVersion
	}
	if e.Type <= PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED {
		return e.Validate()
	}
	if e.Payload != nil {
		return ErrPayloadMismatch
	}
	return nil
}

func payloadMatchesType(e *PeerEvent) bool {
	switch e.Type {
	case PeerEventType_PEER_EVENT_TYPE_BOS:
		_, ok := e.Payload.(*PeerEvent_Bos)
		return ok
	case PeerEventType_PEER_EVENT_TYPE_EOS:
		_, ok := e.Payload.(*PeerEvent_Eos)
		return ok
	case PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA:
		_, ok := e.Payload.(*PeerEvent_TextDelta)
		return ok
	case PeerEventType_PEER_EVENT_TYPE_TEXT_DONE:
		_, ok := e.Payload.(*PeerEvent_TextDone)
		return ok
	case PeerEventType_PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED:
		_, ok := e.Payload.(*PeerEvent_WorkspaceHistoryUpdated)
		return ok
	case PeerEventType_PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED:
		_, ok := e.Payload.(*PeerEvent_FriendRelationshipUpdated)
		return ok
	case PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED:
		_, ok := e.Payload.(*PeerEvent_FriendGroupUpdated)
		return ok
	default:
		return false
	}
}

// StreamID returns the logical stream identifier carried by stream events.
func (e *PeerEvent) StreamID() string {
	switch payload := e.GetPayload().(type) {
	case *PeerEvent_Bos:
		return payload.Bos.GetStreamId()
	case *PeerEvent_Eos:
		return payload.Eos.GetStreamId()
	case *PeerEvent_TextDelta:
		return payload.TextDelta.GetStreamId()
	case *PeerEvent_TextDone:
		return payload.TextDone.GetStreamId()
	default:
		return ""
	}
}

// Label returns the stream label carried by lifecycle and text events.
func (e *PeerEvent) Label() string {
	switch payload := e.GetPayload().(type) {
	case *PeerEvent_Bos:
		return payload.Bos.GetLabel()
	case *PeerEvent_Eos:
		return payload.Eos.GetLabel()
	case *PeerEvent_TextDelta:
		return payload.TextDelta.GetLabel()
	case *PeerEvent_TextDone:
		return payload.TextDone.GetLabel()
	default:
		return ""
	}
}

// Text returns the text carried by text events.
func (e *PeerEvent) Text() string {
	switch payload := e.GetPayload().(type) {
	case *PeerEvent_TextDelta:
		return payload.TextDelta.GetText()
	case *PeerEvent_TextDone:
		return payload.TextDone.GetText()
	default:
		return ""
	}
}

// StreamError returns the typed logical-stream error on EOS.
func (e *PeerEvent) StreamError() *EventError {
	if payload := e.GetEos(); payload != nil {
		return payload.GetError()
	}
	return nil
}

// StreamKindValue returns the stream kind for lifecycle events.
func (e *PeerEvent) StreamKindValue() StreamKind {
	switch payload := e.GetPayload().(type) {
	case *PeerEvent_Bos:
		return payload.Bos.GetKind()
	case *PeerEvent_Eos:
		return payload.Eos.GetKind()
	default:
		return StreamKind_STREAM_KIND_UNSPECIFIED
	}
}

// TimestampUnixMilli returns the event timestamp when the payload carries one.
func (e *PeerEvent) TimestampUnixMilli() int64 {
	switch payload := e.GetPayload().(type) {
	case *PeerEvent_Bos:
		return payload.Bos.GetTimestampUnixMs()
	case *PeerEvent_Eos:
		return payload.Eos.GetTimestampUnixMs()
	case *PeerEvent_TextDelta:
		return payload.TextDelta.GetTimestampUnixMs()
	case *PeerEvent_TextDone:
		return payload.TextDone.GetTimestampUnixMs()
	case *PeerEvent_WorkspaceHistoryUpdated:
		return payload.WorkspaceHistoryUpdated.GetLastUpdatedAtUnixMs()
	case *PeerEvent_FriendRelationshipUpdated:
		return payload.FriendRelationshipUpdated.GetRevisionUnixMs()
	case *PeerEvent_FriendGroupUpdated:
		return payload.FriendGroupUpdated.GetRevisionUnixMs()
	default:
		return 0
	}
}
