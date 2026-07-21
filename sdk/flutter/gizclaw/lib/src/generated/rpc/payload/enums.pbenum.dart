// This is a generated file - do not edit.
//
// Generated from payload/enums.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:protobuf/protobuf.dart' as $pb;

class IconFormat extends $pb.ProtobufEnum {
  static const IconFormat ICON_FORMAT_UNSPECIFIED =
      IconFormat._(0, _omitEnumNames ? '' : 'ICON_FORMAT_UNSPECIFIED');
  static const IconFormat ICON_FORMAT_PIXA =
      IconFormat._(1, _omitEnumNames ? '' : 'ICON_FORMAT_PIXA');
  static const IconFormat ICON_FORMAT_PNG =
      IconFormat._(2, _omitEnumNames ? '' : 'ICON_FORMAT_PNG');

  static const $core.List<IconFormat> values = <IconFormat>[
    ICON_FORMAT_UNSPECIFIED,
    ICON_FORMAT_PIXA,
    ICON_FORMAT_PNG,
  ];

  static final $core.List<IconFormat?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static IconFormat? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const IconFormat._(super.value, super.name);
}

class ASTTranslateMode extends $pb.ProtobufEnum {
  static const ASTTranslateMode ASTTRANSLATE_MODE_UNSPECIFIED =
      ASTTranslateMode._(
          0, _omitEnumNames ? '' : 'ASTTRANSLATE_MODE_UNSPECIFIED');
  static const ASTTranslateMode ASTTRANSLATE_MODE_S2T =
      ASTTranslateMode._(1, _omitEnumNames ? '' : 'ASTTRANSLATE_MODE_S2T');
  static const ASTTranslateMode ASTTRANSLATE_MODE_S2S =
      ASTTranslateMode._(2, _omitEnumNames ? '' : 'ASTTRANSLATE_MODE_S2S');

  static const $core.List<ASTTranslateMode> values = <ASTTranslateMode>[
    ASTTRANSLATE_MODE_UNSPECIFIED,
    ASTTRANSLATE_MODE_S2T,
    ASTTRANSLATE_MODE_S2S,
  ];

  static final $core.List<ASTTranslateMode?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static ASTTranslateMode? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ASTTranslateMode._(super.value, super.name);
}

class ASTTranslateWorkspaceParametersAgentType extends $pb.ProtobufEnum {
  static const ASTTranslateWorkspaceParametersAgentType
      ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED =
      ASTTranslateWorkspaceParametersAgentType._(
          0,
          _omitEnumNames
              ? ''
              : 'ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED');
  static const ASTTranslateWorkspaceParametersAgentType
      ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_AST_TRANSLATE =
      ASTTranslateWorkspaceParametersAgentType._(
          1,
          _omitEnumNames
              ? ''
              : 'ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_AST_TRANSLATE');

  static const $core.List<ASTTranslateWorkspaceParametersAgentType> values =
      <ASTTranslateWorkspaceParametersAgentType>[
    ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED,
    ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_AST_TRANSLATE,
  ];

  static final $core.List<ASTTranslateWorkspaceParametersAgentType?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 1);
  static ASTTranslateWorkspaceParametersAgentType? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ASTTranslateWorkspaceParametersAgentType._(super.value, super.name);
}

class ChatRoomMode extends $pb.ProtobufEnum {
  static const ChatRoomMode CHAT_ROOM_MODE_UNSPECIFIED =
      ChatRoomMode._(0, _omitEnumNames ? '' : 'CHAT_ROOM_MODE_UNSPECIFIED');
  static const ChatRoomMode CHAT_ROOM_MODE_DIRECT =
      ChatRoomMode._(1, _omitEnumNames ? '' : 'CHAT_ROOM_MODE_DIRECT');
  static const ChatRoomMode CHAT_ROOM_MODE_GROUP =
      ChatRoomMode._(2, _omitEnumNames ? '' : 'CHAT_ROOM_MODE_GROUP');

  static const $core.List<ChatRoomMode> values = <ChatRoomMode>[
    CHAT_ROOM_MODE_UNSPECIFIED,
    CHAT_ROOM_MODE_DIRECT,
    CHAT_ROOM_MODE_GROUP,
  ];

  static final $core.List<ChatRoomMode?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static ChatRoomMode? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ChatRoomMode._(super.value, super.name);
}

class ChatRoomWorkspaceParametersAgentType extends $pb.ProtobufEnum {
  static const ChatRoomWorkspaceParametersAgentType
      CHAT_ROOM_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED =
      ChatRoomWorkspaceParametersAgentType._(
          0,
          _omitEnumNames
              ? ''
              : 'CHAT_ROOM_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED');
  static const ChatRoomWorkspaceParametersAgentType
      CHAT_ROOM_WORKSPACE_PARAMETERS_AGENT_TYPE_CHATROOM =
      ChatRoomWorkspaceParametersAgentType._(
          1,
          _omitEnumNames
              ? ''
              : 'CHAT_ROOM_WORKSPACE_PARAMETERS_AGENT_TYPE_CHATROOM');

  static const $core.List<ChatRoomWorkspaceParametersAgentType> values =
      <ChatRoomWorkspaceParametersAgentType>[
    CHAT_ROOM_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED,
    CHAT_ROOM_WORKSPACE_PARAMETERS_AGENT_TYPE_CHATROOM,
  ];

