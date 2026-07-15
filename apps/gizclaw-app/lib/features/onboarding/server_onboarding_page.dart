import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';

import '../../giz_ui/giz_ui.dart';

class ServerOnboardingPage extends StatefulWidget {
  const ServerOnboardingPage({super.key});

  @override
  State<ServerOnboardingPage> createState() => _ServerOnboardingPageState();
}

class _ServerOnboardingPageState extends State<ServerOnboardingPage> {
  static const _features = [
    _OnboardingFeature(
      id: 'daily-companion',
      imagePath: 'assets/workflows/daily-companion.png',
      title: 'Agents that feel close',
      description:
          'Talk naturally with always-ready companions for planning, ideas, and everyday help.',
      eyebrow: 'ALWAYS READY',
      articleTitle: 'Built around your day',
      articleBody:
          'Keep a personal agent close for quick questions, planning, and the small decisions that keep your day moving.',
      articleHighlight:
          'Your conversations stay connected through the GizClaw server you choose.',
    ),
    _OnboardingFeature(
      id: 'flowcraft-studio',
      imagePath: 'assets/workflows/flowcraft-studio.png',
      title: 'Workflows that move with you',
      description:
          'Turn reusable workflows into structured work you can run from any connected device.',
      eyebrow: 'REUSABLE WORK',
      articleTitle: 'Make great work repeatable',
      articleBody:
          'Build a workflow once, then launch the same structured process whenever you need it—from your phone or another connected device.',
      articleHighlight:
          'Carry the process between devices without rebuilding it every time.',
    ),
    _OnboardingFeature(
      id: 'realtime-lab',
      imagePath: 'assets/workflows/realtime-lab.png',
      title: 'Realtime by design',
      description:
          'Run low-latency voice sessions while your server keeps every device in the loop.',
      eyebrow: 'LOW LATENCY',
      articleTitle: 'Voice that keeps up',
      articleBody:
          'Start a natural voice session and let GizClaw coordinate the realtime experience across your connected devices.',
      articleHighlight:
          'Fast responses, one server, and every connected device in the loop.',
    ),
  ];

  late final PageController _pageController;
  int _pageIndex = 0;

  @override
  void initState() {
    super.initState();
    _pageController = PageController(viewportFraction: 0.88);
  }

  @override
  void dispose() {
    _pageController.dispose();
    super.dispose();
  }

