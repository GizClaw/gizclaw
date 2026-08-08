package friend

import "errors"

var (
	// ErrInviteTokenRequired reports a missing or whitespace-only Friend invite token.
	ErrInviteTokenRequired = errors.New("social: friend invite token is required")
	// ErrInviteTokenUnavailable reports a Friend invite token that cannot be used.
	ErrInviteTokenUnavailable = errors.New("social: friend invite token is unavailable")
	// ErrInviteTokenSelfOwned reports an attempt to use the caller's own Friend invite token.
	ErrInviteTokenSelfOwned = errors.New("social: friend invite token belongs to the caller")
	// ErrInviteTokenLookupFailed reports an internal failure while resolving a Friend invite token.
	ErrInviteTokenLookupFailed = errors.New("social: friend invite token lookup failed")
)
