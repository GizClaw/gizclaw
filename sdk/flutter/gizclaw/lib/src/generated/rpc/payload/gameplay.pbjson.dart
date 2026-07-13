// This is a generated file - do not edit.
//
// Generated from payload/gameplay.proto.

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

@$core.Deprecated('Use badgeDescriptor instead')
const Badge$json = {
  '1': 'Badge',
  '2': [
    {'1': 'active', '3': 1, '4': 1, '5': 8, '10': 'active'},
    {'1': 'badge_def_id', '3': 2, '4': 1, '5': 9, '10': 'badgeDefId'},
    {'1': 'created_at', '3': 3, '4': 1, '5': 9, '10': 'createdAt'},
    {'1': 'exp', '3': 4, '4': 1, '5': 3, '10': 'exp'},
    {'1': 'id', '3': 5, '4': 1, '5': 9, '10': 'id'},
    {'1': 'level', '3': 6, '4': 1, '5': 3, '10': 'level'},
    {'1': 'owner_public_key', '3': 7, '4': 1, '5': 9, '10': 'ownerPublicKey'},
    {'1': 'progress', '3': 8, '4': 1, '5': 3, '10': 'progress'},
    {'1': 'updated_at', '3': 9, '4': 1, '5': 9, '10': 'updatedAt'},
  ],
};

/// Descriptor for `Badge`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List badgeDescriptor = $convert.base64Decode(
    'CgVCYWRnZRIWCgZhY3RpdmUYASABKAhSBmFjdGl2ZRIgCgxiYWRnZV9kZWZfaWQYAiABKAlSCm'
    'JhZGdlRGVmSWQSHQoKY3JlYXRlZF9hdBgDIAEoCVIJY3JlYXRlZEF0EhAKA2V4cBgEIAEoA1ID'
    'ZXhwEg4KAmlkGAUgASgJUgJpZBIUCgVsZXZlbBgGIAEoA1IFbGV2ZWwSKAoQb3duZXJfcHVibG'
    'ljX2tleRgHIAEoCVIOb3duZXJQdWJsaWNLZXkSGgoIcHJvZ3Jlc3MYCCABKANSCHByb2dyZXNz'
    'Eh0KCnVwZGF0ZWRfYXQYCSABKAlSCXVwZGF0ZWRBdA==');

@$core.Deprecated('Use badgeDefPixaDownloadRequestDescriptor instead')
const BadgeDefPixaDownloadRequest$json = {
  '1': 'BadgeDefPixaDownloadRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `BadgeDefPixaDownloadRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List badgeDefPixaDownloadRequestDescriptor =
    $convert.base64Decode(
        'ChtCYWRnZURlZlBpeGFEb3dubG9hZFJlcXVlc3QSDgoCaWQYASABKAlSAmlk');

@$core.Deprecated('Use badgeDefPixaDownloadResponseDescriptor instead')
const BadgeDefPixaDownloadResponse$json = {
  '1': 'BadgeDefPixaDownloadResponse',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'pixa_path',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'pixaPath',
      '17': true
    },
    {'1': 'size_bytes', '3': 3, '4': 1, '5': 3, '10': 'sizeBytes'},
  ],
  '8': [
    {'1': '_pixa_path'},
  ],
};

/// Descriptor for `BadgeDefPixaDownloadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List badgeDefPixaDownloadResponseDescriptor =
    $convert.base64Decode(
        'ChxCYWRnZURlZlBpeGFEb3dubG9hZFJlc3BvbnNlEg4KAmlkGAEgASgJUgJpZBIgCglwaXhhX3'
        'BhdGgYAiABKAlIAFIIcGl4YVBhdGiIAQESHQoKc2l6ZV9ieXRlcxgDIAEoA1IJc2l6ZUJ5dGVz'
        'QgwKCl9waXhhX3BhdGg=');

@$core.Deprecated('Use badgeListResponseDescriptor instead')
const BadgeListResponse$json = {
  '1': 'BadgeListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Badge',
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
  ],
  '8': [
    {'1': '_next_cursor'},
  ],
};

/// Descriptor for `BadgeListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List badgeListResponseDescriptor = $convert.base64Decode(
    'ChFCYWRnZUxpc3RSZXNwb25zZRIZCghoYXNfbmV4dBgBIAEoCFIHaGFzTmV4dBIrCgVpdGVtcx'
    'gCIAMoCzIVLmdpemNsYXcucnBjLnYxLkJhZGdlUgVpdGVtcxIkCgtuZXh0X2N1cnNvchgDIAEo'
    'CUgAUgpuZXh0Q3Vyc29yiAEBQg4KDF9uZXh0X2N1cnNvcg==');

@$core.Deprecated('Use gameResultDescriptor instead')
const GameResult$json = {
  '1': 'GameResult',
  '2': [
    {'1': 'created_at', '3': 1, '4': 1, '5': 9, '10': 'createdAt'},
    {
      '1': 'difficulty',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'difficulty',
      '17': true
    },
    {
      '1': 'duration_ms',
      '3': 3,
      '4': 1,
      '5': 3,
      '9': 1,
      '10': 'durationMs',
      '17': true
    },
    {'1': 'game_def_id', '3': 4, '4': 1, '5': 9, '10': 'gameDefId'},
    {'1': 'id', '3': 5, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'idempotency_key',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'idempotencyKey',
      '17': true
    },
    {
      '1': 'max_score',
      '3': 7,
      '4': 1,
      '5': 3,
      '9': 3,
      '10': 'maxScore',
      '17': true
    },
    {'1': 'occurred_at', '3': 8, '4': 1, '5': 9, '10': 'occurredAt'},
    {
      '1': 'outcome',
      '3': 9,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'outcome',
      '17': true
    },
    {'1': 'owner_public_key', '3': 10, '4': 1, '5': 9, '10': 'ownerPublicKey'},
    {
      '1': 'payload',
      '3': 11,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameplayMetadata',
      '9': 5,
      '10': 'payload',
      '17': true
    },
    {'1': 'pet_id', '3': 12, '4': 1, '5': 9, '10': 'petId'},
    {'1': 'ruleset_name', '3': 13, '4': 1, '5': 9, '10': 'rulesetName'},
    {'1': 'score', '3': 14, '4': 1, '5': 3, '9': 6, '10': 'score', '17': true},
  ],
  '8': [
    {'1': '_difficulty'},
    {'1': '_duration_ms'},
    {'1': '_idempotency_key'},
    {'1': '_max_score'},
    {'1': '_outcome'},
    {'1': '_payload'},
    {'1': '_score'},
  ],
};

/// Descriptor for `GameResult`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List gameResultDescriptor = $convert.base64Decode(
    'CgpHYW1lUmVzdWx0Eh0KCmNyZWF0ZWRfYXQYASABKAlSCWNyZWF0ZWRBdBIjCgpkaWZmaWN1bH'
    'R5GAIgASgJSABSCmRpZmZpY3VsdHmIAQESJAoLZHVyYXRpb25fbXMYAyABKANIAVIKZHVyYXRp'
    'b25Nc4gBARIeCgtnYW1lX2RlZl9pZBgEIAEoCVIJZ2FtZURlZklkEg4KAmlkGAUgASgJUgJpZB'
    'IsCg9pZGVtcG90ZW5jeV9rZXkYBiABKAlIAlIOaWRlbXBvdGVuY3lLZXmIAQESIAoJbWF4X3Nj'
    'b3JlGAcgASgDSANSCG1heFNjb3JliAEBEh8KC29jY3VycmVkX2F0GAggASgJUgpvY2N1cnJlZE'
    'F0Eh0KB291dGNvbWUYCSABKAlIBFIHb3V0Y29tZYgBARIoChBvd25lcl9wdWJsaWNfa2V5GAog'
    'ASgJUg5vd25lclB1YmxpY0tleRI/CgdwYXlsb2FkGAsgASgLMiAuZ2l6Y2xhdy5ycGMudjEuR2'
    'FtZXBsYXlNZXRhZGF0YUgFUgdwYXlsb2FkiAEBEhUKBnBldF9pZBgMIAEoCVIFcGV0SWQSIQoM'
    'cnVsZXNldF9uYW1lGA0gASgJUgtydWxlc2V0TmFtZRIZCgVzY29yZRgOIAEoA0gGUgVzY29yZY'
    'gBAUINCgtfZGlmZmljdWx0eUIOCgxfZHVyYXRpb25fbXNCEgoQX2lkZW1wb3RlbmN5X2tleUIM'
    'CgpfbWF4X3Njb3JlQgoKCF9vdXRjb21lQgoKCF9wYXlsb2FkQggKBl9zY29yZQ==');

@$core.Deprecated('Use gameResultListResponseDescriptor instead')
const GameResultListResponse$json = {
  '1': 'GameResultListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameResult',
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
  ],
  '8': [
    {'1': '_next_cursor'},
  ],
};

/// Descriptor for `GameResultListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List gameResultListResponseDescriptor = $convert.base64Decode(
    'ChZHYW1lUmVzdWx0TGlzdFJlc3BvbnNlEhkKCGhhc19uZXh0GAEgASgIUgdoYXNOZXh0EjAKBW'
    'l0ZW1zGAIgAygLMhouZ2l6Y2xhdy5ycGMudjEuR2FtZVJlc3VsdFIFaXRlbXMSJAoLbmV4dF9j'
    'dXJzb3IYAyABKAlIAFIKbmV4dEN1cnNvcogBAUIOCgxfbmV4dF9jdXJzb3I=');

@$core.Deprecated('Use gameRewardSpecDescriptor instead')
const GameRewardSpec$json = {
  '1': 'GameRewardSpec',
  '2': [
    {
      '1': 'badge_exp_delta',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameRewardSpec.BadgeExpDeltaEntry',
      '10': 'badgeExpDelta'
    },
    {
      '1': 'pet_exp_delta',
      '3': 4,
      '4': 1,
      '5': 3,
      '9': 0,
      '10': 'petExpDelta',
      '17': true
    },
    {
      '1': 'points_delta',
      '3': 5,
      '4': 1,
      '5': 3,
      '9': 1,
      '10': 'pointsDelta',
      '17': true
    },
  ],
  '3': [GameRewardSpec_BadgeExpDeltaEntry$json],
  '8': [
    {'1': '_pet_exp_delta'},
    {'1': '_points_delta'},
  ],
  '9': [
    {'1': 1, '2': 2},
    {'1': 3, '2': 4},
  ],
  '10': ['ability_delta', 'life_delta'],
};

@$core.Deprecated('Use gameRewardSpecDescriptor instead')
const GameRewardSpec_BadgeExpDeltaEntry$json = {
  '1': 'BadgeExpDeltaEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 3, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `GameRewardSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List gameRewardSpecDescriptor = $convert.base64Decode(
    'Cg5HYW1lUmV3YXJkU3BlYxJZCg9iYWRnZV9leHBfZGVsdGEYAiADKAsyMS5naXpjbGF3LnJwYy'
    '52MS5HYW1lUmV3YXJkU3BlYy5CYWRnZUV4cERlbHRhRW50cnlSDWJhZGdlRXhwRGVsdGESJwoN'
    'cGV0X2V4cF9kZWx0YRgEIAEoA0gAUgtwZXRFeHBEZWx0YYgBARImCgxwb2ludHNfZGVsdGEYBS'
    'ABKANIAVILcG9pbnRzRGVsdGGIAQEaQAoSQmFkZ2VFeHBEZWx0YUVudHJ5EhAKA2tleRgBIAEo'
    'CVIDa2V5EhQKBXZhbHVlGAIgASgDUgV2YWx1ZToCOAFCEAoOX3BldF9leHBfZGVsdGFCDwoNX3'
    'BvaW50c19kZWx0YUoECAEQAkoECAMQBFINYWJpbGl0eV9kZWx0YVIKbGlmZV9kZWx0YQ==');

