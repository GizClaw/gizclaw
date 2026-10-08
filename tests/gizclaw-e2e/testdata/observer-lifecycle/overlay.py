"""Replace test providers and profiling cadence without changing stream handling."""
import json
import pathlib
import sys

root, output = map(pathlib.Path, sys.argv[1:3])
docker = len(sys.argv) > 3
fixture = root / "tests/gizclaw-e2e/testdata/observer-lifecycle"
overlay_path = output / "overlay.json"
mapping = json.loads(overlay_path.read_text())


def replace(source, replacement):
    source, replacement = str(source), str(replacement)
    if docker:
        source = source.replace(str(root), "/src")
        replacement = replacement.replace(str(output), "/out").replace(str(root), "/src")
    mapping["Replace"][source] = replacement


# The existing slow-TTS fixture retains its cancellation-aware ASR and TTS.
provider = output / "provider.go"
text = (root / "tests/gizclaw-e2e/testdata/slow-tts/provider.go").read_text()
old = 'return nil, errors.New("slow-tts fixture uses deterministic workflow script nodes")'
assert text.count(old) == 1
provider.write_text(text.replace(old, "return lifecycleGenerator{}, nil"))
package = root / "pkgs/gizclaw/services/ai/peergenx"
replace(package / "latency_fixture.go", provider)
replace(package / "observer_fixture.go", fixture / "provider.go")

# Keep the native profiler's capture/manifest implementation; only its cadence
# changes so bounded local tests include both task-end and delayed snapshots.
source = root / "cmd/internal/server/profiling.go"
text = source.read_text()
old = "profilingInterval      = 5 * time.Minute"
assert text.count(old) == 1
replacement = output / "profiling.go"
replacement.write_text(text.replace(old, "profilingInterval      = 2 * time.Second"))
replace(source, replacement)

# Seed a real Model resource through Admin HTTP. All provider credentials are
# local fixture values, and the Docker network has no external connectivity.
source = root / "tests/gizclaw-e2e/cmd/multiserver-seed/slow_tts.go"
text = source.read_text()
old = "spec := runtimeProfileSpec(true)"
assert text.count(old) == 1
text = text.replace(old, '''var llmData apitypes.ModelProviderData
    if err := llmData.FromVolcTenantModelProviderData(apitypes.VolcTenantModelProviderData{
        ApiMode: apitypes.VolcTenantModelProviderDataApiModeChatCompletions,
        UpstreamModel: new("fixture"),
    }); err != nil { return err }
    if err := upsertModel(ctx, api, adminhttp.ModelUpsert{
        Id: "lifecycle-llm", Kind: apitypes.ModelKindLlm,
        Source: apitypes.ModelSourceManual,
        Provider: apitypes.ModelProvider{Kind: apitypes.ModelProviderKindVolcTenant, Id: tenantID},
        ProviderData: llmData,
    }); err != nil { return err }
    spec := runtimeProfileSpec(true)
    (*spec.Resources.Models)["llm"] = binding("lifecycle-llm", "Fixture", "Fixture")''')
replacement = output / "slow_tts.go"
replacement.write_text(text)
replace(source, replacement)

# Evidence uses full public identities so a successful Peer delete cannot erase
# the join between a Giztest task, Workspace name and persisted scope markers.
source = root / "cmd/internal/commands/giztest/clients.go"
text = source.read_text()
old = "client.KeyPair.Public.ShortString()"
assert text.count(old) == 1
replacement = output / "clients.go"
replacement.write_text(text.replace(old, "client.KeyPair.Public.String()"))
replace(source, replacement)
overlay_path.write_text(json.dumps(mapping))
