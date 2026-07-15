import 'dart:io';

import 'package:device_info_plus/device_info_plus.dart';
import 'package:gizclaw/gizclaw.dart';

Future<DeviceInfo> loadMobileDeviceInfo({DeviceInfoPlugin? plugin}) async {
  final deviceInfo = plugin ?? DeviceInfoPlugin();
  try {
    if (Platform.isIOS) {
      final info = await deviceInfo.iosInfo;
      return DeviceInfo(
        name: _nonEmpty(info.name, fallback: info.modelName),
        hardware: HardwareInfo(
          hardwareRevision: info.utsname.machine,
          manufacturer: 'Apple',
          model: info.modelName,
          labels: [
            PeerLabel(key: 'platform', value: 'ios'),
            PeerLabel(key: 'os_version', value: info.systemVersion),
            PeerLabel(
              key: 'physical_device',
              value: info.isPhysicalDevice.toString(),
            ),
          ],
        ),
      );
    }
    if (Platform.isAndroid) {
      final info = await deviceInfo.androidInfo;
      return DeviceInfo(
        name: _nonEmpty(info.name, fallback: info.model),
        hardware: HardwareInfo(
          hardwareRevision: info.hardware,
          manufacturer: info.manufacturer,
          model: info.model,
          labels: [
            PeerLabel(key: 'platform', value: 'android'),
            PeerLabel(key: 'os_version', value: info.version.release),
            PeerLabel(
              key: 'physical_device',
              value: info.isPhysicalDevice.toString(),
            ),
          ],
        ),
      );
    }
  } catch (_) {
    // Keep identity initialization available even when the platform plugin
    // cannot provide detailed hardware information.
  }
  return fallbackMobileDeviceInfo();
}

DeviceInfo fallbackMobileDeviceInfo() {
  final platform = Platform.operatingSystem;
  return DeviceInfo(
    name: 'GizClaw App',
    hardware: HardwareInfo(
      labels: [
        PeerLabel(key: 'platform', value: platform),
        PeerLabel(key: 'os_version', value: Platform.operatingSystemVersion),
      ],
    ),
  );
}

String _nonEmpty(String value, {required String fallback}) {
  final trimmed = value.trim();
  return trimmed.isEmpty ? fallback : trimmed;
}
