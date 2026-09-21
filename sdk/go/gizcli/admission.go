package gizcli

import (
	"errors"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giznetpb"
)

// RegistrationTokenCredentialType identifies the GizClaw built-in token policy.
const RegistrationTokenCredentialType = "gizclaw.com/registration_token"

// RegistrationTokenCredential constructs a credential for the GizClaw built-in
// policy, rejecting values exceeding 512 UTF-8 bytes before transport I/O.
func RegistrationTokenCredential(token string) (*giznetpb.AdmissionCredential, error) {
	if len(token) > giznet.MaxAdmissionCredentialValueBytes || !utf8.ValidString(token) {
		return nil, errors.New("invalid registration token: expected at most 512 UTF-8 bytes")
	}
	return &giznetpb.AdmissionCredential{Version: 1, Type: RegistrationTokenCredentialType, Value: token}, nil
}
