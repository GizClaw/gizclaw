import 'package:flutter/widgets.dart';

import 'app/giz_claw_app.dart';

export 'app/giz_claw_app.dart';
export 'features/chats/chat_pages.dart';
export 'features/social/social_pages.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const GizClawApp());
}
