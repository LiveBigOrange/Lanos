import 'package:flutter/material.dart';

import '../models/device.dart';
import '../services/api_client.dart';
import '../services/device_service.dart';
import '../widgets/device_card.dart';

/// Home page: shows the local device card at the top followed by a live
/// list of online peers discovered via mDNS.
///
/// P1 W2: polling-based (1s refresh via [DeviceService]). P1 W3 will switch
/// the device list to SSE for real-time updates; the send-file flow lands
/// in P1 W4.
class HomePage extends StatefulWidget {
  const HomePage({super.key, required this.api});

  final ApiClient api;

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  late final DeviceService _deviceService;

  @override
  void initState() {
    super.initState();
    _deviceService = DeviceService(widget.api);
    _deviceService.start();
  }

  @override
  void dispose() {
    _deviceService.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Lanos'),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            tooltip: '刷新设备列表',
            onPressed: _deviceService.refresh,
          ),
          IconButton(
            icon: const Icon(Icons.settings_outlined),
            tooltip: '设置',
            onPressed: () {
              // TODO P2 W7: settings page.
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(content: Text('设置页面将在 P2 阶段实现')),
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
              const _SectionHeader('附近设备'),
              Expanded(child: _peerList(context, snap)),
            ],
          );
        },
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () {
          // TODO P1 W4: open file picker + device picker to send.
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(content: Text('文件发送将在 P1 W4 实现')),
          );
        },
        icon: const Icon(Icons.send),
        label: const Text('发送文件'),
      ),
    );
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
              color: Theme.of(context).colorScheme.primary.withValues(alpha: 0.4),
            ),
            const SizedBox(height: 12),
            Text(
              '正在搜索附近设备...',
              style: Theme.of(context).textTheme.bodyLarge,
            ),
            const SizedBox(height: 4),
            Text(
              '确保其他设备已安装 Lanos 并连接同一 Wi-Fi',
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
          onTap: () {
            // TODO P1 W4: open send-file flow targeting this device.
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(content: Text('即将支持向 ${dev.name} 发送文件')),
            );
          },
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
                '正在重新连接 gcd…',
                style: TextStyle(color: Theme.of(context).colorScheme.onErrorContainer),
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
