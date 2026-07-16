// ignore: unused_import
import 'package:intl/intl.dart' as intl;
import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for English (`en`).
class AppLocalizationsEn extends AppLocalizations {
  AppLocalizationsEn([String locale = 'en']) : super(locale);

  @override
  String get appTitle => 'GizClaw OpenSource';

  @override
  String get language => 'Language';

  @override
  String get chooseLanguage => 'Choose Language';

  @override
  String get languageSystemDefault => 'System (Default)';

  @override
  String get languageEnglish => 'English';

  @override
  String get languageSimplifiedChinese => '简体中文';

  @override
  String get languageSaveFailedTitle => 'Language Not Saved';

  @override
  String get languageSaveFailedMessage =>
      'The language changed for this session, but the preference could not be saved.';

  @override
  String get appSettings => 'APP';

  @override
  String get commonBack => 'Back';

  @override
  String get commonCancel => 'Cancel';

  @override
  String get commonClose => 'Close';

  @override
  String get commonOk => 'OK';

  @override
  String get commonRetry => 'Retry';

  @override
  String get microphonePermissionDeniedTitle =>
      'Microphone permission required';

  @override
  String get microphonePermissionDeniedMessage =>
      'Allow microphone access in Settings, then tap the red microphone to retry.';

  @override
  String get microphoneUnavailableTitle => 'Microphone unavailable';

  @override
  String get microphoneUnavailableMessage =>
      'GizClaw could not start the microphone. Check the audio input and tap the red microphone to retry.';

  @override
  String get microphoneUnavailableStatus => 'MICROPHONE UNAVAILABLE';

  @override
  String get microphonePermissionRequiredStatus =>
      'MICROPHONE PERMISSION REQUIRED';

  @override
  String get microphoneReadinessUnavailableStatus =>
      'MICROPHONE READINESS UNAVAILABLE';

  @override
  String get microphoneNoOutboundAudioStatus =>
      'MICROPHONE PRODUCED NO OUTBOUND AUDIO';

  @override
  String get microphoneRecovering => 'RECOVERING MICROPHONE';

  @override
  String get microphonePermissionRetrySemantics =>
      'Microphone unavailable: permission required. Double tap to retry';

  @override
  String get microphoneCaptureRetrySemantics =>
      'Microphone unavailable: capture failed. Double tap to retry';

  @override
  String get voiceHoldToTalkSemantics => 'Hold to talk';

  @override
  String get voiceStartRealtimeSemantics => 'Start realtime call';

  @override
  String get voiceEndRealtimeSemantics => 'End realtime call';

  @override
  String get conversationStatusConnecting => 'CONNECTING';

  @override
  String get conversationStatusNoActive => 'NO ACTIVE CONVERSATION';

  @override
  String get conversationStatusRealtimeLive => 'REALTIME LIVE';

  @override
  String get conversationStatusListening => 'LISTENING';

  @override
  String get conversationStatusSpeaking => 'SPEAKING';

  @override
  String get conversationStatusRealtimeReady => 'REALTIME READY';

  @override
  String get conversationStatusHoldToTalk => 'HOLD TO TALK';

  @override
  String get onboardingHeroTitle => 'Your agents, everywhere.';

  @override
  String get onboardingHeroDescription =>
      'Connect to a GizClaw server to unlock voice, workflows, and companions across your devices.';

  @override
  String get onboardingConnectServer => 'Get Started by Connecting a Server';

  @override
  String get onboardingConnectHint =>
      'Enter a server access point or scan a QR code.';

  @override
  String onboardingReadStory({required String title}) {
    return 'Read $title';
  }

  @override
  String get onboardingReadStoryAction => 'READ STORY';

  @override
  String get onboardingAgentsTitle => 'Agents that feel close';

  @override
  String get onboardingAgentsDescription =>
      'Talk naturally with always-ready companions for planning, ideas, and everyday help.';

  @override
  String get onboardingAgentsEyebrow => 'ALWAYS READY';

  @override
  String get onboardingAgentsArticleTitle => 'Built around your day';

  @override
  String get onboardingAgentsArticleBody =>
      'Keep a personal agent close for quick questions, planning, and the small decisions that keep your day moving.';

  @override
  String get onboardingAgentsArticleHighlight =>
      'Your conversations stay connected through the GizClaw server you choose.';

  @override
  String get onboardingWorkflowsTitle => 'Workflows that move with you';

  @override
  String get onboardingWorkflowsDescription =>
      'Turn reusable workflows into structured work you can run from any connected device.';

  @override
  String get onboardingWorkflowsEyebrow => 'REUSABLE WORK';

  @override
  String get onboardingWorkflowsArticleTitle => 'Make great work repeatable';

