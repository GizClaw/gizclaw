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

@$core.Deprecated('Use clientDeviceStatusGetRequestDescriptor instead')
const ClientDeviceStatusGetRequest$json = {
  '1': 'ClientDeviceStatusGetRequest',
};

/// Descriptor for `ClientDeviceStatusGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceStatusGetRequestDescriptor =
    $convert.base64Decode('ChxDbGllbnREZXZpY2VTdGF0dXNHZXRSZXF1ZXN0');

@$core.Deprecated('Use clientDeviceStatusGetResponseDescriptor instead')
const ClientDeviceStatusGetResponse$json = {
  '1': 'ClientDeviceStatusGetResponse',
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

/// Descriptor for `ClientDeviceStatusGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceStatusGetResponseDescriptor =
    $convert.base64Decode(
        'Ch1DbGllbnREZXZpY2VTdGF0dXNHZXRSZXNwb25zZRIwCgV2YWx1ZRgBIAEoCzIaLmdpemNsYX'
        'cucnBjLnYxLlBlZXJTdGF0dXNSBXZhbHVl');

@$core.Deprecated('Use clientDeviceVolumeSetRequestDescriptor instead')
const ClientDeviceVolumeSetRequest$json = {
  '1': 'ClientDeviceVolumeSetRequest',
  '2': [
    {'1': 'level', '3': 1, '4': 1, '5': 3, '10': 'level'},
    {'1': 'muted', '3': 2, '4': 1, '5': 8, '10': 'muted'},
  ],
};

/// Descriptor for `ClientDeviceVolumeSetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceVolumeSetRequestDescriptor =
    $convert.base64Decode(
        'ChxDbGllbnREZXZpY2VWb2x1bWVTZXRSZXF1ZXN0EhQKBWxldmVsGAEgASgDUgVsZXZlbBIUCg'
        'VtdXRlZBgCIAEoCFIFbXV0ZWQ=');

@$core.Deprecated('Use clientDeviceVolumeSetResponseDescriptor instead')
const ClientDeviceVolumeSetResponse$json = {
  '1': 'ClientDeviceVolumeSetResponse',
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

/// Descriptor for `ClientDeviceVolumeSetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceVolumeSetResponseDescriptor =
    $convert.base64Decode(
        'Ch1DbGllbnREZXZpY2VWb2x1bWVTZXRSZXNwb25zZRIwCgV2YWx1ZRgBIAEoCzIaLmdpemNsYX'
        'cucnBjLnYxLlBlZXJTdGF0dXNSBXZhbHVl');

@$core.Deprecated('Use clientDeviceSoundPlayRequestDescriptor instead')
const ClientDeviceSoundPlayRequest$json = {
  '1': 'ClientDeviceSoundPlayRequest',
  '2': [
    {'1': 'sound', '3': 1, '4': 1, '5': 9, '10': 'sound'},
    {
      '1': 'duration_ms',
      '3': 2,
      '4': 1,
      '5': 3,
      '9': 0,
      '10': 'durationMs',
      '17': true
    },
  ],
  '8': [
    {'1': '_duration_ms'},
  ],
};

/// Descriptor for `ClientDeviceSoundPlayRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceSoundPlayRequestDescriptor =
    $convert.base64Decode(
        'ChxDbGllbnREZXZpY2VTb3VuZFBsYXlSZXF1ZXN0EhQKBXNvdW5kGAEgASgJUgVzb3VuZBIkCg'
        'tkdXJhdGlvbl9tcxgCIAEoA0gAUgpkdXJhdGlvbk1ziAEBQg4KDF9kdXJhdGlvbl9tcw==');

@$core.Deprecated('Use clientDeviceSoundPlayResponseDescriptor instead')
const ClientDeviceSoundPlayResponse$json = {
  '1': 'ClientDeviceSoundPlayResponse',
};

/// Descriptor for `ClientDeviceSoundPlayResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceSoundPlayResponseDescriptor =
    $convert.base64Decode('Ch1DbGllbnREZXZpY2VTb3VuZFBsYXlSZXNwb25zZQ==');

@$core.Deprecated('Use clientDeviceRebootRequestDescriptor instead')
const ClientDeviceRebootRequest$json = {
  '1': 'ClientDeviceRebootRequest',
  '2': [
    {
      '1': 'delay_ms',
      '3': 1,
      '4': 1,
      '5': 3,
      '9': 0,
      '10': 'delayMs',
      '17': true
    },
  ],
  '8': [
    {'1': '_delay_ms'},
  ],
};

/// Descriptor for `ClientDeviceRebootRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceRebootRequestDescriptor =
    $convert.base64Decode(
        'ChlDbGllbnREZXZpY2VSZWJvb3RSZXF1ZXN0Eh4KCGRlbGF5X21zGAEgASgDSABSB2RlbGF5TX'
        'OIAQFCCwoJX2RlbGF5X21z');

@$core.Deprecated('Use clientDeviceRebootResponseDescriptor instead')
const ClientDeviceRebootResponse$json = {
  '1': 'ClientDeviceRebootResponse',
};

/// Descriptor for `ClientDeviceRebootResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceRebootResponseDescriptor =
    $convert.base64Decode('ChpDbGllbnREZXZpY2VSZWJvb3RSZXNwb25zZQ==');

@$core.Deprecated('Use wifiStatusDescriptor instead')
const WifiStatus$json = {
  '1': 'WifiStatus',
  '2': [
    {'1': 'connected', '3': 1, '4': 1, '5': 8, '10': 'connected'},
    {'1': 'ssid', '3': 2, '4': 1, '5': 9, '9': 0, '10': 'ssid', '17': true},
    {
      '1': 'rssi_dbm',
      '3': 3,
      '4': 1,
      '5': 3,
      '9': 1,
      '10': 'rssiDbm',
      '17': true
    },
    {'1': 'ip', '3': 4, '4': 1, '5': 9, '9': 2, '10': 'ip', '17': true},
    {'1': 'bssid', '3': 5, '4': 1, '5': 9, '9': 3, '10': 'bssid', '17': true},
  ],
  '8': [
    {'1': '_ssid'},
    {'1': '_rssi_dbm'},
    {'1': '_ip'},
    {'1': '_bssid'},
  ],
};

/// Descriptor for `WifiStatus`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List wifiStatusDescriptor = $convert.base64Decode(
    'CgpXaWZpU3RhdHVzEhwKCWNvbm5lY3RlZBgBIAEoCFIJY29ubmVjdGVkEhcKBHNzaWQYAiABKA'
    'lIAFIEc3NpZIgBARIeCghyc3NpX2RibRgDIAEoA0gBUgdyc3NpRGJtiAEBEhMKAmlwGAQgASgJ'
    'SAJSAmlwiAEBEhkKBWJzc2lkGAUgASgJSANSBWJzc2lkiAEBQgcKBV9zc2lkQgsKCV9yc3NpX2'
    'RibUIFCgNfaXBCCAoGX2Jzc2lk');

@$core.Deprecated('Use wifiSavedNetworkDescriptor instead')
const WifiSavedNetwork$json = {
  '1': 'WifiSavedNetwork',
  '2': [
    {'1': 'ssid', '3': 1, '4': 1, '5': 9, '10': 'ssid'},
  ],
};

/// Descriptor for `WifiSavedNetwork`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List wifiSavedNetworkDescriptor = $convert
    .base64Decode('ChBXaWZpU2F2ZWROZXR3b3JrEhIKBHNzaWQYASABKAlSBHNzaWQ=');

@$core.Deprecated('Use clientWifiStatusGetRequestDescriptor instead')
const ClientWifiStatusGetRequest$json = {
  '1': 'ClientWifiStatusGetRequest',
};

/// Descriptor for `ClientWifiStatusGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientWifiStatusGetRequestDescriptor =
    $convert.base64Decode('ChpDbGllbnRXaWZpU3RhdHVzR2V0UmVxdWVzdA==');

@$core.Deprecated('Use clientWifiStatusGetResponseDescriptor instead')
const ClientWifiStatusGetResponse$json = {
  '1': 'ClientWifiStatusGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WifiStatus',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ClientWifiStatusGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientWifiStatusGetResponseDescriptor =
    $convert.base64Decode(
        'ChtDbGllbnRXaWZpU3RhdHVzR2V0UmVzcG9uc2USMAoFdmFsdWUYASABKAsyGi5naXpjbGF3Ln'
        'JwYy52MS5XaWZpU3RhdHVzUgV2YWx1ZQ==');

@$core.Deprecated('Use clientWifiSavedListRequestDescriptor instead')
const ClientWifiSavedListRequest$json = {
  '1': 'ClientWifiSavedListRequest',
};

/// Descriptor for `ClientWifiSavedListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientWifiSavedListRequestDescriptor =
    $convert.base64Decode('ChpDbGllbnRXaWZpU2F2ZWRMaXN0UmVxdWVzdA==');

@$core.Deprecated('Use clientWifiSavedListResponseDescriptor instead')
const ClientWifiSavedListResponse$json = {
  '1': 'ClientWifiSavedListResponse',
  '2': [
    {
      '1': 'networks',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WifiSavedNetwork',
      '10': 'networks'
    },
  ],
};

/// Descriptor for `ClientWifiSavedListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientWifiSavedListResponseDescriptor =
    $convert.base64Decode(
        'ChtDbGllbnRXaWZpU2F2ZWRMaXN0UmVzcG9uc2USPAoIbmV0d29ya3MYASADKAsyIC5naXpjbG'
        'F3LnJwYy52MS5XaWZpU2F2ZWROZXR3b3JrUghuZXR3b3Jrcw==');

@$core.Deprecated('Use clientWifiSavedForgetRequestDescriptor instead')
const ClientWifiSavedForgetRequest$json = {
  '1': 'ClientWifiSavedForgetRequest',
  '2': [
    {'1': 'ssid', '3': 1, '4': 1, '5': 9, '10': 'ssid'},
  ],
};

/// Descriptor for `ClientWifiSavedForgetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientWifiSavedForgetRequestDescriptor =
    $convert.base64Decode(
        'ChxDbGllbnRXaWZpU2F2ZWRGb3JnZXRSZXF1ZXN0EhIKBHNzaWQYASABKAlSBHNzaWQ=');

@$core.Deprecated('Use clientWifiSavedForgetResponseDescriptor instead')
const ClientWifiSavedForgetResponse$json = {
  '1': 'ClientWifiSavedForgetResponse',
};

/// Descriptor for `ClientWifiSavedForgetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientWifiSavedForgetResponseDescriptor =
    $convert.base64Decode('Ch1DbGllbnRXaWZpU2F2ZWRGb3JnZXRSZXNwb25zZQ==');

@$core.Deprecated('Use wifiScanResultDescriptor instead')
const WifiScanResult$json = {
  '1': 'WifiScanResult',
  '2': [
    {'1': 'ssid', '3': 1, '4': 1, '5': 9, '10': 'ssid'},
    {'1': 'bssid', '3': 2, '4': 1, '5': 9, '9': 0, '10': 'bssid', '17': true},
    {
      '1': 'rssi_dbm',
      '3': 3,
      '4': 1,
      '5': 3,
      '9': 1,
      '10': 'rssiDbm',
      '17': true
    },
    {
      '1': 'frequency_mhz',
      '3': 4,
      '4': 1,
      '5': 3,
      '9': 2,
      '10': 'frequencyMhz',
      '17': true
    },
    {
      '1': 'security',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'security',
      '17': true
    },
  ],
  '8': [
    {'1': '_bssid'},
    {'1': '_rssi_dbm'},
    {'1': '_frequency_mhz'},
    {'1': '_security'},
  ],
};

/// Descriptor for `WifiScanResult`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List wifiScanResultDescriptor = $convert.base64Decode(
    'Cg5XaWZpU2NhblJlc3VsdBISCgRzc2lkGAEgASgJUgRzc2lkEhkKBWJzc2lkGAIgASgJSABSBW'
    'Jzc2lkiAEBEh4KCHJzc2lfZGJtGAMgASgDSAFSB3Jzc2lEYm2IAQESKAoNZnJlcXVlbmN5X21o'
    'ehgEIAEoA0gCUgxmcmVxdWVuY3lNaHqIAQESHwoIc2VjdXJpdHkYBSABKAlIA1IIc2VjdXJpdH'
    'mIAQFCCAoGX2Jzc2lkQgsKCV9yc3NpX2RibUIQCg5fZnJlcXVlbmN5X21oekILCglfc2VjdXJp'
    'dHk=');

@$core.Deprecated('Use clientWifiScanRequestDescriptor instead')
const ClientWifiScanRequest$json = {
  '1': 'ClientWifiScanRequest',
  '2': [
    {
      '1': 'timeout_ms',
      '3': 1,
      '4': 1,
      '5': 3,
      '9': 0,
      '10': 'timeoutMs',
      '17': true
    },
  ],
  '8': [
    {'1': '_timeout_ms'},
  ],
};

/// Descriptor for `ClientWifiScanRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientWifiScanRequestDescriptor = $convert.base64Decode(
    'ChVDbGllbnRXaWZpU2NhblJlcXVlc3QSIgoKdGltZW91dF9tcxgBIAEoA0gAUgl0aW1lb3V0TX'
    'OIAQFCDQoLX3RpbWVvdXRfbXM=');

