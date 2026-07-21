import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:lanos/l10n/app_localizations.dart';
import 'package:lanos/widgets/sas_confirm_dialog.dart';

void main() {
  group('SasConfirmDialog', () {
    testWidgets('shows device name and 4-digit code', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Builder(
            builder: (context) => Scaffold(
              body: ElevatedButton(
                onPressed: () => showSasConfirmDialog(
                  context,
                  deviceName: 'My MacBook',
                  sasCode: '0420',
                ),
                child: const Text('open'),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      expect(find.text('确认配对'), findsOneWidget);
      expect(find.textContaining('My MacBook'), findsOneWidget);
      expect(find.text('0420'), findsOneWidget);
      expect(find.text('确认'), findsOneWidget);
      expect(find.text('取消'), findsOneWidget);
    });

    testWidgets('onCancel fires when cancel tapped and dialog closes',
        (tester) async {
      bool cancelled = false;
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Builder(
            builder: (context) => Scaffold(
              body: ElevatedButton(
                onPressed: () => showSasConfirmDialog(
                  context,
                  deviceName: 'Peer',
                  sasCode: '1337',
                  onCancel: () => cancelled = true,
                ),
                child: const Text('open'),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('取消'));
      await tester.pumpAndSettle();

      expect(cancelled, isTrue);
      expect(find.text('确认配对'), findsNothing);
    });

    testWidgets('onConfirm fires when confirm tapped and dialog closes',
        (tester) async {
      bool confirmed = false;
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Builder(
            builder: (context) => Scaffold(
              body: ElevatedButton(
                onPressed: () => showSasConfirmDialog(
                  context,
                  deviceName: 'Peer',
                  sasCode: '9999',
                  onConfirm: () => confirmed = true,
                ),
                child: const Text('open'),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('确认'));
      await tester.pumpAndSettle();

      expect(confirmed, isTrue);
      expect(find.text('确认配对'), findsNothing);
    });

    testWidgets('auto-times out after the given duration', (tester) async {
      bool timedOut = false;
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Builder(
            builder: (context) => Scaffold(
              body: ElevatedButton(
                onPressed: () => showSasConfirmDialog(
                  context,
                  deviceName: 'Peer',
                  sasCode: '0001',
                  timeout: const Duration(seconds: 2),
                  onTimeout: () => timedOut = true,
                ),
                child: const Text('open'),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      // Fast-forward past the 2s timeout to trigger auto-cancel, then
      // settle the dialog exit animation so the route is fully removed.
      await tester.pump(const Duration(seconds: 3));
      await tester.pumpAndSettle();

      expect(timedOut, isTrue);
      expect(find.text('确认配对'), findsNothing);
    });

    testWidgets('countdown starts at the full duration and decrements',
        (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Builder(
            builder: (context) => Scaffold(
              body: ElevatedButton(
                onPressed: () => showSasConfirmDialog(
                  context,
                  deviceName: 'Peer',
                  sasCode: '0042',
                  timeout: const Duration(seconds: 5),
                ),
                child: const Text('open'),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      expect(find.text('5 秒后自动取消'), findsOneWidget);
      await tester.pump(const Duration(seconds: 1));
      expect(find.text('4 秒后自动取消'), findsOneWidget);
    });

    testWidgets('barrier tap does not dismiss the dialog', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Builder(
            builder: (context) => Scaffold(
              body: ElevatedButton(
                onPressed: () => showSasConfirmDialog(
                  context,
                  deviceName: 'Peer',
                  sasCode: '0123',
                  timeout: const Duration(seconds: 30),
                ),
                child: const Text('open'),
              ),
            ),
          ),
        ),
      );
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      // Tap outside the dialog (the barrier).
      await tester.tapAt(const Offset(10, 10));
      await tester.pumpAndSettle();

      expect(find.text('确认配对'), findsOneWidget);
    });
  });
}
