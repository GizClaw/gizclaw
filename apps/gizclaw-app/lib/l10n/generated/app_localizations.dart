import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/intl.dart' as intl;

import 'app_localizations_en.dart';
import 'app_localizations_zh.dart';

// ignore_for_file: type=lint

/// Callers can lookup localized strings with an instance of AppLocalizations
/// returned by `AppLocalizations.of(context)`.
///
/// Applications need to include `AppLocalizations.delegate()` in their app's
/// `localizationDelegates` list, and the locales they support in the app's
/// `supportedLocales` list. For example:
///
/// ```dart
/// import 'generated/app_localizations.dart';
///
/// return MaterialApp(
///   localizationsDelegates: AppLocalizations.localizationsDelegates,
///   supportedLocales: AppLocalizations.supportedLocales,
///   home: MyApplicationHome(),
/// );
/// ```
///
/// ## Update pubspec.yaml
///
/// Please make sure to update your pubspec.yaml to include the following
/// packages:
///
/// ```yaml
/// dependencies:
///   # Internationalization support.
///   flutter_localizations:
///     sdk: flutter
///   intl: any # Use the pinned version from flutter_localizations
///
///   # Rest of dependencies
/// ```
///
/// ## iOS Applications
///
/// iOS applications define key application metadata, including supported
/// locales, in an Info.plist file that is built into the application bundle.
/// To configure the locales supported by your app, you’ll need to edit this
/// file.
///
/// First, open your project’s ios/Runner.xcworkspace Xcode workspace file.
/// Then, in the Project Navigator, open the Info.plist file under the Runner
/// project’s Runner folder.
///
/// Next, select the Information Property List item, select Add Item from the
/// Editor menu, then select Localizations from the pop-up menu.
///
/// Select and expand the newly-created Localizations item then, for each
/// locale your application supports, add a new item and select the locale
/// you wish to add from the pop-up menu in the Value field. This list should
/// be consistent with the languages listed in the AppLocalizations.supportedLocales
/// property.
abstract class AppLocalizations {
  AppLocalizations(String locale)
    : localeName = intl.Intl.canonicalizedLocale(locale.toString());

  final String localeName;

  static AppLocalizations of(BuildContext context) {
    return Localizations.of<AppLocalizations>(context, AppLocalizations)!;
  }

  static const LocalizationsDelegate<AppLocalizations> delegate =
      _AppLocalizationsDelegate();

  /// A list of this localizations delegate along with the default localizations
  /// delegates.
  ///
  /// Returns a list of localizations delegates containing this delegate along with
  /// GlobalMaterialLocalizations.delegate, GlobalCupertinoLocalizations.delegate,
  /// and GlobalWidgetsLocalizations.delegate.
  ///
  /// Additional delegates can be added by appending to this list in
  /// MaterialApp. This list does not have to be used at all if a custom list
  /// of delegates is preferred or required.
  static const List<LocalizationsDelegate<dynamic>> localizationsDelegates =
      <LocalizationsDelegate<dynamic>>[
        delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
      ];

  /// A list of this localizations delegate's supported locales.
  static const List<Locale> supportedLocales = <Locale>[
    Locale('en'),
    Locale('zh'),
    Locale('zh', 'CN'),
  ];

  /// Application title.
  ///
  /// In en, this message translates to:
  /// **'GizClaw OpenSource'**
  String get appTitle;

  /// Unified Workspace destination title.
  ///
  /// In en, this message translates to:
  /// **'Workspaces'**
  String get workspacesTitle;

  /// Unified Workspace creation action and sheet title.
  ///
  /// In en, this message translates to:
  /// **'New Workspace'**
  String get newWorkspace;

  /// Empty state for the unified Workspace list.
  ///
  /// In en, this message translates to:
  /// **'No workspaces yet.'**
  String get noWorkspaces;

  /// Fixed Doubao realtime Workflow title.
  ///
  /// In en, this message translates to:
  /// **'Doubao Realtime'**
  String get workflowDoubaoRealtimeTitle;

  /// Fixed Doubao realtime Workflow subtitle.
  ///
  /// In en, this message translates to:
  /// **'Live voice conversation'**
  String get workflowDoubaoRealtimeSubtitle;

  /// Fixed Chinese and English translation Workflow title.
  ///
  /// In en, this message translates to:
  /// **'Chinese ↔ English'**
  String get workflowTranslateZhEnTitle;

  /// Fixed Chinese to Japanese translation Workflow title.
  ///
  /// In en, this message translates to:
  /// **'Chinese → Japanese'**
  String get workflowTranslateZhJaTitle;

