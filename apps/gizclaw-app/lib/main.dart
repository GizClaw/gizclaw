import 'package:flutter/widgets.dart';

import 'app/giz_claw_app.dart';
import 'data/mobile_data_controller.dart';
import 'identity/app_identity_store.dart';

export 'app/giz_claw_app.dart';
export 'features/active/active_workspace_page.dart';
export 'features/chats/chat_pages.dart';
export 'features/social/social_pages.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final identityStore = AppIdentityStore();
  final profile = await identityStore.loadProfile();
  runApp(
    GizClawApp(
      dataController: MobileDataController(
        profile: profile,
        identityStore: identityStore,
      ),
    ),
  );
}
