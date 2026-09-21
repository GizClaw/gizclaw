import 'generated/giznet/admission.pb.dart';
import 'signaling.dart';

/// Namespaced type owned by the GizClaw registration-token policy.
const registrationTokenCredentialType = 'gizclaw.com/registration_token';

/// Constructs a credential, rejecting values exceeding 512 UTF-8 bytes.
AdmissionCredential registrationTokenCredential(String value) {
  final credential = AdmissionCredential(
    version: 1,
    type: registrationTokenCredentialType,
    value: value,
  );
  encodeAdmissionCredential(credential);
  return credential;
}
