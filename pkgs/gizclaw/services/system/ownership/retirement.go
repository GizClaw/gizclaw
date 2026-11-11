package ownership

import "errors"

// ErrOwnerRetired reports that a Peer owner's retirement fence rejects new resources.
var ErrOwnerRetired = errors.New("peer: deletion pending")
