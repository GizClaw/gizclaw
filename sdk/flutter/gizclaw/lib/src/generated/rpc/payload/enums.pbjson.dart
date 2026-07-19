// This is a generated file - do not edit.
//
// Generated from payload/enums.proto.

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

@$core.Deprecated('Use iconFormatDescriptor instead')
const IconFormat$json = {
  '1': 'IconFormat',
  '2': [
    {'1': 'ICON_FORMAT_UNSPECIFIED', '2': 0},
    {'1': 'ICON_FORMAT_PIXA', '2': 1},
    {'1': 'ICON_FORMAT_PNG', '2': 2},
  ],
};

/// Descriptor for `IconFormat`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List iconFormatDescriptor = $convert.base64Decode(
    'CgpJY29uRm9ybWF0EhsKF0lDT05fRk9STUFUX1VOU1BFQ0lGSUVEEAASFAoQSUNPTl9GT1JNQV'
    'RfUElYQRABEhMKD0lDT05fRk9STUFUX1BORxAC');

@$core.Deprecated('Use resourceSourceDescriptor instead')
const ResourceSource$json = {
  '1': 'ResourceSource',
  '2': [
    {'1': 'RESOURCE_SOURCE_UNSPECIFIED', '2': 0},
    {'1': 'RESOURCE_SOURCE_RUNTIME', '2': 1},
    {'1': 'RESOURCE_SOURCE_OWNED', '2': 2},
  ],
};

/// Descriptor for `ResourceSource`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List resourceSourceDescriptor = $convert.base64Decode(
    'Cg5SZXNvdXJjZVNvdXJjZRIfChtSRVNPVVJDRV9TT1VSQ0VfVU5TUEVDSUZJRUQQABIbChdSRV'
    'NPVVJDRV9TT1VSQ0VfUlVOVElNRRABEhkKFVJFU09VUkNFX1NPVVJDRV9PV05FRBAC');

@$core.Deprecated('Use aSTTranslateModeDescriptor instead')
const ASTTranslateMode$json = {
  '1': 'ASTTranslateMode',
  '2': [
    {'1': 'ASTTRANSLATE_MODE_UNSPECIFIED', '2': 0},
    {'1': 'ASTTRANSLATE_MODE_S2T', '2': 1},
    {'1': 'ASTTRANSLATE_MODE_S2S', '2': 2},
  ],
};

/// Descriptor for `ASTTranslateMode`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List aSTTranslateModeDescriptor = $convert.base64Decode(
    'ChBBU1RUcmFuc2xhdGVNb2RlEiEKHUFTVFRSQU5TTEFURV9NT0RFX1VOU1BFQ0lGSUVEEAASGQ'
    'oVQVNUVFJBTlNMQVRFX01PREVfUzJUEAESGQoVQVNUVFJBTlNMQVRFX01PREVfUzJTEAI=');

@$core.Deprecated(
    'Use aSTTranslateWorkspaceParametersAgentTypeDescriptor instead')
const ASTTranslateWorkspaceParametersAgentType$json = {
  '1': 'ASTTranslateWorkspaceParametersAgentType',
  '2': [
    {'1': 'ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED', '2': 0},
    {'1': 'ASTTRANSLATE_WORKSPACE_PARAMETERS_AGENT_TYPE_AST_TRANSLATE', '2': 1},
  ],
};

/// Descriptor for `ASTTranslateWorkspaceParametersAgentType`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List aSTTranslateWorkspaceParametersAgentTypeDescriptor =
    $convert.base64Decode(
        'CihBU1RUcmFuc2xhdGVXb3Jrc3BhY2VQYXJhbWV0ZXJzQWdlbnRUeXBlEjwKOEFTVFRSQU5TTE'
        'FURV9XT1JLU1BBQ0VfUEFSQU1FVEVSU19BR0VOVF9UWVBFX1VOU1BFQ0lGSUVEEAASPgo6QVNU'
        'VFJBTlNMQVRFX1dPUktTUEFDRV9QQVJBTUVURVJTX0FHRU5UX1RZUEVfQVNUX1RSQU5TTEFURR'
        'AB');

@$core.Deprecated('Use chatRoomModeDescriptor instead')
const ChatRoomMode$json = {
  '1': 'ChatRoomMode',
  '2': [
    {'1': 'CHAT_ROOM_MODE_UNSPECIFIED', '2': 0},
    {'1': 'CHAT_ROOM_MODE_DIRECT', '2': 1},
    {'1': 'CHAT_ROOM_MODE_GROUP', '2': 2},
  ],
};

/// Descriptor for `ChatRoomMode`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List chatRoomModeDescriptor = $convert.base64Decode(
    'CgxDaGF0Um9vbU1vZGUSHgoaQ0hBVF9ST09NX01PREVfVU5TUEVDSUZJRUQQABIZChVDSEFUX1'
    'JPT01fTU9ERV9ESVJFQ1QQARIYChRDSEFUX1JPT01fTU9ERV9HUk9VUBAC');

@$core.Deprecated('Use chatRoomWorkspaceParametersAgentTypeDescriptor instead')
const ChatRoomWorkspaceParametersAgentType$json = {
  '1': 'ChatRoomWorkspaceParametersAgentType',
  '2': [
    {'1': 'CHAT_ROOM_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED', '2': 0},
    {'1': 'CHAT_ROOM_WORKSPACE_PARAMETERS_AGENT_TYPE_CHATROOM', '2': 1},
  ],
};

