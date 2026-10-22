// This is a generated file - do not edit.
//
// Generated from payload/management.proto.

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

@$core.Deprecated('Use contentAgeOptionDescriptor instead')
const ContentAgeOption$json = {
  '1': 'ContentAgeOption',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'display_name', '3': 2, '4': 1, '5': 9, '10': 'displayName'},
    {
      '1': 'min_age_years',
      '3': 3,
      '4': 1,
      '5': 13,
      '9': 0,
      '10': 'minAgeYears',
      '17': true
    },
    {
      '1': 'max_age_years',
      '3': 4,
      '4': 1,
      '5': 13,
      '9': 1,
      '10': 'maxAgeYears',
      '17': true
    },
  ],
  '8': [
    {'1': '_min_age_years'},
    {'1': '_max_age_years'},
  ],
};

/// Descriptor for `ContentAgeOption`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contentAgeOptionDescriptor = $convert.base64Decode(
    'ChBDb250ZW50QWdlT3B0aW9uEg4KAmlkGAEgASgJUgJpZBIhCgxkaXNwbGF5X25hbWUYAiABKA'
    'lSC2Rpc3BsYXlOYW1lEicKDW1pbl9hZ2VfeWVhcnMYAyABKA1IAFILbWluQWdlWWVhcnOIAQES'
    'JwoNbWF4X2FnZV95ZWFycxgEIAEoDUgBUgttYXhBZ2VZZWFyc4gBAUIQCg5fbWluX2FnZV95ZW'
    'Fyc0IQCg5fbWF4X2FnZV95ZWFycw==');

@$core.Deprecated('Use contentAgeStateDescriptor instead')
const ContentAgeState$json = {
  '1': 'ContentAgeState',
  '2': [
    {'1': 'option_id', '3': 1, '4': 1, '5': 9, '10': 'optionId'},
    {
      '1': 'options',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ContentAgeOption',
      '10': 'options'
    },
  ],
};

/// Descriptor for `ContentAgeState`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contentAgeStateDescriptor = $convert.base64Decode(
    'Cg9Db250ZW50QWdlU3RhdGUSGwoJb3B0aW9uX2lkGAEgASgJUghvcHRpb25JZBI6CgdvcHRpb2'
    '5zGAIgAygLMiAuZ2l6Y2xhdy5ycGMudjEuQ29udGVudEFnZU9wdGlvblIHb3B0aW9ucw==');

@$core.Deprecated('Use clientContentAgeGetRequestDescriptor instead')
const ClientContentAgeGetRequest$json = {
  '1': 'ClientContentAgeGetRequest',
};

/// Descriptor for `ClientContentAgeGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientContentAgeGetRequestDescriptor =
    $convert.base64Decode('ChpDbGllbnRDb250ZW50QWdlR2V0UmVxdWVzdA==');

@$core.Deprecated('Use clientContentAgeGetResponseDescriptor instead')
const ClientContentAgeGetResponse$json = {
  '1': 'ClientContentAgeGetResponse',
  '2': [
    {
      '1': 'state',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ContentAgeState',
      '10': 'state'
    },
  ],
};

/// Descriptor for `ClientContentAgeGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientContentAgeGetResponseDescriptor =
    $convert.base64Decode(
        'ChtDbGllbnRDb250ZW50QWdlR2V0UmVzcG9uc2USNQoFc3RhdGUYASABKAsyHy5naXpjbGF3Ln'
        'JwYy52MS5Db250ZW50QWdlU3RhdGVSBXN0YXRl');

@$core.Deprecated('Use clientContentAgeSetRequestDescriptor instead')
const ClientContentAgeSetRequest$json = {
  '1': 'ClientContentAgeSetRequest',
  '2': [
    {'1': 'option_id', '3': 1, '4': 1, '5': 9, '10': 'optionId'},
  ],
};

/// Descriptor for `ClientContentAgeSetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientContentAgeSetRequestDescriptor =
    $convert.base64Decode(
        'ChpDbGllbnRDb250ZW50QWdlU2V0UmVxdWVzdBIbCglvcHRpb25faWQYASABKAlSCG9wdGlvbk'
        'lk');

