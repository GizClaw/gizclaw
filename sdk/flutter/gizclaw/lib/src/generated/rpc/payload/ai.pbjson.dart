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

@$core.Deprecated('Use aliasI18nTextDescriptor instead')
const AliasI18nText$json = {
  '1': 'AliasI18nText',
  '2': [
    {'1': 'display_name', '3': 1, '4': 1, '5': 9, '10': 'displayName'},
    {
      '1': 'description',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'description',
      '17': true
    },
  ],
  '8': [
    {'1': '_description'},
  ],
};

/// Descriptor for `AliasI18nText`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aliasI18nTextDescriptor = $convert.base64Decode(
    'Cg1BbGlhc0kxOG5UZXh0EiEKDGRpc3BsYXlfbmFtZRgBIAEoCVILZGlzcGxheU5hbWUSJQoLZG'
    'VzY3JpcHRpb24YAiABKAlIAFILZGVzY3JpcHRpb26IAQFCDgoMX2Rlc2NyaXB0aW9u');

@$core.Deprecated('Use speechTranscribeRequestDescriptor instead')
const SpeechTranscribeRequest$json = {
  '1': 'SpeechTranscribeRequest',
  '2': [
    {'1': 'model_alias', '3': 1, '4': 1, '5': 9, '10': 'modelAlias'},
    {'1': 'content_type', '3': 2, '4': 1, '5': 9, '10': 'contentType'},
    {
      '1': 'language',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'language',
      '17': true
    },
  ],
  '8': [
    {'1': '_language'},
  ],
};

/// Descriptor for `SpeechTranscribeRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List speechTranscribeRequestDescriptor = $convert.base64Decode(
    'ChdTcGVlY2hUcmFuc2NyaWJlUmVxdWVzdBIfCgttb2RlbF9hbGlhcxgBIAEoCVIKbW9kZWxBbG'
    'lhcxIhCgxjb250ZW50X3R5cGUYAiABKAlSC2NvbnRlbnRUeXBlEh8KCGxhbmd1YWdlGAMgASgJ'
    'SABSCGxhbmd1YWdliAEBQgsKCV9sYW5ndWFnZQ==');

@$core.Deprecated('Use speechTranscribeResponseDescriptor instead')
const SpeechTranscribeResponse$json = {
  '1': 'SpeechTranscribeResponse',
  '2': [
    {'1': 'transcript', '3': 1, '4': 1, '5': 9, '10': 'transcript'},
  ],
};

/// Descriptor for `SpeechTranscribeResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List speechTranscribeResponseDescriptor =
    $convert.base64Decode(
        'ChhTcGVlY2hUcmFuc2NyaWJlUmVzcG9uc2USHgoKdHJhbnNjcmlwdBgBIAEoCVIKdHJhbnNjcm'
        'lwdA==');

@$core.Deprecated('Use speechSynthesizeRequestDescriptor instead')
const SpeechSynthesizeRequest$json = {
  '1': 'SpeechSynthesizeRequest',
  '2': [
    {'1': 'voice_alias', '3': 1, '4': 1, '5': 9, '10': 'voiceAlias'},
    {'1': 'text', '3': 2, '4': 1, '5': 9, '10': 'text'},
    {
      '1': 'accepted_content_types',
      '3': 3,
      '4': 3,
      '5': 9,
      '10': 'acceptedContentTypes'
    },
  ],
};

/// Descriptor for `SpeechSynthesizeRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List speechSynthesizeRequestDescriptor = $convert.base64Decode(
    'ChdTcGVlY2hTeW50aGVzaXplUmVxdWVzdBIfCgt2b2ljZV9hbGlhcxgBIAEoCVIKdm9pY2VBbG'
    'lhcxISCgR0ZXh0GAIgASgJUgR0ZXh0EjQKFmFjY2VwdGVkX2NvbnRlbnRfdHlwZXMYAyADKAlS'
    'FGFjY2VwdGVkQ29udGVudFR5cGVz');

