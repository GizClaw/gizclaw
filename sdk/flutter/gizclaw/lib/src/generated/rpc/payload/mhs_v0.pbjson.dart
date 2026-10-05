// This is a generated file - do not edit.
//
// Generated from payload/mhs_v0.proto.

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

@$core.Deprecated('Use clientHwdDescriptor instead')
const ClientHwd$json = {
  '1': 'ClientHwd',
  '2': [
    {'1': 'CLIENT_HWD_UNSPECIFIED', '2': 0},
    {'1': 'CLIENT_HWD_WIFI', '2': 1, '3': {}},
    {'1': 'CLIENT_HWD_BLE', '2': 2, '3': {}},
    {'1': 'CLIENT_HWD_MODEM', '2': 3, '3': {}},
    {'1': 'CLIENT_HWD_BATTERY', '2': 4, '3': {}},
    {'1': 'CLIENT_HWD_MIC', '2': 5, '3': {}},
    {'1': 'CLIENT_HWD_DISPLAY', '2': 6, '3': {}},
    {'1': 'CLIENT_HWD_LED', '2': 7, '3': {}},
    {'1': 'CLIENT_HWD_SPEAKER', '2': 8, '3': {}},
  ],
};

/// Descriptor for `ClientHwd`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List clientHwdDescriptor = $convert.base64Decode(
    'CglDbGllbnRId2QSGgoWQ0xJRU5UX0hXRF9VTlNQRUNJRklFRBAAEjQKD0NMSUVOVF9IV0RfV0'
    'lGSRABGh/S8xgbCgR3aWZpEhNXaWZpSHdkUmVhZFJlc3BvbnNlEjEKDkNMSUVOVF9IV0RfQkxF'
    'EAIaHdLzGBkKA2JsZRISQmxlSHdkUmVhZFJlc3BvbnNlEjcKEENMSUVOVF9IV0RfTU9ERU0QAx'
    'oh0vMYHQoFbW9kZW0SFE1vZGVtSHdkUmVhZFJlc3BvbnNlEj0KEkNMSUVOVF9IV0RfQkFUVEVS'
    'WRAEGiXS8xghCgdiYXR0ZXJ5EhZCYXR0ZXJ5SHdkUmVhZFJlc3BvbnNlEjEKDkNMSUVOVF9IV0'
    'RfTUlDEAUaHdLzGBkKA21pYxISTWljSHdkUmVhZFJlc3BvbnNlEm4KEkNMSUVOVF9IV0RfRElT'
    'UExBWRAGGlbS8xhSCgdkaXNwbGF5EhZEaXNwbGF5SHdkUmVhZFJlc3BvbnNlGhZEaXNwbGF5SH'
    'dkV3JpdGVSZXF1ZXN0IhdEaXNwbGF5SHdkV3JpdGVSZXNwb25zZRJaCg5DTElFTlRfSFdEX0xF'
    'RBAHGkbS8xhCCgNsZWQSEkxlZEh3ZFJlYWRSZXNwb25zZRoSTGVkSHdkV3JpdGVSZXF1ZXN0Ih'
    'NMZWRId2RXcml0ZVJlc3BvbnNlEm4KEkNMSUVOVF9IV0RfU1BFQUtFUhAIGlbS8xhSCgdzcGVh'
    'a2VyEhZTcGVha2VySHdkUmVhZFJlc3BvbnNlGhZTcGVha2VySHdkV3JpdGVSZXF1ZXN0IhdTcG'
    'Vha2VySHdkV3JpdGVSZXNwb25zZQ==');

@$core.Deprecated('Use clientHwdOptionsDescriptor instead')
const ClientHwdOptions$json = {
  '1': 'ClientHwdOptions',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
    {'1': 'read_response', '3': 2, '4': 1, '5': 9, '10': 'readResponse'},
    {'1': 'write_request', '3': 3, '4': 1, '5': 9, '10': 'writeRequest'},
    {'1': 'write_response', '3': 4, '4': 1, '5': 9, '10': 'writeResponse'},
  ],
};

/// Descriptor for `ClientHwdOptions`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientHwdOptionsDescriptor = $convert.base64Decode(
    'ChBDbGllbnRId2RPcHRpb25zEhIKBG5hbWUYASABKAlSBG5hbWUSIwoNcmVhZF9yZXNwb25zZR'
    'gCIAEoCVIMcmVhZFJlc3BvbnNlEiMKDXdyaXRlX3JlcXVlc3QYAyABKAlSDHdyaXRlUmVxdWVz'
    'dBIlCg53cml0ZV9yZXNwb25zZRgEIAEoCVINd3JpdGVSZXNwb25zZQ==');

