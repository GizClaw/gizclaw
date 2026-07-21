// This is a generated file - do not edit.
//
// Generated from payload/gameplay.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

class PetBehavior extends $pb.ProtobufEnum {
  static const PetBehavior PET_BEHAVIOR_UNSPECIFIED =
      PetBehavior._(0, _omitEnumNames ? '' : 'PET_BEHAVIOR_UNSPECIFIED');
  static const PetBehavior PET_BEHAVIOR_FEED =
      PetBehavior._(1, _omitEnumNames ? '' : 'PET_BEHAVIOR_FEED');
  static const PetBehavior PET_BEHAVIOR_BATHE =
      PetBehavior._(2, _omitEnumNames ? '' : 'PET_BEHAVIOR_BATHE');
  static const PetBehavior PET_BEHAVIOR_PLAY =
      PetBehavior._(3, _omitEnumNames ? '' : 'PET_BEHAVIOR_PLAY');
  static const PetBehavior PET_BEHAVIOR_HEAL =
      PetBehavior._(4, _omitEnumNames ? '' : 'PET_BEHAVIOR_HEAL');

  static const $core.List<PetBehavior> values = <PetBehavior>[
    PET_BEHAVIOR_UNSPECIFIED,
    PET_BEHAVIOR_FEED,
    PET_BEHAVIOR_BATHE,
    PET_BEHAVIOR_PLAY,
    PET_BEHAVIOR_HEAL,
  ];

  static final $core.List<PetBehavior?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 4);
  static PetBehavior? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const PetBehavior._(super.value, super.name);
}

class PetLifecycle extends $pb.ProtobufEnum {
  static const PetLifecycle PET_LIFECYCLE_UNSPECIFIED =
      PetLifecycle._(0, _omitEnumNames ? '' : 'PET_LIFECYCLE_UNSPECIFIED');
  static const PetLifecycle PET_LIFECYCLE_ALIVE =
      PetLifecycle._(1, _omitEnumNames ? '' : 'PET_LIFECYCLE_ALIVE');
  static const PetLifecycle PET_LIFECYCLE_DEAD =
      PetLifecycle._(2, _omitEnumNames ? '' : 'PET_LIFECYCLE_DEAD');

  static const $core.List<PetLifecycle> values = <PetLifecycle>[
    PET_LIFECYCLE_UNSPECIFIED,
    PET_LIFECYCLE_ALIVE,
    PET_LIFECYCLE_DEAD,
  ];

  static final $core.List<PetLifecycle?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static PetLifecycle? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const PetLifecycle._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
