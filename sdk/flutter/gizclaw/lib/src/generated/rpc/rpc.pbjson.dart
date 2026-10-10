// This is a generated file - do not edit.
//
// Generated from rpc.proto.

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

@$core.Deprecated('Use statusCodeDescriptor instead')
const StatusCode$json = {
  '1': 'StatusCode',
  '2': [
    {'1': 'STATUS_CODE_OK', '2': 0},
    {'1': 'STATUS_CODE_CANCELLED', '2': 1},
    {'1': 'STATUS_CODE_UNKNOWN', '2': 2},
    {'1': 'STATUS_CODE_INVALID_ARGUMENT', '2': 3},
    {'1': 'STATUS_CODE_DEADLINE_EXCEEDED', '2': 4},
    {'1': 'STATUS_CODE_NOT_FOUND', '2': 5},
    {'1': 'STATUS_CODE_ALREADY_EXISTS', '2': 6},
    {'1': 'STATUS_CODE_PERMISSION_DENIED', '2': 7},
    {'1': 'STATUS_CODE_RESOURCE_EXHAUSTED', '2': 8},
    {'1': 'STATUS_CODE_FAILED_PRECONDITION', '2': 9},
    {'1': 'STATUS_CODE_ABORTED', '2': 10},
    {'1': 'STATUS_CODE_OUT_OF_RANGE', '2': 11},
    {'1': 'STATUS_CODE_UNIMPLEMENTED', '2': 12},
    {'1': 'STATUS_CODE_INTERNAL', '2': 13},
    {'1': 'STATUS_CODE_UNAVAILABLE', '2': 14},
    {'1': 'STATUS_CODE_DATA_LOSS', '2': 15},
    {'1': 'STATUS_CODE_UNAUTHENTICATED', '2': 16},
  ],
};

/// Descriptor for `StatusCode`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List statusCodeDescriptor = $convert.base64Decode(
    'CgpTdGF0dXNDb2RlEhIKDlNUQVRVU19DT0RFX09LEAASGQoVU1RBVFVTX0NPREVfQ0FOQ0VMTE'
    'VEEAESFwoTU1RBVFVTX0NPREVfVU5LTk9XThACEiAKHFNUQVRVU19DT0RFX0lOVkFMSURfQVJH'
    'VU1FTlQQAxIhCh1TVEFUVVNfQ09ERV9ERUFETElORV9FWENFRURFRBAEEhkKFVNUQVRVU19DT0'
    'RFX05PVF9GT1VORBAFEh4KGlNUQVRVU19DT0RFX0FMUkVBRFlfRVhJU1RTEAYSIQodU1RBVFVT'
    'X0NPREVfUEVSTUlTU0lPTl9ERU5JRUQQBxIiCh5TVEFUVVNfQ09ERV9SRVNPVVJDRV9FWEhBVV'
    'NURUQQCBIjCh9TVEFUVVNfQ09ERV9GQUlMRURfUFJFQ09ORElUSU9OEAkSFwoTU1RBVFVTX0NP'
    'REVfQUJPUlRFRBAKEhwKGFNUQVRVU19DT0RFX09VVF9PRl9SQU5HRRALEh0KGVNUQVRVU19DT0'
    'RFX1VOSU1QTEVNRU5URUQQDBIYChRTVEFUVVNfQ09ERV9JTlRFUk5BTBANEhsKF1NUQVRVU19D'
    'T0RFX1VOQVZBSUxBQkxFEA4SGQoVU1RBVFVTX0NPREVfREFUQV9MT1NTEA8SHwobU1RBVFVTX0'
    'NPREVfVU5BVVRIRU5USUNBVEVEEBA=');

