// This is a generated file - do not edit.
//
// Generated from payload/ai.proto.

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

@$core.Deprecated('Use aSTTranslateExternalVoiceParametersDescriptor instead')
const ASTTranslateExternalVoiceParameters$json = {
  '1': 'ASTTranslateExternalVoiceParameters',
  '2': [
    {'1': 'tts_voice', '3': 1, '4': 1, '5': 9, '10': 'ttsVoice'},
  ],
};

/// Descriptor for `ASTTranslateExternalVoiceParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aSTTranslateExternalVoiceParametersDescriptor =
    $convert.base64Decode(
        'CiNBU1RUcmFuc2xhdGVFeHRlcm5hbFZvaWNlUGFyYW1ldGVycxIbCgl0dHNfdm9pY2UYASABKA'
        'lSCHR0c1ZvaWNl');

@$core.Deprecated('Use aSTTranslateInternalSpeakerParametersDescriptor instead')
const ASTTranslateInternalSpeakerParameters$json = {
  '1': 'ASTTranslateInternalSpeakerParameters',
  '2': [
    {
      '1': 'is_custom_speaker',
      '3': 1,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'isCustomSpeaker',
      '17': true
    },
    {'1': 'speaker_id', '3': 2, '4': 1, '5': 9, '10': 'speakerId'},
    {
      '1': 'speech_rate',
      '3': 3,
      '4': 1,
      '5': 3,
      '9': 1,
      '10': 'speechRate',
      '17': true
    },
    {
      '1': 'tts_resource_id',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'ttsResourceId',
      '17': true
    },
  ],
  '8': [
    {'1': '_is_custom_speaker'},
    {'1': '_speech_rate'},
    {'1': '_tts_resource_id'},
  ],
};

/// Descriptor for `ASTTranslateInternalSpeakerParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aSTTranslateInternalSpeakerParametersDescriptor =
    $convert.base64Decode(
        'CiVBU1RUcmFuc2xhdGVJbnRlcm5hbFNwZWFrZXJQYXJhbWV0ZXJzEi8KEWlzX2N1c3RvbV9zcG'
        'Vha2VyGAEgASgISABSD2lzQ3VzdG9tU3BlYWtlcogBARIdCgpzcGVha2VyX2lkGAIgASgJUglz'
        'cGVha2VySWQSJAoLc3BlZWNoX3JhdGUYAyABKANIAVIKc3BlZWNoUmF0ZYgBARIrCg90dHNfcm'
        'Vzb3VyY2VfaWQYBCABKAlIAlINdHRzUmVzb3VyY2VJZIgBAUIUChJfaXNfY3VzdG9tX3NwZWFr'
        'ZXJCDgoMX3NwZWVjaF9yYXRlQhIKEF90dHNfcmVzb3VyY2VfaWQ=');

@$core.Deprecated('Use aSTTranslateVoiceParametersDescriptor instead')
const ASTTranslateVoiceParameters$json = {
  '1': 'ASTTranslateVoiceParameters',
  '2': [
    {
      '1': 'asttranslate_internal_speaker_parameters',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ASTTranslateInternalSpeakerParameters',
      '9': 0,
      '10': 'asttranslateInternalSpeakerParameters'
    },
    {
      '1': 'asttranslate_external_voice_parameters',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ASTTranslateExternalVoiceParameters',
      '9': 0,
      '10': 'asttranslateExternalVoiceParameters'
    },
  ],
  '8': [
    {'1': 'value'},
  ],
};

/// Descriptor for `ASTTranslateVoiceParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aSTTranslateVoiceParametersDescriptor = $convert.base64Decode(
    'ChtBU1RUcmFuc2xhdGVWb2ljZVBhcmFtZXRlcnMSkAEKKGFzdHRyYW5zbGF0ZV9pbnRlcm5hbF'
    '9zcGVha2VyX3BhcmFtZXRlcnMYASABKAsyNS5naXpjbGF3LnJwYy52MS5BU1RUcmFuc2xhdGVJ'
    'bnRlcm5hbFNwZWFrZXJQYXJhbWV0ZXJzSABSJWFzdHRyYW5zbGF0ZUludGVybmFsU3BlYWtlcl'
    'BhcmFtZXRlcnMSigEKJmFzdHRyYW5zbGF0ZV9leHRlcm5hbF92b2ljZV9wYXJhbWV0ZXJzGAIg'
    'ASgLMjMuZ2l6Y2xhdy5ycGMudjEuQVNUVHJhbnNsYXRlRXh0ZXJuYWxWb2ljZVBhcmFtZXRlcn'
    'NIAFIjYXN0dHJhbnNsYXRlRXh0ZXJuYWxWb2ljZVBhcmFtZXRlcnNCBwoFdmFsdWU=');

@$core.Deprecated('Use aSTTranslateWorkflowSpecDescriptor instead')
const ASTTranslateWorkflowSpec$json = {
  '1': 'ASTTranslateWorkflowSpec',
  '2': [
    {
      '1': 'denoise',
      '3': 1,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'denoise',
      '17': true
    },
    {
      '1': 'enable_source_language_detect',
      '3': 2,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'enableSourceLanguageDetect',
      '17': true
    },
    {
      '1': 'is_custom_speaker',
      '3': 3,
      '4': 1,
      '5': 8,
      '9': 2,
      '10': 'isCustomSpeaker',
      '17': true
    },
    {
      '1': 'mode',
      '3': 4,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ASTTranslateMode',
      '9': 3,
      '10': 'mode',
      '17': true
    },
    {
      '1': 'resource_id',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'resourceId',
      '17': true
    },
    {
      '1': 'speaker_id',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 5,
      '10': 'speakerId',
      '17': true
    },
    {
      '1': 'speech_rate',
      '3': 7,
      '4': 1,
      '5': 3,
      '9': 6,
      '10': 'speechRate',
      '17': true
    },
    {
      '1': 'translation_model',
      '3': 8,
      '4': 1,
      '5': 9,
      '10': 'translationModel'
    },
    {
      '1': 'tts_resource_id',
      '3': 9,
      '4': 1,
      '5': 9,
      '9': 7,
      '10': 'ttsResourceId',
      '17': true
    },
    {
      '1': 'voice',
      '3': 10,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ASTTranslateVoiceParameters',
      '9': 8,
      '10': 'voice',
      '17': true
    },
  ],
  '8': [
    {'1': '_denoise'},
    {'1': '_enable_source_language_detect'},
    {'1': '_is_custom_speaker'},
    {'1': '_mode'},
    {'1': '_resource_id'},
    {'1': '_speaker_id'},
    {'1': '_speech_rate'},
    {'1': '_tts_resource_id'},
    {'1': '_voice'},
  ],
};

/// Descriptor for `ASTTranslateWorkflowSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aSTTranslateWorkflowSpecDescriptor = $convert.base64Decode(
    'ChhBU1RUcmFuc2xhdGVXb3JrZmxvd1NwZWMSHQoHZGVub2lzZRgBIAEoCEgAUgdkZW5vaXNliA'
    'EBEkYKHWVuYWJsZV9zb3VyY2VfbGFuZ3VhZ2VfZGV0ZWN0GAIgASgISAFSGmVuYWJsZVNvdXJj'
    'ZUxhbmd1YWdlRGV0ZWN0iAEBEi8KEWlzX2N1c3RvbV9zcGVha2VyGAMgASgISAJSD2lzQ3VzdG'
    '9tU3BlYWtlcogBARI5CgRtb2RlGAQgASgOMiAuZ2l6Y2xhdy5ycGMudjEuQVNUVHJhbnNsYXRl'
    'TW9kZUgDUgRtb2RliAEBEiQKC3Jlc291cmNlX2lkGAUgASgJSARSCnJlc291cmNlSWSIAQESIg'
    'oKc3BlYWtlcl9pZBgGIAEoCUgFUglzcGVha2VySWSIAQESJAoLc3BlZWNoX3JhdGUYByABKANI'
    'BlIKc3BlZWNoUmF0ZYgBARIrChF0cmFuc2xhdGlvbl9tb2RlbBgIIAEoCVIQdHJhbnNsYXRpb2'
    '5Nb2RlbBIrCg90dHNfcmVzb3VyY2VfaWQYCSABKAlIB1INdHRzUmVzb3VyY2VJZIgBARJGCgV2'
    'b2ljZRgKIAEoCzIrLmdpemNsYXcucnBjLnYxLkFTVFRyYW5zbGF0ZVZvaWNlUGFyYW1ldGVyc0'
    'gIUgV2b2ljZYgBAUIKCghfZGVub2lzZUIgCh5fZW5hYmxlX3NvdXJjZV9sYW5ndWFnZV9kZXRl'
    'Y3RCFAoSX2lzX2N1c3RvbV9zcGVha2VyQgcKBV9tb2RlQg4KDF9yZXNvdXJjZV9pZEINCgtfc3'
    'BlYWtlcl9pZEIOCgxfc3BlZWNoX3JhdGVCEgoQX3R0c19yZXNvdXJjZV9pZEIICgZfdm9pY2U=');

@$core.Deprecated('Use aSTTranslateWorkspaceParametersDescriptor instead')
const ASTTranslateWorkspaceParameters$json = {
  '1': 'ASTTranslateWorkspaceParameters',
  '2': [
    {
      '1': 'agent_type',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ASTTranslateWorkspaceParametersAgentType',
      '10': 'agentType'
    },
    {
      '1': 'denoise',
      '3': 2,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'denoise',
      '17': true
    },
    {'1': 'e2e', '3': 3, '4': 1, '5': 8, '9': 1, '10': 'e2e', '17': true},
    {
      '1': 'enable_source_language_detect',
      '3': 4,
      '4': 1,
      '5': 8,
      '9': 2,
      '10': 'enableSourceLanguageDetect',
      '17': true
    },
    {
      '1': 'input',
      '3': 5,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.WorkspaceInputMode',
      '9': 3,
      '10': 'input',
      '17': true
    },
    {
      '1': 'is_custom_speaker',
      '3': 6,
      '4': 1,
      '5': 8,
      '9': 4,
      '10': 'isCustomSpeaker',
      '17': true
    },
    {
      '1': 'lang_pair',
      '3': 7,
      '4': 1,
      '5': 9,
      '9': 5,
      '10': 'langPair',
      '17': true
    },
    {
      '1': 'mode',
      '3': 8,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ASTTranslateMode',
      '9': 6,
      '10': 'mode',
      '17': true
    },
    {
      '1': 'speaker_id',
      '3': 9,
      '4': 1,
      '5': 9,
      '9': 7,
      '10': 'speakerId',
      '17': true
    },
    {
      '1': 'speech_rate',
      '3': 10,
      '4': 1,
      '5': 3,
      '9': 8,
      '10': 'speechRate',
      '17': true
    },
    {
      '1': 'translation_model',
      '3': 11,
      '4': 1,
      '5': 9,
      '9': 9,
      '10': 'translationModel',
      '17': true
    },
    {
      '1': 'tts_resource_id',
      '3': 12,
      '4': 1,
      '5': 9,
      '9': 10,
      '10': 'ttsResourceId',
      '17': true
    },
    {
      '1': 'voice',
      '3': 13,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ASTTranslateVoiceParameters',
      '9': 11,
      '10': 'voice',
      '17': true
    },
  ],
  '8': [
    {'1': '_denoise'},
    {'1': '_e2e'},
    {'1': '_enable_source_language_detect'},
    {'1': '_input'},
    {'1': '_is_custom_speaker'},
    {'1': '_lang_pair'},
    {'1': '_mode'},
    {'1': '_speaker_id'},
    {'1': '_speech_rate'},
    {'1': '_translation_model'},
    {'1': '_tts_resource_id'},
    {'1': '_voice'},
  ],
};

/// Descriptor for `ASTTranslateWorkspaceParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aSTTranslateWorkspaceParametersDescriptor = $convert.base64Decode(
    'Ch9BU1RUcmFuc2xhdGVXb3Jrc3BhY2VQYXJhbWV0ZXJzElcKCmFnZW50X3R5cGUYASABKA4yOC'
    '5naXpjbGF3LnJwYy52MS5BU1RUcmFuc2xhdGVXb3Jrc3BhY2VQYXJhbWV0ZXJzQWdlbnRUeXBl'
    'UglhZ2VudFR5cGUSHQoHZGVub2lzZRgCIAEoCEgAUgdkZW5vaXNliAEBEhUKA2UyZRgDIAEoCE'
    'gBUgNlMmWIAQESRgodZW5hYmxlX3NvdXJjZV9sYW5ndWFnZV9kZXRlY3QYBCABKAhIAlIaZW5h'
    'YmxlU291cmNlTGFuZ3VhZ2VEZXRlY3SIAQESPQoFaW5wdXQYBSABKA4yIi5naXpjbGF3LnJwYy'
    '52MS5Xb3Jrc3BhY2VJbnB1dE1vZGVIA1IFaW5wdXSIAQESLwoRaXNfY3VzdG9tX3NwZWFrZXIY'
    'BiABKAhIBFIPaXNDdXN0b21TcGVha2VyiAEBEiAKCWxhbmdfcGFpchgHIAEoCUgFUghsYW5nUG'
    'FpcogBARI5CgRtb2RlGAggASgOMiAuZ2l6Y2xhdy5ycGMudjEuQVNUVHJhbnNsYXRlTW9kZUgG'
    'UgRtb2RliAEBEiIKCnNwZWFrZXJfaWQYCSABKAlIB1IJc3BlYWtlcklkiAEBEiQKC3NwZWVjaF'
    '9yYXRlGAogASgDSAhSCnNwZWVjaFJhdGWIAQESMAoRdHJhbnNsYXRpb25fbW9kZWwYCyABKAlI'
    'CVIQdHJhbnNsYXRpb25Nb2RlbIgBARIrCg90dHNfcmVzb3VyY2VfaWQYDCABKAlIClINdHRzUm'
    'Vzb3VyY2VJZIgBARJGCgV2b2ljZRgNIAEoCzIrLmdpemNsYXcucnBjLnYxLkFTVFRyYW5zbGF0'
    'ZVZvaWNlUGFyYW1ldGVyc0gLUgV2b2ljZYgBAUIKCghfZGVub2lzZUIGCgRfZTJlQiAKHl9lbm'
    'FibGVfc291cmNlX2xhbmd1YWdlX2RldGVjdEIICgZfaW5wdXRCFAoSX2lzX2N1c3RvbV9zcGVh'
    'a2VyQgwKCl9sYW5nX3BhaXJCBwoFX21vZGVCDQoLX3NwZWFrZXJfaWRCDgoMX3NwZWVjaF9yYX'
    'RlQhQKEl90cmFuc2xhdGlvbl9tb2RlbEISChBfdHRzX3Jlc291cmNlX2lkQggKBl92b2ljZQ==');

@$core.Deprecated('Use chatRoomWorkflowHistorySpecDescriptor instead')
const ChatRoomWorkflowHistorySpec$json = {
  '1': 'ChatRoomWorkflowHistorySpec',
  '2': [
    {'1': 'ttl', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'ttl', '17': true},
  ],
  '8': [
    {'1': '_ttl'},
  ],
};

/// Descriptor for `ChatRoomWorkflowHistorySpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List chatRoomWorkflowHistorySpecDescriptor =
    $convert.base64Decode(
        'ChtDaGF0Um9vbVdvcmtmbG93SGlzdG9yeVNwZWMSFQoDdHRsGAEgASgJSABSA3R0bIgBAUIGCg'
        'RfdHRs');

@$core.Deprecated('Use chatRoomWorkflowSpecDescriptor instead')
const ChatRoomWorkflowSpec$json = {
  '1': 'ChatRoomWorkflowSpec',
  '2': [
    {
      '1': 'history',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ChatRoomWorkflowHistorySpec',
      '10': 'history'
    },
    {
      '1': 'transcript',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ChatRoomWorkflowTranscriptSpec',
      '9': 0,
      '10': 'transcript',
      '17': true
    },
  ],
  '8': [
    {'1': '_transcript'},
  ],
};

/// Descriptor for `ChatRoomWorkflowSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List chatRoomWorkflowSpecDescriptor = $convert.base64Decode(
    'ChRDaGF0Um9vbVdvcmtmbG93U3BlYxJFCgdoaXN0b3J5GAEgASgLMisuZ2l6Y2xhdy5ycGMudj'
    'EuQ2hhdFJvb21Xb3JrZmxvd0hpc3RvcnlTcGVjUgdoaXN0b3J5ElMKCnRyYW5zY3JpcHQYAiAB'
    'KAsyLi5naXpjbGF3LnJwYy52MS5DaGF0Um9vbVdvcmtmbG93VHJhbnNjcmlwdFNwZWNIAFIKdH'
    'JhbnNjcmlwdIgBAUINCgtfdHJhbnNjcmlwdA==');

@$core.Deprecated('Use chatRoomWorkflowTranscriptSpecDescriptor instead')
const ChatRoomWorkflowTranscriptSpec$json = {
  '1': 'ChatRoomWorkflowTranscriptSpec',
  '2': [
    {
      '1': 'asr_model',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'asrModel',
      '17': true
    },
    {
      '1': 'enabled',
      '3': 2,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'enabled',
      '17': true
    },
  ],
  '8': [
    {'1': '_asr_model'},
    {'1': '_enabled'},
  ],
};

/// Descriptor for `ChatRoomWorkflowTranscriptSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List chatRoomWorkflowTranscriptSpecDescriptor =
    $convert.base64Decode(
        'Ch5DaGF0Um9vbVdvcmtmbG93VHJhbnNjcmlwdFNwZWMSIAoJYXNyX21vZGVsGAEgASgJSABSCG'
        'Fzck1vZGVsiAEBEh0KB2VuYWJsZWQYAiABKAhIAVIHZW5hYmxlZIgBAUIMCgpfYXNyX21vZGVs'
        'QgoKCF9lbmFibGVk');

@$core.Deprecated('Use chatRoomWorkspaceHistoryParametersDescriptor instead')
const ChatRoomWorkspaceHistoryParameters$json = {
  '1': 'ChatRoomWorkspaceHistoryParameters',
  '2': [
    {'1': 'ttl', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'ttl', '17': true},
  ],
  '8': [
    {'1': '_ttl'},
  ],
};

/// Descriptor for `ChatRoomWorkspaceHistoryParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List chatRoomWorkspaceHistoryParametersDescriptor =
    $convert.base64Decode(
        'CiJDaGF0Um9vbVdvcmtzcGFjZUhpc3RvcnlQYXJhbWV0ZXJzEhUKA3R0bBgBIAEoCUgAUgN0dG'
        'yIAQFCBgoEX3R0bA==');

@$core.Deprecated('Use chatRoomWorkspaceParametersDescriptor instead')
const ChatRoomWorkspaceParameters$json = {
  '1': 'ChatRoomWorkspaceParameters',
  '2': [
    {
      '1': 'agent_type',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ChatRoomWorkspaceParametersAgentType',
      '10': 'agentType'
    },
    {
      '1': 'history',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ChatRoomWorkspaceHistoryParameters',
      '9': 0,
      '10': 'history',
      '17': true
    },
    {
      '1': 'input',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.WorkspaceInputMode',
      '9': 1,
      '10': 'input',
      '17': true
    },
    {
      '1': 'mode',
      '3': 4,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ChatRoomMode',
      '9': 2,
      '10': 'mode',
      '17': true
    },
    {
      '1': 'transcript',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ChatRoomWorkspaceTranscriptParameters',
      '9': 3,
      '10': 'transcript',
      '17': true
    },
  ],
  '8': [
    {'1': '_history'},
    {'1': '_input'},
    {'1': '_mode'},
    {'1': '_transcript'},
  ],
};

