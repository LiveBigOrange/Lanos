import 'package:flutter_test/flutter_test.dart';

import 'package:lanos/utils/format.dart';

void main() {
  group('formatBytes', () {
    test('formats zero and small values in bytes', () {
      expect(formatBytes(0), '0 B');
      expect(formatBytes(512), '512 B');
      expect(formatBytes(1023), '1023 B');
    });

    test('formats kilobytes', () {
      expect(formatBytes(1024), '1.00 KB');
      expect(formatBytes(1536), '1.50 KB');
      expect(formatBytes(10240), '10.0 KB');
      expect(formatBytes(102400), '100 KB');
    });

    test('formats megabytes', () {
      expect(formatBytes(1 << 20), '1.00 MB');
      expect(formatBytes((2.3 * (1 << 20)).round()), '2.30 MB');
    });

    test('formats gigabytes and terabytes', () {
      expect(formatBytes(1 << 30), '1.00 GB');
      expect(formatBytes(1 << 40), '1.00 TB');
    });

    test('clamps negative to zero', () {
      expect(formatBytes(-100), '0 B');
    });
  });

  group('formatSpeed', () {
    test('formats zero and negative as 0 B/s', () {
      expect(formatSpeed(0), '0 B/s');
      expect(formatSpeed(-1), '0 B/s');
      expect(formatSpeed(double.nan), '0 B/s');
    });

    test('formats positive throughput', () {
      expect(formatSpeed(1024 * 1024), '1.00 MB/s');
      expect(formatSpeed(820 * 1024), '820 KB/s');
    });
  });

  group('formatDuration', () {
    test('null yields placeholder', () {
      expect(formatDuration(null), '--');
    });

    test('zero or sub-second yields doneLabel', () {
      expect(formatDuration(Duration.zero), 'Done');
      expect(formatDuration(const Duration(milliseconds: 500)), 'Done');
      expect(formatDuration(Duration.zero, doneLabel: '完成'), '完成');
    });

    test('seconds', () {
      expect(formatDuration(const Duration(seconds: 12)), '12s');
    });

    test('minutes and seconds', () {
      expect(formatDuration(const Duration(minutes: 1, seconds: 23)), '1m 23s');
    });

    test('hours and minutes', () {
      expect(formatDuration(const Duration(hours: 1, minutes: 5)), '1h 05m');
    });
  });
}
