import 'package:flutter/cupertino.dart';
import 'package:flutter/services.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:go_router/go_router.dart';

import '../../data/mobile_data_controller.dart';
import '../../giz_ui/giz_ui.dart';
import '../../identity/app_identity_store.dart';
import '../../prototype/prototype_models.dart';

class FriendsPage extends StatelessWidget {
  const FriendsPage({super.key});

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final friendChats = data.chatroomWorkspaces
        .where((item) => item.kind == ChatroomWorkspaceKind.direct)
        .toList(growable: false);
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: CustomScrollView(
          key: const PageStorageKey('friends-scroll'),
          slivers: [
            CupertinoSliverRefreshControl(onRefresh: data.refresh),
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 12, 12, 16),
              sliver: SliverToBoxAdapter(
                child: Row(
                  children: [
                    const Expanded(
                      child: Text('Friends', style: GizText.pageTitle),
                    ),
                    GizPageActionButton(
                      icon: GizIcons.person_add,
                      semanticLabel: 'Add friend',
                      onPressed: () => _showFriendConnect(context, data),
                    ),
                  ],
                ),
              ),
            ),
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
              sliver: SliverToBoxAdapter(
                child: Text(
                  'YOUR CIRCLE',
                  style: GizText.label.copyWith(color: GizColors.secondaryInk),
                ),
              ),
            ),
            if (friendChats.isEmpty)
              SliverFillRemaining(
                hasScrollBody: false,
                child: Center(
                  child: Text(
                    'No friends yet.',
                    style: GizText.body.copyWith(color: GizColors.secondaryInk),
                  ),
                ),
              )
            else
              SliverList.builder(
                itemCount: friendChats.length,
                itemBuilder: (context, index) {
                  return FriendRow(
                        friend: friendChats[index],
                        index: index,
                        onDelete: () =>
                            _deleteFriend(context, data, friendChats[index]),
                      )
                      .animate(delay: (index * 45).ms)
                      .fadeIn(duration: 280.ms)
                      .slideY(begin: 0.05, end: 0, curve: Curves.easeOutCubic);
                },
              ),
            const SliverPadding(padding: EdgeInsets.only(bottom: 112)),
          ],
        ),
      ),
    );
  }

  Future<void> _showFriendConnect(
    BuildContext context,
    MobileDataController data,
  ) async {
    final friend = await showCupertinoModalPopup<FriendObject>(
      context: context,
      builder: (context) => _FriendConnectSheet(data: data),
    );
    if (!context.mounted || friend == null) return;
    final workspaceName = friend.workspaceName.trim();
    if (workspaceName.isEmpty) return;
    context.push(
      '/raids/drivers/chatroom/${Uri.encodeComponent(workspaceName)}',
    );
  }

  Future<void> _deleteFriend(
    BuildContext context,
    MobileDataController data,
    ChatroomWorkspaceMetadata friend,
  ) async {
    final confirmed = await showCupertinoDialog<bool>(
      context: context,
      builder: (context) => CupertinoAlertDialog(
        title: Text('Remove ${friend.title}?'),
        content: const Text('The direct chat workspace will also be removed.'),
        actions: [
          CupertinoDialogAction(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          CupertinoDialogAction(
            isDestructiveAction: true,
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Remove'),
          ),
        ],
      ),
    );
    if (confirmed != true || !context.mounted) return;
    try {
      await data.deleteFriend(friend.resourceId);
    } catch (error) {
      if (context.mounted) await _showFriendError(context, error);
    }
  }
}

