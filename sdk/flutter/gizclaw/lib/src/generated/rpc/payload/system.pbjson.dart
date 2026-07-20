// This is a generated file - do not edit.
//
// Generated from payload/system.proto.

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

@$core.Deprecated('Use clientGetIdentifiersRequestDescriptor instead')
const ClientGetIdentifiersRequest$json = {
  '1': 'ClientGetIdentifiersRequest',
};

/// Descriptor for `ClientGetIdentifiersRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientGetIdentifiersRequestDescriptor =
    $convert.base64Decode('ChtDbGllbnRHZXRJZGVudGlmaWVyc1JlcXVlc3Q=');

@$core.Deprecated('Use clientGetIdentifiersResponseDescriptor instead')
const ClientGetIdentifiersResponse$json = {
  '1': 'ClientGetIdentifiersResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DeviceIdentifiers',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ClientGetIdentifiersResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientGetIdentifiersResponseDescriptor =
    $convert.base64Decode(
        'ChxDbGllbnRHZXRJZGVudGlmaWVyc1Jlc3BvbnNlEjcKBXZhbHVlGAEgASgLMiEuZ2l6Y2xhdy'
        '5ycGMudjEuRGV2aWNlSWRlbnRpZmllcnNSBXZhbHVl');

@$core.Deprecated('Use clientGetInfoRequestDescriptor instead')
const ClientGetInfoRequest$json = {
  '1': 'ClientGetInfoRequest',
};

/// Descriptor for `ClientGetInfoRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientGetInfoRequestDescriptor =
    $convert.base64Decode('ChRDbGllbnRHZXRJbmZvUmVxdWVzdA==');

@$core.Deprecated('Use clientGetInfoResponseDescriptor instead')
const ClientGetInfoResponse$json = {
  '1': 'ClientGetInfoResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.HardwareInfo',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ClientGetInfoResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientGetInfoResponseDescriptor = $convert.base64Decode(
    'ChVDbGllbnRHZXRJbmZvUmVzcG9uc2USMgoFdmFsdWUYASABKAsyHC5naXpjbGF3LnJwYy52MS'
    '5IYXJkd2FyZUluZm9SBXZhbHVl');

@$core.Deprecated('Use deviceInfoDescriptor instead')
const DeviceInfo$json = {
  '1': 'DeviceInfo',
  '2': [
    {
      '1': 'hardware',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.HardwareInfo',
      '9': 0,
      '10': 'hardware',
      '17': true
    },
    {'1': 'name', '3': 2, '4': 1, '5': 9, '9': 1, '10': 'name', '17': true},
    {'1': 'emoji', '3': 4, '4': 1, '5': 9, '9': 2, '10': 'emoji', '17': true},
    {
      '1': 'identifiers',
      '3': 5,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DeviceIdentifiers',
      '9': 3,
      '10': 'identifiers',
      '17': true
    },
  ],
  '8': [
    {'1': '_hardware'},
    {'1': '_name'},
    {'1': '_emoji'},
    {'1': '_identifiers'},
  ],
};

/// Descriptor for `DeviceInfo`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List deviceInfoDescriptor = $convert.base64Decode(
    'CgpEZXZpY2VJbmZvEj0KCGhhcmR3YXJlGAEgASgLMhwuZ2l6Y2xhdy5ycGMudjEuSGFyZHdhcm'
    'VJbmZvSABSCGhhcmR3YXJliAEBEhcKBG5hbWUYAiABKAlIAVIEbmFtZYgBARIZCgVlbW9qaRgE'
    'IAEoCUgCUgVlbW9qaYgBARJICgtpZGVudGlmaWVycxgFIAEoCzIhLmdpemNsYXcucnBjLnYxLk'
    'RldmljZUlkZW50aWZpZXJzSANSC2lkZW50aWZpZXJziAEBQgsKCV9oYXJkd2FyZUIHCgVfbmFt'
    'ZUIICgZfZW1vamlCDgoMX2lkZW50aWZpZXJz');

@$core.Deprecated('Use deviceProfileDescriptor instead')
const DeviceProfile$json = {
  '1': 'DeviceProfile',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'name', '17': true},
    {'1': 'emoji', '3': 2, '4': 1, '5': 9, '9': 1, '10': 'emoji', '17': true},
  ],
  '8': [
    {'1': '_name'},
    {'1': '_emoji'},
  ],
};

