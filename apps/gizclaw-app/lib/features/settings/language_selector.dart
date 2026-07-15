import 'package:flutter/cupertino.dart';

import '../../app/app_locale_controller.dart';
import '../../l10n/l10n.dart';

class LanguageSelectorButton extends StatelessWidget {
  const LanguageSelectorButton({super.key, this.compact = false});

  final bool compact;

  @override
  Widget build(BuildContext context) {
    final locale = AppLocaleScope.watch(context);
    return CupertinoButton(
      padding: compact
          ? const EdgeInsets.symmetric(horizontal: 12, vertical: 8)
          : null,
      onPressed: () => showLanguageSelector(context),
      child: Text(_preferenceLabel(context, locale.preference)),
    );
  }
}

Future<void> showLanguageSelector(BuildContext context) async {
  final controller = AppLocaleScope.read(context);
  final selected = await showCupertinoModalPopup<AppLanguagePreference>(
    context: context,
    builder: (sheetContext) => CupertinoActionSheet(
      title: Text(sheetContext.l10n.chooseLanguage),
      actions: AppLanguagePreference.values
          .map(
            (preference) => CupertinoActionSheetAction(
              onPressed: () => Navigator.of(sheetContext).pop(preference),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Text(_preferenceLabel(sheetContext, preference)),
                  if (preference == controller.preference) ...[
                    const SizedBox(width: 8),
                    const Icon(CupertinoIcons.check_mark, size: 18),
                  ],
                ],
              ),
            ),
          )
          .toList(growable: false),
      cancelButton: CupertinoActionSheetAction(
        onPressed: () => Navigator.of(sheetContext).pop(),
        child: Text(sheetContext.l10n.commonCancel),
      ),
    ),
  );
  if (selected == null || selected == controller.preference) return;
  try {
    await controller.setPreference(selected);
  } catch (_) {
    if (!context.mounted) return;
    await showCupertinoDialog<void>(
      context: context,
      builder: (dialogContext) => CupertinoAlertDialog(
        title: Text(dialogContext.l10n.languageSaveFailedTitle),
        content: Text(dialogContext.l10n.languageSaveFailedMessage),
        actions: [
          CupertinoDialogAction(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: Text(dialogContext.l10n.commonOk),
          ),
        ],
      ),
    );
  }
}

String _preferenceLabel(
  BuildContext context,
  AppLanguagePreference preference,
) => switch (preference) {
  AppLanguagePreference.system => context.l10n.languageSystemDefault,
  AppLanguagePreference.english => context.l10n.languageEnglish,
  AppLanguagePreference.simplifiedChinese =>
    context.l10n.languageSimplifiedChinese,
};
