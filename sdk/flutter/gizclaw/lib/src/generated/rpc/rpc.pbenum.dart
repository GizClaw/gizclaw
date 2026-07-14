// This is a generated file - do not edit.
//
// Generated from rpc.proto.

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

class RpcMethod extends $pb.ProtobufEnum {
  static const RpcMethod RPC_METHOD_UNSPECIFIED =
      RpcMethod._(0, _omitEnumNames ? '' : 'RPC_METHOD_UNSPECIFIED');
  static const RpcMethod RPC_METHOD_ALL_PING =
      RpcMethod._(1, _omitEnumNames ? '' : 'RPC_METHOD_ALL_PING');
  static const RpcMethod RPC_METHOD_ALL_SPEED_TEST_RUN =
      RpcMethod._(2, _omitEnumNames ? '' : 'RPC_METHOD_ALL_SPEED_TEST_RUN');
  static const RpcMethod RPC_METHOD_CLIENT_INFO_GET =
      RpcMethod._(3, _omitEnumNames ? '' : 'RPC_METHOD_CLIENT_INFO_GET');
  static const RpcMethod RPC_METHOD_CLIENT_IDENTIFIERS_GET =
      RpcMethod._(4, _omitEnumNames ? '' : 'RPC_METHOD_CLIENT_IDENTIFIERS_GET');
  static const RpcMethod RPC_METHOD_SERVER_INFO_GET =
      RpcMethod._(5, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_INFO_GET');
  static const RpcMethod RPC_METHOD_SERVER_INFO_PUT =
      RpcMethod._(6, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_INFO_PUT');
  static const RpcMethod RPC_METHOD_SERVER_RUNTIME_GET =
      RpcMethod._(7, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUNTIME_GET');
  static const RpcMethod RPC_METHOD_SERVER_STATUS_GET =
      RpcMethod._(8, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_STATUS_GET');
  static const RpcMethod RPC_METHOD_SERVER_RUN_AGENT_GET =
      RpcMethod._(9, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_AGENT_GET');
  static const RpcMethod RPC_METHOD_SERVER_RUN_AGENT_SET =
      RpcMethod._(10, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_AGENT_SET');
  static const RpcMethod RPC_METHOD_SERVER_RUN_WORKSPACE_GET = RpcMethod._(
      11, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_WORKSPACE_GET');
  static const RpcMethod RPC_METHOD_SERVER_RUN_WORKSPACE_SET = RpcMethod._(
      12, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_WORKSPACE_SET');
  static const RpcMethod RPC_METHOD_SERVER_RUN_WORKSPACE_RELOAD = RpcMethod._(
      13, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_WORKSPACE_RELOAD');
  static const RpcMethod RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY = RpcMethod._(
      14, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY');
  static const RpcMethod RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY_PLAY =
      RpcMethod._(15,
          _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY_PLAY');
  static const RpcMethod RPC_METHOD_SERVER_RUN_WORKSPACE_MEMORY_STATS =
      RpcMethod._(16,
          _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_WORKSPACE_MEMORY_STATS');
  static const RpcMethod RPC_METHOD_SERVER_RUN_WORKSPACE_RECALL = RpcMethod._(
      17, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_WORKSPACE_RECALL');
  static const RpcMethod RPC_METHOD_SERVER_RUN_RELOAD =
      RpcMethod._(18, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_RELOAD');
  static const RpcMethod RPC_METHOD_SERVER_RUN_STATUS =
      RpcMethod._(19, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_STATUS');
  static const RpcMethod RPC_METHOD_SERVER_RUN_STOP =
      RpcMethod._(20, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_STOP');
  static const RpcMethod RPC_METHOD_SERVER_RUN_SAY =
      RpcMethod._(21, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_RUN_SAY');
  static const RpcMethod RPC_METHOD_SERVER_FIRMWARE_LIST =
      RpcMethod._(22, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FIRMWARE_LIST');
  static const RpcMethod RPC_METHOD_SERVER_FIRMWARE_GET =
      RpcMethod._(23, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FIRMWARE_GET');
  static const RpcMethod RPC_METHOD_SERVER_FIRMWARE_FILES_DOWNLOAD = RpcMethod
      ._(24, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FIRMWARE_FILES_DOWNLOAD');
  static const RpcMethod RPC_METHOD_SERVER_WORKSPACE_LIST =
      RpcMethod._(25, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_WORKSPACE_LIST');
  static const RpcMethod RPC_METHOD_SERVER_WORKSPACE_GET =
      RpcMethod._(26, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_WORKSPACE_GET');
  static const RpcMethod RPC_METHOD_SERVER_WORKSPACE_CREATE = RpcMethod._(
      27, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_WORKSPACE_CREATE');
  static const RpcMethod RPC_METHOD_SERVER_WORKSPACE_PUT =
      RpcMethod._(28, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_WORKSPACE_PUT');
  static const RpcMethod RPC_METHOD_SERVER_WORKSPACE_DELETE = RpcMethod._(
      29, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_WORKSPACE_DELETE');
  static const RpcMethod RPC_METHOD_SERVER_WORKSPACE_HISTORY_LIST = RpcMethod._(
      30, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_WORKSPACE_HISTORY_LIST');
  static const RpcMethod RPC_METHOD_SERVER_WORKSPACE_HISTORY_GET = RpcMethod._(
      31, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_WORKSPACE_HISTORY_GET');
  static const RpcMethod RPC_METHOD_SERVER_WORKSPACE_HISTORY_AUDIO_GET =
      RpcMethod._(
          32,
          _omitEnumNames
              ? ''
              : 'RPC_METHOD_SERVER_WORKSPACE_HISTORY_AUDIO_GET');
  static const RpcMethod RPC_METHOD_SERVER_WORKFLOW_LIST =
      RpcMethod._(33, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_WORKFLOW_LIST');
  static const RpcMethod RPC_METHOD_SERVER_WORKFLOW_GET =
      RpcMethod._(34, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_WORKFLOW_GET');
  static const RpcMethod RPC_METHOD_SERVER_WORKFLOW_CREATE = RpcMethod._(
      35, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_WORKFLOW_CREATE');
  static const RpcMethod RPC_METHOD_SERVER_WORKFLOW_PUT =
      RpcMethod._(36, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_WORKFLOW_PUT');
  static const RpcMethod RPC_METHOD_SERVER_WORKFLOW_DELETE = RpcMethod._(
      37, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_WORKFLOW_DELETE');
  static const RpcMethod RPC_METHOD_SERVER_MODEL_LIST =
      RpcMethod._(38, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_MODEL_LIST');
  static const RpcMethod RPC_METHOD_SERVER_MODEL_GET =
      RpcMethod._(39, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_MODEL_GET');
  static const RpcMethod RPC_METHOD_SERVER_MODEL_CREATE =
      RpcMethod._(40, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_MODEL_CREATE');
  static const RpcMethod RPC_METHOD_SERVER_MODEL_PUT =
      RpcMethod._(41, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_MODEL_PUT');
  static const RpcMethod RPC_METHOD_SERVER_MODEL_DELETE =
      RpcMethod._(42, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_MODEL_DELETE');
  static const RpcMethod RPC_METHOD_SERVER_VOICE_LIST =
      RpcMethod._(43, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_VOICE_LIST');
  static const RpcMethod RPC_METHOD_SERVER_VOICE_GET =
      RpcMethod._(44, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_VOICE_GET');
  static const RpcMethod RPC_METHOD_SERVER_CREDENTIAL_LIST = RpcMethod._(
      45, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_CREDENTIAL_LIST');
  static const RpcMethod RPC_METHOD_SERVER_CREDENTIAL_GET =
      RpcMethod._(46, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_CREDENTIAL_GET');
  static const RpcMethod RPC_METHOD_SERVER_CREDENTIAL_CREATE = RpcMethod._(
      47, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_CREDENTIAL_CREATE');
  static const RpcMethod RPC_METHOD_SERVER_CREDENTIAL_PUT =
      RpcMethod._(48, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_CREDENTIAL_PUT');
  static const RpcMethod RPC_METHOD_SERVER_CREDENTIAL_DELETE = RpcMethod._(
      49, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_CREDENTIAL_DELETE');
  static const RpcMethod RPC_METHOD_SERVER_CONTACT_LIST =
      RpcMethod._(50, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_CONTACT_LIST');
  static const RpcMethod RPC_METHOD_SERVER_CONTACT_GET =
      RpcMethod._(51, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_CONTACT_GET');
  static const RpcMethod RPC_METHOD_SERVER_CONTACT_CREATE =
      RpcMethod._(52, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_CONTACT_CREATE');
  static const RpcMethod RPC_METHOD_SERVER_CONTACT_PUT =
      RpcMethod._(53, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_CONTACT_PUT');
  static const RpcMethod RPC_METHOD_SERVER_CONTACT_DELETE =
      RpcMethod._(54, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_CONTACT_DELETE');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_GET = RpcMethod
      ._(55, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_GET');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CREATE =
      RpcMethod._(56,
          _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CREATE');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CLEAR =
      RpcMethod._(57,
          _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CLEAR');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_ADD =
      RpcMethod._(58, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_ADD');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_LIST =
      RpcMethod._(59, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_LIST');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_DELETE =
      RpcMethod._(60, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_DELETE');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_LIST = RpcMethod._(
      61, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_GROUP_LIST');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_GET = RpcMethod._(
      62, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_GROUP_GET');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_CREATE = RpcMethod._(
      63, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_GROUP_CREATE');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_PUT = RpcMethod._(
      64, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_GROUP_PUT');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_DELETE = RpcMethod._(
      65, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_GROUP_DELETE');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_GET =
      RpcMethod._(
          66,
          _omitEnumNames
              ? ''
              : 'RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_GET');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CREATE =
      RpcMethod._(
          67,
          _omitEnumNames
              ? ''
              : 'RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CREATE');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CLEAR =
      RpcMethod._(
          68,
          _omitEnumNames
              ? ''
              : 'RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CLEAR');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_JOIN = RpcMethod._(
      69, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_GROUP_JOIN');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_LIST =
      RpcMethod._(70,
          _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_LIST');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_ADD =
      RpcMethod._(71,
          _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_ADD');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_PUT =
      RpcMethod._(72,
          _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_PUT');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_DELETE =
      RpcMethod._(
          73,
          _omitEnumNames
              ? ''
              : 'RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_DELETE');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_LIST =
      RpcMethod._(74,
          _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_LIST');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_GET =
      RpcMethod._(75,
          _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_GET');
  static const RpcMethod RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_SEND =
      RpcMethod._(76,
          _omitEnumNames ? '' : 'RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_SEND');
  static const RpcMethod RPC_METHOD_SERVER_GAME_RULESET_GET = RpcMethod._(
      77, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_GAME_RULESET_GET');
  static const RpcMethod RPC_METHOD_SERVER_BADGE_DEF_PIXA_DOWNLOAD = RpcMethod
      ._(79, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_BADGE_DEF_PIXA_DOWNLOAD');
  static const RpcMethod RPC_METHOD_SERVER_PET_LIST =
      RpcMethod._(80, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_PET_LIST');
  static const RpcMethod RPC_METHOD_SERVER_PET_GET =
      RpcMethod._(81, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_PET_GET');
  static const RpcMethod RPC_METHOD_SERVER_PET_ADOPT =
      RpcMethod._(82, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_PET_ADOPT');
  static const RpcMethod RPC_METHOD_SERVER_PET_PUT =
      RpcMethod._(83, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_PET_PUT');
  static const RpcMethod RPC_METHOD_SERVER_PET_DELETE =
      RpcMethod._(84, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_PET_DELETE');
  static const RpcMethod RPC_METHOD_SERVER_PET_DRIVE =
      RpcMethod._(85, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_PET_DRIVE');
  static const RpcMethod RPC_METHOD_SERVER_POINTS_GET =
      RpcMethod._(86, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_POINTS_GET');
  static const RpcMethod RPC_METHOD_SERVER_POINTS_TRANSACTIONS_LIST =
      RpcMethod._(87,
          _omitEnumNames ? '' : 'RPC_METHOD_SERVER_POINTS_TRANSACTIONS_LIST');
  static const RpcMethod RPC_METHOD_SERVER_POINTS_TRANSACTIONS_GET = RpcMethod
      ._(88, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_POINTS_TRANSACTIONS_GET');
  static const RpcMethod RPC_METHOD_SERVER_BADGE_LIST =
      RpcMethod._(89, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_BADGE_LIST');
  static const RpcMethod RPC_METHOD_SERVER_BADGE_GET =
      RpcMethod._(90, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_BADGE_GET');
  static const RpcMethod RPC_METHOD_SERVER_GAME_RESULT_LIST = RpcMethod._(
      91, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_GAME_RESULT_LIST');
  static const RpcMethod RPC_METHOD_SERVER_GAME_RESULT_GET = RpcMethod._(
      92, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_GAME_RESULT_GET');
  static const RpcMethod RPC_METHOD_SERVER_REWARD_GRANT_LIST = RpcMethod._(
      93, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_REWARD_GRANT_LIST');
  static const RpcMethod RPC_METHOD_SERVER_REWARD_GRANT_GET = RpcMethod._(
      94, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_REWARD_GRANT_GET');
  static const RpcMethod RPC_METHOD_SERVER_TOOL_LIST =
      RpcMethod._(95, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_TOOL_LIST');
  static const RpcMethod RPC_METHOD_SERVER_TOOL_GET =
      RpcMethod._(96, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_TOOL_GET');
  static const RpcMethod RPC_METHOD_SERVER_TOOL_CREATE =
      RpcMethod._(97, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_TOOL_CREATE');
  static const RpcMethod RPC_METHOD_SERVER_TOOL_PUT =
      RpcMethod._(98, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_TOOL_PUT');
  static const RpcMethod RPC_METHOD_SERVER_TOOL_DELETE =
      RpcMethod._(99, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_TOOL_DELETE');
  static const RpcMethod RPC_METHOD_CLIENT_TOOL_INVOKE =
      RpcMethod._(100, _omitEnumNames ? '' : 'RPC_METHOD_CLIENT_TOOL_INVOKE');
  static const RpcMethod RPC_METHOD_SERVER_PEER_LOOKUP =
      RpcMethod._(101, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_PEER_LOOKUP');
  static const RpcMethod RPC_METHOD_SERVER_PEER_ASSIGN =
      RpcMethod._(102, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_PEER_ASSIGN');
  static const RpcMethod RPC_METHOD_SERVER_ROUTE_RESOLVE =
      RpcMethod._(103, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_ROUTE_RESOLVE');
  static const RpcMethod RPC_METHOD_SERVER_PET_ACTIONS_GET = RpcMethod._(
      104, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_PET_ACTIONS_GET');
  static const RpcMethod RPC_METHOD_SERVER_PET_PIXA_DOWNLOAD = RpcMethod._(
      105, _omitEnumNames ? '' : 'RPC_METHOD_SERVER_PET_PIXA_DOWNLOAD');

  static const $core.List<RpcMethod> values = <RpcMethod>[
    RPC_METHOD_UNSPECIFIED,
    RPC_METHOD_ALL_PING,
    RPC_METHOD_ALL_SPEED_TEST_RUN,
    RPC_METHOD_CLIENT_INFO_GET,
    RPC_METHOD_CLIENT_IDENTIFIERS_GET,
    RPC_METHOD_SERVER_INFO_GET,
    RPC_METHOD_SERVER_INFO_PUT,
    RPC_METHOD_SERVER_RUNTIME_GET,
    RPC_METHOD_SERVER_STATUS_GET,
    RPC_METHOD_SERVER_RUN_AGENT_GET,
    RPC_METHOD_SERVER_RUN_AGENT_SET,
    RPC_METHOD_SERVER_RUN_WORKSPACE_GET,
    RPC_METHOD_SERVER_RUN_WORKSPACE_SET,
    RPC_METHOD_SERVER_RUN_WORKSPACE_RELOAD,
    RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY,
    RPC_METHOD_SERVER_RUN_WORKSPACE_HISTORY_PLAY,
    RPC_METHOD_SERVER_RUN_WORKSPACE_MEMORY_STATS,
    RPC_METHOD_SERVER_RUN_WORKSPACE_RECALL,
    RPC_METHOD_SERVER_RUN_RELOAD,
    RPC_METHOD_SERVER_RUN_STATUS,
    RPC_METHOD_SERVER_RUN_STOP,
    RPC_METHOD_SERVER_RUN_SAY,
    RPC_METHOD_SERVER_FIRMWARE_LIST,
    RPC_METHOD_SERVER_FIRMWARE_GET,
    RPC_METHOD_SERVER_FIRMWARE_FILES_DOWNLOAD,
    RPC_METHOD_SERVER_WORKSPACE_LIST,
    RPC_METHOD_SERVER_WORKSPACE_GET,
    RPC_METHOD_SERVER_WORKSPACE_CREATE,
    RPC_METHOD_SERVER_WORKSPACE_PUT,
    RPC_METHOD_SERVER_WORKSPACE_DELETE,
    RPC_METHOD_SERVER_WORKSPACE_HISTORY_LIST,
    RPC_METHOD_SERVER_WORKSPACE_HISTORY_GET,
    RPC_METHOD_SERVER_WORKSPACE_HISTORY_AUDIO_GET,
    RPC_METHOD_SERVER_WORKFLOW_LIST,
    RPC_METHOD_SERVER_WORKFLOW_GET,
    RPC_METHOD_SERVER_WORKFLOW_CREATE,
    RPC_METHOD_SERVER_WORKFLOW_PUT,
    RPC_METHOD_SERVER_WORKFLOW_DELETE,
    RPC_METHOD_SERVER_MODEL_LIST,
    RPC_METHOD_SERVER_MODEL_GET,
    RPC_METHOD_SERVER_MODEL_CREATE,
    RPC_METHOD_SERVER_MODEL_PUT,
    RPC_METHOD_SERVER_MODEL_DELETE,
    RPC_METHOD_SERVER_VOICE_LIST,
    RPC_METHOD_SERVER_VOICE_GET,
    RPC_METHOD_SERVER_CREDENTIAL_LIST,
    RPC_METHOD_SERVER_CREDENTIAL_GET,
    RPC_METHOD_SERVER_CREDENTIAL_CREATE,
    RPC_METHOD_SERVER_CREDENTIAL_PUT,
    RPC_METHOD_SERVER_CREDENTIAL_DELETE,
    RPC_METHOD_SERVER_CONTACT_LIST,
    RPC_METHOD_SERVER_CONTACT_GET,
    RPC_METHOD_SERVER_CONTACT_CREATE,
    RPC_METHOD_SERVER_CONTACT_PUT,
    RPC_METHOD_SERVER_CONTACT_DELETE,
    RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_GET,
    RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CREATE,
    RPC_METHOD_SERVER_FRIEND_INVITE_TOKEN_CLEAR,
    RPC_METHOD_SERVER_FRIEND_ADD,
    RPC_METHOD_SERVER_FRIEND_LIST,
    RPC_METHOD_SERVER_FRIEND_DELETE,
    RPC_METHOD_SERVER_FRIEND_GROUP_LIST,
    RPC_METHOD_SERVER_FRIEND_GROUP_GET,
    RPC_METHOD_SERVER_FRIEND_GROUP_CREATE,
    RPC_METHOD_SERVER_FRIEND_GROUP_PUT,
    RPC_METHOD_SERVER_FRIEND_GROUP_DELETE,
    RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_GET,
    RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CREATE,
    RPC_METHOD_SERVER_FRIEND_GROUP_INVITE_TOKEN_CLEAR,
    RPC_METHOD_SERVER_FRIEND_GROUP_JOIN,
    RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_LIST,
    RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_ADD,
    RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_PUT,
    RPC_METHOD_SERVER_FRIEND_GROUP_MEMBERS_DELETE,
    RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_LIST,
    RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_GET,
    RPC_METHOD_SERVER_FRIEND_GROUP_MESSAGES_SEND,
    RPC_METHOD_SERVER_GAME_RULESET_GET,
    RPC_METHOD_SERVER_BADGE_DEF_PIXA_DOWNLOAD,
    RPC_METHOD_SERVER_PET_LIST,
    RPC_METHOD_SERVER_PET_GET,
    RPC_METHOD_SERVER_PET_ADOPT,
    RPC_METHOD_SERVER_PET_PUT,
    RPC_METHOD_SERVER_PET_DELETE,
    RPC_METHOD_SERVER_PET_DRIVE,
    RPC_METHOD_SERVER_POINTS_GET,
    RPC_METHOD_SERVER_POINTS_TRANSACTIONS_LIST,
    RPC_METHOD_SERVER_POINTS_TRANSACTIONS_GET,
    RPC_METHOD_SERVER_BADGE_LIST,
    RPC_METHOD_SERVER_BADGE_GET,
    RPC_METHOD_SERVER_GAME_RESULT_LIST,
    RPC_METHOD_SERVER_GAME_RESULT_GET,
    RPC_METHOD_SERVER_REWARD_GRANT_LIST,
    RPC_METHOD_SERVER_REWARD_GRANT_GET,
    RPC_METHOD_SERVER_TOOL_LIST,
    RPC_METHOD_SERVER_TOOL_GET,
    RPC_METHOD_SERVER_TOOL_CREATE,
    RPC_METHOD_SERVER_TOOL_PUT,
    RPC_METHOD_SERVER_TOOL_DELETE,
    RPC_METHOD_CLIENT_TOOL_INVOKE,
    RPC_METHOD_SERVER_PEER_LOOKUP,
    RPC_METHOD_SERVER_PEER_ASSIGN,
    RPC_METHOD_SERVER_ROUTE_RESOLVE,
    RPC_METHOD_SERVER_PET_ACTIONS_GET,
    RPC_METHOD_SERVER_PET_PIXA_DOWNLOAD,
  ];

  static final $core.List<RpcMethod?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 105);
  static RpcMethod? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const RpcMethod._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
