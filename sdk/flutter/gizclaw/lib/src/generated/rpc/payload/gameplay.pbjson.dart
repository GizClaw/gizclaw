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
      '1': 'ability_delta',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.StatMap',
      '9': 0,
      '10': 'abilityDelta',
      '17': true
    },
    {
      '1': 'badge_exp_delta',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameRewardSpec.BadgeExpDeltaEntry',
      '10': 'badgeExpDelta'
    },
    {
      '1': 'life_delta',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.StatMap',
      '9': 1,
      '10': 'lifeDelta',
      '17': true
    },
    {
      '1': 'pet_exp_delta',
      '3': 4,
      '4': 1,
      '5': 3,
      '9': 2,
      '10': 'petExpDelta',
      '17': true
    },
    {
      '1': 'points_delta',
      '3': 5,
      '4': 1,
      '5': 3,
      '9': 3,
      '10': 'pointsDelta',
      '17': true
    },
  ],
  '3': [GameRewardSpec_BadgeExpDeltaEntry$json],
  '8': [
    {'1': '_ability_delta'},
    {'1': '_life_delta'},
    {'1': '_pet_exp_delta'},
    {'1': '_points_delta'},
  ],
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
    'Cg5HYW1lUmV3YXJkU3BlYxJBCg1hYmlsaXR5X2RlbHRhGAEgASgLMhcuZ2l6Y2xhdy5ycGMudj'
    'EuU3RhdE1hcEgAUgxhYmlsaXR5RGVsdGGIAQESWQoPYmFkZ2VfZXhwX2RlbHRhGAIgAygLMjEu'
    'Z2l6Y2xhdy5ycGMudjEuR2FtZVJld2FyZFNwZWMuQmFkZ2VFeHBEZWx0YUVudHJ5Ug1iYWRnZU'
    'V4cERlbHRhEjsKCmxpZmVfZGVsdGEYAyABKAsyFy5naXpjbGF3LnJwYy52MS5TdGF0TWFwSAFS'
    'CWxpZmVEZWx0YYgBARInCg1wZXRfZXhwX2RlbHRhGAQgASgDSAJSC3BldEV4cERlbHRhiAEBEi'
    'YKDHBvaW50c19kZWx0YRgFIAEoA0gDUgtwb2ludHNEZWx0YYgBARpAChJCYWRnZUV4cERlbHRh'
    'RW50cnkSEAoDa2V5GAEgASgJUgNrZXkSFAoFdmFsdWUYAiABKANSBXZhbHVlOgI4AUIQCg5fYW'
    'JpbGl0eV9kZWx0YUINCgtfbGlmZV9kZWx0YUIQCg5fcGV0X2V4cF9kZWx0YUIPCg1fcG9pbnRz'
    'X2RlbHRh');

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
      '1': 'action_costs',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameRulesetDriveSpec.ActionCostsEntry',
      '10': 'actionCosts'
    },
    {
      '1': 'action_rewards',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GameRulesetDriveSpec.ActionRewardsEntry',
      '10': 'actionRewards'
    },
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
    {
      '1': 'life_decay_per_hour',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.StatMap',
      '9': 1,
      '10': 'lifeDecayPerHour',
      '17': true
    },
  ],
  '3': [
    GameRulesetDriveSpec_ActionCostsEntry$json,
    GameRulesetDriveSpec_ActionRewardsEntry$json,
    GameRulesetDriveSpec_GameRewardsEntry$json
  ],
  '8': [
    {'1': '_default_reward'},
    {'1': '_life_decay_per_hour'},
  ],
};

@$core.Deprecated('Use gameRulesetDriveSpecDescriptor instead')
const GameRulesetDriveSpec_ActionCostsEntry$json = {
  '1': 'ActionCostsEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 3, '10': 'value'},
  ],
  '7': {'7': true},
};