@$core.Deprecated('Use gameRulesetDescriptor instead')
const GameRuleset$json = {
  '1': 'GameRuleset',
  '2': [
    {'1': 'created_at', '3': 1, '4': 1, '5': 9, '10': 'createdAt'},
    {'1': 'name', '3': 2, '4': 1, '5': 9, '10': 'name'},
    {
      '1': 'spec',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameRulesetSpec',
      '10': 'spec'
    },
    {'1': 'updated_at', '3': 4, '4': 1, '5': 9, '10': 'updatedAt'},
  ],
};

/// Descriptor for `GameRuleset`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List gameRulesetDescriptor = $convert.base64Decode(
    'CgtHYW1lUnVsZXNldBIdCgpjcmVhdGVkX2F0GAEgASgJUgljcmVhdGVkQXQSEgoEbmFtZRgCIA'
    'EoCVIEbmFtZRIzCgRzcGVjGAMgASgLMh8uZ2l6Y2xhdy5ycGMudjEuR2FtZVJ1bGVzZXRTcGVj'
    'UgRzcGVjEh0KCnVwZGF0ZWRfYXQYBCABKAlSCXVwZGF0ZWRBdA==');

@$core.Deprecated('Use gameRulesetDriveSpecDescriptor instead')
const GameRulesetDriveSpec$json = {
  '1': 'GameRulesetDriveSpec',
  '2': [
    {
      '1': 'default_reward',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameRewardSpec',
      '9': 0,
      '10': 'defaultReward',
      '17': true
    },
    {
      '1': 'game_rewards',
      '3': 4,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameRulesetDriveSpec.GameRewardsEntry',
      '10': 'gameRewards'
    },
  ],
  '3': [GameRulesetDriveSpec_GameRewardsEntry$json],
  '8': [
    {'1': '_default_reward'},
  ],
  '9': [
    {'1': 1, '2': 2},
    {'1': 2, '2': 3},
    {'1': 5, '2': 6},
  ],
  '10': ['action_costs', 'action_rewards', 'life_decay_per_hour'],
};

@$core.Deprecated('Use gameRulesetDriveSpecDescriptor instead')
const GameRulesetDriveSpec_GameRewardsEntry$json = {
  '1': 'GameRewardsEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {
      '1': 'value',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameRewardSpec',
      '10': 'value'
    },
  ],
  '7': {'7': true},
};

/// Descriptor for `GameRulesetDriveSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List gameRulesetDriveSpecDescriptor = $convert.base64Decode(
    'ChRHYW1lUnVsZXNldERyaXZlU3BlYxJKCg5kZWZhdWx0X3Jld2FyZBgDIAEoCzIeLmdpemNsYX'
    'cucnBjLnYxLkdhbWVSZXdhcmRTcGVjSABSDWRlZmF1bHRSZXdhcmSIAQESWAoMZ2FtZV9yZXdh'
    'cmRzGAQgAygLMjUuZ2l6Y2xhdy5ycGMudjEuR2FtZVJ1bGVzZXREcml2ZVNwZWMuR2FtZVJld2'
    'FyZHNFbnRyeVILZ2FtZVJld2FyZHMaXgoQR2FtZVJld2FyZHNFbnRyeRIQCgNrZXkYASABKAlS'
    'A2tleRI0CgV2YWx1ZRgCIAEoCzIeLmdpemNsYXcucnBjLnYxLkdhbWVSZXdhcmRTcGVjUgV2YW'
    'x1ZToCOAFCEQoPX2RlZmF1bHRfcmV3YXJkSgQIARACSgQIAhADSgQIBRAGUgxhY3Rpb25fY29z'
    'dHNSDmFjdGlvbl9yZXdhcmRzUhNsaWZlX2RlY2F5X3Blcl9ob3Vy');

@$core.Deprecated('Use gameRulesetPetPoolEntryDescriptor instead')
const GameRulesetPetPoolEntry$json = {
  '1': 'GameRulesetPetPoolEntry',
  '2': [
    {
      '1': 'adoption_cost',
      '3': 1,
      '4': 1,
      '5': 3,
      '9': 0,
      '10': 'adoptionCost',
      '17': true
    },
    {'1': 'petdef_id', '3': 2, '4': 1, '5': 9, '10': 'petdefId'},
    {'1': 'rarity', '3': 3, '4': 1, '5': 9, '9': 1, '10': 'rarity', '17': true},
    {'1': 'weight', '3': 4, '4': 1, '5': 3, '10': 'weight'},
    {
      '1': 'workflow_name',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'workflowName',
      '17': true
    },
  ],
  '8': [
    {'1': '_adoption_cost'},
    {'1': '_rarity'},
    {'1': '_workflow_name'},
  ],
};

/// Descriptor for `GameRulesetPetPoolEntry`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List gameRulesetPetPoolEntryDescriptor = $convert.base64Decode(
    'ChdHYW1lUnVsZXNldFBldFBvb2xFbnRyeRIoCg1hZG9wdGlvbl9jb3N0GAEgASgDSABSDGFkb3'
    'B0aW9uQ29zdIgBARIbCglwZXRkZWZfaWQYAiABKAlSCHBldGRlZklkEhsKBnJhcml0eRgDIAEo'
    'CUgBUgZyYXJpdHmIAQESFgoGd2VpZ2h0GAQgASgDUgZ3ZWlnaHQSKAoNd29ya2Zsb3dfbmFtZR'
    'gFIAEoCUgCUgx3b3JrZmxvd05hbWWIAQFCEAoOX2Fkb3B0aW9uX2Nvc3RCCQoHX3Jhcml0eUIQ'
    'Cg5fd29ya2Zsb3dfbmFtZQ==');

@$core.Deprecated('Use gameRulesetPointsSpecDescriptor instead')
const GameRulesetPointsSpec$json = {
  '1': 'GameRulesetPointsSpec',
  '2': [
    {
      '1': 'initial_balance',
      '3': 1,
      '4': 1,
      '5': 3,
      '9': 0,
      '10': 'initialBalance',
      '17': true
    },
  ],
  '8': [
    {'1': '_initial_balance'},
  ],
};

/// Descriptor for `GameRulesetPointsSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List gameRulesetPointsSpecDescriptor = $convert.base64Decode(
    'ChVHYW1lUnVsZXNldFBvaW50c1NwZWMSLAoPaW5pdGlhbF9iYWxhbmNlGAEgASgDSABSDmluaX'
    'RpYWxCYWxhbmNliAEBQhIKEF9pbml0aWFsX2JhbGFuY2U=');

@$core.Deprecated('Use gameRulesetSpecDescriptor instead')
const GameRulesetSpec$json = {
  '1': 'GameRulesetSpec',
  '2': [
    {'1': 'badge_def_ids', '3': 1, '4': 3, '5': 9, '10': 'badgeDefIds'},
    {
      '1': 'default_workflow_name',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'defaultWorkflowName',
      '17': true
    },
    {
      '1': 'description',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'description',
      '17': true
    },
    {
      '1': 'drive',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameRulesetDriveSpec',
      '9': 2,
      '10': 'drive',
      '17': true
    },
    {'1': 'enabled', '3': 5, '4': 1, '5': 8, '10': 'enabled'},
    {'1': 'game_def_ids', '3': 6, '4': 3, '5': 9, '10': 'gameDefIds'},
    {
      '1': 'metadata',
      '3': 7,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameplayMetadata',
      '9': 3,
      '10': 'metadata',
      '17': true
    },
    {
      '1': 'pet_pool',
      '3': 8,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameRulesetPetPoolEntry',
      '10': 'petPool'
    },
    {
      '1': 'points',
      '3': 9,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameRulesetPointsSpec',
      '9': 4,
      '10': 'points',
      '17': true
    },
  ],
  '8': [
    {'1': '_default_workflow_name'},
    {'1': '_description'},
    {'1': '_drive'},
    {'1': '_metadata'},
    {'1': '_points'},
  ],
};

/// Descriptor for `GameRulesetSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List gameRulesetSpecDescriptor = $convert.base64Decode(
    'Cg9HYW1lUnVsZXNldFNwZWMSIgoNYmFkZ2VfZGVmX2lkcxgBIAMoCVILYmFkZ2VEZWZJZHMSNw'
    'oVZGVmYXVsdF93b3JrZmxvd19uYW1lGAIgASgJSABSE2RlZmF1bHRXb3JrZmxvd05hbWWIAQES'
    'JQoLZGVzY3JpcHRpb24YAyABKAlIAVILZGVzY3JpcHRpb26IAQESPwoFZHJpdmUYBCABKAsyJC'
    '5naXpjbGF3LnJwYy52MS5HYW1lUnVsZXNldERyaXZlU3BlY0gCUgVkcml2ZYgBARIYCgdlbmFi'
    'bGVkGAUgASgIUgdlbmFibGVkEiAKDGdhbWVfZGVmX2lkcxgGIAMoCVIKZ2FtZURlZklkcxJBCg'
    'htZXRhZGF0YRgHIAEoCzIgLmdpemNsYXcucnBjLnYxLkdhbWVwbGF5TWV0YWRhdGFIA1IIbWV0'
    'YWRhdGGIAQESQgoIcGV0X3Bvb2wYCCADKAsyJy5naXpjbGF3LnJwYy52MS5HYW1lUnVsZXNldF'
    'BldFBvb2xFbnRyeVIHcGV0UG9vbBJCCgZwb2ludHMYCSABKAsyJS5naXpjbGF3LnJwYy52MS5H'
    'YW1lUnVsZXNldFBvaW50c1NwZWNIBFIGcG9pbnRziAEBQhgKFl9kZWZhdWx0X3dvcmtmbG93X2'
    '5hbWVCDgoMX2Rlc2NyaXB0aW9uQggKBl9kcml2ZUILCglfbWV0YWRhdGFCCQoHX3BvaW50cw==');

@$core.Deprecated('Use gameplayGetRequestDescriptor instead')
const GameplayGetRequest$json = {
  '1': 'GameplayGetRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `GameplayGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List gameplayGetRequestDescriptor =
    $convert.base64Decode('ChJHYW1lcGxheUdldFJlcXVlc3QSDgoCaWQYASABKAlSAmlk');

@$core.Deprecated('Use gameplayListRequestDescriptor instead')
const GameplayListRequest$json = {
  '1': 'GameplayListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `GameplayListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List gameplayListRequestDescriptor = $convert.base64Decode(
    'ChNHYW1lcGxheUxpc3RSZXF1ZXN0EhsKBmN1cnNvchgBIAEoCUgAUgZjdXJzb3KIAQESGQoFbG'
    'ltaXQYAiABKANIAVIFbGltaXSIAQFCCQoHX2N1cnNvckIICgZfbGltaXQ=');

@$core.Deprecated('Use gameplayMetadataDescriptor instead')
const GameplayMetadata$json = {
  '1': 'GameplayMetadata',
  '2': [
    {
      '1': 'fields',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '10': 'fields'
    },
  ],
};

/// Descriptor for `GameplayMetadata`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List gameplayMetadataDescriptor = $convert.base64Decode(
    'ChBHYW1lcGxheU1ldGFkYXRhEi8KBmZpZWxkcxgBIAEoCzIXLmdvb2dsZS5wcm90b2J1Zi5TdH'
    'J1Y3RSBmZpZWxkcw==');

