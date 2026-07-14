import 'dart:ui';

import 'package:flutter/cupertino.dart';
import 'package:flutter_animate/flutter_animate.dart';

import 'giz_icons.dart';

export 'giz_icons.dart';

abstract final class GizColors {
  static const canvas = Color(0xFFF5F6F2);
  static const surface = Color(0xFFFFFFFF);
  static const ink = Color(0xFF13211C);
  static const secondaryInk = Color(0xFF64716B);
  static const separator = Color(0xFFDCE3DD);

  // System interactions use a brighter blue than message bubbles.
  static const primary = Color(0xFF2F8FFF);
  static const primaryHighlight = Color(0xFF5AABFF);
  static const primaryShadow = Color(0xFF2478DC);
  static const onPrimary = surface;
  static const accent = Color(0xFFC7EA62);
  static const onAccent = ink;
  static const messageBlue = Color(0xFF007AFF);
  static const messageIncoming = Color(0xFFE9E9EB);
  static const blue = Color(0xFF55758A);
  static const coral = Color(0xFFD78165);
  static const lavender = Color(0xFF9786B8);
  static const success = Color(0xFF42A878);
}

String gizResourceInitial(String id) {
  final normalized = id.trim();
  if (normalized.isEmpty) return '?';
  return String.fromCharCode(normalized.runes.first).toUpperCase();
}

class GizResourceInitial extends StatelessWidget {
  const GizResourceInitial({super.key, required this.id, this.size = 50});

  final String id;
  final double size;

  @override
  Widget build(BuildContext context) {
    return GizSquircle(
      key: ValueKey('resource-initial-$id'),
      borderRadius: GizCorners.icon(size),
      child: Container(
        width: size,
        height: size,
        alignment: Alignment.center,
        color: const Color(0xFFDCEEFF),
        child: Text(
          gizResourceInitial(id),
          style: GizText.sectionTitle.copyWith(color: GizColors.primaryShadow),
        ),
      ),
    );
  }
}

abstract final class GizText {
  static const hero = TextStyle(
    fontFamily: 'NotoSansSC',
    color: GizColors.ink,
    fontSize: 38,
    height: 1.04,
    fontWeight: FontWeight.w800,
    letterSpacing: 0,
  );

  static const pageTitle = TextStyle(
    fontFamily: 'NotoSansSC',
    color: GizColors.ink,
    fontSize: 30,
    height: 1.08,
    fontWeight: FontWeight.w800,
    letterSpacing: 0,
  );

  static const sectionTitle = TextStyle(
    fontFamily: 'NotoSansSC',
    color: GizColors.ink,
    fontSize: 21,
    height: 1.2,
    fontWeight: FontWeight.w800,
    letterSpacing: 0,
  );

  static const title = TextStyle(
    fontFamily: 'NotoSansSC',
    color: GizColors.ink,
    fontSize: 16,
    height: 1.3,
    fontWeight: FontWeight.w700,
    letterSpacing: 0,
  );

  static const body = TextStyle(
    fontFamily: 'NotoSansSC',
    color: GizColors.ink,
    fontSize: 14,
    height: 1.45,
    fontWeight: FontWeight.w400,
    letterSpacing: 0,
  );

  static const label = TextStyle(
    fontFamily: 'NotoSansSC',
    color: GizColors.ink,
    fontSize: 11,
    height: 1.2,
    fontWeight: FontWeight.w700,
    letterSpacing: 0,
  );
}

const gizCupertinoTheme = CupertinoThemeData(
  brightness: Brightness.light,
  primaryColor: GizColors.primary,
  primaryContrastingColor: GizColors.onPrimary,
  scaffoldBackgroundColor: GizColors.canvas,
  barBackgroundColor: Color(0xF7F5F6F2),
  textTheme: CupertinoTextThemeData(
    textStyle: GizText.body,
    actionTextStyle: GizText.title,
    navTitleTextStyle: GizText.title,
    navLargeTitleTextStyle: GizText.pageTitle,
    tabLabelTextStyle: GizText.label,
  ),
);