  /// Fixed Chinese to Korean translation Workflow title.
  ///
  /// In en, this message translates to:
  /// **'Chinese → Korean'**
  String get workflowTranslateZhKoTitle;

  /// Fixed Chinese to Spanish translation Workflow title.
  ///
  /// In en, this message translates to:
  /// **'Chinese → Spanish'**
  String get workflowTranslateZhEsTitle;

  /// Fixed translation Workflow subtitle.
  ///
  /// In en, this message translates to:
  /// **'Realtime speech translation'**
  String get workflowTranslationSubtitle;

  /// Fixed chat Raid Workflow title.
  ///
  /// In en, this message translates to:
  /// **'Chat'**
  String get workflowChatTitle;

  /// Fixed chat Raid Workflow subtitle.
  ///
  /// In en, this message translates to:
  /// **'General AI assistant'**
  String get workflowChatSubtitle;

  /// Fixed journey Raid Workflow title.
  ///
  /// In en, this message translates to:
  /// **'Journey'**
  String get workflowJourneyTitle;

  /// Fixed journey Raid Workflow subtitle.
  ///
  /// In en, this message translates to:
  /// **'Interactive story guide'**
  String get workflowJourneySubtitle;

  /// Fixed murder mystery Raid Workflow title.
  ///
  /// In en, this message translates to:
  /// **'Murder Mystery'**
  String get workflowMurderMysteryTitle;

  /// Fixed murder mystery Raid Workflow subtitle.
  ///
  /// In en, this message translates to:
  /// **'Interactive mystery game'**
  String get workflowMurderMysterySubtitle;

  /// Fixed internal chatroom Workflow title.
  ///
  /// In en, this message translates to:
  /// **'Chatroom'**
  String get workflowChatroomTitle;

  /// Fixed internal chatroom Workflow subtitle.
  ///
  /// In en, this message translates to:
  /// **'Friend and group conversation'**
  String get workflowChatroomSubtitle;

  /// Language setting title.
  ///
  /// In en, this message translates to:
  /// **'Language'**
  String get language;

  /// Language selector page title.
  ///
  /// In en, this message translates to:
  /// **'Choose Language'**
  String get chooseLanguage;

  /// System language preference option.
  ///
  /// In en, this message translates to:
  /// **'System (Default)'**
  String get languageSystemDefault;

  /// English language preference option.
  ///
  /// In en, this message translates to:
  /// **'English'**
  String get languageEnglish;

  /// Simplified Chinese language preference option.
  ///
  /// In en, this message translates to:
  /// **'简体中文'**
  String get languageSimplifiedChinese;

  /// Language preference write failure title.
  ///
  /// In en, this message translates to:
  /// **'Language Not Saved'**
  String get languageSaveFailedTitle;

  /// Language preference write failure message.
  ///
  /// In en, this message translates to:
  /// **'The language changed for this session, but the preference could not be saved.'**
  String get languageSaveFailedMessage;

  /// App settings section heading.
  ///
  /// In en, this message translates to:
  /// **'APP'**
  String get appSettings;

  /// Back navigation label.
  ///
  /// In en, this message translates to:
  /// **'Back'**
  String get commonBack;

  /// Cancel action label.
  ///
  /// In en, this message translates to:
  /// **'Cancel'**
  String get commonCancel;

  /// Close action label.
  ///
  /// In en, this message translates to:
  /// **'Close'**
  String get commonClose;

  /// Confirmation action label.
  ///
  /// In en, this message translates to:
  /// **'OK'**
  String get commonOk;

  /// Retry action label.
  ///
  /// In en, this message translates to:
  /// **'Retry'**
  String get commonRetry;

  /// Title for iOS LAN connection recovery guidance.
  ///
  /// In en, this message translates to:
  /// **'Local network connection unavailable'**
  String get localNetworkRecoveryTitle;

  /// Recovery guidance for an iOS connection failure to a LAN Server.
  ///
  /// In en, this message translates to:
  /// **'In iOS Settings, allow GizClaw to access the Local Network. Make sure this phone and the Server are on the same reachable Wi-Fi, then retry. VPN, guest Wi-Fi, or access-point isolation can also block the connection.'**
  String get localNetworkRecoveryMessage;

  /// Workspace model-picker loading state.
  ///
  /// In en, this message translates to:
  /// **'Loading available models…'**
  String get workspaceModelLoading;