class GroupsPage extends StatelessWidget {
  const GroupsPage({super.key});

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final groups = data.chatroomWorkspaces
        .where((item) => item.kind == ChatroomWorkspaceKind.group)
        .toList(growable: false);
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: CustomScrollView(
          key: const PageStorageKey('groups-scroll'),
          slivers: [
            CupertinoSliverRefreshControl(onRefresh: data.refresh),
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 12, 12, 16),
              sliver: SliverToBoxAdapter(
                child: Row(
                  children: [
                    const Expanded(
                      child: Text('Groups', style: GizText.pageTitle),
                    ),
                    GizPageActionButton(
                      icon: GizIcons.person_3_fill,
                      semanticLabel: 'Create group',
                      onPressed: () => _showCreateGroup(context, data),
                    ),
                  ],
                ),
              ),
            ),
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
              sliver: SliverToBoxAdapter(
                child: Text(
                  'VOICE ROOMS',
                  style: GizText.label.copyWith(color: GizColors.secondaryInk),
                ),
              ),
            ),
            if (groups.isEmpty)
              SliverFillRemaining(
                hasScrollBody: false,
                child: Center(
                  child: Text(
                    'No groups yet.',
                    style: GizText.body.copyWith(color: GizColors.secondaryInk),
                  ),
                ),
              )
            else
              SliverList.builder(
                itemCount: groups.length,
                itemBuilder: (context, index) {
                  final group = groups[index];
                  return GizListRow(
                        leading: const GizIconTile(
                          icon: GizIcons.person_3_fill,
                          backgroundColor: Color(0xFFDDE8FF),
                          foregroundColor: Color(0xFF315E9D),
                          size: 52,
                          iconSize: 22,
                        ),
                        title: group.title,
                        subtitle: group.description.trim().isEmpty
                            ? 'Group voice chat'
                            : group.description.trim(),
                        onPressed: () => context.push(
                          '/groups/'
                          '${Uri.encodeComponent(group.workspaceName)}',
                        ),
                      )
                      .animate(delay: (index * 45).ms)
                      .fadeIn(duration: 280.ms)
                      .slideY(begin: 0.05, end: 0, curve: Curves.easeOutCubic);
                },
              ),
            const SliverPadding(padding: EdgeInsets.only(bottom: 112)),
          ],
        ),
      ),
    );
  }

  Future<void> _showCreateGroup(
    BuildContext context,
    MobileDataController data,
  ) async {
    final group = await showCupertinoModalPopup<FriendGroupObject>(
      context: context,
      builder: (context) => _CreateGroupSheet(data: data),
    );
    if (!context.mounted || group == null) return;
    final workspaceName = group.workspaceName.trim();
    if (workspaceName.isEmpty) return;
    context.push('/groups/${Uri.encodeComponent(workspaceName)}');
  }
}

class FriendRow extends StatelessWidget {
  const FriendRow({
    super.key,
    required this.friend,
    required this.index,
    required this.onDelete,
  });

  final ChatroomWorkspaceMetadata friend;
  final int index;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    const avatarColors = [
      Color(0xFFFFDCD0),
      Color(0xFFD9F2EA),
      Color(0xFFD9E8FF),
    ];
    return GizListRow(
      leading: GizSquircle(
        borderRadius: GizCorners.icon(52),
        child: Container(
          width: 52,
          height: 52,
          alignment: Alignment.center,
          color: avatarColors[index % avatarColors.length],
          child: Text(
            friend.title.substring(0, 1).toUpperCase(),
            style: GizText.sectionTitle,
          ),
        ),
      ),
      title: friend.title,
      subtitle: 'Direct chat',
      onPressed: () => _openChat(context),
      trailing: CupertinoButton(
        minimumSize: const Size.square(40),
        padding: EdgeInsets.zero,
        onPressed: () => _showActions(context),
        child: const Icon(
          GizIcons.ellipsis,
          size: 20,
          color: GizColors.secondaryInk,
        ),
      ),
    );
  }

  void _openChat(BuildContext context) {
    context.push(
      '/raids/drivers/chatroom/'
      '${Uri.encodeComponent(friend.workspaceName)}',
    );
  }

  Future<void> _showActions(BuildContext context) async {
    final action = await showCupertinoModalPopup<String>(
      context: context,
      builder: (context) => CupertinoActionSheet(
        title: Text(friend.title),
        actions: [
          CupertinoActionSheetAction(
            onPressed: () => Navigator.pop(context, 'chat'),
            child: const Text('Open Chat'),
          ),
          CupertinoActionSheetAction(
            isDestructiveAction: true,
            onPressed: () => Navigator.pop(context, 'delete'),
            child: const Text('Remove Friend'),
          ),
        ],
        cancelButton: CupertinoActionSheetAction(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancel'),
        ),
      ),
    );
    if (!context.mounted) return;
    if (action == 'chat') _openChat(context);
    if (action == 'delete') onDelete();
  }
}

class _CreateGroupSheet extends StatefulWidget {
  const _CreateGroupSheet({required this.data});

  final MobileDataController data;

  @override
  State<_CreateGroupSheet> createState() => _CreateGroupSheetState();
}