@$core.Deprecated('Use clientMhsV0ReadRequestDescriptor instead')
const ClientMhsV0ReadRequest$json = {
  '1': 'ClientMhsV0ReadRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'hwd',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ClientHwd',
      '10': 'hwd'
    },
  ],
};

/// Descriptor for `ClientMhsV0ReadRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientMhsV0ReadRequestDescriptor =
    $convert.base64Decode(
        'ChZDbGllbnRNaHNWMFJlYWRSZXF1ZXN0Eg4KAmlkGAEgASgJUgJpZBIrCgNod2QYAiABKA4yGS'
        '5naXpjbGF3LnJwYy52MS5DbGllbnRId2RSA2h3ZA==');

@$core.Deprecated('Use mhsV0WriteCapabilitiesDescriptor instead')
const MhsV0WriteCapabilities$json = {
  '1': 'MhsV0WriteCapabilities',
  '2': [
    {'1': 'fields', '3': 1, '4': 3, '5': 9, '10': 'fields'},
  ],
};

/// Descriptor for `MhsV0WriteCapabilities`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List mhsV0WriteCapabilitiesDescriptor =
    $convert.base64Decode(
        'ChZNaHNWMFdyaXRlQ2FwYWJpbGl0aWVzEhYKBmZpZWxkcxgBIAMoCVIGZmllbGRz');

@$core.Deprecated('Use clientMhsV0ReadResponseDescriptor instead')
const ClientMhsV0ReadResponse$json = {
  '1': 'ClientMhsV0ReadResponse',
  '2': [
    {'1': 'payload', '3': 1, '4': 1, '5': 12, '10': 'payload'},
    {
      '1': 'write_capabilities',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.MhsV0WriteCapabilities',
      '9': 0,
      '10': 'writeCapabilities',
      '17': true
    },
  ],
  '8': [
    {'1': '_write_capabilities'},
  ],
};

/// Descriptor for `ClientMhsV0ReadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientMhsV0ReadResponseDescriptor = $convert.base64Decode(
    'ChdDbGllbnRNaHNWMFJlYWRSZXNwb25zZRIYCgdwYXlsb2FkGAEgASgMUgdwYXlsb2FkEloKEn'
    'dyaXRlX2NhcGFiaWxpdGllcxgCIAEoCzImLmdpemNsYXcucnBjLnYxLk1oc1YwV3JpdGVDYXBh'
    'YmlsaXRpZXNIAFIRd3JpdGVDYXBhYmlsaXRpZXOIAQFCFQoTX3dyaXRlX2NhcGFiaWxpdGllcw'
    '==');

@$core.Deprecated('Use clientMhsV0WriteRequestDescriptor instead')
const ClientMhsV0WriteRequest$json = {
  '1': 'ClientMhsV0WriteRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'hwd',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ClientHwd',
      '10': 'hwd'
    },
    {'1': 'payload', '3': 3, '4': 1, '5': 12, '10': 'payload'},
  ],
};

/// Descriptor for `ClientMhsV0WriteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientMhsV0WriteRequestDescriptor = $convert.base64Decode(
    'ChdDbGllbnRNaHNWMFdyaXRlUmVxdWVzdBIOCgJpZBgBIAEoCVICaWQSKwoDaHdkGAIgASgOMh'
    'kuZ2l6Y2xhdy5ycGMudjEuQ2xpZW50SHdkUgNod2QSGAoHcGF5bG9hZBgDIAEoDFIHcGF5bG9h'
    'ZA==');

@$core.Deprecated('Use clientMhsV0WriteResponseDescriptor instead')
const ClientMhsV0WriteResponse$json = {
  '1': 'ClientMhsV0WriteResponse',
  '2': [
    {'1': 'payload', '3': 1, '4': 1, '5': 12, '10': 'payload'},
  ],
};

/// Descriptor for `ClientMhsV0WriteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientMhsV0WriteResponseDescriptor =
    $convert.base64Decode(
        'ChhDbGllbnRNaHNWMFdyaXRlUmVzcG9uc2USGAoHcGF5bG9hZBgBIAEoDFIHcGF5bG9hZA==');