  /// FlowCraft generation model field label.
  ///
  /// In en, this message translates to:
  /// **'Generation model'**
  String get generationModel;

  /// FlowCraft memory extraction model field label.
  ///
  /// In en, this message translates to:
  /// **'Extraction model'**
  String get extractionModel;

  /// FlowCraft embedding model field label.
  ///
  /// In en, this message translates to:
  /// **'Embedding model'**
  String get embeddingModel;

  /// Workspace model-picker action.
  ///
  /// In en, this message translates to:
  /// **'Choose a model'**
  String get chooseModel;

  /// Error shown when FlowCraft workspace creation has no visible LLM models.
  ///
  /// In en, this message translates to:
  /// **'No accessible generation models are available. Ask a Server administrator to configure and share an LLM model.'**
  String get noCompatibleGeneratorModels;

  /// Title shown when microphone permission is denied.
  ///
  /// In en, this message translates to:
  /// **'Microphone permission required'**
  String get microphonePermissionDeniedTitle;

  /// Recovery guidance for denied microphone permission.
  ///
  /// In en, this message translates to:
  /// **'Allow microphone access in Settings, then tap the red microphone to retry.'**
  String get microphonePermissionDeniedMessage;

  /// Title shown when microphone capture is unavailable.
  ///
  /// In en, this message translates to:
  /// **'Microphone unavailable'**
  String get microphoneUnavailableTitle;

  /// Recovery guidance for unavailable microphone capture.
  ///
  /// In en, this message translates to:
  /// **'GizClaw could not start the microphone. Check the audio input and tap the red microphone to retry.'**
  String get microphoneUnavailableMessage;

  /// Conversation control status when microphone capture is unavailable.
  ///
  /// In en, this message translates to:
  /// **'MICROPHONE UNAVAILABLE'**
  String get microphoneUnavailableStatus;

  /// Conversation control status when microphone permission is denied.
  ///
  /// In en, this message translates to:
  /// **'MICROPHONE PERMISSION REQUIRED'**
  String get microphonePermissionRequiredStatus;

  /// Conversation control status while microphone capture is being recovered.
  ///
  /// In en, this message translates to:
  /// **'RECOVERING MICROPHONE'**
  String get microphoneRecovering;

  /// Accessibility label for retrying denied microphone permission.
  ///
  /// In en, this message translates to:
  /// **'Microphone unavailable: permission required. Double tap to retry'**
  String get microphonePermissionRetrySemantics;

  /// Accessibility label for retrying unavailable microphone capture.
  ///
  /// In en, this message translates to:
  /// **'Microphone unavailable: capture failed. Double tap to retry'**
  String get microphoneCaptureRetrySemantics;

  /// Accessibility label for push-to-talk.
  ///
  /// In en, this message translates to:
  /// **'Hold to talk'**
  String get voiceHoldToTalkSemantics;

  /// Accessibility label for starting realtime voice input.
  ///
  /// In en, this message translates to:
  /// **'Start realtime call'**
  String get voiceStartRealtimeSemantics;

  /// Accessibility label for ending realtime voice input.
  ///
  /// In en, this message translates to:
  /// **'End realtime call'**
  String get voiceEndRealtimeSemantics;

  /// Conversation connection status.
  ///
  /// In en, this message translates to:
  /// **'CONNECTING'**
  String get conversationStatusConnecting;

  /// Conversation status when no workspace is active.
  ///
  /// In en, this message translates to:
  /// **'NO ACTIVE CONVERSATION'**
  String get conversationStatusNoActive;

  /// Conversation status for active realtime voice input.
  ///
  /// In en, this message translates to:
  /// **'REALTIME LIVE'**
  String get conversationStatusRealtimeLive;

  /// Conversation status for active push-to-talk input.
  ///
  /// In en, this message translates to:
  /// **'LISTENING'**
  String get conversationStatusListening;

  /// Conversation status while assistant audio is playing.
  ///
  /// In en, this message translates to:
  /// **'SPEAKING'**
  String get conversationStatusSpeaking;

  /// Conversation status when realtime voice input is ready.
  ///
  /// In en, this message translates to:
  /// **'REALTIME READY'**
  String get conversationStatusRealtimeReady;

  /// Conversation status when push-to-talk is ready.
  ///
  /// In en, this message translates to:
  /// **'HOLD TO TALK'**
  String get conversationStatusHoldToTalk;

  /// Button that returns a conversation to its newest message.
  ///
  /// In en, this message translates to:
  /// **'New messages below'**
  String get newMessagesBelow;