@$core.Deprecated('Use petDescriptor instead')
const Pet$json = {
  '1': 'Pet',
  '2': [
    {'1': 'created_at', '3': 2, '4': 1, '5': 9, '10': 'createdAt'},
    {'1': 'display_name', '3': 3, '4': 1, '5': 9, '10': 'displayName'},
    {'1': 'id', '3': 5, '4': 1, '5': 9, '10': 'id'},
    {'1': 'last_active_at', '3': 6, '4': 1, '5': 9, '10': 'lastActiveAt'},
    {
      '1': 'life',
      '3': 8,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetLife',
      '10': 'life'
    },
    {'1': 'owner_public_key', '3': 9, '4': 1, '5': 9, '10': 'ownerPublicKey'},
    {'1': 'petdef_id', '3': 10, '4': 1, '5': 9, '10': 'petdefId'},
    {'1': 'ruleset_name', '3': 11, '4': 1, '5': 9, '10': 'rulesetName'},
    {'1': 'updated_at', '3': 12, '4': 1, '5': 9, '10': 'updatedAt'},
    {
      '1': 'workflow_name',
      '3': 13,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'workflowName',
      '17': true
    },
    {'1': 'workspace_name', '3': 14, '4': 1, '5': 9, '10': 'workspaceName'},
    {
      '1': 'progression',
      '3': 15,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetProgression',
      '10': 'progression'
    },
  ],
  '8': [
    {'1': '_workflow_name'},
  ],
  '9': [
    {'1': 1, '2': 2},
    {'1': 4, '2': 5},
    {'1': 7, '2': 8},
  ],
  '10': ['ability', 'exp', 'level'],
};

/// Descriptor for `Pet`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petDescriptor = $convert.base64Decode(
    'CgNQZXQSHQoKY3JlYXRlZF9hdBgCIAEoCVIJY3JlYXRlZEF0EiEKDGRpc3BsYXlfbmFtZRgDIA'
    'EoCVILZGlzcGxheU5hbWUSDgoCaWQYBSABKAlSAmlkEiQKDmxhc3RfYWN0aXZlX2F0GAYgASgJ'
    'UgxsYXN0QWN0aXZlQXQSKwoEbGlmZRgIIAEoCzIXLmdpemNsYXcucnBjLnYxLlBldExpZmVSBG'
    'xpZmUSKAoQb3duZXJfcHVibGljX2tleRgJIAEoCVIOb3duZXJQdWJsaWNLZXkSGwoJcGV0ZGVm'
    'X2lkGAogASgJUghwZXRkZWZJZBIhCgxydWxlc2V0X25hbWUYCyABKAlSC3J1bGVzZXROYW1lEh'
    '0KCnVwZGF0ZWRfYXQYDCABKAlSCXVwZGF0ZWRBdBIoCg13b3JrZmxvd19uYW1lGA0gASgJSABS'
    'DHdvcmtmbG93TmFtZYgBARIlCg53b3Jrc3BhY2VfbmFtZRgOIAEoCVINd29ya3NwYWNlTmFtZR'
    'JACgtwcm9ncmVzc2lvbhgPIAEoCzIeLmdpemNsYXcucnBjLnYxLlBldFByb2dyZXNzaW9uUgtw'
    'cm9ncmVzc2lvbkIQCg5fd29ya2Zsb3dfbmFtZUoECAEQAkoECAQQBUoECAcQCFIHYWJpbGl0eV'
    'IDZXhwUgVsZXZlbA==');

@$core.Deprecated('Use petAdoptRequestDescriptor instead')
const PetAdoptRequest$json = {
  '1': 'PetAdoptRequest',
  '2': [
    {
      '1': 'display_name',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'displayName',
      '17': true
    },
    {
      '1': 'ruleset_name',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'rulesetName',
      '17': true
    },
  ],
  '8': [
    {'1': '_display_name'},
    {'1': '_ruleset_name'},
  ],
};

/// Descriptor for `PetAdoptRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petAdoptRequestDescriptor = $convert.base64Decode(
    'Cg9QZXRBZG9wdFJlcXVlc3QSJgoMZGlzcGxheV9uYW1lGAEgASgJSABSC2Rpc3BsYXlOYW1liA'
    'EBEiYKDHJ1bGVzZXRfbmFtZRgCIAEoCUgBUgtydWxlc2V0TmFtZYgBAUIPCg1fZGlzcGxheV9u'
    'YW1lQg8KDV9ydWxlc2V0X25hbWU=');

@$core.Deprecated('Use petAdoptResponseDescriptor instead')
const PetAdoptResponse$json = {
  '1': 'PetAdoptResponse',
  '2': [
    {
      '1': 'pet',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Pet',
      '10': 'pet'
    },
    {
      '1': 'points',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PointsAccount',
      '10': 'points'
    },
    {
      '1': 'transaction',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PointsTransaction',
      '10': 'transaction'
    },
  ],
};

/// Descriptor for `PetAdoptResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petAdoptResponseDescriptor = $convert.base64Decode(
    'ChBQZXRBZG9wdFJlc3BvbnNlEiUKA3BldBgBIAEoCzITLmdpemNsYXcucnBjLnYxLlBldFIDcG'
    'V0EjUKBnBvaW50cxgCIAEoCzIdLmdpemNsYXcucnBjLnYxLlBvaW50c0FjY291bnRSBnBvaW50'
    'cxJDCgt0cmFuc2FjdGlvbhgDIAEoCzIhLmdpemNsYXcucnBjLnYxLlBvaW50c1RyYW5zYWN0aW'
    '9uUgt0cmFuc2FjdGlvbg==');

@$core.Deprecated('Use petDefPixaDownloadRequestDescriptor instead')
const PetDefPixaDownloadRequest$json = {
  '1': 'PetDefPixaDownloadRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `PetDefPixaDownloadRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petDefPixaDownloadRequestDescriptor =
    $convert.base64Decode(
        'ChlQZXREZWZQaXhhRG93bmxvYWRSZXF1ZXN0Eg4KAmlkGAEgASgJUgJpZA==');

@$core.Deprecated('Use petDefPixaDownloadResponseDescriptor instead')
const PetDefPixaDownloadResponse$json = {
  '1': 'PetDefPixaDownloadResponse',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'pixa_path',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'pixaPath',
      '17': true
    },
    {'1': 'size_bytes', '3': 3, '4': 1, '5': 3, '10': 'sizeBytes'},
  ],
  '8': [
    {'1': '_pixa_path'},
  ],
};

/// Descriptor for `PetDefPixaDownloadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petDefPixaDownloadResponseDescriptor =
    $convert.base64Decode(
        'ChpQZXREZWZQaXhhRG93bmxvYWRSZXNwb25zZRIOCgJpZBgBIAEoCVICaWQSIAoJcGl4YV9wYX'
        'RoGAIgASgJSABSCHBpeGFQYXRoiAEBEh0KCnNpemVfYnl0ZXMYAyABKANSCXNpemVCeXRlc0IM'
        'CgpfcGl4YV9wYXRo');

@$core.Deprecated('Use petPixaDownloadRequestDescriptor instead')
const PetPixaDownloadRequest$json = {
  '1': 'PetPixaDownloadRequest',
  '2': [
    {'1': 'pet_id', '3': 1, '4': 1, '5': 9, '10': 'petId'},
  ],
};

/// Descriptor for `PetPixaDownloadRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPixaDownloadRequestDescriptor =
    $convert.base64Decode(
        'ChZQZXRQaXhhRG93bmxvYWRSZXF1ZXN0EhUKBnBldF9pZBgBIAEoCVIFcGV0SWQ=');

@$core.Deprecated('Use petPixaDownloadResponseDescriptor instead')
const PetPixaDownloadResponse$json = {
  '1': 'PetPixaDownloadResponse',
  '2': [
    {'1': 'pet_id', '3': 1, '4': 1, '5': 9, '10': 'petId'},
    {'1': 'petdef_id', '3': 2, '4': 1, '5': 9, '10': 'petdefId'},
    {
      '1': 'pixa_path',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'pixaPath',
      '17': true
    },
    {'1': 'size_bytes', '3': 4, '4': 1, '5': 3, '10': 'sizeBytes'},
  ],
  '8': [
    {'1': '_pixa_path'},
  ],
};

/// Descriptor for `PetPixaDownloadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPixaDownloadResponseDescriptor = $convert.base64Decode(
    'ChdQZXRQaXhhRG93bmxvYWRSZXNwb25zZRIVCgZwZXRfaWQYASABKAlSBXBldElkEhsKCXBldG'
    'RlZl9pZBgCIAEoCVIIcGV0ZGVmSWQSIAoJcGl4YV9wYXRoGAMgASgJSABSCHBpeGFQYXRoiAEB'
    'Eh0KCnNpemVfYnl0ZXMYBCABKANSCXNpemVCeXRlc0IMCgpfcGl4YV9wYXRo');

@$core.Deprecated('Use petPresentationDescriptor instead')
const PetPresentation$json = {
  '1': 'PetPresentation',
  '2': [
    {'1': 'pet_id', '3': 1, '4': 1, '5': 9, '10': 'petId'},
    {'1': 'petdef_id', '3': 2, '4': 1, '5': 9, '10': 'petdefId'},
    {'1': 'default_locale', '3': 3, '4': 1, '5': 9, '10': 'defaultLocale'},
    {
      '1': 'attr',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationAttrSpec',
      '10': 'attr'
    },
    {
      '1': 'drive',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationDriveSpec',
      '10': 'drive'
    },
    {
      '1': 'pixa_metadata',
      '3': 6,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationPixaMetadata',
      '10': 'pixaMetadata'
    },
    {
      '1': 'i18n',
      '3': 7,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationI18nSpec',
      '10': 'i18n'
    },
    {
      '1': 'pixa_path',
      '3': 8,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'pixaPath',
      '17': true
    },
    {'1': 'petdef_updated_at', '3': 9, '4': 1, '5': 9, '10': 'petdefUpdatedAt'},
  ],
  '8': [
    {'1': '_pixa_path'},
  ],
};

/// Descriptor for `PetPresentation`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationDescriptor = $convert.base64Decode(
    'Cg9QZXRQcmVzZW50YXRpb24SFQoGcGV0X2lkGAEgASgJUgVwZXRJZBIbCglwZXRkZWZfaWQYAi'
    'ABKAlSCHBldGRlZklkEiUKDmRlZmF1bHRfbG9jYWxlGAMgASgJUg1kZWZhdWx0TG9jYWxlEjsK'
    'BGF0dHIYBCABKAsyJy5naXpjbGF3LnJwYy52MS5QZXRQcmVzZW50YXRpb25BdHRyU3BlY1IEYX'
    'R0chI+CgVkcml2ZRgFIAEoCzIoLmdpemNsYXcucnBjLnYxLlBldFByZXNlbnRhdGlvbkRyaXZl'
    'U3BlY1IFZHJpdmUSUAoNcGl4YV9tZXRhZGF0YRgGIAEoCzIrLmdpemNsYXcucnBjLnYxLlBldF'
    'ByZXNlbnRhdGlvblBpeGFNZXRhZGF0YVIMcGl4YU1ldGFkYXRhEjsKBGkxOG4YByABKAsyJy5n'
    'aXpjbGF3LnJwYy52MS5QZXRQcmVzZW50YXRpb25JMThuU3BlY1IEaTE4bhIgCglwaXhhX3BhdG'
    'gYCCABKAlIAFIIcGl4YVBhdGiIAQESKgoRcGV0ZGVmX3VwZGF0ZWRfYXQYCSABKAlSD3BldGRl'
    'ZlVwZGF0ZWRBdEIMCgpfcGl4YV9wYXRo');

@$core.Deprecated('Use petPresentationActionEffectSpecDescriptor instead')
const PetPresentationActionEffectSpec$json = {
  '1': 'PetPresentationActionEffectSpec',
  '2': [
    {
      '1': 'attr_delta',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationAttrDelta',
      '9': 0,
      '10': 'attrDelta',
      '17': true
    },
    {
      '1': 'pet_exp_delta',
      '3': 2,
      '4': 1,
      '5': 3,
      '9': 1,
      '10': 'petExpDelta',
      '17': true
    },
  ],
  '8': [
    {'1': '_attr_delta'},
    {'1': '_pet_exp_delta'},
  ],
};

/// Descriptor for `PetPresentationActionEffectSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationActionEffectSpecDescriptor =
    $convert.base64Decode(
        'Ch9QZXRQcmVzZW50YXRpb25BY3Rpb25FZmZlY3RTcGVjEkwKCmF0dHJfZGVsdGEYASABKAsyKC'
        '5naXpjbGF3LnJwYy52MS5QZXRQcmVzZW50YXRpb25BdHRyRGVsdGFIAFIJYXR0ckRlbHRhiAEB'
        'EicKDXBldF9leHBfZGVsdGEYAiABKANIAVILcGV0RXhwRGVsdGGIAQFCDQoLX2F0dHJfZGVsdG'
        'FCEAoOX3BldF9leHBfZGVsdGE=');

