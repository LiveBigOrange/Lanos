import 'package:flutter/material.dart';

/// A discovered Lanos device (either the local device or a peer).
///
/// Mirrors the JSON shape of `GET /api/v1/devices` from PRD §5.4.
@immutable
class Device {
  const Device({
    required this.id,
    required this.name,
    required this.platform,
    required this.port,
    required this.pubHash,
    required this.ipVersion,
    this.ipv4 = const [],
    this.ipv6 = const [],
    this.hostname = '',
    this.firstSeen,
    this.lastSeen,
  });

  /// Device identifier (= pubHash). Used to deduplicate announcements.
  final String id;

  /// Human-readable device name (URL-decoded).
  final String name;

  /// Platform string: linux / darwin / windows / android / ios.
  final String platform;

  /// Peer's Lanos API+transfer TCP port.
  final int port;

  /// Public-key hash (32 hex chars = 16 bytes).
  final String pubHash;

  /// Advertised IP capability: "4" | "6" | "46".
  final String ipVersion;

  /// Resolved IPv4 addresses (may be empty until AAAA completes).
  final List<String> ipv4;

  /// Resolved IPv6 global unicast addresses.
  final List<String> ipv6;

  /// mDNS hostname (e.g. "macbook.local.").
  final String hostname;

  final DateTime? firstSeen;
  final DateTime? lastSeen;

  factory Device.fromJson(Map<String, dynamic> json) {
    return Device(
      id: json['id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      platform: json['platform'] as String? ?? '',
      port: (json['port'] as num?)?.toInt() ?? 0,
      pubHash: json['pub_hash'] as String? ?? '',
      ipVersion: json['ip_version'] as String? ?? '4',
      ipv4: (json['ipv4'] as List?)?.map((e) => e as String).toList() ?? const [],
      ipv6: (json['ipv6'] as List?)?.map((e) => e as String).toList() ?? const [],
      hostname: json['hostname'] as String? ?? '',
      firstSeen: _parseTime(json['first_seen']),
      lastSeen: _parseTime(json['last_seen']),
    );
  }

  static DateTime? _parseTime(dynamic v) {
    if (v is! String || v.isEmpty) return null;
    return DateTime.tryParse(v);
  }

  /// The primary address to display. Prefers IPv4, then IPv6, then hostname.
  String get primaryAddress {
    if (ipv4.isNotEmpty) return ipv4.first;
    if (ipv6.isNotEmpty) return ipv6.first;
    return hostname;
  }

  /// Whether this device advertises IPv6 support.
  bool get supportsIPv6 => ipVersion == '6' || ipVersion == '46';

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is Device && runtimeType == other.runtimeType && id == other.id && name == other.name && port == other.port && pubHash == other.pubHash && ipVersion == other.ipVersion && _listEq(ipv4, other.ipv4) && _listEq(ipv6, other.ipv6) && hostname == other.hostname;

  @override
  int get hashCode => Object.hash(id, name, port, pubHash, ipVersion, hostname);

  static bool _listEq(List<String> a, List<String> b) {
    if (a.length != b.length) return false;
    for (var i = 0; i < a.length; i++) {
      if (a[i] != b[i]) return false;
    }
    return true;
  }
}

/// Maps a platform string to a representative IconData.
IconData iconForPlatform(String platform) {
  switch (platform) {
    case 'linux':
      return Icons.computer;
    case 'darwin':
      return Icons.laptop_mac;
    case 'windows':
      return Icons.laptop_windows;
    case 'android':
      return Icons.phone_android;
    case 'ios':
      return Icons.phone_iphone;
    default:
      return Icons.devices_other;
  }
}

/// Human-readable label for the platform string.
String labelForPlatform(String platform) {
  switch (platform) {
    case 'linux':
      return 'Linux';
    case 'darwin':
      return 'macOS';
    case 'windows':
      return 'Windows';
    case 'android':
      return 'Android';
    case 'ios':
      return 'iOS';
    default:
      return platform.isEmpty ? 'Unknown' : platform;
  }
}
