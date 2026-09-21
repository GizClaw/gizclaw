package gizcli

import "github.com/GizClaw/gizclaw-go/pkgs/giznet/giznetpb"

// RegistrationTokenCredential constructs a credential for the GizClaw built-in
// registration-token policy. The transport encodes it when dialing.
func RegistrationTokenCredential(token string) *giznetpb.AdmissionCredential {
	return &giznetpb.AdmissionCredential{
		Version: 1, Type: "registration_token", Value: token,
	}
}
