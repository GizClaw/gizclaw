// This is a generated file - do not edit.
//
// Generated from payload/app.proto.

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

@$core.Deprecated('Use installedAppDescriptor instead')
const InstalledApp$json = {
  '1': 'InstalledApp',
  '2': [
    {'1': 'app_name', '3': 1, '4': 1, '5': 9, '10': 'appName'},
    {'1': 'sha256', '3': 2, '4': 1, '5': 9, '10': 'sha256'},
  ],
};

/// Descriptor for `InstalledApp`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List installedAppDescriptor = $convert.base64Decode(
    'CgxJbnN0YWxsZWRBcHASGQoIYXBwX25hbWUYASABKAlSB2FwcE5hbWUSFgoGc2hhMjU2GAIgAS'
    'gJUgZzaGEyNTY=');

@$core.Deprecated('Use clientAppListRequestDescriptor instead')
const ClientAppListRequest$json = {
  '1': 'ClientAppListRequest',
};

/// Descriptor for `ClientAppListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAppListRequestDescriptor =
    $convert.base64Decode('ChRDbGllbnRBcHBMaXN0UmVxdWVzdA==');

@$core.Deprecated('Use clientAppListResponseDescriptor instead')
const ClientAppListResponse$json = {
  '1': 'ClientAppListResponse',
  '2': [
    {'1': 'runtime', '3': 1, '4': 1, '5': 9, '10': 'runtime'},
    {
      '1': 'apps',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.InstalledApp',
      '10': 'apps'
    },
  ],
};

/// Descriptor for `ClientAppListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAppListResponseDescriptor = $convert.base64Decode(
    'ChVDbGllbnRBcHBMaXN0UmVzcG9uc2USGAoHcnVudGltZRgBIAEoCVIHcnVudGltZRIwCgRhcH'
    'BzGAIgAygLMhwuZ2l6Y2xhdy5ycGMudjEuSW5zdGFsbGVkQXBwUgRhcHBz');

@$core.Deprecated('Use clientAppInstallRequestDescriptor instead')
const ClientAppInstallRequest$json = {
  '1': 'ClientAppInstallRequest',
  '2': [
    {'1': 'app_name', '3': 1, '4': 1, '5': 9, '10': 'appName'},
    {'1': 'url', '3': 2, '4': 1, '5': 9, '10': 'url'},
    {'1': 'sha256', '3': 3, '4': 1, '5': 9, '10': 'sha256'},
    {'1': 'size', '3': 4, '4': 1, '5': 3, '10': 'size'},
  ],
};

/// Descriptor for `ClientAppInstallRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAppInstallRequestDescriptor = $convert.base64Decode(
    'ChdDbGllbnRBcHBJbnN0YWxsUmVxdWVzdBIZCghhcHBfbmFtZRgBIAEoCVIHYXBwTmFtZRIQCg'
    'N1cmwYAiABKAlSA3VybBIWCgZzaGEyNTYYAyABKAlSBnNoYTI1NhISCgRzaXplGAQgASgDUgRz'
    'aXpl');

@$core.Deprecated('Use clientAppInstallResponseDescriptor instead')
const ClientAppInstallResponse$json = {
  '1': 'ClientAppInstallResponse',
};

/// Descriptor for `ClientAppInstallResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAppInstallResponseDescriptor =
    $convert.base64Decode('ChhDbGllbnRBcHBJbnN0YWxsUmVzcG9uc2U=');

@$core.Deprecated('Use clientAppUninstallRequestDescriptor instead')
const ClientAppUninstallRequest$json = {
  '1': 'ClientAppUninstallRequest',
  '2': [
    {'1': 'app_name', '3': 1, '4': 1, '5': 9, '10': 'appName'},
  ],
};

/// Descriptor for `ClientAppUninstallRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAppUninstallRequestDescriptor =
    $convert.base64Decode(
        'ChlDbGllbnRBcHBVbmluc3RhbGxSZXF1ZXN0EhkKCGFwcF9uYW1lGAEgASgJUgdhcHBOYW1l');

@$core.Deprecated('Use clientAppUninstallResponseDescriptor instead')
const ClientAppUninstallResponse$json = {
  '1': 'ClientAppUninstallResponse',
};

/// Descriptor for `ClientAppUninstallResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAppUninstallResponseDescriptor =
    $convert.base64Decode('ChpDbGllbnRBcHBVbmluc3RhbGxSZXNwb25zZQ==');

