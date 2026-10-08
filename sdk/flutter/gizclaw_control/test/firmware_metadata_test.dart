import 'package:gizclaw_control/gizclaw_control.dart';
import 'package:test/test.dart';

void main() {
  test('firmware metadata preserves arbitrary JSON and original channels', () {
    final metadata = <String, Object?>{
      'modem': {
        'version': 'vendor-2026.10',
        'urls': [
          'https://firmware.example/ap.bin',
          'https://firmware.example/cp.bin',
        ],
      },
      'text': 'hello',
      'number': 42,
      'boolean': true,
      'array': [1, 'x', null],
      'null': null,
    };
    final json = <String, Object?>{
      'slots': {
        'stable': <String, Object?>{},
        'beta': <String, Object?>{},
        'develop': <String, Object?>{},
      },
      'metadata': metadata,
    };
    final firmware = DeviceFirmware.fromJson(json);
    expect(firmware.metadata, metadata);
    expect(firmware.toJson(), json);
    expect(firmware.metadata!.containsKey('null'), isTrue);
    json.remove('metadata');
    expect(DeviceFirmware.fromJson(json).metadata, isNull);
    expect(DeviceFirmware.fromJson(json).toJson(), json);
  });
}