class _CreateGroupSheetState extends State<_CreateGroupSheet> {
  final _nameController = TextEditingController();
  final _descriptionController = TextEditingController();
  bool _busy = false;
  Object? _error;

  @override
  void dispose() {
    _nameController.dispose();
    _descriptionController.dispose();
    super.dispose();
  }

  Future<void> _create() async {
    final name = _nameController.text.trim();
    if (_busy || name.isEmpty) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final group = await widget.data.createFriendGroup(
        name: name,
        description: _descriptionController.text,
      );
      if (mounted) Navigator.pop(context, group);
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _busy = false;
        _error = error;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final background = CupertinoColors.systemBackground.resolveFrom(context);
    final safeBottom = MediaQuery.viewPaddingOf(context).bottom;
    return Container(
      key: const ValueKey('create-group-sheet'),
      decoration: BoxDecoration(
        color: background,
        borderRadius: const BorderRadius.vertical(top: Radius.circular(16)),
      ),
      padding: EdgeInsets.fromLTRB(20, 12, 20, 20 + safeBottom),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Center(
            child: Container(
              width: 36,
              height: 5,
              decoration: BoxDecoration(
                color: CupertinoColors.systemGrey4.resolveFrom(context),
                borderRadius: BorderRadius.circular(3),
              ),
            ),
          ),
          const SizedBox(height: 18),
          Text('Create Group', style: GizText.sectionTitle),
          const SizedBox(height: 16),
          CupertinoTextField(
            controller: _nameController,
            placeholder: 'Group name',
            textInputAction: TextInputAction.next,
            padding: const EdgeInsets.all(14),
          ),
          const SizedBox(height: 10),
          CupertinoTextField(
            controller: _descriptionController,
            placeholder: 'Description (optional)',
            textInputAction: TextInputAction.done,
            onSubmitted: (_) => _create(),
            padding: const EdgeInsets.all(14),
          ),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(
              _friendErrorMessage(_error!),
              textAlign: TextAlign.center,
              style: GizText.body.copyWith(
                color: CupertinoColors.systemRed.resolveFrom(context),
              ),
            ),
          ],
          const SizedBox(height: 14),
          CupertinoButton.filled(
            onPressed: _busy ? null : _create,
            child: _busy
                ? const CupertinoActivityIndicator()
                : const Text('Create Group'),
          ),
          SizedBox(height: MediaQuery.viewInsetsOf(context).bottom),
        ],
      ),
    );
  }
}

enum _FriendSheetMode { add, invite }

class _FriendConnectSheet extends StatefulWidget {
  const _FriendConnectSheet({required this.data});

  final MobileDataController data;

  @override
  State<_FriendConnectSheet> createState() => _FriendConnectSheetState();
}

class _FriendConnectSheetState extends State<_FriendConnectSheet> {
  final _inviteController = TextEditingController();
  final _tokenController = TextEditingController(text: 'No active invite');
  _FriendSheetMode _mode = _FriendSheetMode.add;
  bool _busy = false;
  bool _copied = false;
  bool _tokenLoaded = false;
  String _token = '';
  String _expiresAt = '';
  Object? _error;

  @override
  void dispose() {
    _inviteController.dispose();
    _tokenController.dispose();
    super.dispose();
  }