@$core.Deprecated('Use rpcMethodDescriptor instead')
const RpcMethod$json = {
  '1': 'RpcMethod',
  '2': [
    {'1': 'RPC_METHOD_UNSPECIFIED', '2': 0},
    {'1': 'RPC_METHOD_CLIENT_LUA_APP_INSTALL', '2': 139, '3': {}},
    {'1': 'RPC_METHOD_ALL_PING', '2': 1, '3': {}},
    {'1': 'RPC_METHOD_ALL_SPEED_TEST_RUN', '2': 2, '3': {}},
    {'1': 'RPC_METHOD_SERVER_INFO_GET', '2': 5, '3': {}},
    {'1': 'RPC_METHOD_SERVER_INFO_PUT', '2': 6, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUNTIME_GET', '2': 7, '3': {}},
    {'1': 'RPC_METHOD_SERVER_STATUS_GET', '2': 8, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUN_AGENT_GET', '2': 9, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUN_AGENT_SET', '2': 10, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUN_WORKSPACE_GET', '2': 11, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUN_WORKSPACE_SET', '2': 12, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUN_WORKSPACE_RELOAD', '2': 13, '3': {}},
    {
      '1': 'RPC_METHOD_SERVER_RUN_WORKSPACE_RELOAD_WITH_OPTIONS',
      '2': 120,
      '3': {}
    },
    {'1': 'RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY', '2': 14, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY_PLAY', '2': 15, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUN_WORKSPACE_MEMORY_STATS', '2': 16, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUN_WORKSPACE_RECALL', '2': 17, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUN_RELOAD', '2': 18, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUN_STATUS', '2': 19, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUN_STOP', '2': 20, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUN_SAY', '2': 21, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FIRMWARE_GET', '2': 22, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FIRMWARE_METADATA_GET', '2': 138, '3': {}},
    {'1': 'RPC_METHOD_SERVER_WORKSPACE_LIST', '2': 24, '3': {}},
    {'1': 'RPC_METHOD_SERVER_WORKSPACE_GET', '2': 25, '3': {}},
    {'1': 'RPC_METHOD_SERVER_WORKSPACE_CREATE', '2': 26, '3': {}},
    {'1': 'RPC_METHOD_SERVER_WORKSPACE_PUT', '2': 27, '3': {}},
    {'1': 'RPC_METHOD_SERVER_WORKSPACE_PARAMETERS_SET', '2': 110, '3': {}},
    {'1': 'RPC_METHOD_SERVER_WORKSPACE_DELETE', '2': 28, '3': {}},
    {'1': 'RPC_METHOD_SERVER_WORKSPACE_HISTORY_LIST', '2': 29, '3': {}},
    {'1': 'RPC_METHOD_SERVER_WORKSPACE_HISTORY_GET', '2': 30, '3': {}},
    {
      '1': 'RPC_METHOD_SERVER_WORKSPACE_HISTORY_AUDIO_DOWNLOAD',
      '2': 31,
      '3': {}
    },
    {'1': 'RPC_METHOD_SERVER_WORKFLOW_LIST', '2': 32, '3': {}},
    {'1': 'RPC_METHOD_SERVER_WORKFLOW_GET', '2': 33, '3': {}},
    {'1': 'RPC_METHOD_SERVER_MODEL_LIST', '2': 34, '3': {}},
    {'1': 'RPC_METHOD_SERVER_MODEL_GET', '2': 35, '3': {}},
    {'1': 'RPC_METHOD_SERVER_VOICE_LIST', '2': 36, '3': {}},
    {'1': 'RPC_METHOD_SERVER_VOICE_GET', '2': 37, '3': {}},
    {'1': 'RPC_METHOD_SERVER_CONTACT_LIST', '2': 38, '3': {}},
    {'1': 'RPC_METHOD_SERVER_CONTACT_GET', '2': 39, '3': {}},
    {'1': 'RPC_METHOD_SERVER_CONTACT_CREATE', '2': 40, '3': {}},
    {'1': 'RPC_METHOD_SERVER_CONTACT_PUT', '2': 41, '3': {}},
    {'1': 'RPC_METHOD_SERVER_CONTACT_DELETE', '2': 42, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_GET', '2': 43, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CREATE', '2': 44, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CLEAR', '2': 45, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_ADD', '2': 46, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_LIST', '2': 47, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_DELETE', '2': 48, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_GROUP_LIST', '2': 49, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_GROUP_GET', '2': 50, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_GROUP_CREATE', '2': 51, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_GROUP_PUT', '2': 52, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_GROUP_DELETE', '2': 53, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_GET', '2': 54, '3': {}},
    {
      '1': 'RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CREATE',
      '2': 55,
      '3': {}
    },
    {
      '1': 'RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CLEAR',
      '2': 56,
      '3': {}
    },
    {'1': 'RPC_METHOD_SERVER_FRIEND_GROUP_JOIN', '2': 57, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_LIST', '2': 58, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_ADD', '2': 59, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_PUT', '2': 60, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_DELETE', '2': 61, '3': {}},
    {'1': 'RPC_METHOD_SERVER_TOOL_LIST', '2': 80, '3': {}},
    {'1': 'RPC_METHOD_SERVER_TOOL_GET', '2': 81, '3': {}},
    {'1': 'RPC_METHOD_SERVER_PEER_LOOKUP', '2': 83, '3': {}},
    {'1': 'RPC_METHOD_SERVER_PEER_ASSIGN', '2': 84, '3': {}},
    {'1': 'RPC_METHOD_SERVER_ROUTE_RESOLVE', '2': 85, '3': {}},
    {'1': 'RPC_METHOD_SERVER_WORKSPACE_ICON_DOWNLOAD', '2': 88, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_INFO_GET', '2': 89, '3': {}},
    {'1': 'RPC_METHOD_SERVER_REGISTER', '2': 90, '3': {}},
    {'1': 'RPC_METHOD_SERVER_SPEECH_TRANSCRIBE', '2': 91, '3': {}},
    {'1': 'RPC_METHOD_SERVER_SPEECH_SYNTHESIZE', '2': 92, '3': {}},
    {'1': 'RPC_METHOD_SERVER_PEER_DELETE', '2': 93, '3': {}},
    {'1': 'RPC_METHOD_SERVER_SPEECH_EXTRACT', '2': 94, '3': {}},
    {'1': 'RPC_METHOD_SERVER_API_KEY_CREATE', '2': 96, '3': {}},
    {'1': 'RPC_METHOD_SERVER_API_KEY_LIST', '2': 97, '3': {}},
    {'1': 'RPC_METHOD_SERVER_API_KEY_REVOKE', '2': 98, '3': {}},
    {'1': 'RPC_METHOD_SERVER_API_KEY_RESOLVE', '2': 99, '3': {}},
    {'1': 'RPC_METHOD_SERVER_RUNTIME_PUT', '2': 112, '3': {}},
    {'1': 'RPC_METHOD_CLIENT_MHS_V0_READ', '2': 133, '3': {}},
    {'1': 'RPC_METHOD_CLIENT_MHS_V0_WRITE', '2': 134, '3': {}},
    {'1': 'RPC_METHOD_CLIENT_TOOL_V0_INVOKE', '2': 135, '3': {}},
    {'1': 'RPC_METHOD_CLIENT_TOOL_V0_LIST', '2': 136, '3': {}},
    {'1': 'RPC_METHOD_CLIENT_RPC_METHODS_LIST', '2': 137, '3': {}},
    {'1': 'RPC_METHOD_SERVER_APP_CONFIG_LIST', '2': 121, '3': {}},
    {'1': 'RPC_METHOD_SERVER_APP_CONFIG_GET', '2': 122, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_PING', '2': 123, '3': {}},
    {'1': 'RPC_METHOD_SERVER_FRIEND_GROUP_PING', '2': 124, '3': {}},
    {'1': 'RPC_METHOD_SERVER_PROFILE_GET', '2': 125, '3': {}},
  ],
};

/// Descriptor for `RpcMethod`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List rpcMethodDescriptor = $convert.base64Decode(
    'CglScGNNZXRob2QSGgoWUlBDX01FVEhPRF9VTlNQRUNJRklFRBAAEoMBCiFSUENfTUVUSE9EX0'
    'NMSUVOVF9MVUFfQVBQX0lOU1RBTEwQiwEaW8LzGFcKFmNsaWVudC5sdWEuYXBwLmluc3RhbGwS'
    'IENsaWVudEx1YUFwcEluc3RhbGxTdHJlYW1SZXF1ZXN0GhtDbGllbnRMdWFBcHBJbnN0YWxsUm'
    'VzcG9uc2USQgoTUlBDX01FVEhPRF9BTExfUElORxABGinC8xglCghhbGwucGluZxILUGluZ1Jl'
    'cXVlc3QaDFBpbmdSZXNwb25zZRJgCh1SUENfTUVUSE9EX0FMTF9TUEVFRF9URVNUX1JVThACGj'
    '3C8xg5ChJhbGwuc3BlZWRfdGVzdC5ydW4SEFNwZWVkVGVzdFJlcXVlc3QaEVNwZWVkVGVzdFJl'
    'c3BvbnNlEmIKGlJQQ19NRVRIT0RfU0VSVkVSX0lORk9fR0VUEAUaQsLzGD4KD3NlcnZlci5pbm'
    'ZvLmdldBIUU2VydmVyR2V0SW5mb1JlcXVlc3QaFVNlcnZlckdldEluZm9SZXNwb25zZRJiChpS'
    'UENfTUVUSE9EX1NFUlZFUl9JTkZPX1BVVBAGGkLC8xg+Cg9zZXJ2ZXIuaW5mby5wdXQSFFNlcn'
    'ZlclB1dEluZm9SZXF1ZXN0GhVTZXJ2ZXJQdXRJbmZvUmVzcG9uc2USbgodUlBDX01FVEhPRF9T'
    'RVJWRVJfUlVOVElNRV9HRVQQBxpLwvMYRwoSc2VydmVyLnJ1bnRpbWUuZ2V0EhdTZXJ2ZXJHZX'
    'RSdW50aW1lUmVxdWVzdBoYU2VydmVyR2V0UnVudGltZVJlc3BvbnNlEmoKHFJQQ19NRVRIT0Rf'
    'U0VSVkVSX1NUQVRVU19HRVQQCBpIwvMYRAoRc2VydmVyLnN0YXR1cy5nZXQSFlNlcnZlckdldF'
    'N0YXR1c1JlcXVlc3QaF1NlcnZlckdldFN0YXR1c1Jlc3BvbnNlEnQKH1JQQ19NRVRIT0RfU0VS'
    'VkVSX1JVTl9BR0VOVF9HRVQQCRpPwvMYSwoUc2VydmVyLnJ1bi5hZ2VudC5nZXQSGFNlcnZlck'
    'dldFJ1bkFnZW50UmVxdWVzdBoZU2VydmVyR2V0UnVuQWdlbnRSZXNwb25zZRJ0Ch9SUENfTUVU'
    'SE9EX1NFUlZFUl9SVU5fQUdFTlRfU0VUEAoaT8LzGEsKFHNlcnZlci5ydW4uYWdlbnQuc2V0Eh'
    'hTZXJ2ZXJTZXRSdW5BZ2VudFJlcXVlc3QaGVNlcnZlclNldFJ1bkFnZW50UmVzcG9uc2UShAEK'
    'I1JQQ19NRVRIT0RfU0VSVkVSX1JVTl9XT1JLU1BBQ0VfR0VUEAsaW8LzGFcKGHNlcnZlci5ydW'
    '4ud29ya3NwYWNlLmdldBIcU2VydmVyR2V0UnVuV29ya3NwYWNlUmVxdWVzdBodU2VydmVyR2V0'
    'UnVuV29ya3NwYWNlUmVzcG9uc2UShAEKI1JQQ19NRVRIT0RfU0VSVkVSX1JVTl9XT1JLU1BBQ0'
    'VfU0VUEAwaW8LzGFcKGHNlcnZlci5ydW4ud29ya3NwYWNlLnNldBIcU2VydmVyU2V0UnVuV29y'
    'a3NwYWNlUmVxdWVzdBodU2VydmVyU2V0UnVuV29ya3NwYWNlUmVzcG9uc2USkAEKJlJQQ19NRV'
    'RIT0RfU0VSVkVSX1JVTl9XT1JLU1BBQ0VfUkVMT0FEEA0aZMLzGGAKG3NlcnZlci5ydW4ud29y'
    'a3NwYWNlLnJlbG9hZBIfU2VydmVyUmVsb2FkUnVuV29ya3NwYWNlUmVxdWVzdBogU2VydmVyUm'
    'Vsb2FkUnVuV29ya3NwYWNlUmVzcG9uc2USwgEKM1JQQ19NRVRIT0RfU0VSVkVSX1JVTl9XT1JL'
    'U1BBQ0VfUkVMT0FEX1dJVEhfT1BUSU9OUxB4GogBwvMYgwEKKHNlcnZlci5ydW4ud29ya3NwYW'
    'NlLnJlbG9hZC13aXRoLW9wdGlvbnMSKlNlcnZlclJlbG9hZFJ1bldvcmtzcGFjZVdpdGhPcHRp'
    'b25zUmVxdWVzdBorU2VydmVyUmVsb2FkUnVuV29ya3NwYWNlV2l0aE9wdGlvbnNSZXNwb25zZR'
    'KcAQonUlBDX01FVEhPRF9TRVJWRVJfUlVOX1dPUktTUEFDRV9ISVNUT1JZEA4ab8LzGGsKHHNl'
    'cnZlci5ydW4ud29ya3NwYWNlLmhpc3RvcnkSJFNlcnZlckxpc3RSdW5Xb3Jrc3BhY2VIaXN0b3'
    'J5UmVxdWVzdBolU2VydmVyTGlzdFJ1bldvcmtzcGFjZUhpc3RvcnlSZXNwb25zZRKmAQosUlBD'
    'X01FVEhPRF9TRVJWRVJfUlVOX1dPUktTUEFDRV9ISVNUT1JZX1BMQVkQDxp0wvMYcAohc2Vydm'
    'VyLnJ1bi53b3Jrc3BhY2UuaGlzdG9yeS5wbGF5EiRTZXJ2ZXJQbGF5UnVuV29ya3NwYWNlSGlz'
    'dG9yeVJlcXVlc3QaJVNlcnZlclBsYXlSdW5Xb3Jrc3BhY2VIaXN0b3J5UmVzcG9uc2USrAEKLF'
    'JQQ19NRVRIT0RfU0VSVkVSX1JVTl9XT1JLU1BBQ0VfTUVNT1JZX1NUQVRTEBAaesLzGHYKIXNl'
    'cnZlci5ydW4ud29ya3NwYWNlLm1lbW9yeS5zdGF0cxInU2VydmVyR2V0UnVuV29ya3NwYWNlTW'
    'Vtb3J5U3RhdHNSZXF1ZXN0GihTZXJ2ZXJHZXRSdW5Xb3Jrc3BhY2VNZW1vcnlTdGF0c1Jlc3Bv'
    'bnNlEpABCiZSUENfTUVUSE9EX1NFUlZFUl9SVU5fV09SS1NQQUNFX1JFQ0FMTBARGmTC8xhgCh'
    'tzZXJ2ZXIucnVuLndvcmtzcGFjZS5yZWNhbGwSH1NlcnZlclJ1bldvcmtzcGFjZVJlY2FsbFJl'
    'cXVlc3QaIFNlcnZlclJ1bldvcmtzcGFjZVJlY2FsbFJlc3BvbnNlEmoKHFJQQ19NRVRIT0RfU0'
    'VSVkVSX1JVTl9SRUxPQUQQEhpIwvMYRAoRc2VydmVyLnJ1bi5yZWxvYWQSFlNlcnZlclJlbG9h'
    'ZFJ1blJlcXVlc3QaF1NlcnZlclJlbG9hZFJ1blJlc3BvbnNlEnAKHFJQQ19NRVRIT0RfU0VSVk'
    'VSX1JVTl9TVEFUVVMQExpOwvMYSgoRc2VydmVyLnJ1bi5zdGF0dXMSGVNlcnZlckdldFJ1blN0'
    'YXR1c1JlcXVlc3QaGlNlcnZlckdldFJ1blN0YXR1c1Jlc3BvbnNlEmIKGlJQQ19NRVRIT0RfU0'
    'VSVkVSX1JVTl9TVE9QEBQaQsLzGD4KD3NlcnZlci5ydW4uc3RvcBIUU2VydmVyU3RvcFJ1blJl'
    'cXVlc3QaFVNlcnZlclN0b3BSdW5SZXNwb25zZRJeChlSUENfTUVUSE9EX1NFUlZFUl9SVU5fU0'
    'FZEBUaP8LzGDsKDnNlcnZlci5ydW4uc2F5EhNTZXJ2ZXJSdW5TYXlSZXF1ZXN0GhRTZXJ2ZXJS'
    'dW5TYXlSZXNwb25zZRJmCh5SUENfTUVUSE9EX1NFUlZFUl9GSVJNV0FSRV9HRVQQFhpCwvMYPg'
    'oTc2VydmVyLmZpcm13YXJlLmdldBISRmlybXdhcmVHZXRSZXF1ZXN0GhNGaXJtd2FyZUdldFJl'
    'c3BvbnNlEokBCidSUENfTUVUSE9EX1NFUlZFUl9GSVJNV0FSRV9NRVRBREFUQV9HRVQQigEaW8'
    'LzGFcKHHNlcnZlci5maXJtd2FyZS5tZXRhZGF0YS5nZXQSGkZpcm13YXJlTWV0YWRhdGFHZXRS'
    'ZXF1ZXN0GhtGaXJtd2FyZU1ldGFkYXRhR2V0UmVzcG9uc2USbgogUlBDX01FVEhPRF9TRVJWRV'
    'JfV09SS1NQQUNFX0xJU1QQGBpIwvMYRAoVc2VydmVyLndvcmtzcGFjZS5saXN0EhRXb3Jrc3Bh'
    'Y2VMaXN0UmVxdWVzdBoVV29ya3NwYWNlTGlzdFJlc3BvbnNlEmoKH1JQQ19NRVRIT0RfU0VSVk'
    'VSX1dPUktTUEFDRV9HRVQQGRpFwvMYQQoUc2VydmVyLndvcmtzcGFjZS5nZXQSE1dvcmtzcGFj'
    'ZUdldFJlcXVlc3QaFFdvcmtzcGFjZUdldFJlc3BvbnNlEnYKIlJQQ19NRVRIT0RfU0VSVkVSX1'
    'dPUktTUEFDRV9DUkVBVEUQGhpOwvMYSgoXc2VydmVyLndvcmtzcGFjZS5jcmVhdGUSFldvcmtz'
    'cGFjZUNyZWF0ZVJlcXVlc3QaF1dvcmtzcGFjZUNyZWF0ZVJlc3BvbnNlEmoKH1JQQ19NRVRIT0'
    'RfU0VSVkVSX1dPUktTUEFDRV9QVVQQGxpFwvMYQQoUc2VydmVyLndvcmtzcGFjZS5wdXQSE1dv'
    'cmtzcGFjZVB1dFJlcXVlc3QaFFdvcmtzcGFjZVB1dFJlc3BvbnNlEpQBCipSUENfTUVUSE9EX1'
    'NFUlZFUl9XT1JLU1BBQ0VfUEFSQU1FVEVSU19TRVQQbhpkwvMYYAofc2VydmVyLndvcmtzcGFj'
    'ZS5wYXJhbWV0ZXJzLnNldBIdV29ya3NwYWNlUGFyYW1ldGVyc1NldFJlcXVlc3QaHldvcmtzcG'
    'FjZVBhcmFtZXRlcnNTZXRSZXNwb25zZRJ2CiJSUENfTUVUSE9EX1NFUlZFUl9XT1JLU1BBQ0Vf'
    'REVMRVRFEBwaTsLzGEoKF3NlcnZlci53b3Jrc3BhY2UuZGVsZXRlEhZXb3Jrc3BhY2VEZWxldG'
    'VSZXF1ZXN0GhdXb3Jrc3BhY2VEZWxldGVSZXNwb25zZRKMAQooUlBDX01FVEhPRF9TRVJWRVJf'
    'V09SS1NQQUNFX0hJU1RPUllfTElTVBAdGl7C8xhaCh1zZXJ2ZXIud29ya3NwYWNlLmhpc3Rvcn'
    'kubGlzdBIbV29ya3NwYWNlSGlzdG9yeUxpc3RSZXF1ZXN0GhxXb3Jrc3BhY2VIaXN0b3J5TGlz'
    'dFJlc3BvbnNlEogBCidSUENfTUVUSE9EX1NFUlZFUl9XT1JLU1BBQ0VfSElTVE9SWV9HRVQQHh'
    'pbwvMYVwocc2VydmVyLndvcmtzcGFjZS5oaXN0b3J5LmdldBIaV29ya3NwYWNlSGlzdG9yeUdl'
    'dFJlcXVlc3QaG1dvcmtzcGFjZUhpc3RvcnlHZXRSZXNwb25zZRKyAQoyUlBDX01FVEhPRF9TRV'
    'JWRVJfV09SS1NQQUNFX0hJU1RPUllfQVVESU9fRE9XTkxPQUQQHxp6wvMYdgonc2VydmVyLndv'
    'cmtzcGFjZS5oaXN0b3J5LmF1ZGlvLmRvd25sb2FkEiRXb3Jrc3BhY2VIaXN0b3J5QXVkaW9Eb3'
    'dubG9hZFJlcXVlc3QaJVdvcmtzcGFjZUhpc3RvcnlBdWRpb0Rvd25sb2FkUmVzcG9uc2USagof'
    'UlBDX01FVEhPRF9TRVJWRVJfV09SS0ZMT1dfTElTVBAgGkXC8xhBChRzZXJ2ZXIud29ya2Zsb3'
    'cubGlzdBITV29ya2Zsb3dMaXN0UmVxdWVzdBoUV29ya2Zsb3dMaXN0UmVzcG9uc2USZgoeUlBD'
    'X01FVEhPRF9TRVJWRVJfV09SS0ZMT1dfR0VUECEaQsLzGD4KE3NlcnZlci53b3JrZmxvdy5nZX'
    'QSEldvcmtmbG93R2V0UmVxdWVzdBoTV29ya2Zsb3dHZXRSZXNwb25zZRJeChxSUENfTUVUSE9E'
    'X1NFUlZFUl9NT0RFTF9MSVNUECIaPMLzGDgKEXNlcnZlci5tb2RlbC5saXN0EhBNb2RlbExpc3'
    'RSZXF1ZXN0GhFNb2RlbExpc3RSZXNwb25zZRJaChtSUENfTUVUSE9EX1NFUlZFUl9NT0RFTF9H'
    'RVQQIxo5wvMYNQoQc2VydmVyLm1vZGVsLmdldBIPTW9kZWxHZXRSZXF1ZXN0GhBNb2RlbEdldF'
    'Jlc3BvbnNlEl4KHFJQQ19NRVRIT0RfU0VSVkVSX1ZPSUNFX0xJU1QQJBo8wvMYOAoRc2VydmVy'
    'LnZvaWNlLmxpc3QSEFZvaWNlTGlzdFJlcXVlc3QaEVZvaWNlTGlzdFJlc3BvbnNlEloKG1JQQ1'
    '9NRVRIT0RfU0VSVkVSX1ZPSUNFX0dFVBAlGjnC8xg1ChBzZXJ2ZXIudm9pY2UuZ2V0Eg9Wb2lj'
    'ZUdldFJlcXVlc3QaEFZvaWNlR2V0UmVzcG9uc2USZgoeUlBDX01FVEhPRF9TRVJWRVJfQ09OVE'
    'FDVF9MSVNUECYaQsLzGD4KE3NlcnZlci5jb250YWN0Lmxpc3QSEkNvbnRhY3RMaXN0UmVxdWVz'
    'dBoTQ29udGFjdExpc3RSZXNwb25zZRJiCh1SUENfTUVUSE9EX1NFUlZFUl9DT05UQUNUX0dFVB'
    'AnGj/C8xg7ChJzZXJ2ZXIuY29udGFjdC5nZXQSEUNvbnRhY3RHZXRSZXF1ZXN0GhJDb250YWN0'
    'R2V0UmVzcG9uc2USbgogUlBDX01FVEhPRF9TRVJWRVJfQ09OVEFDVF9DUkVBVEUQKBpIwvMYRA'
    'oVc2VydmVyLmNvbnRhY3QuY3JlYXRlEhRDb250YWN0Q3JlYXRlUmVxdWVzdBoVQ29udGFjdENy'
    'ZWF0ZVJlc3BvbnNlEmIKHVJQQ19NRVRIT0RfU0VSVkVSX0NPTlRBQ1RfUFVUECkaP8LzGDsKEn'
    'NlcnZlci5jb250YWN0LnB1dBIRQ29udGFjdFB1dFJlcXVlc3QaEkNvbnRhY3RQdXRSZXNwb25z'
    'ZRJuCiBSUENfTUVUSE9EX1NFUlZFUl9DT05UQUNUX0RFTEVURRAqGkjC8xhEChVzZXJ2ZXIuY2'
    '9udGFjdC5kZWxldGUSFENvbnRhY3REZWxldGVSZXF1ZXN0GhVDb250YWN0RGVsZXRlUmVzcG9u'
    'c2USjgEKKVJQQ19NRVRIT0RfU0VSVkVSX0ZSSUVORF9JTlZJVEVfVE9LRU5fR0VUECsaX8LzGF'
    'sKHnNlcnZlci5mcmllbmQuaW52aXRlX3Rva2VuLmdldBIbRnJpZW5kSW52aXRlVG9rZW5HZXRS'
    'ZXF1ZXN0GhxGcmllbmRJbnZpdGVUb2tlbkdldFJlc3BvbnNlEpoBCixSUENfTUVUSE9EX1NFUl'
    'ZFUl9GUklFTkRfSU5WSVRFX1RPS0VOX0NSRUFURRAsGmjC8xhkCiFzZXJ2ZXIuZnJpZW5kLmlu'
    'dml0ZV90b2tlbi5jcmVhdGUSHkZyaWVuZEludml0ZVRva2VuQ3JlYXRlUmVxdWVzdBofRnJpZW'
    '5kSW52aXRlVG9rZW5DcmVhdGVSZXNwb25zZRKWAQorUlBDX01FVEhPRF9TRVJWRVJfRlJJRU5E'
    'X0lOVklURV9UT0tFTl9DTEVBUhAtGmXC8xhhCiBzZXJ2ZXIuZnJpZW5kLmludml0ZV90b2tlbi'
    '5jbGVhchIdRnJpZW5kSW52aXRlVG9rZW5DbGVhclJlcXVlc3QaHkZyaWVuZEludml0ZVRva2Vu'
    'Q2xlYXJSZXNwb25zZRJeChxSUENfTUVUSE9EX1NFUlZFUl9GUklFTkRfQUREEC4aPMLzGDgKEX'
    'NlcnZlci5mcmllbmQuYWRkEhBGcmllbmRBZGRSZXF1ZXN0GhFGcmllbmRBZGRSZXNwb25zZRJi'
    'Ch1SUENfTUVUSE9EX1NFUlZFUl9GUklFTkRfTElTVBAvGj/C8xg7ChJzZXJ2ZXIuZnJpZW5kLm'
    'xpc3QSEUZyaWVuZExpc3RSZXF1ZXN0GhJGcmllbmRMaXN0UmVzcG9uc2USagofUlBDX01FVEhP'
    'RF9TRVJWRVJfRlJJRU5EX0RFTEVURRAwGkXC8xhBChRzZXJ2ZXIuZnJpZW5kLmRlbGV0ZRITRn'
    'JpZW5kRGVsZXRlUmVxdWVzdBoURnJpZW5kRGVsZXRlUmVzcG9uc2USeAojUlBDX01FVEhPRF9T'
    'RVJWRVJfRlJJRU5EX0dST1VQX0xJU1QQMRpPwvMYSwoYc2VydmVyLmZyaWVuZF9ncm91cC5saX'
    'N0EhZGcmllbmRHcm91cExpc3RSZXF1ZXN0GhdGcmllbmRHcm91cExpc3RSZXNwb25zZRJ0CiJS'
    'UENfTUVUSE9EX1NFUlZFUl9GUklFTkRfR1JPVVBfR0VUEDIaTMLzGEgKF3NlcnZlci5mcmllbm'
    'RfZ3JvdXAuZ2V0EhVGcmllbmRHcm91cEdldFJlcXVlc3QaFkZyaWVuZEdyb3VwR2V0UmVzcG9u'
    'c2USgAEKJVJQQ19NRVRIT0RfU0VSVkVSX0ZSSUVORF9HUk9VUF9DUkVBVEUQMxpVwvMYUQoac2'
    'VydmVyLmZyaWVuZF9ncm91cC5jcmVhdGUSGEZyaWVuZEdyb3VwQ3JlYXRlUmVxdWVzdBoZRnJp'
    'ZW5kR3JvdXBDcmVhdGVSZXNwb25zZRJ0CiJSUENfTUVUSE9EX1NFUlZFUl9GUklFTkRfR1JPVV'
    'BfUFVUEDQaTMLzGEgKF3NlcnZlci5mcmllbmRfZ3JvdXAucHV0EhVGcmllbmRHcm91cFB1dFJl'
    'cXVlc3QaFkZyaWVuZEdyb3VwUHV0UmVzcG9uc2USgAEKJVJQQ19NRVRIT0RfU0VSVkVSX0ZSSU'
    'VORF9HUk9VUF9ERUxFVEUQNRpVwvMYUQoac2VydmVyLmZyaWVuZF9ncm91cC5kZWxldGUSGEZy'
    'aWVuZEdyb3VwRGVsZXRlUmVxdWVzdBoZRnJpZW5kR3JvdXBEZWxldGVSZXNwb25zZRKkAQovUl'
    'BDX01FVEhPRF9TRVJWRVJfRlJJRU5EX0dST1VQX0lOVklURV9UT0tFTl9HRVQQNhpvwvMYawok'
    'c2VydmVyLmZyaWVuZF9ncm91cC5pbnZpdGVfdG9rZW4uZ2V0EiBGcmllbmRHcm91cEludml0ZV'
    'Rva2VuR2V0UmVxdWVzdBohRnJpZW5kR3JvdXBJbnZpdGVUb2tlbkdldFJlc3BvbnNlErABCjJS'
    'UENfTUVUSE9EX1NFUlZFUl9GUklFTkRfR1JPVVBfSU5WSVRFX1RPS0VOX0NSRUFURRA3GnjC8x'
    'h0CidzZXJ2ZXIuZnJpZW5kX2dyb3VwLmludml0ZV90b2tlbi5jcmVhdGUSI0ZyaWVuZEdyb3Vw'
    'SW52aXRlVG9rZW5DcmVhdGVSZXF1ZXN0GiRGcmllbmRHcm91cEludml0ZVRva2VuQ3JlYXRlUm'
    'VzcG9uc2USrAEKMVJQQ19NRVRIT0RfU0VSVkVSX0ZSSUVORF9HUk9VUF9JTlZJVEVfVE9LRU5f'
    'Q0xFQVIQOBp1wvMYcQomc2VydmVyLmZyaWVuZF9ncm91cC5pbnZpdGVfdG9rZW4uY2xlYXISIk'
    'ZyaWVuZEdyb3VwSW52aXRlVG9rZW5DbGVhclJlcXVlc3QaI0ZyaWVuZEdyb3VwSW52aXRlVG9r'
    'ZW5DbGVhclJlc3BvbnNlEngKI1JQQ19NRVRIT0RfU0VSVkVSX0ZSSUVORF9HUk9VUF9KT0lOED'
    'kaT8LzGEsKGHNlcnZlci5mcmllbmRfZ3JvdXAuam9pbhIWRnJpZW5kR3JvdXBKb2luUmVxdWVz'
    'dBoXRnJpZW5kR3JvdXBKb2luUmVzcG9uc2USlAEKK1JQQ19NRVRIT0RfU0VSVkVSX0ZSSUVORF'
    '9HUk9VUF9NRU1CRVJTX0xJU1QQOhpjwvMYXwogc2VydmVyLmZyaWVuZF9ncm91cC5tZW1iZXJz'
    'Lmxpc3QSHEZyaWVuZEdyb3VwTWVtYmVyTGlzdFJlcXVlc3QaHUZyaWVuZEdyb3VwTWVtYmVyTG'
    'lzdFJlc3BvbnNlEpABCipSUENfTUVUSE9EX1NFUlZFUl9GUklFTkRfR1JPVVBfTUVNQkVSU19B'
    'REQQOxpgwvMYXAofc2VydmVyLmZyaWVuZF9ncm91cC5tZW1iZXJzLmFkZBIbRnJpZW5kR3JvdX'
    'BNZW1iZXJBZGRSZXF1ZXN0GhxGcmllbmRHcm91cE1lbWJlckFkZFJlc3BvbnNlEpABCipSUENf'
    'TUVUSE9EX1NFUlZFUl9GUklFTkRfR1JPVVBfTUVNQkVSU19QVVQQPBpgwvMYXAofc2VydmVyLm'
    'ZyaWVuZF9ncm91cC5tZW1iZXJzLnB1dBIbRnJpZW5kR3JvdXBNZW1iZXJQdXRSZXF1ZXN0GhxG'
    'cmllbmRHcm91cE1lbWJlclB1dFJlc3BvbnNlEpwBCi1SUENfTUVUSE9EX1NFUlZFUl9GUklFTk'
    'RfR1JPVVBfTUVNQkVSU19ERUxFVEUQPRppwvMYZQoic2VydmVyLmZyaWVuZF9ncm91cC5tZW1i'
    'ZXJzLmRlbGV0ZRIeRnJpZW5kR3JvdXBNZW1iZXJEZWxldGVSZXF1ZXN0Gh9GcmllbmRHcm91cE'
    '1lbWJlckRlbGV0ZVJlc3BvbnNlEloKG1JQQ19NRVRIT0RfU0VSVkVSX1RPT0xfTElTVBBQGjnC'
    '8xg1ChBzZXJ2ZXIudG9vbC5saXN0Eg9Ub29sTGlzdFJlcXVlc3QaEFRvb2xMaXN0UmVzcG9uc2'
    'USVgoaUlBDX01FVEhPRF9TRVJWRVJfVE9PTF9HRVQQURo2wvMYMgoPc2VydmVyLnRvb2wuZ2V0'
    'Eg5Ub29sR2V0UmVxdWVzdBoPVG9vbEdldFJlc3BvbnNlEm4KHVJQQ19NRVRIT0RfU0VSVkVSX1'
    'BFRVJfTE9PS1VQEFMaS8LzGEcKEnNlcnZlci5wZWVyLmxvb2t1cBIXU2VydmVyUGVlckxvb2t1'
    'cFJlcXVlc3QaGFNlcnZlclBlZXJMb29rdXBSZXNwb25zZRJuCh1SUENfTUVUSE9EX1NFUlZFUl'
    '9QRUVSX0FTU0lHThBUGkvC8xhHChJzZXJ2ZXIucGVlci5hc3NpZ24SF1NlcnZlclBlZXJBc3Np'
    'Z25SZXF1ZXN0GhhTZXJ2ZXJQZWVyQXNzaWduUmVzcG9uc2USdgofUlBDX01FVEhPRF9TRVJWRV'
    'JfUk9VVEVfUkVTT0xWRRBVGlHC8xhNChRzZXJ2ZXIucm91dGUucmVzb2x2ZRIZU2VydmVyUm91'
    'dGVSZXNvbHZlUmVxdWVzdBoaU2VydmVyUm91dGVSZXNvbHZlUmVzcG9uc2USkAEKKVJQQ19NRV'
    'RIT0RfU0VSVkVSX1dPUktTUEFDRV9JQ09OX0RPV05MT0FEEFgaYcLzGF0KHnNlcnZlci53b3Jr'
    'c3BhY2UuaWNvbi5kb3dubG9hZBIcV29ya3NwYWNlSWNvbkRvd25sb2FkUmVxdWVzdBodV29ya3'
    'NwYWNlSWNvbkRvd25sb2FkUmVzcG9uc2UScAohUlBDX01FVEhPRF9TRVJWRVJfRlJJRU5EX0lO'
    'Rk9fR0VUEFkaScLzGEUKFnNlcnZlci5mcmllbmQuaW5mby5nZXQSFEZyaWVuZEluZm9HZXRSZX'
    'F1ZXN0GhVGcmllbmRJbmZvR2V0UmVzcG9uc2USZAoaUlBDX01FVEhPRF9TRVJWRVJfUkVHSVNU'
    'RVIQWhpEwvMYQAoPc2VydmVyLnJlZ2lzdGVyEhVTZXJ2ZXJSZWdpc3RlclJlcXVlc3QaFlNlcn'
    'ZlclJlZ2lzdGVyUmVzcG9uc2USegojUlBDX01FVEhPRF9TRVJWRVJfU1BFRUNIX1RSQU5TQ1JJ'
    'QkUQWxpRwvMYTQoYc2VydmVyLnNwZWVjaC50cmFuc2NyaWJlEhdTcGVlY2hUcmFuc2NyaWJlUm'
    'VxdWVzdBoYU3BlZWNoVHJhbnNjcmliZVJlc3BvbnNlEnoKI1JQQ19NRVRIT0RfU0VSVkVSX1NQ'
    'RUVDSF9TWU5USEVTSVpFEFwaUcLzGE0KGHNlcnZlci5zcGVlY2guc3ludGhlc2l6ZRIXU3BlZW'
    'NoU3ludGhlc2l6ZVJlcXVlc3QaGFNwZWVjaFN5bnRoZXNpemVSZXNwb25zZRJuCh1SUENfTUVU'
    'SE9EX1NFUlZFUl9QRUVSX0RFTEVURRBdGkvC8xhHChJzZXJ2ZXIucGVlci5kZWxldGUSF1Nlcn'
    'ZlclBlZXJEZWxldGVSZXF1ZXN0GhhTZXJ2ZXJQZWVyRGVsZXRlUmVzcG9uc2USbgogUlBDX01F'
    'VEhPRF9TRVJWRVJfU1BFRUNIX0VYVFJBQ1QQXhpIwvMYRAoVc2VydmVyLnNwZWVjaC5leHRyYW'
    'N0EhRTcGVlY2hFeHRyYWN0UmVxdWVzdBoVU3BlZWNoRXh0cmFjdFJlc3BvbnNlEmwKIFJQQ19N'
    'RVRIT0RfU0VSVkVSX0FQSV9LRVlfQ1JFQVRFEGAaRsLzGEIKFXNlcnZlci5hcGlfa2V5LmNyZW'
    'F0ZRITQVBJS2V5Q3JlYXRlUmVxdWVzdBoUQVBJS2V5Q3JlYXRlUmVzcG9uc2USZAoeUlBDX01F'
    'VEhPRF9TRVJWRVJfQVBJX0tFWV9MSVNUEGEaQMLzGDwKE3NlcnZlci5hcGlfa2V5Lmxpc3QSEU'
    'FQSUtleUxpc3RSZXF1ZXN0GhJBUElLZXlMaXN0UmVzcG9uc2USbAogUlBDX01FVEhPRF9TRVJW'
    'RVJfQVBJX0tFWV9SRVZPS0UQYhpGwvMYQgoVc2VydmVyLmFwaV9rZXkucmV2b2tlEhNBUElLZX'
    'lSZXZva2VSZXF1ZXN0GhRBUElLZXlSZXZva2VSZXNwb25zZRJ8CiFSUENfTUVUSE9EX1NFUlZF'
    'Ul9BUElfS0VZX1JFU09MVkUQYxpVwvMYUQoWc2VydmVyLmFwaV9rZXkucmVzb2x2ZRIaU2Vydm'
    'VyQVBJS2V5UmVzb2x2ZVJlcXVlc3QaG1NlcnZlckFQSUtleVJlc29sdmVSZXNwb25zZRJuCh1S'
    'UENfTUVUSE9EX1NFUlZFUl9SVU5USU1FX1BVVBBwGkvC8xhHChJzZXJ2ZXIucnVudGltZS5wdX'
    'QSF1NlcnZlclB1dFJ1bnRpbWVSZXF1ZXN0GhhTZXJ2ZXJQdXRSdW50aW1lUmVzcG9uc2USbQod'
    'UlBDX01FVEhPRF9DTElFTlRfTUhTX1YwX1JFQUQQhQEaScLzGEUKEmNsaWVudC5taHMudjAucm'
    'VhZBIWQ2xpZW50TWhzVjBSZWFkUmVxdWVzdBoXQ2xpZW50TWhzVjBSZWFkUmVzcG9uc2UScQoe'
    'UlBDX01FVEhPRF9DTElFTlRfTUhTX1YwX1dSSVRFEIYBGkzC8xhIChNjbGllbnQubWhzLnYwLn'
    'dyaXRlEhdDbGllbnRNaHNWMFdyaXRlUmVxdWVzdBoYQ2xpZW50TWhzVjBXcml0ZVJlc3BvbnNl'
    'EnkKIFJQQ19NRVRIT0RfQ0xJRU5UX1RPT0xfVjBfSU5WT0tFEIcBGlLC8xhOChVjbGllbnQudG'
    '9vbC52MC5pbnZva2USGUNsaWVudFRvb2xWMEludm9rZVJlcXVlc3QaGkNsaWVudFRvb2xWMElu'
    'dm9rZVJlc3BvbnNlEnEKHlJQQ19NRVRIT0RfQ0xJRU5UX1RPT0xfVjBfTElTVBCIARpMwvMYSA'
    'oTY2xpZW50LnRvb2wudjAubGlzdBIXQ2xpZW50VG9vbFYwTGlzdFJlcXVlc3QaGENsaWVudFRv'
    'b2xWMExpc3RSZXNwb25zZRKBAQoiUlBDX01FVEhPRF9DTElFTlRfUlBDX01FVEhPRFNfTElTVB'
    'CJARpYwvMYVAoXY2xpZW50LnJwYy5tZXRob2RzLmxpc3QSG0NsaWVudFJwY01ldGhvZHNMaXN0'
    'UmVxdWVzdBocQ2xpZW50UnBjTWV0aG9kc0xpc3RSZXNwb25zZRJwCiFSUENfTUVUSE9EX1NFUl'
    'ZFUl9BUFBfQ09ORklHX0xJU1QQeRpJwvMYRQoWc2VydmVyLmFwcF9jb25maWcubGlzdBIUQXBw'
    'Q29uZmlnTGlzdFJlcXVlc3QaFUFwcENvbmZpZ0xpc3RSZXNwb25zZRJsCiBSUENfTUVUSE9EX1'
    'NFUlZFUl9BUFBfQ09ORklHX0dFVBB6GkbC8xhCChVzZXJ2ZXIuYXBwX2NvbmZpZy5nZXQSE0Fw'
    'cENvbmZpZ0dldFJlcXVlc3QaFEFwcENvbmZpZ0dldFJlc3BvbnNlEmIKHVJQQ19NRVRIT0RfU0'
    'VSVkVSX0ZSSUVORF9QSU5HEHsaP8LzGDsKEnNlcnZlci5mcmllbmQucGluZxIRRnJpZW5kUGlu'
    'Z1JlcXVlc3QaEkZyaWVuZFBpbmdSZXNwb25zZRJ4CiNSUENfTUVUSE9EX1NFUlZFUl9GUklFTk'
    'RfR1JPVVBfUElORxB8Gk/C8xhLChhzZXJ2ZXIuZnJpZW5kX2dyb3VwLnBpbmcSFkZyaWVuZEdy'
    'b3VwUGluZ1JlcXVlc3QaF0ZyaWVuZEdyb3VwUGluZ1Jlc3BvbnNlEmIKHVJQQ19NRVRIT0RfU0'
    'VSVkVSX1BST0ZJTEVfR0VUEH0aP8LzGDsKEnNlcnZlci5wcm9maWxlLmdldBIRUHJvZmlsZUdl'
    'dFJlcXVlc3QaElByb2ZpbGVHZXRSZXNwb25zZQ==');

@$core.Deprecated('Use rpcResponseDescriptor instead')
const RpcResponse$json = {
  '1': 'RpcResponse',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'payload', '3': 2, '4': 1, '5': 12, '9': 0, '10': 'payload'},
    {
      '1': 'status',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.RpcStatus',
      '9': 0,
      '10': 'status'
    },
  ],
  '8': [
    {'1': 'body'},
  ],
};

/// Descriptor for `RpcResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List rpcResponseDescriptor = $convert.base64Decode(
    'CgtScGNSZXNwb25zZRIOCgJpZBgBIAEoCVICaWQSGgoHcGF5bG9hZBgCIAEoDEgAUgdwYXlsb2'
    'FkEjMKBnN0YXR1cxgDIAEoCzIZLmdpemNsYXcucnBjLnYxLlJwY1N0YXR1c0gAUgZzdGF0dXNC'
    'BgoEYm9keQ==');

@$core.Deprecated('Use rpcStreamFrameDescriptor instead')
const RpcStreamFrame$json = {
  '1': 'RpcStreamFrame',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'payload', '3': 2, '4': 1, '5': 12, '9': 0, '10': 'payload'},
    {
      '1': 'status',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.RpcStatus',
      '9': 0,
      '10': 'status'
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
    'lsb2FkEjMKBnN0YXR1cxgDIAEoCzIZLmdpemNsYXcucnBjLnYxLlJwY1N0YXR1c0gAUgZzdGF0'
    'dXMSMAoDZW5kGAQgASgLMhwuZ2l6Y2xhdy5ycGMudjEuUnBjU3RyZWFtRW5kSABSA2VuZEIGCg'
    'Rib2R5');

@$core.Deprecated('Use rpcStatusDescriptor instead')
const RpcStatus$json = {
  '1': 'RpcStatus',
  '2': [
    {
      '1': 'code',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.StatusCode',
      '10': 'code'
    },
    {'1': 'message', '3': 2, '4': 1, '5': 9, '10': 'message'},
    {
      '1': 'info',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ErrorInfo',
      '10': 'info'
    },
  ],
};

/// Descriptor for `RpcStatus`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List rpcStatusDescriptor = $convert.base64Decode(
    'CglScGNTdGF0dXMSLgoEY29kZRgBIAEoDjIaLmdpemNsYXcucnBjLnYxLlN0YXR1c0NvZGVSBG'
    'NvZGUSGAoHbWVzc2FnZRgCIAEoCVIHbWVzc2FnZRItCgRpbmZvGAMgASgLMhkuZ2l6Y2xhdy5y'
    'cGMudjEuRXJyb3JJbmZvUgRpbmZv');

