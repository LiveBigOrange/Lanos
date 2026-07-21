import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';

import '../models/device.dart';
import 'api_client.dart';
import 'sse_client.dart';

/// Snapshot of the device inventory returned by `GET /api/v1/devices`.
@immutable
class DeviceSnapshot {
  const DeviceSnapshot({
    this.self,
    this.peers = const [],
    this.error,
    this.updatedAt,
  });

  final Device? self;
  final List<Device> peers;
  final String? error;
  final DateTime? updatedAt;

  bool get hasError => error != null;
  bool get isEmpty => self == null && peers.isEmpty;
}

/// Function that fetches the raw device-list JSON. Defaults to calling
/// [ApiClient.get] on `/api/v1/devices`; tests can substitute a fake.
typedef DeviceFetcher = Future<Map<String, dynamic>> Function();

/// Combines SSE (`/api/v1/events`) with an initial HTTP fetch (`/api/v1/devices`)
/// for real-time device presence without polling overhead.
class DeviceService extends ChangeNotifier {
  DeviceService(ApiClient api, {Duration interval = const Duration(seconds: 5)})
      : this._withApi(api, interval);

  DeviceService._withApi(ApiClient api, Duration interval)
      : _fetch = (() => api.get('/api/v1/devices')),
        _interval = interval,
        _sse = SseClient(port: api.port, token: api.token);

  @visibleForTesting
  DeviceService.withFetcher(this._fetch, this._interval) : _sse = null;

  final DeviceFetcher _fetch;
  final Duration _interval;

  DeviceSnapshot _snapshot = const DeviceSnapshot();
  DeviceSnapshot get snapshot => _snapshot;

  Timer? _pollTimer;
  bool _fetching = false;
  bool _disposed = false;
  SseClient? _sse;
  StreamSubscription<SseEvent>? _sseSub;

  void start() {
    _doFetch();
    _pollTimer = Timer.periodic(_interval, (_) => _doFetch());
    _sse?.connect();
    _sseSub = _sse?.stream.listen(_onSseEvent, onError: (_) {});
  }

  void stop() {
    _pollTimer?.cancel();
    _pollTimer = null;
    _sseSub?.cancel();
    _sseSub = null;
  }

  Future<void> refresh() => _doFetch();

  void _onSseEvent(SseEvent ev) {
    if (_disposed) return;
    Map<String, dynamic> payload;
    try {
      payload = jsonDecode(ev.data) as Map<String, dynamic>;
    } catch (_) {
      return;
    }
    if (payload['type'] == null || payload['device'] == null) return;

    final deviceJson = payload['device'] as Map<String, dynamic>;
    final device = Device.fromJson(deviceJson);
    final eventType = payload['type'] as String;
    final peers = List<Device>.of(_snapshot.peers);
    final idx = peers.indexWhere((d) => d.id == device.id);

    switch (eventType) {
      case 'online':
      case 'update':
        if (idx >= 0) {
          peers[idx] = device;
        } else {
          peers.add(device);
        }
      case 'offline':
        if (idx >= 0) peers.removeAt(idx);
    }
    _snapshot = DeviceSnapshot(
      self: _snapshot.self,
      peers: peers,
      updatedAt: DateTime.now(),
    );
    notifyListeners();
  }

  Future<void> _doFetch() async {
    if (_fetching || _disposed) return;
    _fetching = true;
    try {
      final json = await _fetch();
      if (_disposed) return;
      final self = json['self'] == null
          ? null
          : Device.fromJson(json['self'] as Map<String, dynamic>);
      final fetchedPeers = (json['peers'] as List?)
              ?.map((e) => Device.fromJson(e as Map<String, dynamic>))
              .toList() ??
          const <Device>[];
      if (_snapshot.self?.id != self?.id ||
          _snapshot.peers.length != fetchedPeers.length ||
          _snapshot.peers.isEmpty) {
        _snapshot = DeviceSnapshot(
          self: self,
          peers: fetchedPeers,
          updatedAt: DateTime.now(),
        );
        notifyListeners();
      }
    } catch (e) {
      if (_disposed) return;
      _snapshot = DeviceSnapshot(
        self: _snapshot.self,
        peers: _snapshot.peers,
        error: e.toString(),
        updatedAt: DateTime.now(),
      );
      notifyListeners();
    } finally {
      _fetching = false;
    }
  }

  @override
  void dispose() {
    _disposed = true;
    stop();
    _sse?.dispose();
    super.dispose();
  }
}