@$core.Deprecated('Use wifiHwdReadResponseDescriptor instead')
const WifiHwdReadResponse$json = {
  '1': 'WifiHwdReadResponse',
  '2': [
    {
      '1': 'connected',
      '3': 1,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'connected',
      '17': true
    },
    {'1': 'ssid', '3': 2, '4': 1, '5': 9, '9': 1, '10': 'ssid', '17': true},
    {'1': 'bssid', '3': 3, '4': 1, '5': 9, '9': 2, '10': 'bssid', '17': true},
    {
      '1': 'rssi_dbm',
      '3': 4,
      '4': 1,
      '5': 5,
      '9': 3,
      '10': 'rssiDbm',
      '17': true
    },
    {'1': 'ip', '3': 5, '4': 1, '5': 9, '9': 4, '10': 'ip', '17': true},
  ],
  '8': [
    {'1': '_connected'},
    {'1': '_ssid'},
    {'1': '_bssid'},
    {'1': '_rssi_dbm'},
    {'1': '_ip'},
  ],
};

/// Descriptor for `WifiHwdReadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List wifiHwdReadResponseDescriptor = $convert.base64Decode(
    'ChNXaWZpSHdkUmVhZFJlc3BvbnNlEiEKCWNvbm5lY3RlZBgBIAEoCEgAUgljb25uZWN0ZWSIAQ'
    'ESFwoEc3NpZBgCIAEoCUgBUgRzc2lkiAEBEhkKBWJzc2lkGAMgASgJSAJSBWJzc2lkiAEBEh4K'
    'CHJzc2lfZGJtGAQgASgFSANSB3Jzc2lEYm2IAQESEwoCaXAYBSABKAlIBFICaXCIAQFCDAoKX2'
    'Nvbm5lY3RlZEIHCgVfc3NpZEIICgZfYnNzaWRCCwoJX3Jzc2lfZGJtQgUKA19pcA==');

@$core.Deprecated('Use bleHwdReadResponseDescriptor instead')
const BleHwdReadResponse$json = {
  '1': 'BleHwdReadResponse',
  '2': [
    {
      '1': 'powered',
      '3': 1,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'powered',
      '17': true
    },
    {
      '1': 'advertising',
      '3': 2,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'advertising',
      '17': true
    },
    {
      '1': 'scanning',
      '3': 3,
      '4': 1,
      '5': 8,
      '9': 2,
      '10': 'scanning',
      '17': true
    },
    {
      '1': 'connection_count',
      '3': 4,
      '4': 1,
      '5': 13,
      '9': 3,
      '10': 'connectionCount',
      '17': true
    },
  ],
  '8': [
    {'1': '_powered'},
    {'1': '_advertising'},
    {'1': '_scanning'},
    {'1': '_connection_count'},
  ],
};

/// Descriptor for `BleHwdReadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List bleHwdReadResponseDescriptor = $convert.base64Decode(
    'ChJCbGVId2RSZWFkUmVzcG9uc2USHQoHcG93ZXJlZBgBIAEoCEgAUgdwb3dlcmVkiAEBEiUKC2'
    'FkdmVydGlzaW5nGAIgASgISAFSC2FkdmVydGlzaW5niAEBEh8KCHNjYW5uaW5nGAMgASgISAJS'
    'CHNjYW5uaW5niAEBEi4KEGNvbm5lY3Rpb25fY291bnQYBCABKA1IA1IPY29ubmVjdGlvbkNvdW'
    '50iAEBQgoKCF9wb3dlcmVkQg4KDF9hZHZlcnRpc2luZ0ILCglfc2Nhbm5pbmdCEwoRX2Nvbm5l'
    'Y3Rpb25fY291bnQ=');

@$core.Deprecated('Use modemHwdReadResponseDescriptor instead')
const ModemHwdReadResponse$json = {
  '1': 'ModemHwdReadResponse',
  '2': [
    {
      '1': 'sim_present',
      '3': 1,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'simPresent',
      '17': true
    },
    {
      '1': 'registered',
      '3': 2,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'registered',
      '17': true
    },
    {'1': 'rat', '3': 3, '4': 1, '5': 9, '9': 2, '10': 'rat', '17': true},
    {
      '1': 'rssi_dbm',
      '3': 4,
      '4': 1,
      '5': 5,
      '9': 3,
      '10': 'rssiDbm',
      '17': true
    },
    {
      '1': 'signal_level',
      '3': 5,
      '4': 1,
      '5': 13,
      '9': 4,
      '10': 'signalLevel',
      '17': true
    },
  ],
  '8': [
    {'1': '_sim_present'},
    {'1': '_registered'},
    {'1': '_rat'},
    {'1': '_rssi_dbm'},
    {'1': '_signal_level'},
  ],
};

