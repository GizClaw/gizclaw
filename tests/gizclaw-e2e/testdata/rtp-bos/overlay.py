"""Select deterministic providers and delay receiver BOS in a disposable build."""
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
    str(package / 'latency_fixture.go'): str(root / 'tests/gizclaw-e2e/testdata/rtp-bos/provider.go'),
}}
# Hold only incoming audio BOS processing while the real RTP reader continues.
# This emulates independent data-channel latency without changing wire data.
receiver = root / 'sdk/go/gizcli/peer_stream.go'
receiver_text = receiver.read_text()
if '\"time\"' not in receiver_text:
    receiver_text = receiver_text.replace('\"sync\"', '\"sync\"\n \"time\"')
needle = '\t\tchunk, err := peerStreamEventToChunk(event)'
assert receiver_text.count(needle) == 1
receiver_text = receiver_text.replace(needle, '''
        if event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_BOS && event.StreamKindValue() == eventpb.StreamKind_STREAM_KIND_AUDIO {
            time.Sleep(200 * time.Millisecond)
        }
''' + needle)
receiver_output = output / 'peer_stream.go'
receiver_output.write_text(receiver_text)
mapping['Replace'][str(receiver)] = str(receiver_output)
if len(sys.argv) > 3:
    mapping = {"Replace": {k.replace(str(root), "/src"): v.replace(str(output), "/out").replace(str(root), "/src") for k, v in mapping["Replace"].items()}}
(output / "overlay.json").write_text(json.dumps(mapping))
