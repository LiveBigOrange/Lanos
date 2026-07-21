import 'package:flutter/material.dart';

import '../l10n/app_localizations.dart';
import '../services/api_client.dart';
import '../services/share_history_service.dart';
import '../services/transfer_service.dart';
import '../utils/format.dart';

class RecordsPage extends StatefulWidget {
  const RecordsPage({super.key, required this.api, this.transferService});

  final ApiClient api;

  /// Optional shared transfer service. When provided the page skips creating
  /// its own, sharing the polling loop with the app-level service.
  final TransferService? transferService;

  @override
  State<RecordsPage> createState() => _RecordsPageState();
}

class _RecordsPageState extends State<RecordsPage>
    with SingleTickerProviderStateMixin {
  late final TabController _tabController;
  late final TransferService _transferService;
  bool _ownsTransferService = false;
  late final ShareHistoryService _shareService;
  final TextEditingController _searchCtrl = TextEditingController();
  final Set<String> _selectedIds = {};
  bool _selectMode = false;
  String _sortBy = 'time';
  String _sortOrder = 'desc';

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 2, vsync: this);
    if (widget.transferService != null) {
      _transferService = widget.transferService!;
      _ownsTransferService = false;
    } else {
      _transferService = TransferService(widget.api);
      _ownsTransferService = true;
    }
    _shareService = ShareHistoryService(widget.api);
    _transferService.start();
    _shareService.start();
    _transferService.addListener(_onChanged);
    _shareService.addListener(_onChanged);
  }

  @override
  void dispose() {
    _tabController.dispose();
    _transferService.removeListener(_onChanged);
    _shareService.removeListener(_onChanged);
    if (_ownsTransferService) _transferService.dispose();
    _shareService.dispose();
    _searchCtrl.dispose();
    super.dispose();
  }

  void _onChanged() {
    if (!mounted) return;
    setState(() {});
  }

  void _toggleSelectMode() {
    setState(() {
      _selectMode = !_selectMode;
      if (!_selectMode) _selectedIds.clear();
    });
  }

  void _toggleSelection(String id) {
    setState(() {
      if (_selectedIds.contains(id)) {
        _selectedIds.remove(id);
      } else {
        _selectedIds.add(id);
      }
    });
  }


  Future<void> _deleteSelected(int tab) async {
    final l10n = AppLocalizations.of(context)!;
    if (_selectedIds.isEmpty) return;
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(l10n.deleteConfirmTitle),
        content: Text(l10n.deleteConfirm(_selectedIds.length)),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: Text(l10n.cancel),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: Text(l10n.delete),
          ),
        ],
      ),
    );
    if (ok != true) return;
    final prefix = tab == 0 ? 'transfers' : 'shares';
    for (final id in _selectedIds) {
      try {
        await widget.api.delete('/api/v1/$prefix/$id');
      } catch (_) {}
    }
    _selectedIds.clear();
    _transferService.refresh();
    _shareService.refresh();
  }

  Future<void> _exportCSV(String type) async {
    final l10n = AppLocalizations.of(context)!;
    try {
      final csv = await widget.api.getRaw('/api/v1/$type/export');
      if (!mounted) return;
      if (csv.length > 500) {
        // On desktop this would use file_selector to save to disk.
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.csvObtained)),
        );
      }
      showDialog(
        context: context,
        builder: (ctx) => AlertDialog(
          title: Text('$type.csv'),
          content: SizedBox(
            width: double.maxFinite,
            child: Text(
              csv.substring(0, csv.length > 1000 ? 1000 : csv.length),
              style: const TextStyle(fontFamily: 'monospace', fontSize: 11),
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: Text(l10n.close),
            ),
          ],
        ),
      );
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(l10n.exportFailed(e.toString()))),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.records),
        bottom: TabBar(
          controller: _tabController,
          tabs: [
            Tab(text: l10n.transferRecords),
            Tab(text: l10n.shareRecords),
          ],
        ),
        actions: [
          IconButton(
            icon: const Icon(Icons.search),
            onPressed: () => _showSearch(),
          ),
          IconButton(
            icon: Icon(_selectMode ? Icons.close : Icons.checklist),
            tooltip: _selectMode ? l10n.exitSelect : l10n.multiSelect,
            onPressed: _toggleSelectMode,
          ),
          if (_selectedIds.isNotEmpty)
            IconButton(
              icon: const Icon(Icons.delete),
              onPressed: () => _deleteSelected(_tabController.index),
            ),
          PopupMenuButton<String>(
            onSelected: (v) {
              if (v == 'sort_time') {
                setState(() {
                  _sortBy = 'time';
                });
              } else if (v == 'sort_size') {
                setState(() {
                  _sortBy = 'size';
                });
              } else if (v == 'sort_name') {
                setState(() {
                  _sortBy = 'name';
                });
              } else if (v == 'sort_status') {
                setState(() {
                  _sortBy = 'status';
                });
              } else if (v == 'order_asc') {
                setState(() {
                  _sortOrder = 'asc';
                });
              } else if (v == 'order_desc') {
                setState(() {
                  _sortOrder = 'desc';
                });
              } else if (v == 'export_transfers') {
                _exportCSV('transfers');
              } else if (v == 'export_shares') {
                _exportCSV('shares');
              }
            },
            itemBuilder: (context) => [
              PopupMenuItem(
                value: 'sort_time',
                child:
                    Text('${l10n.sortByTime}${_sortBy == 'time' ? ' ✓' : ''}'),
              ),
              PopupMenuItem(
                value: 'sort_size',
                child:
                    Text('${l10n.sortBySize}${_sortBy == 'size' ? ' ✓' : ''}'),
              ),
              PopupMenuItem(
                value: 'sort_name',
                child:
                    Text('${l10n.sortByName}${_sortBy == 'name' ? ' ✓' : ''}'),
              ),
              PopupMenuItem(
                value: 'sort_status',
                child: Text(
                  '${l10n.sortByStatus}${_sortBy == 'status' ? ' ✓' : ''}',
                ),
              ),
              const PopupMenuDivider(),
              PopupMenuItem(
                value: 'order_desc',
                child: Text(
                  '${l10n.descending}${_sortOrder == 'desc' ? ' ✓' : ''}',
                ),
              ),
              PopupMenuItem(
                value: 'order_asc',
                child:
                    Text('${l10n.ascending}${_sortOrder == 'asc' ? ' ✓' : ''}'),
              ),
              const PopupMenuDivider(),
              PopupMenuItem(
                value: 'export_transfers',
                child: Text(l10n.exportTransfersCSV),
              ),
              PopupMenuItem(
                value: 'export_shares',
                child: Text(l10n.exportSharesCSV),
              ),
            ],
          ),
        ],
      ),
      body: TabBarView(
        controller: _tabController,
        children: [
          _buildTransferTab(),
          _buildShareTab(),
        ],
      ),
    );
  }

  Widget _buildTransferTab() {
    final l10n = AppLocalizations.of(context)!;
    if (_transferService.items.isEmpty) {
      return _emptyState(l10n.noTransferRecords, l10n.noTransferRecordsHint);
    }
    return ListView.builder(
      padding: const EdgeInsets.only(bottom: 16),
      itemCount: _transferService.items.length,
      itemBuilder: (context, i) {
        final t = _transferService.items[i];
        final selected = _selectedIds.contains(t.id);
        final dirLabel = t.direction == TransferDirection.outgoing
            ? l10n.directionSend
            : l10n.directionReceive;
        final statusLabel = switch (t.status) {
          TransferStatus.completed => l10n.statusCompleted,
          TransferStatus.failed => l10n.statusFailed,
          TransferStatus.cancelled => l10n.statusCancelled,
          TransferStatus.pending => l10n.statusPending,
          _ => l10n.statusTransferring,
        };
        final timeStr =
            t.startedAt != null ? formatDateTime(t.startedAt!, l10n) : '';
        return Card(
          margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 2),
          child: ListTile(
            selected: selected,
            leading: _selectMode
                ? Checkbox(
                    value: selected,
                    onChanged: (_) => _toggleSelection(t.id),
                  )
                : Icon(
                    t.direction == TransferDirection.outgoing
                        ? Icons.arrow_upward
                        : Icons.arrow_downward,
                    color: t.status == TransferStatus.completed
                        ? Colors.green
                        : t.status == TransferStatus.failed
                            ? Colors.red
                            : null,
                  ),
            title:
                Text(t.fileName, maxLines: 1, overflow: TextOverflow.ellipsis),
            subtitle:
                Text('$dirLabel · $statusLabel · ${formatBytes(t.fileSize)}'),
            trailing: timeStr.isNotEmpty
                ? Text(timeStr, style: Theme.of(context).textTheme.bodySmall)
                : null,
            onLongPress: _selectMode
                ? null
                : () {
                    _toggleSelectMode();
                    _toggleSelection(t.id);
                  },
          ),
        );
      },
    );
  }

  Widget _buildShareTab() {
    final l10n = AppLocalizations.of(context)!;
    if (_shareService.items.isEmpty) {
      return _emptyState(l10n.noShareRecords, l10n.noShareRecordsHint);
    }
    return ListView.builder(
      padding: const EdgeInsets.only(bottom: 16),
      itemCount: _shareService.items.length,
      itemBuilder: (context, i) {
        final s = _shareService.items[i];
        final selected = _selectedIds.contains(s.id);
        return Card(
          margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 2),
          child: ListTile(
            selected: selected,
            leading: _selectMode
                ? Checkbox(
                    value: selected,
                    onChanged: (_) => _toggleSelection(s.id),
                  )
                : const Icon(Icons.link),
            title: Text(
              s.filePath.isNotEmpty ? s.filePath.split('/').last : s.id,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
            subtitle: Text(
              '${s.statusLabel(l10n)} · ${formatBytes(s.size)} · ${l10n.downloadsCount(s.downloads)}',
            ),
            trailing: Text(
              formatDateTime(s.createdAt, l10n),
              style: Theme.of(context).textTheme.bodySmall,
            ),
            onLongPress: _selectMode
                ? null
                : () {
                    _toggleSelectMode();
                    _toggleSelection(s.id);
                  },
          ),
        );
      },
    );
  }

  Widget _emptyState(String title, String hint) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            Icons.history,
            size: 64,
            color: Theme.of(context).colorScheme.primary.withValues(alpha: 0.4),
          ),
          const SizedBox(height: 12),
          Text(title, style: Theme.of(context).textTheme.bodyLarge),
          const SizedBox(height: 4),
          Text(
            hint,
            style: Theme.of(context).textTheme.bodySmall,
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }

  void _showSearch() async {
    final l10n = AppLocalizations.of(context)!;
    final q = await showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(l10n.search),
        content: TextField(
          autofocus: true,
          decoration: InputDecoration(
            hintText: l10n.searchHint,
            prefixIcon: const Icon(Icons.search),
          ),
          onSubmitted: (v) => Navigator.pop(ctx, v),
        ),
      ),
    );
    if (q != null && q.isNotEmpty) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(l10n.searchQueryHint(q))),
      );
    }
  }
}

String formatDateTime(DateTime dt, AppLocalizations l10n) {
  final now = DateTime.now();
  final diff = now.difference(dt);
  if (diff.inMinutes < 60) return l10n.minutesAgo(diff.inMinutes);
  if (diff.inHours < 24) return l10n.hoursAgo(diff.inHours);
  if (diff.inDays < 7) return l10n.daysAgo(diff.inDays);
  return l10n.monthDay(dt.day.toString(), dt.month.toString());
}
