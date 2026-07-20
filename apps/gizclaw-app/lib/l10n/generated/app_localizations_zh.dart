// ignore: unused_import
import 'package:intl/intl.dart' as intl;
import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for Chinese (`zh`).
class AppLocalizationsZh extends AppLocalizations {
  AppLocalizationsZh([String locale = 'zh']) : super(locale);

  @override
  String get appTitle => 'GizClaw OpenSource';

  @override
  String get workspacesTitle => '工作空间';

  @override
  String get newWorkspace => '新建工作空间';

  @override
  String get noWorkspaces => '还没有工作空间。';

  @override
  String get workflowDoubaoRealtimeTitle => '豆包实时对话';

  @override
  String get workflowDoubaoRealtimeSubtitle => '实时语音对话';

  @override
  String get workflowTranslateZhEnTitle => '中英互译';

  @override
  String get workflowTranslateZhJaTitle => '中译日';

  @override
  String get workflowTranslateZhKoTitle => '中译韩';

  @override
  String get workflowTranslateZhEsTitle => '中译西';

  @override
  String get workflowTranslationSubtitle => '实时语音翻译';

  @override
  String get workflowChatTitle => '聊天';

  @override
  String get workflowChatSubtitle => '通用 AI 助手';

  @override
  String get workflowJourneyTitle => '旅程';

  @override
  String get workflowJourneySubtitle => '互动故事向导';

  @override
  String get workflowMurderMysteryTitle => '谋杀之谜';

  @override
  String get workflowMurderMysterySubtitle => '互动推理游戏';

  @override
  String get workflowChatroomTitle => '聊天室';

  @override
  String get workflowChatroomSubtitle => '好友与群组对话';

  @override
  String get language => '语言';

  @override
  String get chooseLanguage => '选择语言';

  @override
  String get languageSystemDefault => '跟随系统';

  @override
  String get languageEnglish => 'English';

  @override
  String get languageSimplifiedChinese => '简体中文';

  @override
  String get languageSaveFailedTitle => '无法保存语言';

  @override
  String get languageSaveFailedMessage => '语言已临时切换，但无法保存到此设备。';

  @override
  String get appSettings => '应用设置';

  @override
  String get commonBack => '返回';

  @override
  String get commonCancel => '取消';

  @override
  String get commonClose => '关闭';

  @override
  String get commonOk => '好';

  @override
  String get commonRetry => '重试';

  @override
  String get localNetworkRecoveryTitle => '无法连接本地网络';

  @override
  String get localNetworkRecoveryMessage =>
      '请在 iOS“设置”中允许 GizClaw 访问“本地网络”，并确认手机与服务器位于可互通的同一 Wi-Fi 后重试。VPN、访客 Wi-Fi 或 AP 隔离也可能阻止连接。';

  @override
  String get workspaceModelLoading => '正在加载可用模型…';

  @override
  String get generationModel => '生成模型';

  @override
  String get extractionModel => '记忆提取模型';

  @override
  String get embeddingModel => '嵌入模型';

  @override
  String get chooseModel => '选择模型';

  @override
  String get noCompatibleGeneratorModels =>
      '没有可访问的生成模型。请让服务器管理员配置并共享一个 LLM 模型。';

  @override
  String get microphonePermissionDeniedTitle => '需要麦克风权限';

  @override
  String get microphonePermissionDeniedMessage => '请在系统设置中允许麦克风访问，然后轻点红色麦克风重试。';

  @override
  String get microphoneUnavailableTitle => '麦克风不可用';

  @override
  String get microphoneUnavailableMessage =>
      'GizClaw 无法启动麦克风。请检查音频输入，然后轻点红色麦克风重试。';

  @override
  String get microphoneUnavailableStatus => '麦克风不可用';

  @override
  String get microphonePermissionRequiredStatus => '需要麦克风权限';

  @override
  String get microphoneRecovering => '正在恢复麦克风';

  @override
  String get microphonePermissionRetrySemantics => '麦克风不可用，需要权限，双击重试';

  @override
  String get microphoneCaptureRetrySemantics => '麦克风不可用，采集失败，双击重试';

  @override
  String get voiceHoldToTalkSemantics => '按住说话';

  @override
  String get voiceStartRealtimeSemantics => '开始实时通话';

  @override
  String get voiceEndRealtimeSemantics => '结束实时通话';

