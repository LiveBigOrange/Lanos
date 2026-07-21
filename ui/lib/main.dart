import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'l10n/app_localizations.dart';
import 'services/api_client.dart';
import 'services/device_service.dart';
import 'services/lifecycle_controller.dart';
import 'services/notification_service.dart';
import 'services/transfer_service.dart';
import 'pages/home_page.dart';
import 'pages/onboarding_page.dart';

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
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
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
/// the loading screen, the error screen, the onboarding, or the [HomePage].
class LanosHome extends StatefulWidget {
  const LanosHome({super.key, this.lifecycleController});

  final LifecycleController? lifecycleController;

  @override
  State<LanosHome> createState() => _LanosHomeState();
}

class _LanosHomeState extends State<LanosHome> {
  late final LifecycleController _lifecycle;
  _LifecyclePhase _phase = _LifecyclePhase.starting;
  String? _error;
  bool _showOnboarding = false;

  TransferService? _transferService;
  NotificationService? _notificationService;
  DeviceService? _deviceService;

  @override
  void initState() {
    super.initState();
    _lifecycle = widget.lifecycleController ?? LifecycleControllerDesktop();
    _checkOnboarding();
    _boot();
  }

  @override
  void dispose() {
    _transferService?.removeListener(_onTransferChange);
    _transferService?.dispose();
    _notificationService?.dispose();
    _deviceService?.dispose();
    _apiClient?.close();
    super.dispose();
  }

  void _onTransferChange() {
    // no-op; notification wiring is in the callback, UI is per-page.
  }

  Future<void> _checkOnboarding() async {
    final prefs = await SharedPreferences.getInstance();
    if (!mounted) return;
    final done = prefs.getBool('onboarding_done') ?? false;
    setState(() => _showOnboarding = !done);
  }

  Future<void> _boot() async {
    try {
      final api = await _lifecycle.start();
      if (!mounted) return;
      _notificationService = NotificationService();
      await _notificationService!.init();
      final ts = TransferService(api);
      ts.onIncomingCompleted =
          (item) => _notificationService!.onFileReceived(item.fileName);
      ts.addListener(_onTransferChange);
      ts.start();
      _transferService = ts;
      final ds = DeviceService(api);
      ds.start();
      _deviceService = ds;
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
    if (_showOnboarding) {
      return OnboardingPage(
        onDone: () {
          setState(() => _showOnboarding = false);
        },
      );
    }
    switch (_phase) {
      case _LifecyclePhase.starting:
        return const _LoadingScreen();
      case _LifecyclePhase.failed:
        return _ErrorScreen(error: _error ?? 'unknown error', onRetry: _boot);
      case _LifecyclePhase.ready:
        return HomePage(
          api: _apiClient!,
          transferService: _transferService,
          deviceService: _deviceService,
        );
    }
  }
}

enum _LifecyclePhase { starting, ready, failed }

class _LoadingScreen extends StatelessWidget {
  const _LoadingScreen();

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    return Scaffold(
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const CircularProgressIndicator(),
            const SizedBox(height: 16),
            Text(l10n.bootingDaemon),
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
    final l10n = AppLocalizations.of(context)!;
    return Scaffold(
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline, size: 64, color: Colors.red),
              const SizedBox(height: 16),
              Text(
                l10n.bootFailed,
                style:
                    const TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 8),
              Text(error, textAlign: TextAlign.center),
              const SizedBox(height: 24),
              FilledButton.icon(
                onPressed: onRetry,
                icon: const Icon(Icons.refresh),
                label: Text(l10n.retryBoot),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