  Future<void> _loadToken() async {
    if (_busy) return;
    _setBusy();
    try {
      final response = await widget.data.getFriendInviteToken();
      if (!mounted) return;
      setState(() {
        _token = response.inviteToken.trim();
        _expiresAt = response.expiresAt.trim();
        _tokenLoaded = true;
        _tokenController.text = _token.isEmpty ? 'No active invite' : _token;
      });
    } catch (error) {
      if (mounted) setState(() => _error = error);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _createToken() async {
    if (_busy) return;
    _setBusy();
    try {
      final response = await widget.data.createFriendInviteToken();
      if (!mounted) return;
      setState(() {
        _token = response.inviteToken.trim();
        _expiresAt = response.expiresAt.trim();
        _tokenLoaded = true;
        _tokenController.text = _token;
      });
    } catch (error) {
      if (mounted) setState(() => _error = error);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _clearToken() async {
    if (_busy || _token.isEmpty) return;
    _setBusy();
    try {
      await widget.data.clearFriendInviteToken();
      if (!mounted) return;
      setState(() {
        _token = '';
        _expiresAt = '';
        _tokenController.text = 'No active invite';
      });
    } catch (error) {
      if (mounted) setState(() => _error = error);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _addFriend() async {
    final token = _inviteController.text.trim();
    if (_busy || token.isEmpty) return;
    _setBusy();
    try {
      final friend = await widget.data.addFriend(token);
      if (mounted) Navigator.pop(context, friend);
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _busy = false;
        _error = error;
      });
    }
  }

  Future<void> _rotateToken() async {
    if (_busy || _token.isEmpty) return;
    _setBusy();
    try {
      await widget.data.clearFriendInviteToken();
      if (!mounted) return;
      setState(() {
        _token = '';
        _expiresAt = '';
        _tokenController.text = 'No active invite';
      });
      final response = await widget.data.createFriendInviteToken();
      if (!mounted) return;
      setState(() {
        _token = response.inviteToken.trim();
        _expiresAt = response.expiresAt.trim();
        _tokenController.text = _token;
      });
    } catch (error) {
      if (mounted) setState(() => _error = error);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _copyToken() async {
    await Clipboard.setData(ClipboardData(text: _token));
    if (!mounted) return;
    setState(() => _copied = true);
    await Future<void>.delayed(const Duration(milliseconds: 1200));
    if (mounted) setState(() => _copied = false);
  }

  void _setBusy() {
    setState(() {
      _busy = true;
      _error = null;
    });
  }

  @override
  Widget build(BuildContext context) {
    final background = CupertinoColors.systemBackground.resolveFrom(context);
    final secondary = CupertinoColors.secondarySystemBackground.resolveFrom(
      context,
    );
    final safeBottom = MediaQuery.viewPaddingOf(context).bottom;
    return Container(
      key: const ValueKey('friend-connect-sheet'),
      decoration: BoxDecoration(
        color: background,
        borderRadius: const BorderRadius.vertical(top: Radius.circular(16)),
      ),
      padding: EdgeInsets.fromLTRB(20, 12, 20, 20 + safeBottom),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Center(
            child: Container(
              width: 36,
              height: 5,
              decoration: BoxDecoration(
                color: CupertinoColors.systemGrey4.resolveFrom(context),
                borderRadius: BorderRadius.circular(3),
              ),
            ),
          ),
          const SizedBox(height: 18),
          Text('Connect', style: GizText.sectionTitle),
          const SizedBox(height: 14),
          CupertinoSlidingSegmentedControl<_FriendSheetMode>(
            groupValue: _mode,
            children: const {
              _FriendSheetMode.add: Padding(
                padding: EdgeInsets.symmetric(vertical: 8),
                child: Text('Add Friend'),
              ),
              _FriendSheetMode.invite: Padding(
                padding: EdgeInsets.symmetric(vertical: 8),
                child: Text('My Invite'),
              ),
            },
            onValueChanged: (value) {
              if (value == null) return;
              setState(() => _mode = value);
              if (value == _FriendSheetMode.invite && !_tokenLoaded) {
                _loadToken();
              }
            },
          ),
          const SizedBox(height: 20),
          AnimatedSwitcher(
            duration: const Duration(milliseconds: 180),
            child: _mode == _FriendSheetMode.add
                ? _buildAddFriend()
                : _buildMyInvite(secondary),
          ),
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(
              _friendErrorMessage(_error!),
              textAlign: TextAlign.center,
              style: GizText.body.copyWith(
                color: CupertinoColors.systemRed.resolveFrom(context),
              ),
            ),
          ],
          SizedBox(height: MediaQuery.viewInsetsOf(context).bottom),
        ],
      ),
    );
  }

  Widget _buildAddFriend() {
    return Column(
      key: const ValueKey('add-friend'),
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        CupertinoTextField(
          controller: _inviteController,
          placeholder: 'Invite token',
          autocorrect: false,
          enableSuggestions: false,
          textInputAction: TextInputAction.done,
          onSubmitted: (_) => _addFriend(),
          padding: const EdgeInsets.all(14),
        ),
        const SizedBox(height: 12),
        CupertinoButton.filled(
          onPressed: _busy ? null : _addFriend,
          child: _busy
              ? const CupertinoActivityIndicator()
              : const Text('Add Friend'),
        ),
      ],
    );
  }

  Widget _buildMyInvite(Color secondary) {
    return Column(
      key: const ValueKey('my-invite'),
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        GizSquircle(
          borderRadius: GizCorners.compactCard,
          child: Container(
            constraints: const BoxConstraints(minHeight: 74),
            padding: const EdgeInsets.all(14),
            color: secondary,
            child: _busy && _token.isEmpty
                ? const Center(child: CupertinoActivityIndicator())
                : Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      CupertinoTextField(
                        controller: _tokenController,
                        readOnly: true,
                        padding: EdgeInsets.zero,
                        decoration: null,
                        style: GizText.title,
                      ),
                      if (_expiresAt.isNotEmpty) ...[
                        const SizedBox(height: 6),
                        Text(
                          'Expires ${_formatInviteExpiry(_expiresAt)}',
                          style: GizText.label.copyWith(
                            color: GizColors.secondaryInk,
                          ),
                        ),
                      ],
                    ],
                  ),
          ),
        ),
        const SizedBox(height: 12),
        Row(
          children: [
            if (_token.isNotEmpty) ...[
              CupertinoButton(
                padding: const EdgeInsets.symmetric(horizontal: 12),
                onPressed: _busy ? null : _clearToken,
                child: const Text('Revoke'),
              ),
              const SizedBox(width: 8),
            ],
            Expanded(
              child: CupertinoButton.filled(
                onPressed: _busy
                    ? null
                    : _token.isEmpty
                    ? _createToken
                    : _copyToken,
                child: Text(
                  _token.isEmpty
                      ? 'Create Invite'
                      : _copied
                      ? 'Copied'
                      : 'Copy Invite',
                ),
              ),
            ),
            if (_token.isNotEmpty) ...[
              const SizedBox(width: 8),
              CupertinoButton(
                padding: const EdgeInsets.symmetric(horizontal: 12),
                onPressed: _busy ? null : _rotateToken,
                child: const Icon(GizIcons.refresh),
              ),
            ],
          ],
        ),
      ],
    );
  }
}

