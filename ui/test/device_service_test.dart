import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:lanos/models/device.dart';
import 'package:lanos/services/device_service.dart';

void main() {
  group('DeviceService', () {
    test('emits a snapshot when fetch succeeds', () async {
      final svc = DeviceService.withFetcher(
        () async => {
          'self': {
            'id': 'a' * 32,
            'name': 'MyMac',
            'platform': 'darwin',
            'port': 52100,
            'pub_hash': 'a' * 32,
            'ip_version': '46',
            'hostname': 'mymac.local.',
          },
          'peers': [
            {
              'id': 'b' * 32,
              'name': 'WinBox',
              'platform': 'windows',
              'port': 52150,
              'pub_hash': 'b' * 32,
              'ip_version': '4',
              'ipv4': ['192.168.1.20'],
              'hostname': 'winbox.local.',
            }
          ],
        },
        const Duration(milliseconds: 10),
      );

      final snapshots = <DeviceSnapshot>[];
      svc.addListener(() => snapshots.add(svc.snapshot));
      svc.start();

      // Wait for the first fetch + notify to fire.
      await Future<void>.delayed(const Duration(milliseconds: 50));

      expect(snapshots, isNotEmpty);
      expect(snapshots.last.self?.name, 'MyMac');
      expect(snapshots.last.peers, hasLength(1));
      expect(snapshots.last.peers.first.name, 'WinBox');
      expect(snapshots.last.peers.first.ipv4, ['192.168.1.20']);
      expect(snapshots.last.hasError, isFalse);

      svc.dispose();
    });

    test('records error without losing prior snapshot', () async {
      var call = 0;
      final svc = DeviceService.withFetcher(
        () async {
          call++;
          if (call == 1) {
            return {
              'self': {
                'id': 'a' * 32,
                'name': 'MyMac',
                'platform': 'darwin',
                'port': 52100,
                'pub_hash': 'a' * 32,
                'ip_version': '46',
              },
              'peers': <Map<String, dynamic>>[],
            };
          }
          throw Exception('network down');
        },
        const Duration(milliseconds: 10),
      );

      final snapshots = <DeviceSnapshot>[];
      svc.addListener(() => snapshots.add(svc.snapshot));
      svc.start();

      // Wait long enough for at least two fetches.
      await Future<void>.delayed(const Duration(milliseconds: 80));

      expect(snapshots.length, greaterThanOrEqualTo(2));
      // The error snapshot should still carry the self device from before.
      final errSnap = snapshots.lastWhere((s) => s.hasError);
      expect(errSnap.self?.name, 'MyMac');
      expect(errSnap.error, isNotNull);

      svc.dispose();
    });

    test('stop cancels the timer', () async {
      var calls = 0;
      final svc = DeviceService.withFetcher(
        () async {
          calls++;
          return {'self': null, 'peers': <Map<String, dynamic>>[]};
        },
        const Duration(milliseconds: 5),
      );
      svc.start();
      await Future<void>.delayed(const Duration(milliseconds: 20));
      final callsBeforeStop = calls;
      svc.stop();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      // After stop, calls should not keep climbing.
      expect(calls - callsBeforeStop, lessThanOrEqualTo(1));
      svc.dispose();
    });
  });

  group('Device model', () {
    test('parses minimal JSON', () {
      final d = Device.fromJson({
        'id': 'abc',
        'name': 'Foo',
        'platform': 'linux',
        'port': 52100,
        'pub_hash': 'abc',
        'ip_version': '4',
      });
      expect(d.name, 'Foo');
      expect(d.platform, 'linux');
      expect(d.port, 52100);
      expect(d.ipv4, isEmpty);
      expect(d.primaryAddress, isEmpty);
    });

    test('primaryAddress prefers IPv4 then IPv6 then hostname', () {
      expect(
        Device(id: 'a', name: 'n', platform: 'linux', port: 1, pubHash: 'a', ipVersion: '4', ipv4: ['1.2.3.4'], hostname: 'h.local').primaryAddress,
        '1.2.3.4',
      );
      expect(
        Device(id: 'a', name: 'n', platform: 'linux', port: 1, pubHash: 'a', ipVersion: '6', ipv6: ['fe80::1'], hostname: 'h.local').primaryAddress,
        'fe80::1',
      );
      expect(
        Device(id: 'a', name: 'n', platform: 'linux', port: 1, pubHash: 'a', ipVersion: '4', hostname: 'h.local').primaryAddress,
        'h.local',
      );
    });

    test('supportsIPv6 reflects ip_version', () {
      expect(Device(id: 'a', name: 'n', platform: '', port: 1, pubHash: 'a', ipVersion: '4').supportsIPv6, isFalse);
      expect(Device(id: 'a', name: 'n', platform: '', port: 1, pubHash: 'a', ipVersion: '6').supportsIPv6, isTrue);
      expect(Device(id: 'a', name: 'n', platform: '', port: 1, pubHash: 'a', ipVersion: '46').supportsIPv6, isTrue);
    });

    test('iconForPlatform returns sensible icons', () {
      expect(iconForPlatform('linux'), Icons.computer);
      expect(iconForPlatform('darwin'), Icons.laptop_mac);
      expect(iconForPlatform('windows'), Icons.laptop_windows);
      expect(iconForPlatform('android'), Icons.phone_android);
      expect(iconForPlatform('ios'), Icons.phone_iphone);
      expect(iconForPlatform(''), Icons.devices_other);
    });

    test('labelForPlatform returns human labels', () {
      expect(labelForPlatform('linux'), 'Linux');
      expect(labelForPlatform('darwin'), 'macOS');
      expect(labelForPlatform('windows'), 'Windows');
      expect(labelForPlatform('android'), 'Android');
      expect(labelForPlatform('ios'), 'iOS');
      expect(labelForPlatform(''), 'Unknown');
      expect(labelForPlatform('haiku'), 'haiku');
    });
  });
}
