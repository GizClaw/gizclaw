// This is a generated file - do not edit.
//
// Generated from payload/firmware.proto.

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

@$core.Deprecated('Use firmwareGetRequestDescriptor instead')
const FirmwareGetRequest$json = {
  '1': 'FirmwareGetRequest',
  '2': [
    {
      '1': 'channel',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.FirmwareChannelName',
      '10': 'channel'
    },
  ],
};

/// Descriptor for `FirmwareGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List firmwareGetRequestDescriptor = $convert.base64Decode(
    'ChJGaXJtd2FyZUdldFJlcXVlc3QSPQoHY2hhbm5lbBgBIAEoDjIjLmdpemNsYXcucnBjLnYxLk'
    'Zpcm13YXJlQ2hhbm5lbE5hbWVSB2NoYW5uZWw=');

@$core.Deprecated('Use firmwareGetResponseDescriptor instead')
const FirmwareGetResponse$json = {
  '1': 'FirmwareGetResponse',
  '2': [
    {'1': 'firmware_name', '3': 1, '4': 1, '5': 9, '10': 'firmwareName'},
    {
      '1': 'channel',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.FirmwareChannelName',
      '10': 'channel'
    },
    {
      '1': 'description',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'description',
      '17': true
    },
    {'1': 'url', '3': 4, '4': 1, '5': 9, '10': 'url'},
    {'1': 'sha256', '3': 5, '4': 1, '5': 9, '10': 'sha256'},
    {'1': 'size', '3': 6, '4': 1, '5': 3, '10': 'size'},
  ],
  '8': [
    {'1': '_description'},
  ],
};

/// Descriptor for `FirmwareGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List firmwareGetResponseDescriptor = $convert.base64Decode(
    'ChNGaXJtd2FyZUdldFJlc3BvbnNlEiMKDWZpcm13YXJlX25hbWUYASABKAlSDGZpcm13YXJlTm'
    'FtZRI9CgdjaGFubmVsGAIgASgOMiMuZ2l6Y2xhdy5ycGMudjEuRmlybXdhcmVDaGFubmVsTmFt'
    'ZVIHY2hhbm5lbBIlCgtkZXNjcmlwdGlvbhgDIAEoCUgAUgtkZXNjcmlwdGlvbogBARIQCgN1cm'
    'wYBCABKAlSA3VybBIWCgZzaGEyNTYYBSABKAlSBnNoYTI1NhISCgRzaXplGAYgASgDUgRzaXpl'
    'Qg4KDF9kZXNjcmlwdGlvbg==');
