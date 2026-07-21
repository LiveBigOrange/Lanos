import 'dart:async';

import 'package:flutter/foundation.dart';

import '../models/device.dart';
import 'api_client.dart';

/// Snapshot of the device inventory returned by `GET /api/v1/devices`.
@immutable
class DeviceSnapshot {
  const DeviceSnapshot({this.self, this.peers = const [], this.error, this.updatedAt});

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

/// Polls `GET /api/v1/devices` at a fixed interval and exposes the result
/// as a [ChangeNotifier] suitable for [AnimatedBuilder] / [ListenableBuilder].
///
/// P1 W2 uses simple 1s polling. P1 W3 will switch to SSE
/// (`GET /api/v1/events` with `text/event-stream`) for real-time updates
/// without polling overhead (see PRD §5.4 SSE event channel).
class DeviceService extends ChangeNotifier {
  /// Constructs a DeviceService bound to [api]. Polling does not start
  /// until [start] is called.
  DeviceService(ApiClient api, {Duration interval = const Duration(seconds: 1)})
      : this.withFetcher(() => api.get('/api/v1/devices'), interval);

  /// Internal constructor that accepts an injectable fetcher for testing.
  @visibleForTesting
  DeviceService.withFetcher(this._fetch, this._interval);

  final DeviceFetcher _fetch;
  final Duration _interval;

  DeviceSnapshot _snapshot = const DeviceSnapshot();
  DeviceSnapshot get snapshot => _snapshot;

  Timer? _timer;
  bool _fetching = false;
  bool _disposed = false;

  /// Starts polling. The first fetch happens immediately, then on every tick.
  void start() {
    if (_timer != null) return;
    _fetch();
    _timer = Timer.periodic(_interval, (_) => _fetch());
  }

  /// Stops polling. Safe to call multiple times.
  void stop() {
    _timer?.cancel();
    _timer = null;
  }

  /// Forces an immediate refresh (e.g. pull-to-refresh).
  Future<void> refresh() => _fetch();

  Future<void> _fetch() async {
    // Re-entrancy guard: skip if a previous fetch is still in flight.
    if (_fetching || _disposed) return;
    _fetching = true;
    try {
      final json = await _fetch();
      if (_disposed) return;
      final self = json['self'] == null ? null : Device.fromJson(json['self'] as Map<String, dynamic>);
      final peers = (json['peers'] as List?)?.map((e) => Device.fromJson(e as Map<String, dynamic>)).toList() ?? const <Device>[];
      _snapshot = DeviceSnapshot(self: self, peers: peers, updatedAt: DateTime.now());
      notifyListeners();
    } catch (e) {
      if (_disposed) return;
      // Keep the previous snapshot but record the error so the UI can show
      // a transient "reconnecting" indicator without losing device state.
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
    super.dispose();
  }
}