@$core.Deprecated('Use errorInfoDescriptor instead')
const ErrorInfo$json = {
  '1': 'ErrorInfo',
  '2': [
    {'1': 'reason', '3': 1, '4': 1, '5': 9, '10': 'reason'},
    {'1': 'domain', '3': 2, '4': 1, '5': 9, '10': 'domain'},
  ],
};

/// Descriptor for `ErrorInfo`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List errorInfoDescriptor = $convert.base64Decode(
    'CglFcnJvckluZm8SFgoGcmVhc29uGAEgASgJUgZyZWFzb24SFgoGZG9tYWluGAIgASgJUgZkb2'
    '1haW4=');

@$core.Deprecated('Use rpcStreamEndDescriptor instead')
const RpcStreamEnd$json = {
  '1': 'RpcStreamEnd',
};

/// Descriptor for `RpcStreamEnd`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List rpcStreamEndDescriptor =
    $convert.base64Decode('CgxScGNTdHJlYW1FbmQ=');

@$core.Deprecated('Use rpcMethodOptionsDescriptor instead')
const RpcMethodOptions$json = {
  '1': 'RpcMethodOptions',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
    {'1': 'request', '3': 2, '4': 1, '5': 9, '10': 'request'},
    {'1': 'response', '3': 3, '4': 1, '5': 9, '10': 'response'},
  ],
};

