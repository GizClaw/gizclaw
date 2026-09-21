import 'generated/giznet/admission.pb.dart';

/// Constructs a credential for the GizClaw registration-token policy.
AdmissionCredential registrationTokenCredential(String value) =>
    AdmissionCredential(version: 1, type: 'registration_token', value: value);
