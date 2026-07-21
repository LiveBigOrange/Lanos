import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:lanos/main.dart';

void main() {
  testWidgets('App boot renders loading screen then fails to find gcd',
      (WidgetTester tester) async {
    // On CI there's no gcd binary, so the lifecycle controller will time out
    // after 5s and show the error screen. We verify the loading state shows.
    await tester.pumpWidget(const LanosApp());

    // Loading screen visible immediately.
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    expect(find.text('正在启动 Lanos 守护进程...'), findsOneWidget);
  });
}
