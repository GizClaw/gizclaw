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
      imagePath: 'assets/workflows/daily-companion.png',
      title: 'Agents that feel close',
      description:
          'Talk naturally with always-ready companions for planning, ideas, and everyday help.',
    ),
    _OnboardingFeature(
      imagePath: 'assets/workflows/flowcraft-studio.png',
      title: 'Workflows that move with you',
      description:
          'Turn reusable workflows into structured work you can run from any connected device.',
    ),
    _OnboardingFeature(
      imagePath: 'assets/workflows/realtime-lab.png',
      title: 'Realtime by design',
      description:
          'Run low-latency voice sessions while your server keeps every device in the loop.',
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
                    child: _FeatureCard(feature: _features[index]),
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
  const _FeatureCard({required this.feature});

  final _OnboardingFeature feature;

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
                child: Image.asset(
                  feature.imagePath,
                  fit: BoxFit.cover,
                  semanticLabel: feature.title,
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
    required this.imagePath,
    required this.title,
    required this.description,
  });

  final String imagePath;
  final String title;
  final String description;
}