abstract final class GizCorners {
  static const compactCard = BorderRadius.all(Radius.circular(16));
  static const card = BorderRadius.all(Radius.circular(20));
  static const hero = BorderRadius.all(Radius.circular(26));

  static BorderRadius icon(double size) => BorderRadius.circular(size * 0.28);
}

class GizSquircle extends StatelessWidget {
  const GizSquircle({
    super.key,
    required this.child,
    this.borderRadius = GizCorners.compactCard,
    this.clipBehavior = Clip.antiAlias,
  });

  final BorderRadiusGeometry borderRadius;
  final Widget child;
  final Clip clipBehavior;

  @override
  Widget build(BuildContext context) {
    return ClipRSuperellipse(
      borderRadius: borderRadius,
      clipBehavior: clipBehavior,
      child: child,
    );
  }
}

class GizIconTile extends StatelessWidget {
  const GizIconTile({
    super.key,
    required this.icon,
    required this.backgroundColor,
    this.foregroundColor = GizColors.ink,
    this.size = 44,
    this.iconSize = 21,
  });

  final Color backgroundColor;
  final Color foregroundColor;
  final IconData icon;
  final double iconSize;
  final double size;

  @override
  Widget build(BuildContext context) {
    return GizSquircle(
      borderRadius: GizCorners.icon(size),
      child: ColoredBox(
        color: backgroundColor,
        child: SizedBox.square(
          dimension: size,
          child: Icon(icon, size: iconSize, color: foregroundColor),
        ),
      ),
    );
  }
}

class GizPageActionButton extends StatelessWidget {
  const GizPageActionButton({
    super.key,
    required this.icon,
    required this.semanticLabel,
    required this.onPressed,
    this.loading = false,
  });

  final IconData icon;
  final bool loading;
  final VoidCallback? onPressed;
  final String semanticLabel;

  @override
  Widget build(BuildContext context) {
    final enabled = onPressed != null && !loading;
    return Semantics(
      label: semanticLabel,
      button: true,
      enabled: enabled,
      excludeSemantics: true,
      child: SizedBox.square(
        dimension: 44,
        child: CupertinoButton(
          minimumSize: const Size.square(44),
          padding: const EdgeInsets.all(8),
          pressedOpacity: 0.62,
          onPressed: enabled ? onPressed : null,
          child: AnimatedSwitcher(
            duration: const Duration(milliseconds: 180),
            child: loading
                ? const CupertinoActivityIndicator(
                    key: ValueKey('page-action-loading'),
                    radius: 10,
                    color: GizColors.primary,
                  )
                : Icon(
                    icon,
                    key: const ValueKey('page-action-icon'),
                    size: 28,
                    color: enabled
                        ? GizColors.primary
                        : GizColors.secondaryInk.withValues(alpha: 0.42),
                  ),
          ),
        ),
      ),
    );
  }
}

class GizVoiceButton extends StatefulWidget {
  const GizVoiceButton({
    super.key,
    required this.enabled,
    required this.recording,
    required this.preparing,
    required this.label,
    required this.accent,
    required this.disabledColor,
    required this.foregroundColor,
    required this.disabledForegroundColor,
    required this.onStart,
    required this.onFinish,
    required this.onCancel,
    this.size = 78,
  });

  final Color accent;
  final Color disabledColor;
  final Color disabledForegroundColor;
  final bool enabled;
  final Color foregroundColor;
  final String label;
  final VoidCallback? onCancel;
  final VoidCallback? onFinish;
  final VoidCallback? onStart;
  final bool preparing;
  final bool recording;
  final double size;

  @override
  State<GizVoiceButton> createState() => _GizVoiceButtonState();
}