  static final $core.List<ChatRoomWorkspaceParametersAgentType?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 1);
  static ChatRoomWorkspaceParametersAgentType? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ChatRoomWorkspaceParametersAgentType._(super.value, super.name);
}

class DashScopeTenantModelProviderDataApiMode extends $pb.ProtobufEnum {
  static const DashScopeTenantModelProviderDataApiMode
      DASH_SCOPE_TENANT_MODEL_PROVIDER_DATA_API_MODE_UNSPECIFIED =
      DashScopeTenantModelProviderDataApiMode._(
          0,
          _omitEnumNames
              ? ''
              : 'DASH_SCOPE_TENANT_MODEL_PROVIDER_DATA_API_MODE_UNSPECIFIED');
  static const DashScopeTenantModelProviderDataApiMode
      DASH_SCOPE_TENANT_MODEL_PROVIDER_DATA_API_MODE_CHAT_COMPLETIONS =
      DashScopeTenantModelProviderDataApiMode._(
          1,
          _omitEnumNames
              ? ''
              : 'DASH_SCOPE_TENANT_MODEL_PROVIDER_DATA_API_MODE_CHAT_COMPLETIONS');
  static const DashScopeTenantModelProviderDataApiMode
      DASH_SCOPE_TENANT_MODEL_PROVIDER_DATA_API_MODE_REALTIME =
      DashScopeTenantModelProviderDataApiMode._(
          2,
          _omitEnumNames
              ? ''
              : 'DASH_SCOPE_TENANT_MODEL_PROVIDER_DATA_API_MODE_REALTIME');

  static const $core.List<DashScopeTenantModelProviderDataApiMode> values =
      <DashScopeTenantModelProviderDataApiMode>[
    DASH_SCOPE_TENANT_MODEL_PROVIDER_DATA_API_MODE_UNSPECIFIED,
    DASH_SCOPE_TENANT_MODEL_PROVIDER_DATA_API_MODE_CHAT_COMPLETIONS,
    DASH_SCOPE_TENANT_MODEL_PROVIDER_DATA_API_MODE_REALTIME,
  ];

  static final $core.List<DashScopeTenantModelProviderDataApiMode?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static DashScopeTenantModelProviderDataApiMode? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const DashScopeTenantModelProviderDataApiMode._(super.value, super.name);
}

class DoubaoRealtimeAudioFormatType extends $pb.ProtobufEnum {
  static const DoubaoRealtimeAudioFormatType
      DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_UNSPECIFIED =
      DoubaoRealtimeAudioFormatType._(
          0,
          _omitEnumNames
              ? ''
              : 'DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_UNSPECIFIED');
  static const DoubaoRealtimeAudioFormatType
      DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_PCM = DoubaoRealtimeAudioFormatType._(
          1, _omitEnumNames ? '' : 'DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_PCM');
  static const DoubaoRealtimeAudioFormatType
      DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_PCM_S16LE =
      DoubaoRealtimeAudioFormatType._(2,
          _omitEnumNames ? '' : 'DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_PCM_S16LE');
  static const DoubaoRealtimeAudioFormatType
      DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_SPEECH_OPUS =
      DoubaoRealtimeAudioFormatType._(
          3,
          _omitEnumNames
              ? ''
              : 'DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_SPEECH_OPUS');
  static const DoubaoRealtimeAudioFormatType
      DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_OGG_OPUS =
      DoubaoRealtimeAudioFormatType._(4,
          _omitEnumNames ? '' : 'DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_OGG_OPUS');

  static const $core.List<DoubaoRealtimeAudioFormatType> values =
      <DoubaoRealtimeAudioFormatType>[
    DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_UNSPECIFIED,
    DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_PCM,
    DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_PCM_S16LE,
    DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_SPEECH_OPUS,
    DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_OGG_OPUS,
  ];

  static final $core.List<DoubaoRealtimeAudioFormatType?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 4);
  static DoubaoRealtimeAudioFormatType? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const DoubaoRealtimeAudioFormatType._(super.value, super.name);
}

class DoubaoRealtimeDialogExtraVolcWebsearchType extends $pb.ProtobufEnum {
  static const DoubaoRealtimeDialogExtraVolcWebsearchType
      DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_UNSPECIFIED =
      DoubaoRealtimeDialogExtraVolcWebsearchType._(
          0,
          _omitEnumNames
              ? ''
              : 'DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_UNSPECIFIED');
  static const DoubaoRealtimeDialogExtraVolcWebsearchType
      DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_WEB =
      DoubaoRealtimeDialogExtraVolcWebsearchType._(
          1,
          _omitEnumNames
              ? ''
              : 'DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_WEB');
  static const DoubaoRealtimeDialogExtraVolcWebsearchType
      DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_WEB_SUMMARY =
      DoubaoRealtimeDialogExtraVolcWebsearchType._(
          2,
          _omitEnumNames
              ? ''
              : 'DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_WEB_SUMMARY');
  static const DoubaoRealtimeDialogExtraVolcWebsearchType
      DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_WEB_AGENT =
      DoubaoRealtimeDialogExtraVolcWebsearchType._(
          3,
          _omitEnumNames
              ? ''
              : 'DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_WEB_AGENT');