String _formatInviteExpiry(String value) {
  final parsed = DateTime.tryParse(value)?.toLocal();
  if (parsed == null) return value;
  String two(int number) => number.toString().padLeft(2, '0');
  return '${parsed.month}/${parsed.day} ${two(parsed.hour)}:${two(parsed.minute)}';
}

String _friendErrorMessage(Object error) {
  final text = error.toString();
  return text.startsWith('Bad state: ') ? text.substring(11) : text;
}

Future<void> _showFriendError(BuildContext context, Object error) =>
    showCupertinoDialog<void>(
      context: context,
      builder: (context) => CupertinoAlertDialog(
        title: const Text('Friend unavailable'),
        content: Text(_friendErrorMessage(error)),
        actions: [
          CupertinoDialogAction(
            onPressed: () => Navigator.pop(context),
            child: const Text('OK'),
          ),
        ],
      ),
    );

class PrototypePetPage extends StatelessWidget {
  const PrototypePetPage({super.key});

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: ListView(
          key: const PageStorageKey('pet-scroll'),
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 112),
          children: [
            const Text('Pets', style: GizText.pageTitle),
            const SizedBox(height: 18),
            AspectRatio(
              aspectRatio: 0.72,
              child: ClipRSuperellipse(
                borderRadius: GizCorners.hero,
                child: Stack(
                  fit: StackFit.expand,
                  children: [
                    Image.asset('assets/pet/miso-cover.png', fit: BoxFit.cover)
                        .animate(
                          onPlay: (controller) =>
                              controller.repeat(reverse: true),
                        )
                        .scaleXY(
                          begin: 1,
                          end: 1.03,
                          duration: 5200.ms,
                          curve: Curves.easeInOut,
                        )
                        .moveY(
                          begin: 3,
                          end: -3,
                          duration: 4200.ms,
                          curve: Curves.easeInOut,
                        ),
                    const DecoratedBox(
                      decoration: BoxDecoration(
                        gradient: LinearGradient(
                          begin: Alignment.topCenter,
                          end: Alignment.bottomCenter,
                          colors: [
                            Color(0x0007100E),
                            Color(0x0007100E),
                            Color(0xE807100E),
                          ],
                          stops: [0, 0.5, 1],
                        ),
                      ),
                    ),
                    const Positioned(
                      left: 18,
                      top: 18,
                      child: GizTag(
                        label: 'Curious today',
                        backgroundColor: Color(0xEFFFFFFF),
                        foregroundColor: GizColors.ink,
                      ),
                    ),
                    Positioned(
                      left: 20,
                      right: 20,
                      bottom: 22,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            'Miso',
                            style: GizText.pageTitle.copyWith(
                              color: GizColors.surface,
                            ),
                          ),
                          const SizedBox(height: 5),
                          Text(
                            'Level 7  |  620 friendship XP',
                            style: GizText.body.copyWith(
                              color: const Color(0xCFFFFFFF),
                            ),
                          ),
                          const SizedBox(height: 14),
                          const _GizProgress(value: 0.62),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 16),
            Row(
              children: [
                Expanded(
                  child: _PetStat(
                    label: 'Mood',
                    value: 'Bright',
                    color: GizColors.accent,
                    icon: GizIcons.sun_max_fill,
                  ),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: _PetStat(
                    label: 'Streak',
                    value: '9 days',
                    color: const Color(0xFFFFDDD2),
                    icon: GizIcons.flame_fill,
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _GizProgress extends StatelessWidget {
  const _GizProgress({required this.value});

  final double value;

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 6,
      decoration: BoxDecoration(
        color: const Color(0x3DFFFFFF),
        borderRadius: BorderRadius.circular(3),
      ),
      alignment: Alignment.centerLeft,
      child: FractionallySizedBox(
        widthFactor: value,
        child: Container(
          decoration: BoxDecoration(
            color: GizColors.accent,
            borderRadius: BorderRadius.circular(3),
          ),
        ),
      ),
    );
  }
}

class _PetStat extends StatelessWidget {
  const _PetStat({
    required this.label,
    required this.value,
    required this.color,
    required this.icon,
  });

  final String label;
  final String value;
  final Color color;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    return GizSquircle(
      borderRadius: GizCorners.compactCard,
      child: Container(
        height: 92,
        padding: const EdgeInsets.all(14),
        color: color,
        child: Row(
          children: [
            Icon(icon, size: 24, color: GizColors.ink),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(label, style: GizText.label),
                  const SizedBox(height: 4),
                  Text(value, style: GizText.title),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class MePage extends StatelessWidget {
  const MePage({super.key});

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final publicKey = data.clientPublicKey;
    final status = _identityConnectionStatus(data.connectionState);
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: ListView(
          key: const PageStorageKey('me-scroll'),
          padding: const EdgeInsets.only(top: 12, bottom: 112),
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 0, 12, 0),
              child: Row(
                children: [
                  const Expanded(
                    child: Text('Identity', style: GizText.pageTitle),
                  ),
                  GizPageActionButton(
                    key: const ValueKey('identity-scan-server-qr'),
                    icon: GizIcons.qr_code,
                    semanticLabel: 'Scan server QR code',
                    onPressed: () => _scanServer(context, data),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 18),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: GizSquircle(
                borderRadius: GizCorners.card,
                child: Container(
                  padding: const EdgeInsets.all(18),
                  decoration: const BoxDecoration(
                    gradient: LinearGradient(
                      begin: Alignment.topLeft,
                      end: Alignment.bottomRight,
                      colors: [
                        GizColors.primaryHighlight,
                        GizColors.primaryShadow,
                      ],
                    ),
                  ),
                  child: Row(
                    children: [
                      const _ProfileMark(),
                      const SizedBox(width: 14),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            const Text(
                              'This device',
                              style: TextStyle(
                                fontFamily: 'NotoSansSC',
                                color: GizColors.surface,
                                fontSize: 17,
                                fontWeight: FontWeight.w700,
                                letterSpacing: 0,
                              ),
                            ),
                            const SizedBox(height: 4),
                            Text(
                              publicKey == null
                                  ? 'Device identity ready'
                                  : _compactIdentity(publicKey),
                              style: const TextStyle(
                                fontFamily: 'NotoSansSC',
                                color: Color(0xAFFFFFFF),
                                fontSize: 13,
                                letterSpacing: 0,
                              ),
                            ),
                          ],
                        ),
                      ),
                      _IdentityStatusPill(
                        label: status.label,
                        color: status.color,
                      ),
                    ],
                  ),
                ),
              ),
            ),
            const SizedBox(height: 28),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: Text(
                'CLIENT',
                style: GizText.label.copyWith(color: GizColors.secondaryInk),
              ),
            ),
            const SizedBox(height: 8),
            SettingsRow(
              icon: GizIcons.person_crop_circle,
              title: 'Public identity',
              value: publicKey == null
                  ? 'Generated on this device'
                  : _compactIdentity(publicKey),
              onPressed: publicKey == null
                  ? null
                  : () => Clipboard.setData(ClipboardData(text: publicKey)),
            ),
            const SettingsRow(
              icon: GizIcons.lock_shield,
              title: 'Private key',
              value: 'Protected by device secure storage',
            ),
            const SizedBox(height: 26),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: Text(
                'CONNECTION',
                style: GizText.label.copyWith(color: GizColors.secondaryInk),
              ),
            ),
            const SizedBox(height: 8),
            if (!data.hasActiveServer)
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 10),
                child: Text(
                  'Choose a server to finish setup and continue.',
                  key: const ValueKey('server-setup-required'),
                  style: GizText.body.copyWith(color: GizColors.secondaryInk),
                ),
              ),
            SettingsRow(
              key: const ValueKey('server-settings-row'),
              icon: GizIcons.antenna_radiowaves_left_right,
              title: 'Server',
              value: data.activeServer == null
                  ? 'Choose a server'
                  : '${data.activeServer!.name} · ${data.activeServer!.accessPoint}',
              onPressed: () => context.push('/identity/servers'),
            ),
            const SizedBox(height: 18),
            SettingsRow(
              icon: GizIcons.arrow_2_circlepath,
              title: 'Transport',
              value: 'WebRTC · ${status.label}',
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _scanServer(
    BuildContext context,
    MobileDataController data,
  ) async {
    final server = await context.push<GizClawServer>('/identity/scan');
    if (!context.mounted || server == null) return;
    try {
      await data.addOrSelectServer(
        name: server.name,
        accessPoint: server.accessPoint,
      );
    } catch (_) {
      if (!context.mounted) return;
      await showCupertinoDialog<void>(
        context: context,
        builder: (dialogContext) => CupertinoAlertDialog(
          title: const Text('Could not add server'),
          content: const Text('Could not add the server. Please try again.'),
          actions: [
            CupertinoDialogAction(
              onPressed: () => Navigator.pop(dialogContext),
              child: const Text('OK'),
            ),
          ],
        ),
      );
    }
  }
}

class _IdentityStatusPill extends StatelessWidget {
  const _IdentityStatusPill({required this.label, required this.color});

  final String label;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.16),
        borderRadius: BorderRadius.circular(99),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 7,
            height: 7,
            decoration: BoxDecoration(color: color, shape: BoxShape.circle),
          ),
          const SizedBox(width: 6),
          Text(label, style: GizText.label.copyWith(color: GizColors.surface)),
        ],
      ),
    );
  }
}

class _ProfileMark extends StatelessWidget {
  const _ProfileMark();