  @override
  String get conversationStatusConnecting => '正在连接';

  @override
  String get conversationStatusNoActive => '没有活动会话';

  @override
  String get conversationStatusRealtimeLive => '实时通话中';

  @override
  String get conversationStatusListening => '正在聆听';

  @override
  String get conversationStatusSpeaking => '正在播放';

  @override
  String get conversationStatusRealtimeReady => '实时通话已就绪';

  @override
  String get conversationStatusHoldToTalk => '按住说话';

  @override
  String get onboardingHeroTitle => '你的智能体，随处相伴。';

  @override
  String get onboardingHeroDescription => '连接 GizClaw 服务器，在所有设备上使用语音、工作流和智能伙伴。';

  @override
  String get onboardingConnectServer => '连接服务器并开始使用';

  @override
  String get onboardingConnectHint => '输入服务器访问地址或扫描二维码。';

  @override
  String onboardingReadStory({required String title}) {
    return '阅读$title';
  }

  @override
  String get onboardingReadStoryAction => '阅读故事';

  @override
  String get onboardingAgentsTitle => '触手可及的智能体';

  @override
  String get onboardingAgentsDescription => '与随时待命的伙伴自然交流，规划日程、激发想法或处理日常事务。';

  @override
  String get onboardingAgentsEyebrow => '随时待命';

  @override
  String get onboardingAgentsArticleTitle => '围绕你的每一天';

  @override
  String get onboardingAgentsArticleBody =>
      '让个人智能体陪在身边，快速回答问题、协助规划，并处理推动一天前进的小决定。';

  @override
  String get onboardingAgentsArticleHighlight => '对话会通过你选择的 GizClaw 服务器保持连接。';

  @override
  String get onboardingWorkflowsTitle => '随你移动的工作流';

  @override
  String get onboardingWorkflowsDescription => '将可复用工作流变成结构化任务，在任何已连接设备上运行。';

  @override
  String get onboardingWorkflowsEyebrow => '复用工作';

  @override
  String get onboardingWorkflowsArticleTitle => '让优秀工作可以重复';

  @override
  String get onboardingWorkflowsArticleBody =>
      '构建一次工作流，需要时即可从手机或其他已连接设备启动相同的结构化流程。';

  @override
  String get onboardingWorkflowsArticleHighlight => '在设备间延续流程，无需每次重新构建。';

  @override
  String get onboardingRealtimeTitle => '为实时而生';

  @override
  String get onboardingRealtimeDescription => '运行低延迟语音会话，由服务器让每台设备保持同步。';

  @override
  String get onboardingRealtimeEyebrow => '低延迟';

  @override
  String get onboardingRealtimeArticleTitle => '跟得上你的语音';

  @override
  String get onboardingRealtimeArticleBody =>
      '开启自然的语音会话，让 GizClaw 在已连接设备间协调实时体验。';

  @override
  String get onboardingRealtimeArticleHighlight => '快速响应、一台服务器，让所有已连接设备保持同步。';

  @override
  String get addServerA11y => '添加服务器';

  @override
  String uiText({required String key}) {
    String _temp0 = intl.Intl.selectLogic(key, {
      'notFound': '未找到',
      'pageUnavailable': '此页面不可用。',
      'servers': '服务器',
      'addServer': '添加服务器',
      'chooseServerSetup': '选择服务器以完成设置并继续。',
      'switchServerFailed': '无法切换服务器，请重试。',
      'addServerDescription': '输入服务器信息或扫描 GizClaw 二维码来添加服务器。',
      'scanQr': '扫描二维码',
      'serverDetails': '服务器信息',
      'name': '名称',
      'scanServer': '扫描服务器',
      'pointCamera': '将相机对准 GizClaw 服务器二维码。',
      'cameraRequired': '扫描服务器二维码需要相机权限。请在“设置”中启用后重试。',
      'cameraFailed': '无法启动相机。请返回后重试。',
      'serverNameRequired': '请输入服务器名称。',
      'serverAccessPointRequired': '请输入服务器访问地址。',
      'serverAccessPointInvalid': '请输入带端口的域名或 IP 地址，例如 gizclaw.local:9820。',
      'serverAccessPointDuplicate': '该访问地址已在列表中。',
      'serverAddFailed': '无法添加服务器，请重试。',
      'identity': '身份',
      'scanServerQr': '扫描服务器二维码',
      'thisDevice': '此设备',
      'deviceIdentityReady': '设备身份已就绪',
      'client': '客户端',
      'publicIdentity': '公开身份',
      'generatedOnDevice': '在此设备上生成',
      'privateKey': '私钥',
      'protectedSecureStorage': '受设备安全存储保护',
      'connection': '连接',
      'chooseServer': '选择服务器',
      'server': '服务器',
      'transport': '传输',
      'connected': '已连接',
      'connecting': '正在连接',
      'offline': '离线',
      'setup': '设置',
      'home': '首页',
      'translate': '翻译',
      'friends': '好友',
      'groups': '群组',
      'pets': '宠物',
      'raids': '任务',
      'other': '$key',
    });
    return '$_temp0';
  }