  static const $core.List<DoubaoRealtimeDialogExtraVolcWebsearchType> values =
      <DoubaoRealtimeDialogExtraVolcWebsearchType>[
    DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_UNSPECIFIED,
    DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_WEB,
    DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_WEB_SUMMARY,
    DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_WEB_AGENT,
  ];

  static final $core.List<DoubaoRealtimeDialogExtraVolcWebsearchType?>
      _byValue = $pb.ProtobufEnum.$_initByValueList(values, 3);
  static DoubaoRealtimeDialogExtraVolcWebsearchType? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const DoubaoRealtimeDialogExtraVolcWebsearchType._(super.value, super.name);
}

class DoubaoRealtimeFunctionToolType extends $pb.ProtobufEnum {
  static const DoubaoRealtimeFunctionToolType
      DOUBAO_REALTIME_FUNCTION_TOOL_TYPE_UNSPECIFIED =
      DoubaoRealtimeFunctionToolType._(
          0,
          _omitEnumNames
              ? ''
              : 'DOUBAO_REALTIME_FUNCTION_TOOL_TYPE_UNSPECIFIED');
  static const DoubaoRealtimeFunctionToolType
      DOUBAO_REALTIME_FUNCTION_TOOL_TYPE_FUNCTION =
      DoubaoRealtimeFunctionToolType._(1,
          _omitEnumNames ? '' : 'DOUBAO_REALTIME_FUNCTION_TOOL_TYPE_FUNCTION');

  static const $core.List<DoubaoRealtimeFunctionToolType> values =
      <DoubaoRealtimeFunctionToolType>[
    DOUBAO_REALTIME_FUNCTION_TOOL_TYPE_UNSPECIFIED,
    DOUBAO_REALTIME_FUNCTION_TOOL_TYPE_FUNCTION,
  ];

  static final $core.List<DoubaoRealtimeFunctionToolType?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 1);
  static DoubaoRealtimeFunctionToolType? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const DoubaoRealtimeFunctionToolType._(super.value, super.name);
}

class DoubaoRealtimeWorkspaceParametersAgentType extends $pb.ProtobufEnum {
  static const DoubaoRealtimeWorkspaceParametersAgentType
      DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED =
      DoubaoRealtimeWorkspaceParametersAgentType._(
          0,
          _omitEnumNames
              ? ''
              : 'DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED');
  static const DoubaoRealtimeWorkspaceParametersAgentType
      DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_DOUBAO_REALTIME =
      DoubaoRealtimeWorkspaceParametersAgentType._(
          1,
          _omitEnumNames
              ? ''
              : 'DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_DOUBAO_REALTIME');

  static const $core.List<DoubaoRealtimeWorkspaceParametersAgentType> values =
      <DoubaoRealtimeWorkspaceParametersAgentType>[
    DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED,
    DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_DOUBAO_REALTIME,
  ];

  static final $core.List<DoubaoRealtimeWorkspaceParametersAgentType?>
      _byValue = $pb.ProtobufEnum.$_initByValueList(values, 1);
  static DoubaoRealtimeWorkspaceParametersAgentType? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const DoubaoRealtimeWorkspaceParametersAgentType._(super.value, super.name);
}

class FirmwareArtifactEntryType extends $pb.ProtobufEnum {
  static const FirmwareArtifactEntryType
      FIRMWARE_ARTIFACT_ENTRY_TYPE_UNSPECIFIED = FirmwareArtifactEntryType._(
          0, _omitEnumNames ? '' : 'FIRMWARE_ARTIFACT_ENTRY_TYPE_UNSPECIFIED');
  static const FirmwareArtifactEntryType FIRMWARE_ARTIFACT_ENTRY_TYPE_FILE =
      FirmwareArtifactEntryType._(
          1, _omitEnumNames ? '' : 'FIRMWARE_ARTIFACT_ENTRY_TYPE_FILE');
  static const FirmwareArtifactEntryType FIRMWARE_ARTIFACT_ENTRY_TYPE_DIR =
      FirmwareArtifactEntryType._(
          2, _omitEnumNames ? '' : 'FIRMWARE_ARTIFACT_ENTRY_TYPE_DIR');

  static const $core.List<FirmwareArtifactEntryType> values =
      <FirmwareArtifactEntryType>[
    FIRMWARE_ARTIFACT_ENTRY_TYPE_UNSPECIFIED,
    FIRMWARE_ARTIFACT_ENTRY_TYPE_FILE,
    FIRMWARE_ARTIFACT_ENTRY_TYPE_DIR,
  ];

  static final $core.List<FirmwareArtifactEntryType?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static FirmwareArtifactEntryType? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const FirmwareArtifactEntryType._(super.value, super.name);
}