  /// Onboarding hero heading.
  ///
  /// In en, this message translates to:
  /// **'Your agents, everywhere.'**
  String get onboardingHeroTitle;

  /// Onboarding hero description.
  ///
  /// In en, this message translates to:
  /// **'Connect to a GizClaw server to unlock voice, workflows, and companions across your devices.'**
  String get onboardingHeroDescription;

  /// Onboarding primary action.
  ///
  /// In en, this message translates to:
  /// **'Get Started by Connecting a Server'**
  String get onboardingConnectServer;

  /// Onboarding server connection hint.
  ///
  /// In en, this message translates to:
  /// **'Enter a server access point or scan a QR code.'**
  String get onboardingConnectHint;

  /// Accessibility label for opening an onboarding story.
  ///
  /// In en, this message translates to:
  /// **'Read {title}'**
  String onboardingReadStory({required String title});

  /// Onboarding story action label.
  ///
  /// In en, this message translates to:
  /// **'READ STORY'**
  String get onboardingReadStoryAction;

  /// Agent onboarding card title.
  ///
  /// In en, this message translates to:
  /// **'Agents that feel close'**
  String get onboardingAgentsTitle;

  /// Agent onboarding card description.
  ///
  /// In en, this message translates to:
  /// **'Talk naturally with always-ready companions for planning, ideas, and everyday help.'**
  String get onboardingAgentsDescription;

  /// Agent onboarding article eyebrow.
  ///
  /// In en, this message translates to:
  /// **'ALWAYS READY'**
  String get onboardingAgentsEyebrow;

  /// Agent onboarding article title.
  ///
  /// In en, this message translates to:
  /// **'Built around your day'**
  String get onboardingAgentsArticleTitle;

  /// Agent onboarding article body.
  ///
  /// In en, this message translates to:
  /// **'Keep a personal agent close for quick questions, planning, and the small decisions that keep your day moving.'**
  String get onboardingAgentsArticleBody;

  /// Agent onboarding article highlight.
  ///
  /// In en, this message translates to:
  /// **'Your conversations stay connected through the GizClaw server you choose.'**
  String get onboardingAgentsArticleHighlight;

  /// Workflow onboarding card title.
  ///
  /// In en, this message translates to:
  /// **'Workflows that move with you'**
  String get onboardingWorkflowsTitle;

  /// Workflow onboarding card description.
  ///
  /// In en, this message translates to:
  /// **'Turn reusable workflows into structured work you can run from any connected device.'**
  String get onboardingWorkflowsDescription;

  /// Workflow onboarding article eyebrow.
  ///
  /// In en, this message translates to:
  /// **'REUSABLE WORK'**
  String get onboardingWorkflowsEyebrow;

  /// Workflow onboarding article title.
  ///
  /// In en, this message translates to:
  /// **'Make great work repeatable'**
  String get onboardingWorkflowsArticleTitle;

  /// Workflow onboarding article body.
  ///
  /// In en, this message translates to:
  /// **'Build a workflow once, then launch the same structured process whenever you need it—from your phone or another connected device.'**
  String get onboardingWorkflowsArticleBody;

  /// Workflow onboarding article highlight.
  ///
  /// In en, this message translates to:
  /// **'Carry the process between devices without rebuilding it every time.'**
  String get onboardingWorkflowsArticleHighlight;

  /// Realtime onboarding card title.
  ///
  /// In en, this message translates to:
  /// **'Realtime by design'**
  String get onboardingRealtimeTitle;

  /// Realtime onboarding card description.
  ///
  /// In en, this message translates to:
  /// **'Run low-latency voice sessions while your server keeps every device in the loop.'**
  String get onboardingRealtimeDescription;

  /// Realtime onboarding article eyebrow.
  ///
  /// In en, this message translates to:
  /// **'LOW LATENCY'**
  String get onboardingRealtimeEyebrow;

  /// Realtime onboarding article title.
  ///
  /// In en, this message translates to:
  /// **'Voice that keeps up'**
  String get onboardingRealtimeArticleTitle;

  /// Realtime onboarding article body.
  ///
  /// In en, this message translates to:
  /// **'Start a natural voice session and let GizClaw coordinate the realtime experience across your connected devices.'**
  String get onboardingRealtimeArticleBody;

  /// Realtime onboarding article highlight.
  ///
  /// In en, this message translates to:
  /// **'Fast responses, one server, and every connected device in the loop.'**
  String get onboardingRealtimeArticleHighlight;