/// Descriptor for `ChatRoomWorkspaceParametersAgentType`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List chatRoomWorkspaceParametersAgentTypeDescriptor =
    $convert.base64Decode(
        'CiRDaGF0Um9vbVdvcmtzcGFjZVBhcmFtZXRlcnNBZ2VudFR5cGUSOQo1Q0hBVF9ST09NX1dPUk'
        'tTUEFDRV9QQVJBTUVURVJTX0FHRU5UX1RZUEVfVU5TUEVDSUZJRUQQABI2CjJDSEFUX1JPT01f'
        'V09SS1NQQUNFX1BBUkFNRVRFUlNfQUdFTlRfVFlQRV9DSEFUUk9PTRAB');

@$core
    .Deprecated('Use dashScopeTenantModelProviderDataApiModeDescriptor instead')
const DashScopeTenantModelProviderDataApiMode$json = {
  '1': 'DashScopeTenantModelProviderDataApiMode',
  '2': [
    {'1': 'DASH_SCOPE_TENANT_MODEL_PROVIDER_DATA_API_MODE_UNSPECIFIED', '2': 0},
    {
      '1': 'DASH_SCOPE_TENANT_MODEL_PROVIDER_DATA_API_MODE_CHAT_COMPLETIONS',
      '2': 1
    },
    {'1': 'DASH_SCOPE_TENANT_MODEL_PROVIDER_DATA_API_MODE_REALTIME', '2': 2},
  ],
};

/// Descriptor for `DashScopeTenantModelProviderDataApiMode`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List dashScopeTenantModelProviderDataApiModeDescriptor =
    $convert.base64Decode(
        'CidEYXNoU2NvcGVUZW5hbnRNb2RlbFByb3ZpZGVyRGF0YUFwaU1vZGUSPgo6REFTSF9TQ09QRV'
        '9URU5BTlRfTU9ERUxfUFJPVklERVJfREFUQV9BUElfTU9ERV9VTlNQRUNJRklFRBAAEkMKP0RB'
        'U0hfU0NPUEVfVEVOQU5UX01PREVMX1BST1ZJREVSX0RBVEFfQVBJX01PREVfQ0hBVF9DT01QTE'
        'VUSU9OUxABEjsKN0RBU0hfU0NPUEVfVEVOQU5UX01PREVMX1BST1ZJREVSX0RBVEFfQVBJX01P'
        'REVfUkVBTFRJTUUQAg==');

@$core.Deprecated('Use doubaoRealtimeAudioFormatTypeDescriptor instead')
const DoubaoRealtimeAudioFormatType$json = {
  '1': 'DoubaoRealtimeAudioFormatType',
  '2': [
    {'1': 'DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_UNSPECIFIED', '2': 0},
    {'1': 'DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_PCM', '2': 1},
    {'1': 'DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_PCM_S16LE', '2': 2},
    {'1': 'DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_SPEECH_OPUS', '2': 3},
    {'1': 'DOUBAO_REALTIME_AUDIO_FORMAT_TYPE_OGG_OPUS', '2': 4},
  ],
};

/// Descriptor for `DoubaoRealtimeAudioFormatType`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeAudioFormatTypeDescriptor = $convert.base64Decode(
    'Ch1Eb3ViYW9SZWFsdGltZUF1ZGlvRm9ybWF0VHlwZRIxCi1ET1VCQU9fUkVBTFRJTUVfQVVESU'
    '9fRk9STUFUX1RZUEVfVU5TUEVDSUZJRUQQABIpCiVET1VCQU9fUkVBTFRJTUVfQVVESU9fRk9S'
    'TUFUX1RZUEVfUENNEAESLworRE9VQkFPX1JFQUxUSU1FX0FVRElPX0ZPUk1BVF9UWVBFX1BDTV'
    '9TMTZMRRACEjEKLURPVUJBT19SRUFMVElNRV9BVURJT19GT1JNQVRfVFlQRV9TUEVFQ0hfT1BV'
    'UxADEi4KKkRPVUJBT19SRUFMVElNRV9BVURJT19GT1JNQVRfVFlQRV9PR0dfT1BVUxAE');

@$core.Deprecated(
    'Use doubaoRealtimeDialogExtraVolcWebsearchTypeDescriptor instead')
const DoubaoRealtimeDialogExtraVolcWebsearchType$json = {
  '1': 'DoubaoRealtimeDialogExtraVolcWebsearchType',
  '2': [
    {
      '1': 'DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_UNSPECIFIED',
      '2': 0
    },
    {'1': 'DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_WEB', '2': 1},
    {
      '1': 'DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_WEB_SUMMARY',
      '2': 2
    },
    {'1': 'DOUBAO_REALTIME_DIALOG_EXTRA_VOLC_WEBSEARCH_TYPE_WEB_AGENT', '2': 3},
  ],
};

/// Descriptor for `DoubaoRealtimeDialogExtraVolcWebsearchType`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List
    doubaoRealtimeDialogExtraVolcWebsearchTypeDescriptor =
    $convert.base64Decode(
        'CipEb3ViYW9SZWFsdGltZURpYWxvZ0V4dHJhVm9sY1dlYnNlYXJjaFR5cGUSQAo8RE9VQkFPX1'
        'JFQUxUSU1FX0RJQUxPR19FWFRSQV9WT0xDX1dFQlNFQVJDSF9UWVBFX1VOU1BFQ0lGSUVEEAAS'
        'OAo0RE9VQkFPX1JFQUxUSU1FX0RJQUxPR19FWFRSQV9WT0xDX1dFQlNFQVJDSF9UWVBFX1dFQh'
        'ABEkAKPERPVUJBT19SRUFMVElNRV9ESUFMT0dfRVhUUkFfVk9MQ19XRUJTRUFSQ0hfVFlQRV9X'
        'RUJfU1VNTUFSWRACEj4KOkRPVUJBT19SRUFMVElNRV9ESUFMT0dfRVhUUkFfVk9MQ19XRUJTRU'
        'FSQ0hfVFlQRV9XRUJfQUdFTlQQAw==');

