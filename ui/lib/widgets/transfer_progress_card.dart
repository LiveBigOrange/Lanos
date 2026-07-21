import 'package:flutter/material.dart';

import '../l10n/app_localizations.dart';
import '../services/transfer_service.dart';
import '../utils/format.dart';
import '../utils/transfer_stats.dart';

/// A single transfer row showing filename, peer, an animated progress bar,
/// live throughput, ETA, and (for active transfers) a cancel button.
///
/// The progress fraction is tween-animated over [progressAnimDuration] so the
/// bar glides between the ~2s backend polls instead of stepping - this keeps
/// the bar visually smooth (the DoD's "60fps 不卡顿") even though the data
/// itself only refreshes every couple of seconds.
///
/// Up (outgoing) and down (incoming) transfers render with distinct direction
/// icons and accent colors so the two directions stay visually separable when
/// many transfers run concurrently on a single long-lived connection.
class TransferProgressCard extends StatelessWidget {
  const TransferProgressCard({
    super.key,
    required this.item,
    required this.stats,
    this.onCancel,
    this.progressAnimDuration = const Duration(milliseconds: 300),
  });

  final TransferItem item;
  final TransferStats stats;
  final VoidCallback? onCancel;
  final Duration progressAnimDuration;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final cs = theme.colorScheme;
    final l10n = AppLocalizations.of(context)!;
    final isOutgoing = item.direction == TransferDirection.outgoing;
    final isActive = item.status == TransferStatus.pending ||
        item.status == TransferStatus.transferring;

    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 12, 12, 12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _header(theme, cs, l10n, isOutgoing),
            const SizedBox(height: 8),
            _progressBar(theme, cs, isOutgoing),
            const SizedBox(height: 6),
            _footer(theme, cs, l10n, isActive),
          ],
        ),
      ),
    );
  }

  Widget _header(
    ThemeData theme,
    ColorScheme cs,
    AppLocalizations l10n,
    bool isOutgoing,
  ) {
    return Row(
      children: [
        Icon(
          isOutgoing ? Icons.arrow_upward : Icons.arrow_downward,
          size: 18,
          color: isOutgoing ? cs.primary : cs.tertiary,
        ),
        const SizedBox(width: 8),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                item.fileName.isEmpty ? '(unknown)' : item.fileName,
                overflow: TextOverflow.ellipsis,
                style: theme.textTheme.bodyMedium?.copyWith(
                  decoration: item.status == TransferStatus.cancelled
                      ? TextDecoration.lineThrough
                      : null,
                ),
              ),
              Text(
                _peerLine(),
                overflow: TextOverflow.ellipsis,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: cs.onSurfaceVariant,
                ),
              ),
            ],
          ),
        ),
        _statusChip(theme, cs, l10n),
      ],
    );
  }

  Widget _progressBar(ThemeData theme, ColorScheme cs, bool isOutgoing) {
    final fraction =
        item.fileSize > 0 ? item.progress.clamp(0.0, 1.0).toDouble() : 0.0;
    final color = isOutgoing ? cs.primary : cs.tertiary;
    return Row(
      children: [
        Expanded(
          child: ClipRRect(
            borderRadius: BorderRadius.circular(4),
            child: TweenAnimationBuilder<double>(
              tween: Tween<double>(begin: 0, end: fraction),
              duration: progressAnimDuration,
              curve: Curves.easeOut,
              builder: (context, value, _) {
                return LinearProgressIndicator(
                  value: value,
                  minHeight: 8,
                  backgroundColor: cs.surfaceContainerHighest,
                  color: color,
                );
              },
            ),
          ),
        ),
        const SizedBox(width: 8),
        SizedBox(
          width: 44,
          child: Text(
            '${(fraction * 100).round()}%',
            textAlign: TextAlign.end,
            style: theme.textTheme.labelSmall?.copyWith(
              color: cs.onSurfaceVariant,
            ),
          ),
        ),
      ],
    );
  }

  Widget _footer(
    ThemeData theme,
    ColorScheme cs,
    AppLocalizations l10n,
    bool isActive,
  ) {
    final speed = isActive ? stats.speedBytesPerSecond : 0.0;
    final eta = isActive ? stats.eta(item.fileSize) : null;
    return Row(
      children: [
        Text(
          '${formatBytes((item.progress * item.fileSize).round())}'
          ' / ${formatBytes(item.fileSize)}',
          style: theme.textTheme.labelSmall?.copyWith(
            color: cs.onSurfaceVariant,
          ),
        ),
        const SizedBox(width: 12),
        if (isActive) ...[
          Icon(Icons.bolt, size: 12, color: cs.onSurfaceVariant),
          const SizedBox(width: 2),
          Text(
            formatSpeed(speed),
            style: theme.textTheme.labelSmall?.copyWith(
              color: cs.onSurfaceVariant,
            ),
          ),
          const SizedBox(width: 12),
          Icon(Icons.schedule, size: 12, color: cs.onSurfaceVariant),
          const SizedBox(width: 2),
          Text(
            formatDuration(eta, doneLabel: l10n.etaComplete),
            style: theme.textTheme.labelSmall?.copyWith(
              color: cs.onSurfaceVariant,
            ),
          ),
        ],
        const Spacer(),
        if (isActive && onCancel != null)
          IconButton(
            icon: const Icon(Icons.close, size: 18),
            tooltip: l10n.cancelTooltip,
            padding: EdgeInsets.zero,
            constraints: const BoxConstraints(minWidth: 32, minHeight: 32),
            onPressed: onCancel,
          ),
        if (item.status == TransferStatus.failed && item.error != null)
          Tooltip(
            message: item.error!,
            child: Icon(Icons.error_outline, size: 16, color: cs.error),
          ),
      ],
    );
  }

  Widget _statusChip(ThemeData theme, ColorScheme cs, AppLocalizations l10n) {
    final (label, fg, bg) = switch (item.status) {
      TransferStatus.pending => (
          l10n.statusWaiting,
          cs.onSurfaceVariant,
          cs.surfaceContainerHighest
        ),
      TransferStatus.transferring => (
          l10n.statusTransferring,
          cs.onPrimaryContainer,
          cs.primaryContainer
        ),
      TransferStatus.completed => (
          l10n.statusCompleted,
          cs.onPrimaryContainer,
          cs.primary
        ),
      TransferStatus.failed => (
          l10n.statusFailed,
          cs.onErrorContainer,
          cs.errorContainer
        ),
      TransferStatus.cancelled => (
          l10n.statusCancelled,
          cs.onSurfaceVariant,
          cs.surfaceContainerHighest
        ),
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(10),
      ),
      child: Text(
        label,
        style: theme.textTheme.labelSmall?.copyWith(color: fg),
      ),
    );
  }

  String _peerLine() {
    final name = item.peer.name.isEmpty ? 'Unknown' : item.peer.name;
    return item.direction == TransferDirection.outgoing ? '→ $name' : '← $name';
  }
}