@$core.Deprecated('Use clientContentAgeSetResponseDescriptor instead')
const ClientContentAgeSetResponse$json = {
  '1': 'ClientContentAgeSetResponse',
  '2': [
    {
      '1': 'applied',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ContentAgeState',
      '10': 'applied'
    },
  ],
};

/// Descriptor for `ClientContentAgeSetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientContentAgeSetResponseDescriptor =
    $convert.base64Decode(
        'ChtDbGllbnRDb250ZW50QWdlU2V0UmVzcG9uc2USOQoHYXBwbGllZBgBIAEoCzIfLmdpemNsYX'
        'cucnBjLnYxLkNvbnRlbnRBZ2VTdGF0ZVIHYXBwbGllZA==');

@$core.Deprecated('Use deviceFeatureDescriptor instead')
const DeviceFeature$json = {
  '1': 'DeviceFeature',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'display_name', '3': 2, '4': 1, '5': 9, '10': 'displayName'},
    {
      '1': 'enabled',
      '3': 3,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'enabled',
      '17': true
    },
    {
      '1': 'writable',
      '3': 4,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'writable',
      '17': true
    },
  ],
  '8': [
    {'1': '_enabled'},
    {'1': '_writable'},
  ],
};

/// Descriptor for `DeviceFeature`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List deviceFeatureDescriptor = $convert.base64Decode(
    'Cg1EZXZpY2VGZWF0dXJlEg4KAmlkGAEgASgJUgJpZBIhCgxkaXNwbGF5X25hbWUYAiABKAlSC2'
    'Rpc3BsYXlOYW1lEh0KB2VuYWJsZWQYAyABKAhIAFIHZW5hYmxlZIgBARIfCgh3cml0YWJsZRgE'
    'IAEoCEgBUgh3cml0YWJsZYgBAUIKCghfZW5hYmxlZEILCglfd3JpdGFibGU=');

@$core.Deprecated('Use clientDeviceFeaturesGetRequestDescriptor instead')
const ClientDeviceFeaturesGetRequest$json = {
  '1': 'ClientDeviceFeaturesGetRequest',
};

/// Descriptor for `ClientDeviceFeaturesGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceFeaturesGetRequestDescriptor =
    $convert.base64Decode('Ch5DbGllbnREZXZpY2VGZWF0dXJlc0dldFJlcXVlc3Q=');

@$core.Deprecated('Use clientDeviceFeaturesGetResponseDescriptor instead')
const ClientDeviceFeaturesGetResponse$json = {
  '1': 'ClientDeviceFeaturesGetResponse',
  '2': [
    {
      '1': 'features',
      '3': 1,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DeviceFeature',
      '10': 'features'
    },
  ],
};

/// Descriptor for `ClientDeviceFeaturesGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceFeaturesGetResponseDescriptor =
    $convert.base64Decode(
        'Ch9DbGllbnREZXZpY2VGZWF0dXJlc0dldFJlc3BvbnNlEjkKCGZlYXR1cmVzGAEgAygLMh0uZ2'
        'l6Y2xhdy5ycGMudjEuRGV2aWNlRmVhdHVyZVIIZmVhdHVyZXM=');

@$core.Deprecated('Use clientDeviceFeatureSetRequestDescriptor instead')
const ClientDeviceFeatureSetRequest$json = {
  '1': 'ClientDeviceFeatureSetRequest',
  '2': [
    {'1': 'feature_id', '3': 1, '4': 1, '5': 9, '10': 'featureId'},
    {
      '1': 'enabled',
      '3': 2,
      '4': 1,
      '5': 8,
      '9': 0,
      '10': 'enabled',
      '17': true
    },
  ],
  '8': [
    {'1': '_enabled'},
  ],
};

/// Descriptor for `ClientDeviceFeatureSetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceFeatureSetRequestDescriptor =
    $convert.base64Decode(
        'Ch1DbGllbnREZXZpY2VGZWF0dXJlU2V0UmVxdWVzdBIdCgpmZWF0dXJlX2lkGAEgASgJUglmZW'
        'F0dXJlSWQSHQoHZW5hYmxlZBgCIAEoCEgAUgdlbmFibGVkiAEBQgoKCF9lbmFibGVk');

@$core.Deprecated('Use clientDeviceFeatureSetResponseDescriptor instead')
const ClientDeviceFeatureSetResponse$json = {
  '1': 'ClientDeviceFeatureSetResponse',
  '2': [
    {
      '1': 'applied',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.DeviceFeature',
      '10': 'applied'
    },
  ],
};