  @override
  String actionText({required String key}) {
    String _temp0 = intl.Intl.selectLogic(key, {
      'unavailable': '不可用',
      'unableActivate': '无法激活',
      'unableSwitchMode': '无法切换模式',
      'actionFailed': '无法完成操作，请重试。',
      'pushToTalk': '按住说话',
      'realtime': '实时',
      'addFriend': '添加好友',
      'remove': '移除',
      'createGroup': '创建群组',
      'createGroupA11y': '创建群组',
      'groupName': '群组名称',
      'optionalDescription': '描述（可选）',
      'connect': '连接',
      'myInvite': '我的邀请',
      'inviteToken': '邀请码',
      'revoke': '撤销',
      'friendUnavailable': '好友不可用',
      'openChat': '打开聊天',
      'removeFriend': '移除好友',
      'directChatRemoved': '对应的私聊工作区也会被移除。',
      'curiousToday': '今天很好奇',
      'mood': '心情',
      'streak': '连续天数',
      'chooseWorkflow': '选择工作流',
      'adoptPet': '领养宠物',
      'namePet': '给宠物取名',
      'optionalName': '名称（可选）',
      'adopt': '领养',
      'other': '$key',
    });
    return '$_temp0';
  }

  @override
  String removeFriendTitle({required String name}) {
    return '移除$name？';
  }

  @override
  String switchToMode({required String mode}) {
    return '切换到$mode';
  }

  @override
  String get addFriendA11y => '添加好友';
}

/// The translations for Chinese, as used in China (`zh_CN`).
class AppLocalizationsZhCn extends AppLocalizationsZh {
  AppLocalizationsZhCn() : super('zh_CN');

  @override
  String get appTitle => 'GizClaw OpenSource';

  @override
  String get workspacesTitle => '工作空间';

  @override
  String get newWorkspace => '新建工作空间';

  @override
  String get noWorkspaces => '还没有工作空间。';

  @override
  String get workflowDoubaoRealtimeTitle => '豆包实时对话';

  @override
  String get workflowDoubaoRealtimeSubtitle => '实时语音对话';

  @override
  String get workflowTranslateZhEnTitle => '中英互译';

  @override
  String get workflowTranslateZhJaTitle => '中译日';

  @override
  String get workflowTranslateZhKoTitle => '中译韩';

  @override
  String get workflowTranslateZhEsTitle => '中译西';

  @override
  String get workflowTranslationSubtitle => '实时语音翻译';

  @override
  String get workflowChatTitle => '聊天';

  @override
  String get workflowChatSubtitle => '通用 AI 助手';

  @override
  String get workflowJourneyTitle => '旅程';

  @override
  String get workflowJourneySubtitle => '互动故事向导';

  @override
  String get workflowMurderMysteryTitle => '谋杀之谜';

  @override
  String get workflowMurderMysterySubtitle => '互动推理游戏';

  @override
  String get workflowChatroomTitle => '聊天室';

  @override
  String get workflowChatroomSubtitle => '好友与群组对话';

  @override
  String get language => '语言';

  @override
  String get chooseLanguage => '选择语言';

  @override
  String get languageSystemDefault => '跟随系统（默认）';

  @override
  String get languageEnglish => 'English';

  @override
  String get languageSimplifiedChinese => '简体中文';

  @override
  String get languageSaveFailedTitle => '语言设置未保存';

  @override
  String get languageSaveFailedMessage => '本次使用的语言已切换，但无法保存该设置。';

  @override
  String get appSettings => '应用';

  @override
  String get commonBack => '返回';

  @override
  String get commonCancel => '取消';

