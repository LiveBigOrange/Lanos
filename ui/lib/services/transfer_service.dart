import 'dart:async';

import 'package:flutter/foundation.dart';

import '../models/device.dart';
import 'api_client.dart';

enum TransferDirection { outgoing, incoming }

enum TransferStatus {
  pending,
  connecting,
  transferring,
  completed,
  failed,
  cancelled,
  awaitingResume,
}

class TransferItem {
  const TransferItem({
    required this.id,
    required this.direction,
    required this.status,
    required this.fileName,
    required this.fileSize,
    required this.peer,
    this.progress = 0.0,
    this.error,
    this.startedAt,
  });

  final String id;
  final TransferDirection direction;
  final TransferStatus status;
  final String fileName;
  final int fileSize;
  final Device peer;
  final double progress;
  final String? error;
  final DateTime? startedAt;

  TransferItem copyWith({
    TransferStatus? status,
    double? progress,
    String? error,
  }) {
    return TransferItem(
      id: id,
      direction: direction,
      status: status ?? this.status,
      fileName: fileName,
      fileSize: fileSize,
      peer: peer,
      progress: progress ?? this.progress,
      error: error ?? this.error,
      startedAt: startedAt ?? this.startedAt,
    );
  }
}

typedef TransferListFetcher = Future<List<Map<String, dynamic>>> Function();

class TransferService extends ChangeNotifier {
  TransferService(
    ApiClient api, {
    Duration interval = const Duration(seconds: 2),
  }) : this._withApi(api, interval);

  TransferService._withApi(ApiClient api, Duration interval)
      : _fetch = (() async => [await api.get('/api/v1/transfers')]),
        _interval = interval,
        _api = api;

  @visibleForTesting
  TransferService.withFetcher(
    this._fetch,
    this._interval,
  ) : _api = null;

  final TransferListFetcher _fetch;
  final Duration _interval;
  final ApiClient? _api;

  /// Fires once per incoming transfer that transitions to [completed]. Wire
  /// this to [NotificationService.onFileReceived] (P1-22).
  void Function(TransferItem)? onIncomingCompleted;

  final Set<String> _notifiedCompletedIds = {};

  List<TransferItem> _items = [];
  List<TransferItem> get items => _items;

  Timer? _timer;
  bool _fetching = false;
  bool _disposed = false;

  void start() {
    if (_timer != null) return;
    _doFetch();
    _timer = Timer.periodic(_interval, (_) => _doFetch());
  }

  void stop() {
    _timer?.cancel();
    _timer = null;
  }

  Future<void> refresh() => _doFetch();

  Future<void> cancelTransfer(String id) async {
    await _api?.post('/api/v1/transfers/$id/cancel');
    await _doFetch();
  }

  Future<void> _doFetch() async {
    if (_fetching || _disposed) return;
    _fetching = true;
    try {
      final jsonList = await _fetch();
      if (_disposed) return;
      final newItems = jsonList.map((json) => _parseItem(json)).toList();

      if (onIncomingCompleted != null) {
        for (final item in newItems) {
          if (item.direction == TransferDirection.incoming &&
              item.status == TransferStatus.completed &&
              !_notifiedCompletedIds.contains(item.id)) {
            _notifiedCompletedIds.add(item.id);
            onIncomingCompleted!(item);
          }
        }
      }
      _notifiedCompletedIds
          .retainWhere((id) => newItems.any((i) => i.id == id));

      _items = newItems;
      notifyListeners();
    } catch (_) {
      // Silently keep stale list; errors will surface when user interacts.
    } finally {
      _fetching = false;
    }
  }

  TransferItem _parseItem(Map<String, dynamic> json) {
    final dir = json['direction'] == 'outgoing'
        ? TransferDirection.outgoing
        : TransferDirection.incoming;
    final status = _parseStatus(json['status'] as String?);
    final peerJson = json['peer'] as Map<String, dynamic>?;
    return TransferItem(
      id: json['id'] as String,
      direction: dir,
      status: status,
      fileName: json['file_name'] as String? ?? '',
      fileSize: json['file_size'] as int? ?? 0,
      peer: peerJson != null
          ? Device.fromJson(peerJson)
          : const Device(
              id: '',
              name: 'Unknown',
              platform: '',
              port: 0,
              pubHash: '',
              ipVersion: '',
            ),
      progress: (json['progress'] as num?)?.toDouble() ?? 0.0,
      error: json['error'] as String?,
      startedAt: json['started_at'] != null
          ? DateTime.tryParse(json['started_at'] as String)
          : null,
    );
  }

  TransferStatus _parseStatus(String? s) {
    switch (s) {
      case 'pending':
        return TransferStatus.pending;
      case 'connecting':
        return TransferStatus.connecting;
      case 'transferring':
        return TransferStatus.transferring;
      case 'completed':
        return TransferStatus.completed;
      case 'failed':
        return TransferStatus.failed;
      case 'cancelled':
        return TransferStatus.cancelled;
      case 'awaiting_resume':
        return TransferStatus.awaitingResume;
      default:
        return TransferStatus.pending;
    }
  }

  @override
  void dispose() {
    _disposed = true;
    stop();
    super.dispose();
  }
}
