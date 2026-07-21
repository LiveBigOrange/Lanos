
import 'package:flutter_local_notifications/flutter_local_notifications.dart';

import 'batched_notifier.dart';

/// Native OS notification integration for file-received events.
///
/// Wraps [flutter_local_notifications] and uses [BatchedNotifier] to merge up
/// to 5 file completions into a single notification (PRD §6.4 / P1-22).
///
/// Call [init] once at startup, then [onFileReceived] for each completed
/// incoming transfer. The batching and formatting logic lives in
/// [BatchedNotifier] / [formatFileBatch] and is fully unit-testable without
/// this class or the platform plugin.
class NotificationService {
  NotificationService({
    FlutterLocalNotificationsPlugin? plugin,
    Duration batchWindow = const Duration(milliseconds: 800),
    int maxBatchSize = 5,
  }) : _plugin = plugin ?? FlutterLocalNotificationsPlugin() {
    _batcher = BatchedNotifier(
      onFlush: _handleFlush,
      batchWindow: batchWindow,
      maxBatchSize: maxBatchSize,
    );
  }

  final FlutterLocalNotificationsPlugin _plugin;
  late final BatchedNotifier _batcher;

  bool _initialized = false;

  /// Initializes the platform notification plugin. Safe to call once at
  /// startup; subsequent calls are no-ops.
  Future<void> init() async {
    if (_initialized) return;
    const initSettings = InitializationSettings(
      android: AndroidInitializationSettings('@mipmap/ic_launcher'),
      iOS: DarwinInitializationSettings(),
      macOS: DarwinInitializationSettings(),
      linux: LinuxInitializationSettings(defaultActionName: 'Open'),
      windows: WindowsInitializationSettings(appName: 'Lanos'),
    );
    await _plugin.initialize(settings: initSettings);
    _initialized = true;
  }

  /// Called when an incoming file transfer completes. The file name is batched;
  /// a notification fires when the batch fills (5 files) or the window expires.
  void onFileReceived(String fileName) {
    _batcher.add(fileName);
  }

  /// Forces any pending files to be notified immediately.
  void flush() => _batcher.flush();

  void _handleFlush(List<String> fileNames) {
    if (!_initialized) return;
    final formatted = formatFileBatch(
      fileNames,
      (
        singleTitle: () => 'File received',
        multiTitle: (n) => '$n files received',
        andMore: (first5, n) => '$first5 and $n more',
      ),
    );
    if (formatted.title.isEmpty) return;
    _plugin.show(
      id: _nextId(),
      title: formatted.title,
      body: formatted.body,
      notificationDetails: const NotificationDetails(
        android: AndroidNotificationDetails(
          'file_received',
          '文件接收',
          importance: Importance.high,
          priority: Priority.high,
        ),
        macOS: DarwinNotificationDetails(),
        linux: LinuxNotificationDetails(),
      ),
    );
  }

  int _idCounter = 0;
  int _nextId() {
    _idCounter = (_idCounter + 1) & 0x7FFFFFFF;
    return _idCounter;
  }

  void dispose() {
    _batcher.dispose();
  }
}
