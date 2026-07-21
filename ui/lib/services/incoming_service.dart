import 'dart:async';

import 'package:flutter/foundation.dart';

import 'api_client.dart';

class IncomingItem {
  const IncomingItem({
    required this.id,
    required this.peerId,
    required this.peerName,
    required this.fileName,
    required this.fileSize,
    required this.status,
    this.savePath,
    this.receivedBytes = 0,
    this.error,
    this.createdAt,
    this.expiresAt,
  });

  final String id;
  final String peerId;
  final String peerName;
  final String fileName;
  final int fileSize;
  final String status;
  final String? savePath;
  final int receivedBytes;
  final String? error;
  final DateTime? createdAt;
  final DateTime? expiresAt;

  bool get isPending => status == 'pending';
  bool get isExpired => status == 'expired';
  bool get isReceiving => status == 'receiving';

  IncomingItem copyWith({
    String? status,
    String? savePath,
    int? receivedBytes,
    String? error,
  }) {
    return IncomingItem(
      id: id,
      peerId: peerId,
      peerName: peerName,
      fileName: fileName,
      fileSize: fileSize,
      status: status ?? this.status,
      savePath: savePath ?? this.savePath,
      receivedBytes: receivedBytes ?? this.receivedBytes,
      error: error ?? this.error,
      createdAt: createdAt,
      expiresAt: expiresAt,
    );
  }
}

class IncomingService extends ChangeNotifier {
  IncomingService(
    ApiClient api, {
    Duration interval = const Duration(seconds: 3),
  })  : _api = api,
        _interval = interval;

  final ApiClient _api;
  final Duration _interval;

  List<IncomingItem> _items = [];
  List<IncomingItem> get items => _items;

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

  Future<void> _doFetch() async {
    if (_fetching || _disposed) return;
    _fetching = true;
    try {
      final json = await _api.get('/api/v1/incoming');
      if (_disposed) return;
      final list = (json['incoming'] as List?) ?? [];
      _items = list.map((e) {
        final m = e as Map<String, dynamic>;
        return IncomingItem(
          id: m['id'] as String,
          peerId: m['peer_id'] as String? ?? '',
          peerName: m['peer_name'] as String? ?? '',
          fileName: m['file_name'] as String? ?? '',
          fileSize: (m['file_size'] as num?)?.toInt() ?? 0,
          status: m['status'] as String? ?? '',
          savePath: m['save_path'] as String?,
          receivedBytes: (m['received_bytes'] as num?)?.toInt() ?? 0,
          error: m['error'] as String?,
          createdAt: m['created_at'] != null
              ? DateTime.tryParse(m['created_at'] as String)
              : null,
          expiresAt: m['expires_at'] != null
              ? DateTime.tryParse(m['expires_at'] as String)
              : null,
        );
      }).toList();
      notifyListeners();
    } catch (_) {
    } finally {
      _fetching = false;
    }
  }

  Future<void> accept(String id, String savePath) async {
    await _api.post('/api/v1/incoming/$id/accept', {'save_path': savePath});
    await _doFetch();
  }

  Future<void> reject(String id) async {
    await _api.post('/api/v1/incoming/$id/reject');
    await _doFetch();
  }

  Future<void> cancel(String id) async {
    await _api.post('/api/v1/incoming/$id/cancel');
    await _doFetch();
  }

  @override
  void dispose() {
    _disposed = true;
    stop();
    super.dispose();
  }
}