/// Descriptor for `ChatRoomWorkspaceParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List chatRoomWorkspaceParametersDescriptor = $convert.base64Decode(
    'ChtDaGF0Um9vbVdvcmtzcGFjZVBhcmFtZXRlcnMSUwoKYWdlbnRfdHlwZRgBIAEoDjI0Lmdpem'
    'NsYXcucnBjLnYxLkNoYXRSb29tV29ya3NwYWNlUGFyYW1ldGVyc0FnZW50VHlwZVIJYWdlbnRU'
    'eXBlElEKB2hpc3RvcnkYAiABKAsyMi5naXpjbGF3LnJwYy52MS5DaGF0Um9vbVdvcmtzcGFjZU'
    'hpc3RvcnlQYXJhbWV0ZXJzSABSB2hpc3RvcnmIAQESPQoFaW5wdXQYAyABKA4yIi5naXpjbGF3'
    'LnJwYy52MS5Xb3Jrc3BhY2VJbnB1dE1vZGVIAVIFaW5wdXSIAQESNQoEbW9kZRgEIAEoDjIcLm'
    'dpemNsYXcucnBjLnYxLkNoYXRSb29tTW9kZUgCUgRtb2RliAEBEloKCnRyYW5zY3JpcHQYBSAB'
    'KAsyNS5naXpjbGF3LnJwYy52MS5DaGF0Um9vbVdvcmtzcGFjZVRyYW5zY3JpcHRQYXJhbWV0ZX'
    'JzSANSCnRyYW5zY3JpcHSIAQFCCgoIX2hpc3RvcnlCCAoGX2lucHV0QgcKBV9tb2RlQg0KC190'
    'cmFuc2NyaXB0');

@$core.Deprecated('Use chatRoomWorkspaceTranscriptParametersDescriptor instead')
const ChatRoomWorkspaceTranscriptParameters$json = {
  '1': 'ChatRoomWorkspaceTranscriptParameters',
  '2': [
    {
      '1': 'asr_model',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'asrModel',
      '17': true
    },
    {
      '1': 'enabled',
      '3': 2,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'enabled',
      '17': true
    },
  ],
  '8': [
    {'1': '_asr_model'},
    {'1': '_enabled'},
  ],
};

/// Descriptor for `ChatRoomWorkspaceTranscriptParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List chatRoomWorkspaceTranscriptParametersDescriptor =
    $convert.base64Decode(
        'CiVDaGF0Um9vbVdvcmtzcGFjZVRyYW5zY3JpcHRQYXJhbWV0ZXJzEiAKCWFzcl9tb2RlbBgBIA'
        'EoCUgAUghhc3JNb2RlbIgBARIdCgdlbmFibGVkGAIgASgISAFSB2VuYWJsZWSIAQFCDAoKX2Fz'
        'cl9tb2RlbEIKCghfZW5hYmxlZA==');

@$core.Deprecated('Use credentialDescriptor instead')
const Credential$json = {
  '1': 'Credential',
  '2': [
    {
      '1': 'body',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.CredentialBody',
      '10': 'body'
    },
    {'1': 'created_at', '3': 2, '4': 1, '5': 9, '10': 'createdAt'},
    {
      '1': 'description',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'description',
      '17': true
    },
    {'1': 'name', '3': 4, '4': 1, '5': 9, '10': 'name'},
    {'1': 'provider', '3': 5, '4': 1, '5': 9, '10': 'provider'},
    {'1': 'updated_at', '3': 6, '4': 1, '5': 9, '10': 'updatedAt'},
  ],
  '8': [
    {'1': '_description'},
  ],
};

/// Descriptor for `Credential`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List credentialDescriptor = $convert.base64Decode(
    'CgpDcmVkZW50aWFsEjIKBGJvZHkYASABKAsyHi5naXpjbGF3LnJwYy52MS5DcmVkZW50aWFsQm'
    '9keVIEYm9keRIdCgpjcmVhdGVkX2F0GAIgASgJUgljcmVhdGVkQXQSJQoLZGVzY3JpcHRpb24Y'
    'AyABKAlIAFILZGVzY3JpcHRpb26IAQESEgoEbmFtZRgEIAEoCVIEbmFtZRIaCghwcm92aWRlch'
    'gFIAEoCVIIcHJvdmlkZXISHQoKdXBkYXRlZF9hdBgGIAEoCVIJdXBkYXRlZEF0Qg4KDF9kZXNj'
    'cmlwdGlvbg==');

@$core.Deprecated('Use credentialBodyDescriptor instead')
const CredentialBody$json = {
  '1': 'CredentialBody',
  '2': [
    {
      '1': 'open_aicredential_body',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.OpenAICredentialBody',
      '9': 0,
      '10': 'openAicredentialBody'
    },
    {
      '1': 'gemini_credential_body',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GeminiCredentialBody',
      '9': 0,
      '10': 'geminiCredentialBody'
    },
    {
      '1': 'dash_scope_credential_body',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DashScopeCredentialBody',
      '9': 0,
      '10': 'dashScopeCredentialBody'
    },
    {
      '1': 'mini_max_credential_body',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.MiniMaxCredentialBody',
      '9': 0,
      '10': 'miniMaxCredentialBody'
    },
    {
      '1': 'volc_credential_body',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.VolcCredentialBody',
      '9': 0,
      '10': 'volcCredentialBody'
    },
  ],
  '8': [
    {'1': 'value'},
  ],
};

/// Descriptor for `CredentialBody`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List credentialBodyDescriptor = $convert.base64Decode(
    'Cg5DcmVkZW50aWFsQm9keRJcChZvcGVuX2FpY3JlZGVudGlhbF9ib2R5GAEgASgLMiQuZ2l6Y2'
    'xhdy5ycGMudjEuT3BlbkFJQ3JlZGVudGlhbEJvZHlIAFIUb3BlbkFpY3JlZGVudGlhbEJvZHkS'
    'XAoWZ2VtaW5pX2NyZWRlbnRpYWxfYm9keRgCIAEoCzIkLmdpemNsYXcucnBjLnYxLkdlbWluaU'
    'NyZWRlbnRpYWxCb2R5SABSFGdlbWluaUNyZWRlbnRpYWxCb2R5EmYKGmRhc2hfc2NvcGVfY3Jl'
    'ZGVudGlhbF9ib2R5GAMgASgLMicuZ2l6Y2xhdy5ycGMudjEuRGFzaFNjb3BlQ3JlZGVudGlhbE'
    'JvZHlIAFIXZGFzaFNjb3BlQ3JlZGVudGlhbEJvZHkSYAoYbWluaV9tYXhfY3JlZGVudGlhbF9i'
    'b2R5GAQgASgLMiUuZ2l6Y2xhdy5ycGMudjEuTWluaU1heENyZWRlbnRpYWxCb2R5SABSFW1pbm'
    'lNYXhDcmVkZW50aWFsQm9keRJWChR2b2xjX2NyZWRlbnRpYWxfYm9keRgFIAEoCzIiLmdpemNs'
    'YXcucnBjLnYxLlZvbGNDcmVkZW50aWFsQm9keUgAUhJ2b2xjQ3JlZGVudGlhbEJvZHlCBwoFdm'
    'FsdWU=');

@$core.Deprecated('Use credentialCreateRequestDescriptor instead')
const CredentialCreateRequest$json = {
  '1': 'CredentialCreateRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Credential',
      '10': 'value'
    },
  ],
};

/// Descriptor for `CredentialCreateRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List credentialCreateRequestDescriptor =
    $convert.base64Decode(
        'ChdDcmVkZW50aWFsQ3JlYXRlUmVxdWVzdBIwCgV2YWx1ZRgBIAEoCzIaLmdpemNsYXcucnBjLn'
        'YxLkNyZWRlbnRpYWxSBXZhbHVl');

@$core.Deprecated('Use credentialCreateResponseDescriptor instead')
const CredentialCreateResponse$json = {
  '1': 'CredentialCreateResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Credential',
      '10': 'value'
    },
  ],
};

/// Descriptor for `CredentialCreateResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List credentialCreateResponseDescriptor =
    $convert.base64Decode(
        'ChhDcmVkZW50aWFsQ3JlYXRlUmVzcG9uc2USMAoFdmFsdWUYASABKAsyGi5naXpjbGF3LnJwYy'
        '52MS5DcmVkZW50aWFsUgV2YWx1ZQ==');

@$core.Deprecated('Use credentialDeleteRequestDescriptor instead')
const CredentialDeleteRequest$json = {
  '1': 'CredentialDeleteRequest',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
  ],
};

/// Descriptor for `CredentialDeleteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List credentialDeleteRequestDescriptor =
    $convert.base64Decode(
        'ChdDcmVkZW50aWFsRGVsZXRlUmVxdWVzdBISCgRuYW1lGAEgASgJUgRuYW1l');

@$core.Deprecated('Use credentialDeleteResponseDescriptor instead')
const CredentialDeleteResponse$json = {
  '1': 'CredentialDeleteResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Credential',
      '10': 'value'
    },
  ],
};

/// Descriptor for `CredentialDeleteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List credentialDeleteResponseDescriptor =
    $convert.base64Decode(
        'ChhDcmVkZW50aWFsRGVsZXRlUmVzcG9uc2USMAoFdmFsdWUYASABKAsyGi5naXpjbGF3LnJwYy'
        '52MS5DcmVkZW50aWFsUgV2YWx1ZQ==');

@$core.Deprecated('Use credentialGetRequestDescriptor instead')
const CredentialGetRequest$json = {
  '1': 'CredentialGetRequest',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
  ],
};

/// Descriptor for `CredentialGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List credentialGetRequestDescriptor = $convert
    .base64Decode('ChRDcmVkZW50aWFsR2V0UmVxdWVzdBISCgRuYW1lGAEgASgJUgRuYW1l');

@$core.Deprecated('Use credentialGetResponseDescriptor instead')
const CredentialGetResponse$json = {
  '1': 'CredentialGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Credential',
      '10': 'value'
    },
  ],
};

/// Descriptor for `CredentialGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List credentialGetResponseDescriptor = $convert.base64Decode(
    'ChVDcmVkZW50aWFsR2V0UmVzcG9uc2USMAoFdmFsdWUYASABKAsyGi5naXpjbGF3LnJwYy52MS'
    '5DcmVkZW50aWFsUgV2YWx1ZQ==');

@$core.Deprecated('Use credentialListRequestDescriptor instead')
const CredentialListRequest$json = {
  '1': 'CredentialListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `CredentialListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List credentialListRequestDescriptor = $convert.base64Decode(
    'ChVDcmVkZW50aWFsTGlzdFJlcXVlc3QSGwoGY3Vyc29yGAEgASgJSABSBmN1cnNvcogBARIZCg'
    'VsaW1pdBgCIAEoA0gBUgVsaW1pdIgBAUIJCgdfY3Vyc29yQggKBl9saW1pdA==');

@$core.Deprecated('Use credentialListResponseDescriptor instead')
const CredentialListResponse$json = {
  '1': 'CredentialListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Credential',
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

/// Descriptor for `CredentialListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List credentialListResponseDescriptor = $convert.base64Decode(
    'ChZDcmVkZW50aWFsTGlzdFJlc3BvbnNlEhkKCGhhc19uZXh0GAEgASgIUgdoYXNOZXh0EjAKBW'
    'l0ZW1zGAIgAygLMhouZ2l6Y2xhdy5ycGMudjEuQ3JlZGVudGlhbFIFaXRlbXMSJAoLbmV4dF9j'
    'dXJzb3IYAyABKAlIAFIKbmV4dEN1cnNvcogBAUIOCgxfbmV4dF9jdXJzb3I=');

@$core.Deprecated('Use credentialPutRequestDescriptor instead')
const CredentialPutRequest$json = {
  '1': 'CredentialPutRequest',
  '2': [
    {
      '1': 'body',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Credential',
      '10': 'body'
    },
    {'1': 'name', '3': 2, '4': 1, '5': 9, '10': 'name'},
  ],
};

/// Descriptor for `CredentialPutRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List credentialPutRequestDescriptor = $convert.base64Decode(
    'ChRDcmVkZW50aWFsUHV0UmVxdWVzdBIuCgRib2R5GAEgASgLMhouZ2l6Y2xhdy5ycGMudjEuQ3'
    'JlZGVudGlhbFIEYm9keRISCgRuYW1lGAIgASgJUgRuYW1l');

@$core.Deprecated('Use credentialPutResponseDescriptor instead')
const CredentialPutResponse$json = {
  '1': 'CredentialPutResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Credential',
      '10': 'value'
    },
  ],
};

/// Descriptor for `CredentialPutResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List credentialPutResponseDescriptor = $convert.base64Decode(
    'ChVDcmVkZW50aWFsUHV0UmVzcG9uc2USMAoFdmFsdWUYASABKAsyGi5naXpjbGF3LnJwYy52MS'
    '5DcmVkZW50aWFsUgV2YWx1ZQ==');

@$core.Deprecated('Use dashScopeCredentialBodyDescriptor instead')
const DashScopeCredentialBody$json = {
  '1': 'DashScopeCredentialBody',
  '2': [
    {
      '1': 'api_key',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'apiKey',
      '17': true
    },
    {
      '1': 'base_url',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'baseUrl',
      '17': true
    },
    {'1': 'token', '3': 3, '4': 1, '5': 9, '9': 2, '10': 'token', '17': true},
  ],
  '8': [
    {'1': '_api_key'},
    {'1': '_base_url'},
    {'1': '_token'},
  ],
};

/// Descriptor for `DashScopeCredentialBody`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List dashScopeCredentialBodyDescriptor = $convert.base64Decode(
    'ChdEYXNoU2NvcGVDcmVkZW50aWFsQm9keRIcCgdhcGlfa2V5GAEgASgJSABSBmFwaUtleYgBAR'
    'IeCghiYXNlX3VybBgCIAEoCUgBUgdiYXNlVXJsiAEBEhkKBXRva2VuGAMgASgJSAJSBXRva2Vu'
    'iAEBQgoKCF9hcGlfa2V5QgsKCV9iYXNlX3VybEIICgZfdG9rZW4=');

@$core.Deprecated('Use dashScopeTenantModelProviderDataDescriptor instead')
const DashScopeTenantModelProviderData$json = {
  '1': 'DashScopeTenantModelProviderData',
  '2': [
    {
      '1': 'api_mode',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.DashScopeTenantModelProviderDataApiMode',
      '9': 0,
      '10': 'apiMode',
      '17': true
    },
    {
      '1': 'upstream_model',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'upstreamModel',
      '17': true
    },
  ],
  '8': [
    {'1': '_api_mode'},
    {'1': '_upstream_model'},
  ],
};

/// Descriptor for `DashScopeTenantModelProviderData`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List dashScopeTenantModelProviderDataDescriptor =
    $convert.base64Decode(
        'CiBEYXNoU2NvcGVUZW5hbnRNb2RlbFByb3ZpZGVyRGF0YRJXCghhcGlfbW9kZRgBIAEoDjI3Lm'
        'dpemNsYXcucnBjLnYxLkRhc2hTY29wZVRlbmFudE1vZGVsUHJvdmlkZXJEYXRhQXBpTW9kZUgA'
        'UgdhcGlNb2RliAEBEioKDnVwc3RyZWFtX21vZGVsGAIgASgJSAFSDXVwc3RyZWFtTW9kZWyIAQ'
        'FCCwoJX2FwaV9tb2RlQhEKD191cHN0cmVhbV9tb2RlbA==');

@$core.Deprecated('Use dashScopeTenantVoiceProviderDataDescriptor instead')
const DashScopeTenantVoiceProviderData$json = {
  '1': 'DashScopeTenantVoiceProviderData',
  '2': [
    {
      '1': 'raw',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '10': 'raw'
    },
    {
      '1': 'voice_id',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'voiceId',
      '17': true
    },
  ],
  '8': [
    {'1': '_voice_id'},
  ],
};

/// Descriptor for `DashScopeTenantVoiceProviderData`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List dashScopeTenantVoiceProviderDataDescriptor =
    $convert.base64Decode(
        'CiBEYXNoU2NvcGVUZW5hbnRWb2ljZVByb3ZpZGVyRGF0YRIpCgNyYXcYASABKAsyFy5nb29nbG'
        'UucHJvdG9idWYuU3RydWN0UgNyYXcSHgoIdm9pY2VfaWQYAiABKAlIAFIHdm9pY2VJZIgBAUIL'
        'Cglfdm9pY2VfaWQ=');

@$core.Deprecated('Use doubaoRealtimeAIGCMetadataDescriptor instead')
const DoubaoRealtimeAIGCMetadata$json = {
  '1': 'DoubaoRealtimeAIGCMetadata',
  '2': [
    {
      '1': 'content_producer',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'contentProducer',
      '17': true
    },
    {
      '1': 'content_propagator',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'contentPropagator',
      '17': true
    },
    {'1': 'enable', '3': 3, '4': 1, '5': 8, '9': 2, '10': 'enable', '17': true},
    {
      '1': 'produce_id',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'produceId',
      '17': true
    },
    {
      '1': 'propagate_id',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'propagateId',
      '17': true
    },
  ],
  '8': [
    {'1': '_content_producer'},
    {'1': '_content_propagator'},
    {'1': '_enable'},
    {'1': '_produce_id'},
    {'1': '_propagate_id'},
  ],
};

/// Descriptor for `DoubaoRealtimeAIGCMetadata`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeAIGCMetadataDescriptor = $convert.base64Decode(
    'ChpEb3ViYW9SZWFsdGltZUFJR0NNZXRhZGF0YRIuChBjb250ZW50X3Byb2R1Y2VyGAEgASgJSA'
    'BSD2NvbnRlbnRQcm9kdWNlcogBARIyChJjb250ZW50X3Byb3BhZ2F0b3IYAiABKAlIAVIRY29u'
    'dGVudFByb3BhZ2F0b3KIAQESGwoGZW5hYmxlGAMgASgISAJSBmVuYWJsZYgBARIiCgpwcm9kdW'
    'NlX2lkGAQgASgJSANSCXByb2R1Y2VJZIgBARImCgxwcm9wYWdhdGVfaWQYBSABKAlIBFILcHJv'
    'cGFnYXRlSWSIAQFCEwoRX2NvbnRlbnRfcHJvZHVjZXJCFQoTX2NvbnRlbnRfcHJvcGFnYXRvck'
    'IJCgdfZW5hYmxlQg0KC19wcm9kdWNlX2lkQg8KDV9wcm9wYWdhdGVfaWQ=');

@$core.Deprecated('Use doubaoRealtimeASRContextDescriptor instead')
const DoubaoRealtimeASRContext$json = {
  '1': 'DoubaoRealtimeASRContext',
  '2': [
    {
      '1': 'correct_words',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeASRContext.CorrectWordsEntry',
      '10': 'correctWords'
    },
    {
      '1': 'hotwords',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeASRHotword',
      '10': 'hotwords'
    },
  ],
  '3': [DoubaoRealtimeASRContext_CorrectWordsEntry$json],
};

