// This is a generated file - do not edit.
//
// Generated from admission.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

/// AdmissionCredential is carried inside an encrypted GZOF offer. The encoded
/// message must fit in 4096 bytes. Only the admission policy interprets fields;
/// Giznet does not assign business meaning to version, type or value.
class AdmissionCredential extends $pb.GeneratedMessage {
  factory AdmissionCredential({
    $core.int? version,
    $core.String? type,
    $core.String? value,
  }) {
    final result = AdmissionCredential._();
    if (version != null) result.version = version;
    if (type != null) result.type = type;
    if (value != null) result.value = value;
    return result;
  }

  AdmissionCredential._();

  factory AdmissionCredential.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      AdmissionCredential()..mergeFromBuffer(data, registry);
  factory AdmissionCredential.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      AdmissionCredential()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'AdmissionCredential',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'giznet.v1'),
      createEmptyInstance: AdmissionCredential.$_createMessage)
    ..aI(1, _omitFieldNames ? '' : 'version', fieldType: $pb.PbFieldType.OU3)
    ..aOS(2, _omitFieldNames ? '' : 'type')
    ..aOS(3, _omitFieldNames ? '' : 'value')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AdmissionCredential clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AdmissionCredential copyWith(void Function(AdmissionCredential) updates) =>
      super.copyWith((message) => updates(message as AdmissionCredential))
          as AdmissionCredential;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  @$core
      .Deprecated('Use AdmissionCredential() / AdmissionCredential.new instead')
  static AdmissionCredential create() => AdmissionCredential._();
  static $pb.GeneratedMessage $_createMessage() => AdmissionCredential._();
  @$core.override
  AdmissionCredential createEmptyInstance() => AdmissionCredential._();
  @$core.pragma('dart2js:noInline')
  static AdmissionCredential getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<AdmissionCredential>(
          AdmissionCredential.$_createMessage);
  static AdmissionCredential? _defaultInstance;

  /// Current credential version is 1; independent of the GZOF envelope version.
  @$pb.TagNumber(1)
  $core.int get version => $_getIZ(0);
  @$pb.TagNumber(1)
  set version($core.int value) => $_setUnsignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasVersion() => $_has(0);
  @$pb.TagNumber(1)
  void clearVersion() => $_clearField(1);

  /// Policy-defined credential kind, at most 128 UTF-8 bytes.
  @$pb.TagNumber(2)
  $core.String get type => $_getSZ(1);
  @$pb.TagNumber(2)
  set type($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasType() => $_has(1);
  @$pb.TagNumber(2)
  void clearType() => $_clearField(2);

  /// Opaque policy input, at most 4096 UTF-8 bytes (also subject to encoded limit).
  @$pb.TagNumber(3)
  $core.String get value => $_getSZ(2);
  @$pb.TagNumber(3)
  set value($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasValue() => $_has(2);
  @$pb.TagNumber(3)
  void clearValue() => $_clearField(3);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
