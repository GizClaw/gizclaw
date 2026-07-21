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

@$core.Deprecated('Use firmwareDescriptor instead')
const Firmware$json = {
  '1': 'Firmware',
  '2': [
    {'1': 'created_at', '3': 1, '4': 1, '5': 9, '10': 'createdAt'},
    {
      '1': 'description',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'description',
      '17': true
    },
    {'1': 'name', '3': 3, '4': 1, '5': 9, '10': 'name'},
    {
      '1': 'slots',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FirmwareSlots',
      '10': 'slots'
    },
    {'1': 'updated_at', '3': 5, '4': 1, '5': 9, '10': 'updatedAt'},
  ],
  '8': [
    {'1': '_description'},
  ],
};

/// Descriptor for `Firmware`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List firmwareDescriptor = $convert.base64Decode(
    'CghGaXJtd2FyZRIdCgpjcmVhdGVkX2F0GAEgASgJUgljcmVhdGVkQXQSJQoLZGVzY3JpcHRpb2'
    '4YAiABKAlIAFILZGVzY3JpcHRpb26IAQESEgoEbmFtZRgDIAEoCVIEbmFtZRIzCgVzbG90cxgE'
    'IAEoCzIdLmdpemNsYXcucnBjLnYxLkZpcm13YXJlU2xvdHNSBXNsb3RzEh0KCnVwZGF0ZWRfYX'
    'QYBSABKAlSCXVwZGF0ZWRBdEIOCgxfZGVzY3JpcHRpb24=');

@$core.Deprecated('Use firmwareArtifactDescriptor instead')
const FirmwareArtifact$json = {
  '1': 'FirmwareArtifact',
  '2': [
    {'1': 'content_type', '3': 1, '4': 1, '5': 9, '10': 'contentType'},
    {'1': 'files_path', '3': 2, '4': 1, '5': 9, '10': 'filesPath'},
    {'1': 'manifest_path', '3': 3, '4': 1, '5': 9, '10': 'manifestPath'},
    {'1': 'sha256', '3': 4, '4': 1, '5': 9, '10': 'sha256'},
    {'1': 'size', '3': 5, '4': 1, '5': 3, '10': 'size'},
    {'1': 'tar_path', '3': 6, '4': 1, '5': 9, '10': 'tarPath'},
    {'1': 'uploaded_at', '3': 7, '4': 1, '5': 9, '10': 'uploadedAt'},
  ],
};

/// Descriptor for `FirmwareArtifact`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List firmwareArtifactDescriptor = $convert.base64Decode(
    'ChBGaXJtd2FyZUFydGlmYWN0EiEKDGNvbnRlbnRfdHlwZRgBIAEoCVILY29udGVudFR5cGUSHQ'
    'oKZmlsZXNfcGF0aBgCIAEoCVIJZmlsZXNQYXRoEiMKDW1hbmlmZXN0X3BhdGgYAyABKAlSDG1h'
    'bmlmZXN0UGF0aBIWCgZzaGEyNTYYBCABKAlSBnNoYTI1NhISCgRzaXplGAUgASgDUgRzaXplEh'
    'kKCHRhcl9wYXRoGAYgASgJUgd0YXJQYXRoEh8KC3VwbG9hZGVkX2F0GAcgASgJUgp1cGxvYWRl'
    'ZEF0');

@$core.Deprecated('Use firmwareArtifactEntryDescriptor instead')
const FirmwareArtifactEntry$json = {
  '1': 'FirmwareArtifactEntry',
  '2': [
    {
      '1': 'content_type',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'contentType',
      '17': true
    },
    {'1': 'mod_time', '3': 2, '4': 1, '5': 9, '10': 'modTime'},
    {'1': 'mode', '3': 3, '4': 1, '5': 5, '10': 'mode'},
    {'1': 'path', '3': 4, '4': 1, '5': 9, '10': 'path'},
    {'1': 'size', '3': 5, '4': 1, '5': 3, '10': 'size'},
    {
      '1': 'type',
      '3': 6,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.FirmwareArtifactEntryType',
      '10': 'type'
    },
  ],
  '8': [
    {'1': '_content_type'},
  ],
};

/// Descriptor for `FirmwareArtifactEntry`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List firmwareArtifactEntryDescriptor = $convert.base64Decode(
    'ChVGaXJtd2FyZUFydGlmYWN0RW50cnkSJgoMY29udGVudF90eXBlGAEgASgJSABSC2NvbnRlbn'
    'RUeXBliAEBEhkKCG1vZF90aW1lGAIgASgJUgdtb2RUaW1lEhIKBG1vZGUYAyABKAVSBG1vZGUS'
    'EgoEcGF0aBgEIAEoCVIEcGF0aBISCgRzaXplGAUgASgDUgRzaXplEj0KBHR5cGUYBiABKA4yKS'
    '5naXpjbGF3LnJwYy52MS5GaXJtd2FyZUFydGlmYWN0RW50cnlUeXBlUgR0eXBlQg8KDV9jb250'
    'ZW50X3R5cGU=');

@$core.Deprecated('Use firmwareFilesDownloadRequestDescriptor instead')
const FirmwareFilesDownloadRequest$json = {
  '1': 'FirmwareFilesDownloadRequest',
  '2': [
    {
      '1': 'channel',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.FirmwareChannelName',
      '10': 'channel'
    },
    {'1': 'path', '3': 2, '4': 1, '5': 9, '10': 'path'},
  ],
};

