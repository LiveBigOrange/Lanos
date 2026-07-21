import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'api_client.dart';

/// Abstract lifecycle controller so we can swap desktop (subprocess) vs
/// mobile (gomobile bind FFI) implementations in P4.
///
/// MVP P0 ships the desktop variant only.
abstract class LifecycleController {
  /// Spawns gcd (if needed), waits for the stdout handshake line, and
  /// returns a ready [ApiClient]. Throws if gcd fails to start within
  /// the 5-second timeout (see PRD §5.1.3).
  Future<ApiClient> start();
}

/// Desktop lifecycle controller: spawns the gcd subprocess and parses the
/// stdout handshake JSON `{"port":...,"api_token":...,"version":...}`.
///
/// See PRD §5.1.3 启动握手协议.
class LifecycleControllerDesktop implements LifecycleController {
  LifecycleControllerDesktop({this.gcdPath});

  /// Optional override for the gcd binary path. Defaults to looking next
  /// to the Flutter executable, then PATH.
  final String? gcdPath;

  Process? _process;
  bool _reuseExisting = false;

  @override
  Future<ApiClient> start() async {
    final path = gcdPath ?? await _resolveGcdPath();
    if (path == null) {
      throw StateError('gcd binary not found');
    }

    _process = await Process.start(
      path,
      [],
      mode: ProcessStartMode.normal,
    );

    // Read the first stdout line within 5 seconds.
    final handshakeLine = await _process!.stdout
        .transform(utf8.decoder)
        .transform(const LineSplitter())
        .first
        .timeout(const Duration(seconds: 5));

    final json = jsonDecode(handshakeLine) as Map<String, dynamic>;
    final port = json['port'] as int;
    final token = json['api_token'] as String;
    final version = json['version'] as String;

    _reuseExisting = (json['already_running'] as bool?) ?? false;
    return ApiClient(port: port, token: token, version: version);
  }

  /// Sends POST /api/v1/shutdown to gracefully terminate gcd, unless the
  /// process was reused (already-running scenario) in which case we leave
  /// it alone.
  Future<void> shutdown(ApiClient api) async {
    if (_reuseExisting || _process == null) return;
    try {
      await api.post('/api/v1/shutdown');
    } finally {
      _process?.kill();
      _process = null;
    }
  }

  Future<String?> _resolveGcdPath() async {
    // TODO P0 W1: bundle gcd in build output and resolve relative to the
    // executable directory. For now, expect it on PATH.
    return Platform.isWindows ? 'gcd.exe' : 'gcd';
  }
}