@$core.Deprecated('Use speechSynthesizeResponseDescriptor instead')
const SpeechSynthesizeResponse$json = {
  '1': 'SpeechSynthesizeResponse',
  '2': [
    {'1': 'content_type', '3': 1, '4': 1, '5': 9, '10': 'contentType'},
    {
      '1': 'sample_rate_hz',
      '3': 2,
      '4': 1,
      '5': 5,
      '9': 0,
      '10': 'sampleRateHz',
      '17': true
    },
    {
      '1': 'channels',
      '3': 3,
      '4': 1,
      '5': 5,
      '9': 1,
      '10': 'channels',
      '17': true
    },
  ],
  '8': [
    {'1': '_sample_rate_hz'},
    {'1': '_channels'},
  ],
};

/// Descriptor for `SpeechSynthesizeResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List speechSynthesizeResponseDescriptor = $convert.base64Decode(
    'ChhTcGVlY2hTeW50aGVzaXplUmVzcG9uc2USIQoMY29udGVudF90eXBlGAEgASgJUgtjb250ZW'
    '50VHlwZRIpCg5zYW1wbGVfcmF0ZV9oehgCIAEoBUgAUgxzYW1wbGVSYXRlSHqIAQESHwoIY2hh'
    'bm5lbHMYAyABKAVIAVIIY2hhbm5lbHOIAQFCEQoPX3NhbXBsZV9yYXRlX2h6QgsKCV9jaGFubm'
    'Vscw==');

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

@$core.Deprecated('Use petConversationParametersDescriptor instead')
const PetConversationParameters$json = {
  '1': 'PetConversationParameters',
  '2': [
    {
      '1': 'initiative',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.PetConversationParametersInitiative',
      '9': 0,
      '10': 'initiative',
      '17': true
    },
  ],
  '8': [
    {'1': '_initiative'},
  ],
};

/// Descriptor for `PetConversationParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petConversationParametersDescriptor = $convert.base64Decode(
    'ChlQZXRDb252ZXJzYXRpb25QYXJhbWV0ZXJzElgKCmluaXRpYXRpdmUYASABKA4yMy5naXpjbG'
    'F3LnJwYy52MS5QZXRDb252ZXJzYXRpb25QYXJhbWV0ZXJzSW5pdGlhdGl2ZUgAUgppbml0aWF0'
    'aXZliAEBQg0KC19pbml0aWF0aXZl');

@$core.Deprecated('Use petPersonaParametersDescriptor instead')
const PetPersonaParameters$json = {
  '1': 'PetPersonaParameters',
  '2': [
    {'1': 'prompt', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'prompt', '17': true},
  ],
  '8': [
    {'1': '_prompt'},
  ],
};

/// Descriptor for `PetPersonaParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petPersonaParametersDescriptor = $convert.base64Decode(
    'ChRQZXRQZXJzb25hUGFyYW1ldGVycxIbCgZwcm9tcHQYASABKAlIAFIGcHJvbXB0iAEBQgkKB1'
    '9wcm9tcHQ=');

@$core.Deprecated('Use petVoiceParametersDescriptor instead')
const PetVoiceParameters$json = {
  '1': 'PetVoiceParameters',
  '2': [
    {'1': 'prompt', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'prompt', '17': true},
    {'1': 'voice_id', '3': 2, '4': 1, '5': 9, '10': 'voiceId'},
  ],
  '8': [
    {'1': '_prompt'},
  ],
};

/// Descriptor for `PetVoiceParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petVoiceParametersDescriptor = $convert.base64Decode(
    'ChJQZXRWb2ljZVBhcmFtZXRlcnMSGwoGcHJvbXB0GAEgASgJSABSBnByb21wdIgBARIZCgh2b2'
    'ljZV9pZBgCIAEoCVIHdm9pY2VJZEIJCgdfcHJvbXB0');

@$core.Deprecated('Use petWorkflowSpecDescriptor instead')
const PetWorkflowSpec$json = {
  '1': 'PetWorkflowSpec',
};

