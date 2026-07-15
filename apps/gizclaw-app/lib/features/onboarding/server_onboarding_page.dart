import 'package:flutter/cupertino.dart';
import 'package:go_router/go_router.dart';

import '../../giz_ui/giz_ui.dart';
import '../../l10n/l10n.dart';
import '../settings/language_selector.dart';

class ServerOnboardingPage extends StatefulWidget {
  const ServerOnboardingPage({super.key});

  @override
  State<ServerOnboardingPage> createState() => _ServerOnboardingPageState();
}

class _ServerOnboardingPageState extends State<ServerOnboardingPage> {
  List<_OnboardingFeature> _features(BuildContext context) => [
    _OnboardingFeature(
      id: 'daily-companion',
      imagePath: 'assets/workflows/daily-companion.png',
      title: context.l10n.onboardingAgentsTitle,
      description: context.l10n.onboardingAgentsDescription,
      eyebrow: context.l10n.onboardingAgentsEyebrow,
      articleTitle: context.l10n.onboardingAgentsArticleTitle,
      articleBody: context.l10n.onboardingAgentsArticleBody,
      articleHighlight: context.l10n.onboardingAgentsArticleHighlight,
    ),
    _OnboardingFeature(
      id: 'flowcraft-studio',
      imagePath: 'assets/workflows/flowcraft-studio.png',
      title: context.l10n.onboardingWorkflowsTitle,
      description: context.l10n.onboardingWorkflowsDescription,
      eyebrow: context.l10n.onboardingWorkflowsEyebrow,
      articleTitle: context.l10n.onboardingWorkflowsArticleTitle,
      articleBody: context.l10n.onboardingWorkflowsArticleBody,
      articleHighlight: context.l10n.onboardingWorkflowsArticleHighlight,
    ),
    _OnboardingFeature(
      id: 'realtime-lab',
      imagePath: 'assets/workflows/realtime-lab.png',
      title: context.l10n.onboardingRealtimeTitle,
      description: context.l10n.onboardingRealtimeDescription,
      eyebrow: context.l10n.onboardingRealtimeEyebrow,
      articleTitle: context.l10n.onboardingRealtimeArticleTitle,
      articleBody: context.l10n.onboardingRealtimeArticleBody,
      articleHighlight: context.l10n.onboardingRealtimeArticleHighlight,
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
    final features = _features(context);
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
                  itemCount: features.length,
                  onPageChanged: (index) => setState(() => _pageIndex = index),
                  itemBuilder: (context, index) => Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 6),
                    child: _FeatureCard(
                      feature: features[index],
                      onImagePressed: () => _openFeature(features[index]),
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 14),
              _PageIndicator(count: features.length, selected: _pageIndex),
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
                        child: Text(context.l10n.onboardingConnectServer),
                      ),
                    ),
                    const SizedBox(height: 10),
                    Text(
                      context.l10n.onboardingConnectHint,
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
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 10, 24, 0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const GizIconTile(
                icon: GizIcons.sparkles,
                backgroundColor: GizColors.primary,
                foregroundColor: GizColors.surface,
                size: 42,
                iconSize: 21,
              ),
              const SizedBox(width: 10),
              const Text('GIZCLAW', style: GizText.label),
              const Spacer(),
              const LanguageSelectorButton(compact: true),
            ],
          ),
          const SizedBox(height: 10),
          Text(context.l10n.onboardingHeroTitle, style: GizText.hero),
          const SizedBox(height: 10),
          Text(context.l10n.onboardingHeroDescription, style: GizText.body),
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
                    label: context.l10n.onboardingReadStory(
                      title: feature.title,
                    ),
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
                              child: Row(
                                mainAxisSize: MainAxisSize.min,
                                children: [
                                  Text(
                                    context.l10n.onboardingReadStoryAction,
                                    style: const TextStyle(
                                      fontFamily: 'NotoSansSC',
                                      color: GizColors.surface,
                                      fontSize: 10,
                                      fontWeight: FontWeight.w800,
                                      letterSpacing: 0.7,
                                    ),
                                  ),
                                  const SizedBox(width: 5),
                                  const Icon(
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
