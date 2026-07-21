// This is a generated file - do not edit.
//
// Generated from payload/workspace.proto.

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

@$core.Deprecated('Use agentSelectionDescriptor instead')
const AgentSelection$json = {
  '1': 'AgentSelection',
  '2': [
    {'1': 'workspace_name', '3': 1, '4': 1, '5': 9, '10': 'workspaceName'},
  ],
};

/// Descriptor for `AgentSelection`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List agentSelectionDescriptor = $convert.base64Decode(
    'Cg5BZ2VudFNlbGVjdGlvbhIlCg53b3Jrc3BhY2VfbmFtZRgBIAEoCVINd29ya3NwYWNlTmFtZQ'
    '==');

@$core.Deprecated('Use peerRunAgentDescriptor instead')
const PeerRunAgent$json = {
  '1': 'PeerRunAgent',
  '2': [
    {
      '1': 'active',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AgentSelection',
      '9': 0,
      '10': 'active',
      '17': true
    },
    {
      '1': 'pending',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AgentSelection',
      '9': 1,
      '10': 'pending',
      '17': true
    },
  ],
  '8': [
    {'1': '_active'},
    {'1': '_pending'},
  ],
};

/// Descriptor for `PeerRunAgent`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunAgentDescriptor = $convert.base64Decode(
    'CgxQZWVyUnVuQWdlbnQSOwoGYWN0aXZlGAEgASgLMh4uZ2l6Y2xhdy5ycGMudjEuQWdlbnRTZW'
    'xlY3Rpb25IAFIGYWN0aXZliAEBEj0KB3BlbmRpbmcYAiABKAsyHi5naXpjbGF3LnJwYy52MS5B'
    'Z2VudFNlbGVjdGlvbkgBUgdwZW5kaW5niAEBQgkKB19hY3RpdmVCCgoIX3BlbmRpbmc=');

@$core.Deprecated('Use peerRunHistoryEntryDescriptor instead')
const PeerRunHistoryEntry$json = {
  '1': 'PeerRunHistoryEntry',
  '2': [
    {'1': 'created_at', '3': 1, '4': 1, '5': 9, '10': 'createdAt'},
    {
      '1': 'gear_id',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'gearId',
      '17': true
    },
    {'1': 'id', '3': 3, '4': 1, '5': 9, '10': 'id'},
    {'1': 'name', '3': 4, '4': 1, '5': 9, '10': 'name'},
    {'1': 'replay_available', '3': 5, '4': 1, '5': 8, '10': 'replayAvailable'},
    {'1': 'text', '3': 6, '4': 1, '5': 9, '10': 'text'},
    {
      '1': 'type',
      '3': 7,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.PeerRunHistoryEntryType',
      '10': 'type'
    },
  ],
  '8': [
    {'1': '_gear_id'},
  ],
};

/// Descriptor for `PeerRunHistoryEntry`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunHistoryEntryDescriptor = $convert.base64Decode(
    'ChNQZWVyUnVuSGlzdG9yeUVudHJ5Eh0KCmNyZWF0ZWRfYXQYASABKAlSCWNyZWF0ZWRBdBIcCg'
    'dnZWFyX2lkGAIgASgJSABSBmdlYXJJZIgBARIOCgJpZBgDIAEoCVICaWQSEgoEbmFtZRgEIAEo'
    'CVIEbmFtZRIpChByZXBsYXlfYXZhaWxhYmxlGAUgASgIUg9yZXBsYXlBdmFpbGFibGUSEgoEdG'
    'V4dBgGIAEoCVIEdGV4dBI7CgR0eXBlGAcgASgOMicuZ2l6Y2xhdy5ycGMudjEuUGVlclJ1bkhp'
    'c3RvcnlFbnRyeVR5cGVSBHR5cGVCCgoIX2dlYXJfaWQ=');

@$core.Deprecated('Use peerRunHistoryListRequestDescriptor instead')
const PeerRunHistoryListRequest$json = {
  '1': 'PeerRunHistoryListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
    {
      '1': 'order',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.PeerRunHistoryListRequestOrder',
      '9': 2,
      '10': 'order',
      '17': true
    },
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
    {'1': '_order'},
  ],
};

/// Descriptor for `PeerRunHistoryListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunHistoryListRequestDescriptor = $convert.base64Decode(
    'ChlQZWVyUnVuSGlzdG9yeUxpc3RSZXF1ZXN0EhsKBmN1cnNvchgBIAEoCUgAUgZjdXJzb3KIAQ'
    'ESGQoFbGltaXQYAiABKANIAVIFbGltaXSIAQESSQoFb3JkZXIYAyABKA4yLi5naXpjbGF3LnJw'
    'Yy52MS5QZWVyUnVuSGlzdG9yeUxpc3RSZXF1ZXN0T3JkZXJIAlIFb3JkZXKIAQFCCQoHX2N1cn'
    'NvckIICgZfbGltaXRCCAoGX29yZGVy');

@$core.Deprecated('Use peerRunHistoryListResponseDescriptor instead')
const PeerRunHistoryListResponse$json = {
  '1': 'PeerRunHistoryListResponse',
  '2': [
    {'1': 'available', '3': 1, '4': 1, '5': 8, '10': 'available'},
    {'1': 'has_next', '3': 2, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 3,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunHistoryEntry',
      '10': 'items'
    },
    {
      '1': 'message',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'message',
      '17': true
    },
    {
      '1': 'next_cursor',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'nextCursor',
      '17': true
    },
  ],
  '8': [
    {'1': '_message'},
    {'1': '_next_cursor'},
  ],
};

/// Descriptor for `PeerRunHistoryListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunHistoryListResponseDescriptor = $convert.base64Decode(
    'ChpQZWVyUnVuSGlzdG9yeUxpc3RSZXNwb25zZRIcCglhdmFpbGFibGUYASABKAhSCWF2YWlsYW'
    'JsZRIZCghoYXNfbmV4dBgCIAEoCFIHaGFzTmV4dBI5CgVpdGVtcxgDIAMoCzIjLmdpemNsYXcu'
    'cnBjLnYxLlBlZXJSdW5IaXN0b3J5RW50cnlSBWl0ZW1zEh0KB21lc3NhZ2UYBCABKAlIAFIHbW'
    'Vzc2FnZYgBARIkCgtuZXh0X2N1cnNvchgFIAEoCUgBUgpuZXh0Q3Vyc29yiAEBQgoKCF9tZXNz'
    'YWdlQg4KDF9uZXh0X2N1cnNvcg==');

@$core.Deprecated('Use peerRunHistoryPlayRequestDescriptor instead')
const PeerRunHistoryPlayRequest$json = {
  '1': 'PeerRunHistoryPlayRequest',
  '2': [
    {'1': 'history_id', '3': 1, '4': 1, '5': 9, '10': 'historyId'},
  ],
};

/// Descriptor for `PeerRunHistoryPlayRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunHistoryPlayRequestDescriptor =
    $convert.base64Decode(
        'ChlQZWVyUnVuSGlzdG9yeVBsYXlSZXF1ZXN0Eh0KCmhpc3RvcnlfaWQYASABKAlSCWhpc3Rvcn'
        'lJZA==');

@$core.Deprecated('Use peerRunHistoryPlayResponseDescriptor instead')
const PeerRunHistoryPlayResponse$json = {
  '1': 'PeerRunHistoryPlayResponse',
  '2': [
    {'1': 'accepted', '3': 1, '4': 1, '5': 8, '10': 'accepted'},
    {'1': 'history_id', '3': 2, '4': 1, '5': 9, '10': 'historyId'},
    {
      '1': 'message',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'message',
      '17': true
    },
    {'1': 'state', '3': 4, '4': 1, '5': 9, '10': 'state'},
  ],
  '8': [
    {'1': '_message'},
  ],
};

/// Descriptor for `PeerRunHistoryPlayResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunHistoryPlayResponseDescriptor =
    $convert.base64Decode(
        'ChpQZWVyUnVuSGlzdG9yeVBsYXlSZXNwb25zZRIaCghhY2NlcHRlZBgBIAEoCFIIYWNjZXB0ZW'
        'QSHQoKaGlzdG9yeV9pZBgCIAEoCVIJaGlzdG9yeUlkEh0KB21lc3NhZ2UYAyABKAlIAFIHbWVz'
        'c2FnZYgBARIUCgVzdGF0ZRgEIAEoCVIFc3RhdGVCCgoIX21lc3NhZ2U=');

