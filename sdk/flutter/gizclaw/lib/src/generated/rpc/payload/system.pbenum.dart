// This is a generated file - do not edit.
//
// Generated from payload/system.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

class AssetOwnerKind extends $pb.ProtobufEnum {
  static const AssetOwnerKind ASSET_OWNER_KIND_UNSPECIFIED =
      AssetOwnerKind._(0, _omitEnumNames ? '' : 'ASSET_OWNER_KIND_UNSPECIFIED');
  static const AssetOwnerKind ASSET_OWNER_KIND_RESOURCE =
      AssetOwnerKind._(1, _omitEnumNames ? '' : 'ASSET_OWNER_KIND_RESOURCE');
  static const AssetOwnerKind ASSET_OWNER_KIND_FRIEND_GROUP_MESSAGE =
      AssetOwnerKind._(
          2, _omitEnumNames ? '' : 'ASSET_OWNER_KIND_FRIEND_GROUP_MESSAGE');

  static const $core.List<AssetOwnerKind> values = <AssetOwnerKind>[
    ASSET_OWNER_KIND_UNSPECIFIED,
    ASSET_OWNER_KIND_RESOURCE,
    ASSET_OWNER_KIND_FRIEND_GROUP_MESSAGE,
  ];

  static final $core.List<AssetOwnerKind?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static AssetOwnerKind? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const AssetOwnerKind._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
