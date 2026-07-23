// This is a generated file - do not edit.
//
// Generated from peer_event.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports
// ignore_for_file: unused_import

import 'dart:convert' as $convert;
import 'dart:core' as $core;
import 'dart:typed_data' as $typed_data;

@$core.Deprecated('Use peerEventTypeDescriptor instead')
const PeerEventType$json = {
  '1': 'PeerEventType',
  '2': [
    {'1': 'PEER_EVENT_TYPE_UNSPECIFIED', '2': 0},
    {'1': 'PEER_EVENT_TYPE_BOS', '2': 1},
    {'1': 'PEER_EVENT_TYPE_EOS', '2': 2},
    {'1': 'PEER_EVENT_TYPE_TEXT_DELTA', '2': 3},
    {'1': 'PEER_EVENT_TYPE_TEXT_DONE', '2': 4},
    {'1': 'PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED', '2': 5},
    {'1': 'PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED', '2': 6},
    {'1': 'PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED', '2': 7},
  ],
};

/// Descriptor for `PeerEventType`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List peerEventTypeDescriptor = $convert.base64Decode(
    'Cg1QZWVyRXZlbnRUeXBlEh8KG1BFRVJfRVZFTlRfVFlQRV9VTlNQRUNJRklFRBAAEhcKE1BFRV'
    'JfRVZFTlRfVFlQRV9CT1MQARIXChNQRUVSX0VWRU5UX1RZUEVfRU9TEAISHgoaUEVFUl9FVkVO'
    'VF9UWVBFX1RFWFRfREVMVEEQAxIdChlQRUVSX0VWRU5UX1RZUEVfVEVYVF9ET05FEAQSLQopUE'
    'VFUl9FVkVOVF9UWVBFX1dPUktTUEFDRV9ISVNUT1JZX1VQREFURUQQBRIvCitQRUVSX0VWRU5U'
    'X1RZUEVfRlJJRU5EX1JFTEFUSU9OU0hJUF9VUERBVEVEEAYSKAokUEVFUl9FVkVOVF9UWVBFX0'
    'ZSSUVORF9HUk9VUF9VUERBVEVEEAc=');

@$core.Deprecated('Use streamKindDescriptor instead')
const StreamKind$json = {
  '1': 'StreamKind',
  '2': [
    {'1': 'STREAM_KIND_UNSPECIFIED', '2': 0},
    {'1': 'STREAM_KIND_TEXT', '2': 1},
    {'1': 'STREAM_KIND_AUDIO', '2': 2},
    {'1': 'STREAM_KIND_VIDEO', '2': 3},
    {'1': 'STREAM_KIND_MIXED', '2': 4},
  ],
};

/// Descriptor for `StreamKind`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List streamKindDescriptor = $convert.base64Decode(
    'CgpTdHJlYW1LaW5kEhsKF1NUUkVBTV9LSU5EX1VOU1BFQ0lGSUVEEAASFAoQU1RSRUFNX0tJTk'
    'RfVEVYVBABEhUKEVNUUkVBTV9LSU5EX0FVRElPEAISFQoRU1RSRUFNX0tJTkRfVklERU8QAxIV'
    'ChFTVFJFQU1fS0lORF9NSVhFRBAE');

@$core.Deprecated('Use workspaceKindDescriptor instead')
const WorkspaceKind$json = {
  '1': 'WorkspaceKind',
  '2': [
    {'1': 'WORKSPACE_KIND_UNSPECIFIED', '2': 0},
    {'1': 'WORKSPACE_KIND_WORKFLOW', '2': 1},
    {'1': 'WORKSPACE_KIND_DIRECT_CHATROOM', '2': 2},
    {'1': 'WORKSPACE_KIND_GROUP_CHATROOM', '2': 3},
  ],
};

/// Descriptor for `WorkspaceKind`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List workspaceKindDescriptor = $convert.base64Decode(
    'Cg1Xb3Jrc3BhY2VLaW5kEh4KGldPUktTUEFDRV9LSU5EX1VOU1BFQ0lGSUVEEAASGwoXV09SS1'
    'NQQUNFX0tJTkRfV09SS0ZMT1cQARIiCh5XT1JLU1BBQ0VfS0lORF9ESVJFQ1RfQ0hBVFJPT00Q'
    'AhIhCh1XT1JLU1BBQ0VfS0lORF9HUk9VUF9DSEFUUk9PTRAD');