@$core.Deprecated('Use doubaoRealtimeFunctionToolTypeDescriptor instead')
const DoubaoRealtimeFunctionToolType$json = {
  '1': 'DoubaoRealtimeFunctionToolType',
  '2': [
    {'1': 'DOUBAO_REALTIME_FUNCTION_TOOL_TYPE_UNSPECIFIED', '2': 0},
    {'1': 'DOUBAO_REALTIME_FUNCTION_TOOL_TYPE_FUNCTION', '2': 1},
  ],
};

/// Descriptor for `DoubaoRealtimeFunctionToolType`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List doubaoRealtimeFunctionToolTypeDescriptor =
    $convert.base64Decode(
        'Ch5Eb3ViYW9SZWFsdGltZUZ1bmN0aW9uVG9vbFR5cGUSMgouRE9VQkFPX1JFQUxUSU1FX0ZVTk'
        'NUSU9OX1RPT0xfVFlQRV9VTlNQRUNJRklFRBAAEi8KK0RPVUJBT19SRUFMVElNRV9GVU5DVElP'
        'Tl9UT09MX1RZUEVfRlVOQ1RJT04QAQ==');

@$core.Deprecated(
    'Use doubaoRealtimeWorkspaceParametersAgentTypeDescriptor instead')
const DoubaoRealtimeWorkspaceParametersAgentType$json = {
  '1': 'DoubaoRealtimeWorkspaceParametersAgentType',
  '2': [
    {
      '1': 'DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED',
      '2': 0
    },
    {
      '1': 'DOUBAO_REALTIME_WORKSPACE_PARAMETERS_AGENT_TYPE_DOUBAO_REALTIME',
      '2': 1
    },
  ],
};

/// Descriptor for `DoubaoRealtimeWorkspaceParametersAgentType`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List
    doubaoRealtimeWorkspaceParametersAgentTypeDescriptor =
    $convert.base64Decode(
        'CipEb3ViYW9SZWFsdGltZVdvcmtzcGFjZVBhcmFtZXRlcnNBZ2VudFR5cGUSPwo7RE9VQkFPX1'
        'JFQUxUSU1FX1dPUktTUEFDRV9QQVJBTUVURVJTX0FHRU5UX1RZUEVfVU5TUEVDSUZJRUQQABJD'
        'Cj9ET1VCQU9fUkVBTFRJTUVfV09SS1NQQUNFX1BBUkFNRVRFUlNfQUdFTlRfVFlQRV9ET1VCQU'
        '9fUkVBTFRJTUUQAQ==');

@$core.Deprecated('Use firmwareArtifactEntryTypeDescriptor instead')
const FirmwareArtifactEntryType$json = {
  '1': 'FirmwareArtifactEntryType',
  '2': [
    {'1': 'FIRMWARE_ARTIFACT_ENTRY_TYPE_UNSPECIFIED', '2': 0},
    {'1': 'FIRMWARE_ARTIFACT_ENTRY_TYPE_FILE', '2': 1},
    {'1': 'FIRMWARE_ARTIFACT_ENTRY_TYPE_DIR', '2': 2},
  ],
};

/// Descriptor for `FirmwareArtifactEntryType`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List firmwareArtifactEntryTypeDescriptor = $convert.base64Decode(
    'ChlGaXJtd2FyZUFydGlmYWN0RW50cnlUeXBlEiwKKEZJUk1XQVJFX0FSVElGQUNUX0VOVFJZX1'
    'RZUEVfVU5TUEVDSUZJRUQQABIlCiFGSVJNV0FSRV9BUlRJRkFDVF9FTlRSWV9UWVBFX0ZJTEUQ'
    'ARIkCiBGSVJNV0FSRV9BUlRJRkFDVF9FTlRSWV9UWVBFX0RJUhAC');

@$core.Deprecated('Use firmwareChannelNameDescriptor instead')
const FirmwareChannelName$json = {
  '1': 'FirmwareChannelName',
  '2': [
    {'1': 'FIRMWARE_CHANNEL_NAME_UNSPECIFIED', '2': 0},
    {'1': 'FIRMWARE_CHANNEL_NAME_STABLE', '2': 1},
    {'1': 'FIRMWARE_CHANNEL_NAME_BETA', '2': 2},
    {'1': 'FIRMWARE_CHANNEL_NAME_DEVELOP', '2': 3},
    {'1': 'FIRMWARE_CHANNEL_NAME_PENDING', '2': 4},
  ],
};

/// Descriptor for `FirmwareChannelName`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List firmwareChannelNameDescriptor = $convert.base64Decode(
    'ChNGaXJtd2FyZUNoYW5uZWxOYW1lEiUKIUZJUk1XQVJFX0NIQU5ORUxfTkFNRV9VTlNQRUNJRk'
    'lFRBAAEiAKHEZJUk1XQVJFX0NIQU5ORUxfTkFNRV9TVEFCTEUQARIeChpGSVJNV0FSRV9DSEFO'
    'TkVMX05BTUVfQkVUQRACEiEKHUZJUk1XQVJFX0NIQU5ORUxfTkFNRV9ERVZFTE9QEAMSIQodRk'
    'lSTVdBUkVfQ0hBTk5FTF9OQU1FX1BFTkRJTkcQBA==');

@$core.Deprecated(
    'Use flowcraftConversationParametersAgentInitiativePolicyDescriptor instead')
