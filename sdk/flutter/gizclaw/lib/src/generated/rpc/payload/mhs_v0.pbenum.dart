// This is a generated file - do not edit.
//
// Generated from payload/mhs_v0.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

class ClientHwd extends $pb.ProtobufEnum {
  static const ClientHwd CLIENT_HWD_UNSPECIFIED =
      ClientHwd._(0, _omitEnumNames ? '' : 'CLIENT_HWD_UNSPECIFIED');
  static const ClientHwd CLIENT_HWD_WIFI =
      ClientHwd._(1, _omitEnumNames ? '' : 'CLIENT_HWD_WIFI');
  static const ClientHwd CLIENT_HWD_BLE =
      ClientHwd._(2, _omitEnumNames ? '' : 'CLIENT_HWD_BLE');
  static const ClientHwd CLIENT_HWD_MODEM =
      ClientHwd._(3, _omitEnumNames ? '' : 'CLIENT_HWD_MODEM');
  static const ClientHwd CLIENT_HWD_BATTERY =
      ClientHwd._(4, _omitEnumNames ? '' : 'CLIENT_HWD_BATTERY');
  static const ClientHwd CLIENT_HWD_MIC =
      ClientHwd._(5, _omitEnumNames ? '' : 'CLIENT_HWD_MIC');
  static const ClientHwd CLIENT_HWD_DISPLAY =
      ClientHwd._(6, _omitEnumNames ? '' : 'CLIENT_HWD_DISPLAY');
  static const ClientHwd CLIENT_HWD_LED =
      ClientHwd._(7, _omitEnumNames ? '' : 'CLIENT_HWD_LED');
  static const ClientHwd CLIENT_HWD_SPEAKER =
      ClientHwd._(8, _omitEnumNames ? '' : 'CLIENT_HWD_SPEAKER');

  static const $core.List<ClientHwd> values = <ClientHwd>[
    CLIENT_HWD_UNSPECIFIED,
    CLIENT_HWD_WIFI,
    CLIENT_HWD_BLE,
    CLIENT_HWD_MODEM,
    CLIENT_HWD_BATTERY,
    CLIENT_HWD_MIC,
    CLIENT_HWD_DISPLAY,
    CLIENT_HWD_LED,
    CLIENT_HWD_SPEAKER,
  ];

  static final $core.List<ClientHwd?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 8);
  static ClientHwd? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ClientHwd._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