@$core.Deprecated('Use clientWifiScanResponseDescriptor instead')
const ClientWifiScanResponse$json = {
  '1': 'ClientWifiScanResponse',
  '2': [
    {
      '1': 'networks',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.WifiScanResult',
      '10': 'networks'
    },
  ],
};

/// Descriptor for `ClientWifiScanResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientWifiScanResponseDescriptor =
    $convert.base64Decode(
        'ChZDbGllbnRXaWZpU2NhblJlc3BvbnNlEjoKCG5ldHdvcmtzGAEgAygLMh4uZ2l6Y2xhdy5ycG'
        'MudjEuV2lmaVNjYW5SZXN1bHRSCG5ldHdvcmtz');

@$core.Deprecated('Use clientWifiConnectRequestDescriptor instead')
const ClientWifiConnectRequest$json = {
  '1': 'ClientWifiConnectRequest',
  '2': [
    {'1': 'ssid', '3': 1, '4': 1, '5': 9, '10': 'ssid'},
    {
      '1': 'passphrase',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'passphrase',
      '17': true
    },
  ],
  '8': [
    {'1': '_passphrase'},
  ],
};

/// Descriptor for `ClientWifiConnectRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientWifiConnectRequestDescriptor =
    $convert.base64Decode(
        'ChhDbGllbnRXaWZpQ29ubmVjdFJlcXVlc3QSEgoEc3NpZBgBIAEoCVIEc3NpZBIjCgpwYXNzcG'
        'hyYXNlGAIgASgJSABSCnBhc3NwaHJhc2WIAQFCDQoLX3Bhc3NwaHJhc2U=');