/// Descriptor for `PetWorkflowSpec`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petWorkflowSpecDescriptor =
    $convert.base64Decode('Cg9QZXRXb3JrZmxvd1NwZWM=');

@$core.Deprecated('Use petWorkspaceParametersDescriptor instead')
const PetWorkspaceParameters$json = {
  '1': 'PetWorkspaceParameters',
  '2': [
    {
      '1': 'agent_type',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.PetWorkspaceParametersAgentType',
      '10': 'agentType'
    },
    {
      '1': 'conversation',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetConversationParameters',
      '9': 0,
      '10': 'conversation',
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
      '1': 'persona',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetPersonaParameters',
      '9': 2,
      '10': 'persona',
      '17': true
    },
    {
      '1': 'voice',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PetVoiceParameters',
      '10': 'voice'
    },
  ],
  '8': [
    {'1': '_conversation'},
    {'1': '_input'},
    {'1': '_persona'},
  ],
};

/// Descriptor for `PetWorkspaceParameters`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List petWorkspaceParametersDescriptor = $convert.base64Decode(
    'ChZQZXRXb3Jrc3BhY2VQYXJhbWV0ZXJzEk4KCmFnZW50X3R5cGUYASABKA4yLy5naXpjbGF3Ln'
    'JwYy52MS5QZXRXb3Jrc3BhY2VQYXJhbWV0ZXJzQWdlbnRUeXBlUglhZ2VudFR5cGUSUgoMY29u'
    'dmVyc2F0aW9uGAIgASgLMikuZ2l6Y2xhdy5ycGMudjEuUGV0Q29udmVyc2F0aW9uUGFyYW1ldG'
    'Vyc0gAUgxjb252ZXJzYXRpb26IAQESPQoFaW5wdXQYAyABKA4yIi5naXpjbGF3LnJwYy52MS5X'
    'b3Jrc3BhY2VJbnB1dE1vZGVIAVIFaW5wdXSIAQESQwoHcGVyc29uYRgEIAEoCzIkLmdpemNsYX'
    'cucnBjLnYxLlBldFBlcnNvbmFQYXJhbWV0ZXJzSAJSB3BlcnNvbmGIAQESOAoFdm9pY2UYBSAB'
    'KAsyIi5naXpjbGF3LnJwYy52MS5QZXRWb2ljZVBhcmFtZXRlcnNSBXZvaWNlQg8KDV9jb252ZX'
    'JzYXRpb25CCAoGX2lucHV0QgoKCF9wZXJzb25h');

@$core.Deprecated('Use modelDescriptor instead')
const Model$json = {
  '1': 'Model',
  '2': [
    {'1': 'alias', '3': 1, '4': 1, '5': 9, '10': 'alias'},
    {
      '1': 'i18n',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Model.I18nEntry',
      '10': 'i18n'
    },
    {
      '1': 'kind',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ModelKind',
      '10': 'kind'
    },
    {
      '1': 'capabilities',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ModelCapabilities',
      '9': 0,
      '10': 'capabilities',
      '17': true
    },
  ],
  '3': [Model_I18nEntry$json],
  '8': [
    {'1': '_capabilities'},
  ],
};

@$core.Deprecated('Use modelDescriptor instead')
const Model_I18nEntry$json = {
  '1': 'I18nEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {
      '1': 'value',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AliasI18nText',
      '10': 'value'
    },
  ],
  '7': {'7': true},
};

