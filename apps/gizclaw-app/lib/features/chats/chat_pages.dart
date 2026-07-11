import 'dart:async';

import 'package:flutter/cupertino.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:go_router/go_router.dart';

import '../../data/mobile_data_controller.dart';
import '../../data/workspace_chat_controller.dart';
import '../../giz_ui/giz_ui.dart';
import '../../prototype/prototype_data.dart';
import '../../prototype/prototype_models.dart';
import '../browse/browse_pages.dart';

class ChatsPage extends StatelessWidget {
  const ChatsPage({super.key});

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Padding(
              padding: EdgeInsets.fromLTRB(20, 12, 20, 16),
              child: Text('Chats', style: GizText.pageTitle),
            ),
            const Expanded(child: _ChatTypeMenu()),
          ],
        ),
      ),
    );
  }
}

class _ChatTypeMenu extends StatelessWidget {
  const _ChatTypeMenu();

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final raidCount = data.workspaces.where((workspace) {
      return data.workflow(workspace.workflowName).family ==
          WorkflowFamily.raid;
    }).length;
    final groupCount = data.workspaces.where((workspace) {
      return data.workflow(workspace.workflowName).family ==
          WorkflowFamily.chatroom;
    }).length;
    return ListView(
      key: const PageStorageKey('chat-types'),
      padding: const EdgeInsets.only(bottom: 112),
      children: [
        GizListRow(
              leading: const _ChatTypeIcon(
                icon: CupertinoIcons.game_controller_solid,
                color: Color(0xFF416986),
              ),
              title: 'Raid',
              subtitle: '$raidCount workspaces',
              onPressed: () => context.push('/chats/raids'),
            )
            .animate()
            .fadeIn(duration: 280.ms)
            .slideY(begin: 0.05, end: 0, curve: Curves.easeOutCubic),
        GizListRow(
              leading: const _ChatTypeIcon(
                icon: CupertinoIcons.person_2_fill,
                color: Color(0xFF1F7A68),
              ),
              title: 'Group Chat',
              subtitle: '$groupCount chatrooms',
              onPressed: () => context.push('/chats/groups'),
            )
            .animate(delay: 45.ms)
            .fadeIn(duration: 280.ms)
            .slideY(begin: 0.05, end: 0, curve: Curves.easeOutCubic),
      ],
    );
  }
}

class _ChatTypeIcon extends StatelessWidget {
  const _ChatTypeIcon({required this.icon, required this.color});

  final IconData icon;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 50,
      height: 50,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: color,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Icon(icon, color: GizColors.surface, size: 25),
    );
  }
}

class RaidWorkspacesPage extends StatelessWidget {
  const RaidWorkspacesPage({super.key});

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final workspaces = data.workspaces
        .where((workspace) {
          return data.workflow(workspace.workflowName).family ==
              WorkflowFamily.raid;
        })
        .toList(growable: false);
    return CupertinoPageScaffold(
      navigationBar: const CupertinoNavigationBar(
        middle: Text('Raid', style: GizText.title),
        border: null,
        transitionBetweenRoutes: false,
      ),
      child: SafeArea(child: _RaidWorkspaceList(workspaces: workspaces)),
    );
  }
}

class _RaidWorkspaceList extends StatelessWidget {
  const _RaidWorkspaceList({required this.workspaces});

  final List<WorkspaceCard> workspaces;

  @override
  Widget build(BuildContext context) {
    if (workspaces.isEmpty) {
      return Center(
        child: Text(
          'No Raid workspaces yet.',
          style: GizText.body.copyWith(color: GizColors.secondaryInk),
        ),
      );
    }
    return ListView.builder(
      key: const PageStorageKey('raid-workspaces'),
      padding: const EdgeInsets.only(bottom: 24),
      itemCount: workspaces.length,
      itemBuilder: (context, index) {
        return WorkspaceListTile(workspace: workspaces[index])
            .animate(delay: (index * 45).ms)
            .fadeIn(duration: 280.ms)
            .slideY(begin: 0.05, end: 0, curve: Curves.easeOutCubic);
      },
    );
  }
}

class GroupChatsPage extends StatelessWidget {
  const GroupChatsPage({super.key});

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      navigationBar: const CupertinoNavigationBar(
        middle: Text('Group Chat', style: GizText.title),
        border: null,
        transitionBetweenRoutes: false,
      ),
      child: SafeArea(
        child: ListView.builder(
          key: const PageStorageKey('group-chats'),
          padding: const EdgeInsets.only(bottom: 24),
          itemCount: chatrooms.length,
          itemBuilder: (context, index) {
            final room = chatrooms[index];
            final palette = [
              const Color(0xFFFFDDD2),
              const Color(0xFFD7ECFF),
              const Color(0xFFE5DDF8),
            ];
            return GizListRow(
                  leading: Container(
                    width: 50,
                    height: 50,
                    alignment: Alignment.center,
                    decoration: BoxDecoration(
                      color: palette[index % palette.length],
                      borderRadius: BorderRadius.circular(8),
                    ),
                    child: Text('${room.memberCount}', style: GizText.title),
                  ),
                  title: room.name,
                  subtitle: room.subtitle,
                  onPressed: () => context.push('/chats/groups/${room.id}'),
                )
                .animate(delay: (index * 45).ms)
                .fadeIn(duration: 280.ms)
                .slideY(begin: 0.05, end: 0, curve: Curves.easeOutCubic);
          },
        ),
      ),
    );
  }
}