@$core.Deprecated('Use peerRunMemoryStatsRequestDescriptor instead')
const PeerRunMemoryStatsRequest$json = {
  '1': 'PeerRunMemoryStatsRequest',
};

/// Descriptor for `PeerRunMemoryStatsRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunMemoryStatsRequestDescriptor =
    $convert.base64Decode('ChlQZWVyUnVuTWVtb3J5U3RhdHNSZXF1ZXN0');

@$core.Deprecated('Use peerRunMemoryStatsResponseDescriptor instead')
const PeerRunMemoryStatsResponse$json = {
  '1': 'PeerRunMemoryStatsResponse',
  '2': [
    {'1': 'available', '3': 1, '4': 1, '5': 8, '10': 'available'},
    {
      '1': 'backend',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'backend',
      '17': true
    },
    {
      '1': 'embedding_enabled',
      '3': 3,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'embeddingEnabled',
      '17': true
    },
    {
      '1': 'embedding_status',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'embeddingStatus',
      '17': true
    },
    {'1': 'enabled', '3': 5, '4': 1, '5': 8, '10': 'enabled'},
    {
      '1': 'index_status',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'indexStatus',
      '17': true
    },
    {'1': 'item_count', '3': 7, '4': 1, '5': 3, '10': 'itemCount'},
    {
      '1': 'last_updated_at',
      '3': 8,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'lastUpdatedAt',
      '17': true
    },
    {
      '1': 'message',
      '3': 9,
      '4': 1,
      '5': 9,
      '9': 5,
      '10': 'message',
      '17': true
    },
    {
      '1': 'metadata',
      '3': 10,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '10': 'metadata'
    },
    {'1': 'storage_bytes', '3': 11, '4': 1, '5': 3, '10': 'storageBytes'},
  ],
  '8': [
    {'1': '_backend'},
    {'1': '_embedding_enabled'},
    {'1': '_embedding_status'},
    {'1': '_index_status'},
    {'1': '_last_updated_at'},
    {'1': '_message'},
  ],
};

/// Descriptor for `PeerRunMemoryStatsResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunMemoryStatsResponseDescriptor = $convert.base64Decode(
    'ChpQZWVyUnVuTWVtb3J5U3RhdHNSZXNwb25zZRIcCglhdmFpbGFibGUYASABKAhSCWF2YWlsYW'
    'JsZRIdCgdiYWNrZW5kGAIgASgJSABSB2JhY2tlbmSIAQESMAoRZW1iZWRkaW5nX2VuYWJsZWQY'
    'AyABKAhIAVIQZW1iZWRkaW5nRW5hYmxlZIgBARIuChBlbWJlZGRpbmdfc3RhdHVzGAQgASgJSA'
    'JSD2VtYmVkZGluZ1N0YXR1c4gBARIYCgdlbmFibGVkGAUgASgIUgdlbmFibGVkEiYKDGluZGV4'
    'X3N0YXR1cxgGIAEoCUgDUgtpbmRleFN0YXR1c4gBARIdCgppdGVtX2NvdW50GAcgASgDUglpdG'
    'VtQ291bnQSKwoPbGFzdF91cGRhdGVkX2F0GAggASgJSARSDWxhc3RVcGRhdGVkQXSIAQESHQoH'
    'bWVzc2FnZRgJIAEoCUgFUgdtZXNzYWdliAEBEjMKCG1ldGFkYXRhGAogASgLMhcuZ29vZ2xlLn'
    'Byb3RvYnVmLlN0cnVjdFIIbWV0YWRhdGESIwoNc3RvcmFnZV9ieXRlcxgLIAEoA1IMc3RvcmFn'
    'ZUJ5dGVzQgoKCF9iYWNrZW5kQhQKEl9lbWJlZGRpbmdfZW5hYmxlZEITChFfZW1iZWRkaW5nX3'
    'N0YXR1c0IPCg1faW5kZXhfc3RhdHVzQhIKEF9sYXN0X3VwZGF0ZWRfYXRCCgoIX21lc3NhZ2U=');

@$core.Deprecated('Use peerRunRecallHitDescriptor instead')
const PeerRunRecallHit$json = {
  '1': 'PeerRunRecallHit',
  '2': [
    {
      '1': 'created_at',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'createdAt',
      '17': true
    },
    {'1': 'id', '3': 2, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'metadata',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '10': 'metadata'
    },
    {'1': 'score', '3': 4, '4': 1, '5': 1, '10': 'score'},
    {'1': 'snippet', '3': 5, '4': 1, '5': 9, '10': 'snippet'},
    {
      '1': 'source_id',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'sourceId',
      '17': true
    },
    {
      '1': 'source_type',
      '3': 7,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'sourceType',
      '17': true
    },
  ],
  '8': [
    {'1': '_created_at'},
    {'1': '_source_id'},
    {'1': '_source_type'},
  ],
};

/// Descriptor for `PeerRunRecallHit`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunRecallHitDescriptor = $convert.base64Decode(
    'ChBQZWVyUnVuUmVjYWxsSGl0EiIKCmNyZWF0ZWRfYXQYASABKAlIAFIJY3JlYXRlZEF0iAEBEg'
    '4KAmlkGAIgASgJUgJpZBIzCghtZXRhZGF0YRgDIAEoCzIXLmdvb2dsZS5wcm90b2J1Zi5TdHJ1'
    'Y3RSCG1ldGFkYXRhEhQKBXNjb3JlGAQgASgBUgVzY29yZRIYCgdzbmlwcGV0GAUgASgJUgdzbm'
    'lwcGV0EiAKCXNvdXJjZV9pZBgGIAEoCUgBUghzb3VyY2VJZIgBARIkCgtzb3VyY2VfdHlwZRgH'
    'IAEoCUgCUgpzb3VyY2VUeXBliAEBQg0KC19jcmVhdGVkX2F0QgwKCl9zb3VyY2VfaWRCDgoMX3'
    'NvdXJjZV90eXBl');