class _GizVoiceButtonState extends State<GizVoiceButton>
    with SingleTickerProviderStateMixin {
  late final AnimationController _pulse = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1500),
  )..repeat();

  @override
  void dispose() {
    _pulse.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Listener(
      onPointerDown: widget.enabled ? (_) => widget.onStart?.call() : null,
      onPointerUp: widget.enabled ? (_) => widget.onFinish?.call() : null,
      onPointerCancel: widget.enabled ? (_) => widget.onCancel?.call() : null,
      child: Semantics(
        button: true,
        enabled: widget.enabled,
        label: widget.label,
        child: AnimatedBuilder(
          animation: _pulse,
          builder: (context, child) {
            final energy = widget.recording ? _pulse.value : 0.0;
            return AnimatedScale(
              scale: widget.recording ? 0.92 : 1,
              duration: const Duration(milliseconds: 140),
              curve: Curves.easeOutCubic,
              child: AnimatedContainer(
                duration: const Duration(milliseconds: 180),
                width: widget.size,
                height: widget.size,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: widget.enabled ? widget.accent : widget.disabledColor,
                  border: Border.all(
                    color: widget.enabled
                        ? widget.accent
                        : widget.disabledForegroundColor.withValues(alpha: 0.3),
                    width: 1.5,
                  ),
                  boxShadow: widget.enabled
                      ? [
                          BoxShadow(
                            color: widget.accent.withValues(
                              alpha: 0.2 + energy * 0.28,
                            ),
                            blurRadius: 20 + energy * 14,
                            spreadRadius: 2 + energy * 6,
                          ),
                        ]
                      : null,
                ),
                child: child,
              ),
            );
          },
          child: widget.preparing
              ? CupertinoActivityIndicator(color: widget.foregroundColor)
              : Icon(
                  widget.recording ? GizIcons.waveform : GizIcons.mic_fill,
                  size: widget.recording ? 30 : 28,
                  color: widget.enabled
                      ? widget.foregroundColor
                      : widget.disabledForegroundColor,
                ),
        ),
      ),
    );
  }
}

class GizPressable extends StatefulWidget {
  const GizPressable({
    super.key,
    required this.onPressed,
    required this.child,
    this.borderRadius = BorderRadius.zero,
    this.pressedColor = const Color(0x10111916),
    this.scaleWhenPressed = 1,
  });

  final VoidCallback? onPressed;
  final Widget child;
  final BorderRadius borderRadius;
  final Color pressedColor;
  final double scaleWhenPressed;

  @override
  State<GizPressable> createState() => _GizPressableState();
}

class _GizPressableState extends State<GizPressable> {
  bool _pressed = false;

  void _setPressed(bool value) {
    if (_pressed == value || widget.onPressed == null) return;
    setState(() => _pressed = value);
  }

  @override
  Widget build(BuildContext context) {
    return Semantics(
      button: true,
      enabled: widget.onPressed != null,
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: widget.onPressed,
        onTapDown: (_) => _setPressed(true),
        onTapUp: (_) => _setPressed(false),
        onTapCancel: () => _setPressed(false),
        child: AnimatedScale(
          duration: 90.ms,
          curve: Curves.easeOut,
          scale: _pressed ? widget.scaleWhenPressed : 1,
          child: GizSquircle(
            borderRadius: widget.borderRadius,
            child: AnimatedContainer(
              duration: 90.ms,
              curve: Curves.easeOut,
              color: _pressed ? widget.pressedColor : const Color(0x00000000),
              child: widget.child,
            ),
          ),
        ),
      ),
    );
  }
}

class GizSectionHeader extends StatelessWidget {
  const GizSectionHeader({
    super.key,
    required this.title,
    this.actionLabel,
    this.onAction,
  });