class FirmwareChannelName extends $pb.ProtobufEnum {
  static const FirmwareChannelName FIRMWARE_CHANNEL_NAME_UNSPECIFIED =
      FirmwareChannelName._(
          0, _omitEnumNames ? '' : 'FIRMWARE_CHANNEL_NAME_UNSPECIFIED');
  static const FirmwareChannelName FIRMWARE_CHANNEL_NAME_STABLE =
      FirmwareChannelName._(
          1, _omitEnumNames ? '' : 'FIRMWARE_CHANNEL_NAME_STABLE');
  static const FirmwareChannelName FIRMWARE_CHANNEL_NAME_BETA =
      FirmwareChannelName._(
          2, _omitEnumNames ? '' : 'FIRMWARE_CHANNEL_NAME_BETA');
  static const FirmwareChannelName FIRMWARE_CHANNEL_NAME_DEVELOP =
      FirmwareChannelName._(
          3, _omitEnumNames ? '' : 'FIRMWARE_CHANNEL_NAME_DEVELOP');
  static const FirmwareChannelName FIRMWARE_CHANNEL_NAME_PENDING =
      FirmwareChannelName._(
          4, _omitEnumNames ? '' : 'FIRMWARE_CHANNEL_NAME_PENDING');

  static const $core.List<FirmwareChannelName> values = <FirmwareChannelName>[
    FIRMWARE_CHANNEL_NAME_UNSPECIFIED,
    FIRMWARE_CHANNEL_NAME_STABLE,
    FIRMWARE_CHANNEL_NAME_BETA,
    FIRMWARE_CHANNEL_NAME_DEVELOP,
    FIRMWARE_CHANNEL_NAME_PENDING,
  ];

  static final $core.List<FirmwareChannelName?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 4);
  static FirmwareChannelName? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const FirmwareChannelName._(super.value, super.name);
}

class FlowcraftConversationParametersAgentInitiativePolicy
    extends $pb.ProtobufEnum {
  static const FlowcraftConversationParametersAgentInitiativePolicy
      FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_UNSPECIFIED =
      FlowcraftConversationParametersAgentInitiativePolicy._(
          0,
          _omitEnumNames
              ? ''
              : 'FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_UNSPECIFIED');
  static const FlowcraftConversationParametersAgentInitiativePolicy
      FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_ONCE_WHEN_EMPTY =
      FlowcraftConversationParametersAgentInitiativePolicy._(
          1,
          _omitEnumNames
              ? ''
              : 'FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_ONCE_WHEN_EMPTY');
  static const FlowcraftConversationParametersAgentInitiativePolicy
      FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_ON_RELOAD =
      FlowcraftConversationParametersAgentInitiativePolicy._(
          2,
          _omitEnumNames
              ? ''
              : 'FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_ON_RELOAD');

  static const $core.List<FlowcraftConversationParametersAgentInitiativePolicy>
      values = <FlowcraftConversationParametersAgentInitiativePolicy>[
    FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_UNSPECIFIED,
    FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_ONCE_WHEN_EMPTY,
    FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_ON_RELOAD,
  ];

  static final $core.List<FlowcraftConversationParametersAgentInitiativePolicy?>
      _byValue = $pb.ProtobufEnum.$_initByValueList(values, 2);
  static FlowcraftConversationParametersAgentInitiativePolicy? valueOf(
          $core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const FlowcraftConversationParametersAgentInitiativePolicy._(
      super.value, super.name);
}

class FlowcraftConversationParametersInitiative extends $pb.ProtobufEnum {
  static const FlowcraftConversationParametersInitiative
      FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_UNSPECIFIED =
      FlowcraftConversationParametersInitiative._(
          0,
          _omitEnumNames
              ? ''
              : 'FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_UNSPECIFIED');
  static const FlowcraftConversationParametersInitiative
      FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_PEER =
      FlowcraftConversationParametersInitiative._(
          1,
          _omitEnumNames
              ? ''
              : 'FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_PEER');
  static const FlowcraftConversationParametersInitiative
      FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_AGENT =
      FlowcraftConversationParametersInitiative._(
          2,
          _omitEnumNames
              ? ''
              : 'FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_AGENT');

  static const $core.List<FlowcraftConversationParametersInitiative> values =
      <FlowcraftConversationParametersInitiative>[
    FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_UNSPECIFIED,
    FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_PEER,
    FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_AGENT,
  ];

  static final $core.List<FlowcraftConversationParametersInitiative?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static FlowcraftConversationParametersInitiative? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const FlowcraftConversationParametersInitiative._(super.value, super.name);
}

class FlowcraftWorkspaceParametersAgentType extends $pb.ProtobufEnum {
  static const FlowcraftWorkspaceParametersAgentType
      FLOWCRAFT_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED =
      FlowcraftWorkspaceParametersAgentType._(
          0,
          _omitEnumNames
              ? ''
              : 'FLOWCRAFT_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED');
  static const FlowcraftWorkspaceParametersAgentType
      FLOWCRAFT_WORKSPACE_PARAMETERS_AGENT_TYPE_FLOWCRAFT =
      FlowcraftWorkspaceParametersAgentType._(
          1,
          _omitEnumNames
              ? ''
              : 'FLOWCRAFT_WORKSPACE_PARAMETERS_AGENT_TYPE_FLOWCRAFT');

  static const $core.List<FlowcraftWorkspaceParametersAgentType> values =
      <FlowcraftWorkspaceParametersAgentType>[
    FLOWCRAFT_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED,
    FLOWCRAFT_WORKSPACE_PARAMETERS_AGENT_TYPE_FLOWCRAFT,
  ];

  static final $core.List<FlowcraftWorkspaceParametersAgentType?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 1);
  static FlowcraftWorkspaceParametersAgentType? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const FlowcraftWorkspaceParametersAgentType._(super.value, super.name);
}