@$core.Deprecated('Use clientWifiConnectResponseDescriptor instead')
const ClientWifiConnectResponse$json = {
  '1': 'ClientWifiConnectResponse',
};

/// Descriptor for `ClientWifiConnectResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientWifiConnectResponseDescriptor =
    $convert.base64Decode('ChlDbGllbnRXaWZpQ29ubmVjdFJlc3BvbnNl');

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

@$core.Deprecated('Use peerOtaStatusDescriptor instead')
const PeerOtaStatus$json = {
  '1': 'PeerOtaStatus',
  '2': [
    {'1': 'state', '3': 1, '4': 1, '5': 9, '10': 'state'},
    {'1': 'update_id', '3': 2, '4': 1, '5': 9, '10': 'updateId'},
    {'1': 'observed_at', '3': 3, '4': 1, '5': 9, '10': 'observedAt'},
    {
      '1': 'download_percent',
      '3': 4,
      '4': 1,
      '5': 1,
      '9': 0,
      '10': 'downloadPercent',
      '17': true
    },
    {
      '1': 'target_version',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'targetVersion',
      '17': true
    },
    {
      '1': 'error_code',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'errorCode',
      '17': true
    },
    {
      '1': 'error_message',
      '3': 7,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'errorMessage',
      '17': true
    },
  ],
  '8': [
    {'1': '_download_percent'},
    {'1': '_target_version'},
    {'1': '_error_code'},
    {'1': '_error_message'},
  ],
};