@$core.Deprecated('Use petPresentationActionSpecDescriptor instead')
const PetPresentationActionSpec$json = {
  '1': 'PetPresentationActionSpec',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'cost', '3': 2, '4': 1, '5': 3, '10': 'cost'},
    {
      '1': 'effect',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationActionEffectSpec',
      '9': 0,
      '10': 'effect',
      '17': true
    },
    {
      '1': 'visual_clip_id',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'visualClipId',
      '17': true
    },
  ],
  '8': [
    {'1': '_effect'},
    {'1': '_visual_clip_id'},
  ],
};

/// Descriptor for `PetPresentationActionSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationActionSpecDescriptor = $convert.base64Decode(
    'ChlQZXRQcmVzZW50YXRpb25BY3Rpb25TcGVjEg4KAmlkGAEgASgJUgJpZBISCgRjb3N0GAIgAS'
    'gDUgRjb3N0EkwKBmVmZmVjdBgDIAEoCzIvLmdpemNsYXcucnBjLnYxLlBldFByZXNlbnRhdGlv'
    'bkFjdGlvbkVmZmVjdFNwZWNIAFIGZWZmZWN0iAEBEikKDnZpc3VhbF9jbGlwX2lkGAQgASgJSA'
    'FSDHZpc3VhbENsaXBJZIgBAUIJCgdfZWZmZWN0QhEKD192aXN1YWxfY2xpcF9pZA==');

@$core.Deprecated('Use petPresentationAttrDeltaDescriptor instead')
const PetPresentationAttrDelta$json = {
  '1': 'PetPresentationAttrDelta',
  '2': [
    {
      '1': 'life',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetLife',
      '9': 0,
      '10': 'life',
      '17': true
    },
  ],
  '8': [
    {'1': '_life'},
  ],
};

/// Descriptor for `PetPresentationAttrDelta`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationAttrDeltaDescriptor =
    $convert.base64Decode(
        'ChhQZXRQcmVzZW50YXRpb25BdHRyRGVsdGESMAoEbGlmZRgBIAEoCzIXLmdpemNsYXcucnBjLn'
        'YxLlBldExpZmVIAFIEbGlmZYgBAUIHCgVfbGlmZQ==');

@$core.Deprecated('Use petPresentationAttrGroupSpecDescriptor instead')
const PetPresentationAttrGroupSpec$json = {
  '1': 'PetPresentationAttrGroupSpec',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationAttrGroupSpec.ValueEntry',
      '10': 'value'
    },
  ],
  '3': [PetPresentationAttrGroupSpec_ValueEntry$json],
};

@$core.Deprecated('Use petPresentationAttrGroupSpecDescriptor instead')
const PetPresentationAttrGroupSpec_ValueEntry$json = {
  '1': 'ValueEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {
      '1': 'value',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationAttrValueSpec',
      '10': 'value'
    },
  ],
  '7': {'7': true},
};

/// Descriptor for `PetPresentationAttrGroupSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationAttrGroupSpecDescriptor = $convert.base64Decode(
    'ChxQZXRQcmVzZW50YXRpb25BdHRyR3JvdXBTcGVjEk0KBXZhbHVlGAEgAygLMjcuZ2l6Y2xhdy'
    '5ycGMudjEuUGV0UHJlc2VudGF0aW9uQXR0ckdyb3VwU3BlYy5WYWx1ZUVudHJ5UgV2YWx1ZRpm'
    'CgpWYWx1ZUVudHJ5EhAKA2tleRgBIAEoCVIDa2V5EkIKBXZhbHVlGAIgASgLMiwuZ2l6Y2xhdy'
    '5ycGMudjEuUGV0UHJlc2VudGF0aW9uQXR0clZhbHVlU3BlY1IFdmFsdWU6AjgB');

@$core.Deprecated('Use petPresentationAttrSpecDescriptor instead')
const PetPresentationAttrSpec$json = {
  '1': 'PetPresentationAttrSpec',
  '2': [
    {
      '1': 'life',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationAttrGroupSpec',
      '10': 'life'
    },
    {
      '1': 'progression',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationAttrGroupSpec',
      '10': 'progression'
    },
  ],
};

/// Descriptor for `PetPresentationAttrSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationAttrSpecDescriptor = $convert.base64Decode(
    'ChdQZXRQcmVzZW50YXRpb25BdHRyU3BlYxJACgRsaWZlGAEgASgLMiwuZ2l6Y2xhdy5ycGMudj'
    'EuUGV0UHJlc2VudGF0aW9uQXR0ckdyb3VwU3BlY1IEbGlmZRJOCgtwcm9ncmVzc2lvbhgCIAEo'
    'CzIsLmdpemNsYXcucnBjLnYxLlBldFByZXNlbnRhdGlvbkF0dHJHcm91cFNwZWNSC3Byb2dyZX'
    'NzaW9u');

@$core.Deprecated('Use petPresentationAttrValueSpecDescriptor instead')
const PetPresentationAttrValueSpec$json = {
  '1': 'PetPresentationAttrValueSpec',
  '2': [
    {'1': 'initial', '3': 1, '4': 1, '5': 3, '10': 'initial'},
  ],
};

/// Descriptor for `PetPresentationAttrValueSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationAttrValueSpecDescriptor =
    $convert.base64Decode(
        'ChxQZXRQcmVzZW50YXRpb25BdHRyVmFsdWVTcGVjEhgKB2luaXRpYWwYASABKANSB2luaXRpYW'
        'w=');

@$core.Deprecated('Use petPresentationDriveSpecDescriptor instead')
const PetPresentationDriveSpec$json = {
  '1': 'PetPresentationDriveSpec',
  '2': [
    {
      '1': 'actions',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationActionSpec',
      '10': 'actions'
    },
  ],
};

/// Descriptor for `PetPresentationDriveSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationDriveSpecDescriptor =
    $convert.base64Decode(
        'ChhQZXRQcmVzZW50YXRpb25Ecml2ZVNwZWMSQwoHYWN0aW9ucxgBIAMoCzIpLmdpemNsYXcucn'
        'BjLnYxLlBldFByZXNlbnRhdGlvbkFjdGlvblNwZWNSB2FjdGlvbnM=');

@$core.Deprecated('Use petPresentationI18nAttrGroupDescriptor instead')
const PetPresentationI18nAttrGroup$json = {
  '1': 'PetPresentationI18nAttrGroup',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationI18nAttrGroup.ValueEntry',
      '10': 'value'
    },
  ],
  '3': [PetPresentationI18nAttrGroup_ValueEntry$json],
};

@$core.Deprecated('Use petPresentationI18nAttrGroupDescriptor instead')
const PetPresentationI18nAttrGroup_ValueEntry$json = {
  '1': 'ValueEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {
      '1': 'value',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationI18nDisplayText',
      '10': 'value'
    },
  ],
  '7': {'7': true},
};

/// Descriptor for `PetPresentationI18nAttrGroup`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationI18nAttrGroupDescriptor = $convert.base64Decode(
    'ChxQZXRQcmVzZW50YXRpb25JMThuQXR0ckdyb3VwEk0KBXZhbHVlGAEgAygLMjcuZ2l6Y2xhdy'
    '5ycGMudjEuUGV0UHJlc2VudGF0aW9uSTE4bkF0dHJHcm91cC5WYWx1ZUVudHJ5UgV2YWx1ZRpo'
    'CgpWYWx1ZUVudHJ5EhAKA2tleRgBIAEoCVIDa2V5EkQKBXZhbHVlGAIgASgLMi4uZ2l6Y2xhdy'
    '5ycGMudjEuUGV0UHJlc2VudGF0aW9uSTE4bkRpc3BsYXlUZXh0UgV2YWx1ZToCOAE=');

@$core.Deprecated('Use petPresentationI18nAttrSpecDescriptor instead')
const PetPresentationI18nAttrSpec$json = {
  '1': 'PetPresentationI18nAttrSpec',
  '2': [
    {
      '1': 'life',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationI18nAttrGroup',
      '9': 0,
      '10': 'life',
      '17': true
    },
    {
      '1': 'progression',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationI18nAttrGroup',
      '9': 1,
      '10': 'progression',
      '17': true
    },
  ],
  '8': [
    {'1': '_life'},
    {'1': '_progression'},
  ],
};

/// Descriptor for `PetPresentationI18nAttrSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationI18nAttrSpecDescriptor = $convert.base64Decode(
    'ChtQZXRQcmVzZW50YXRpb25JMThuQXR0clNwZWMSRQoEbGlmZRgBIAEoCzIsLmdpemNsYXcucn'
    'BjLnYxLlBldFByZXNlbnRhdGlvbkkxOG5BdHRyR3JvdXBIAFIEbGlmZYgBARJTCgtwcm9ncmVz'
    'c2lvbhgCIAEoCzIsLmdpemNsYXcucnBjLnYxLlBldFByZXNlbnRhdGlvbkkxOG5BdHRyR3JvdX'
    'BIAVILcHJvZ3Jlc3Npb26IAQFCBwoFX2xpZmVCDgoMX3Byb2dyZXNzaW9u');

@$core.Deprecated('Use petPresentationI18nCatalogDescriptor instead')
const PetPresentationI18nCatalog$json = {
  '1': 'PetPresentationI18nCatalog',
  '2': [
    {
      '1': 'display_name',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'displayName',
      '17': true
    },
    {
      '1': 'description',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'description',
      '17': true
    },
    {
      '1': 'attr',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationI18nAttrSpec',
      '9': 2,
      '10': 'attr',
      '17': true
    },
    {
      '1': 'drive',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationI18nDriveSpec',
      '9': 3,
      '10': 'drive',
      '17': true
    },
  ],
  '8': [
    {'1': '_display_name'},
    {'1': '_description'},
    {'1': '_attr'},
    {'1': '_drive'},
  ],
};

/// Descriptor for `PetPresentationI18nCatalog`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationI18nCatalogDescriptor = $convert.base64Decode(
    'ChpQZXRQcmVzZW50YXRpb25JMThuQ2F0YWxvZxImCgxkaXNwbGF5X25hbWUYASABKAlIAFILZG'
    'lzcGxheU5hbWWIAQESJQoLZGVzY3JpcHRpb24YAiABKAlIAVILZGVzY3JpcHRpb26IAQESRAoE'
    'YXR0chgDIAEoCzIrLmdpemNsYXcucnBjLnYxLlBldFByZXNlbnRhdGlvbkkxOG5BdHRyU3BlY0'
    'gCUgRhdHRyiAEBEkcKBWRyaXZlGAQgASgLMiwuZ2l6Y2xhdy5ycGMudjEuUGV0UHJlc2VudGF0'
    'aW9uSTE4bkRyaXZlU3BlY0gDUgVkcml2ZYgBAUIPCg1fZGlzcGxheV9uYW1lQg4KDF9kZXNjcm'
    'lwdGlvbkIHCgVfYXR0ckIICgZfZHJpdmU=');

@$core.Deprecated('Use petPresentationI18nDisplayTextDescriptor instead')
const PetPresentationI18nDisplayText$json = {
  '1': 'PetPresentationI18nDisplayText',
  '2': [
    {'1': 'display_name', '3': 1, '4': 1, '5': 9, '10': 'displayName'},
  ],
};

/// Descriptor for `PetPresentationI18nDisplayText`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationI18nDisplayTextDescriptor =
    $convert.base64Decode(
        'Ch5QZXRQcmVzZW50YXRpb25JMThuRGlzcGxheVRleHQSIQoMZGlzcGxheV9uYW1lGAEgASgJUg'
        'tkaXNwbGF5TmFtZQ==');

