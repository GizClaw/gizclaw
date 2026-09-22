// This is a generated file - do not edit.
//
// Generated from payload/tool.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

/// ClientTool identifies one device-provided procedure. The value selects both
/// payload messages, so a caller never guesses a type from a name. Numbers are
/// the wire identity and are never reused; names are the stable identifier in
/// documentation and SDK surfaces.
class ClientTool extends $pb.ProtobufEnum {
  static const ClientTool CLIENT_TOOL_UNSPECIFIED =
      ClientTool._(0, _omitEnumNames ? '' : 'CLIENT_TOOL_UNSPECIFIED');
  static const ClientTool CLIENT_TOOL_INFO_GET =
      ClientTool._(1, _omitEnumNames ? '' : 'CLIENT_TOOL_INFO_GET');
  static const ClientTool CLIENT_TOOL_IDENTIFIERS_GET =
      ClientTool._(2, _omitEnumNames ? '' : 'CLIENT_TOOL_IDENTIFIERS_GET');
  static const ClientTool CLIENT_TOOL_DEVICE_STATUS_GET =
      ClientTool._(3, _omitEnumNames ? '' : 'CLIENT_TOOL_DEVICE_STATUS_GET');
  static const ClientTool CLIENT_TOOL_DEVICE_REBOOT =
      ClientTool._(4, _omitEnumNames ? '' : 'CLIENT_TOOL_DEVICE_REBOOT');
  static const ClientTool CLIENT_TOOL_DEVICE_FACTORY_RESET =
      ClientTool._(5, _omitEnumNames ? '' : 'CLIENT_TOOL_DEVICE_FACTORY_RESET');
  static const ClientTool CLIENT_TOOL_DEVICE_FIND =
      ClientTool._(6, _omitEnumNames ? '' : 'CLIENT_TOOL_DEVICE_FIND');
  static const ClientTool CLIENT_TOOL_SOUND_PLAY =
      ClientTool._(7, _omitEnumNames ? '' : 'CLIENT_TOOL_SOUND_PLAY');
  static const ClientTool CLIENT_TOOL_WIFI_SCAN =
      ClientTool._(8, _omitEnumNames ? '' : 'CLIENT_TOOL_WIFI_SCAN');
  static const ClientTool CLIENT_TOOL_WIFI_CONNECT =
      ClientTool._(9, _omitEnumNames ? '' : 'CLIENT_TOOL_WIFI_CONNECT');
  static const ClientTool CLIENT_TOOL_WIFI_SAVED_LIST =
      ClientTool._(10, _omitEnumNames ? '' : 'CLIENT_TOOL_WIFI_SAVED_LIST');
  static const ClientTool CLIENT_TOOL_WIFI_SAVED_FORGET =
      ClientTool._(11, _omitEnumNames ? '' : 'CLIENT_TOOL_WIFI_SAVED_FORGET');
  static const ClientTool CLIENT_TOOL_FIRMWARE_UPDATE =
      ClientTool._(12, _omitEnumNames ? '' : 'CLIENT_TOOL_FIRMWARE_UPDATE');
  static const ClientTool CLIENT_TOOL_AUDIOPLAYER_GET =
      ClientTool._(13, _omitEnumNames ? '' : 'CLIENT_TOOL_AUDIOPLAYER_GET');
  static const ClientTool CLIENT_TOOL_AUDIOPLAYER_PLAY =
      ClientTool._(14, _omitEnumNames ? '' : 'CLIENT_TOOL_AUDIOPLAYER_PLAY');
  static const ClientTool CLIENT_TOOL_AUDIOPLAYER_STOP =
      ClientTool._(15, _omitEnumNames ? '' : 'CLIENT_TOOL_AUDIOPLAYER_STOP');
  static const ClientTool CLIENT_TOOL_AUDIOPLAYER_MODE_SET = ClientTool._(
      16, _omitEnumNames ? '' : 'CLIENT_TOOL_AUDIOPLAYER_MODE_SET');
  static const ClientTool CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_GET = ClientTool._(
      17, _omitEnumNames ? '' : 'CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_GET');
  static const ClientTool CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_SET = ClientTool._(
      18, _omitEnumNames ? '' : 'CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_SET');
  static const ClientTool CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_APPEND =
      ClientTool._(
          19, _omitEnumNames ? '' : 'CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_APPEND');
  static const ClientTool CLIENT_TOOL_RUN_WORKSPACE_SET =
      ClientTool._(20, _omitEnumNames ? '' : 'CLIENT_TOOL_RUN_WORKSPACE_SET');
  static const ClientTool CLIENT_TOOL_SOCIAL_PING =
      ClientTool._(21, _omitEnumNames ? '' : 'CLIENT_TOOL_SOCIAL_PING');

  static const $core.List<ClientTool> values = <ClientTool>[
    CLIENT_TOOL_UNSPECIFIED,
    CLIENT_TOOL_INFO_GET,
    CLIENT_TOOL_IDENTIFIERS_GET,
    CLIENT_TOOL_DEVICE_STATUS_GET,
    CLIENT_TOOL_DEVICE_REBOOT,
    CLIENT_TOOL_DEVICE_FACTORY_RESET,
    CLIENT_TOOL_DEVICE_FIND,
    CLIENT_TOOL_SOUND_PLAY,
    CLIENT_TOOL_WIFI_SCAN,
    CLIENT_TOOL_WIFI_CONNECT,
    CLIENT_TOOL_WIFI_SAVED_LIST,
    CLIENT_TOOL_WIFI_SAVED_FORGET,
    CLIENT_TOOL_FIRMWARE_UPDATE,
    CLIENT_TOOL_AUDIOPLAYER_GET,
    CLIENT_TOOL_AUDIOPLAYER_PLAY,
    CLIENT_TOOL_AUDIOPLAYER_STOP,
    CLIENT_TOOL_AUDIOPLAYER_MODE_SET,
    CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_GET,
    CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_SET,
    CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_APPEND,
    CLIENT_TOOL_RUN_WORKSPACE_SET,
    CLIENT_TOOL_SOCIAL_PING,
  ];

  static final $core.List<ClientTool?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 21);
  static ClientTool? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ClientTool._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