/// Descriptor for `ClientDeviceFeatureSetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientDeviceFeatureSetResponseDescriptor =
    $convert.base64Decode(
        'Ch5DbGllbnREZXZpY2VGZWF0dXJlU2V0UmVzcG9uc2USNwoHYXBwbGllZBgBIAEoCzIdLmdpem'
        'NsYXcucnBjLnYxLkRldmljZUZlYXR1cmVSB2FwcGxpZWQ=');

@$core.Deprecated('Use usageTimeStateDescriptor instead')
const UsageTimeState$json = {
  '1': 'UsageTimeState',
  '2': [
    {
      '1': 'limit_seconds',
      '3': 1,
      '4': 1,
      '5': 13,
      '9': 0,
      '10': 'limitSeconds',
      '17': true
    },
    {
      '1': 'used_seconds',
      '3': 2,
      '4': 1,
      '5': 13,
      '9': 1,
      '10': 'usedSeconds',
      '17': true
    },
    {
      '1': 'exhausted',
      '3': 3,
      '4': 1,
      '5': 8,
      '9': 2,
      '10': 'exhausted',
      '17': true
    },
  ],
  '8': [
    {'1': '_limit_seconds'},
    {'1': '_used_seconds'},
    {'1': '_exhausted'},
  ],
};

/// Descriptor for `UsageTimeState`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List usageTimeStateDescriptor = $convert.base64Decode(
    'Cg5Vc2FnZVRpbWVTdGF0ZRIoCg1saW1pdF9zZWNvbmRzGAEgASgNSABSDGxpbWl0U2Vjb25kc4'
    'gBARImCgx1c2VkX3NlY29uZHMYAiABKA1IAVILdXNlZFNlY29uZHOIAQESIQoJZXhoYXVzdGVk'
    'GAMgASgISAJSCWV4aGF1c3RlZIgBAUIQCg5fbGltaXRfc2Vjb25kc0IPCg1fdXNlZF9zZWNvbm'
    'RzQgwKCl9leGhhdXN0ZWQ=');

@$core.Deprecated('Use clientUsageTimeGetRequestDescriptor instead')
const ClientUsageTimeGetRequest$json = {
  '1': 'ClientUsageTimeGetRequest',
};

/// Descriptor for `ClientUsageTimeGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientUsageTimeGetRequestDescriptor =
    $convert.base64Decode('ChlDbGllbnRVc2FnZVRpbWVHZXRSZXF1ZXN0');

@$core.Deprecated('Use clientUsageTimeGetResponseDescriptor instead')
const ClientUsageTimeGetResponse$json = {
  '1': 'ClientUsageTimeGetResponse',
  '2': [
    {
      '1': 'state',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.UsageTimeState',
      '10': 'state'
    },
  ],
};

/// Descriptor for `ClientUsageTimeGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientUsageTimeGetResponseDescriptor =
    $convert.base64Decode(
        'ChpDbGllbnRVc2FnZVRpbWVHZXRSZXNwb25zZRI0CgVzdGF0ZRgBIAEoCzIeLmdpemNsYXcucn'
        'BjLnYxLlVzYWdlVGltZVN0YXRlUgVzdGF0ZQ==');

@$core.Deprecated('Use clientUsageTimeSetRequestDescriptor instead')
const ClientUsageTimeSetRequest$json = {
  '1': 'ClientUsageTimeSetRequest',
  '2': [
    {
      '1': 'limit_seconds',
      '3': 1,
      '4': 1,
      '5': 13,
      '9': 0,
      '10': 'limitSeconds',
      '17': true
    },
  ],
  '8': [
    {'1': '_limit_seconds'},
  ],
};

/// Descriptor for `ClientUsageTimeSetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientUsageTimeSetRequestDescriptor =
    $convert.base64Decode(
        'ChlDbGllbnRVc2FnZVRpbWVTZXRSZXF1ZXN0EigKDWxpbWl0X3NlY29uZHMYASABKA1IAFIMbG'
        'ltaXRTZWNvbmRziAEBQhAKDl9saW1pdF9zZWNvbmRz');