const FlowcraftConversationParametersAgentInitiativePolicy$json = {
  '1': 'FlowcraftConversationParametersAgentInitiativePolicy',
  '2': [
    {
      '1':
          'FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_UNSPECIFIED',
      '2': 0
    },
    {
      '1':
          'FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_ONCE_WHEN_EMPTY',
      '2': 1
    },
    {
      '1':
          'FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_ON_RELOAD',
      '2': 2
    },
  ],
};

/// Descriptor for `FlowcraftConversationParametersAgentInitiativePolicy`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List
    flowcraftConversationParametersAgentInitiativePolicyDescriptor =
    $convert.base64Decode(
        'CjRGbG93Y3JhZnRDb252ZXJzYXRpb25QYXJhbWV0ZXJzQWdlbnRJbml0aWF0aXZlUG9saWN5Ek'
        'kKRUZMT1dDUkFGVF9DT05WRVJTQVRJT05fUEFSQU1FVEVSU19BR0VOVF9JTklUSUFUSVZFX1BP'
        'TElDWV9VTlNQRUNJRklFRBAAEk0KSUZMT1dDUkFGVF9DT05WRVJTQVRJT05fUEFSQU1FVEVSU1'
        '9BR0VOVF9JTklUSUFUSVZFX1BPTElDWV9PTkNFX1dIRU5fRU1QVFkQARJHCkNGTE9XQ1JBRlRf'
        'Q09OVkVSU0FUSU9OX1BBUkFNRVRFUlNfQUdFTlRfSU5JVElBVElWRV9QT0xJQ1lfT05fUkVMT0'
        'FEEAI=');

@$core.Deprecated(
    'Use flowcraftConversationParametersInitiativeDescriptor instead')
const FlowcraftConversationParametersInitiative$json = {
  '1': 'FlowcraftConversationParametersInitiative',
  '2': [
    {'1': 'FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_UNSPECIFIED', '2': 0},
    {'1': 'FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_PEER', '2': 1},
    {'1': 'FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_AGENT', '2': 2},
  ],
};

/// Descriptor for `FlowcraftConversationParametersInitiative`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List
    flowcraftConversationParametersInitiativeDescriptor = $convert.base64Decode(
        'CilGbG93Y3JhZnRDb252ZXJzYXRpb25QYXJhbWV0ZXJzSW5pdGlhdGl2ZRI8CjhGTE9XQ1JBRl'
        'RfQ09OVkVSU0FUSU9OX1BBUkFNRVRFUlNfSU5JVElBVElWRV9VTlNQRUNJRklFRBAAEjUKMUZM'
        'T1dDUkFGVF9DT05WRVJTQVRJT05fUEFSQU1FVEVSU19JTklUSUFUSVZFX1BFRVIQARI2CjJGTE'
        '9XQ1JBRlRfQ09OVkVSU0FUSU9OX1BBUkFNRVRFUlNfSU5JVElBVElWRV9BR0VOVBAC');

@$core.Deprecated('Use flowcraftWorkspaceParametersAgentTypeDescriptor instead')
const FlowcraftWorkspaceParametersAgentType$json = {
  '1': 'FlowcraftWorkspaceParametersAgentType',
  '2': [
    {'1': 'FLOWCRAFT_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED', '2': 0},
    {'1': 'FLOWCRAFT_WORKSPACE_PARAMETERS_AGENT_TYPE_FLOWCRAFT', '2': 1},
  ],
};

/// Descriptor for `FlowcraftWorkspaceParametersAgentType`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List flowcraftWorkspaceParametersAgentTypeDescriptor =
    $convert.base64Decode(
        'CiVGbG93Y3JhZnRXb3Jrc3BhY2VQYXJhbWV0ZXJzQWdlbnRUeXBlEjkKNUZMT1dDUkFGVF9XT1'
        'JLU1BBQ0VfUEFSQU1FVEVSU19BR0VOVF9UWVBFX1VOU1BFQ0lGSUVEEAASNwozRkxPV0NSQUZU'
        'X1dPUktTUEFDRV9QQVJBTUVURVJTX0FHRU5UX1RZUEVfRkxPV0NSQUZUEAE=');

@$core.Deprecated('Use petConversationParametersInitiativeDescriptor instead')
const PetConversationParametersInitiative$json = {
  '1': 'PetConversationParametersInitiative',
  '2': [
    {'1': 'PET_CONVERSATION_PARAMETERS_INITIATIVE_UNSPECIFIED', '2': 0},
    {'1': 'PET_CONVERSATION_PARAMETERS_INITIATIVE_PEER', '2': 1},
    {'1': 'PET_CONVERSATION_PARAMETERS_INITIATIVE_AGENT', '2': 2},
  ],
};

/// Descriptor for `PetConversationParametersInitiative`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List petConversationParametersInitiativeDescriptor =
    $convert.base64Decode(
        'CiNQZXRDb252ZXJzYXRpb25QYXJhbWV0ZXJzSW5pdGlhdGl2ZRI2CjJQRVRfQ09OVkVSU0FUSU'
        '9OX1BBUkFNRVRFUlNfSU5JVElBVElWRV9VTlNQRUNJRklFRBAAEi8KK1BFVF9DT05WRVJTQVRJ'
        'T05fUEFSQU1FVEVSU19JTklUSUFUSVZFX1BFRVIQARIwCixQRVRfQ09OVkVSU0FUSU9OX1BBUk'
        'FNRVRFUlNfSU5JVElBVElWRV9BR0VOVBAC');

@$core.Deprecated('Use petWorkspaceParametersAgentTypeDescriptor instead')
const PetWorkspaceParametersAgentType$json = {
  '1': 'PetWorkspaceParametersAgentType',
  '2': [
    {'1': 'PET_WORKSPACE_PARAMETERS_AGENT_TYPE_UNSPECIFIED', '2': 0},
    {'1': 'PET_WORKSPACE_PARAMETERS_AGENT_TYPE_PET', '2': 1},
  ],
};

