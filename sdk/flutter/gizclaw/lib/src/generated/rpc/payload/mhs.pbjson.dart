// This is a generated file - do not edit.
//
// Generated from payload/mhs.proto.

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

@$core.Deprecated('Use mhsValueDescriptor instead')
const MhsValue$json = {
  '1': 'MhsValue',
  '2': [
    {'1': 'bool_value', '3': 1, '4': 1, '5': 8, '9': 0, '10': 'boolValue'},
    {'1': 'int_value', '3': 2, '4': 1, '5': 3, '9': 0, '10': 'intValue'},
    {'1': 'double_value', '3': 3, '4': 1, '5': 1, '9': 0, '10': 'doubleValue'},
    {'1': 'string_value', '3': 4, '4': 1, '5': 9, '9': 0, '10': 'stringValue'},
  ],
  '8': [
    {'1': 'value'},
  ],
};

/// Descriptor for `MhsValue`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List mhsValueDescriptor = $convert.base64Decode(
    'CghNaHNWYWx1ZRIfCgpib29sX3ZhbHVlGAEgASgISABSCWJvb2xWYWx1ZRIdCglpbnRfdmFsdW'
    'UYAiABKANIAFIIaW50VmFsdWUSIwoMZG91YmxlX3ZhbHVlGAMgASgBSABSC2RvdWJsZVZhbHVl'
    'EiMKDHN0cmluZ192YWx1ZRgEIAEoCUgAUgtzdHJpbmdWYWx1ZUIHCgV2YWx1ZQ==');

@$core.Deprecated('Use mhsStateRefDescriptor instead')
const MhsStateRef$json = {
  '1': 'MhsStateRef',
  '2': [
    {'1': 'device_id', '3': 1, '4': 1, '5': 9, '10': 'deviceId'},
    {'1': 'state', '3': 2, '4': 1, '5': 9, '10': 'state'},
  ],
};

/// Descriptor for `MhsStateRef`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List mhsStateRefDescriptor = $convert.base64Decode(
    'CgtNaHNTdGF0ZVJlZhIbCglkZXZpY2VfaWQYASABKAlSCGRldmljZUlkEhQKBXN0YXRlGAIgAS'
    'gJUgVzdGF0ZQ==');

@$core.Deprecated('Use mhsStateValueDescriptor instead')
const MhsStateValue$json = {
  '1': 'MhsStateValue',
  '2': [
    {'1': 'device_id', '3': 1, '4': 1, '5': 9, '10': 'deviceId'},
    {'1': 'state', '3': 2, '4': 1, '5': 9, '10': 'state'},
    {
      '1': 'value',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.MhsValue',
      '10': 'value'
    },
  ],
};

/// Descriptor for `MhsStateValue`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List mhsStateValueDescriptor = $convert.base64Decode(
    'Cg1NaHNTdGF0ZVZhbHVlEhsKCWRldmljZV9pZBgBIAEoCVIIZGV2aWNlSWQSFAoFc3RhdGUYAi'
    'ABKAlSBXN0YXRlEi4KBXZhbHVlGAMgASgLMhguZ2l6Y2xhdy5ycGMudjEuTWhzVmFsdWVSBXZh'
    'bHVl');

@$core.Deprecated('Use clientMhsV0ReadRequestDescriptor instead')
const ClientMhsV0ReadRequest$json = {
  '1': 'ClientMhsV0ReadRequest',
  '2': [
    {
      '1': 'states',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.MhsStateRef',
      '10': 'states'
    },
  ],
};

/// Descriptor for `ClientMhsV0ReadRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientMhsV0ReadRequestDescriptor =
    $convert.base64Decode(
        'ChZDbGllbnRNaHNWMFJlYWRSZXF1ZXN0EjMKBnN0YXRlcxgBIAMoCzIbLmdpemNsYXcucnBjLn'
        'YxLk1oc1N0YXRlUmVmUgZzdGF0ZXM=');

@$core.Deprecated('Use clientMhsV0ReadResponseDescriptor instead')
const ClientMhsV0ReadResponse$json = {
  '1': 'ClientMhsV0ReadResponse',
  '2': [
    {
      '1': 'states',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.MhsStateValue',
      '10': 'states'
    },
  ],
};

/// Descriptor for `ClientMhsV0ReadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientMhsV0ReadResponseDescriptor =
    $convert.base64Decode(
        'ChdDbGllbnRNaHNWMFJlYWRSZXNwb25zZRI1CgZzdGF0ZXMYASADKAsyHS5naXpjbGF3LnJwYy'
        '52MS5NaHNTdGF0ZVZhbHVlUgZzdGF0ZXM=');

@$core.Deprecated('Use clientMhsV0WriteRequestDescriptor instead')
const ClientMhsV0WriteRequest$json = {
  '1': 'ClientMhsV0WriteRequest',
  '2': [
    {
      '1': 'states',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.MhsStateValue',
      '10': 'states'
    },
  ],
};

/// Descriptor for `ClientMhsV0WriteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientMhsV0WriteRequestDescriptor =
    $convert.base64Decode(
        'ChdDbGllbnRNaHNWMFdyaXRlUmVxdWVzdBI1CgZzdGF0ZXMYASADKAsyHS5naXpjbGF3LnJwYy'
        '52MS5NaHNTdGF0ZVZhbHVlUgZzdGF0ZXM=');

@$core.Deprecated('Use clientMhsV0WriteResponseDescriptor instead')
const ClientMhsV0WriteResponse$json = {
  '1': 'ClientMhsV0WriteResponse',
  '2': [
    {
      '1': 'states',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.MhsStateValue',
      '10': 'states'
    },
  ],
};

/// Descriptor for `ClientMhsV0WriteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientMhsV0WriteResponseDescriptor =
    $convert.base64Decode(
        'ChhDbGllbnRNaHNWMFdyaXRlUmVzcG9uc2USNQoGc3RhdGVzGAEgAygLMh0uZ2l6Y2xhdy5ycG'
        'MudjEuTWhzU3RhdGVWYWx1ZVIGc3RhdGVz');
