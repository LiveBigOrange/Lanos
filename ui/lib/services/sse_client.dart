import 'dart:async';
import 'dart:convert';
import 'dart:io';

class SseEvent {
  const SseEvent({required this.event, required this.data});

  final String event;
  final String data;
}

class SseClient {
  SseClient({required this.port, required this.token});

  final int port;
  final String token;

  HttpClient? _http;
  StreamSubscription? _sub;
  bool _disposed = false;

  final StreamController<SseEvent> _controller =
      StreamController<SseEvent>.broadcast();

  Stream<SseEvent> get stream => _controller.stream;

  void connect() {
    if (_disposed) return;
    _startConnection();
  }

  Future<void> _startConnection() async {
    try {
      final req = await _httpClient().getUrl(
        Uri.http('127.0.0.1:$port', '/api/v1/events'),
      );
      req.headers.set('Authorization', 'Bearer $token');
      req.headers.set('Accept', 'text/event-stream');
      final resp = await req.close();
      _reconnectDelay = 500;
      String? currentEvent;

      StringBuffer currentData = StringBuffer();
      _sub =
          resp.transform(utf8.decoder).transform(const LineSplitter()).listen(
        (line) {
          if (line.startsWith('event: ')) {
            currentEvent = line.substring(7);
          } else if (line.startsWith('data: ')) {
            if (currentData.isNotEmpty) currentData.write('\n');
            currentData.write(line.substring(6));

          } else if (line.isEmpty && currentEvent != null) {
            _controller.add(
              SseEvent(
                event: currentEvent!,
                data: currentData.toString(),
              ),
            );
            currentEvent = null;
            currentData = StringBuffer();
          }
        },
        onDone: _onDisconnected,
        onError: (_) => _onDisconnected(),
        cancelOnError: false,
      );
    } catch (_) {
      _onDisconnected();
    }
  }

  int _reconnectDelay = 500;

  void _onDisconnected() {
    if (_disposed) return;
    _sub?.cancel();
    _sub = null;
    final delay = _reconnectDelay;
    _reconnectDelay = (_reconnectDelay * 2).clamp(500, 30000);
    Timer(Duration(milliseconds: delay), () {
      if (_disposed) return;
      _startConnection();
    });
  }

  HttpClient _httpClient() => _http ??= HttpClient();

  void dispose() {
    _disposed = true;
    _sub?.cancel();
    _http?.close();
    _http = null;
  }
}
