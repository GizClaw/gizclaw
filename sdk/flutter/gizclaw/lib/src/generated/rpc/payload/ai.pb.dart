// This is a generated file - do not edit.
//
// Generated from payload/ai.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:fixnum/fixnum.dart' as $fixnum;
import 'package:protobuf/protobuf.dart' as $pb;
import 'package:protobuf/well_known_types/google/protobuf/struct.pb.dart' as $0;

import 'enums.pbenum.dart' as $1;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

class ASTTranslateExternalVoiceParameters extends $pb.GeneratedMessage {
  factory ASTTranslateExternalVoiceParameters({
    $core.String? ttsVoice,
  }) {
    final result = create();
    if (ttsVoice != null) result.ttsVoice = ttsVoice;
    return result;
  }

  ASTTranslateExternalVoiceParameters._();

  factory ASTTranslateExternalVoiceParameters.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ASTTranslateExternalVoiceParameters.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ASTTranslateExternalVoiceParameters',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'ttsVoice')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ASTTranslateExternalVoiceParameters clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ASTTranslateExternalVoiceParameters copyWith(
          void Function(ASTTranslateExternalVoiceParameters) updates) =>
      super.copyWith((message) =>
              updates(message as ASTTranslateExternalVoiceParameters))
          as ASTTranslateExternalVoiceParameters;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ASTTranslateExternalVoiceParameters create() =>
      ASTTranslateExternalVoiceParameters._();
  @$core.override
  ASTTranslateExternalVoiceParameters createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ASTTranslateExternalVoiceParameters getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ASTTranslateExternalVoiceParameters>(create);
  static ASTTranslateExternalVoiceParameters? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get ttsVoice => $_getSZ(0);
  @$pb.TagNumber(1)
  set ttsVoice($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasTtsVoice() => $_has(0);
  @$pb.TagNumber(1)
  void clearTtsVoice() => $_clearField(1);
}

class ASTTranslateInternalSpeakerParameters extends $pb.GeneratedMessage {
  factory ASTTranslateInternalSpeakerParameters({
    $core.bool? isCustomSpeaker,
    $core.String? speakerId,
    $fixnum.Int64? speechRate,
    $core.String? ttsResourceId,
  }) {
    final result = create();
    if (isCustomSpeaker != null) result.isCustomSpeaker = isCustomSpeaker;
    if (speakerId != null) result.speakerId = speakerId;
    if (speechRate != null) result.speechRate = speechRate;
    if (ttsResourceId != null) result.ttsResourceId = ttsResourceId;
    return result;
  }

  ASTTranslateInternalSpeakerParameters._();

