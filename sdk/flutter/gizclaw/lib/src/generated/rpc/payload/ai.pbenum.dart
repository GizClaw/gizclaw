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

class WorkflowLocale extends $pb.ProtobufEnum {
  static const WorkflowLocale WORKFLOW_LOCALE_UNSPECIFIED =
      WorkflowLocale._(0, _omitEnumNames ? '' : 'WORKFLOW_LOCALE_UNSPECIFIED');
  static const WorkflowLocale WORKFLOW_LOCALE_EN =
      WorkflowLocale._(1, _omitEnumNames ? '' : 'WORKFLOW_LOCALE_EN');
  static const WorkflowLocale WORKFLOW_LOCALE_ZH_CN =
      WorkflowLocale._(2, _omitEnumNames ? '' : 'WORKFLOW_LOCALE_ZH_CN');

  static const $core.List<WorkflowLocale> values = <WorkflowLocale>[
    WORKFLOW_LOCALE_UNSPECIFIED,
    WORKFLOW_LOCALE_EN,
    WORKFLOW_LOCALE_ZH_CN,
  ];

  static final $core.List<WorkflowLocale?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static WorkflowLocale? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const WorkflowLocale._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
