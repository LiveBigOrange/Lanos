import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:lanos/l10n/app_localizations.dart';
import 'package:lanos/pages/transfer_page.dart';
import 'package:lanos/services/api_client.dart';
import 'package:lanos/services/transfer_service.dart';

List<Map<String, dynamic>> _itemsJson() {
  return [
    {
      'id': 'up1',
      'direction': 'outgoing',
      'status': 'transferring',
      'file_name': 'photo.zip',
      'file_size': 50 * 1024 * 1024,
      'progress': 0.5,
      'peer': {
        'id': 'peer1',
        'name': 'MacBook',
        'platform': 'darwin',
        'port': 52150,
        'pub_hash': 'b' * 32,
        'ip_version': '46',
      },
    },
    {
      'id': 'down1',
      'direction': 'incoming',
      'status': 'transferring',
      'file_name': 'slides.pdf',
      'file_size': 5 * 1024 * 1024,
      'progress': 0.2,
      'peer': {
        'id': 'peer2',
        'name': 'WinBox',
        'platform': 'windows',
        'port': 52160,
        'pub_hash': 'c' * 32,
        'ip_version': '4',
      },
    },
  ];
}

void main() {
  group('TransferPage', () {
    testWidgets('shows empty state when no transfers', (tester) async {
      final svc = TransferService.withFetcher(
        () async => <Map<String, dynamic>>[],
        const Duration(seconds: 60),
      );
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: TransferPage(api: _DummyApi(), service: svc),
        ),
      );
      await tester.pump();
      expect(find.text('暂无传输记录'), findsOneWidget);
      svc.dispose();
    });

    testWidgets('renders dual up/down sections with cards', (tester) async {
      final svc = TransferService.withFetcher(
        () async => _itemsJson(),
        const Duration(seconds: 60),
      );
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: TransferPage(api: _DummyApi(), service: svc),
        ),
      );
      // Let the first fetch + listeners settle.
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 50));

      expect(find.textContaining('上行'), findsOneWidget);
      expect(find.textContaining('下行'), findsOneWidget);
      expect(find.text('photo.zip'), findsOneWidget);
      expect(find.text('slides.pdf'), findsOneWidget);
      // Both directions visible at once.
      expect(find.byIcon(Icons.arrow_upward), findsOneWidget);
      expect(find.byIcon(Icons.arrow_downward), findsOneWidget);
      svc.dispose();
    });

    testWidgets('cancel button invokes service cancel', (tester) async {
      final items = _itemsJson();
      final svc = TransferService.withFetcher(
        () async => items,
        const Duration(seconds: 60),
      );
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          locale: const Locale('zh'),
          home: TransferPage(api: _DummyApi(), service: svc),
        ),
      );
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 50));

      expect(find.byTooltip('取消'), findsNWidgets(2));
      // Simulate the backend update before tapping.
      items[0]['status'] = 'cancelled';
      await tester.tap(find.byTooltip('取消').first);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 50));
      // After service refreshes, outgoing should read '已取消'.
      expect(find.text('已取消'), findsOneWidget);
      svc.dispose();
    });
  });
}

class _DummyApi extends ApiClient {
  _DummyApi() : super(port: 0, token: '', version: '');
}
