import 'package:flutter/material.dart';

import '../l10n/app_localizations.dart';
import '../services/api_client.dart';

class SettingsPage extends StatefulWidget {
  const SettingsPage({super.key, required this.api});

  final ApiClient api;

  @override
  State<SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends State<SettingsPage> {
  final _nameCtrl = TextEditingController();
  String _autoReceive = 'ask';
  String _conflictPolicy = 'skip';
  bool _stealth = false;
  bool _ipv6Preferred = true;
  bool _notifyReceive = true;
  bool _notifySent = true;
  bool _notifyShare = true;
  int _retention = 1000;
  bool _loading = true;
  String _version = '';

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final result = await widget.api.get('/api/v1/settings');
      final cfg = result['config'] as Map<String, dynamic>? ?? {};
      final ver = await widget.api.get('/api/v1/version');
      if (!mounted) return;
      setState(() {
        _nameCtrl.text = cfg['device_name'] as String? ?? '';
        _autoReceive = cfg['auto_receive'] as String? ?? 'ask';
        _conflictPolicy = cfg['conflict_policy'] as String? ?? 'skip';
        _stealth = cfg['stealth_mode'] as bool? ?? false;
        _ipv6Preferred = cfg['ipv6_preferred'] as bool? ?? true;
        _notifyReceive = cfg['notify_receive'] as bool? ?? true;
        _notifySent = cfg['notify_sent'] as bool? ?? true;
        _notifyShare = cfg['notify_share'] as bool? ?? true;
        _retention = cfg['record_retention'] as int? ?? 1000;
        _version = ver['version'] as String? ?? '';
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _loading = false);
      final l10n = AppLocalizations.of(context)!;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(l10n.loadSettingsFailed(e.toString()))),
      );
    }
  }

  Future<void> _save() async {
    final l10n = AppLocalizations.of(context)!;
    try {
      await widget.api.post('/api/v1/settings', {
        'device_name': _nameCtrl.text,
        'auto_receive': _autoReceive,
        'conflict_policy': _conflictPolicy,
        'stealth_mode': _stealth,
        'ipv6_preferred': _ipv6Preferred,
        'notify_receive': _notifyReceive,
        'notify_sent': _notifySent,
        'notify_share': _notifyShare,
        'record_retention': _retention,
      });
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(l10n.settingsSaved)),
      );
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(l10n.saveFailed(e.toString()))),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return Scaffold(
        appBar: AppBar(title: Text(AppLocalizations.of(context)!.settings)),
        body: const Center(child: CircularProgressIndicator()),
      );
    }
    final l10n = AppLocalizations.of(context)!;
    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.settings),
        actions: [
          IconButton(
            icon: const Icon(Icons.save),
            tooltip: l10n.saveSettings,
            onPressed: _save,
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.only(bottom: 32),
        children: [
          _section(l10n.sectionDevice, [
            _TextFieldTile(
              label: l10n.deviceName,
              controller: _nameCtrl,
            ),
          ]),
          _section(l10n.sectionTransfer, [
            _DropdownTile<String>(
              label: l10n.autoReceive,
              value: _autoReceive,
              items: [
                DropdownMenuItem(
                  value: 'ask',
                  child: Text(l10n.autoReceiveAsk),
                ),
                DropdownMenuItem(
                  value: 'trusted',
                  child: Text(l10n.autoReceiveTrusted),
                ),
                DropdownMenuItem(
                  value: 'all',
                  child: Text(l10n.autoReceiveAll),
                ),
              ],
              onChanged: (v) => setState(() => _autoReceive = v!),
            ),
            _DropdownTile<String>(
              label: l10n.conflictPolicy,
              value: _conflictPolicy,
              items: [
                DropdownMenuItem(
                  value: 'skip',
                  child: Text(l10n.conflictPolicySkip),
                ),
                DropdownMenuItem(
                  value: 'overwrite',
                  child: Text(l10n.conflictPolicyOverwrite),
                ),
                DropdownMenuItem(
                  value: 'keep_both',
                  child: Text(l10n.conflictPolicyKeepBoth),
                ),
              ],
              onChanged: (v) => setState(() => _conflictPolicy = v!),
            ),
          ]),
          _section(l10n.sectionNetwork, [
            _SwitchTile(
              label: l10n.stealthMode,
              subtitle: l10n.stealthModeHint,
              value: _stealth,
              onChanged: (v) => setState(() => _stealth = v),
            ),
            _SwitchTile(
              label: l10n.ipv6Preferred,
              subtitle: l10n.ipv6PreferredHint,
              value: _ipv6Preferred,
              onChanged: (v) => setState(() => _ipv6Preferred = v),
            ),
          ]),
          _section(l10n.sectionNotify, [
            _SwitchTile(
              label: l10n.notifyReceive,
              value: _notifyReceive,
              onChanged: (v) => setState(() => _notifyReceive = v),
            ),
            _SwitchTile(
              label: l10n.notifySent,
              value: _notifySent,
              onChanged: (v) => setState(() => _notifySent = v),
            ),
            _SwitchTile(
              label: l10n.notifyShare,
              value: _notifyShare,
              onChanged: (v) => setState(() => _notifyShare = v),
            ),
          ]),
          _section(l10n.sectionStorage, [
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
              child: Row(
                children: [
                  Text(l10n.recordRetention),
                  const Spacer(),
                  DropdownButton<int>(
                    value: _retention <= 0 ? 0 : _retention,
                    items: [
                      DropdownMenuItem(
                        value: 0,
                        child: Text(l10n.recordRetentionUnlimited),
                      ),
                      DropdownMenuItem(
                        value: 500,
                        child: Text(l10n.recordsN(500)),
                      ),
                      DropdownMenuItem(
                        value: 1000,
                        child: Text(l10n.recordsN(1000)),
                      ),
                      DropdownMenuItem(
                        value: 5000,
                        child: Text(l10n.recordsN(5000)),
                      ),
                    ],
                    onChanged: (v) => setState(() => _retention = v ?? 1000),
                  ),
                ],
              ),
            ),
          ]),
          _section(l10n.sectionAbout, [
            ListTile(
              leading: const Icon(Icons.info_outline),
              title: const Text('Lanos'),
              subtitle: Text(l10n.versionValue(_version)),
            ),
          ]),
        ],
      ),
    );
  }

  Widget _section(String title, List<Widget> children) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 20, 16, 4),
          child: Text(
            title,
            style: Theme.of(context).textTheme.labelMedium?.copyWith(
                  color: Theme.of(context).colorScheme.primary,
                  fontWeight: FontWeight.w600,
                ),
          ),
        ),
        ...children,
        const Divider(height: 1),
      ],
    );
  }
}