/// Descriptor for `DeviceProfile`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List deviceProfileDescriptor = $convert.base64Decode(
    'Cg1EZXZpY2VQcm9maWxlEhcKBG5hbWUYASABKAlIAFIEbmFtZYgBARIZCgVlbW9qaRgCIAEoCU'
    'gBUgVlbW9qaYgBAUIHCgVfbmFtZUIICgZfZW1vamk=');

@$core.Deprecated('Use deviceIdentifiersDescriptor instead')
const DeviceIdentifiers$json = {
  '1': 'DeviceIdentifiers',
  '2': [
    {'1': 'sn', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'sn', '17': true},
    {
      '1': 'imeis',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerIMEI',
      '10': 'imeis'
    },
    {
      '1': 'labels',
      '3': 3,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerLabel',
      '10': 'labels'
    },
  ],
  '8': [
    {'1': '_sn'},
  ],
};

/// Descriptor for `DeviceIdentifiers`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List deviceIdentifiersDescriptor = $convert.base64Decode(
    'ChFEZXZpY2VJZGVudGlmaWVycxITCgJzbhgBIAEoCUgAUgJzbogBARIuCgVpbWVpcxgCIAMoCz'
    'IYLmdpemNsYXcucnBjLnYxLlBlZXJJTUVJUgVpbWVpcxIxCgZsYWJlbHMYAyADKAsyGS5naXpj'
    'bGF3LnJwYy52MS5QZWVyTGFiZWxSBmxhYmVsc0IFCgNfc24=');

@$core.Deprecated('Use hardwareInfoDescriptor instead')
const HardwareInfo$json = {
  '1': 'HardwareInfo',
  '2': [
    {
      '1': 'hardware_revision',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'hardwareRevision',
      '17': true
    },
    {
      '1': 'manufacturer',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'manufacturer',
      '17': true
    },
    {'1': 'model', '3': 3, '4': 1, '5': 9, '9': 2, '10': 'model', '17': true},
  ],
  '8': [
    {'1': '_hardware_revision'},
    {'1': '_manufacturer'},
    {'1': '_model'},
  ],
};

/// Descriptor for `HardwareInfo`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List hardwareInfoDescriptor = $convert.base64Decode(
    'CgxIYXJkd2FyZUluZm8SMAoRaGFyZHdhcmVfcmV2aXNpb24YASABKAlIAFIQaGFyZHdhcmVSZX'
    'Zpc2lvbogBARInCgxtYW51ZmFjdHVyZXIYAiABKAlIAVIMbWFudWZhY3R1cmVyiAEBEhkKBW1v'
    'ZGVsGAMgASgJSAJSBW1vZGVsiAEBQhQKEl9oYXJkd2FyZV9yZXZpc2lvbkIPCg1fbWFudWZhY3'
    'R1cmVyQggKBl9tb2RlbA==');

@$core.Deprecated('Use peerIMEIDescriptor instead')
const PeerIMEI$json = {
  '1': 'PeerIMEI',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'name', '17': true},
    {'1': 'serial', '3': 2, '4': 1, '5': 9, '10': 'serial'},
    {'1': 'tac', '3': 3, '4': 1, '5': 9, '10': 'tac'},
  ],
  '8': [
    {'1': '_name'},
  ],
};

/// Descriptor for `PeerIMEI`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerIMEIDescriptor = $convert.base64Decode(
    'CghQZWVySU1FSRIXCgRuYW1lGAEgASgJSABSBG5hbWWIAQESFgoGc2VyaWFsGAIgASgJUgZzZX'
    'JpYWwSEAoDdGFjGAMgASgJUgN0YWNCBwoFX25hbWU=');

@$core.Deprecated('Use peerLabelDescriptor instead')
const PeerLabel$json = {
  '1': 'PeerLabel',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 9, '10': 'value'},
  ],
};

/// Descriptor for `PeerLabel`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerLabelDescriptor = $convert.base64Decode(
    'CglQZWVyTGFiZWwSEAoDa2V5GAEgASgJUgNrZXkSFAoFdmFsdWUYAiABKAlSBXZhbHVl');