@$core.Deprecated('Use clientAppInvokeRequestDescriptor instead')
const ClientAppInvokeRequest$json = {
  '1': 'ClientAppInvokeRequest',
  '2': [
    {'1': 'app_name', '3': 1, '4': 1, '5': 9, '10': 'appName'},
    {'1': 'method', '3': 2, '4': 1, '5': 9, '10': 'method'},
    {'1': 'args_json', '3': 3, '4': 1, '5': 9, '10': 'argsJson'},
  ],
};

/// Descriptor for `ClientAppInvokeRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAppInvokeRequestDescriptor =
    $convert.base64Decode(
        'ChZDbGllbnRBcHBJbnZva2VSZXF1ZXN0EhkKCGFwcF9uYW1lGAEgASgJUgdhcHBOYW1lEhYKBm'
        '1ldGhvZBgCIAEoCVIGbWV0aG9kEhsKCWFyZ3NfanNvbhgDIAEoCVIIYXJnc0pzb24=');

@$core.Deprecated('Use clientAppInvokeResponseDescriptor instead')
const ClientAppInvokeResponse$json = {
  '1': 'ClientAppInvokeResponse',
  '2': [
    {'1': 'result_json', '3': 1, '4': 1, '5': 9, '10': 'resultJson'},
  ],
};

/// Descriptor for `ClientAppInvokeResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAppInvokeResponseDescriptor =
    $convert.base64Decode(
        'ChdDbGllbnRBcHBJbnZva2VSZXNwb25zZRIfCgtyZXN1bHRfanNvbhgBIAEoCVIKcmVzdWx0Sn'
        'Nvbg==');

@$core.Deprecated('Use clientAppJobStartRequestDescriptor instead')
const ClientAppJobStartRequest$json = {
  '1': 'ClientAppJobStartRequest',
  '2': [
    {'1': 'app_name', '3': 1, '4': 1, '5': 9, '10': 'appName'},
    {'1': 'method', '3': 2, '4': 1, '5': 9, '10': 'method'},
    {'1': 'args_json', '3': 3, '4': 1, '5': 9, '10': 'argsJson'},
  ],
};

/// Descriptor for `ClientAppJobStartRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAppJobStartRequestDescriptor =
    $convert.base64Decode(
        'ChhDbGllbnRBcHBKb2JTdGFydFJlcXVlc3QSGQoIYXBwX25hbWUYASABKAlSB2FwcE5hbWUSFg'
        'oGbWV0aG9kGAIgASgJUgZtZXRob2QSGwoJYXJnc19qc29uGAMgASgJUghhcmdzSnNvbg==');

@$core.Deprecated('Use clientAppJobStartResponseDescriptor instead')
const ClientAppJobStartResponse$json = {
  '1': 'ClientAppJobStartResponse',
  '2': [
    {'1': 'job_id', '3': 1, '4': 1, '5': 13, '10': 'jobId'},
  ],
};

/// Descriptor for `ClientAppJobStartResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAppJobStartResponseDescriptor =
    $convert.base64Decode(
        'ChlDbGllbnRBcHBKb2JTdGFydFJlc3BvbnNlEhUKBmpvYl9pZBgBIAEoDVIFam9iSWQ=');

@$core.Deprecated('Use clientAppJobCancelRequestDescriptor instead')
const ClientAppJobCancelRequest$json = {
  '1': 'ClientAppJobCancelRequest',
  '2': [
    {'1': 'job_id', '3': 1, '4': 1, '5': 13, '10': 'jobId'},
  ],
};

/// Descriptor for `ClientAppJobCancelRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAppJobCancelRequestDescriptor =
    $convert.base64Decode(
        'ChlDbGllbnRBcHBKb2JDYW5jZWxSZXF1ZXN0EhUKBmpvYl9pZBgBIAEoDVIFam9iSWQ=');

@$core.Deprecated('Use clientAppJobCancelResponseDescriptor instead')
const ClientAppJobCancelResponse$json = {
  '1': 'ClientAppJobCancelResponse',
};

/// Descriptor for `ClientAppJobCancelResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAppJobCancelResponseDescriptor =
    $convert.base64Decode('ChpDbGllbnRBcHBKb2JDYW5jZWxSZXNwb25zZQ==');