class PetConversationParametersInitiative extends $pb.ProtobufEnum {
  static const PetConversationParametersInitiative
      PET_CONVERSATION_PARAMETERS_INITIATIVE_UNSPECIFIED =
      PetConversationParametersInitiative._(
          0,
          _omitEnumNames
              ? ''
              : 'PET_CONVERSATION_PARAMETERS_INITIATIVE_UNSPECIFIED');
  static const PetConversationParametersInitiative
      PET_CONVERSATION_PARAMETERS_INITIATIVE_PEER =
      PetConversationParametersInitiative._(1,
          _omitEnumNames ? '' : 'PET_CONVERSATION_PARAMETERS_INITIATIVE_PEER');
  static const PetConversationParametersInitiative
      PET_CONVERSATION_PARAMETERS_INITIATIVE_AGENT =
      PetConversationParametersInitiative._(2,
          _omitEnumNames ? '' : 'PET_CONVERSATION_PARAMETERS_INITIATIVE_AGENT');

  static const $core.List<PetConversationParametersInitiative> values =
      <PetConversationParametersInitiative>[
    PET_CONVERSATION_PARAMETERS_INITIATIVE_UNSPECIFIED,
    PET_CONVERSATION_PARAMETERS_INITIATIVE_PEER,
    PET_CONVERSATION_PARAMETERS_INITIATIVE_AGENT,
  ];

  static final $core.List<PetConversationParametersInitiative?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static PetConversationParametersInitiative? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const PetConversationParametersInitiative._(super.value, super.name);
}

class PetWorkspaceParametersAgentType extends $pb.ProtobufEnum {
  static const PetWorkspaceParametersAgentType
      PET_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED =
      PetWorkspaceParametersAgentType._(
          0,
          _omitEnumNames
              ? ''
              : 'PET_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED');
  static const PetWorkspaceParametersAgentType
      PET_WORKSPACE_PARAMETERS_AGENT_TYPE_PET =
      PetWorkspaceParametersAgentType._(
          1, _omitEnumNames ? '' : 'PET_WORKSPACE_PARAMETERS_AGENT_TYPE_PET');

  static const $core.List<PetWorkspaceParametersAgentType> values =
      <PetWorkspaceParametersAgentType>[
    PET_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED,
    PET_WORKSPACE_PARAMETERS_AGENT_TYPE_PET,
  ];

  static final $core.List<PetWorkspaceParametersAgentType?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 1);
  static PetWorkspaceParametersAgentType? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const PetWorkspaceParametersAgentType._(super.value, super.name);
}

class FriendGroupMemberMutableRole extends $pb.ProtobufEnum {
  static const FriendGroupMemberMutableRole
      FRIEND_GROUP_MEMBER_MUTABLE_ROLE_UNSPECIFIED =
      FriendGroupMemberMutableRole._(0,
          _omitEnumNames ? '' : 'FRIEND_GROUP_MEMBER_MUTABLE_ROLE_UNSPECIFIED');
  static const FriendGroupMemberMutableRole
      FRIEND_GROUP_MEMBER_MUTABLE_ROLE_ADMIN = FriendGroupMemberMutableRole._(
          1, _omitEnumNames ? '' : 'FRIEND_GROUP_MEMBER_MUTABLE_ROLE_ADMIN');
  static const FriendGroupMemberMutableRole
      FRIEND_GROUP_MEMBER_MUTABLE_ROLE_MEMBER = FriendGroupMemberMutableRole._(
          2, _omitEnumNames ? '' : 'FRIEND_GROUP_MEMBER_MUTABLE_ROLE_MEMBER');

  static const $core.List<FriendGroupMemberMutableRole> values =
      <FriendGroupMemberMutableRole>[
    FRIEND_GROUP_MEMBER_MUTABLE_ROLE_UNSPECIFIED,
    FRIEND_GROUP_MEMBER_MUTABLE_ROLE_ADMIN,
    FRIEND_GROUP_MEMBER_MUTABLE_ROLE_MEMBER,
  ];

  static final $core.List<FriendGroupMemberMutableRole?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static FriendGroupMemberMutableRole? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const FriendGroupMemberMutableRole._(super.value, super.name);
}

class FriendGroupMemberRole extends $pb.ProtobufEnum {
  static const FriendGroupMemberRole FRIEND_GROUP_MEMBER_ROLE_UNSPECIFIED =
      FriendGroupMemberRole._(
          0, _omitEnumNames ? '' : 'FRIEND_GROUP_MEMBER_ROLE_UNSPECIFIED');
  static const FriendGroupMemberRole FRIEND_GROUP_MEMBER_ROLE_OWNER =
      FriendGroupMemberRole._(
          1, _omitEnumNames ? '' : 'FRIEND_GROUP_MEMBER_ROLE_OWNER');
  static const FriendGroupMemberRole FRIEND_GROUP_MEMBER_ROLE_ADMIN =
      FriendGroupMemberRole._(
          2, _omitEnumNames ? '' : 'FRIEND_GROUP_MEMBER_ROLE_ADMIN');
  static const FriendGroupMemberRole FRIEND_GROUP_MEMBER_ROLE_MEMBER =
      FriendGroupMemberRole._(
          3, _omitEnumNames ? '' : 'FRIEND_GROUP_MEMBER_ROLE_MEMBER');

