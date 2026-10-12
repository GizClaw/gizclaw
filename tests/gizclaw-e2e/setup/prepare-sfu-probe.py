"""Build-only overlay delaying a real revocation EOS after Runtime Error."""
import difflib
import json
import pathlib
import sys

source, output, milliseconds = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]), int(sys.argv[3])
if not 0 < milliseconds <= 10000:
    raise SystemExit("SFU terminal delay must be 1..10000 milliseconds")
text = source.read_text()
anchor = "func (h *PeerConn) broadcastAgentOutputError(_ context.Context, _ string, err error) {\n"
if text.count(anchor) != 1:
    raise SystemExit("SFU terminal callback changed; review the probe before running")
injected = text.replace(anchor, anchor + f'''
    // Local test instrumentation; delays the original EOS without changing it.
    if err != nil && strings.Contains(err.Error(), "sfu: workspace access revoked") {{
        slog.Info("test: delaying real SFU revocation terminal", "delay_ms", {milliseconds}, "peer_public_key", h.Conn.PublicKey().String())
        time.Sleep({milliseconds} * time.Millisecond)
    }}
''')
output.mkdir(parents=True, exist_ok=True)
(output / "peer_conn.go").write_text(injected)
(output / "overlay.json").write_text(json.dumps({"Replace": {
    "/src/pkgs/gizclaw/peer_conn.go": "/probe/peer_conn.go"
}}))
(output / "injection.patch").write_text("".join(difflib.unified_diff(
    text.splitlines(True), injected.splitlines(True),
    fromfile="a/pkgs/gizclaw/peer_conn.go", tofile="b/pkgs/gizclaw/peer_conn.go")))