/// Descriptor for `Model`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelDescriptor = $convert.base64Decode(
    'CgVNb2RlbBIUCgVhbGlhcxgBIAEoCVIFYWxpYXMSMwoEaTE4bhgCIAMoCzIfLmdpemNsYXcucn'
    'BjLnYxLk1vZGVsLkkxOG5FbnRyeVIEaTE4bhItCgRraW5kGAMgASgOMhkuZ2l6Y2xhdy5ycGMu'
    'djEuTW9kZWxLaW5kUgRraW5kEkoKDGNhcGFiaWxpdGllcxgEIAEoCzIhLmdpemNsYXcucnBjLn'
    'YxLk1vZGVsQ2FwYWJpbGl0aWVzSABSDGNhcGFiaWxpdGllc4gBARpWCglJMThuRW50cnkSEAoD'
    'a2V5GAEgASgJUgNrZXkSMwoFdmFsdWUYAiABKAsyHS5naXpjbGF3LnJwYy52MS5BbGlhc0kxOG'
    '5UZXh0UgV2YWx1ZToCOAFCDwoNX2NhcGFiaWxpdGllcw==');

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

@$core.Deprecated('Use modelGetRequestDescriptor instead')
const ModelGetRequest$json = {
  '1': 'ModelGetRequest',
  '2': [
    {'1': 'alias', '3': 1, '4': 1, '5': 9, '10': 'alias'},
  ],
};

/// Descriptor for `ModelGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelGetRequestDescriptor = $convert
    .base64Decode('Cg9Nb2RlbEdldFJlcXVlc3QSFAoFYWxpYXMYASABKAlSBWFsaWFz');

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

/// Descriptor for `ModelGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelGetResponseDescriptor = $convert.base64Decode(
    'ChBNb2RlbEdldFJlc3BvbnNlEisKBXZhbHVlGAEgASgLMhUuZ2l6Y2xhdy5ycGMudjEuTW9kZW'
    'xSBXZhbHVlEjAKFHJ1bnRpbWVfcHJvZmlsZV9uYW1lGAIgASgJUhJydW50aW1lUHJvZmlsZU5h'
    'bWUSOAoYcnVudGltZV9wcm9maWxlX3JldmlzaW9uGAMgASgJUhZydW50aW1lUHJvZmlsZVJldm'
    'lzaW9u');

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

/// Descriptor for `ModelListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modelListResponseDescriptor = $convert.base64Decode(
    'ChFNb2RlbExpc3RSZXNwb25zZRIZCghoYXNfbmV4dBgBIAEoCFIHaGFzTmV4dBIrCgVpdGVtcx'
    'gCIAMoCzIVLmdpemNsYXcucnBjLnYxLk1vZGVsUgVpdGVtcxIkCgtuZXh0X2N1cnNvchgDIAEo'
    'CUgAUgpuZXh0Q3Vyc29yiAEBEjAKFHJ1bnRpbWVfcHJvZmlsZV9uYW1lGAQgASgJUhJydW50aW'
    '1lUHJvZmlsZU5hbWUSOAoYcnVudGltZV9wcm9maWxlX3JldmlzaW9uGAUgASgJUhZydW50aW1l'
    'UHJvZmlsZVJldmlzaW9uQg4KDF9uZXh0X2N1cnNvcg==');

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

@$core.Deprecated('Use voiceDescriptor instead')
const Voice$json = {
  '1': 'Voice',
  '2': [
    {'1': 'alias', '3': 1, '4': 1, '5': 9, '10': 'alias'},
    {
      '1': 'i18n',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Voice.I18nEntry',
      '10': 'i18n'
    },
  ],
  '3': [Voice_I18nEntry$json],
};

@$core.Deprecated('Use voiceDescriptor instead')
const Voice_I18nEntry$json = {
  '1': 'I18nEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {
      '1': 'value',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AliasI18nText',
      '10': 'value'
    },
  ],
  '7': {'7': true},
};

/// Descriptor for `Voice`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List voiceDescriptor = $convert.base64Decode(
    'CgVWb2ljZRIUCgVhbGlhcxgBIAEoCVIFYWxpYXMSMwoEaTE4bhgCIAMoCzIfLmdpemNsYXcucn'
    'BjLnYxLlZvaWNlLkkxOG5FbnRyeVIEaTE4bhpWCglJMThuRW50cnkSEAoDa2V5GAEgASgJUgNr'
    'ZXkSMwoFdmFsdWUYAiABKAsyHS5naXpjbGF3LnJwYy52MS5BbGlhc0kxOG5UZXh0UgV2YWx1ZT'
    'oCOAE=');