/// Descriptor for `ModemHwdReadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List modemHwdReadResponseDescriptor = $convert.base64Decode(
    'ChRNb2RlbUh3ZFJlYWRSZXNwb25zZRIkCgtzaW1fcHJlc2VudBgBIAEoCEgAUgpzaW1QcmVzZW'
    '50iAEBEiMKCnJlZ2lzdGVyZWQYAiABKAhIAVIKcmVnaXN0ZXJlZIgBARIVCgNyYXQYAyABKAlI'
    'AlIDcmF0iAEBEh4KCHJzc2lfZGJtGAQgASgFSANSB3Jzc2lEYm2IAQESJgoMc2lnbmFsX2xldm'
    'VsGAUgASgNSARSC3NpZ25hbExldmVsiAEBQg4KDF9zaW1fcHJlc2VudEINCgtfcmVnaXN0ZXJl'
    'ZEIGCgRfcmF0QgsKCV9yc3NpX2RibUIPCg1fc2lnbmFsX2xldmVs');

@$core.Deprecated('Use batteryHwdReadResponseDescriptor instead')
const BatteryHwdReadResponse$json = {
  '1': 'BatteryHwdReadResponse',
  '2': [
    {
      '1': 'percent',
      '3': 1,
      '4': 1,
      '5': 1,
      '9': 0,
      '10': 'percent',
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
      '1': 'voltage_mv',
      '3': 3,
      '4': 1,
      '5': 1,
      '9': 2,
      '10': 'voltageMv',
      '17': true
    },
  ],
  '8': [
    {'1': '_percent'},
    {'1': '_charging'},
    {'1': '_voltage_mv'},
  ],
};

/// Descriptor for `BatteryHwdReadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List batteryHwdReadResponseDescriptor = $convert.base64Decode(
    'ChZCYXR0ZXJ5SHdkUmVhZFJlc3BvbnNlEh0KB3BlcmNlbnQYASABKAFIAFIHcGVyY2VudIgBAR'
    'IfCghjaGFyZ2luZxgCIAEoCEgBUghjaGFyZ2luZ4gBARIiCgp2b2x0YWdlX212GAMgASgBSAJS'
    'CXZvbHRhZ2VNdogBAUIKCghfcGVyY2VudEILCglfY2hhcmdpbmdCDQoLX3ZvbHRhZ2VfbXY=');

@$core.Deprecated('Use micHwdReadResponseDescriptor instead')
const MicHwdReadResponse$json = {
  '1': 'MicHwdReadResponse',
  '2': [
    {
      '1': 'available',
      '3': 1,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'available',
      '17': true
    },
    {
      '1': 'capturing',
      '3': 2,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'capturing',
      '17': true
    },
  ],
  '8': [
    {'1': '_available'},
    {'1': '_capturing'},
  ],
};

/// Descriptor for `MicHwdReadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List micHwdReadResponseDescriptor = $convert.base64Decode(
    'ChJNaWNId2RSZWFkUmVzcG9uc2USIQoJYXZhaWxhYmxlGAEgASgISABSCWF2YWlsYWJsZYgBAR'
    'IhCgljYXB0dXJpbmcYAiABKAhIAVIJY2FwdHVyaW5niAEBQgwKCl9hdmFpbGFibGVCDAoKX2Nh'
    'cHR1cmluZw==');

@$core.Deprecated('Use displayHwdReadResponseDescriptor instead')
const DisplayHwdReadResponse$json = {
  '1': 'DisplayHwdReadResponse',
  '2': [
    {
      '1': 'brightness_percent',
      '3': 1,
      '4': 1,
      '5': 13,
      '9': 0,
      '10': 'brightnessPercent',
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
    {
      '1': 'off_timeout_ms',
      '3': 3,
      '4': 1,
      '5': 13,
      '9': 2,
      '10': 'offTimeoutMs',
      '17': true
    },
  ],
  '8': [
    {'1': '_brightness_percent'},
    {'1': '_enabled'},
    {'1': '_off_timeout_ms'},
  ],
};

/// Descriptor for `DisplayHwdReadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List displayHwdReadResponseDescriptor = $convert.base64Decode(
    'ChZEaXNwbGF5SHdkUmVhZFJlc3BvbnNlEjIKEmJyaWdodG5lc3NfcGVyY2VudBgBIAEoDUgAUh'
    'FicmlnaHRuZXNzUGVyY2VudIgBARIdCgdlbmFibGVkGAIgASgISAFSB2VuYWJsZWSIAQESKQoO'
    'b2ZmX3RpbWVvdXRfbXMYAyABKA1IAlIMb2ZmVGltZW91dE1ziAEBQhUKE19icmlnaHRuZXNzX3'
    'BlcmNlbnRCCgoIX2VuYWJsZWRCEQoPX29mZl90aW1lb3V0X21z');

