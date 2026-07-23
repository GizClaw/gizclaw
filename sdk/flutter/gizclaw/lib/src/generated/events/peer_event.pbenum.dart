// This is a generated file - do not edit.
//
// Generated from peer_event.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

class PeerEventType extends $pb.ProtobufEnum {
  static const PeerEventType PEER_EVENT_TYPE_UNSPECIFIED =
      PeerEventType._(0, _omitEnumNames ? '' : 'PEER_EVENT_TYPE_UNSPECIFIED');
  static const PeerEventType PEER_EVENT_TYPE_BOS =
      PeerEventType._(1, _omitEnumNames ? '' : 'PEER_EVENT_TYPE_BOS');
  static const PeerEventType PEER_EVENT_TYPE_EOS =
      PeerEventType._(2, _omitEnumNames ? '' : 'PEER_EVENT_TYPE_EOS');
  static const PeerEventType PEER_EVENT_TYPE_TEXT_DELTA =
      PeerEventType._(3, _omitEnumNames ? '' : 'PEER_EVENT_TYPE_TEXT_DELTA');
  static const PeerEventType PEER_EVENT_TYPE_TEXT_DONE =
      PeerEventType._(4, _omitEnumNames ? '' : 'PEER_EVENT_TYPE_TEXT_DONE');
  static const PeerEventType PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED =
      PeerEventType._(
          5, _omitEnumNames ? '' : 'PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED');
  static const PeerEventType PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED =
      PeerEventType._(6,
          _omitEnumNames ? '' : 'PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED');
  static const PeerEventType PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED =
      PeerEventType._(
          7, _omitEnumNames ? '' : 'PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED');

  static const $core.List<PeerEventType> values = <PeerEventType>[
    PEER_EVENT_TYPE_UNSPECIFIED,
    PEER_EVENT_TYPE_BOS,
    PEER_EVENT_TYPE_EOS,
    PEER_EVENT_TYPE_TEXT_DELTA,
    PEER_EVENT_TYPE_TEXT_DONE,
    PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED,
    PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED,
    PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED,
  ];

  static final $core.List<PeerEventType?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 7);
  static PeerEventType? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const PeerEventType._(super.value, super.name);
}

class StreamKind extends $pb.ProtobufEnum {
  static const StreamKind STREAM_KIND_UNSPECIFIED =
      StreamKind._(0, _omitEnumNames ? '' : 'STREAM_KIND_UNSPECIFIED');
  static const StreamKind STREAM_KIND_TEXT =
      StreamKind._(1, _omitEnumNames ? '' : 'STREAM_KIND_TEXT');
  static const StreamKind STREAM_KIND_AUDIO =
      StreamKind._(2, _omitEnumNames ? '' : 'STREAM_KIND_AUDIO');
  static const StreamKind STREAM_KIND_VIDEO =
      StreamKind._(3, _omitEnumNames ? '' : 'STREAM_KIND_VIDEO');
  static const StreamKind STREAM_KIND_MIXED =
      StreamKind._(4, _omitEnumNames ? '' : 'STREAM_KIND_MIXED');

  static const $core.List<StreamKind> values = <StreamKind>[
    STREAM_KIND_UNSPECIFIED,
    STREAM_KIND_TEXT,
    STREAM_KIND_AUDIO,
    STREAM_KIND_VIDEO,
    STREAM_KIND_MIXED,
  ];

  static final $core.List<StreamKind?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 4);
  static StreamKind? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const StreamKind._(super.value, super.name);
}

class WorkspaceKind extends $pb.ProtobufEnum {
  static const WorkspaceKind WORKSPACE_KIND_UNSPECIFIED =
      WorkspaceKind._(0, _omitEnumNames ? '' : 'WORKSPACE_KIND_UNSPECIFIED');
  static const WorkspaceKind WORKSPACE_KIND_WORKFLOW =
      WorkspaceKind._(1, _omitEnumNames ? '' : 'WORKSPACE_KIND_WORKFLOW');
  static const WorkspaceKind WORKSPACE_KIND_DIRECT_CHATROOM = WorkspaceKind._(
      2, _omitEnumNames ? '' : 'WORKSPACE_KIND_DIRECT_CHATROOM');
  static const WorkspaceKind WORKSPACE_KIND_GROUP_CHATROOM =
      WorkspaceKind._(3, _omitEnumNames ? '' : 'WORKSPACE_KIND_GROUP_CHATROOM');

