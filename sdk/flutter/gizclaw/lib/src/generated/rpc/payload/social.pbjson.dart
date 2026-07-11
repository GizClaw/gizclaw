// This is a generated file - do not edit.
//
// Generated from payload/social.proto.

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

@$core.Deprecated('Use contactCreateRequestDescriptor instead')
const ContactCreateRequest$json = {
  '1': 'ContactCreateRequest',
  '2': [
    {
      '1': 'display_name',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'displayName',
      '17': true
    },
    {
      '1': 'phone_number',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'phoneNumber',
      '17': true
    },
  ],
  '8': [
    {'1': '_display_name'},
    {'1': '_phone_number'},
  ],
};

/// Descriptor for `ContactCreateRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contactCreateRequestDescriptor = $convert.base64Decode(
    'ChRDb250YWN0Q3JlYXRlUmVxdWVzdBImCgxkaXNwbGF5X25hbWUYASABKAlIAFILZGlzcGxheU'
    '5hbWWIAQESJgoMcGhvbmVfbnVtYmVyGAIgASgJSAFSC3Bob25lTnVtYmVyiAEBQg8KDV9kaXNw'
    'bGF5X25hbWVCDwoNX3Bob25lX251bWJlcg==');

@$core.Deprecated('Use contactCreateResponseDescriptor instead')
const ContactCreateResponse$json = {
  '1': 'ContactCreateResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ContactObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ContactCreateResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contactCreateResponseDescriptor = $convert.base64Decode(
    'ChVDb250YWN0Q3JlYXRlUmVzcG9uc2USMwoFdmFsdWUYASABKAsyHS5naXpjbGF3LnJwYy52MS'
    '5Db250YWN0T2JqZWN0UgV2YWx1ZQ==');

@$core.Deprecated('Use contactDeleteRequestDescriptor instead')
const ContactDeleteRequest$json = {
  '1': 'ContactDeleteRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `ContactDeleteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contactDeleteRequestDescriptor = $convert
    .base64Decode('ChRDb250YWN0RGVsZXRlUmVxdWVzdBIOCgJpZBgBIAEoCVICaWQ=');

@$core.Deprecated('Use contactDeleteResponseDescriptor instead')
const ContactDeleteResponse$json = {
  '1': 'ContactDeleteResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ContactObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ContactDeleteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contactDeleteResponseDescriptor = $convert.base64Decode(
    'ChVDb250YWN0RGVsZXRlUmVzcG9uc2USMwoFdmFsdWUYASABKAsyHS5naXpjbGF3LnJwYy52MS'
    '5Db250YWN0T2JqZWN0UgV2YWx1ZQ==');

@$core.Deprecated('Use contactGetRequestDescriptor instead')
const ContactGetRequest$json = {
  '1': 'ContactGetRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `ContactGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contactGetRequestDescriptor =
    $convert.base64Decode('ChFDb250YWN0R2V0UmVxdWVzdBIOCgJpZBgBIAEoCVICaWQ=');

@$core.Deprecated('Use contactGetResponseDescriptor instead')
const ContactGetResponse$json = {
  '1': 'ContactGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ContactObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ContactGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contactGetResponseDescriptor = $convert.base64Decode(
    'ChJDb250YWN0R2V0UmVzcG9uc2USMwoFdmFsdWUYASABKAsyHS5naXpjbGF3LnJwYy52MS5Db2'
    '50YWN0T2JqZWN0UgV2YWx1ZQ==');

@$core.Deprecated('Use contactListRequestDescriptor instead')
const ContactListRequest$json = {
  '1': 'ContactListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `ContactListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contactListRequestDescriptor = $convert.base64Decode(
    'ChJDb250YWN0TGlzdFJlcXVlc3QSGwoGY3Vyc29yGAEgASgJSABSBmN1cnNvcogBARIZCgVsaW'
    '1pdBgCIAEoA0gBUgVsaW1pdIgBAUIJCgdfY3Vyc29yQggKBl9saW1pdA==');

@$core.Deprecated('Use contactListResponseDescriptor instead')
const ContactListResponse$json = {
  '1': 'ContactListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ContactObject',
      '10': 'items'
    },
    {
      '1': 'next_cursor',
      '3': 3,
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

/// Descriptor for `ContactListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contactListResponseDescriptor = $convert.base64Decode(
    'ChNDb250YWN0TGlzdFJlc3BvbnNlEhkKCGhhc19uZXh0GAEgASgIUgdoYXNOZXh0EjMKBWl0ZW'
    '1zGAIgAygLMh0uZ2l6Y2xhdy5ycGMudjEuQ29udGFjdE9iamVjdFIFaXRlbXMSJAoLbmV4dF9j'
    'dXJzb3IYAyABKAlIAFIKbmV4dEN1cnNvcogBAUIOCgxfbmV4dF9jdXJzb3I=');

@$core.Deprecated('Use contactObjectDescriptor instead')
const ContactObject$json = {
  '1': 'ContactObject',
  '2': [
    {
      '1': 'created_at',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'createdAt',
      '17': true
    },
    {
      '1': 'display_name',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'displayName',
      '17': true
    },
    {'1': 'id', '3': 3, '4': 1, '5': 9, '9': 2, '10': 'id', '17': true},
    {
      '1': 'phone_number',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'phoneNumber',
      '17': true
    },
    {
      '1': 'updated_at',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'updatedAt',
      '17': true
    },
  ],
  '8': [
    {'1': '_created_at'},
    {'1': '_display_name'},
    {'1': '_id'},
    {'1': '_phone_number'},
    {'1': '_updated_at'},
  ],
};

/// Descriptor for `ContactObject`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contactObjectDescriptor = $convert.base64Decode(
    'Cg1Db250YWN0T2JqZWN0EiIKCmNyZWF0ZWRfYXQYASABKAlIAFIJY3JlYXRlZEF0iAEBEiYKDG'
    'Rpc3BsYXlfbmFtZRgCIAEoCUgBUgtkaXNwbGF5TmFtZYgBARITCgJpZBgDIAEoCUgCUgJpZIgB'
    'ARImCgxwaG9uZV9udW1iZXIYBCABKAlIA1ILcGhvbmVOdW1iZXKIAQESIgoKdXBkYXRlZF9hdB'
    'gFIAEoCUgEUgl1cGRhdGVkQXSIAQFCDQoLX2NyZWF0ZWRfYXRCDwoNX2Rpc3BsYXlfbmFtZUIF'
    'CgNfaWRCDwoNX3Bob25lX251bWJlckINCgtfdXBkYXRlZF9hdA==');

@$core.Deprecated('Use contactPutRequestDescriptor instead')
const ContactPutRequest$json = {
  '1': 'ContactPutRequest',
  '2': [
    {
      '1': 'display_name',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'displayName',
      '17': true
    },
    {'1': 'id', '3': 2, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'phone_number',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'phoneNumber',
      '17': true
    },
  ],
  '8': [
    {'1': '_display_name'},
    {'1': '_phone_number'},
  ],
};

/// Descriptor for `ContactPutRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contactPutRequestDescriptor = $convert.base64Decode(
    'ChFDb250YWN0UHV0UmVxdWVzdBImCgxkaXNwbGF5X25hbWUYASABKAlIAFILZGlzcGxheU5hbW'
    'WIAQESDgoCaWQYAiABKAlSAmlkEiYKDHBob25lX251bWJlchgDIAEoCUgBUgtwaG9uZU51bWJl'
    'cogBAUIPCg1fZGlzcGxheV9uYW1lQg8KDV9waG9uZV9udW1iZXI=');