/// Descriptor for `PetWorkspaceParametersAgentType`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List petWorkspaceParametersAgentTypeDescriptor =
    $convert.base64Decode(
        'Ch9QZXRXb3Jrc3BhY2VQYXJhbWV0ZXJzQWdlbnRUeXBlEjMKL1BFVF9XT1JLU1BBQ0VfUEFSQU'
        '1FVEVSU19BR0VOVF9UWVBFX1VOU1BFQ0lGSUVEEAASKwonUEVUX1dPUktTUEFDRV9QQVJBTUVU'
        'RVJTX0FHRU5UX1RZUEVfUEVUEAE=');

@$core.Deprecated('Use friendGroupMemberMutableRoleDescriptor instead')
const FriendGroupMemberMutableRole$json = {
  '1': 'FriendGroupMemberMutableRole',
  '2': [
    {'1': 'FRIEND_GROUP_MEMBER_MUTABLE_ROLE_UNSPECIFIED', '2': 0},
    {'1': 'FRIEND_GROUP_MEMBER_MUTABLE_ROLE_ADMIN', '2': 1},
    {'1': 'FRIEND_GROUP_MEMBER_MUTABLE_ROLE_MEMBER', '2': 2},
  ],
};

/// Descriptor for `FriendGroupMemberMutableRole`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List friendGroupMemberMutableRoleDescriptor = $convert.base64Decode(
    'ChxGcmllbmRHcm91cE1lbWJlck11dGFibGVSb2xlEjAKLEZSSUVORF9HUk9VUF9NRU1CRVJfTV'
    'VUQUJMRV9ST0xFX1VOU1BFQ0lGSUVEEAASKgomRlJJRU5EX0dST1VQX01FTUJFUl9NVVRBQkxF'
    'X1JPTEVfQURNSU4QARIrCidGUklFTkRfR1JPVVBfTUVNQkVSX01VVEFCTEVfUk9MRV9NRU1CRV'
    'IQAg==');

@$core.Deprecated('Use friendGroupMemberRoleDescriptor instead')
const FriendGroupMemberRole$json = {
  '1': 'FriendGroupMemberRole',
  '2': [
    {'1': 'FRIEND_GROUP_MEMBER_ROLE_UNSPECIFIED', '2': 0},
    {'1': 'FRIEND_GROUP_MEMBER_ROLE_OWNER', '2': 1},
    {'1': 'FRIEND_GROUP_MEMBER_ROLE_ADMIN', '2': 2},
    {'1': 'FRIEND_GROUP_MEMBER_ROLE_MEMBER', '2': 3},
  ],
};

/// Descriptor for `FriendGroupMemberRole`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List friendGroupMemberRoleDescriptor = $convert.base64Decode(
    'ChVGcmllbmRHcm91cE1lbWJlclJvbGUSKAokRlJJRU5EX0dST1VQX01FTUJFUl9ST0xFX1VOU1'
    'BFQ0lGSUVEEAASIgoeRlJJRU5EX0dST1VQX01FTUJFUl9ST0xFX09XTkVSEAESIgoeRlJJRU5E'
    'X0dST1VQX01FTUJFUl9ST0xFX0FETUlOEAISIwofRlJJRU5EX0dST1VQX01FTUJFUl9ST0xFX0'
    '1FTUJFUhAD');

@$core.Deprecated('Use peerRoleDescriptor instead')
const PeerRole$json = {
  '1': 'PeerRole',
  '2': [
    {'1': 'PEER_ROLE_UNSPECIFIED', '2': 0},
    {'1': 'PEER_ROLE_ADMIN', '2': 1},
    {'1': 'PEER_ROLE_SERVER', '2': 2},
    {'1': 'PEER_ROLE_EDGE_NODE', '2': 3},
    {'1': 'PEER_ROLE_CLIENT', '2': 4},
  ],
};

/// Descriptor for `PeerRole`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List peerRoleDescriptor = $convert.base64Decode(
    'CghQZWVyUm9sZRIZChVQRUVSX1JPTEVfVU5TUEVDSUZJRUQQABITCg9QRUVSX1JPTEVfQURNSU'
    '4QARIUChBQRUVSX1JPTEVfU0VSVkVSEAISFwoTUEVFUl9ST0xFX0VER0VfTk9ERRADEhQKEFBF'
    'RVJfUk9MRV9DTElFTlQQBA==');

@$core.Deprecated('Use modelKindDescriptor instead')
const ModelKind$json = {
  '1': 'ModelKind',
  '2': [
    {'1': 'MODEL_KIND_UNSPECIFIED', '2': 0},
    {'1': 'MODEL_KIND_LLM', '2': 1},
    {'1': 'MODEL_KIND_TTS', '2': 2},
    {'1': 'MODEL_KIND_ASR', '2': 3},
    {'1': 'MODEL_KIND_REALTIME', '2': 4},
    {'1': 'MODEL_KIND_TRANSLATION', '2': 5},
    {'1': 'MODEL_KIND_EMBEDDING', '2': 6},
  ],
};

/// Descriptor for `ModelKind`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List modelKindDescriptor = $convert.base64Decode(
    'CglNb2RlbEtpbmQSGgoWTU9ERUxfS0lORF9VTlNQRUNJRklFRBAAEhIKDk1PREVMX0tJTkRfTE'
    'xNEAESEgoOTU9ERUxfS0lORF9UVFMQAhISCg5NT0RFTF9LSU5EX0FTUhADEhcKE01PREVMX0tJ'
    'TkRfUkVBTFRJTUUQBBIaChZNT0RFTF9LSU5EX1RSQU5TTEFUSU9OEAUSGAoUTU9ERUxfS0lORF'
    '9FTUJFRERJTkcQBg==');