  static const $core.List<FriendGroupMemberRole> values =
      <FriendGroupMemberRole>[
    FRIEND_GROUP_MEMBER_ROLE_UNSPECIFIED,
    FRIEND_GROUP_MEMBER_ROLE_OWNER,
    FRIEND_GROUP_MEMBER_ROLE_ADMIN,
    FRIEND_GROUP_MEMBER_ROLE_MEMBER,
  ];

  static final $core.List<FriendGroupMemberRole?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 3);
  static FriendGroupMemberRole? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const FriendGroupMemberRole._(super.value, super.name);
}

class PeerRole extends $pb.ProtobufEnum {
  static const PeerRole PEER_ROLE_UNSPECIFIED =
      PeerRole._(0, _omitEnumNames ? '' : 'PEER_ROLE_UNSPECIFIED');
  static const PeerRole PEER_ROLE_ADMIN =
      PeerRole._(1, _omitEnumNames ? '' : 'PEER_ROLE_ADMIN');
  static const PeerRole PEER_ROLE_SERVER =
      PeerRole._(2, _omitEnumNames ? '' : 'PEER_ROLE_SERVER');
  static const PeerRole PEER_ROLE_EDGE_NODE =
      PeerRole._(3, _omitEnumNames ? '' : 'PEER_ROLE_EDGE_NODE');
  static const PeerRole PEER_ROLE_CLIENT =
      PeerRole._(4, _omitEnumNames ? '' : 'PEER_ROLE_CLIENT');

  static const $core.List<PeerRole> values = <PeerRole>[
    PEER_ROLE_UNSPECIFIED,
    PEER_ROLE_ADMIN,
    PEER_ROLE_SERVER,
    PEER_ROLE_EDGE_NODE,
    PEER_ROLE_CLIENT,
  ];

  static final $core.List<PeerRole?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 4);
  static PeerRole? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const PeerRole._(super.value, super.name);
}

class ModelKind extends $pb.ProtobufEnum {
  static const ModelKind MODEL_KIND_UNSPECIFIED =
      ModelKind._(0, _omitEnumNames ? '' : 'MODEL_KIND_UNSPECIFIED');
  static const ModelKind MODEL_KIND_LLM =
      ModelKind._(1, _omitEnumNames ? '' : 'MODEL_KIND_LLM');
  static const ModelKind MODEL_KIND_TTS =
      ModelKind._(2, _omitEnumNames ? '' : 'MODEL_KIND_TTS');
  static const ModelKind MODEL_KIND_ASR =
      ModelKind._(3, _omitEnumNames ? '' : 'MODEL_KIND_ASR');
  static const ModelKind MODEL_KIND_REALTIME =
      ModelKind._(4, _omitEnumNames ? '' : 'MODEL_KIND_REALTIME');
  static const ModelKind MODEL_KIND_TRANSLATION =
      ModelKind._(5, _omitEnumNames ? '' : 'MODEL_KIND_TRANSLATION');
  static const ModelKind MODEL_KIND_EMBEDDING =
      ModelKind._(6, _omitEnumNames ? '' : 'MODEL_KIND_EMBEDDING');

  static const $core.List<ModelKind> values = <ModelKind>[
    MODEL_KIND_UNSPECIFIED,
    MODEL_KIND_LLM,
    MODEL_KIND_TTS,
    MODEL_KIND_ASR,
    MODEL_KIND_REALTIME,
    MODEL_KIND_TRANSLATION,
    MODEL_KIND_EMBEDDING,
  ];

  static final $core.List<ModelKind?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 6);
  static ModelKind? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const ModelKind._(super.value, super.name);
}

class PeerRunHistoryEntryType extends $pb.ProtobufEnum {
  static const PeerRunHistoryEntryType PEER_RUN_HISTORY_ENTRY_TYPE_UNSPECIFIED =
      PeerRunHistoryEntryType._(
          0, _omitEnumNames ? '' : 'PEER_RUN_HISTORY_ENTRY_TYPE_UNSPECIFIED');
  static const PeerRunHistoryEntryType PEER_RUN_HISTORY_ENTRY_TYPE_GEAR =
      PeerRunHistoryEntryType._(
          1, _omitEnumNames ? '' : 'PEER_RUN_HISTORY_ENTRY_TYPE_GEAR');
  static const PeerRunHistoryEntryType PEER_RUN_HISTORY_ENTRY_TYPE_AGENT =
      PeerRunHistoryEntryType._(
          2, _omitEnumNames ? '' : 'PEER_RUN_HISTORY_ENTRY_TYPE_AGENT');

  static const $core.List<PeerRunHistoryEntryType> values =
      <PeerRunHistoryEntryType>[
    PEER_RUN_HISTORY_ENTRY_TYPE_UNSPECIFIED,
    PEER_RUN_HISTORY_ENTRY_TYPE_GEAR,
    PEER_RUN_HISTORY_ENTRY_TYPE_AGENT,
  ];

  static final $core.List<PeerRunHistoryEntryType?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static PeerRunHistoryEntryType? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const PeerRunHistoryEntryType._(super.value, super.name);
}