@$core.Deprecated('Use friendRelationshipChangeDescriptor instead')
const FriendRelationshipChange$json = {
  '1': 'FriendRelationshipChange',
  '2': [
    {'1': 'FRIEND_RELATIONSHIP_CHANGE_UNSPECIFIED', '2': 0},
    {'1': 'FRIEND_RELATIONSHIP_CHANGE_CREATED', '2': 1},
    {'1': 'FRIEND_RELATIONSHIP_CHANGE_DELETED', '2': 2},
  ],
};

/// Descriptor for `FriendRelationshipChange`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List friendRelationshipChangeDescriptor = $convert.base64Decode(
    'ChhGcmllbmRSZWxhdGlvbnNoaXBDaGFuZ2USKgomRlJJRU5EX1JFTEFUSU9OU0hJUF9DSEFOR0'
    'VfVU5TUEVDSUZJRUQQABImCiJGUklFTkRfUkVMQVRJT05TSElQX0NIQU5HRV9DUkVBVEVEEAES'
    'JgoiRlJJRU5EX1JFTEFUSU9OU0hJUF9DSEFOR0VfREVMRVRFRBAC');

@$core.Deprecated('Use friendGroupChangeDescriptor instead')
const FriendGroupChange$json = {
  '1': 'FriendGroupChange',
  '2': [
    {'1': 'FRIEND_GROUP_CHANGE_UNSPECIFIED', '2': 0},
    {'1': 'FRIEND_GROUP_CHANGE_CREATED', '2': 1},
    {'1': 'FRIEND_GROUP_CHANGE_DELETED', '2': 2},
    {'1': 'FRIEND_GROUP_CHANGE_MEMBER_ADDED', '2': 3},
    {'1': 'FRIEND_GROUP_CHANGE_MEMBER_REMOVED', '2': 4},
    {'1': 'FRIEND_GROUP_CHANGE_MEMBER_ROLE_CHANGED', '2': 5},
    {'1': 'FRIEND_GROUP_CHANGE_METADATA_UPDATED', '2': 6},
  ],
};

/// Descriptor for `FriendGroupChange`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List friendGroupChangeDescriptor = $convert.base64Decode(
    'ChFGcmllbmRHcm91cENoYW5nZRIjCh9GUklFTkRfR1JPVVBfQ0hBTkdFX1VOU1BFQ0lGSUVEEA'
    'ASHwobRlJJRU5EX0dST1VQX0NIQU5HRV9DUkVBVEVEEAESHwobRlJJRU5EX0dST1VQX0NIQU5H'
    'RV9ERUxFVEVEEAISJAogRlJJRU5EX0dST1VQX0NIQU5HRV9NRU1CRVJfQURERUQQAxImCiJGUk'
    'lFTkRfR1JPVVBfQ0hBTkdFX01FTUJFUl9SRU1PVkVEEAQSKwonRlJJRU5EX0dST1VQX0NIQU5H'
    'RV9NRU1CRVJfUk9MRV9DSEFOR0VEEAUSKAokRlJJRU5EX0dST1VQX0NIQU5HRV9NRVRBREFUQV'
    '9VUERBVEVEEAY=');

@$core.Deprecated('Use peerEventDescriptor instead')
const PeerEvent$json = {
  '1': 'PeerEvent',
  '2': [
    {'1': 'version', '3': 1, '4': 1, '5': 13, '10': 'version'},
    {
      '1': 'type',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.events.v1.PeerEventType',
      '10': 'type'
    },
    {
      '1': 'bos',
      '3': 10,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.events.v1.StreamBegin',
      '9': 0,
      '10': 'bos'
    },
    {
      '1': 'eos',
      '3': 11,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.events.v1.StreamEnd',
      '9': 0,
      '10': 'eos'
    },
    {
      '1': 'text_delta',
      '3': 12,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.events.v1.TextDelta',
      '9': 0,
      '10': 'textDelta'
    },
    {
      '1': 'text_done',
      '3': 13,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.events.v1.TextDone',
      '9': 0,
      '10': 'textDone'
    },
    {
      '1': 'workspace_history_updated',
      '3': 14,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.events.v1.WorkspaceHistoryUpdated',
      '9': 0,
      '10': 'workspaceHistoryUpdated'
    },
    {
      '1': 'friend_relationship_updated',
      '3': 15,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.events.v1.FriendRelationshipUpdated',
      '9': 0,
      '10': 'friendRelationshipUpdated'
    },
    {
      '1': 'friend_group_updated',
      '3': 16,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.events.v1.FriendGroupUpdated',
      '9': 0,
      '10': 'friendGroupUpdated'
    },
  ],
  '8': [
    {'1': 'payload'},
  ],
};