@$core.Deprecated('Use petPresentationI18nDriveSpecDescriptor instead')
const PetPresentationI18nDriveSpec$json = {
  '1': 'PetPresentationI18nDriveSpec',
  '2': [
    {
      '1': 'actions',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationI18nDriveSpec.ActionsEntry',
      '10': 'actions'
    },
  ],
  '3': [PetPresentationI18nDriveSpec_ActionsEntry$json],
};

@$core.Deprecated('Use petPresentationI18nDriveSpecDescriptor instead')
const PetPresentationI18nDriveSpec_ActionsEntry$json = {
  '1': 'ActionsEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {
      '1': 'value',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationI18nDisplayText',
      '10': 'value'
    },
  ],
  '7': {'7': true},
};

/// Descriptor for `PetPresentationI18nDriveSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationI18nDriveSpecDescriptor = $convert.base64Decode(
    'ChxQZXRQcmVzZW50YXRpb25JMThuRHJpdmVTcGVjElMKB2FjdGlvbnMYASADKAsyOS5naXpjbG'
    'F3LnJwYy52MS5QZXRQcmVzZW50YXRpb25JMThuRHJpdmVTcGVjLkFjdGlvbnNFbnRyeVIHYWN0'
    'aW9ucxpqCgxBY3Rpb25zRW50cnkSEAoDa2V5GAEgASgJUgNrZXkSRAoFdmFsdWUYAiABKAsyLi'
    '5naXpjbGF3LnJwYy52MS5QZXRQcmVzZW50YXRpb25JMThuRGlzcGxheVRleHRSBXZhbHVlOgI4'
    'AQ==');

@$core.Deprecated('Use petPresentationI18nSpecDescriptor instead')
const PetPresentationI18nSpec$json = {
  '1': 'PetPresentationI18nSpec',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationI18nSpec.ValueEntry',
      '10': 'value'
    },
  ],
  '3': [PetPresentationI18nSpec_ValueEntry$json],
};

@$core.Deprecated('Use petPresentationI18nSpecDescriptor instead')
const PetPresentationI18nSpec_ValueEntry$json = {
  '1': 'ValueEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {
      '1': 'value',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationI18nCatalog',
      '10': 'value'
    },
  ],
  '7': {'7': true},
};

/// Descriptor for `PetPresentationI18nSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationI18nSpecDescriptor = $convert.base64Decode(
    'ChdQZXRQcmVzZW50YXRpb25JMThuU3BlYxJICgV2YWx1ZRgBIAMoCzIyLmdpemNsYXcucnBjLn'
    'YxLlBldFByZXNlbnRhdGlvbkkxOG5TcGVjLlZhbHVlRW50cnlSBXZhbHVlGmQKClZhbHVlRW50'
    'cnkSEAoDa2V5GAEgASgJUgNrZXkSQAoFdmFsdWUYAiABKAsyKi5naXpjbGF3LnJwYy52MS5QZX'
    'RQcmVzZW50YXRpb25JMThuQ2F0YWxvZ1IFdmFsdWU6AjgB');

@$core.Deprecated('Use petPresentationPixaCanvasMetadataDescriptor instead')
const PetPresentationPixaCanvasMetadata$json = {
  '1': 'PetPresentationPixaCanvasMetadata',
  '2': [
    {'1': 'width', '3': 1, '4': 1, '5': 3, '10': 'width'},
    {'1': 'height', '3': 2, '4': 1, '5': 3, '10': 'height'},
  ],
};

/// Descriptor for `PetPresentationPixaCanvasMetadata`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationPixaCanvasMetadataDescriptor =
    $convert.base64Decode(
        'CiFQZXRQcmVzZW50YXRpb25QaXhhQ2FudmFzTWV0YWRhdGESFAoFd2lkdGgYASABKANSBXdpZH'
        'RoEhYKBmhlaWdodBgCIAEoA1IGaGVpZ2h0');

@$core.Deprecated('Use petPresentationPixaClipMetadataDescriptor instead')
const PetPresentationPixaClipMetadata$json = {
  '1': 'PetPresentationPixaClipMetadata',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'action_id',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'actionId',
      '17': true
    },
    {'1': 'pixa_clip_name', '3': 3, '4': 1, '5': 9, '10': 'pixaClipName'},
  ],
  '8': [
    {'1': '_action_id'},
  ],
};

/// Descriptor for `PetPresentationPixaClipMetadata`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationPixaClipMetadataDescriptor =
    $convert.base64Decode(
        'Ch9QZXRQcmVzZW50YXRpb25QaXhhQ2xpcE1ldGFkYXRhEg4KAmlkGAEgASgJUgJpZBIgCglhY3'
        'Rpb25faWQYAiABKAlIAFIIYWN0aW9uSWSIAQESJAoOcGl4YV9jbGlwX25hbWUYAyABKAlSDHBp'
        'eGFDbGlwTmFtZUIMCgpfYWN0aW9uX2lk');

@$core.Deprecated('Use petPresentationPixaMetadataDescriptor instead')
const PetPresentationPixaMetadata$json = {
  '1': 'PetPresentationPixaMetadata',
  '2': [
    {'1': 'version', '3': 1, '4': 1, '5': 9, '10': 'version'},
    {
      '1': 'canvas',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationPixaCanvasMetadata',
      '10': 'canvas'
    },
    {
      '1': 'clips',
      '3': 3,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentationPixaClipMetadata',
      '10': 'clips'
    },
  ],
};

/// Descriptor for `PetPresentationPixaMetadata`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPresentationPixaMetadataDescriptor = $convert.base64Decode(
    'ChtQZXRQcmVzZW50YXRpb25QaXhhTWV0YWRhdGESGAoHdmVyc2lvbhgBIAEoCVIHdmVyc2lvbh'
    'JJCgZjYW52YXMYAiABKAsyMS5naXpjbGF3LnJwYy52MS5QZXRQcmVzZW50YXRpb25QaXhhQ2Fu'
    'dmFzTWV0YWRhdGFSBmNhbnZhcxJFCgVjbGlwcxgDIAMoCzIvLmdpemNsYXcucnBjLnYxLlBldF'
    'ByZXNlbnRhdGlvblBpeGFDbGlwTWV0YWRhdGFSBWNsaXBz');

@$core.Deprecated('Use petDeleteRequestDescriptor instead')
const PetDeleteRequest$json = {
  '1': 'PetDeleteRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `PetDeleteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petDeleteRequestDescriptor =
    $convert.base64Decode('ChBQZXREZWxldGVSZXF1ZXN0Eg4KAmlkGAEgASgJUgJpZA==');

@$core.Deprecated('Use petDriveGameResultInputDescriptor instead')
const PetDriveGameResultInput$json = {
  '1': 'PetDriveGameResultInput',
  '2': [
    {
      '1': 'difficulty',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'difficulty',
      '17': true
    },
    {
      '1': 'duration_ms',
      '3': 2,
      '4': 1,
      '5': 3,
      '9': 1,
      '10': 'durationMs',
      '17': true
    },
    {'1': 'game_def_id', '3': 3, '4': 1, '5': 9, '10': 'gameDefId'},
    {
      '1': 'idempotency_key',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'idempotencyKey',
      '17': true
    },
    {
      '1': 'max_score',
      '3': 5,
      '4': 1,
      '5': 3,
      '9': 3,
      '10': 'maxScore',
      '17': true
    },
    {
      '1': 'occurred_at',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'occurredAt',
      '17': true
    },
    {
      '1': 'outcome',
      '3': 7,
      '4': 1,
      '5': 9,
      '9': 5,
      '10': 'outcome',
      '17': true
    },
    {
      '1': 'payload',
      '3': 8,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameplayMetadata',
      '9': 6,
      '10': 'payload',
      '17': true
    },
    {'1': 'score', '3': 9, '4': 1, '5': 3, '9': 7, '10': 'score', '17': true},
  ],
  '8': [
    {'1': '_difficulty'},
    {'1': '_duration_ms'},
    {'1': '_idempotency_key'},
    {'1': '_max_score'},
    {'1': '_occurred_at'},
    {'1': '_outcome'},
    {'1': '_payload'},
    {'1': '_score'},
  ],
};

/// Descriptor for `PetDriveGameResultInput`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petDriveGameResultInputDescriptor = $convert.base64Decode(
    'ChdQZXREcml2ZUdhbWVSZXN1bHRJbnB1dBIjCgpkaWZmaWN1bHR5GAEgASgJSABSCmRpZmZpY3'
    'VsdHmIAQESJAoLZHVyYXRpb25fbXMYAiABKANIAVIKZHVyYXRpb25Nc4gBARIeCgtnYW1lX2Rl'
    'Zl9pZBgDIAEoCVIJZ2FtZURlZklkEiwKD2lkZW1wb3RlbmN5X2tleRgEIAEoCUgCUg5pZGVtcG'
    '90ZW5jeUtleYgBARIgCgltYXhfc2NvcmUYBSABKANIA1IIbWF4U2NvcmWIAQESJAoLb2NjdXJy'
    'ZWRfYXQYBiABKAlIBFIKb2NjdXJyZWRBdIgBARIdCgdvdXRjb21lGAcgASgJSAVSB291dGNvbW'
    'WIAQESPwoHcGF5bG9hZBgIIAEoCzIgLmdpemNsYXcucnBjLnYxLkdhbWVwbGF5TWV0YWRhdGFI'
    'BlIHcGF5bG9hZIgBARIZCgVzY29yZRgJIAEoA0gHUgVzY29yZYgBAUINCgtfZGlmZmljdWx0eU'
    'IOCgxfZHVyYXRpb25fbXNCEgoQX2lkZW1wb3RlbmN5X2tleUIMCgpfbWF4X3Njb3JlQg4KDF9v'
    'Y2N1cnJlZF9hdEIKCghfb3V0Y29tZUIKCghfcGF5bG9hZEIICgZfc2NvcmU=');

@$core.Deprecated('Use petDriveRequestDescriptor instead')
const PetDriveRequest$json = {
  '1': 'PetDriveRequest',
  '2': [
    {'1': 'action', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'action', '17': true},
    {
      '1': 'game_result',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetDriveGameResultInput',
      '9': 1,
      '10': 'gameResult',
      '17': true
    },
    {'1': 'pet_id', '3': 3, '4': 1, '5': 9, '10': 'petId'},
  ],
  '8': [
    {'1': '_action'},
    {'1': '_game_result'},
  ],
};

/// Descriptor for `PetDriveRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petDriveRequestDescriptor = $convert.base64Decode(
    'Cg9QZXREcml2ZVJlcXVlc3QSGwoGYWN0aW9uGAEgASgJSABSBmFjdGlvbogBARJNCgtnYW1lX3'
    'Jlc3VsdBgCIAEoCzInLmdpemNsYXcucnBjLnYxLlBldERyaXZlR2FtZVJlc3VsdElucHV0SAFS'
    'CmdhbWVSZXN1bHSIAQESFQoGcGV0X2lkGAMgASgJUgVwZXRJZEIJCgdfYWN0aW9uQg4KDF9nYW'
    '1lX3Jlc3VsdA==');

@$core.Deprecated('Use petDriveResponseDescriptor instead')
const PetDriveResponse$json = {
  '1': 'PetDriveResponse',
  '2': [
    {
      '1': 'badges',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Badge',
      '10': 'badges'
    },
    {
      '1': 'game_result',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameResult',
      '9': 0,
      '10': 'gameResult',
      '17': true
    },
    {
      '1': 'pet',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Pet',
      '10': 'pet'
    },
    {
      '1': 'points',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PointsAccount',
      '10': 'points'
    },
    {
      '1': 'reward_grants',
      '3': 5,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.RewardGrant',
      '10': 'rewardGrants'
    },
    {
      '1': 'transactions',
      '3': 6,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PointsTransaction',
      '10': 'transactions'
    },
  ],
  '8': [
    {'1': '_game_result'},
  ],
};

