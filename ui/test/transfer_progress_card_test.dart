import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:lanos/l10n/app_localizations.dart';
import 'package:lanos/models/device.dart';
import 'package:lanos/services/transfer_service.dart';
import 'package:lanos/utils/format.dart';
import 'package:lanos/utils/transfer_stats.dart';
import 'package:lanos/widgets/transfer_progress_card.dart';

Device _peer() => const Device(
      id: 'peer1',
      name: 'WinBox',
      platform: 'windows',
      port: 52150,
      pubHash: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
      ipVersion: '4',
    );

TransferItem _item({
  TransferDirection direction = TransferDirection.outgoing,
  TransferStatus status = TransferStatus.transferring,
  String fileName = 'report.pdf',
  int fileSize = 10 * 1024 * 1024,
  double progress = 0.4,
  String? error,
}) {
  return TransferItem(
    id: 't1',
    direction: direction,
    status: status,
    fileName: fileName,
    fileSize: fileSize,
    peer: _peer(),
    progress: progress,
    error: error,
  );
}

void main() {
  group('TransferProgressCard', () {
    testWidgets('renders filename, peer, percent and byte count',
        (tester) async {
      final stats = TransferStats(now: () => DateTime(2024, 1, 1));
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Scaffold(
            body: TransferProgressCard(item: _item(), stats: stats),
          ),
        ),
      );

      expect(find.text('report.pdf'), findsOneWidget);
      expect(find.textContaining('WinBox'), findsOneWidget);
      expect(find.text('40%'), findsOneWidget);
      expect(
        find.textContaining(formatBytes((0.4 * 10 * 1024 * 1024).round())),
        findsOneWidget,
      );
    });

    testWidgets('shows cancel button for active transfers', (tester) async {
      final stats = TransferStats(now: () => DateTime(2024, 1, 1));
      var cancelled = false;
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Scaffold(
            body: TransferProgressCard(
              item: _item(status: TransferStatus.transferring),
              stats: stats,
              onCancel: () => cancelled = true,
            ),
          ),
        ),
      );

      final cancelBtn = find.byTooltip('取消');
      expect(cancelBtn, findsOneWidget);
      await tester.tap(cancelBtn);
      expect(cancelled, isTrue);
    });

    testWidgets('hides cancel button for completed transfers', (tester) async {
      final stats = TransferStats(now: () => DateTime(2024, 1, 1));
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Scaffold(
            body: TransferProgressCard(
              item: _item(status: TransferStatus.completed, progress: 1.0),
              stats: stats,
            ),
          ),
        ),
      );

      expect(find.byTooltip('取消'), findsNothing);
      expect(find.text('已完成'), findsOneWidget);
    });

    testWidgets('shows throughput and ETA while transferring', (tester) async {
      var t = DateTime(2024, 1, 1);
      final stats = TransferStats(now: () => t);
      // 10 MB file, 2 MB done in 1s -> 2 MB/s, 8 MB left -> 4s.
      stats.record(0);
      t = t.add(const Duration(seconds: 1));
      stats.record(2 * 1024 * 1024);

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Scaffold(
            body: TransferProgressCard(
              item: _item(progress: 0.2),
              stats: stats,
            ),
          ),
        ),
      );
      await tester.pump(const Duration(milliseconds: 50));

      expect(find.textContaining('MB/s'), findsOneWidget);
      expect(find.text('4s'), findsOneWidget);
    });

    testWidgets('incoming uses down arrow direction', (tester) async {
      final stats = TransferStats(now: () => DateTime(2024, 1, 1));
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Scaffold(
            body: TransferProgressCard(
              item: _item(direction: TransferDirection.incoming),
              stats: stats,
            ),
          ),
        ),
      );
      expect(find.byIcon(Icons.arrow_downward), findsOneWidget);
      expect(find.byIcon(Icons.arrow_upward), findsNothing);
      expect(find.textContaining('WinBox'), findsOneWidget);
    });

    testWidgets('failed transfer shows error tooltip icon', (tester) async {
      final stats = TransferStats(now: () => DateTime(2024, 1, 1));
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Scaffold(
            body: TransferProgressCard(
              item:
                  _item(status: TransferStatus.failed, error: 'network reset'),
              stats: stats,
            ),
          ),
        ),
      );
      expect(find.text('失败'), findsOneWidget);
      expect(find.byIcon(Icons.error_outline), findsOneWidget);
      expect(find.byTooltip('取消'), findsNothing);
    });

    testWidgets('cancelled transfer strikes through filename', (tester) async {
      final stats = TransferStats(now: () => DateTime(2024, 1, 1));
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: Scaffold(
            body: TransferProgressCard(
              item: _item(status: TransferStatus.cancelled),
              stats: stats,
            ),
          ),
        ),
      );
      expect(find.text('已取消'), findsOneWidget);
      final nameWidget = tester.widget<Text>(find.text('report.pdf'));
      expect(nameWidget.style?.decoration, TextDecoration.lineThrough);
    });
  });
}
