// This is a generated file - do not edit.
//
// Generated from payload/edge.proto.

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

@$core.Deprecated('Use peerAssignmentDescriptor instead')
const PeerAssignment$json = {
  '1': 'PeerAssignment',
  '2': [
    {'1': 'peer_public_key', '3': 1, '4': 1, '5': 9, '10': 'peerPublicKey'},
    {'1': 'server_public_key', '3': 2, '4': 1, '5': 9, '10': 'serverPublicKey'},
    {'1': 'server_endpoint', '3': 3, '4': 1, '5': 9, '10': 'serverEndpoint'},
    {
      '1': 'role',
      '3': 4,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.PeerRole',
      '10': 'role'
    },
    {'1': 'version', '3': 5, '4': 1, '5': 3, '10': 'version'},
    {'1': 'updated_at', '3': 6, '4': 1, '5': 9, '10': 'updatedAt'},
  ],
};

/// Descriptor for `PeerAssignment`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List peerAssignmentDescriptor = $convert.base64Decode(
    'Cg5QZWVyQXNzaWdubWVudBImCg9wZWVyX3B1YmxpY19rZXkYASABKAlSDXBlZXJQdWJsaWNLZX'
    'kSKgoRc2VydmVyX3B1YmxpY19rZXkYAiABKAlSD3NlcnZlclB1YmxpY0tleRInCg9zZXJ2ZXJf'
    'ZW5kcG9pbnQYAyABKAlSDnNlcnZlckVuZHBvaW50EiwKBHJvbGUYBCABKA4yGC5naXpjbGF3Ln'
    'JwYy52MS5QZWVyUm9sZVIEcm9sZRIYCgd2ZXJzaW9uGAUgASgDUgd2ZXJzaW9uEh0KCnVwZGF0'
    'ZWRfYXQYBiABKAlSCXVwZGF0ZWRBdA==');

@$core.Deprecated('Use serverPeerLookupRequestDescriptor instead')
const ServerPeerLookupRequest$json = {
  '1': 'ServerPeerLookupRequest',
  '2': [
    {'1': 'peer_public_key', '3': 1, '4': 1, '5': 9, '10': 'peerPublicKey'},
  ],
};

/// Descriptor for `ServerPeerLookupRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPeerLookupRequestDescriptor =
    $convert.base64Decode(
        'ChdTZXJ2ZXJQZWVyTG9va3VwUmVxdWVzdBImCg9wZWVyX3B1YmxpY19rZXkYASABKAlSDXBlZX'
        'JQdWJsaWNLZXk=');

@$core.Deprecated('Use serverPeerLookupResponseDescriptor instead')
const ServerPeerLookupResponse$json = {
  '1': 'ServerPeerLookupResponse',
  '2': [
    {
      '1': 'assignment',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerAssignment',
      '10': 'assignment'
    },
  ],
};

/// Descriptor for `ServerPeerLookupResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPeerLookupResponseDescriptor =
    $convert.base64Decode(
        'ChhTZXJ2ZXJQZWVyTG9va3VwUmVzcG9uc2USPgoKYXNzaWdubWVudBgBIAEoCzIeLmdpemNsYX'
        'cucnBjLnYxLlBlZXJBc3NpZ25tZW50Ugphc3NpZ25tZW50');

@$core.Deprecated('Use serverPeerAssignRequestDescriptor instead')
const ServerPeerAssignRequest$json = {
  '1': 'ServerPeerAssignRequest',
  '2': [
    {'1': 'peer_public_key', '3': 1, '4': 1, '5': 9, '10': 'peerPublicKey'},
    {
      '1': 'expected_version',
      '3': 2,
      '4': 1,
      '5': 3,
      '9': 0,
      '10': 'expectedVersion',
      '17': true
    },
  ],
  '8': [
    {'1': '_expected_version'},
  ],
};

/// Descriptor for `ServerPeerAssignRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPeerAssignRequestDescriptor = $convert.base64Decode(
    'ChdTZXJ2ZXJQZWVyQXNzaWduUmVxdWVzdBImCg9wZWVyX3B1YmxpY19rZXkYASABKAlSDXBlZX'
    'JQdWJsaWNLZXkSLgoQZXhwZWN0ZWRfdmVyc2lvbhgCIAEoA0gAUg9leHBlY3RlZFZlcnNpb26I'
    'AQFCEwoRX2V4cGVjdGVkX3ZlcnNpb24=');

@$core.Deprecated('Use serverPeerAssignResponseDescriptor instead')
const ServerPeerAssignResponse$json = {
  '1': 'ServerPeerAssignResponse',
  '2': [
    {
      '1': 'assignment',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerAssignment',
      '10': 'assignment'
    },
  ],
};

/// Descriptor for `ServerPeerAssignResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverPeerAssignResponseDescriptor =
    $convert.base64Decode(
        'ChhTZXJ2ZXJQZWVyQXNzaWduUmVzcG9uc2USPgoKYXNzaWdubWVudBgBIAEoCzIeLmdpemNsYX'
        'cucnBjLnYxLlBlZXJBc3NpZ25tZW50Ugphc3NpZ25tZW50');

@$core.Deprecated('Use serverRouteResolveRequestDescriptor instead')
const ServerRouteResolveRequest$json = {
  '1': 'ServerRouteResolveRequest',
  '2': [
    {
      '1': 'target_peer_public_key',
      '3': 1,
      '4': 1,
      '5': 9,
      '10': 'targetPeerPublicKey'
    },
  ],
};

/// Descriptor for `ServerRouteResolveRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRouteResolveRequestDescriptor =
    $convert.base64Decode(
        'ChlTZXJ2ZXJSb3V0ZVJlc29sdmVSZXF1ZXN0EjMKFnRhcmdldF9wZWVyX3B1YmxpY19rZXkYAS'
        'ABKAlSE3RhcmdldFBlZXJQdWJsaWNLZXk=');

@$core.Deprecated('Use serverRouteResolveResponseDescriptor instead')
const ServerRouteResolveResponse$json = {
  '1': 'ServerRouteResolveResponse',
  '2': [
    {
      '1': 'assignment',
      '3': 1,
      '4': 1,
      '5': 11,
      '6': '.gizclaw.rpc.v1.PeerAssignment',
      '10': 'assignment'
    },
  ],
};

/// Descriptor for `ServerRouteResolveResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverRouteResolveResponseDescriptor =
    $convert.base64Decode(
        'ChpTZXJ2ZXJSb3V0ZVJlc29sdmVSZXNwb25zZRI+Cgphc3NpZ25tZW50GAEgASgLMh4uZ2l6Y2'
        'xhdy5ycGMudjEuUGVlckFzc2lnbm1lbnRSCmFzc2lnbm1lbnQ=');