@$core.Deprecated('Use modelProviderKindDescriptor instead')
const ModelProviderKind$json = {
  '1': 'ModelProviderKind',
  '2': [
    {'1': 'MODEL_PROVIDER_KIND_UNSPECIFIED', '2': 0},
    {'1': 'MODEL_PROVIDER_KIND_GEMINI_TENANT', '2': 1},
    {'1': 'MODEL_PROVIDER_KIND_DASHSCOPE_TENANT', '2': 2},
    {'1': 'MODEL_PROVIDER_KIND_OPENAI_TENANT', '2': 3},
    {'1': 'MODEL_PROVIDER_KIND_VOLC_TENANT', '2': 4},
  ],
};

/// Descriptor for `ModelProviderKind`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List modelProviderKindDescriptor = $convert.base64Decode(
    'ChFNb2RlbFByb3ZpZGVyS2luZBIjCh9NT0RFTF9QUk9WSURFUl9LSU5EX1VOU1BFQ0lGSUVEEA'
    'ASJQohTU9ERUxfUFJPVklERVJfS0lORF9HRU1JTklfVEVOQU5UEAESKAokTU9ERUxfUFJPVklE'
    'RVJfS0lORF9EQVNIU0NPUEVfVEVOQU5UEAISJQohTU9ERUxfUFJPVklERVJfS0lORF9PUEVOQU'
    'lfVEVOQU5UEAMSIwofTU9ERUxfUFJPVklERVJfS0lORF9WT0xDX1RFTkFOVBAE');

@$core.Deprecated('Use modelSourceDescriptor instead')
const ModelSource$json = {
  '1': 'ModelSource',
  '2': [
    {'1': 'MODEL_SOURCE_UNSPECIFIED', '2': 0},
    {'1': 'MODEL_SOURCE_SYNC', '2': 1},
    {'1': 'MODEL_SOURCE_MANUAL', '2': 2},
  ],
};

/// Descriptor for `ModelSource`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List modelSourceDescriptor = $convert.base64Decode(
    'CgtNb2RlbFNvdXJjZRIcChhNT0RFTF9TT1VSQ0VfVU5TUEVDSUZJRUQQABIVChFNT0RFTF9TT1'
    'VSQ0VfU1lOQxABEhcKE01PREVMX1NPVVJDRV9NQU5VQUwQAg==');

@$core.Deprecated('Use peerRunHistoryEntryTypeDescriptor instead')
const PeerRunHistoryEntryType$json = {
  '1': 'PeerRunHistoryEntryType',
  '2': [
    {'1': 'PEER_RUN_HISTORY_ENTRY_TYPE_UNSPECIFIED', '2': 0},
    {'1': 'PEER_RUN_HISTORY_ENTRY_TYPE_GEAR', '2': 1},
    {'1': 'PEER_RUN_HISTORY_ENTRY_TYPE_AGENT', '2': 2},
  ],
};

/// Descriptor for `PeerRunHistoryEntryType`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List peerRunHistoryEntryTypeDescriptor = $convert.base64Decode(
    'ChdQZWVyUnVuSGlzdG9yeUVudHJ5VHlwZRIrCidQRUVSX1JVTl9ISVNUT1JZX0VOVFJZX1RZUE'
    'VfVU5TUEVDSUZJRUQQABIkCiBQRUVSX1JVTl9ISVNUT1JZX0VOVFJZX1RZUEVfR0VBUhABEiUK'
    'IVBFRVJfUlVOX0hJU1RPUllfRU5UUllfVFlQRV9BR0VOVBAC');

@$core.Deprecated('Use peerRunHistoryListRequestOrderDescriptor instead')
const PeerRunHistoryListRequestOrder$json = {
  '1': 'PeerRunHistoryListRequestOrder',
  '2': [
    {'1': 'PEER_RUN_HISTORY_LIST_REQUEST_ORDER_UNSPECIFIED', '2': 0},
    {'1': 'PEER_RUN_HISTORY_LIST_REQUEST_ORDER_ASC', '2': 1},
    {'1': 'PEER_RUN_HISTORY_LIST_REQUEST_ORDER_DESC', '2': 2},
  ],
};

/// Descriptor for `PeerRunHistoryListRequestOrder`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List peerRunHistoryListRequestOrderDescriptor =
    $convert.base64Decode(
        'Ch5QZWVyUnVuSGlzdG9yeUxpc3RSZXF1ZXN0T3JkZXISMwovUEVFUl9SVU5fSElTVE9SWV9MSV'
        'NUX1JFUVVFU1RfT1JERVJfVU5TUEVDSUZJRUQQABIrCidQRUVSX1JVTl9ISVNUT1JZX0xJU1Rf'
        'UkVRVUVTVF9PUkRFUl9BU0MQARIsCihQRUVSX1JVTl9ISVNUT1JZX0xJU1RfUkVRVUVTVF9PUk'
        'RFUl9ERVNDEAI=');

@$core.Deprecated('Use peerRunStatusStateDescriptor instead')
const PeerRunStatusState$json = {
  '1': 'PeerRunStatusState',
  '2': [
    {'1': 'PEER_RUN_STATUS_STATE_UNSPECIFIED', '2': 0},
    {'1': 'PEER_RUN_STATUS_STATE_STOPPED', '2': 1},
    {'1': 'PEER_RUN_STATUS_STATE_STARTING', '2': 2},
    {'1': 'PEER_RUN_STATUS_STATE_RUNNING', '2': 3},
    {'1': 'PEER_RUN_STATUS_STATE_STOPPING', '2': 4},
    {'1': 'PEER_RUN_STATUS_STATE_ERROR', '2': 5},
  ],
};