@$core.Deprecated('Use gameRulesetDriveSpecDescriptor instead')
const GameRulesetDriveSpec_ActionRewardsEntry$json = {
  '1': 'ActionRewardsEntry',
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
    'ChRHYW1lUnVsZXNldERyaXZlU3BlYxJYCgxhY3Rpb25fY29zdHMYASADKAsyNS5naXpjbGF3Ln'
    'JwYy52MS5HYW1lUnVsZXNldERyaXZlU3BlYy5BY3Rpb25Db3N0c0VudHJ5UgthY3Rpb25Db3N0'
    'cxJeCg5hY3Rpb25fcmV3YXJkcxgCIAMoCzI3LmdpemNsYXcucnBjLnYxLkdhbWVSdWxlc2V0RH'
    'JpdmVTcGVjLkFjdGlvblJld2FyZHNFbnRyeVINYWN0aW9uUmV3YXJkcxJKCg5kZWZhdWx0X3Jl'
    'd2FyZBgDIAEoCzIeLmdpemNsYXcucnBjLnYxLkdhbWVSZXdhcmRTcGVjSABSDWRlZmF1bHRSZX'
    'dhcmSIAQESWAoMZ2FtZV9yZXdhcmRzGAQgAygLMjUuZ2l6Y2xhdy5ycGMudjEuR2FtZVJ1bGVz'
    'ZXREcml2ZVNwZWMuR2FtZVJld2FyZHNFbnRyeVILZ2FtZVJld2FyZHMSSwoTbGlmZV9kZWNheV'
    '9wZXJfaG91chgFIAEoCzIXLmdpemNsYXcucnBjLnYxLlN0YXRNYXBIAVIQbGlmZURlY2F5UGVy'
    'SG91cogBARo+ChBBY3Rpb25Db3N0c0VudHJ5EhAKA2tleRgBIAEoCVIDa2V5EhQKBXZhbHVlGA'
    'IgASgDUgV2YWx1ZToCOAEaYAoSQWN0aW9uUmV3YXJkc0VudHJ5EhAKA2tleRgBIAEoCVIDa2V5'
    'EjQKBXZhbHVlGAIgASgLMh4uZ2l6Y2xhdy5ycGMudjEuR2FtZVJld2FyZFNwZWNSBXZhbHVlOg'
    'I4ARpeChBHYW1lUmV3YXJkc0VudHJ5EhAKA2tleRgBIAEoCVIDa2V5EjQKBXZhbHVlGAIgASgL'
    'Mh4uZ2l6Y2xhdy5ycGMudjEuR2FtZVJld2FyZFNwZWNSBXZhbHVlOgI4AUIRCg9fZGVmYXVsdF'
    '9yZXdhcmRCFgoUX2xpZmVfZGVjYXlfcGVyX2hvdXI=');

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
    {
      '1': 'ability',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.StatMap',
      '10': 'ability'
    },
    {'1': 'created_at', '3': 2, '4': 1, '5': 9, '10': 'createdAt'},
    {'1': 'display_name', '3': 3, '4': 1, '5': 9, '10': 'displayName'},
    {'1': 'exp', '3': 4, '4': 1, '5': 3, '10': 'exp'},
    {'1': 'id', '3': 5, '4': 1, '5': 9, '10': 'id'},
    {'1': 'last_active_at', '3': 6, '4': 1, '5': 9, '10': 'lastActiveAt'},
    {'1': 'level', '3': 7, '4': 1, '5': 3, '10': 'level'},
    {
      '1': 'life',
      '3': 8,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.StatMap',
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
  ],
  '8': [
    {'1': '_workflow_name'},
  ],
};