@$core.Deprecated('Use clientUsageTimeSetResponseDescriptor instead')
const ClientUsageTimeSetResponse$json = {
  '1': 'ClientUsageTimeSetResponse',
  '2': [
    {
      '1': 'applied',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.UsageTimeState',
      '10': 'applied'
    },
  ],
};

/// Descriptor for `ClientUsageTimeSetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientUsageTimeSetResponseDescriptor =
    $convert.base64Decode(
        'ChpDbGllbnRVc2FnZVRpbWVTZXRSZXNwb25zZRI4CgdhcHBsaWVkGAEgASgLMh4uZ2l6Y2xhdy'
        '5ycGMudjEuVXNhZ2VUaW1lU3RhdGVSB2FwcGxpZWQ=');

@$core.Deprecated('Use aISafetyOptionDescriptor instead')
const AISafetyOption$json = {
  '1': 'AISafetyOption',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'display_name', '3': 2, '4': 1, '5': 9, '10': 'displayName'},
  ],
};

/// Descriptor for `AISafetyOption`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aISafetyOptionDescriptor = $convert.base64Decode(
    'Cg5BSVNhZmV0eU9wdGlvbhIOCgJpZBgBIAEoCVICaWQSIQoMZGlzcGxheV9uYW1lGAIgASgJUg'
    'tkaXNwbGF5TmFtZQ==');

@$core.Deprecated('Use aISafetyStateDescriptor instead')
const AISafetyState$json = {
  '1': 'AISafetyState',
  '2': [
    {'1': 'level', '3': 1, '4': 1, '5': 9, '10': 'level'},
    {
      '1': 'options',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AISafetyOption',
      '10': 'options'
    },
  ],
};

/// Descriptor for `AISafetyState`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List aISafetyStateDescriptor = $convert.base64Decode(
    'Cg1BSVNhZmV0eVN0YXRlEhQKBWxldmVsGAEgASgJUgVsZXZlbBI4CgdvcHRpb25zGAIgAygLMh'
    '4uZ2l6Y2xhdy5ycGMudjEuQUlTYWZldHlPcHRpb25SB29wdGlvbnM=');

@$core.Deprecated('Use clientAISafetyGetRequestDescriptor instead')
const ClientAISafetyGetRequest$json = {
  '1': 'ClientAISafetyGetRequest',
};

/// Descriptor for `ClientAISafetyGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAISafetyGetRequestDescriptor =
    $convert.base64Decode('ChhDbGllbnRBSVNhZmV0eUdldFJlcXVlc3Q=');

@$core.Deprecated('Use clientAISafetyGetResponseDescriptor instead')
const ClientAISafetyGetResponse$json = {
  '1': 'ClientAISafetyGetResponse',
  '2': [
    {
      '1': 'state',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AISafetyState',
      '10': 'state'
    },
  ],
};

/// Descriptor for `ClientAISafetyGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAISafetyGetResponseDescriptor =
    $convert.base64Decode(
        'ChlDbGllbnRBSVNhZmV0eUdldFJlc3BvbnNlEjMKBXN0YXRlGAEgASgLMh0uZ2l6Y2xhdy5ycG'
        'MudjEuQUlTYWZldHlTdGF0ZVIFc3RhdGU=');

@$core.Deprecated('Use clientAISafetySetRequestDescriptor instead')
const ClientAISafetySetRequest$json = {
  '1': 'ClientAISafetySetRequest',
  '2': [
    {'1': 'level', '3': 1, '4': 1, '5': 9, '10': 'level'},
  ],
};

/// Descriptor for `ClientAISafetySetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAISafetySetRequestDescriptor =
    $convert.base64Decode(
        'ChhDbGllbnRBSVNhZmV0eVNldFJlcXVlc3QSFAoFbGV2ZWwYASABKAlSBWxldmVs');

@$core.Deprecated('Use clientAISafetySetResponseDescriptor instead')
const ClientAISafetySetResponse$json = {
  '1': 'ClientAISafetySetResponse',
  '2': [
    {
      '1': 'applied',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.AISafetyState',
      '10': 'applied'
    },
  ],
};

/// Descriptor for `ClientAISafetySetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientAISafetySetResponseDescriptor =
    $convert.base64Decode(
        'ChlDbGllbnRBSVNhZmV0eVNldFJlc3BvbnNlEjcKB2FwcGxpZWQYASABKAsyHS5naXpjbGF3Ln'
        'JwYy52MS5BSVNhZmV0eVN0YXRlUgdhcHBsaWVk');