  static const $core.List<WorkspaceKind> values = <WorkspaceKind>[
    WORKSPACE_KIND_UNSPECIFIED,
    WORKSPACE_KIND_WORKFLOW,
    WORKSPACE_KIND_DIRECT_CHATROOM,
    WORKSPACE_KIND_GROUP_CHATROOM,
  ];

  static final $core.List<WorkspaceKind?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 3);
  static WorkspaceKind? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const WorkspaceKind._(super.value, super.name);
}

class FriendRelationshipChange extends $pb.ProtobufEnum {
  static const FriendRelationshipChange FRIEND_RELATIONSHIP_CHANGE_UNSPECIFIED =
      FriendRelationshipChange._(
          0, _omitEnumNames ? '' : 'FRIEND_RELATIONSHIP_CHANGE_UNSPECIFIED');
  static const FriendRelationshipChange FRIEND_RELATIONSHIP_CHANGE_CREATED =
      FriendRelationshipChange._(
          1, _omitEnumNames ? '' : 'FRIEND_RELATIONSHIP_CHANGE_CREATED');
  static const FriendRelationshipChange FRIEND_RELATIONSHIP_CHANGE_DELETED =
      FriendRelationshipChange._(
          2, _omitEnumNames ? '' : 'FRIEND_RELATIONSHIP_CHANGE_DELETED');

  static const $core.List<FriendRelationshipChange> values =
      <FriendRelationshipChange>[
    FRIEND_RELATIONSHIP_CHANGE_UNSPECIFIED,
    FRIEND_RELATIONSHIP_CHANGE_CREATED,
    FRIEND_RELATIONSHIP_CHANGE_DELETED,
  ];

  static final $core.List<FriendRelationshipChange?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static FriendRelationshipChange? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const FriendRelationshipChange._(super.value, super.name);
}

class FriendGroupChange extends $pb.ProtobufEnum {
  static const FriendGroupChange FRIEND_GROUP_CHANGE_UNSPECIFIED =
      FriendGroupChange._(
          0, _omitEnumNames ? '' : 'FRIEND_GROUP_CHANGE_UNSPECIFIED');
  static const FriendGroupChange FRIEND_GROUP_CHANGE_CREATED =
      FriendGroupChange._(
          1, _omitEnumNames ? '' : 'FRIEND_GROUP_CHANGE_CREATED');
  static const FriendGroupChange FRIEND_GROUP_CHANGE_DELETED =
      FriendGroupChange._(
          2, _omitEnumNames ? '' : 'FRIEND_GROUP_CHANGE_DELETED');
  static const FriendGroupChange FRIEND_GROUP_CHANGE_MEMBER_ADDED =
      FriendGroupChange._(
          3, _omitEnumNames ? '' : 'FRIEND_GROUP_CHANGE_MEMBER_ADDED');
  static const FriendGroupChange FRIEND_GROUP_CHANGE_MEMBER_REMOVED =
      FriendGroupChange._(
          4, _omitEnumNames ? '' : 'FRIEND_GROUP_CHANGE_MEMBER_REMOVED');
  static const FriendGroupChange FRIEND_GROUP_CHANGE_MEMBER_ROLE_CHANGED =
      FriendGroupChange._(
          5, _omitEnumNames ? '' : 'FRIEND_GROUP_CHANGE_MEMBER_ROLE_CHANGED');
  static const FriendGroupChange FRIEND_GROUP_CHANGE_METADATA_UPDATED =
      FriendGroupChange._(
          6, _omitEnumNames ? '' : 'FRIEND_GROUP_CHANGE_METADATA_UPDATED');

  static const $core.List<FriendGroupChange> values = <FriendGroupChange>[
    FRIEND_GROUP_CHANGE_UNSPECIFIED,
    FRIEND_GROUP_CHANGE_CREATED,
    FRIEND_GROUP_CHANGE_DELETED,
    FRIEND_GROUP_CHANGE_MEMBER_ADDED,
    FRIEND_GROUP_CHANGE_MEMBER_REMOVED,
    FRIEND_GROUP_CHANGE_MEMBER_ROLE_CHANGED,
    FRIEND_GROUP_CHANGE_METADATA_UPDATED,
  ];

  static final $core.List<FriendGroupChange?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 6);
  static FriendGroupChange? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const FriendGroupChange._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
