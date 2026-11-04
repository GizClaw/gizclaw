"""Build-only observers and deterministic providers; production wire is unchanged."""
import json
import os
import pathlib
import shutil
import subprocess
import sys

root, output = map(pathlib.Path, sys.argv[1:3])
output.mkdir(parents=True, exist_ok=True)
imports = '"github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/netemtrace"'
mapping = {}


def overlay(relative, changes):
    source = root / relative
    text = source.read_text()
    text = text.replace('import (', 'import (\n\t' + imports, 1)
    for old, new in changes:
        if text.count(old) != 1:
            raise RuntimeError(f"{relative}: expected exactly one {old!r}")
        text = text.replace(old, new)
    target = output / relative.replace('/', '_')
    target.write_text(text)
    mapping[str(source)] = str(target)


# Reuse the existing Eino+ASR+valid-Opus fixture. No receiver timing overlay.
package = root / 'pkgs/gizclaw/services/ai/peergenx'
source = package / 'service.go'
text = source.read_text()
if text.count('DefaultBuilder{}') != 2:
    raise RuntimeError('provider Builder wiring changed')
replacement = output / 'service.go'
replacement.write_text(text.replace('DefaultBuilder{}', 'latencyFixtureBuilder{}'))
mapping[str(source)] = str(replacement)
provider = (root / 'tests/gizclaw-e2e/testdata/slow-tts/provider.go').read_text()
provider = provider.replace('import (', 'import (\n\t' + imports, 1)
# Keep the fixture startup contract; vary its actual encoded tone so source
# packets are distinguishable without inventing an RTP sequence on the wire.
provider = provider.replace('t.frequency*float64(i*320+j)', '(t.frequency+float64(i)*3)*float64(i*320+j)')
provider = provider.replace('seen = true', 'seen = true\n\t\t\t\t\tnetemtrace.Record("asr_input", b.Data, b.MIMEType, -1, -1)')
provider = provider.replace('if c.IsBeginOfStream() {', 'if c.IsBeginOfStream() {\n\t\t\t\tnetemtrace.Record("asr_bos", nil, c.Ctrl.StreamID, -1, -1)')
provider = provider.replace('if seen && !sent &&', 'if c.IsEndOfStream() { netemtrace.Record("asr_eos", nil, id, -1, -1) }\n\t\t\tif seen && !sent &&')
provider = provider.replace('packet, err := enc.Encode(pcm, len(pcm))', 'packet, err := enc.Encode(pcm, len(pcm))\n\t\t\t\tnetemtrace.Record("tts_output", packet, id, int64(i), -1)')
if os.environ.get('GIZCLAW_NETEM_CONTINUITY') == '1':
    old = 'for i := 0; i < 80; i++ {\n\t\t\tif err := latencyWait(ctx, 20*time.Millisecond); err != nil {'
    new = 'deadline := time.Now()\n\t\tfor i := 0; i < 800; i++ {\n\t\t\tif err := latencyWait(ctx, time.Until(deadline.Add(time.Duration(i+1)*20*time.Millisecond))); err != nil {'
    if provider.count(old) != 1:
        raise RuntimeError('continuous fixture pacing seam changed')
    provider = provider.replace(old, new)
provider_file = output / 'provider.go'
provider_file.write_text(provider)
mapping[str(package / 'netem_fixture.go')] = str(provider_file)

