package chatroom

import "testing"

func TestAccessErrorsHaveStableCodesAndRetryability(t *testing.T) {
	tests := []struct {
		name      string
		err       *AccessError
		code      string
		retryable bool
	}{
		{name: "friend removed", err: FriendRemovedError(), code: AccessCodeFriendRemoved},
		{name: "member removed", err: MemberRemovedError(), code: AccessCodeMemberRemoved},
		{name: "group deleted", err: GroupDeletedError(), code: AccessCodeGroupDeleted},
		{name: "check failed", err: AccessCheckFailedError(), code: AccessCodeCheckFailed, retryable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.err.Code != test.code || test.err.Retryable != test.retryable || test.err.Error() == "" {
				t.Fatalf("AccessError = %+v, want code=%q retryable=%t with fallback message", test.err, test.code, test.retryable)
			}
		})
	}
}
