// This is a generated file - do not edit.
//
// Generated from payload/lua_app.proto.

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

@$core.Deprecated('Use luaAppInfoDescriptor instead')
const LuaAppInfo$json = {
  '1': 'LuaAppInfo',
  '2': [
    {'1': 'app_id', '3': 1, '4': 1, '5': 9, '10': 'appId'},
    {'1': 'version', '3': 2, '4': 1, '5': 9, '10': 'version'},
    {
      '1': 'display_name',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'displayName',
      '17': true
    },
    {
      '1': 'description',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'description',
      '17': true
    },
  ],
  '8': [
    {'1': '_display_name'},
    {'1': '_description'},
  ],
};

/// Descriptor for `LuaAppInfo`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List luaAppInfoDescriptor = $convert.base64Decode(
    'CgpMdWFBcHBJbmZvEhUKBmFwcF9pZBgBIAEoCVIFYXBwSWQSGAoHdmVyc2lvbhgCIAEoCVIHdm'
    'Vyc2lvbhImCgxkaXNwbGF5X25hbWUYAyABKAlIAFILZGlzcGxheU5hbWWIAQESJQoLZGVzY3Jp'
    'cHRpb24YBCABKAlIAVILZGVzY3JpcHRpb26IAQFCDwoNX2Rpc3BsYXlfbmFtZUIOCgxfZGVzY3'
    'JpcHRpb24=');

@$core.Deprecated('Use clientLuaAppListRequestDescriptor instead')
const ClientLuaAppListRequest$json = {
  '1': 'ClientLuaAppListRequest',
};

/// Descriptor for `ClientLuaAppListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientLuaAppListRequestDescriptor =
    $convert.base64Decode('ChdDbGllbnRMdWFBcHBMaXN0UmVxdWVzdA==');

@$core.Deprecated('Use clientLuaAppListResponseDescriptor instead')
const ClientLuaAppListResponse$json = {
  '1': 'ClientLuaAppListResponse',
  '2': [
    {
      '1': 'apps',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.LuaAppInfo',
      '10': 'apps'
    },
  ],
};

/// Descriptor for `ClientLuaAppListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientLuaAppListResponseDescriptor =
    $convert.base64Decode(
        'ChhDbGllbnRMdWFBcHBMaXN0UmVzcG9uc2USLgoEYXBwcxgBIAMoCzIaLmdpemNsYXcucnBjLn'
        'YxLkx1YUFwcEluZm9SBGFwcHM=');

@$core.Deprecated('Use clientLuaAppInstallRequestDescriptor instead')
const ClientLuaAppInstallRequest$json = {
  '1': 'ClientLuaAppInstallRequest',
  '2': [
    {'1': 'url', '3': 1, '4': 1, '5': 9, '10': 'url'},
    {'1': 'sha256', '3': 2, '4': 1, '5': 9, '9': 0, '10': 'sha256', '17': true},
  ],
  '8': [
    {'1': '_sha256'},
  ],
};

/// Descriptor for `ClientLuaAppInstallRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientLuaAppInstallRequestDescriptor =
    $convert.base64Decode(
        'ChpDbGllbnRMdWFBcHBJbnN0YWxsUmVxdWVzdBIQCgN1cmwYASABKAlSA3VybBIbCgZzaGEyNT'
        'YYAiABKAlIAFIGc2hhMjU2iAEBQgkKB19zaGEyNTY=');

@$core.Deprecated('Use clientLuaAppInstallResponseDescriptor instead')
const ClientLuaAppInstallResponse$json = {
  '1': 'ClientLuaAppInstallResponse',
  '2': [
    {
      '1': 'app',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.LuaAppInfo',
      '10': 'app'
    },
  ],
};

/// Descriptor for `ClientLuaAppInstallResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientLuaAppInstallResponseDescriptor =
    $convert.base64Decode(
        'ChtDbGllbnRMdWFBcHBJbnN0YWxsUmVzcG9uc2USLAoDYXBwGAEgASgLMhouZ2l6Y2xhdy5ycG'
        'MudjEuTHVhQXBwSW5mb1IDYXBw');

@$core.Deprecated('Use clientLuaAppRunRequestDescriptor instead')
const ClientLuaAppRunRequest$json = {
  '1': 'ClientLuaAppRunRequest',
  '2': [
    {'1': 'app_id', '3': 1, '4': 1, '5': 9, '10': 'appId'},
    {
      '1': 'params',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ClientLuaAppRunRequest.ParamsEntry',
      '10': 'params'
    },
  ],
  '3': [ClientLuaAppRunRequest_ParamsEntry$json],
};

@$core.Deprecated('Use clientLuaAppRunRequestDescriptor instead')
const ClientLuaAppRunRequest_ParamsEntry$json = {
  '1': 'ParamsEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 9, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `ClientLuaAppRunRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientLuaAppRunRequestDescriptor = $convert.base64Decode(
    'ChZDbGllbnRMdWFBcHBSdW5SZXF1ZXN0EhUKBmFwcF9pZBgBIAEoCVIFYXBwSWQSSgoGcGFyYW'
    '1zGAIgAygLMjIuZ2l6Y2xhdy5ycGMudjEuQ2xpZW50THVhQXBwUnVuUmVxdWVzdC5QYXJhbXNF'
    'bnRyeVIGcGFyYW1zGjkKC1BhcmFtc0VudHJ5EhAKA2tleRgBIAEoCVIDa2V5EhQKBXZhbHVlGA'
    'IgASgJUgV2YWx1ZToCOAE=');

@$core.Deprecated('Use clientLuaAppRunResponseDescriptor instead')
const ClientLuaAppRunResponse$json = {
  '1': 'ClientLuaAppRunResponse',
};

/// Descriptor for `ClientLuaAppRunResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientLuaAppRunResponseDescriptor =
    $convert.base64Decode('ChdDbGllbnRMdWFBcHBSdW5SZXNwb25zZQ==');