  @override
  String get commonClose => '关闭';

  @override
  String get commonOk => '好';

  @override
  String get commonRetry => '重试';

  @override
  String get localNetworkRecoveryTitle => '无法连接本地网络';

  @override
  String get localNetworkRecoveryMessage =>
      '请在 iOS“设置”中允许 GizClaw 访问“本地网络”，并确认手机与服务器位于可互通的同一 Wi-Fi 后重试。VPN、访客 Wi-Fi 或 AP 隔离也可能阻止连接。';

  @override
  String get workspaceModelLoading => '正在加载可用模型…';

  @override
  String get generationModel => '生成模型';

  @override
  String get extractionModel => '记忆提取模型';

  @override
  String get embeddingModel => '嵌入模型';

  @override
  String get chooseModel => '选择模型';

  @override
  String get noCompatibleGeneratorModels =>
      '没有可访问的生成模型。请让服务器管理员配置并共享一个 LLM 模型。';

  @override
  String get microphonePermissionDeniedTitle => '需要麦克风权限';

  @override
  String get microphonePermissionDeniedMessage => '请在系统设置中允许麦克风访问，然后轻点红色麦克风重试。';

  @override
  String get microphoneUnavailableTitle => '麦克风不可用';

  @override
  String get microphoneUnavailableMessage =>
      'GizClaw 无法启动麦克风。请检查音频输入，然后轻点红色麦克风重试。';

  @override
  String get microphoneUnavailableStatus => '麦克风不可用';

  @override
  String get microphonePermissionRequiredStatus => '需要麦克风权限';

  @override
  String get microphoneRecovering => '正在恢复麦克风';

  @override
  String get microphonePermissionRetrySemantics => '麦克风不可用，需要权限，双击重试';

  @override
  String get microphoneCaptureRetrySemantics => '麦克风不可用，采集失败，双击重试';

  @override
  String get voiceHoldToTalkSemantics => '按住说话';

  @override
  String get voiceStartRealtimeSemantics => '开始实时通话';

  @override
  String get voiceEndRealtimeSemantics => '结束实时通话';

  @override
  String get conversationStatusConnecting => '正在连接';

  @override
  String get conversationStatusNoActive => '没有活动会话';

  @override
  String get conversationStatusRealtimeLive => '实时通话中';

  @override
  String get conversationStatusListening => '正在聆听';

  @override
  String get conversationStatusSpeaking => '正在播放';

  @override
  String get conversationStatusRealtimeReady => '实时通话已就绪';

  @override
  String get conversationStatusHoldToTalk => '按住说话';

  @override
  String get onboardingHeroTitle => '你的智能体，随处相伴。';

  @override
  String get onboardingHeroDescription => '连接 GizClaw 服务器，在所有设备上使用语音、工作流和智能伙伴。';

  @override
  String get onboardingConnectServer => '连接服务器并开始使用';

  @override
  String get onboardingConnectHint => '输入服务器访问地址或扫描二维码。';

  @override
  String onboardingReadStory({required String title}) {
    return '阅读$title';
  }

  @override
  String get onboardingReadStoryAction => '阅读故事';

  @override
  String get onboardingAgentsTitle => '触手可及的智能体';

  @override
  String get onboardingAgentsDescription => '与随时待命的伙伴自然交流，规划日程、激发想法或处理日常事务。';

  @override
  String get onboardingAgentsEyebrow => '随时待命';

  @override
  String get onboardingAgentsArticleTitle => '围绕你的每一天';

  @override
  String get onboardingAgentsArticleBody =>
      '让个人智能体陪在身边，快速回答问题、协助规划，并处理推动一天前进的小决定。';

  @override
  String get onboardingAgentsArticleHighlight => '对话会通过你选择的 GizClaw 服务器保持连接。';

  @override
  String get onboardingWorkflowsTitle => '随你移动的工作流';

  @override
  String get onboardingWorkflowsDescription => '将可复用工作流变成结构化任务，在任何已连接设备上运行。';

  @override
  String get onboardingWorkflowsEyebrow => '复用工作';

  @override
  String get onboardingWorkflowsArticleTitle => '让优秀工作可以重复';

  @override
  String get onboardingWorkflowsArticleBody =>
      '构建一次工作流，需要时即可从手机或其他已连接设备启动相同的结构化流程。';