/// Descriptor for `PeerRunStatusState`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List peerRunStatusStateDescriptor = $convert.base64Decode(
    'ChJQZWVyUnVuU3RhdHVzU3RhdGUSJQohUEVFUl9SVU5fU1RBVFVTX1NUQVRFX1VOU1BFQ0lGSU'
    'VEEAASIQodUEVFUl9SVU5fU1RBVFVTX1NUQVRFX1NUT1BQRUQQARIiCh5QRUVSX1JVTl9TVEFU'
    'VVNfU1RBVEVfU1RBUlRJTkcQAhIhCh1QRUVSX1JVTl9TVEFUVVNfU1RBVEVfUlVOTklORxADEi'
    'IKHlBFRVJfUlVOX1NUQVRVU19TVEFURV9TVE9QUElORxAEEh8KG1BFRVJfUlVOX1NUQVRVU19T'
    'VEFURV9FUlJPUhAF');

@$core.Deprecated('Use voiceProviderKindDescriptor instead')
const VoiceProviderKind$json = {
  '1': 'VoiceProviderKind',
  '2': [
    {'1': 'VOICE_PROVIDER_KIND_UNSPECIFIED', '2': 0},
    {'1': 'VOICE_PROVIDER_KIND_GEMINI_TENANT', '2': 1},
    {'1': 'VOICE_PROVIDER_KIND_DASHSCOPE_TENANT', '2': 2},
    {'1': 'VOICE_PROVIDER_KIND_OPENAI_TENANT', '2': 3},
    {'1': 'VOICE_PROVIDER_KIND_MINIMAX_TENANT', '2': 4},
    {'1': 'VOICE_PROVIDER_KIND_VOLC_TENANT', '2': 5},
  ],
};

/// Descriptor for `VoiceProviderKind`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List voiceProviderKindDescriptor = $convert.base64Decode(
    'ChFWb2ljZVByb3ZpZGVyS2luZBIjCh9WT0lDRV9QUk9WSURFUl9LSU5EX1VOU1BFQ0lGSUVEEA'
    'ASJQohVk9JQ0VfUFJPVklERVJfS0lORF9HRU1JTklfVEVOQU5UEAESKAokVk9JQ0VfUFJPVklE'
    'RVJfS0lORF9EQVNIU0NPUEVfVEVOQU5UEAISJQohVk9JQ0VfUFJPVklERVJfS0lORF9PUEVOQU'
    'lfVEVOQU5UEAMSJgoiVk9JQ0VfUFJPVklERVJfS0lORF9NSU5JTUFYX1RFTkFOVBAEEiMKH1ZP'
    'SUNFX1BST1ZJREVSX0tJTkRfVk9MQ19URU5BTlQQBQ==');

@$core.Deprecated('Use voiceSourceDescriptor instead')
const VoiceSource$json = {
  '1': 'VoiceSource',
  '2': [
    {'1': 'VOICE_SOURCE_UNSPECIFIED', '2': 0},
    {'1': 'VOICE_SOURCE_SYNC', '2': 1},
    {'1': 'VOICE_SOURCE_MANUAL', '2': 2},
  ],
};

/// Descriptor for `VoiceSource`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List voiceSourceDescriptor = $convert.base64Decode(
    'CgtWb2ljZVNvdXJjZRIcChhWT0lDRV9TT1VSQ0VfVU5TUEVDSUZJRUQQABIVChFWT0lDRV9TT1'
    'VSQ0VfU1lOQxABEhcKE1ZPSUNFX1NPVVJDRV9NQU5VQUwQAg==');

@$core.Deprecated('Use volcTenantModelProviderDataApiModeDescriptor instead')
const VolcTenantModelProviderDataApiMode$json = {
  '1': 'VolcTenantModelProviderDataApiMode',
  '2': [
    {'1': 'VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_UNSPECIFIED', '2': 0},
    {'1': 'VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_ASR', '2': 1},
    {'1': 'VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_TTS', '2': 2},
    {'1': 'VOLC_TENANT_MODEL_PROVIDER_DATA_API_MODE_REALTIME', '2': 3},
  ],
};

/// Descriptor for `VolcTenantModelProviderDataApiMode`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List volcTenantModelProviderDataApiModeDescriptor =
    $convert.base64Decode(
        'CiJWb2xjVGVuYW50TW9kZWxQcm92aWRlckRhdGFBcGlNb2RlEjgKNFZPTENfVEVOQU5UX01PRE'
        'VMX1BST1ZJREVSX0RBVEFfQVBJX01PREVfVU5TUEVDSUZJRUQQABIwCixWT0xDX1RFTkFOVF9N'
        'T0RFTF9QUk9WSURFUl9EQVRBX0FQSV9NT0RFX0FTUhABEjAKLFZPTENfVEVOQU5UX01PREVMX1'
        'BST1ZJREVSX0RBVEFfQVBJX01PREVfVFRTEAISNQoxVk9MQ19URU5BTlRfTU9ERUxfUFJPVklE'
        'RVJfREFUQV9BUElfTU9ERV9SRUFMVElNRRAD');

