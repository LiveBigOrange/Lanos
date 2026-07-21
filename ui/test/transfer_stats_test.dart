import 'package:flutter_test/flutter_test.dart';

import 'package:lanos/utils/transfer_stats.dart';

void main() {
  group('TransferStats', () {
    test('speed is zero before two samples', () {
      final stats = TransferStats(now: () => DateTime(2024, 1, 1, 0, 0, 0));
      expect(stats.speedBytesPerSecond, 0);
      stats.record(0);
      expect(stats.speedBytesPerSecond, 0);
    });

    test('computes speed over the sample window', () {
      var t = DateTime(2024, 1, 1, 0, 0, 0);
      final stats = TransferStats(now: () => t);
      stats.record(0);
      t = t.add(const Duration(seconds: 2));
      stats.record(10 * 1024 * 1024); // 10 MB in 2s = 5 MB/s
      expect(stats.speedBytesPerSecond, closeTo(5 * 1024 * 1024, 1));
    });

    test('ETA derives from remaining bytes and speed', () {
      var t = DateTime(2024, 1, 1, 0, 0, 0);
      final stats = TransferStats(now: () => t);
      // 100 MB file, 10 MB transferred in 2s -> 5 MB/s -> 90 MB left -> 18s.
      const fileSize = 100 * 1024 * 1024;
      stats.record(0);
      t = t.add(const Duration(seconds: 2));
      stats.record(10 * 1024 * 1024);
      final eta = stats.eta(fileSize);
      expect(eta, isNotNull);
      expect(eta!.inSeconds, closeTo(18, 1));
    });

    test('ETA is zero when already complete', () {
      final stats = TransferStats(now: () => DateTime(2024, 1, 1));
      stats.record(0);
      stats.record(1024);
      expect(stats.eta(1024), Duration.zero);
    });

    test('ETA is null when speed unknown', () {
      final stats = TransferStats(now: () => DateTime(2024, 1, 1));
      stats.record(0);
      expect(stats.eta(1024), isNull);
    });

    test('sliding window keeps at most maxSamples', () {
      var t = DateTime(2024, 1, 1);
      final stats = TransferStats(maxSamples: 3, now: () => t);
      for (var i = 0; i < 10; i++) {
        stats.record(i * 1024);
        t = t.add(const Duration(seconds: 1));
      }
      expect(stats.sampleCount, 3);
    });

    test('reset clears samples', () {
      final stats = TransferStats(now: () => DateTime(2024, 1, 1));
      stats.record(0);
      stats.record(1024);
      expect(stats.sampleCount, 2);
      stats.reset();
      expect(stats.sampleCount, 0);
      expect(stats.lastBytes, 0);
    });

    test('byte-count regression clears the window', () {
      var t = DateTime(2024, 1, 1);
      final stats = TransferStats(now: () => t);
      stats.record(5000);
      t = t.add(const Duration(seconds: 1));
      stats.record(8000);
      expect(stats.speedBytesPerSecond, greaterThan(0));
      // A smaller value signals a fresh transfer reusing the tracker.
      stats.record(100);
      expect(stats.sampleCount, 1);
      expect(stats.speedBytesPerSecond, 0);
    });

    test('speed is zero when no time elapsed between samples', () {
      final fixed = DateTime(2024, 1, 1);
      final stats = TransferStats(now: () => fixed);
      stats.record(0);
      stats.record(1024); // same timestamp
      expect(stats.speedBytesPerSecond, 0);
    });
  });
}