class PeerRunHistoryListRequestOrder extends $pb.ProtobufEnum {
  static const PeerRunHistoryListRequestOrder
      PEER_RUN_HISTORY_LIST_REQUEST_ORDER_UNSPECIFIED =
      PeerRunHistoryListRequestOrder._(
          0,
          _omitEnumNames
              ? ''
              : 'PEER_RUN_HISTORY_LIST_REQUEST_ORDER_UNSPECIFIED');
  static const PeerRunHistoryListRequestOrder
      PEER_RUN_HISTORY_LIST_REQUEST_ORDER_ASC =
      PeerRunHistoryListRequestOrder._(
          1, _omitEnumNames ? '' : 'PEER_RUN_HISTORY_LIST_REQUEST_ORDER_ASC');
  static const PeerRunHistoryListRequestOrder
      PEER_RUN_HISTORY_LIST_REQUEST_ORDER_DESC =
      PeerRunHistoryListRequestOrder._(
          2, _omitEnumNames ? '' : 'PEER_RUN_HISTORY_LIST_REQUEST_ORDER_DESC');

  static const $core.List<PeerRunHistoryListRequestOrder> values =
      <PeerRunHistoryListRequestOrder>[
    PEER_RUN_HISTORY_LIST_REQUEST_ORDER_UNSPECIFIED,
    PEER_RUN_HISTORY_LIST_REQUEST_ORDER_ASC,
    PEER_RUN_HISTORY_LIST_REQUEST_ORDER_DESC,
  ];

  static final $core.List<PeerRunHistoryListRequestOrder?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static PeerRunHistoryListRequestOrder? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const PeerRunHistoryListRequestOrder._(super.value, super.name);
}

class PeerRunStatusState extends $pb.ProtobufEnum {
  static const PeerRunStatusState PEER_RUN_STATUS_STATE_UNSPECIFIED =
      PeerRunStatusState._(
          0, _omitEnumNames ? '' : 'PEER_RUN_STATUS_STATE_UNSPECIFIED');
  static const PeerRunStatusState PEER_RUN_STATUS_STATE_STOPPED =
      PeerRunStatusState._(
          1, _omitEnumNames ? '' : 'PEER_RUN_STATUS_STATE_STOPPED');
  static const PeerRunStatusState PEER_RUN_STATUS_STATE_STARTING =
      PeerRunStatusState._(
          2, _omitEnumNames ? '' : 'PEER_RUN_STATUS_STATE_STARTING');
  static const PeerRunStatusState PEER_RUN_STATUS_STATE_RUNNING =
      PeerRunStatusState._(
          3, _omitEnumNames ? '' : 'PEER_RUN_STATUS_STATE_RUNNING');
  static const PeerRunStatusState PEER_RUN_STATUS_STATE_STOPPING =
      PeerRunStatusState._(
          4, _omitEnumNames ? '' : 'PEER_RUN_STATUS_STATE_STOPPING');
  static const PeerRunStatusState PEER_RUN_STATUS_STATE_ERROR =
      PeerRunStatusState._(
          5, _omitEnumNames ? '' : 'PEER_RUN_STATUS_STATE_ERROR');

  static const $core.List<PeerRunStatusState> values = <PeerRunStatusState>[
    PEER_RUN_STATUS_STATE_UNSPECIFIED,
    PEER_RUN_STATUS_STATE_STOPPED,
    PEER_RUN_STATUS_STATE_STARTING,
    PEER_RUN_STATUS_STATE_RUNNING,
    PEER_RUN_STATUS_STATE_STOPPING,
    PEER_RUN_STATUS_STATE_ERROR,
  ];

  static final $core.List<PeerRunStatusState?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 5);
  static PeerRunStatusState? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const PeerRunStatusState._(super.value, super.name);
}

class VolcTenantModelProviderDataApiMode extends $pb.ProtobufEnum {
  static const VolcTenantModelProviderDataApiMode
      VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_UNSPECIFIED =
      VolcTenantModelProviderDataApiMode._(
          0,
          _omitEnumNames
              ? ''
              : 'VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_UNSPECIFIED');
  static const VolcTenantModelProviderDataApiMode
      VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_ASR =
      VolcTenantModelProviderDataApiMode._(1,
          _omitEnumNames ? '' : 'VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_ASR');
  static const VolcTenantModelProviderDataApiMode
      VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_TTS =
      VolcTenantModelProviderDataApiMode._(2,
          _omitEnumNames ? '' : 'VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_TTS');
  static const VolcTenantModelProviderDataApiMode
      VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_REALTIME =
      VolcTenantModelProviderDataApiMode._(
          3,
          _omitEnumNames
              ? ''
              : 'VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_REALTIME');

  static const $core.List<VolcTenantModelProviderDataApiMode> values =
      <VolcTenantModelProviderDataApiMode>[
    VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_UNSPECIFIED,
    VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_ASR,
    VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_TTS,
    VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_REALTIME,
  ];

  static final $core.List<VolcTenantModelProviderDataApiMode?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 3);
  static VolcTenantModelProviderDataApiMode? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const VolcTenantModelProviderDataApiMode._(super.value, super.name);
}

