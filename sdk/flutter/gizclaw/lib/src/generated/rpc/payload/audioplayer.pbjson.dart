// This is a generated file - do not edit.
//
// Generated from payload/audioplayer.proto.

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

@$core.Deprecated('Use audioPlayerItemDescriptor instead')
const AudioPlayerItem$json = {
  '1': 'AudioPlayerItem',
  '2': [
    {'1': 'url', '3': 1, '4': 1, '5': 9, '10': 'url'},
    {'1': 'title', '3': 2, '4': 1, '5': 9, '9': 0, '10': 'title', '17': true},
    {
      '1': 'source_ref',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'sourceRef',
      '17': true
    },
  ],
  '8': [
    {'1': '_title'},
    {'1': '_source_ref'},
  ],
};

/// Descriptor for `AudioPlayerItem`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List audioPlayerItemDescriptor = $convert.base64Decode(
    'Cg9BdWRpb1BsYXllckl0ZW0SEAoDdXJsGAEgASgJUgN1cmwSGQoFdGl0bGUYAiABKAlIAFIFdG'
    'l0bGWIAQESIgoKc291cmNlX3JlZhgDIAEoCUgBUglzb3VyY2VSZWaIAQFCCAoGX3RpdGxlQg0K'
    'C19zb3VyY2VfcmVm');

@$core.Deprecated('Use audioPlayerStatusDescriptor instead')
const AudioPlayerStatus$json = {
  '1': 'AudioPlayerStatus',
  '2': [
    {'1': 'state', '3': 1, '4': 1, '5': 9, '10': 'state'},
    {
      '1': 'current_index',
      '3': 2,
      '4': 1,
      '5': 13,
      '9': 0,
      '10': 'currentIndex',
      '17': true
    },
    {'1': 'position_ms', '3': 3, '4': 1, '5': 4, '10': 'positionMs'},
    {
      '1': 'duration_ms',
      '3': 4,
      '4': 1,
      '5': 4,
      '9': 1,
      '10': 'durationMs',
      '17': true
    },
    {'1': 'repeat', '3': 5, '4': 1, '5': 9, '10': 'repeat'},
    {'1': 'playlist_length', '3': 6, '4': 1, '5': 13, '10': 'playlistLength'},
    {
      '1': 'playlist_revision',
      '3': 7,
      '4': 1,
      '5': 13,
      '10': 'playlistRevision'
    },
    {
      '1': 'error_code',
      '3': 8,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'errorCode',
      '17': true
    },
    {
      '1': 'error_message',
      '3': 9,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'errorMessage',
      '17': true
    },
    {
      '1': 'observed_at_unix_ms',
      '3': 10,
      '4': 1,
      '5': 3,
      '10': 'observedAtUnixMs'
    },
  ],
  '8': [
    {'1': '_current_index'},
    {'1': '_duration_ms'},
    {'1': '_error_code'},
    {'1': '_error_message'},
  ],
};

/// Descriptor for `AudioPlayerStatus`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List audioPlayerStatusDescriptor = $convert.base64Decode(
    'ChFBdWRpb1BsYXllclN0YXR1cxIUCgVzdGF0ZRgBIAEoCVIFc3RhdGUSKAoNY3VycmVudF9pbm'
    'RleBgCIAEoDUgAUgxjdXJyZW50SW5kZXiIAQESHwoLcG9zaXRpb25fbXMYAyABKARSCnBvc2l0'
    'aW9uTXMSJAoLZHVyYXRpb25fbXMYBCABKARIAVIKZHVyYXRpb25Nc4gBARIWCgZyZXBlYXQYBS'
    'ABKAlSBnJlcGVhdBInCg9wbGF5bGlzdF9sZW5ndGgYBiABKA1SDnBsYXlsaXN0TGVuZ3RoEisK'
    'EXBsYXlsaXN0X3JldmlzaW9uGAcgASgNUhBwbGF5bGlzdFJldmlzaW9uEiIKCmVycm9yX2NvZG'
    'UYCCABKAlIAlIJZXJyb3JDb2RliAEBEigKDWVycm9yX21lc3NhZ2UYCSABKAlIA1IMZXJyb3JN'
    'ZXNzYWdliAEBEi0KE29ic2VydmVkX2F0X3VuaXhfbXMYCiABKANSEG9ic2VydmVkQXRVbml4TX'
    'NCEAoOX2N1cnJlbnRfaW5kZXhCDgoMX2R1cmF0aW9uX21zQg0KC19lcnJvcl9jb2RlQhAKDl9l'
    'cnJvcl9tZXNzYWdl');

@$core.Deprecated('Use clientDeviceAudioPlayerGetRequestDescriptor instead')
const ClientDeviceAudioPlayerGetRequest$json = {
  '1': 'ClientDeviceAudioPlayerGetRequest',
};

/// Descriptor for `ClientDeviceAudioPlayerGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceAudioPlayerGetRequestDescriptor =
    $convert.base64Decode('CiFDbGllbnREZXZpY2VBdWRpb1BsYXllckdldFJlcXVlc3Q=');