/// Descriptor for `PeerEvent`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerEventDescriptor = $convert.base64Decode(
    'CglQZWVyRXZlbnQSGAoHdmVyc2lvbhgBIAEoDVIHdmVyc2lvbhI0CgR0eXBlGAIgASgOMiAuZ2'
    'l6Y2xhdy5ldmVudHMudjEuUGVlckV2ZW50VHlwZVIEdHlwZRIyCgNib3MYCiABKAsyHi5naXpj'
    'bGF3LmV2ZW50cy52MS5TdHJlYW1CZWdpbkgAUgNib3MSMAoDZW9zGAsgASgLMhwuZ2l6Y2xhdy'
    '5ldmVudHMudjEuU3RyZWFtRW5kSABSA2VvcxI9Cgp0ZXh0X2RlbHRhGAwgASgLMhwuZ2l6Y2xh'
    'dy5ldmVudHMudjEuVGV4dERlbHRhSABSCXRleHREZWx0YRI6Cgl0ZXh0X2RvbmUYDSABKAsyGy'
    '5naXpjbGF3LmV2ZW50cy52MS5UZXh0RG9uZUgAUgh0ZXh0RG9uZRJoChl3b3Jrc3BhY2VfaGlz'
    'dG9yeV91cGRhdGVkGA4gASgLMiouZ2l6Y2xhdy5ldmVudHMudjEuV29ya3NwYWNlSGlzdG9yeV'
    'VwZGF0ZWRIAFIXd29ya3NwYWNlSGlzdG9yeVVwZGF0ZWQSbgobZnJpZW5kX3JlbGF0aW9uc2hp'
    'cF91cGRhdGVkGA8gASgLMiwuZ2l6Y2xhdy5ldmVudHMudjEuRnJpZW5kUmVsYXRpb25zaGlwVX'
    'BkYXRlZEgAUhlmcmllbmRSZWxhdGlvbnNoaXBVcGRhdGVkElkKFGZyaWVuZF9ncm91cF91cGRh'
    'dGVkGBAgASgLMiUuZ2l6Y2xhdy5ldmVudHMudjEuRnJpZW5kR3JvdXBVcGRhdGVkSABSEmZyaW'
    'VuZEdyb3VwVXBkYXRlZEIJCgdwYXlsb2Fk');

@$core.Deprecated('Use streamBeginDescriptor instead')
const StreamBegin$json = {
  '1': 'StreamBegin',
  '2': [
    {'1': 'stream_id', '3': 1, '4': 1, '5': 9, '10': 'streamId'},
    {'1': 'sequence', '3': 2, '4': 1, '5': 4, '10': 'sequence'},
    {'1': 'timestamp_unix_ms', '3': 3, '4': 1, '5': 3, '10': 'timestampUnixMs'},
    {
      '1': 'kind',
      '3': 4,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.events.v1.StreamKind',
      '10': 'kind'
    },
    {'1': 'label', '3': 5, '4': 1, '5': 9, '10': 'label'},
    {'1': 'mime_type', '3': 6, '4': 1, '5': 9, '10': 'mimeType'},
  ],
};

/// Descriptor for `StreamBegin`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List streamBeginDescriptor = $convert.base64Decode(
    'CgtTdHJlYW1CZWdpbhIbCglzdHJlYW1faWQYASABKAlSCHN0cmVhbUlkEhoKCHNlcXVlbmNlGA'
    'IgASgEUghzZXF1ZW5jZRIqChF0aW1lc3RhbXBfdW5peF9tcxgDIAEoA1IPdGltZXN0YW1wVW5p'
    'eE1zEjEKBGtpbmQYBCABKA4yHS5naXpjbGF3LmV2ZW50cy52MS5TdHJlYW1LaW5kUgRraW5kEh'
    'QKBWxhYmVsGAUgASgJUgVsYWJlbBIbCgltaW1lX3R5cGUYBiABKAlSCG1pbWVUeXBl');

@$core.Deprecated('Use streamEndDescriptor instead')
const StreamEnd$json = {
  '1': 'StreamEnd',
  '2': [
    {'1': 'stream_id', '3': 1, '4': 1, '5': 9, '10': 'streamId'},
    {'1': 'sequence', '3': 2, '4': 1, '5': 4, '10': 'sequence'},
    {'1': 'timestamp_unix_ms', '3': 3, '4': 1, '5': 3, '10': 'timestampUnixMs'},
    {
      '1': 'kind',
      '3': 4,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.events.v1.StreamKind',
      '10': 'kind'
    },
    {'1': 'label', '3': 5, '4': 1, '5': 9, '10': 'label'},
    {'1': 'mime_type', '3': 6, '4': 1, '5': 9, '10': 'mimeType'},
    {
      '1': 'error',
      '3': 7,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.events.v1.EventError',
      '10': 'error'
    },
  ],
};