/// Descriptor for `PetDriveResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petDriveResponseDescriptor = $convert.base64Decode(
    'ChBQZXREcml2ZVJlc3BvbnNlEi0KBmJhZGdlcxgBIAMoCzIVLmdpemNsYXcucnBjLnYxLkJhZG'
    'dlUgZiYWRnZXMSQAoLZ2FtZV9yZXN1bHQYAiABKAsyGi5naXpjbGF3LnJwYy52MS5HYW1lUmVz'
    'dWx0SABSCmdhbWVSZXN1bHSIAQESJQoDcGV0GAMgASgLMhMuZ2l6Y2xhdy5ycGMudjEuUGV0Ug'
    'NwZXQSNQoGcG9pbnRzGAQgASgLMh0uZ2l6Y2xhdy5ycGMudjEuUG9pbnRzQWNjb3VudFIGcG9p'
    'bnRzEkAKDXJld2FyZF9ncmFudHMYBSADKAsyGy5naXpjbGF3LnJwYy52MS5SZXdhcmRHcmFudF'
    'IMcmV3YXJkR3JhbnRzEkUKDHRyYW5zYWN0aW9ucxgGIAMoCzIhLmdpemNsYXcucnBjLnYxLlBv'
    'aW50c1RyYW5zYWN0aW9uUgx0cmFuc2FjdGlvbnNCDgoMX2dhbWVfcmVzdWx0');

@$core.Deprecated('Use petGetRequestDescriptor instead')
const PetGetRequest$json = {
  '1': 'PetGetRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `PetGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petGetRequestDescriptor =
    $convert.base64Decode('Cg1QZXRHZXRSZXF1ZXN0Eg4KAmlkGAEgASgJUgJpZA==');

@$core.Deprecated('Use petListResponseDescriptor instead')
const PetListResponse$json = {
  '1': 'PetListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Pet',
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
  ],
  '8': [
    {'1': '_next_cursor'},
  ],
};

/// Descriptor for `PetListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petListResponseDescriptor = $convert.base64Decode(
    'Cg9QZXRMaXN0UmVzcG9uc2USGQoIaGFzX25leHQYASABKAhSB2hhc05leHQSKQoFaXRlbXMYAi'
    'ADKAsyEy5naXpjbGF3LnJwYy52MS5QZXRSBWl0ZW1zEiQKC25leHRfY3Vyc29yGAMgASgJSABS'
    'Cm5leHRDdXJzb3KIAQFCDgoMX25leHRfY3Vyc29y');

