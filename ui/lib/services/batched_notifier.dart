import 'dart:async';

/// Merges multiple file-received events into a single notification.
///
/// Strategy (PRD §6.4 / P1-22): accumulate up to [maxBatchSize] file names. When
/// the batch fills (5 files) it flushes immediately; otherwise a [batchWindow]
/// timer flushes whatever has accumulated. This prevents notification spam when
/// many small files arrive in quick succession.
class BatchedNotifier {
  BatchedNotifier({
    required this.onFlush,
    this.batchWindow = const Duration(milliseconds: 800),
    this.maxBatchSize = 5,
  });

  /// Called with the accumulated file names whenever a batch is flushed.
  void Function(List<String> fileNames) onFlush;

  /// How long to wait for more files before flushing a partial batch.
  final Duration batchWindow;

  /// Maximum files per notification. A full batch flushes immediately.
  final int maxBatchSize;

  final List<String> _pending = [];
  Timer? _timer;
  bool _disposed = false;

  /// Adds a file name to the current batch. If the batch reaches
  /// [maxBatchSize] it flushes immediately; otherwise the [batchWindow] timer
  /// (re)starts.
  void add(String fileName) {
    if (_disposed) return;
    _pending.add(fileName);
    if (_pending.length >= maxBatchSize) {
      _flushNow();
      return;
    }
    _timer?.cancel();
    _timer = Timer(batchWindow, _flushNow);
  }

  /// Flushes any pending files immediately.
  void flush() => _flushNow();

  void _flushNow() {
    _timer?.cancel();
    _timer = null;
    if (_pending.isEmpty) return;
    final batch = List<String>.of(_pending);
    _pending.clear();
    onFlush(batch);
  }

  /// Flushes remaining files and prevents further additions.
  void dispose() {
    if (_disposed) return;
    _disposed = true;
    _flushNow();
  }
}

typedef L10nFileReceive = ({
  String Function() singleTitle,
  String Function(int count) multiTitle,
  String Function(String first5, int count) andMore,
});

({String title, String body}) formatFileBatch(
  List<String> fileNames,
  L10nFileReceive l10n,
) {
  switch (fileNames.length) {
    case 0:
      return (title: '', body: '');
    case 1:
      return (title: l10n.singleTitle(), body: fileNames.first);
    default:
      final n = fileNames.length;
      final title = l10n.multiTitle(n);
      final body = n <= 5
          ? fileNames.join('、')
          : l10n.andMore(fileNames.take(5).join('、'), n);
      return (title: title, body: body);
  }
}
