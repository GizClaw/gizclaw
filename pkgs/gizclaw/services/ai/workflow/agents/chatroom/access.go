package chatroom

// Stable access error codes returned when a social Chatroom cannot accept the
// caller's next turn. Clients localize these codes; Message is only a fallback.
const (
	AccessCodeFriendRemoved = "CHATROOM_FRIEND_REMOVED"
	AccessCodeMemberRemoved = "CHATROOM_MEMBER_REMOVED"
	AccessCodeGroupDeleted  = "CHATROOM_GROUP_DELETED"
	AccessCodeCheckFailed   = "CHATROOM_ACCESS_CHECK_FAILED"
)

// AccessError is the Chatroom-owned, transport-independent result of a
// per-turn authorization check.
type AccessError struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *AccessError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func FriendRemovedError() *AccessError {
	return &AccessError{
		Code:    AccessCodeFriendRemoved,
		Message: "the direct chat relationship no longer exists",
	}
}

func MemberRemovedError() *AccessError {
	return &AccessError{
		Code:    AccessCodeMemberRemoved,
		Message: "the peer is no longer a group member",
	}
}

func GroupDeletedError() *AccessError {
	return &AccessError{
		Code:    AccessCodeGroupDeleted,
		Message: "the group chat no longer exists",
	}
}

func AccessCheckFailedError() *AccessError {
	return &AccessError{
		Code:      AccessCodeCheckFailed,
		Message:   "chatroom access could not be verified",
		Retryable: true,
	}
}
