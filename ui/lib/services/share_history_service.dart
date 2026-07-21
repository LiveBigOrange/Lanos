import 'dart:async';

import 'package:flutter/foundation.dart';

import '../models/share_record.dart';
import 'api_client.dart';

class ShareHistoryService extends ChangeNotifier {
  ShareHistoryService(
    ApiClient api, {
    String query = '',
    int limit = 100,
    String sortBy = 'time',
    String order = 'desc',
  }) {
    _fetcher = () => api.get('/api/v1/shares/history'
        '?q=${Uri.encodeComponent(query)}'
        '&limit=$limit'
        '&sort_by=$sortBy'
        '&order=$order');
  }

  @visibleForTesting
  ShareHistoryService.withFetcher(this._fetcher);

  late final Future<Map<String, dynamic>> Function() _fetcher;

  List<ShareRecord> _items = [];
  List<ShareRecord> get items => _items;

  Timer? _timer;
  bool _busy = false;
  bool _disposed = false;

  void start() {
    if (_timer != null) return;
    _doFetch();
    _timer = Timer.periodic(const Duration(seconds: 5), (_) => _doFetch());
  }

  void stop() {
    _timer?.cancel();
    _timer = null;
  }

  Future<void> refresh() => _doFetch();

  Future<void> _doFetch() async {
    if (_busy || _disposed) return;
    _busy = true;
    try {
      final result = await _fetcher();
      if (_disposed) return;
      final list = result['shares'] as List<dynamic>? ?? [];
      _items = list
          .map((j) => ShareRecord.fromJson(j as Map<String, dynamic>))
          .toList();
      notifyListeners();
    } catch (_) {
    } finally {
      _busy = false;
    }
  }

  @override
  void dispose() {
    _disposed = true;
    stop();
    super.dispose();
  }
}
