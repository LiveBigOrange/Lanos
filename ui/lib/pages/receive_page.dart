import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';

import '../l10n/app_localizations.dart';
import '../services/api_client.dart';
import '../services/incoming_service.dart';
import '../utils/format.dart';

class ReceivePage extends StatefulWidget {
  const ReceivePage({super.key, required this.api});

  final ApiClient api;

  @override
  State<ReceivePage> createState() => _ReceivePageState();
}

class _ReceivePageState extends State<ReceivePage> {
  late final IncomingService _service;

  @override
  void initState() {
    super.initState();
    _service = IncomingService(widget.api);
    _service.start();
  }

  @override
  void dispose() {
    _service.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _service,
      builder: (context, _) {
        final items = _service.items;
        return Scaffold(
          appBar: AppBar(
            title: Text(AppLocalizations.of(context)!.receive),
          ),
          body: items.isEmpty ? _emptyState(context) : _list(items),
        );
      },
    );
  }

  Widget _emptyState(BuildContext context) {
    final theme = Theme.of(context);
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            Icons.file_download_outlined,
            size: 64,
            color: theme.colorScheme.primary.withValues(alpha: 0.4),
          ),
          const SizedBox(height: 12),
          Text(
            AppLocalizations.of(context)!.noReceiveRequests,
            style: theme.textTheme.bodyLarge,
          ),
          const SizedBox(height: 4),
          Text(
            AppLocalizations.of(context)!.noReceiveRequestsHint,
            style: theme.textTheme.bodySmall,
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }

  Widget _list(List<IncomingItem> items) {
    return ListView.builder(
      padding: const EdgeInsets.only(bottom: 16, top: 4),
      itemCount: items.length,
      itemBuilder: (context, i) => _itemCard(items[i]),
    );
  }

  Widget _itemCard(IncomingItem item) {
    final l10n = AppLocalizations.of(context)!;
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Icon(Icons.insert_drive_file_outlined, size: 20),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    item.fileName,
                    style: const TextStyle(fontWeight: FontWeight.w600),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 4),
            Text(
              '${formatBytes(item.fileSize)} · ${item.peerName}',
              style: Theme.of(context).textTheme.bodySmall,
            ),
            if (item.error != null)
              Padding(
                padding: const EdgeInsets.only(top: 4),
                child: Text(
                  item.error!,
                  style: TextStyle(
                    color: Theme.of(context).colorScheme.error,
                    fontSize: 12,
                  ),
                ),
              ),
            if (item.isPending || item.isReceiving) ...[
              const SizedBox(height: 12),
              Row(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  TextButton(
                    onPressed: item.isReceiving ? null : () => _reject(item.id),
                    child: Text(l10n.cancel),
                  ),
                  const SizedBox(width: 8),
                  FilledButton(
                    onPressed: item.isReceiving ? null : () => _accept(item),
                    child: Text(l10n.confirm),
                  ),
                ],
              ),
            ],
          ],
        ),
      ),
    );
  }

  Future<void> _accept(IncomingItem item) async {
    String? dir;
    try {
      dir = await FilePicker.getDirectoryPath();
    } catch (_) {
      // fallback
    }
    dir ??= (await getApplicationDocumentsDirectory()).path;
    final savePath = p.join(dir, p.basename(item.fileName));
    await _service.accept(item.id, savePath);
  }

  Future<void> _reject(String id) async {
    await _service.reject(id);
  }
}