/// Descriptor for `PeerOtaStatus`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerOtaStatusDescriptor = $convert.base64Decode(
    'Cg1QZWVyT3RhU3RhdHVzEhQKBXN0YXRlGAEgASgJUgVzdGF0ZRIbCgl1cGRhdGVfaWQYAiABKA'
    'lSCHVwZGF0ZUlkEh8KC29ic2VydmVkX2F0GAMgASgJUgpvYnNlcnZlZEF0Ei4KEGRvd25sb2Fk'
    'X3BlcmNlbnQYBCABKAFIAFIPZG93bmxvYWRQZXJjZW50iAEBEioKDnRhcmdldF92ZXJzaW9uGA'
    'UgASgJSAFSDXRhcmdldFZlcnNpb26IAQESIgoKZXJyb3JfY29kZRgGIAEoCUgCUgllcnJvckNv'
    'ZGWIAQESKAoNZXJyb3JfbWVzc2FnZRgHIAEoCUgDUgxlcnJvck1lc3NhZ2WIAQFCEwoRX2Rvd2'
    '5sb2FkX3BlcmNlbnRCEQoPX3RhcmdldF92ZXJzaW9uQg0KC19lcnJvcl9jb2RlQhAKDl9lcnJv'
    'cl9tZXNzYWdl');