@$core.Deprecated('Use peerStatusDescriptor instead')
const PeerStatus$json = {
  '1': 'PeerStatus',
  '2': [
    {
      '1': 'battery_percent',
      '3': 1,
      '4': 1,
      '5': 3,
      '9': 0,
      '10': 'batteryPercent',
      '17': true
    },
    {
      '1': 'charging',
      '3': 2,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'charging',
      '17': true
    },
    {
      '1': 'details',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.google.protobuf.Struct',
      '10': 'details'
    },
    {
      '1': 'gnss_accuracy_m',
      '3': 4,
      '4': 1,
      '5': 1,
      '9': 2,
      '10': 'gnssAccuracyM',
      '17': true
    },
    {
      '1': 'gnss_altitude_m',
      '3': 5,
      '4': 1,
      '5': 1,
      '9': 3,
      '10': 'gnssAltitudeM',
      '17': true
    },
    {
      '1': 'gnss_latitude',
      '3': 6,
      '4': 1,
      '5': 1,
      '9': 4,
      '10': 'gnssLatitude',
      '17': true
    },
    {
      '1': 'gnss_longitude',
      '3': 7,
      '4': 1,
      '5': 1,
      '9': 5,
      '10': 'gnssLongitude',
      '17': true
    },
    {
      '1': 'labels',
      '3': 8,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerStatus.LabelsEntry',
      '10': 'labels'
    },
    {'1': 'muted', '3': 9, '4': 1, '5': 8, '9': 6, '10': 'muted', '17': true},
    {
      '1': 'reported_at',
      '3': 10,
      '4': 1,
      '5': 9,
      '9': 7,
      '10': 'reportedAt',
      '17': true
    },
    {
      '1': 'volume',
      '3': 11,
      '4': 1,
      '5': 3,
      '9': 8,
      '10': 'volume',
      '17': true
    },
  ],
  '3': [PeerStatus_LabelsEntry$json],
  '8': [
    {'1': '_battery_percent'},
    {'1': '_charging'},
    {'1': '_gnss_accuracy_m'},
    {'1': '_gnss_altitude_m'},
    {'1': '_gnss_latitude'},
    {'1': '_gnss_longitude'},
    {'1': '_muted'},
    {'1': '_reported_at'},
    {'1': '_volume'},
  ],
};

@$core.Deprecated('Use peerStatusDescriptor instead')
const PeerStatus_LabelsEntry$json = {
  '1': 'LabelsEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 9, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `PeerStatus`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerStatusDescriptor = $convert.base64Decode(
    'CgpQZWVyU3RhdHVzEiwKD2JhdHRlcnlfcGVyY2VudBgBIAEoA0gAUg5iYXR0ZXJ5UGVyY2VudI'
    'gBARIfCghjaGFyZ2luZxgCIAEoCEgBUghjaGFyZ2luZ4gBARIxCgdkZXRhaWxzGAMgASgLMhcu'
    'Z29vZ2xlLnByb3RvYnVmLlN0cnVjdFIHZGV0YWlscxIrCg9nbnNzX2FjY3VyYWN5X20YBCABKA'
    'FIAlINZ25zc0FjY3VyYWN5TYgBARIrCg9nbnNzX2FsdGl0dWRlX20YBSABKAFIA1INZ25zc0Fs'
    'dGl0dWRlTYgBARIoCg1nbnNzX2xhdGl0dWRlGAYgASgBSARSDGduc3NMYXRpdHVkZYgBARIqCg'
    '5nbnNzX2xvbmdpdHVkZRgHIAEoAUgFUg1nbnNzTG9uZ2l0dWRliAEBEj4KBmxhYmVscxgIIAMo'
    'CzImLmdpemNsYXcucnBjLnYxLlBlZXJTdGF0dXMuTGFiZWxzRW50cnlSBmxhYmVscxIZCgVtdX'
    'RlZBgJIAEoCEgGUgVtdXRlZIgBARIkCgtyZXBvcnRlZF9hdBgKIAEoCUgHUgpyZXBvcnRlZEF0'
    'iAEBEhsKBnZvbHVtZRgLIAEoA0gIUgZ2b2x1bWWIAQEaOQoLTGFiZWxzRW50cnkSEAoDa2V5GA'
    'EgASgJUgNrZXkSFAoFdmFsdWUYAiABKAlSBXZhbHVlOgI4AUISChBfYmF0dGVyeV9wZXJjZW50'
    'QgsKCV9jaGFyZ2luZ0ISChBfZ25zc19hY2N1cmFjeV9tQhIKEF9nbnNzX2FsdGl0dWRlX21CEA'
    'oOX2duc3NfbGF0aXR1ZGVCEQoPX2duc3NfbG9uZ2l0dWRlQggKBl9tdXRlZEIOCgxfcmVwb3J0'
    'ZWRfYXRCCQoHX3ZvbHVtZQ==');

@$core.Deprecated('Use pingRequestDescriptor instead')
const PingRequest$json = {
  '1': 'PingRequest',
  '2': [
    {'1': 'client_send_time', '3': 1, '4': 1, '5': 3, '10': 'clientSendTime'},
  ],
};

