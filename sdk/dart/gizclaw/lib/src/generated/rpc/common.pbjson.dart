// This is a generated file - do not edit.
//
// Generated from common.proto.

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

@$core.Deprecated('Use rpcErrorCodeDescriptor instead')
const RpcErrorCode$json = {
  '1': 'RpcErrorCode',
  '2': [
    {'1': 'RPC_ERROR_CODE_UNSPECIFIED', '2': 0},
    {'1': 'RPC_ERROR_CODE_PARSE_ERROR', '2': -32700},
    {'1': 'RPC_ERROR_CODE_INVALID_REQUEST', '2': -32600},
    {'1': 'RPC_ERROR_CODE_METHOD_NOT_FOUND', '2': -32601},
    {'1': 'RPC_ERROR_CODE_INVALID_PARAMS', '2': -32602},
    {'1': 'RPC_ERROR_CODE_INTERNAL_ERROR', '2': -32603},
    {'1': 'RPC_ERROR_CODE_BAD_REQUEST', '2': 400},
    {'1': 'RPC_ERROR_CODE_FORBIDDEN', '2': 403},
    {'1': 'RPC_ERROR_CODE_NOT_FOUND', '2': 404},
    {'1': 'RPC_ERROR_CODE_CONFLICT', '2': 409},
  ],
};

/// Descriptor for `RpcErrorCode`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List rpcErrorCodeDescriptor = $convert.base64Decode(
    'CgxScGNFcnJvckNvZGUSHgoaUlBDX0VSUk9SX0NPREVfVU5TUEVDSUZJRUQQABInChpSUENfRV'
    'JST1JfQ09ERV9QQVJTRV9FUlJPUhDEgP7///////8BEisKHlJQQ19FUlJPUl9DT0RFX0lOVkFM'
    'SURfUkVRVUVTVBCogf7///////8BEiwKH1JQQ19FUlJPUl9DT0RFX01FVEhPRF9OT1RfRk9VTk'
    'QQp4H+////////ARIqCh1SUENfRVJST1JfQ09ERV9JTlZBTElEX1BBUkFNUxCmgf7///////8B'
    'EioKHVJQQ19FUlJPUl9DT0RFX0lOVEVSTkFMX0VSUk9SEKWB/v///////wESHwoaUlBDX0VSUk'
    '9SX0NPREVfQkFEX1JFUVVFU1QQkAMSHQoYUlBDX0VSUk9SX0NPREVfRk9SQklEREVOEJMDEh0K'
    'GFJQQ19FUlJPUl9DT0RFX05PVF9GT1VORBCUAxIcChdSUENfRVJST1JfQ09ERV9DT05GTElDVB'
    'CZAw==');

@$core.Deprecated('Use rpcResponseDescriptor instead')
const RpcResponse$json = {
  '1': 'RpcResponse',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'payload', '3': 2, '4': 1, '5': 12, '9': 0, '10': 'payload'},
    {
      '1': 'error',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.RpcError',
      '9': 0,
      '10': 'error'
    },
  ],
  '8': [
    {'1': 'body'},
  ],
};

/// Descriptor for `RpcResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List rpcResponseDescriptor = $convert.base64Decode(
    'CgtScGNSZXNwb25zZRIOCgJpZBgBIAEoCVICaWQSGgoHcGF5bG9hZBgCIAEoDEgAUgdwYXlsb2'
    'FkEjAKBWVycm9yGAMgASgLMhguZ2l6Y2xhdy5ycGMudjEuUnBjRXJyb3JIAFIFZXJyb3JCBgoE'
    'Ym9keQ==');

@$core.Deprecated('Use rpcStreamFrameDescriptor instead')
const RpcStreamFrame$json = {
  '1': 'RpcStreamFrame',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'payload', '3': 2, '4': 1, '5': 12, '9': 0, '10': 'payload'},
    {
      '1': 'error',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.RpcError',
      '9': 0,
      '10': 'error'
    },
    {
      '1': 'end',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.RpcStreamEnd',
      '9': 0,
      '10': 'end'
    },
  ],
  '8': [
    {'1': 'body'},
  ],
};

/// Descriptor for `RpcStreamFrame`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List rpcStreamFrameDescriptor = $convert.base64Decode(
    'Cg5ScGNTdHJlYW1GcmFtZRIOCgJpZBgBIAEoCVICaWQSGgoHcGF5bG9hZBgCIAEoDEgAUgdwYX'
    'lsb2FkEjAKBWVycm9yGAMgASgLMhguZ2l6Y2xhdy5ycGMudjEuUnBjRXJyb3JIAFIFZXJyb3IS'
    'MAoDZW5kGAQgASgLMhwuZ2l6Y2xhdy5ycGMudjEuUnBjU3RyZWFtRW5kSABSA2VuZEIGCgRib2'
    'R5');

@$core.Deprecated('Use rpcErrorDescriptor instead')
const RpcError$json = {
  '1': 'RpcError',
  '2': [
    {
      '1': 'code',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.RpcErrorCode',
      '10': 'code'
    },
    {'1': 'message', '3': 2, '4': 1, '5': 9, '10': 'message'},
  ],
};

/// Descriptor for `RpcError`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List rpcErrorDescriptor = $convert.base64Decode(
    'CghScGNFcnJvchIwCgRjb2RlGAEgASgOMhwuZ2l6Y2xhdy5ycGMudjEuUnBjRXJyb3JDb2RlUg'
    'Rjb2RlEhgKB21lc3NhZ2UYAiABKAlSB21lc3NhZ2U=');

@$core.Deprecated('Use rpcStreamEndDescriptor instead')
const RpcStreamEnd$json = {
  '1': 'RpcStreamEnd',
};

/// Descriptor for `RpcStreamEnd`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List rpcStreamEndDescriptor =
    $convert.base64Decode('CgxScGNTdHJlYW1FbmQ=');