/// Descriptor for `StreamEnd`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List streamEndDescriptor = $convert.base64Decode(
    'CglTdHJlYW1FbmQSGwoJc3RyZWFtX2lkGAEgASgJUghzdHJlYW1JZBIaCghzZXF1ZW5jZRgCIA'
    'EoBFIIc2VxdWVuY2USKgoRdGltZXN0YW1wX3VuaXhfbXMYAyABKANSD3RpbWVzdGFtcFVuaXhN'
    'cxIxCgRraW5kGAQgASgOMh0uZ2l6Y2xhdy5ldmVudHMudjEuU3RyZWFtS2luZFIEa2luZBIUCg'
    'VsYWJlbBgFIAEoCVIFbGFiZWwSGwoJbWltZV90eXBlGAYgASgJUghtaW1lVHlwZRIzCgVlcnJv'
    'chgHIAEoCzIdLmdpemNsYXcuZXZlbnRzLnYxLkV2ZW50RXJyb3JSBWVycm9y');

@$core.Deprecated('Use eventErrorDescriptor instead')
const EventError$json = {
  '1': 'EventError',
  '2': [
    {'1': 'code', '3': 1, '4': 1, '5': 9, '10': 'code'},
    {'1': 'message', '3': 2, '4': 1, '5': 9, '10': 'message'},
    {'1': 'retryable', '3': 3, '4': 1, '5': 8, '10': 'retryable'},
  ],
};

/// Descriptor for `EventError`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List eventErrorDescriptor = $convert.base64Decode(
    'CgpFdmVudEVycm9yEhIKBGNvZGUYASABKAlSBGNvZGUSGAoHbWVzc2FnZRgCIAEoCVIHbWVzc2'
    'FnZRIcCglyZXRyeWFibGUYAyABKAhSCXJldHJ5YWJsZQ==');

@$core.Deprecated('Use textDeltaDescriptor instead')
const TextDelta$json = {
  '1': 'TextDelta',
  '2': [
    {'1': 'stream_id', '3': 1, '4': 1, '5': 9, '10': 'streamId'},
    {'1': 'sequence', '3': 2, '4': 1, '5': 4, '10': 'sequence'},
    {'1': 'timestamp_unix_ms', '3': 3, '4': 1, '5': 3, '10': 'timestampUnixMs'},
    {'1': 'label', '3': 4, '4': 1, '5': 9, '10': 'label'},
    {'1': 'text', '3': 5, '4': 1, '5': 9, '10': 'text'},
  ],
};

/// Descriptor for `TextDelta`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List textDeltaDescriptor = $convert.base64Decode(
    'CglUZXh0RGVsdGESGwoJc3RyZWFtX2lkGAEgASgJUghzdHJlYW1JZBIaCghzZXF1ZW5jZRgCIA'
    'EoBFIIc2VxdWVuY2USKgoRdGltZXN0YW1wX3VuaXhfbXMYAyABKANSD3RpbWVzdGFtcFVuaXhN'
    'cxIUCgVsYWJlbBgEIAEoCVIFbGFiZWwSEgoEdGV4dBgFIAEoCVIEdGV4dA==');

@$core.Deprecated('Use textDoneDescriptor instead')
const TextDone$json = {
  '1': 'TextDone',
  '2': [
    {'1': 'stream_id', '3': 1, '4': 1, '5': 9, '10': 'streamId'},
    {'1': 'sequence', '3': 2, '4': 1, '5': 4, '10': 'sequence'},
    {'1': 'timestamp_unix_ms', '3': 3, '4': 1, '5': 3, '10': 'timestampUnixMs'},
    {'1': 'label', '3': 4, '4': 1, '5': 9, '10': 'label'},
    {'1': 'text', '3': 5, '4': 1, '5': 9, '10': 'text'},
  ],
};

/// Descriptor for `TextDone`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List textDoneDescriptor = $convert.base64Decode(
    'CghUZXh0RG9uZRIbCglzdHJlYW1faWQYASABKAlSCHN0cmVhbUlkEhoKCHNlcXVlbmNlGAIgAS'
    'gEUghzZXF1ZW5jZRIqChF0aW1lc3RhbXBfdW5peF9tcxgDIAEoA1IPdGltZXN0YW1wVW5peE1z'
    'EhQKBWxhYmVsGAQgASgJUgVsYWJlbBISCgR0ZXh0GAUgASgJUgR0ZXh0');