@$core.Deprecated('Use peerRunRecallRequestDescriptor instead')
const PeerRunRecallRequest$json = {
  '1': 'PeerRunRecallRequest',
  '2': [
    {
      '1': 'filters',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '9': 0,
      '10': 'filters',
      '17': true
    },
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
    {'1': 'query', '3': 3, '4': 1, '5': 9, '10': 'query'},
  ],
  '8': [
    {'1': '_filters'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `PeerRunRecallRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunRecallRequestDescriptor = $convert.base64Decode(
    'ChRQZWVyUnVuUmVjYWxsUmVxdWVzdBI2CgdmaWx0ZXJzGAEgASgLMhcuZ29vZ2xlLnByb3RvYn'
    'VmLlN0cnVjdEgAUgdmaWx0ZXJziAEBEhkKBWxpbWl0GAIgASgDSAFSBWxpbWl0iAEBEhQKBXF1'
    'ZXJ5GAMgASgJUgVxdWVyeUIKCghfZmlsdGVyc0IICgZfbGltaXQ=');

@$core.Deprecated('Use peerRunRecallResponseDescriptor instead')
const PeerRunRecallResponse$json = {
  '1': 'PeerRunRecallResponse',
  '2': [
    {'1': 'available', '3': 1, '4': 1, '5': 8, '10': 'available'},
    {
      '1': 'hits',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunRecallHit',
      '10': 'hits'
    },
    {
      '1': 'message',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'message',
      '17': true
    },
  ],
  '8': [
    {'1': '_message'},
  ],
};

/// Descriptor for `PeerRunRecallResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunRecallResponseDescriptor = $convert.base64Decode(
    'ChVQZWVyUnVuUmVjYWxsUmVzcG9uc2USHAoJYXZhaWxhYmxlGAEgASgIUglhdmFpbGFibGUSNA'
    'oEaGl0cxgCIAMoCzIgLmdpemNsYXcucnBjLnYxLlBlZXJSdW5SZWNhbGxIaXRSBGhpdHMSHQoH'
    'bWVzc2FnZRgDIAEoCUgAUgdtZXNzYWdliAEBQgoKCF9tZXNzYWdl');

@$core.Deprecated('Use peerRunStatusDescriptor instead')
const PeerRunStatus$json = {
  '1': 'PeerRunStatus',
  '2': [
    {
      '1': 'message',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'message',
      '17': true
    },
    {
      '1': 'started_at',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'startedAt',
      '17': true
    },
    {
      '1': 'state',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.PeerRunStatusState',
      '10': 'state'
    },
    {
      '1': 'updated_at',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'updatedAt',
      '17': true
    },
    {
      '1': 'workspace_name',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'workspaceName',
      '17': true
    },
  ],
  '8': [
    {'1': '_message'},
    {'1': '_started_at'},
    {'1': '_updated_at'},
    {'1': '_workspace_name'},
  ],
};

/// Descriptor for `PeerRunStatus`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunStatusDescriptor = $convert.base64Decode(
    'Cg1QZWVyUnVuU3RhdHVzEh0KB21lc3NhZ2UYASABKAlIAFIHbWVzc2FnZYgBARIiCgpzdGFydG'
    'VkX2F0GAIgASgJSAFSCXN0YXJ0ZWRBdIgBARI4CgVzdGF0ZRgDIAEoDjIiLmdpemNsYXcucnBj'
    'LnYxLlBlZXJSdW5TdGF0dXNTdGF0ZVIFc3RhdGUSIgoKdXBkYXRlZF9hdBgEIAEoCUgCUgl1cG'
    'RhdGVkQXSIAQESKgoOd29ya3NwYWNlX25hbWUYBSABKAlIA1INd29ya3NwYWNlTmFtZYgBAUIK'
    'CghfbWVzc2FnZUINCgtfc3RhcnRlZF9hdEINCgtfdXBkYXRlZF9hdEIRCg9fd29ya3NwYWNlX2'
    '5hbWU=');

@$core.Deprecated('Use peerRunWorkspaceStateDescriptor instead')
const PeerRunWorkspaceState$json = {
  '1': 'PeerRunWorkspaceState',
  '2': [
    {
      '1': 'active_workspace_name',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'activeWorkspaceName',
      '17': true
    },
    {
      '1': 'agent_type',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'agentType',
      '17': true
    },
    {
      '1': 'history_available',
      '3': 3,
      '4': 1,
      '5': 8,
      '9': 2,
      '10': 'historyAvailable',
      '17': true
    },
    {
      '1': 'memory_stats_available',
      '3': 4,
      '4': 1,
      '5': 8,
      '9': 3,
      '10': 'memoryStatsAvailable',
      '17': true
    },
    {
      '1': 'message',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'message',
      '17': true
    },
    {
      '1': 'pending_workspace_name',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 5,
      '10': 'pendingWorkspaceName',
      '17': true
    },
    {
      '1': 'recall_available',
      '3': 7,
      '4': 1,
      '5': 8,
      '9': 6,
      '10': 'recallAvailable',
      '17': true
    },
    {
      '1': 'runtime_state',
      '3': 8,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.PeerRunStatusState',
      '10': 'runtimeState'
    },
    {
      '1': 'selected_workspace_name',
      '3': 9,
      '4': 1,
      '5': 9,
      '9': 7,
      '10': 'selectedWorkspaceName',
      '17': true
    },
    {
      '1': 'started_at',
      '3': 10,
      '4': 1,
      '5': 9,
      '9': 8,
      '10': 'startedAt',
      '17': true
    },
    {
      '1': 'updated_at',
      '3': 11,
      '4': 1,
      '5': 9,
      '9': 9,
      '10': 'updatedAt',
      '17': true
    },
    {
      '1': 'workflow_name',
      '3': 12,
      '4': 1,
      '5': 9,
      '9': 10,
      '10': 'workflowName',
      '17': true
    },
    {'1': 'workspace_name', '3': 13, '4': 1, '5': 9, '10': 'workspaceName'},
  ],
  '8': [
    {'1': '_active_workspace_name'},
    {'1': '_agent_type'},
    {'1': '_history_available'},
    {'1': '_memory_stats_available'},
    {'1': '_message'},
    {'1': '_pending_workspace_name'},
    {'1': '_recall_available'},
    {'1': '_selected_workspace_name'},
    {'1': '_started_at'},
    {'1': '_updated_at'},
    {'1': '_workflow_name'},
  ],
};

/// Descriptor for `PeerRunWorkspaceState`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerRunWorkspaceStateDescriptor = $convert.base64Decode(
    'ChVQZWVyUnVuV29ya3NwYWNlU3RhdGUSNwoVYWN0aXZlX3dvcmtzcGFjZV9uYW1lGAEgASgJSA'
    'BSE2FjdGl2ZVdvcmtzcGFjZU5hbWWIAQESIgoKYWdlbnRfdHlwZRgCIAEoCUgBUglhZ2VudFR5'
    'cGWIAQESMAoRaGlzdG9yeV9hdmFpbGFibGUYAyABKAhIAlIQaGlzdG9yeUF2YWlsYWJsZYgBAR'
    'I5ChZtZW1vcnlfc3RhdHNfYXZhaWxhYmxlGAQgASgISANSFG1lbW9yeVN0YXRzQXZhaWxhYmxl'
    'iAEBEh0KB21lc3NhZ2UYBSABKAlIBFIHbWVzc2FnZYgBARI5ChZwZW5kaW5nX3dvcmtzcGFjZV'
    '9uYW1lGAYgASgJSAVSFHBlbmRpbmdXb3Jrc3BhY2VOYW1liAEBEi4KEHJlY2FsbF9hdmFpbGFi'
    'bGUYByABKAhIBlIPcmVjYWxsQXZhaWxhYmxliAEBEkcKDXJ1bnRpbWVfc3RhdGUYCCABKA4yIi'
    '5naXpjbGF3LnJwYy52MS5QZWVyUnVuU3RhdHVzU3RhdGVSDHJ1bnRpbWVTdGF0ZRI7ChdzZWxl'
    'Y3RlZF93b3Jrc3BhY2VfbmFtZRgJIAEoCUgHUhVzZWxlY3RlZFdvcmtzcGFjZU5hbWWIAQESIg'
    'oKc3RhcnRlZF9hdBgKIAEoCUgIUglzdGFydGVkQXSIAQESIgoKdXBkYXRlZF9hdBgLIAEoCUgJ'
    'Ugl1cGRhdGVkQXSIAQESKAoNd29ya2Zsb3dfbmFtZRgMIAEoCUgKUgx3b3JrZmxvd05hbWWIAQ'
    'ESJQoOd29ya3NwYWNlX25hbWUYDSABKAlSDXdvcmtzcGFjZU5hbWVCGAoWX2FjdGl2ZV93b3Jr'
    'c3BhY2VfbmFtZUINCgtfYWdlbnRfdHlwZUIUChJfaGlzdG9yeV9hdmFpbGFibGVCGQoXX21lbW'
    '9yeV9zdGF0c19hdmFpbGFibGVCCgoIX21lc3NhZ2VCGQoXX3BlbmRpbmdfd29ya3NwYWNlX25h'
    'bWVCEwoRX3JlY2FsbF9hdmFpbGFibGVCGgoYX3NlbGVjdGVkX3dvcmtzcGFjZV9uYW1lQg0KC1'
    '9zdGFydGVkX2F0Qg0KC191cGRhdGVkX2F0QhAKDl93b3JrZmxvd19uYW1l');

@$core.Deprecated('Use serverGetRunAgentRequestDescriptor instead')
const ServerGetRunAgentRequest$json = {
  '1': 'ServerGetRunAgentRequest',
};

/// Descriptor for `ServerGetRunAgentRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetRunAgentRequestDescriptor =
    $convert.base64Decode('ChhTZXJ2ZXJHZXRSdW5BZ2VudFJlcXVlc3Q=');

@$core.Deprecated('Use serverGetRunAgentResponseDescriptor instead')
const ServerGetRunAgentResponse$json = {
  '1': 'ServerGetRunAgentResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunAgent',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGetRunAgentResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetRunAgentResponseDescriptor =
    $convert.base64Decode(
        'ChlTZXJ2ZXJHZXRSdW5BZ2VudFJlc3BvbnNlEjIKBXZhbHVlGAEgASgLMhwuZ2l6Y2xhdy5ycG'
        'MudjEuUGVlclJ1bkFnZW50UgV2YWx1ZQ==');

@$core.Deprecated('Use serverGetRunStatusRequestDescriptor instead')
const ServerGetRunStatusRequest$json = {
  '1': 'ServerGetRunStatusRequest',
};

/// Descriptor for `ServerGetRunStatusRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetRunStatusRequestDescriptor =
    $convert.base64Decode('ChlTZXJ2ZXJHZXRSdW5TdGF0dXNSZXF1ZXN0');

@$core.Deprecated('Use serverGetRunStatusResponseDescriptor instead')
const ServerGetRunStatusResponse$json = {
  '1': 'ServerGetRunStatusResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunStatus',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGetRunStatusResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetRunStatusResponseDescriptor =
    $convert.base64Decode(
        'ChpTZXJ2ZXJHZXRSdW5TdGF0dXNSZXNwb25zZRIzCgV2YWx1ZRgBIAEoCzIdLmdpemNsYXcucn'
        'BjLnYxLlBlZXJSdW5TdGF0dXNSBXZhbHVl');

@$core
    .Deprecated('Use serverGetRunWorkspaceMemoryStatsRequestDescriptor instead')
const ServerGetRunWorkspaceMemoryStatsRequest$json = {
  '1': 'ServerGetRunWorkspaceMemoryStatsRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunMemoryStatsRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGetRunWorkspaceMemoryStatsRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetRunWorkspaceMemoryStatsRequestDescriptor =
    $convert.base64Decode(
        'CidTZXJ2ZXJHZXRSdW5Xb3Jrc3BhY2VNZW1vcnlTdGF0c1JlcXVlc3QSPwoFdmFsdWUYASABKA'
        'syKS5naXpjbGF3LnJwYy52MS5QZWVyUnVuTWVtb3J5U3RhdHNSZXF1ZXN0UgV2YWx1ZQ==');

@$core.Deprecated(
    'Use serverGetRunWorkspaceMemoryStatsResponseDescriptor instead')
const ServerGetRunWorkspaceMemoryStatsResponse$json = {
  '1': 'ServerGetRunWorkspaceMemoryStatsResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunMemoryStatsResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGetRunWorkspaceMemoryStatsResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetRunWorkspaceMemoryStatsResponseDescriptor =
    $convert.base64Decode(
        'CihTZXJ2ZXJHZXRSdW5Xb3Jrc3BhY2VNZW1vcnlTdGF0c1Jlc3BvbnNlEkAKBXZhbHVlGAEgAS'
        'gLMiouZ2l6Y2xhdy5ycGMudjEuUGVlclJ1bk1lbW9yeVN0YXRzUmVzcG9uc2VSBXZhbHVl');

@$core.Deprecated('Use serverGetRunWorkspaceRequestDescriptor instead')
const ServerGetRunWorkspaceRequest$json = {
  '1': 'ServerGetRunWorkspaceRequest',
};

/// Descriptor for `ServerGetRunWorkspaceRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetRunWorkspaceRequestDescriptor =
    $convert.base64Decode('ChxTZXJ2ZXJHZXRSdW5Xb3Jrc3BhY2VSZXF1ZXN0');

@$core.Deprecated('Use serverGetRunWorkspaceResponseDescriptor instead')
const ServerGetRunWorkspaceResponse$json = {
  '1': 'ServerGetRunWorkspaceResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunWorkspaceState',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGetRunWorkspaceResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetRunWorkspaceResponseDescriptor =
    $convert.base64Decode(
        'Ch1TZXJ2ZXJHZXRSdW5Xb3Jrc3BhY2VSZXNwb25zZRI7CgV2YWx1ZRgBIAEoCzIlLmdpemNsYX'
        'cucnBjLnYxLlBlZXJSdW5Xb3Jrc3BhY2VTdGF0ZVIFdmFsdWU=');

@$core.Deprecated('Use serverGetRuntimeRequestDescriptor instead')
const ServerGetRuntimeRequest$json = {
  '1': 'ServerGetRuntimeRequest',
};

/// Descriptor for `ServerGetRuntimeRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetRuntimeRequestDescriptor =
    $convert.base64Decode('ChdTZXJ2ZXJHZXRSdW50aW1lUmVxdWVzdA==');

@$core.Deprecated('Use serverGetRuntimeResponseDescriptor instead')
const ServerGetRuntimeResponse$json = {
  '1': 'ServerGetRuntimeResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Runtime',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGetRuntimeResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetRuntimeResponseDescriptor =
    $convert.base64Decode(
        'ChhTZXJ2ZXJHZXRSdW50aW1lUmVzcG9uc2USLQoFdmFsdWUYASABKAsyFy5naXpjbGF3LnJwYy'
        '52MS5SdW50aW1lUgV2YWx1ZQ==');

@$core.Deprecated('Use serverListRunWorkspaceHistoryRequestDescriptor instead')
const ServerListRunWorkspaceHistoryRequest$json = {
  '1': 'ServerListRunWorkspaceHistoryRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunHistoryListRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerListRunWorkspaceHistoryRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverListRunWorkspaceHistoryRequestDescriptor =
    $convert.base64Decode(
        'CiRTZXJ2ZXJMaXN0UnVuV29ya3NwYWNlSGlzdG9yeVJlcXVlc3QSPwoFdmFsdWUYASABKAsyKS'
        '5naXpjbGF3LnJwYy52MS5QZWVyUnVuSGlzdG9yeUxpc3RSZXF1ZXN0UgV2YWx1ZQ==');

@$core.Deprecated('Use serverListRunWorkspaceHistoryResponseDescriptor instead')
const ServerListRunWorkspaceHistoryResponse$json = {
  '1': 'ServerListRunWorkspaceHistoryResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunHistoryListResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerListRunWorkspaceHistoryResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverListRunWorkspaceHistoryResponseDescriptor =
    $convert.base64Decode(
        'CiVTZXJ2ZXJMaXN0UnVuV29ya3NwYWNlSGlzdG9yeVJlc3BvbnNlEkAKBXZhbHVlGAEgASgLMi'
        'ouZ2l6Y2xhdy5ycGMudjEuUGVlclJ1bkhpc3RvcnlMaXN0UmVzcG9uc2VSBXZhbHVl');

@$core.Deprecated('Use serverPlayRunWorkspaceHistoryRequestDescriptor instead')
const ServerPlayRunWorkspaceHistoryRequest$json = {
  '1': 'ServerPlayRunWorkspaceHistoryRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunHistoryPlayRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPlayRunWorkspaceHistoryRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPlayRunWorkspaceHistoryRequestDescriptor =
    $convert.base64Decode(
        'CiRTZXJ2ZXJQbGF5UnVuV29ya3NwYWNlSGlzdG9yeVJlcXVlc3QSPwoFdmFsdWUYASABKAsyKS'
        '5naXpjbGF3LnJwYy52MS5QZWVyUnVuSGlzdG9yeVBsYXlSZXF1ZXN0UgV2YWx1ZQ==');

@$core.Deprecated('Use serverPlayRunWorkspaceHistoryResponseDescriptor instead')
const ServerPlayRunWorkspaceHistoryResponse$json = {
  '1': 'ServerPlayRunWorkspaceHistoryResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunHistoryPlayResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPlayRunWorkspaceHistoryResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPlayRunWorkspaceHistoryResponseDescriptor =
    $convert.base64Decode(
        'CiVTZXJ2ZXJQbGF5UnVuV29ya3NwYWNlSGlzdG9yeVJlc3BvbnNlEkAKBXZhbHVlGAEgASgLMi'
        'ouZ2l6Y2xhdy5ycGMudjEuUGVlclJ1bkhpc3RvcnlQbGF5UmVzcG9uc2VSBXZhbHVl');

@$core.Deprecated('Use serverReloadRunRequestDescriptor instead')
const ServerReloadRunRequest$json = {
  '1': 'ServerReloadRunRequest',
};

/// Descriptor for `ServerReloadRunRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverReloadRunRequestDescriptor =
    $convert.base64Decode('ChZTZXJ2ZXJSZWxvYWRSdW5SZXF1ZXN0');

@$core.Deprecated('Use serverReloadRunResponseDescriptor instead')
const ServerReloadRunResponse$json = {
  '1': 'ServerReloadRunResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunStatus',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerReloadRunResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverReloadRunResponseDescriptor =
    $convert.base64Decode(
        'ChdTZXJ2ZXJSZWxvYWRSdW5SZXNwb25zZRIzCgV2YWx1ZRgBIAEoCzIdLmdpemNsYXcucnBjLn'
        'YxLlBlZXJSdW5TdGF0dXNSBXZhbHVl');

@$core.Deprecated('Use serverReloadRunWorkspaceRequestDescriptor instead')
const ServerReloadRunWorkspaceRequest$json = {
  '1': 'ServerReloadRunWorkspaceRequest',
};

/// Descriptor for `ServerReloadRunWorkspaceRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverReloadRunWorkspaceRequestDescriptor =
    $convert.base64Decode('Ch9TZXJ2ZXJSZWxvYWRSdW5Xb3Jrc3BhY2VSZXF1ZXN0');

@$core.Deprecated('Use serverReloadRunWorkspaceResponseDescriptor instead')
const ServerReloadRunWorkspaceResponse$json = {
  '1': 'ServerReloadRunWorkspaceResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunWorkspaceState',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerReloadRunWorkspaceResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverReloadRunWorkspaceResponseDescriptor =
    $convert.base64Decode(
        'CiBTZXJ2ZXJSZWxvYWRSdW5Xb3Jrc3BhY2VSZXNwb25zZRI7CgV2YWx1ZRgBIAEoCzIlLmdpem'
        'NsYXcucnBjLnYxLlBlZXJSdW5Xb3Jrc3BhY2VTdGF0ZVIFdmFsdWU=');

@$core.Deprecated('Use serverRunSayRequestDescriptor instead')
const ServerRunSayRequest$json = {
  '1': 'ServerRunSayRequest',
  '2': [
    {'1': 'text', '3': 1, '4': 1, '5': 9, '10': 'text'},
    {'1': 'voice_alias', '3': 2, '4': 1, '5': 9, '10': 'voiceAlias'},
  ],
};

/// Descriptor for `ServerRunSayRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRunSayRequestDescriptor = $convert.base64Decode(
    'ChNTZXJ2ZXJSdW5TYXlSZXF1ZXN0EhIKBHRleHQYASABKAlSBHRleHQSHwoLdm9pY2VfYWxpYX'
    'MYAiABKAlSCnZvaWNlQWxpYXM=');

@$core.Deprecated('Use serverRunSayResponseDescriptor instead')
const ServerRunSayResponse$json = {
  '1': 'ServerRunSayResponse',
  '2': [
    {'1': 'accepted', '3': 1, '4': 1, '5': 8, '10': 'accepted'},
  ],
};

/// Descriptor for `ServerRunSayResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRunSayResponseDescriptor =
    $convert.base64Decode(
        'ChRTZXJ2ZXJSdW5TYXlSZXNwb25zZRIaCghhY2NlcHRlZBgBIAEoCFIIYWNjZXB0ZWQ=');

@$core.Deprecated('Use serverRunWorkspaceRecallRequestDescriptor instead')
const ServerRunWorkspaceRecallRequest$json = {
  '1': 'ServerRunWorkspaceRecallRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunRecallRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerRunWorkspaceRecallRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRunWorkspaceRecallRequestDescriptor =
    $convert.base64Decode(
        'Ch9TZXJ2ZXJSdW5Xb3Jrc3BhY2VSZWNhbGxSZXF1ZXN0EjoKBXZhbHVlGAEgASgLMiQuZ2l6Y2'
        'xhdy5ycGMudjEuUGVlclJ1blJlY2FsbFJlcXVlc3RSBXZhbHVl');

@$core.Deprecated('Use serverRunWorkspaceRecallResponseDescriptor instead')
const ServerRunWorkspaceRecallResponse$json = {
  '1': 'ServerRunWorkspaceRecallResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunRecallResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerRunWorkspaceRecallResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRunWorkspaceRecallResponseDescriptor =
    $convert.base64Decode(
        'CiBTZXJ2ZXJSdW5Xb3Jrc3BhY2VSZWNhbGxSZXNwb25zZRI7CgV2YWx1ZRgBIAEoCzIlLmdpem'
        'NsYXcucnBjLnYxLlBlZXJSdW5SZWNhbGxSZXNwb25zZVIFdmFsdWU=');

@$core.Deprecated('Use serverSetRunAgentRequestDescriptor instead')
const ServerSetRunAgentRequest$json = {
  '1': 'ServerSetRunAgentRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AgentSelection',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerSetRunAgentRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverSetRunAgentRequestDescriptor =
    $convert.base64Decode(
        'ChhTZXJ2ZXJTZXRSdW5BZ2VudFJlcXVlc3QSNAoFdmFsdWUYASABKAsyHi5naXpjbGF3LnJwYy'
        '52MS5BZ2VudFNlbGVjdGlvblIFdmFsdWU=');

@$core.Deprecated('Use serverSetRunAgentResponseDescriptor instead')
const ServerSetRunAgentResponse$json = {
  '1': 'ServerSetRunAgentResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunAgent',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerSetRunAgentResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverSetRunAgentResponseDescriptor =
    $convert.base64Decode(
        'ChlTZXJ2ZXJTZXRSdW5BZ2VudFJlc3BvbnNlEjIKBXZhbHVlGAEgASgLMhwuZ2l6Y2xhdy5ycG'
        'MudjEuUGVlclJ1bkFnZW50UgV2YWx1ZQ==');

@$core.Deprecated('Use serverSetRunWorkspaceRequestDescriptor instead')
const ServerSetRunWorkspaceRequest$json = {
  '1': 'ServerSetRunWorkspaceRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AgentSelection',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerSetRunWorkspaceRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverSetRunWorkspaceRequestDescriptor =
    $convert.base64Decode(
        'ChxTZXJ2ZXJTZXRSdW5Xb3Jrc3BhY2VSZXF1ZXN0EjQKBXZhbHVlGAEgASgLMh4uZ2l6Y2xhdy'
        '5ycGMudjEuQWdlbnRTZWxlY3Rpb25SBXZhbHVl');

@$core.Deprecated('Use serverSetRunWorkspaceResponseDescriptor instead')
const ServerSetRunWorkspaceResponse$json = {
  '1': 'ServerSetRunWorkspaceResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunWorkspaceState',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerSetRunWorkspaceResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverSetRunWorkspaceResponseDescriptor =
    $convert.base64Decode(
        'Ch1TZXJ2ZXJTZXRSdW5Xb3Jrc3BhY2VSZXNwb25zZRI7CgV2YWx1ZRgBIAEoCzIlLmdpemNsYX'
        'cucnBjLnYxLlBlZXJSdW5Xb3Jrc3BhY2VTdGF0ZVIFdmFsdWU=');

@$core.Deprecated('Use serverStopRunRequestDescriptor instead')
const ServerStopRunRequest$json = {
  '1': 'ServerStopRunRequest',
};

/// Descriptor for `ServerStopRunRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverStopRunRequestDescriptor =
    $convert.base64Decode('ChRTZXJ2ZXJTdG9wUnVuUmVxdWVzdA==');

@$core.Deprecated('Use serverStopRunResponseDescriptor instead')
const ServerStopRunResponse$json = {
  '1': 'ServerStopRunResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunStatus',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerStopRunResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverStopRunResponseDescriptor = $convert.base64Decode(
    'ChVTZXJ2ZXJTdG9wUnVuUmVzcG9uc2USMwoFdmFsdWUYASABKAsyHS5naXpjbGF3LnJwYy52MS'
    '5QZWVyUnVuU3RhdHVzUgV2YWx1ZQ==');

@$core.Deprecated('Use workspaceDescriptor instead')
const Workspace$json = {
  '1': 'Workspace',
  '2': [
    {'1': 'created_at', '3': 1, '4': 1, '5': 9, '10': 'createdAt'},
    {'1': 'last_active_at', '3': 2, '4': 1, '5': 9, '10': 'lastActiveAt'},
    {'1': 'name', '3': 3, '4': 1, '5': 9, '10': 'name'},
    {
      '1': 'parameters',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkspaceParameters',
      '9': 0,
      '10': 'parameters',
      '17': true
    },
    {'1': 'updated_at', '3': 5, '4': 1, '5': 9, '10': 'updatedAt'},
    {'1': 'workflow_alias', '3': 6, '4': 1, '5': 9, '10': 'workflowAlias'},
    {
      '1': 'toolkit',
      '3': 7,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ToolkitPolicy',
      '9': 1,
      '10': 'toolkit',
      '17': true
    },
    {'1': 'system', '3': 8, '4': 1, '5': 8, '10': 'system'},
    {
      '1': 'icon',
      '3': 9,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Icon',
      '9': 2,
      '10': 'icon',
      '17': true
    },
    {
      '1': 'owner_public_key',
      '3': 10,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'ownerPublicKey',
      '17': true
    },
    {'1': 'available', '3': 11, '4': 1, '5': 8, '10': 'available'},
  ],
  '8': [
    {'1': '_parameters'},
    {'1': '_toolkit'},
    {'1': '_icon'},
    {'1': '_owner_public_key'},
  ],
};

/// Descriptor for `Workspace`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceDescriptor = $convert.base64Decode(
    'CglXb3Jrc3BhY2USHQoKY3JlYXRlZF9hdBgBIAEoCVIJY3JlYXRlZEF0EiQKDmxhc3RfYWN0aX'
    'ZlX2F0GAIgASgJUgxsYXN0QWN0aXZlQXQSEgoEbmFtZRgDIAEoCVIEbmFtZRJICgpwYXJhbWV0'
    'ZXJzGAQgASgLMiMuZ2l6Y2xhdy5ycGMudjEuV29ya3NwYWNlUGFyYW1ldGVyc0gAUgpwYXJhbW'
    'V0ZXJziAEBEh0KCnVwZGF0ZWRfYXQYBSABKAlSCXVwZGF0ZWRBdBIlCg53b3JrZmxvd19hbGlh'
    'cxgGIAEoCVINd29ya2Zsb3dBbGlhcxI8Cgd0b29sa2l0GAcgASgLMh0uZ2l6Y2xhdy5ycGMudj'
    'EuVG9vbGtpdFBvbGljeUgBUgd0b29sa2l0iAEBEhYKBnN5c3RlbRgIIAEoCFIGc3lzdGVtEi0K'
    'BGljb24YCSABKAsyFC5naXpjbGF3LnJwYy52MS5JY29uSAJSBGljb26IAQESLQoQb3duZXJfcH'
    'VibGljX2tleRgKIAEoCUgDUg5vd25lclB1YmxpY0tleYgBARIcCglhdmFpbGFibGUYCyABKAhS'
    'CWF2YWlsYWJsZUINCgtfcGFyYW1ldGVyc0IKCghfdG9vbGtpdEIHCgVfaWNvbkITChFfb3duZX'
    'JfcHVibGljX2tleQ==');

@$core.Deprecated('Use workspaceCreateBodyDescriptor instead')
const WorkspaceCreateBody$json = {
  '1': 'WorkspaceCreateBody',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
    {
      '1': 'parameters',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkspaceParameters',
      '9': 0,
      '10': 'parameters',
      '17': true
    },
    {'1': 'workflow_alias', '3': 3, '4': 1, '5': 9, '10': 'workflowAlias'},
    {
      '1': 'toolkit',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ToolkitPolicy',
      '9': 1,
      '10': 'toolkit',
      '17': true
    },
    {'1': 'collection', '3': 5, '4': 1, '5': 9, '10': 'collection'},
  ],
  '8': [
    {'1': '_parameters'},
    {'1': '_toolkit'},
  ],
};

/// Descriptor for `WorkspaceCreateBody`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceCreateBodyDescriptor = $convert.base64Decode(
    'ChNXb3Jrc3BhY2VDcmVhdGVCb2R5EhIKBG5hbWUYASABKAlSBG5hbWUSSAoKcGFyYW1ldGVycx'
    'gCIAEoCzIjLmdpemNsYXcucnBjLnYxLldvcmtzcGFjZVBhcmFtZXRlcnNIAFIKcGFyYW1ldGVy'
    'c4gBARIlCg53b3JrZmxvd19hbGlhcxgDIAEoCVINd29ya2Zsb3dBbGlhcxI8Cgd0b29sa2l0GA'
    'QgASgLMh0uZ2l6Y2xhdy5ycGMudjEuVG9vbGtpdFBvbGljeUgBUgd0b29sa2l0iAEBEh4KCmNv'
    'bGxlY3Rpb24YBSABKAlSCmNvbGxlY3Rpb25CDQoLX3BhcmFtZXRlcnNCCgoIX3Rvb2xraXQ=');

@$core.Deprecated('Use workspacePutBodyDescriptor instead')
const WorkspacePutBody$json = {
  '1': 'WorkspacePutBody',
  '2': [
    {
      '1': 'parameters',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkspaceParameters',
      '9': 0,
      '10': 'parameters',
      '17': true
    },
    {
      '1': 'toolkit',
      '3': 7,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ToolkitPolicy',
      '9': 1,
      '10': 'toolkit',
      '17': true
    },
  ],
  '8': [
    {'1': '_parameters'},
    {'1': '_toolkit'},
  ],
};

/// Descriptor for `WorkspacePutBody`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspacePutBodyDescriptor = $convert.base64Decode(
    'ChBXb3Jrc3BhY2VQdXRCb2R5EkgKCnBhcmFtZXRlcnMYBCABKAsyIy5naXpjbGF3LnJwYy52MS'
    '5Xb3Jrc3BhY2VQYXJhbWV0ZXJzSABSCnBhcmFtZXRlcnOIAQESPAoHdG9vbGtpdBgHIAEoCzId'
    'LmdpemNsYXcucnBjLnYxLlRvb2xraXRQb2xpY3lIAVIHdG9vbGtpdIgBAUINCgtfcGFyYW1ldG'
    'Vyc0IKCghfdG9vbGtpdA==');

@$core.Deprecated('Use workspaceIconDownloadRequestDescriptor instead')
const WorkspaceIconDownloadRequest$json = {
  '1': 'WorkspaceIconDownloadRequest',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
    {
      '1': 'format',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.IconFormat',
      '10': 'format'
    },
  ],
};

/// Descriptor for `WorkspaceIconDownloadRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceIconDownloadRequestDescriptor =
    $convert.base64Decode(
        'ChxXb3Jrc3BhY2VJY29uRG93bmxvYWRSZXF1ZXN0EhIKBG5hbWUYASABKAlSBG5hbWUSMgoGZm'
        '9ybWF0GAIgASgOMhouZ2l6Y2xhdy5ycGMudjEuSWNvbkZvcm1hdFIGZm9ybWF0');

@$core.Deprecated('Use workspaceIconDownloadResponseDescriptor instead')
const WorkspaceIconDownloadResponse$json = {
  '1': 'WorkspaceIconDownloadResponse',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
    {
      '1': 'format',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.IconFormat',
      '10': 'format'
    },
    {'1': 'size_bytes', '3': 3, '4': 1, '5': 3, '10': 'sizeBytes'},
  ],
};

/// Descriptor for `WorkspaceIconDownloadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceIconDownloadResponseDescriptor =
    $convert.base64Decode(
        'Ch1Xb3Jrc3BhY2VJY29uRG93bmxvYWRSZXNwb25zZRISCgRuYW1lGAEgASgJUgRuYW1lEjIKBm'
        'Zvcm1hdBgCIAEoDjIaLmdpemNsYXcucnBjLnYxLkljb25Gb3JtYXRSBmZvcm1hdBIdCgpzaXpl'
        'X2J5dGVzGAMgASgDUglzaXplQnl0ZXM=');

@$core.Deprecated('Use workspaceCreateRequestDescriptor instead')
const WorkspaceCreateRequest$json = {
  '1': 'WorkspaceCreateRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkspaceCreateBody',
      '10': 'value'
    },
  ],
};

/// Descriptor for `WorkspaceCreateRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceCreateRequestDescriptor =
    $convert.base64Decode(
        'ChZXb3Jrc3BhY2VDcmVhdGVSZXF1ZXN0EjkKBXZhbHVlGAEgASgLMiMuZ2l6Y2xhdy5ycGMudj'
        'EuV29ya3NwYWNlQ3JlYXRlQm9keVIFdmFsdWU=');

@$core.Deprecated('Use workspaceCreateResponseDescriptor instead')
const WorkspaceCreateResponse$json = {
  '1': 'WorkspaceCreateResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Workspace',
      '10': 'value'
    },
  ],
};

/// Descriptor for `WorkspaceCreateResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceCreateResponseDescriptor =
    $convert.base64Decode(
        'ChdXb3Jrc3BhY2VDcmVhdGVSZXNwb25zZRIvCgV2YWx1ZRgBIAEoCzIZLmdpemNsYXcucnBjLn'
        'YxLldvcmtzcGFjZVIFdmFsdWU=');

@$core.Deprecated('Use workspaceDeleteRequestDescriptor instead')
const WorkspaceDeleteRequest$json = {
  '1': 'WorkspaceDeleteRequest',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
  ],
};

/// Descriptor for `WorkspaceDeleteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceDeleteRequestDescriptor =
    $convert.base64Decode(
        'ChZXb3Jrc3BhY2VEZWxldGVSZXF1ZXN0EhIKBG5hbWUYASABKAlSBG5hbWU=');

@$core.Deprecated('Use workspaceDeleteResponseDescriptor instead')
const WorkspaceDeleteResponse$json = {
  '1': 'WorkspaceDeleteResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Workspace',
      '10': 'value'
    },
  ],
};

/// Descriptor for `WorkspaceDeleteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceDeleteResponseDescriptor =
    $convert.base64Decode(
        'ChdXb3Jrc3BhY2VEZWxldGVSZXNwb25zZRIvCgV2YWx1ZRgBIAEoCzIZLmdpemNsYXcucnBjLn'
        'YxLldvcmtzcGFjZVIFdmFsdWU=');

@$core.Deprecated('Use workspaceGetRequestDescriptor instead')
const WorkspaceGetRequest$json = {
  '1': 'WorkspaceGetRequest',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
  ],
};

/// Descriptor for `WorkspaceGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceGetRequestDescriptor = $convert
    .base64Decode('ChNXb3Jrc3BhY2VHZXRSZXF1ZXN0EhIKBG5hbWUYASABKAlSBG5hbWU=');

@$core.Deprecated('Use workspaceGetResponseDescriptor instead')
const WorkspaceGetResponse$json = {
  '1': 'WorkspaceGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Workspace',
      '10': 'value'
    },
    {
      '1': 'runtime_profile_name',
      '3': 2,
      '4': 1,
      '5': 9,
      '10': 'runtimeProfileName'
    },
    {
      '1': 'runtime_profile_revision',
      '3': 3,
      '4': 1,
      '5': 9,
      '10': 'runtimeProfileRevision'
    },
  ],
};

/// Descriptor for `WorkspaceGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceGetResponseDescriptor = $convert.base64Decode(
    'ChRXb3Jrc3BhY2VHZXRSZXNwb25zZRIvCgV2YWx1ZRgBIAEoCzIZLmdpemNsYXcucnBjLnYxLl'
    'dvcmtzcGFjZVIFdmFsdWUSMAoUcnVudGltZV9wcm9maWxlX25hbWUYAiABKAlSEnJ1bnRpbWVQ'
    'cm9maWxlTmFtZRI4ChhydW50aW1lX3Byb2ZpbGVfcmV2aXNpb24YAyABKAlSFnJ1bnRpbWVQcm'
    '9maWxlUmV2aXNpb24=');

@$core.Deprecated('Use workspaceHistoryAudioGetRequestDescriptor instead')
const WorkspaceHistoryAudioGetRequest$json = {
  '1': 'WorkspaceHistoryAudioGetRequest',
  '2': [
    {'1': 'history_id', '3': 1, '4': 1, '5': 9, '10': 'historyId'},
    {'1': 'workspace_name', '3': 2, '4': 1, '5': 9, '10': 'workspaceName'},
  ],
};

/// Descriptor for `WorkspaceHistoryAudioGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceHistoryAudioGetRequestDescriptor =
    $convert.base64Decode(
        'Ch9Xb3Jrc3BhY2VIaXN0b3J5QXVkaW9HZXRSZXF1ZXN0Eh0KCmhpc3RvcnlfaWQYASABKAlSCW'
        'hpc3RvcnlJZBIlCg53b3Jrc3BhY2VfbmFtZRgCIAEoCVINd29ya3NwYWNlTmFtZQ==');

@$core.Deprecated('Use workspaceHistoryAudioGetResponseDescriptor instead')
const WorkspaceHistoryAudioGetResponse$json = {
  '1': 'WorkspaceHistoryAudioGetResponse',
  '2': [
    {'1': 'history_id', '3': 1, '4': 1, '5': 9, '10': 'historyId'},
    {'1': 'mime_type', '3': 2, '4': 1, '5': 9, '10': 'mimeType'},
    {'1': 'size_bytes', '3': 3, '4': 1, '5': 3, '10': 'sizeBytes'},
    {'1': 'workspace_name', '3': 4, '4': 1, '5': 9, '10': 'workspaceName'},
  ],
};

/// Descriptor for `WorkspaceHistoryAudioGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceHistoryAudioGetResponseDescriptor =
    $convert.base64Decode(
        'CiBXb3Jrc3BhY2VIaXN0b3J5QXVkaW9HZXRSZXNwb25zZRIdCgpoaXN0b3J5X2lkGAEgASgJUg'
        'loaXN0b3J5SWQSGwoJbWltZV90eXBlGAIgASgJUghtaW1lVHlwZRIdCgpzaXplX2J5dGVzGAMg'
        'ASgDUglzaXplQnl0ZXMSJQoOd29ya3NwYWNlX25hbWUYBCABKAlSDXdvcmtzcGFjZU5hbWU=');

@$core.Deprecated('Use workspaceHistoryGetRequestDescriptor instead')
const WorkspaceHistoryGetRequest$json = {
  '1': 'WorkspaceHistoryGetRequest',
  '2': [
    {'1': 'history_id', '3': 1, '4': 1, '5': 9, '10': 'historyId'},
    {'1': 'workspace_name', '3': 2, '4': 1, '5': 9, '10': 'workspaceName'},
  ],
};

/// Descriptor for `WorkspaceHistoryGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceHistoryGetRequestDescriptor =
    $convert.base64Decode(
        'ChpXb3Jrc3BhY2VIaXN0b3J5R2V0UmVxdWVzdBIdCgpoaXN0b3J5X2lkGAEgASgJUgloaXN0b3'
        'J5SWQSJQoOd29ya3NwYWNlX25hbWUYAiABKAlSDXdvcmtzcGFjZU5hbWU=');

@$core.Deprecated('Use workspaceHistoryGetResponseDescriptor instead')
const WorkspaceHistoryGetResponse$json = {
  '1': 'WorkspaceHistoryGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunHistoryEntry',
      '10': 'value'
    },
  ],
};