@$core.Deprecated('Use doubaoRealtimeASRContextDescriptor instead')
const DoubaoRealtimeASRContext_CorrectWordsEntry$json = {
  '1': 'CorrectWordsEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 9, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `DoubaoRealtimeASRContext`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeASRContextDescriptor = $convert.base64Decode(
    'ChhEb3ViYW9SZWFsdGltZUFTUkNvbnRleHQSXwoNY29ycmVjdF93b3JkcxgBIAMoCzI6Lmdpem'
    'NsYXcucnBjLnYxLkRvdWJhb1JlYWx0aW1lQVNSQ29udGV4dC5Db3JyZWN0V29yZHNFbnRyeVIM'
    'Y29ycmVjdFdvcmRzEkQKCGhvdHdvcmRzGAIgAygLMiguZ2l6Y2xhdy5ycGMudjEuRG91YmFvUm'
    'VhbHRpbWVBU1JIb3R3b3JkUghob3R3b3Jkcxo/ChFDb3JyZWN0V29yZHNFbnRyeRIQCgNrZXkY'
    'ASABKAlSA2tleRIUCgV2YWx1ZRgCIAEoCVIFdmFsdWU6AjgB');

@$core.Deprecated('Use doubaoRealtimeASRExtensionDescriptor instead')
const DoubaoRealtimeASRExtension$json = {
  '1': 'DoubaoRealtimeASRExtension',
  '2': [
    {
      '1': 'extra',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeASRExtra',
      '9': 0,
      '10': 'extra',
      '17': true
    },
  ],
  '8': [
    {'1': '_extra'},
  ],
};

/// Descriptor for `DoubaoRealtimeASRExtension`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeASRExtensionDescriptor =
    $convert.base64Decode(
        'ChpEb3ViYW9SZWFsdGltZUFTUkV4dGVuc2lvbhJBCgVleHRyYRgBIAEoCzImLmdpemNsYXcucn'
        'BjLnYxLkRvdWJhb1JlYWx0aW1lQVNSRXh0cmFIAFIFZXh0cmGIAQFCCAoGX2V4dHJh');

@$core.Deprecated('Use doubaoRealtimeASRExtraDescriptor instead')
const DoubaoRealtimeASRExtra$json = {
  '1': 'DoubaoRealtimeASRExtra',
  '2': [
    {
      '1': 'boosting_table_id',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'boostingTableId',
      '17': true
    },
    {
      '1': 'boosting_table_name',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'boostingTableName',
      '17': true
    },
    {
      '1': 'context',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeASRContext',
      '9': 2,
      '10': 'context',
      '17': true
    },
    {
      '1': 'enable_asr_twopass',
      '3': 4,
      '4': 1,
      '5': 8,
      '9': 3,
      '10': 'enableAsrTwopass',
      '17': true
    },
    {
      '1': 'enable_custom_vad',
      '3': 5,
      '4': 1,
      '5': 8,
      '9': 4,
      '10': 'enableCustomVad',
      '17': true
    },
    {
      '1': 'end_smooth_window_ms',
      '3': 6,
      '4': 1,
      '5': 3,
      '9': 5,
      '10': 'endSmoothWindowMs',
      '17': true
    },
    {
      '1': 'regex_correct_table_id',
      '3': 7,
      '4': 1,
      '5': 9,
      '9': 6,
      '10': 'regexCorrectTableId',
      '17': true
    },
    {
      '1': 'regex_correct_table_name',
      '3': 8,
      '4': 1,
      '5': 9,
      '9': 7,
      '10': 'regexCorrectTableName',
      '17': true
    },
  ],
  '8': [
    {'1': '_boosting_table_id'},
    {'1': '_boosting_table_name'},
    {'1': '_context'},
    {'1': '_enable_asr_twopass'},
    {'1': '_enable_custom_vad'},
    {'1': '_end_smooth_window_ms'},
    {'1': '_regex_correct_table_id'},
    {'1': '_regex_correct_table_name'},
  ],
};

/// Descriptor for `DoubaoRealtimeASRExtra`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeASRExtraDescriptor = $convert.base64Decode(
    'ChZEb3ViYW9SZWFsdGltZUFTUkV4dHJhEi8KEWJvb3N0aW5nX3RhYmxlX2lkGAEgASgJSABSD2'
    'Jvb3N0aW5nVGFibGVJZIgBARIzChNib29zdGluZ190YWJsZV9uYW1lGAIgASgJSAFSEWJvb3N0'
    'aW5nVGFibGVOYW1liAEBEkcKB2NvbnRleHQYAyABKAsyKC5naXpjbGF3LnJwYy52MS5Eb3ViYW'
    '9SZWFsdGltZUFTUkNvbnRleHRIAlIHY29udGV4dIgBARIxChJlbmFibGVfYXNyX3R3b3Bhc3MY'
    'BCABKAhIA1IQZW5hYmxlQXNyVHdvcGFzc4gBARIvChFlbmFibGVfY3VzdG9tX3ZhZBgFIAEoCE'
    'gEUg9lbmFibGVDdXN0b21WYWSIAQESNAoUZW5kX3Ntb290aF93aW5kb3dfbXMYBiABKANIBVIR'
    'ZW5kU21vb3RoV2luZG93TXOIAQESOAoWcmVnZXhfY29ycmVjdF90YWJsZV9pZBgHIAEoCUgGUh'
    'NyZWdleENvcnJlY3RUYWJsZUlkiAEBEjwKGHJlZ2V4X2NvcnJlY3RfdGFibGVfbmFtZRgIIAEo'
    'CUgHUhVyZWdleENvcnJlY3RUYWJsZU5hbWWIAQFCFAoSX2Jvb3N0aW5nX3RhYmxlX2lkQhYKFF'
    '9ib29zdGluZ190YWJsZV9uYW1lQgoKCF9jb250ZXh0QhUKE19lbmFibGVfYXNyX3R3b3Bhc3NC'
    'FAoSX2VuYWJsZV9jdXN0b21fdmFkQhcKFV9lbmRfc21vb3RoX3dpbmRvd19tc0IZChdfcmVnZX'
    'hfY29ycmVjdF90YWJsZV9pZEIbChlfcmVnZXhfY29ycmVjdF90YWJsZV9uYW1l');

@$core.Deprecated('Use doubaoRealtimeASRHotwordDescriptor instead')
const DoubaoRealtimeASRHotword$json = {
  '1': 'DoubaoRealtimeASRHotword',
  '2': [
    {'1': 'word', '3': 1, '4': 1, '5': 9, '10': 'word'},
  ],
};

/// Descriptor for `DoubaoRealtimeASRHotword`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeASRHotwordDescriptor =
    $convert.base64Decode(
        'ChhEb3ViYW9SZWFsdGltZUFTUkhvdHdvcmQSEgoEd29yZBgBIAEoCVIEd29yZA==');

@$core.Deprecated('Use doubaoRealtimeAudioDescriptor instead')
const DoubaoRealtimeAudio$json = {
  '1': 'DoubaoRealtimeAudio',
  '2': [
    {
      '1': 'input',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeAudioInput',
      '10': 'input'
    },
    {
      '1': 'output',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeAudioOutput',
      '10': 'output'
    },
  ],
};

/// Descriptor for `DoubaoRealtimeAudio`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeAudioDescriptor = $convert.base64Decode(
    'ChNEb3ViYW9SZWFsdGltZUF1ZGlvEj4KBWlucHV0GAEgASgLMiguZ2l6Y2xhdy5ycGMudjEuRG'
    '91YmFvUmVhbHRpbWVBdWRpb0lucHV0UgVpbnB1dBJBCgZvdXRwdXQYAiABKAsyKS5naXpjbGF3'
    'LnJwYy52MS5Eb3ViYW9SZWFsdGltZUF1ZGlvT3V0cHV0UgZvdXRwdXQ=');

@$core.Deprecated('Use doubaoRealtimeAudioFormatDescriptor instead')
const DoubaoRealtimeAudioFormat$json = {
  '1': 'DoubaoRealtimeAudioFormat',
  '2': [
    {'1': 'rate', '3': 1, '4': 1, '5': 3, '10': 'rate'},
    {
      '1': 'type',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeAudioFormatType',
      '10': 'type'
    },
  ],
};

/// Descriptor for `DoubaoRealtimeAudioFormat`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeAudioFormatDescriptor = $convert.base64Decode(
    'ChlEb3ViYW9SZWFsdGltZUF1ZGlvRm9ybWF0EhIKBHJhdGUYASABKANSBHJhdGUSQQoEdHlwZR'
    'gCIAEoDjItLmdpemNsYXcucnBjLnYxLkRvdWJhb1JlYWx0aW1lQXVkaW9Gb3JtYXRUeXBlUgR0'
    'eXBl');

@$core.Deprecated('Use doubaoRealtimeAudioInputDescriptor instead')
const DoubaoRealtimeAudioInput$json = {
  '1': 'DoubaoRealtimeAudioInput',
  '2': [
    {
      '1': 'format',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeAudioFormat',
      '10': 'format'
    },
  ],
};

/// Descriptor for `DoubaoRealtimeAudioInput`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeAudioInputDescriptor =
    $convert.base64Decode(
        'ChhEb3ViYW9SZWFsdGltZUF1ZGlvSW5wdXQSQQoGZm9ybWF0GAEgASgLMikuZ2l6Y2xhdy5ycG'
        'MudjEuRG91YmFvUmVhbHRpbWVBdWRpb0Zvcm1hdFIGZm9ybWF0');

@$core.Deprecated('Use doubaoRealtimeAudioOutputDescriptor instead')
const DoubaoRealtimeAudioOutput$json = {
  '1': 'DoubaoRealtimeAudioOutput',
  '2': [
    {
      '1': 'format',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeAudioFormat',
      '10': 'format'
    },
    {
      '1': 'loudness',
      '3': 2,
      '4': 1,
      '5': 3,
      '9': 0,
      '10': 'loudness',
      '17': true
    },
    {'1': 'speed', '3': 3, '4': 1, '5': 3, '9': 1, '10': 'speed', '17': true},
    {'1': 'voice', '3': 4, '4': 1, '5': 9, '9': 2, '10': 'voice', '17': true},
  ],
  '8': [
    {'1': '_loudness'},
    {'1': '_speed'},
    {'1': '_voice'},
  ],
};

/// Descriptor for `DoubaoRealtimeAudioOutput`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeAudioOutputDescriptor = $convert.base64Decode(
    'ChlEb3ViYW9SZWFsdGltZUF1ZGlvT3V0cHV0EkEKBmZvcm1hdBgBIAEoCzIpLmdpemNsYXcucn'
    'BjLnYxLkRvdWJhb1JlYWx0aW1lQXVkaW9Gb3JtYXRSBmZvcm1hdBIfCghsb3VkbmVzcxgCIAEo'
    'A0gAUghsb3VkbmVzc4gBARIZCgVzcGVlZBgDIAEoA0gBUgVzcGVlZIgBARIZCgV2b2ljZRgEIA'
    'EoCUgCUgV2b2ljZYgBAUILCglfbG91ZG5lc3NCCAoGX3NwZWVkQggKBl92b2ljZQ==');

@$core.Deprecated('Use doubaoRealtimeDialogExtensionDescriptor instead')
const DoubaoRealtimeDialogExtension$json = {
  '1': 'DoubaoRealtimeDialogExtension',
  '2': [
    {
      '1': 'extra',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeDialogExtra',
      '9': 0,
      '10': 'extra',
      '17': true
    },
  ],
  '8': [
    {'1': '_extra'},
  ],
};

/// Descriptor for `DoubaoRealtimeDialogExtension`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeDialogExtensionDescriptor =
    $convert.base64Decode(
        'Ch1Eb3ViYW9SZWFsdGltZURpYWxvZ0V4dGVuc2lvbhJECgVleHRyYRgBIAEoCzIpLmdpemNsYX'
        'cucnBjLnYxLkRvdWJhb1JlYWx0aW1lRGlhbG9nRXh0cmFIAFIFZXh0cmGIAQFCCAoGX2V4dHJh');

@$core.Deprecated('Use doubaoRealtimeDialogExtraDescriptor instead')
const DoubaoRealtimeDialogExtra$json = {
  '1': 'DoubaoRealtimeDialogExtra',
  '2': [
    {
      '1': 'audit_response',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'auditResponse',
      '17': true
    },
    {
      '1': 'enable_conversation_truncate',
      '3': 2,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'enableConversationTruncate',
      '17': true
    },
    {
      '1': 'enable_loudness_norm',
      '3': 3,
      '4': 1,
      '5': 8,
      '9': 2,
      '10': 'enableLoudnessNorm',
      '17': true
    },
    {
      '1': 'enable_music',
      '3': 4,
      '4': 1,
      '5': 8,
      '9': 3,
      '10': 'enableMusic',
      '17': true
    },
    {
      '1': 'enable_user_query_exit',
      '3': 5,
      '4': 1,
      '5': 8,
      '9': 4,
      '10': 'enableUserQueryExit',
      '17': true
    },
    {
      '1': 'enable_volc_websearch',
      '3': 6,
      '4': 1,
      '5': 8,
      '9': 5,
      '10': 'enableVolcWebsearch',
      '17': true
    },
    {
      '1': 'strict_audit',
      '3': 7,
      '4': 1,
      '5': 8,
      '9': 6,
      '10': 'strictAudit',
      '17': true
    },
    {
      '1': 'volc_websearch_bot_id',
      '3': 8,
      '4': 1,
      '5': 9,
      '9': 7,
      '10': 'volcWebsearchBotId',
      '17': true
    },
    {
      '1': 'volc_websearch_no_result_message',
      '3': 9,
      '4': 1,
      '5': 9,
      '9': 8,
      '10': 'volcWebsearchNoResultMessage',
      '17': true
    },
    {
      '1': 'volc_websearch_result_count',
      '3': 10,
      '4': 1,
      '5': 3,
      '9': 9,
      '10': 'volcWebsearchResultCount',
      '17': true
    },
    {
      '1': 'volc_websearch_type',
      '3': 11,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeDialogExtraVolcWebsearchType',
      '9': 10,
      '10': 'volcWebsearchType',
      '17': true
    },
  ],
  '8': [
    {'1': '_audit_response'},
    {'1': '_enable_conversation_truncate'},
    {'1': '_enable_loudness_norm'},
    {'1': '_enable_music'},
    {'1': '_enable_user_query_exit'},
    {'1': '_enable_volc_websearch'},
    {'1': '_strict_audit'},
    {'1': '_volc_websearch_bot_id'},
    {'1': '_volc_websearch_no_result_message'},
    {'1': '_volc_websearch_result_count'},
    {'1': '_volc_websearch_type'},
  ],
};

/// Descriptor for `DoubaoRealtimeDialogExtra`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeDialogExtraDescriptor = $convert.base64Decode(
    'ChlEb3ViYW9SZWFsdGltZURpYWxvZ0V4dHJhEioKDmF1ZGl0X3Jlc3BvbnNlGAEgASgJSABSDW'
    'F1ZGl0UmVzcG9uc2WIAQESRQocZW5hYmxlX2NvbnZlcnNhdGlvbl90cnVuY2F0ZRgCIAEoCEgB'
    'UhplbmFibGVDb252ZXJzYXRpb25UcnVuY2F0ZYgBARI1ChRlbmFibGVfbG91ZG5lc3Nfbm9ybR'
    'gDIAEoCEgCUhJlbmFibGVMb3VkbmVzc05vcm2IAQESJgoMZW5hYmxlX211c2ljGAQgASgISANS'
    'C2VuYWJsZU11c2ljiAEBEjgKFmVuYWJsZV91c2VyX3F1ZXJ5X2V4aXQYBSABKAhIBFITZW5hYm'
    'xlVXNlclF1ZXJ5RXhpdIgBARI3ChVlbmFibGVfdm9sY193ZWJzZWFyY2gYBiABKAhIBVITZW5h'
    'YmxlVm9sY1dlYnNlYXJjaIgBARImCgxzdHJpY3RfYXVkaXQYByABKAhIBlILc3RyaWN0QXVkaX'
    'SIAQESNgoVdm9sY193ZWJzZWFyY2hfYm90X2lkGAggASgJSAdSEnZvbGNXZWJzZWFyY2hCb3RJ'
    'ZIgBARJLCiB2b2xjX3dlYnNlYXJjaF9ub19yZXN1bHRfbWVzc2FnZRgJIAEoCUgIUhx2b2xjV2'
    'Vic2VhcmNoTm9SZXN1bHRNZXNzYWdliAEBEkIKG3ZvbGNfd2Vic2VhcmNoX3Jlc3VsdF9jb3Vu'
    'dBgKIAEoA0gJUhh2b2xjV2Vic2VhcmNoUmVzdWx0Q291bnSIAQESbwoTdm9sY193ZWJzZWFyY2'
    'hfdHlwZRgLIAEoDjI6LmdpemNsYXcucnBjLnYxLkRvdWJhb1JlYWx0aW1lRGlhbG9nRXh0cmFW'
    'b2xjV2Vic2VhcmNoVHlwZUgKUhF2b2xjV2Vic2VhcmNoVHlwZYgBAUIRCg9fYXVkaXRfcmVzcG'
    '9uc2VCHwodX2VuYWJsZV9jb252ZXJzYXRpb25fdHJ1bmNhdGVCFwoVX2VuYWJsZV9sb3VkbmVz'
    'c19ub3JtQg8KDV9lbmFibGVfbXVzaWNCGQoXX2VuYWJsZV91c2VyX3F1ZXJ5X2V4aXRCGAoWX2'
    'VuYWJsZV92b2xjX3dlYnNlYXJjaEIPCg1fc3RyaWN0X2F1ZGl0QhgKFl92b2xjX3dlYnNlYXJj'
    'aF9ib3RfaWRCIwohX3ZvbGNfd2Vic2VhcmNoX25vX3Jlc3VsdF9tZXNzYWdlQh4KHF92b2xjX3'
    'dlYnNlYXJjaF9yZXN1bHRfY291bnRCFgoUX3ZvbGNfd2Vic2VhcmNoX3R5cGU=');

@$core.Deprecated('Use doubaoRealtimeExtensionDescriptor instead')
const DoubaoRealtimeExtension$json = {
  '1': 'DoubaoRealtimeExtension',
  '2': [
    {
      '1': 'asr',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeASRExtension',
      '9': 0,
      '10': 'asr',
      '17': true
    },
    {
      '1': 'dialog',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeDialogExtension',
      '9': 1,
      '10': 'dialog',
      '17': true
    },
    {
      '1': 'tts',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeTTSExtension',
      '9': 2,
      '10': 'tts',
      '17': true
    },
  ],
  '8': [
    {'1': '_asr'},
    {'1': '_dialog'},
    {'1': '_tts'},
  ],
};

/// Descriptor for `DoubaoRealtimeExtension`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeExtensionDescriptor = $convert.base64Decode(
    'ChdEb3ViYW9SZWFsdGltZUV4dGVuc2lvbhJBCgNhc3IYASABKAsyKi5naXpjbGF3LnJwYy52MS'
    '5Eb3ViYW9SZWFsdGltZUFTUkV4dGVuc2lvbkgAUgNhc3KIAQESSgoGZGlhbG9nGAIgASgLMi0u'
    'Z2l6Y2xhdy5ycGMudjEuRG91YmFvUmVhbHRpbWVEaWFsb2dFeHRlbnNpb25IAVIGZGlhbG9niA'
    'EBEkEKA3R0cxgDIAEoCzIqLmdpemNsYXcucnBjLnYxLkRvdWJhb1JlYWx0aW1lVFRTRXh0ZW5z'
    'aW9uSAJSA3R0c4gBAUIGCgRfYXNyQgkKB19kaWFsb2dCBgoEX3R0cw==');

