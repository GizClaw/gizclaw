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

import 'ai.pbenum.dart';
import 'enums.pbenum.dart' as $1;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

export 'ai.pbenum.dart';

class AliasI18nText extends $pb.GeneratedMessage {
  factory AliasI18nText({
    $core.String? displayName,
    $core.String? description,
  }) {
    final result = create();
    if (displayName != null) result.displayName = displayName;
    if (description != null) result.description = description;
    return result;
  }

  AliasI18nText._();

  factory AliasI18nText.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory AliasI18nText.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'AliasI18nText',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'displayName')
    ..aOS(2, _omitFieldNames ? '' : 'description')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AliasI18nText clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  AliasI18nText copyWith(void Function(AliasI18nText) updates) =>
      super.copyWith((message) => updates(message as AliasI18nText))
          as AliasI18nText;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static AliasI18nText create() => AliasI18nText._();
  @$core.override
  AliasI18nText createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static AliasI18nText getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<AliasI18nText>(create);
  static AliasI18nText? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get displayName => $_getSZ(0);
  @$pb.TagNumber(1)
  set displayName($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasDisplayName() => $_has(0);
  @$pb.TagNumber(1)
  void clearDisplayName() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get description => $_getSZ(1);
  @$pb.TagNumber(2)
  set description($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasDescription() => $_has(1);
  @$pb.TagNumber(2)
  void clearDescription() => $_clearField(2);
}

class SpeechTranscribeRequest extends $pb.GeneratedMessage {
  factory SpeechTranscribeRequest({
    $core.String? modelAlias,
    $core.String? contentType,
    $core.String? language,
  }) {
    final result = create();
    if (modelAlias != null) result.modelAlias = modelAlias;
    if (contentType != null) result.contentType = contentType;
    if (language != null) result.language = language;
    return result;
  }

  SpeechTranscribeRequest._();

  factory SpeechTranscribeRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SpeechTranscribeRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SpeechTranscribeRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'modelAlias')
    ..aOS(2, _omitFieldNames ? '' : 'contentType')
    ..aOS(3, _omitFieldNames ? '' : 'language')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeechTranscribeRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeechTranscribeRequest copyWith(
          void Function(SpeechTranscribeRequest) updates) =>
      super.copyWith((message) => updates(message as SpeechTranscribeRequest))
          as SpeechTranscribeRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SpeechTranscribeRequest create() => SpeechTranscribeRequest._();
  @$core.override
  SpeechTranscribeRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SpeechTranscribeRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SpeechTranscribeRequest>(create);
  static SpeechTranscribeRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get modelAlias => $_getSZ(0);
  @$pb.TagNumber(1)
  set modelAlias($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasModelAlias() => $_has(0);
  @$pb.TagNumber(1)
  void clearModelAlias() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get contentType => $_getSZ(1);
  @$pb.TagNumber(2)
  set contentType($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasContentType() => $_has(1);
  @$pb.TagNumber(2)
  void clearContentType() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get language => $_getSZ(2);
  @$pb.TagNumber(3)
  set language($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasLanguage() => $_has(2);
  @$pb.TagNumber(3)
  void clearLanguage() => $_clearField(3);
}

class SpeechTranscribeResponse extends $pb.GeneratedMessage {
  factory SpeechTranscribeResponse({
    $core.String? transcript,
  }) {
    final result = create();
    if (transcript != null) result.transcript = transcript;
    return result;
  }

  SpeechTranscribeResponse._();

  factory SpeechTranscribeResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SpeechTranscribeResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SpeechTranscribeResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'transcript')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeechTranscribeResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeechTranscribeResponse copyWith(
          void Function(SpeechTranscribeResponse) updates) =>
      super.copyWith((message) => updates(message as SpeechTranscribeResponse))
          as SpeechTranscribeResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SpeechTranscribeResponse create() => SpeechTranscribeResponse._();
  @$core.override
  SpeechTranscribeResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SpeechTranscribeResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SpeechTranscribeResponse>(create);
  static SpeechTranscribeResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get transcript => $_getSZ(0);
  @$pb.TagNumber(1)
  set transcript($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasTranscript() => $_has(0);
  @$pb.TagNumber(1)
  void clearTranscript() => $_clearField(1);
}

class SpeechSynthesizeRequest extends $pb.GeneratedMessage {
  factory SpeechSynthesizeRequest({
    $core.String? voiceAlias,
    $core.String? text,
    $core.Iterable<$core.String>? acceptedContentTypes,
  }) {
    final result = create();
    if (voiceAlias != null) result.voiceAlias = voiceAlias;
    if (text != null) result.text = text;
    if (acceptedContentTypes != null)
      result.acceptedContentTypes.addAll(acceptedContentTypes);
    return result;
  }

  SpeechSynthesizeRequest._();

  factory SpeechSynthesizeRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SpeechSynthesizeRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SpeechSynthesizeRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'voiceAlias')
    ..aOS(2, _omitFieldNames ? '' : 'text')
    ..pPS(3, _omitFieldNames ? '' : 'acceptedContentTypes')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeechSynthesizeRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeechSynthesizeRequest copyWith(
          void Function(SpeechSynthesizeRequest) updates) =>
      super.copyWith((message) => updates(message as SpeechSynthesizeRequest))
          as SpeechSynthesizeRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SpeechSynthesizeRequest create() => SpeechSynthesizeRequest._();
  @$core.override
  SpeechSynthesizeRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SpeechSynthesizeRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SpeechSynthesizeRequest>(create);
  static SpeechSynthesizeRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get voiceAlias => $_getSZ(0);
  @$pb.TagNumber(1)
  set voiceAlias($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasVoiceAlias() => $_has(0);
  @$pb.TagNumber(1)
  void clearVoiceAlias() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get text => $_getSZ(1);
  @$pb.TagNumber(2)
  set text($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasText() => $_has(1);
  @$pb.TagNumber(2)
  void clearText() => $_clearField(2);

  @$pb.TagNumber(3)
  $pb.PbList<$core.String> get acceptedContentTypes => $_getList(2);
}

class SpeechSynthesizeResponse extends $pb.GeneratedMessage {
  factory SpeechSynthesizeResponse({
    $core.String? contentType,
    $core.int? sampleRateHz,
    $core.int? channels,
  }) {
    final result = create();
    if (contentType != null) result.contentType = contentType;
    if (sampleRateHz != null) result.sampleRateHz = sampleRateHz;
    if (channels != null) result.channels = channels;
    return result;
  }

  SpeechSynthesizeResponse._();

  factory SpeechSynthesizeResponse.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SpeechSynthesizeResponse.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SpeechSynthesizeResponse',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'contentType')
    ..aI(2, _omitFieldNames ? '' : 'sampleRateHz')
    ..aI(3, _omitFieldNames ? '' : 'channels')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeechSynthesizeResponse clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SpeechSynthesizeResponse copyWith(
          void Function(SpeechSynthesizeResponse) updates) =>
      super.copyWith((message) => updates(message as SpeechSynthesizeResponse))
          as SpeechSynthesizeResponse;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SpeechSynthesizeResponse create() => SpeechSynthesizeResponse._();
  @$core.override
  SpeechSynthesizeResponse createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SpeechSynthesizeResponse getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SpeechSynthesizeResponse>(create);
  static SpeechSynthesizeResponse? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get contentType => $_getSZ(0);
  @$pb.TagNumber(1)
  set contentType($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasContentType() => $_has(0);
  @$pb.TagNumber(1)
  void clearContentType() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.int get sampleRateHz => $_getIZ(1);
  @$pb.TagNumber(2)
  set sampleRateHz($core.int value) => $_setSignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSampleRateHz() => $_has(1);
  @$pb.TagNumber(2)
  void clearSampleRateHz() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.int get channels => $_getIZ(2);
  @$pb.TagNumber(3)
  set channels($core.int value) => $_setSignedInt32(2, value);
  @$pb.TagNumber(3)
  $core.bool hasChannels() => $_has(2);
  @$pb.TagNumber(3)
  void clearChannels() => $_clearField(3);
}

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
    $1.ASTTranslateMode? mode,
    $core.String? resourceId,
    $core.String? translationModel,
    ASTTranslateVoiceParameters? voice,
    $core.String? langPair,
  }) {
    final result = create();
    if (denoise != null) result.denoise = denoise;
    if (enableSourceLanguageDetect != null)
      result.enableSourceLanguageDetect = enableSourceLanguageDetect;
    if (mode != null) result.mode = mode;
    if (resourceId != null) result.resourceId = resourceId;
    if (translationModel != null) result.translationModel = translationModel;
    if (voice != null) result.voice = voice;
    if (langPair != null) result.langPair = langPair;
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
    ..aE<$1.ASTTranslateMode>(3, _omitFieldNames ? '' : 'mode',
        enumValues: $1.ASTTranslateMode.values)
    ..aOS(4, _omitFieldNames ? '' : 'resourceId')
    ..aOS(5, _omitFieldNames ? '' : 'translationModel')
    ..aOM<ASTTranslateVoiceParameters>(6, _omitFieldNames ? '' : 'voice',
        subBuilder: ASTTranslateVoiceParameters.create)
    ..aOS(7, _omitFieldNames ? '' : 'langPair')
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
  $1.ASTTranslateMode get mode => $_getN(2);
  @$pb.TagNumber(3)
  set mode($1.ASTTranslateMode value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasMode() => $_has(2);
  @$pb.TagNumber(3)
  void clearMode() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get resourceId => $_getSZ(3);
  @$pb.TagNumber(4)
  set resourceId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasResourceId() => $_has(3);
  @$pb.TagNumber(4)
  void clearResourceId() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get translationModel => $_getSZ(4);
  @$pb.TagNumber(5)
  set translationModel($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasTranslationModel() => $_has(4);
  @$pb.TagNumber(5)
  void clearTranslationModel() => $_clearField(5);

  @$pb.TagNumber(6)
  ASTTranslateVoiceParameters get voice => $_getN(5);
  @$pb.TagNumber(6)
  set voice(ASTTranslateVoiceParameters value) => $_setField(6, value);
  @$pb.TagNumber(6)
  $core.bool hasVoice() => $_has(5);
  @$pb.TagNumber(6)
  void clearVoice() => $_clearField(6);
  @$pb.TagNumber(6)
  ASTTranslateVoiceParameters ensureVoice() => $_ensure(5);

  @$pb.TagNumber(7)
  $core.String get langPair => $_getSZ(6);
  @$pb.TagNumber(7)
  set langPair($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasLangPair() => $_has(6);
  @$pb.TagNumber(7)
  void clearLangPair() => $_clearField(7);
}

class ASTTranslateWorkspaceParameters extends $pb.GeneratedMessage {
  factory ASTTranslateWorkspaceParameters({
    $1.ASTTranslateWorkspaceParametersAgentType? agentType,
    $core.bool? denoise,
    $core.bool? e2e,
    $core.bool? enableSourceLanguageDetect,
    $1.WorkspaceInputMode? input,
    $core.String? langPair,
    $1.ASTTranslateMode? mode,
    $core.String? translationModel,
    ASTTranslateVoiceParameters? voice,
  }) {
    final result = create();
    if (agentType != null) result.agentType = agentType;
    if (denoise != null) result.denoise = denoise;
    if (e2e != null) result.e2e = e2e;
    if (enableSourceLanguageDetect != null)
      result.enableSourceLanguageDetect = enableSourceLanguageDetect;
    if (input != null) result.input = input;
    if (langPair != null) result.langPair = langPair;
    if (mode != null) result.mode = mode;
    if (translationModel != null) result.translationModel = translationModel;
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
    ..aOS(6, _omitFieldNames ? '' : 'langPair')
    ..aE<$1.ASTTranslateMode>(7, _omitFieldNames ? '' : 'mode',
        enumValues: $1.ASTTranslateMode.values)
    ..aOS(8, _omitFieldNames ? '' : 'translationModel')
    ..aOM<ASTTranslateVoiceParameters>(9, _omitFieldNames ? '' : 'voice',
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
  $core.String get langPair => $_getSZ(5);
  @$pb.TagNumber(6)
  set langPair($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasLangPair() => $_has(5);
  @$pb.TagNumber(6)
  void clearLangPair() => $_clearField(6);

  @$pb.TagNumber(7)
  $1.ASTTranslateMode get mode => $_getN(6);
  @$pb.TagNumber(7)
  set mode($1.ASTTranslateMode value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasMode() => $_has(6);
  @$pb.TagNumber(7)
  void clearMode() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get translationModel => $_getSZ(7);
  @$pb.TagNumber(8)
  set translationModel($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasTranslationModel() => $_has(7);
  @$pb.TagNumber(8)
  void clearTranslationModel() => $_clearField(8);

  @$pb.TagNumber(9)
  ASTTranslateVoiceParameters get voice => $_getN(8);
  @$pb.TagNumber(9)
  set voice(ASTTranslateVoiceParameters value) => $_setField(9, value);
  @$pb.TagNumber(9)
  $core.bool hasVoice() => $_has(8);
  @$pb.TagNumber(9)
  void clearVoice() => $_clearField(9);
  @$pb.TagNumber(9)
  ASTTranslateVoiceParameters ensureVoice() => $_ensure(8);
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
    $1.WorkspaceInputMode? input,
  }) {
    final result = create();
    if (agentType != null) result.agentType = agentType;
    if (conversation != null) result.conversation = conversation;
    if (e2e != null) result.e2e = e2e;
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

  @$pb.TagNumber(7)
  $1.WorkspaceInputMode get input => $_getN(3);
  @$pb.TagNumber(7)
  set input($1.WorkspaceInputMode value) => $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasInput() => $_has(3);
  @$pb.TagNumber(7)
  void clearInput() => $_clearField(7);
}

class PetWorkflowSpec extends $pb.GeneratedMessage {
  factory PetWorkflowSpec({
    $1.ReusableWorkflowDriver? driver,
    ToolkitPolicy? toolkit,
    FlowcraftWorkflowSpec? flowcraft,
    DoubaoRealtimeWorkflowSpec? doubaoRealtime,
    ASTTranslateWorkflowSpec? astTranslate,
    ChatRoomWorkflowSpec? chatroom,
  }) {
    final result = create();
    if (driver != null) result.driver = driver;
    if (toolkit != null) result.toolkit = toolkit;
    if (flowcraft != null) result.flowcraft = flowcraft;
    if (doubaoRealtime != null) result.doubaoRealtime = doubaoRealtime;
    if (astTranslate != null) result.astTranslate = astTranslate;
    if (chatroom != null) result.chatroom = chatroom;
    return result;
  }

  PetWorkflowSpec._();

  factory PetWorkflowSpec.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PetWorkflowSpec.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PetWorkflowSpec',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aE<$1.ReusableWorkflowDriver>(1, _omitFieldNames ? '' : 'driver',
        enumValues: $1.ReusableWorkflowDriver.values)
    ..aOM<ToolkitPolicy>(2, _omitFieldNames ? '' : 'toolkit',
        subBuilder: ToolkitPolicy.create)
    ..aOM<FlowcraftWorkflowSpec>(3, _omitFieldNames ? '' : 'flowcraft',
        subBuilder: FlowcraftWorkflowSpec.create)
    ..aOM<DoubaoRealtimeWorkflowSpec>(
        4, _omitFieldNames ? '' : 'doubaoRealtime',
        subBuilder: DoubaoRealtimeWorkflowSpec.create)
    ..aOM<ASTTranslateWorkflowSpec>(5, _omitFieldNames ? '' : 'astTranslate',
        subBuilder: ASTTranslateWorkflowSpec.create)
    ..aOM<ChatRoomWorkflowSpec>(6, _omitFieldNames ? '' : 'chatroom',
        subBuilder: ChatRoomWorkflowSpec.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetWorkflowSpec clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PetWorkflowSpec copyWith(void Function(PetWorkflowSpec) updates) =>
      super.copyWith((message) => updates(message as PetWorkflowSpec))
          as PetWorkflowSpec;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PetWorkflowSpec create() => PetWorkflowSpec._();
  @$core.override
  PetWorkflowSpec createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PetWorkflowSpec getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PetWorkflowSpec>(create);
  static PetWorkflowSpec? _defaultInstance;

  @$pb.TagNumber(1)
  $1.ReusableWorkflowDriver get driver => $_getN(0);
  @$pb.TagNumber(1)
  set driver($1.ReusableWorkflowDriver value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasDriver() => $_has(0);
  @$pb.TagNumber(1)
  void clearDriver() => $_clearField(1);

  @$pb.TagNumber(2)
  ToolkitPolicy get toolkit => $_getN(1);
  @$pb.TagNumber(2)
  set toolkit(ToolkitPolicy value) => $_setField(2, value);
  @$pb.TagNumber(2)
  $core.bool hasToolkit() => $_has(1);
  @$pb.TagNumber(2)
  void clearToolkit() => $_clearField(2);
  @$pb.TagNumber(2)
  ToolkitPolicy ensureToolkit() => $_ensure(1);

  @$pb.TagNumber(3)
  FlowcraftWorkflowSpec get flowcraft => $_getN(2);
  @$pb.TagNumber(3)
  set flowcraft(FlowcraftWorkflowSpec value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasFlowcraft() => $_has(2);
  @$pb.TagNumber(3)
  void clearFlowcraft() => $_clearField(3);
  @$pb.TagNumber(3)
  FlowcraftWorkflowSpec ensureFlowcraft() => $_ensure(2);

  @$pb.TagNumber(4)
  DoubaoRealtimeWorkflowSpec get doubaoRealtime => $_getN(3);
  @$pb.TagNumber(4)
  set doubaoRealtime(DoubaoRealtimeWorkflowSpec value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasDoubaoRealtime() => $_has(3);
  @$pb.TagNumber(4)
  void clearDoubaoRealtime() => $_clearField(4);
  @$pb.TagNumber(4)
  DoubaoRealtimeWorkflowSpec ensureDoubaoRealtime() => $_ensure(3);

  @$pb.TagNumber(5)
  ASTTranslateWorkflowSpec get astTranslate => $_getN(4);
  @$pb.TagNumber(5)
  set astTranslate(ASTTranslateWorkflowSpec value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasAstTranslate() => $_has(4);
  @$pb.TagNumber(5)
  void clearAstTranslate() => $_clearField(5);
  @$pb.TagNumber(5)
  ASTTranslateWorkflowSpec ensureAstTranslate() => $_ensure(4);

  @$pb.TagNumber(6)
  ChatRoomWorkflowSpec get chatroom => $_getN(5);
  @$pb.TagNumber(6)
  set chatroom(ChatRoomWorkflowSpec value) => $_setField(6, value);
  @$pb.TagNumber(6)
  $core.bool hasChatroom() => $_has(5);
  @$pb.TagNumber(6)
  void clearChatroom() => $_clearField(6);
  @$pb.TagNumber(6)
  ChatRoomWorkflowSpec ensureChatroom() => $_ensure(5);
}

enum Model_ProviderData {
  openaiTenant,
  geminiTenant,
  dashscopeTenant,
  volcTenant,
  minimaxTenant,
  deepseekTenant,
  notSet
}

class Model extends $pb.GeneratedMessage {
  factory Model({
    $core.String? alias,
    $core.Iterable<$core.MapEntry<$core.String, AliasI18nText>>? i18n,
    $1.ModelKind? kind,
    OpenAITenantModelProviderData? openaiTenant,
    GeminiTenantModelProviderData? geminiTenant,
    DashScopeTenantModelProviderData? dashscopeTenant,
    VolcTenantModelProviderData? volcTenant,
    MiniMaxTenantModelProviderData? minimaxTenant,
    DeepSeekTenantModelProviderData? deepseekTenant,
    ModelProviderKind? providerKind,
  }) {
    final result = create();
    if (alias != null) result.alias = alias;
    if (i18n != null) result.i18n.addEntries(i18n);
    if (kind != null) result.kind = kind;
    if (openaiTenant != null) result.openaiTenant = openaiTenant;
    if (geminiTenant != null) result.geminiTenant = geminiTenant;
    if (dashscopeTenant != null) result.dashscopeTenant = dashscopeTenant;
    if (volcTenant != null) result.volcTenant = volcTenant;
    if (minimaxTenant != null) result.minimaxTenant = minimaxTenant;
    if (deepseekTenant != null) result.deepseekTenant = deepseekTenant;
    if (providerKind != null) result.providerKind = providerKind;
    return result;
  }

  Model._();

  factory Model.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Model.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static const $core.Map<$core.int, Model_ProviderData>
      _Model_ProviderDataByTag = {
    5: Model_ProviderData.openaiTenant,
    6: Model_ProviderData.geminiTenant,
    7: Model_ProviderData.dashscopeTenant,
    8: Model_ProviderData.volcTenant,
    9: Model_ProviderData.minimaxTenant,
    10: Model_ProviderData.deepseekTenant,
    0: Model_ProviderData.notSet
  };
  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Model',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..oo(0, [5, 6, 7, 8, 9, 10])
    ..aOS(1, _omitFieldNames ? '' : 'alias')
    ..m<$core.String, AliasI18nText>(2, _omitFieldNames ? '' : 'i18n',
        entryClassName: 'Model.I18nEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OM,
        valueCreator: AliasI18nText.create,
        valueDefaultOrMaker: AliasI18nText.getDefault,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..aE<$1.ModelKind>(3, _omitFieldNames ? '' : 'kind',
        enumValues: $1.ModelKind.values)
    ..aOM<OpenAITenantModelProviderData>(
        5, _omitFieldNames ? '' : 'openaiTenant',
        subBuilder: OpenAITenantModelProviderData.create)
    ..aOM<GeminiTenantModelProviderData>(
        6, _omitFieldNames ? '' : 'geminiTenant',
        subBuilder: GeminiTenantModelProviderData.create)
    ..aOM<DashScopeTenantModelProviderData>(
        7, _omitFieldNames ? '' : 'dashscopeTenant',
        subBuilder: DashScopeTenantModelProviderData.create)
    ..aOM<VolcTenantModelProviderData>(8, _omitFieldNames ? '' : 'volcTenant',
        subBuilder: VolcTenantModelProviderData.create)
    ..aOM<MiniMaxTenantModelProviderData>(
        9, _omitFieldNames ? '' : 'minimaxTenant',
        subBuilder: MiniMaxTenantModelProviderData.create)
    ..aOM<DeepSeekTenantModelProviderData>(
        10, _omitFieldNames ? '' : 'deepseekTenant',
        subBuilder: DeepSeekTenantModelProviderData.create)
    ..aE<ModelProviderKind>(11, _omitFieldNames ? '' : 'providerKind',
        enumValues: ModelProviderKind.values)
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

  @$pb.TagNumber(5)
  @$pb.TagNumber(6)
  @$pb.TagNumber(7)
  @$pb.TagNumber(8)
  @$pb.TagNumber(9)
  @$pb.TagNumber(10)
  Model_ProviderData whichProviderData() =>
      _Model_ProviderDataByTag[$_whichOneof(0)]!;
  @$pb.TagNumber(5)
  @$pb.TagNumber(6)
  @$pb.TagNumber(7)
  @$pb.TagNumber(8)
  @$pb.TagNumber(9)
  @$pb.TagNumber(10)
  void clearProviderData() => $_clearField($_whichOneof(0));

  @$pb.TagNumber(1)
  $core.String get alias => $_getSZ(0);
  @$pb.TagNumber(1)
  set alias($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAlias() => $_has(0);
  @$pb.TagNumber(1)
  void clearAlias() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbMap<$core.String, AliasI18nText> get i18n => $_getMap(1);

  @$pb.TagNumber(3)
  $1.ModelKind get kind => $_getN(2);
  @$pb.TagNumber(3)
  set kind($1.ModelKind value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasKind() => $_has(2);
  @$pb.TagNumber(3)
  void clearKind() => $_clearField(3);

  @$pb.TagNumber(5)
  OpenAITenantModelProviderData get openaiTenant => $_getN(3);
  @$pb.TagNumber(5)
  set openaiTenant(OpenAITenantModelProviderData value) => $_setField(5, value);
  @$pb.TagNumber(5)
  $core.bool hasOpenaiTenant() => $_has(3);
  @$pb.TagNumber(5)
  void clearOpenaiTenant() => $_clearField(5);
  @$pb.TagNumber(5)
  OpenAITenantModelProviderData ensureOpenaiTenant() => $_ensure(3);

  @$pb.TagNumber(6)
  GeminiTenantModelProviderData get geminiTenant => $_getN(4);
  @$pb.TagNumber(6)
  set geminiTenant(GeminiTenantModelProviderData value) => $_setField(6, value);
  @$pb.TagNumber(6)
  $core.bool hasGeminiTenant() => $_has(4);
  @$pb.TagNumber(6)
  void clearGeminiTenant() => $_clearField(6);
  @$pb.TagNumber(6)
  GeminiTenantModelProviderData ensureGeminiTenant() => $_ensure(4);

  @$pb.TagNumber(7)
  DashScopeTenantModelProviderData get dashscopeTenant => $_getN(5);
  @$pb.TagNumber(7)
  set dashscopeTenant(DashScopeTenantModelProviderData value) =>
      $_setField(7, value);
  @$pb.TagNumber(7)
  $core.bool hasDashscopeTenant() => $_has(5);
  @$pb.TagNumber(7)
  void clearDashscopeTenant() => $_clearField(7);
  @$pb.TagNumber(7)
  DashScopeTenantModelProviderData ensureDashscopeTenant() => $_ensure(5);

  @$pb.TagNumber(8)
  VolcTenantModelProviderData get volcTenant => $_getN(6);
  @$pb.TagNumber(8)
  set volcTenant(VolcTenantModelProviderData value) => $_setField(8, value);
  @$pb.TagNumber(8)
  $core.bool hasVolcTenant() => $_has(6);
  @$pb.TagNumber(8)
  void clearVolcTenant() => $_clearField(8);
  @$pb.TagNumber(8)
  VolcTenantModelProviderData ensureVolcTenant() => $_ensure(6);

  @$pb.TagNumber(9)
  MiniMaxTenantModelProviderData get minimaxTenant => $_getN(7);
  @$pb.TagNumber(9)
  set minimaxTenant(MiniMaxTenantModelProviderData value) =>
      $_setField(9, value);
  @$pb.TagNumber(9)
  $core.bool hasMinimaxTenant() => $_has(7);
  @$pb.TagNumber(9)
  void clearMinimaxTenant() => $_clearField(9);
  @$pb.TagNumber(9)
  MiniMaxTenantModelProviderData ensureMinimaxTenant() => $_ensure(7);

  @$pb.TagNumber(10)
  DeepSeekTenantModelProviderData get deepseekTenant => $_getN(8);
  @$pb.TagNumber(10)
  set deepseekTenant(DeepSeekTenantModelProviderData value) =>
      $_setField(10, value);
  @$pb.TagNumber(10)
  $core.bool hasDeepseekTenant() => $_has(8);
  @$pb.TagNumber(10)
  void clearDeepseekTenant() => $_clearField(10);
  @$pb.TagNumber(10)
  DeepSeekTenantModelProviderData ensureDeepseekTenant() => $_ensure(8);

  @$pb.TagNumber(11)
  ModelProviderKind get providerKind => $_getN(9);
  @$pb.TagNumber(11)
  set providerKind(ModelProviderKind value) => $_setField(11, value);
  @$pb.TagNumber(11)
  $core.bool hasProviderKind() => $_has(9);
  @$pb.TagNumber(11)
  void clearProviderKind() => $_clearField(11);
}

class OpenAITenantModelProviderData extends $pb.GeneratedMessage {
  factory OpenAITenantModelProviderData({
    $core.String? upstreamModel,
    $core.bool? supportJsonOutput,
    $core.bool? supportToolCalls,
    $core.bool? supportTextOnly,
    $core.bool? supportTemperature,
    $core.bool? supportThinking,
    $core.bool? useSystemRole,
    $core.String? thinkingParam,
    $core.String? thinkingLevelParam,
    $core.Iterable<$core.String>? thinkingLevels,
    $core.String? defaultThinkingLevel,
  }) {
    final result = create();
    if (upstreamModel != null) result.upstreamModel = upstreamModel;
    if (supportJsonOutput != null) result.supportJsonOutput = supportJsonOutput;
    if (supportToolCalls != null) result.supportToolCalls = supportToolCalls;
    if (supportTextOnly != null) result.supportTextOnly = supportTextOnly;
    if (supportTemperature != null)
      result.supportTemperature = supportTemperature;
    if (supportThinking != null) result.supportThinking = supportThinking;
    if (useSystemRole != null) result.useSystemRole = useSystemRole;
    if (thinkingParam != null) result.thinkingParam = thinkingParam;
    if (thinkingLevelParam != null)
      result.thinkingLevelParam = thinkingLevelParam;
    if (thinkingLevels != null) result.thinkingLevels.addAll(thinkingLevels);
    if (defaultThinkingLevel != null)
      result.defaultThinkingLevel = defaultThinkingLevel;
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
    ..aOS(1, _omitFieldNames ? '' : 'upstreamModel')
    ..aOB(2, _omitFieldNames ? '' : 'supportJsonOutput')
    ..aOB(3, _omitFieldNames ? '' : 'supportToolCalls')
    ..aOB(4, _omitFieldNames ? '' : 'supportTextOnly')
    ..aOB(5, _omitFieldNames ? '' : 'supportTemperature')
    ..aOB(6, _omitFieldNames ? '' : 'supportThinking')
    ..aOB(7, _omitFieldNames ? '' : 'useSystemRole')
    ..aOS(8, _omitFieldNames ? '' : 'thinkingParam')
    ..aOS(9, _omitFieldNames ? '' : 'thinkingLevelParam')
    ..pPS(10, _omitFieldNames ? '' : 'thinkingLevels')
    ..aOS(11, _omitFieldNames ? '' : 'defaultThinkingLevel')
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
  $core.String get upstreamModel => $_getSZ(0);
  @$pb.TagNumber(1)
  set upstreamModel($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasUpstreamModel() => $_has(0);
  @$pb.TagNumber(1)
  void clearUpstreamModel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.bool get supportJsonOutput => $_getBF(1);
  @$pb.TagNumber(2)
  set supportJsonOutput($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSupportJsonOutput() => $_has(1);
  @$pb.TagNumber(2)
  void clearSupportJsonOutput() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get supportToolCalls => $_getBF(2);
  @$pb.TagNumber(3)
  set supportToolCalls($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSupportToolCalls() => $_has(2);
  @$pb.TagNumber(3)
  void clearSupportToolCalls() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get supportTextOnly => $_getBF(3);
  @$pb.TagNumber(4)
  set supportTextOnly($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSupportTextOnly() => $_has(3);
  @$pb.TagNumber(4)
  void clearSupportTextOnly() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get supportTemperature => $_getBF(4);
  @$pb.TagNumber(5)
  set supportTemperature($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSupportTemperature() => $_has(4);
  @$pb.TagNumber(5)
  void clearSupportTemperature() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.bool get supportThinking => $_getBF(5);
  @$pb.TagNumber(6)
  set supportThinking($core.bool value) => $_setBool(5, value);
  @$pb.TagNumber(6)
  $core.bool hasSupportThinking() => $_has(5);
  @$pb.TagNumber(6)
  void clearSupportThinking() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get useSystemRole => $_getBF(6);
  @$pb.TagNumber(7)
  set useSystemRole($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasUseSystemRole() => $_has(6);
  @$pb.TagNumber(7)
  void clearUseSystemRole() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get thinkingParam => $_getSZ(7);
  @$pb.TagNumber(8)
  set thinkingParam($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasThinkingParam() => $_has(7);
  @$pb.TagNumber(8)
  void clearThinkingParam() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get thinkingLevelParam => $_getSZ(8);
  @$pb.TagNumber(9)
  set thinkingLevelParam($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasThinkingLevelParam() => $_has(8);
  @$pb.TagNumber(9)
  void clearThinkingLevelParam() => $_clearField(9);

  @$pb.TagNumber(10)
  $pb.PbList<$core.String> get thinkingLevels => $_getList(9);

  @$pb.TagNumber(11)
  $core.String get defaultThinkingLevel => $_getSZ(10);
  @$pb.TagNumber(11)
  set defaultThinkingLevel($core.String value) => $_setString(10, value);
  @$pb.TagNumber(11)
  $core.bool hasDefaultThinkingLevel() => $_has(10);
  @$pb.TagNumber(11)
  void clearDefaultThinkingLevel() => $_clearField(11);
}

class GeminiTenantModelProviderData extends $pb.GeneratedMessage {
  factory GeminiTenantModelProviderData({
    $core.String? upstreamModel,
    $core.bool? supportJsonOutput,
    $core.bool? supportToolCalls,
    $core.bool? supportTextOnly,
    $core.bool? supportTemperature,
    $core.bool? supportThinking,
    $core.bool? useSystemRole,
    $core.String? thinkingParam,
    $core.String? thinkingLevelParam,
    $core.Iterable<$core.String>? thinkingLevels,
    $core.String? defaultThinkingLevel,
  }) {
    final result = create();
    if (upstreamModel != null) result.upstreamModel = upstreamModel;
    if (supportJsonOutput != null) result.supportJsonOutput = supportJsonOutput;
    if (supportToolCalls != null) result.supportToolCalls = supportToolCalls;
    if (supportTextOnly != null) result.supportTextOnly = supportTextOnly;
    if (supportTemperature != null)
      result.supportTemperature = supportTemperature;
    if (supportThinking != null) result.supportThinking = supportThinking;
    if (useSystemRole != null) result.useSystemRole = useSystemRole;
    if (thinkingParam != null) result.thinkingParam = thinkingParam;
    if (thinkingLevelParam != null)
      result.thinkingLevelParam = thinkingLevelParam;
    if (thinkingLevels != null) result.thinkingLevels.addAll(thinkingLevels);
    if (defaultThinkingLevel != null)
      result.defaultThinkingLevel = defaultThinkingLevel;
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
    ..aOB(2, _omitFieldNames ? '' : 'supportJsonOutput')
    ..aOB(3, _omitFieldNames ? '' : 'supportToolCalls')
    ..aOB(4, _omitFieldNames ? '' : 'supportTextOnly')
    ..aOB(5, _omitFieldNames ? '' : 'supportTemperature')
    ..aOB(6, _omitFieldNames ? '' : 'supportThinking')
    ..aOB(7, _omitFieldNames ? '' : 'useSystemRole')
    ..aOS(8, _omitFieldNames ? '' : 'thinkingParam')
    ..aOS(9, _omitFieldNames ? '' : 'thinkingLevelParam')
    ..pPS(10, _omitFieldNames ? '' : 'thinkingLevels')
    ..aOS(11, _omitFieldNames ? '' : 'defaultThinkingLevel')
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

  @$pb.TagNumber(2)
  $core.bool get supportJsonOutput => $_getBF(1);
  @$pb.TagNumber(2)
  set supportJsonOutput($core.bool value) => $_setBool(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSupportJsonOutput() => $_has(1);
  @$pb.TagNumber(2)
  void clearSupportJsonOutput() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get supportToolCalls => $_getBF(2);
  @$pb.TagNumber(3)
  set supportToolCalls($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSupportToolCalls() => $_has(2);
  @$pb.TagNumber(3)
  void clearSupportToolCalls() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get supportTextOnly => $_getBF(3);
  @$pb.TagNumber(4)
  set supportTextOnly($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSupportTextOnly() => $_has(3);
  @$pb.TagNumber(4)
  void clearSupportTextOnly() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get supportTemperature => $_getBF(4);
  @$pb.TagNumber(5)
  set supportTemperature($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSupportTemperature() => $_has(4);
  @$pb.TagNumber(5)
  void clearSupportTemperature() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.bool get supportThinking => $_getBF(5);
  @$pb.TagNumber(6)
  set supportThinking($core.bool value) => $_setBool(5, value);
  @$pb.TagNumber(6)
  $core.bool hasSupportThinking() => $_has(5);
  @$pb.TagNumber(6)
  void clearSupportThinking() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get useSystemRole => $_getBF(6);
  @$pb.TagNumber(7)
  set useSystemRole($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasUseSystemRole() => $_has(6);
  @$pb.TagNumber(7)
  void clearUseSystemRole() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get thinkingParam => $_getSZ(7);
  @$pb.TagNumber(8)
  set thinkingParam($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasThinkingParam() => $_has(7);
  @$pb.TagNumber(8)
  void clearThinkingParam() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get thinkingLevelParam => $_getSZ(8);
  @$pb.TagNumber(9)
  set thinkingLevelParam($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasThinkingLevelParam() => $_has(8);
  @$pb.TagNumber(9)
  void clearThinkingLevelParam() => $_clearField(9);

  @$pb.TagNumber(10)
  $pb.PbList<$core.String> get thinkingLevels => $_getList(9);

  @$pb.TagNumber(11)
  $core.String get defaultThinkingLevel => $_getSZ(10);
  @$pb.TagNumber(11)
  set defaultThinkingLevel($core.String value) => $_setString(10, value);
  @$pb.TagNumber(11)
  $core.bool hasDefaultThinkingLevel() => $_has(10);
  @$pb.TagNumber(11)
  void clearDefaultThinkingLevel() => $_clearField(11);
}

class DashScopeTenantModelProviderData extends $pb.GeneratedMessage {
  factory DashScopeTenantModelProviderData({
    $core.String? upstreamModel,
    $core.String? apiMode,
    $core.bool? supportJsonOutput,
    $core.bool? supportToolCalls,
    $core.bool? supportTextOnly,
    $core.bool? supportTemperature,
    $core.bool? supportThinking,
    $core.bool? useSystemRole,
    $core.String? thinkingParam,
    $core.String? thinkingLevelParam,
    $core.Iterable<$core.String>? thinkingLevels,
    $core.String? defaultThinkingLevel,
  }) {
    final result = create();
    if (upstreamModel != null) result.upstreamModel = upstreamModel;
    if (apiMode != null) result.apiMode = apiMode;
    if (supportJsonOutput != null) result.supportJsonOutput = supportJsonOutput;
    if (supportToolCalls != null) result.supportToolCalls = supportToolCalls;
    if (supportTextOnly != null) result.supportTextOnly = supportTextOnly;
    if (supportTemperature != null)
      result.supportTemperature = supportTemperature;
    if (supportThinking != null) result.supportThinking = supportThinking;
    if (useSystemRole != null) result.useSystemRole = useSystemRole;
    if (thinkingParam != null) result.thinkingParam = thinkingParam;
    if (thinkingLevelParam != null)
      result.thinkingLevelParam = thinkingLevelParam;
    if (thinkingLevels != null) result.thinkingLevels.addAll(thinkingLevels);
    if (defaultThinkingLevel != null)
      result.defaultThinkingLevel = defaultThinkingLevel;
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
    ..aOS(1, _omitFieldNames ? '' : 'upstreamModel')
    ..aOS(2, _omitFieldNames ? '' : 'apiMode')
    ..aOB(3, _omitFieldNames ? '' : 'supportJsonOutput')
    ..aOB(4, _omitFieldNames ? '' : 'supportToolCalls')
    ..aOB(5, _omitFieldNames ? '' : 'supportTextOnly')
    ..aOB(6, _omitFieldNames ? '' : 'supportTemperature')
    ..aOB(7, _omitFieldNames ? '' : 'supportThinking')
    ..aOB(8, _omitFieldNames ? '' : 'useSystemRole')
    ..aOS(9, _omitFieldNames ? '' : 'thinkingParam')
    ..aOS(10, _omitFieldNames ? '' : 'thinkingLevelParam')
    ..pPS(11, _omitFieldNames ? '' : 'thinkingLevels')
    ..aOS(12, _omitFieldNames ? '' : 'defaultThinkingLevel')
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
  $core.String get upstreamModel => $_getSZ(0);
  @$pb.TagNumber(1)
  set upstreamModel($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasUpstreamModel() => $_has(0);
  @$pb.TagNumber(1)
  void clearUpstreamModel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get apiMode => $_getSZ(1);
  @$pb.TagNumber(2)
  set apiMode($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasApiMode() => $_has(1);
  @$pb.TagNumber(2)
  void clearApiMode() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get supportJsonOutput => $_getBF(2);
  @$pb.TagNumber(3)
  set supportJsonOutput($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSupportJsonOutput() => $_has(2);
  @$pb.TagNumber(3)
  void clearSupportJsonOutput() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get supportToolCalls => $_getBF(3);
  @$pb.TagNumber(4)
  set supportToolCalls($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSupportToolCalls() => $_has(3);
  @$pb.TagNumber(4)
  void clearSupportToolCalls() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get supportTextOnly => $_getBF(4);
  @$pb.TagNumber(5)
  set supportTextOnly($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSupportTextOnly() => $_has(4);
  @$pb.TagNumber(5)
  void clearSupportTextOnly() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.bool get supportTemperature => $_getBF(5);
  @$pb.TagNumber(6)
  set supportTemperature($core.bool value) => $_setBool(5, value);
  @$pb.TagNumber(6)
  $core.bool hasSupportTemperature() => $_has(5);
  @$pb.TagNumber(6)
  void clearSupportTemperature() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get supportThinking => $_getBF(6);
  @$pb.TagNumber(7)
  set supportThinking($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasSupportThinking() => $_has(6);
  @$pb.TagNumber(7)
  void clearSupportThinking() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.bool get useSystemRole => $_getBF(7);
  @$pb.TagNumber(8)
  set useSystemRole($core.bool value) => $_setBool(7, value);
  @$pb.TagNumber(8)
  $core.bool hasUseSystemRole() => $_has(7);
  @$pb.TagNumber(8)
  void clearUseSystemRole() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get thinkingParam => $_getSZ(8);
  @$pb.TagNumber(9)
  set thinkingParam($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasThinkingParam() => $_has(8);
  @$pb.TagNumber(9)
  void clearThinkingParam() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get thinkingLevelParam => $_getSZ(9);
  @$pb.TagNumber(10)
  set thinkingLevelParam($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasThinkingLevelParam() => $_has(9);
  @$pb.TagNumber(10)
  void clearThinkingLevelParam() => $_clearField(10);

  @$pb.TagNumber(11)
  $pb.PbList<$core.String> get thinkingLevels => $_getList(10);

  @$pb.TagNumber(12)
  $core.String get defaultThinkingLevel => $_getSZ(11);
  @$pb.TagNumber(12)
  set defaultThinkingLevel($core.String value) => $_setString(11, value);
  @$pb.TagNumber(12)
  $core.bool hasDefaultThinkingLevel() => $_has(11);
  @$pb.TagNumber(12)
  void clearDefaultThinkingLevel() => $_clearField(12);
}

class VolcTenantModelProviderData extends $pb.GeneratedMessage {
  factory VolcTenantModelProviderData({
    $core.String? upstreamModel,
    $core.String? resourceId,
    $core.String? apiMode,
    $core.bool? supportJsonOutput,
    $core.bool? supportToolCalls,
    $core.bool? supportTextOnly,
    $core.bool? supportTemperature,
    $core.bool? supportThinking,
    $core.bool? useSystemRole,
    $core.String? thinkingParam,
    $core.String? thinkingLevelParam,
    $core.Iterable<$core.String>? thinkingLevels,
    $core.String? defaultThinkingLevel,
  }) {
    final result = create();
    if (upstreamModel != null) result.upstreamModel = upstreamModel;
    if (resourceId != null) result.resourceId = resourceId;
    if (apiMode != null) result.apiMode = apiMode;
    if (supportJsonOutput != null) result.supportJsonOutput = supportJsonOutput;
    if (supportToolCalls != null) result.supportToolCalls = supportToolCalls;
    if (supportTextOnly != null) result.supportTextOnly = supportTextOnly;
    if (supportTemperature != null)
      result.supportTemperature = supportTemperature;
    if (supportThinking != null) result.supportThinking = supportThinking;
    if (useSystemRole != null) result.useSystemRole = useSystemRole;
    if (thinkingParam != null) result.thinkingParam = thinkingParam;
    if (thinkingLevelParam != null)
      result.thinkingLevelParam = thinkingLevelParam;
    if (thinkingLevels != null) result.thinkingLevels.addAll(thinkingLevels);
    if (defaultThinkingLevel != null)
      result.defaultThinkingLevel = defaultThinkingLevel;
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
    ..aOS(1, _omitFieldNames ? '' : 'upstreamModel')
    ..aOS(2, _omitFieldNames ? '' : 'resourceId')
    ..aOS(3, _omitFieldNames ? '' : 'apiMode')
    ..aOB(4, _omitFieldNames ? '' : 'supportJsonOutput')
    ..aOB(5, _omitFieldNames ? '' : 'supportToolCalls')
    ..aOB(6, _omitFieldNames ? '' : 'supportTextOnly')
    ..aOB(7, _omitFieldNames ? '' : 'supportTemperature')
    ..aOB(8, _omitFieldNames ? '' : 'supportThinking')
    ..aOB(9, _omitFieldNames ? '' : 'useSystemRole')
    ..aOS(10, _omitFieldNames ? '' : 'thinkingParam')
    ..aOS(11, _omitFieldNames ? '' : 'thinkingLevelParam')
    ..pPS(12, _omitFieldNames ? '' : 'thinkingLevels')
    ..aOS(13, _omitFieldNames ? '' : 'defaultThinkingLevel')
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
  $core.String get upstreamModel => $_getSZ(0);
  @$pb.TagNumber(1)
  set upstreamModel($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasUpstreamModel() => $_has(0);
  @$pb.TagNumber(1)
  void clearUpstreamModel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get resourceId => $_getSZ(1);
  @$pb.TagNumber(2)
  set resourceId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasResourceId() => $_has(1);
  @$pb.TagNumber(2)
  void clearResourceId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get apiMode => $_getSZ(2);
  @$pb.TagNumber(3)
  set apiMode($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasApiMode() => $_has(2);
  @$pb.TagNumber(3)
  void clearApiMode() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get supportJsonOutput => $_getBF(3);
  @$pb.TagNumber(4)
  set supportJsonOutput($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSupportJsonOutput() => $_has(3);
  @$pb.TagNumber(4)
  void clearSupportJsonOutput() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get supportToolCalls => $_getBF(4);
  @$pb.TagNumber(5)
  set supportToolCalls($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSupportToolCalls() => $_has(4);
  @$pb.TagNumber(5)
  void clearSupportToolCalls() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.bool get supportTextOnly => $_getBF(5);
  @$pb.TagNumber(6)
  set supportTextOnly($core.bool value) => $_setBool(5, value);
  @$pb.TagNumber(6)
  $core.bool hasSupportTextOnly() => $_has(5);
  @$pb.TagNumber(6)
  void clearSupportTextOnly() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get supportTemperature => $_getBF(6);
  @$pb.TagNumber(7)
  set supportTemperature($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasSupportTemperature() => $_has(6);
  @$pb.TagNumber(7)
  void clearSupportTemperature() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.bool get supportThinking => $_getBF(7);
  @$pb.TagNumber(8)
  set supportThinking($core.bool value) => $_setBool(7, value);
  @$pb.TagNumber(8)
  $core.bool hasSupportThinking() => $_has(7);
  @$pb.TagNumber(8)
  void clearSupportThinking() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.bool get useSystemRole => $_getBF(8);
  @$pb.TagNumber(9)
  set useSystemRole($core.bool value) => $_setBool(8, value);
  @$pb.TagNumber(9)
  $core.bool hasUseSystemRole() => $_has(8);
  @$pb.TagNumber(9)
  void clearUseSystemRole() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get thinkingParam => $_getSZ(9);
  @$pb.TagNumber(10)
  set thinkingParam($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasThinkingParam() => $_has(9);
  @$pb.TagNumber(10)
  void clearThinkingParam() => $_clearField(10);

  @$pb.TagNumber(11)
  $core.String get thinkingLevelParam => $_getSZ(10);
  @$pb.TagNumber(11)
  set thinkingLevelParam($core.String value) => $_setString(10, value);
  @$pb.TagNumber(11)
  $core.bool hasThinkingLevelParam() => $_has(10);
  @$pb.TagNumber(11)
  void clearThinkingLevelParam() => $_clearField(11);

  @$pb.TagNumber(12)
  $pb.PbList<$core.String> get thinkingLevels => $_getList(11);

  @$pb.TagNumber(13)
  $core.String get defaultThinkingLevel => $_getSZ(12);
  @$pb.TagNumber(13)
  set defaultThinkingLevel($core.String value) => $_setString(12, value);
  @$pb.TagNumber(13)
  $core.bool hasDefaultThinkingLevel() => $_has(12);
  @$pb.TagNumber(13)
  void clearDefaultThinkingLevel() => $_clearField(13);
}

class MiniMaxTenantModelProviderData extends $pb.GeneratedMessage {
  factory MiniMaxTenantModelProviderData({
    $core.String? upstreamModel,
    $core.String? apiMode,
    $core.bool? supportJsonOutput,
    $core.bool? supportToolCalls,
    $core.bool? supportTextOnly,
    $core.bool? supportTemperature,
    $core.bool? supportThinking,
    $core.bool? useSystemRole,
    $core.String? thinkingParam,
    $core.String? thinkingLevelParam,
    $core.Iterable<$core.String>? thinkingLevels,
    $core.String? defaultThinkingLevel,
  }) {
    final result = create();
    if (upstreamModel != null) result.upstreamModel = upstreamModel;
    if (apiMode != null) result.apiMode = apiMode;
    if (supportJsonOutput != null) result.supportJsonOutput = supportJsonOutput;
    if (supportToolCalls != null) result.supportToolCalls = supportToolCalls;
    if (supportTextOnly != null) result.supportTextOnly = supportTextOnly;
    if (supportTemperature != null)
      result.supportTemperature = supportTemperature;
    if (supportThinking != null) result.supportThinking = supportThinking;
    if (useSystemRole != null) result.useSystemRole = useSystemRole;
    if (thinkingParam != null) result.thinkingParam = thinkingParam;
    if (thinkingLevelParam != null)
      result.thinkingLevelParam = thinkingLevelParam;
    if (thinkingLevels != null) result.thinkingLevels.addAll(thinkingLevels);
    if (defaultThinkingLevel != null)
      result.defaultThinkingLevel = defaultThinkingLevel;
    return result;
  }

  MiniMaxTenantModelProviderData._();

  factory MiniMaxTenantModelProviderData.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory MiniMaxTenantModelProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'MiniMaxTenantModelProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'upstreamModel')
    ..aOS(2, _omitFieldNames ? '' : 'apiMode')
    ..aOB(3, _omitFieldNames ? '' : 'supportJsonOutput')
    ..aOB(4, _omitFieldNames ? '' : 'supportToolCalls')
    ..aOB(5, _omitFieldNames ? '' : 'supportTextOnly')
    ..aOB(6, _omitFieldNames ? '' : 'supportTemperature')
    ..aOB(7, _omitFieldNames ? '' : 'supportThinking')
    ..aOB(8, _omitFieldNames ? '' : 'useSystemRole')
    ..aOS(9, _omitFieldNames ? '' : 'thinkingParam')
    ..aOS(10, _omitFieldNames ? '' : 'thinkingLevelParam')
    ..pPS(11, _omitFieldNames ? '' : 'thinkingLevels')
    ..aOS(12, _omitFieldNames ? '' : 'defaultThinkingLevel')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MiniMaxTenantModelProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  MiniMaxTenantModelProviderData copyWith(
          void Function(MiniMaxTenantModelProviderData) updates) =>
      super.copyWith(
              (message) => updates(message as MiniMaxTenantModelProviderData))
          as MiniMaxTenantModelProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static MiniMaxTenantModelProviderData create() =>
      MiniMaxTenantModelProviderData._();
  @$core.override
  MiniMaxTenantModelProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static MiniMaxTenantModelProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<MiniMaxTenantModelProviderData>(create);
  static MiniMaxTenantModelProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get upstreamModel => $_getSZ(0);
  @$pb.TagNumber(1)
  set upstreamModel($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasUpstreamModel() => $_has(0);
  @$pb.TagNumber(1)
  void clearUpstreamModel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get apiMode => $_getSZ(1);
  @$pb.TagNumber(2)
  set apiMode($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasApiMode() => $_has(1);
  @$pb.TagNumber(2)
  void clearApiMode() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get supportJsonOutput => $_getBF(2);
  @$pb.TagNumber(3)
  set supportJsonOutput($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSupportJsonOutput() => $_has(2);
  @$pb.TagNumber(3)
  void clearSupportJsonOutput() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get supportToolCalls => $_getBF(3);
  @$pb.TagNumber(4)
  set supportToolCalls($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSupportToolCalls() => $_has(3);
  @$pb.TagNumber(4)
  void clearSupportToolCalls() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get supportTextOnly => $_getBF(4);
  @$pb.TagNumber(5)
  set supportTextOnly($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSupportTextOnly() => $_has(4);
  @$pb.TagNumber(5)
  void clearSupportTextOnly() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.bool get supportTemperature => $_getBF(5);
  @$pb.TagNumber(6)
  set supportTemperature($core.bool value) => $_setBool(5, value);
  @$pb.TagNumber(6)
  $core.bool hasSupportTemperature() => $_has(5);
  @$pb.TagNumber(6)
  void clearSupportTemperature() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get supportThinking => $_getBF(6);
  @$pb.TagNumber(7)
  set supportThinking($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasSupportThinking() => $_has(6);
  @$pb.TagNumber(7)
  void clearSupportThinking() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.bool get useSystemRole => $_getBF(7);
  @$pb.TagNumber(8)
  set useSystemRole($core.bool value) => $_setBool(7, value);
  @$pb.TagNumber(8)
  $core.bool hasUseSystemRole() => $_has(7);
  @$pb.TagNumber(8)
  void clearUseSystemRole() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get thinkingParam => $_getSZ(8);
  @$pb.TagNumber(9)
  set thinkingParam($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasThinkingParam() => $_has(8);
  @$pb.TagNumber(9)
  void clearThinkingParam() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get thinkingLevelParam => $_getSZ(9);
  @$pb.TagNumber(10)
  set thinkingLevelParam($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasThinkingLevelParam() => $_has(9);
  @$pb.TagNumber(10)
  void clearThinkingLevelParam() => $_clearField(10);

  @$pb.TagNumber(11)
  $pb.PbList<$core.String> get thinkingLevels => $_getList(10);

  @$pb.TagNumber(12)
  $core.String get defaultThinkingLevel => $_getSZ(11);
  @$pb.TagNumber(12)
  set defaultThinkingLevel($core.String value) => $_setString(11, value);
  @$pb.TagNumber(12)
  $core.bool hasDefaultThinkingLevel() => $_has(11);
  @$pb.TagNumber(12)
  void clearDefaultThinkingLevel() => $_clearField(12);
}

class DeepSeekTenantModelProviderData extends $pb.GeneratedMessage {
  factory DeepSeekTenantModelProviderData({
    $core.String? upstreamModel,
    $core.String? apiMode,
    $core.bool? supportJsonOutput,
    $core.bool? supportToolCalls,
    $core.bool? supportTextOnly,
    $core.bool? supportTemperature,
    $core.bool? supportThinking,
    $core.bool? useSystemRole,
    $core.String? thinkingParam,
    $core.String? thinkingLevelParam,
    $core.Iterable<$core.String>? thinkingLevels,
    $core.String? defaultThinkingLevel,
  }) {
    final result = create();
    if (upstreamModel != null) result.upstreamModel = upstreamModel;
    if (apiMode != null) result.apiMode = apiMode;
    if (supportJsonOutput != null) result.supportJsonOutput = supportJsonOutput;
    if (supportToolCalls != null) result.supportToolCalls = supportToolCalls;
    if (supportTextOnly != null) result.supportTextOnly = supportTextOnly;
    if (supportTemperature != null)
      result.supportTemperature = supportTemperature;
    if (supportThinking != null) result.supportThinking = supportThinking;
    if (useSystemRole != null) result.useSystemRole = useSystemRole;
    if (thinkingParam != null) result.thinkingParam = thinkingParam;
    if (thinkingLevelParam != null)
      result.thinkingLevelParam = thinkingLevelParam;
    if (thinkingLevels != null) result.thinkingLevels.addAll(thinkingLevels);
    if (defaultThinkingLevel != null)
      result.defaultThinkingLevel = defaultThinkingLevel;
    return result;
  }

  DeepSeekTenantModelProviderData._();

  factory DeepSeekTenantModelProviderData.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory DeepSeekTenantModelProviderData.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'DeepSeekTenantModelProviderData',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'upstreamModel')
    ..aOS(2, _omitFieldNames ? '' : 'apiMode')
    ..aOB(3, _omitFieldNames ? '' : 'supportJsonOutput')
    ..aOB(4, _omitFieldNames ? '' : 'supportToolCalls')
    ..aOB(5, _omitFieldNames ? '' : 'supportTextOnly')
    ..aOB(6, _omitFieldNames ? '' : 'supportTemperature')
    ..aOB(7, _omitFieldNames ? '' : 'supportThinking')
    ..aOB(8, _omitFieldNames ? '' : 'useSystemRole')
    ..aOS(9, _omitFieldNames ? '' : 'thinkingParam')
    ..aOS(10, _omitFieldNames ? '' : 'thinkingLevelParam')
    ..pPS(11, _omitFieldNames ? '' : 'thinkingLevels')
    ..aOS(12, _omitFieldNames ? '' : 'defaultThinkingLevel')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeepSeekTenantModelProviderData clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  DeepSeekTenantModelProviderData copyWith(
          void Function(DeepSeekTenantModelProviderData) updates) =>
      super.copyWith(
              (message) => updates(message as DeepSeekTenantModelProviderData))
          as DeepSeekTenantModelProviderData;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static DeepSeekTenantModelProviderData create() =>
      DeepSeekTenantModelProviderData._();
  @$core.override
  DeepSeekTenantModelProviderData createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static DeepSeekTenantModelProviderData getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<DeepSeekTenantModelProviderData>(
          create);
  static DeepSeekTenantModelProviderData? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get upstreamModel => $_getSZ(0);
  @$pb.TagNumber(1)
  set upstreamModel($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasUpstreamModel() => $_has(0);
  @$pb.TagNumber(1)
  void clearUpstreamModel() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get apiMode => $_getSZ(1);
  @$pb.TagNumber(2)
  set apiMode($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasApiMode() => $_has(1);
  @$pb.TagNumber(2)
  void clearApiMode() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.bool get supportJsonOutput => $_getBF(2);
  @$pb.TagNumber(3)
  set supportJsonOutput($core.bool value) => $_setBool(2, value);
  @$pb.TagNumber(3)
  $core.bool hasSupportJsonOutput() => $_has(2);
  @$pb.TagNumber(3)
  void clearSupportJsonOutput() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get supportToolCalls => $_getBF(3);
  @$pb.TagNumber(4)
  set supportToolCalls($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSupportToolCalls() => $_has(3);
  @$pb.TagNumber(4)
  void clearSupportToolCalls() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.bool get supportTextOnly => $_getBF(4);
  @$pb.TagNumber(5)
  set supportTextOnly($core.bool value) => $_setBool(4, value);
  @$pb.TagNumber(5)
  $core.bool hasSupportTextOnly() => $_has(4);
  @$pb.TagNumber(5)
  void clearSupportTextOnly() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.bool get supportTemperature => $_getBF(5);
  @$pb.TagNumber(6)
  set supportTemperature($core.bool value) => $_setBool(5, value);
  @$pb.TagNumber(6)
  $core.bool hasSupportTemperature() => $_has(5);
  @$pb.TagNumber(6)
  void clearSupportTemperature() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.bool get supportThinking => $_getBF(6);
  @$pb.TagNumber(7)
  set supportThinking($core.bool value) => $_setBool(6, value);
  @$pb.TagNumber(7)
  $core.bool hasSupportThinking() => $_has(6);
  @$pb.TagNumber(7)
  void clearSupportThinking() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.bool get useSystemRole => $_getBF(7);
  @$pb.TagNumber(8)
  set useSystemRole($core.bool value) => $_setBool(7, value);
  @$pb.TagNumber(8)
  $core.bool hasUseSystemRole() => $_has(7);
  @$pb.TagNumber(8)
  void clearUseSystemRole() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get thinkingParam => $_getSZ(8);
  @$pb.TagNumber(9)
  set thinkingParam($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasThinkingParam() => $_has(8);
  @$pb.TagNumber(9)
  void clearThinkingParam() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get thinkingLevelParam => $_getSZ(9);
  @$pb.TagNumber(10)
  set thinkingLevelParam($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasThinkingLevelParam() => $_has(9);
  @$pb.TagNumber(10)
  void clearThinkingLevelParam() => $_clearField(10);

  @$pb.TagNumber(11)
  $pb.PbList<$core.String> get thinkingLevels => $_getList(10);

  @$pb.TagNumber(12)
  $core.String get defaultThinkingLevel => $_getSZ(11);
  @$pb.TagNumber(12)
  set defaultThinkingLevel($core.String value) => $_setString(11, value);
  @$pb.TagNumber(12)
  $core.bool hasDefaultThinkingLevel() => $_has(11);
  @$pb.TagNumber(12)
  void clearDefaultThinkingLevel() => $_clearField(12);
}

class ModelGetRequest extends $pb.GeneratedMessage {
  factory ModelGetRequest({
    $core.String? alias,
  }) {
    final result = create();
    if (alias != null) result.alias = alias;
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
    ..aOS(1, _omitFieldNames ? '' : 'alias')
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
  $core.String get alias => $_getSZ(0);
  @$pb.TagNumber(1)
  set alias($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAlias() => $_has(0);
  @$pb.TagNumber(1)
  void clearAlias() => $_clearField(1);
}

class ModelGetResponse extends $pb.GeneratedMessage {
  factory ModelGetResponse({
    Model? value,
    $core.String? runtimeProfileName,
    $core.String? runtimeProfileRevision,
  }) {
    final result = create();
    if (value != null) result.value = value;
    if (runtimeProfileName != null)
      result.runtimeProfileName = runtimeProfileName;
    if (runtimeProfileRevision != null)
      result.runtimeProfileRevision = runtimeProfileRevision;
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
    ..aOS(2, _omitFieldNames ? '' : 'runtimeProfileName')
    ..aOS(3, _omitFieldNames ? '' : 'runtimeProfileRevision')
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

  @$pb.TagNumber(2)
  $core.String get runtimeProfileName => $_getSZ(1);
  @$pb.TagNumber(2)
  set runtimeProfileName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasRuntimeProfileName() => $_has(1);
  @$pb.TagNumber(2)
  void clearRuntimeProfileName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get runtimeProfileRevision => $_getSZ(2);
  @$pb.TagNumber(3)
  set runtimeProfileRevision($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasRuntimeProfileRevision() => $_has(2);
  @$pb.TagNumber(3)
  void clearRuntimeProfileRevision() => $_clearField(3);
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
    $core.String? runtimeProfileName,
    $core.String? runtimeProfileRevision,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    if (runtimeProfileName != null)
      result.runtimeProfileName = runtimeProfileName;
    if (runtimeProfileRevision != null)
      result.runtimeProfileRevision = runtimeProfileRevision;
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
    ..aOS(4, _omitFieldNames ? '' : 'runtimeProfileName')
    ..aOS(5, _omitFieldNames ? '' : 'runtimeProfileRevision')
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

  @$pb.TagNumber(4)
  $core.String get runtimeProfileName => $_getSZ(3);
  @$pb.TagNumber(4)
  set runtimeProfileName($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasRuntimeProfileName() => $_has(3);
  @$pb.TagNumber(4)
  void clearRuntimeProfileName() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get runtimeProfileRevision => $_getSZ(4);
  @$pb.TagNumber(5)
  set runtimeProfileRevision($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasRuntimeProfileRevision() => $_has(4);
  @$pb.TagNumber(5)
  void clearRuntimeProfileRevision() => $_clearField(5);
}

class Voice extends $pb.GeneratedMessage {
  factory Voice({
    $core.String? alias,
    $core.Iterable<$core.MapEntry<$core.String, AliasI18nText>>? i18n,
  }) {
    final result = create();
    if (alias != null) result.alias = alias;
    if (i18n != null) result.i18n.addEntries(i18n);
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
    ..aOS(1, _omitFieldNames ? '' : 'alias')
    ..m<$core.String, AliasI18nText>(2, _omitFieldNames ? '' : 'i18n',
        entryClassName: 'Voice.I18nEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OM,
        valueCreator: AliasI18nText.create,
        valueDefaultOrMaker: AliasI18nText.getDefault,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
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
  $core.String get alias => $_getSZ(0);
  @$pb.TagNumber(1)
  set alias($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAlias() => $_has(0);
  @$pb.TagNumber(1)
  void clearAlias() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbMap<$core.String, AliasI18nText> get i18n => $_getMap(1);
}

class VoiceGetRequest extends $pb.GeneratedMessage {
  factory VoiceGetRequest({
    $core.String? alias,
  }) {
    final result = create();
    if (alias != null) result.alias = alias;
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
    ..aOS(1, _omitFieldNames ? '' : 'alias')
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
  $core.String get alias => $_getSZ(0);
  @$pb.TagNumber(1)
  set alias($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAlias() => $_has(0);
  @$pb.TagNumber(1)
  void clearAlias() => $_clearField(1);
}

class VoiceGetResponse extends $pb.GeneratedMessage {
  factory VoiceGetResponse({
    Voice? value,
    $core.String? runtimeProfileName,
    $core.String? runtimeProfileRevision,
  }) {
    final result = create();
    if (value != null) result.value = value;
    if (runtimeProfileName != null)
      result.runtimeProfileName = runtimeProfileName;
    if (runtimeProfileRevision != null)
      result.runtimeProfileRevision = runtimeProfileRevision;
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
    ..aOS(2, _omitFieldNames ? '' : 'runtimeProfileName')
    ..aOS(3, _omitFieldNames ? '' : 'runtimeProfileRevision')
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

  @$pb.TagNumber(2)
  $core.String get runtimeProfileName => $_getSZ(1);
  @$pb.TagNumber(2)
  set runtimeProfileName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasRuntimeProfileName() => $_has(1);
  @$pb.TagNumber(2)
  void clearRuntimeProfileName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get runtimeProfileRevision => $_getSZ(2);
  @$pb.TagNumber(3)
  set runtimeProfileRevision($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasRuntimeProfileRevision() => $_has(2);
  @$pb.TagNumber(3)
  void clearRuntimeProfileRevision() => $_clearField(3);
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
    $core.String? runtimeProfileName,
    $core.String? runtimeProfileRevision,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    if (runtimeProfileName != null)
      result.runtimeProfileName = runtimeProfileName;
    if (runtimeProfileRevision != null)
      result.runtimeProfileRevision = runtimeProfileRevision;
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
    ..aOS(4, _omitFieldNames ? '' : 'runtimeProfileName')
    ..aOS(5, _omitFieldNames ? '' : 'runtimeProfileRevision')
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

  @$pb.TagNumber(4)
  $core.String get runtimeProfileName => $_getSZ(3);
  @$pb.TagNumber(4)
  set runtimeProfileName($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasRuntimeProfileName() => $_has(3);
  @$pb.TagNumber(4)
  void clearRuntimeProfileName() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get runtimeProfileRevision => $_getSZ(4);
  @$pb.TagNumber(5)
  set runtimeProfileRevision($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasRuntimeProfileRevision() => $_has(4);
  @$pb.TagNumber(5)
  void clearRuntimeProfileRevision() => $_clearField(5);
}

class Workflow extends $pb.GeneratedMessage {
  factory Workflow({
    $core.String? alias,
    $core.Iterable<$core.MapEntry<$core.String, AliasI18nText>>? i18n,
    $core.String? collection,
    $1.WorkflowDriver? driver,
    $core.String? workspaceLangPair,
  }) {
    final result = create();
    if (alias != null) result.alias = alias;
    if (i18n != null) result.i18n.addEntries(i18n);
    if (collection != null) result.collection = collection;
    if (driver != null) result.driver = driver;
    if (workspaceLangPair != null) result.workspaceLangPair = workspaceLangPair;
    return result;
  }

  Workflow._();

  factory Workflow.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Workflow.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Workflow',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'gizclaw.rpc.v1'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'alias')
    ..m<$core.String, AliasI18nText>(2, _omitFieldNames ? '' : 'i18n',
        entryClassName: 'Workflow.I18nEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OM,
        valueCreator: AliasI18nText.create,
        valueDefaultOrMaker: AliasI18nText.getDefault,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..aOS(3, _omitFieldNames ? '' : 'collection')
    ..aE<$1.WorkflowDriver>(4, _omitFieldNames ? '' : 'driver',
        enumValues: $1.WorkflowDriver.values)
    ..aOS(5, _omitFieldNames ? '' : 'workspaceLangPair')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Workflow clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Workflow copyWith(void Function(Workflow) updates) =>
      super.copyWith((message) => updates(message as Workflow)) as Workflow;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Workflow create() => Workflow._();
  @$core.override
  Workflow createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Workflow getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Workflow>(create);
  static Workflow? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get alias => $_getSZ(0);
  @$pb.TagNumber(1)
  set alias($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAlias() => $_has(0);
  @$pb.TagNumber(1)
  void clearAlias() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbMap<$core.String, AliasI18nText> get i18n => $_getMap(1);

  @$pb.TagNumber(3)
  $core.String get collection => $_getSZ(2);
  @$pb.TagNumber(3)
  set collection($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasCollection() => $_has(2);
  @$pb.TagNumber(3)
  void clearCollection() => $_clearField(3);

  @$pb.TagNumber(4)
  $1.WorkflowDriver get driver => $_getN(3);
  @$pb.TagNumber(4)
  set driver($1.WorkflowDriver value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasDriver() => $_has(3);
  @$pb.TagNumber(4)
  void clearDriver() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get workspaceLangPair => $_getSZ(4);
  @$pb.TagNumber(5)
  set workspaceLangPair($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasWorkspaceLangPair() => $_has(4);
  @$pb.TagNumber(5)
  void clearWorkspaceLangPair() => $_clearField(5);
}

class WorkflowGetRequest extends $pb.GeneratedMessage {
  factory WorkflowGetRequest({
    $core.String? alias,
  }) {
    final result = create();
    if (alias != null) result.alias = alias;
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
    ..aOS(1, _omitFieldNames ? '' : 'alias')
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
  $core.String get alias => $_getSZ(0);
  @$pb.TagNumber(1)
  set alias($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAlias() => $_has(0);
  @$pb.TagNumber(1)
  void clearAlias() => $_clearField(1);
}

class WorkflowGetResponse extends $pb.GeneratedMessage {
  factory WorkflowGetResponse({
    Workflow? value,
    $core.String? runtimeProfileName,
    $core.String? runtimeProfileRevision,
  }) {
    final result = create();
    if (value != null) result.value = value;
    if (runtimeProfileName != null)
      result.runtimeProfileName = runtimeProfileName;
    if (runtimeProfileRevision != null)
      result.runtimeProfileRevision = runtimeProfileRevision;
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
    ..aOM<Workflow>(1, _omitFieldNames ? '' : 'value',
        subBuilder: Workflow.create)
    ..aOS(2, _omitFieldNames ? '' : 'runtimeProfileName')
    ..aOS(3, _omitFieldNames ? '' : 'runtimeProfileRevision')
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
  Workflow get value => $_getN(0);
  @$pb.TagNumber(1)
  set value(Workflow value) => $_setField(1, value);
  @$pb.TagNumber(1)
  $core.bool hasValue() => $_has(0);
  @$pb.TagNumber(1)
  void clearValue() => $_clearField(1);
  @$pb.TagNumber(1)
  Workflow ensureValue() => $_ensure(0);

  @$pb.TagNumber(2)
  $core.String get runtimeProfileName => $_getSZ(1);
  @$pb.TagNumber(2)
  set runtimeProfileName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasRuntimeProfileName() => $_has(1);
  @$pb.TagNumber(2)
  void clearRuntimeProfileName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get runtimeProfileRevision => $_getSZ(2);
  @$pb.TagNumber(3)
  set runtimeProfileRevision($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasRuntimeProfileRevision() => $_has(2);
  @$pb.TagNumber(3)
  void clearRuntimeProfileRevision() => $_clearField(3);
}

class WorkflowListRequest extends $pb.GeneratedMessage {
  factory WorkflowListRequest({
    $core.String? cursor,
    $fixnum.Int64? limit,
    $core.String? collection,
  }) {
    final result = create();
    if (cursor != null) result.cursor = cursor;
    if (limit != null) result.limit = limit;
    if (collection != null) result.collection = collection;
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
    ..aOS(3, _omitFieldNames ? '' : 'collection')
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

  @$pb.TagNumber(3)
  $core.String get collection => $_getSZ(2);
  @$pb.TagNumber(3)
  set collection($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasCollection() => $_has(2);
  @$pb.TagNumber(3)
  void clearCollection() => $_clearField(3);
}

class WorkflowListResponse extends $pb.GeneratedMessage {
  factory WorkflowListResponse({
    $core.bool? hasNext,
    $core.Iterable<Workflow>? items,
    $core.String? nextCursor,
    $core.String? runtimeProfileName,
    $core.String? runtimeProfileRevision,
  }) {
    final result = create();
    if (hasNext != null) result.hasNext = hasNext;
    if (items != null) result.items.addAll(items);
    if (nextCursor != null) result.nextCursor = nextCursor;
    if (runtimeProfileName != null)
      result.runtimeProfileName = runtimeProfileName;
    if (runtimeProfileRevision != null)
      result.runtimeProfileRevision = runtimeProfileRevision;
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
    ..pPM<Workflow>(2, _omitFieldNames ? '' : 'items',
        subBuilder: Workflow.create)
    ..aOS(3, _omitFieldNames ? '' : 'nextCursor')
    ..aOS(4, _omitFieldNames ? '' : 'runtimeProfileName')
    ..aOS(5, _omitFieldNames ? '' : 'runtimeProfileRevision')
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
  $pb.PbList<Workflow> get items => $_getList(1);

  @$pb.TagNumber(3)
  $core.String get nextCursor => $_getSZ(2);
  @$pb.TagNumber(3)
  set nextCursor($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasNextCursor() => $_has(2);
  @$pb.TagNumber(3)
  void clearNextCursor() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get runtimeProfileName => $_getSZ(3);
  @$pb.TagNumber(4)
  set runtimeProfileName($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasRuntimeProfileName() => $_has(3);
  @$pb.TagNumber(4)
  void clearRuntimeProfileName() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get runtimeProfileRevision => $_getSZ(4);
  @$pb.TagNumber(5)
  set runtimeProfileRevision($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasRuntimeProfileRevision() => $_has(4);
  @$pb.TagNumber(5)
  void clearRuntimeProfileRevision() => $_clearField(5);
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

class Tool extends $pb.GeneratedMessage {
  factory Tool({
    $core.String? alias,
    $core.Iterable<$core.MapEntry<$core.String, AliasI18nText>>? i18n,
    $0.Struct? inputSchema,
    $0.Struct? outputSchema,
  }) {
    final result = create();
    if (alias != null) result.alias = alias;
    if (i18n != null) result.i18n.addEntries(i18n);
    if (inputSchema != null) result.inputSchema = inputSchema;
    if (outputSchema != null) result.outputSchema = outputSchema;
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
    ..aOS(1, _omitFieldNames ? '' : 'alias')
    ..m<$core.String, AliasI18nText>(2, _omitFieldNames ? '' : 'i18n',
        entryClassName: 'Tool.I18nEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.OM,
        valueCreator: AliasI18nText.create,
        valueDefaultOrMaker: AliasI18nText.getDefault,
        packageName: const $pb.PackageName('gizclaw.rpc.v1'))
    ..aOM<$0.Struct>(3, _omitFieldNames ? '' : 'inputSchema',
        subBuilder: $0.Struct.create)
    ..aOM<$0.Struct>(4, _omitFieldNames ? '' : 'outputSchema',
        subBuilder: $0.Struct.create)
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
  $core.String get alias => $_getSZ(0);
  @$pb.TagNumber(1)
  set alias($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAlias() => $_has(0);
  @$pb.TagNumber(1)
  void clearAlias() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbMap<$core.String, AliasI18nText> get i18n => $_getMap(1);

  @$pb.TagNumber(3)
  $0.Struct get inputSchema => $_getN(2);
  @$pb.TagNumber(3)
  set inputSchema($0.Struct value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasInputSchema() => $_has(2);
  @$pb.TagNumber(3)
  void clearInputSchema() => $_clearField(3);
  @$pb.TagNumber(3)
  $0.Struct ensureInputSchema() => $_ensure(2);

  @$pb.TagNumber(4)
  $0.Struct get outputSchema => $_getN(3);
  @$pb.TagNumber(4)
  set outputSchema($0.Struct value) => $_setField(4, value);
  @$pb.TagNumber(4)
  $core.bool hasOutputSchema() => $_has(3);
  @$pb.TagNumber(4)
  void clearOutputSchema() => $_clearField(4);
  @$pb.TagNumber(4)
  $0.Struct ensureOutputSchema() => $_ensure(3);
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
    $core.String? runtimeProfileName,
    $core.String? runtimeProfileRevision,
  }) {
    final result = create();
    if (items != null) result.items.addAll(items);
    if (hasNext != null) result.hasNext = hasNext;
    if (nextCursor != null) result.nextCursor = nextCursor;
    if (runtimeProfileName != null)
      result.runtimeProfileName = runtimeProfileName;
    if (runtimeProfileRevision != null)
      result.runtimeProfileRevision = runtimeProfileRevision;
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
    ..aOS(4, _omitFieldNames ? '' : 'runtimeProfileName')
    ..aOS(5, _omitFieldNames ? '' : 'runtimeProfileRevision')
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

  @$pb.TagNumber(4)
  $core.String get runtimeProfileName => $_getSZ(3);
  @$pb.TagNumber(4)
  set runtimeProfileName($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasRuntimeProfileName() => $_has(3);
  @$pb.TagNumber(4)
  void clearRuntimeProfileName() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get runtimeProfileRevision => $_getSZ(4);
  @$pb.TagNumber(5)
  set runtimeProfileRevision($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasRuntimeProfileRevision() => $_has(4);
  @$pb.TagNumber(5)
  void clearRuntimeProfileRevision() => $_clearField(5);
}

class ToolGetRequest extends $pb.GeneratedMessage {
  factory ToolGetRequest({
    $core.String? alias,
  }) {
    final result = create();
    if (alias != null) result.alias = alias;
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
    ..aOS(1, _omitFieldNames ? '' : 'alias')
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
  $core.String get alias => $_getSZ(0);
  @$pb.TagNumber(1)
  set alias($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAlias() => $_has(0);
  @$pb.TagNumber(1)
  void clearAlias() => $_clearField(1);
}

class ToolGetResponse extends $pb.GeneratedMessage {
  factory ToolGetResponse({
    Tool? value,
    $core.String? runtimeProfileName,
    $core.String? runtimeProfileRevision,
  }) {
    final result = create();
    if (value != null) result.value = value;
    if (runtimeProfileName != null)
      result.runtimeProfileName = runtimeProfileName;
    if (runtimeProfileRevision != null)
      result.runtimeProfileRevision = runtimeProfileRevision;
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
    ..aOS(2, _omitFieldNames ? '' : 'runtimeProfileName')
    ..aOS(3, _omitFieldNames ? '' : 'runtimeProfileRevision')
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

  @$pb.TagNumber(2)
  $core.String get runtimeProfileName => $_getSZ(1);
  @$pb.TagNumber(2)
  set runtimeProfileName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasRuntimeProfileName() => $_has(1);
  @$pb.TagNumber(2)
  void clearRuntimeProfileName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get runtimeProfileRevision => $_getSZ(2);
  @$pb.TagNumber(3)
  set runtimeProfileRevision($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasRuntimeProfileRevision() => $_has(2);
  @$pb.TagNumber(3)
  void clearRuntimeProfileRevision() => $_clearField(3);
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
