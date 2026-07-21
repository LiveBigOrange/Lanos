import 'package:flutter/material.dart';

import '../l10n/app_localizations.dart';
import '../models/device.dart';

/// A single row representing a discovered device.
///
/// Shows the platform icon, name, address and (for self) a "本机" badge.
/// The [onTap] callback triggers the send-file flow when provided.
class DeviceCard extends StatelessWidget {
  const DeviceCard({
    super.key,
    required this.device,
    this.isSelf = false,
    this.onTap,
  });

  final Device device;
  final bool isSelf;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final cs = theme.colorScheme;

    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
      child: ListTile(
        onTap: onTap,
        leading: CircleAvatar(
          backgroundColor:
              isSelf ? cs.primaryContainer : cs.surfaceContainerHighest,
          foregroundColor: isSelf ? cs.onPrimaryContainer : cs.onSurfaceVariant,
          child: Icon(iconForPlatform(device.platform)),
        ),
        title: Row(
          children: [
            Flexible(
              child: Text(
                device.name.isEmpty ? '(unnamed)' : device.name,
                overflow: TextOverflow.ellipsis,
                style: theme.textTheme.titleMedium,
              ),
            ),
            if (isSelf) ...[
              const SizedBox(width: 8),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                decoration: BoxDecoration(
                  color: cs.primaryContainer,
                  borderRadius: BorderRadius.circular(4),
                ),
                child: Text(
                  AppLocalizations.of(context)!.thisDevice,
                  style: theme.textTheme.labelSmall
                      ?.copyWith(color: cs.onPrimaryContainer),
                ),
              ),
            ],
          ],
        ),
        subtitle: Text(
          _subtitle(),
          style: theme.textTheme.bodySmall,
          overflow: TextOverflow.ellipsis,
        ),
        trailing: _ipVersionBadge(theme),
      ),
    );
  }

  String _subtitle() {
    final parts = <String>[];
    parts.add(labelForPlatform(device.platform));
    if (device.primaryAddress.isNotEmpty &&
        device.primaryAddress != device.hostname) {
      parts.add(device.primaryAddress);
    }
    return parts.join(' · ');
  }

  Widget? _ipVersionBadge(ThemeData theme) {
    if (device.ipVersion == '46') {
      return Container(
        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
        decoration: BoxDecoration(
          border: Border.all(color: theme.colorScheme.outlineVariant),
          borderRadius: BorderRadius.circular(4),
        ),
        child: Text('IPv4+6', style: theme.textTheme.labelSmall),
      );
    }
    if (device.ipVersion == '6') {
      return Container(
        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
        decoration: BoxDecoration(
          border: Border.all(color: theme.colorScheme.outlineVariant),
          borderRadius: BorderRadius.circular(4),
        ),
        child: Text('IPv6', style: theme.textTheme.labelSmall),
      );
    }
    return null;
  }
}
