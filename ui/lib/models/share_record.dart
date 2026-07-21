import 'package:flutter/foundation.dart';

import '../l10n/app_localizations.dart';

@immutable
class ShareRecord {
  const ShareRecord({
    required this.id,
    required this.kind,
    required this.target,
    required this.filePath,
    required this.size,
    required this.status,
    required this.createdAt,
    this.expiresAt,
    this.downloads = 0,
    this.maxDownloads,
  });

  final String id;
  final String kind;
  final String target;
  final String filePath;
  final int size;
  final String status;
  final DateTime createdAt;
  final DateTime? expiresAt;
  final int downloads;
  final int? maxDownloads;

  factory ShareRecord.fromJson(Map<String, dynamic> json) {
    return ShareRecord(
      id: json['id'] as String? ?? '',
      kind: json['kind'] as String? ?? '',
      target: json['target'] as String? ?? '',
      filePath: json['file_path'] as String? ?? '',
      size: json['size'] as int? ?? 0,
      status: json['status'] as String? ?? '',
      createdAt: json['created_at'] != null
          ? DateTime.parse(json['created_at'] as String)
          : DateTime.now(),
      expiresAt: json['expires_at'] != null
          ? DateTime.tryParse(json['expires_at'] as String)
          : null,
      downloads: json['downloads'] as int? ?? 0,
      maxDownloads: json['max_downloads'] as int?,
    );
  }

  String statusLabel(AppLocalizations l10n) {
    switch (status) {
      case 'active':
        return l10n.shareStatusActive;
      case 'expired':
        return l10n.shareStatusExpired;
      case 'stopped':
        return l10n.shareStatusStopped;
      case 'completed':
        return l10n.shareStatusCompleted;
      default:
        return status;
    }
  }
}
