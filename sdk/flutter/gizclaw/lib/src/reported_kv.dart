import 'dart:convert';
import 'generated/rpc/payload/system.pb.dart';

/// The wire keeps bounded JSON text; callers can inspect the exact encoding
/// through jsonValue or read the structured value through this extension.
/// Check the reportedKv map for key presence before reading a nullable value.
extension PeerKVEntryJSON on PeerKVEntry {
  Object? get value => jsonDecode(jsonValue);
}
