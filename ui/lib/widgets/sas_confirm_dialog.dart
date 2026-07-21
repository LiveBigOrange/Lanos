import 'dart:async';

import 'package:flutter/material.dart';

/// A dialog that shows the 4-digit SAS (Short Authentication String) code for
/// the user to visually compare with the peer device's display, then confirm
/// or cancel the pairing.
///
/// Per PRD §3.3 / docs/PROTOCOL.md §3.3, after a Noise XX handshake both sides
/// derive the same 4-digit code from the handshake hash. The user must confirm
/// the numbers match before any transfer proceeds. If the user does not
/// confirm within [timeout] (default 30s per PRD §3.3 "超时机制：30 秒未点
/// 确认自动取消"), the dialog auto-cancels and [onTimeout] is called.
///
/// The dialog is non-dismissable by tapping outside (barrierDismissible =
/// false) so that an accidental tap cannot silently accept a pairing.
class SasConfirmDialog extends StatefulWidget {
  const SasConfirmDialog({
    super.key,
    required this.deviceName,
    required this.sasCode,
    this.timeout = const Duration(seconds: 30),
    this.onConfirm,
    this.onCancel,
    this.onTimeout,
  });

  /// The peer device's display name, shown so the user knows which device
  /// they are confirming.
  final String deviceName;

  /// The 4-digit SAS code computed locally from the handshake hash.
  final String sasCode;

  /// How long to wait before auto-cancelling. PRD §3.3: 30 seconds.
  final Duration timeout;

  /// Called when the user taps "确认" (confirm).
  final VoidCallback? onConfirm;

  /// Called when the user taps "取消" (cancel).
  final VoidCallback? onCancel;

  /// Called when the [timeout] elapses without a user action.
  final VoidCallback? onTimeout;

  @override
  State<SasConfirmDialog> createState() => _SasConfirmDialogState();
}

class _SasConfirmDialogState extends State<SasConfirmDialog> {
  Timer? _timer;
  late int _remaining;

  @override
  void initState() {
    super.initState();
    _remaining = widget.timeout.inSeconds;
    _timer = Timer.periodic(const Duration(seconds: 1), (t) {
      if (!mounted) {
        t.cancel();
        return;
      }
      setState(() => _remaining--);
      if (_remaining <= 0) {
        t.cancel();
        widget.onTimeout?.call();
        _close();
      }
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  void _close() {
    if (mounted) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final urgent = _remaining <= 5;
    return PopScope(
      canPop: false,
      child: AlertDialog(
        title: const Text('确认配对'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              '请与 "${widget.deviceName}" 核对以下数字是否一致，',
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 4),
            const Text(
              '一致后点击"确认"以建立加密连接。',
              textAlign: TextAlign.center,
              style: TextStyle(fontSize: 13),
            ),
            const SizedBox(height: 20),
            Text(
              widget.sasCode,
              style: TextStyle(
                fontSize: 56,
                fontWeight: FontWeight.bold,
                letterSpacing: 12,
                color: theme.colorScheme.primary,
              ),
            ),
            const SizedBox(height: 16),
            Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Icon(
                  urgent ? Icons.timer_off : Icons.timer,
                  size: 16,
                  color: urgent ? theme.colorScheme.error : theme.colorScheme.onSurfaceVariant,
                ),
                const SizedBox(width: 6),
                Text(
                  '$_remaining 秒后自动取消',
                  style: TextStyle(
                    color: urgent ? theme.colorScheme.error : theme.colorScheme.onSurfaceVariant,
                    fontWeight: urgent ? FontWeight.bold : FontWeight.normal,
                  ),
                ),
              ],
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () {
              widget.onCancel?.call();
              _close();
            },
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () {
              widget.onConfirm?.call();
              _close();
            },
            child: const Text('确认'),
          ),
        ],
      ),
    );
  }
}

/// Convenience helper to show the [SasConfirmDialog] as a modal route.
Future<void> showSasConfirmDialog(
  BuildContext context, {
  required String deviceName,
  required String sasCode,
  Duration timeout = const Duration(seconds: 30),
  VoidCallback? onConfirm,
  VoidCallback? onCancel,
  VoidCallback? onTimeout,
}) {
  return showDialog<void>(
    context: context,
    barrierDismissible: false,
    builder: (_) => SasConfirmDialog(
      deviceName: deviceName,
      sasCode: sasCode,
      timeout: timeout,
      onConfirm: onConfirm,
      onCancel: onCancel,
      onTimeout: onTimeout,
    ),
  );
}