@$core.Deprecated('Use peerStatusDescriptor instead')
const PeerStatus$json = {
  '1': 'PeerStatus',
  '2': [
    {
      '1': 'ota',
      '3': 13,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerOtaStatus',
      '10': 'ota'
    },
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
      '1': 'firmware_sha256',
      '3': 12,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'firmwareSha256',
      '17': true
    },
    {
      '1': 'gnss_accuracy_m',
      '3': 4,
      '4': 1,
      '5': 1,
      '9': 3,
      '10': 'gnssAccuracyM',
      '17': true
    },
    {
      '1': 'gnss_altitude_m',
      '3': 5,
      '4': 1,
      '5': 1,
      '9': 4,
      '10': 'gnssAltitudeM',
      '17': true
    },
    {
      '1': 'gnss_latitude',
      '3': 6,
      '4': 1,
      '5': 1,
      '9': 5,
      '10': 'gnssLatitude',
      '17': true
    },
    {
      '1': 'gnss_longitude',
      '3': 7,
      '4': 1,
      '5': 1,
      '9': 6,
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
    {'1': 'muted', '3': 9, '4': 1, '5': 8, '9': 7, '10': 'muted', '17': true},
    {
      '1': 'reported_at',
      '3': 10,
      '4': 1,
      '5': 9,
      '9': 8,
      '10': 'reportedAt',
      '17': true
    },
    {
      '1': 'volume',
      '3': 11,
      '4': 1,
      '5': 3,
      '9': 9,
      '10': 'volume',
      '17': true
    },
  ],
  '3': [PeerStatus_LabelsEntry$json],
  '8': [
    {'1': '_battery_percent'},
    {'1': '_charging'},
    {'1': '_firmware_sha256'},
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
    'CgpQZWVyU3RhdHVzEi8KA290YRgNIAEoCzIdLmdpemNsYXcucnBjLnYxLlBlZXJPdGFTdGF0dX'
    'NSA290YRIsCg9iYXR0ZXJ5X3BlcmNlbnQYASABKANIAFIOYmF0dGVyeVBlcmNlbnSIAQESHwoI'
    'Y2hhcmdpbmcYAiABKAhIAVIIY2hhcmdpbmeIAQESMQoHZGV0YWlscxgDIAEoCzIXLmdvb2dsZS'
    '5wcm90b2J1Zi5TdHJ1Y3RSB2RldGFpbHMSLAoPZmlybXdhcmVfc2hhMjU2GAwgASgJSAJSDmZp'
    'cm13YXJlU2hhMjU2iAEBEisKD2duc3NfYWNjdXJhY3lfbRgEIAEoAUgDUg1nbnNzQWNjdXJhY3'
    'lNiAEBEisKD2duc3NfYWx0aXR1ZGVfbRgFIAEoAUgEUg1nbnNzQWx0aXR1ZGVNiAEBEigKDWdu'
    'c3NfbGF0aXR1ZGUYBiABKAFIBVIMZ25zc0xhdGl0dWRliAEBEioKDmduc3NfbG9uZ2l0dWRlGA'
    'cgASgBSAZSDWduc3NMb25naXR1ZGWIAQESPgoGbGFiZWxzGAggAygLMiYuZ2l6Y2xhdy5ycGMu'
    'djEuUGVlclN0YXR1cy5MYWJlbHNFbnRyeVIGbGFiZWxzEhkKBW11dGVkGAkgASgISAdSBW11dG'
    'VkiAEBEiQKC3JlcG9ydGVkX2F0GAogASgJSAhSCnJlcG9ydGVkQXSIAQESGwoGdm9sdW1lGAsg'
    'ASgDSAlSBnZvbHVtZYgBARo5CgtMYWJlbHNFbnRyeRIQCgNrZXkYASABKAlSA2tleRIUCgV2YW'
    'x1ZRgCIAEoCVIFdmFsdWU6AjgBQhIKEF9iYXR0ZXJ5X3BlcmNlbnRCCwoJX2NoYXJnaW5nQhIK'
    'EF9maXJtd2FyZV9zaGEyNTZCEgoQX2duc3NfYWNjdXJhY3lfbUISChBfZ25zc19hbHRpdHVkZV'
    '9tQhAKDl9nbnNzX2xhdGl0dWRlQhEKD19nbnNzX2xvbmdpdHVkZUIICgZfbXV0ZWRCDgoMX3Jl'
    'cG9ydGVkX2F0QgkKB192b2x1bWU=');

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
  ],
};