/// Descriptor for `WorkspaceHistoryGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceHistoryGetResponseDescriptor =
    $convert.base64Decode(
        'ChtXb3Jrc3BhY2VIaXN0b3J5R2V0UmVzcG9uc2USOQoFdmFsdWUYASABKAsyIy5naXpjbGF3Ln'
        'JwYy52MS5QZWVyUnVuSGlzdG9yeUVudHJ5UgV2YWx1ZQ==');

@$core.Deprecated('Use workspaceHistoryListRequestDescriptor instead')
const WorkspaceHistoryListRequest$json = {
  '1': 'WorkspaceHistoryListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
    {
      '1': 'order',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.WorkspaceHistoryListRequestOrder',
      '9': 2,
      '10': 'order',
      '17': true
    },
    {'1': 'workspace_name', '3': 4, '4': 1, '5': 9, '10': 'workspaceName'},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
    {'1': '_order'},
  ],
};

/// Descriptor for `WorkspaceHistoryListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceHistoryListRequestDescriptor = $convert.base64Decode(
    'ChtXb3Jrc3BhY2VIaXN0b3J5TGlzdFJlcXVlc3QSGwoGY3Vyc29yGAEgASgJSABSBmN1cnNvco'
    'gBARIZCgVsaW1pdBgCIAEoA0gBUgVsaW1pdIgBARJLCgVvcmRlchgDIAEoDjIwLmdpemNsYXcu'
    'cnBjLnYxLldvcmtzcGFjZUhpc3RvcnlMaXN0UmVxdWVzdE9yZGVySAJSBW9yZGVyiAEBEiUKDn'
    'dvcmtzcGFjZV9uYW1lGAQgASgJUg13b3Jrc3BhY2VOYW1lQgkKB19jdXJzb3JCCAoGX2xpbWl0'
    'QggKBl9vcmRlcg==');