@$core.Deprecated('Use voiceGetRequestDescriptor instead')
const VoiceGetRequest$json = {
  '1': 'VoiceGetRequest',
  '2': [
    {'1': 'alias', '3': 1, '4': 1, '5': 9, '10': 'alias'},
  ],
};

/// Descriptor for `VoiceGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List voiceGetRequestDescriptor = $convert
    .base64Decode('Cg9Wb2ljZUdldFJlcXVlc3QSFAoFYWxpYXMYASABKAlSBWFsaWFz');

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

/// Descriptor for `VoiceGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List voiceGetResponseDescriptor = $convert.base64Decode(
    'ChBWb2ljZUdldFJlc3BvbnNlEisKBXZhbHVlGAEgASgLMhUuZ2l6Y2xhdy5ycGMudjEuVm9pY2'
    'VSBXZhbHVlEjAKFHJ1bnRpbWVfcHJvZmlsZV9uYW1lGAIgASgJUhJydW50aW1lUHJvZmlsZU5h'
    'bWUSOAoYcnVudGltZV9wcm9maWxlX3JldmlzaW9uGAMgASgJUhZydW50aW1lUHJvZmlsZVJldm'
    'lzaW9u');

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

/// Descriptor for `VoiceListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List voiceListResponseDescriptor = $convert.base64Decode(
    'ChFWb2ljZUxpc3RSZXNwb25zZRIZCghoYXNfbmV4dBgBIAEoCFIHaGFzTmV4dBIrCgVpdGVtcx'
    'gCIAMoCzIVLmdpemNsYXcucnBjLnYxLlZvaWNlUgVpdGVtcxIkCgtuZXh0X2N1cnNvchgDIAEo'
    'CUgAUgpuZXh0Q3Vyc29yiAEBEjAKFHJ1bnRpbWVfcHJvZmlsZV9uYW1lGAQgASgJUhJydW50aW'
    '1lUHJvZmlsZU5hbWUSOAoYcnVudGltZV9wcm9maWxlX3JldmlzaW9uGAUgASgJUhZydW50aW1l'
    'UHJvZmlsZVJldmlzaW9uQg4KDF9uZXh0X2N1cnNvcg==');

@$core.Deprecated('Use workflowDescriptor instead')
const Workflow$json = {
  '1': 'Workflow',
  '2': [
    {'1': 'alias', '3': 1, '4': 1, '5': 9, '10': 'alias'},
    {
      '1': 'i18n',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Workflow.I18nEntry',
      '10': 'i18n'
    },
    {'1': 'collection', '3': 3, '4': 1, '5': 9, '10': 'collection'},
    {
      '1': 'driver',
      '3': 4,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.WorkflowDriver',
      '10': 'driver'
    },
  ],
  '3': [Workflow_I18nEntry$json],
};

@$core.Deprecated('Use workflowDescriptor instead')
const Workflow_I18nEntry$json = {
  '1': 'I18nEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {
      '1': 'value',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AliasI18nText',
      '10': 'value'
    },
  ],
  '7': {'7': true},
};

/// Descriptor for `Workflow`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowDescriptor = $convert.base64Decode(
    'CghXb3JrZmxvdxIUCgVhbGlhcxgBIAEoCVIFYWxpYXMSNgoEaTE4bhgCIAMoCzIiLmdpemNsYX'
    'cucnBjLnYxLldvcmtmbG93LkkxOG5FbnRyeVIEaTE4bhIeCgpjb2xsZWN0aW9uGAMgASgJUgpj'
    'b2xsZWN0aW9uEjYKBmRyaXZlchgEIAEoDjIeLmdpemNsYXcucnBjLnYxLldvcmtmbG93RHJpdm'
    'VyUgZkcml2ZXIaVgoJSTE4bkVudHJ5EhAKA2tleRgBIAEoCVIDa2V5EjMKBXZhbHVlGAIgASgL'
    'Mh0uZ2l6Y2xhdy5ycGMudjEuQWxpYXNJMThuVGV4dFIFdmFsdWU6AjgB');