/// Descriptor for `PingRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pingRequestDescriptor = $convert.base64Decode(
    'CgtQaW5nUmVxdWVzdBIoChBjbGllbnRfc2VuZF90aW1lGAEgASgDUg5jbGllbnRTZW5kVGltZQ'
    '==');

@$core.Deprecated('Use pingResponseDescriptor instead')
const PingResponse$json = {
  '1': 'PingResponse',
  '2': [
    {'1': 'server_time', '3': 1, '4': 1, '5': 3, '10': 'serverTime'},
  ],
};

/// Descriptor for `PingResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List pingResponseDescriptor = $convert.base64Decode(
    'CgxQaW5nUmVzcG9uc2USHwoLc2VydmVyX3RpbWUYASABKANSCnNlcnZlclRpbWU=');

@$core.Deprecated('Use serverRegisterRequestDescriptor instead')
const ServerRegisterRequest$json = {
  '1': 'ServerRegisterRequest',
  '2': [
    {'1': 'token', '3': 1, '4': 1, '5': 9, '10': 'token'},
  ],
};

/// Descriptor for `ServerRegisterRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRegisterRequestDescriptor =
    $convert.base64Decode(
        'ChVTZXJ2ZXJSZWdpc3RlclJlcXVlc3QSFAoFdG9rZW4YASABKAlSBXRva2Vu');

@$core.Deprecated('Use serverRegisterResponseDescriptor instead')
const ServerRegisterResponse$json = {
  '1': 'ServerRegisterResponse',
  '2': [
    {
      '1': 'runtime_profile_name',
      '3': 1,
      '4': 1,
      '5': 9,
      '10': 'runtimeProfileName'
    },
    {
      '1': 'firmware_id',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'firmwareId',
      '17': true
    },
  ],
  '8': [
    {'1': '_firmware_id'},
  ],
};

/// Descriptor for `ServerRegisterResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRegisterResponseDescriptor = $convert.base64Decode(
    'ChZTZXJ2ZXJSZWdpc3RlclJlc3BvbnNlEjAKFHJ1bnRpbWVfcHJvZmlsZV9uYW1lGAEgASgJUh'
    'JydW50aW1lUHJvZmlsZU5hbWUSJAoLZmlybXdhcmVfaWQYAiABKAlIAFIKZmlybXdhcmVJZIgB'
    'AUIOCgxfZmlybXdhcmVfaWQ=');

@$core.Deprecated('Use runtimeDescriptor instead')
const Runtime$json = {
  '1': 'Runtime',
  '2': [
    {
      '1': 'last_addr',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'lastAddr',
      '17': true
    },
    {'1': 'last_seen_at', '3': 2, '4': 1, '5': 9, '10': 'lastSeenAt'},
    {'1': 'online', '3': 3, '4': 1, '5': 8, '10': 'online'},
    {
      '1': 'rx_bytes',
      '3': 4,
      '4': 1,
      '5': 4,
      '9': 1,
      '10': 'rxBytes',
      '17': true
    },
    {
      '1': 'tx_bytes',
      '3': 5,
      '4': 1,
      '5': 4,
      '9': 2,
      '10': 'txBytes',
      '17': true
    },
  ],
  '8': [
    {'1': '_last_addr'},
    {'1': '_rx_bytes'},
    {'1': '_tx_bytes'},
  ],
};

/// Descriptor for `Runtime`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List runtimeDescriptor = $convert.base64Decode(
    'CgdSdW50aW1lEiAKCWxhc3RfYWRkchgBIAEoCUgAUghsYXN0QWRkcogBARIgCgxsYXN0X3NlZW'
    '5fYXQYAiABKAlSCmxhc3RTZWVuQXQSFgoGb25saW5lGAMgASgIUgZvbmxpbmUSHgoIcnhfYnl0'
    'ZXMYBCABKARIAVIHcnhCeXRlc4gBARIeCgh0eF9ieXRlcxgFIAEoBEgCUgd0eEJ5dGVziAEBQg'
    'wKCl9sYXN0X2FkZHJCCwoJX3J4X2J5dGVzQgsKCV90eF9ieXRlcw==');

@$core.Deprecated('Use serverGetInfoRequestDescriptor instead')
const ServerGetInfoRequest$json = {
  '1': 'ServerGetInfoRequest',
};