/// Descriptor for `RpcMethodOptions`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List rpcMethodOptionsDescriptor = $convert.base64Decode(
    'ChBScGNNZXRob2RPcHRpb25zEhIKBG5hbWUYASABKAlSBG5hbWUSGAoHcmVxdWVzdBgCIAEoCV'
    'IHcmVxdWVzdBIaCghyZXNwb25zZRgDIAEoCVIIcmVzcG9uc2U=');

@$core.Deprecated('Use rpcRequestDescriptor instead')
const RpcRequest$json = {
  '1': 'RpcRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'method',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.RpcMethod',
      '10': 'method'
    },
    {
      '1': 'payload',
      '3': 3,
      '4': 1,
      '5': 12,
      '9': 0,
      '10': 'payload',
      '17': true
    },
  ],
  '8': [
    {'1': '_payload'},
  ],
};

/// Descriptor for `RpcRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List rpcRequestDescriptor = $convert.base64Decode(
    'CgpScGNSZXF1ZXN0Eg4KAmlkGAEgASgJUgJpZBIxCgZtZXRob2QYAiABKA4yGS5naXpjbGF3Ln'
    'JwYy52MS5ScGNNZXRob2RSBm1ldGhvZBIdCgdwYXlsb2FkGAMgASgMSABSB3BheWxvYWSIAQFC'
    'CgoIX3BheWxvYWQ=');
