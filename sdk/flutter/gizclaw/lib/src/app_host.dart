import 'app_job_completion.dart';
import 'generated/rpc/payload.pb.dart' as payload;

/// Injected device App runtime used by the client.app.* RPC handlers.
/// The caller owns the host and must close it after stopping RPC handling.
abstract class GizClawAppHost {
  Stream<GizClawAppJobCompletion> get jobCompletions;
  Future<payload.ClientAppListResponse> list();
  Future<void> install({
    required String appName,
    required String url,
    required String sha256,
    required int size,
  });
  Future<void> uninstall(String appName);
  Future<String> invoke(String appName, String method, String argsJson);
  Future<int> startJob(String appName, String method, String argsJson);
  Future<void> cancelJob(int jobId);
  Future<void> close();
}
