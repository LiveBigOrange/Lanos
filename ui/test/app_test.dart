import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:lanos/l10n/app_localizations.dart';
import 'package:lanos/main.dart';
import 'package:lanos/services/api_client.dart';
import 'package:lanos/services/lifecycle_controller.dart';

class _NoopLifecycleController implements LifecycleController {
  @override
  Future<ApiClient> start() async =>
      Completer<ApiClient>().future; // Never completes (loading state).
}

void main() {
  testWidgets('App boot renders loading screen', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        locale: const Locale('zh'),
        home: LanosHome(lifecycleController: _NoopLifecycleController()),
      ),
    );

    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    expect(find.text('正在启动 Lanos 守护进程...'), findsOneWidget);
  });
}