@$core.Deprecated('Use clientManagementPasswordGetRequestDescriptor instead')
const ClientManagementPasswordGetRequest$json = {
  '1': 'ClientManagementPasswordGetRequest',
};

/// Descriptor for `ClientManagementPasswordGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientManagementPasswordGetRequestDescriptor =
    $convert.base64Decode('CiJDbGllbnRNYW5hZ2VtZW50UGFzc3dvcmRHZXRSZXF1ZXN0');

@$core.Deprecated('Use clientManagementPasswordGetResponseDescriptor instead')
const ClientManagementPasswordGetResponse$json = {
  '1': 'ClientManagementPasswordGetResponse',
  '2': [
    {'1': 'is_set', '3': 1, '4': 1, '5': 8, '9': 0, '10': 'isSet', '17': true},
  ],
  '8': [
    {'1': '_is_set'},
  ],
};

/// Descriptor for `ClientManagementPasswordGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientManagementPasswordGetResponseDescriptor =
    $convert.base64Decode(
        'CiNDbGllbnRNYW5hZ2VtZW50UGFzc3dvcmRHZXRSZXNwb25zZRIaCgZpc19zZXQYASABKAhIAF'
        'IFaXNTZXSIAQFCCQoHX2lzX3NldA==');

@$core.Deprecated('Use clientManagementPasswordSetRequestDescriptor instead')
const ClientManagementPasswordSetRequest$json = {
  '1': 'ClientManagementPasswordSetRequest',
  '2': [
    {'1': 'password', '3': 1, '4': 1, '5': 9, '10': 'password'},
  ],
};

/// Descriptor for `ClientManagementPasswordSetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientManagementPasswordSetRequestDescriptor =
    $convert.base64Decode(
        'CiJDbGllbnRNYW5hZ2VtZW50UGFzc3dvcmRTZXRSZXF1ZXN0EhoKCHBhc3N3b3JkGAEgASgJUg'
        'hwYXNzd29yZA==');

@$core.Deprecated('Use clientManagementPasswordSetResponseDescriptor instead')
const ClientManagementPasswordSetResponse$json = {
  '1': 'ClientManagementPasswordSetResponse',
  '2': [
    {'1': 'is_set', '3': 1, '4': 1, '5': 8, '9': 0, '10': 'isSet', '17': true},
  ],
  '8': [
    {'1': '_is_set'},
  ],
};

/// Descriptor for `ClientManagementPasswordSetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientManagementPasswordSetResponseDescriptor =
    $convert.base64Decode(
        'CiNDbGllbnRNYW5hZ2VtZW50UGFzc3dvcmRTZXRSZXNwb25zZRIaCgZpc19zZXQYASABKAhIAF'
        'IFaXNTZXSIAQFCCQoHX2lzX3NldA==');

@$core.Deprecated('Use clientManagementPasswordResetRequestDescriptor instead')
const ClientManagementPasswordResetRequest$json = {
  '1': 'ClientManagementPasswordResetRequest',
};

/// Descriptor for `ClientManagementPasswordResetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientManagementPasswordResetRequestDescriptor =
    $convert
        .base64Decode('CiRDbGllbnRNYW5hZ2VtZW50UGFzc3dvcmRSZXNldFJlcXVlc3Q=');

@$core.Deprecated('Use clientManagementPasswordResetResponseDescriptor instead')
const ClientManagementPasswordResetResponse$json = {
  '1': 'ClientManagementPasswordResetResponse',
  '2': [
    {'1': 'is_set', '3': 1, '4': 1, '5': 8, '9': 0, '10': 'isSet', '17': true},
  ],
  '8': [
    {'1': '_is_set'},
  ],
};

/// Descriptor for `ClientManagementPasswordResetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientManagementPasswordResetResponseDescriptor =
    $convert.base64Decode(
        'CiVDbGllbnRNYW5hZ2VtZW50UGFzc3dvcmRSZXNldFJlc3BvbnNlEhoKBmlzX3NldBgBIAEoCE'
        'gAUgVpc1NldIgBAUIJCgdfaXNfc2V0');