@$core.Deprecated('Use workspaceHistoryListResponseDescriptor instead')
const WorkspaceHistoryListResponse$json = {
  '1': 'WorkspaceHistoryListResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerRunHistoryListResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `WorkspaceHistoryListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceHistoryListResponseDescriptor =
    $convert.base64Decode(
        'ChxXb3Jrc3BhY2VIaXN0b3J5TGlzdFJlc3BvbnNlEkAKBXZhbHVlGAEgASgLMiouZ2l6Y2xhdy'
        '5ycGMudjEuUGVlclJ1bkhpc3RvcnlMaXN0UmVzcG9uc2VSBXZhbHVl');

@$core.Deprecated('Use workspaceListRequestDescriptor instead')
const WorkspaceListRequest$json = {
  '1': 'WorkspaceListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
    {'1': 'prefix', '3': 3, '4': 1, '5': 9, '9': 2, '10': 'prefix', '17': true},
    {'1': 'collection', '3': 4, '4': 1, '5': 9, '10': 'collection'},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
    {'1': '_prefix'},
  ],
};

/// Descriptor for `WorkspaceListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceListRequestDescriptor = $convert.base64Decode(
    'ChRXb3Jrc3BhY2VMaXN0UmVxdWVzdBIbCgZjdXJzb3IYASABKAlIAFIGY3Vyc29yiAEBEhkKBW'
    'xpbWl0GAIgASgDSAFSBWxpbWl0iAEBEhsKBnByZWZpeBgDIAEoCUgCUgZwcmVmaXiIAQESHgoK'
    'Y29sbGVjdGlvbhgEIAEoCVIKY29sbGVjdGlvbkIJCgdfY3Vyc29yQggKBl9saW1pdEIJCgdfcH'
    'JlZml4');

