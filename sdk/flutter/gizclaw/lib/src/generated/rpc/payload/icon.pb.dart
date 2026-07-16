// This is a generated file - do not edit.
//
// Generated from payload/icon.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

class Icon extends $pb.GeneratedMessage {
  factory Icon({
    $core.String? pixa,
    $core.String? png,
  }) {
    final result = create();
    if (pixa != null) result.pixa = pixa;
    if (png != null) result.png = png;
    return result;
  }

  Icon._();

  factory Icon.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Icon.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Icon',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'pixa')
    ..aOS(2, _omitFieldNames ? '' : 'png')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Icon clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Icon copyWith(void Function(Icon) updates) =>
      super.copyWith((message) => updates(message as Icon)) as Icon;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Icon create() => Icon._();
  @$core.override
  Icon createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Icon getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Icon>(create);
  static Icon? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get pixa => $_getSZ(0);
  @$pb.TagNumber(1)
  set pixa($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPixa() => $_has(0);
  @$pb.TagNumber(1)
  void clearPixa() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get png => $_getSZ(1);
  @$pb.TagNumber(2)
  set png($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPng() => $_has(1);
  @$pb.TagNumber(2)
  void clearPng() => $_clearField(2);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
