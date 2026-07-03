# Giznet E2E Tests

These tests exercise the public giznet transport surface through gizwebrtc.

Run them explicitly with:

```sh
go test -tags giznet_e2e ./tests/giznet-e2e/...
```

Run the WebRTC HTTP benchmark smoke with:

```sh
go test -tags giznet_e2e ./tests/giznet-e2e/webrtc -run '^$' -bench BenchmarkWebRTCHTTPRoundTrip -benchtime=1x
```