@$core.Deprecated('Use contactPutResponseDescriptor instead')
const ContactPutResponse$json = {
  '1': 'ContactPutResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.ContactObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `ContactPutResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List contactPutResponseDescriptor = $convert.base64Decode(
    'ChJDb250YWN0UHV0UmVzcG9uc2USMwoFdmFsdWUYASABKAsyHS5naXpjbGF3LnJwYy52MS5Db2'
    '50YWN0T2JqZWN0UgV2YWx1ZQ==');

@$core.Deprecated('Use friendAddRequestDescriptor instead')
const FriendAddRequest$json = {
  '1': 'FriendAddRequest',
  '2': [
    {'1': 'invite_token', '3': 1, '4': 1, '5': 9, '10': 'inviteToken'},
  ],
};

/// Descriptor for `FriendAddRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendAddRequestDescriptor = $convert.base64Decode(
    'ChBGcmllbmRBZGRSZXF1ZXN0EiEKDGludml0ZV90b2tlbhgBIAEoCVILaW52aXRlVG9rZW4=');

@$core.Deprecated('Use friendAddResponseDescriptor instead')
const FriendAddResponse$json = {
  '1': 'FriendAddResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `FriendAddResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendAddResponseDescriptor = $convert.base64Decode(
    'ChFGcmllbmRBZGRSZXNwb25zZRIyCgV2YWx1ZRgBIAEoCzIcLmdpemNsYXcucnBjLnYxLkZyaW'
    'VuZE9iamVjdFIFdmFsdWU=');

