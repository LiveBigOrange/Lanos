/// Tracks byte-progress samples for a single transfer and computes a smoothed
/// throughput (bytes/second) plus ETA.
///
/// The transfer backend is polled every ~2s by [TransferService]; each poll
/// yields a fresh `bytesTransferred` value. We keep a sliding window of the
/// most recent samples and derive speed from the window's endpoints (oldest vs
/// newest), which is both stable and cheap. ETA is `remainingBytes / speed`.
///
/// The clock is injectable via [now] so tests can drive time deterministically
/// without `Future.delayed`.
library;

import 'dart:collection';

class TransferStats {
  TransferStats({this.maxSamples = 8, this.now});

  final int maxSamples;

  final DateTime Function()? now;

  final Queue<_Sample> _samples = Queue<_Sample>();
  int _lastBytes = 0;

  void record(int bytesTransferred) {
    final t = now?.call() ?? DateTime.now();
    if (_samples.isNotEmpty && bytesTransferred < _lastBytes) {
      _samples.clear();
    }
    _samples.addLast(_Sample(t, bytesTransferred));
    while (_samples.length > maxSamples) {
      _samples.removeFirst();
    }
    _lastBytes = bytesTransferred;
  }

  double get speedBytesPerSecond {
    if (_samples.length < 2) return 0;
    final first = _samples.first;
    final last = _samples.last;
    final dtMicros = last.time.difference(first.time).inMicroseconds;
    if (dtMicros <= 0) return 0;
    final db = last.bytes - first.bytes;
    if (db <= 0) return 0;
    return db * 1000000.0 / dtMicros;
  }

  Duration? eta(int fileSize) {
    final remaining = fileSize - _lastBytes;
    if (remaining <= 0) return Duration.zero;
    final speed = speedBytesPerSecond;
    if (speed <= 0) return null;
    return Duration(milliseconds: (remaining / speed * 1000).round());
  }

  int get lastBytes => _lastBytes;

  int get sampleCount => _samples.length;

  void reset() {
    _samples.clear();
    _lastBytes = 0;
  }
}

class _Sample {
  const _Sample(this.time, this.bytes);

  final DateTime time;
  final int bytes;
}
