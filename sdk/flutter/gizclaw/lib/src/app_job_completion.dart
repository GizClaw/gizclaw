/// Terminal outcome of one local App job. The RPC contract returns only its
/// job ID; applications can observe completion and failures through this event.
class GizClawAppJobCompletion {
  const GizClawAppJobCompletion({
    required this.jobId,
    this.resultJson,
    this.error,
  });
  final int jobId;
  final String? resultJson;
  final String? error;
}