@$core.Deprecated('Use doubaoRealtimeFunctionToolDescriptor instead')
const DoubaoRealtimeFunctionTool$json = {
  '1': 'DoubaoRealtimeFunctionTool',
  '2': [
    {
      '1': 'description',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'description',
      '17': true
    },
    {'1': 'name', '3': 2, '4': 1, '5': 9, '10': 'name'},
    {
      '1': 'parameters',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeJSONSchema',
      '9': 1,
      '10': 'parameters',
      '17': true
    },
    {'1': 'strict', '3': 4, '4': 1, '5': 8, '9': 2, '10': 'strict', '17': true},
    {
      '1': 'type',
      '3': 5,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeFunctionToolType',
      '10': 'type'
    },
  ],
  '8': [
    {'1': '_description'},
    {'1': '_parameters'},
    {'1': '_strict'},
  ],
};

/// Descriptor for `DoubaoRealtimeFunctionTool`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeFunctionToolDescriptor = $convert.base64Decode(
    'ChpEb3ViYW9SZWFsdGltZUZ1bmN0aW9uVG9vbBIlCgtkZXNjcmlwdGlvbhgBIAEoCUgAUgtkZX'
    'NjcmlwdGlvbogBARISCgRuYW1lGAIgASgJUgRuYW1lEk0KCnBhcmFtZXRlcnMYAyABKAsyKC5n'
    'aXpjbGF3LnJwYy52MS5Eb3ViYW9SZWFsdGltZUpTT05TY2hlbWFIAVIKcGFyYW1ldGVyc4gBAR'
    'IbCgZzdHJpY3QYBCABKAhIAlIGc3RyaWN0iAEBEkIKBHR5cGUYBSABKA4yLi5naXpjbGF3LnJw'
    'Yy52MS5Eb3ViYW9SZWFsdGltZUZ1bmN0aW9uVG9vbFR5cGVSBHR5cGVCDgoMX2Rlc2NyaXB0aW'
    '9uQg0KC19wYXJhbWV0ZXJzQgkKB19zdHJpY3Q=');

@$core.Deprecated('Use doubaoRealtimeJSONSchemaDescriptor instead')
const DoubaoRealtimeJSONSchema$json = {
  '1': 'DoubaoRealtimeJSONSchema',
  '2': [
    {
      '1': 'additional_properties',
      '3': 1,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'additionalProperties',
      '17': true
    },
    {
      '1': 'any_of',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeJSONSchema',
      '10': 'anyOf'
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
    {'1': 'enum_values', '3': 4, '4': 3, '5': 9, '10': 'enum'},
    {
      '1': 'items',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeJSONSchema',
      '9': 2,
      '10': 'items',
      '17': true
    },
    {
      '1': 'max_length',
      '3': 6,
      '4': 1,
      '5': 3,
      '9': 3,
      '10': 'maxLength',
      '17': true
    },
    {
      '1': 'maximum',
      '3': 7,
      '4': 1,
      '5': 1,
      '9': 4,
      '10': 'maximum',
      '17': true
    },
    {
      '1': 'min_length',
      '3': 8,
      '4': 1,
      '5': 3,
      '9': 5,
      '10': 'minLength',
      '17': true
    },
    {
      '1': 'minimum',
      '3': 9,
      '4': 1,
      '5': 1,
      '9': 6,
      '10': 'minimum',
      '17': true
    },
    {
      '1': 'properties',
      '3': 10,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeJSONSchema.PropertiesEntry',
      '10': 'properties'
    },
    {'1': 'required', '3': 11, '4': 3, '5': 9, '10': 'required'},
    {'1': 'type', '3': 12, '4': 1, '5': 9, '9': 7, '10': 'type', '17': true},
  ],
  '3': [DoubaoRealtimeJSONSchema_PropertiesEntry$json],
  '8': [
    {'1': '_additional_properties'},
    {'1': '_description'},
    {'1': '_items'},
    {'1': '_max_length'},
    {'1': '_maximum'},
    {'1': '_min_length'},
    {'1': '_minimum'},
    {'1': '_type'},
  ],
};

@$core.Deprecated('Use doubaoRealtimeJSONSchemaDescriptor instead')
const DoubaoRealtimeJSONSchema_PropertiesEntry$json = {
  '1': 'PropertiesEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {
      '1': 'value',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeJSONSchema',
      '10': 'value'
    },
  ],
  '7': {'7': true},
};

/// Descriptor for `DoubaoRealtimeJSONSchema`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeJSONSchemaDescriptor = $convert.base64Decode(
    'ChhEb3ViYW9SZWFsdGltZUpTT05TY2hlbWESOAoVYWRkaXRpb25hbF9wcm9wZXJ0aWVzGAEgAS'
    'gISABSFGFkZGl0aW9uYWxQcm9wZXJ0aWVziAEBEj8KBmFueV9vZhgCIAMoCzIoLmdpemNsYXcu'
    'cnBjLnYxLkRvdWJhb1JlYWx0aW1lSlNPTlNjaGVtYVIFYW55T2YSJQoLZGVzY3JpcHRpb24YAy'
    'ABKAlIAVILZGVzY3JpcHRpb26IAQESGQoLZW51bV92YWx1ZXMYBCADKAlSBGVudW0SQwoFaXRl'
    'bXMYBSABKAsyKC5naXpjbGF3LnJwYy52MS5Eb3ViYW9SZWFsdGltZUpTT05TY2hlbWFIAlIFaX'
    'RlbXOIAQESIgoKbWF4X2xlbmd0aBgGIAEoA0gDUgltYXhMZW5ndGiIAQESHQoHbWF4aW11bRgH'
    'IAEoAUgEUgdtYXhpbXVtiAEBEiIKCm1pbl9sZW5ndGgYCCABKANIBVIJbWluTGVuZ3RoiAEBEh'
    '0KB21pbmltdW0YCSABKAFIBlIHbWluaW11bYgBARJYCgpwcm9wZXJ0aWVzGAogAygLMjguZ2l6'
    'Y2xhdy5ycGMudjEuRG91YmFvUmVhbHRpbWVKU09OU2NoZW1hLlByb3BlcnRpZXNFbnRyeVIKcH'
    'JvcGVydGllcxIaCghyZXF1aXJlZBgLIAMoCVIIcmVxdWlyZWQSFwoEdHlwZRgMIAEoCUgHUgR0'
    'eXBliAEBGmcKD1Byb3BlcnRpZXNFbnRyeRIQCgNrZXkYASABKAlSA2tleRI+CgV2YWx1ZRgCIA'
    'EoCzIoLmdpemNsYXcucnBjLnYxLkRvdWJhb1JlYWx0aW1lSlNPTlNjaGVtYVIFdmFsdWU6AjgB'
    'QhgKFl9hZGRpdGlvbmFsX3Byb3BlcnRpZXNCDgoMX2Rlc2NyaXB0aW9uQggKBl9pdGVtc0INCg'
    'tfbWF4X2xlbmd0aEIKCghfbWF4aW11bUINCgtfbWluX2xlbmd0aEIKCghfbWluaW11bUIHCgVf'
    'dHlwZQ==');

@$core.Deprecated('Use doubaoRealtimeTTSExtensionDescriptor instead')
const DoubaoRealtimeTTSExtension$json = {
  '1': 'DoubaoRealtimeTTSExtension',
  '2': [
    {
      '1': 'extra',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeTTSExtra',
      '9': 0,
      '10': 'extra',
      '17': true
    },
  ],
  '8': [
    {'1': '_extra'},
  ],
};

/// Descriptor for `DoubaoRealtimeTTSExtension`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeTTSExtensionDescriptor =
    $convert.base64Decode(
        'ChpEb3ViYW9SZWFsdGltZVRUU0V4dGVuc2lvbhJBCgVleHRyYRgBIAEoCzImLmdpemNsYXcucn'
        'BjLnYxLkRvdWJhb1JlYWx0aW1lVFRTRXh0cmFIAFIFZXh0cmGIAQFCCAoGX2V4dHJh');

@$core.Deprecated('Use doubaoRealtimeTTSExtraDescriptor instead')
const DoubaoRealtimeTTSExtra$json = {
  '1': 'DoubaoRealtimeTTSExtra',
  '2': [
    {
      '1': 'aigc_metadata',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeAIGCMetadata',
      '9': 0,
      '10': 'aigcMetadata',
      '17': true
    },
    {
      '1': 'explicit_dialect',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'explicitDialect',
      '17': true
    },
    {
      '1': 'tts_2_0_model',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'tts20Model',
      '17': true
    },
  ],
  '8': [
    {'1': '_aigc_metadata'},
    {'1': '_explicit_dialect'},
    {'1': '_tts_2_0_model'},
  ],
};

/// Descriptor for `DoubaoRealtimeTTSExtra`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeTTSExtraDescriptor = $convert.base64Decode(
    'ChZEb3ViYW9SZWFsdGltZVRUU0V4dHJhElQKDWFpZ2NfbWV0YWRhdGEYASABKAsyKi5naXpjbG'
    'F3LnJwYy52MS5Eb3ViYW9SZWFsdGltZUFJR0NNZXRhZGF0YUgAUgxhaWdjTWV0YWRhdGGIAQES'
    'LgoQZXhwbGljaXRfZGlhbGVjdBgCIAEoCUgBUg9leHBsaWNpdERpYWxlY3SIAQESJgoNdHRzXz'
    'JfMF9tb2RlbBgDIAEoCUgCUgp0dHMyME1vZGVsiAEBQhAKDl9haWdjX21ldGFkYXRhQhMKEV9l'
    'eHBsaWNpdF9kaWFsZWN0QhAKDl90dHNfMl8wX21vZGVs');

@$core.Deprecated('Use doubaoRealtimeWorkflowSpecDescriptor instead')
const DoubaoRealtimeWorkflowSpec$json = {
  '1': 'DoubaoRealtimeWorkflowSpec',
  '2': [
    {
      '1': 'audio',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeAudio',
      '9': 0,
      '10': 'audio',
      '17': true
    },
    {
      '1': 'extension',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeExtension',
      '9': 1,
      '10': 'extension',
      '17': true
    },
    {
      '1': 'instructions',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'instructions',
      '17': true
    },
    {'1': 'model', '3': 4, '4': 1, '5': 9, '10': 'model'},
    {
      '1': 'tools',
      '3': 5,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeFunctionTool',
      '10': 'tools'
    },
  ],
  '8': [
    {'1': '_audio'},
    {'1': '_extension'},
    {'1': '_instructions'},
  ],
};

/// Descriptor for `DoubaoRealtimeWorkflowSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeWorkflowSpecDescriptor = $convert.base64Decode(
    'ChpEb3ViYW9SZWFsdGltZVdvcmtmbG93U3BlYxI+CgVhdWRpbxgBIAEoCzIjLmdpemNsYXcucn'
    'BjLnYxLkRvdWJhb1JlYWx0aW1lQXVkaW9IAFIFYXVkaW+IAQESSgoJZXh0ZW5zaW9uGAIgASgL'
    'MicuZ2l6Y2xhdy5ycGMudjEuRG91YmFvUmVhbHRpbWVFeHRlbnNpb25IAVIJZXh0ZW5zaW9uiA'
    'EBEicKDGluc3RydWN0aW9ucxgDIAEoCUgCUgxpbnN0cnVjdGlvbnOIAQESFAoFbW9kZWwYBCAB'
    'KAlSBW1vZGVsEkAKBXRvb2xzGAUgAygLMiouZ2l6Y2xhdy5ycGMudjEuRG91YmFvUmVhbHRpbW'
    'VGdW5jdGlvblRvb2xSBXRvb2xzQggKBl9hdWRpb0IMCgpfZXh0ZW5zaW9uQg8KDV9pbnN0cnVj'
    'dGlvbnM=');

@$core.Deprecated('Use doubaoRealtimeWorkspaceParametersDescriptor instead')
const DoubaoRealtimeWorkspaceParameters$json = {
  '1': 'DoubaoRealtimeWorkspaceParameters',
  '2': [
    {
      '1': 'agent_type',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeWorkspaceParametersAgentType',
      '10': 'agentType'
    },
    {
      '1': 'audio',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeAudio',
      '9': 0,
      '10': 'audio',
      '17': true
    },
    {'1': 'e2e', '3': 3, '4': 1, '5': 8, '9': 1, '10': 'e2e', '17': true},
    {
      '1': 'extension',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeExtension',
      '9': 2,
      '10': 'extension',
      '17': true
    },
    {
      '1': 'input',
      '3': 5,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.WorkspaceInputMode',
      '9': 3,
      '10': 'input',
      '17': true
    },
    {
      '1': 'instructions',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'instructions',
      '17': true
    },
    {'1': 'model', '3': 7, '4': 1, '5': 9, '9': 5, '10': 'model', '17': true},
    {
      '1': 'tools',
      '3': 8,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeFunctionTool',
      '10': 'tools'
    },
  ],
  '8': [
    {'1': '_audio'},
    {'1': '_e2e'},
    {'1': '_extension'},
    {'1': '_input'},
    {'1': '_instructions'},
    {'1': '_model'},
  ],
};

/// Descriptor for `DoubaoRealtimeWorkspaceParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeWorkspaceParametersDescriptor = $convert.base64Decode(
    'CiFEb3ViYW9SZWFsdGltZVdvcmtzcGFjZVBhcmFtZXRlcnMSWQoKYWdlbnRfdHlwZRgBIAEoDj'
    'I6LmdpemNsYXcucnBjLnYxLkRvdWJhb1JlYWx0aW1lV29ya3NwYWNlUGFyYW1ldGVyc0FnZW50'
    'VHlwZVIJYWdlbnRUeXBlEj4KBWF1ZGlvGAIgASgLMiMuZ2l6Y2xhdy5ycGMudjEuRG91YmFvUm'
    'VhbHRpbWVBdWRpb0gAUgVhdWRpb4gBARIVCgNlMmUYAyABKAhIAVIDZTJliAEBEkoKCWV4dGVu'
    'c2lvbhgEIAEoCzInLmdpemNsYXcucnBjLnYxLkRvdWJhb1JlYWx0aW1lRXh0ZW5zaW9uSAJSCW'
    'V4dGVuc2lvbogBARI9CgVpbnB1dBgFIAEoDjIiLmdpemNsYXcucnBjLnYxLldvcmtzcGFjZUlu'
    'cHV0TW9kZUgDUgVpbnB1dIgBARInCgxpbnN0cnVjdGlvbnMYBiABKAlIBFIMaW5zdHJ1Y3Rpb2'
    '5ziAEBEhkKBW1vZGVsGAcgASgJSAVSBW1vZGVsiAEBEkAKBXRvb2xzGAggAygLMiouZ2l6Y2xh'
    'dy5ycGMudjEuRG91YmFvUmVhbHRpbWVGdW5jdGlvblRvb2xSBXRvb2xzQggKBl9hdWRpb0IGCg'
    'RfZTJlQgwKCl9leHRlbnNpb25CCAoGX2lucHV0Qg8KDV9pbnN0cnVjdGlvbnNCCAoGX21vZGVs');

@$core.Deprecated('Use flowcraftConversationParametersDescriptor instead')
const FlowcraftConversationParameters$json = {
  '1': 'FlowcraftConversationParameters',
  '2': [
    {
      '1': 'agent_initiative_policy',
      '3': 1,
      '4': 1,
      '5': 14,
      '6':
          '.gizclaw.rpc.v1.FlowcraftConversationParametersAgentInitiativePolicy',
      '9': 0,
      '10': 'agentInitiativePolicy',
      '17': true
    },
    {
      '1': 'initiative',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.FlowcraftConversationParametersInitiative',
      '9': 1,
      '10': 'initiative',
      '17': true
    },
  ],
  '8': [
    {'1': '_agent_initiative_policy'},
    {'1': '_initiative'},
  ],
};

/// Descriptor for `FlowcraftConversationParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List flowcraftConversationParametersDescriptor = $convert.base64Decode(
    'Ch9GbG93Y3JhZnRDb252ZXJzYXRpb25QYXJhbWV0ZXJzEoEBChdhZ2VudF9pbml0aWF0aXZlX3'
    'BvbGljeRgBIAEoDjJELmdpemNsYXcucnBjLnYxLkZsb3djcmFmdENvbnZlcnNhdGlvblBhcmFt'
    'ZXRlcnNBZ2VudEluaXRpYXRpdmVQb2xpY3lIAFIVYWdlbnRJbml0aWF0aXZlUG9saWN5iAEBEl'
    '4KCmluaXRpYXRpdmUYAiABKA4yOS5naXpjbGF3LnJwYy52MS5GbG93Y3JhZnRDb252ZXJzYXRp'
    'b25QYXJhbWV0ZXJzSW5pdGlhdGl2ZUgBUgppbml0aWF0aXZliAEBQhoKGF9hZ2VudF9pbml0aW'
    'F0aXZlX3BvbGljeUINCgtfaW5pdGlhdGl2ZQ==');

@$core.Deprecated('Use flowcraftWorkflowSpecDescriptor instead')
const FlowcraftWorkflowSpec$json = {
  '1': 'FlowcraftWorkflowSpec',
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

/// Descriptor for `FlowcraftWorkflowSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List flowcraftWorkflowSpecDescriptor = $convert.base64Decode(
    'ChVGbG93Y3JhZnRXb3JrZmxvd1NwZWMSLwoGZmllbGRzGAEgASgLMhcuZ29vZ2xlLnByb3RvYn'
    'VmLlN0cnVjdFIGZmllbGRz');

@$core.Deprecated('Use flowcraftWorkspaceParametersDescriptor instead')
const FlowcraftWorkspaceParameters$json = {
  '1': 'FlowcraftWorkspaceParameters',
  '2': [
    {
      '1': 'agent_type',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.FlowcraftWorkspaceParametersAgentType',
      '10': 'agentType'
    },
    {
      '1': 'conversation',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FlowcraftConversationParameters',
      '9': 0,
      '10': 'conversation',
      '17': true
    },
    {'1': 'e2e', '3': 3, '4': 1, '5': 8, '9': 1, '10': 'e2e', '17': true},
    {
      '1': 'embedding_model',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'embeddingModel',
      '17': true
    },
    {
      '1': 'extract_model',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'extractModel',
      '17': true
    },
    {
      '1': 'generate_model',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'generateModel',
      '17': true
    },
    {
      '1': 'input',
      '3': 7,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.WorkspaceInputMode',
      '9': 5,
      '10': 'input',
      '17': true
    },
  ],
  '8': [
    {'1': '_conversation'},
    {'1': '_e2e'},
    {'1': '_embedding_model'},
    {'1': '_extract_model'},
    {'1': '_generate_model'},
    {'1': '_input'},
  ],
};