  void _openFeature(_OnboardingFeature feature) {
    Navigator.of(context).push(
      CupertinoPageRoute<void>(
        builder: (context) => _OnboardingArticlePage(feature: feature),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      child: DecoratedBox(
        decoration: const BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [Color(0xFFEAF4FF), GizColors.canvas],
            stops: [0, 0.54],
          ),
        ),
        child: SafeArea(
          child: Column(
            children: [
              const _OnboardingHeader(),
              const SizedBox(height: 18),
              Expanded(
                child: PageView.builder(
                  key: const ValueKey('server-onboarding-features'),
                  controller: _pageController,
                  itemCount: _features.length,
                  onPageChanged: (index) => setState(() => _pageIndex = index),
                  itemBuilder: (context, index) => Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 6),
                    child: _FeatureCard(
                      feature: _features[index],
                      onImagePressed: () => _openFeature(_features[index]),
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 14),
              _PageIndicator(count: _features.length, selected: _pageIndex),
              const SizedBox(height: 18),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 14),
                child: Column(
                  children: [
                    SizedBox(
                      width: double.infinity,
                      child: CupertinoButton.filled(
                        key: const ValueKey('server-onboarding-cta'),
                        borderRadius: BorderRadius.circular(18),
                        padding: const EdgeInsets.symmetric(vertical: 16),
                        onPressed: () => context.push('/setup/servers'),
                        child: const Text('Get Started by Adding a Server'),
                      ),
                    ),
                    const SizedBox(height: 10),
                    Text(
                      'Choose a preset, enter an access point, or scan a QR code.',
                      textAlign: TextAlign.center,
                      style: GizText.label.copyWith(
                        color: GizColors.secondaryInk,
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _OnboardingHeader extends StatelessWidget {
  const _OnboardingHeader();

  @override
  Widget build(BuildContext context) {
    return const Padding(
      padding: EdgeInsets.fromLTRB(24, 18, 24, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              GizIconTile(
                icon: GizIcons.sparkles,
                backgroundColor: GizColors.primary,
                foregroundColor: GizColors.surface,
                size: 42,
                iconSize: 21,
              ),
              SizedBox(width: 10),
              Text('GIZCLAW', style: GizText.label),
            ],
          ),
          SizedBox(height: 18),
          Text('Your agents, everywhere.', style: GizText.hero),
          SizedBox(height: 10),
          Text(
            'Connect to a GizClaw server to unlock voice, workflows, and companions across your devices.',
            style: GizText.body,
          ),
        ],
      ),
    );
  }
}

class _FeatureCard extends StatelessWidget {
  const _FeatureCard({required this.feature, required this.onImagePressed});

  final _OnboardingFeature feature;
  final VoidCallback onImagePressed;

  @override
  Widget build(BuildContext context) {
    return GizSquircle(
      borderRadius: GizCorners.hero,
      child: DecoratedBox(
        decoration: const BoxDecoration(
          color: GizColors.surface,
          boxShadow: [
            BoxShadow(
              color: Color(0x1A2F607A),
              blurRadius: 30,
              offset: Offset(0, 14),
            ),
          ],
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              child: SizedBox(
                width: double.infinity,
                child: CupertinoButton(
                  key: ValueKey('onboarding-story-${feature.id}'),
                  minimumSize: Size.zero,
                  padding: EdgeInsets.zero,
                  pressedOpacity: 0.86,
                  onPressed: onImagePressed,
                  child: Semantics(
                    label: 'Read ${feature.title}',
                    button: true,
                    excludeSemantics: true,
                    child: Stack(
                      fit: StackFit.expand,
                      children: [
                        Hero(
                          tag: feature.heroTag,
                          child: Image.asset(
                            feature.imagePath,
                            fit: BoxFit.cover,
                            semanticLabel: feature.title,
                          ),
                        ),
                        Positioned(
                          right: 12,
                          bottom: 12,
                          child: IgnorePointer(
                            child: Container(
                              padding: const EdgeInsets.symmetric(
                                horizontal: 11,
                                vertical: 8,
                              ),
                              decoration: BoxDecoration(
                                color: const Color(0xD913211C),
                                borderRadius: BorderRadius.circular(99),
                              ),
                              child: const Row(
                                mainAxisSize: MainAxisSize.min,
                                children: [
                                  Text(
                                    'READ STORY',
                                    style: TextStyle(
                                      fontFamily: 'NotoSansSC',
                                      color: GizColors.surface,
                                      fontSize: 10,
                                      fontWeight: FontWeight.w800,
                                      letterSpacing: 0.7,
                                    ),
                                  ),
                                  SizedBox(width: 5),
                                  Icon(
                                    GizIcons.arrow_up_right,
                                    size: 13,
                                    color: GizColors.surface,
                                  ),
                                ],
                              ),
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 20),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(feature.title, style: GizText.sectionTitle),
                  const SizedBox(height: 6),
                  Text(
                    feature.description,
                    style: GizText.body.copyWith(color: GizColors.secondaryInk),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _OnboardingArticlePage extends StatelessWidget {
  const _OnboardingArticlePage({required this.feature});

  final _OnboardingFeature feature;

  @override
  Widget build(BuildContext context) {
    return CupertinoPageScaffold(
      backgroundColor: GizColors.canvas,
      navigationBar: CupertinoNavigationBar(
        middle: Text(feature.title),
        border: null,
        transitionBetweenRoutes: false,
      ),
      child: SafeArea(
        child: ListView(
          key: ValueKey('onboarding-article-${feature.id}'),
          padding: const EdgeInsets.only(bottom: 40),
          children: [
            Hero(
              tag: feature.heroTag,
              child: AspectRatio(
                aspectRatio: 1.28,
                child: Image.asset(
                  feature.imagePath,
                  fit: BoxFit.cover,
                  semanticLabel: feature.title,
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 26, 24, 0),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    feature.eyebrow,
                    style: GizText.label.copyWith(
                      color: GizColors.primaryShadow,
                      letterSpacing: 1.1,
                    ),
                  ),
                  const SizedBox(height: 10),
                  Text(feature.articleTitle, style: GizText.pageTitle),
                  const SizedBox(height: 14),
                  Text(
                    feature.articleBody,
                    style: GizText.body.copyWith(
                      color: GizColors.secondaryInk,
                      fontSize: 16,
                      height: 1.5,
                    ),
                  ),
                  const SizedBox(height: 24),
                  GizSquircle(
                    borderRadius: GizCorners.card,
                    child: Container(
                      width: double.infinity,
                      padding: const EdgeInsets.all(18),
                      color: const Color(0xFFE4F1FF),
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const GizIconTile(
                            icon: GizIcons.sparkles,
                            backgroundColor: GizColors.primary,
                            foregroundColor: GizColors.surface,
                            size: 38,
                            iconSize: 18,
                          ),
                          const SizedBox(width: 13),
                          Expanded(
                            child: Text(
                              feature.articleHighlight,
                              style: GizText.body.copyWith(
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _PageIndicator extends StatelessWidget {
  const _PageIndicator({required this.count, required this.selected});

  final int count;
  final int selected;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        for (var index = 0; index < count; index++)
          AnimatedContainer(
            duration: const Duration(milliseconds: 180),
            width: selected == index ? 22 : 7,
            height: 7,
            margin: const EdgeInsets.symmetric(horizontal: 3),
            decoration: BoxDecoration(
              color: selected == index
                  ? GizColors.primary
                  : GizColors.separator,
              borderRadius: BorderRadius.circular(99),
            ),
          ),
      ],
    );
  }
}

class _OnboardingFeature {
  const _OnboardingFeature({
    required this.id,
    required this.imagePath,
    required this.title,
    required this.description,
    required this.eyebrow,
    required this.articleTitle,
    required this.articleBody,
    required this.articleHighlight,
  });

  final String articleBody;
  final String articleHighlight;
  final String articleTitle;
  final String description;
  final String eyebrow;
  final String id;
  final String imagePath;
  final String title;

  String get heroTag => 'onboarding-feature-$id';
}