@$core.Deprecated('Use workspaceListResponseDescriptor instead')
const WorkspaceListResponse$json = {
  '1': 'WorkspaceListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Workspace',
      '10': 'items'
    },
    {
      '1': 'next_cursor',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'nextCursor',
      '17': true
    },
    {
      '1': 'runtime_profile_name',
      '3': 4,
      '4': 1,
      '5': 9,
      '10': 'runtimeProfileName'
    },
    {
      '1': 'runtime_profile_revision',
      '3': 5,
      '4': 1,
      '5': 9,
      '10': 'runtimeProfileRevision'
    },
  ],
  '8': [
    {'1': '_next_cursor'},
  ],
};

/// Descriptor for `WorkspaceListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceListResponseDescriptor = $convert.base64Decode(
    'ChVXb3Jrc3BhY2VMaXN0UmVzcG9uc2USGQoIaGFzX25leHQYASABKAhSB2hhc05leHQSLwoFaX'
    'RlbXMYAiADKAsyGS5naXpjbGF3LnJwYy52MS5Xb3Jrc3BhY2VSBWl0ZW1zEiQKC25leHRfY3Vy'
    'c29yGAMgASgJSABSCm5leHRDdXJzb3KIAQESMAoUcnVudGltZV9wcm9maWxlX25hbWUYBCABKA'
    'lSEnJ1bnRpbWVQcm9maWxlTmFtZRI4ChhydW50aW1lX3Byb2ZpbGVfcmV2aXNpb24YBSABKAlS'
    'FnJ1bnRpbWVQcm9maWxlUmV2aXNpb25CDgoMX25leHRfY3Vyc29y');