  @override
  Widget build(BuildContext context) {
    return GizSquircle(
      borderRadius: GizCorners.icon(54),
      child: Container(
        width: 54,
        height: 54,
        alignment: Alignment.center,
        color: const Color(0xFFDCEEFF),
        child: Text(
          'GC',
          style: GizText.title.copyWith(color: GizColors.primaryShadow),
        ),
      ),
    );
  }
}

class SettingsRow extends StatelessWidget {
  const SettingsRow({
    super.key,
    required this.icon,
    required this.title,
    required this.value,
    this.onPressed,
  });

  final IconData icon;
  final String title;
  final String value;
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) {
    return GizListRow(
      leading: SizedBox(
        width: 36,
        height: 36,
        child: Icon(icon, size: 22, color: GizColors.primary),
      ),
      title: title,
      subtitle: value,
      onPressed: onPressed,
    );
  }
}

({String label, Color color}) _identityConnectionStatus(
  MobileConnectionState state,
) => switch (state) {
  MobileConnectionState.connected => (
    label: 'Connected',
    color: GizColors.success,
  ),
  MobileConnectionState.connecting => (
    label: 'Connecting',
    color: GizColors.coral,
  ),
  MobileConnectionState.offline => (label: 'Offline', color: GizColors.coral),
  MobileConnectionState.unconfigured => (
    label: 'Setup',
    color: GizColors.lavender,
  ),
};

String _compactIdentity(String value) {
  if (value.length <= 18) return value;
  return '${value.substring(0, 10)}…${value.substring(value.length - 6)}';
}