  @override
  String get onboardingWorkflowsArticleHighlight => '在设备间延续流程，无需每次重新构建。';

  @override
  String get onboardingRealtimeTitle => '为实时而生';

  @override
  String get onboardingRealtimeDescription => '运行低延迟语音会话，由服务器让每台设备保持同步。';

  @override
  String get onboardingRealtimeEyebrow => '低延迟';

  @override
  String get onboardingRealtimeArticleTitle => '跟得上你的语音';

  @override
  String get onboardingRealtimeArticleBody =>
      '开启自然的语音会话，让 GizClaw 在已连接设备间协调实时体验。';

  @override
  String get onboardingRealtimeArticleHighlight => '快速响应、一台服务器，让所有已连接设备保持同步。';

  @override
  String get addServerA11y => '添加服务器';

  @override
  String uiText({required String key}) {
    String _temp0 = intl.Intl.selectLogic(key, {
      'notFound': '未找到',
      'pageUnavailable': '此页面不可用。',
      'servers': '服务器',
      'addServer': '添加服务器',
      'chooseServerSetup': '选择服务器以完成设置并继续。',
      'switchServerFailed': '无法切换服务器，请重试。',
      'addServerDescription': '输入服务器信息或扫描 GizClaw 二维码来添加服务器。',
      'scanQr': '扫描二维码',
      'serverDetails': '服务器信息',
      'name': '名称',
      'scanServer': '扫描服务器',
      'pointCamera': '将相机对准 GizClaw 服务器二维码。',
      'cameraRequired': '扫描服务器二维码需要相机权限。请在“设置”中启用后重试。',
      'cameraFailed': '无法启动相机。请返回后重试。',
      'serverNameRequired': '请输入服务器名称。',
      'serverAccessPointRequired': '请输入服务器访问地址。',
      'serverAccessPointInvalid': '请输入带端口的域名或 IP 地址，例如 gizclaw.local:9820。',
      'serverAccessPointDuplicate': '该访问地址已在列表中。',
      'serverAddFailed': '无法添加服务器，请重试。',
      'identity': '身份',
      'scanServerQr': '扫描服务器二维码',
      'thisDevice': '此设备',
      'deviceIdentityReady': '设备身份已就绪',
      'client': '客户端',
      'publicIdentity': '公开身份',
      'generatedOnDevice': '在此设备上生成',
      'privateKey': '私钥',
      'protectedSecureStorage': '受设备安全存储保护',
      'connection': '连接',
      'chooseServer': '选择服务器',
      'server': '服务器',
      'transport': '传输',
      'connected': '已连接',
      'connecting': '正在连接',
      'offline': '离线',
      'setup': '设置',
      'home': '首页',
      'translate': '翻译',
      'friends': '好友',
      'groups': '群组',
      'pets': '宠物',
      'raids': '任务',
      'other': '$key',
    });
    return '$_temp0';
  }

  @override
  String actionText({required String key}) {
    String _temp0 = intl.Intl.selectLogic(key, {
      'unavailable': '不可用',
      'unableActivate': '无法激活',
      'unableSwitchMode': '无法切换模式',
      'actionFailed': '无法完成操作，请重试。',
      'pushToTalk': '按住说话',
      'realtime': '实时',
      'addFriend': '添加好友',
      'remove': '移除',
      'createGroup': '创建群组',
      'createGroupA11y': '创建群组',
      'groupName': '群组名称',
      'optionalDescription': '描述（可选）',
      'connect': '连接',
      'myInvite': '我的邀请',
      'inviteToken': '邀请码',
      'revoke': '撤销',
      'friendUnavailable': '好友不可用',
      'openChat': '打开聊天',
      'removeFriend': '移除好友',
      'directChatRemoved': '对应的私聊工作区也会被移除。',
      'curiousToday': '今天很好奇',
      'mood': '心情',
      'streak': '连续天数',
      'chooseWorkflow': '选择工作流',
      'adoptPet': '领养宠物',
      'namePet': '给宠物取名',
      'optionalName': '名称（可选）',
      'adopt': '领养',
      'other': '$key',
    });
    return '$_temp0';
  }

  @override
  String removeFriendTitle({required String name}) {
    return '移除$name？';
  }

  @override
  String switchToMode({required String mode}) {
    return '切换到$mode';
  }

  @override
  String get addFriendA11y => '添加好友';
}