@$core.Deprecated('Use workflowDriverDescriptor instead')
const WorkflowDriver$json = {
  '1': 'WorkflowDriver',
  '2': [
    {'1': 'WORKFLOW_DRIVER_UNSPECIFIED', '2': 0},
    {'1': 'WORKFLOW_DRIVER_FLOWCRAFT', '2': 1},
    {'1': 'WORKFLOW_DRIVER_DOUBAO_REALTIME', '2': 2},
    {'1': 'WORKFLOW_DRIVER_AST_TRANSLATE', '2': 3},
    {'1': 'WORKFLOW_DRIVER_CHATROOM', '2': 4},
    {'1': 'WORKFLOW_DRIVER_PET', '2': 5},
  ],
};

/// Descriptor for `WorkflowDriver`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List workflowDriverDescriptor = $convert.base64Decode(
    'Cg5Xb3JrZmxvd0RyaXZlchIfChtXT1JLRkxPV19EUklWRVJfVU5TUEVDSUZJRUQQABIdChlXT1'
    'JLRkxPV19EUklWRVJfRkxPV0NSQUZUEAESIwofV09SS0ZMT1dfRFJJVkVSX0RPVUJBT19SRUFM'
    'VElNRRACEiEKHVdPUktGTE9XX0RSSVZFUl9BU1RfVFJBTlNMQVRFEAMSHAoYV09SS0ZMT1dfRF'
    'JJVkVSX0NIQVRST09NEAQSFwoTV09SS0ZMT1dfRFJJVkVSX1BFVBAF');

@$core.Deprecated('Use workspaceHistoryListRequestOrderDescriptor instead')
const WorkspaceHistoryListRequestOrder$json = {
  '1': 'WorkspaceHistoryListRequestOrder',
  '2': [
    {'1': 'WORKSPACE_HISTORY_LIST_REQUEST_ORDER_UNSPECIFIED', '2': 0},
    {'1': 'WORKSPACE_HISTORY_LIST_REQUEST_ORDER_ASC', '2': 1},
    {'1': 'WORKSPACE_HISTORY_LIST_REQUEST_ORDER_DESC', '2': 2},
  ],
};

/// Descriptor for `WorkspaceHistoryListRequestOrder`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List workspaceHistoryListRequestOrderDescriptor =
    $convert.base64Decode(
        'CiBXb3Jrc3BhY2VIaXN0b3J5TGlzdFJlcXVlc3RPcmRlchI0CjBXT1JLU1BBQ0VfSElTVE9SWV'
        '9MSVNUX1JFUVVFU1RfT1JERVJfVU5TUEVDSUZJRUQQABIsCihXT1JLU1BBQ0VfSElTVE9SWV9M'
        'SVNUX1JFUVVFU1RfT1JERVJfQVNDEAESLQopV09SS1NQQUNFX0hJU1RPUllfTElTVF9SRVFVRV'
        'NUX09SREVSX0RFU0MQAg==');

@$core.Deprecated('Use workspaceInputModeDescriptor instead')
const WorkspaceInputMode$json = {
  '1': 'WorkspaceInputMode',
  '2': [
    {'1': 'WORKSPACE_INPUT_MODE_UNSPECIFIED', '2': 0},
    {'1': 'WORKSPACE_INPUT_MODE_PUSH_TO_TALK', '2': 1},
    {'1': 'WORKSPACE_INPUT_MODE_REALTIME', '2': 2},
  ],
};

/// Descriptor for `WorkspaceInputMode`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List workspaceInputModeDescriptor = $convert.base64Decode(
    'ChJXb3Jrc3BhY2VJbnB1dE1vZGUSJAogV09SS1NQQUNFX0lOUFVUX01PREVfVU5TUEVDSUZJRU'
    'QQABIlCiFXT1JLU1BBQ0VfSU5QVVRfTU9ERV9QVVNIX1RPX1RBTEsQARIhCh1XT1JLU1BBQ0Vf'
    'SU5QVVRfTU9ERV9SRUFMVElNRRAC');

@$core.Deprecated('Use toolSourceDescriptor instead')
const ToolSource$json = {
  '1': 'ToolSource',
  '2': [
    {'1': 'TOOL_SOURCE_UNSPECIFIED', '2': 0},
    {'1': 'TOOL_SOURCE_BUILTIN', '2': 1},
    {'1': 'TOOL_SOURCE_DEVICE', '2': 2},
    {'1': 'TOOL_SOURCE_ADMIN', '2': 3},
  ],
};

/// Descriptor for `ToolSource`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List toolSourceDescriptor = $convert.base64Decode(
    'CgpUb29sU291cmNlEhsKF1RPT0xfU09VUkNFX1VOU1BFQ0lGSUVEEAASFwoTVE9PTF9TT1VSQ0'
    'VfQlVJTFRJThABEhYKElRPT0xfU09VUkNFX0RFVklDRRACEhUKEVRPT0xfU09VUkNFX0FETUlO'
    'EAM=');

@$core.Deprecated('Use toolExecutorKindDescriptor instead')
const ToolExecutorKind$json = {
  '1': 'ToolExecutorKind',
  '2': [
    {'1': 'TOOL_EXECUTOR_KIND_UNSPECIFIED', '2': 0},
    {'1': 'TOOL_EXECUTOR_KIND_BUILTIN', '2': 1},
    {'1': 'TOOL_EXECUTOR_KIND_DEVICE_RPC', '2': 2},
  ],
};

/// Descriptor for `ToolExecutorKind`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List toolExecutorKindDescriptor = $convert.base64Decode(
    'ChBUb29sRXhlY3V0b3JLaW5kEiIKHlRPT0xfRVhFQ1VUT1JfS0lORF9VTlNQRUNJRklFRBAAEh'
    '4KGlRPT0xfRVhFQ1VUT1JfS0lORF9CVUlMVElOEAESIQodVE9PTF9FWEVDVVRPUl9LSU5EX0RF'
    'VklDRV9SUEMQAg==');