@$core.Deprecated('Use clientDeviceAudioPlayerGetResponseDescriptor instead')
const ClientDeviceAudioPlayerGetResponse$json = {
  '1': 'ClientDeviceAudioPlayerGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AudioPlayerStatus',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ClientDeviceAudioPlayerGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceAudioPlayerGetResponseDescriptor =
    $convert.base64Decode(
        'CiJDbGllbnREZXZpY2VBdWRpb1BsYXllckdldFJlc3BvbnNlEjcKBXZhbHVlGAEgASgLMiEuZ2'
        'l6Y2xhdy5ycGMudjEuQXVkaW9QbGF5ZXJTdGF0dXNSBXZhbHVl');

@$core.Deprecated(
    'Use clientDeviceAudioPlayerPlaylistGetRequestDescriptor instead')
const ClientDeviceAudioPlayerPlaylistGetRequest$json = {
  '1': 'ClientDeviceAudioPlayerPlaylistGetRequest',
};

/// Descriptor for `ClientDeviceAudioPlayerPlaylistGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List
    clientDeviceAudioPlayerPlaylistGetRequestDescriptor = $convert.base64Decode(
        'CilDbGllbnREZXZpY2VBdWRpb1BsYXllclBsYXlsaXN0R2V0UmVxdWVzdA==');

@$core.Deprecated(
    'Use clientDeviceAudioPlayerPlaylistGetResponseDescriptor instead')
const ClientDeviceAudioPlayerPlaylistGetResponse$json = {
  '1': 'ClientDeviceAudioPlayerPlaylistGetResponse',
  '2': [
    {
      '1': 'items',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AudioPlayerItem',
      '10': 'items'
    },
    {
      '1': 'playlist_revision',
      '3': 2,
      '4': 1,
      '5': 13,
      '10': 'playlistRevision'
    },
  ],
};

/// Descriptor for `ClientDeviceAudioPlayerPlaylistGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List
    clientDeviceAudioPlayerPlaylistGetResponseDescriptor =
    $convert.base64Decode(
        'CipDbGllbnREZXZpY2VBdWRpb1BsYXllclBsYXlsaXN0R2V0UmVzcG9uc2USNQoFaXRlbXMYAS'
        'ADKAsyHy5naXpjbGF3LnJwYy52MS5BdWRpb1BsYXllckl0ZW1SBWl0ZW1zEisKEXBsYXlsaXN0'
        'X3JldmlzaW9uGAIgASgNUhBwbGF5bGlzdFJldmlzaW9u');

@$core.Deprecated(
    'Use clientDeviceAudioPlayerPlaylistSetRequestDescriptor instead')
const ClientDeviceAudioPlayerPlaylistSetRequest$json = {
  '1': 'ClientDeviceAudioPlayerPlaylistSetRequest',
  '2': [
    {
      '1': 'items',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AudioPlayerItem',
      '10': 'items'
    },
  ],
};

/// Descriptor for `ClientDeviceAudioPlayerPlaylistSetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List
    clientDeviceAudioPlayerPlaylistSetRequestDescriptor = $convert.base64Decode(
        'CilDbGllbnREZXZpY2VBdWRpb1BsYXllclBsYXlsaXN0U2V0UmVxdWVzdBI1CgVpdGVtcxgBIA'
        'MoCzIfLmdpemNsYXcucnBjLnYxLkF1ZGlvUGxheWVySXRlbVIFaXRlbXM=');

@$core.Deprecated(
    'Use clientDeviceAudioPlayerPlaylistSetResponseDescriptor instead')
const ClientDeviceAudioPlayerPlaylistSetResponse$json = {
  '1': 'ClientDeviceAudioPlayerPlaylistSetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AudioPlayerStatus',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ClientDeviceAudioPlayerPlaylistSetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List
    clientDeviceAudioPlayerPlaylistSetResponseDescriptor =
    $convert.base64Decode(
        'CipDbGllbnREZXZpY2VBdWRpb1BsYXllclBsYXlsaXN0U2V0UmVzcG9uc2USNwoFdmFsdWUYAS'
        'ABKAsyIS5naXpjbGF3LnJwYy52MS5BdWRpb1BsYXllclN0YXR1c1IFdmFsdWU=');

@$core.Deprecated(
    'Use clientDeviceAudioPlayerPlaylistAppendRequestDescriptor instead')
const ClientDeviceAudioPlayerPlaylistAppendRequest$json = {
  '1': 'ClientDeviceAudioPlayerPlaylistAppendRequest',
  '2': [
    {
      '1': 'items',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AudioPlayerItem',
      '10': 'items'
    },
  ],
};

/// Descriptor for `ClientDeviceAudioPlayerPlaylistAppendRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List
    clientDeviceAudioPlayerPlaylistAppendRequestDescriptor =
    $convert.base64Decode(
        'CixDbGllbnREZXZpY2VBdWRpb1BsYXllclBsYXlsaXN0QXBwZW5kUmVxdWVzdBI1CgVpdGVtcx'
        'gBIAMoCzIfLmdpemNsYXcucnBjLnYxLkF1ZGlvUGxheWVySXRlbVIFaXRlbXM=');

@$core.Deprecated(
    'Use clientDeviceAudioPlayerPlaylistAppendResponseDescriptor instead')
const ClientDeviceAudioPlayerPlaylistAppendResponse$json = {
  '1': 'ClientDeviceAudioPlayerPlaylistAppendResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AudioPlayerStatus',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ClientDeviceAudioPlayerPlaylistAppendResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List
    clientDeviceAudioPlayerPlaylistAppendResponseDescriptor =
    $convert.base64Decode(
        'Ci1DbGllbnREZXZpY2VBdWRpb1BsYXllclBsYXlsaXN0QXBwZW5kUmVzcG9uc2USNwoFdmFsdW'
        'UYASABKAsyIS5naXpjbGF3LnJwYy52MS5BdWRpb1BsYXllclN0YXR1c1IFdmFsdWU=');

@$core.Deprecated('Use clientDeviceAudioPlayerPlayRequestDescriptor instead')
const ClientDeviceAudioPlayerPlayRequest$json = {
  '1': 'ClientDeviceAudioPlayerPlayRequest',
  '2': [
    {'1': 'index', '3': 1, '4': 1, '5': 13, '9': 0, '10': 'index', '17': true},
  ],
  '8': [
    {'1': '_index'},
  ],
};

/// Descriptor for `ClientDeviceAudioPlayerPlayRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceAudioPlayerPlayRequestDescriptor =
    $convert.base64Decode(
        'CiJDbGllbnREZXZpY2VBdWRpb1BsYXllclBsYXlSZXF1ZXN0EhkKBWluZGV4GAEgASgNSABSBW'
        'luZGV4iAEBQggKBl9pbmRleA==');

@$core.Deprecated('Use clientDeviceAudioPlayerPlayResponseDescriptor instead')
const ClientDeviceAudioPlayerPlayResponse$json = {
  '1': 'ClientDeviceAudioPlayerPlayResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AudioPlayerStatus',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ClientDeviceAudioPlayerPlayResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceAudioPlayerPlayResponseDescriptor =
    $convert.base64Decode(
        'CiNDbGllbnREZXZpY2VBdWRpb1BsYXllclBsYXlSZXNwb25zZRI3CgV2YWx1ZRgBIAEoCzIhLm'
        'dpemNsYXcucnBjLnYxLkF1ZGlvUGxheWVyU3RhdHVzUgV2YWx1ZQ==');

@$core.Deprecated('Use clientDeviceAudioPlayerStopRequestDescriptor instead')
const ClientDeviceAudioPlayerStopRequest$json = {
  '1': 'ClientDeviceAudioPlayerStopRequest',
};

/// Descriptor for `ClientDeviceAudioPlayerStopRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceAudioPlayerStopRequestDescriptor =
    $convert.base64Decode('CiJDbGllbnREZXZpY2VBdWRpb1BsYXllclN0b3BSZXF1ZXN0');

@$core.Deprecated('Use clientDeviceAudioPlayerStopResponseDescriptor instead')
const ClientDeviceAudioPlayerStopResponse$json = {
  '1': 'ClientDeviceAudioPlayerStopResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AudioPlayerStatus',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ClientDeviceAudioPlayerStopResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceAudioPlayerStopResponseDescriptor =
    $convert.base64Decode(
        'CiNDbGllbnREZXZpY2VBdWRpb1BsYXllclN0b3BSZXNwb25zZRI3CgV2YWx1ZRgBIAEoCzIhLm'
        'dpemNsYXcucnBjLnYxLkF1ZGlvUGxheWVyU3RhdHVzUgV2YWx1ZQ==');

@$core.Deprecated('Use clientDeviceAudioPlayerModeSetRequestDescriptor instead')
const ClientDeviceAudioPlayerModeSetRequest$json = {
  '1': 'ClientDeviceAudioPlayerModeSetRequest',
  '2': [
    {'1': 'repeat', '3': 1, '4': 1, '5': 9, '10': 'repeat'},
  ],
};

/// Descriptor for `ClientDeviceAudioPlayerModeSetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceAudioPlayerModeSetRequestDescriptor =
    $convert.base64Decode(
        'CiVDbGllbnREZXZpY2VBdWRpb1BsYXllck1vZGVTZXRSZXF1ZXN0EhYKBnJlcGVhdBgBIAEoCV'
        'IGcmVwZWF0');

@$core
    .Deprecated('Use clientDeviceAudioPlayerModeSetResponseDescriptor instead')
const ClientDeviceAudioPlayerModeSetResponse$json = {
  '1': 'ClientDeviceAudioPlayerModeSetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AudioPlayerStatus',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ClientDeviceAudioPlayerModeSetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceAudioPlayerModeSetResponseDescriptor =
    $convert.base64Decode(
        'CiZDbGllbnREZXZpY2VBdWRpb1BsYXllck1vZGVTZXRSZXNwb25zZRI3CgV2YWx1ZRgBIAEoCz'
        'IhLmdpemNsYXcucnBjLnYxLkF1ZGlvUGxheWVyU3RhdHVzUgV2YWx1ZQ==');