class WorkspaceChatPage extends StatefulWidget {
  const WorkspaceChatPage({super.key, required this.workspaceName});

  final String workspaceName;

  @override
  State<WorkspaceChatPage> createState() => _WorkspaceChatPageState();
}

class _WorkspaceChatPageState extends State<WorkspaceChatPage> {
  final _scrollController = ScrollController();
  WorkspaceChatController? _chat;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (_chat != null) return;
    final data = MobileDataScope.watch(context);
    if (data.connectionState == MobileConnectionState.connecting) return;
    final chat = data.createWorkspaceChat(widget.workspaceName);
    _chat = chat;
    chat.addListener(_handleChatChanged);
    unawaited(chat.start());
  }

  void _handleChatChanged() {
    if (!mounted) return;
    setState(() {});
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollController.hasClients) {
        _scrollController.animateTo(
          _scrollController.position.maxScrollExtent,
          duration: const Duration(milliseconds: 220),
          curve: Curves.easeOutCubic,
        );
      }
    });
  }

  @override
  void dispose() {
    _chat?.removeListener(_handleChatChanged);
    _chat?.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final workspace = data.workspace(widget.workspaceName);
    final workflow = data.workflow(workspace.workflowName);
    final chat = _chat;
    final messages = chat?.messages ?? const <WorkspaceChatMessage>[];
    return CupertinoPageScaffold(
      navigationBar: CupertinoNavigationBar(
        middle: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(workspace.name, style: GizText.title),
            Text(
              workflow.title,
              style: GizText.label.copyWith(color: GizColors.secondaryInk),
            ),
          ],
        ),
        trailing: const GizSignalPulse(size: 24),
        border: null,
        transitionBetweenRoutes: false,
      ),
      child: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: _WorkspaceMessageList(
                controller: _scrollController,
                messages: messages,
                state: chat?.state ?? WorkspaceChatState.loading,
                color: workflow.bannerColor,
                error: chat?.lastError,
              ),
            ),
            _PushToTalkControl(chat: chat),
          ],
        ),
      ),
    );
  }
}

class _WorkspaceMessageList extends StatelessWidget {
  const _WorkspaceMessageList({
    required this.controller,
    required this.messages,
    required this.state,
    required this.color,
    required this.error,
  });

  final Color color;
  final ScrollController controller;
  final Object? error;
  final List<WorkspaceChatMessage> messages;
  final WorkspaceChatState state;

  @override
  Widget build(BuildContext context) {
    if (messages.isEmpty &&
        (state == WorkspaceChatState.loading ||
            state == WorkspaceChatState.connecting)) {
      return const Center(child: CupertinoActivityIndicator());
    }
    if (messages.isEmpty) {
      final unavailable =
          state == WorkspaceChatState.error ||
          state == WorkspaceChatState.offline;
      return Center(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 36),
          child: Text(
            unavailable
                ? 'This conversation is unavailable right now.'
                : 'Start a new conversation.',
            textAlign: TextAlign.center,
            style: GizText.body.copyWith(color: GizColors.secondaryInk),
          ),
        ),
      );
    }
    return ListView.separated(
      controller: controller,
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 18),
      itemCount: messages.length + (error == null ? 0 : 1),
      separatorBuilder: (_, _) => const SizedBox(height: 10),
      itemBuilder: (context, index) {
        if (index == messages.length) {
          return Text(
            'Live updates paused. Showing saved messages.',
            textAlign: TextAlign.center,
            style: GizText.label.copyWith(color: GizColors.secondaryInk),
          );
        }
        final message = messages[index];
        return _ChatBubble(
          text: message.text.isEmpty ? '...' : message.text,
          incoming: message.incoming,
          color: color,
          state: message.state,
        );
      },
    );
  }
}

class _PushToTalkControl extends StatelessWidget {
  const _PushToTalkControl({required this.chat});

  final WorkspaceChatController? chat;