@$core.Deprecated('Use workspaceHistoryUpdatedDescriptor instead')
const WorkspaceHistoryUpdated$json = {
  '1': 'WorkspaceHistoryUpdated',
  '2': [
    {'1': 'workspace_name', '3': 1, '4': 1, '5': 9, '10': 'workspaceName'},
    {
      '1': 'workspace_kind',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.events.v1.WorkspaceKind',
      '10': 'workspaceKind'
    },
    {
      '1': 'last_updated_at_unix_ms',
      '3': 3,
      '4': 1,
      '5': 3,
      '10': 'lastUpdatedAtUnixMs'
    },
  ],
};

/// Descriptor for `WorkspaceHistoryUpdated`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceHistoryUpdatedDescriptor = $convert.base64Decode(
    'ChdXb3Jrc3BhY2VIaXN0b3J5VXBkYXRlZBIlCg53b3Jrc3BhY2VfbmFtZRgBIAEoCVINd29ya3'
    'NwYWNlTmFtZRJHCg53b3Jrc3BhY2Vfa2luZBgCIAEoDjIgLmdpemNsYXcuZXZlbnRzLnYxLldv'
    'cmtzcGFjZUtpbmRSDXdvcmtzcGFjZUtpbmQSNAoXbGFzdF91cGRhdGVkX2F0X3VuaXhfbXMYAy'
    'ABKANSE2xhc3RVcGRhdGVkQXRVbml4TXM=');

@$core.Deprecated('Use friendRelationshipUpdatedDescriptor instead')
const FriendRelationshipUpdated$json = {
  '1': 'FriendRelationshipUpdated',
  '2': [
    {'1': 'peer_public_key', '3': 1, '4': 1, '5': 9, '10': 'peerPublicKey'},
    {'1': 'workspace_name', '3': 2, '4': 1, '5': 9, '10': 'workspaceName'},
    {
      '1': 'change',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.events.v1.FriendRelationshipChange',
      '10': 'change'
    },
    {'1': 'revision_unix_ms', '3': 4, '4': 1, '5': 3, '10': 'revisionUnixMs'},
  ],
};

/// Descriptor for `FriendRelationshipUpdated`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendRelationshipUpdatedDescriptor = $convert.base64Decode(
    'ChlGcmllbmRSZWxhdGlvbnNoaXBVcGRhdGVkEiYKD3BlZXJfcHVibGljX2tleRgBIAEoCVINcG'
    'VlclB1YmxpY0tleRIlCg53b3Jrc3BhY2VfbmFtZRgCIAEoCVINd29ya3NwYWNlTmFtZRJDCgZj'
    'aGFuZ2UYAyABKA4yKy5naXpjbGF3LmV2ZW50cy52MS5GcmllbmRSZWxhdGlvbnNoaXBDaGFuZ2'
    'VSBmNoYW5nZRIoChByZXZpc2lvbl91bml4X21zGAQgASgDUg5yZXZpc2lvblVuaXhNcw==');

@$core.Deprecated('Use friendGroupUpdatedDescriptor instead')
const FriendGroupUpdated$json = {
  '1': 'FriendGroupUpdated',
  '2': [
    {'1': 'friend_group_id', '3': 1, '4': 1, '5': 9, '10': 'friendGroupId'},
    {'1': 'workspace_name', '3': 2, '4': 1, '5': 9, '10': 'workspaceName'},
    {
      '1': 'change',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.events.v1.FriendGroupChange',
      '10': 'change'
    },
    {'1': 'revision_unix_ms', '3': 4, '4': 1, '5': 3, '10': 'revisionUnixMs'},
    {
      '1': 'affected_peer_public_key',
      '3': 5,
      '4': 1,
      '5': 9,
      '10': 'affectedPeerPublicKey'
    },
  ],
};

/// Descriptor for `FriendGroupUpdated`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupUpdatedDescriptor = $convert.base64Decode(
    'ChJGcmllbmRHcm91cFVwZGF0ZWQSJgoPZnJpZW5kX2dyb3VwX2lkGAEgASgJUg1mcmllbmRHcm'
    '91cElkEiUKDndvcmtzcGFjZV9uYW1lGAIgASgJUg13b3Jrc3BhY2VOYW1lEjwKBmNoYW5nZRgD'
    'IAEoDjIkLmdpemNsYXcuZXZlbnRzLnYxLkZyaWVuZEdyb3VwQ2hhbmdlUgZjaGFuZ2USKAoQcm'
    'V2aXNpb25fdW5peF9tcxgEIAEoA1IOcmV2aXNpb25Vbml4TXMSNwoYYWZmZWN0ZWRfcGVlcl9w'
    'dWJsaWNfa2V5GAUgASgJUhVhZmZlY3RlZFBlZXJQdWJsaWNLZXk=');
