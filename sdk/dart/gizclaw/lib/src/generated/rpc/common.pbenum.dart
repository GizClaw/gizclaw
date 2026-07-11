// This is a generated file - do not edit.
//
// Generated from common.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

class RpcErrorCode extends $pb.ProtobufEnum {
  static const RpcErrorCode RPC_ERROR_CODE_UNSPECIFIED =
      RpcErrorCode._(0, _omitEnumNames ? '' : 'RPC_ERROR_CODE_UNSPECIFIED');
  static const RpcErrorCode RPC_ERROR_CODE_PARSE_ERROR = RpcErrorCode._(
      -32700, _omitEnumNames ? '' : 'RPC_ERROR_CODE_PARSE_ERROR');
  static const RpcErrorCode RPC_ERROR_CODE_INVALID_REQUEST = RpcErrorCode._(
      -32600, _omitEnumNames ? '' : 'RPC_ERROR_CODE_INVALID_REQUEST');
  static const RpcErrorCode RPC_ERROR_CODE_METHOD_NOT_FOUND = RpcErrorCode._(
      -32601, _omitEnumNames ? '' : 'RPC_ERROR_CODE_METHOD_NOT_FOUND');
  static const RpcErrorCode RPC_ERROR_CODE_INVALID_PARAMS = RpcErrorCode._(
      -32602, _omitEnumNames ? '' : 'RPC_ERROR_CODE_INVALID_PARAMS');
  static const RpcErrorCode RPC_ERROR_CODE_INTERNAL_ERROR = RpcErrorCode._(
      -32603, _omitEnumNames ? '' : 'RPC_ERROR_CODE_INTERNAL_ERROR');
  static const RpcErrorCode RPC_ERROR_CODE_BAD_REQUEST =
      RpcErrorCode._(400, _omitEnumNames ? '' : 'RPC_ERROR_CODE_BAD_REQUEST');
  static const RpcErrorCode RPC_ERROR_CODE_FORBIDDEN =
      RpcErrorCode._(403, _omitEnumNames ? '' : 'RPC_ERROR_CODE_FORBIDDEN');
  static const RpcErrorCode RPC_ERROR_CODE_NOT_FOUND =
      RpcErrorCode._(404, _omitEnumNames ? '' : 'RPC_ERROR_CODE_NOT_FOUND');
  static const RpcErrorCode RPC_ERROR_CODE_CONFLICT =
      RpcErrorCode._(409, _omitEnumNames ? '' : 'RPC_ERROR_CODE_CONFLICT');

  static const $core.List<RpcErrorCode> values = <RpcErrorCode>[
    RPC_ERROR_CODE_UNSPECIFIED,
    RPC_ERROR_CODE_PARSE_ERROR,
    RPC_ERROR_CODE_INVALID_REQUEST,
    RPC_ERROR_CODE_METHOD_NOT_FOUND,
    RPC_ERROR_CODE_INVALID_PARAMS,
    RPC_ERROR_CODE_INTERNAL_ERROR,
    RPC_ERROR_CODE_BAD_REQUEST,
    RPC_ERROR_CODE_FORBIDDEN,
    RPC_ERROR_CODE_NOT_FOUND,
    RPC_ERROR_CODE_CONFLICT,
  ];

  static final $core.Map<$core.int, RpcErrorCode> _byValue =
      $pb.ProtobufEnum.initByValue(values);
  static RpcErrorCode? valueOf($core.int value) => _byValue[value];

  const RpcErrorCode._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
