import 'dart:async';

import 'package:flutter/foundation.dart';

import '../models/device.dart';
import 'api_client.dart';

enum TransferDirection { outgoing, incoming }

enum TransferStatus { pending, transferring, completed, failed, cancelled }

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
  TransferService(ApiClient api, {Duration interval = const Duration(seconds: 2)})
      : this.withFetcher(() => api.get('/api/v1/transfers'), interval);

  @visibleForTesting
  TransferService.withFetcher(this._fetch, this._interval);

  final TransferListFetcher _fetch;
  final Duration _interval;

  List<TransferItem> _items = [];
  List<TransferItem> get items => _items;

  Timer? _timer;
  bool _fetching = false;
  bool _disposed = false;

  void start() {
    if (_timer != null) return;
    _fetch();
    _timer = Timer.periodic(_interval, (_) => _fetch());
  }

  void stop() {
    _timer?.cancel();
    _timer = null;
  }

  Future<void> refresh() => _fetch();

  Future<void> cancelTransfer(String id) async {
    // TODO: POST /api/v1/transfers/{id}/cancel when API endpoint exists.
    _items = _items.map((t) {
      return t.id == id && t.status == TransferStatus.transferring
          ? t.copyWith(status: TransferStatus.cancelled)
          : t;
    }).toList();
    notifyListeners();
  }

  Future<void> _fetch() async {
    if (_fetching || _disposed) return;
    _fetching = true;
    try {
      final jsonList = await _fetch();
      if (_disposed) return;
      _items = jsonList.map((json) => _parseItem(json)).toList();
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
          : const Device(id: '', name: 'Unknown', addresses: []),
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
      case 'transferring':
        return TransferStatus.transferring;
      case 'completed':
        return TransferStatus.completed;
      case 'failed':
        return TransferStatus.failed;
      case 'cancelled':
        return TransferStatus.cancelled;
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