@$core.Deprecated('Use workspaceParametersDescriptor instead')
const WorkspaceParameters$json = {
  '1': 'WorkspaceParameters',
  '2': [
    {
      '1': 'flowcraft_workspace_parameters',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FlowcraftWorkspaceParameters',
      '9': 0,
      '10': 'flowcraftWorkspaceParameters'
    },
    {
      '1': 'doubao_realtime_workspace_parameters',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeWorkspaceParameters',
      '9': 0,
      '10': 'doubaoRealtimeWorkspaceParameters'
    },
    {
      '1': 'asttranslate_workspace_parameters',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ASTTranslateWorkspaceParameters',
      '9': 0,
      '10': 'asttranslateWorkspaceParameters'
    },
    {
      '1': 'chat_room_workspace_parameters',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ChatRoomWorkspaceParameters',
      '9': 0,
      '10': 'chatRoomWorkspaceParameters'
    },
    {
      '1': 'pet_workspace_parameters',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetWorkspaceParameters',
      '9': 0,
      '10': 'petWorkspaceParameters'
    },
  ],
  '8': [
    {'1': 'value'},
  ],
};

/// Descriptor for `WorkspaceParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspaceParametersDescriptor = $convert.base64Decode(
    'ChNXb3Jrc3BhY2VQYXJhbWV0ZXJzEnQKHmZsb3djcmFmdF93b3Jrc3BhY2VfcGFyYW1ldGVycx'
    'gBIAEoCzIsLmdpemNsYXcucnBjLnYxLkZsb3djcmFmdFdvcmtzcGFjZVBhcmFtZXRlcnNIAFIc'
    'Zmxvd2NyYWZ0V29ya3NwYWNlUGFyYW1ldGVycxKEAQokZG91YmFvX3JlYWx0aW1lX3dvcmtzcG'
    'FjZV9wYXJhbWV0ZXJzGAIgASgLMjEuZ2l6Y2xhdy5ycGMudjEuRG91YmFvUmVhbHRpbWVXb3Jr'
    'c3BhY2VQYXJhbWV0ZXJzSABSIWRvdWJhb1JlYWx0aW1lV29ya3NwYWNlUGFyYW1ldGVycxJ9Ci'
    'Fhc3R0cmFuc2xhdGVfd29ya3NwYWNlX3BhcmFtZXRlcnMYAyABKAsyLy5naXpjbGF3LnJwYy52'
    'MS5BU1RUcmFuc2xhdGVXb3Jrc3BhY2VQYXJhbWV0ZXJzSABSH2FzdHRyYW5zbGF0ZVdvcmtzcG'
    'FjZVBhcmFtZXRlcnMScgoeY2hhdF9yb29tX3dvcmtzcGFjZV9wYXJhbWV0ZXJzGAQgASgLMisu'
    'Z2l6Y2xhdy5ycGMudjEuQ2hhdFJvb21Xb3Jrc3BhY2VQYXJhbWV0ZXJzSABSG2NoYXRSb29tV2'
    '9ya3NwYWNlUGFyYW1ldGVycxJiChhwZXRfd29ya3NwYWNlX3BhcmFtZXRlcnMYBSABKAsyJi5n'
    'aXpjbGF3LnJwYy52MS5QZXRXb3Jrc3BhY2VQYXJhbWV0ZXJzSABSFnBldFdvcmtzcGFjZVBhcm'
    'FtZXRlcnNCBwoFdmFsdWU=');