@$core.Deprecated('Use displayHwdWriteRequestDescriptor instead')
const DisplayHwdWriteRequest$json = {
  '1': 'DisplayHwdWriteRequest',
  '2': [
    {
      '1': 'brightness_percent',
      '3': 1,
      '4': 1,
      '5': 13,
      '9': 0,
      '10': 'brightnessPercent',
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
    {
      '1': 'off_timeout_ms',
      '3': 3,
      '4': 1,
      '5': 13,
      '9': 2,
      '10': 'offTimeoutMs',
      '17': true
    },
  ],
  '8': [
    {'1': '_brightness_percent'},
    {'1': '_enabled'},
    {'1': '_off_timeout_ms'},
  ],
};

/// Descriptor for `DisplayHwdWriteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List displayHwdWriteRequestDescriptor = $convert.base64Decode(
    'ChZEaXNwbGF5SHdkV3JpdGVSZXF1ZXN0EjIKEmJyaWdodG5lc3NfcGVyY2VudBgBIAEoDUgAUh'
    'FicmlnaHRuZXNzUGVyY2VudIgBARIdCgdlbmFibGVkGAIgASgISAFSB2VuYWJsZWSIAQESKQoO'
    'b2ZmX3RpbWVvdXRfbXMYAyABKA1IAlIMb2ZmVGltZW91dE1ziAEBQhUKE19icmlnaHRuZXNzX3'
    'BlcmNlbnRCCgoIX2VuYWJsZWRCEQoPX29mZl90aW1lb3V0X21z');

@$core.Deprecated('Use displayHwdWriteResponseDescriptor instead')
const DisplayHwdWriteResponse$json = {
  '1': 'DisplayHwdWriteResponse',
  '2': [
    {
      '1': 'applied',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DisplayHwdReadResponse',
      '10': 'applied'
    },
  ],
};

/// Descriptor for `DisplayHwdWriteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List displayHwdWriteResponseDescriptor =
    $convert.base64Decode(
        'ChdEaXNwbGF5SHdkV3JpdGVSZXNwb25zZRJACgdhcHBsaWVkGAEgASgLMiYuZ2l6Y2xhdy5ycG'
        'MudjEuRGlzcGxheUh3ZFJlYWRSZXNwb25zZVIHYXBwbGllZA==');

@$core.Deprecated('Use ledHwdReadResponseDescriptor instead')
const LedHwdReadResponse$json = {
  '1': 'LedHwdReadResponse',
  '2': [
    {
      '1': 'enabled',
      '3': 1,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'enabled',
      '17': true
    },
    {
      '1': 'brightness_percent',
      '3': 2,
      '4': 1,
      '5': 13,
      '9': 1,
      '10': 'brightnessPercent',
      '17': true
    },
  ],
  '8': [
    {'1': '_enabled'},
    {'1': '_brightness_percent'},
  ],
};

/// Descriptor for `LedHwdReadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List ledHwdReadResponseDescriptor = $convert.base64Decode(
    'ChJMZWRId2RSZWFkUmVzcG9uc2USHQoHZW5hYmxlZBgBIAEoCEgAUgdlbmFibGVkiAEBEjIKEm'
    'JyaWdodG5lc3NfcGVyY2VudBgCIAEoDUgBUhFicmlnaHRuZXNzUGVyY2VudIgBAUIKCghfZW5h'
    'YmxlZEIVChNfYnJpZ2h0bmVzc19wZXJjZW50');

@$core.Deprecated('Use ledHwdWriteRequestDescriptor instead')
const LedHwdWriteRequest$json = {
  '1': 'LedHwdWriteRequest',
  '2': [
    {
      '1': 'enabled',
      '3': 1,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'enabled',
      '17': true
    },
    {
      '1': 'brightness_percent',
      '3': 2,
      '4': 1,
      '5': 13,
      '9': 1,
      '10': 'brightnessPercent',
      '17': true
    },
  ],
  '8': [
    {'1': '_enabled'},
    {'1': '_brightness_percent'},
  ],
};

