import 'dart:convert';
import 'package:protobuf/protobuf.dart';
import 'generated/rpc/payload.pb.dart' as p;

bool _text(String value, int max) =>
    value.isNotEmpty &&
    utf8.encode(value).length <= max &&
    !value.contains('\u0000');
bool _feature(p.DeviceFeature v) =>
    _text(v.id, 64) &&
    _text(v.displayName, 128) &&
    v.hasEnabled() &&
    v.hasWritable();
bool _age(p.ContentAgeState v) {
  if (!_text(v.optionId, 64) || v.options.isEmpty || v.options.length > 16) {
    return false;
  }
  final seen = <String>{};
  for (final item in v.options) {
    if (!_text(item.id, 64) ||
        !_text(item.displayName, 128) ||
        !seen.add(item.id) ||
        (item.hasMinAgeYears() &&
            item.hasMaxAgeYears() &&
            item.minAgeYears > item.maxAgeYears)) {
      return false;
    }
  }
  return seen.contains(v.optionId);
}

bool _safety(p.AISafetyState v) {
  if (!_text(v.level, 64) || v.options.isEmpty || v.options.length > 16) {
    return false;
  }
  final seen = <String>{};
  for (final item in v.options) {
    if (!_text(item.id, 64) ||
        !_text(item.displayName, 128) ||
        !seen.add(item.id)) {
      return false;
    }
  }
  return seen.contains(v.level);
}

bool _usage(p.UsageTimeState v) =>
    v.hasLimitSeconds() &&
    v.hasUsedSeconds() &&
    v.hasExhausted() &&
    v.exhausted == (v.limitSeconds > 0 && v.usedSeconds >= v.limitSeconds);

bool validManagementResult(GeneratedMessage request, GeneratedMessage result) =>
    switch (result) {
      p.ClientContentAgeGetResponse r => r.hasState() && _age(r.state),
      p.ClientContentAgeSetResponse r => r.hasApplied() && _age(r.applied),
      p.ClientDeviceFeaturesGetResponse r =>
        r.features.length <= 32 &&
            r.features.every(_feature) &&
            r.features.map((f) => f.id).toSet().length == r.features.length,
      p.ClientDeviceFeatureSetResponse r =>
        request is p.ClientDeviceFeatureSetRequest &&
            r.hasApplied() &&
            _feature(r.applied) &&
            r.applied.writable &&
            r.applied.id == request.featureId,
      p.ClientUsageTimeGetResponse r => r.hasState() && _usage(r.state),
      p.ClientUsageTimeSetResponse r => r.hasApplied() && _usage(r.applied),
      p.ClientAISafetyGetResponse r => r.hasState() && _safety(r.state),
      p.ClientAISafetySetResponse r => r.hasApplied() && _safety(r.applied),
      p.ClientManagementPasswordGetResponse r => r.hasIsSet(),
      p.ClientManagementPasswordSetResponse r => r.hasIsSet() && r.isSet,
      p.ClientManagementPasswordResetResponse r => r.hasIsSet() && !r.isSet,
      _ => true,
    };
bool validNfcSnapshot(p.NfcReadSnapshot v) =>
    v.readAtUnixMs > 0 &&
    v.uid.length <= 32 &&
    v.rawData.length <= 1024 &&
    (!v.hasTechnology() || _text(v.technology, 32)) &&
    (!v.hasDataFormat() || _text(v.dataFormat, 32)) &&
    (!v.hasErrorCode() || _text(v.errorCode, 64)) &&
    (v.result == 'success'
        ? !v.hasErrorCode()
        : v.result == 'failed' && v.hasErrorCode());
