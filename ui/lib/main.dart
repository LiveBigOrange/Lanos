import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'services/api_client.dart';
import 'services/lifecycle_controller.dart';
import 'pages/home_page.dart';

/// Lanos application entry point.
///
/// On desktop, [LifecycleControllerDesktop] spawns the gcd subprocess,
/// reads the stdout handshake JSON (port + api_token) and exposes an
/// [ApiClient] used by the UI. On mobile, the equivalent is the FFI
/// client backed by gomobile bind (lands in P4 W12).
void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const LanosApp());
}

class LanosApp extends StatelessWidget {
  const LanosApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Lanos',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        useMaterial3: true,
        colorSchemeSeed: const Color(0xFF3B82F6),
        brightness: Brightness.light,
      ),
      darkTheme: ThemeData(
        useMaterial3: true,
        colorSchemeSeed: const Color(0xFF3B82F6),
        brightness: Brightness.dark,
      ),
      home: const LanosHome(),
    );
  }
}

/// Top-level widget that owns the [LifecycleController] and renders either
/// the loading screen, the error screen, or the [HomePage].
class LanosHome extends StatefulWidget {
  const LanosHome({super.key});

  @override
  State<LanosHome> createState() => _LanosHomeState();
}

class _LanosHomeState extends State<LanosHome> {
  late final LifecycleController _lifecycle;
  _LifecyclePhase _phase = _LifecyclePhase.starting;
  String? _error;

  @override
  void initState() {
    super.initState();
    _lifecycle = LifecycleController();
    _boot();
  }

  Future<void> _boot() async {
    try {
      final api = await _lifecycle.start();
      if (!mounted) return;
      setState(() {
        _apiClient = api;
        _phase = _LifecyclePhase.ready;
      });
    } on PlatformException catch (e) {
      if (!mounted) return;
      setState(() {
        _phase = _LifecyclePhase.failed;
        _error = 'gcd launch failed: ${e.message ?? e.code}';
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _phase = _LifecyclePhase.failed;
        _error = 'gcd launch failed: $e';
      });
    }
  }

  ApiClient? _apiClient;

  @override
  Widget build(BuildContext context) {
    switch (_phase) {
      case _LifecyclePhase.starting:
        return const _LoadingScreen();
      case _LifecyclePhase.failed:
        return _ErrorScreen(error: _error ?? 'unknown error', onRetry: _boot);
      case _LifecyclePhase.ready:
        return HomePage(api: _apiClient!);
    }
  }
}

enum _LifecyclePhase { starting, ready, failed }

class _LoadingScreen extends StatelessWidget {
  const _LoadingScreen();

  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            CircularProgressIndicator(),
            SizedBox(height: 16),
            Text('正在启动 Lanos 守护进程...'),
          ],
        ),
      ),
    );
  }
}

class _ErrorScreen extends StatelessWidget {
  const _ErrorScreen({required this.error, required this.onRetry});

  final String error;
  final Future<void> Function() onRetry;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline, size: 64, color: Colors.red),
              const SizedBox(height: 16),
              const Text('启动失败', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600)),
              const SizedBox(height: 8),
              Text(error, textAlign: TextAlign.center),
              const SizedBox(height: 24),
              FilledButton.icon(
                onPressed: onRetry,
                icon: const Icon(Icons.refresh),
                label: const Text('重试启动'),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