@$core.Deprecated('Use workflowGetRequestDescriptor instead')
const WorkflowGetRequest$json = {
  '1': 'WorkflowGetRequest',
  '2': [
    {'1': 'alias', '3': 1, '4': 1, '5': 9, '10': 'alias'},
  ],
  '9': [
    {'1': 2, '2': 3},
    {'1': 3, '2': 4},
  ],
};

/// Descriptor for `WorkflowGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowGetRequestDescriptor = $convert.base64Decode(
    'ChJXb3JrZmxvd0dldFJlcXVlc3QSFAoFYWxpYXMYASABKAlSBWFsaWFzSgQIAhADSgQIAxAE');

@$core.Deprecated('Use workflowGetResponseDescriptor instead')
const WorkflowGetResponse$json = {
  '1': 'WorkflowGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Workflow',
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

/// Descriptor for `WorkflowGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowGetResponseDescriptor = $convert.base64Decode(
    'ChNXb3JrZmxvd0dldFJlc3BvbnNlEi4KBXZhbHVlGAEgASgLMhguZ2l6Y2xhdy5ycGMudjEuV2'
    '9ya2Zsb3dSBXZhbHVlEjAKFHJ1bnRpbWVfcHJvZmlsZV9uYW1lGAIgASgJUhJydW50aW1lUHJv'
    'ZmlsZU5hbWUSOAoYcnVudGltZV9wcm9maWxlX3JldmlzaW9uGAMgASgJUhZydW50aW1lUHJvZm'
    'lsZVJldmlzaW9u');

@$core.Deprecated('Use workflowListRequestDescriptor instead')
const WorkflowListRequest$json = {
  '1': 'WorkflowListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
    {'1': 'collection', '3': 3, '4': 1, '5': 9, '10': 'collection'},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
  ],
  '9': [
    {'1': 4, '2': 5},
  ],
};

/// Descriptor for `WorkflowListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowListRequestDescriptor = $convert.base64Decode(
    'ChNXb3JrZmxvd0xpc3RSZXF1ZXN0EhsKBmN1cnNvchgBIAEoCUgAUgZjdXJzb3KIAQESGQoFbG'
    'ltaXQYAiABKANIAVIFbGltaXSIAQESHgoKY29sbGVjdGlvbhgDIAEoCVIKY29sbGVjdGlvbkIJ'
    'CgdfY3Vyc29yQggKBl9saW1pdEoECAQQBQ==');

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
      '6': '.gizclaw.rpc.v1.Workflow',
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

/// Descriptor for `WorkflowListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List workflowListResponseDescriptor = $convert.base64Decode(
    'ChRXb3JrZmxvd0xpc3RSZXNwb25zZRIZCghoYXNfbmV4dBgBIAEoCFIHaGFzTmV4dBIuCgVpdG'
    'VtcxgCIAMoCzIYLmdpemNsYXcucnBjLnYxLldvcmtmbG93UgVpdGVtcxIkCgtuZXh0X2N1cnNv'
    'chgDIAEoCUgAUgpuZXh0Q3Vyc29yiAEBEjAKFHJ1bnRpbWVfcHJvZmlsZV9uYW1lGAQgASgJUh'
    'JydW50aW1lUHJvZmlsZU5hbWUSOAoYcnVudGltZV9wcm9maWxlX3JldmlzaW9uGAUgASgJUhZy'
    'dW50aW1lUHJvZmlsZVJldmlzaW9uQg4KDF9uZXh0X2N1cnNvcg==');

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

