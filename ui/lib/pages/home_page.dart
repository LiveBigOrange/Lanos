import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import '../models/device.dart';
import '../services/api_client.dart';
import '../services/device_service.dart';
import '../services/transfer_service.dart';
import '../widgets/device_card.dart';
import '../l10n/app_localizations.dart';
import 'records_page.dart';
import 'settings_page.dart';
import 'transfer_page.dart';

/// Home page: shows the local device card at the top followed by a live
/// list of online peers discovered via mDNS.
///
/// P1 W2: polling-based (1s refresh via [DeviceService]). P1 W3 will switch
/// the device list to SSE for real-time updates; the send-file flow lands
/// in P1 W4.
class HomePage extends StatefulWidget {
  const HomePage({
    super.key,
    required this.api,
    this.transferService,
    this.deviceService,
  });

  final ApiClient api;

  final TransferService? transferService;

  final DeviceService? deviceService;

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  late final DeviceService _deviceService;
  bool _ownsDeviceService = false;

  @override
  void initState() {
    super.initState();
    if (widget.deviceService != null) {
      _deviceService = widget.deviceService!;
      _ownsDeviceService = false;
    } else {
      _deviceService = DeviceService(widget.api);
      _ownsDeviceService = true;
      _deviceService.start();
    }
  }

  @override
  void dispose() {
    if (_ownsDeviceService) _deviceService.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(AppLocalizations.of(context)!.appTitle),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            tooltip: AppLocalizations.of(context)!.refreshDevices,
            onPressed: _deviceService.refresh,
          ),
          IconButton(
            icon: const Icon(Icons.swap_vert),
            tooltip: AppLocalizations.of(context)!.transfers,
            onPressed: () {
              Navigator.push(
                context,
                MaterialPageRoute(
                  builder: (_) => TransferPage(
                    api: widget.api,
                    service: widget.transferService,
                  ),
                ),
              );
            },
          ),
          IconButton(
            icon: const Icon(Icons.history),
            tooltip: AppLocalizations.of(context)!.records,
            onPressed: () {
              Navigator.push(
                context,
                MaterialPageRoute(
                  builder: (_) => RecordsPage(
                    api: widget.api,
                    transferService: widget.transferService,
                  ),
                ),
              );
            },
          ),
          IconButton(
            icon: const Icon(Icons.settings_outlined),
            tooltip: AppLocalizations.of(context)!.settings,
            onPressed: () {
              Navigator.push(
                context,
                MaterialPageRoute(
                  builder: (_) => SettingsPage(api: widget.api),
                ),
              );
            },
          ),
        ],
      ),
      body: AnimatedBuilder(
        animation: _deviceService,
        builder: (context, _) {
          final snap = _deviceService.snapshot;
          return Column(
            children: [
              if (snap.hasError) _reconnectingBanner(context, snap.error!),
              if (snap.self != null)
                Padding(
                  padding: const EdgeInsets.only(top: 8),
                  child: DeviceCard(device: snap.self!, isSelf: true),
                ),
              _SectionHeader(AppLocalizations.of(context)!.nearbyDevices),
              Expanded(child: _peerList(context, snap)),
            ],
          );
        },
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _pickFileAndSelectDevice,
        icon: const Icon(Icons.send),
        label: Text(AppLocalizations.of(context)!.sendFile),
      ),
    );
  }

  Future<void> _pickFileAndSelectDevice({String? targetPeerId}) async {
    final result = await FilePicker.pickFiles(
      allowMultiple: false,
    );
    if (result == null || result.files.isEmpty) return;
    final path = result.files.single.path;
    if (path == null) return;
    if (!mounted) return;

    if (targetPeerId != null) {
      await _sendFile(targetPeerId, path);
      return;
    }

    final snap = _deviceService.snapshot;
    if (snap.peers.isEmpty) return;
    if (snap.peers.length == 1) {
      await _sendFile(snap.peers.first.id, path);
      return;
    }

    final peer = await showDialog<Device>(
      context: context,
      builder: (ctx) => SimpleDialog(
        title: Text(AppLocalizations.of(context)!.selectDevice),
        children: snap.peers
            .map(
              (d) => SimpleDialogOption(
                onPressed: () => Navigator.pop(ctx, d),
                child: Text(d.name),
              ),
            )
            .toList(),
      ),
    );
    if (peer != null) await _sendFile(peer.id, path);
  }

  Future<void> _sendFile(String peerId, String filePath) async {
    try {
      await widget.api.post('/api/v1/transfers', {
        'peer_id': peerId,
        'file_path': filePath,
      });
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(AppLocalizations.of(context)!.sendingStarted),
          ),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(AppLocalizations.of(context)!.sendFailed('$e')),
          ),
        );
      }
    }
  }

  Widget _peerList(BuildContext context, DeviceSnapshot snap) {
    if (snap.peers.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.wifi_tethering,
              size: 64,
              color:
                  Theme.of(context).colorScheme.primary.withValues(alpha: 0.4),
            ),
            const SizedBox(height: 12),
            Text(
              AppLocalizations.of(context)!.searchingDevices,
              style: Theme.of(context).textTheme.bodyLarge,
            ),
            const SizedBox(height: 4),
            Text(
              AppLocalizations.of(context)!.searchingDevicesHint,
              style: Theme.of(context).textTheme.bodySmall,
              textAlign: TextAlign.center,
            ),
          ],
        ),
      );
    }
    return ListView.builder(
      padding: const EdgeInsets.only(bottom: 88, top: 4),
      itemCount: snap.peers.length,
      itemBuilder: (context, i) {
        final dev = snap.peers[i];
        return DeviceCard(
          device: dev,
          onTap: () => _pickFileAndSelectDevice(targetPeerId: dev.id),
        );
      },
    );
  }

  Widget _reconnectingBanner(BuildContext context, String error) {
    return Material(
      color: Theme.of(context).colorScheme.errorContainer,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        child: Row(
          children: [
            Icon(
              Icons.cloud_off,
              size: 18,
              color: Theme.of(context).colorScheme.onErrorContainer,
            ),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                AppLocalizations.of(context)!.reconnectingGcd,
                style: TextStyle(
                  color: Theme.of(context).colorScheme.onErrorContainer,
                ),
              ),
            ),
            const SizedBox(
              width: 14,
              height: 14,
              child: CircularProgressIndicator(strokeWidth: 2),
            ),
          ],
        ),
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