/// Descriptor for `FlowcraftWorkspaceParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List flowcraftWorkspaceParametersDescriptor = $convert.base64Decode(
    'ChxGbG93Y3JhZnRXb3Jrc3BhY2VQYXJhbWV0ZXJzElQKCmFnZW50X3R5cGUYASABKA4yNS5naX'
    'pjbGF3LnJwYy52MS5GbG93Y3JhZnRXb3Jrc3BhY2VQYXJhbWV0ZXJzQWdlbnRUeXBlUglhZ2Vu'
    'dFR5cGUSWAoMY29udmVyc2F0aW9uGAIgASgLMi8uZ2l6Y2xhdy5ycGMudjEuRmxvd2NyYWZ0Q2'
    '9udmVyc2F0aW9uUGFyYW1ldGVyc0gAUgxjb252ZXJzYXRpb26IAQESFQoDZTJlGAMgASgISAFS'
    'A2UyZYgBARIsCg9lbWJlZGRpbmdfbW9kZWwYBCABKAlIAlIOZW1iZWRkaW5nTW9kZWyIAQESKA'
    'oNZXh0cmFjdF9tb2RlbBgFIAEoCUgDUgxleHRyYWN0TW9kZWyIAQESKgoOZ2VuZXJhdGVfbW9k'
    'ZWwYBiABKAlIBFINZ2VuZXJhdGVNb2RlbIgBARI9CgVpbnB1dBgHIAEoDjIiLmdpemNsYXcucn'
    'BjLnYxLldvcmtzcGFjZUlucHV0TW9kZUgFUgVpbnB1dIgBAUIPCg1fY29udmVyc2F0aW9uQgYK'
    'BF9lMmVCEgoQX2VtYmVkZGluZ19tb2RlbEIQCg5fZXh0cmFjdF9tb2RlbEIRCg9fZ2VuZXJhdG'
    'VfbW9kZWxCCAoGX2lucHV0');

@$core.Deprecated('Use geminiCredentialBodyDescriptor instead')
const GeminiCredentialBody$json = {
  '1': 'GeminiCredentialBody',
  '2': [
    {
      '1': 'api_key',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'apiKey',
      '17': true
    },
    {
      '1': 'base_url',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'baseUrl',
      '17': true
    },
    {'1': 'token', '3': 3, '4': 1, '5': 9, '9': 2, '10': 'token', '17': true},
  ],
  '8': [
    {'1': '_api_key'},
    {'1': '_base_url'},
    {'1': '_token'},
  ],
};

/// Descriptor for `GeminiCredentialBody`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List geminiCredentialBodyDescriptor = $convert.base64Decode(
    'ChRHZW1pbmlDcmVkZW50aWFsQm9keRIcCgdhcGlfa2V5GAEgASgJSABSBmFwaUtleYgBARIeCg'
    'hiYXNlX3VybBgCIAEoCUgBUgdiYXNlVXJsiAEBEhkKBXRva2VuGAMgASgJSAJSBXRva2VuiAEB'
    'QgoKCF9hcGlfa2V5QgsKCV9iYXNlX3VybEIICgZfdG9rZW4=');

@$core.Deprecated('Use geminiTenantModelProviderDataDescriptor instead')
const GeminiTenantModelProviderData$json = {
  '1': 'GeminiTenantModelProviderData',
  '2': [
    {
      '1': 'upstream_model',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'upstreamModel',
      '17': true
    },
  ],
  '8': [
    {'1': '_upstream_model'},
  ],
};

/// Descriptor for `GeminiTenantModelProviderData`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List geminiTenantModelProviderDataDescriptor =
    $convert.base64Decode(
        'Ch1HZW1pbmlUZW5hbnRNb2RlbFByb3ZpZGVyRGF0YRIqCg51cHN0cmVhbV9tb2RlbBgBIAEoCU'
        'gAUg11cHN0cmVhbU1vZGVsiAEBQhEKD191cHN0cmVhbV9tb2RlbA==');

@$core.Deprecated('Use geminiTenantVoiceProviderDataDescriptor instead')
const GeminiTenantVoiceProviderData$json = {
  '1': 'GeminiTenantVoiceProviderData',
  '2': [
    {
      '1': 'raw',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '10': 'raw'
    },
    {
      '1': 'voice_id',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'voiceId',
      '17': true
    },
  ],
  '8': [
    {'1': '_voice_id'},
  ],
};

/// Descriptor for `GeminiTenantVoiceProviderData`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List geminiTenantVoiceProviderDataDescriptor =
    $convert.base64Decode(
        'Ch1HZW1pbmlUZW5hbnRWb2ljZVByb3ZpZGVyRGF0YRIpCgNyYXcYASABKAsyFy5nb29nbGUucH'
        'JvdG9idWYuU3RydWN0UgNyYXcSHgoIdm9pY2VfaWQYAiABKAlIAFIHdm9pY2VJZIgBAUILCglf'
        'dm9pY2VfaWQ=');

@$core.Deprecated('Use miniMaxCredentialBodyDescriptor instead')
const MiniMaxCredentialBody$json = {
  '1': 'MiniMaxCredentialBody',
  '2': [
    {
      '1': 'api_key',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'apiKey',
      '17': true
    },
    {
      '1': 'base_url',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'baseUrl',
      '17': true
    },
    {
      '1': 'minimax_voice_base_url',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'minimaxVoiceBaseUrl',
      '17': true
    },
    {'1': 'token', '3': 4, '4': 1, '5': 9, '9': 3, '10': 'token', '17': true},
    {
      '1': 'voice_base_url',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'voiceBaseUrl',
      '17': true
    },
  ],
  '8': [
    {'1': '_api_key'},
    {'1': '_base_url'},
    {'1': '_minimax_voice_base_url'},
    {'1': '_token'},
    {'1': '_voice_base_url'},
  ],
};

/// Descriptor for `MiniMaxCredentialBody`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List miniMaxCredentialBodyDescriptor = $convert.base64Decode(
    'ChVNaW5pTWF4Q3JlZGVudGlhbEJvZHkSHAoHYXBpX2tleRgBIAEoCUgAUgZhcGlLZXmIAQESHg'
    'oIYmFzZV91cmwYAiABKAlIAVIHYmFzZVVybIgBARI4ChZtaW5pbWF4X3ZvaWNlX2Jhc2VfdXJs'
    'GAMgASgJSAJSE21pbmltYXhWb2ljZUJhc2VVcmyIAQESGQoFdG9rZW4YBCABKAlIA1IFdG9rZW'
    '6IAQESKQoOdm9pY2VfYmFzZV91cmwYBSABKAlIBFIMdm9pY2VCYXNlVXJsiAEBQgoKCF9hcGlf'
    'a2V5QgsKCV9iYXNlX3VybEIZChdfbWluaW1heF92b2ljZV9iYXNlX3VybEIICgZfdG9rZW5CEQ'
    'oPX3ZvaWNlX2Jhc2VfdXJs');

@$core.Deprecated('Use miniMaxTenantVoiceProviderDataDescriptor instead')
const MiniMaxTenantVoiceProviderData$json = {
  '1': 'MiniMaxTenantVoiceProviderData',
  '2': [
    {'1': 'format', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'format', '17': true},
    {'1': 'model', '3': 2, '4': 1, '5': 9, '9': 1, '10': 'model', '17': true},
    {
      '1': 'raw',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '10': 'raw'
    },
    {
      '1': 'sample_rate',
      '3': 4,
      '4': 1,
      '5': 3,
      '9': 2,
      '10': 'sampleRate',
      '17': true
    },
    {
      '1': 'voice_id',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'voiceId',
      '17': true
    },
    {
      '1': 'voice_type',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'voiceType',
      '17': true
    },
  ],
  '8': [
    {'1': '_format'},
    {'1': '_model'},
    {'1': '_sample_rate'},
    {'1': '_voice_id'},
    {'1': '_voice_type'},
  ],
};

/// Descriptor for `MiniMaxTenantVoiceProviderData`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List miniMaxTenantVoiceProviderDataDescriptor = $convert.base64Decode(
    'Ch5NaW5pTWF4VGVuYW50Vm9pY2VQcm92aWRlckRhdGESGwoGZm9ybWF0GAEgASgJSABSBmZvcm'
    '1hdIgBARIZCgVtb2RlbBgCIAEoCUgBUgVtb2RlbIgBARIpCgNyYXcYAyABKAsyFy5nb29nbGUu'
    'cHJvdG9idWYuU3RydWN0UgNyYXcSJAoLc2FtcGxlX3JhdGUYBCABKANIAlIKc2FtcGxlUmF0ZY'
    'gBARIeCgh2b2ljZV9pZBgFIAEoCUgDUgd2b2ljZUlkiAEBEiIKCnZvaWNlX3R5cGUYBiABKAlI'
    'BFIJdm9pY2VUeXBliAEBQgkKB19mb3JtYXRCCAoGX21vZGVsQg4KDF9zYW1wbGVfcmF0ZUILCg'
    'lfdm9pY2VfaWRCDQoLX3ZvaWNlX3R5cGU=');

@$core.Deprecated('Use modelDescriptor instead')
const Model$json = {
  '1': 'Model',
  '2': [
    {
      '1': 'capabilities',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ModelCapabilities',
      '9': 0,
      '10': 'capabilities',
      '17': true
    },
    {'1': 'created_at', '3': 2, '4': 1, '5': 9, '10': 'createdAt'},
    {
      '1': 'description',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'description',
      '17': true
    },
    {'1': 'id', '3': 4, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'kind',
      '3': 5,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ModelKind',
      '10': 'kind'
    },
    {'1': 'name', '3': 6, '4': 1, '5': 9, '9': 2, '10': 'name', '17': true},
    {
      '1': 'provider',
      '3': 7,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ModelProvider',
      '10': 'provider'
    },
    {
      '1': 'provider_data',
      '3': 8,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ModelProviderData',
      '9': 3,
      '10': 'providerData',
      '17': true
    },
    {
      '1': 'source',
      '3': 9,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ModelSource',
      '10': 'source'
    },
    {
      '1': 'synced_at',
      '3': 10,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'syncedAt',
      '17': true
    },
    {'1': 'updated_at', '3': 11, '4': 1, '5': 9, '10': 'updatedAt'},
  ],
  '8': [
    {'1': '_capabilities'},
    {'1': '_description'},
    {'1': '_name'},
    {'1': '_provider_data'},
    {'1': '_synced_at'},
  ],
};

/// Descriptor for `Model`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelDescriptor = $convert.base64Decode(
    'CgVNb2RlbBJKCgxjYXBhYmlsaXRpZXMYASABKAsyIS5naXpjbGF3LnJwYy52MS5Nb2RlbENhcG'
    'FiaWxpdGllc0gAUgxjYXBhYmlsaXRpZXOIAQESHQoKY3JlYXRlZF9hdBgCIAEoCVIJY3JlYXRl'
    'ZEF0EiUKC2Rlc2NyaXB0aW9uGAMgASgJSAFSC2Rlc2NyaXB0aW9uiAEBEg4KAmlkGAQgASgJUg'
    'JpZBItCgRraW5kGAUgASgOMhkuZ2l6Y2xhdy5ycGMudjEuTW9kZWxLaW5kUgRraW5kEhcKBG5h'
    'bWUYBiABKAlIAlIEbmFtZYgBARI5Cghwcm92aWRlchgHIAEoCzIdLmdpemNsYXcucnBjLnYxLk'
    '1vZGVsUHJvdmlkZXJSCHByb3ZpZGVyEksKDXByb3ZpZGVyX2RhdGEYCCABKAsyIS5naXpjbGF3'
    'LnJwYy52MS5Nb2RlbFByb3ZpZGVyRGF0YUgDUgxwcm92aWRlckRhdGGIAQESMwoGc291cmNlGA'
    'kgASgOMhsuZ2l6Y2xhdy5ycGMudjEuTW9kZWxTb3VyY2VSBnNvdXJjZRIgCglzeW5jZWRfYXQY'
    'CiABKAlIBFIIc3luY2VkQXSIAQESHQoKdXBkYXRlZF9hdBgLIAEoCVIJdXBkYXRlZEF0Qg8KDV'
    '9jYXBhYmlsaXRpZXNCDgoMX2Rlc2NyaXB0aW9uQgcKBV9uYW1lQhAKDl9wcm92aWRlcl9kYXRh'
    'QgwKCl9zeW5jZWRfYXQ=');

@$core.Deprecated('Use modelCapabilitiesDescriptor instead')
const ModelCapabilities$json = {
  '1': 'ModelCapabilities',
  '2': [
    {
      '1': 'json_output',
      '3': 1,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'jsonOutput',
      '17': true
    },
    {
      '1': 'system_role',
      '3': 2,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'systemRole',
      '17': true
    },
    {
      '1': 'temperature',
      '3': 3,
      '4': 1,
      '5': 8,
      '9': 2,
      '10': 'temperature',
      '17': true
    },
    {
      '1': 'text_only',
      '3': 4,
      '4': 1,
      '5': 8,
      '9': 3,
      '10': 'textOnly',
      '17': true
    },
    {
      '1': 'thinking',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ModelThinkingCapability',
      '9': 4,
      '10': 'thinking',
      '17': true
    },
    {
      '1': 'tool_calls',
      '3': 6,
      '4': 1,
      '5': 8,
      '9': 5,
      '10': 'toolCalls',
      '17': true
    },
  ],
  '8': [
    {'1': '_json_output'},
    {'1': '_system_role'},
    {'1': '_temperature'},
    {'1': '_text_only'},
    {'1': '_thinking'},
    {'1': '_tool_calls'},
  ],
};

/// Descriptor for `ModelCapabilities`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelCapabilitiesDescriptor = $convert.base64Decode(
    'ChFNb2RlbENhcGFiaWxpdGllcxIkCgtqc29uX291dHB1dBgBIAEoCEgAUgpqc29uT3V0cHV0iA'
    'EBEiQKC3N5c3RlbV9yb2xlGAIgASgISAFSCnN5c3RlbVJvbGWIAQESJQoLdGVtcGVyYXR1cmUY'
    'AyABKAhIAlILdGVtcGVyYXR1cmWIAQESIAoJdGV4dF9vbmx5GAQgASgISANSCHRleHRPbmx5iA'
    'EBEkgKCHRoaW5raW5nGAUgASgLMicuZ2l6Y2xhdy5ycGMudjEuTW9kZWxUaGlua2luZ0NhcGFi'
    'aWxpdHlIBFIIdGhpbmtpbmeIAQESIgoKdG9vbF9jYWxscxgGIAEoCEgFUgl0b29sQ2FsbHOIAQ'
    'FCDgoMX2pzb25fb3V0cHV0Qg4KDF9zeXN0ZW1fcm9sZUIOCgxfdGVtcGVyYXR1cmVCDAoKX3Rl'
    'eHRfb25seUILCglfdGhpbmtpbmdCDQoLX3Rvb2xfY2FsbHM=');