/// Descriptor for `ServerGetInfoRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetInfoRequestDescriptor =
    $convert.base64Decode('ChRTZXJ2ZXJHZXRJbmZvUmVxdWVzdA==');

@$core.Deprecated('Use serverGetInfoResponseDescriptor instead')
const ServerGetInfoResponse$json = {
  '1': 'ServerGetInfoResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DeviceInfo',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGetInfoResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetInfoResponseDescriptor = $convert.base64Decode(
    'ChVTZXJ2ZXJHZXRJbmZvUmVzcG9uc2USMAoFdmFsdWUYASABKAsyGi5naXpjbGF3LnJwYy52MS'
    '5EZXZpY2VJbmZvUgV2YWx1ZQ==');

@$core.Deprecated('Use serverGetStatusRequestDescriptor instead')
const ServerGetStatusRequest$json = {
  '1': 'ServerGetStatusRequest',
};

/// Descriptor for `ServerGetStatusRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetStatusRequestDescriptor =
    $convert.base64Decode('ChZTZXJ2ZXJHZXRTdGF0dXNSZXF1ZXN0');

@$core.Deprecated('Use serverGetStatusResponseDescriptor instead')
const ServerGetStatusResponse$json = {
  '1': 'ServerGetStatusResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerStatus',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerGetStatusResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverGetStatusResponseDescriptor =
    $convert.base64Decode(
        'ChdTZXJ2ZXJHZXRTdGF0dXNSZXNwb25zZRIwCgV2YWx1ZRgBIAEoCzIaLmdpemNsYXcucnBjLn'
        'YxLlBlZXJTdGF0dXNSBXZhbHVl');

@$core.Deprecated('Use serverPutInfoRequestDescriptor instead')
const ServerPutInfoRequest$json = {
  '1': 'ServerPutInfoRequest',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DeviceProfile',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPutInfoRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPutInfoRequestDescriptor = $convert.base64Decode(
    'ChRTZXJ2ZXJQdXRJbmZvUmVxdWVzdBIzCgV2YWx1ZRgBIAEoCzIdLmdpemNsYXcucnBjLnYxLk'
    'RldmljZVByb2ZpbGVSBXZhbHVl');

@$core.Deprecated('Use serverPutInfoResponseDescriptor instead')
const ServerPutInfoResponse$json = {
  '1': 'ServerPutInfoResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DeviceInfo',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ServerPutInfoResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPutInfoResponseDescriptor = $convert.base64Decode(
    'ChVTZXJ2ZXJQdXRJbmZvUmVzcG9uc2USMAoFdmFsdWUYASABKAsyGi5naXpjbGF3LnJwYy52MS'
    '5EZXZpY2VJbmZvUgV2YWx1ZQ==');

@$core.Deprecated('Use speedTestRequestDescriptor instead')
const SpeedTestRequest$json = {
  '1': 'SpeedTestRequest',
  '2': [
    {
      '1': 'down_content_length',
      '3': 1,
      '4': 1,
      '5': 3,
      '10': 'downContentLength'
    },
    {'1': 'up_content_length', '3': 2, '4': 1, '5': 3, '10': 'upContentLength'},
  ],
};

/// Descriptor for `SpeedTestRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List speedTestRequestDescriptor = $convert.base64Decode(
    'ChBTcGVlZFRlc3RSZXF1ZXN0Ei4KE2Rvd25fY29udGVudF9sZW5ndGgYASABKANSEWRvd25Db2'
    '50ZW50TGVuZ3RoEioKEXVwX2NvbnRlbnRfbGVuZ3RoGAIgASgDUg91cENvbnRlbnRMZW5ndGg=');

@$core.Deprecated('Use speedTestResponseDescriptor instead')
const SpeedTestResponse$json = {
  '1': 'SpeedTestResponse',
  '2': [
    {
      '1': 'down_content_length',
      '3': 1,
      '4': 1,
      '5': 3,
      '10': 'downContentLength'
    },
    {'1': 'up_content_length', '3': 2, '4': 1, '5': 3, '10': 'upContentLength'},
  ],
};

/// Descriptor for `SpeedTestResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List speedTestResponseDescriptor = $convert.base64Decode(
    'ChFTcGVlZFRlc3RSZXNwb25zZRIuChNkb3duX2NvbnRlbnRfbGVuZ3RoGAEgASgDUhFkb3duQ2'
    '9udGVudExlbmd0aBIqChF1cF9jb250ZW50X2xlbmd0aBgCIAEoA1IPdXBDb250ZW50TGVuZ3Ro');
