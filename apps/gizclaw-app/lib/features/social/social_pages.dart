import 'package:flutter/cupertino.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:go_router/go_router.dart';

import '../../data/mobile_data_controller.dart';
import '../../giz_ui/giz_ui.dart';
import '../../prototype/prototype_models.dart';

class FriendsPage extends StatelessWidget {
  const FriendsPage({super.key});

  @override
  Widget build(BuildContext context) {
    final friendChats = MobileDataScope.watch(context).chatroomWorkspaces
        .where((item) => item.kind == ChatroomWorkspaceKind.direct)
        .toList(growable: false);
    return CupertinoPageScaffold(
      child: SafeArea(
        bottom: false,
        child: CustomScrollView(
          key: const PageStorageKey('friends-scroll'),
          slivers: [
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
                      onPressed: () {},
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
                  return FriendRow(friend: friendChats[index], index: index)
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
}

class FriendRow extends StatelessWidget {
  const FriendRow({super.key, required this.friend, required this.index});

  final ChatroomWorkspaceMetadata friend;
  final int index;

  @override
  Widget build(BuildContext context) {
    const avatarColors = [
      Color(0xFFFFDCD0),
      Color(0xFFD9F2EA),
      Color(0xFFD9E8FF),
    ];
    return GizListRow(
      leading: Container(
        width: 52,
        height: 52,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: avatarColors[index % avatarColors.length],
          borderRadius: BorderRadius.circular(8),
        ),
        child: Text(
          friend.title.substring(0, 1).toUpperCase(),
          style: GizText.sectionTitle,
        ),
      ),
      title: friend.title,
      subtitle: 'Direct chat',
      onPressed: () => _openChat(context),
      trailing: Container(
        width: 40,
        height: 40,
        alignment: Alignment.center,
        decoration: const BoxDecoration(
          color: Color(0xFFE3E8E1),
          shape: BoxShape.circle,
        ),
        child: const Icon(
          CupertinoIcons.chat_bubble,
          size: 18,
          color: GizColors.ink,
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
}

class PetPage extends StatelessWidget {
  const PetPage({super.key});

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
              child: ClipRRect(
                borderRadius: BorderRadius.circular(12),
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
    return Container(
      height: 92,
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: color,
        borderRadius: BorderRadius.circular(8),
      ),
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
              child: Container(
                padding: const EdgeInsets.all(18),
                decoration: BoxDecoration(
                  color: GizColors.ink,
                  borderRadius: BorderRadius.circular(8),
                ),
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
    return Container(
      width: 54,
      height: 54,
      alignment: Alignment.center,
      decoration: const BoxDecoration(
        color: GizColors.accent,
        shape: BoxShape.circle,
      ),
      child: const Text('GC', style: GizText.title),
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
