"""Build-only native TiKV usage controls; production gains no endpoints.

Usage: prepare-tikv-usage-probe.py REPOSITORY OUTPUT
The Docker compiler uses OUTPUT/overlay.json mounted at /probe. Host compile
checks can use OUTPUT/overlay.local.json. Both overlays consume the same source.
"""
import difflib
import hashlib
import json
import pathlib
import subprocess
import sys


def replace_once(text, before, after, name):
    if text.count(before) != 1:
        raise SystemExit(f"{name}: wiring changed; review the isolated usage probe")
    return text.replace(before, after)


def prepare(root, output):
    root, output = root.resolve(), output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    replacements = {}
    sources = {}
    patches = []

    path = "pkgs/store/storage/tikv.go"
    original = text = (root / path).read_text()
    text = replace_once(text, '\t"maps"\n', '\t"maps"\n\t"os"\n', path)
    text = replace_once(
        text, '\t"github.com/tikv/client-go/v2/config"',
        '\t"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"\n'
        '\t"github.com/tikv/client-go/v2/config"', path,
    )
    text = replace_once(text, '\tstore, err := tikv.NewKVStore', '''\tvar transport tikv.Client = rpc
    if address := os.Getenv("GIZCLAW_E2E_TIKV_USAGE_ADDRESS"); address != "" {
        if os.Getenv("GIZCLAW_E2E_TIKV_RPC_GATE_ADDRESS") != "" {
            _ = rpc.Close()
            _ = safePoint.Close()
            client.Close()
            return nil, fmt.Errorf("usage and contention native probes are separate fixtures")
        }
        transport, err = tikvfixture.StartUsageGate(rpc, address)
        if err != nil {
            _ = rpc.Close()
            _ = safePoint.Close()
            client.Close()
            return nil, tiKVOpenError(name, "start isolated usage probe", err)
        }
    }
\tstore, err := tikv.NewKVStore''', path)
    text = replace_once(text, 'codec, safePoint, rpc,', 'codec, safePoint, transport,', path)
    text = replace_once(text, 'if err != nil {\n\t\t_ = rpc.Close()', 'if err != nil {\n\t\t_ = transport.Close()', path)
    text = replace_once(text, '\treturn store, nil', '''\tif gate, ok := transport.(*tikvfixture.UsageGate); ok {
        store.SetOracle(gate.WrapReadClock(store.GetOracle(), cfg.StaleReadDelay))
    }
\treturn store, nil''', path)
    replacements[path] = "storage-tikv.go"
    sources[path] = original
    formatted = subprocess.run(["gofmt"], input=text, text=True, capture_output=True, check=True).stdout
    (output / replacements[path]).write_text(formatted)
    patches.extend(difflib.unified_diff(original.splitlines(True), formatted.splitlines(True), fromfile=f"a/{path}", tofile=f"b/{path}"))

    path = "pkgs/gizclaw/server.go"
    original = text = (root / path).read_text()
    if '\t"os"\n' not in text:
        text = replace_once(text, '\t"net/http"\n', '\t"net/http"\n\t"os"\n', path)
    text = replace_once(
        text, '\t"github.com/GizClaw/gizclaw-go/pkgs/store/kv"',
        '\t"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"\n'
        '\t"github.com/GizClaw/gizclaw-go/pkgs/store/kv"', path,
    )
    for dependency in ("api/adminhttp", "internal/socialutil", "services/system/ownership"):
        import_line = f'\t"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/{dependency}"'
        if import_line not in text:
            text = replace_once(text, '\t"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"', import_line + '\n\t"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"', path)
    text = replace_once(text, '\ts.apiKeys = apiKeyServer\n', '''\ts.apiKeys = apiKeyServer
    if os.Getenv("GIZCLAW_E2E_TIKV_USAGE_ADDRESS") != "" {
        if err := tikvfixture.AttachUsageServices(&tikvfixture.UsageServices{
            Stores: map[string]kv.Store{"peer": s.PeerStore, "friend": s.FriendStore, "group": s.FriendGroupStore, "apikey": s.APIKeyStore},
            Workspaces: s.WorkspaceDB.DB, AdminEndpoint: s.PublicEndpoint, Peers: peersServer, Routes: manager.PeerRoutes,
            Friends: friendServer, Groups: friendGroupServer, Contacts: contactServer, Presence: manager.Presence,
            Materialize: func(ctx context.Context, operation, name, member string) (any, error) {
                var binding socialutil.SFUWorkspaceBinding
                var err error
                if operation == "group.materialize" {
                    binding, err = friendGroupServer.ResolveSFUWorkspaceBindingByName(ctx, name, member)
                } else {
                    binding, err = friendServer.ResolveSFUWorkspaceBindingByName(ctx, name, member)
                }
                if err != nil { return nil, err }
                item, _, err := workspaceServer.CreateSystemWorkspace(ownership.WithOwner(ctx, binding.Owner), adminhttp.WorkspaceUpsert{
                    Id: binding.WorkspaceID, Name: binding.WorkspaceName, WorkflowId: socialutil.SFUWorkflowID,
                })
                return item, err
            },
        }); err != nil { return err }
    }
''', path)
    replacements[path] = "gizclaw-server.go"
    sources[path] = original
    formatted = subprocess.run(["gofmt"], input=text, text=True, capture_output=True, check=True).stdout
    (output / replacements[path]).write_text(formatted)
    patches.extend(difflib.unified_diff(original.splitlines(True), formatted.splitlines(True), fromfile=f"a/{path}", tofile=f"b/{path}"))

    path = "pkgs/gizclaw/server_social_recovery.go"
    original = text = (root / path).read_text()
    text = replace_once(text, '\t"context"\n', '\t"context"\n\t"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"\n', path)
    text = replace_once(text, 'task.reconcile(ctx)', 'tikvfixture.RunUsageAutomaticRecovery(ctx, task.reconcile)', path)
    replacements[path] = "gizclaw-social-recovery.go"
    sources[path] = original
    formatted = subprocess.run(["gofmt"], input=text, text=True, capture_output=True, check=True).stdout
    (output / replacements[path]).write_text(formatted)
    patches.extend(difflib.unified_diff(original.splitlines(True), formatted.splitlines(True), fromfile=f"a/{path}", tofile=f"b/{path}"))

    (output / "overlay.json").write_text(json.dumps({"Replace": {f"/src/{path}": f"/probe/{name}" for path, name in replacements.items()}}, indent=2) + "\n")
    (output / "overlay.local.json").write_text(json.dumps({"Replace": {str(root / path): str(output / name) for path, name in replacements.items()}}, indent=2) + "\n")
    consumed = sorted((root / "tests/testsupport/tikvfixture").glob("usage_*.go"))
    consumed += sorted((root / "tests/gizclaw-e2e/giztest").glob("tikv.usage*.giztest.yaml"))
    manifest = {
        "source_sha256": {path: hashlib.sha256(value.encode()).hexdigest() for path, value in sources.items()},
        "probe_sha256": {str(path.relative_to(root)): hashlib.sha256(path.read_bytes()).hexdigest() for path in consumed},
        "scope": "private Docker usage fixture; original TiKV requests, responses and product services",
        "control_address_env": "GIZCLAW_E2E_TIKV_USAGE_ADDRESS",
    }
    (output / "tikv-usage-source.json").write_text(json.dumps(manifest, indent=2) + "\n")
    (output / "injection.patch").write_text("".join(patches))


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit("usage: prepare-tikv-usage-probe.py REPOSITORY OUTPUT")
    prepare(pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]))
