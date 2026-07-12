import 'package:flutter/cupertino.dart';
import 'package:flutter/services.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:gizclaw/gizclaw.dart';
import 'package:go_router/go_router.dart';

import '../../data/mobile_data_controller.dart';
import '../../giz_ui/giz_ui.dart';
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
                    CupertinoButton(
                      padding: const EdgeInsets.all(8),
                      onPressed: () => _showFriendConnect(context, data),
                      child: const Icon(
                        CupertinoIcons.person_add,
                        size: 23,
                        semanticLabel: 'Add friend',
                      ),
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
      '/chats/drivers/chatroom/${Uri.encodeComponent(workspaceName)}',
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
          CupertinoIcons.ellipsis,
          size: 20,
          color: GizColors.secondaryInk,
        ),
      ),
    );
  }

  void _openChat(BuildContext context) {
    context.push(
      '/chats/drivers/chatroom/'
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
    return SafeArea(
      top: false,
      child: Container(
        decoration: BoxDecoration(
          color: background,
          borderRadius: const BorderRadius.vertical(top: Radius.circular(16)),
        ),
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 20),
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
                child: const Icon(CupertinoIcons.refresh),
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
            const Text('Pet', style: GizText.pageTitle),
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
                    icon: CupertinoIcons.sun_max_fill,
                  ),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: _PetStat(
                    label: 'Streak',
                    value: '9 days',
                    color: const Color(0xFFFFDDD2),
                    icon: CupertinoIcons.flame_fill,
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
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: ListView(
          key: const PageStorageKey('me-scroll'),
          padding: const EdgeInsets.only(top: 12, bottom: 112),
          children: [
            const Padding(
              padding: EdgeInsets.symmetric(horizontal: 20),
              child: Text('Me', style: GizText.pageTitle),
            ),
            const SizedBox(height: 18),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: GizSquircle(
                borderRadius: GizCorners.card,
                child: Container(
                  padding: const EdgeInsets.all(18),
                  color: GizColors.ink,
                  child: const Row(
                    children: [
                      _ProfileMark(),
                      SizedBox(width: 14),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              'Local client',
                              style: TextStyle(
                                fontFamily: 'Manrope',
                                color: GizColors.surface,
                                fontSize: 17,
                                fontWeight: FontWeight.w700,
                                letterSpacing: 0,
                              ),
                            ),
                            SizedBox(height: 4),
                            Text(
                              'Connected over WebRTC',
                              style: TextStyle(
                                fontFamily: 'Manrope',
                                color: Color(0xAFFFFFFF),
                                fontSize: 13,
                                letterSpacing: 0,
                              ),
                            ),
                          ],
                        ),
                      ),
                      GizSignalPulse(size: 28),
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
            const SettingsRow(
              icon: CupertinoIcons.person_crop_circle,
              title: 'Identity',
              value: 'client-local',
            ),
            const SettingsRow(
              icon: CupertinoIcons.antenna_radiowaves_left_right,
              title: 'Server',
              value: '127.0.0.1:9820',
            ),
            const SettingsRow(
              icon: CupertinoIcons.lock_shield,
              title: 'Connection',
              value: 'WebRTC',
            ),
            const SettingsRow(
              icon: CupertinoIcons.arrow_2_circlepath,
              title: 'Local cache',
              value: 'Prototype data',
            ),
          ],
        ),
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
        color: GizColors.accent,
        child: const Text('GC', style: GizText.title),
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
  });

  final IconData icon;
  final String title;
  final String value;

  @override
  Widget build(BuildContext context) {
    return GizListRow(
      leading: SizedBox(
        width: 36,
        height: 36,
        child: Icon(icon, size: 22, color: GizColors.ink),
      ),
      title: title,
      subtitle: value,
      onPressed: () {},
    );
  }
}
