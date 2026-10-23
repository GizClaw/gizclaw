"""Install a forwarding RPC observer only in the local E2E Server binary."""
import difflib
import hashlib
import json
import pathlib
import subprocess
import sys

source, output = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
base = sys.argv[3] if len(sys.argv) > 3 else ""
root = source.parents[3]
base_commit = subprocess.check_output(["git", "-C", str(root), "rev-parse", "--verify", "--end-of-options", f"{base}^{{commit}}"], text=True).strip() if base else None
text = source.read_text()
anchors = [
    '\t"maps"\n',
    '\t"github.com/tikv/client-go/v2/config"',
    '\tstore, err := tikv.NewKVStore',
    'codec, safePoint, rpc,',
    '\t\t_ = rpc.Close()',
]
if any(text.count(anchor) != 1 for anchor in anchors):
    raise SystemExit("TiKV client wiring changed; review the E2E observer")
injected = text.replace(anchors[0], anchors[0] + '\t"os"\n')
injected = injected.replace(anchors[1], '\t"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"\n' + anchors[1])
injected = injected.replace(anchors[2], '''\tvar transport tikv.Client = rpc
    if address := os.Getenv("GIZCLAW_E2E_TIKV_RPC_GATE_ADDRESS"); address != "" {
        transport, err = tikvfixture.StartClientGate(rpc, address)
        if err != nil {
            _ = rpc.Close()
            _ = safePoint.Close()
            client.Close()
            return nil, tiKVOpenError(name, "start E2E RPC gate", err)
        }
    }
''' + anchors[2])
injected = injected.replace(anchors[3], 'codec, safePoint, transport,')
# The Store owns the wrapper after successful construction.
injected = injected.replace('if err != nil {\n\t\t_ = rpc.Close()', 'if err != nil {\n\t\t_ = transport.Close()')
output.mkdir(parents=True, exist_ok=True)
(output / "tikv.go").write_text(injected)
replacements = {"/src/pkgs/store/storage/tikv.go": "/probe/tikv.go"}
sources = {"pkgs/store/storage/tikv.go": text}
for name in ("tikv.go", "sets_tikv.go"):
    path = f"pkgs/store/kv/{name}"
    value = subprocess.check_output(["git", "-C", str(root), "show", f"{base_commit}:{path}"], text=True) if base_commit else (root / path).read_text()
    sources[path] = value
    if base:
        replacement = f"kv-{name}"
        (output / replacement).write_text(value)
        replacements[f"/src/{path}"] = f"/probe/{replacement}"
(output / "overlay.json").write_text(json.dumps({"Replace": replacements}) + "\n")
(output / "tikv-source.json").write_text(json.dumps({
    "kv_base_commit": base_commit,
    "source_sha256": {name: hashlib.sha256(value.encode()).hexdigest() for name, value in sources.items()},
    "observer_sha256": hashlib.sha256((root / "tests/testsupport/tikvfixture/client_gate.go").read_bytes()).hexdigest(),
    "test_sha256": hashlib.sha256((root / "tests/gizclaw-e2e/go/multiserver/tikv_contention_test.go").read_bytes()).hexdigest(),
}, indent=2) + "\n")
(output / "injection.patch").write_text("".join(difflib.unified_diff(
    text.splitlines(True), injected.splitlines(True),
    fromfile="a/pkgs/store/storage/tikv.go", tofile="b/pkgs/store/storage/tikv.go")))
