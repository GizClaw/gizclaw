"""Select local providers while retaining the real Doubao ASR adapter and SDK."""
import json
import pathlib
import sys

root, output = map(pathlib.Path, sys.argv[1:3])
package = root / "pkgs/gizclaw/services/ai/peergenx"
source = package / "service.go"
text = source.read_text()
assert text.count("DefaultBuilder{}") == 2
replacement = output / "service.go"
replacement.write_text(text.replace("DefaultBuilder{}", "latencyFixtureBuilder{}"))
provider = output / "latency_fixture.go"
text = (root / "tests/gizclaw-e2e/testdata/slow-tts/provider.go").read_text()
needle = 'return latencyASR{realtime: cfg.Params["emit_interim"] == true || cfg.Params["emit_interim"] == "true"}, nil'
assert text.count(needle) == 1
provider.write_text(text.replace(needle, "return newASRRecoveryFixture(cfg)"))
mapping = {"Replace": {
    str(source): str(replacement),
    str(package / "latency_fixture.go"): str(provider),
    str(package / "asr_recovery_fixture.go"): str(root / "tests/gizclaw-e2e/testdata/asr-recovery/provider.go"),
}}
if "test" in sys.argv[3:]:
    mapping["Replace"][str(package / "asr_recovery_fixture_test.go")] = str(root / "tests/gizclaw-e2e/testdata/asr-recovery/provider_test.go")
if "docker" in sys.argv[3:]:
    mapping = {"Replace": {k.replace(str(root), "/src"): v.replace(str(output), "/out").replace(str(root), "/src") for k, v in mapping["Replace"].items()}}
(output / "overlay.json").write_text(json.dumps(mapping))