/// Descriptor for `LedHwdWriteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List ledHwdWriteRequestDescriptor = $convert.base64Decode(
    'ChJMZWRId2RXcml0ZVJlcXVlc3QSHQoHZW5hYmxlZBgBIAEoCEgAUgdlbmFibGVkiAEBEjIKEm'
    'JyaWdodG5lc3NfcGVyY2VudBgCIAEoDUgBUhFicmlnaHRuZXNzUGVyY2VudIgBAUIKCghfZW5h'
    'YmxlZEIVChNfYnJpZ2h0bmVzc19wZXJjZW50');

@$core.Deprecated('Use ledHwdWriteResponseDescriptor instead')
const LedHwdWriteResponse$json = {
  '1': 'LedHwdWriteResponse',
  '2': [
    {
      '1': 'applied',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.LedHwdReadResponse',
      '10': 'applied'
    },
  ],
};

/// Descriptor for `LedHwdWriteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List ledHwdWriteResponseDescriptor = $convert.base64Decode(
    'ChNMZWRId2RXcml0ZVJlc3BvbnNlEjwKB2FwcGxpZWQYASABKAsyIi5naXpjbGF3LnJwYy52MS'
    '5MZWRId2RSZWFkUmVzcG9uc2VSB2FwcGxpZWQ=');

@$core.Deprecated('Use speakerHwdReadResponseDescriptor instead')
const SpeakerHwdReadResponse$json = {
  '1': 'SpeakerHwdReadResponse',
  '2': [
    {
      '1': 'volume_percent',
      '3': 1,
      '4': 1,
      '5': 13,
      '9': 0,
      '10': 'volumePercent',
      '17': true
    },
    {'1': 'muted', '3': 2, '4': 1, '5': 8, '9': 1, '10': 'muted', '17': true},
  ],
  '8': [
    {'1': '_volume_percent'},
    {'1': '_muted'},
  ],
};

/// Descriptor for `SpeakerHwdReadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List speakerHwdReadResponseDescriptor = $convert.base64Decode(
    'ChZTcGVha2VySHdkUmVhZFJlc3BvbnNlEioKDnZvbHVtZV9wZXJjZW50GAEgASgNSABSDXZvbH'
    'VtZVBlcmNlbnSIAQESGQoFbXV0ZWQYAiABKAhIAVIFbXV0ZWSIAQFCEQoPX3ZvbHVtZV9wZXJj'
    'ZW50QggKBl9tdXRlZA==');

@$core.Deprecated('Use speakerHwdWriteRequestDescriptor instead')
const SpeakerHwdWriteRequest$json = {
  '1': 'SpeakerHwdWriteRequest',
  '2': [
    {
      '1': 'volume_percent',
      '3': 1,
      '4': 1,
      '5': 13,
      '9': 0,
      '10': 'volumePercent',
      '17': true
    },
    {'1': 'muted', '3': 2, '4': 1, '5': 8, '9': 1, '10': 'muted', '17': true},
  ],
  '8': [
    {'1': '_volume_percent'},
    {'1': '_muted'},
  ],
};

/// Descriptor for `SpeakerHwdWriteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List speakerHwdWriteRequestDescriptor = $convert.base64Decode(
    'ChZTcGVha2VySHdkV3JpdGVSZXF1ZXN0EioKDnZvbHVtZV9wZXJjZW50GAEgASgNSABSDXZvbH'
    'VtZVBlcmNlbnSIAQESGQoFbXV0ZWQYAiABKAhIAVIFbXV0ZWSIAQFCEQoPX3ZvbHVtZV9wZXJj'
    'ZW50QggKBl9tdXRlZA==');

@$core.Deprecated('Use speakerHwdWriteResponseDescriptor instead')
const SpeakerHwdWriteResponse$json = {
  '1': 'SpeakerHwdWriteResponse',
  '2': [
    {
      '1': 'applied',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.SpeakerHwdReadResponse',
      '10': 'applied'
    },
  ],
};

/// Descriptor for `SpeakerHwdWriteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List speakerHwdWriteResponseDescriptor =
    $convert.base64Decode(
        'ChdTcGVha2VySHdkV3JpdGVSZXNwb25zZRJACgdhcHBsaWVkGAEgASgLMiYuZ2l6Y2xhdy5ycG'
        'MudjEuU3BlYWtlckh3ZFJlYWRSZXNwb25zZVIHYXBwbGllZA==');