@$core.Deprecated('Use workspacePutRequestDescriptor instead')
const WorkspacePutRequest$json = {
  '1': 'WorkspacePutRequest',
  '2': [
    {
      '1': 'body',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkspacePutBody',
      '10': 'body'
    },
    {'1': 'name', '3': 2, '4': 1, '5': 9, '10': 'name'},
  ],
};

/// Descriptor for `WorkspacePutRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspacePutRequestDescriptor = $convert.base64Decode(
    'ChNXb3Jrc3BhY2VQdXRSZXF1ZXN0EjQKBGJvZHkYASABKAsyIC5naXpjbGF3LnJwYy52MS5Xb3'
    'Jrc3BhY2VQdXRCb2R5UgRib2R5EhIKBG5hbWUYAiABKAlSBG5hbWU=');

@$core.Deprecated('Use workspacePutResponseDescriptor instead')
const WorkspacePutResponse$json = {
  '1': 'WorkspacePutResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Workspace',
      '10': 'value'
    },
  ],
};

/// Descriptor for `WorkspacePutResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workspacePutResponseDescriptor = $convert.base64Decode(
    'ChRXb3Jrc3BhY2VQdXRSZXNwb25zZRIvCgV2YWx1ZRgBIAEoCzIZLmdpemNsYXcucnBjLnYxLl'
    'dvcmtzcGFjZVIFdmFsdWU=');