/// Descriptor for `ServerRegisterResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRegisterResponseDescriptor =
    $convert.base64Decode(
        'ChZTZXJ2ZXJSZWdpc3RlclJlc3BvbnNlEjAKFHJ1bnRpbWVfcHJvZmlsZV9uYW1lGAEgASgJUh'
        'JydW50aW1lUHJvZmlsZU5hbWU=');

@$core.Deprecated('Use aPIKeyDescriptor instead')
const APIKey$json = {
  '1': 'APIKey',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
    {'1': 'display_name', '3': 2, '4': 1, '5': 9, '10': 'displayName'},
    {'1': 'prefix', '3': 3, '4': 1, '5': 9, '10': 'prefix'},
    {'1': 'manage_api_keys', '3': 4, '4': 1, '5': 8, '10': 'manageApiKeys'},
    {'1': 'created_at', '3': 5, '4': 1, '5': 9, '10': 'createdAt'},
    {'1': 'api_key', '3': 6, '4': 1, '5': 9, '10': 'apiKey'},
  ],
};

/// Descriptor for `APIKey`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aPIKeyDescriptor = $convert.base64Decode(
    'CgZBUElLZXkSEgoEbmFtZRgBIAEoCVIEbmFtZRIhCgxkaXNwbGF5X25hbWUYAiABKAlSC2Rpc3'
    'BsYXlOYW1lEhYKBnByZWZpeBgDIAEoCVIGcHJlZml4EiYKD21hbmFnZV9hcGlfa2V5cxgEIAEo'
    'CFINbWFuYWdlQXBpS2V5cxIdCgpjcmVhdGVkX2F0GAUgASgJUgljcmVhdGVkQXQSFwoHYXBpX2'
    'tleRgGIAEoCVIGYXBpS2V5');

@$core.Deprecated('Use aPIKeyCreateRequestDescriptor instead')
const APIKeyCreateRequest$json = {
  '1': 'APIKeyCreateRequest',
  '2': [
    {'1': 'display_name', '3': 1, '4': 1, '5': 9, '10': 'displayName'},
    {'1': 'manage_api_keys', '3': 2, '4': 1, '5': 8, '10': 'manageApiKeys'},
  ],
};