  final String title;
  final String? actionLabel;
  final VoidCallback? onAction;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 0, 12, 0),
      child: Row(
        children: [
          Expanded(child: Text(title, style: GizText.sectionTitle)),
          if (actionLabel != null)
            CupertinoButton(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
              onPressed: onAction,
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    actionLabel!,
                    style: GizText.body.copyWith(fontWeight: FontWeight.w700),
                  ),
                  const SizedBox(width: 5),
                  const Icon(GizIcons.arrow_right, size: 17),
                ],
              ),
            ),
        ],
      ),
    );
  }
}

class GizListRow extends StatelessWidget {
  const GizListRow({
    super.key,
    required this.leading,
    required this.title,
    required this.subtitle,
    required this.onPressed,
    this.trailing,
    this.showSeparator = true,
  });

  final Widget leading;
  final String title;
  final String subtitle;
  final VoidCallback? onPressed;
  final Widget? trailing;
  final bool showSeparator;

  @override
  Widget build(BuildContext context) {
    return GizPressable(
      onPressed: onPressed,
      child: Container(
        padding: const EdgeInsets.fromLTRB(20, 14, 16, 14),
        decoration: BoxDecoration(
          border: showSeparator
              ? const Border(bottom: BorderSide(color: GizColors.separator))
              : null,
        ),
        child: Row(
          children: [
            leading,
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: GizText.title,
                  ),
                  const SizedBox(height: 4),
                  Text(
                    subtitle,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: GizText.body.copyWith(color: GizColors.secondaryInk),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 10),
            trailing ??
                (onPressed == null
                    ? const SizedBox(width: 18)
                    : const Icon(
                        GizIcons.chevron_forward,
                        size: 18,
                        color: GizColors.secondaryInk,
                      )),
          ],
        ),
      ),
    );
  }
}

class GizTag extends StatelessWidget {
  const GizTag({
    super.key,
    required this.label,
    this.backgroundColor = GizColors.ink,
    this.foregroundColor = GizColors.surface,
  });

  final String label;
  final Color backgroundColor;
  final Color foregroundColor;

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: BoxDecoration(
        color: backgroundColor,
        borderRadius: BorderRadius.circular(99),
      ),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
        child: Text(
          label.toUpperCase(),
          style: GizText.label.copyWith(color: foregroundColor, fontSize: 10),
        ),
      ),
    );
  }
}

class GizSignalPulse extends StatefulWidget {
  const GizSignalPulse({super.key, this.size = 30});

  final double size;

  @override
  State<GizSignalPulse> createState() => _GizSignalPulseState();
}

class _GizSignalPulseState extends State<GizSignalPulse>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: 1800.ms,
  )..repeat();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return SizedBox.square(
      dimension: widget.size,
      child: AnimatedBuilder(
        animation: _controller,
        builder: (context, child) {
          return Stack(
            alignment: Alignment.center,
            children: [
              for (final offset in [0.0, 0.42])
                _PulseRing(
                  progress: (_controller.value + offset) % 1,
                  size: widget.size,
                ),
              Container(
                width: widget.size * 0.28,
                height: widget.size * 0.28,
                decoration: const BoxDecoration(
                  color: GizColors.accent,
                  shape: BoxShape.circle,
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}

class _PulseRing extends StatelessWidget {
  const _PulseRing({required this.progress, required this.size});

  final double progress;
  final double size;

  @override
  Widget build(BuildContext context) {
    return Transform.scale(
      scale: 0.35 + progress * 0.65,
      child: Opacity(
        opacity: (1 - progress) * 0.7,
        child: Container(
          width: size,
          height: size,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            border: Border.all(color: GizColors.accent),
          ),
        ),
      ),
    );
  }
}

class GizGlassBar extends StatelessWidget {
  const GizGlassBar({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    return ClipRect(
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: 20, sigmaY: 20),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: dark ? const Color(0xF213211C) : const Color(0xF7F5F6F2),
            border: Border(
              top: BorderSide(
                color: dark ? const Color(0x22FFFFFF) : GizColors.separator,
              ),
            ),
          ),
          child: child,
        ),
      ),
    );
  }
}
