import 'package:flutter/cupertino.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:go_router/go_router.dart';

import '../../data/mobile_data_controller.dart';
import '../../giz_ui/giz_ui.dart';
import '../../prototype/prototype_data.dart';
import '../../prototype/prototype_models.dart';
import '../browse/browse_pages.dart';

enum ChatListMode { workspaces, groups }

class ChatsPage extends StatelessWidget {
  const ChatsPage({super.key, required this.mode});

  final ChatListMode mode;

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
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: SizedBox(
                width: double.infinity,
                child: CupertinoSlidingSegmentedControl<ChatListMode>(
                  groupValue: mode,
                  thumbColor: GizColors.ink,
                  backgroundColor: const Color(0xFFE3E7E0),
                  padding: const EdgeInsets.all(3),
                  children: {
                    ChatListMode.workspaces: _SegmentLabel(
                      label: 'Workspace',
                      selected: mode == ChatListMode.workspaces,
                    ),
                    ChatListMode.groups: _SegmentLabel(
                      label: 'Group Chat',
                      selected: mode == ChatListMode.groups,
                    ),
                  },
                  onValueChanged: (value) {
                    if (value == ChatListMode.groups) {
                      context.go('/chats/groups');
                    } else if (value == ChatListMode.workspaces) {
                      context.go('/chats/workspaces');
                    }
                  },
                ),
              ),
            ),
            const SizedBox(height: 10),
            Expanded(
              child: AnimatedSwitcher(
                duration: 220.ms,
                switchInCurve: Curves.easeOutCubic,
                switchOutCurve: Curves.easeInCubic,
                transitionBuilder: (child, animation) {
                  return FadeTransition(
                    opacity: animation,
                    child: SlideTransition(
                      position: Tween<Offset>(
                        begin: const Offset(0.02, 0),
                        end: Offset.zero,
                      ).animate(animation),
                      child: child,
                    ),
                  );
                },
                child: mode == ChatListMode.workspaces
                    ? const _WorkspaceChats(key: ValueKey('workspaces'))
                    : const _GroupChats(key: ValueKey('groups')),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _SegmentLabel extends StatelessWidget {
  const _SegmentLabel({required this.label, required this.selected});

  final String label;
  final bool selected;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 9),
      child: Text(
        label,
        style: GizText.label.copyWith(
          color: selected ? GizColors.surface : GizColors.secondaryInk,
          fontSize: 13,
        ),
      ),
    );
  }
}

class _WorkspaceChats extends StatelessWidget {
  const _WorkspaceChats({super.key});

  @override
  Widget build(BuildContext context) {
    final workspaces = MobileDataScope.watch(context).workspaces;
    if (workspaces.isEmpty) {
      return Center(
        child: Text(
          'No synced workspaces yet.',
          style: GizText.body.copyWith(color: GizColors.secondaryInk),
        ),
      );
    }
    return ListView.builder(
      key: const PageStorageKey('workspace-chats'),
      padding: const EdgeInsets.only(bottom: 112),
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

class _GroupChats extends StatelessWidget {
  const _GroupChats({super.key});

  @override
  Widget build(BuildContext context) {
    return ListView.builder(
      key: const PageStorageKey('group-chats'),
      padding: const EdgeInsets.only(bottom: 112),
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
  final _controller = TextEditingController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final workspace = data.workspace(widget.workspaceName);
    final workflow = data.workflow(workspace.workflowName);
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
      ),
      child: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(16, 20, 16, 18),
                children: [
                  _ChatBubble(
                    text:
                        'This workspace is ready. What would you like to work on?',
                    incoming: true,
                    color: workflow.bannerColor,
                  ),
                  const SizedBox(height: 10),
                  const _ChatBubble(
                    text: 'Help me turn today into a short, focused plan.',
                    incoming: false,
                    color: GizColors.ink,
                  ),
                  const SizedBox(height: 10),
                  _ChatBubble(
                    text:
                        'Start with one outcome, then choose two actions that move it forward.',
                    incoming: true,
                    color: workflow.bannerColor,
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
  });

  final String text;
  final bool incoming;
  final Color color;

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
            child: Text(
              text,
              style: GizText.body.copyWith(
                color: incoming ? GizColors.ink : GizColors.surface,
              ),
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
              onPressed: () => controller.clear(),
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