  factory ASTTranslateInternalSpeakerParameters.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ASTTranslateInternalSpeakerParameters.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ASTTranslateInternalSpeakerParameters',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'isCustomSpeaker')
    ..aOS(2, _omitFieldNames ? '' : 'speakerId')
    ..aInt64(3, _omitFieldNames ? '' : 'speechRate')
    ..aOS(4, _omitFieldNames ? '' : 'ttsResourceId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ASTTranslateInternalSpeakerParameters clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ASTTranslateInternalSpeakerParameters copyWith(
          void Function(ASTTranslateInternalSpeakerParameters) updates) =>
      super.copyWith((message) =>
              updates(message as ASTTranslateInternalSpeakerParameters))
          as ASTTranslateInternalSpeakerParameters;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ASTTranslateInternalSpeakerParameters create() =>
      ASTTranslateInternalSpeakerParameters._();
  @$core.override
  ASTTranslateInternalSpeakerParameters createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ASTTranslateInternalSpeakerParameters getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ASTTranslateInternalSpeakerParameters>(create);
  static ASTTranslateInternalSpeakerParameters? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get isCustomSpeaker => $_getBF(0);
  @$pb.TagNumber(1)
  set isCustomSpeaker($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasIsCustomSpeaker() => $_has(0);
  @$pb.TagNumber(1)
  void clearIsCustomSpeaker() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get speakerId => $_getSZ(1);
  @$pb.TagNumber(2)
  set speakerId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSpeakerId() => $_has(1);
  @$pb.TagNumber(2)
  void clearSpeakerId() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get speechRate => $_getI64(2);
  @$pb.TagNumber(3)
  set speechRate($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSpeechRate() => $_has(2);
  @$pb.TagNumber(3)
  void clearSpeechRate() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get ttsResourceId => $_getSZ(3);
  @$pb.TagNumber(4)
  set ttsResourceId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasTtsResourceId() => $_has(3);
  @$pb.TagNumber(4)
  void clearTtsResourceId() => $_clearField(4);
}

enum ASTTranslateVoiceParameters_Value {
  asttranslateInternalSpeakerParameters,
  asttranslateExternalVoiceParameters,
  notSet
}

class ASTTranslateVoiceParameters extends $pb.GeneratedMessage {
  factory ASTTranslateVoiceParameters({
    ASTTranslateInternalSpeakerParameters?
        asttranslateInternalSpeakerParameters,
    ASTTranslateExternalVoiceParameters? asttranslateExternalVoiceParameters,
  }) {
    final result = create();
    if (asttranslateInternalSpeakerParameters != null)
      result.asttranslateInternalSpeakerParameters =
          asttranslateInternalSpeakerParameters;
    if (asttranslateExternalVoiceParameters != null)
      result.asttranslateExternalVoiceParameters =
          asttranslateExternalVoiceParameters;
    return result;
  }

  ASTTranslateVoiceParameters._();

  factory ASTTranslateVoiceParameters.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ASTTranslateVoiceParameters.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, ASTTranslateVoiceParameters_Value>
      _ASTTranslateVoiceParameters_ValueByTag = {
    1: ASTTranslateVoiceParameters_Value.asttranslateInternalSpeakerParameters,
    2: ASTTranslateVoiceParameters_Value.asttranslateExternalVoiceParameters,
    0: ASTTranslateVoiceParameters_Value.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ASTTranslateVoiceParameters',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..oo(0, [1, 2])
    ..aOM<ASTTranslateInternalSpeakerParameters>(
        1, _omitFieldNames ? '' : 'asttranslateInternalSpeakerParameters',
        subBuilder: ASTTranslateInternalSpeakerParameters.create)
    ..aOM<ASTTranslateExternalVoiceParameters>(
        2, _omitFieldNames ? '' : 'asttranslateExternalVoiceParameters',
        subBuilder: ASTTranslateExternalVoiceParameters.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ASTTranslateVoiceParameters clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ASTTranslateVoiceParameters copyWith(
          void Function(ASTTranslateVoiceParameters) updates) =>
      super.copyWith(
              (message) => updates(message as ASTTranslateVoiceParameters))
          as ASTTranslateVoiceParameters;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ASTTranslateVoiceParameters create() =>
      ASTTranslateVoiceParameters._();
  @$core.override
  ASTTranslateVoiceParameters createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ASTTranslateVoiceParameters getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ASTTranslateVoiceParameters>(create);
  static ASTTranslateVoiceParameters? _defaultInstance;

  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  ASTTranslateVoiceParameters_Value whichValue() =>
      _ASTTranslateVoiceParameters_ValueByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  void clearValue() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  ASTTranslateInternalSpeakerParameters
      get asttranslateInternalSpeakerParameters => $_getN(0);
  @$pb.TagNumber(1)
  set asttranslateInternalSpeakerParameters(
          ASTTranslateInternalSpeakerParameters value) =>
      $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAsttranslateInternalSpeakerParameters() => $_has(0);
  @$pb.TagNumber(1)
  void clearAsttranslateInternalSpeakerParameters() => $_clearField(1);
  @$pb.TagNumber(1)
  ASTTranslateInternalSpeakerParameters
      ensureAsttranslateInternalSpeakerParameters() => $_ensure(0);

  @$pb.TagNumber(2)
  ASTTranslateExternalVoiceParameters get asttranslateExternalVoiceParameters =>
      $_getN(1);
  @$pb.TagNumber(2)
  set asttranslateExternalVoiceParameters(
          ASTTranslateExternalVoiceParameters value) =>
      $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasAsttranslateExternalVoiceParameters() => $_has(1);
  @$pb.TagNumber(2)
  void clearAsttranslateExternalVoiceParameters() => $_clearField(2);
  @$pb.TagNumber(2)
  ASTTranslateExternalVoiceParameters
      ensureAsttranslateExternalVoiceParameters() => $_ensure(1);
}

class ASTTranslateWorkflowSpec extends $pb.GeneratedMessage {
  factory ASTTranslateWorkflowSpec({
    $core.bool? denoise,
    $core.bool? enableSourceLanguageDetect,
    $core.bool? isCustomSpeaker,
    $1.ASTTranslateMode? mode,
    $core.String? resourceId,
    $core.String? speakerId,
    $fixnum.Int64? speechRate,
    $core.String? translationModel,
    $core.String? ttsResourceId,
    ASTTranslateVoiceParameters? voice,
  }) {
    final result = create();
    if (denoise != null) result.denoise = denoise;
    if (enableSourceLanguageDetect != null)
      result.enableSourceLanguageDetect = enableSourceLanguageDetect;
    if (isCustomSpeaker != null) result.isCustomSpeaker = isCustomSpeaker;
    if (mode != null) result.mode = mode;
    if (resourceId != null) result.resourceId = resourceId;
    if (speakerId != null) result.speakerId = speakerId;
    if (speechRate != null) result.speechRate = speechRate;
    if (translationModel != null) result.translationModel = translationModel;
    if (ttsResourceId != null) result.ttsResourceId = ttsResourceId;
    if (voice != null) result.voice = voice;
    return result;
  }

  ASTTranslateWorkflowSpec._();

  factory ASTTranslateWorkflowSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ASTTranslateWorkflowSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ASTTranslateWorkflowSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'denoise')
    ..aOB(2, _omitFieldNames ? '' : 'enableSourceLanguageDetect')
    ..aOB(3, _omitFieldNames ? '' : 'isCustomSpeaker')
    ..aE<$1.ASTTranslateMode>(4, _omitFieldNames ? '' : 'mode',
        enumValues: $1.ASTTranslateMode.values)
    ..aOS(5, _omitFieldNames ? '' : 'resourceId')
    ..aOS(6, _omitFieldNames ? '' : 'speakerId')
    ..aInt64(7, _omitFieldNames ? '' : 'speechRate')
    ..aOS(8, _omitFieldNames ? '' : 'translationModel')
    ..aOS(9, _omitFieldNames ? '' : 'ttsResourceId')
    ..aOM<ASTTranslateVoiceParameters>(10, _omitFieldNames ? '' : 'voice',
        subBuilder: ASTTranslateVoiceParameters.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ASTTranslateWorkflowSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ASTTranslateWorkflowSpec copyWith(
          void Function(ASTTranslateWorkflowSpec) updates) =>
      super.copyWith((message) => updates(message as ASTTranslateWorkflowSpec))
          as ASTTranslateWorkflowSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ASTTranslateWorkflowSpec create() => ASTTranslateWorkflowSpec._();
  @$core.override
  ASTTranslateWorkflowSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ASTTranslateWorkflowSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ASTTranslateWorkflowSpec>(create);
  static ASTTranslateWorkflowSpec? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get denoise => $_getBF(0);
  @$pb.TagNumber(1)
  set denoise($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDenoise() => $_has(0);
  @$pb.TagNumber(1)
  void clearDenoise() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get enableSourceLanguageDetect => $_getBF(1);
  @$pb.TagNumber(2)
  set enableSourceLanguageDetect($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasEnableSourceLanguageDetect() => $_has(1);
  @$pb.TagNumber(2)
  void clearEnableSourceLanguageDetect() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get isCustomSpeaker => $_getBF(2);
  @$pb.TagNumber(3)
  set isCustomSpeaker($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasIsCustomSpeaker() => $_has(2);
  @$pb.TagNumber(3)
  void clearIsCustomSpeaker() => $_clearField(3);

  @$pb.TagNumber(4)
  $1.ASTTranslateMode get mode => $_getN(3);
  @$pb.TagNumber(4)
  set mode($1.ASTTranslateMode value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasMode() => $_has(3);
  @$pb.TagNumber(4)
  void clearMode() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get resourceId => $_getSZ(4);
  @$pb.TagNumber(5)
  set resourceId($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasResourceId() => $_has(4);
  @$pb.TagNumber(5)
  void clearResourceId() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get speakerId => $_getSZ(5);
  @$pb.TagNumber(6)
  set speakerId($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasSpeakerId() => $_has(5);
  @$pb.TagNumber(6)
  void clearSpeakerId() => $_clearField(6);

  @$pb.TagNumber(7)
  $fixnum.Int64 get speechRate => $_getI64(6);
  @$pb.TagNumber(7)
  set speechRate($fixnum.Int64 value) => $_setInt64(6, value);
  @$pb.TagNumber(7)
  $core.bool hasSpeechRate() => $_has(6);
  @$pb.TagNumber(7)
  void clearSpeechRate() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get translationModel => $_getSZ(7);
  @$pb.TagNumber(8)
  set translationModel($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasTranslationModel() => $_has(7);
  @$pb.TagNumber(8)
  void clearTranslationModel() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get ttsResourceId => $_getSZ(8);
  @$pb.TagNumber(9)
  set ttsResourceId($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasTtsResourceId() => $_has(8);
  @$pb.TagNumber(9)
  void clearTtsResourceId() => $_clearField(9);

  @$pb.TagNumber(10)
  ASTTranslateVoiceParameters get voice => $_getN(9);
  @$pb.TagNumber(10)
  set voice(ASTTranslateVoiceParameters value) => $_setField(10, value);
  @$pb.TagNumber(10)
  $core.bool hasVoice() => $_has(9);
  @$pb.TagNumber(10)
  void clearVoice() => $_clearField(10);
  @$pb.TagNumber(10)
  ASTTranslateVoiceParameters ensureVoice() => $_ensure(9);
}

class ASTTranslateWorkspaceParameters extends $pb.GeneratedMessage {
  factory ASTTranslateWorkspaceParameters({
    $1.ASTTranslateWorkspaceParametersAgentType? agentType,
    $core.bool? denoise,
    $core.bool? e2e,
    $core.bool? enableSourceLanguageDetect,
    $1.WorkspaceInputMode? input,
    $core.bool? isCustomSpeaker,
    $core.String? langPair,
    $1.ASTTranslateMode? mode,
    $core.String? speakerId,
    $fixnum.Int64? speechRate,
    $core.String? translationModel,
    $core.String? ttsResourceId,
    ASTTranslateVoiceParameters? voice,
  }) {
    final result = create();
    if (agentType != null) result.agentType = agentType;
    if (denoise != null) result.denoise = denoise;
    if (e2e != null) result.e2e = e2e;
    if (enableSourceLanguageDetect != null)
      result.enableSourceLanguageDetect = enableSourceLanguageDetect;
    if (input != null) result.input = input;
    if (isCustomSpeaker != null) result.isCustomSpeaker = isCustomSpeaker;
    if (langPair != null) result.langPair = langPair;
    if (mode != null) result.mode = mode;
    if (speakerId != null) result.speakerId = speakerId;
    if (speechRate != null) result.speechRate = speechRate;
    if (translationModel != null) result.translationModel = translationModel;
    if (ttsResourceId != null) result.ttsResourceId = ttsResourceId;
    if (voice != null) result.voice = voice;
    return result;
  }

  ASTTranslateWorkspaceParameters._();

  factory ASTTranslateWorkspaceParameters.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ASTTranslateWorkspaceParameters.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ASTTranslateWorkspaceParameters',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<$1.ASTTranslateWorkspaceParametersAgentType>(
        1, _omitFieldNames ? '' : 'agentType',
        enumValues: $1.ASTTranslateWorkspaceParametersAgentType.values)
    ..aOB(2, _omitFieldNames ? '' : 'denoise')
    ..aOB(3, _omitFieldNames ? '' : 'e2e')
    ..aOB(4, _omitFieldNames ? '' : 'enableSourceLanguageDetect')
    ..aE<$1.WorkspaceInputMode>(5, _omitFieldNames ? '' : 'input',
        enumValues: $1.WorkspaceInputMode.values)
    ..aOB(6, _omitFieldNames ? '' : 'isCustomSpeaker')
    ..aOS(7, _omitFieldNames ? '' : 'langPair')
    ..aE<$1.ASTTranslateMode>(8, _omitFieldNames ? '' : 'mode',
        enumValues: $1.ASTTranslateMode.values)
    ..aOS(9, _omitFieldNames ? '' : 'speakerId')
    ..aInt64(10, _omitFieldNames ? '' : 'speechRate')
    ..aOS(11, _omitFieldNames ? '' : 'translationModel')
    ..aOS(12, _omitFieldNames ? '' : 'ttsResourceId')
    ..aOM<ASTTranslateVoiceParameters>(13, _omitFieldNames ? '' : 'voice',
        subBuilder: ASTTranslateVoiceParameters.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ASTTranslateWorkspaceParameters clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ASTTranslateWorkspaceParameters copyWith(
          void Function(ASTTranslateWorkspaceParameters) updates) =>
      super.copyWith(
              (message) => updates(message as ASTTranslateWorkspaceParameters))
          as ASTTranslateWorkspaceParameters;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ASTTranslateWorkspaceParameters create() =>
      ASTTranslateWorkspaceParameters._();
  @$core.override
  ASTTranslateWorkspaceParameters createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ASTTranslateWorkspaceParameters getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ASTTranslateWorkspaceParameters>(
          create);
  static ASTTranslateWorkspaceParameters? _defaultInstance;

  @$pb.TagNumber(1)
  $1.ASTTranslateWorkspaceParametersAgentType get agentType => $_getN(0);
  @$pb.TagNumber(1)
  set agentType($1.ASTTranslateWorkspaceParametersAgentType value) =>
      $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAgentType() => $_has(0);
  @$pb.TagNumber(1)
  void clearAgentType() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get denoise => $_getBF(1);
  @$pb.TagNumber(2)
  set denoise($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDenoise() => $_has(1);
  @$pb.TagNumber(2)
  void clearDenoise() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get e2e => $_getBF(2);
  @$pb.TagNumber(3)
  set e2e($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasE2e() => $_has(2);
  @$pb.TagNumber(3)
  void clearE2e() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get enableSourceLanguageDetect => $_getBF(3);
  @$pb.TagNumber(4)
  set enableSourceLanguageDetect($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasEnableSourceLanguageDetect() => $_has(3);
  @$pb.TagNumber(4)
  void clearEnableSourceLanguageDetect() => $_clearField(4);

  @$pb.TagNumber(5)
  $1.WorkspaceInputMode get input => $_getN(4);
  @$pb.TagNumber(5)
  set input($1.WorkspaceInputMode value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasInput() => $_has(4);
  @$pb.TagNumber(5)
  void clearInput() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.bool get isCustomSpeaker => $_getBF(5);
  @$pb.TagNumber(6)
  set isCustomSpeaker($core.bool value) => $_setBool(5, value);
  @$pb.TagNumber(6)
  $core.bool hasIsCustomSpeaker() => $_has(5);
  @$pb.TagNumber(6)
  void clearIsCustomSpeaker() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get langPair => $_getSZ(6);
  @$pb.TagNumber(7)
  set langPair($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasLangPair() => $_has(6);
  @$pb.TagNumber(7)
  void clearLangPair() => $_clearField(7);

  @$pb.TagNumber(8)
  $1.ASTTranslateMode get mode => $_getN(7);
  @$pb.TagNumber(8)
  set mode($1.ASTTranslateMode value) => $_setField(8, value);
  @$pb.TagNumber(8)
  $core.bool hasMode() => $_has(7);
  @$pb.TagNumber(8)
  void clearMode() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get speakerId => $_getSZ(8);
  @$pb.TagNumber(9)
  set speakerId($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasSpeakerId() => $_has(8);
  @$pb.TagNumber(9)
  void clearSpeakerId() => $_clearField(9);

  @$pb.TagNumber(10)
  $fixnum.Int64 get speechRate => $_getI64(9);
  @$pb.TagNumber(10)
  set speechRate($fixnum.Int64 value) => $_setInt64(9, value);
  @$pb.TagNumber(10)
  $core.bool hasSpeechRate() => $_has(9);
  @$pb.TagNumber(10)
  void clearSpeechRate() => $_clearField(10);

  @$pb.TagNumber(11)
  $core.String get translationModel => $_getSZ(10);
  @$pb.TagNumber(11)
  set translationModel($core.String value) => $_setString(10, value);
  @$pb.TagNumber(11)
  $core.bool hasTranslationModel() => $_has(10);
  @$pb.TagNumber(11)
  void clearTranslationModel() => $_clearField(11);

  @$pb.TagNumber(12)
  $core.String get ttsResourceId => $_getSZ(11);
  @$pb.TagNumber(12)
  set ttsResourceId($core.String value) => $_setString(11, value);
  @$pb.TagNumber(12)
  $core.bool hasTtsResourceId() => $_has(11);
  @$pb.TagNumber(12)
  void clearTtsResourceId() => $_clearField(12);

  @$pb.TagNumber(13)
  ASTTranslateVoiceParameters get voice => $_getN(12);
  @$pb.TagNumber(13)
  set voice(ASTTranslateVoiceParameters value) => $_setField(13, value);
  @$pb.TagNumber(13)
  $core.bool hasVoice() => $_has(12);
  @$pb.TagNumber(13)
  void clearVoice() => $_clearField(13);
  @$pb.TagNumber(13)
  ASTTranslateVoiceParameters ensureVoice() => $_ensure(12);
}

class ChatRoomWorkflowHistorySpec extends $pb.GeneratedMessage {
  factory ChatRoomWorkflowHistorySpec({
    $core.String? ttl,
  }) {
    final result = create();
    if (ttl != null) result.ttl = ttl;
    return result;
  }

  ChatRoomWorkflowHistorySpec._();

  factory ChatRoomWorkflowHistorySpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ChatRoomWorkflowHistorySpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ChatRoomWorkflowHistorySpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'ttl')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatRoomWorkflowHistorySpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatRoomWorkflowHistorySpec copyWith(
          void Function(ChatRoomWorkflowHistorySpec) updates) =>
      super.copyWith(
              (message) => updates(message as ChatRoomWorkflowHistorySpec))
          as ChatRoomWorkflowHistorySpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ChatRoomWorkflowHistorySpec create() =>
      ChatRoomWorkflowHistorySpec._();
  @$core.override
  ChatRoomWorkflowHistorySpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ChatRoomWorkflowHistorySpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ChatRoomWorkflowHistorySpec>(create);
  static ChatRoomWorkflowHistorySpec? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get ttl => $_getSZ(0);
  @$pb.TagNumber(1)
  set ttl($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasTtl() => $_has(0);
  @$pb.TagNumber(1)
  void clearTtl() => $_clearField(1);
}

class ChatRoomWorkflowSpec extends $pb.GeneratedMessage {
  factory ChatRoomWorkflowSpec({
    ChatRoomWorkflowHistorySpec? history,
    ChatRoomWorkflowTranscriptSpec? transcript,
  }) {
    final result = create();
    if (history != null) result.history = history;
    if (transcript != null) result.transcript = transcript;
    return result;
  }

  ChatRoomWorkflowSpec._();

  factory ChatRoomWorkflowSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ChatRoomWorkflowSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ChatRoomWorkflowSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<ChatRoomWorkflowHistorySpec>(1, _omitFieldNames ? '' : 'history',
        subBuilder: ChatRoomWorkflowHistorySpec.create)
    ..aOM<ChatRoomWorkflowTranscriptSpec>(
        2, _omitFieldNames ? '' : 'transcript',
        subBuilder: ChatRoomWorkflowTranscriptSpec.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatRoomWorkflowSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatRoomWorkflowSpec copyWith(void Function(ChatRoomWorkflowSpec) updates) =>
      super.copyWith((message) => updates(message as ChatRoomWorkflowSpec))
          as ChatRoomWorkflowSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ChatRoomWorkflowSpec create() => ChatRoomWorkflowSpec._();
  @$core.override
  ChatRoomWorkflowSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ChatRoomWorkflowSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ChatRoomWorkflowSpec>(create);
  static ChatRoomWorkflowSpec? _defaultInstance;

  @$pb.TagNumber(1)
  ChatRoomWorkflowHistorySpec get history => $_getN(0);
  @$pb.TagNumber(1)
  set history(ChatRoomWorkflowHistorySpec value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasHistory() => $_has(0);
  @$pb.TagNumber(1)
  void clearHistory() => $_clearField(1);
  @$pb.TagNumber(1)
  ChatRoomWorkflowHistorySpec ensureHistory() => $_ensure(0);

  @$pb.TagNumber(2)
  ChatRoomWorkflowTranscriptSpec get transcript => $_getN(1);
  @$pb.TagNumber(2)
  set transcript(ChatRoomWorkflowTranscriptSpec value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasTranscript() => $_has(1);
  @$pb.TagNumber(2)
  void clearTranscript() => $_clearField(2);
  @$pb.TagNumber(2)
  ChatRoomWorkflowTranscriptSpec ensureTranscript() => $_ensure(1);
}

class ChatRoomWorkflowTranscriptSpec extends $pb.GeneratedMessage {
  factory ChatRoomWorkflowTranscriptSpec({
    $core.String? asrModel,
    $core.bool? enabled,
  }) {
    final result = create();
    if (asrModel != null) result.asrModel = asrModel;
    if (enabled != null) result.enabled = enabled;
    return result;
  }

  ChatRoomWorkflowTranscriptSpec._();

  factory ChatRoomWorkflowTranscriptSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ChatRoomWorkflowTranscriptSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ChatRoomWorkflowTranscriptSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'asrModel')
    ..aOB(2, _omitFieldNames ? '' : 'enabled')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatRoomWorkflowTranscriptSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatRoomWorkflowTranscriptSpec copyWith(
          void Function(ChatRoomWorkflowTranscriptSpec) updates) =>
      super.copyWith(
              (message) => updates(message as ChatRoomWorkflowTranscriptSpec))
          as ChatRoomWorkflowTranscriptSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ChatRoomWorkflowTranscriptSpec create() =>
      ChatRoomWorkflowTranscriptSpec._();
  @$core.override
  ChatRoomWorkflowTranscriptSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ChatRoomWorkflowTranscriptSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ChatRoomWorkflowTranscriptSpec>(create);
  static ChatRoomWorkflowTranscriptSpec? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get asrModel => $_getSZ(0);
  @$pb.TagNumber(1)
  set asrModel($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAsrModel() => $_has(0);
  @$pb.TagNumber(1)
  void clearAsrModel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get enabled => $_getBF(1);
  @$pb.TagNumber(2)
  set enabled($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasEnabled() => $_has(1);
  @$pb.TagNumber(2)
  void clearEnabled() => $_clearField(2);
}

class ChatRoomWorkspaceHistoryParameters extends $pb.GeneratedMessage {
  factory ChatRoomWorkspaceHistoryParameters({
    $core.String? ttl,
  }) {
    final result = create();
    if (ttl != null) result.ttl = ttl;
    return result;
  }

  ChatRoomWorkspaceHistoryParameters._();

  factory ChatRoomWorkspaceHistoryParameters.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ChatRoomWorkspaceHistoryParameters.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ChatRoomWorkspaceHistoryParameters',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'ttl')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatRoomWorkspaceHistoryParameters clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatRoomWorkspaceHistoryParameters copyWith(
          void Function(ChatRoomWorkspaceHistoryParameters) updates) =>
      super.copyWith((message) =>
              updates(message as ChatRoomWorkspaceHistoryParameters))
          as ChatRoomWorkspaceHistoryParameters;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ChatRoomWorkspaceHistoryParameters create() =>
      ChatRoomWorkspaceHistoryParameters._();
  @$core.override
  ChatRoomWorkspaceHistoryParameters createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ChatRoomWorkspaceHistoryParameters getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ChatRoomWorkspaceHistoryParameters>(
          create);
  static ChatRoomWorkspaceHistoryParameters? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get ttl => $_getSZ(0);
  @$pb.TagNumber(1)
  set ttl($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasTtl() => $_has(0);
  @$pb.TagNumber(1)
  void clearTtl() => $_clearField(1);
}

class ChatRoomWorkspaceParameters extends $pb.GeneratedMessage {
  factory ChatRoomWorkspaceParameters({
    $1.ChatRoomWorkspaceParametersAgentType? agentType,
    ChatRoomWorkspaceHistoryParameters? history,
    $1.WorkspaceInputMode? input,
    $1.ChatRoomMode? mode,
    ChatRoomWorkspaceTranscriptParameters? transcript,
  }) {
    final result = create();
    if (agentType != null) result.agentType = agentType;
    if (history != null) result.history = history;
    if (input != null) result.input = input;
    if (mode != null) result.mode = mode;
    if (transcript != null) result.transcript = transcript;
    return result;
  }

  ChatRoomWorkspaceParameters._();

  factory ChatRoomWorkspaceParameters.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ChatRoomWorkspaceParameters.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ChatRoomWorkspaceParameters',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<$1.ChatRoomWorkspaceParametersAgentType>(
        1, _omitFieldNames ? '' : 'agentType',
        enumValues: $1.ChatRoomWorkspaceParametersAgentType.values)
    ..aOM<ChatRoomWorkspaceHistoryParameters>(
        2, _omitFieldNames ? '' : 'history',
        subBuilder: ChatRoomWorkspaceHistoryParameters.create)
    ..aE<$1.WorkspaceInputMode>(3, _omitFieldNames ? '' : 'input',
        enumValues: $1.WorkspaceInputMode.values)
    ..aE<$1.ChatRoomMode>(4, _omitFieldNames ? '' : 'mode',
        enumValues: $1.ChatRoomMode.values)
    ..aOM<ChatRoomWorkspaceTranscriptParameters>(
        5, _omitFieldNames ? '' : 'transcript',
        subBuilder: ChatRoomWorkspaceTranscriptParameters.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatRoomWorkspaceParameters clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatRoomWorkspaceParameters copyWith(
          void Function(ChatRoomWorkspaceParameters) updates) =>
      super.copyWith(
              (message) => updates(message as ChatRoomWorkspaceParameters))
          as ChatRoomWorkspaceParameters;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ChatRoomWorkspaceParameters create() =>
      ChatRoomWorkspaceParameters._();
  @$core.override
  ChatRoomWorkspaceParameters createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ChatRoomWorkspaceParameters getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ChatRoomWorkspaceParameters>(create);
  static ChatRoomWorkspaceParameters? _defaultInstance;

  @$pb.TagNumber(1)
  $1.ChatRoomWorkspaceParametersAgentType get agentType => $_getN(0);
  @$pb.TagNumber(1)
  set agentType($1.ChatRoomWorkspaceParametersAgentType value) =>
      $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAgentType() => $_has(0);
  @$pb.TagNumber(1)
  void clearAgentType() => $_clearField(1);

  @$pb.TagNumber(2)
  ChatRoomWorkspaceHistoryParameters get history => $_getN(1);
  @$pb.TagNumber(2)
  set history(ChatRoomWorkspaceHistoryParameters value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasHistory() => $_has(1);
  @$pb.TagNumber(2)
  void clearHistory() => $_clearField(2);
  @$pb.TagNumber(2)
  ChatRoomWorkspaceHistoryParameters ensureHistory() => $_ensure(1);

  @$pb.TagNumber(3)
  $1.WorkspaceInputMode get input => $_getN(2);
  @$pb.TagNumber(3)
  set input($1.WorkspaceInputMode value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasInput() => $_has(2);
  @$pb.TagNumber(3)
  void clearInput() => $_clearField(3);

  @$pb.TagNumber(4)
  $1.ChatRoomMode get mode => $_getN(3);
  @$pb.TagNumber(4)
  set mode($1.ChatRoomMode value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasMode() => $_has(3);
  @$pb.TagNumber(4)
  void clearMode() => $_clearField(4);

  @$pb.TagNumber(5)
  ChatRoomWorkspaceTranscriptParameters get transcript => $_getN(4);
  @$pb.TagNumber(5)
  set transcript(ChatRoomWorkspaceTranscriptParameters value) =>
      $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasTranscript() => $_has(4);
  @$pb.TagNumber(5)
  void clearTranscript() => $_clearField(5);
  @$pb.TagNumber(5)
  ChatRoomWorkspaceTranscriptParameters ensureTranscript() => $_ensure(4);
}

class ChatRoomWorkspaceTranscriptParameters extends $pb.GeneratedMessage {
  factory ChatRoomWorkspaceTranscriptParameters({
    $core.String? asrModel,
    $core.bool? enabled,
  }) {
    final result = create();
    if (asrModel != null) result.asrModel = asrModel;
    if (enabled != null) result.enabled = enabled;
    return result;
  }

  ChatRoomWorkspaceTranscriptParameters._();

  factory ChatRoomWorkspaceTranscriptParameters.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ChatRoomWorkspaceTranscriptParameters.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ChatRoomWorkspaceTranscriptParameters',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'asrModel')
    ..aOB(2, _omitFieldNames ? '' : 'enabled')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatRoomWorkspaceTranscriptParameters clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatRoomWorkspaceTranscriptParameters copyWith(
          void Function(ChatRoomWorkspaceTranscriptParameters) updates) =>
      super.copyWith((message) =>
              updates(message as ChatRoomWorkspaceTranscriptParameters))
          as ChatRoomWorkspaceTranscriptParameters;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ChatRoomWorkspaceTranscriptParameters create() =>
      ChatRoomWorkspaceTranscriptParameters._();
  @$core.override
  ChatRoomWorkspaceTranscriptParameters createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ChatRoomWorkspaceTranscriptParameters getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<
          ChatRoomWorkspaceTranscriptParameters>(create);
  static ChatRoomWorkspaceTranscriptParameters? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get asrModel => $_getSZ(0);
  @$pb.TagNumber(1)
  set asrModel($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAsrModel() => $_has(0);
  @$pb.TagNumber(1)
  void clearAsrModel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get enabled => $_getBF(1);
  @$pb.TagNumber(2)
  set enabled($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasEnabled() => $_has(1);
  @$pb.TagNumber(2)
  void clearEnabled() => $_clearField(2);
}

class Credential extends $pb.GeneratedMessage {
  factory Credential({
    CredentialBody? body,
    $core.String? createdAt,
    $core.String? description,
    $core.String? name,
    $core.String? provider,
    $core.String? updatedAt,
  }) {
    final result = create();
    if (body != null) result.body = body;
    if (createdAt != null) result.createdAt = createdAt;
    if (description != null) result.description = description;
    if (name != null) result.name = name;
    if (provider != null) result.provider = provider;
    if (updatedAt != null) result.updatedAt = updatedAt;
    return result;
  }

  Credential._();

  factory Credential.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Credential.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Credential',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<CredentialBody>(1, _omitFieldNames ? '' : 'body',
        subBuilder: CredentialBody.create)
    ..aOS(2, _omitFieldNames ? '' : 'createdAt')
    ..aOS(3, _omitFieldNames ? '' : 'description')
    ..aOS(4, _omitFieldNames ? '' : 'name')
    ..aOS(5, _omitFieldNames ? '' : 'provider')
    ..aOS(6, _omitFieldNames ? '' : 'updatedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Credential clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Credential copyWith(void Function(Credential) updates) =>
      super.copyWith((message) => updates(message as Credential)) as Credential;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Credential create() => Credential._();
  @$core.override
  Credential createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Credential getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<Credential>(create);
  static Credential? _defaultInstance;

  @$pb.TagNumber(1)
  CredentialBody get body => $_getN(0);
  @$pb.TagNumber(1)
  set body(CredentialBody value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasBody() => $_has(0);
  @$pb.TagNumber(1)
  void clearBody() => $_clearField(1);
  @$pb.TagNumber(1)
  CredentialBody ensureBody() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get createdAt => $_getSZ(1);
  @$pb.TagNumber(2)
  set createdAt($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCreatedAt() => $_has(1);
  @$pb.TagNumber(2)
  void clearCreatedAt() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get description => $_getSZ(2);
  @$pb.TagNumber(3)
  set description($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDescription() => $_has(2);
  @$pb.TagNumber(3)
  void clearDescription() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get name => $_getSZ(3);
  @$pb.TagNumber(4)
  set name($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasName() => $_has(3);
  @$pb.TagNumber(4)
  void clearName() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get provider => $_getSZ(4);
  @$pb.TagNumber(5)
  set provider($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasProvider() => $_has(4);
  @$pb.TagNumber(5)
  void clearProvider() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get updatedAt => $_getSZ(5);
  @$pb.TagNumber(6)
  set updatedAt($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasUpdatedAt() => $_has(5);
  @$pb.TagNumber(6)
  void clearUpdatedAt() => $_clearField(6);
}

enum CredentialBody_Value {
  openAicredentialBody,
  geminiCredentialBody,
  dashScopeCredentialBody,
  miniMaxCredentialBody,
  volcCredentialBody,
  notSet
}

class CredentialBody extends $pb.GeneratedMessage {
  factory CredentialBody({
    OpenAICredentialBody? openAicredentialBody,
    GeminiCredentialBody? geminiCredentialBody,
    DashScopeCredentialBody? dashScopeCredentialBody,
    MiniMaxCredentialBody? miniMaxCredentialBody,
    VolcCredentialBody? volcCredentialBody,
  }) {
    final result = create();
    if (openAicredentialBody != null)
      result.openAicredentialBody = openAicredentialBody;
    if (geminiCredentialBody != null)
      result.geminiCredentialBody = geminiCredentialBody;
    if (dashScopeCredentialBody != null)
      result.dashScopeCredentialBody = dashScopeCredentialBody;
    if (miniMaxCredentialBody != null)
      result.miniMaxCredentialBody = miniMaxCredentialBody;
    if (volcCredentialBody != null)
      result.volcCredentialBody = volcCredentialBody;
    return result;
  }

  CredentialBody._();

  factory CredentialBody.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CredentialBody.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, CredentialBody_Value>
      _CredentialBody_ValueByTag = {
    1: CredentialBody_Value.openAicredentialBody,
    2: CredentialBody_Value.geminiCredentialBody,
    3: CredentialBody_Value.dashScopeCredentialBody,
    4: CredentialBody_Value.miniMaxCredentialBody,
    5: CredentialBody_Value.volcCredentialBody,
    0: CredentialBody_Value.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CredentialBody',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..oo(0, [1, 2, 3, 4, 5])
    ..aOM<OpenAICredentialBody>(
        1, _omitFieldNames ? '' : 'openAicredentialBody',
        subBuilder: OpenAICredentialBody.create)
    ..aOM<GeminiCredentialBody>(
        2, _omitFieldNames ? '' : 'geminiCredentialBody',
        subBuilder: GeminiCredentialBody.create)
    ..aOM<DashScopeCredentialBody>(
        3, _omitFieldNames ? '' : 'dashScopeCredentialBody',
        subBuilder: DashScopeCredentialBody.create)
    ..aOM<MiniMaxCredentialBody>(
        4, _omitFieldNames ? '' : 'miniMaxCredentialBody',
        subBuilder: MiniMaxCredentialBody.create)
    ..aOM<VolcCredentialBody>(5, _omitFieldNames ? '' : 'volcCredentialBody',
        subBuilder: VolcCredentialBody.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialBody clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialBody copyWith(void Function(CredentialBody) updates) =>
      super.copyWith((message) => updates(message as CredentialBody))
          as CredentialBody;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CredentialBody create() => CredentialBody._();
  @$core.override
  CredentialBody createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CredentialBody getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CredentialBody>(create);
  static CredentialBody? _defaultInstance;

  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  @$pb.TagNumber(5)
  CredentialBody_Value whichValue() =>
      _CredentialBody_ValueByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  @$pb.TagNumber(5)
  void clearValue() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  OpenAICredentialBody get openAicredentialBody => $_getN(0);
  @$pb.TagNumber(1)
  set openAicredentialBody(OpenAICredentialBody value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasOpenAicredentialBody() => $_has(0);
  @$pb.TagNumber(1)
  void clearOpenAicredentialBody() => $_clearField(1);
  @$pb.TagNumber(1)
  OpenAICredentialBody ensureOpenAicredentialBody() => $_ensure(0);

  @$pb.TagNumber(2)
  GeminiCredentialBody get geminiCredentialBody => $_getN(1);
  @$pb.TagNumber(2)
  set geminiCredentialBody(GeminiCredentialBody value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasGeminiCredentialBody() => $_has(1);
  @$pb.TagNumber(2)
  void clearGeminiCredentialBody() => $_clearField(2);
  @$pb.TagNumber(2)
  GeminiCredentialBody ensureGeminiCredentialBody() => $_ensure(1);

  @$pb.TagNumber(3)
  DashScopeCredentialBody get dashScopeCredentialBody => $_getN(2);
  @$pb.TagNumber(3)
  set dashScopeCredentialBody(DashScopeCredentialBody value) =>
      $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasDashScopeCredentialBody() => $_has(2);
  @$pb.TagNumber(3)
  void clearDashScopeCredentialBody() => $_clearField(3);
  @$pb.TagNumber(3)
  DashScopeCredentialBody ensureDashScopeCredentialBody() => $_ensure(2);

  @$pb.TagNumber(4)
  MiniMaxCredentialBody get miniMaxCredentialBody => $_getN(3);
  @$pb.TagNumber(4)
  set miniMaxCredentialBody(MiniMaxCredentialBody value) =>
      $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasMiniMaxCredentialBody() => $_has(3);
  @$pb.TagNumber(4)
  void clearMiniMaxCredentialBody() => $_clearField(4);
  @$pb.TagNumber(4)
  MiniMaxCredentialBody ensureMiniMaxCredentialBody() => $_ensure(3);

  @$pb.TagNumber(5)
  VolcCredentialBody get volcCredentialBody => $_getN(4);
  @$pb.TagNumber(5)
  set volcCredentialBody(VolcCredentialBody value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasVolcCredentialBody() => $_has(4);
  @$pb.TagNumber(5)
  void clearVolcCredentialBody() => $_clearField(5);
  @$pb.TagNumber(5)
  VolcCredentialBody ensureVolcCredentialBody() => $_ensure(4);
}

class CredentialCreateRequest extends $pb.GeneratedMessage {
  factory CredentialCreateRequest({
    Credential? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  CredentialCreateRequest._();

  factory CredentialCreateRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CredentialCreateRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CredentialCreateRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Credential>(1, _omitFieldNames ? '' : 'value',
        subBuilder: Credential.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialCreateRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialCreateRequest copyWith(
          void Function(CredentialCreateRequest) updates) =>
      super.copyWith((message) => updates(message as CredentialCreateRequest))
          as CredentialCreateRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CredentialCreateRequest create() => CredentialCreateRequest._();
  @$core.override
  CredentialCreateRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CredentialCreateRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CredentialCreateRequest>(create);
  static CredentialCreateRequest? _defaultInstance;

  @$pb.TagNumber(1)
  Credential get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Credential value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Credential ensureValue() => $_ensure(0);
}

class CredentialCreateResponse extends $pb.GeneratedMessage {
  factory CredentialCreateResponse({
    Credential? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  CredentialCreateResponse._();

  factory CredentialCreateResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CredentialCreateResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CredentialCreateResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Credential>(1, _omitFieldNames ? '' : 'value',
        subBuilder: Credential.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialCreateResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialCreateResponse copyWith(
          void Function(CredentialCreateResponse) updates) =>
      super.copyWith((message) => updates(message as CredentialCreateResponse))
          as CredentialCreateResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CredentialCreateResponse create() => CredentialCreateResponse._();
  @$core.override
  CredentialCreateResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CredentialCreateResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CredentialCreateResponse>(create);
  static CredentialCreateResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Credential get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Credential value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Credential ensureValue() => $_ensure(0);
}

class CredentialDeleteRequest extends $pb.GeneratedMessage {
  factory CredentialDeleteRequest({
    $core.String? name,
  }) {
    final result = create();
    if (name != null) result.name = name;
    return result;
  }

  CredentialDeleteRequest._();

  factory CredentialDeleteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CredentialDeleteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CredentialDeleteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialDeleteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialDeleteRequest copyWith(
          void Function(CredentialDeleteRequest) updates) =>
      super.copyWith((message) => updates(message as CredentialDeleteRequest))
          as CredentialDeleteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CredentialDeleteRequest create() => CredentialDeleteRequest._();
  @$core.override
  CredentialDeleteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CredentialDeleteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CredentialDeleteRequest>(create);
  static CredentialDeleteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);
}

class CredentialDeleteResponse extends $pb.GeneratedMessage {
  factory CredentialDeleteResponse({
    Credential? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  CredentialDeleteResponse._();

  factory CredentialDeleteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CredentialDeleteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CredentialDeleteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Credential>(1, _omitFieldNames ? '' : 'value',
        subBuilder: Credential.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialDeleteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialDeleteResponse copyWith(
          void Function(CredentialDeleteResponse) updates) =>
      super.copyWith((message) => updates(message as CredentialDeleteResponse))
          as CredentialDeleteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CredentialDeleteResponse create() => CredentialDeleteResponse._();
  @$core.override
  CredentialDeleteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CredentialDeleteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CredentialDeleteResponse>(create);
  static CredentialDeleteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Credential get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Credential value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Credential ensureValue() => $_ensure(0);
}

class CredentialGetRequest extends $pb.GeneratedMessage {
  factory CredentialGetRequest({
    $core.String? name,
  }) {
    final result = create();
    if (name != null) result.name = name;
    return result;
  }

  CredentialGetRequest._();

  factory CredentialGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CredentialGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CredentialGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialGetRequest copyWith(void Function(CredentialGetRequest) updates) =>
      super.copyWith((message) => updates(message as CredentialGetRequest))
          as CredentialGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CredentialGetRequest create() => CredentialGetRequest._();
  @$core.override
  CredentialGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CredentialGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CredentialGetRequest>(create);
  static CredentialGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);
}

class CredentialGetResponse extends $pb.GeneratedMessage {
  factory CredentialGetResponse({
    Credential? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  CredentialGetResponse._();

  factory CredentialGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CredentialGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CredentialGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Credential>(1, _omitFieldNames ? '' : 'value',
        subBuilder: Credential.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialGetResponse copyWith(
          void Function(CredentialGetResponse) updates) =>
      super.copyWith((message) => updates(message as CredentialGetResponse))
          as CredentialGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CredentialGetResponse create() => CredentialGetResponse._();
  @$core.override
  CredentialGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CredentialGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CredentialGetResponse>(create);
  static CredentialGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Credential get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Credential value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Credential ensureValue() => $_ensure(0);
}

class CredentialListRequest extends $pb.GeneratedMessage {
  factory CredentialListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    return result;
  }

  CredentialListRequest._();

  factory CredentialListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CredentialListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CredentialListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialListRequest copyWith(
          void Function(CredentialListRequest) updates) =>
      super.copyWith((message) => updates(message as CredentialListRequest))
          as CredentialListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CredentialListRequest create() => CredentialListRequest._();
  @$core.override
  CredentialListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CredentialListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CredentialListRequest>(create);
  static CredentialListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get limit => $_getI64(1);
  @$pb.TagNumber(2)
  set limit($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLimit() => $_has(1);
  @$pb.TagNumber(2)
  void clearLimit() => $_clearField(2);
}

class CredentialListResponse extends $pb.GeneratedMessage {
  factory CredentialListResponse({
    $core.bool? hasNext,
    $core.Iterable<Credential>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  CredentialListResponse._();

  factory CredentialListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CredentialListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CredentialListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<Credential>(2, _omitFieldNames ? '' : 'items',
        subBuilder: Credential.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialListResponse copyWith(
          void Function(CredentialListResponse) updates) =>
      super.copyWith((message) => updates(message as CredentialListResponse))
          as CredentialListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CredentialListResponse create() => CredentialListResponse._();
  @$core.override
  CredentialListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CredentialListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CredentialListResponse>(create);
  static CredentialListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<Credential> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class CredentialPutRequest extends $pb.GeneratedMessage {
  factory CredentialPutRequest({
    Credential? body,
    $core.String? name,
  }) {
    final result = create();
    if (body != null) result.body = body;
    if (name != null) result.name = name;
    return result;
  }

  CredentialPutRequest._();

  factory CredentialPutRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CredentialPutRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CredentialPutRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Credential>(1, _omitFieldNames ? '' : 'body',
        subBuilder: Credential.create)
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialPutRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialPutRequest copyWith(void Function(CredentialPutRequest) updates) =>
      super.copyWith((message) => updates(message as CredentialPutRequest))
          as CredentialPutRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CredentialPutRequest create() => CredentialPutRequest._();
  @$core.override
  CredentialPutRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CredentialPutRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CredentialPutRequest>(create);
  static CredentialPutRequest? _defaultInstance;

  @$pb.TagNumber(1)
  Credential get body => $_getN(0);
  @$pb.TagNumber(1)
  set body(Credential value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasBody() => $_has(0);
  @$pb.TagNumber(1)
  void clearBody() => $_clearField(1);
  @$pb.TagNumber(1)
  Credential ensureBody() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);
}

class CredentialPutResponse extends $pb.GeneratedMessage {
  factory CredentialPutResponse({
    Credential? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  CredentialPutResponse._();

  factory CredentialPutResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory CredentialPutResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'CredentialPutResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Credential>(1, _omitFieldNames ? '' : 'value',
        subBuilder: Credential.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialPutResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  CredentialPutResponse copyWith(
          void Function(CredentialPutResponse) updates) =>
      super.copyWith((message) => updates(message as CredentialPutResponse))
          as CredentialPutResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static CredentialPutResponse create() => CredentialPutResponse._();
  @$core.override
  CredentialPutResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static CredentialPutResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<CredentialPutResponse>(create);
  static CredentialPutResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Credential get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Credential value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Credential ensureValue() => $_ensure(0);
}

class DashScopeCredentialBody extends $pb.GeneratedMessage {
  factory DashScopeCredentialBody({
    $core.String? apiKey,
    $core.String? baseUrl,
    $core.String? token,
  }) {
    final result = create();
    if (apiKey != null) result.apiKey = apiKey;
    if (baseUrl != null) result.baseUrl = baseUrl;
    if (token != null) result.token = token;
    return result;
  }

  DashScopeCredentialBody._();

  factory DashScopeCredentialBody.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DashScopeCredentialBody.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DashScopeCredentialBody',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'apiKey')
    ..aOS(2, _omitFieldNames ? '' : 'baseUrl')
    ..aOS(3, _omitFieldNames ? '' : 'token')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DashScopeCredentialBody clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DashScopeCredentialBody copyWith(
          void Function(DashScopeCredentialBody) updates) =>
      super.copyWith((message) => updates(message as DashScopeCredentialBody))
          as DashScopeCredentialBody;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DashScopeCredentialBody create() => DashScopeCredentialBody._();
  @$core.override
  DashScopeCredentialBody createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DashScopeCredentialBody getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DashScopeCredentialBody>(create);
  static DashScopeCredentialBody? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get apiKey => $_getSZ(0);
  @$pb.TagNumber(1)
  set apiKey($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasApiKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearApiKey() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get baseUrl => $_getSZ(1);
  @$pb.TagNumber(2)
  set baseUrl($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBaseUrl() => $_has(1);
  @$pb.TagNumber(2)
  void clearBaseUrl() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get token => $_getSZ(2);
  @$pb.TagNumber(3)
  set token($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasToken() => $_has(2);
  @$pb.TagNumber(3)
  void clearToken() => $_clearField(3);
}

class DashScopeTenantModelProviderData extends $pb.GeneratedMessage {
  factory DashScopeTenantModelProviderData({
    $1.DashScopeTenantModelProviderDataApiMode? apiMode,
    $core.String? upstreamModel,
  }) {
    final result = create();
    if (apiMode != null) result.apiMode = apiMode;
    if (upstreamModel != null) result.upstreamModel = upstreamModel;
    return result;
  }

  DashScopeTenantModelProviderData._();

  factory DashScopeTenantModelProviderData.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DashScopeTenantModelProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DashScopeTenantModelProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<$1.DashScopeTenantModelProviderDataApiMode>(
        1, _omitFieldNames ? '' : 'apiMode',
        enumValues: $1.DashScopeTenantModelProviderDataApiMode.values)
    ..aOS(2, _omitFieldNames ? '' : 'upstreamModel')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DashScopeTenantModelProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DashScopeTenantModelProviderData copyWith(
          void Function(DashScopeTenantModelProviderData) updates) =>
      super.copyWith(
              (message) => updates(message as DashScopeTenantModelProviderData))
          as DashScopeTenantModelProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DashScopeTenantModelProviderData create() =>
      DashScopeTenantModelProviderData._();
  @$core.override
  DashScopeTenantModelProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DashScopeTenantModelProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DashScopeTenantModelProviderData>(
          create);
  static DashScopeTenantModelProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  $1.DashScopeTenantModelProviderDataApiMode get apiMode => $_getN(0);
  @$pb.TagNumber(1)
  set apiMode($1.DashScopeTenantModelProviderDataApiMode value) =>
      $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasApiMode() => $_has(0);
  @$pb.TagNumber(1)
  void clearApiMode() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get upstreamModel => $_getSZ(1);
  @$pb.TagNumber(2)
  set upstreamModel($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasUpstreamModel() => $_has(1);
  @$pb.TagNumber(2)
  void clearUpstreamModel() => $_clearField(2);
}

class DashScopeTenantVoiceProviderData extends $pb.GeneratedMessage {
  factory DashScopeTenantVoiceProviderData({
    $0.Struct? raw,
    $core.String? voiceId,
  }) {
    final result = create();
    if (raw != null) result.raw = raw;
    if (voiceId != null) result.voiceId = voiceId;
    return result;
  }

  DashScopeTenantVoiceProviderData._();

  factory DashScopeTenantVoiceProviderData.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DashScopeTenantVoiceProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DashScopeTenantVoiceProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<$0.Struct>(1, _omitFieldNames ? '' : 'raw',
        subBuilder: $0.Struct.create)
    ..aOS(2, _omitFieldNames ? '' : 'voiceId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DashScopeTenantVoiceProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DashScopeTenantVoiceProviderData copyWith(
          void Function(DashScopeTenantVoiceProviderData) updates) =>
      super.copyWith(
              (message) => updates(message as DashScopeTenantVoiceProviderData))
          as DashScopeTenantVoiceProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DashScopeTenantVoiceProviderData create() =>
      DashScopeTenantVoiceProviderData._();
  @$core.override
  DashScopeTenantVoiceProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DashScopeTenantVoiceProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DashScopeTenantVoiceProviderData>(
          create);
  static DashScopeTenantVoiceProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  $0.Struct get raw => $_getN(0);
  @$pb.TagNumber(1)
  set raw($0.Struct value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasRaw() => $_has(0);
  @$pb.TagNumber(1)
  void clearRaw() => $_clearField(1);
  @$pb.TagNumber(1)
  $0.Struct ensureRaw() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get voiceId => $_getSZ(1);
  @$pb.TagNumber(2)
  set voiceId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasVoiceId() => $_has(1);
  @$pb.TagNumber(2)
  void clearVoiceId() => $_clearField(2);
}

class DoubaoRealtimeAIGCMetadata extends $pb.GeneratedMessage {
  factory DoubaoRealtimeAIGCMetadata({
    $core.String? contentProducer,
    $core.String? contentPropagator,
    $core.bool? enable,
    $core.String? produceId,
    $core.String? propagateId,
  }) {
    final result = create();
    if (contentProducer != null) result.contentProducer = contentProducer;
    if (contentPropagator != null) result.contentPropagator = contentPropagator;
    if (enable != null) result.enable = enable;
    if (produceId != null) result.produceId = produceId;
    if (propagateId != null) result.propagateId = propagateId;
    return result;
  }

  DoubaoRealtimeAIGCMetadata._();

  factory DoubaoRealtimeAIGCMetadata.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeAIGCMetadata.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeAIGCMetadata',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'contentProducer')
    ..aOS(2, _omitFieldNames ? '' : 'contentPropagator')
    ..aOB(3, _omitFieldNames ? '' : 'enable')
    ..aOS(4, _omitFieldNames ? '' : 'produceId')
    ..aOS(5, _omitFieldNames ? '' : 'propagateId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeAIGCMetadata clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeAIGCMetadata copyWith(
          void Function(DoubaoRealtimeAIGCMetadata) updates) =>
      super.copyWith(
              (message) => updates(message as DoubaoRealtimeAIGCMetadata))
          as DoubaoRealtimeAIGCMetadata;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeAIGCMetadata create() => DoubaoRealtimeAIGCMetadata._();
  @$core.override
  DoubaoRealtimeAIGCMetadata createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeAIGCMetadata getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeAIGCMetadata>(create);
  static DoubaoRealtimeAIGCMetadata? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get contentProducer => $_getSZ(0);
  @$pb.TagNumber(1)
  set contentProducer($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasContentProducer() => $_has(0);
  @$pb.TagNumber(1)
  void clearContentProducer() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get contentPropagator => $_getSZ(1);
  @$pb.TagNumber(2)
  set contentPropagator($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasContentPropagator() => $_has(1);
  @$pb.TagNumber(2)
  void clearContentPropagator() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get enable => $_getBF(2);
  @$pb.TagNumber(3)
  set enable($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasEnable() => $_has(2);
  @$pb.TagNumber(3)
  void clearEnable() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get produceId => $_getSZ(3);
  @$pb.TagNumber(4)
  set produceId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasProduceId() => $_has(3);
  @$pb.TagNumber(4)
  void clearProduceId() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get propagateId => $_getSZ(4);
  @$pb.TagNumber(5)
  set propagateId($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasPropagateId() => $_has(4);
  @$pb.TagNumber(5)
  void clearPropagateId() => $_clearField(5);
}

class DoubaoRealtimeASRContext extends $pb.GeneratedMessage {
  factory DoubaoRealtimeASRContext({
    $core.Iterable<$core.MapEntry<$core.String, $core.String>>? correctWords,
    $core.Iterable<DoubaoRealtimeASRHotword>? hotwords,
  }) {
    final result = create();
    if (correctWords != null) result.correctWords.addEntries(correctWords);
    if (hotwords != null) result.hotwords.addAll(hotwords);
    return result;
  }

  DoubaoRealtimeASRContext._();

  factory DoubaoRealtimeASRContext.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeASRContext.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeASRContext',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..m<$core.String, $core.String>(1, _omitFieldNames ? '' : 'correctWords',
        entryClassName: 'DoubaoRealtimeASRContext.CorrectWordsEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OS,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..pPM<DoubaoRealtimeASRHotword>(2, _omitFieldNames ? '' : 'hotwords',
        subBuilder: DoubaoRealtimeASRHotword.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeASRContext clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeASRContext copyWith(
          void Function(DoubaoRealtimeASRContext) updates) =>
      super.copyWith((message) => updates(message as DoubaoRealtimeASRContext))
          as DoubaoRealtimeASRContext;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeASRContext create() => DoubaoRealtimeASRContext._();
  @$core.override
  DoubaoRealtimeASRContext createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeASRContext getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeASRContext>(create);
  static DoubaoRealtimeASRContext? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbMap<$core.String, $core.String> get correctWords => $_getMap(0);

  @$pb.TagNumber(2)
  $pb.PbList<DoubaoRealtimeASRHotword> get hotwords => $_getList(1);
}

class DoubaoRealtimeASRExtension extends $pb.GeneratedMessage {
  factory DoubaoRealtimeASRExtension({
    DoubaoRealtimeASRExtra? extra,
  }) {
    final result = create();
    if (extra != null) result.extra = extra;
    return result;
  }

  DoubaoRealtimeASRExtension._();

  factory DoubaoRealtimeASRExtension.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeASRExtension.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeASRExtension',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DoubaoRealtimeASRExtra>(1, _omitFieldNames ? '' : 'extra',
        subBuilder: DoubaoRealtimeASRExtra.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeASRExtension clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeASRExtension copyWith(
          void Function(DoubaoRealtimeASRExtension) updates) =>
      super.copyWith(
              (message) => updates(message as DoubaoRealtimeASRExtension))
          as DoubaoRealtimeASRExtension;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeASRExtension create() => DoubaoRealtimeASRExtension._();
  @$core.override
  DoubaoRealtimeASRExtension createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeASRExtension getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeASRExtension>(create);
  static DoubaoRealtimeASRExtension? _defaultInstance;

  @$pb.TagNumber(1)
  DoubaoRealtimeASRExtra get extra => $_getN(0);
  @$pb.TagNumber(1)
  set extra(DoubaoRealtimeASRExtra value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasExtra() => $_has(0);
  @$pb.TagNumber(1)
  void clearExtra() => $_clearField(1);
  @$pb.TagNumber(1)
  DoubaoRealtimeASRExtra ensureExtra() => $_ensure(0);
}

class DoubaoRealtimeASRExtra extends $pb.GeneratedMessage {
  factory DoubaoRealtimeASRExtra({
    $core.String? boostingTableId,
    $core.String? boostingTableName,
    DoubaoRealtimeASRContext? context,
    $core.bool? enableAsrTwopass,
    $core.bool? enableCustomVad,
    $fixnum.Int64? endSmoothWindowMs,
    $core.String? regexCorrectTableId,
    $core.String? regexCorrectTableName,
  }) {
    final result = create();
    if (boostingTableId != null) result.boostingTableId = boostingTableId;
    if (boostingTableName != null) result.boostingTableName = boostingTableName;
    if (context != null) result.context = context;
    if (enableAsrTwopass != null) result.enableAsrTwopass = enableAsrTwopass;
    if (enableCustomVad != null) result.enableCustomVad = enableCustomVad;
    if (endSmoothWindowMs != null) result.endSmoothWindowMs = endSmoothWindowMs;
    if (regexCorrectTableId != null)
      result.regexCorrectTableId = regexCorrectTableId;
    if (regexCorrectTableName != null)
      result.regexCorrectTableName = regexCorrectTableName;
    return result;
  }

  DoubaoRealtimeASRExtra._();

  factory DoubaoRealtimeASRExtra.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeASRExtra.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeASRExtra',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'boostingTableId')
    ..aOS(2, _omitFieldNames ? '' : 'boostingTableName')
    ..aOM<DoubaoRealtimeASRContext>(3, _omitFieldNames ? '' : 'context',
        subBuilder: DoubaoRealtimeASRContext.create)
    ..aOB(4, _omitFieldNames ? '' : 'enableAsrTwopass')
    ..aOB(5, _omitFieldNames ? '' : 'enableCustomVad')
    ..aInt64(6, _omitFieldNames ? '' : 'endSmoothWindowMs')
    ..aOS(7, _omitFieldNames ? '' : 'regexCorrectTableId')
    ..aOS(8, _omitFieldNames ? '' : 'regexCorrectTableName')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeASRExtra clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeASRExtra copyWith(
          void Function(DoubaoRealtimeASRExtra) updates) =>
      super.copyWith((message) => updates(message as DoubaoRealtimeASRExtra))
          as DoubaoRealtimeASRExtra;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeASRExtra create() => DoubaoRealtimeASRExtra._();
  @$core.override
  DoubaoRealtimeASRExtra createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeASRExtra getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeASRExtra>(create);
  static DoubaoRealtimeASRExtra? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get boostingTableId => $_getSZ(0);
  @$pb.TagNumber(1)
  set boostingTableId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasBoostingTableId() => $_has(0);
  @$pb.TagNumber(1)
  void clearBoostingTableId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get boostingTableName => $_getSZ(1);
  @$pb.TagNumber(2)
  set boostingTableName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBoostingTableName() => $_has(1);
  @$pb.TagNumber(2)
  void clearBoostingTableName() => $_clearField(2);

  @$pb.TagNumber(3)
  DoubaoRealtimeASRContext get context => $_getN(2);
  @$pb.TagNumber(3)
  set context(DoubaoRealtimeASRContext value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasContext() => $_has(2);
  @$pb.TagNumber(3)
  void clearContext() => $_clearField(3);
  @$pb.TagNumber(3)
  DoubaoRealtimeASRContext ensureContext() => $_ensure(2);

  @$pb.TagNumber(4)
  $core.bool get enableAsrTwopass => $_getBF(3);
  @$pb.TagNumber(4)
  set enableAsrTwopass($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasEnableAsrTwopass() => $_has(3);
  @$pb.TagNumber(4)
  void clearEnableAsrTwopass() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get enableCustomVad => $_getBF(4);
  @$pb.TagNumber(5)
  set enableCustomVad($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasEnableCustomVad() => $_has(4);
  @$pb.TagNumber(5)
  void clearEnableCustomVad() => $_clearField(5);

  @$pb.TagNumber(6)
  $fixnum.Int64 get endSmoothWindowMs => $_getI64(5);
  @$pb.TagNumber(6)
  set endSmoothWindowMs($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasEndSmoothWindowMs() => $_has(5);
  @$pb.TagNumber(6)
  void clearEndSmoothWindowMs() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get regexCorrectTableId => $_getSZ(6);
  @$pb.TagNumber(7)
  set regexCorrectTableId($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasRegexCorrectTableId() => $_has(6);
  @$pb.TagNumber(7)
  void clearRegexCorrectTableId() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get regexCorrectTableName => $_getSZ(7);
  @$pb.TagNumber(8)
  set regexCorrectTableName($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasRegexCorrectTableName() => $_has(7);
  @$pb.TagNumber(8)
  void clearRegexCorrectTableName() => $_clearField(8);
}

class DoubaoRealtimeASRHotword extends $pb.GeneratedMessage {
  factory DoubaoRealtimeASRHotword({
    $core.String? word,
  }) {
    final result = create();
    if (word != null) result.word = word;
    return result;
  }

  DoubaoRealtimeASRHotword._();

  factory DoubaoRealtimeASRHotword.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeASRHotword.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeASRHotword',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'word')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeASRHotword clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeASRHotword copyWith(
          void Function(DoubaoRealtimeASRHotword) updates) =>
      super.copyWith((message) => updates(message as DoubaoRealtimeASRHotword))
          as DoubaoRealtimeASRHotword;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeASRHotword create() => DoubaoRealtimeASRHotword._();
  @$core.override
  DoubaoRealtimeASRHotword createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeASRHotword getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeASRHotword>(create);
  static DoubaoRealtimeASRHotword? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get word => $_getSZ(0);
  @$pb.TagNumber(1)
  set word($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasWord() => $_has(0);
  @$pb.TagNumber(1)
  void clearWord() => $_clearField(1);
}

class DoubaoRealtimeAudio extends $pb.GeneratedMessage {
  factory DoubaoRealtimeAudio({
    DoubaoRealtimeAudioInput? input,
    DoubaoRealtimeAudioOutput? output,
  }) {
    final result = create();
    if (input != null) result.input = input;
    if (output != null) result.output = output;
    return result;
  }

  DoubaoRealtimeAudio._();

  factory DoubaoRealtimeAudio.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeAudio.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeAudio',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DoubaoRealtimeAudioInput>(1, _omitFieldNames ? '' : 'input',
        subBuilder: DoubaoRealtimeAudioInput.create)
    ..aOM<DoubaoRealtimeAudioOutput>(2, _omitFieldNames ? '' : 'output',
        subBuilder: DoubaoRealtimeAudioOutput.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeAudio clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeAudio copyWith(void Function(DoubaoRealtimeAudio) updates) =>
      super.copyWith((message) => updates(message as DoubaoRealtimeAudio))
          as DoubaoRealtimeAudio;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeAudio create() => DoubaoRealtimeAudio._();
  @$core.override
  DoubaoRealtimeAudio createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeAudio getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeAudio>(create);
  static DoubaoRealtimeAudio? _defaultInstance;

  @$pb.TagNumber(1)
  DoubaoRealtimeAudioInput get input => $_getN(0);
  @$pb.TagNumber(1)
  set input(DoubaoRealtimeAudioInput value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasInput() => $_has(0);
  @$pb.TagNumber(1)
  void clearInput() => $_clearField(1);
  @$pb.TagNumber(1)
  DoubaoRealtimeAudioInput ensureInput() => $_ensure(0);

  @$pb.TagNumber(2)
  DoubaoRealtimeAudioOutput get output => $_getN(1);
  @$pb.TagNumber(2)
  set output(DoubaoRealtimeAudioOutput value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasOutput() => $_has(1);
  @$pb.TagNumber(2)
  void clearOutput() => $_clearField(2);
  @$pb.TagNumber(2)
  DoubaoRealtimeAudioOutput ensureOutput() => $_ensure(1);
}

class DoubaoRealtimeAudioFormat extends $pb.GeneratedMessage {
  factory DoubaoRealtimeAudioFormat({
    $fixnum.Int64? rate,
    $1.DoubaoRealtimeAudioFormatType? type,
  }) {
    final result = create();
    if (rate != null) result.rate = rate;
    if (type != null) result.type = type;
    return result;
  }

  DoubaoRealtimeAudioFormat._();

  factory DoubaoRealtimeAudioFormat.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeAudioFormat.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeAudioFormat',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'rate')
    ..aE<$1.DoubaoRealtimeAudioFormatType>(2, _omitFieldNames ? '' : 'type',
        enumValues: $1.DoubaoRealtimeAudioFormatType.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeAudioFormat clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeAudioFormat copyWith(
          void Function(DoubaoRealtimeAudioFormat) updates) =>
      super.copyWith((message) => updates(message as DoubaoRealtimeAudioFormat))
          as DoubaoRealtimeAudioFormat;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeAudioFormat create() => DoubaoRealtimeAudioFormat._();
  @$core.override
  DoubaoRealtimeAudioFormat createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeAudioFormat getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeAudioFormat>(create);
  static DoubaoRealtimeAudioFormat? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get rate => $_getI64(0);
  @$pb.TagNumber(1)
  set rate($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasRate() => $_has(0);
  @$pb.TagNumber(1)
  void clearRate() => $_clearField(1);

  @$pb.TagNumber(2)
  $1.DoubaoRealtimeAudioFormatType get type => $_getN(1);
  @$pb.TagNumber(2)
  set type($1.DoubaoRealtimeAudioFormatType value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasType() => $_has(1);
  @$pb.TagNumber(2)
  void clearType() => $_clearField(2);
}

class DoubaoRealtimeAudioInput extends $pb.GeneratedMessage {
  factory DoubaoRealtimeAudioInput({
    DoubaoRealtimeAudioFormat? format,
  }) {
    final result = create();
    if (format != null) result.format = format;
    return result;
  }

  DoubaoRealtimeAudioInput._();

  factory DoubaoRealtimeAudioInput.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeAudioInput.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeAudioInput',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DoubaoRealtimeAudioFormat>(1, _omitFieldNames ? '' : 'format',
        subBuilder: DoubaoRealtimeAudioFormat.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeAudioInput clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeAudioInput copyWith(
          void Function(DoubaoRealtimeAudioInput) updates) =>
      super.copyWith((message) => updates(message as DoubaoRealtimeAudioInput))
          as DoubaoRealtimeAudioInput;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeAudioInput create() => DoubaoRealtimeAudioInput._();
  @$core.override
  DoubaoRealtimeAudioInput createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeAudioInput getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeAudioInput>(create);
  static DoubaoRealtimeAudioInput? _defaultInstance;

  @$pb.TagNumber(1)
  DoubaoRealtimeAudioFormat get format => $_getN(0);
  @$pb.TagNumber(1)
  set format(DoubaoRealtimeAudioFormat value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasFormat() => $_has(0);
  @$pb.TagNumber(1)
  void clearFormat() => $_clearField(1);
  @$pb.TagNumber(1)
  DoubaoRealtimeAudioFormat ensureFormat() => $_ensure(0);
}

class DoubaoRealtimeAudioOutput extends $pb.GeneratedMessage {
  factory DoubaoRealtimeAudioOutput({
    DoubaoRealtimeAudioFormat? format,
    $fixnum.Int64? loudness,
    $fixnum.Int64? speed,
    $core.String? voice,
  }) {
    final result = create();
    if (format != null) result.format = format;
    if (loudness != null) result.loudness = loudness;
    if (speed != null) result.speed = speed;
    if (voice != null) result.voice = voice;
    return result;
  }

  DoubaoRealtimeAudioOutput._();

  factory DoubaoRealtimeAudioOutput.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeAudioOutput.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeAudioOutput',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DoubaoRealtimeAudioFormat>(1, _omitFieldNames ? '' : 'format',
        subBuilder: DoubaoRealtimeAudioFormat.create)
    ..aInt64(2, _omitFieldNames ? '' : 'loudness')
    ..aInt64(3, _omitFieldNames ? '' : 'speed')
    ..aOS(4, _omitFieldNames ? '' : 'voice')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeAudioOutput clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeAudioOutput copyWith(
          void Function(DoubaoRealtimeAudioOutput) updates) =>
      super.copyWith((message) => updates(message as DoubaoRealtimeAudioOutput))
          as DoubaoRealtimeAudioOutput;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeAudioOutput create() => DoubaoRealtimeAudioOutput._();
  @$core.override
  DoubaoRealtimeAudioOutput createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeAudioOutput getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeAudioOutput>(create);
  static DoubaoRealtimeAudioOutput? _defaultInstance;

  @$pb.TagNumber(1)
  DoubaoRealtimeAudioFormat get format => $_getN(0);
  @$pb.TagNumber(1)
  set format(DoubaoRealtimeAudioFormat value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasFormat() => $_has(0);
  @$pb.TagNumber(1)
  void clearFormat() => $_clearField(1);
  @$pb.TagNumber(1)
  DoubaoRealtimeAudioFormat ensureFormat() => $_ensure(0);

  @$pb.TagNumber(2)
  $fixnum.Int64 get loudness => $_getI64(1);
  @$pb.TagNumber(2)
  set loudness($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLoudness() => $_has(1);
  @$pb.TagNumber(2)
  void clearLoudness() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get speed => $_getI64(2);
  @$pb.TagNumber(3)
  set speed($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSpeed() => $_has(2);
  @$pb.TagNumber(3)
  void clearSpeed() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get voice => $_getSZ(3);
  @$pb.TagNumber(4)
  set voice($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasVoice() => $_has(3);
  @$pb.TagNumber(4)
  void clearVoice() => $_clearField(4);
}

class DoubaoRealtimeDialogExtension extends $pb.GeneratedMessage {
  factory DoubaoRealtimeDialogExtension({
    DoubaoRealtimeDialogExtra? extra,
  }) {
    final result = create();
    if (extra != null) result.extra = extra;
    return result;
  }

  DoubaoRealtimeDialogExtension._();

  factory DoubaoRealtimeDialogExtension.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeDialogExtension.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeDialogExtension',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DoubaoRealtimeDialogExtra>(1, _omitFieldNames ? '' : 'extra',
        subBuilder: DoubaoRealtimeDialogExtra.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeDialogExtension clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeDialogExtension copyWith(
          void Function(DoubaoRealtimeDialogExtension) updates) =>
      super.copyWith(
              (message) => updates(message as DoubaoRealtimeDialogExtension))
          as DoubaoRealtimeDialogExtension;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeDialogExtension create() =>
      DoubaoRealtimeDialogExtension._();
  @$core.override
  DoubaoRealtimeDialogExtension createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeDialogExtension getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeDialogExtension>(create);
  static DoubaoRealtimeDialogExtension? _defaultInstance;

  @$pb.TagNumber(1)
  DoubaoRealtimeDialogExtra get extra => $_getN(0);
  @$pb.TagNumber(1)
  set extra(DoubaoRealtimeDialogExtra value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasExtra() => $_has(0);
  @$pb.TagNumber(1)
  void clearExtra() => $_clearField(1);
  @$pb.TagNumber(1)
  DoubaoRealtimeDialogExtra ensureExtra() => $_ensure(0);
}

class DoubaoRealtimeDialogExtra extends $pb.GeneratedMessage {
  factory DoubaoRealtimeDialogExtra({
    $core.String? auditResponse,
    $core.bool? enableConversationTruncate,
    $core.bool? enableLoudnessNorm,
    $core.bool? enableMusic,
    $core.bool? enableUserQueryExit,
    $core.bool? enableVolcWebsearch,
    $core.bool? strictAudit,
    $core.String? volcWebsearchBotId,
    $core.String? volcWebsearchNoResultMessage,
    $fixnum.Int64? volcWebsearchResultCount,
    $1.DoubaoRealtimeDialogExtraVolcWebsearchType? volcWebsearchType,
  }) {
    final result = create();
    if (auditResponse != null) result.auditResponse = auditResponse;
    if (enableConversationTruncate != null)
      result.enableConversationTruncate = enableConversationTruncate;
    if (enableLoudnessNorm != null)
      result.enableLoudnessNorm = enableLoudnessNorm;
    if (enableMusic != null) result.enableMusic = enableMusic;
    if (enableUserQueryExit != null)
      result.enableUserQueryExit = enableUserQueryExit;
    if (enableVolcWebsearch != null)
      result.enableVolcWebsearch = enableVolcWebsearch;
    if (strictAudit != null) result.strictAudit = strictAudit;
    if (volcWebsearchBotId != null)
      result.volcWebsearchBotId = volcWebsearchBotId;
    if (volcWebsearchNoResultMessage != null)
      result.volcWebsearchNoResultMessage = volcWebsearchNoResultMessage;
    if (volcWebsearchResultCount != null)
      result.volcWebsearchResultCount = volcWebsearchResultCount;
    if (volcWebsearchType != null) result.volcWebsearchType = volcWebsearchType;
    return result;
  }

  DoubaoRealtimeDialogExtra._();

  factory DoubaoRealtimeDialogExtra.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeDialogExtra.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeDialogExtra',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'auditResponse')
    ..aOB(2, _omitFieldNames ? '' : 'enableConversationTruncate')
    ..aOB(3, _omitFieldNames ? '' : 'enableLoudnessNorm')
    ..aOB(4, _omitFieldNames ? '' : 'enableMusic')
    ..aOB(5, _omitFieldNames ? '' : 'enableUserQueryExit')
    ..aOB(6, _omitFieldNames ? '' : 'enableVolcWebsearch')
    ..aOB(7, _omitFieldNames ? '' : 'strictAudit')
    ..aOS(8, _omitFieldNames ? '' : 'volcWebsearchBotId')
    ..aOS(9, _omitFieldNames ? '' : 'volcWebsearchNoResultMessage')
    ..aInt64(10, _omitFieldNames ? '' : 'volcWebsearchResultCount')
    ..aE<$1.DoubaoRealtimeDialogExtraVolcWebsearchType>(
        11, _omitFieldNames ? '' : 'volcWebsearchType',
        enumValues: $1.DoubaoRealtimeDialogExtraVolcWebsearchType.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeDialogExtra clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeDialogExtra copyWith(
          void Function(DoubaoRealtimeDialogExtra) updates) =>
      super.copyWith((message) => updates(message as DoubaoRealtimeDialogExtra))
          as DoubaoRealtimeDialogExtra;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeDialogExtra create() => DoubaoRealtimeDialogExtra._();
  @$core.override
  DoubaoRealtimeDialogExtra createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeDialogExtra getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeDialogExtra>(create);
  static DoubaoRealtimeDialogExtra? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get auditResponse => $_getSZ(0);
  @$pb.TagNumber(1)
  set auditResponse($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAuditResponse() => $_has(0);
  @$pb.TagNumber(1)
  void clearAuditResponse() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get enableConversationTruncate => $_getBF(1);
  @$pb.TagNumber(2)
  set enableConversationTruncate($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasEnableConversationTruncate() => $_has(1);
  @$pb.TagNumber(2)
  void clearEnableConversationTruncate() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get enableLoudnessNorm => $_getBF(2);
  @$pb.TagNumber(3)
  set enableLoudnessNorm($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasEnableLoudnessNorm() => $_has(2);
  @$pb.TagNumber(3)
  void clearEnableLoudnessNorm() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get enableMusic => $_getBF(3);
  @$pb.TagNumber(4)
  set enableMusic($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasEnableMusic() => $_has(3);
  @$pb.TagNumber(4)
  void clearEnableMusic() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get enableUserQueryExit => $_getBF(4);
  @$pb.TagNumber(5)
  set enableUserQueryExit($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasEnableUserQueryExit() => $_has(4);
  @$pb.TagNumber(5)
  void clearEnableUserQueryExit() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.bool get enableVolcWebsearch => $_getBF(5);
  @$pb.TagNumber(6)
  set enableVolcWebsearch($core.bool value) => $_setBool(5, value);
  @$pb.TagNumber(6)
  $core.bool hasEnableVolcWebsearch() => $_has(5);
  @$pb.TagNumber(6)
  void clearEnableVolcWebsearch() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get strictAudit => $_getBF(6);
  @$pb.TagNumber(7)
  set strictAudit($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasStrictAudit() => $_has(6);
  @$pb.TagNumber(7)
  void clearStrictAudit() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get volcWebsearchBotId => $_getSZ(7);
  @$pb.TagNumber(8)
  set volcWebsearchBotId($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasVolcWebsearchBotId() => $_has(7);
  @$pb.TagNumber(8)
  void clearVolcWebsearchBotId() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get volcWebsearchNoResultMessage => $_getSZ(8);
  @$pb.TagNumber(9)
  set volcWebsearchNoResultMessage($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasVolcWebsearchNoResultMessage() => $_has(8);
  @$pb.TagNumber(9)
  void clearVolcWebsearchNoResultMessage() => $_clearField(9);

  @$pb.TagNumber(10)
  $fixnum.Int64 get volcWebsearchResultCount => $_getI64(9);
  @$pb.TagNumber(10)
  set volcWebsearchResultCount($fixnum.Int64 value) => $_setInt64(9, value);
  @$pb.TagNumber(10)
  $core.bool hasVolcWebsearchResultCount() => $_has(9);
  @$pb.TagNumber(10)
  void clearVolcWebsearchResultCount() => $_clearField(10);

  @$pb.TagNumber(11)
  $1.DoubaoRealtimeDialogExtraVolcWebsearchType get volcWebsearchType =>
      $_getN(10);
  @$pb.TagNumber(11)
  set volcWebsearchType($1.DoubaoRealtimeDialogExtraVolcWebsearchType value) =>
      $_setField(11, value);
  @$pb.TagNumber(11)
  $core.bool hasVolcWebsearchType() => $_has(10);
  @$pb.TagNumber(11)
  void clearVolcWebsearchType() => $_clearField(11);
}

class DoubaoRealtimeExtension extends $pb.GeneratedMessage {
  factory DoubaoRealtimeExtension({
    DoubaoRealtimeASRExtension? asr,
    DoubaoRealtimeDialogExtension? dialog,
    DoubaoRealtimeTTSExtension? tts,
  }) {
    final result = create();
    if (asr != null) result.asr = asr;
    if (dialog != null) result.dialog = dialog;
    if (tts != null) result.tts = tts;
    return result;
  }

  DoubaoRealtimeExtension._();

  factory DoubaoRealtimeExtension.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeExtension.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeExtension',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DoubaoRealtimeASRExtension>(1, _omitFieldNames ? '' : 'asr',
        subBuilder: DoubaoRealtimeASRExtension.create)
    ..aOM<DoubaoRealtimeDialogExtension>(2, _omitFieldNames ? '' : 'dialog',
        subBuilder: DoubaoRealtimeDialogExtension.create)
    ..aOM<DoubaoRealtimeTTSExtension>(3, _omitFieldNames ? '' : 'tts',
        subBuilder: DoubaoRealtimeTTSExtension.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeExtension clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeExtension copyWith(
          void Function(DoubaoRealtimeExtension) updates) =>
      super.copyWith((message) => updates(message as DoubaoRealtimeExtension))
          as DoubaoRealtimeExtension;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeExtension create() => DoubaoRealtimeExtension._();
  @$core.override
  DoubaoRealtimeExtension createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeExtension getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeExtension>(create);
  static DoubaoRealtimeExtension? _defaultInstance;

  @$pb.TagNumber(1)
  DoubaoRealtimeASRExtension get asr => $_getN(0);
  @$pb.TagNumber(1)
  set asr(DoubaoRealtimeASRExtension value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAsr() => $_has(0);
  @$pb.TagNumber(1)
  void clearAsr() => $_clearField(1);
  @$pb.TagNumber(1)
  DoubaoRealtimeASRExtension ensureAsr() => $_ensure(0);

  @$pb.TagNumber(2)
  DoubaoRealtimeDialogExtension get dialog => $_getN(1);
  @$pb.TagNumber(2)
  set dialog(DoubaoRealtimeDialogExtension value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasDialog() => $_has(1);
  @$pb.TagNumber(2)
  void clearDialog() => $_clearField(2);
  @$pb.TagNumber(2)
  DoubaoRealtimeDialogExtension ensureDialog() => $_ensure(1);

  @$pb.TagNumber(3)
  DoubaoRealtimeTTSExtension get tts => $_getN(2);
  @$pb.TagNumber(3)
  set tts(DoubaoRealtimeTTSExtension value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasTts() => $_has(2);
  @$pb.TagNumber(3)
  void clearTts() => $_clearField(3);
  @$pb.TagNumber(3)
  DoubaoRealtimeTTSExtension ensureTts() => $_ensure(2);
}

class DoubaoRealtimeFunctionTool extends $pb.GeneratedMessage {
  factory DoubaoRealtimeFunctionTool({
    $core.String? description,
    $core.String? name,
    DoubaoRealtimeJSONSchema? parameters,
    $core.bool? strict,
    $1.DoubaoRealtimeFunctionToolType? type,
  }) {
    final result = create();
    if (description != null) result.description = description;
    if (name != null) result.name = name;
    if (parameters != null) result.parameters = parameters;
    if (strict != null) result.strict = strict;
    if (type != null) result.type = type;
    return result;
  }

  DoubaoRealtimeFunctionTool._();

  factory DoubaoRealtimeFunctionTool.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeFunctionTool.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeFunctionTool',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'description')
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..aOM<DoubaoRealtimeJSONSchema>(3, _omitFieldNames ? '' : 'parameters',
        subBuilder: DoubaoRealtimeJSONSchema.create)
    ..aOB(4, _omitFieldNames ? '' : 'strict')
    ..aE<$1.DoubaoRealtimeFunctionToolType>(5, _omitFieldNames ? '' : 'type',
        enumValues: $1.DoubaoRealtimeFunctionToolType.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeFunctionTool clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeFunctionTool copyWith(
          void Function(DoubaoRealtimeFunctionTool) updates) =>
      super.copyWith(
              (message) => updates(message as DoubaoRealtimeFunctionTool))
          as DoubaoRealtimeFunctionTool;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeFunctionTool create() => DoubaoRealtimeFunctionTool._();
  @$core.override
  DoubaoRealtimeFunctionTool createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeFunctionTool getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeFunctionTool>(create);
  static DoubaoRealtimeFunctionTool? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get description => $_getSZ(0);
  @$pb.TagNumber(1)
  set description($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDescription() => $_has(0);
  @$pb.TagNumber(1)
  void clearDescription() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);

  @$pb.TagNumber(3)
  DoubaoRealtimeJSONSchema get parameters => $_getN(2);
  @$pb.TagNumber(3)
  set parameters(DoubaoRealtimeJSONSchema value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasParameters() => $_has(2);
  @$pb.TagNumber(3)
  void clearParameters() => $_clearField(3);
  @$pb.TagNumber(3)
  DoubaoRealtimeJSONSchema ensureParameters() => $_ensure(2);

  @$pb.TagNumber(4)
  $core.bool get strict => $_getBF(3);
  @$pb.TagNumber(4)
  set strict($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasStrict() => $_has(3);
  @$pb.TagNumber(4)
  void clearStrict() => $_clearField(4);

  @$pb.TagNumber(5)
  $1.DoubaoRealtimeFunctionToolType get type => $_getN(4);
  @$pb.TagNumber(5)
  set type($1.DoubaoRealtimeFunctionToolType value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasType() => $_has(4);
  @$pb.TagNumber(5)
  void clearType() => $_clearField(5);
}

class DoubaoRealtimeJSONSchema extends $pb.GeneratedMessage {
  factory DoubaoRealtimeJSONSchema({
    $core.bool? additionalProperties,
    $core.Iterable<DoubaoRealtimeJSONSchema>? anyOf,
    $core.String? description,
    $core.Iterable<$core.String>? enumValues,
    DoubaoRealtimeJSONSchema? items,
    $fixnum.Int64? maxLength,
    $core.double? maximum,
    $fixnum.Int64? minLength,
    $core.double? minimum,
    $core.Iterable<$core.MapEntry<$core.String, DoubaoRealtimeJSONSchema>>?
        properties,
    $core.Iterable<$core.String>? required,
    $core.String? type,
  }) {
    final result = create();
    if (additionalProperties != null)
      result.additionalProperties = additionalProperties;
    if (anyOf != null) result.anyOf.addAll(anyOf);
    if (description != null) result.description = description;
    if (enumValues != null) result.enumValues.addAll(enumValues);
    if (items != null) result.items = items;
    if (maxLength != null) result.maxLength = maxLength;
    if (maximum != null) result.maximum = maximum;
    if (minLength != null) result.minLength = minLength;
    if (minimum != null) result.minimum = minimum;
    if (properties != null) result.properties.addEntries(properties);
    if (required != null) result.required.addAll(required);
    if (type != null) result.type = type;
    return result;
  }

  DoubaoRealtimeJSONSchema._();

  factory DoubaoRealtimeJSONSchema.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeJSONSchema.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeJSONSchema',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'additionalProperties')
    ..pPM<DoubaoRealtimeJSONSchema>(2, _omitFieldNames ? '' : 'anyOf',
        subBuilder: DoubaoRealtimeJSONSchema.create)
    ..aOS(3, _omitFieldNames ? '' : 'description')
    ..pPS(4, _omitFieldNames ? '' : 'enum', protoName: 'enum_values')
    ..aOM<DoubaoRealtimeJSONSchema>(5, _omitFieldNames ? '' : 'items',
        subBuilder: DoubaoRealtimeJSONSchema.create)
    ..aInt64(6, _omitFieldNames ? '' : 'maxLength')
    ..aD(7, _omitFieldNames ? '' : 'maximum')
    ..aInt64(8, _omitFieldNames ? '' : 'minLength')
    ..aD(9, _omitFieldNames ? '' : 'minimum')
    ..m<$core.String, DoubaoRealtimeJSONSchema>(
        10, _omitFieldNames ? '' : 'properties',
        entryClassName: 'DoubaoRealtimeJSONSchema.PropertiesEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OM,
        valueCreator: DoubaoRealtimeJSONSchema.create,
        valueDefaultOrMaker: DoubaoRealtimeJSONSchema.getDefault,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..pPS(11, _omitFieldNames ? '' : 'required')
    ..aOS(12, _omitFieldNames ? '' : 'type')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeJSONSchema clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeJSONSchema copyWith(
          void Function(DoubaoRealtimeJSONSchema) updates) =>
      super.copyWith((message) => updates(message as DoubaoRealtimeJSONSchema))
          as DoubaoRealtimeJSONSchema;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeJSONSchema create() => DoubaoRealtimeJSONSchema._();
  @$core.override
  DoubaoRealtimeJSONSchema createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeJSONSchema getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeJSONSchema>(create);
  static DoubaoRealtimeJSONSchema? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get additionalProperties => $_getBF(0);
  @$pb.TagNumber(1)
  set additionalProperties($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAdditionalProperties() => $_has(0);
  @$pb.TagNumber(1)
  void clearAdditionalProperties() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<DoubaoRealtimeJSONSchema> get anyOf => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get description => $_getSZ(2);
  @$pb.TagNumber(3)
  set description($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDescription() => $_has(2);
  @$pb.TagNumber(3)
  void clearDescription() => $_clearField(3);

  @$pb.TagNumber(4)
  $pb.PbList<$core.String> get enumValues => $_getList(3);

  @$pb.TagNumber(5)
  DoubaoRealtimeJSONSchema get items => $_getN(4);
  @$pb.TagNumber(5)
  set items(DoubaoRealtimeJSONSchema value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasItems() => $_has(4);
  @$pb.TagNumber(5)
  void clearItems() => $_clearField(5);
  @$pb.TagNumber(5)
  DoubaoRealtimeJSONSchema ensureItems() => $_ensure(4);

  @$pb.TagNumber(6)
  $fixnum.Int64 get maxLength => $_getI64(5);
  @$pb.TagNumber(6)
  set maxLength($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasMaxLength() => $_has(5);
  @$pb.TagNumber(6)
  void clearMaxLength() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.double get maximum => $_getN(6);
  @$pb.TagNumber(7)
  set maximum($core.double value) => $_setDouble(6, value);
  @$pb.TagNumber(7)
  $core.bool hasMaximum() => $_has(6);
  @$pb.TagNumber(7)
  void clearMaximum() => $_clearField(7);

  @$pb.TagNumber(8)
  $fixnum.Int64 get minLength => $_getI64(7);
  @$pb.TagNumber(8)
  set minLength($fixnum.Int64 value) => $_setInt64(7, value);
  @$pb.TagNumber(8)
  $core.bool hasMinLength() => $_has(7);
  @$pb.TagNumber(8)
  void clearMinLength() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.double get minimum => $_getN(8);
  @$pb.TagNumber(9)
  set minimum($core.double value) => $_setDouble(8, value);
  @$pb.TagNumber(9)
  $core.bool hasMinimum() => $_has(8);
  @$pb.TagNumber(9)
  void clearMinimum() => $_clearField(9);

  @$pb.TagNumber(10)
  $pb.PbMap<$core.String, DoubaoRealtimeJSONSchema> get properties =>
      $_getMap(9);

  @$pb.TagNumber(11)
  $pb.PbList<$core.String> get required => $_getList(10);

  @$pb.TagNumber(12)
  $core.String get type => $_getSZ(11);
  @$pb.TagNumber(12)
  set type($core.String value) => $_setString(11, value);
  @$pb.TagNumber(12)
  $core.bool hasType() => $_has(11);
  @$pb.TagNumber(12)
  void clearType() => $_clearField(12);
}

class DoubaoRealtimeTTSExtension extends $pb.GeneratedMessage {
  factory DoubaoRealtimeTTSExtension({
    DoubaoRealtimeTTSExtra? extra,
  }) {
    final result = create();
    if (extra != null) result.extra = extra;
    return result;
  }

  DoubaoRealtimeTTSExtension._();

  factory DoubaoRealtimeTTSExtension.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeTTSExtension.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeTTSExtension',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DoubaoRealtimeTTSExtra>(1, _omitFieldNames ? '' : 'extra',
        subBuilder: DoubaoRealtimeTTSExtra.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeTTSExtension clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeTTSExtension copyWith(
          void Function(DoubaoRealtimeTTSExtension) updates) =>
      super.copyWith(
              (message) => updates(message as DoubaoRealtimeTTSExtension))
          as DoubaoRealtimeTTSExtension;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeTTSExtension create() => DoubaoRealtimeTTSExtension._();
  @$core.override
  DoubaoRealtimeTTSExtension createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeTTSExtension getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeTTSExtension>(create);
  static DoubaoRealtimeTTSExtension? _defaultInstance;

  @$pb.TagNumber(1)
  DoubaoRealtimeTTSExtra get extra => $_getN(0);
  @$pb.TagNumber(1)
  set extra(DoubaoRealtimeTTSExtra value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasExtra() => $_has(0);
  @$pb.TagNumber(1)
  void clearExtra() => $_clearField(1);
  @$pb.TagNumber(1)
  DoubaoRealtimeTTSExtra ensureExtra() => $_ensure(0);
}

class DoubaoRealtimeTTSExtra extends $pb.GeneratedMessage {
  factory DoubaoRealtimeTTSExtra({
    DoubaoRealtimeAIGCMetadata? aigcMetadata,
    $core.String? explicitDialect,
    $core.String? tts20Model,
  }) {
    final result = create();
    if (aigcMetadata != null) result.aigcMetadata = aigcMetadata;
    if (explicitDialect != null) result.explicitDialect = explicitDialect;
    if (tts20Model != null) result.tts20Model = tts20Model;
    return result;
  }

  DoubaoRealtimeTTSExtra._();

  factory DoubaoRealtimeTTSExtra.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeTTSExtra.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeTTSExtra',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DoubaoRealtimeAIGCMetadata>(1, _omitFieldNames ? '' : 'aigcMetadata',
        subBuilder: DoubaoRealtimeAIGCMetadata.create)
    ..aOS(2, _omitFieldNames ? '' : 'explicitDialect')
    ..aOS(3, _omitFieldNames ? '' : 'tts20Model', protoName: 'tts_2_0_model')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeTTSExtra clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeTTSExtra copyWith(
          void Function(DoubaoRealtimeTTSExtra) updates) =>
      super.copyWith((message) => updates(message as DoubaoRealtimeTTSExtra))
          as DoubaoRealtimeTTSExtra;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeTTSExtra create() => DoubaoRealtimeTTSExtra._();
  @$core.override
  DoubaoRealtimeTTSExtra createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeTTSExtra getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeTTSExtra>(create);
  static DoubaoRealtimeTTSExtra? _defaultInstance;

  @$pb.TagNumber(1)
  DoubaoRealtimeAIGCMetadata get aigcMetadata => $_getN(0);
  @$pb.TagNumber(1)
  set aigcMetadata(DoubaoRealtimeAIGCMetadata value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAigcMetadata() => $_has(0);
  @$pb.TagNumber(1)
  void clearAigcMetadata() => $_clearField(1);
  @$pb.TagNumber(1)
  DoubaoRealtimeAIGCMetadata ensureAigcMetadata() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get explicitDialect => $_getSZ(1);
  @$pb.TagNumber(2)
  set explicitDialect($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasExplicitDialect() => $_has(1);
  @$pb.TagNumber(2)
  void clearExplicitDialect() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get tts20Model => $_getSZ(2);
  @$pb.TagNumber(3)
  set tts20Model($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasTts20Model() => $_has(2);
  @$pb.TagNumber(3)
  void clearTts20Model() => $_clearField(3);
}

class DoubaoRealtimeWorkflowSpec extends $pb.GeneratedMessage {
  factory DoubaoRealtimeWorkflowSpec({
    DoubaoRealtimeAudio? audio,
    DoubaoRealtimeExtension? extension_2,
    $core.String? instructions,
    $core.String? model,
    $core.Iterable<DoubaoRealtimeFunctionTool>? tools,
  }) {
    final result = create();
    if (audio != null) result.audio = audio;
    if (extension_2 != null) result.extension_2 = extension_2;
    if (instructions != null) result.instructions = instructions;
    if (model != null) result.model = model;
    if (tools != null) result.tools.addAll(tools);
    return result;
  }

  DoubaoRealtimeWorkflowSpec._();

  factory DoubaoRealtimeWorkflowSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeWorkflowSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeWorkflowSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<DoubaoRealtimeAudio>(1, _omitFieldNames ? '' : 'audio',
        subBuilder: DoubaoRealtimeAudio.create)
    ..aOM<DoubaoRealtimeExtension>(2, _omitFieldNames ? '' : 'extension',
        subBuilder: DoubaoRealtimeExtension.create)
    ..aOS(3, _omitFieldNames ? '' : 'instructions')
    ..aOS(4, _omitFieldNames ? '' : 'model')
    ..pPM<DoubaoRealtimeFunctionTool>(5, _omitFieldNames ? '' : 'tools',
        subBuilder: DoubaoRealtimeFunctionTool.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeWorkflowSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeWorkflowSpec copyWith(
          void Function(DoubaoRealtimeWorkflowSpec) updates) =>
      super.copyWith(
              (message) => updates(message as DoubaoRealtimeWorkflowSpec))
          as DoubaoRealtimeWorkflowSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeWorkflowSpec create() => DoubaoRealtimeWorkflowSpec._();
  @$core.override
  DoubaoRealtimeWorkflowSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeWorkflowSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeWorkflowSpec>(create);
  static DoubaoRealtimeWorkflowSpec? _defaultInstance;

  @$pb.TagNumber(1)
  DoubaoRealtimeAudio get audio => $_getN(0);
  @$pb.TagNumber(1)
  set audio(DoubaoRealtimeAudio value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAudio() => $_has(0);
  @$pb.TagNumber(1)
  void clearAudio() => $_clearField(1);
  @$pb.TagNumber(1)
  DoubaoRealtimeAudio ensureAudio() => $_ensure(0);

  @$pb.TagNumber(2)
  DoubaoRealtimeExtension get extension_2 => $_getN(1);
  @$pb.TagNumber(2)
  set extension_2(DoubaoRealtimeExtension value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasExtension_2() => $_has(1);
  @$pb.TagNumber(2)
  void clearExtension_2() => $_clearField(2);
  @$pb.TagNumber(2)
  DoubaoRealtimeExtension ensureExtension_2() => $_ensure(1);

  @$pb.TagNumber(3)
  $core.String get instructions => $_getSZ(2);
  @$pb.TagNumber(3)
  set instructions($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasInstructions() => $_has(2);
  @$pb.TagNumber(3)
  void clearInstructions() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get model => $_getSZ(3);
  @$pb.TagNumber(4)
  set model($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasModel() => $_has(3);
  @$pb.TagNumber(4)
  void clearModel() => $_clearField(4);

  @$pb.TagNumber(5)
  $pb.PbList<DoubaoRealtimeFunctionTool> get tools => $_getList(4);
}

class DoubaoRealtimeWorkspaceParameters extends $pb.GeneratedMessage {
  factory DoubaoRealtimeWorkspaceParameters({
    $1.DoubaoRealtimeWorkspaceParametersAgentType? agentType,
    DoubaoRealtimeAudio? audio,
    $core.bool? e2e,
    DoubaoRealtimeExtension? extension_4,
    $1.WorkspaceInputMode? input,
    $core.String? instructions,
    $core.String? model,
    $core.Iterable<DoubaoRealtimeFunctionTool>? tools,
  }) {
    final result = create();
    if (agentType != null) result.agentType = agentType;
    if (audio != null) result.audio = audio;
    if (e2e != null) result.e2e = e2e;
    if (extension_4 != null) result.extension_4 = extension_4;
    if (input != null) result.input = input;
    if (instructions != null) result.instructions = instructions;
    if (model != null) result.model = model;
    if (tools != null) result.tools.addAll(tools);
    return result;
  }

  DoubaoRealtimeWorkspaceParameters._();

  factory DoubaoRealtimeWorkspaceParameters.fromBuffer(
          $core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DoubaoRealtimeWorkspaceParameters.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DoubaoRealtimeWorkspaceParameters',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<$1.DoubaoRealtimeWorkspaceParametersAgentType>(
        1, _omitFieldNames ? '' : 'agentType',
        enumValues: $1.DoubaoRealtimeWorkspaceParametersAgentType.values)
    ..aOM<DoubaoRealtimeAudio>(2, _omitFieldNames ? '' : 'audio',
        subBuilder: DoubaoRealtimeAudio.create)
    ..aOB(3, _omitFieldNames ? '' : 'e2e')
    ..aOM<DoubaoRealtimeExtension>(4, _omitFieldNames ? '' : 'extension',
        subBuilder: DoubaoRealtimeExtension.create)
    ..aE<$1.WorkspaceInputMode>(5, _omitFieldNames ? '' : 'input',
        enumValues: $1.WorkspaceInputMode.values)
    ..aOS(6, _omitFieldNames ? '' : 'instructions')
    ..aOS(7, _omitFieldNames ? '' : 'model')
    ..pPM<DoubaoRealtimeFunctionTool>(8, _omitFieldNames ? '' : 'tools',
        subBuilder: DoubaoRealtimeFunctionTool.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeWorkspaceParameters clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DoubaoRealtimeWorkspaceParameters copyWith(
          void Function(DoubaoRealtimeWorkspaceParameters) updates) =>
      super.copyWith((message) =>
              updates(message as DoubaoRealtimeWorkspaceParameters))
          as DoubaoRealtimeWorkspaceParameters;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeWorkspaceParameters create() =>
      DoubaoRealtimeWorkspaceParameters._();
  @$core.override
  DoubaoRealtimeWorkspaceParameters createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DoubaoRealtimeWorkspaceParameters getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DoubaoRealtimeWorkspaceParameters>(
          create);
  static DoubaoRealtimeWorkspaceParameters? _defaultInstance;

  @$pb.TagNumber(1)
  $1.DoubaoRealtimeWorkspaceParametersAgentType get agentType => $_getN(0);
  @$pb.TagNumber(1)
  set agentType($1.DoubaoRealtimeWorkspaceParametersAgentType value) =>
      $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAgentType() => $_has(0);
  @$pb.TagNumber(1)
  void clearAgentType() => $_clearField(1);

  @$pb.TagNumber(2)
  DoubaoRealtimeAudio get audio => $_getN(1);
  @$pb.TagNumber(2)
  set audio(DoubaoRealtimeAudio value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasAudio() => $_has(1);
  @$pb.TagNumber(2)
  void clearAudio() => $_clearField(2);
  @$pb.TagNumber(2)
  DoubaoRealtimeAudio ensureAudio() => $_ensure(1);

  @$pb.TagNumber(3)
  $core.bool get e2e => $_getBF(2);
  @$pb.TagNumber(3)
  set e2e($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasE2e() => $_has(2);
  @$pb.TagNumber(3)
  void clearE2e() => $_clearField(3);

  @$pb.TagNumber(4)
  DoubaoRealtimeExtension get extension_4 => $_getN(3);
  @$pb.TagNumber(4)
  set extension_4(DoubaoRealtimeExtension value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasExtension_4() => $_has(3);
  @$pb.TagNumber(4)
  void clearExtension_4() => $_clearField(4);
  @$pb.TagNumber(4)
  DoubaoRealtimeExtension ensureExtension_4() => $_ensure(3);

  @$pb.TagNumber(5)
  $1.WorkspaceInputMode get input => $_getN(4);
  @$pb.TagNumber(5)
  set input($1.WorkspaceInputMode value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasInput() => $_has(4);
  @$pb.TagNumber(5)
  void clearInput() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get instructions => $_getSZ(5);
  @$pb.TagNumber(6)
  set instructions($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasInstructions() => $_has(5);
  @$pb.TagNumber(6)
  void clearInstructions() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get model => $_getSZ(6);
  @$pb.TagNumber(7)
  set model($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasModel() => $_has(6);
  @$pb.TagNumber(7)
  void clearModel() => $_clearField(7);

  @$pb.TagNumber(8)
  $pb.PbList<DoubaoRealtimeFunctionTool> get tools => $_getList(7);
}

class FlowcraftConversationParameters extends $pb.GeneratedMessage {
  factory FlowcraftConversationParameters({
    $1.FlowcraftConversationParametersAgentInitiativePolicy?
        agentInitiativePolicy,
    $1.FlowcraftConversationParametersInitiative? initiative,
  }) {
    final result = create();
    if (agentInitiativePolicy != null)
      result.agentInitiativePolicy = agentInitiativePolicy;
    if (initiative != null) result.initiative = initiative;
    return result;
  }

  FlowcraftConversationParameters._();

  factory FlowcraftConversationParameters.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FlowcraftConversationParameters.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FlowcraftConversationParameters',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<$1.FlowcraftConversationParametersAgentInitiativePolicy>(
        1, _omitFieldNames ? '' : 'agentInitiativePolicy',
        enumValues:
            $1.FlowcraftConversationParametersAgentInitiativePolicy.values)
    ..aE<$1.FlowcraftConversationParametersInitiative>(
        2, _omitFieldNames ? '' : 'initiative',
        enumValues: $1.FlowcraftConversationParametersInitiative.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FlowcraftConversationParameters clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FlowcraftConversationParameters copyWith(
          void Function(FlowcraftConversationParameters) updates) =>
      super.copyWith(
              (message) => updates(message as FlowcraftConversationParameters))
          as FlowcraftConversationParameters;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FlowcraftConversationParameters create() =>
      FlowcraftConversationParameters._();
  @$core.override
  FlowcraftConversationParameters createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FlowcraftConversationParameters getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FlowcraftConversationParameters>(
          create);
  static FlowcraftConversationParameters? _defaultInstance;

  @$pb.TagNumber(1)
  $1.FlowcraftConversationParametersAgentInitiativePolicy
      get agentInitiativePolicy => $_getN(0);
  @$pb.TagNumber(1)
  set agentInitiativePolicy(
          $1.FlowcraftConversationParametersAgentInitiativePolicy value) =>
      $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAgentInitiativePolicy() => $_has(0);
  @$pb.TagNumber(1)
  void clearAgentInitiativePolicy() => $_clearField(1);

  @$pb.TagNumber(2)
  $1.FlowcraftConversationParametersInitiative get initiative => $_getN(1);
  @$pb.TagNumber(2)
  set initiative($1.FlowcraftConversationParametersInitiative value) =>
      $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasInitiative() => $_has(1);
  @$pb.TagNumber(2)
  void clearInitiative() => $_clearField(2);
}

class FlowcraftWorkflowSpec extends $pb.GeneratedMessage {
  factory FlowcraftWorkflowSpec({
    $0.Struct? fields,
  }) {
    final result = create();
    if (fields != null) result.fields = fields;
    return result;
  }

  FlowcraftWorkflowSpec._();

  factory FlowcraftWorkflowSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FlowcraftWorkflowSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FlowcraftWorkflowSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<$0.Struct>(1, _omitFieldNames ? '' : 'fields',
        subBuilder: $0.Struct.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FlowcraftWorkflowSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FlowcraftWorkflowSpec copyWith(
          void Function(FlowcraftWorkflowSpec) updates) =>
      super.copyWith((message) => updates(message as FlowcraftWorkflowSpec))
          as FlowcraftWorkflowSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FlowcraftWorkflowSpec create() => FlowcraftWorkflowSpec._();
  @$core.override
  FlowcraftWorkflowSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FlowcraftWorkflowSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FlowcraftWorkflowSpec>(create);
  static FlowcraftWorkflowSpec? _defaultInstance;

  @$pb.TagNumber(1)
  $0.Struct get fields => $_getN(0);
  @$pb.TagNumber(1)
  set fields($0.Struct value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasFields() => $_has(0);
  @$pb.TagNumber(1)
  void clearFields() => $_clearField(1);
  @$pb.TagNumber(1)
  $0.Struct ensureFields() => $_ensure(0);
}

class FlowcraftWorkspaceParameters extends $pb.GeneratedMessage {
  factory FlowcraftWorkspaceParameters({
    $1.FlowcraftWorkspaceParametersAgentType? agentType,
    FlowcraftConversationParameters? conversation,
    $core.bool? e2e,
    $core.String? embeddingModel,
    $core.String? extractModel,
    $core.String? generateModel,
    $1.WorkspaceInputMode? input,
  }) {
    final result = create();
    if (agentType != null) result.agentType = agentType;
    if (conversation != null) result.conversation = conversation;
    if (e2e != null) result.e2e = e2e;
    if (embeddingModel != null) result.embeddingModel = embeddingModel;
    if (extractModel != null) result.extractModel = extractModel;
    if (generateModel != null) result.generateModel = generateModel;
    if (input != null) result.input = input;
    return result;
  }

  FlowcraftWorkspaceParameters._();

  factory FlowcraftWorkspaceParameters.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory FlowcraftWorkspaceParameters.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'FlowcraftWorkspaceParameters',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<$1.FlowcraftWorkspaceParametersAgentType>(
        1, _omitFieldNames ? '' : 'agentType',
        enumValues: $1.FlowcraftWorkspaceParametersAgentType.values)
    ..aOM<FlowcraftConversationParameters>(
        2, _omitFieldNames ? '' : 'conversation',
        subBuilder: FlowcraftConversationParameters.create)
    ..aOB(3, _omitFieldNames ? '' : 'e2e')
    ..aOS(4, _omitFieldNames ? '' : 'embeddingModel')
    ..aOS(5, _omitFieldNames ? '' : 'extractModel')
    ..aOS(6, _omitFieldNames ? '' : 'generateModel')
    ..aE<$1.WorkspaceInputMode>(7, _omitFieldNames ? '' : 'input',
        enumValues: $1.WorkspaceInputMode.values)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FlowcraftWorkspaceParameters clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  FlowcraftWorkspaceParameters copyWith(
          void Function(FlowcraftWorkspaceParameters) updates) =>
      super.copyWith(
              (message) => updates(message as FlowcraftWorkspaceParameters))
          as FlowcraftWorkspaceParameters;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static FlowcraftWorkspaceParameters create() =>
      FlowcraftWorkspaceParameters._();
  @$core.override
  FlowcraftWorkspaceParameters createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static FlowcraftWorkspaceParameters getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<FlowcraftWorkspaceParameters>(create);
  static FlowcraftWorkspaceParameters? _defaultInstance;

  @$pb.TagNumber(1)
  $1.FlowcraftWorkspaceParametersAgentType get agentType => $_getN(0);
  @$pb.TagNumber(1)
  set agentType($1.FlowcraftWorkspaceParametersAgentType value) =>
      $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAgentType() => $_has(0);
  @$pb.TagNumber(1)
  void clearAgentType() => $_clearField(1);

  @$pb.TagNumber(2)
  FlowcraftConversationParameters get conversation => $_getN(1);
  @$pb.TagNumber(2)
  set conversation(FlowcraftConversationParameters value) =>
      $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasConversation() => $_has(1);
  @$pb.TagNumber(2)
  void clearConversation() => $_clearField(2);
  @$pb.TagNumber(2)
  FlowcraftConversationParameters ensureConversation() => $_ensure(1);

  @$pb.TagNumber(3)
  $core.bool get e2e => $_getBF(2);
  @$pb.TagNumber(3)
  set e2e($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasE2e() => $_has(2);
  @$pb.TagNumber(3)
  void clearE2e() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get embeddingModel => $_getSZ(3);
  @$pb.TagNumber(4)
  set embeddingModel($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasEmbeddingModel() => $_has(3);
  @$pb.TagNumber(4)
  void clearEmbeddingModel() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get extractModel => $_getSZ(4);
  @$pb.TagNumber(5)
  set extractModel($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasExtractModel() => $_has(4);
  @$pb.TagNumber(5)
  void clearExtractModel() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get generateModel => $_getSZ(5);
  @$pb.TagNumber(6)
  set generateModel($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasGenerateModel() => $_has(5);
  @$pb.TagNumber(6)
  void clearGenerateModel() => $_clearField(6);

  @$pb.TagNumber(7)
  $1.WorkspaceInputMode get input => $_getN(6);
  @$pb.TagNumber(7)
  set input($1.WorkspaceInputMode value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasInput() => $_has(6);
  @$pb.TagNumber(7)
  void clearInput() => $_clearField(7);
}

class GeminiCredentialBody extends $pb.GeneratedMessage {
  factory GeminiCredentialBody({
    $core.String? apiKey,
    $core.String? baseUrl,
    $core.String? token,
  }) {
    final result = create();
    if (apiKey != null) result.apiKey = apiKey;
    if (baseUrl != null) result.baseUrl = baseUrl;
    if (token != null) result.token = token;
    return result;
  }

  GeminiCredentialBody._();

  factory GeminiCredentialBody.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GeminiCredentialBody.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GeminiCredentialBody',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'apiKey')
    ..aOS(2, _omitFieldNames ? '' : 'baseUrl')
    ..aOS(3, _omitFieldNames ? '' : 'token')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GeminiCredentialBody clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GeminiCredentialBody copyWith(void Function(GeminiCredentialBody) updates) =>
      super.copyWith((message) => updates(message as GeminiCredentialBody))
          as GeminiCredentialBody;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GeminiCredentialBody create() => GeminiCredentialBody._();
  @$core.override
  GeminiCredentialBody createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GeminiCredentialBody getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GeminiCredentialBody>(create);
  static GeminiCredentialBody? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get apiKey => $_getSZ(0);
  @$pb.TagNumber(1)
  set apiKey($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasApiKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearApiKey() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get baseUrl => $_getSZ(1);
  @$pb.TagNumber(2)
  set baseUrl($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBaseUrl() => $_has(1);
  @$pb.TagNumber(2)
  void clearBaseUrl() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get token => $_getSZ(2);
  @$pb.TagNumber(3)
  set token($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasToken() => $_has(2);
  @$pb.TagNumber(3)
  void clearToken() => $_clearField(3);
}

class GeminiTenantModelProviderData extends $pb.GeneratedMessage {
  factory GeminiTenantModelProviderData({
    $core.String? upstreamModel,
  }) {
    final result = create();
    if (upstreamModel != null) result.upstreamModel = upstreamModel;
    return result;
  }

  GeminiTenantModelProviderData._();

  factory GeminiTenantModelProviderData.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GeminiTenantModelProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GeminiTenantModelProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'upstreamModel')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GeminiTenantModelProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GeminiTenantModelProviderData copyWith(
          void Function(GeminiTenantModelProviderData) updates) =>
      super.copyWith(
              (message) => updates(message as GeminiTenantModelProviderData))
          as GeminiTenantModelProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GeminiTenantModelProviderData create() =>
      GeminiTenantModelProviderData._();
  @$core.override
  GeminiTenantModelProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GeminiTenantModelProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GeminiTenantModelProviderData>(create);
  static GeminiTenantModelProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get upstreamModel => $_getSZ(0);
  @$pb.TagNumber(1)
  set upstreamModel($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasUpstreamModel() => $_has(0);
  @$pb.TagNumber(1)
  void clearUpstreamModel() => $_clearField(1);
}

class GeminiTenantVoiceProviderData extends $pb.GeneratedMessage {
  factory GeminiTenantVoiceProviderData({
    $0.Struct? raw,
    $core.String? voiceId,
  }) {
    final result = create();
    if (raw != null) result.raw = raw;
    if (voiceId != null) result.voiceId = voiceId;
    return result;
  }

  GeminiTenantVoiceProviderData._();

  factory GeminiTenantVoiceProviderData.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory GeminiTenantVoiceProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'GeminiTenantVoiceProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<$0.Struct>(1, _omitFieldNames ? '' : 'raw',
        subBuilder: $0.Struct.create)
    ..aOS(2, _omitFieldNames ? '' : 'voiceId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GeminiTenantVoiceProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  GeminiTenantVoiceProviderData copyWith(
          void Function(GeminiTenantVoiceProviderData) updates) =>
      super.copyWith(
              (message) => updates(message as GeminiTenantVoiceProviderData))
          as GeminiTenantVoiceProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static GeminiTenantVoiceProviderData create() =>
      GeminiTenantVoiceProviderData._();
  @$core.override
  GeminiTenantVoiceProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static GeminiTenantVoiceProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<GeminiTenantVoiceProviderData>(create);
  static GeminiTenantVoiceProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  $0.Struct get raw => $_getN(0);
  @$pb.TagNumber(1)
  set raw($0.Struct value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasRaw() => $_has(0);
  @$pb.TagNumber(1)
  void clearRaw() => $_clearField(1);
  @$pb.TagNumber(1)
  $0.Struct ensureRaw() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get voiceId => $_getSZ(1);
  @$pb.TagNumber(2)
  set voiceId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasVoiceId() => $_has(1);
  @$pb.TagNumber(2)
  void clearVoiceId() => $_clearField(2);
}

class MiniMaxCredentialBody extends $pb.GeneratedMessage {
  factory MiniMaxCredentialBody({
    $core.String? apiKey,
    $core.String? baseUrl,
    $core.String? minimaxVoiceBaseUrl,
    $core.String? token,
    $core.String? voiceBaseUrl,
  }) {
    final result = create();
    if (apiKey != null) result.apiKey = apiKey;
    if (baseUrl != null) result.baseUrl = baseUrl;
    if (minimaxVoiceBaseUrl != null)
      result.minimaxVoiceBaseUrl = minimaxVoiceBaseUrl;
    if (token != null) result.token = token;
    if (voiceBaseUrl != null) result.voiceBaseUrl = voiceBaseUrl;
    return result;
  }

  MiniMaxCredentialBody._();

  factory MiniMaxCredentialBody.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory MiniMaxCredentialBody.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'MiniMaxCredentialBody',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'apiKey')
    ..aOS(2, _omitFieldNames ? '' : 'baseUrl')
    ..aOS(3, _omitFieldNames ? '' : 'minimaxVoiceBaseUrl')
    ..aOS(4, _omitFieldNames ? '' : 'token')
    ..aOS(5, _omitFieldNames ? '' : 'voiceBaseUrl')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MiniMaxCredentialBody clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MiniMaxCredentialBody copyWith(
          void Function(MiniMaxCredentialBody) updates) =>
      super.copyWith((message) => updates(message as MiniMaxCredentialBody))
          as MiniMaxCredentialBody;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static MiniMaxCredentialBody create() => MiniMaxCredentialBody._();
  @$core.override
  MiniMaxCredentialBody createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static MiniMaxCredentialBody getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<MiniMaxCredentialBody>(create);
  static MiniMaxCredentialBody? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get apiKey => $_getSZ(0);
  @$pb.TagNumber(1)
  set apiKey($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasApiKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearApiKey() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get baseUrl => $_getSZ(1);
  @$pb.TagNumber(2)
  set baseUrl($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBaseUrl() => $_has(1);
  @$pb.TagNumber(2)
  void clearBaseUrl() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get minimaxVoiceBaseUrl => $_getSZ(2);
  @$pb.TagNumber(3)
  set minimaxVoiceBaseUrl($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMinimaxVoiceBaseUrl() => $_has(2);
  @$pb.TagNumber(3)
  void clearMinimaxVoiceBaseUrl() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get token => $_getSZ(3);
  @$pb.TagNumber(4)
  set token($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasToken() => $_has(3);
  @$pb.TagNumber(4)
  void clearToken() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get voiceBaseUrl => $_getSZ(4);
  @$pb.TagNumber(5)
  set voiceBaseUrl($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasVoiceBaseUrl() => $_has(4);
  @$pb.TagNumber(5)
  void clearVoiceBaseUrl() => $_clearField(5);
}

class MiniMaxTenantVoiceProviderData extends $pb.GeneratedMessage {
  factory MiniMaxTenantVoiceProviderData({
    $core.String? format,
    $core.String? model,
    $0.Struct? raw,
    $fixnum.Int64? sampleRate,
    $core.String? voiceId,
    $core.String? voiceType,
  }) {
    final result = create();
    if (format != null) result.format = format;
    if (model != null) result.model = model;
    if (raw != null) result.raw = raw;
    if (sampleRate != null) result.sampleRate = sampleRate;
    if (voiceId != null) result.voiceId = voiceId;
    if (voiceType != null) result.voiceType = voiceType;
    return result;
  }

  MiniMaxTenantVoiceProviderData._();

  factory MiniMaxTenantVoiceProviderData.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory MiniMaxTenantVoiceProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'MiniMaxTenantVoiceProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'format')
    ..aOS(2, _omitFieldNames ? '' : 'model')
    ..aOM<$0.Struct>(3, _omitFieldNames ? '' : 'raw',
        subBuilder: $0.Struct.create)
    ..aInt64(4, _omitFieldNames ? '' : 'sampleRate')
    ..aOS(5, _omitFieldNames ? '' : 'voiceId')
    ..aOS(6, _omitFieldNames ? '' : 'voiceType')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MiniMaxTenantVoiceProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MiniMaxTenantVoiceProviderData copyWith(
          void Function(MiniMaxTenantVoiceProviderData) updates) =>
      super.copyWith(
              (message) => updates(message as MiniMaxTenantVoiceProviderData))
          as MiniMaxTenantVoiceProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static MiniMaxTenantVoiceProviderData create() =>
      MiniMaxTenantVoiceProviderData._();
  @$core.override
  MiniMaxTenantVoiceProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static MiniMaxTenantVoiceProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<MiniMaxTenantVoiceProviderData>(create);
  static MiniMaxTenantVoiceProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get format => $_getSZ(0);
  @$pb.TagNumber(1)
  set format($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasFormat() => $_has(0);
  @$pb.TagNumber(1)
  void clearFormat() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get model => $_getSZ(1);
  @$pb.TagNumber(2)
  set model($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasModel() => $_has(1);
  @$pb.TagNumber(2)
  void clearModel() => $_clearField(2);

  @$pb.TagNumber(3)
  $0.Struct get raw => $_getN(2);
  @$pb.TagNumber(3)
  set raw($0.Struct value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasRaw() => $_has(2);
  @$pb.TagNumber(3)
  void clearRaw() => $_clearField(3);
  @$pb.TagNumber(3)
  $0.Struct ensureRaw() => $_ensure(2);

  @$pb.TagNumber(4)
  $fixnum.Int64 get sampleRate => $_getI64(3);
  @$pb.TagNumber(4)
  set sampleRate($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSampleRate() => $_has(3);
  @$pb.TagNumber(4)
  void clearSampleRate() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get voiceId => $_getSZ(4);
  @$pb.TagNumber(5)
  set voiceId($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasVoiceId() => $_has(4);
  @$pb.TagNumber(5)
  void clearVoiceId() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get voiceType => $_getSZ(5);
  @$pb.TagNumber(6)
  set voiceType($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasVoiceType() => $_has(5);
  @$pb.TagNumber(6)
  void clearVoiceType() => $_clearField(6);
}

class Model extends $pb.GeneratedMessage {
  factory Model({
    ModelCapabilities? capabilities,
    $core.String? createdAt,
    $core.String? description,
    $core.String? id,
    $1.ModelKind? kind,
    $core.String? name,
    ModelProvider? provider,
    ModelProviderData? providerData,
    $1.ModelSource? source,
    $core.String? syncedAt,
    $core.String? updatedAt,
  }) {
    final result = create();
    if (capabilities != null) result.capabilities = capabilities;
    if (createdAt != null) result.createdAt = createdAt;
    if (description != null) result.description = description;
    if (id != null) result.id = id;
    if (kind != null) result.kind = kind;
    if (name != null) result.name = name;
    if (provider != null) result.provider = provider;
    if (providerData != null) result.providerData = providerData;
    if (source != null) result.source = source;
    if (syncedAt != null) result.syncedAt = syncedAt;
    if (updatedAt != null) result.updatedAt = updatedAt;
    return result;
  }

  Model._();

  factory Model.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Model.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Model',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<ModelCapabilities>(1, _omitFieldNames ? '' : 'capabilities',
        subBuilder: ModelCapabilities.create)
    ..aOS(2, _omitFieldNames ? '' : 'createdAt')
    ..aOS(3, _omitFieldNames ? '' : 'description')
    ..aOS(4, _omitFieldNames ? '' : 'id')
    ..aE<$1.ModelKind>(5, _omitFieldNames ? '' : 'kind',
        enumValues: $1.ModelKind.values)
    ..aOS(6, _omitFieldNames ? '' : 'name')
    ..aOM<ModelProvider>(7, _omitFieldNames ? '' : 'provider',
        subBuilder: ModelProvider.create)
    ..aOM<ModelProviderData>(8, _omitFieldNames ? '' : 'providerData',
        subBuilder: ModelProviderData.create)
    ..aE<$1.ModelSource>(9, _omitFieldNames ? '' : 'source',
        enumValues: $1.ModelSource.values)
    ..aOS(10, _omitFieldNames ? '' : 'syncedAt')
    ..aOS(11, _omitFieldNames ? '' : 'updatedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Model clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Model copyWith(void Function(Model) updates) =>
      super.copyWith((message) => updates(message as Model)) as Model;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Model create() => Model._();
  @$core.override
  Model createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Model getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Model>(create);
  static Model? _defaultInstance;

  @$pb.TagNumber(1)
  ModelCapabilities get capabilities => $_getN(0);
  @$pb.TagNumber(1)
  set capabilities(ModelCapabilities value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasCapabilities() => $_has(0);
  @$pb.TagNumber(1)
  void clearCapabilities() => $_clearField(1);
  @$pb.TagNumber(1)
  ModelCapabilities ensureCapabilities() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get createdAt => $_getSZ(1);
  @$pb.TagNumber(2)
  set createdAt($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCreatedAt() => $_has(1);
  @$pb.TagNumber(2)
  void clearCreatedAt() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get description => $_getSZ(2);
  @$pb.TagNumber(3)
  set description($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDescription() => $_has(2);
  @$pb.TagNumber(3)
  void clearDescription() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get id => $_getSZ(3);
  @$pb.TagNumber(4)
  set id($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasId() => $_has(3);
  @$pb.TagNumber(4)
  void clearId() => $_clearField(4);

  @$pb.TagNumber(5)
  $1.ModelKind get kind => $_getN(4);
  @$pb.TagNumber(5)
  set kind($1.ModelKind value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasKind() => $_has(4);
  @$pb.TagNumber(5)
  void clearKind() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get name => $_getSZ(5);
  @$pb.TagNumber(6)
  set name($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasName() => $_has(5);
  @$pb.TagNumber(6)
  void clearName() => $_clearField(6);

  @$pb.TagNumber(7)
  ModelProvider get provider => $_getN(6);
  @$pb.TagNumber(7)
  set provider(ModelProvider value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasProvider() => $_has(6);
  @$pb.TagNumber(7)
  void clearProvider() => $_clearField(7);
  @$pb.TagNumber(7)
  ModelProvider ensureProvider() => $_ensure(6);

  @$pb.TagNumber(8)
  ModelProviderData get providerData => $_getN(7);
  @$pb.TagNumber(8)
  set providerData(ModelProviderData value) => $_setField(8, value);
  @$pb.TagNumber(8)
  $core.bool hasProviderData() => $_has(7);
  @$pb.TagNumber(8)
  void clearProviderData() => $_clearField(8);
  @$pb.TagNumber(8)
  ModelProviderData ensureProviderData() => $_ensure(7);

  @$pb.TagNumber(9)
  $1.ModelSource get source => $_getN(8);
  @$pb.TagNumber(9)
  set source($1.ModelSource value) => $_setField(9, value);
  @$pb.TagNumber(9)
  $core.bool hasSource() => $_has(8);
  @$pb.TagNumber(9)
  void clearSource() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get syncedAt => $_getSZ(9);
  @$pb.TagNumber(10)
  set syncedAt($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasSyncedAt() => $_has(9);
  @$pb.TagNumber(10)
  void clearSyncedAt() => $_clearField(10);

  @$pb.TagNumber(11)
  $core.String get updatedAt => $_getSZ(10);
  @$pb.TagNumber(11)
  set updatedAt($core.String value) => $_setString(10, value);
  @$pb.TagNumber(11)
  $core.bool hasUpdatedAt() => $_has(10);
  @$pb.TagNumber(11)
  void clearUpdatedAt() => $_clearField(11);
}

class ModelCapabilities extends $pb.GeneratedMessage {
  factory ModelCapabilities({
    $core.bool? jsonOutput,
    $core.bool? systemRole,
    $core.bool? temperature,
    $core.bool? textOnly,
    ModelThinkingCapability? thinking,
    $core.bool? toolCalls,
  }) {
    final result = create();
    if (jsonOutput != null) result.jsonOutput = jsonOutput;
    if (systemRole != null) result.systemRole = systemRole;
    if (temperature != null) result.temperature = temperature;
    if (textOnly != null) result.textOnly = textOnly;
    if (thinking != null) result.thinking = thinking;
    if (toolCalls != null) result.toolCalls = toolCalls;
    return result;
  }

  ModelCapabilities._();

  factory ModelCapabilities.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelCapabilities.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelCapabilities',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'jsonOutput')
    ..aOB(2, _omitFieldNames ? '' : 'systemRole')
    ..aOB(3, _omitFieldNames ? '' : 'temperature')
    ..aOB(4, _omitFieldNames ? '' : 'textOnly')
    ..aOM<ModelThinkingCapability>(5, _omitFieldNames ? '' : 'thinking',
        subBuilder: ModelThinkingCapability.create)
    ..aOB(6, _omitFieldNames ? '' : 'toolCalls')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelCapabilities clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelCapabilities copyWith(void Function(ModelCapabilities) updates) =>
      super.copyWith((message) => updates(message as ModelCapabilities))
          as ModelCapabilities;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelCapabilities create() => ModelCapabilities._();
  @$core.override
  ModelCapabilities createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelCapabilities getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelCapabilities>(create);
  static ModelCapabilities? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get jsonOutput => $_getBF(0);
  @$pb.TagNumber(1)
  set jsonOutput($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasJsonOutput() => $_has(0);
  @$pb.TagNumber(1)
  void clearJsonOutput() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get systemRole => $_getBF(1);
  @$pb.TagNumber(2)
  set systemRole($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSystemRole() => $_has(1);
  @$pb.TagNumber(2)
  void clearSystemRole() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get temperature => $_getBF(2);
  @$pb.TagNumber(3)
  set temperature($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasTemperature() => $_has(2);
  @$pb.TagNumber(3)
  void clearTemperature() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get textOnly => $_getBF(3);
  @$pb.TagNumber(4)
  set textOnly($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasTextOnly() => $_has(3);
  @$pb.TagNumber(4)
  void clearTextOnly() => $_clearField(4);

  @$pb.TagNumber(5)
  ModelThinkingCapability get thinking => $_getN(4);
  @$pb.TagNumber(5)
  set thinking(ModelThinkingCapability value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasThinking() => $_has(4);
  @$pb.TagNumber(5)
  void clearThinking() => $_clearField(5);
  @$pb.TagNumber(5)
  ModelThinkingCapability ensureThinking() => $_ensure(4);

  @$pb.TagNumber(6)
  $core.bool get toolCalls => $_getBF(5);
  @$pb.TagNumber(6)
  set toolCalls($core.bool value) => $_setBool(5, value);
  @$pb.TagNumber(6)
  $core.bool hasToolCalls() => $_has(5);
  @$pb.TagNumber(6)
  void clearToolCalls() => $_clearField(6);
}

class ModelCreateRequest extends $pb.GeneratedMessage {
  factory ModelCreateRequest({
    Model? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ModelCreateRequest._();

  factory ModelCreateRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelCreateRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelCreateRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Model>(1, _omitFieldNames ? '' : 'value', subBuilder: Model.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelCreateRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelCreateRequest copyWith(void Function(ModelCreateRequest) updates) =>
      super.copyWith((message) => updates(message as ModelCreateRequest))
          as ModelCreateRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelCreateRequest create() => ModelCreateRequest._();
  @$core.override
  ModelCreateRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelCreateRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelCreateRequest>(create);
  static ModelCreateRequest? _defaultInstance;

  @$pb.TagNumber(1)
  Model get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Model value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Model ensureValue() => $_ensure(0);
}

class ModelCreateResponse extends $pb.GeneratedMessage {
  factory ModelCreateResponse({
    Model? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ModelCreateResponse._();

  factory ModelCreateResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelCreateResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelCreateResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Model>(1, _omitFieldNames ? '' : 'value', subBuilder: Model.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelCreateResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelCreateResponse copyWith(void Function(ModelCreateResponse) updates) =>
      super.copyWith((message) => updates(message as ModelCreateResponse))
          as ModelCreateResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelCreateResponse create() => ModelCreateResponse._();
  @$core.override
  ModelCreateResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelCreateResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelCreateResponse>(create);
  static ModelCreateResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Model get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Model value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Model ensureValue() => $_ensure(0);
}

class ModelDeleteRequest extends $pb.GeneratedMessage {
  factory ModelDeleteRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  ModelDeleteRequest._();

  factory ModelDeleteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelDeleteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelDeleteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelDeleteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelDeleteRequest copyWith(void Function(ModelDeleteRequest) updates) =>
      super.copyWith((message) => updates(message as ModelDeleteRequest))
          as ModelDeleteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelDeleteRequest create() => ModelDeleteRequest._();
  @$core.override
  ModelDeleteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelDeleteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelDeleteRequest>(create);
  static ModelDeleteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class ModelDeleteResponse extends $pb.GeneratedMessage {
  factory ModelDeleteResponse({
    Model? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ModelDeleteResponse._();

  factory ModelDeleteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelDeleteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelDeleteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Model>(1, _omitFieldNames ? '' : 'value', subBuilder: Model.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelDeleteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelDeleteResponse copyWith(void Function(ModelDeleteResponse) updates) =>
      super.copyWith((message) => updates(message as ModelDeleteResponse))
          as ModelDeleteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelDeleteResponse create() => ModelDeleteResponse._();
  @$core.override
  ModelDeleteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelDeleteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelDeleteResponse>(create);
  static ModelDeleteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Model get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Model value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Model ensureValue() => $_ensure(0);
}

class ModelGetRequest extends $pb.GeneratedMessage {
  factory ModelGetRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  ModelGetRequest._();

  factory ModelGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelGetRequest copyWith(void Function(ModelGetRequest) updates) =>
      super.copyWith((message) => updates(message as ModelGetRequest))
          as ModelGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelGetRequest create() => ModelGetRequest._();
  @$core.override
  ModelGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelGetRequest>(create);
  static ModelGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class ModelGetResponse extends $pb.GeneratedMessage {
  factory ModelGetResponse({
    Model? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ModelGetResponse._();

  factory ModelGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Model>(1, _omitFieldNames ? '' : 'value', subBuilder: Model.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelGetResponse copyWith(void Function(ModelGetResponse) updates) =>
      super.copyWith((message) => updates(message as ModelGetResponse))
          as ModelGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelGetResponse create() => ModelGetResponse._();
  @$core.override
  ModelGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelGetResponse>(create);
  static ModelGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Model get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Model value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Model ensureValue() => $_ensure(0);
}

class ModelListRequest extends $pb.GeneratedMessage {
  factory ModelListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    return result;
  }

  ModelListRequest._();

  factory ModelListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelListRequest copyWith(void Function(ModelListRequest) updates) =>
      super.copyWith((message) => updates(message as ModelListRequest))
          as ModelListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelListRequest create() => ModelListRequest._();
  @$core.override
  ModelListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelListRequest>(create);
  static ModelListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get limit => $_getI64(1);
  @$pb.TagNumber(2)
  set limit($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLimit() => $_has(1);
  @$pb.TagNumber(2)
  void clearLimit() => $_clearField(2);
}

class ModelListResponse extends $pb.GeneratedMessage {
  factory ModelListResponse({
    $core.bool? hasNext,
    $core.Iterable<Model>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  ModelListResponse._();

  factory ModelListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<Model>(2, _omitFieldNames ? '' : 'items', subBuilder: Model.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelListResponse copyWith(void Function(ModelListResponse) updates) =>
      super.copyWith((message) => updates(message as ModelListResponse))
          as ModelListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelListResponse create() => ModelListResponse._();
  @$core.override
  ModelListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelListResponse>(create);
  static ModelListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<Model> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class ModelProvider extends $pb.GeneratedMessage {
  factory ModelProvider({
    $1.ModelProviderKind? kind,
    $core.String? name,
  }) {
    final result = create();
    if (kind != null) result.kind = kind;
    if (name != null) result.name = name;
    return result;
  }

  ModelProvider._();

  factory ModelProvider.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelProvider.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelProvider',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<$1.ModelProviderKind>(1, _omitFieldNames ? '' : 'kind',
        enumValues: $1.ModelProviderKind.values)
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelProvider clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelProvider copyWith(void Function(ModelProvider) updates) =>
      super.copyWith((message) => updates(message as ModelProvider))
          as ModelProvider;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelProvider create() => ModelProvider._();
  @$core.override
  ModelProvider createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelProvider getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelProvider>(create);
  static ModelProvider? _defaultInstance;

  @$pb.TagNumber(1)
  $1.ModelProviderKind get kind => $_getN(0);
  @$pb.TagNumber(1)
  set kind($1.ModelProviderKind value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasKind() => $_has(0);
  @$pb.TagNumber(1)
  void clearKind() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);
}

enum ModelProviderData_Value {
  geminiTenantModelProviderData,
  dashScopeTenantModelProviderData,
  openAitenantModelProviderData,
  volcTenantModelProviderData,
  notSet
}

class ModelProviderData extends $pb.GeneratedMessage {
  factory ModelProviderData({
    GeminiTenantModelProviderData? geminiTenantModelProviderData,
    DashScopeTenantModelProviderData? dashScopeTenantModelProviderData,
    OpenAITenantModelProviderData? openAitenantModelProviderData,
    VolcTenantModelProviderData? volcTenantModelProviderData,
  }) {
    final result = create();
    if (geminiTenantModelProviderData != null)
      result.geminiTenantModelProviderData = geminiTenantModelProviderData;
    if (dashScopeTenantModelProviderData != null)
      result.dashScopeTenantModelProviderData =
          dashScopeTenantModelProviderData;
    if (openAitenantModelProviderData != null)
      result.openAitenantModelProviderData = openAitenantModelProviderData;
    if (volcTenantModelProviderData != null)
      result.volcTenantModelProviderData = volcTenantModelProviderData;
    return result;
  }

  ModelProviderData._();

  factory ModelProviderData.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, ModelProviderData_Value>
      _ModelProviderData_ValueByTag = {
    1: ModelProviderData_Value.geminiTenantModelProviderData,
    2: ModelProviderData_Value.dashScopeTenantModelProviderData,
    3: ModelProviderData_Value.openAitenantModelProviderData,
    4: ModelProviderData_Value.volcTenantModelProviderData,
    0: ModelProviderData_Value.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..oo(0, [1, 2, 3, 4])
    ..aOM<GeminiTenantModelProviderData>(
        1, _omitFieldNames ? '' : 'geminiTenantModelProviderData',
        subBuilder: GeminiTenantModelProviderData.create)
    ..aOM<DashScopeTenantModelProviderData>(
        2, _omitFieldNames ? '' : 'dashScopeTenantModelProviderData',
        subBuilder: DashScopeTenantModelProviderData.create)
    ..aOM<OpenAITenantModelProviderData>(
        3, _omitFieldNames ? '' : 'openAitenantModelProviderData',
        subBuilder: OpenAITenantModelProviderData.create)
    ..aOM<VolcTenantModelProviderData>(
        4, _omitFieldNames ? '' : 'volcTenantModelProviderData',
        subBuilder: VolcTenantModelProviderData.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelProviderData copyWith(void Function(ModelProviderData) updates) =>
      super.copyWith((message) => updates(message as ModelProviderData))
          as ModelProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelProviderData create() => ModelProviderData._();
  @$core.override
  ModelProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelProviderData>(create);
  static ModelProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  ModelProviderData_Value whichValue() =>
      _ModelProviderData_ValueByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  void clearValue() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  GeminiTenantModelProviderData get geminiTenantModelProviderData => $_getN(0);
  @$pb.TagNumber(1)
  set geminiTenantModelProviderData(GeminiTenantModelProviderData value) =>
      $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasGeminiTenantModelProviderData() => $_has(0);
  @$pb.TagNumber(1)
  void clearGeminiTenantModelProviderData() => $_clearField(1);
  @$pb.TagNumber(1)
  GeminiTenantModelProviderData ensureGeminiTenantModelProviderData() =>
      $_ensure(0);

  @$pb.TagNumber(2)
  DashScopeTenantModelProviderData get dashScopeTenantModelProviderData =>
      $_getN(1);
  @$pb.TagNumber(2)
  set dashScopeTenantModelProviderData(
          DashScopeTenantModelProviderData value) =>
      $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasDashScopeTenantModelProviderData() => $_has(1);
  @$pb.TagNumber(2)
  void clearDashScopeTenantModelProviderData() => $_clearField(2);
  @$pb.TagNumber(2)
  DashScopeTenantModelProviderData ensureDashScopeTenantModelProviderData() =>
      $_ensure(1);

  @$pb.TagNumber(3)
  OpenAITenantModelProviderData get openAitenantModelProviderData => $_getN(2);
  @$pb.TagNumber(3)
  set openAitenantModelProviderData(OpenAITenantModelProviderData value) =>
      $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasOpenAitenantModelProviderData() => $_has(2);
  @$pb.TagNumber(3)
  void clearOpenAitenantModelProviderData() => $_clearField(3);
  @$pb.TagNumber(3)
  OpenAITenantModelProviderData ensureOpenAitenantModelProviderData() =>
      $_ensure(2);

  @$pb.TagNumber(4)
  VolcTenantModelProviderData get volcTenantModelProviderData => $_getN(3);
  @$pb.TagNumber(4)
  set volcTenantModelProviderData(VolcTenantModelProviderData value) =>
      $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasVolcTenantModelProviderData() => $_has(3);
  @$pb.TagNumber(4)
  void clearVolcTenantModelProviderData() => $_clearField(4);
  @$pb.TagNumber(4)
  VolcTenantModelProviderData ensureVolcTenantModelProviderData() =>
      $_ensure(3);
}

class ModelPutRequest extends $pb.GeneratedMessage {
  factory ModelPutRequest({
    Model? body,
    $core.String? id,
  }) {
    final result = create();
    if (body != null) result.body = body;
    if (id != null) result.id = id;
    return result;
  }

  ModelPutRequest._();

  factory ModelPutRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelPutRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelPutRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Model>(1, _omitFieldNames ? '' : 'body', subBuilder: Model.create)
    ..aOS(2, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelPutRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelPutRequest copyWith(void Function(ModelPutRequest) updates) =>
      super.copyWith((message) => updates(message as ModelPutRequest))
          as ModelPutRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelPutRequest create() => ModelPutRequest._();
  @$core.override
  ModelPutRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelPutRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelPutRequest>(create);
  static ModelPutRequest? _defaultInstance;

  @$pb.TagNumber(1)
  Model get body => $_getN(0);
  @$pb.TagNumber(1)
  set body(Model value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasBody() => $_has(0);
  @$pb.TagNumber(1)
  void clearBody() => $_clearField(1);
  @$pb.TagNumber(1)
  Model ensureBody() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get id => $_getSZ(1);
  @$pb.TagNumber(2)
  set id($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasId() => $_has(1);
  @$pb.TagNumber(2)
  void clearId() => $_clearField(2);
}

class ModelPutResponse extends $pb.GeneratedMessage {
  factory ModelPutResponse({
    Model? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ModelPutResponse._();

  factory ModelPutResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelPutResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelPutResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Model>(1, _omitFieldNames ? '' : 'value', subBuilder: Model.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelPutResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelPutResponse copyWith(void Function(ModelPutResponse) updates) =>
      super.copyWith((message) => updates(message as ModelPutResponse))
          as ModelPutResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelPutResponse create() => ModelPutResponse._();
  @$core.override
  ModelPutResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelPutResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelPutResponse>(create);
  static ModelPutResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Model get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Model value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Model ensureValue() => $_ensure(0);
}

class ModelThinkingCapability extends $pb.GeneratedMessage {
  factory ModelThinkingCapability({
    $core.String? defaultLevel,
    $core.String? levelParam,
    $core.Iterable<$core.String>? levels,
    $core.String? param,
    $core.bool? supported,
  }) {
    final result = create();
    if (defaultLevel != null) result.defaultLevel = defaultLevel;
    if (levelParam != null) result.levelParam = levelParam;
    if (levels != null) result.levels.addAll(levels);
    if (param != null) result.param = param;
    if (supported != null) result.supported = supported;
    return result;
  }

  ModelThinkingCapability._();

  factory ModelThinkingCapability.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ModelThinkingCapability.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ModelThinkingCapability',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'defaultLevel')
    ..aOS(2, _omitFieldNames ? '' : 'levelParam')
    ..pPS(3, _omitFieldNames ? '' : 'levels')
    ..aOS(4, _omitFieldNames ? '' : 'param')
    ..aOB(5, _omitFieldNames ? '' : 'supported')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelThinkingCapability clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ModelThinkingCapability copyWith(
          void Function(ModelThinkingCapability) updates) =>
      super.copyWith((message) => updates(message as ModelThinkingCapability))
          as ModelThinkingCapability;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ModelThinkingCapability create() => ModelThinkingCapability._();
  @$core.override
  ModelThinkingCapability createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ModelThinkingCapability getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ModelThinkingCapability>(create);
  static ModelThinkingCapability? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get defaultLevel => $_getSZ(0);
  @$pb.TagNumber(1)
  set defaultLevel($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDefaultLevel() => $_has(0);
  @$pb.TagNumber(1)
  void clearDefaultLevel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get levelParam => $_getSZ(1);
  @$pb.TagNumber(2)
  set levelParam($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLevelParam() => $_has(1);
  @$pb.TagNumber(2)
  void clearLevelParam() => $_clearField(2);

  @$pb.TagNumber(3)
  $pb.PbList<$core.String> get levels => $_getList(2);

  @$pb.TagNumber(4)
  $core.String get param => $_getSZ(3);
  @$pb.TagNumber(4)
  set param($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasParam() => $_has(3);
  @$pb.TagNumber(4)
  void clearParam() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get supported => $_getBF(4);
  @$pb.TagNumber(5)
  set supported($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSupported() => $_has(4);
  @$pb.TagNumber(5)
  void clearSupported() => $_clearField(5);
}

class OpenAICredentialBody extends $pb.GeneratedMessage {
  factory OpenAICredentialBody({
    $core.String? apiKey,
    $core.String? baseUrl,
    $core.String? organization,
    $core.String? project,
    $core.String? token,
  }) {
    final result = create();
    if (apiKey != null) result.apiKey = apiKey;
    if (baseUrl != null) result.baseUrl = baseUrl;
    if (organization != null) result.organization = organization;
    if (project != null) result.project = project;
    if (token != null) result.token = token;
    return result;
  }

  OpenAICredentialBody._();

  factory OpenAICredentialBody.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory OpenAICredentialBody.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'OpenAICredentialBody',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'apiKey')
    ..aOS(2, _omitFieldNames ? '' : 'baseUrl')
    ..aOS(3, _omitFieldNames ? '' : 'organization')
    ..aOS(4, _omitFieldNames ? '' : 'project')
    ..aOS(5, _omitFieldNames ? '' : 'token')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  OpenAICredentialBody clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  OpenAICredentialBody copyWith(void Function(OpenAICredentialBody) updates) =>
      super.copyWith((message) => updates(message as OpenAICredentialBody))
          as OpenAICredentialBody;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static OpenAICredentialBody create() => OpenAICredentialBody._();
  @$core.override
  OpenAICredentialBody createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static OpenAICredentialBody getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<OpenAICredentialBody>(create);
  static OpenAICredentialBody? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get apiKey => $_getSZ(0);
  @$pb.TagNumber(1)
  set apiKey($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasApiKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearApiKey() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get baseUrl => $_getSZ(1);
  @$pb.TagNumber(2)
  set baseUrl($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasBaseUrl() => $_has(1);
  @$pb.TagNumber(2)
  void clearBaseUrl() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get organization => $_getSZ(2);
  @$pb.TagNumber(3)
  set organization($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasOrganization() => $_has(2);
  @$pb.TagNumber(3)
  void clearOrganization() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get project => $_getSZ(3);
  @$pb.TagNumber(4)
  set project($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasProject() => $_has(3);
  @$pb.TagNumber(4)
  void clearProject() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get token => $_getSZ(4);
  @$pb.TagNumber(5)
  set token($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasToken() => $_has(4);
  @$pb.TagNumber(5)
  void clearToken() => $_clearField(5);
}

class OpenAITenantModelProviderData extends $pb.GeneratedMessage {
  factory OpenAITenantModelProviderData({
    $core.String? defaultThinkingLevel,
    $core.bool? supportJsonOutput,
    $core.bool? supportTextOnly,
    $core.bool? supportThinking,
    $core.bool? supportToolCalls,
    $core.String? thinkingLevelParam,
    $core.Iterable<$core.String>? thinkingLevels,
    $core.String? thinkingParam,
    $core.String? upstreamModel,
    $core.bool? useSystemRole,
  }) {
    final result = create();
    if (defaultThinkingLevel != null)
      result.defaultThinkingLevel = defaultThinkingLevel;
    if (supportJsonOutput != null) result.supportJsonOutput = supportJsonOutput;
    if (supportTextOnly != null) result.supportTextOnly = supportTextOnly;
    if (supportThinking != null) result.supportThinking = supportThinking;
    if (supportToolCalls != null) result.supportToolCalls = supportToolCalls;
    if (thinkingLevelParam != null)
      result.thinkingLevelParam = thinkingLevelParam;
    if (thinkingLevels != null) result.thinkingLevels.addAll(thinkingLevels);
    if (thinkingParam != null) result.thinkingParam = thinkingParam;
    if (upstreamModel != null) result.upstreamModel = upstreamModel;
    if (useSystemRole != null) result.useSystemRole = useSystemRole;
    return result;
  }

  OpenAITenantModelProviderData._();

  factory OpenAITenantModelProviderData.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory OpenAITenantModelProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'OpenAITenantModelProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'defaultThinkingLevel')
    ..aOB(2, _omitFieldNames ? '' : 'supportJsonOutput')
    ..aOB(3, _omitFieldNames ? '' : 'supportTextOnly')
    ..aOB(4, _omitFieldNames ? '' : 'supportThinking')
    ..aOB(5, _omitFieldNames ? '' : 'supportToolCalls')
    ..aOS(6, _omitFieldNames ? '' : 'thinkingLevelParam')
    ..pPS(7, _omitFieldNames ? '' : 'thinkingLevels')
    ..aOS(8, _omitFieldNames ? '' : 'thinkingParam')
    ..aOS(9, _omitFieldNames ? '' : 'upstreamModel')
    ..aOB(10, _omitFieldNames ? '' : 'useSystemRole')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  OpenAITenantModelProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  OpenAITenantModelProviderData copyWith(
          void Function(OpenAITenantModelProviderData) updates) =>
      super.copyWith(
              (message) => updates(message as OpenAITenantModelProviderData))
          as OpenAITenantModelProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static OpenAITenantModelProviderData create() =>
      OpenAITenantModelProviderData._();
  @$core.override
  OpenAITenantModelProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static OpenAITenantModelProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<OpenAITenantModelProviderData>(create);
  static OpenAITenantModelProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get defaultThinkingLevel => $_getSZ(0);
  @$pb.TagNumber(1)
  set defaultThinkingLevel($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDefaultThinkingLevel() => $_has(0);
  @$pb.TagNumber(1)
  void clearDefaultThinkingLevel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get supportJsonOutput => $_getBF(1);
  @$pb.TagNumber(2)
  set supportJsonOutput($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSupportJsonOutput() => $_has(1);
  @$pb.TagNumber(2)
  void clearSupportJsonOutput() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get supportTextOnly => $_getBF(2);
  @$pb.TagNumber(3)
  set supportTextOnly($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSupportTextOnly() => $_has(2);
  @$pb.TagNumber(3)
  void clearSupportTextOnly() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get supportThinking => $_getBF(3);
  @$pb.TagNumber(4)
  set supportThinking($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSupportThinking() => $_has(3);
  @$pb.TagNumber(4)
  void clearSupportThinking() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get supportToolCalls => $_getBF(4);
  @$pb.TagNumber(5)
  set supportToolCalls($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSupportToolCalls() => $_has(4);
  @$pb.TagNumber(5)
  void clearSupportToolCalls() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get thinkingLevelParam => $_getSZ(5);
  @$pb.TagNumber(6)
  set thinkingLevelParam($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasThinkingLevelParam() => $_has(5);
  @$pb.TagNumber(6)
  void clearThinkingLevelParam() => $_clearField(6);

  @$pb.TagNumber(7)
  $pb.PbList<$core.String> get thinkingLevels => $_getList(6);

  @$pb.TagNumber(8)
  $core.String get thinkingParam => $_getSZ(7);
  @$pb.TagNumber(8)
  set thinkingParam($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasThinkingParam() => $_has(7);
  @$pb.TagNumber(8)
  void clearThinkingParam() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get upstreamModel => $_getSZ(8);
  @$pb.TagNumber(9)
  set upstreamModel($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasUpstreamModel() => $_has(8);
  @$pb.TagNumber(9)
  void clearUpstreamModel() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.bool get useSystemRole => $_getBF(9);
  @$pb.TagNumber(10)
  set useSystemRole($core.bool value) => $_setBool(9, value);
  @$pb.TagNumber(10)
  $core.bool hasUseSystemRole() => $_has(9);
  @$pb.TagNumber(10)
  void clearUseSystemRole() => $_clearField(10);
}

class OpenAITenantVoiceProviderData extends $pb.GeneratedMessage {
  factory OpenAITenantVoiceProviderData({
    $0.Struct? raw,
    $core.String? voiceId,
  }) {
    final result = create();
    if (raw != null) result.raw = raw;
    if (voiceId != null) result.voiceId = voiceId;
    return result;
  }

  OpenAITenantVoiceProviderData._();

  factory OpenAITenantVoiceProviderData.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory OpenAITenantVoiceProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'OpenAITenantVoiceProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<$0.Struct>(1, _omitFieldNames ? '' : 'raw',
        subBuilder: $0.Struct.create)
    ..aOS(2, _omitFieldNames ? '' : 'voiceId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  OpenAITenantVoiceProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  OpenAITenantVoiceProviderData copyWith(
          void Function(OpenAITenantVoiceProviderData) updates) =>
      super.copyWith(
              (message) => updates(message as OpenAITenantVoiceProviderData))
          as OpenAITenantVoiceProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static OpenAITenantVoiceProviderData create() =>
      OpenAITenantVoiceProviderData._();
  @$core.override
  OpenAITenantVoiceProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static OpenAITenantVoiceProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<OpenAITenantVoiceProviderData>(create);
  static OpenAITenantVoiceProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  $0.Struct get raw => $_getN(0);
  @$pb.TagNumber(1)
  set raw($0.Struct value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasRaw() => $_has(0);
  @$pb.TagNumber(1)
  void clearRaw() => $_clearField(1);
  @$pb.TagNumber(1)
  $0.Struct ensureRaw() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get voiceId => $_getSZ(1);
  @$pb.TagNumber(2)
  set voiceId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasVoiceId() => $_has(1);
  @$pb.TagNumber(2)
  void clearVoiceId() => $_clearField(2);
}

class Voice extends $pb.GeneratedMessage {
  factory Voice({
    $core.String? createdAt,
    $core.String? description,
    $core.String? id,
    $core.String? name,
    VoiceProvider? provider,
    VoiceProviderData? providerData,
    $1.VoiceSource? source,
    $core.String? syncedAt,
    $core.String? updatedAt,
  }) {
    final result = create();
    if (createdAt != null) result.createdAt = createdAt;
    if (description != null) result.description = description;
    if (id != null) result.id = id;
    if (name != null) result.name = name;
    if (provider != null) result.provider = provider;
    if (providerData != null) result.providerData = providerData;
    if (source != null) result.source = source;
    if (syncedAt != null) result.syncedAt = syncedAt;
    if (updatedAt != null) result.updatedAt = updatedAt;
    return result;
  }

  Voice._();

  factory Voice.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Voice.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Voice',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'createdAt')
    ..aOS(2, _omitFieldNames ? '' : 'description')
    ..aOS(3, _omitFieldNames ? '' : 'id')
    ..aOS(4, _omitFieldNames ? '' : 'name')
    ..aOM<VoiceProvider>(5, _omitFieldNames ? '' : 'provider',
        subBuilder: VoiceProvider.create)
    ..aOM<VoiceProviderData>(6, _omitFieldNames ? '' : 'providerData',
        subBuilder: VoiceProviderData.create)
    ..aE<$1.VoiceSource>(7, _omitFieldNames ? '' : 'source',
        enumValues: $1.VoiceSource.values)
    ..aOS(8, _omitFieldNames ? '' : 'syncedAt')
    ..aOS(9, _omitFieldNames ? '' : 'updatedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Voice clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Voice copyWith(void Function(Voice) updates) =>
      super.copyWith((message) => updates(message as Voice)) as Voice;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Voice create() => Voice._();
  @$core.override
  Voice createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Voice getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Voice>(create);
  static Voice? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get createdAt => $_getSZ(0);
  @$pb.TagNumber(1)
  set createdAt($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCreatedAt() => $_has(0);
  @$pb.TagNumber(1)
  void clearCreatedAt() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get description => $_getSZ(1);
  @$pb.TagNumber(2)
  set description($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDescription() => $_has(1);
  @$pb.TagNumber(2)
  void clearDescription() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get id => $_getSZ(2);
  @$pb.TagNumber(3)
  set id($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasId() => $_has(2);
  @$pb.TagNumber(3)
  void clearId() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get name => $_getSZ(3);
  @$pb.TagNumber(4)
  set name($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasName() => $_has(3);
  @$pb.TagNumber(4)
  void clearName() => $_clearField(4);

  @$pb.TagNumber(5)
  VoiceProvider get provider => $_getN(4);
  @$pb.TagNumber(5)
  set provider(VoiceProvider value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasProvider() => $_has(4);
  @$pb.TagNumber(5)
  void clearProvider() => $_clearField(5);
  @$pb.TagNumber(5)
  VoiceProvider ensureProvider() => $_ensure(4);

  @$pb.TagNumber(6)
  VoiceProviderData get providerData => $_getN(5);
  @$pb.TagNumber(6)
  set providerData(VoiceProviderData value) => $_setField(6, value);
  @$pb.TagNumber(6)
  $core.bool hasProviderData() => $_has(5);
  @$pb.TagNumber(6)
  void clearProviderData() => $_clearField(6);
  @$pb.TagNumber(6)
  VoiceProviderData ensureProviderData() => $_ensure(5);

  @$pb.TagNumber(7)
  $1.VoiceSource get source => $_getN(6);
  @$pb.TagNumber(7)
  set source($1.VoiceSource value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasSource() => $_has(6);
  @$pb.TagNumber(7)
  void clearSource() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get syncedAt => $_getSZ(7);
  @$pb.TagNumber(8)
  set syncedAt($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasSyncedAt() => $_has(7);
  @$pb.TagNumber(8)
  void clearSyncedAt() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get updatedAt => $_getSZ(8);
  @$pb.TagNumber(9)
  set updatedAt($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasUpdatedAt() => $_has(8);
  @$pb.TagNumber(9)
  void clearUpdatedAt() => $_clearField(9);
}

class VoiceGetRequest extends $pb.GeneratedMessage {
  factory VoiceGetRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  VoiceGetRequest._();

  factory VoiceGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory VoiceGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'VoiceGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VoiceGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VoiceGetRequest copyWith(void Function(VoiceGetRequest) updates) =>
      super.copyWith((message) => updates(message as VoiceGetRequest))
          as VoiceGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static VoiceGetRequest create() => VoiceGetRequest._();
  @$core.override
  VoiceGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static VoiceGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<VoiceGetRequest>(create);
  static VoiceGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class VoiceGetResponse extends $pb.GeneratedMessage {
  factory VoiceGetResponse({
    Voice? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  VoiceGetResponse._();

  factory VoiceGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory VoiceGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'VoiceGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Voice>(1, _omitFieldNames ? '' : 'value', subBuilder: Voice.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VoiceGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VoiceGetResponse copyWith(void Function(VoiceGetResponse) updates) =>
      super.copyWith((message) => updates(message as VoiceGetResponse))
          as VoiceGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static VoiceGetResponse create() => VoiceGetResponse._();
  @$core.override
  VoiceGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static VoiceGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<VoiceGetResponse>(create);
  static VoiceGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Voice get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Voice value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Voice ensureValue() => $_ensure(0);
}

class VoiceListRequest extends $pb.GeneratedMessage {
  factory VoiceListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    return result;
  }

  VoiceListRequest._();

  factory VoiceListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory VoiceListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'VoiceListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VoiceListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VoiceListRequest copyWith(void Function(VoiceListRequest) updates) =>
      super.copyWith((message) => updates(message as VoiceListRequest))
          as VoiceListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static VoiceListRequest create() => VoiceListRequest._();
  @$core.override
  VoiceListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static VoiceListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<VoiceListRequest>(create);
  static VoiceListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get limit => $_getI64(1);
  @$pb.TagNumber(2)
  set limit($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLimit() => $_has(1);
  @$pb.TagNumber(2)
  void clearLimit() => $_clearField(2);
}

class VoiceListResponse extends $pb.GeneratedMessage {
  factory VoiceListResponse({
    $core.bool? hasNext,
    $core.Iterable<Voice>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  VoiceListResponse._();

  factory VoiceListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory VoiceListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'VoiceListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<Voice>(2, _omitFieldNames ? '' : 'items', subBuilder: Voice.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VoiceListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VoiceListResponse copyWith(void Function(VoiceListResponse) updates) =>
      super.copyWith((message) => updates(message as VoiceListResponse))
          as VoiceListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static VoiceListResponse create() => VoiceListResponse._();
  @$core.override
  VoiceListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static VoiceListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<VoiceListResponse>(create);
  static VoiceListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<Voice> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class VoiceProvider extends $pb.GeneratedMessage {
  factory VoiceProvider({
    $1.VoiceProviderKind? kind,
    $core.String? name,
  }) {
    final result = create();
    if (kind != null) result.kind = kind;
    if (name != null) result.name = name;
    return result;
  }

  VoiceProvider._();

  factory VoiceProvider.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory VoiceProvider.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'VoiceProvider',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<$1.VoiceProviderKind>(1, _omitFieldNames ? '' : 'kind',
        enumValues: $1.VoiceProviderKind.values)
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VoiceProvider clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VoiceProvider copyWith(void Function(VoiceProvider) updates) =>
      super.copyWith((message) => updates(message as VoiceProvider))
          as VoiceProvider;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static VoiceProvider create() => VoiceProvider._();
  @$core.override
  VoiceProvider createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static VoiceProvider getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<VoiceProvider>(create);
  static VoiceProvider? _defaultInstance;

  @$pb.TagNumber(1)
  $1.VoiceProviderKind get kind => $_getN(0);
  @$pb.TagNumber(1)
  set kind($1.VoiceProviderKind value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasKind() => $_has(0);
  @$pb.TagNumber(1)
  void clearKind() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);
}

enum VoiceProviderData_Value {
  geminiTenantVoiceProviderData,
  dashScopeTenantVoiceProviderData,
  openAitenantVoiceProviderData,
  miniMaxTenantVoiceProviderData,
  volcTenantVoiceProviderData,
  notSet
}

class VoiceProviderData extends $pb.GeneratedMessage {
  factory VoiceProviderData({
    GeminiTenantVoiceProviderData? geminiTenantVoiceProviderData,
    DashScopeTenantVoiceProviderData? dashScopeTenantVoiceProviderData,
    OpenAITenantVoiceProviderData? openAitenantVoiceProviderData,
    MiniMaxTenantVoiceProviderData? miniMaxTenantVoiceProviderData,
    VolcTenantVoiceProviderData? volcTenantVoiceProviderData,
  }) {
    final result = create();
    if (geminiTenantVoiceProviderData != null)
      result.geminiTenantVoiceProviderData = geminiTenantVoiceProviderData;
    if (dashScopeTenantVoiceProviderData != null)
      result.dashScopeTenantVoiceProviderData =
          dashScopeTenantVoiceProviderData;
    if (openAitenantVoiceProviderData != null)
      result.openAitenantVoiceProviderData = openAitenantVoiceProviderData;
    if (miniMaxTenantVoiceProviderData != null)
      result.miniMaxTenantVoiceProviderData = miniMaxTenantVoiceProviderData;
    if (volcTenantVoiceProviderData != null)
      result.volcTenantVoiceProviderData = volcTenantVoiceProviderData;
    return result;
  }

  VoiceProviderData._();

  factory VoiceProviderData.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory VoiceProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, VoiceProviderData_Value>
      _VoiceProviderData_ValueByTag = {
    1: VoiceProviderData_Value.geminiTenantVoiceProviderData,
    2: VoiceProviderData_Value.dashScopeTenantVoiceProviderData,
    3: VoiceProviderData_Value.openAitenantVoiceProviderData,
    4: VoiceProviderData_Value.miniMaxTenantVoiceProviderData,
    5: VoiceProviderData_Value.volcTenantVoiceProviderData,
    0: VoiceProviderData_Value.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'VoiceProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..oo(0, [1, 2, 3, 4, 5])
    ..aOM<GeminiTenantVoiceProviderData>(
        1, _omitFieldNames ? '' : 'geminiTenantVoiceProviderData',
        subBuilder: GeminiTenantVoiceProviderData.create)
    ..aOM<DashScopeTenantVoiceProviderData>(
        2, _omitFieldNames ? '' : 'dashScopeTenantVoiceProviderData',
        subBuilder: DashScopeTenantVoiceProviderData.create)
    ..aOM<OpenAITenantVoiceProviderData>(
        3, _omitFieldNames ? '' : 'openAitenantVoiceProviderData',
        subBuilder: OpenAITenantVoiceProviderData.create)
    ..aOM<MiniMaxTenantVoiceProviderData>(
        4, _omitFieldNames ? '' : 'miniMaxTenantVoiceProviderData',
        subBuilder: MiniMaxTenantVoiceProviderData.create)
    ..aOM<VolcTenantVoiceProviderData>(
        5, _omitFieldNames ? '' : 'volcTenantVoiceProviderData',
        subBuilder: VolcTenantVoiceProviderData.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VoiceProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VoiceProviderData copyWith(void Function(VoiceProviderData) updates) =>
      super.copyWith((message) => updates(message as VoiceProviderData))
          as VoiceProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static VoiceProviderData create() => VoiceProviderData._();
  @$core.override
  VoiceProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static VoiceProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<VoiceProviderData>(create);
  static VoiceProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  @$pb.TagNumber(5)
  VoiceProviderData_Value whichValue() =>
      _VoiceProviderData_ValueByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(1)
  @$pb.TagNumber(2)
  @$pb.TagNumber(3)
  @$pb.TagNumber(4)
  @$pb.TagNumber(5)
  void clearValue() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  GeminiTenantVoiceProviderData get geminiTenantVoiceProviderData => $_getN(0);
  @$pb.TagNumber(1)
  set geminiTenantVoiceProviderData(GeminiTenantVoiceProviderData value) =>
      $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasGeminiTenantVoiceProviderData() => $_has(0);
  @$pb.TagNumber(1)
  void clearGeminiTenantVoiceProviderData() => $_clearField(1);
  @$pb.TagNumber(1)
  GeminiTenantVoiceProviderData ensureGeminiTenantVoiceProviderData() =>
      $_ensure(0);

  @$pb.TagNumber(2)
  DashScopeTenantVoiceProviderData get dashScopeTenantVoiceProviderData =>
      $_getN(1);
  @$pb.TagNumber(2)
  set dashScopeTenantVoiceProviderData(
          DashScopeTenantVoiceProviderData value) =>
      $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasDashScopeTenantVoiceProviderData() => $_has(1);
  @$pb.TagNumber(2)
  void clearDashScopeTenantVoiceProviderData() => $_clearField(2);
  @$pb.TagNumber(2)
  DashScopeTenantVoiceProviderData ensureDashScopeTenantVoiceProviderData() =>
      $_ensure(1);

  @$pb.TagNumber(3)
  OpenAITenantVoiceProviderData get openAitenantVoiceProviderData => $_getN(2);
  @$pb.TagNumber(3)
  set openAitenantVoiceProviderData(OpenAITenantVoiceProviderData value) =>
      $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasOpenAitenantVoiceProviderData() => $_has(2);
  @$pb.TagNumber(3)
  void clearOpenAitenantVoiceProviderData() => $_clearField(3);
  @$pb.TagNumber(3)
  OpenAITenantVoiceProviderData ensureOpenAitenantVoiceProviderData() =>
      $_ensure(2);

  @$pb.TagNumber(4)
  MiniMaxTenantVoiceProviderData get miniMaxTenantVoiceProviderData =>
      $_getN(3);
  @$pb.TagNumber(4)
  set miniMaxTenantVoiceProviderData(MiniMaxTenantVoiceProviderData value) =>
      $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasMiniMaxTenantVoiceProviderData() => $_has(3);
  @$pb.TagNumber(4)
  void clearMiniMaxTenantVoiceProviderData() => $_clearField(4);
  @$pb.TagNumber(4)
  MiniMaxTenantVoiceProviderData ensureMiniMaxTenantVoiceProviderData() =>
      $_ensure(3);

  @$pb.TagNumber(5)
  VolcTenantVoiceProviderData get volcTenantVoiceProviderData => $_getN(4);
  @$pb.TagNumber(5)
  set volcTenantVoiceProviderData(VolcTenantVoiceProviderData value) =>
      $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasVolcTenantVoiceProviderData() => $_has(4);
  @$pb.TagNumber(5)
  void clearVolcTenantVoiceProviderData() => $_clearField(5);
  @$pb.TagNumber(5)
  VolcTenantVoiceProviderData ensureVolcTenantVoiceProviderData() =>
      $_ensure(4);
}

class VolcCredentialBody extends $pb.GeneratedMessage {
  factory VolcCredentialBody({
    $core.String? apiKey,
    $core.String? appId,
    $core.String? openapiAccessKey,
    $core.String? openapiAccessKeyId,
    $core.String? searchApiKey,
  }) {
    final result = create();
    if (apiKey != null) result.apiKey = apiKey;
    if (appId != null) result.appId = appId;
    if (openapiAccessKey != null) result.openapiAccessKey = openapiAccessKey;
    if (openapiAccessKeyId != null)
      result.openapiAccessKeyId = openapiAccessKeyId;
    if (searchApiKey != null) result.searchApiKey = searchApiKey;
    return result;
  }

  VolcCredentialBody._();

  factory VolcCredentialBody.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory VolcCredentialBody.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'VolcCredentialBody',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'apiKey')
    ..aOS(2, _omitFieldNames ? '' : 'appId')
    ..aOS(3, _omitFieldNames ? '' : 'openapiAccessKey')
    ..aOS(4, _omitFieldNames ? '' : 'openapiAccessKeyId')
    ..aOS(5, _omitFieldNames ? '' : 'searchApiKey')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VolcCredentialBody clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VolcCredentialBody copyWith(void Function(VolcCredentialBody) updates) =>
      super.copyWith((message) => updates(message as VolcCredentialBody))
          as VolcCredentialBody;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static VolcCredentialBody create() => VolcCredentialBody._();
  @$core.override
  VolcCredentialBody createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static VolcCredentialBody getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<VolcCredentialBody>(create);
  static VolcCredentialBody? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get apiKey => $_getSZ(0);
  @$pb.TagNumber(1)
  set apiKey($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasApiKey() => $_has(0);
  @$pb.TagNumber(1)
  void clearApiKey() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get appId => $_getSZ(1);
  @$pb.TagNumber(2)
  set appId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasAppId() => $_has(1);
  @$pb.TagNumber(2)
  void clearAppId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get openapiAccessKey => $_getSZ(2);
  @$pb.TagNumber(3)
  set openapiAccessKey($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasOpenapiAccessKey() => $_has(2);
  @$pb.TagNumber(3)
  void clearOpenapiAccessKey() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get openapiAccessKeyId => $_getSZ(3);
  @$pb.TagNumber(4)
  set openapiAccessKeyId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasOpenapiAccessKeyId() => $_has(3);
  @$pb.TagNumber(4)
  void clearOpenapiAccessKeyId() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get searchApiKey => $_getSZ(4);
  @$pb.TagNumber(5)
  set searchApiKey($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSearchApiKey() => $_has(4);
  @$pb.TagNumber(5)
  void clearSearchApiKey() => $_clearField(5);
}

class VolcTenantModelProviderData extends $pb.GeneratedMessage {
  factory VolcTenantModelProviderData({
    $1.VolcTenantModelProviderDataApiMode? apiMode,
    $core.String? defaultThinkingLevel,
    $core.String? resourceId,
    $core.bool? supportJsonOutput,
    $core.bool? supportTextOnly,
    $core.bool? supportThinking,
    $core.bool? supportToolCalls,
    $core.String? thinkingLevelParam,
    $core.Iterable<$core.String>? thinkingLevels,
    $core.String? thinkingParam,
    $core.String? upstreamModel,
    $core.bool? useSystemRole,
  }) {
    final result = create();
    if (apiMode != null) result.apiMode = apiMode;
    if (defaultThinkingLevel != null)
      result.defaultThinkingLevel = defaultThinkingLevel;
    if (resourceId != null) result.resourceId = resourceId;
    if (supportJsonOutput != null) result.supportJsonOutput = supportJsonOutput;
    if (supportTextOnly != null) result.supportTextOnly = supportTextOnly;
    if (supportThinking != null) result.supportThinking = supportThinking;
    if (supportToolCalls != null) result.supportToolCalls = supportToolCalls;
    if (thinkingLevelParam != null)
      result.thinkingLevelParam = thinkingLevelParam;
    if (thinkingLevels != null) result.thinkingLevels.addAll(thinkingLevels);
    if (thinkingParam != null) result.thinkingParam = thinkingParam;
    if (upstreamModel != null) result.upstreamModel = upstreamModel;
    if (useSystemRole != null) result.useSystemRole = useSystemRole;
    return result;
  }

  VolcTenantModelProviderData._();

  factory VolcTenantModelProviderData.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory VolcTenantModelProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'VolcTenantModelProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<$1.VolcTenantModelProviderDataApiMode>(
        1, _omitFieldNames ? '' : 'apiMode',
        enumValues: $1.VolcTenantModelProviderDataApiMode.values)
    ..aOS(2, _omitFieldNames ? '' : 'defaultThinkingLevel')
    ..aOS(3, _omitFieldNames ? '' : 'resourceId')
    ..aOB(4, _omitFieldNames ? '' : 'supportJsonOutput')
    ..aOB(5, _omitFieldNames ? '' : 'supportTextOnly')
    ..aOB(6, _omitFieldNames ? '' : 'supportThinking')
    ..aOB(7, _omitFieldNames ? '' : 'supportToolCalls')
    ..aOS(8, _omitFieldNames ? '' : 'thinkingLevelParam')
    ..pPS(9, _omitFieldNames ? '' : 'thinkingLevels')
    ..aOS(10, _omitFieldNames ? '' : 'thinkingParam')
    ..aOS(11, _omitFieldNames ? '' : 'upstreamModel')
    ..aOB(12, _omitFieldNames ? '' : 'useSystemRole')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VolcTenantModelProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VolcTenantModelProviderData copyWith(
          void Function(VolcTenantModelProviderData) updates) =>
      super.copyWith(
              (message) => updates(message as VolcTenantModelProviderData))
          as VolcTenantModelProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static VolcTenantModelProviderData create() =>
      VolcTenantModelProviderData._();
  @$core.override
  VolcTenantModelProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static VolcTenantModelProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<VolcTenantModelProviderData>(create);
  static VolcTenantModelProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  $1.VolcTenantModelProviderDataApiMode get apiMode => $_getN(0);
  @$pb.TagNumber(1)
  set apiMode($1.VolcTenantModelProviderDataApiMode value) =>
      $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasApiMode() => $_has(0);
  @$pb.TagNumber(1)
  void clearApiMode() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get defaultThinkingLevel => $_getSZ(1);
  @$pb.TagNumber(2)
  set defaultThinkingLevel($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDefaultThinkingLevel() => $_has(1);
  @$pb.TagNumber(2)
  void clearDefaultThinkingLevel() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get resourceId => $_getSZ(2);
  @$pb.TagNumber(3)
  set resourceId($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasResourceId() => $_has(2);
  @$pb.TagNumber(3)
  void clearResourceId() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get supportJsonOutput => $_getBF(3);
  @$pb.TagNumber(4)
  set supportJsonOutput($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSupportJsonOutput() => $_has(3);
  @$pb.TagNumber(4)
  void clearSupportJsonOutput() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get supportTextOnly => $_getBF(4);
  @$pb.TagNumber(5)
  set supportTextOnly($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSupportTextOnly() => $_has(4);
  @$pb.TagNumber(5)
  void clearSupportTextOnly() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.bool get supportThinking => $_getBF(5);
  @$pb.TagNumber(6)
  set supportThinking($core.bool value) => $_setBool(5, value);
  @$pb.TagNumber(6)
  $core.bool hasSupportThinking() => $_has(5);
  @$pb.TagNumber(6)
  void clearSupportThinking() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get supportToolCalls => $_getBF(6);
  @$pb.TagNumber(7)
  set supportToolCalls($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasSupportToolCalls() => $_has(6);
  @$pb.TagNumber(7)
  void clearSupportToolCalls() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get thinkingLevelParam => $_getSZ(7);
  @$pb.TagNumber(8)
  set thinkingLevelParam($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasThinkingLevelParam() => $_has(7);
  @$pb.TagNumber(8)
  void clearThinkingLevelParam() => $_clearField(8);

  @$pb.TagNumber(9)
  $pb.PbList<$core.String> get thinkingLevels => $_getList(8);

  @$pb.TagNumber(10)
  $core.String get thinkingParam => $_getSZ(9);
  @$pb.TagNumber(10)
  set thinkingParam($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasThinkingParam() => $_has(9);
  @$pb.TagNumber(10)
  void clearThinkingParam() => $_clearField(10);

  @$pb.TagNumber(11)
  $core.String get upstreamModel => $_getSZ(10);
  @$pb.TagNumber(11)
  set upstreamModel($core.String value) => $_setString(10, value);
  @$pb.TagNumber(11)
  $core.bool hasUpstreamModel() => $_has(10);
  @$pb.TagNumber(11)
  void clearUpstreamModel() => $_clearField(11);

  @$pb.TagNumber(12)
  $core.bool get useSystemRole => $_getBF(11);
  @$pb.TagNumber(12)
  set useSystemRole($core.bool value) => $_setBool(11, value);
  @$pb.TagNumber(12)
  $core.bool hasUseSystemRole() => $_has(11);
  @$pb.TagNumber(12)
  void clearUseSystemRole() => $_clearField(12);
}

class VolcTenantVoiceProviderData extends $pb.GeneratedMessage {
  factory VolcTenantVoiceProviderData({
    $0.Struct? raw,
    $core.String? resourceId,
    $core.String? state,
    $core.String? status,
    $core.String? voiceId,
  }) {
    final result = create();
    if (raw != null) result.raw = raw;
    if (resourceId != null) result.resourceId = resourceId;
    if (state != null) result.state = state;
    if (status != null) result.status = status;
    if (voiceId != null) result.voiceId = voiceId;
    return result;
  }

  VolcTenantVoiceProviderData._();

  factory VolcTenantVoiceProviderData.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory VolcTenantVoiceProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'VolcTenantVoiceProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<$0.Struct>(1, _omitFieldNames ? '' : 'raw',
        subBuilder: $0.Struct.create)
    ..aOS(2, _omitFieldNames ? '' : 'resourceId')
    ..aOS(3, _omitFieldNames ? '' : 'state')
    ..aOS(4, _omitFieldNames ? '' : 'status')
    ..aOS(5, _omitFieldNames ? '' : 'voiceId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VolcTenantVoiceProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  VolcTenantVoiceProviderData copyWith(
          void Function(VolcTenantVoiceProviderData) updates) =>
      super.copyWith(
              (message) => updates(message as VolcTenantVoiceProviderData))
          as VolcTenantVoiceProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static VolcTenantVoiceProviderData create() =>
      VolcTenantVoiceProviderData._();
  @$core.override
  VolcTenantVoiceProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static VolcTenantVoiceProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<VolcTenantVoiceProviderData>(create);
  static VolcTenantVoiceProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  $0.Struct get raw => $_getN(0);
  @$pb.TagNumber(1)
  set raw($0.Struct value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasRaw() => $_has(0);
  @$pb.TagNumber(1)
  void clearRaw() => $_clearField(1);
  @$pb.TagNumber(1)
  $0.Struct ensureRaw() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get resourceId => $_getSZ(1);
  @$pb.TagNumber(2)
  set resourceId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasResourceId() => $_has(1);
  @$pb.TagNumber(2)
  void clearResourceId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get state => $_getSZ(2);
  @$pb.TagNumber(3)
  set state($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasState() => $_has(2);
  @$pb.TagNumber(3)
  void clearState() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get status => $_getSZ(3);
  @$pb.TagNumber(4)
  set status($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasStatus() => $_has(3);
  @$pb.TagNumber(4)
  void clearStatus() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get voiceId => $_getSZ(4);
  @$pb.TagNumber(5)
  set voiceId($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasVoiceId() => $_has(4);
  @$pb.TagNumber(5)
  void clearVoiceId() => $_clearField(5);
}

class WorkflowCreateRequest extends $pb.GeneratedMessage {
  factory WorkflowCreateRequest({
    WorkflowDocument? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  WorkflowCreateRequest._();

  factory WorkflowCreateRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowCreateRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowCreateRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<WorkflowDocument>(1, _omitFieldNames ? '' : 'value',
        subBuilder: WorkflowDocument.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowCreateRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowCreateRequest copyWith(
          void Function(WorkflowCreateRequest) updates) =>
      super.copyWith((message) => updates(message as WorkflowCreateRequest))
          as WorkflowCreateRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowCreateRequest create() => WorkflowCreateRequest._();
  @$core.override
  WorkflowCreateRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowCreateRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowCreateRequest>(create);
  static WorkflowCreateRequest? _defaultInstance;

  @$pb.TagNumber(1)
  WorkflowDocument get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(WorkflowDocument value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  WorkflowDocument ensureValue() => $_ensure(0);
}

class WorkflowCreateResponse extends $pb.GeneratedMessage {
  factory WorkflowCreateResponse({
    WorkflowDocument? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  WorkflowCreateResponse._();

  factory WorkflowCreateResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowCreateResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowCreateResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<WorkflowDocument>(1, _omitFieldNames ? '' : 'value',
        subBuilder: WorkflowDocument.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowCreateResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowCreateResponse copyWith(
          void Function(WorkflowCreateResponse) updates) =>
      super.copyWith((message) => updates(message as WorkflowCreateResponse))
          as WorkflowCreateResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowCreateResponse create() => WorkflowCreateResponse._();
  @$core.override
  WorkflowCreateResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowCreateResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowCreateResponse>(create);
  static WorkflowCreateResponse? _defaultInstance;

  @$pb.TagNumber(1)
  WorkflowDocument get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(WorkflowDocument value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  WorkflowDocument ensureValue() => $_ensure(0);
}

class WorkflowDeleteRequest extends $pb.GeneratedMessage {
  factory WorkflowDeleteRequest({
    $core.String? name,
  }) {
    final result = create();
    if (name != null) result.name = name;
    return result;
  }

  WorkflowDeleteRequest._();

  factory WorkflowDeleteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowDeleteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowDeleteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowDeleteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowDeleteRequest copyWith(
          void Function(WorkflowDeleteRequest) updates) =>
      super.copyWith((message) => updates(message as WorkflowDeleteRequest))
          as WorkflowDeleteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowDeleteRequest create() => WorkflowDeleteRequest._();
  @$core.override
  WorkflowDeleteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowDeleteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowDeleteRequest>(create);
  static WorkflowDeleteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);
}

class WorkflowDeleteResponse extends $pb.GeneratedMessage {
  factory WorkflowDeleteResponse({
    WorkflowDocument? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  WorkflowDeleteResponse._();

  factory WorkflowDeleteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowDeleteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowDeleteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<WorkflowDocument>(1, _omitFieldNames ? '' : 'value',
        subBuilder: WorkflowDocument.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowDeleteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowDeleteResponse copyWith(
          void Function(WorkflowDeleteResponse) updates) =>
      super.copyWith((message) => updates(message as WorkflowDeleteResponse))
          as WorkflowDeleteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowDeleteResponse create() => WorkflowDeleteResponse._();
  @$core.override
  WorkflowDeleteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowDeleteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowDeleteResponse>(create);
  static WorkflowDeleteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  WorkflowDocument get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(WorkflowDocument value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  WorkflowDocument ensureValue() => $_ensure(0);
}

class WorkflowDocument extends $pb.GeneratedMessage {
  factory WorkflowDocument({
    WorkflowMetadata? metadata,
    WorkflowSpec? spec,
    WorkflowI18n? i18n,
  }) {
    final result = create();
    if (metadata != null) result.metadata = metadata;
    if (spec != null) result.spec = spec;
    if (i18n != null) result.i18n = i18n;
    return result;
  }

  WorkflowDocument._();

  factory WorkflowDocument.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowDocument.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowDocument',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<WorkflowMetadata>(1, _omitFieldNames ? '' : 'metadata',
        subBuilder: WorkflowMetadata.create)
    ..aOM<WorkflowSpec>(2, _omitFieldNames ? '' : 'spec',
        subBuilder: WorkflowSpec.create)
    ..aOM<WorkflowI18n>(3, _omitFieldNames ? '' : 'i18n',
        subBuilder: WorkflowI18n.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowDocument clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowDocument copyWith(void Function(WorkflowDocument) updates) =>
      super.copyWith((message) => updates(message as WorkflowDocument))
          as WorkflowDocument;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowDocument create() => WorkflowDocument._();
  @$core.override
  WorkflowDocument createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowDocument getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowDocument>(create);
  static WorkflowDocument? _defaultInstance;

  @$pb.TagNumber(1)
  WorkflowMetadata get metadata => $_getN(0);
  @$pb.TagNumber(1)
  set metadata(WorkflowMetadata value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasMetadata() => $_has(0);
  @$pb.TagNumber(1)
  void clearMetadata() => $_clearField(1);
  @$pb.TagNumber(1)
  WorkflowMetadata ensureMetadata() => $_ensure(0);

  @$pb.TagNumber(2)
  WorkflowSpec get spec => $_getN(1);
  @$pb.TagNumber(2)
  set spec(WorkflowSpec value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasSpec() => $_has(1);
  @$pb.TagNumber(2)
  void clearSpec() => $_clearField(2);
  @$pb.TagNumber(2)
  WorkflowSpec ensureSpec() => $_ensure(1);

  @$pb.TagNumber(3)
  WorkflowI18n get i18n => $_getN(2);
  @$pb.TagNumber(3)
  set i18n(WorkflowI18n value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasI18n() => $_has(2);
  @$pb.TagNumber(3)
  void clearI18n() => $_clearField(3);
  @$pb.TagNumber(3)
  WorkflowI18n ensureI18n() => $_ensure(2);
}

class WorkflowGetRequest extends $pb.GeneratedMessage {
  factory WorkflowGetRequest({
    $core.String? name,
  }) {
    final result = create();
    if (name != null) result.name = name;
    return result;
  }

  WorkflowGetRequest._();

  factory WorkflowGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowGetRequest copyWith(void Function(WorkflowGetRequest) updates) =>
      super.copyWith((message) => updates(message as WorkflowGetRequest))
          as WorkflowGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowGetRequest create() => WorkflowGetRequest._();
  @$core.override
  WorkflowGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowGetRequest>(create);
  static WorkflowGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);
}

class WorkflowGetResponse extends $pb.GeneratedMessage {
  factory WorkflowGetResponse({
    WorkflowDocument? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  WorkflowGetResponse._();

  factory WorkflowGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<WorkflowDocument>(1, _omitFieldNames ? '' : 'value',
        subBuilder: WorkflowDocument.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowGetResponse copyWith(void Function(WorkflowGetResponse) updates) =>
      super.copyWith((message) => updates(message as WorkflowGetResponse))
          as WorkflowGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowGetResponse create() => WorkflowGetResponse._();
  @$core.override
  WorkflowGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowGetResponse>(create);
  static WorkflowGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  WorkflowDocument get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(WorkflowDocument value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  WorkflowDocument ensureValue() => $_ensure(0);
}

class WorkflowListRequest extends $pb.GeneratedMessage {
  factory WorkflowListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    return result;
  }

  WorkflowListRequest._();

  factory WorkflowListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowListRequest copyWith(void Function(WorkflowListRequest) updates) =>
      super.copyWith((message) => updates(message as WorkflowListRequest))
          as WorkflowListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowListRequest create() => WorkflowListRequest._();
  @$core.override
  WorkflowListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowListRequest>(create);
  static WorkflowListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get limit => $_getI64(1);
  @$pb.TagNumber(2)
  set limit($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLimit() => $_has(1);
  @$pb.TagNumber(2)
  void clearLimit() => $_clearField(2);
}

class WorkflowListResponse extends $pb.GeneratedMessage {
  factory WorkflowListResponse({
    $core.bool? hasNext,
    $core.Iterable<WorkflowDocument>? items,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  WorkflowListResponse._();

  factory WorkflowListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOB(1, _omitFieldNames ? '' : 'hasNext')
    ..pPM<WorkflowDocument>(2, _omitFieldNames ? '' : 'items',
        subBuilder: WorkflowDocument.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowListResponse copyWith(void Function(WorkflowListResponse) updates) =>
      super.copyWith((message) => updates(message as WorkflowListResponse))
          as WorkflowListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowListResponse create() => WorkflowListResponse._();
  @$core.override
  WorkflowListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowListResponse>(create);
  static WorkflowListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.bool get hasNext => $_getBF(0);
  @$pb.TagNumber(1)
  set hasNext($core.bool value) => $_setBool(0, value);
  @$pb.TagNumber(1)
  $core.bool hasHasNext() => $_has(0);
  @$pb.TagNumber(1)
  void clearHasNext() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<WorkflowDocument> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class WorkflowMetadata extends $pb.GeneratedMessage {
  factory WorkflowMetadata({
    $core.String? name,
  }) {
    final result = create();
    if (name != null) result.name = name;
    return result;
  }

  WorkflowMetadata._();

  factory WorkflowMetadata.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowMetadata.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowMetadata',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowMetadata clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowMetadata copyWith(void Function(WorkflowMetadata) updates) =>
      super.copyWith((message) => updates(message as WorkflowMetadata))
          as WorkflowMetadata;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowMetadata create() => WorkflowMetadata._();
  @$core.override
  WorkflowMetadata createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowMetadata getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowMetadata>(create);
  static WorkflowMetadata? _defaultInstance;

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);
}

class WorkflowI18nCatalog extends $pb.GeneratedMessage {
  factory WorkflowI18nCatalog({
    $core.String? name,
    $core.String? description,
  }) {
    final result = create();
    if (name != null) result.name = name;
    if (description != null) result.description = description;
    return result;
  }

  WorkflowI18nCatalog._();

  factory WorkflowI18nCatalog.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowI18nCatalog.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowI18nCatalog',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..aOS(2, _omitFieldNames ? '' : 'description')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowI18nCatalog clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowI18nCatalog copyWith(void Function(WorkflowI18nCatalog) updates) =>
      super.copyWith((message) => updates(message as WorkflowI18nCatalog))
          as WorkflowI18nCatalog;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowI18nCatalog create() => WorkflowI18nCatalog._();
  @$core.override
  WorkflowI18nCatalog createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowI18nCatalog getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowI18nCatalog>(create);
  static WorkflowI18nCatalog? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get description => $_getSZ(1);
  @$pb.TagNumber(2)
  set description($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDescription() => $_has(1);
  @$pb.TagNumber(2)
  void clearDescription() => $_clearField(2);
}

class WorkflowI18n extends $pb.GeneratedMessage {
  factory WorkflowI18n({
    $core.String? defaultLocale,
    $core.Iterable<$core.MapEntry<$core.String, WorkflowI18nCatalog>>? value,
  }) {
    final result = create();
    if (defaultLocale != null) result.defaultLocale = defaultLocale;
    if (value != null) result.value.addEntries(value);
    return result;
  }

  WorkflowI18n._();

  factory WorkflowI18n.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowI18n.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowI18n',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'defaultLocale')
    ..m<$core.String, WorkflowI18nCatalog>(2, _omitFieldNames ? '' : 'value',
        entryClassName: 'WorkflowI18n.ValueEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OM,
        valueCreator: WorkflowI18nCatalog.create,
        valueDefaultOrMaker: WorkflowI18nCatalog.getDefault,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowI18n clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowI18n copyWith(void Function(WorkflowI18n) updates) =>
      super.copyWith((message) => updates(message as WorkflowI18n))
          as WorkflowI18n;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowI18n create() => WorkflowI18n._();
  @$core.override
  WorkflowI18n createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowI18n getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowI18n>(create);
  static WorkflowI18n? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get defaultLocale => $_getSZ(0);
  @$pb.TagNumber(1)
  set defaultLocale($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDefaultLocale() => $_has(0);
  @$pb.TagNumber(1)
  void clearDefaultLocale() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbMap<$core.String, WorkflowI18nCatalog> get value => $_getMap(1);
}

class ToolkitPolicyToolIds extends $pb.GeneratedMessage {
  factory ToolkitPolicyToolIds({
    $core.Iterable<$core.String>? value,
  }) {
    final result = create();
    if (value != null) result.value.addAll(value);
    return result;
  }

  ToolkitPolicyToolIds._();

  factory ToolkitPolicyToolIds.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolkitPolicyToolIds.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolkitPolicyToolIds',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPS(1, _omitFieldNames ? '' : 'value')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolkitPolicyToolIds clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolkitPolicyToolIds copyWith(void Function(ToolkitPolicyToolIds) updates) =>
      super.copyWith((message) => updates(message as ToolkitPolicyToolIds))
          as ToolkitPolicyToolIds;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolkitPolicyToolIds create() => ToolkitPolicyToolIds._();
  @$core.override
  ToolkitPolicyToolIds createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolkitPolicyToolIds getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolkitPolicyToolIds>(create);
  static ToolkitPolicyToolIds? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<$core.String> get value => $_getList(0);
}

class ToolkitPolicy extends $pb.GeneratedMessage {
  factory ToolkitPolicy({
    ToolkitPolicyToolIds? toolIds,
  }) {
    final result = create();
    if (toolIds != null) result.toolIds = toolIds;
    return result;
  }

  ToolkitPolicy._();

  factory ToolkitPolicy.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolkitPolicy.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolkitPolicy',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<ToolkitPolicyToolIds>(1, _omitFieldNames ? '' : 'toolIds',
        subBuilder: ToolkitPolicyToolIds.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolkitPolicy clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolkitPolicy copyWith(void Function(ToolkitPolicy) updates) =>
      super.copyWith((message) => updates(message as ToolkitPolicy))
          as ToolkitPolicy;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolkitPolicy create() => ToolkitPolicy._();
  @$core.override
  ToolkitPolicy createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolkitPolicy getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolkitPolicy>(create);
  static ToolkitPolicy? _defaultInstance;

  @$pb.TagNumber(1)
  ToolkitPolicyToolIds get toolIds => $_getN(0);
  @$pb.TagNumber(1)
  set toolIds(ToolkitPolicyToolIds value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasToolIds() => $_has(0);
  @$pb.TagNumber(1)
  void clearToolIds() => $_clearField(1);
  @$pb.TagNumber(1)
  ToolkitPolicyToolIds ensureToolIds() => $_ensure(0);
}

class WorkflowPutRequest extends $pb.GeneratedMessage {
  factory WorkflowPutRequest({
    WorkflowDocument? body,
    $core.String? name,
  }) {
    final result = create();
    if (body != null) result.body = body;
    if (name != null) result.name = name;
    return result;
  }

  WorkflowPutRequest._();

  factory WorkflowPutRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowPutRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowPutRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<WorkflowDocument>(1, _omitFieldNames ? '' : 'body',
        subBuilder: WorkflowDocument.create)
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowPutRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowPutRequest copyWith(void Function(WorkflowPutRequest) updates) =>
      super.copyWith((message) => updates(message as WorkflowPutRequest))
          as WorkflowPutRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowPutRequest create() => WorkflowPutRequest._();
  @$core.override
  WorkflowPutRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowPutRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowPutRequest>(create);
  static WorkflowPutRequest? _defaultInstance;

  @$pb.TagNumber(1)
  WorkflowDocument get body => $_getN(0);
  @$pb.TagNumber(1)
  set body(WorkflowDocument value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasBody() => $_has(0);
  @$pb.TagNumber(1)
  void clearBody() => $_clearField(1);
  @$pb.TagNumber(1)
  WorkflowDocument ensureBody() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);
}

class WorkflowPutResponse extends $pb.GeneratedMessage {
  factory WorkflowPutResponse({
    WorkflowDocument? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  WorkflowPutResponse._();

  factory WorkflowPutResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowPutResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowPutResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<WorkflowDocument>(1, _omitFieldNames ? '' : 'value',
        subBuilder: WorkflowDocument.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowPutResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowPutResponse copyWith(void Function(WorkflowPutResponse) updates) =>
      super.copyWith((message) => updates(message as WorkflowPutResponse))
          as WorkflowPutResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowPutResponse create() => WorkflowPutResponse._();
  @$core.override
  WorkflowPutResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowPutResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowPutResponse>(create);
  static WorkflowPutResponse? _defaultInstance;

  @$pb.TagNumber(1)
  WorkflowDocument get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(WorkflowDocument value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  WorkflowDocument ensureValue() => $_ensure(0);
}

class WorkflowSpec extends $pb.GeneratedMessage {
  factory WorkflowSpec({
    ASTTranslateWorkflowSpec? astTranslate,
    ChatRoomWorkflowSpec? chatroom,
    DoubaoRealtimeWorkflowSpec? doubaoRealtime,
    $1.WorkflowDriver? driver,
    FlowcraftWorkflowSpec? flowcraft,
    ToolkitPolicy? toolkit,
  }) {
    final result = create();
    if (astTranslate != null) result.astTranslate = astTranslate;
    if (chatroom != null) result.chatroom = chatroom;
    if (doubaoRealtime != null) result.doubaoRealtime = doubaoRealtime;
    if (driver != null) result.driver = driver;
    if (flowcraft != null) result.flowcraft = flowcraft;
    if (toolkit != null) result.toolkit = toolkit;
    return result;
  }

  WorkflowSpec._();

  factory WorkflowSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WorkflowSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WorkflowSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<ASTTranslateWorkflowSpec>(1, _omitFieldNames ? '' : 'astTranslate',
        subBuilder: ASTTranslateWorkflowSpec.create)
    ..aOM<ChatRoomWorkflowSpec>(2, _omitFieldNames ? '' : 'chatroom',
        subBuilder: ChatRoomWorkflowSpec.create)
    ..aOM<DoubaoRealtimeWorkflowSpec>(
        3, _omitFieldNames ? '' : 'doubaoRealtime',
        subBuilder: DoubaoRealtimeWorkflowSpec.create)
    ..aE<$1.WorkflowDriver>(4, _omitFieldNames ? '' : 'driver',
        enumValues: $1.WorkflowDriver.values)
    ..aOM<FlowcraftWorkflowSpec>(5, _omitFieldNames ? '' : 'flowcraft',
        subBuilder: FlowcraftWorkflowSpec.create)
    ..aOM<ToolkitPolicy>(6, _omitFieldNames ? '' : 'toolkit',
        subBuilder: ToolkitPolicy.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WorkflowSpec copyWith(void Function(WorkflowSpec) updates) =>
      super.copyWith((message) => updates(message as WorkflowSpec))
          as WorkflowSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WorkflowSpec create() => WorkflowSpec._();
  @$core.override
  WorkflowSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WorkflowSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WorkflowSpec>(create);
  static WorkflowSpec? _defaultInstance;

  @$pb.TagNumber(1)
  ASTTranslateWorkflowSpec get astTranslate => $_getN(0);
  @$pb.TagNumber(1)
  set astTranslate(ASTTranslateWorkflowSpec value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasAstTranslate() => $_has(0);
  @$pb.TagNumber(1)
  void clearAstTranslate() => $_clearField(1);
  @$pb.TagNumber(1)
  ASTTranslateWorkflowSpec ensureAstTranslate() => $_ensure(0);

  @$pb.TagNumber(2)
  ChatRoomWorkflowSpec get chatroom => $_getN(1);
  @$pb.TagNumber(2)
  set chatroom(ChatRoomWorkflowSpec value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasChatroom() => $_has(1);
  @$pb.TagNumber(2)
  void clearChatroom() => $_clearField(2);
  @$pb.TagNumber(2)
  ChatRoomWorkflowSpec ensureChatroom() => $_ensure(1);

  @$pb.TagNumber(3)
  DoubaoRealtimeWorkflowSpec get doubaoRealtime => $_getN(2);
  @$pb.TagNumber(3)
  set doubaoRealtime(DoubaoRealtimeWorkflowSpec value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasDoubaoRealtime() => $_has(2);
  @$pb.TagNumber(3)
  void clearDoubaoRealtime() => $_clearField(3);
  @$pb.TagNumber(3)
  DoubaoRealtimeWorkflowSpec ensureDoubaoRealtime() => $_ensure(2);

  @$pb.TagNumber(4)
  $1.WorkflowDriver get driver => $_getN(3);
  @$pb.TagNumber(4)
  set driver($1.WorkflowDriver value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasDriver() => $_has(3);
  @$pb.TagNumber(4)
  void clearDriver() => $_clearField(4);

  @$pb.TagNumber(5)
  FlowcraftWorkflowSpec get flowcraft => $_getN(4);
  @$pb.TagNumber(5)
  set flowcraft(FlowcraftWorkflowSpec value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasFlowcraft() => $_has(4);
  @$pb.TagNumber(5)
  void clearFlowcraft() => $_clearField(5);
  @$pb.TagNumber(5)
  FlowcraftWorkflowSpec ensureFlowcraft() => $_ensure(4);

  @$pb.TagNumber(6)
  ToolkitPolicy get toolkit => $_getN(5);
  @$pb.TagNumber(6)
  set toolkit(ToolkitPolicy value) => $_setField(6, value);
  @$pb.TagNumber(6)
  $core.bool hasToolkit() => $_has(5);
  @$pb.TagNumber(6)
  void clearToolkit() => $_clearField(6);
  @$pb.TagNumber(6)
  ToolkitPolicy ensureToolkit() => $_ensure(5);
}

class ToolExecutor extends $pb.GeneratedMessage {
  factory ToolExecutor({
    $1.ToolExecutorKind? kind,
    $core.String? name,
    $core.String? method,
    $core.String? peerId,
    $0.Struct? config,
  }) {
    final result = create();
    if (kind != null) result.kind = kind;
    if (name != null) result.name = name;
    if (method != null) result.method = method;
    if (peerId != null) result.peerId = peerId;
    if (config != null) result.config = config;
    return result;
  }

  ToolExecutor._();

  factory ToolExecutor.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolExecutor.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolExecutor',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<$1.ToolExecutorKind>(1, _omitFieldNames ? '' : 'kind',
        enumValues: $1.ToolExecutorKind.values)
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..aOS(3, _omitFieldNames ? '' : 'method')
    ..aOS(4, _omitFieldNames ? '' : 'peerId')
    ..aOM<$0.Struct>(5, _omitFieldNames ? '' : 'config',
        subBuilder: $0.Struct.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolExecutor clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolExecutor copyWith(void Function(ToolExecutor) updates) =>
      super.copyWith((message) => updates(message as ToolExecutor))
          as ToolExecutor;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolExecutor create() => ToolExecutor._();
  @$core.override
  ToolExecutor createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolExecutor getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolExecutor>(create);
  static ToolExecutor? _defaultInstance;

  @$pb.TagNumber(1)
  $1.ToolExecutorKind get kind => $_getN(0);
  @$pb.TagNumber(1)
  set kind($1.ToolExecutorKind value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasKind() => $_has(0);
  @$pb.TagNumber(1)
  void clearKind() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get method => $_getSZ(2);
  @$pb.TagNumber(3)
  set method($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMethod() => $_has(2);
  @$pb.TagNumber(3)
  void clearMethod() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get peerId => $_getSZ(3);
  @$pb.TagNumber(4)
  set peerId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasPeerId() => $_has(3);
  @$pb.TagNumber(4)
  void clearPeerId() => $_clearField(4);

  @$pb.TagNumber(5)
  $0.Struct get config => $_getN(4);
  @$pb.TagNumber(5)
  set config($0.Struct value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasConfig() => $_has(4);
  @$pb.TagNumber(5)
  void clearConfig() => $_clearField(5);
  @$pb.TagNumber(5)
  $0.Struct ensureConfig() => $_ensure(4);
}

class ToolTriggerExample extends $pb.GeneratedMessage {
  factory ToolTriggerExample({
    $core.String? input,
    $0.Struct? args,
    $core.String? output,
  }) {
    final result = create();
    if (input != null) result.input = input;
    if (args != null) result.args = args;
    if (output != null) result.output = output;
    return result;
  }

  ToolTriggerExample._();

  factory ToolTriggerExample.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolTriggerExample.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolTriggerExample',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'input')
    ..aOM<$0.Struct>(2, _omitFieldNames ? '' : 'args',
        subBuilder: $0.Struct.create)
    ..aOS(3, _omitFieldNames ? '' : 'output')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolTriggerExample clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolTriggerExample copyWith(void Function(ToolTriggerExample) updates) =>
      super.copyWith((message) => updates(message as ToolTriggerExample))
          as ToolTriggerExample;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolTriggerExample create() => ToolTriggerExample._();
  @$core.override
  ToolTriggerExample createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolTriggerExample getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolTriggerExample>(create);
  static ToolTriggerExample? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get input => $_getSZ(0);
  @$pb.TagNumber(1)
  set input($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasInput() => $_has(0);
  @$pb.TagNumber(1)
  void clearInput() => $_clearField(1);

  @$pb.TagNumber(2)
  $0.Struct get args => $_getN(1);
  @$pb.TagNumber(2)
  set args($0.Struct value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasArgs() => $_has(1);
  @$pb.TagNumber(2)
  void clearArgs() => $_clearField(2);
  @$pb.TagNumber(2)
  $0.Struct ensureArgs() => $_ensure(1);

  @$pb.TagNumber(3)
  $core.String get output => $_getSZ(2);
  @$pb.TagNumber(3)
  set output($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasOutput() => $_has(2);
  @$pb.TagNumber(3)
  void clearOutput() => $_clearField(3);
}

class ToolTrigger extends $pb.GeneratedMessage {
  factory ToolTrigger({
    $core.String? name,
    $core.String? description,
    $core.Iterable<$core.String>? patterns,
    $core.Iterable<ToolTriggerExample>? examples,
    $0.Struct? metadata,
  }) {
    final result = create();
    if (name != null) result.name = name;
    if (description != null) result.description = description;
    if (patterns != null) result.patterns.addAll(patterns);
    if (examples != null) result.examples.addAll(examples);
    if (metadata != null) result.metadata = metadata;
    return result;
  }

  ToolTrigger._();

  factory ToolTrigger.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolTrigger.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolTrigger',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'name')
    ..aOS(2, _omitFieldNames ? '' : 'description')
    ..pPS(3, _omitFieldNames ? '' : 'patterns')
    ..pPM<ToolTriggerExample>(4, _omitFieldNames ? '' : 'examples',
        subBuilder: ToolTriggerExample.create)
    ..aOM<$0.Struct>(5, _omitFieldNames ? '' : 'metadata',
        subBuilder: $0.Struct.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolTrigger clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolTrigger copyWith(void Function(ToolTrigger) updates) =>
      super.copyWith((message) => updates(message as ToolTrigger))
          as ToolTrigger;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolTrigger create() => ToolTrigger._();
  @$core.override
  ToolTrigger createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolTrigger getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolTrigger>(create);
  static ToolTrigger? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get name => $_getSZ(0);
  @$pb.TagNumber(1)
  set name($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasName() => $_has(0);
  @$pb.TagNumber(1)
  void clearName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get description => $_getSZ(1);
  @$pb.TagNumber(2)
  set description($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDescription() => $_has(1);
  @$pb.TagNumber(2)
  void clearDescription() => $_clearField(2);

  @$pb.TagNumber(3)
  $pb.PbList<$core.String> get patterns => $_getList(2);

  @$pb.TagNumber(4)
  $pb.PbList<ToolTriggerExample> get examples => $_getList(3);

  @$pb.TagNumber(5)
  $0.Struct get metadata => $_getN(4);
  @$pb.TagNumber(5)
  set metadata($0.Struct value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasMetadata() => $_has(4);
  @$pb.TagNumber(5)
  void clearMetadata() => $_clearField(5);
  @$pb.TagNumber(5)
  $0.Struct ensureMetadata() => $_ensure(4);
}

class Tool extends $pb.GeneratedMessage {
  factory Tool({
    $core.String? id,
    $core.String? name,
    $core.String? description,
    $1.ToolSource? source,
    $core.bool? enabled,
    $core.String? ownerPeer,
    $core.String? version,
    $0.Struct? inputSchema,
    $0.Struct? outputSchema,
    $core.Iterable<ToolTrigger>? triggers,
    ToolExecutor? executor,
    $0.Struct? metadata,
    $core.String? createdAt,
    $core.String? updatedAt,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (name != null) result.name = name;
    if (description != null) result.description = description;
    if (source != null) result.source = source;
    if (enabled != null) result.enabled = enabled;
    if (ownerPeer != null) result.ownerPeer = ownerPeer;
    if (version != null) result.version = version;
    if (inputSchema != null) result.inputSchema = inputSchema;
    if (outputSchema != null) result.outputSchema = outputSchema;
    if (triggers != null) result.triggers.addAll(triggers);
    if (executor != null) result.executor = executor;
    if (metadata != null) result.metadata = metadata;
    if (createdAt != null) result.createdAt = createdAt;
    if (updatedAt != null) result.updatedAt = updatedAt;
    return result;
  }

  Tool._();

  factory Tool.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Tool.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Tool',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..aOS(3, _omitFieldNames ? '' : 'description')
    ..aE<$1.ToolSource>(4, _omitFieldNames ? '' : 'source',
        enumValues: $1.ToolSource.values)
    ..aOB(5, _omitFieldNames ? '' : 'enabled')
    ..aOS(6, _omitFieldNames ? '' : 'ownerPeer')
    ..aOS(7, _omitFieldNames ? '' : 'version')
    ..aOM<$0.Struct>(8, _omitFieldNames ? '' : 'inputSchema',
        subBuilder: $0.Struct.create)
    ..aOM<$0.Struct>(9, _omitFieldNames ? '' : 'outputSchema',
        subBuilder: $0.Struct.create)
    ..pPM<ToolTrigger>(10, _omitFieldNames ? '' : 'triggers',
        subBuilder: ToolTrigger.create)
    ..aOM<ToolExecutor>(11, _omitFieldNames ? '' : 'executor',
        subBuilder: ToolExecutor.create)
    ..aOM<$0.Struct>(12, _omitFieldNames ? '' : 'metadata',
        subBuilder: $0.Struct.create)
    ..aOS(13, _omitFieldNames ? '' : 'createdAt')
    ..aOS(14, _omitFieldNames ? '' : 'updatedAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Tool clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Tool copyWith(void Function(Tool) updates) =>
      super.copyWith((message) => updates(message as Tool)) as Tool;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Tool create() => Tool._();
  @$core.override
  Tool createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Tool getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Tool>(create);
  static Tool? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get description => $_getSZ(2);
  @$pb.TagNumber(3)
  set description($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasDescription() => $_has(2);
  @$pb.TagNumber(3)
  void clearDescription() => $_clearField(3);

  @$pb.TagNumber(4)
  $1.ToolSource get source => $_getN(3);
  @$pb.TagNumber(4)
  set source($1.ToolSource value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasSource() => $_has(3);
  @$pb.TagNumber(4)
  void clearSource() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get enabled => $_getBF(4);
  @$pb.TagNumber(5)
  set enabled($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasEnabled() => $_has(4);
  @$pb.TagNumber(5)
  void clearEnabled() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get ownerPeer => $_getSZ(5);
  @$pb.TagNumber(6)
  set ownerPeer($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasOwnerPeer() => $_has(5);
  @$pb.TagNumber(6)
  void clearOwnerPeer() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get version => $_getSZ(6);
  @$pb.TagNumber(7)
  set version($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasVersion() => $_has(6);
  @$pb.TagNumber(7)
  void clearVersion() => $_clearField(7);

  @$pb.TagNumber(8)
  $0.Struct get inputSchema => $_getN(7);
  @$pb.TagNumber(8)
  set inputSchema($0.Struct value) => $_setField(8, value);
  @$pb.TagNumber(8)
  $core.bool hasInputSchema() => $_has(7);
  @$pb.TagNumber(8)
  void clearInputSchema() => $_clearField(8);
  @$pb.TagNumber(8)
  $0.Struct ensureInputSchema() => $_ensure(7);

  @$pb.TagNumber(9)
  $0.Struct get outputSchema => $_getN(8);
  @$pb.TagNumber(9)
  set outputSchema($0.Struct value) => $_setField(9, value);
  @$pb.TagNumber(9)
  $core.bool hasOutputSchema() => $_has(8);
  @$pb.TagNumber(9)
  void clearOutputSchema() => $_clearField(9);
  @$pb.TagNumber(9)
  $0.Struct ensureOutputSchema() => $_ensure(8);

  @$pb.TagNumber(10)
  $pb.PbList<ToolTrigger> get triggers => $_getList(9);

  @$pb.TagNumber(11)
  ToolExecutor get executor => $_getN(10);
  @$pb.TagNumber(11)
  set executor(ToolExecutor value) => $_setField(11, value);
  @$pb.TagNumber(11)
  $core.bool hasExecutor() => $_has(10);
  @$pb.TagNumber(11)
  void clearExecutor() => $_clearField(11);
  @$pb.TagNumber(11)
  ToolExecutor ensureExecutor() => $_ensure(10);

  @$pb.TagNumber(12)
  $0.Struct get metadata => $_getN(11);
  @$pb.TagNumber(12)
  set metadata($0.Struct value) => $_setField(12, value);
  @$pb.TagNumber(12)
  $core.bool hasMetadata() => $_has(11);
  @$pb.TagNumber(12)
  void clearMetadata() => $_clearField(12);
  @$pb.TagNumber(12)
  $0.Struct ensureMetadata() => $_ensure(11);

  @$pb.TagNumber(13)
  $core.String get createdAt => $_getSZ(12);
  @$pb.TagNumber(13)
  set createdAt($core.String value) => $_setString(12, value);
  @$pb.TagNumber(13)
  $core.bool hasCreatedAt() => $_has(12);
  @$pb.TagNumber(13)
  void clearCreatedAt() => $_clearField(13);

  @$pb.TagNumber(14)
  $core.String get updatedAt => $_getSZ(13);
  @$pb.TagNumber(14)
  set updatedAt($core.String value) => $_setString(13, value);
  @$pb.TagNumber(14)
  $core.bool hasUpdatedAt() => $_has(13);
  @$pb.TagNumber(14)
  void clearUpdatedAt() => $_clearField(14);
}

class ToolListRequest extends $pb.GeneratedMessage {
  factory ToolListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    return result;
  }

  ToolListRequest._();

  factory ToolListRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolListRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolListRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'cursor')
    ..aInt64(2, _omitFieldNames ? '' : 'limit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolListRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolListRequest copyWith(void Function(ToolListRequest) updates) =>
      super.copyWith((message) => updates(message as ToolListRequest))
          as ToolListRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolListRequest create() => ToolListRequest._();
  @$core.override
  ToolListRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolListRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolListRequest>(create);
  static ToolListRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get cursor => $_getSZ(0);
  @$pb.TagNumber(1)
  set cursor($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCursor() => $_has(0);
  @$pb.TagNumber(1)
  void clearCursor() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get limit => $_getI64(1);
  @$pb.TagNumber(2)
  set limit($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasLimit() => $_has(1);
  @$pb.TagNumber(2)
  void clearLimit() => $_clearField(2);
}

class ToolListResponse extends $pb.GeneratedMessage {
  factory ToolListResponse({
    $core.Iterable<Tool>? items,
    $core.bool? hasNext,
    $core.String? nextCursor,
  }) {
    final result = create();
    if (items != null) result.items.addAll(items);
    if (hasNext != null) result.hasNext = hasNext;
    if (nextCursor != null) result.nextCursor = nextCursor;
    return result;
  }

  ToolListResponse._();

  factory ToolListResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolListResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolListResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..pPM<Tool>(1, _omitFieldNames ? '' : 'items', subBuilder: Tool.create)
    ..aOB(2, _omitFieldNames ? '' : 'hasNext')
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolListResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolListResponse copyWith(void Function(ToolListResponse) updates) =>
      super.copyWith((message) => updates(message as ToolListResponse))
          as ToolListResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolListResponse create() => ToolListResponse._();
  @$core.override
  ToolListResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolListResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolListResponse>(create);
  static ToolListResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<Tool> get items => $_getList(0);

  @$pb.TagNumber(2)
  $core.bool get hasNext => $_getBF(1);
  @$pb.TagNumber(2)
  set hasNext($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasHasNext() => $_has(1);
  @$pb.TagNumber(2)
  void clearHasNext() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);
}

class ToolGetRequest extends $pb.GeneratedMessage {
  factory ToolGetRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  ToolGetRequest._();

  factory ToolGetRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolGetRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolGetRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolGetRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolGetRequest copyWith(void Function(ToolGetRequest) updates) =>
      super.copyWith((message) => updates(message as ToolGetRequest))
          as ToolGetRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolGetRequest create() => ToolGetRequest._();
  @$core.override
  ToolGetRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolGetRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolGetRequest>(create);
  static ToolGetRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class ToolGetResponse extends $pb.GeneratedMessage {
  factory ToolGetResponse({
    Tool? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ToolGetResponse._();

  factory ToolGetResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolGetResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolGetResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Tool>(1, _omitFieldNames ? '' : 'value', subBuilder: Tool.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolGetResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolGetResponse copyWith(void Function(ToolGetResponse) updates) =>
      super.copyWith((message) => updates(message as ToolGetResponse))
          as ToolGetResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolGetResponse create() => ToolGetResponse._();
  @$core.override
  ToolGetResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolGetResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolGetResponse>(create);
  static ToolGetResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Tool get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Tool value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Tool ensureValue() => $_ensure(0);
}

class ToolCreateRequest extends $pb.GeneratedMessage {
  factory ToolCreateRequest({
    Tool? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ToolCreateRequest._();

  factory ToolCreateRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolCreateRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolCreateRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Tool>(1, _omitFieldNames ? '' : 'value', subBuilder: Tool.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolCreateRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolCreateRequest copyWith(void Function(ToolCreateRequest) updates) =>
      super.copyWith((message) => updates(message as ToolCreateRequest))
          as ToolCreateRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolCreateRequest create() => ToolCreateRequest._();
  @$core.override
  ToolCreateRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolCreateRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolCreateRequest>(create);
  static ToolCreateRequest? _defaultInstance;

  @$pb.TagNumber(1)
  Tool get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Tool value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Tool ensureValue() => $_ensure(0);
}

class ToolCreateResponse extends $pb.GeneratedMessage {
  factory ToolCreateResponse({
    Tool? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ToolCreateResponse._();

  factory ToolCreateResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolCreateResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolCreateResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Tool>(1, _omitFieldNames ? '' : 'value', subBuilder: Tool.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolCreateResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolCreateResponse copyWith(void Function(ToolCreateResponse) updates) =>
      super.copyWith((message) => updates(message as ToolCreateResponse))
          as ToolCreateResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolCreateResponse create() => ToolCreateResponse._();
  @$core.override
  ToolCreateResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolCreateResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolCreateResponse>(create);
  static ToolCreateResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Tool get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Tool value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Tool ensureValue() => $_ensure(0);
}

class ToolPutRequest extends $pb.GeneratedMessage {
  factory ToolPutRequest({
    $core.String? id,
    Tool? body,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (body != null) result.body = body;
    return result;
  }

  ToolPutRequest._();

  factory ToolPutRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolPutRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolPutRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOM<Tool>(2, _omitFieldNames ? '' : 'body', subBuilder: Tool.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolPutRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolPutRequest copyWith(void Function(ToolPutRequest) updates) =>
      super.copyWith((message) => updates(message as ToolPutRequest))
          as ToolPutRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolPutRequest create() => ToolPutRequest._();
  @$core.override
  ToolPutRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolPutRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolPutRequest>(create);
  static ToolPutRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  Tool get body => $_getN(1);
  @$pb.TagNumber(2)
  set body(Tool value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasBody() => $_has(1);
  @$pb.TagNumber(2)
  void clearBody() => $_clearField(2);
  @$pb.TagNumber(2)
  Tool ensureBody() => $_ensure(1);
}

class ToolPutResponse extends $pb.GeneratedMessage {
  factory ToolPutResponse({
    Tool? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ToolPutResponse._();

  factory ToolPutResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolPutResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolPutResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Tool>(1, _omitFieldNames ? '' : 'value', subBuilder: Tool.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolPutResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolPutResponse copyWith(void Function(ToolPutResponse) updates) =>
      super.copyWith((message) => updates(message as ToolPutResponse))
          as ToolPutResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolPutResponse create() => ToolPutResponse._();
  @$core.override
  ToolPutResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolPutResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolPutResponse>(create);
  static ToolPutResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Tool get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Tool value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Tool ensureValue() => $_ensure(0);
}

class ToolDeleteRequest extends $pb.GeneratedMessage {
  factory ToolDeleteRequest({
    $core.String? id,
  }) {
    final result = create();
    if (id != null) result.id = id;
    return result;
  }

  ToolDeleteRequest._();

  factory ToolDeleteRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolDeleteRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolDeleteRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolDeleteRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolDeleteRequest copyWith(void Function(ToolDeleteRequest) updates) =>
      super.copyWith((message) => updates(message as ToolDeleteRequest))
          as ToolDeleteRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolDeleteRequest create() => ToolDeleteRequest._();
  @$core.override
  ToolDeleteRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolDeleteRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolDeleteRequest>(create);
  static ToolDeleteRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);
}

class ToolDeleteResponse extends $pb.GeneratedMessage {
  factory ToolDeleteResponse({
    Tool? value,
  }) {
    final result = create();
    if (value != null) result.value = value;
    return result;
  }

  ToolDeleteResponse._();

  factory ToolDeleteResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolDeleteResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolDeleteResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOM<Tool>(1, _omitFieldNames ? '' : 'value', subBuilder: Tool.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolDeleteResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolDeleteResponse copyWith(void Function(ToolDeleteResponse) updates) =>
      super.copyWith((message) => updates(message as ToolDeleteResponse))
          as ToolDeleteResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolDeleteResponse create() => ToolDeleteResponse._();
  @$core.override
  ToolDeleteResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolDeleteResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolDeleteResponse>(create);
  static ToolDeleteResponse? _defaultInstance;

  @$pb.TagNumber(1)
  Tool get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Tool value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Tool ensureValue() => $_ensure(0);
}

class ToolInvokeRequest extends $pb.GeneratedMessage {
  factory ToolInvokeRequest({
    $core.String? callId,
    $core.String? toolId,
    $core.String? method,
    $0.Struct? args,
  }) {
    final result = create();
    if (callId != null) result.callId = callId;
    if (toolId != null) result.toolId = toolId;
    if (method != null) result.method = method;
    if (args != null) result.args = args;
    return result;
  }

  ToolInvokeRequest._();

  factory ToolInvokeRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolInvokeRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolInvokeRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'callId')
    ..aOS(2, _omitFieldNames ? '' : 'toolId')
    ..aOS(3, _omitFieldNames ? '' : 'method')
    ..aOM<$0.Struct>(4, _omitFieldNames ? '' : 'args',
        subBuilder: $0.Struct.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolInvokeRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolInvokeRequest copyWith(void Function(ToolInvokeRequest) updates) =>
      super.copyWith((message) => updates(message as ToolInvokeRequest))
          as ToolInvokeRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolInvokeRequest create() => ToolInvokeRequest._();
  @$core.override
  ToolInvokeRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolInvokeRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolInvokeRequest>(create);
  static ToolInvokeRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get callId => $_getSZ(0);
  @$pb.TagNumber(1)
  set callId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCallId() => $_has(0);
  @$pb.TagNumber(1)
  void clearCallId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get toolId => $_getSZ(1);
  @$pb.TagNumber(2)
  set toolId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasToolId() => $_has(1);
  @$pb.TagNumber(2)
  void clearToolId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get method => $_getSZ(2);
  @$pb.TagNumber(3)
  set method($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMethod() => $_has(2);
  @$pb.TagNumber(3)
  void clearMethod() => $_clearField(3);

  @$pb.TagNumber(4)
  $0.Struct get args => $_getN(3);
  @$pb.TagNumber(4)
  set args($0.Struct value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasArgs() => $_has(3);
  @$pb.TagNumber(4)
  void clearArgs() => $_clearField(4);
  @$pb.TagNumber(4)
  $0.Struct ensureArgs() => $_ensure(3);
}

class ToolInvokeResponse extends $pb.GeneratedMessage {
  factory ToolInvokeResponse({
    $core.String? dataJson,
  }) {
    final result = create();
    if (dataJson != null) result.dataJson = dataJson;
    return result;
  }

  ToolInvokeResponse._();

  factory ToolInvokeResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ToolInvokeResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ToolInvokeResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'dataJson')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolInvokeResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ToolInvokeResponse copyWith(void Function(ToolInvokeResponse) updates) =>
      super.copyWith((message) => updates(message as ToolInvokeResponse))
          as ToolInvokeResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ToolInvokeResponse create() => ToolInvokeResponse._();
  @$core.override
  ToolInvokeResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ToolInvokeResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ToolInvokeResponse>(create);
  static ToolInvokeResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get dataJson => $_getSZ(0);
  @$pb.TagNumber(1)
  set dataJson($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDataJson() => $_has(0);
  @$pb.TagNumber(1)
  void clearDataJson() => $_clearField(1);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