@$core.Deprecated('Use friendDeleteRequestDescriptor instead')
const FriendDeleteRequest$json = {
  '1': 'FriendDeleteRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `FriendDeleteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendDeleteRequestDescriptor = $convert
    .base64Decode('ChNGcmllbmREZWxldGVSZXF1ZXN0Eg4KAmlkGAEgASgJUgJpZA==');

@$core.Deprecated('Use friendDeleteResponseDescriptor instead')
const FriendDeleteResponse$json = {
  '1': 'FriendDeleteResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `FriendDeleteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendDeleteResponseDescriptor = $convert.base64Decode(
    'ChRGcmllbmREZWxldGVSZXNwb25zZRIyCgV2YWx1ZRgBIAEoCzIcLmdpemNsYXcucnBjLnYxLk'
    'ZyaWVuZE9iamVjdFIFdmFsdWU=');

@$core.Deprecated('Use friendGroupCreateRequestDescriptor instead')
const FriendGroupCreateRequest$json = {
  '1': 'FriendGroupCreateRequest',
  '2': [
    {
      '1': 'description',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'description',
      '17': true
    },
    {'1': 'name', '3': 2, '4': 1, '5': 9, '10': 'name'},
  ],
  '8': [
    {'1': '_description'},
  ],
};

/// Descriptor for `FriendGroupCreateRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupCreateRequestDescriptor =
    $convert.base64Decode(
        'ChhGcmllbmRHcm91cENyZWF0ZVJlcXVlc3QSJQoLZGVzY3JpcHRpb24YASABKAlIAFILZGVzY3'
        'JpcHRpb26IAQESEgoEbmFtZRgCIAEoCVIEbmFtZUIOCgxfZGVzY3JpcHRpb24=');

@$core.Deprecated('Use friendGroupCreateResponseDescriptor instead')
const FriendGroupCreateResponse$json = {
  '1': 'FriendGroupCreateResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `FriendGroupCreateResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupCreateResponseDescriptor =
    $convert.base64Decode(
        'ChlGcmllbmRHcm91cENyZWF0ZVJlc3BvbnNlEjcKBXZhbHVlGAEgASgLMiEuZ2l6Y2xhdy5ycG'
        'MudjEuRnJpZW5kR3JvdXBPYmplY3RSBXZhbHVl');

@$core.Deprecated('Use friendGroupDeleteRequestDescriptor instead')
const FriendGroupDeleteRequest$json = {
  '1': 'FriendGroupDeleteRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `FriendGroupDeleteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupDeleteRequestDescriptor = $convert
    .base64Decode('ChhGcmllbmRHcm91cERlbGV0ZVJlcXVlc3QSDgoCaWQYASABKAlSAmlk');

@$core.Deprecated('Use friendGroupDeleteResponseDescriptor instead')
const FriendGroupDeleteResponse$json = {
  '1': 'FriendGroupDeleteResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `FriendGroupDeleteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupDeleteResponseDescriptor =
    $convert.base64Decode(
        'ChlGcmllbmRHcm91cERlbGV0ZVJlc3BvbnNlEjcKBXZhbHVlGAEgASgLMiEuZ2l6Y2xhdy5ycG'
        'MudjEuRnJpZW5kR3JvdXBPYmplY3RSBXZhbHVl');

@$core.Deprecated('Use friendGroupGetRequestDescriptor instead')
const FriendGroupGetRequest$json = {
  '1': 'FriendGroupGetRequest',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `FriendGroupGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupGetRequestDescriptor = $convert
    .base64Decode('ChVGcmllbmRHcm91cEdldFJlcXVlc3QSDgoCaWQYASABKAlSAmlk');

@$core.Deprecated('Use friendGroupGetResponseDescriptor instead')
const FriendGroupGetResponse$json = {
  '1': 'FriendGroupGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `FriendGroupGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupGetResponseDescriptor =
    $convert.base64Decode(
        'ChZGcmllbmRHcm91cEdldFJlc3BvbnNlEjcKBXZhbHVlGAEgASgLMiEuZ2l6Y2xhdy5ycGMudj'
        'EuRnJpZW5kR3JvdXBPYmplY3RSBXZhbHVl');

@$core.Deprecated('Use friendGroupInviteTokenClearRequestDescriptor instead')
const FriendGroupInviteTokenClearRequest$json = {
  '1': 'FriendGroupInviteTokenClearRequest',
  '2': [
    {'1': 'friend_group_id', '3': 1, '4': 1, '5': 9, '10': 'friendGroupId'},
  ],
};

/// Descriptor for `FriendGroupInviteTokenClearRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupInviteTokenClearRequestDescriptor =
    $convert.base64Decode(
        'CiJGcmllbmRHcm91cEludml0ZVRva2VuQ2xlYXJSZXF1ZXN0EiYKD2ZyaWVuZF9ncm91cF9pZB'
        'gBIAEoCVINZnJpZW5kR3JvdXBJZA==');

@$core.Deprecated('Use friendGroupInviteTokenClearResponseDescriptor instead')
const FriendGroupInviteTokenClearResponse$json = {
  '1': 'FriendGroupInviteTokenClearResponse',
};

/// Descriptor for `FriendGroupInviteTokenClearResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupInviteTokenClearResponseDescriptor =
    $convert
        .base64Decode('CiNGcmllbmRHcm91cEludml0ZVRva2VuQ2xlYXJSZXNwb25zZQ==');

@$core.Deprecated('Use friendGroupInviteTokenCreateRequestDescriptor instead')
const FriendGroupInviteTokenCreateRequest$json = {
  '1': 'FriendGroupInviteTokenCreateRequest',
  '2': [
    {'1': 'friend_group_id', '3': 1, '4': 1, '5': 9, '10': 'friendGroupId'},
  ],
};

/// Descriptor for `FriendGroupInviteTokenCreateRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupInviteTokenCreateRequestDescriptor =
    $convert.base64Decode(
        'CiNGcmllbmRHcm91cEludml0ZVRva2VuQ3JlYXRlUmVxdWVzdBImCg9mcmllbmRfZ3JvdXBfaW'
        'QYASABKAlSDWZyaWVuZEdyb3VwSWQ=');

@$core.Deprecated('Use friendGroupInviteTokenCreateResponseDescriptor instead')
const FriendGroupInviteTokenCreateResponse$json = {
  '1': 'FriendGroupInviteTokenCreateResponse',
  '2': [
    {'1': 'expires_at', '3': 1, '4': 1, '5': 9, '10': 'expiresAt'},
    {'1': 'invite_token', '3': 2, '4': 1, '5': 9, '10': 'inviteToken'},
  ],
};

/// Descriptor for `FriendGroupInviteTokenCreateResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupInviteTokenCreateResponseDescriptor =
    $convert.base64Decode(
        'CiRGcmllbmRHcm91cEludml0ZVRva2VuQ3JlYXRlUmVzcG9uc2USHQoKZXhwaXJlc19hdBgBIA'
        'EoCVIJZXhwaXJlc0F0EiEKDGludml0ZV90b2tlbhgCIAEoCVILaW52aXRlVG9rZW4=');

@$core.Deprecated('Use friendGroupInviteTokenGetRequestDescriptor instead')
const FriendGroupInviteTokenGetRequest$json = {
  '1': 'FriendGroupInviteTokenGetRequest',
  '2': [
    {'1': 'friend_group_id', '3': 1, '4': 1, '5': 9, '10': 'friendGroupId'},
  ],
};

/// Descriptor for `FriendGroupInviteTokenGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupInviteTokenGetRequestDescriptor =
    $convert.base64Decode(
        'CiBGcmllbmRHcm91cEludml0ZVRva2VuR2V0UmVxdWVzdBImCg9mcmllbmRfZ3JvdXBfaWQYAS'
        'ABKAlSDWZyaWVuZEdyb3VwSWQ=');

@$core.Deprecated('Use friendGroupInviteTokenGetResponseDescriptor instead')
const FriendGroupInviteTokenGetResponse$json = {
  '1': 'FriendGroupInviteTokenGetResponse',
  '2': [
    {
      '1': 'expires_at',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'expiresAt',
      '17': true
    },
    {
      '1': 'invite_token',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'inviteToken',
      '17': true
    },
  ],
  '8': [
    {'1': '_expires_at'},
    {'1': '_invite_token'},
  ],
};

/// Descriptor for `FriendGroupInviteTokenGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupInviteTokenGetResponseDescriptor =
    $convert.base64Decode(
        'CiFGcmllbmRHcm91cEludml0ZVRva2VuR2V0UmVzcG9uc2USIgoKZXhwaXJlc19hdBgBIAEoCU'
        'gAUglleHBpcmVzQXSIAQESJgoMaW52aXRlX3Rva2VuGAIgASgJSAFSC2ludml0ZVRva2VuiAEB'
        'Qg0KC19leHBpcmVzX2F0Qg8KDV9pbnZpdGVfdG9rZW4=');

@$core.Deprecated('Use friendGroupJoinRequestDescriptor instead')
const FriendGroupJoinRequest$json = {
  '1': 'FriendGroupJoinRequest',
  '2': [
    {'1': 'invite_token', '3': 1, '4': 1, '5': 9, '10': 'inviteToken'},
  ],
};

/// Descriptor for `FriendGroupJoinRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupJoinRequestDescriptor =
    $convert.base64Decode(
        'ChZGcmllbmRHcm91cEpvaW5SZXF1ZXN0EiEKDGludml0ZV90b2tlbhgBIAEoCVILaW52aXRlVG'
        '9rZW4=');

@$core.Deprecated('Use friendGroupJoinResponseDescriptor instead')
const FriendGroupJoinResponse$json = {
  '1': 'FriendGroupJoinResponse',
  '2': [
    {
      '1': 'group',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupObject',
      '10': 'group'
    },
    {
      '1': 'member',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupMemberObject',
      '10': 'member'
    },
  ],
};

/// Descriptor for `FriendGroupJoinResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupJoinResponseDescriptor = $convert.base64Decode(
    'ChdGcmllbmRHcm91cEpvaW5SZXNwb25zZRI3CgVncm91cBgBIAEoCzIhLmdpemNsYXcucnBjLn'
    'YxLkZyaWVuZEdyb3VwT2JqZWN0UgVncm91cBI/CgZtZW1iZXIYAiABKAsyJy5naXpjbGF3LnJw'
    'Yy52MS5GcmllbmRHcm91cE1lbWJlck9iamVjdFIGbWVtYmVy');

@$core.Deprecated('Use friendGroupListRequestDescriptor instead')
const FriendGroupListRequest$json = {
  '1': 'FriendGroupListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `FriendGroupListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupListRequestDescriptor =
    $convert.base64Decode(
        'ChZGcmllbmRHcm91cExpc3RSZXF1ZXN0EhsKBmN1cnNvchgBIAEoCUgAUgZjdXJzb3KIAQESGQ'
        'oFbGltaXQYAiABKANIAVIFbGltaXSIAQFCCQoHX2N1cnNvckIICgZfbGltaXQ=');

@$core.Deprecated('Use friendGroupListResponseDescriptor instead')
const FriendGroupListResponse$json = {
  '1': 'FriendGroupListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupObject',
      '10': 'items'
    },
    {
      '1': 'next_cursor',
      '3': 3,
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

/// Descriptor for `FriendGroupListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupListResponseDescriptor = $convert.base64Decode(
    'ChdGcmllbmRHcm91cExpc3RSZXNwb25zZRIZCghoYXNfbmV4dBgBIAEoCFIHaGFzTmV4dBI3Cg'
    'VpdGVtcxgCIAMoCzIhLmdpemNsYXcucnBjLnYxLkZyaWVuZEdyb3VwT2JqZWN0UgVpdGVtcxIk'
    'CgtuZXh0X2N1cnNvchgDIAEoCUgAUgpuZXh0Q3Vyc29yiAEBQg4KDF9uZXh0X2N1cnNvcg==');

@$core.Deprecated('Use friendGroupMemberAddRequestDescriptor instead')
const FriendGroupMemberAddRequest$json = {
  '1': 'FriendGroupMemberAddRequest',
  '2': [
    {'1': 'friend_group_id', '3': 1, '4': 1, '5': 9, '10': 'friendGroupId'},
    {'1': 'peer_public_key', '3': 2, '4': 1, '5': 9, '10': 'peerPublicKey'},
    {
      '1': 'role',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.FriendGroupMemberMutableRole',
      '10': 'role'
    },
  ],
};

/// Descriptor for `FriendGroupMemberAddRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMemberAddRequestDescriptor = $convert.base64Decode(
    'ChtGcmllbmRHcm91cE1lbWJlckFkZFJlcXVlc3QSJgoPZnJpZW5kX2dyb3VwX2lkGAEgASgJUg'
    '1mcmllbmRHcm91cElkEiYKD3BlZXJfcHVibGljX2tleRgCIAEoCVINcGVlclB1YmxpY0tleRJA'
    'CgRyb2xlGAMgASgOMiwuZ2l6Y2xhdy5ycGMudjEuRnJpZW5kR3JvdXBNZW1iZXJNdXRhYmxlUm'
    '9sZVIEcm9sZQ==');

@$core.Deprecated('Use friendGroupMemberAddResponseDescriptor instead')
const FriendGroupMemberAddResponse$json = {
  '1': 'FriendGroupMemberAddResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupMemberObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `FriendGroupMemberAddResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMemberAddResponseDescriptor =
    $convert.base64Decode(
        'ChxGcmllbmRHcm91cE1lbWJlckFkZFJlc3BvbnNlEj0KBXZhbHVlGAEgASgLMicuZ2l6Y2xhdy'
        '5ycGMudjEuRnJpZW5kR3JvdXBNZW1iZXJPYmplY3RSBXZhbHVl');

@$core.Deprecated('Use friendGroupMemberDeleteRequestDescriptor instead')
const FriendGroupMemberDeleteRequest$json = {
  '1': 'FriendGroupMemberDeleteRequest',
  '2': [
    {'1': 'friend_group_id', '3': 1, '4': 1, '5': 9, '10': 'friendGroupId'},
    {'1': 'id', '3': 2, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `FriendGroupMemberDeleteRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMemberDeleteRequestDescriptor =
    $convert.base64Decode(
        'Ch5GcmllbmRHcm91cE1lbWJlckRlbGV0ZVJlcXVlc3QSJgoPZnJpZW5kX2dyb3VwX2lkGAEgAS'
        'gJUg1mcmllbmRHcm91cElkEg4KAmlkGAIgASgJUgJpZA==');

@$core.Deprecated('Use friendGroupMemberDeleteResponseDescriptor instead')
const FriendGroupMemberDeleteResponse$json = {
  '1': 'FriendGroupMemberDeleteResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupMemberObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `FriendGroupMemberDeleteResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMemberDeleteResponseDescriptor =
    $convert.base64Decode(
        'Ch9GcmllbmRHcm91cE1lbWJlckRlbGV0ZVJlc3BvbnNlEj0KBXZhbHVlGAEgASgLMicuZ2l6Y2'
        'xhdy5ycGMudjEuRnJpZW5kR3JvdXBNZW1iZXJPYmplY3RSBXZhbHVl');

@$core.Deprecated('Use friendGroupMemberListRequestDescriptor instead')
const FriendGroupMemberListRequest$json = {
  '1': 'FriendGroupMemberListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {
      '1': 'friend_group_id',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'friendGroupId',
      '17': true
    },
    {'1': 'limit', '3': 3, '4': 1, '5': 3, '9': 2, '10': 'limit', '17': true},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_friend_group_id'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `FriendGroupMemberListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMemberListRequestDescriptor = $convert.base64Decode(
    'ChxGcmllbmRHcm91cE1lbWJlckxpc3RSZXF1ZXN0EhsKBmN1cnNvchgBIAEoCUgAUgZjdXJzb3'
    'KIAQESKwoPZnJpZW5kX2dyb3VwX2lkGAIgASgJSAFSDWZyaWVuZEdyb3VwSWSIAQESGQoFbGlt'
    'aXQYAyABKANIAlIFbGltaXSIAQFCCQoHX2N1cnNvckISChBfZnJpZW5kX2dyb3VwX2lkQggKBl'
    '9saW1pdA==');

@$core.Deprecated('Use friendGroupMemberListResponseDescriptor instead')
const FriendGroupMemberListResponse$json = {
  '1': 'FriendGroupMemberListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupMemberObject',
      '10': 'items'
    },
    {
      '1': 'next_cursor',
      '3': 3,
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

/// Descriptor for `FriendGroupMemberListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMemberListResponseDescriptor = $convert.base64Decode(
    'Ch1GcmllbmRHcm91cE1lbWJlckxpc3RSZXNwb25zZRIZCghoYXNfbmV4dBgBIAEoCFIHaGFzTm'
    'V4dBI9CgVpdGVtcxgCIAMoCzInLmdpemNsYXcucnBjLnYxLkZyaWVuZEdyb3VwTWVtYmVyT2Jq'
    'ZWN0UgVpdGVtcxIkCgtuZXh0X2N1cnNvchgDIAEoCUgAUgpuZXh0Q3Vyc29yiAEBQg4KDF9uZX'
    'h0X2N1cnNvcg==');

@$core.Deprecated('Use friendGroupMemberObjectDescriptor instead')
const FriendGroupMemberObject$json = {
  '1': 'FriendGroupMemberObject',
  '2': [
    {
      '1': 'created_at',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'createdAt',
      '17': true
    },
    {
      '1': 'friend_group_id',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'friendGroupId',
      '17': true
    },
    {'1': 'id', '3': 3, '4': 1, '5': 9, '9': 2, '10': 'id', '17': true},
    {
      '1': 'peer_public_key',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'peerPublicKey',
      '17': true
    },
    {
      '1': 'role',
      '3': 5,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.FriendGroupMemberRole',
      '9': 4,
      '10': 'role',
      '17': true
    },
    {
      '1': 'updated_at',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 5,
      '10': 'updatedAt',
      '17': true
    },
  ],
  '8': [
    {'1': '_created_at'},
    {'1': '_friend_group_id'},
    {'1': '_id'},
    {'1': '_peer_public_key'},
    {'1': '_role'},
    {'1': '_updated_at'},
  ],
};

/// Descriptor for `FriendGroupMemberObject`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMemberObjectDescriptor = $convert.base64Decode(
    'ChdGcmllbmRHcm91cE1lbWJlck9iamVjdBIiCgpjcmVhdGVkX2F0GAEgASgJSABSCWNyZWF0ZW'
    'RBdIgBARIrCg9mcmllbmRfZ3JvdXBfaWQYAiABKAlIAVINZnJpZW5kR3JvdXBJZIgBARITCgJp'
    'ZBgDIAEoCUgCUgJpZIgBARIrCg9wZWVyX3B1YmxpY19rZXkYBCABKAlIA1INcGVlclB1YmxpY0'
    'tleYgBARI+CgRyb2xlGAUgASgOMiUuZ2l6Y2xhdy5ycGMudjEuRnJpZW5kR3JvdXBNZW1iZXJS'
    'b2xlSARSBHJvbGWIAQESIgoKdXBkYXRlZF9hdBgGIAEoCUgFUgl1cGRhdGVkQXSIAQFCDQoLX2'
    'NyZWF0ZWRfYXRCEgoQX2ZyaWVuZF9ncm91cF9pZEIFCgNfaWRCEgoQX3BlZXJfcHVibGljX2tl'
    'eUIHCgVfcm9sZUINCgtfdXBkYXRlZF9hdA==');

@$core.Deprecated('Use friendGroupMemberPutRequestDescriptor instead')
const FriendGroupMemberPutRequest$json = {
  '1': 'FriendGroupMemberPutRequest',
  '2': [
    {'1': 'friend_group_id', '3': 1, '4': 1, '5': 9, '10': 'friendGroupId'},
    {'1': 'id', '3': 2, '4': 1, '5': 9, '10': 'id'},
    {
      '1': 'role',
      '3': 3,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.FriendGroupMemberMutableRole',
      '10': 'role'
    },
  ],
};

/// Descriptor for `FriendGroupMemberPutRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMemberPutRequestDescriptor =
    $convert.base64Decode(
        'ChtGcmllbmRHcm91cE1lbWJlclB1dFJlcXVlc3QSJgoPZnJpZW5kX2dyb3VwX2lkGAEgASgJUg'
        '1mcmllbmRHcm91cElkEg4KAmlkGAIgASgJUgJpZBJACgRyb2xlGAMgASgOMiwuZ2l6Y2xhdy5y'
        'cGMudjEuRnJpZW5kR3JvdXBNZW1iZXJNdXRhYmxlUm9sZVIEcm9sZQ==');

@$core.Deprecated('Use friendGroupMemberPutResponseDescriptor instead')
const FriendGroupMemberPutResponse$json = {
  '1': 'FriendGroupMemberPutResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupMemberObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `FriendGroupMemberPutResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMemberPutResponseDescriptor =
    $convert.base64Decode(
        'ChxGcmllbmRHcm91cE1lbWJlclB1dFJlc3BvbnNlEj0KBXZhbHVlGAEgASgLMicuZ2l6Y2xhdy'
        '5ycGMudjEuRnJpZW5kR3JvdXBNZW1iZXJPYmplY3RSBXZhbHVl');

@$core.Deprecated('Use friendGroupMessageGetRequestDescriptor instead')
const FriendGroupMessageGetRequest$json = {
  '1': 'FriendGroupMessageGetRequest',
  '2': [
    {'1': 'friend_group_id', '3': 1, '4': 1, '5': 9, '10': 'friendGroupId'},
    {'1': 'id', '3': 2, '4': 1, '5': 9, '10': 'id'},
  ],
};

/// Descriptor for `FriendGroupMessageGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMessageGetRequestDescriptor =
    $convert.base64Decode(
        'ChxGcmllbmRHcm91cE1lc3NhZ2VHZXRSZXF1ZXN0EiYKD2ZyaWVuZF9ncm91cF9pZBgBIAEoCV'
        'INZnJpZW5kR3JvdXBJZBIOCgJpZBgCIAEoCVICaWQ=');

@$core.Deprecated('Use friendGroupMessageGetResponseDescriptor instead')
const FriendGroupMessageGetResponse$json = {
  '1': 'FriendGroupMessageGetResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupMessageObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `FriendGroupMessageGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMessageGetResponseDescriptor =
    $convert.base64Decode(
        'Ch1GcmllbmRHcm91cE1lc3NhZ2VHZXRSZXNwb25zZRI+CgV2YWx1ZRgBIAEoCzIoLmdpemNsYX'
        'cucnBjLnYxLkZyaWVuZEdyb3VwTWVzc2FnZU9iamVjdFIFdmFsdWU=');

@$core.Deprecated('Use friendGroupMessageListRequestDescriptor instead')
const FriendGroupMessageListRequest$json = {
  '1': 'FriendGroupMessageListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {
      '1': 'friend_group_id',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'friendGroupId',
      '17': true
    },
    {'1': 'limit', '3': 3, '4': 1, '5': 3, '9': 2, '10': 'limit', '17': true},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_friend_group_id'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `FriendGroupMessageListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMessageListRequestDescriptor = $convert.base64Decode(
    'Ch1GcmllbmRHcm91cE1lc3NhZ2VMaXN0UmVxdWVzdBIbCgZjdXJzb3IYASABKAlIAFIGY3Vyc2'
    '9yiAEBEisKD2ZyaWVuZF9ncm91cF9pZBgCIAEoCUgBUg1mcmllbmRHcm91cElkiAEBEhkKBWxp'
    'bWl0GAMgASgDSAJSBWxpbWl0iAEBQgkKB19jdXJzb3JCEgoQX2ZyaWVuZF9ncm91cF9pZEIICg'
    'ZfbGltaXQ=');

@$core.Deprecated('Use friendGroupMessageListResponseDescriptor instead')
const FriendGroupMessageListResponse$json = {
  '1': 'FriendGroupMessageListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupMessageObject',
      '10': 'items'
    },
    {
      '1': 'next_cursor',
      '3': 3,
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

/// Descriptor for `FriendGroupMessageListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMessageListResponseDescriptor =
    $convert.base64Decode(
        'Ch5GcmllbmRHcm91cE1lc3NhZ2VMaXN0UmVzcG9uc2USGQoIaGFzX25leHQYASABKAhSB2hhc0'
        '5leHQSPgoFaXRlbXMYAiADKAsyKC5naXpjbGF3LnJwYy52MS5GcmllbmRHcm91cE1lc3NhZ2VP'
        'YmplY3RSBWl0ZW1zEiQKC25leHRfY3Vyc29yGAMgASgJSABSCm5leHRDdXJzb3KIAQFCDgoMX2'
        '5leHRfY3Vyc29y');

@$core.Deprecated('Use friendGroupMessageObjectDescriptor instead')
const FriendGroupMessageObject$json = {
  '1': 'FriendGroupMessageObject',
  '2': [
    {
      '1': 'audio_content_type',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'audioContentType',
      '17': true
    },
    {
      '1': 'audio_path',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'audioPath',
      '17': true
    },
    {
      '1': 'audio_size_bytes',
      '3': 3,
      '4': 1,
      '5': 3,
      '9': 2,
      '10': 'audioSizeBytes',
      '17': true
    },
    {
      '1': 'created_at',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'createdAt',
      '17': true
    },
    {
      '1': 'expires_at',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'expiresAt',
      '17': true
    },
    {
      '1': 'friend_group_id',
      '3': 6,
      '4': 1,
      '5': 9,
      '9': 5,
      '10': 'friendGroupId',
      '17': true
    },
    {'1': 'id', '3': 7, '4': 1, '5': 9, '9': 6, '10': 'id', '17': true},
    {
      '1': 'sender_peer_public_key',
      '3': 8,
      '4': 1,
      '5': 9,
      '9': 7,
      '10': 'senderPeerPublicKey',
      '17': true
    },
    {
      '1': 'ttl_seconds',
      '3': 9,
      '4': 1,
      '5': 3,
      '9': 8,
      '10': 'ttlSeconds',
      '17': true
    },
  ],
  '8': [
    {'1': '_audio_content_type'},
    {'1': '_audio_path'},
    {'1': '_audio_size_bytes'},
    {'1': '_created_at'},
    {'1': '_expires_at'},
    {'1': '_friend_group_id'},
    {'1': '_id'},
    {'1': '_sender_peer_public_key'},
    {'1': '_ttl_seconds'},
  ],
};

/// Descriptor for `FriendGroupMessageObject`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMessageObjectDescriptor = $convert.base64Decode(
    'ChhGcmllbmRHcm91cE1lc3NhZ2VPYmplY3QSMQoSYXVkaW9fY29udGVudF90eXBlGAEgASgJSA'
    'BSEGF1ZGlvQ29udGVudFR5cGWIAQESIgoKYXVkaW9fcGF0aBgCIAEoCUgBUglhdWRpb1BhdGiI'
    'AQESLQoQYXVkaW9fc2l6ZV9ieXRlcxgDIAEoA0gCUg5hdWRpb1NpemVCeXRlc4gBARIiCgpjcm'
    'VhdGVkX2F0GAQgASgJSANSCWNyZWF0ZWRBdIgBARIiCgpleHBpcmVzX2F0GAUgASgJSARSCWV4'
    'cGlyZXNBdIgBARIrCg9mcmllbmRfZ3JvdXBfaWQYBiABKAlIBVINZnJpZW5kR3JvdXBJZIgBAR'
    'ITCgJpZBgHIAEoCUgGUgJpZIgBARI4ChZzZW5kZXJfcGVlcl9wdWJsaWNfa2V5GAggASgJSAdS'
    'E3NlbmRlclBlZXJQdWJsaWNLZXmIAQESJAoLdHRsX3NlY29uZHMYCSABKANICFIKdHRsU2Vjb2'
    '5kc4gBAUIVChNfYXVkaW9fY29udGVudF90eXBlQg0KC19hdWRpb19wYXRoQhMKEV9hdWRpb19z'
    'aXplX2J5dGVzQg0KC19jcmVhdGVkX2F0Qg0KC19leHBpcmVzX2F0QhIKEF9mcmllbmRfZ3JvdX'
    'BfaWRCBQoDX2lkQhkKF19zZW5kZXJfcGVlcl9wdWJsaWNfa2V5Qg4KDF90dGxfc2Vjb25kcw==');

@$core.Deprecated('Use friendGroupMessageSendRequestDescriptor instead')
const FriendGroupMessageSendRequest$json = {
  '1': 'FriendGroupMessageSendRequest',
  '2': [
    {'1': 'audio_base64', '3': 1, '4': 1, '5': 12, '10': 'audioBase64'},
    {
      '1': 'audio_content_type',
      '3': 2,
      '4': 1,
      '5': 9,
      '10': 'audioContentType'
    },
    {'1': 'friend_group_id', '3': 3, '4': 1, '5': 9, '10': 'friendGroupId'},
    {
      '1': 'ttl_seconds',
      '3': 4,
      '4': 1,
      '5': 3,
      '9': 0,
      '10': 'ttlSeconds',
      '17': true
    },
  ],
  '8': [
    {'1': '_ttl_seconds'},
  ],
};

/// Descriptor for `FriendGroupMessageSendRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMessageSendRequestDescriptor = $convert.base64Decode(
    'Ch1GcmllbmRHcm91cE1lc3NhZ2VTZW5kUmVxdWVzdBIhCgxhdWRpb19iYXNlNjQYASABKAxSC2'
    'F1ZGlvQmFzZTY0EiwKEmF1ZGlvX2NvbnRlbnRfdHlwZRgCIAEoCVIQYXVkaW9Db250ZW50VHlw'
    'ZRImCg9mcmllbmRfZ3JvdXBfaWQYAyABKAlSDWZyaWVuZEdyb3VwSWQSJAoLdHRsX3NlY29uZH'
    'MYBCABKANIAFIKdHRsU2Vjb25kc4gBAUIOCgxfdHRsX3NlY29uZHM=');

@$core.Deprecated('Use friendGroupMessageSendResponseDescriptor instead')
const FriendGroupMessageSendResponse$json = {
  '1': 'FriendGroupMessageSendResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupMessageObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `FriendGroupMessageSendResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupMessageSendResponseDescriptor =
    $convert.base64Decode(
        'Ch5GcmllbmRHcm91cE1lc3NhZ2VTZW5kUmVzcG9uc2USPgoFdmFsdWUYASABKAsyKC5naXpjbG'
        'F3LnJwYy52MS5GcmllbmRHcm91cE1lc3NhZ2VPYmplY3RSBXZhbHVl');

@$core.Deprecated('Use friendGroupObjectDescriptor instead')
const FriendGroupObject$json = {
  '1': 'FriendGroupObject',
  '2': [
    {
      '1': 'created_at',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'createdAt',
      '17': true
    },
    {
      '1': 'created_by_peer_public_key',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'createdByPeerPublicKey',
      '17': true
    },
    {
      '1': 'description',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'description',
      '17': true
    },
    {'1': 'id', '3': 4, '4': 1, '5': 9, '9': 3, '10': 'id', '17': true},
    {
      '1': 'my_role',
      '3': 5,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.FriendGroupMemberRole',
      '9': 4,
      '10': 'myRole',
      '17': true
    },
    {'1': 'name', '3': 6, '4': 1, '5': 9, '9': 5, '10': 'name', '17': true},
    {
      '1': 'updated_at',
      '3': 7,
      '4': 1,
      '5': 9,
      '9': 6,
      '10': 'updatedAt',
      '17': true
    },
    {
      '1': 'workspace_name',
      '3': 8,
      '4': 1,
      '5': 9,
      '9': 7,
      '10': 'workspaceName',
      '17': true
    },
  ],
  '8': [
    {'1': '_created_at'},
    {'1': '_created_by_peer_public_key'},
    {'1': '_description'},
    {'1': '_id'},
    {'1': '_my_role'},
    {'1': '_name'},
    {'1': '_updated_at'},
    {'1': '_workspace_name'},
  ],
};

/// Descriptor for `FriendGroupObject`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupObjectDescriptor = $convert.base64Decode(
    'ChFGcmllbmRHcm91cE9iamVjdBIiCgpjcmVhdGVkX2F0GAEgASgJSABSCWNyZWF0ZWRBdIgBAR'
    'I/ChpjcmVhdGVkX2J5X3BlZXJfcHVibGljX2tleRgCIAEoCUgBUhZjcmVhdGVkQnlQZWVyUHVi'
    'bGljS2V5iAEBEiUKC2Rlc2NyaXB0aW9uGAMgASgJSAJSC2Rlc2NyaXB0aW9uiAEBEhMKAmlkGA'
    'QgASgJSANSAmlkiAEBEkMKB215X3JvbGUYBSABKA4yJS5naXpjbGF3LnJwYy52MS5GcmllbmRH'
    'cm91cE1lbWJlclJvbGVIBFIGbXlSb2xliAEBEhcKBG5hbWUYBiABKAlIBVIEbmFtZYgBARIiCg'
    'p1cGRhdGVkX2F0GAcgASgJSAZSCXVwZGF0ZWRBdIgBARIqCg53b3Jrc3BhY2VfbmFtZRgIIAEo'
    'CUgHUg13b3Jrc3BhY2VOYW1liAEBQg0KC19jcmVhdGVkX2F0Qh0KG19jcmVhdGVkX2J5X3BlZX'
    'JfcHVibGljX2tleUIOCgxfZGVzY3JpcHRpb25CBQoDX2lkQgoKCF9teV9yb2xlQgcKBV9uYW1l'
    'Qg0KC191cGRhdGVkX2F0QhEKD193b3Jrc3BhY2VfbmFtZQ==');

@$core.Deprecated('Use friendGroupPutRequestDescriptor instead')
const FriendGroupPutRequest$json = {
  '1': 'FriendGroupPutRequest',
  '2': [
    {
      '1': 'description',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'description',
      '17': true
    },
    {'1': 'id', '3': 2, '4': 1, '5': 9, '10': 'id'},
    {'1': 'name', '3': 3, '4': 1, '5': 9, '9': 1, '10': 'name', '17': true},
  ],
  '8': [
    {'1': '_description'},
    {'1': '_name'},
  ],
};

/// Descriptor for `FriendGroupPutRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupPutRequestDescriptor = $convert.base64Decode(
    'ChVGcmllbmRHcm91cFB1dFJlcXVlc3QSJQoLZGVzY3JpcHRpb24YASABKAlIAFILZGVzY3JpcH'
    'Rpb26IAQESDgoCaWQYAiABKAlSAmlkEhcKBG5hbWUYAyABKAlIAVIEbmFtZYgBAUIOCgxfZGVz'
    'Y3JpcHRpb25CBwoFX25hbWU=');

@$core.Deprecated('Use friendGroupPutResponseDescriptor instead')
const FriendGroupPutResponse$json = {
  '1': 'FriendGroupPutResponse',
  '2': [
    {
      '1': 'value',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendGroupObject',
      '10': 'value'
    },
  ],
};

/// Descriptor for `FriendGroupPutResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendGroupPutResponseDescriptor =
    $convert.base64Decode(
        'ChZGcmllbmRHcm91cFB1dFJlc3BvbnNlEjcKBXZhbHVlGAEgASgLMiEuZ2l6Y2xhdy5ycGMudj'
        'EuRnJpZW5kR3JvdXBPYmplY3RSBXZhbHVl');

@$core.Deprecated('Use friendInviteTokenClearRequestDescriptor instead')
const FriendInviteTokenClearRequest$json = {
  '1': 'FriendInviteTokenClearRequest',
};

/// Descriptor for `FriendInviteTokenClearRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendInviteTokenClearRequestDescriptor =
    $convert.base64Decode('Ch1GcmllbmRJbnZpdGVUb2tlbkNsZWFyUmVxdWVzdA==');

@$core.Deprecated('Use friendInviteTokenClearResponseDescriptor instead')
const FriendInviteTokenClearResponse$json = {
  '1': 'FriendInviteTokenClearResponse',
};

/// Descriptor for `FriendInviteTokenClearResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendInviteTokenClearResponseDescriptor =
    $convert.base64Decode('Ch5GcmllbmRJbnZpdGVUb2tlbkNsZWFyUmVzcG9uc2U=');

@$core.Deprecated('Use friendInviteTokenCreateRequestDescriptor instead')
const FriendInviteTokenCreateRequest$json = {
  '1': 'FriendInviteTokenCreateRequest',
};

/// Descriptor for `FriendInviteTokenCreateRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendInviteTokenCreateRequestDescriptor =
    $convert.base64Decode('Ch5GcmllbmRJbnZpdGVUb2tlbkNyZWF0ZVJlcXVlc3Q=');

@$core.Deprecated('Use friendInviteTokenCreateResponseDescriptor instead')
const FriendInviteTokenCreateResponse$json = {
  '1': 'FriendInviteTokenCreateResponse',
  '2': [
    {'1': 'expires_at', '3': 1, '4': 1, '5': 9, '10': 'expiresAt'},
    {'1': 'invite_token', '3': 2, '4': 1, '5': 9, '10': 'inviteToken'},
  ],
};

/// Descriptor for `FriendInviteTokenCreateResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendInviteTokenCreateResponseDescriptor =
    $convert.base64Decode(
        'Ch9GcmllbmRJbnZpdGVUb2tlbkNyZWF0ZVJlc3BvbnNlEh0KCmV4cGlyZXNfYXQYASABKAlSCW'
        'V4cGlyZXNBdBIhCgxpbnZpdGVfdG9rZW4YAiABKAlSC2ludml0ZVRva2Vu');

@$core.Deprecated('Use friendInviteTokenGetRequestDescriptor instead')
const FriendInviteTokenGetRequest$json = {
  '1': 'FriendInviteTokenGetRequest',
};

/// Descriptor for `FriendInviteTokenGetRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendInviteTokenGetRequestDescriptor =
    $convert.base64Decode('ChtGcmllbmRJbnZpdGVUb2tlbkdldFJlcXVlc3Q=');

@$core.Deprecated('Use friendInviteTokenGetResponseDescriptor instead')
const FriendInviteTokenGetResponse$json = {
  '1': 'FriendInviteTokenGetResponse',
  '2': [
    {
      '1': 'expires_at',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'expiresAt',
      '17': true
    },
    {
      '1': 'invite_token',
      '3': 2,
      '4': 1,
      '5': 9,
      '9': 1,
      '10': 'inviteToken',
      '17': true
    },
  ],
  '8': [
    {'1': '_expires_at'},
    {'1': '_invite_token'},
  ],
};

/// Descriptor for `FriendInviteTokenGetResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendInviteTokenGetResponseDescriptor =
    $convert.base64Decode(
        'ChxGcmllbmRJbnZpdGVUb2tlbkdldFJlc3BvbnNlEiIKCmV4cGlyZXNfYXQYASABKAlIAFIJZX'
        'hwaXJlc0F0iAEBEiYKDGludml0ZV90b2tlbhgCIAEoCUgBUgtpbnZpdGVUb2tlbogBAUINCgtf'
        'ZXhwaXJlc19hdEIPCg1faW52aXRlX3Rva2Vu');

@$core.Deprecated('Use friendListRequestDescriptor instead')
const FriendListRequest$json = {
  '1': 'FriendListRequest',
  '2': [
    {'1': 'cursor', '3': 1, '4': 1, '5': 9, '9': 0, '10': 'cursor', '17': true},
    {'1': 'limit', '3': 2, '4': 1, '5': 3, '9': 1, '10': 'limit', '17': true},
  ],
  '8': [
    {'1': '_cursor'},
    {'1': '_limit'},
  ],
};

/// Descriptor for `FriendListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendListRequestDescriptor = $convert.base64Decode(
    'ChFGcmllbmRMaXN0UmVxdWVzdBIbCgZjdXJzb3IYASABKAlIAFIGY3Vyc29yiAEBEhkKBWxpbW'
    'l0GAIgASgDSAFSBWxpbWl0iAEBQgkKB19jdXJzb3JCCAoGX2xpbWl0');

@$core.Deprecated('Use friendListResponseDescriptor instead')
const FriendListResponse$json = {
  '1': 'FriendListResponse',
  '2': [
    {'1': 'has_next', '3': 1, '4': 1, '5': 8, '10': 'hasNext'},
    {
      '1': 'items',
      '3': 2,
      '4': 3,
      '5': 11,
      '6': '.gizclaw.rpc.v1.FriendObject',
      '10': 'items'
    },
    {
      '1': 'next_cursor',
      '3': 3,
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

/// Descriptor for `FriendListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendListResponseDescriptor = $convert.base64Decode(
    'ChJGcmllbmRMaXN0UmVzcG9uc2USGQoIaGFzX25leHQYASABKAhSB2hhc05leHQSMgoFaXRlbX'
    'MYAiADKAsyHC5naXpjbGF3LnJwYy52MS5GcmllbmRPYmplY3RSBWl0ZW1zEiQKC25leHRfY3Vy'
    'c29yGAMgASgJSABSCm5leHRDdXJzb3KIAQFCDgoMX25leHRfY3Vyc29y');

@$core.Deprecated('Use friendObjectDescriptor instead')
const FriendObject$json = {
  '1': 'FriendObject',
  '2': [
    {
      '1': 'created_at',
      '3': 1,
      '4': 1,
      '5': 9,
      '9': 0,
      '10': 'createdAt',
      '17': true
    },
    {'1': 'id', '3': 2, '4': 1, '5': 9, '9': 1, '10': 'id', '17': true},
    {
      '1': 'peer_public_key',
      '3': 3,
      '4': 1,
      '5': 9,
      '9': 2,
      '10': 'peerPublicKey',
      '17': true
    },
    {
      '1': 'updated_at',
      '3': 4,
      '4': 1,
      '5': 9,
      '9': 3,
      '10': 'updatedAt',
      '17': true
    },
    {
      '1': 'workspace_name',
      '3': 5,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'workspaceName',
      '17': true
    },
  ],
  '8': [
    {'1': '_created_at'},
    {'1': '_id'},
    {'1': '_peer_public_key'},
    {'1': '_updated_at'},
    {'1': '_workspace_name'},
  ],
};

/// Descriptor for `FriendObject`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List friendObjectDescriptor = $convert.base64Decode(
    'CgxGcmllbmRPYmplY3QSIgoKY3JlYXRlZF9hdBgBIAEoCUgAUgljcmVhdGVkQXSIAQESEwoCaW'
    'QYAiABKAlIAVICaWSIAQESKwoPcGVlcl9wdWJsaWNfa2V5GAMgASgJSAJSDXBlZXJQdWJsaWNL'
    'ZXmIAQESIgoKdXBkYXRlZF9hdBgEIAEoCUgDUgl1cGRhdGVkQXSIAQESKgoOd29ya3NwYWNlX2'
    '5hbWUYBSABKAlIBFINd29ya3NwYWNlTmFtZYgBAUINCgtfY3JlYXRlZF9hdEIFCgNfaWRCEgoQ'
    'X3BlZXJfcHVibGljX2tleUINCgtfdXBkYXRlZF9hdEIRCg9fd29ya3NwYWNlX25hbWU=');