@$core.Deprecated('Use modelCreateRequestDescriptor instead')
const ModelCreateRequest$json = {
  '1': 'ModelCreateRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Model',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ModelCreateRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelCreateRequestDescriptor = $convert.base64Decode(
    'ChJNb2RlbENyZWF0ZVJlcXVlc3QSKwoFdmFsdWUYASABKAsyFS5naXpjbGF3LnJwYy52MS5Nb2'
    'RlbFIFdmFsdWU=');

@$core.Deprecated('Use modelCreateResponseDescriptor instead')
const ModelCreateResponse$json = {
  '1': 'ModelCreateResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Model',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ModelCreateResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelCreateResponseDescriptor = $convert.base64Decode(
    'ChNNb2RlbENyZWF0ZVJlc3BvbnNlEisKBXZhbHVlGAEgASgLMhUuZ2l6Y2xhdy5ycGMudjEuTW'
    '9kZWxSBXZhbHVl');

@$core.Deprecated('Use modelDeleteRequestDescriptor instead')
const ModelDeleteRequest$json = {
  '1': 'ModelDeleteRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `ModelDeleteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelDeleteRequestDescriptor =
    $convert.base64Decode('ChJNb2RlbERlbGV0ZVJlcXVlc3QSDgoCaWQYASABKAlSAmlk');

@$core.Deprecated('Use modelDeleteResponseDescriptor instead')
const ModelDeleteResponse$json = {
  '1': 'ModelDeleteResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Model',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ModelDeleteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelDeleteResponseDescriptor = $convert.base64Decode(
    'ChNNb2RlbERlbGV0ZVJlc3BvbnNlEisKBXZhbHVlGAEgASgLMhUuZ2l6Y2xhdy5ycGMudjEuTW'
    '9kZWxSBXZhbHVl');

@$core.Deprecated('Use modelGetRequestDescriptor instead')
const ModelGetRequest$json = {
  '1': 'ModelGetRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `ModelGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelGetRequestDescriptor =
    $convert.base64Decode('Cg9Nb2RlbEdldFJlcXVlc3QSDgoCaWQYASABKAlSAmlk');

@$core.Deprecated('Use modelGetResponseDescriptor instead')
const ModelGetResponse$json = {
  '1': 'ModelGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Model',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ModelGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelGetResponseDescriptor = $convert.base64Decode(
    'ChBNb2RlbEdldFJlc3BvbnNlEisKBXZhbHVlGAEgASgLMhUuZ2l6Y2xhdy5ycGMudjEuTW9kZW'
    'xSBXZhbHVl');

@$core.Deprecated('Use modelListRequestDescriptor instead')
const ModelListRequest$json = {
  '1': 'ModelListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `ModelListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelListRequestDescriptor = $convert.base64Decode(
    'ChBNb2RlbExpc3RSZXF1ZXN0EhsKBmN1cnNvchgBIAEoCUgAUgZjdXJzb3KIAQESGQoFbGltaX'
    'QYAiABKANIAVIFbGltaXSIAQFCCQoHX2N1cnNvckIICgZfbGltaXQ=');

@$core.Deprecated('Use modelListResponseDescriptor instead')
const ModelListResponse$json = {
  '1': 'ModelListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Model',
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

/// Descriptor for `ModelListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelListResponseDescriptor = $convert.base64Decode(
    'ChFNb2RlbExpc3RSZXNwb25zZRIZCghoYXNfbmV4dBgBIAEoCFIHaGFzTmV4dBIrCgVpdGVtcx'
    'gCIAMoCzIVLmdpemNsYXcucnBjLnYxLk1vZGVsUgVpdGVtcxIkCgtuZXh0X2N1cnNvchgDIAEo'
    'CUgAUgpuZXh0Q3Vyc29yiAEBQg4KDF9uZXh0X2N1cnNvcg==');

@$core.Deprecated('Use modelProviderDescriptor instead')
const ModelProvider$json = {
  '1': 'ModelProvider',
  '2': [
    {
      '1': 'kind',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ModelProviderKind',
      '10': 'kind'
    },
    {'1': 'name', '3': 2, '4': 1, '5': 9, '10': 'name'},
  ],
};

/// Descriptor for `ModelProvider`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelProviderDescriptor = $convert.base64Decode(
    'Cg1Nb2RlbFByb3ZpZGVyEjUKBGtpbmQYASABKA4yIS5naXpjbGF3LnJwYy52MS5Nb2RlbFByb3'
    'ZpZGVyS2luZFIEa2luZBISCgRuYW1lGAIgASgJUgRuYW1l');

@$core.Deprecated('Use modelProviderDataDescriptor instead')
const ModelProviderData$json = {
  '1': 'ModelProviderData',
  '2': [
    {
      '1': 'gemini_tenant_model_provider_data',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GeminiTenantModelProviderData',
      '9': 0,
      '10': 'geminiTenantModelProviderData'
    },
    {
      '1': 'dash_scope_tenant_model_provider_data',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DashScopeTenantModelProviderData',
      '9': 0,
      '10': 'dashScopeTenantModelProviderData'
    },
    {
      '1': 'open_aitenant_model_provider_data',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.OpenAITenantModelProviderData',
      '9': 0,
      '10': 'openAitenantModelProviderData'
    },
    {
      '1': 'volc_tenant_model_provider_data',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.VolcTenantModelProviderData',
      '9': 0,
      '10': 'volcTenantModelProviderData'
    },
  ],
  '8': [
    {'1': 'value'},
  ],
};

/// Descriptor for `ModelProviderData`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelProviderDataDescriptor = $convert.base64Decode(
    'ChFNb2RlbFByb3ZpZGVyRGF0YRJ5CiFnZW1pbmlfdGVuYW50X21vZGVsX3Byb3ZpZGVyX2RhdG'
    'EYASABKAsyLS5naXpjbGF3LnJwYy52MS5HZW1pbmlUZW5hbnRNb2RlbFByb3ZpZGVyRGF0YUgA'
    'Uh1nZW1pbmlUZW5hbnRNb2RlbFByb3ZpZGVyRGF0YRKDAQolZGFzaF9zY29wZV90ZW5hbnRfbW'
    '9kZWxfcHJvdmlkZXJfZGF0YRgCIAEoCzIwLmdpemNsYXcucnBjLnYxLkRhc2hTY29wZVRlbmFu'
    'dE1vZGVsUHJvdmlkZXJEYXRhSABSIGRhc2hTY29wZVRlbmFudE1vZGVsUHJvdmlkZXJEYXRhEn'
    'kKIW9wZW5fYWl0ZW5hbnRfbW9kZWxfcHJvdmlkZXJfZGF0YRgDIAEoCzItLmdpemNsYXcucnBj'
    'LnYxLk9wZW5BSVRlbmFudE1vZGVsUHJvdmlkZXJEYXRhSABSHW9wZW5BaXRlbmFudE1vZGVsUH'
    'JvdmlkZXJEYXRhEnMKH3ZvbGNfdGVuYW50X21vZGVsX3Byb3ZpZGVyX2RhdGEYBCABKAsyKy5n'
    'aXpjbGF3LnJwYy52MS5Wb2xjVGVuYW50TW9kZWxQcm92aWRlckRhdGFIAFIbdm9sY1RlbmFudE'
    '1vZGVsUHJvdmlkZXJEYXRhQgcKBXZhbHVl');

@$core.Deprecated('Use modelPutRequestDescriptor instead')
const ModelPutRequest$json = {
  '1': 'ModelPutRequest',
  '2': [
    {
      '1': 'body',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Model',
      '10': 'body'
    },
    {'1': 'id', '3': 2, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `ModelPutRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelPutRequestDescriptor = $convert.base64Decode(
    'Cg9Nb2RlbFB1dFJlcXVlc3QSKQoEYm9keRgBIAEoCzIVLmdpemNsYXcucnBjLnYxLk1vZGVsUg'
    'Rib2R5Eg4KAmlkGAIgASgJUgJpZA==');

@$core.Deprecated('Use modelPutResponseDescriptor instead')
const ModelPutResponse$json = {
  '1': 'ModelPutResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Model',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ModelPutResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelPutResponseDescriptor = $convert.base64Decode(
    'ChBNb2RlbFB1dFJlc3BvbnNlEisKBXZhbHVlGAEgASgLMhUuZ2l6Y2xhdy5ycGMudjEuTW9kZW'
    'xSBXZhbHVl');

@$core.Deprecated('Use modelThinkingCapabilityDescriptor instead')
const ModelThinkingCapability$json = {
  '1': 'ModelThinkingCapability',
  '2': [
    {
      '1': 'default_level',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'defaultLevel',
      '17': true
    },
    {
      '1': 'level_param',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'levelParam',
      '17': true
    },
    {'1': 'levels', '3': 3, '4': 3, '5': 9, '10': 'levels'},
    {'1': 'param', '3': 4, '4': 1, '5': 9, '9': 2, '10': 'param', '17': true},
    {'1': 'supported', '3': 5, '4': 1, '5': 8, '10': 'supported'},
  ],
  '8': [
    {'1': '_default_level'},
    {'1': '_level_param'},
    {'1': '_param'},
  ],
};

/// Descriptor for `ModelThinkingCapability`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelThinkingCapabilityDescriptor = $convert.base64Decode(
    'ChdNb2RlbFRoaW5raW5nQ2FwYWJpbGl0eRIoCg1kZWZhdWx0X2xldmVsGAEgASgJSABSDGRlZm'
    'F1bHRMZXZlbIgBARIkCgtsZXZlbF9wYXJhbRgCIAEoCUgBUgpsZXZlbFBhcmFtiAEBEhYKBmxl'
    'dmVscxgDIAMoCVIGbGV2ZWxzEhkKBXBhcmFtGAQgASgJSAJSBXBhcmFtiAEBEhwKCXN1cHBvcn'
    'RlZBgFIAEoCFIJc3VwcG9ydGVkQhAKDl9kZWZhdWx0X2xldmVsQg4KDF9sZXZlbF9wYXJhbUII'
    'CgZfcGFyYW0=');

@$core.Deprecated('Use openAICredentialBodyDescriptor instead')
const OpenAICredentialBody$json = {
  '1': 'OpenAICredentialBody',
  '2': [
    {
      '1': 'api_key',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'apiKey',
      '17': true
    },
    {
      '1': 'base_url',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'baseUrl',
      '17': true
    },
    {
      '1': 'organization',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'organization',
      '17': true
    },
    {
      '1': 'project',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'project',
      '17': true
    },
    {'1': 'token', '3': 5, '4': 1, '5': 9, '9': 4, '10': 'token', '17': true},
  ],
  '8': [
    {'1': '_api_key'},
    {'1': '_base_url'},
    {'1': '_organization'},
    {'1': '_project'},
    {'1': '_token'},
  ],
};

/// Descriptor for `OpenAICredentialBody`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List openAICredentialBodyDescriptor = $convert.base64Decode(
    'ChRPcGVuQUlDcmVkZW50aWFsQm9keRIcCgdhcGlfa2V5GAEgASgJSABSBmFwaUtleYgBARIeCg'
    'hiYXNlX3VybBgCIAEoCUgBUgdiYXNlVXJsiAEBEicKDG9yZ2FuaXphdGlvbhgDIAEoCUgCUgxv'
    'cmdhbml6YXRpb26IAQESHQoHcHJvamVjdBgEIAEoCUgDUgdwcm9qZWN0iAEBEhkKBXRva2VuGA'
    'UgASgJSARSBXRva2VuiAEBQgoKCF9hcGlfa2V5QgsKCV9iYXNlX3VybEIPCg1fb3JnYW5pemF0'
    'aW9uQgoKCF9wcm9qZWN0QggKBl90b2tlbg==');

@$core.Deprecated('Use openAITenantModelProviderDataDescriptor instead')
const OpenAITenantModelProviderData$json = {
  '1': 'OpenAITenantModelProviderData',
  '2': [
    {
      '1': 'default_thinking_level',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'defaultThinkingLevel',
      '17': true
    },
    {
      '1': 'support_json_output',
      '3': 2,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'supportJsonOutput',
      '17': true
    },
    {
      '1': 'support_text_only',
      '3': 3,
      '4': 1,
      '5': 8,
      '9': 2,
      '10': 'supportTextOnly',
      '17': true
    },
    {
      '1': 'support_thinking',
      '3': 4,
      '4': 1,
      '5': 8,
      '9': 3,
      '10': 'supportThinking',
      '17': true
    },
    {
      '1': 'support_tool_calls',
      '3': 5,
      '4': 1,
      '5': 8,
      '9': 4,
      '10': 'supportToolCalls',
      '17': true
    },
    {
      '1': 'thinking_level_param',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 5,
      '10': 'thinkingLevelParam',
      '17': true
    },
    {'1': 'thinking_levels', '3': 7, '4': 3, '5': 9, '10': 'thinkingLevels'},
    {
      '1': 'thinking_param',
      '3': 8,
      '4': 1,
      '5': 9,
      '9': 6,
      '10': 'thinkingParam',
      '17': true
    },
    {
      '1': 'upstream_model',
      '3': 9,
      '4': 1,
      '5': 9,
      '9': 7,
      '10': 'upstreamModel',
      '17': true
    },
    {
      '1': 'use_system_role',
      '3': 10,
      '4': 1,
      '5': 8,
      '9': 8,
      '10': 'useSystemRole',
      '17': true
    },
  ],
  '8': [
    {'1': '_default_thinking_level'},
    {'1': '_support_json_output'},
    {'1': '_support_text_only'},
    {'1': '_support_thinking'},
    {'1': '_support_tool_calls'},
    {'1': '_thinking_level_param'},
    {'1': '_thinking_param'},
    {'1': '_upstream_model'},
    {'1': '_use_system_role'},
  ],
};

/// Descriptor for `OpenAITenantModelProviderData`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List openAITenantModelProviderDataDescriptor = $convert.base64Decode(
    'Ch1PcGVuQUlUZW5hbnRNb2RlbFByb3ZpZGVyRGF0YRI5ChZkZWZhdWx0X3RoaW5raW5nX2xldm'
    'VsGAEgASgJSABSFGRlZmF1bHRUaGlua2luZ0xldmVsiAEBEjMKE3N1cHBvcnRfanNvbl9vdXRw'
    'dXQYAiABKAhIAVIRc3VwcG9ydEpzb25PdXRwdXSIAQESLwoRc3VwcG9ydF90ZXh0X29ubHkYAy'
    'ABKAhIAlIPc3VwcG9ydFRleHRPbmx5iAEBEi4KEHN1cHBvcnRfdGhpbmtpbmcYBCABKAhIA1IP'
    'c3VwcG9ydFRoaW5raW5niAEBEjEKEnN1cHBvcnRfdG9vbF9jYWxscxgFIAEoCEgEUhBzdXBwb3'
    'J0VG9vbENhbGxziAEBEjUKFHRoaW5raW5nX2xldmVsX3BhcmFtGAYgASgJSAVSEnRoaW5raW5n'
    'TGV2ZWxQYXJhbYgBARInCg90aGlua2luZ19sZXZlbHMYByADKAlSDnRoaW5raW5nTGV2ZWxzEi'
    'oKDnRoaW5raW5nX3BhcmFtGAggASgJSAZSDXRoaW5raW5nUGFyYW2IAQESKgoOdXBzdHJlYW1f'
    'bW9kZWwYCSABKAlIB1INdXBzdHJlYW1Nb2RlbIgBARIrCg91c2Vfc3lzdGVtX3JvbGUYCiABKA'
    'hICFINdXNlU3lzdGVtUm9sZYgBAUIZChdfZGVmYXVsdF90aGlua2luZ19sZXZlbEIWChRfc3Vw'
    'cG9ydF9qc29uX291dHB1dEIUChJfc3VwcG9ydF90ZXh0X29ubHlCEwoRX3N1cHBvcnRfdGhpbm'
    'tpbmdCFQoTX3N1cHBvcnRfdG9vbF9jYWxsc0IXChVfdGhpbmtpbmdfbGV2ZWxfcGFyYW1CEQoP'
    'X3RoaW5raW5nX3BhcmFtQhEKD191cHN0cmVhbV9tb2RlbEISChBfdXNlX3N5c3RlbV9yb2xl');

@$core.Deprecated('Use openAITenantVoiceProviderDataDescriptor instead')
const OpenAITenantVoiceProviderData$json = {
  '1': 'OpenAITenantVoiceProviderData',
  '2': [
    {
      '1': 'raw',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '10': 'raw'
    },
    {
      '1': 'voice_id',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'voiceId',
      '17': true
    },
  ],
  '8': [
    {'1': '_voice_id'},
  ],
};

/// Descriptor for `OpenAITenantVoiceProviderData`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List openAITenantVoiceProviderDataDescriptor =
    $convert.base64Decode(
        'Ch1PcGVuQUlUZW5hbnRWb2ljZVByb3ZpZGVyRGF0YRIpCgNyYXcYASABKAsyFy5nb29nbGUucH'
        'JvdG9idWYuU3RydWN0UgNyYXcSHgoIdm9pY2VfaWQYAiABKAlIAFIHdm9pY2VJZIgBAUILCglf'
        'dm9pY2VfaWQ=');

@$core.Deprecated('Use voiceDescriptor instead')
const Voice$json = {
  '1': 'Voice',
  '2': [
    {'1': 'created_at', '3': 1, '4': 1, '5': 9, '10': 'createdAt'},
    {
      '1': 'description',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'description',
      '17': true
    },
    {'1': 'id', '3': 3, '4': 1, '5': 9, '10': 'id'},
    {'1': 'name', '3': 4, '4': 1, '5': 9, '9': 1, '10': 'name', '17': true},
    {
      '1': 'provider',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.VoiceProvider',
      '10': 'provider'
    },
    {
      '1': 'provider_data',
      '3': 6,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.VoiceProviderData',
      '9': 2,
      '10': 'providerData',
      '17': true
    },
    {
      '1': 'source',
      '3': 7,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.VoiceSource',
      '10': 'source'
    },
    {
      '1': 'synced_at',
      '3': 8,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'syncedAt',
      '17': true
    },
    {'1': 'updated_at', '3': 9, '4': 1, '5': 9, '10': 'updatedAt'},
  ],
  '8': [
    {'1': '_description'},
    {'1': '_name'},
    {'1': '_provider_data'},
    {'1': '_synced_at'},
  ],
};

/// Descriptor for `Voice`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List voiceDescriptor = $convert.base64Decode(
    'CgVWb2ljZRIdCgpjcmVhdGVkX2F0GAEgASgJUgljcmVhdGVkQXQSJQoLZGVzY3JpcHRpb24YAi'
    'ABKAlIAFILZGVzY3JpcHRpb26IAQESDgoCaWQYAyABKAlSAmlkEhcKBG5hbWUYBCABKAlIAVIE'
    'bmFtZYgBARI5Cghwcm92aWRlchgFIAEoCzIdLmdpemNsYXcucnBjLnYxLlZvaWNlUHJvdmlkZX'
    'JSCHByb3ZpZGVyEksKDXByb3ZpZGVyX2RhdGEYBiABKAsyIS5naXpjbGF3LnJwYy52MS5Wb2lj'
    'ZVByb3ZpZGVyRGF0YUgCUgxwcm92aWRlckRhdGGIAQESMwoGc291cmNlGAcgASgOMhsuZ2l6Y2'
    'xhdy5ycGMudjEuVm9pY2VTb3VyY2VSBnNvdXJjZRIgCglzeW5jZWRfYXQYCCABKAlIA1IIc3lu'
    'Y2VkQXSIAQESHQoKdXBkYXRlZF9hdBgJIAEoCVIJdXBkYXRlZEF0Qg4KDF9kZXNjcmlwdGlvbk'
    'IHCgVfbmFtZUIQCg5fcHJvdmlkZXJfZGF0YUIMCgpfc3luY2VkX2F0');

@$core.Deprecated('Use voiceGetRequestDescriptor instead')
const VoiceGetRequest$json = {
  '1': 'VoiceGetRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `VoiceGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List voiceGetRequestDescriptor =
    $convert.base64Decode('Cg9Wb2ljZUdldFJlcXVlc3QSDgoCaWQYASABKAlSAmlk');

@$core.Deprecated('Use voiceGetResponseDescriptor instead')
const VoiceGetResponse$json = {
  '1': 'VoiceGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Voice',
      '10': 'value'
    },
  ],
};

/// Descriptor for `VoiceGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List voiceGetResponseDescriptor = $convert.base64Decode(
    'ChBWb2ljZUdldFJlc3BvbnNlEisKBXZhbHVlGAEgASgLMhUuZ2l6Y2xhdy5ycGMudjEuVm9pY2'
    'VSBXZhbHVl');

@$core.Deprecated('Use voiceListRequestDescriptor instead')
const VoiceListRequest$json = {
  '1': 'VoiceListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `VoiceListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List voiceListRequestDescriptor = $convert.base64Decode(
    'ChBWb2ljZUxpc3RSZXF1ZXN0EhsKBmN1cnNvchgBIAEoCUgAUgZjdXJzb3KIAQESGQoFbGltaX'
    'QYAiABKANIAVIFbGltaXSIAQFCCQoHX2N1cnNvckIICgZfbGltaXQ=');

@$core.Deprecated('Use voiceListResponseDescriptor instead')
const VoiceListResponse$json = {
  '1': 'VoiceListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Voice',
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

/// Descriptor for `VoiceListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List voiceListResponseDescriptor = $convert.base64Decode(
    'ChFWb2ljZUxpc3RSZXNwb25zZRIZCghoYXNfbmV4dBgBIAEoCFIHaGFzTmV4dBIrCgVpdGVtcx'
    'gCIAMoCzIVLmdpemNsYXcucnBjLnYxLlZvaWNlUgVpdGVtcxIkCgtuZXh0X2N1cnNvchgDIAEo'
    'CUgAUgpuZXh0Q3Vyc29yiAEBQg4KDF9uZXh0X2N1cnNvcg==');

@$core.Deprecated('Use voiceProviderDescriptor instead')
const VoiceProvider$json = {
  '1': 'VoiceProvider',
  '2': [
    {
      '1': 'kind',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.VoiceProviderKind',
      '10': 'kind'
    },
    {'1': 'name', '3': 2, '4': 1, '5': 9, '10': 'name'},
  ],
};

/// Descriptor for `VoiceProvider`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List voiceProviderDescriptor = $convert.base64Decode(
    'Cg1Wb2ljZVByb3ZpZGVyEjUKBGtpbmQYASABKA4yIS5naXpjbGF3LnJwYy52MS5Wb2ljZVByb3'
    'ZpZGVyS2luZFIEa2luZBISCgRuYW1lGAIgASgJUgRuYW1l');

@$core.Deprecated('Use voiceProviderDataDescriptor instead')
const VoiceProviderData$json = {
  '1': 'VoiceProviderData',
  '2': [
    {
      '1': 'gemini_tenant_voice_provider_data',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.GeminiTenantVoiceProviderData',
      '9': 0,
      '10': 'geminiTenantVoiceProviderData'
    },
    {
      '1': 'dash_scope_tenant_voice_provider_data',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DashScopeTenantVoiceProviderData',
      '9': 0,
      '10': 'dashScopeTenantVoiceProviderData'
    },
    {
      '1': 'open_aitenant_voice_provider_data',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.OpenAITenantVoiceProviderData',
      '9': 0,
      '10': 'openAitenantVoiceProviderData'
    },
    {
      '1': 'mini_max_tenant_voice_provider_data',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.MiniMaxTenantVoiceProviderData',
      '9': 0,
      '10': 'miniMaxTenantVoiceProviderData'
    },
    {
      '1': 'volc_tenant_voice_provider_data',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.VolcTenantVoiceProviderData',
      '9': 0,
      '10': 'volcTenantVoiceProviderData'
    },
  ],
  '8': [
    {'1': 'value'},
  ],
};

/// Descriptor for `VoiceProviderData`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List voiceProviderDataDescriptor = $convert.base64Decode(
    'ChFWb2ljZVByb3ZpZGVyRGF0YRJ5CiFnZW1pbmlfdGVuYW50X3ZvaWNlX3Byb3ZpZGVyX2RhdG'
    'EYASABKAsyLS5naXpjbGF3LnJwYy52MS5HZW1pbmlUZW5hbnRWb2ljZVByb3ZpZGVyRGF0YUgA'
    'Uh1nZW1pbmlUZW5hbnRWb2ljZVByb3ZpZGVyRGF0YRKDAQolZGFzaF9zY29wZV90ZW5hbnRfdm'
    '9pY2VfcHJvdmlkZXJfZGF0YRgCIAEoCzIwLmdpemNsYXcucnBjLnYxLkRhc2hTY29wZVRlbmFu'
    'dFZvaWNlUHJvdmlkZXJEYXRhSABSIGRhc2hTY29wZVRlbmFudFZvaWNlUHJvdmlkZXJEYXRhEn'
    'kKIW9wZW5fYWl0ZW5hbnRfdm9pY2VfcHJvdmlkZXJfZGF0YRgDIAEoCzItLmdpemNsYXcucnBj'
    'LnYxLk9wZW5BSVRlbmFudFZvaWNlUHJvdmlkZXJEYXRhSABSHW9wZW5BaXRlbmFudFZvaWNlUH'
    'JvdmlkZXJEYXRhEn0KI21pbmlfbWF4X3RlbmFudF92b2ljZV9wcm92aWRlcl9kYXRhGAQgASgL'
    'Mi4uZ2l6Y2xhdy5ycGMudjEuTWluaU1heFRlbmFudFZvaWNlUHJvdmlkZXJEYXRhSABSHm1pbm'
    'lNYXhUZW5hbnRWb2ljZVByb3ZpZGVyRGF0YRJzCh92b2xjX3RlbmFudF92b2ljZV9wcm92aWRl'
    'cl9kYXRhGAUgASgLMisuZ2l6Y2xhdy5ycGMudjEuVm9sY1RlbmFudFZvaWNlUHJvdmlkZXJEYX'
    'RhSABSG3ZvbGNUZW5hbnRWb2ljZVByb3ZpZGVyRGF0YUIHCgV2YWx1ZQ==');

@$core.Deprecated('Use volcCredentialBodyDescriptor instead')
const VolcCredentialBody$json = {
  '1': 'VolcCredentialBody',
  '2': [
    {
      '1': 'api_key',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'apiKey',
      '17': true
    },
    {'1': 'app_id', '3': 2, '4': 1, '5': 9, '9': 1, '10': 'appId', '17': true},
    {
      '1': 'openapi_access_key',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'openapiAccessKey',
      '17': true
    },
    {
      '1': 'openapi_access_key_id',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'openapiAccessKeyId',
      '17': true
    },
    {
      '1': 'search_api_key',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'searchApiKey',
      '17': true
    },
  ],
  '8': [
    {'1': '_api_key'},
    {'1': '_app_id'},
    {'1': '_openapi_access_key'},
    {'1': '_openapi_access_key_id'},
    {'1': '_search_api_key'},
  ],
};

/// Descriptor for `VolcCredentialBody`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List volcCredentialBodyDescriptor = $convert.base64Decode(
    'ChJWb2xjQ3JlZGVudGlhbEJvZHkSHAoHYXBpX2tleRgBIAEoCUgAUgZhcGlLZXmIAQESGgoGYX'
    'BwX2lkGAIgASgJSAFSBWFwcElkiAEBEjEKEm9wZW5hcGlfYWNjZXNzX2tleRgDIAEoCUgCUhBv'
    'cGVuYXBpQWNjZXNzS2V5iAEBEjYKFW9wZW5hcGlfYWNjZXNzX2tleV9pZBgEIAEoCUgDUhJvcG'
    'VuYXBpQWNjZXNzS2V5SWSIAQESKQoOc2VhcmNoX2FwaV9rZXkYBSABKAlIBFIMc2VhcmNoQXBp'
    'S2V5iAEBQgoKCF9hcGlfa2V5QgkKB19hcHBfaWRCFQoTX29wZW5hcGlfYWNjZXNzX2tleUIYCh'
    'Zfb3BlbmFwaV9hY2Nlc3Nfa2V5X2lkQhEKD19zZWFyY2hfYXBpX2tleQ==');

@$core.Deprecated('Use volcTenantModelProviderDataDescriptor instead')
const VolcTenantModelProviderData$json = {
  '1': 'VolcTenantModelProviderData',
  '2': [
    {
      '1': 'api_mode',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.VolcTenantModelProviderDataApiMode',
      '9': 0,
      '10': 'apiMode',
      '17': true
    },
    {
      '1': 'default_thinking_level',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'defaultThinkingLevel',
      '17': true
    },
    {
      '1': 'resource_id',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'resourceId',
      '17': true
    },
    {
      '1': 'support_json_output',
      '3': 4,
      '4': 1,
      '5': 8,
      '9': 3,
      '10': 'supportJsonOutput',
      '17': true
    },
    {
      '1': 'support_text_only',
      '3': 5,
      '4': 1,
      '5': 8,
      '9': 4,
      '10': 'supportTextOnly',
      '17': true
    },
    {
      '1': 'support_thinking',
      '3': 6,
      '4': 1,
      '5': 8,
      '9': 5,
      '10': 'supportThinking',
      '17': true
    },
    {
      '1': 'support_tool_calls',
      '3': 7,
      '4': 1,
      '5': 8,
      '9': 6,
      '10': 'supportToolCalls',
      '17': true
    },
    {
      '1': 'thinking_level_param',
      '3': 8,
      '4': 1,
      '5': 9,
      '9': 7,
      '10': 'thinkingLevelParam',
      '17': true
    },
    {'1': 'thinking_levels', '3': 9, '4': 3, '5': 9, '10': 'thinkingLevels'},
    {
      '1': 'thinking_param',
      '3': 10,
      '4': 1,
      '5': 9,
      '9': 8,
      '10': 'thinkingParam',
      '17': true
    },
    {
      '1': 'upstream_model',
      '3': 11,
      '4': 1,
      '5': 9,
      '9': 9,
      '10': 'upstreamModel',
      '17': true
    },
    {
      '1': 'use_system_role',
      '3': 12,
      '4': 1,
      '5': 8,
      '9': 10,
      '10': 'useSystemRole',
      '17': true
    },
  ],
  '8': [
    {'1': '_api_mode'},
    {'1': '_default_thinking_level'},
    {'1': '_resource_id'},
    {'1': '_support_json_output'},
    {'1': '_support_text_only'},
    {'1': '_support_thinking'},
    {'1': '_support_tool_calls'},
    {'1': '_thinking_level_param'},
    {'1': '_thinking_param'},
    {'1': '_upstream_model'},
    {'1': '_use_system_role'},
  ],
};

/// Descriptor for `VolcTenantModelProviderData`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List volcTenantModelProviderDataDescriptor = $convert.base64Decode(
    'ChtWb2xjVGVuYW50TW9kZWxQcm92aWRlckRhdGESUgoIYXBpX21vZGUYASABKA4yMi5naXpjbG'
    'F3LnJwYy52MS5Wb2xjVGVuYW50TW9kZWxQcm92aWRlckRhdGFBcGlNb2RlSABSB2FwaU1vZGWI'
    'AQESOQoWZGVmYXVsdF90aGlua2luZ19sZXZlbBgCIAEoCUgBUhRkZWZhdWx0VGhpbmtpbmdMZX'
    'ZlbIgBARIkCgtyZXNvdXJjZV9pZBgDIAEoCUgCUgpyZXNvdXJjZUlkiAEBEjMKE3N1cHBvcnRf'
    'anNvbl9vdXRwdXQYBCABKAhIA1IRc3VwcG9ydEpzb25PdXRwdXSIAQESLwoRc3VwcG9ydF90ZX'
    'h0X29ubHkYBSABKAhIBFIPc3VwcG9ydFRleHRPbmx5iAEBEi4KEHN1cHBvcnRfdGhpbmtpbmcY'
    'BiABKAhIBVIPc3VwcG9ydFRoaW5raW5niAEBEjEKEnN1cHBvcnRfdG9vbF9jYWxscxgHIAEoCE'
    'gGUhBzdXBwb3J0VG9vbENhbGxziAEBEjUKFHRoaW5raW5nX2xldmVsX3BhcmFtGAggASgJSAdS'
    'EnRoaW5raW5nTGV2ZWxQYXJhbYgBARInCg90aGlua2luZ19sZXZlbHMYCSADKAlSDnRoaW5raW'
    '5nTGV2ZWxzEioKDnRoaW5raW5nX3BhcmFtGAogASgJSAhSDXRoaW5raW5nUGFyYW2IAQESKgoO'
    'dXBzdHJlYW1fbW9kZWwYCyABKAlICVINdXBzdHJlYW1Nb2RlbIgBARIrCg91c2Vfc3lzdGVtX3'
    'JvbGUYDCABKAhIClINdXNlU3lzdGVtUm9sZYgBAUILCglfYXBpX21vZGVCGQoXX2RlZmF1bHRf'
    'dGhpbmtpbmdfbGV2ZWxCDgoMX3Jlc291cmNlX2lkQhYKFF9zdXBwb3J0X2pzb25fb3V0cHV0Qh'
    'QKEl9zdXBwb3J0X3RleHRfb25seUITChFfc3VwcG9ydF90aGlua2luZ0IVChNfc3VwcG9ydF90'
    'b29sX2NhbGxzQhcKFV90aGlua2luZ19sZXZlbF9wYXJhbUIRCg9fdGhpbmtpbmdfcGFyYW1CEQ'
    'oPX3Vwc3RyZWFtX21vZGVsQhIKEF91c2Vfc3lzdGVtX3JvbGU=');