  @override
  String get onboardingWorkflowsArticleBody =>
      'Build a workflow once, then launch the same structured process whenever you need it—from your phone or another connected device.';

  @override
  String get onboardingWorkflowsArticleHighlight =>
      'Carry the process between devices without rebuilding it every time.';

  @override
  String get onboardingRealtimeTitle => 'Realtime by design';

  @override
  String get onboardingRealtimeDescription =>
      'Run low-latency voice sessions while your server keeps every device in the loop.';

  @override
  String get onboardingRealtimeEyebrow => 'LOW LATENCY';

  @override
  String get onboardingRealtimeArticleTitle => 'Voice that keeps up';

  @override
  String get onboardingRealtimeArticleBody =>
      'Start a natural voice session and let GizClaw coordinate the realtime experience across your connected devices.';

  @override
  String get onboardingRealtimeArticleHighlight =>
      'Fast responses, one server, and every connected device in the loop.';

  @override
  String get addServerA11y => 'Add server';

  @override
  String uiText({required String key}) {
    String _temp0 = intl.Intl.selectLogic(key, {
      'notFound': 'Not found',
      'pageUnavailable': 'This page is unavailable.',
      'servers': 'Servers',
      'addServer': 'Add Server',
      'chooseServerSetup': 'Choose a server to finish setup and continue.',
      'switchServerFailed': 'Could not switch servers. Please try again.',
      'addServerDescription':
          'Add a server by entering its details or scanning a GizClaw QR code.',
      'scanQr': 'Scan QR Code',
      'serverDetails': 'SERVER DETAILS',
      'name': 'Name',
      'scanServer': 'Scan Server',
      'pointCamera': 'Point the camera at a GizClaw server QR code.',
      'cameraRequired':
          'Camera access is required to scan a server QR code. Enable it in Settings and try again.',
      'cameraFailed': 'The camera could not start. Go back and try again.',
      'serverNameRequired': 'Enter a server name.',
      'serverAccessPointRequired': 'Enter a server access point.',
      'serverAccessPointInvalid':
          'Use a domain or IP address with a port, for example gizclaw.local:9820.',
      'serverAccessPointDuplicate': 'This access point is already in the list.',
      'serverAddFailed': 'Could not add the server. Please try again.',
      'identity': 'Identity',
      'scanServerQr': 'Scan server QR code',
      'thisDevice': 'This device',
      'deviceIdentityReady': 'Device identity ready',
      'client': 'CLIENT',
      'publicIdentity': 'Public identity',
      'generatedOnDevice': 'Generated on this device',
      'privateKey': 'Private key',
      'protectedSecureStorage': 'Protected by device secure storage',
      'connection': 'CONNECTION',
      'chooseServer': 'Choose a server',
      'server': 'Server',
      'transport': 'Transport',
      'connected': 'Connected',
      'connecting': 'Connecting',
      'offline': 'Offline',
      'setup': 'Setup',
      'home': 'Home',
      'translate': 'Translate',
      'friends': 'Friends',
      'groups': 'Groups',
      'pets': 'Pets',
      'raids': 'Raids',
      'other': '$key',
    });
    return '$_temp0';
  }

  @override
  String actionText({required String key}) {
    String _temp0 = intl.Intl.selectLogic(key, {
      'unableActivate': 'Unable to activate',
      'unableSwitchMode': 'Unable to switch mode',
      'actionFailed': 'The action could not be completed. Please try again.',
      'pushToTalk': 'Push to talk',
      'realtime': 'Realtime',
      'addFriend': 'Add Friend',
      'remove': 'Remove',
      'createGroup': 'Create Group',
      'createGroupA11y': 'Create group',
      'groupName': 'Group name',
      'optionalDescription': 'Description (optional)',
      'connect': 'Connect',
      'myInvite': 'My Invite',
      'inviteToken': 'Invite token',
      'revoke': 'Revoke',
      'friendUnavailable': 'Friend unavailable',
      'openChat': 'Open Chat',
      'removeFriend': 'Remove Friend',
      'directChatRemoved': 'The direct chat workspace will also be removed.',
      'curiousToday': 'Curious today',
      'mood': 'Mood',
      'streak': 'Streak',
      'chooseWorkflow': 'Choose Workflow',
      'adoptPet': 'Adopt a pet',
      'namePet': 'Name your pet',
      'optionalName': 'Optional name',
      'adopt': 'Adopt',
      'other': '$key',
    });
    return '$_temp0';
  }

  @override
  String removeFriendTitle({required String name}) {
    return 'Remove $name?';
  }

  @override
  String switchToMode({required String mode}) {
    return 'Switch to $mode';
  }

  @override
  String get addFriendA11y => 'Add friend';
}
