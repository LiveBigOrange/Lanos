// Human-readable formatters for the transfer UI: byte sizes, throughput, and
// remaining-time durations.
//
// All formatters are pure functions so they can be unit-tested without any
// widget plumbing.

/// Formats a byte count as a compact human-readable string.
///
/// Examples: `0 B`, `1.5 KB`, `2.30 MB`, `4.7 GB`, `1.0 TB`.
String formatBytes(int bytes) {
  if (bytes < 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  var size = bytes.toDouble();
  var unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit++;
  }
  if (unit == 0) return '${bytes.toInt()} B';
  return '${size.toStringAsFixed(size >= 100 ? 0 : (size >= 10 ? 1 : 2))} ${units[unit]}';
}

/// Formats a throughput in bytes/second.
///
/// Examples: `0 B/s`, `1.5 MB/s`, `820 KB/s`.
String formatSpeed(double bytesPerSecond) {
  if (bytesPerSecond <= 0 || !bytesPerSecond.isFinite) return '0 B/s';
  return '${formatBytes(bytesPerSecond.round())}/s';
}

/// Formats a remaining [Duration] for an ETA display.
///
/// Returns `'--'` for `null` (unknown) and `'完成'` for a zero/short duration.
/// Examples: `12s`, `1m 23s`, `--`.
String formatDuration(Duration? d, {String doneLabel = 'Done'}) {
  if (d == null) return '--';
  if (d.inSeconds <= 0) return doneLabel;
  if (d.inHours > 0) {
    return '${d.inHours}h ${(d.inMinutes % 60).toString().padLeft(2, '0')}m';
  }
  if (d.inMinutes > 0) {
    return '${d.inMinutes}m ${(d.inSeconds % 60).toString().padLeft(2, '0')}s';
  }
  return '${d.inSeconds}s';
}