overlay('pkgs/gizclaw/peer_conn.go', [
    ('case giznet.ProtocolOpusPacket:\n\t\t\tchunk, ok := opusPacketChunk(buf[:n])',
     'case giznet.ProtocolOpusPacket:\n\t\t\tnetemtrace.Record("server_input", buf[:n], "", -1, -1)\n\t\t\tchunk, ok := opusPacketChunk(buf[:n])'),
    ('\t\t\tif !authorized {\n\t\t\t\tcontinue\n\t\t\t}',
     '\t\t\tif !authorized {\n\t\t\t\tnetemtrace.Record("server_input_rejected", buf[:n], "", -1, -1)\n\t\t\t\tcontinue\n\t\t\t}'),
    ('_, err := h.Conn.Write(giznet.ProtocolOpusPacket, packet)',
     '_, err := h.Conn.Write(giznet.ProtocolOpusPacket, packet)\n\tif err == nil { netemtrace.Record("server_output", packet, "", -1, -1) }'),
])
overlay('sdk/go/gizcli/peer_stream.go', [
    ('func (s *PeerStream) pushMergedPacket(payload []byte) error {',
     'func (s *PeerStream) pushMergedPacket(payload []byte) error {\n\tnetemtrace.Record("client_raw", payload, "", -1, -1)'),
    ('s.ignoredPostEOSPackets.Add(1)',
     's.ignoredPostEOSPackets.Add(1)\n\t\tnetemtrace.Record("client_post_eos_drop", payload, "", -1, -1)'),
    ('s.startupAudio = append(s.startupAudio, chunk)',
     'netemtrace.Record("client_pre_bos_buffer", payload, "", -1, -1)\n\t\ts.startupAudio = append(s.startupAudio, chunk)'),
])
for relative in ['pkgs/gizclaw/peer_stream_event.go', 'sdk/go/gizcli/peer_stream_event.go']:
    overlay(relative, [
        ('return event, nil',
         'netemtrace.Record("control_read", nil, event.Type.String()+"|"+event.StreamKindValue().String()+"|"+event.StreamID(), -1, -1)\n\treturn event, nil'),
        ('return rpcapi.WriteFrame(w, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: data})',
         'netemtrace.Record("control_write", nil, event.Type.String()+"|"+event.StreamKindValue().String()+"|"+event.StreamID(), -1, -1)\n\treturn rpcapi.WriteFrame(w, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: data})'),
    ])

overlay('pkgs/giznet/gizwebrtc/conn.go', [
    ('c.enqueueRemoteOpusFrame(pkt.Payload)',
     'netemtrace.Record("rtp_read", pkt.Payload, fmt.Sprint(pkt.SSRC), int64(pkt.SequenceNumber), int64(pkt.Timestamp))\n\t\tc.enqueueRemoteOpusFrame(pkt.Payload)'),
    ('n, err := writePacket(raw, protocol, payload)',
     'n, err := writePacket(raw, protocol, payload)\n\tif err == nil && protocol == giznet.ProtocolTunnelPacket && len(payload) > 17 && payload[0] == 2 { netemtrace.Record("tunnel_wire_send", payload[17:], fmt.Sprintf("%x", payload[1:17]), -1, -1) }'),
    ('\t\tc.enqueuePacket(pkt)',
     '\t\tif pkt.protocol == giznet.ProtocolTunnelPacket && len(pkt.payload) > 17 && pkt.payload[0] == 2 { netemtrace.Record("tunnel_wire_read", pkt.payload[17:], fmt.Sprintf("%x", pkt.payload[1:17]), -1, -1) }\n\t\tc.enqueuePacket(pkt)'),
    ('ticks := codecconv.OpusPacketRTPTicks(payload)',
     'netemtrace.Record("rtp_write_sample", payload, "", -1, -1)\n\tticks := codecconv.OpusPacketRTPTicks(payload)'),
    ('c.markMetricsEstablished()\n\tclose(c.readyCh)',
     '''lifetime, retries := -1, -1
\tif value := dc.MaxPacketLifeTime(); value != nil { lifetime = int(*value) }
\tif value := dc.MaxRetransmits(); value != nil { retries = int(*value) }
\tnetemtrace.Record("packet_policy", nil, fmt.Sprintf("ordered=%t,lifetime_ms=%d,max_retransmits=%d", dc.Ordered(), lifetime, retries), -1, -1)
\tc.markMetricsEstablished()
\tclose(c.readyCh)'''),
])
overlay('pkgs/giznet/giztunnel/conn.go', [
    ('case c.readCh <- packet:\n\t\tc.touch()\n\t\treturn nil',
     'case c.readCh <- packet:\n\t\tc.touch()\n\t\tif protocol == giznet.ProtocolOpusPacket { netemtrace.Record("tunnel_enqueue", packet.payload, "", -1, -1) }\n\t\treturn nil'),
    ('// audio, but it must not lose its control and service channels.\n\t\t\tc.touch()',
     '// audio, but it must not lose its control and service channels.\n\t\t\tnetemtrace.Record("tunnel_drop", packet.payload, "", -1, -1)\n\t\t\tc.touch()'),
])