@$core.Deprecated('Use volcTenantVoiceProviderDataDescriptor instead')
const VolcTenantVoiceProviderData$json = {
  '1': 'VolcTenantVoiceProviderData',
  '2': [
    {
      '1': 'raw',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '10': 'raw'
    },
    {
      '1': 'resource_id',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'resourceId',
      '17': true
    },
    {'1': 'state', '3': 3, '4': 1, '5': 9, '9': 1, '10': 'state', '17': true},
    {'1': 'status', '3': 4, '4': 1, '5': 9, '9': 2, '10': 'status', '17': true},
    {
      '1': 'voice_id',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'voiceId',
      '17': true
    },
  ],
  '8': [
    {'1': '_resource_id'},
    {'1': '_state'},
    {'1': '_status'},
    {'1': '_voice_id'},
  ],
};

/// Descriptor for `VolcTenantVoiceProviderData`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List volcTenantVoiceProviderDataDescriptor = $convert.base64Decode(
    'ChtWb2xjVGVuYW50Vm9pY2VQcm92aWRlckRhdGESKQoDcmF3GAEgASgLMhcuZ29vZ2xlLnByb3'
    'RvYnVmLlN0cnVjdFIDcmF3EiQKC3Jlc291cmNlX2lkGAIgASgJSABSCnJlc291cmNlSWSIAQES'
    'GQoFc3RhdGUYAyABKAlIAVIFc3RhdGWIAQESGwoGc3RhdHVzGAQgASgJSAJSBnN0YXR1c4gBAR'
    'IeCgh2b2ljZV9pZBgFIAEoCUgDUgd2b2ljZUlkiAEBQg4KDF9yZXNvdXJjZV9pZEIICgZfc3Rh'
    'dGVCCQoHX3N0YXR1c0ILCglfdm9pY2VfaWQ=');

@$core.Deprecated('Use workflowCreateRequestDescriptor instead')
const WorkflowCreateRequest$json = {
  '1': 'WorkflowCreateRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkflowDocument',
      '10': 'value'
    },
  ],
};

/// Descriptor for `WorkflowCreateRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowCreateRequestDescriptor = $convert.base64Decode(
    'ChVXb3JrZmxvd0NyZWF0ZVJlcXVlc3QSNgoFdmFsdWUYASABKAsyIC5naXpjbGF3LnJwYy52MS'
    '5Xb3JrZmxvd0RvY3VtZW50UgV2YWx1ZQ==');

@$core.Deprecated('Use workflowCreateResponseDescriptor instead')
const WorkflowCreateResponse$json = {
  '1': 'WorkflowCreateResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkflowDocument',
      '10': 'value'
    },
  ],
};

/// Descriptor for `WorkflowCreateResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowCreateResponseDescriptor =
    $convert.base64Decode(
        'ChZXb3JrZmxvd0NyZWF0ZVJlc3BvbnNlEjYKBXZhbHVlGAEgASgLMiAuZ2l6Y2xhdy5ycGMudj'
        'EuV29ya2Zsb3dEb2N1bWVudFIFdmFsdWU=');

@$core.Deprecated('Use workflowDeleteRequestDescriptor instead')
const WorkflowDeleteRequest$json = {
  '1': 'WorkflowDeleteRequest',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
  ],
};

/// Descriptor for `WorkflowDeleteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowDeleteRequestDescriptor =
    $convert.base64Decode(
        'ChVXb3JrZmxvd0RlbGV0ZVJlcXVlc3QSEgoEbmFtZRgBIAEoCVIEbmFtZQ==');

@$core.Deprecated('Use workflowDeleteResponseDescriptor instead')
const WorkflowDeleteResponse$json = {
  '1': 'WorkflowDeleteResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkflowDocument',
      '10': 'value'
    },
  ],
};

/// Descriptor for `WorkflowDeleteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowDeleteResponseDescriptor =
    $convert.base64Decode(
        'ChZXb3JrZmxvd0RlbGV0ZVJlc3BvbnNlEjYKBXZhbHVlGAEgASgLMiAuZ2l6Y2xhdy5ycGMudj'
        'EuV29ya2Zsb3dEb2N1bWVudFIFdmFsdWU=');

@$core.Deprecated('Use workflowDocumentDescriptor instead')
const WorkflowDocument$json = {
  '1': 'WorkflowDocument',
  '2': [
    {
      '1': 'metadata',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkflowMetadata',
      '10': 'metadata'
    },
    {
      '1': 'spec',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkflowSpec',
      '10': 'spec'
    },
  ],
};

/// Descriptor for `WorkflowDocument`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowDocumentDescriptor = $convert.base64Decode(
    'ChBXb3JrZmxvd0RvY3VtZW50EjwKCG1ldGFkYXRhGAEgASgLMiAuZ2l6Y2xhdy5ycGMudjEuV2'
    '9ya2Zsb3dNZXRhZGF0YVIIbWV0YWRhdGESMAoEc3BlYxgCIAEoCzIcLmdpemNsYXcucnBjLnYx'
    'LldvcmtmbG93U3BlY1IEc3BlYw==');

@$core.Deprecated('Use workflowGetRequestDescriptor instead')
const WorkflowGetRequest$json = {
  '1': 'WorkflowGetRequest',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
  ],
};

/// Descriptor for `WorkflowGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowGetRequestDescriptor = $convert
    .base64Decode('ChJXb3JrZmxvd0dldFJlcXVlc3QSEgoEbmFtZRgBIAEoCVIEbmFtZQ==');

@$core.Deprecated('Use workflowGetResponseDescriptor instead')
const WorkflowGetResponse$json = {
  '1': 'WorkflowGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkflowDocument',
      '10': 'value'
    },
  ],
};

/// Descriptor for `WorkflowGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowGetResponseDescriptor = $convert.base64Decode(
    'ChNXb3JrZmxvd0dldFJlc3BvbnNlEjYKBXZhbHVlGAEgASgLMiAuZ2l6Y2xhdy5ycGMudjEuV2'
    '9ya2Zsb3dEb2N1bWVudFIFdmFsdWU=');

@$core.Deprecated('Use workflowListRequestDescriptor instead')
const WorkflowListRequest$json = {
  '1': 'WorkflowListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `WorkflowListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowListRequestDescriptor = $convert.base64Decode(
    'ChNXb3JrZmxvd0xpc3RSZXF1ZXN0EhsKBmN1cnNvchgBIAEoCUgAUgZjdXJzb3KIAQESGQoFbG'
    'ltaXQYAiABKANIAVIFbGltaXSIAQFCCQoHX2N1cnNvckIICgZfbGltaXQ=');

@$core.Deprecated('Use workflowListResponseDescriptor instead')
const WorkflowListResponse$json = {
  '1': 'WorkflowListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkflowDocument',
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

/// Descriptor for `WorkflowListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowListResponseDescriptor = $convert.base64Decode(
    'ChRXb3JrZmxvd0xpc3RSZXNwb25zZRIZCghoYXNfbmV4dBgBIAEoCFIHaGFzTmV4dBI2CgVpdG'
    'VtcxgCIAMoCzIgLmdpemNsYXcucnBjLnYxLldvcmtmbG93RG9jdW1lbnRSBWl0ZW1zEiQKC25l'
    'eHRfY3Vyc29yGAMgASgJSABSCm5leHRDdXJzb3KIAQFCDgoMX25leHRfY3Vyc29y');

@$core.Deprecated('Use workflowMetadataDescriptor instead')
const WorkflowMetadata$json = {
  '1': 'WorkflowMetadata',
  '2': [
    {
      '1': 'description',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'description',
      '17': true
    },
    {'1': 'name', '3': 2, '4': 1, '5': 9, '10': 'name'},
  ],
  '8': [
    {'1': '_description'},
  ],
};

/// Descriptor for `WorkflowMetadata`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowMetadataDescriptor = $convert.base64Decode(
    'ChBXb3JrZmxvd01ldGFkYXRhEiUKC2Rlc2NyaXB0aW9uGAEgASgJSABSC2Rlc2NyaXB0aW9uiA'
    'EBEhIKBG5hbWUYAiABKAlSBG5hbWVCDgoMX2Rlc2NyaXB0aW9u');

@$core.Deprecated('Use toolkitPolicyToolIdsDescriptor instead')
const ToolkitPolicyToolIds$json = {
  '1': 'ToolkitPolicyToolIds',
  '2': [
    {'1': 'value', '3': 1, '4': 3, '5': 9, '10': 'value'},
  ],
};

/// Descriptor for `ToolkitPolicyToolIds`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolkitPolicyToolIdsDescriptor =
    $convert.base64Decode(
        'ChRUb29sa2l0UG9saWN5VG9vbElkcxIUCgV2YWx1ZRgBIAMoCVIFdmFsdWU=');

@$core.Deprecated('Use toolkitPolicyDescriptor instead')
const ToolkitPolicy$json = {
  '1': 'ToolkitPolicy',
  '2': [
    {
      '1': 'tool_ids',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ToolkitPolicyToolIds',
      '9': 0,
      '10': 'toolIds',
      '17': true
    },
  ],
  '8': [
    {'1': '_tool_ids'},
  ],
};

/// Descriptor for `ToolkitPolicy`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolkitPolicyDescriptor = $convert.base64Decode(
    'Cg1Ub29sa2l0UG9saWN5EkQKCHRvb2xfaWRzGAEgASgLMiQuZ2l6Y2xhdy5ycGMudjEuVG9vbG'
    'tpdFBvbGljeVRvb2xJZHNIAFIHdG9vbElkc4gBAUILCglfdG9vbF9pZHM=');

