import 'package:flutter/material.dart';

import '../l10n/app_localizations.dart';
import '../services/api_client.dart';
import '../services/transfer_service.dart';
import '../utils/transfer_stats.dart';
import '../widgets/transfer_progress_card.dart';

/// Transfer page: shows active and recent transfers split into two sections -
/// 上行 (outgoing) and 下行 (incoming) - each with live progress bars,
/// throughput, and ETA.
///
/// Per P1 W4 DoD, both directions render concurrently on a single screen so
/// bidirectional traffic over one long-lived connection is visible at a glance.
/// The page owns a [TransferService] (2s polling) and a per-transfer
/// [TransferStats] map so throughput/ETA survive widget rebuilds.
class TransferPage extends StatefulWidget {
  const TransferPage({super.key, required this.api, this.service});

  final ApiClient api;

  /// Optional pre-built [TransferService] for tests. When null the page
  /// creates and owns its own service bound to [api].
  final TransferService? service;

  @override
  State<TransferPage> createState() => _TransferPageState();
}

class _TransferPageState extends State<TransferPage> {
  late final TransferService _service;
  late final bool _ownsService;
  final Map<String, TransferStats> _stats = {};

  @override
  void initState() {
    super.initState();
    if (widget.service != null) {
      _service = widget.service!;
      _ownsService = false;
    } else {
      _service = TransferService(widget.api);
      _ownsService = true;
    }
    _service.addListener(_onChanged);
    _service.start();
  }

  @override
  void dispose() {
    _service.removeListener(_onChanged);
    if (_ownsService) {
      _service.dispose();
    }
    super.dispose();
  }

  void _onChanged() {
    if (!mounted) return;
    // Feed each live item's byte count into its stats tracker, and prune
    // trackers for transfers that have disappeared from the backend list.
    final liveIds = <String>{};
    for (final item in _service.items) {
      liveIds.add(item.id);
      var st = _stats[item.id];
      if (st == null) {
        st = TransferStats();
        _stats[item.id] = st;
      }
      final bytes = (item.progress * item.fileSize).round();
      st.record(bytes);
    }
    _stats.removeWhere((id, _) => !liveIds.contains(id));
    setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    final items = _service.items;
    final outgoing =
        items.where((t) => t.direction == TransferDirection.outgoing).toList();
    final incoming =
        items.where((t) => t.direction == TransferDirection.incoming).toList();

    return Scaffold(
      appBar: AppBar(title: Text(AppLocalizations.of(context)!.transfers)),
      body: items.isEmpty
          ? _emptyState(context)
          : ListView(
              padding: const EdgeInsets.only(top: 4, bottom: 16),
              children: [
                if (outgoing.isNotEmpty) ...[
                  _SectionHeader(
                    '${AppLocalizations.of(context)!.outgoing} (${outgoing.length})',
                  ),
                  for (final t in outgoing)
                    TransferProgressCard(
                      key: ValueKey(t.id),
                      item: t,
                      stats: _stats[t.id]!,
                      onCancel: () => _service.cancelTransfer(t.id),
                    ),
                ],
                if (incoming.isNotEmpty) ...[
                  _SectionHeader(
                    '${AppLocalizations.of(context)!.incoming} (${incoming.length})',
                  ),
                  for (final t in incoming)
                    TransferProgressCard(
                      key: ValueKey(t.id),
                      item: t,
                      stats: _stats[t.id]!,
                      onCancel: () => _service.cancelTransfer(t.id),
                    ),
                ],
              ],
            ),
    );
  }

  Widget _emptyState(BuildContext context) {
    final theme = Theme.of(context);
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            Icons.swap_vert,
            size: 64,
            color: theme.colorScheme.primary.withValues(alpha: 0.4),
          ),
          const SizedBox(height: 12),
          Text(
            AppLocalizations.of(context)!.noTransferProgress,
            style: theme.textTheme.bodyLarge,
          ),
          const SizedBox(height: 4),
          Text(
            AppLocalizations.of(context)!.noTransferProgressHint,
            style: theme.textTheme.bodySmall,
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }
}

class _SectionHeader extends StatelessWidget {
  const _SectionHeader(this.text);

  final String text;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 12, 20, 4),
      child: Align(
        alignment: Alignment.centerLeft,
        child: Text(
          text,
          style: Theme.of(context).textTheme.labelMedium?.copyWith(
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
        ),
      ),
    );
  }
}