/// Descriptor for `Pet`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petDescriptor = $convert.base64Decode(
    'CgNQZXQSMQoHYWJpbGl0eRgBIAEoCzIXLmdpemNsYXcucnBjLnYxLlN0YXRNYXBSB2FiaWxpdH'
    'kSHQoKY3JlYXRlZF9hdBgCIAEoCVIJY3JlYXRlZEF0EiEKDGRpc3BsYXlfbmFtZRgDIAEoCVIL'
    'ZGlzcGxheU5hbWUSEAoDZXhwGAQgASgDUgNleHASDgoCaWQYBSABKAlSAmlkEiQKDmxhc3RfYW'
    'N0aXZlX2F0GAYgASgJUgxsYXN0QWN0aXZlQXQSFAoFbGV2ZWwYByABKANSBWxldmVsEisKBGxp'
    'ZmUYCCABKAsyFy5naXpjbGF3LnJwYy52MS5TdGF0TWFwUgRsaWZlEigKEG93bmVyX3B1YmxpY1'
    '9rZXkYCSABKAlSDm93bmVyUHVibGljS2V5EhsKCXBldGRlZl9pZBgKIAEoCVIIcGV0ZGVmSWQS'
    'IQoMcnVsZXNldF9uYW1lGAsgASgJUgtydWxlc2V0TmFtZRIdCgp1cGRhdGVkX2F0GAwgASgJUg'
    'l1cGRhdGVkQXQSKAoNd29ya2Zsb3dfbmFtZRgNIAEoCUgAUgx3b3JrZmxvd05hbWWIAQESJQoO'
    'd29ya3NwYWNlX25hbWUYDiABKAlSDXdvcmtzcGFjZU5hbWVCEAoOX3dvcmtmbG93X25hbWU=');

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
      '1': 'ability_delta',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.StatMap',
      '9': 0,
      '10': 'abilityDelta',
      '17': true
    },
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
      '9': 1,
      '10': 'gameResultId',
      '17': true
    },
    {'1': 'id', '3': 5, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'life_delta',
      '3': 6,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.StatMap',
      '9': 2,
      '10': 'lifeDelta',
      '17': true
    },
    {'1': 'owner_public_key', '3': 7, '4': 1, '5': 9, '10': 'ownerPublicKey'},
    {'1': 'pet_exp_delta', '3': 8, '4': 1, '5': 3, '10': 'petExpDelta'},
    {'1': 'pet_id', '3': 9, '4': 1, '5': 9, '9': 3, '10': 'petId', '17': true},
    {'1': 'points_delta', '3': 10, '4': 1, '5': 3, '10': 'pointsDelta'},
    {
      '1': 'reason',
      '3': 11,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'reason',
      '17': true
    },
    {'1': 'ruleset_name', '3': 12, '4': 1, '5': 9, '10': 'rulesetName'},
    {'1': 'source_id', '3': 13, '4': 1, '5': 9, '10': 'sourceId'},
    {'1': 'source_type', '3': 14, '4': 1, '5': 9, '10': 'sourceType'},
  ],
  '3': [RewardGrant_BadgeExpDeltaEntry$json],
  '8': [
    {'1': '_ability_delta'},
    {'1': '_game_result_id'},
    {'1': '_life_delta'},
    {'1': '_pet_id'},
    {'1': '_reason'},
  ],
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
    'CgtSZXdhcmRHcmFudBJBCg1hYmlsaXR5X2RlbHRhGAEgASgLMhcuZ2l6Y2xhdy5ycGMudjEuU3'
    'RhdE1hcEgAUgxhYmlsaXR5RGVsdGGIAQESVgoPYmFkZ2VfZXhwX2RlbHRhGAIgAygLMi4uZ2l6'
    'Y2xhdy5ycGMudjEuUmV3YXJkR3JhbnQuQmFkZ2VFeHBEZWx0YUVudHJ5Ug1iYWRnZUV4cERlbH'
    'RhEh0KCmNyZWF0ZWRfYXQYAyABKAlSCWNyZWF0ZWRBdBIpCg5nYW1lX3Jlc3VsdF9pZBgEIAEo'
    'CUgBUgxnYW1lUmVzdWx0SWSIAQESDgoCaWQYBSABKAlSAmlkEjsKCmxpZmVfZGVsdGEYBiABKA'
    'syFy5naXpjbGF3LnJwYy52MS5TdGF0TWFwSAJSCWxpZmVEZWx0YYgBARIoChBvd25lcl9wdWJs'
    'aWNfa2V5GAcgASgJUg5vd25lclB1YmxpY0tleRIiCg1wZXRfZXhwX2RlbHRhGAggASgDUgtwZX'
    'RFeHBEZWx0YRIaCgZwZXRfaWQYCSABKAlIA1IFcGV0SWSIAQESIQoMcG9pbnRzX2RlbHRhGAog'
    'ASgDUgtwb2ludHNEZWx0YRIbCgZyZWFzb24YCyABKAlIBFIGcmVhc29uiAEBEiEKDHJ1bGVzZX'
    'RfbmFtZRgMIAEoCVILcnVsZXNldE5hbWUSGwoJc291cmNlX2lkGA0gASgJUghzb3VyY2VJZBIf'
    'Cgtzb3VyY2VfdHlwZRgOIAEoCVIKc291cmNlVHlwZRpAChJCYWRnZUV4cERlbHRhRW50cnkSEA'
    'oDa2V5GAEgASgJUgNrZXkSFAoFdmFsdWUYAiABKANSBXZhbHVlOgI4AUIQCg5fYWJpbGl0eV9k'
    'ZWx0YUIRCg9fZ2FtZV9yZXN1bHRfaWRCDQoLX2xpZmVfZGVsdGFCCQoHX3BldF9pZEIJCgdfcm'
    'Vhc29u');

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

@$core.Deprecated('Use statMapDescriptor instead')
const StatMap$json = {
  '1': 'StatMap',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.StatMap.ValueEntry',
      '10': 'value'
    },
  ],
  '3': [StatMap_ValueEntry$json],
};

@$core.Deprecated('Use statMapDescriptor instead')
const StatMap_ValueEntry$json = {
  '1': 'ValueEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 3, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `StatMap`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List statMapDescriptor = $convert.base64Decode(
    'CgdTdGF0TWFwEjgKBXZhbHVlGAEgAygLMiIuZ2l6Y2xhdy5ycGMudjEuU3RhdE1hcC5WYWx1ZU'
    'VudHJ5UgV2YWx1ZRo4CgpWYWx1ZUVudHJ5EhAKA2tleRgBIAEoCVIDa2V5EhQKBXZhbHVlGAIg'
    'ASgDUgV2YWx1ZToCOAE=');