/// Descriptor for `APIKeyCreateRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aPIKeyCreateRequestDescriptor = $convert.base64Decode(
    'ChNBUElLZXlDcmVhdGVSZXF1ZXN0EiEKDGRpc3BsYXlfbmFtZRgBIAEoCVILZGlzcGxheU5hbW'
    'USJgoPbWFuYWdlX2FwaV9rZXlzGAIgASgIUg1tYW5hZ2VBcGlLZXlz');

@$core.Deprecated('Use aPIKeyCreateResponseDescriptor instead')
const APIKeyCreateResponse$json = {
  '1': 'APIKeyCreateResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.APIKey',
      '10': 'value'
    },
    {'1': 'api_key', '3': 2, '4': 1, '5': 9, '10': 'apiKey'},
  ],
};

/// Descriptor for `APIKeyCreateResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aPIKeyCreateResponseDescriptor = $convert.base64Decode(
    'ChRBUElLZXlDcmVhdGVSZXNwb25zZRIsCgV2YWx1ZRgBIAEoCzIWLmdpemNsYXcucnBjLnYxLk'
    'FQSUtleVIFdmFsdWUSFwoHYXBpX2tleRgCIAEoCVIGYXBpS2V5');

@$core.Deprecated('Use aPIKeyListRequestDescriptor instead')
const APIKeyListRequest$json = {
  '1': 'APIKeyListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `APIKeyListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aPIKeyListRequestDescriptor = $convert.base64Decode(
    'ChFBUElLZXlMaXN0UmVxdWVzdBIbCgZjdXJzb3IYASABKAlIAFIGY3Vyc29yiAEBEhkKBWxpbW'
    'l0GAIgASgDSAFSBWxpbWl0iAEBQgkKB19jdXJzb3JCCAoGX2xpbWl0');

@$core.Deprecated('Use aPIKeyListResponseDescriptor instead')
const APIKeyListResponse$json = {
  '1': 'APIKeyListResponse',
  '2': [
    {
      '1': 'items',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.APIKey',
      '10': 'items'
    },
    {
      '1': 'next_cursor',
      '3': 2,
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

/// Descriptor for `APIKeyListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aPIKeyListResponseDescriptor = $convert.base64Decode(
    'ChJBUElLZXlMaXN0UmVzcG9uc2USLAoFaXRlbXMYASADKAsyFi5naXpjbGF3LnJwYy52MS5BUE'
    'lLZXlSBWl0ZW1zEiQKC25leHRfY3Vyc29yGAIgASgJSABSCm5leHRDdXJzb3KIAQFCDgoMX25l'
    'eHRfY3Vyc29y');

@$core.Deprecated('Use aPIKeyRevokeRequestDescriptor instead')
const APIKeyRevokeRequest$json = {
  '1': 'APIKeyRevokeRequest',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
  ],
};

/// Descriptor for `APIKeyRevokeRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aPIKeyRevokeRequestDescriptor = $convert
    .base64Decode('ChNBUElLZXlSZXZva2VSZXF1ZXN0EhIKBG5hbWUYASABKAlSBG5hbWU=');

@$core.Deprecated('Use aPIKeyRevokeResponseDescriptor instead')
const APIKeyRevokeResponse$json = {
  '1': 'APIKeyRevokeResponse',
};

/// Descriptor for `APIKeyRevokeResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aPIKeyRevokeResponseDescriptor =
    $convert.base64Decode('ChRBUElLZXlSZXZva2VSZXNwb25zZQ==');

@$core.Deprecated('Use serverPeerDeleteRequestDescriptor instead')
const ServerPeerDeleteRequest$json = {
  '1': 'ServerPeerDeleteRequest',
};

/// Descriptor for `ServerPeerDeleteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPeerDeleteRequestDescriptor =
    $convert.base64Decode('ChdTZXJ2ZXJQZWVyRGVsZXRlUmVxdWVzdA==');

@$core.Deprecated('Use serverPeerDeleteResponseDescriptor instead')
const ServerPeerDeleteResponse$json = {
  '1': 'ServerPeerDeleteResponse',
};

/// Descriptor for `ServerPeerDeleteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPeerDeleteResponseDescriptor =
    $convert.base64Decode('ChhTZXJ2ZXJQZWVyRGVsZXRlUmVzcG9uc2U=');

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
