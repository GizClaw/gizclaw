// This is a generated file - do not edit.
//
// Generated from payload/social.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

/// SocialPingResult is the outcome of one friend ping or Friend Group rally.
class SocialPingResult extends $pb.ProtobufEnum {
  static const SocialPingResult SOCIAL_PING_RESULT_UNSPECIFIED =
      SocialPingResult._(
          0, _omitEnumNames ? '' : 'SOCIAL_PING_RESULT_UNSPECIFIED');

  /// At least one target device acknowledged the ping.
  static const SocialPingResult SOCIAL_PING_RESULT_DELIVERED =
      SocialPingResult._(
          1, _omitEnumNames ? '' : 'SOCIAL_PING_RESULT_DELIVERED');

  /// No target device is online; nothing was sent and no rate-limit window
  /// was started.
  static const SocialPingResult SOCIAL_PING_RESULT_NOT_ONLINE =
      SocialPingResult._(
          2, _omitEnumNames ? '' : 'SOCIAL_PING_RESULT_NOT_ONLINE');

  /// The friend pair or the Friend Group pinged within the last minute.
  static const SocialPingResult SOCIAL_PING_RESULT_RATE_LIMITED =
      SocialPingResult._(
          3, _omitEnumNames ? '' : 'SOCIAL_PING_RESULT_RATE_LIMITED');

  static const $core.List<SocialPingResult> values = <SocialPingResult>[
    SOCIAL_PING_RESULT_UNSPECIFIED,
    SOCIAL_PING_RESULT_DELIVERED,
    SOCIAL_PING_RESULT_NOT_ONLINE,
    SOCIAL_PING_RESULT_RATE_LIMITED,
  ];

  static final $core.List<SocialPingResult?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 3);
  static SocialPingResult? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const SocialPingResult._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