  /// Accessibility label for adding a server.
  ///
  /// In en, this message translates to:
  /// **'Add server'**
  String get addServerA11y;

  /// Shared fixed application UI labels.
  ///
  /// In en, this message translates to:
  /// **'{key, select, notFound {Not found} pageUnavailable {This page is unavailable.} servers {Servers} addServer {Add Server} chooseServerSetup {Choose a server to finish setup and continue.} switchServerFailed {Could not switch servers. Please try again.} addServerDescription {Add a server by entering its details or scanning a GizClaw QR code.} scanQr {Scan QR Code} serverDetails {SERVER DETAILS} name {Name} scanServer {Scan Server} pointCamera {Point the camera at a GizClaw server QR code.} cameraRequired {Camera access is required to scan a server QR code. Enable it in Settings and try again.} cameraFailed {The camera could not start. Go back and try again.} serverNameRequired {Enter a server name.} serverAccessPointRequired {Enter a server access point.} serverAccessPointInvalid {Use a domain or IP address with a port, for example gizclaw.local:9820.} serverAccessPointDuplicate {This access point is already in the list.} serverAddFailed {Could not add the server. Please try again.} identity {Identity} scanServerQr {Scan server QR code} thisDevice {This device} deviceIdentityReady {Device identity ready} client {CLIENT} publicIdentity {Public identity} generatedOnDevice {Generated on this device} privateKey {Private key} protectedSecureStorage {Protected by device secure storage} connection {CONNECTION} chooseServer {Choose a server} server {Server} transport {Transport} connected {Connected} connecting {Connecting} offline {Offline} setup {Setup} home {Home} translate {Translate} friends {Friends} groups {Groups} pets {Pets} raids {Raids} other {{key}}}'**
  String uiText({required String key});

  /// Shared actions, field labels, and short status text.
  ///
  /// In en, this message translates to:
  /// **'{key, select, unavailable {Unavailable} unableActivate {Unable to activate} unableSwitchMode {Unable to switch mode} actionFailed {The action could not be completed. Please try again.} pushToTalk {Push to talk} realtime {Realtime} addFriend {Add Friend} remove {Remove} createGroup {Create Group} createGroupA11y {Create group} groupName {Group name} optionalDescription {Description (optional)} connect {Connect} myInvite {My Invite} inviteToken {Invite token} revoke {Revoke} friendUnavailable {Friend unavailable} openChat {Open Chat} removeFriend {Remove Friend} directChatRemoved {The direct chat workspace will also be removed.} curiousToday {Curious today} mood {Mood} streak {Streak} chooseWorkflow {Choose Workflow} adoptPet {Adopt a pet} namePet {Name your pet} optionalName {Optional name} adopt {Adopt} other {{key}}}'**
  String actionText({required String key});

  /// Remove friend confirmation title.
  ///
  /// In en, this message translates to:
  /// **'Remove {name}?'**
  String removeFriendTitle({required String name});

  /// Accessibility label for switching voice mode.
  ///
  /// In en, this message translates to:
  /// **'Switch to {mode}'**
  String switchToMode({required String mode});

  /// Accessibility label for adding a friend.
  ///
  /// In en, this message translates to:
  /// **'Add friend'**
  String get addFriendA11y;
}

class _AppLocalizationsDelegate
    extends LocalizationsDelegate<AppLocalizations> {
  const _AppLocalizationsDelegate();

  @override
  Future<AppLocalizations> load(Locale locale) {
    return SynchronousFuture<AppLocalizations>(lookupAppLocalizations(locale));
  }

  @override
  bool isSupported(Locale locale) =>
      <String>['en', 'zh'].contains(locale.languageCode);

  @override
  bool shouldReload(_AppLocalizationsDelegate old) => false;
}

AppLocalizations lookupAppLocalizations(Locale locale) {
  // Lookup logic when language+country codes are specified.
  switch (locale.languageCode) {
    case 'zh':
      {
        switch (locale.countryCode) {
          case 'CN':
            return AppLocalizationsZhCn();
        }
        break;
      }
  }

  // Lookup logic when only language code is specified.
  switch (locale.languageCode) {
    case 'en':
      return AppLocalizationsEn();
    case 'zh':
      return AppLocalizationsZh();
  }

  throw FlutterError(
    'AppLocalizations.delegate failed to load unsupported locale "$locale". This is likely '
    'an issue with the localizations generation tool. Please file an issue '
    'on GitHub with a reproducible sample app and the gen-l10n configuration '
    'that was used.',
  );
}