class _TextFieldTile extends StatelessWidget {
  const _TextFieldTile({required this.label, required this.controller});

  final String label;
  final TextEditingController controller;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      child: TextField(
        controller: controller,
        decoration: InputDecoration(
          labelText: label,
          border: const OutlineInputBorder(),
          isDense: true,
        ),
      ),
    );
  }
}

class _DropdownTile<T> extends StatelessWidget {
  const _DropdownTile({
    required this.label,
    required this.value,
    required this.items,
    required this.onChanged,
  });

  final String label;
  final T value;
  final List<DropdownMenuItem<T>> items;
  final ValueChanged<T?> onChanged;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      child: Row(
        children: [
          Text(label),
          const Spacer(),
          DropdownButton<T>(
            value: value,
            items: items,
            onChanged: onChanged,
          ),
        ],
      ),
    );
  }
}

class _SwitchTile extends StatelessWidget {
  const _SwitchTile({
    required this.label,
    this.subtitle,
    required this.value,
    required this.onChanged,
  });

  final String label;
  final String? subtitle;
  final bool value;
  final ValueChanged<bool> onChanged;

  @override
  Widget build(BuildContext context) {
    return SwitchListTile(
      title: Text(label),
      subtitle: subtitle != null ? Text(subtitle!) : null,
      value: value,
      onChanged: onChanged,
    );
  }
}