@$core.Deprecated('Use workflowPutRequestDescriptor instead')
const WorkflowPutRequest$json = {
  '1': 'WorkflowPutRequest',
  '2': [
    {
      '1': 'body',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkflowDocument',
      '10': 'body'
    },
    {'1': 'name', '3': 2, '4': 1, '5': 9, '10': 'name'},
  ],
};

/// Descriptor for `WorkflowPutRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowPutRequestDescriptor = $convert.base64Decode(
    'ChJXb3JrZmxvd1B1dFJlcXVlc3QSNAoEYm9keRgBIAEoCzIgLmdpemNsYXcucnBjLnYxLldvcm'
    'tmbG93RG9jdW1lbnRSBGJvZHkSEgoEbmFtZRgCIAEoCVIEbmFtZQ==');

@$core.Deprecated('Use workflowPutResponseDescriptor instead')
const WorkflowPutResponse$json = {
  '1': 'WorkflowPutResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WorkflowDocument',
      '10': 'value'
    },
  ],
};

/// Descriptor for `WorkflowPutResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowPutResponseDescriptor = $convert.base64Decode(
    'ChNXb3JrZmxvd1B1dFJlc3BvbnNlEjYKBXZhbHVlGAEgASgLMiAuZ2l6Y2xhdy5ycGMudjEuV2'
    '9ya2Zsb3dEb2N1bWVudFIFdmFsdWU=');

@$core.Deprecated('Use workflowSpecDescriptor instead')
const WorkflowSpec$json = {
  '1': 'WorkflowSpec',
  '2': [
    {
      '1': 'ast_translate',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ASTTranslateWorkflowSpec',
      '9': 0,
      '10': 'astTranslate',
      '17': true
    },
    {
      '1': 'chatroom',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ChatRoomWorkflowSpec',
      '9': 1,
      '10': 'chatroom',
      '17': true
    },
    {
      '1': 'doubao_realtime',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DoubaoRealtimeWorkflowSpec',
      '9': 2,
      '10': 'doubaoRealtime',
      '17': true
    },
    {
      '1': 'driver',
      '3': 4,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.WorkflowDriver',
      '10': 'driver'
    },
    {
      '1': 'flowcraft',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FlowcraftWorkflowSpec',
      '9': 3,
      '10': 'flowcraft',
      '17': true
    },
    {
      '1': 'toolkit',
      '3': 6,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ToolkitPolicy',
      '9': 4,
      '10': 'toolkit',
      '17': true
    },
  ],
  '8': [
    {'1': '_ast_translate'},
    {'1': '_chatroom'},
    {'1': '_doubao_realtime'},
    {'1': '_flowcraft'},
    {'1': '_toolkit'},
  ],
};

/// Descriptor for `WorkflowSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowSpecDescriptor = $convert.base64Decode(
    'CgxXb3JrZmxvd1NwZWMSUgoNYXN0X3RyYW5zbGF0ZRgBIAEoCzIoLmdpemNsYXcucnBjLnYxLk'
    'FTVFRyYW5zbGF0ZVdvcmtmbG93U3BlY0gAUgxhc3RUcmFuc2xhdGWIAQESRQoIY2hhdHJvb20Y'
    'AiABKAsyJC5naXpjbGF3LnJwYy52MS5DaGF0Um9vbVdvcmtmbG93U3BlY0gBUghjaGF0cm9vbY'
    'gBARJYCg9kb3ViYW9fcmVhbHRpbWUYAyABKAsyKi5naXpjbGF3LnJwYy52MS5Eb3ViYW9SZWFs'
    'dGltZVdvcmtmbG93U3BlY0gCUg5kb3ViYW9SZWFsdGltZYgBARI2CgZkcml2ZXIYBCABKA4yHi'
    '5naXpjbGF3LnJwYy52MS5Xb3JrZmxvd0RyaXZlclIGZHJpdmVyEkgKCWZsb3djcmFmdBgFIAEo'
    'CzIlLmdpemNsYXcucnBjLnYxLkZsb3djcmFmdFdvcmtmbG93U3BlY0gDUglmbG93Y3JhZnSIAQ'
    'ESPAoHdG9vbGtpdBgGIAEoCzIdLmdpemNsYXcucnBjLnYxLlRvb2xraXRQb2xpY3lIBFIHdG9v'
    'bGtpdIgBAUIQCg5fYXN0X3RyYW5zbGF0ZUILCglfY2hhdHJvb21CEgoQX2RvdWJhb19yZWFsdG'
    'ltZUIMCgpfZmxvd2NyYWZ0QgoKCF90b29sa2l0');

@$core.Deprecated('Use toolExecutorDescriptor instead')
const ToolExecutor$json = {
  '1': 'ToolExecutor',
  '2': [
    {
      '1': 'kind',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ToolExecutorKind',
      '10': 'kind'
    },
    {'1': 'name', '3': 2, '4': 1, '5': 9, '9': 0, '10': 'name', '17': true},
    {'1': 'method', '3': 3, '4': 1, '5': 9, '9': 1, '10': 'method', '17': true},
    {
      '1': 'peer_id',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'peerId',
      '17': true
    },
    {
      '1': 'config',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '9': 3,
      '10': 'config',
      '17': true
    },
  ],
  '8': [
    {'1': '_name'},
    {'1': '_method'},
    {'1': '_peer_id'},
    {'1': '_config'},
  ],
};

/// Descriptor for `ToolExecutor`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolExecutorDescriptor = $convert.base64Decode(
    'CgxUb29sRXhlY3V0b3ISNAoEa2luZBgBIAEoDjIgLmdpemNsYXcucnBjLnYxLlRvb2xFeGVjdX'
    'RvcktpbmRSBGtpbmQSFwoEbmFtZRgCIAEoCUgAUgRuYW1liAEBEhsKBm1ldGhvZBgDIAEoCUgB'
    'UgZtZXRob2SIAQESHAoHcGVlcl9pZBgEIAEoCUgCUgZwZWVySWSIAQESNAoGY29uZmlnGAUgAS'
    'gLMhcuZ29vZ2xlLnByb3RvYnVmLlN0cnVjdEgDUgZjb25maWeIAQFCBwoFX25hbWVCCQoHX21l'
    'dGhvZEIKCghfcGVlcl9pZEIJCgdfY29uZmln');

@$core.Deprecated('Use toolTriggerExampleDescriptor instead')
const ToolTriggerExample$json = {
  '1': 'ToolTriggerExample',
  '2': [
    {'1': 'input', '3': 1, '4': 1, '5': 9, '10': 'input'},
    {
      '1': 'args',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '9': 0,
      '10': 'args',
      '17': true
    },
    {'1': 'output', '3': 3, '4': 1, '5': 9, '9': 1, '10': 'output', '17': true},
  ],
  '8': [
    {'1': '_args'},
    {'1': '_output'},
  ],
};

/// Descriptor for `ToolTriggerExample`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolTriggerExampleDescriptor = $convert.base64Decode(
    'ChJUb29sVHJpZ2dlckV4YW1wbGUSFAoFaW5wdXQYASABKAlSBWlucHV0EjAKBGFyZ3MYAiABKA'
    'syFy5nb29nbGUucHJvdG9idWYuU3RydWN0SABSBGFyZ3OIAQESGwoGb3V0cHV0GAMgASgJSAFS'
    'Bm91dHB1dIgBAUIHCgVfYXJnc0IJCgdfb3V0cHV0');

@$core.Deprecated('Use toolTriggerDescriptor instead')
const ToolTrigger$json = {
  '1': 'ToolTrigger',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
    {
      '1': 'description',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'description',
      '17': true
    },
    {'1': 'patterns', '3': 3, '4': 3, '5': 9, '10': 'patterns'},
    {
      '1': 'examples',
      '3': 4,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ToolTriggerExample',
      '10': 'examples'
    },
    {
      '1': 'metadata',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '9': 1,
      '10': 'metadata',
      '17': true
    },
  ],
  '8': [
    {'1': '_description'},
    {'1': '_metadata'},
  ],
};

/// Descriptor for `ToolTrigger`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolTriggerDescriptor = $convert.base64Decode(
    'CgtUb29sVHJpZ2dlchISCgRuYW1lGAEgASgJUgRuYW1lEiUKC2Rlc2NyaXB0aW9uGAIgASgJSA'
    'BSC2Rlc2NyaXB0aW9uiAEBEhoKCHBhdHRlcm5zGAMgAygJUghwYXR0ZXJucxI+CghleGFtcGxl'
    'cxgEIAMoCzIiLmdpemNsYXcucnBjLnYxLlRvb2xUcmlnZ2VyRXhhbXBsZVIIZXhhbXBsZXMSOA'
    'oIbWV0YWRhdGEYBSABKAsyFy5nb29nbGUucHJvdG9idWYuU3RydWN0SAFSCG1ldGFkYXRhiAEB'
    'Qg4KDF9kZXNjcmlwdGlvbkILCglfbWV0YWRhdGE=');

@$core.Deprecated('Use toolDescriptor instead')
const Tool$json = {
  '1': 'Tool',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'name', '3': 2, '4': 1, '5': 9, '9': 0, '10': 'name', '17': true},
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
      '1': 'source',
      '3': 4,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ToolSource',
      '10': 'source'
    },
    {
      '1': 'enabled',
      '3': 5,
      '4': 1,
      '5': 8,
      '9': 2,
      '10': 'enabled',
      '17': true
    },
    {
      '1': 'owner_peer',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'ownerPeer',
      '17': true
    },
    {
      '1': 'version',
      '3': 7,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'version',
      '17': true
    },
    {
      '1': 'input_schema',
      '3': 8,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '10': 'inputSchema'
    },
    {
      '1': 'output_schema',
      '3': 9,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '9': 5,
      '10': 'outputSchema',
      '17': true
    },
    {
      '1': 'triggers',
      '3': 10,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ToolTrigger',
      '10': 'triggers'
    },
    {
      '1': 'executor',
      '3': 11,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ToolExecutor',
      '10': 'executor'
    },
    {
      '1': 'metadata',
      '3': 12,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '9': 6,
      '10': 'metadata',
      '17': true
    },
    {'1': 'created_at', '3': 13, '4': 1, '5': 9, '10': 'createdAt'},
    {'1': 'updated_at', '3': 14, '4': 1, '5': 9, '10': 'updatedAt'},
  ],
  '8': [
    {'1': '_name'},
    {'1': '_description'},
    {'1': '_enabled'},
    {'1': '_owner_peer'},
    {'1': '_version'},
    {'1': '_output_schema'},
    {'1': '_metadata'},
  ],
};

/// Descriptor for `Tool`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolDescriptor = $convert.base64Decode(
    'CgRUb29sEg4KAmlkGAEgASgJUgJpZBIXCgRuYW1lGAIgASgJSABSBG5hbWWIAQESJQoLZGVzY3'
    'JpcHRpb24YAyABKAlIAVILZGVzY3JpcHRpb26IAQESMgoGc291cmNlGAQgASgOMhouZ2l6Y2xh'
    'dy5ycGMudjEuVG9vbFNvdXJjZVIGc291cmNlEh0KB2VuYWJsZWQYBSABKAhIAlIHZW5hYmxlZI'
    'gBARIiCgpvd25lcl9wZWVyGAYgASgJSANSCW93bmVyUGVlcogBARIdCgd2ZXJzaW9uGAcgASgJ'
    'SARSB3ZlcnNpb26IAQESOgoMaW5wdXRfc2NoZW1hGAggASgLMhcuZ29vZ2xlLnByb3RvYnVmLl'
    'N0cnVjdFILaW5wdXRTY2hlbWESQQoNb3V0cHV0X3NjaGVtYRgJIAEoCzIXLmdvb2dsZS5wcm90'
    'b2J1Zi5TdHJ1Y3RIBVIMb3V0cHV0U2NoZW1hiAEBEjcKCHRyaWdnZXJzGAogAygLMhsuZ2l6Y2'
    'xhdy5ycGMudjEuVG9vbFRyaWdnZXJSCHRyaWdnZXJzEjgKCGV4ZWN1dG9yGAsgASgLMhwuZ2l6'
    'Y2xhdy5ycGMudjEuVG9vbEV4ZWN1dG9yUghleGVjdXRvchI4CghtZXRhZGF0YRgMIAEoCzIXLm'
    'dvb2dsZS5wcm90b2J1Zi5TdHJ1Y3RIBlIIbWV0YWRhdGGIAQESHQoKY3JlYXRlZF9hdBgNIAEo'
    'CVIJY3JlYXRlZEF0Eh0KCnVwZGF0ZWRfYXQYDiABKAlSCXVwZGF0ZWRBdEIHCgVfbmFtZUIOCg'
    'xfZGVzY3JpcHRpb25CCgoIX2VuYWJsZWRCDQoLX293bmVyX3BlZXJCCgoIX3ZlcnNpb25CEAoO'
    'X291dHB1dF9zY2hlbWFCCwoJX21ldGFkYXRh');

@$core.Deprecated('Use toolListRequestDescriptor instead')
const ToolListRequest$json = {
  '1': 'ToolListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `ToolListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolListRequestDescriptor = $convert.base64Decode(
    'Cg9Ub29sTGlzdFJlcXVlc3QSGwoGY3Vyc29yGAEgASgJSABSBmN1cnNvcogBARIZCgVsaW1pdB'
    'gCIAEoA0gBUgVsaW1pdIgBAUIJCgdfY3Vyc29yQggKBl9saW1pdA==');

@$core.Deprecated('Use toolListResponseDescriptor instead')
const ToolListResponse$json = {
  '1': 'ToolListResponse',
  '2': [
    {
      '1': 'items',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Tool',
      '10': 'items'
    },
    {'1': 'has_next', '3': 2, '4': 1, '5': 8, '10': 'hasNext'},
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

/// Descriptor for `ToolListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolListResponseDescriptor = $convert.base64Decode(
    'ChBUb29sTGlzdFJlc3BvbnNlEioKBWl0ZW1zGAEgAygLMhQuZ2l6Y2xhdy5ycGMudjEuVG9vbF'
    'IFaXRlbXMSGQoIaGFzX25leHQYAiABKAhSB2hhc05leHQSJAoLbmV4dF9jdXJzb3IYAyABKAlI'
    'AFIKbmV4dEN1cnNvcogBAUIOCgxfbmV4dF9jdXJzb3I=');

@$core.Deprecated('Use toolGetRequestDescriptor instead')
const ToolGetRequest$json = {
  '1': 'ToolGetRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `ToolGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolGetRequestDescriptor =
    $convert.base64Decode('Cg5Ub29sR2V0UmVxdWVzdBIOCgJpZBgBIAEoCVICaWQ=');

@$core.Deprecated('Use toolGetResponseDescriptor instead')
const ToolGetResponse$json = {
  '1': 'ToolGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Tool',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ToolGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolGetResponseDescriptor = $convert.base64Decode(
    'Cg9Ub29sR2V0UmVzcG9uc2USKgoFdmFsdWUYASABKAsyFC5naXpjbGF3LnJwYy52MS5Ub29sUg'
    'V2YWx1ZQ==');

@$core.Deprecated('Use toolCreateRequestDescriptor instead')
const ToolCreateRequest$json = {
  '1': 'ToolCreateRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Tool',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ToolCreateRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolCreateRequestDescriptor = $convert.base64Decode(
    'ChFUb29sQ3JlYXRlUmVxdWVzdBIqCgV2YWx1ZRgBIAEoCzIULmdpemNsYXcucnBjLnYxLlRvb2'
    'xSBXZhbHVl');

@$core.Deprecated('Use toolCreateResponseDescriptor instead')
const ToolCreateResponse$json = {
  '1': 'ToolCreateResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Tool',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ToolCreateResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolCreateResponseDescriptor = $convert.base64Decode(
    'ChJUb29sQ3JlYXRlUmVzcG9uc2USKgoFdmFsdWUYASABKAsyFC5naXpjbGF3LnJwYy52MS5Ub2'
    '9sUgV2YWx1ZQ==');

@$core.Deprecated('Use toolPutRequestDescriptor instead')
const ToolPutRequest$json = {
  '1': 'ToolPutRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'body',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Tool',
      '10': 'body'
    },
  ],
};

/// Descriptor for `ToolPutRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolPutRequestDescriptor = $convert.base64Decode(
    'Cg5Ub29sUHV0UmVxdWVzdBIOCgJpZBgBIAEoCVICaWQSKAoEYm9keRgCIAEoCzIULmdpemNsYX'
    'cucnBjLnYxLlRvb2xSBGJvZHk=');

@$core.Deprecated('Use toolPutResponseDescriptor instead')
const ToolPutResponse$json = {
  '1': 'ToolPutResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Tool',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ToolPutResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolPutResponseDescriptor = $convert.base64Decode(
    'Cg9Ub29sUHV0UmVzcG9uc2USKgoFdmFsdWUYASABKAsyFC5naXpjbGF3LnJwYy52MS5Ub29sUg'
    'V2YWx1ZQ==');

@$core.Deprecated('Use toolDeleteRequestDescriptor instead')
const ToolDeleteRequest$json = {
  '1': 'ToolDeleteRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `ToolDeleteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolDeleteRequestDescriptor =
    $convert.base64Decode('ChFUb29sRGVsZXRlUmVxdWVzdBIOCgJpZBgBIAEoCVICaWQ=');

@$core.Deprecated('Use toolDeleteResponseDescriptor instead')
const ToolDeleteResponse$json = {
  '1': 'ToolDeleteResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Tool',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ToolDeleteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolDeleteResponseDescriptor = $convert.base64Decode(
    'ChJUb29sRGVsZXRlUmVzcG9uc2USKgoFdmFsdWUYASABKAsyFC5naXpjbGF3LnJwYy52MS5Ub2'
    '9sUgV2YWx1ZQ==');

@$core.Deprecated('Use toolInvokeRequestDescriptor instead')
const ToolInvokeRequest$json = {
  '1': 'ToolInvokeRequest',
  '2': [
    {'1': 'call_id', '3': 1, '4': 1, '5': 9, '10': 'callId'},
    {'1': 'tool_id', '3': 2, '4': 1, '5': 9, '10': 'toolId'},
    {'1': 'method', '3': 3, '4': 1, '5': 9, '10': 'method'},
    {
      '1': 'args',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '10': 'args'
    },
  ],
};

/// Descriptor for `ToolInvokeRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolInvokeRequestDescriptor = $convert.base64Decode(
    'ChFUb29sSW52b2tlUmVxdWVzdBIXCgdjYWxsX2lkGAEgASgJUgZjYWxsSWQSFwoHdG9vbF9pZB'
    'gCIAEoCVIGdG9vbElkEhYKBm1ldGhvZBgDIAEoCVIGbWV0aG9kEisKBGFyZ3MYBCABKAsyFy5n'
    'b29nbGUucHJvdG9idWYuU3RydWN0UgRhcmdz');

@$core.Deprecated('Use toolInvokeResponseDescriptor instead')
const ToolInvokeResponse$json = {
  '1': 'ToolInvokeResponse',
  '2': [
    {'1': 'data_json', '3': 1, '4': 1, '5': 9, '10': 'dataJson'},
  ],
};

/// Descriptor for `ToolInvokeResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolInvokeResponseDescriptor =
    $convert.base64Decode(
        'ChJUb29sSW52b2tlUmVzcG9uc2USGwoJZGF0YV9qc29uGAEgASgJUghkYXRhSnNvbg==');