@$core.Deprecated('Use toolDescriptor instead')
const Tool$json = {
  '1': 'Tool',
  '2': [
    {'1': 'alias', '3': 1, '4': 1, '5': 9, '10': 'alias'},
    {
      '1': 'i18n',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Tool.I18nEntry',
      '10': 'i18n'
    },
    {
      '1': 'input_schema',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '10': 'inputSchema'
    },
    {
      '1': 'output_schema',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '9': 0,
      '10': 'outputSchema',
      '17': true
    },
  ],
  '3': [Tool_I18nEntry$json],
  '8': [
    {'1': '_output_schema'},
  ],
};

@$core.Deprecated('Use toolDescriptor instead')
const Tool_I18nEntry$json = {
  '1': 'I18nEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {
      '1': 'value',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AliasI18nText',
      '10': 'value'
    },
  ],
  '7': {'7': true},
};

/// Descriptor for `Tool`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolDescriptor = $convert.base64Decode(
    'CgRUb29sEhQKBWFsaWFzGAEgASgJUgVhbGlhcxIyCgRpMThuGAIgAygLMh4uZ2l6Y2xhdy5ycG'
    'MudjEuVG9vbC5JMThuRW50cnlSBGkxOG4SOgoMaW5wdXRfc2NoZW1hGAMgASgLMhcuZ29vZ2xl'
    'LnByb3RvYnVmLlN0cnVjdFILaW5wdXRTY2hlbWESQQoNb3V0cHV0X3NjaGVtYRgEIAEoCzIXLm'
    'dvb2dsZS5wcm90b2J1Zi5TdHJ1Y3RIAFIMb3V0cHV0U2NoZW1hiAEBGlYKCUkxOG5FbnRyeRIQ'
    'CgNrZXkYASABKAlSA2tleRIzCgV2YWx1ZRgCIAEoCzIdLmdpemNsYXcucnBjLnYxLkFsaWFzST'
    'E4blRleHRSBXZhbHVlOgI4AUIQCg5fb3V0cHV0X3NjaGVtYQ==');

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

/// Descriptor for `ToolListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolListResponseDescriptor = $convert.base64Decode(
    'ChBUb29sTGlzdFJlc3BvbnNlEioKBWl0ZW1zGAEgAygLMhQuZ2l6Y2xhdy5ycGMudjEuVG9vbF'
    'IFaXRlbXMSGQoIaGFzX25leHQYAiABKAhSB2hhc05leHQSJAoLbmV4dF9jdXJzb3IYAyABKAlI'
    'AFIKbmV4dEN1cnNvcogBARIwChRydW50aW1lX3Byb2ZpbGVfbmFtZRgEIAEoCVIScnVudGltZV'
    'Byb2ZpbGVOYW1lEjgKGHJ1bnRpbWVfcHJvZmlsZV9yZXZpc2lvbhgFIAEoCVIWcnVudGltZVBy'
    'b2ZpbGVSZXZpc2lvbkIOCgxfbmV4dF9jdXJzb3I=');

@$core.Deprecated('Use toolGetRequestDescriptor instead')
const ToolGetRequest$json = {
  '1': 'ToolGetRequest',
  '2': [
    {'1': 'alias', '3': 1, '4': 1, '5': 9, '10': 'alias'},
  ],
};

/// Descriptor for `ToolGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolGetRequestDescriptor = $convert
    .base64Decode('Cg5Ub29sR2V0UmVxdWVzdBIUCgVhbGlhcxgBIAEoCVIFYWxpYXM=');

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

/// Descriptor for `ToolGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List toolGetResponseDescriptor = $convert.base64Decode(
    'Cg9Ub29sR2V0UmVzcG9uc2USKgoFdmFsdWUYASABKAsyFC5naXpjbGF3LnJwYy52MS5Ub29sUg'
    'V2YWx1ZRIwChRydW50aW1lX3Byb2ZpbGVfbmFtZRgCIAEoCVIScnVudGltZVByb2ZpbGVOYW1l'
    'EjgKGHJ1bnRpbWVfcHJvZmlsZV9yZXZpc2lvbhgDIAEoCVIWcnVudGltZVByb2ZpbGVSZXZpc2'
    'lvbg==');

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
