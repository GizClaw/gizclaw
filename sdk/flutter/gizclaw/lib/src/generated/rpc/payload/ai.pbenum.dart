// This is a generated file - do not edit.
//
// Generated from payload/ai.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

class ModelProviderKind extends $pb.ProtobufEnum {
  static const ModelProviderKind MODEL_PROVIDER_KIND_UNSPECIFIED =
      ModelProviderKind._(
          0, _omitEnumNames ? '' : 'MODEL_PROVIDER_KIND_UNSPECIFIED');
  static const ModelProviderKind MODEL_PROVIDER_KIND_OPENAI_TENANT =
      ModelProviderKind._(
          1, _omitEnumNames ? '' : 'MODEL_PROVIDER_KIND_OPENAI_TENANT');
  static const ModelProviderKind MODEL_PROVIDER_KIND_GEMINI_TENANT =
      ModelProviderKind._(
          2, _omitEnumNames ? '' : 'MODEL_PROVIDER_KIND_GEMINI_TENANT');
  static const ModelProviderKind MODEL_PROVIDER_KIND_DASHSCOPE_TENANT =
      ModelProviderKind._(
          3, _omitEnumNames ? '' : 'MODEL_PROVIDER_KIND_DASHSCOPE_TENANT');
  static const ModelProviderKind MODEL_PROVIDER_KIND_VOLC_TENANT =
      ModelProviderKind._(
          4, _omitEnumNames ? '' : 'MODEL_PROVIDER_KIND_VOLC_TENANT');
  static const ModelProviderKind MODEL_PROVIDER_KIND_MINIMAX_TENANT =
      ModelProviderKind._(
          5, _omitEnumNames ? '' : 'MODEL_PROVIDER_KIND_MINIMAX_TENANT');
  static const ModelProviderKind MODEL_PROVIDER_KIND_DEEPSEEK_TENANT =
      ModelProviderKind._(
          6, _omitEnumNames ? '' : 'MODEL_PROVIDER_KIND_DEEPSEEK_TENANT');

  static const $core.List<ModelProviderKind> values = <ModelProviderKind>[
    MODEL_PROVIDER_KIND_UNSPECIFIED,
    MODEL_PROVIDER_KIND_OPENAI_TENANT,
    MODEL_PROVIDER_KIND_GEMINI_TENANT,
    MODEL_PROVIDER_KIND_DASHSCOPE_TENANT,
    MODEL_PROVIDER_KIND_VOLC_TENANT,
    MODEL_PROVIDER_KIND_MINIMAX_TENANT,
    MODEL_PROVIDER_KIND_DEEPSEEK_TENANT,
  ];

  static final $core.List<ModelProviderKind?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 6);
  static ModelProviderKind? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ModelProviderKind._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