class WorkflowDriver extends $pb.ProtobufEnum {
  static const WorkflowDriver WORKFLOW_DRIVER_UNSPECIFIED =
      WorkflowDriver._(0, _omitEnumNames ? '' : 'WORKFLOW_DRIVER_UNSPECIFIED');
  static const WorkflowDriver WORKFLOW_DRIVER_FLOWCRAFT =
      WorkflowDriver._(1, _omitEnumNames ? '' : 'WORKFLOW_DRIVER_FLOWCRAFT');
  static const WorkflowDriver WORKFLOW_DRIVER_DOUBAO_REALTIME =
      WorkflowDriver._(
          2, _omitEnumNames ? '' : 'WORKFLOW_DRIVER_DOUBAO_REALTIME');
  static const WorkflowDriver WORKFLOW_DRIVER_AST_TRANSLATE = WorkflowDriver._(
      3, _omitEnumNames ? '' : 'WORKFLOW_DRIVER_AST_TRANSLATE');
  static const WorkflowDriver WORKFLOW_DRIVER_CHATROOM =
      WorkflowDriver._(4, _omitEnumNames ? '' : 'WORKFLOW_DRIVER_CHATROOM');
  static const WorkflowDriver WORKFLOW_DRIVER_PET =
      WorkflowDriver._(5, _omitEnumNames ? '' : 'WORKFLOW_DRIVER_PET');

  static const $core.List<WorkflowDriver> values = <WorkflowDriver>[
    WORKFLOW_DRIVER_UNSPECIFIED,
    WORKFLOW_DRIVER_FLOWCRAFT,
    WORKFLOW_DRIVER_DOUBAO_REALTIME,
    WORKFLOW_DRIVER_AST_TRANSLATE,
    WORKFLOW_DRIVER_CHATROOM,
    WORKFLOW_DRIVER_PET,
  ];

  static final $core.List<WorkflowDriver?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 5);
  static WorkflowDriver? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const WorkflowDriver._(super.value, super.name);
}

class WorkspaceHistoryListRequestOrder extends $pb.ProtobufEnum {
  static const WorkspaceHistoryListRequestOrder
      WORKSPACE_HISTORY_LIST_REQUEST_ORDER_UNSPECIFIED =
      WorkspaceHistoryListRequestOrder._(
          0,
          _omitEnumNames
              ? ''
              : 'WORKSPACE_HISTORY_LIST_REQUEST_ORDER_UNSPECIFIED');
  static const WorkspaceHistoryListRequestOrder
      WORKSPACE_HISTORY_LIST_REQUEST_ORDER_ASC =
      WorkspaceHistoryListRequestOrder._(
          1, _omitEnumNames ? '' : 'WORKSPACE_HISTORY_LIST_REQUEST_ORDER_ASC');
  static const WorkspaceHistoryListRequestOrder
      WORKSPACE_HISTORY_LIST_REQUEST_ORDER_DESC =
      WorkspaceHistoryListRequestOrder._(
          2, _omitEnumNames ? '' : 'WORKSPACE_HISTORY_LIST_REQUEST_ORDER_DESC');

  static const $core.List<WorkspaceHistoryListRequestOrder> values =
      <WorkspaceHistoryListRequestOrder>[
    WORKSPACE_HISTORY_LIST_REQUEST_ORDER_UNSPECIFIED,
    WORKSPACE_HISTORY_LIST_REQUEST_ORDER_ASC,
    WORKSPACE_HISTORY_LIST_REQUEST_ORDER_DESC,
  ];

  static final $core.List<WorkspaceHistoryListRequestOrder?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static WorkspaceHistoryListRequestOrder? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const WorkspaceHistoryListRequestOrder._(super.value, super.name);
}

class WorkspaceInputMode extends $pb.ProtobufEnum {
  static const WorkspaceInputMode WORKSPACE_INPUT_MODE_UNSPECIFIED =
      WorkspaceInputMode._(
          0, _omitEnumNames ? '' : 'WORKSPACE_INPUT_MODE_UNSPECIFIED');
  static const WorkspaceInputMode WORKSPACE_INPUT_MODE_PUSH_TO_TALK =
      WorkspaceInputMode._(
          1, _omitEnumNames ? '' : 'WORKSPACE_INPUT_MODE_PUSH_TO_TALK');
  static const WorkspaceInputMode WORKSPACE_INPUT_MODE_REALTIME =
      WorkspaceInputMode._(
          2, _omitEnumNames ? '' : 'WORKSPACE_INPUT_MODE_REALTIME');

  static const $core.List<WorkspaceInputMode> values = <WorkspaceInputMode>[
    WORKSPACE_INPUT_MODE_UNSPECIFIED,
    WORKSPACE_INPUT_MODE_PUSH_TO_TALK,
    WORKSPACE_INPUT_MODE_REALTIME,
  ];

  static final $core.List<WorkspaceInputMode?> _byValue =
      $pb.ProtobufEnum.$_initByValueList(values, 2);
  static WorkspaceInputMode? valueOf($core.int value) =>
      value < 0 || value >= _byValue.length ? null : _byValue[value];

  const WorkspaceInputMode._(super.value, super.name);
}

const $core.bool _omitEnumNames =
    $core.bool.fromEnvironment('protobuf.omit_enum_names');