@$core.Deprecated('Use petPutRequestDescriptor instead')
const PetPutRequest$json = {
  '1': 'PetPutRequest',
  '2': [
    {'1': 'display_name', '3': 1, '4': 1, '5': 9, '10': 'displayName'},
    {'1': 'id', '3': 2, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `PetPutRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPutRequestDescriptor = $convert.base64Decode(
    'Cg1QZXRQdXRSZXF1ZXN0EiEKDGRpc3BsYXlfbmFtZRgBIAEoCVILZGlzcGxheU5hbWUSDgoCaW'
    'QYAiABKAlSAmlk');

@$core.Deprecated('Use pointsAccountDescriptor instead')
const PointsAccount$json = {
  '1': 'PointsAccount',
  '2': [
    {'1': 'balance', '3': 1, '4': 1, '5': 3, '10': 'balance'},
    {'1': 'created_at', '3': 2, '4': 1, '5': 9, '10': 'createdAt'},
    {'1': 'owner_public_key', '3': 3, '4': 1, '5': 9, '10': 'ownerPublicKey'},
    {'1': 'ruleset_name', '3': 4, '4': 1, '5': 9, '10': 'rulesetName'},
    {'1': 'updated_at', '3': 5, '4': 1, '5': 9, '10': 'updatedAt'},
  ],
};

/// Descriptor for `PointsAccount`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pointsAccountDescriptor = $convert.base64Decode(
    'Cg1Qb2ludHNBY2NvdW50EhgKB2JhbGFuY2UYASABKANSB2JhbGFuY2USHQoKY3JlYXRlZF9hdB'
    'gCIAEoCVIJY3JlYXRlZEF0EigKEG93bmVyX3B1YmxpY19rZXkYAyABKAlSDm93bmVyUHVibGlj'
    'S2V5EiEKDHJ1bGVzZXRfbmFtZRgEIAEoCVILcnVsZXNldE5hbWUSHQoKdXBkYXRlZF9hdBgFIA'
    'EoCVIJdXBkYXRlZEF0');

@$core.Deprecated('Use pointsTransactionDescriptor instead')
const PointsTransaction$json = {
  '1': 'PointsTransaction',
  '2': [
    {'1': 'balance_after', '3': 1, '4': 1, '5': 3, '10': 'balanceAfter'},
    {'1': 'created_at', '3': 2, '4': 1, '5': 9, '10': 'createdAt'},
    {'1': 'delta', '3': 3, '4': 1, '5': 3, '10': 'delta'},
    {
      '1': 'game_result_id',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'gameResultId',
      '17': true
    },
    {'1': 'id', '3': 5, '4': 1, '5': 9, '10': 'id'},
    {'1': 'owner_public_key', '3': 6, '4': 1, '5': 9, '10': 'ownerPublicKey'},
    {'1': 'pet_id', '3': 7, '4': 1, '5': 9, '9': 1, '10': 'petId', '17': true},
    {'1': 'reason', '3': 8, '4': 1, '5': 9, '10': 'reason'},
    {
      '1': 'reward_grant_id',
      '3': 9,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'rewardGrantId',
      '17': true
    },
    {'1': 'ruleset_name', '3': 10, '4': 1, '5': 9, '10': 'rulesetName'},
    {'1': 'source_id', '3': 11, '4': 1, '5': 9, '10': 'sourceId'},
    {'1': 'source_type', '3': 12, '4': 1, '5': 9, '10': 'sourceType'},
  ],
  '8': [
    {'1': '_game_result_id'},
    {'1': '_pet_id'},
    {'1': '_reward_grant_id'},
  ],
};

/// Descriptor for `PointsTransaction`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pointsTransactionDescriptor = $convert.base64Decode(
    'ChFQb2ludHNUcmFuc2FjdGlvbhIjCg1iYWxhbmNlX2FmdGVyGAEgASgDUgxiYWxhbmNlQWZ0ZX'
    'ISHQoKY3JlYXRlZF9hdBgCIAEoCVIJY3JlYXRlZEF0EhQKBWRlbHRhGAMgASgDUgVkZWx0YRIp'
    'Cg5nYW1lX3Jlc3VsdF9pZBgEIAEoCUgAUgxnYW1lUmVzdWx0SWSIAQESDgoCaWQYBSABKAlSAm'
    'lkEigKEG93bmVyX3B1YmxpY19rZXkYBiABKAlSDm93bmVyUHVibGljS2V5EhoKBnBldF9pZBgH'
    'IAEoCUgBUgVwZXRJZIgBARIWCgZyZWFzb24YCCABKAlSBnJlYXNvbhIrCg9yZXdhcmRfZ3Jhbn'
    'RfaWQYCSABKAlIAlINcmV3YXJkR3JhbnRJZIgBARIhCgxydWxlc2V0X25hbWUYCiABKAlSC3J1'
    'bGVzZXROYW1lEhsKCXNvdXJjZV9pZBgLIAEoCVIIc291cmNlSWQSHwoLc291cmNlX3R5cGUYDC'
    'ABKAlSCnNvdXJjZVR5cGVCEQoPX2dhbWVfcmVzdWx0X2lkQgkKB19wZXRfaWRCEgoQX3Jld2Fy'
    'ZF9ncmFudF9pZA==');

@$core.Deprecated('Use pointsTransactionListResponseDescriptor instead')
const PointsTransactionListResponse$json = {
  '1': 'PointsTransactionListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PointsTransaction',
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
  ],
  '8': [
    {'1': '_next_cursor'},
  ],
};

/// Descriptor for `PointsTransactionListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pointsTransactionListResponseDescriptor = $convert.base64Decode(
    'Ch1Qb2ludHNUcmFuc2FjdGlvbkxpc3RSZXNwb25zZRIZCghoYXNfbmV4dBgBIAEoCFIHaGFzTm'
    'V4dBI3CgVpdGVtcxgCIAMoCzIhLmdpemNsYXcucnBjLnYxLlBvaW50c1RyYW5zYWN0aW9uUgVp'
    'dGVtcxIkCgtuZXh0X2N1cnNvchgDIAEoCUgAUgpuZXh0Q3Vyc29yiAEBQg4KDF9uZXh0X2N1cn'
    'Nvcg==');

@$core.Deprecated('Use rewardGrantDescriptor instead')
const RewardGrant$json = {
  '1': 'RewardGrant',
  '2': [
    {
      '1': 'badge_exp_delta',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.RewardGrant.BadgeExpDeltaEntry',
      '10': 'badgeExpDelta'
    },
    {'1': 'created_at', '3': 3, '4': 1, '5': 9, '10': 'createdAt'},
    {
      '1': 'game_result_id',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'gameResultId',
      '17': true
    },
    {'1': 'id', '3': 5, '4': 1, '5': 9, '10': 'id'},
    {'1': 'owner_public_key', '3': 7, '4': 1, '5': 9, '10': 'ownerPublicKey'},
    {'1': 'pet_exp_delta', '3': 8, '4': 1, '5': 3, '10': 'petExpDelta'},
    {'1': 'pet_id', '3': 9, '4': 1, '5': 9, '9': 1, '10': 'petId', '17': true},
    {'1': 'points_delta', '3': 10, '4': 1, '5': 3, '10': 'pointsDelta'},
    {
      '1': 'reason',
      '3': 11,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'reason',
      '17': true
    },
    {'1': 'ruleset_name', '3': 12, '4': 1, '5': 9, '10': 'rulesetName'},
    {'1': 'source_id', '3': 13, '4': 1, '5': 9, '10': 'sourceId'},
    {'1': 'source_type', '3': 14, '4': 1, '5': 9, '10': 'sourceType'},
  ],
  '3': [RewardGrant_BadgeExpDeltaEntry$json],
  '8': [
    {'1': '_game_result_id'},
    {'1': '_pet_id'},
    {'1': '_reason'},
  ],
  '9': [
    {'1': 1, '2': 2},
    {'1': 6, '2': 7},
  ],
  '10': ['ability_delta', 'life_delta'],
};

@$core.Deprecated('Use rewardGrantDescriptor instead')
const RewardGrant_BadgeExpDeltaEntry$json = {
  '1': 'BadgeExpDeltaEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 3, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `RewardGrant`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List rewardGrantDescriptor = $convert.base64Decode(
    'CgtSZXdhcmRHcmFudBJWCg9iYWRnZV9leHBfZGVsdGEYAiADKAsyLi5naXpjbGF3LnJwYy52MS'
    '5SZXdhcmRHcmFudC5CYWRnZUV4cERlbHRhRW50cnlSDWJhZGdlRXhwRGVsdGESHQoKY3JlYXRl'
    'ZF9hdBgDIAEoCVIJY3JlYXRlZEF0EikKDmdhbWVfcmVzdWx0X2lkGAQgASgJSABSDGdhbWVSZX'
    'N1bHRJZIgBARIOCgJpZBgFIAEoCVICaWQSKAoQb3duZXJfcHVibGljX2tleRgHIAEoCVIOb3du'
    'ZXJQdWJsaWNLZXkSIgoNcGV0X2V4cF9kZWx0YRgIIAEoA1ILcGV0RXhwRGVsdGESGgoGcGV0X2'
    'lkGAkgASgJSAFSBXBldElkiAEBEiEKDHBvaW50c19kZWx0YRgKIAEoA1ILcG9pbnRzRGVsdGES'
    'GwoGcmVhc29uGAsgASgJSAJSBnJlYXNvbogBARIhCgxydWxlc2V0X25hbWUYDCABKAlSC3J1bG'
    'VzZXROYW1lEhsKCXNvdXJjZV9pZBgNIAEoCVIIc291cmNlSWQSHwoLc291cmNlX3R5cGUYDiAB'
    'KAlSCnNvdXJjZVR5cGUaQAoSQmFkZ2VFeHBEZWx0YUVudHJ5EhAKA2tleRgBIAEoCVIDa2V5Eh'
    'QKBXZhbHVlGAIgASgDUgV2YWx1ZToCOAFCEQoPX2dhbWVfcmVzdWx0X2lkQgkKB19wZXRfaWRC'
    'CQoHX3JlYXNvbkoECAEQAkoECAYQB1INYWJpbGl0eV9kZWx0YVIKbGlmZV9kZWx0YQ==');

@$core.Deprecated('Use rewardGrantListResponseDescriptor instead')
const RewardGrantListResponse$json = {
  '1': 'RewardGrantListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.RewardGrant',
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
  ],
  '8': [
    {'1': '_next_cursor'},
  ],
};

/// Descriptor for `RewardGrantListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List rewardGrantListResponseDescriptor = $convert.base64Decode(
    'ChdSZXdhcmRHcmFudExpc3RSZXNwb25zZRIZCghoYXNfbmV4dBgBIAEoCFIHaGFzTmV4dBIxCg'
    'VpdGVtcxgCIAMoCzIbLmdpemNsYXcucnBjLnYxLlJld2FyZEdyYW50UgVpdGVtcxIkCgtuZXh0'
    'X2N1cnNvchgDIAEoCUgAUgpuZXh0Q3Vyc29yiAEBQg4KDF9uZXh0X2N1cnNvcg==');

@$core.Deprecated('Use serverBadgeGetRequestDescriptor instead')
const ServerBadgeGetRequest$json = {
  '1': 'ServerBadgeGetRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameplayGetRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerBadgeGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverBadgeGetRequestDescriptor = $convert.base64Decode(
    'ChVTZXJ2ZXJCYWRnZUdldFJlcXVlc3QSOAoFdmFsdWUYASABKAsyIi5naXpjbGF3LnJwYy52MS'
    '5HYW1lcGxheUdldFJlcXVlc3RSBXZhbHVl');

@$core.Deprecated('Use serverBadgeGetResponseDescriptor instead')
const ServerBadgeGetResponse$json = {
  '1': 'ServerBadgeGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Badge',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerBadgeGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverBadgeGetResponseDescriptor =
    $convert.base64Decode(
        'ChZTZXJ2ZXJCYWRnZUdldFJlc3BvbnNlEisKBXZhbHVlGAEgASgLMhUuZ2l6Y2xhdy5ycGMudj'
        'EuQmFkZ2VSBXZhbHVl');

@$core.Deprecated('Use serverBadgeListRequestDescriptor instead')
const ServerBadgeListRequest$json = {
  '1': 'ServerBadgeListRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameplayListRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerBadgeListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverBadgeListRequestDescriptor =
    $convert.base64Decode(
        'ChZTZXJ2ZXJCYWRnZUxpc3RSZXF1ZXN0EjkKBXZhbHVlGAEgASgLMiMuZ2l6Y2xhdy5ycGMudj'
        'EuR2FtZXBsYXlMaXN0UmVxdWVzdFIFdmFsdWU=');

@$core.Deprecated('Use serverBadgeListResponseDescriptor instead')
const ServerBadgeListResponse$json = {
  '1': 'ServerBadgeListResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.BadgeListResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerBadgeListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverBadgeListResponseDescriptor =
    $convert.base64Decode(
        'ChdTZXJ2ZXJCYWRnZUxpc3RSZXNwb25zZRI3CgV2YWx1ZRgBIAEoCzIhLmdpemNsYXcucnBjLn'
        'YxLkJhZGdlTGlzdFJlc3BvbnNlUgV2YWx1ZQ==');

@$core.Deprecated('Use serverGameResultGetRequestDescriptor instead')
const ServerGameResultGetRequest$json = {
  '1': 'ServerGameResultGetRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameplayGetRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGameResultGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGameResultGetRequestDescriptor =
    $convert.base64Decode(
        'ChpTZXJ2ZXJHYW1lUmVzdWx0R2V0UmVxdWVzdBI4CgV2YWx1ZRgBIAEoCzIiLmdpemNsYXcucn'
        'BjLnYxLkdhbWVwbGF5R2V0UmVxdWVzdFIFdmFsdWU=');

@$core.Deprecated('Use serverGameResultGetResponseDescriptor instead')
const ServerGameResultGetResponse$json = {
  '1': 'ServerGameResultGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameResult',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGameResultGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGameResultGetResponseDescriptor =
    $convert.base64Decode(
        'ChtTZXJ2ZXJHYW1lUmVzdWx0R2V0UmVzcG9uc2USMAoFdmFsdWUYASABKAsyGi5naXpjbGF3Ln'
        'JwYy52MS5HYW1lUmVzdWx0UgV2YWx1ZQ==');

@$core.Deprecated('Use serverGameResultListRequestDescriptor instead')
const ServerGameResultListRequest$json = {
  '1': 'ServerGameResultListRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameplayListRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGameResultListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGameResultListRequestDescriptor =
    $convert.base64Decode(
        'ChtTZXJ2ZXJHYW1lUmVzdWx0TGlzdFJlcXVlc3QSOQoFdmFsdWUYASABKAsyIy5naXpjbGF3Ln'
        'JwYy52MS5HYW1lcGxheUxpc3RSZXF1ZXN0UgV2YWx1ZQ==');

@$core.Deprecated('Use serverGameResultListResponseDescriptor instead')
const ServerGameResultListResponse$json = {
  '1': 'ServerGameResultListResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameResultListResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGameResultListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGameResultListResponseDescriptor =
    $convert.base64Decode(
        'ChxTZXJ2ZXJHYW1lUmVzdWx0TGlzdFJlc3BvbnNlEjwKBXZhbHVlGAEgASgLMiYuZ2l6Y2xhdy'
        '5ycGMudjEuR2FtZVJlc3VsdExpc3RSZXNwb25zZVIFdmFsdWU=');

@$core.Deprecated('Use serverGameRulesetGetRequestDescriptor instead')
const ServerGameRulesetGetRequest$json = {
  '1': 'ServerGameRulesetGetRequest',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'name', '17': true},
  ],
  '8': [
    {'1': '_name'},
  ],
};

/// Descriptor for `ServerGameRulesetGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGameRulesetGetRequestDescriptor =
    $convert.base64Decode(
        'ChtTZXJ2ZXJHYW1lUnVsZXNldEdldFJlcXVlc3QSFwoEbmFtZRgBIAEoCUgAUgRuYW1liAEBQg'
        'cKBV9uYW1l');

@$core.Deprecated('Use serverGameRulesetGetResponseDescriptor instead')
const ServerGameRulesetGetResponse$json = {
  '1': 'ServerGameRulesetGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameRuleset',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGameRulesetGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGameRulesetGetResponseDescriptor =
    $convert.base64Decode(
        'ChxTZXJ2ZXJHYW1lUnVsZXNldEdldFJlc3BvbnNlEjEKBXZhbHVlGAEgASgLMhsuZ2l6Y2xhdy'
        '5ycGMudjEuR2FtZVJ1bGVzZXRSBXZhbHVl');

@$core.Deprecated('Use serverPetAdoptRequestDescriptor instead')
const ServerPetAdoptRequest$json = {
  '1': 'ServerPetAdoptRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetAdoptRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetAdoptRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetAdoptRequestDescriptor = $convert.base64Decode(
    'ChVTZXJ2ZXJQZXRBZG9wdFJlcXVlc3QSNQoFdmFsdWUYASABKAsyHy5naXpjbGF3LnJwYy52MS'
    '5QZXRBZG9wdFJlcXVlc3RSBXZhbHVl');

@$core.Deprecated('Use serverPetAdoptResponseDescriptor instead')
const ServerPetAdoptResponse$json = {
  '1': 'ServerPetAdoptResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetAdoptResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetAdoptResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetAdoptResponseDescriptor =
    $convert.base64Decode(
        'ChZTZXJ2ZXJQZXRBZG9wdFJlc3BvbnNlEjYKBXZhbHVlGAEgASgLMiAuZ2l6Y2xhdy5ycGMudj'
        'EuUGV0QWRvcHRSZXNwb25zZVIFdmFsdWU=');

@$core.Deprecated('Use serverPetDeleteRequestDescriptor instead')
const ServerPetDeleteRequest$json = {
  '1': 'ServerPetDeleteRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetDeleteRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetDeleteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetDeleteRequestDescriptor =
    $convert.base64Decode(
        'ChZTZXJ2ZXJQZXREZWxldGVSZXF1ZXN0EjYKBXZhbHVlGAEgASgLMiAuZ2l6Y2xhdy5ycGMudj'
        'EuUGV0RGVsZXRlUmVxdWVzdFIFdmFsdWU=');

@$core.Deprecated('Use serverPetDeleteResponseDescriptor instead')
const ServerPetDeleteResponse$json = {
  '1': 'ServerPetDeleteResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Pet',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetDeleteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetDeleteResponseDescriptor =
    $convert.base64Decode(
        'ChdTZXJ2ZXJQZXREZWxldGVSZXNwb25zZRIpCgV2YWx1ZRgBIAEoCzITLmdpemNsYXcucnBjLn'
        'YxLlBldFIFdmFsdWU=');

@$core.Deprecated('Use serverPetDriveRequestDescriptor instead')
const ServerPetDriveRequest$json = {
  '1': 'ServerPetDriveRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetDriveRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetDriveRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetDriveRequestDescriptor = $convert.base64Decode(
    'ChVTZXJ2ZXJQZXREcml2ZVJlcXVlc3QSNQoFdmFsdWUYASABKAsyHy5naXpjbGF3LnJwYy52MS'
    '5QZXREcml2ZVJlcXVlc3RSBXZhbHVl');

@$core.Deprecated('Use serverPetDriveResponseDescriptor instead')
const ServerPetDriveResponse$json = {
  '1': 'ServerPetDriveResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetDriveResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetDriveResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetDriveResponseDescriptor =
    $convert.base64Decode(
        'ChZTZXJ2ZXJQZXREcml2ZVJlc3BvbnNlEjYKBXZhbHVlGAEgASgLMiAuZ2l6Y2xhdy5ycGMudj'
        'EuUGV0RHJpdmVSZXNwb25zZVIFdmFsdWU=');

@$core.Deprecated('Use serverPetGetRequestDescriptor instead')
const ServerPetGetRequest$json = {
  '1': 'ServerPetGetRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetGetRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetGetRequestDescriptor = $convert.base64Decode(
    'ChNTZXJ2ZXJQZXRHZXRSZXF1ZXN0EjMKBXZhbHVlGAEgASgLMh0uZ2l6Y2xhdy5ycGMudjEuUG'
    'V0R2V0UmVxdWVzdFIFdmFsdWU=');

@$core.Deprecated('Use serverPetGetResponseDescriptor instead')
const ServerPetGetResponse$json = {
  '1': 'ServerPetGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Pet',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetGetResponseDescriptor = $convert.base64Decode(
    'ChRTZXJ2ZXJQZXRHZXRSZXNwb25zZRIpCgV2YWx1ZRgBIAEoCzITLmdpemNsYXcucnBjLnYxLl'
    'BldFIFdmFsdWU=');

@$core.Deprecated('Use serverPetPixaDownloadRequestDescriptor instead')
const ServerPetPixaDownloadRequest$json = {
  '1': 'ServerPetPixaDownloadRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPixaDownloadRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetPixaDownloadRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetPixaDownloadRequestDescriptor =
    $convert.base64Decode(
        'ChxTZXJ2ZXJQZXRQaXhhRG93bmxvYWRSZXF1ZXN0EjwKBXZhbHVlGAEgASgLMiYuZ2l6Y2xhdy'
        '5ycGMudjEuUGV0UGl4YURvd25sb2FkUmVxdWVzdFIFdmFsdWU=');

@$core.Deprecated('Use serverPetPixaDownloadResponseDescriptor instead')
const ServerPetPixaDownloadResponse$json = {
  '1': 'ServerPetPixaDownloadResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPixaDownloadResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetPixaDownloadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetPixaDownloadResponseDescriptor =
    $convert.base64Decode(
        'Ch1TZXJ2ZXJQZXRQaXhhRG93bmxvYWRSZXNwb25zZRI9CgV2YWx1ZRgBIAEoCzInLmdpemNsYX'
        'cucnBjLnYxLlBldFBpeGFEb3dubG9hZFJlc3BvbnNlUgV2YWx1ZQ==');

@$core.Deprecated('Use serverPetPresentationGetRequestDescriptor instead')
const ServerPetPresentationGetRequest$json = {
  '1': 'ServerPetPresentationGetRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetGetRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetPresentationGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetPresentationGetRequestDescriptor =
    $convert.base64Decode(
        'Ch9TZXJ2ZXJQZXRQcmVzZW50YXRpb25HZXRSZXF1ZXN0EjMKBXZhbHVlGAEgASgLMh0uZ2l6Y2'
        'xhdy5ycGMudjEuUGV0R2V0UmVxdWVzdFIFdmFsdWU=');

@$core.Deprecated('Use serverPetPresentationGetResponseDescriptor instead')
const ServerPetPresentationGetResponse$json = {
  '1': 'ServerPetPresentationGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPresentation',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetPresentationGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetPresentationGetResponseDescriptor =
    $convert.base64Decode(
        'CiBTZXJ2ZXJQZXRQcmVzZW50YXRpb25HZXRSZXNwb25zZRI1CgV2YWx1ZRgBIAEoCzIfLmdpem'
        'NsYXcucnBjLnYxLlBldFByZXNlbnRhdGlvblIFdmFsdWU=');

@$core.Deprecated('Use serverPetListRequestDescriptor instead')
const ServerPetListRequest$json = {
  '1': 'ServerPetListRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameplayListRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetListRequestDescriptor = $convert.base64Decode(
    'ChRTZXJ2ZXJQZXRMaXN0UmVxdWVzdBI5CgV2YWx1ZRgBIAEoCzIjLmdpemNsYXcucnBjLnYxLk'
    'dhbWVwbGF5TGlzdFJlcXVlc3RSBXZhbHVl');

@$core.Deprecated('Use serverPetListResponseDescriptor instead')
const ServerPetListResponse$json = {
  '1': 'ServerPetListResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetListResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetListResponseDescriptor = $convert.base64Decode(
    'ChVTZXJ2ZXJQZXRMaXN0UmVzcG9uc2USNQoFdmFsdWUYASABKAsyHy5naXpjbGF3LnJwYy52MS'
    '5QZXRMaXN0UmVzcG9uc2VSBXZhbHVl');

@$core.Deprecated('Use serverPetPutRequestDescriptor instead')
const ServerPetPutRequest$json = {
  '1': 'ServerPetPutRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPutRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetPutRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetPutRequestDescriptor = $convert.base64Decode(
    'ChNTZXJ2ZXJQZXRQdXRSZXF1ZXN0EjMKBXZhbHVlGAEgASgLMh0uZ2l6Y2xhdy5ycGMudjEuUG'
    'V0UHV0UmVxdWVzdFIFdmFsdWU=');

@$core.Deprecated('Use serverPetPutResponseDescriptor instead')
const ServerPetPutResponse$json = {
  '1': 'ServerPetPutResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Pet',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPetPutResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPetPutResponseDescriptor = $convert.base64Decode(
    'ChRTZXJ2ZXJQZXRQdXRSZXNwb25zZRIpCgV2YWx1ZRgBIAEoCzITLmdpemNsYXcucnBjLnYxLl'
    'BldFIFdmFsdWU=');

@$core.Deprecated('Use serverPointsGetRequestDescriptor instead')
const ServerPointsGetRequest$json = {
  '1': 'ServerPointsGetRequest',
  '2': [
    {
      '1': 'ruleset_name',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'rulesetName',
      '17': true
    },
  ],
  '8': [
    {'1': '_ruleset_name'},
  ],
};

/// Descriptor for `ServerPointsGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPointsGetRequestDescriptor =
    $convert.base64Decode(
        'ChZTZXJ2ZXJQb2ludHNHZXRSZXF1ZXN0EiYKDHJ1bGVzZXRfbmFtZRgBIAEoCUgAUgtydWxlc2'
        'V0TmFtZYgBAUIPCg1fcnVsZXNldF9uYW1l');

@$core.Deprecated('Use serverPointsGetResponseDescriptor instead')
const ServerPointsGetResponse$json = {
  '1': 'ServerPointsGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PointsAccount',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPointsGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPointsGetResponseDescriptor =
    $convert.base64Decode(
        'ChdTZXJ2ZXJQb2ludHNHZXRSZXNwb25zZRIzCgV2YWx1ZRgBIAEoCzIdLmdpemNsYXcucnBjLn'
        'YxLlBvaW50c0FjY291bnRSBXZhbHVl');

@$core.Deprecated('Use serverPointsTransactionGetRequestDescriptor instead')
const ServerPointsTransactionGetRequest$json = {
  '1': 'ServerPointsTransactionGetRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameplayGetRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPointsTransactionGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPointsTransactionGetRequestDescriptor =
    $convert.base64Decode(
        'CiFTZXJ2ZXJQb2ludHNUcmFuc2FjdGlvbkdldFJlcXVlc3QSOAoFdmFsdWUYASABKAsyIi5naX'
        'pjbGF3LnJwYy52MS5HYW1lcGxheUdldFJlcXVlc3RSBXZhbHVl');

@$core.Deprecated('Use serverPointsTransactionGetResponseDescriptor instead')
const ServerPointsTransactionGetResponse$json = {
  '1': 'ServerPointsTransactionGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PointsTransaction',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPointsTransactionGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPointsTransactionGetResponseDescriptor =
    $convert.base64Decode(
        'CiJTZXJ2ZXJQb2ludHNUcmFuc2FjdGlvbkdldFJlc3BvbnNlEjcKBXZhbHVlGAEgASgLMiEuZ2'
        'l6Y2xhdy5ycGMudjEuUG9pbnRzVHJhbnNhY3Rpb25SBXZhbHVl');

@$core.Deprecated('Use serverPointsTransactionListRequestDescriptor instead')
const ServerPointsTransactionListRequest$json = {
  '1': 'ServerPointsTransactionListRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameplayListRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPointsTransactionListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPointsTransactionListRequestDescriptor =
    $convert.base64Decode(
        'CiJTZXJ2ZXJQb2ludHNUcmFuc2FjdGlvbkxpc3RSZXF1ZXN0EjkKBXZhbHVlGAEgASgLMiMuZ2'
        'l6Y2xhdy5ycGMudjEuR2FtZXBsYXlMaXN0UmVxdWVzdFIFdmFsdWU=');

@$core.Deprecated('Use serverPointsTransactionListResponseDescriptor instead')
const ServerPointsTransactionListResponse$json = {
  '1': 'ServerPointsTransactionListResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PointsTransactionListResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPointsTransactionListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPointsTransactionListResponseDescriptor =
    $convert.base64Decode(
        'CiNTZXJ2ZXJQb2ludHNUcmFuc2FjdGlvbkxpc3RSZXNwb25zZRJDCgV2YWx1ZRgBIAEoCzItLm'
        'dpemNsYXcucnBjLnYxLlBvaW50c1RyYW5zYWN0aW9uTGlzdFJlc3BvbnNlUgV2YWx1ZQ==');

@$core.Deprecated('Use serverRewardGrantGetRequestDescriptor instead')
const ServerRewardGrantGetRequest$json = {
  '1': 'ServerRewardGrantGetRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameplayGetRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerRewardGrantGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRewardGrantGetRequestDescriptor =
    $convert.base64Decode(
        'ChtTZXJ2ZXJSZXdhcmRHcmFudEdldFJlcXVlc3QSOAoFdmFsdWUYASABKAsyIi5naXpjbGF3Ln'
        'JwYy52MS5HYW1lcGxheUdldFJlcXVlc3RSBXZhbHVl');

@$core.Deprecated('Use serverRewardGrantGetResponseDescriptor instead')
const ServerRewardGrantGetResponse$json = {
  '1': 'ServerRewardGrantGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.RewardGrant',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerRewardGrantGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRewardGrantGetResponseDescriptor =
    $convert.base64Decode(
        'ChxTZXJ2ZXJSZXdhcmRHcmFudEdldFJlc3BvbnNlEjEKBXZhbHVlGAEgASgLMhsuZ2l6Y2xhdy'
        '5ycGMudjEuUmV3YXJkR3JhbnRSBXZhbHVl');

@$core.Deprecated('Use serverRewardGrantListRequestDescriptor instead')
const ServerRewardGrantListRequest$json = {
  '1': 'ServerRewardGrantListRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameplayListRequest',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerRewardGrantListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRewardGrantListRequestDescriptor =
    $convert.base64Decode(
        'ChxTZXJ2ZXJSZXdhcmRHcmFudExpc3RSZXF1ZXN0EjkKBXZhbHVlGAEgASgLMiMuZ2l6Y2xhdy'
        '5ycGMudjEuR2FtZXBsYXlMaXN0UmVxdWVzdFIFdmFsdWU=');

@$core.Deprecated('Use serverRewardGrantListResponseDescriptor instead')
const ServerRewardGrantListResponse$json = {
  '1': 'ServerRewardGrantListResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.RewardGrantListResponse',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerRewardGrantListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRewardGrantListResponseDescriptor =
    $convert.base64Decode(
        'Ch1TZXJ2ZXJSZXdhcmRHcmFudExpc3RSZXNwb25zZRI9CgV2YWx1ZRgBIAEoCzInLmdpemNsYX'
        'cucnBjLnYxLlJld2FyZEdyYW50TGlzdFJlc3BvbnNlUgV2YWx1ZQ==');

@$core.Deprecated('Use petLifeDescriptor instead')
const PetLife$json = {
  '1': 'PetLife',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetLife.ValueEntry',
      '10': 'value'
    },
  ],
  '3': [PetLife_ValueEntry$json],
};

@$core.Deprecated('Use petLifeDescriptor instead')
const PetLife_ValueEntry$json = {
  '1': 'ValueEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 3, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `PetLife`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petLifeDescriptor = $convert.base64Decode(
    'CgdQZXRMaWZlEjgKBXZhbHVlGAEgAygLMiIuZ2l6Y2xhdy5ycGMudjEuUGV0TGlmZS5WYWx1ZU'
    'VudHJ5UgV2YWx1ZRo4CgpWYWx1ZUVudHJ5EhAKA2tleRgBIAEoCVIDa2V5EhQKBXZhbHVlGAIg'
    'ASgDUgV2YWx1ZToCOAE=');

@$core.Deprecated('Use petProgressionDescriptor instead')
const PetProgression$json = {
  '1': 'PetProgression',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetProgression.ValueEntry',
      '10': 'value'
    },
  ],
  '3': [PetProgression_ValueEntry$json],
};

@$core.Deprecated('Use petProgressionDescriptor instead')
const PetProgression_ValueEntry$json = {
  '1': 'ValueEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 3, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `PetProgression`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petProgressionDescriptor = $convert.base64Decode(
    'Cg5QZXRQcm9ncmVzc2lvbhI/CgV2YWx1ZRgBIAMoCzIpLmdpemNsYXcucnBjLnYxLlBldFByb2'
    'dyZXNzaW9uLlZhbHVlRW50cnlSBXZhbHVlGjgKClZhbHVlRW50cnkSEAoDa2V5GAEgASgJUgNr'
    'ZXkSFAoFdmFsdWUYAiABKANSBXZhbHVlOgI4AQ==');