/// Descriptor for `FirmwareFilesDownloadRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List firmwareFilesDownloadRequestDescriptor =
    $convert.base64Decode(
        'ChxGaXJtd2FyZUZpbGVzRG93bmxvYWRSZXF1ZXN0Ej0KB2NoYW5uZWwYASABKA4yIy5naXpjbG'
        'F3LnJwYy52MS5GaXJtd2FyZUNoYW5uZWxOYW1lUgdjaGFubmVsEhIKBHBhdGgYAiABKAlSBHBh'
        'dGg=');

@$core.Deprecated('Use firmwareFilesDownloadResponseDescriptor instead')
const FirmwareFilesDownloadResponse$json = {
  '1': 'FirmwareFilesDownloadResponse',
  '2': [
    {
      '1': 'artifact',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FirmwareArtifact',
      '10': 'artifact'
    },
    {
      '1': 'channel',
      '3': 2,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.FirmwareChannelName',
      '10': 'channel'
    },
    {
      '1': 'file',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FirmwareArtifactEntry',
      '10': 'file'
    },
    {'1': 'firmware_id', '3': 4, '4': 1, '5': 9, '10': 'firmwareId'},
    {'1': 'path', '3': 5, '4': 1, '5': 9, '10': 'path'},
  ],
};

/// Descriptor for `FirmwareFilesDownloadResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List firmwareFilesDownloadResponseDescriptor = $convert.base64Decode(
    'Ch1GaXJtd2FyZUZpbGVzRG93bmxvYWRSZXNwb25zZRI8CghhcnRpZmFjdBgBIAEoCzIgLmdpem'
    'NsYXcucnBjLnYxLkZpcm13YXJlQXJ0aWZhY3RSCGFydGlmYWN0Ej0KB2NoYW5uZWwYAiABKA4y'
    'Iy5naXpjbGF3LnJwYy52MS5GaXJtd2FyZUNoYW5uZWxOYW1lUgdjaGFubmVsEjkKBGZpbGUYAy'
    'ABKAsyJS5naXpjbGF3LnJwYy52MS5GaXJtd2FyZUFydGlmYWN0RW50cnlSBGZpbGUSHwoLZmly'
    'bXdhcmVfaWQYBCABKAlSCmZpcm13YXJlSWQSEgoEcGF0aBgFIAEoCVIEcGF0aA==');

@$core.Deprecated('Use firmwareGetRequestDescriptor instead')
const FirmwareGetRequest$json = {
  '1': 'FirmwareGetRequest',
};

/// Descriptor for `FirmwareGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List firmwareGetRequestDescriptor =
    $convert.base64Decode('ChJGaXJtd2FyZUdldFJlcXVlc3Q=');

@$core.Deprecated('Use firmwareGetResponseDescriptor instead')
const FirmwareGetResponse$json = {
  '1': 'FirmwareGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.Firmware',
      '10': 'value'
    },
  ],
};

/// Descriptor for `FirmwareGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List firmwareGetResponseDescriptor = $convert.base64Decode(
    'ChNGaXJtd2FyZUdldFJlc3BvbnNlEi4KBXZhbHVlGAEgASgLMhguZ2l6Y2xhdy5ycGMudjEuRm'
    'lybXdhcmVSBXZhbHVl');

@$core.Deprecated('Use firmwareSlotDescriptor instead')
const FirmwareSlot$json = {
  '1': 'FirmwareSlot',
  '2': [
    {
      '1': 'artifact',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FirmwareArtifact',
      '9': 0,
      '10': 'artifact',
      '17': true
    },
    {
      '1': 'description',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'description',
      '17': true
    },
  ],
  '8': [
    {'1': '_artifact'},
    {'1': '_description'},
  ],
};

/// Descriptor for `FirmwareSlot`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List firmwareSlotDescriptor = $convert.base64Decode(
    'CgxGaXJtd2FyZVNsb3QSQQoIYXJ0aWZhY3QYASABKAsyIC5naXpjbGF3LnJwYy52MS5GaXJtd2'
    'FyZUFydGlmYWN0SABSCGFydGlmYWN0iAEBEiUKC2Rlc2NyaXB0aW9uGAIgASgJSAFSC2Rlc2Ny'
    'aXB0aW9uiAEBQgsKCV9hcnRpZmFjdEIOCgxfZGVzY3JpcHRpb24=');

@$core.Deprecated('Use firmwareSlotsDescriptor instead')
const FirmwareSlots$json = {
  '1': 'FirmwareSlots',
  '2': [
    {
      '1': 'beta',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FirmwareSlot',
      '10': 'beta'
    },
    {
      '1': 'develop',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FirmwareSlot',
      '10': 'develop'
    },
    {
      '1': 'pending',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FirmwareSlot',
      '10': 'pending'
    },
    {
      '1': 'stable',
      '3': 4,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FirmwareSlot',
      '10': 'stable'
    },
  ],
};

/// Descriptor for `FirmwareSlots`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List firmwareSlotsDescriptor = $convert.base64Decode(
    'Cg1GaXJtd2FyZVNsb3RzEjAKBGJldGEYASABKAsyHC5naXpjbGF3LnJwYy52MS5GaXJtd2FyZV'
    'Nsb3RSBGJldGESNgoHZGV2ZWxvcBgCIAEoCzIcLmdpemNsYXcucnBjLnYxLkZpcm13YXJlU2xv'
    'dFIHZGV2ZWxvcBI2CgdwZW5kaW5nGAMgASgLMhwuZ2l6Y2xhdy5ycGMudjEuRmlybXdhcmVTbG'
    '90UgdwZW5kaW5nEjQKBnN0YWJsZRgEIAEoCzIcLmdpemNsYXcucnBjLnYxLkZpcm13YXJlU2xv'
    'dFIGc3RhYmxl');