  @override
  Widget build(BuildContext context) {
    final controller = chat;
    final enabled = controller?.canRecord ?? false;
    final recording = controller?.recording ?? false;
    final preparing = controller?.startingInput ?? false;
    final label = recording
        ? 'Release to send'
        : preparing
        ? 'Opening microphone'
        : enabled
        ? 'Hold to talk'
        : 'Voice unavailable';
    return DecoratedBox(
      decoration: const BoxDecoration(
        color: Color(0xFAF4F5F1),
        border: Border(top: BorderSide(color: GizColors.separator)),
      ),
      child: SizedBox(
        height: 116,
        width: double.infinity,
        child: Center(
          child: Listener(
            onPointerDown: enabled
                ? (_) => unawaited(controller!.startInput())
                : null,
            onPointerUp: enabled
                ? (_) => unawaited(controller!.finishInput())
                : null,
            onPointerCancel: enabled
                ? (_) => unawaited(
                    controller!.finishInput(error: 'recording canceled'),
                  )
                : null,
            child: Semantics(
              button: true,
              enabled: enabled,
              label: label,
              child: AnimatedScale(
                scale: recording ? 1.08 : 1,
                duration: const Duration(milliseconds: 160),
                curve: Curves.easeOutCubic,
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 180),
                  curve: Curves.easeOutCubic,
                  width: 190,
                  height: 64,
                  decoration: BoxDecoration(
                    color: recording ? GizColors.accent : GizColors.ink,
                    borderRadius: BorderRadius.circular(32),
                    boxShadow: recording
                        ? [
                            BoxShadow(
                              color: GizColors.accent.withValues(alpha: 0.28),
                              blurRadius: 20,
                              spreadRadius: 5,
                            ),
                          ]
                        : const [],
                  ),
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Icon(
                        recording
                            ? CupertinoIcons.waveform
                            : CupertinoIcons.mic_fill,
                        size: 22,
                        color: recording ? GizColors.ink : GizColors.surface,
                      ),
                      const SizedBox(width: 9),
                      Flexible(
                        child: Text(
                          label,
                          maxLines: 1,
                          overflow: TextOverflow.fade,
                          softWrap: false,
                          style: GizText.label.copyWith(
                            color: recording
                                ? GizColors.ink
                                : GizColors.surface,
                            fontSize: 14,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class GroupChatPage extends StatefulWidget {
  const GroupChatPage({super.key, required this.room});

  final ChatroomCard room;

  @override
  State<GroupChatPage> createState() => _GroupChatPageState();
}

class _GroupChatPageState extends State<GroupChatPage> {
  final _controller = TextEditingController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      navigationBar: CupertinoNavigationBar(
        middle: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(widget.room.name, style: GizText.title),
            Text(
              '${widget.room.memberCount} members',
              style: GizText.label.copyWith(color: GizColors.secondaryInk),
            ),
          ],
        ),
        border: null,
        transitionBetweenRoutes: false,
      ),
      child: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(16, 20, 16, 18),
                children: const [
                  _ChatBubble(
                    text: 'Avery: The new workflow is live.',
                    incoming: true,
                    color: GizColors.blue,
                  ),
                  SizedBox(height: 10),
                  _ChatBubble(
                    text: 'I will test it from mobile.',
                    incoming: false,
                    color: GizColors.ink,
                  ),
                ],
              ),
            ),
            _Composer(controller: _controller),
          ],
        ),
      ),
    );
  }
}

class _ChatBubble extends StatelessWidget {
  const _ChatBubble({
    required this.text,
    required this.incoming,
    required this.color,
    this.state,
  });

  final String text;
  final bool incoming;
  final Color color;
  final WorkspaceMessageState? state;

  @override
  Widget build(BuildContext context) {
    return Align(
      alignment: incoming ? Alignment.centerLeft : Alignment.centerRight,
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 290),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: incoming ? GizColors.surface : color,
            borderRadius: BorderRadius.circular(8),
            border: incoming ? Border.all(color: GizColors.separator) : null,
          ),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 11),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                Text(
                  text,
                  style: GizText.body.copyWith(
                    color: incoming ? GizColors.ink : GizColors.surface,
                  ),
                ),
                if (state == WorkspaceMessageState.streaming ||
                    state == WorkspaceMessageState.failed) ...[
                  const SizedBox(height: 4),
                  Text(
                    state == WorkspaceMessageState.failed
                        ? 'Not delivered'
                        : 'Responding',
                    style: GizText.label.copyWith(
                      color: incoming
                          ? GizColors.secondaryInk
                          : GizColors.surface.withValues(alpha: 0.72),
                    ),
                  ),
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _Composer extends StatelessWidget {
  const _Composer({required this.controller});

  final TextEditingController controller;

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: const BoxDecoration(
        color: Color(0xFAF4F5F1),
        border: Border(top: BorderSide(color: GizColors.separator)),
      ),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 9, 10, 9),
        child: Row(
          children: [
            Expanded(
              child: CupertinoTextField(
                controller: controller,
                minLines: 1,
                maxLines: 4,
                placeholder: 'Message',
                padding: const EdgeInsets.symmetric(
                  horizontal: 14,
                  vertical: 11,
                ),
                style: GizText.body,
                textInputAction: TextInputAction.send,
                onSubmitted: (_) => controller.clear(),
                decoration: BoxDecoration(
                  color: GizColors.surface,
                  borderRadius: BorderRadius.circular(8),
                  border: Border.all(color: GizColors.separator),
                ),
              ),
            ),
            const SizedBox(width: 8),
            CupertinoButton(
              minimumSize: const Size.square(42),
              padding: EdgeInsets.zero,
              color: GizColors.ink,
              borderRadius: BorderRadius.circular(21),
              onPressed: controller.clear,
              child: const Icon(
                CupertinoIcons.arrow_up,
                size: 20,
                color: GizColors.surface,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
