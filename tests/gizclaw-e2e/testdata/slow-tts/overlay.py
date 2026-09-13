"""Select only the provider builder in a disposable Go build overlay."""
import json
import pathlib
import sys
root, output = map(pathlib.Path, sys.argv[1:3])
package = root / 'pkgs/gizclaw/services/ai/peergenx'
source = package / 'service.go'
text = source.read_text()
assert text.count('DefaultBuilder{}') == 2
replacement = output / 'service.go'
replacement.write_text(text.replace('DefaultBuilder{}', 'latencyFixtureBuilder{}'))
mapping = {'Replace': {
    str(source): str(replacement),
    str(package / 'latency_fixture.go'): str(root / 'tests/gizclaw-e2e/testdata/slow-tts/provider.go'),
}}
if len(sys.argv) > 3:
    mapping = {"Replace": {k.replace(str(root), "/src"): v.replace(str(output), "/out").replace(str(root), "/src") for k, v in mapping["Replace"].items()}}
(output / "overlay.json").write_text(json.dumps(mapping))