# Retain the historical buffer-selected candidate as an explicit diagnostic
# control. Normal qualification uses the production policy, without this flag.
if os.environ.get('GIZCLAW_NETEM_ORDERED_TIMED_PACKET_PROBE') == '1':
    old = 'packetOptions, err := packetDataChannelInit(cfg.PacketDataChannelPolicy)'
    new = '''packetOptions, err := packetDataChannelInit(cfg.PacketDataChannelPolicy)
\tif cfg.SCTPReceiveBufferSize == GatewaySCTPReceiveBufferSize {
\t\tordered, lifetime := true, uint16(200)
\t\tpacketOptions = &webrtc.DataChannelInit{Ordered: &ordered, MaxPacketLifeTime: &lifetime}
\t\tnetemtrace.Record("packet_policy_probe", nil, "ordered_lifetime_200ms", -1, -1)
\t}'''
    overlay('pkgs/giznet/gizwebrtc/dial.go', [(old, new)])

# Causal probe only. This is not a complete zero-retransmission policy fix:
# retry/abandonment semantics must be qualified separately before production use.
if os.environ.get('GIZCLAW_NETEM_SCTP_DEFER_INITIAL_PROBE') == '1':
    module = json.loads(subprocess.check_output([
        'go', 'mod', 'download', '-json',
        'github.com/GizClaw/pion-sctp@v0.0.0-20260930221805-07dcddc21e92'], text=True))
    dependency = output / 'sctp-probe'
    shutil.copytree(module['Dir'], dependency)
    source = dependency / 'association.go'
    text = source.read_text()
    needle = 'chunkPayload.nSent = 1\n\n\ta.checkPartialReliabilityStatus(chunkPayload)'
    if text.count(needle) != 1:
        raise RuntimeError('SCTP initial-send causal probe seam changed')
    source.chmod(0o600)
    source.write_text(text.replace(needle, 'chunkPayload.nSent = 1'))
    # A disposable -modfile replacement selects this copy. GOMODCACHE is never
    # modified, and production go.mod/go.sum remain untouched.

# Qualify an actual local SCTP owner repair from an immutable private snapshot.
# The shared module cache and the repository's go.mod/go.sum stay unchanged.
if os.environ.get('GIZCLAW_NETEM_SCTP_SOURCE'):
    if any(os.environ.get(name) == '1' for name in [
            'GIZCLAW_NETEM_SCTP_DEFER_INITIAL_PROBE', 'GIZCLAW_NETEM_ORDERED_TIMED_PACKET_PROBE']):
        raise RuntimeError('SCTP owner qualification cannot use diagnostic probe flags')
    source = pathlib.Path(os.environ['GIZCLAW_NETEM_SCTP_SOURCE']).resolve()
    if (source / 'go.mod').read_text().splitlines()[0] != 'module github.com/pion/sctp':
        raise RuntimeError('SCTP qualification source is not github.com/pion/sctp')
    target = output / 'sctp-source'
    names = subprocess.check_output([
        'git', '-C', str(source), 'ls-files', '--cached', '--others', '--exclude-standard', '-z']).split(b'\0')
    for name in sorted(set(names) - {b''}):
        relative = pathlib.Path(os.fsdecode(name))
        original = source / relative
        if not original.is_file() or original.is_symlink():
            raise RuntimeError(f'SCTP qualification requires regular source files: {relative}')
        copied = target / relative
        copied.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(original, copied)
    import hashlib
    hashes = {str(p.relative_to(target)): hashlib.sha256(p.read_bytes()).hexdigest()
              for p in sorted(target.rglob('*')) if p.is_file()}
    identity = {
        'base_commit': subprocess.check_output(['git', '-C', str(source), 'rev-parse', 'HEAD'], text=True).strip(),
        'branch': subprocess.check_output(['git', '-C', str(source), 'branch', '--show-current'], text=True).strip(),
        'source_sha256': hashes,
        'snapshot_sha256': hashlib.sha256(json.dumps(hashes, sort_keys=True).encode()).hexdigest(),
    }
    (output / 'sctp-source-identity.json').write_text(json.dumps(identity, indent=2) + '\n')

# Paths in the overlay are interpreted inside the compiler container.
container_mapping = {k.replace(str(root), '/src'): v.replace(str(output), '/out') for k, v in mapping.items()}
(output / 'overlay.json').write_text(json.dumps({'Replace': container_mapping}, indent=2) + '\n')
