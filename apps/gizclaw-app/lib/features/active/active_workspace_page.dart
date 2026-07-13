import 'dart:async';

import 'package:flutter/cupertino.dart';

import '../../app/global_conversation_control.dart';
import '../../data/mobile_data_controller.dart';
import '../../giz_ui/giz_ui.dart';
import '../chats/chat_pages.dart';
import '../pet/pet_page.dart';

class ActiveWorkspacePage extends StatefulWidget {
  const ActiveWorkspacePage({super.key});

  @override
  State<ActiveWorkspacePage> createState() => _ActiveWorkspacePageState();
}

class _ActiveWorkspacePageState extends State<ActiveWorkspacePage> {
  String? _workspaceName;
  MobileWorkspaceDestination? _destination;
  Object? _error;
  int _request = 0;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final data = MobileDataScope.watch(context);
    final workspaceName = data.activeWorkspaceName;
    if (workspaceName == _workspaceName) {
      final cached = workspaceName == null
          ? null
          : data.cachedDestinationForWorkspace(workspaceName);
      if (cached != null && !_sameDestination(cached, _destination)) {
        _destination = cached;
        _error = null;
        _request += 1;
      }
      return;
    }
    _workspaceName = workspaceName;
    _destination = null;
    _error = null;
    final request = ++_request;
    if (workspaceName != null) {
      unawaited(_resolve(data, workspaceName, request));
    }
  }

  bool _sameDestination(
    MobileWorkspaceDestination left,
    MobileWorkspaceDestination? right,
  ) {
    return right != null &&
        left.surface == right.surface &&
        left.workspaceName == right.workspaceName &&
        left.resourceId == right.resourceId &&
        left.driver == right.driver;
  }

  Future<void> _resolve(
    MobileDataController data,
    String workspaceName,
    int request,
  ) async {
    try {
      final destination = await data.destinationForWorkspace(workspaceName);
      if (!mounted || request != _request) return;
      setState(() => _destination = destination);
    } catch (error) {
      if (!mounted || request != _request) return;
      setState(() => _error = error);
    }
  }

  @override
  Widget build(BuildContext context) {
    final data = MobileDataScope.watch(context);
    final workspaceName = data.activeWorkspaceName;
    final destination = _destination;
    if (workspaceName != null &&
        destination != null &&
        destination.workspaceName == workspaceName) {
      return switch (destination.surface) {
        MobileWorkspaceSurface.pet => PetDetailPage(
          key: ValueKey('active-pet-${destination.resourceId}'),
          petId: destination.resourceId!,
        ),
        MobileWorkspaceSurface.friend ||
        MobileWorkspaceSurface.group => ChatroomWorkspacePage(
          key: ValueKey('active-chatroom-$workspaceName'),
          workspaceName: workspaceName,
        ),
        MobileWorkspaceSurface.raid => WorkspaceChatPage(
          key: ValueKey('active-raid-$workspaceName'),
          workspaceName: workspaceName,
        ),
      };
    }

    final loading =
        workspaceName != null ||
        data.refreshing ||
        data.connectionState == MobileConnectionState.connecting;
    return _ActiveWorkspaceStatus(loading: loading, error: _error);
  }
}

class _ActiveWorkspaceStatus extends StatelessWidget {
  const _ActiveWorkspaceStatus({required this.loading, required this.error});

  final Object? error;
  final bool loading;

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    final textColor = dark ? CupertinoColors.white : GizColors.ink;
    final muted = dark ? const Color(0xFF94A39C) : GizColors.secondaryInk;
    return CupertinoPageScaffold(
      backgroundColor: dark ? const Color(0xFF0A100D) : GizColors.canvas,
      child: SafeArea(
        bottom: false,
        child: Padding(
          padding: EdgeInsets.only(
            bottom: GlobalConversationOverlay.bottomContentInset(context),
          ),
          child: Center(
            child: AnimatedSwitcher(
              duration: const Duration(milliseconds: 240),
              child: loading
                  ? const CupertinoActivityIndicator(
                      key: ValueKey('active-workspace-loading'),
                    )
                  : Column(
                      key: const ValueKey('active-workspace-empty'),
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(
                          error == null
                              ? CupertinoIcons.waveform
                              : CupertinoIcons.exclamationmark_circle,
                          size: 34,
                          color: muted,
                        ),
                        const SizedBox(height: 12),
                        Text(
                          error == null
                              ? 'No active conversation'
                              : 'Active workspace unavailable',
                          style: GizText.title.copyWith(color: textColor),
                        ),
                      ],
                    ),
            ),
          ),
        ),
      ),
    );
  }
}
