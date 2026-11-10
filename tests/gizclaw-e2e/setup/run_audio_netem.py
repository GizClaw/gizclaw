"""Run the bounded local-only matrix and retain qdisc and per-frame evidence."""
import base64
import collections
import hashlib
import json
import os
import pathlib
import shlex
import subprocess
import sys
import time

run_dir = pathlib.Path(sys.argv[1]).resolve()
root = pathlib.Path(os.environ['NETEM_ROOT'])
reports = run_dir / 'reports'
project = os.environ['NETEM_PROJECT']
if not project.startswith('gizclaw-audio-netem-'):
    raise RuntimeError('refusing a non-netem Docker project')
compose = ['docker', 'compose', '--env-file', '/dev/null', '-p', project, '-f',
           str(root / 'tests/gizclaw-e2e/docker/compose.audio-netem.yaml')]
commands = []


def command(args, check=True, timeout=90):
    commands.append(shlex.join(args))
    completed = subprocess.run(args, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=timeout)
    if check and completed.returncode:
        raise RuntimeError(f'{shlex.join(args)}\n{completed.stdout}')
    return completed


def execute(service, *args, check=True):
    return command([*compose, 'exec', '-T', service, *args], check=check)


def save(path, value):
    path.write_text(json.dumps(value, indent=2) + '\n')


# Container ownership and topology are checked before any mutation.
identities = {}
for name in ['server', 'edge', 'test', 'server-netem', 'edge-netem', 'log-sink']:
    cid = command([*compose, 'ps', '-q', name]).stdout.strip()
    value = json.loads(command(['docker', 'inspect', cid]).stdout)[0]
    if value['Config']['Labels'].get('com.docker.compose.project') != project:
        raise RuntimeError('container project mismatch')
    if value['HostConfig']['Privileged']:
        raise RuntimeError('privileged container is forbidden')
    value['Config']['Env'] = [entry if entry.startswith('NETEM_NODE=') else entry.split('=', 1)[0] + '=<redacted>'
                              for entry in value['Config'].get('Env', [])]
    identities[name] = value
save(reports / 'containers.json', identities)
interfaces = {}
for node, other in [('edge', 'server'), ('server', 'edge')]:
    uplink = identities[other]['NetworkSettings']['Networks'][project + '_uplink']['IPAddress']
    route = json.loads(execute(node + '-netem', 'ip', '-j', 'route', 'get', uplink).stdout)[0]
    iface = route['dev']
    addr = json.loads(execute(node + '-netem', 'ip', '-j', 'addr', 'show', 'dev', iface).stdout)
    own = identities[node]['NetworkSettings']['Networks'][project + '_uplink']['IPAddress']
    if not any(item.get('local') == own for entry in addr for item in entry.get('addr_info', [])):
        raise RuntimeError('selected interface is not the dedicated uplink')
    interfaces[node] = iface
save(reports / 'interfaces.json', interfaces)
manifest = {
    'head': command(['git', '-C', str(root), 'rev-parse', 'HEAD']).stdout.strip(),
    'dirty': command(['git', '-C', str(root), 'status', '--short']).stdout,
    'kernel': execute('edge-netem', 'uname', '-a').stdout.strip(),
    'tc': execute('edge-netem', 'tc', '-V').stdout.strip(),
    'docker': command(['docker', 'version', '--format', '{{json .}}']).stdout,
    'base': json.loads(command(['docker', 'image', 'inspect', os.environ['NETEM_BASE_IMAGE']]).stdout),
    'tools': json.loads(command(['docker', 'image', 'inspect', os.environ['NETEM_TOOLS_IMAGE']]).stdout),
    'binary_sha256': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in (run_dir / 'bin').iterdir() if p.is_file()},
    'source_sha256': {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest()
                      for folder in ['pkgs/giznet', 'pkgs/gizedge', 'tests/gizclaw-e2e/netemtrace']
                      for p in (root / folder).rglob('*.go')},
    'input': {'mode': 'push-to-talk', 'frames': 160, 'frame_ms': 20, 'sample_rate': 16000,
              'pcm_seed': [7919, 104729], 'tone_hz': 440, 'noise_amplitude': 2000},
    'sctp_qualification': (json.loads((run_dir / 'bin/sctp-source-identity.json').read_text())
                           if (run_dir / 'bin/sctp-source-identity.json').exists() else None),
    'diagnostic_only_ordered_timed_packet_policy': os.environ.get('GIZCLAW_NETEM_ORDERED_TIMED_PACKET_PROBE') == '1',
    'provider': 'slow-tts fixture with per-frame tone variation; ASR emits only after nonempty real input',
    'playback': {'prebuffer_ms': 500, 'frame_ms': 20, 'physical_speaker': False},
}
save(reports / 'manifest.json', manifest)


def reset():
    for node, iface in interfaces.items():
        execute(node + '-netem', 'tc', 'qdisc', 'del', 'dev', iface, 'root', check=False)


def qdisc():
    return {node: execute(node + '-netem', 'tc', '-s', '-d', 'qdisc', 'show', 'dev', iface).stdout
            for node, iface in interfaces.items()}


def traces(text):
    values = []
    for line in text.splitlines():
        if 'NETEM_TRACE ' in line:
            try:
                item = json.loads(line.split('NETEM_TRACE ', 1)[1])
                if (not isinstance(item, dict) or type(item.get('ns')) is not int or item['ns'] <= 0
                        or not all(isinstance(item.get(key), str) for key in ['node', 'stage', 'hash', 'detail'])
                        or not item['node'] or not item['stage']
                        or any(type(item.get(key)) is not int for key in ['bytes', 'sequence', 'timestamp'])
                        or item['bytes'] < 0 or bool(item['hash']) != (item['bytes'] > 0)
                        or (item['hash'] and (len(item['hash']) != 64
                            or any(c not in '0123456789abcdef' for c in item['hash'])))
                        or item.get('error')):
                    raise ValueError('invalid observation schema or observer error')
                if 'pcm_bytes' in item and (type(item['pcm_bytes']) is not int or item['pcm_bytes'] <= 0
                        or item['pcm_bytes'] % 2 or type(item.get('concealed')) is not bool
                        or not isinstance(item.get('pcm_hash'), str) or len(item['pcm_hash']) != 64
                        or item['concealed'] == bool(item['hash'])):
                    raise ValueError('invalid recovered audio provenance')
                values.append(item)
            except (ValueError, TypeError) as error:
                # Ordinary log noise has no marker. A malformed marked record
                # means packet evidence may be truncated and must fail closed.
                raise RuntimeError('malformed NETEM_TRACE observation') from error
    return values


def compare(source, received, identity='hash'):
    indices = collections.defaultdict(list)
    for i, item in enumerate(source):
        indices[item[identity]].append(i)
    ambiguous = {key: ids for key, ids in indices.items() if len(ids) > 1}
    seen = collections.Counter()
    order = []
    unknown = 0
    for item in received:
        ids = indices.get(item[identity], [])
        count = seen[item[identity]]
        seen[item[identity]] += 1
        if not ids:
            unknown += 1
        else:
            order.append(ids[min(count, len(ids) - 1)])
    missing = [i for key, ids in indices.items() for i in ids[min(seen[key], len(ids)):]]
    duplicate = sum(max(0, count - len(indices.get(key, []))) for key, count in seen.items() if key in indices)
    intervals = [(b['ns'] - a['ns']) / 1e6 for a, b in zip(received, received[1:])]
    # A late ordinal is one below a previously received maximum; count no
    # ambiguous hashes as proven reorder. This is payload order, not UDP order.
    maximum = -1
    reordered = []
    for i in order:
        if i < maximum:
            reordered.append(i)
        maximum = max(maximum, i)
    return {'sent': len(source), 'received': len(received), 'missing_indices': sorted(missing),
            'duplicate': duplicate, 'unknown': unknown, 'ambiguous_source_hashes': len(ambiguous),
            'received_order': order, 'reordered_indices': reordered if not ambiguous else [],
            'max_gap_ms': max(intervals, default=0), 'intervals_ms': intervals,
            'content_bytes': sum(item['bytes'] for item in received)}


def select_trace(observed, node, stage):
    return sorted((item for item in observed if item['node'] == node and item['stage'] == stage),
                  key=lambda item: item['ns'])


def recovery_summary(items):
    """Describe actual recovered PCM, originals, PLC and the retained RTP clock."""
    errors = []
    previous = {}
    for item in items:
        # A real Opus packet is at most 120 ms. One synthesized PLC interval
        # may cover several missing packets, bounded by the receiver's 500 ms.
        maximum_duration48 = 24000 if item.get('concealed') else 5760
        if (not item.get('pcm_bytes') or item.get('concealed') == bool(item['hash'])
                or not item.get('source') or not 0 <= item['sequence'] <= 65535
                or not 0 <= item['timestamp'] <= 0xffffffff
                or not 0 < item.get('duration48', 0) <= maximum_duration48
                or item.get('duration48', 0) % 120
                or item['pcm_bytes'] * 3 != item.get('duration48', 0) * 2):
            errors.append('missing PCM/provenance/RTP metadata')
            continue
        source = item['source']
        if source in previous:
            last = previous[source]
            sequence_delta = (item['sequence'] - last['sequence']) % 65536
            if last.get('concealed'):
                # The PLC sequence marks the first missing packet. Variable
                # packet durations prevent inferring an exact packet count.
                # Every skipped packet must fit at least 2.5 ms (120 ticks).
                if not 1 <= sequence_delta <= last['duration48'] // 120:
                    errors.append('invalid recovered RTP sequence after PLC')
            elif sequence_delta != 1:
                errors.append('noncontiguous recovered RTP sequence')
            if (item['timestamp'] - last['timestamp']) % 2**32 != last['duration48']:
                errors.append('noncontiguous recovered RTP timestamp')
        previous[source] = item
    return {'frames': len(items), 'original_packets': sum(bool(x['hash']) for x in items),
            'pcm_bytes': sum(x.get('pcm_bytes', 0) for x in items),
            'concealed_frames': sum(x.get('concealed', False) for x in items),
            'concealed_samples': sum(x.get('pcm_bytes', 0) // 2 for x in items if x.get('concealed')),
            'concealed_ms': sum(x.get('pcm_bytes', 0) / 32 for x in items if x.get('concealed')),
            'clock_errors': sorted(set(errors))}


def endpoint_evidence(observed, expected_up, minimum_down, delivered_down=None):
    """Require intact ordered originals and real PCM at both recovered endpoints.

    Wire vectors must be complete, but arrival order is diagnostic. Recovery
    must restore the exact source order; neither PLC nor a surviving suffix can
    satisfy the healthy/delay/jitter acceptance contract.
    """
    errors, counts, recovery, wire = [], {}, {}, {}

    def select(node, stage):
        return select_trace(observed, node, stage)

    up = select('client', 'rtp_write_sample')
    down = select('server', 'server_output')
    if expected_up <= 0 or len(up) != expected_up:
        errors.append('incomplete independent input packet evidence')
    if minimum_down <= 0 or len(down) < minimum_down:
        errors.append('incomplete reply source evidence')

    def check(node, stage, reference, ordered=False, pcm=False):
        items = select(node, stage)
        counts[f'{node}/{stage}'] = len(items)
        hashes = [x['hash'] for x in items]
        wanted = [x['hash'] for x in reference]
        if not wanted or any(not value for value in hashes) or collections.Counter(hashes) != collections.Counter(wanted):
            errors.append(f'{node}/{stage}: incomplete original payload vector')
        elif ordered and hashes != wanted:
            errors.append(f'{node}/{stage}: recovered original payload order differs')
        if pcm and any(not x.get('pcm_bytes') or x.get('concealed') for x in items):
            errors.append(f'{node}/{stage}: missing real PCM or unexpected PLC')
        return items

    for direction, source, sender, receiver, endpoint, stage in [
            ('up', up, 'edge', 'server', 'server', 'server_input'),
            ('down', down, 'server', 'edge', 'client', 'client_recovered')]:
        sent = check(sender, 'tunnel_wire_send', source)
        received = check(receiver, 'tunnel_wire_read', source)
        if any(x.get('wire_version') != 3 or not x.get('wire_hash') or x['sequence'] < 0 for x in sent + received):
            errors.append(f'{direction}: missing v3 wire RTP evidence')
        wire[direction] = compare(sent, received, 'wire_hash')
        if any(wire[direction][key] for key in ['missing_indices', 'duplicate', 'unknown']):
            errors.append(f'{direction}: wire RTP inventory differs')
        boundary = check(endpoint, stage, source, ordered=True, pcm=True)
        frames = check(endpoint, 'media_recovered', source, ordered=True, pcm=True)
        recovery[direction] = recovery_summary(frames)
        if recovery[direction]['clock_errors']:
            errors.append(f'{direction}: recovered RTP clock is not contiguous')
        if [(x.get('pcm_hash'), x.get('pcm_bytes')) for x in frames] != [
                (x.get('pcm_hash'), x.get('pcm_bytes')) for x in boundary]:
            errors.append(f'{direction}: endpoint PCM differs from receiver output')

    clock_key = lambda x: (x['hash'], x['sequence'], x['timestamp'])
    for direction, original, stages in [
            ('up', up, [('edge', 'rtp_read'), ('edge', 'tunnel_wire_send'), ('server', 'media_recovered')]),
            ('down', select('server', 'tunnel_wire_send'),
             [('edge', 'rtp_write_packet'), ('client', 'rtp_read'), ('client', 'media_recovered')])]:
        for node, stage in stages:
            items = select(node, stage)
            if collections.Counter(map(clock_key, items)) != collections.Counter(map(clock_key, original)):
                errors.append(f'{direction}: {node}/{stage} changed RTP sequence or timestamp')
    check('edge', 'rtp_read', up)
    check('edge', 'rtp_write_packet', down)
    check('client', 'rtp_read', down)
    delivered = select('client', 'client_delivered')
    discarded = select('client', 'client_post_eos_drop')
    counts.update({'input_packets': expected_up, 'physical_reply_packets': len(down),
                   'delivered_reply_packets': len(delivered), 'post_eos_packets': len(discarded)})
    reconciled = sorted(delivered + discarded, key=lambda item: item['ns'])
    if ([x['hash'] for x in reconciled] != [x['hash'] for x in down]
            or any(not x.get('pcm_bytes') or x.get('concealed') for x in reconciled)):
        errors.append('down: delivered audio and explicit post-EOS discards do not reconcile with source')
    if delivered_down is not None and len(delivered) != delivered_down:
        errors.append('down: delivered frame count differs from Giztest playback evidence')
    if not delivered:
        errors.append('down: no delivered PCM')
    return {'passed': not errors, 'errors': errors, 'counts': counts, 'recovery': recovery, 'wire': wire}


def continuity_evidence(observed, rounds, frames_per_reply):
    """Reconcile recovered endpoint vectors with all three Giztest rounds."""
    errors = []
    expected_rounds = {f'continuous_round_{index}' for index in range(1, 4)}
    if len(rounds) != 3 or {item.get('id') for item in rounds} != expected_rounds:
        errors.append('missing or duplicate Giztest rounds')
    expected_up = 0
    delivered_down = 0
    for item in rounds:
        pushed = item.get('pushed_packets')
        inputs = item.get('input_packets')
        packets = (item.get('pacing') or {}).get('packets')
        if item.get('status') != 'passed':
            errors.append(f"{item.get('id')}: Giztest round did not pass")
        if type(pushed) is not int or type(inputs) is not int or pushed <= 0 or pushed != inputs:
            errors.append(f"{item.get('id')}: missing or incomplete input packet count")
        else:
            expected_up += pushed
        if type(packets) is not int or packets < frames_per_reply:
            errors.append(f"{item.get('id')}: missing or incomplete reply packet count")
        else:
            delivered_down += packets
        pacing = item.get('pacing') or {}
        if (not isinstance(pacing.get('audio_ms'), (int, float))
                or pacing['audio_ms'] < frames_per_reply * 20
                or pacing.get('prebuffer_ms') != 500 or pacing.get('underruns') != 0
                or not isinstance(pacing.get('minimum_buffer_ms'), (int, float))
                or pacing['minimum_buffer_ms'] < 0):
            errors.append(f"{item.get('id')}: incomplete sustained playback or underrun evidence")

    def select(node, stage):
        return sorted((item for item in observed if item['node'] == node and item['stage'] == stage),
                      key=lambda item: item['ns'])

    # The provider's fixed frame budget and per-stream ordinals independently
    # prove that no reply's source trace was omitted or reduced to a suffix.
    source_streams = collections.defaultdict(list)
    for item in select('server', 'tts_output'):
        source_streams[item['detail']].append(item['sequence'])
    if (len(source_streams) != 3 or '' in source_streams or
            any(sequence != list(range(frames_per_reply)) for sequence in source_streams.values())):
        errors.append('incomplete provider frame evidence for the three replies')

    endpoint = endpoint_evidence(observed, expected_up, 3 * frames_per_reply, delivered_down)
    errors.extend(endpoint['errors'])
    return {**endpoint, 'passed': not errors, 'errors': errors}


def loss_continuity_document(document):
    """Change only loss-case completeness to recovered PCM duration."""
    packets = '      /audio_pacing/packets: {minimum: 800}\n'
    duration = '      /audio_pacing/audio_ms: {minimum: 16000}'
    if document.count(packets) != 1 or document.count(duration) != 1:
        raise RuntimeError('continuous document original-audio expectations changed')
    return document.replace(packets, '').replace(duration, '      /audio_pacing/playout_ms: {minimum: 16000}')


def loss_recovery_evidence(observed, rounds, frames_per_reply):
    """Prove downlink loss became matching PLC and reached real playback.

    This verdict never changes the strict original-packet verdict. Missing
    source/tail traces, ineffective loss injection and PLC with invented encoded
    bytes fail rather than qualifying a surviving suffix as recovered audio.
    """
    errors, wire, recovery = [], {}, {}
    expected_rounds = {f'continuous_round_{index}' for index in range(1, 4)}
    if len(rounds) != 3 or {item.get('id') for item in rounds} != expected_rounds:
        errors.append('missing or duplicate Giztest rounds')
    expected_up, delivered_originals, playout_ms = 0, 0, 0
    for item in rounds:
        pushed, inputs = item.get('pushed_packets'), item.get('input_packets')
        pacing = item.get('pacing') or {}
        if item.get('status') != 'passed':
            errors.append(f"{item.get('id')}: Giztest round did not pass")
        if type(pushed) is not int or pushed <= 0 or pushed != inputs:
            errors.append('missing or incomplete independent input counts')
        else:
            expected_up += pushed
        if type(pacing.get('packets')) is not int or pacing['packets'] <= 0:
            errors.append('missing original playback packet count')
        else:
            delivered_originals += pacing['packets']
        duration = pacing.get('playout_ms')
        if (not isinstance(duration, (int, float)) or duration < frames_per_reply * 20
                or pacing.get('prebuffer_ms') != 500 or pacing.get('underruns') != 0
                or not isinstance(pacing.get('minimum_buffer_ms'), (int, float))
                or pacing['minimum_buffer_ms'] < 0):
            errors.append('incomplete recovered playback or underrun evidence')
        else:
            playout_ms += duration
    provider = collections.defaultdict(list)
    for item in select_trace(observed, 'server', 'tts_output'):
        provider[item['detail']].append(item['sequence'])
    if (len(provider) != 3 or '' in provider
            or any(ids != list(range(frames_per_reply)) for ids in provider.values())):
        errors.append('incomplete provider frame evidence for the three replies')

    clock_key = lambda x: (x['hash'], x['sequence'], x['timestamp'])
    pcm_key = lambda x: (x['hash'], x.get('pcm_hash'), x.get('pcm_bytes'), x.get('concealed'))
    down_frames = []
    for direction, source, sender, receiver, endpoint, stage in [
            ('up', select_trace(observed, 'client', 'rtp_write_sample'), 'edge', 'server', 'server', 'server_input'),
            ('down', select_trace(observed, 'server', 'server_output'), 'server', 'edge', 'client', 'client_recovered')]:
        sent = select_trace(observed, sender, 'tunnel_wire_send')
        received = select_trace(observed, receiver, 'tunnel_wire_read')
        if (not source or collections.Counter(x['hash'] for x in sent) != collections.Counter(x['hash'] for x in source)
                or any(not x['hash'] for x in source)):
            errors.append(f'{direction}: incomplete independent source/wire evidence')
        if direction == 'up' and (expected_up <= 0 or len(source) != expected_up
                or collections.Counter(map(clock_key, source)) != collections.Counter(map(clock_key, sent))):
            errors.append('up: incomplete input RTP evidence')
        if direction == 'down' and len(source) < 3 * frames_per_reply:
            errors.append('down: incomplete reply source evidence')
        if any(x.get('wire_version') != 3 or not x.get('wire_hash') or x['sequence'] < 0 for x in sent + received):
            errors.append(f'{direction}: missing v3 wire RTP evidence')
        previous = {}
        for item in sent:
            duration = item.get('duration48', 0)
            session, ssrc = item.get('detail'), item.get('ssrc')
            # RecordTunnel parses the wire SessionID and SSRC independently;
            # process-local SourceID is deliberately absent from the envelope.
            if (not isinstance(session, str) or len(session) != 32
                    or any(char not in '0123456789abcdef' for char in session)
                    or type(ssrc) is not int or not 0 <= ssrc < 2**32
                    or type(duration) is not int or not 0 < duration <= 5760 or duration % 120):
                errors.append(f'{direction}: missing source RTP duration or identity')
                continue
            identity = (session, ssrc)
            if identity in previous:
                last = previous[identity]
                if ((item['sequence'] - last['sequence']) % 65536 != 1
                        or (item['timestamp'] - last['timestamp']) % 2**32 != last.get('duration48')):
                    errors.append(f'{direction}: noncontiguous source RTP clock')
            previous[identity] = item
        metric = compare(sent, received, 'wire_hash')
        wire[direction] = metric
        if metric['duplicate'] or metric['unknown'] or metric['ambiguous_source_hashes']:
            errors.append(f'{direction}: duplicate, unknown or ambiguous wire RTP')
        missing = set(metric['missing_indices'])
        if direction == 'up' and missing:
            errors.append('up: unexpected loss outside the downlink injection')
        if direction == 'down' and (not missing or any(i == 0 or i == len(sent) - 1 for i in missing)):
            errors.append('down: no exclusively interior observed media loss')
        # The Edge must forward exactly the surviving RTP packets unchanged.
        if direction == 'down':
            for node, wire_stage in [('edge', 'rtp_write_packet'), ('client', 'rtp_read')]:
                if collections.Counter(map(clock_key, select_trace(observed, node, wire_stage))) != collections.Counter(map(clock_key, received)):
                    errors.append(f'down: {node}/{wire_stage} differs from surviving wire RTP')
        elif collections.Counter(map(clock_key, select_trace(observed, 'edge', 'rtp_read'))) != collections.Counter(map(clock_key, sent)):
            errors.append('up: Edge changed input RTP')
        frames = select_trace(observed, endpoint, 'media_recovered')
        boundary = select_trace(observed, endpoint, stage)
        recovery[direction] = recovery_summary(frames)
        if not frames or recovery[direction]['clock_errors']:
            errors.append(f'{direction}: missing or noncontiguous recovered RTP/PCM')
        if list(map(pcm_key, frames)) != list(map(pcm_key, boundary)):
            errors.append(f'{direction}: recovered PCM did not reach the application unchanged')
        index = 0
        for frame in frames:
            if index >= len(sent):
                errors.append(f'{direction}: excess recovered media')
                break
            expected = sent[index]
            if (frame['sequence'], frame['timestamp']) != (expected['sequence'], expected['timestamp']):
                errors.append(f'{direction}: recovered frame does not start at the expected source RTP clock')
                break
            if frame.get('concealed'):
                duration = frame.get('duration48', 0)
                covered = 0
                while index < len(sent) and covered < duration:
                    if index not in missing or sent[index].get('duration48', 0) <= 0:
                        errors.append(f'{direction}: PLC does not cover observed wire loss')
                        break
                    covered += sent[index]['duration48']
                    index += 1
                if covered != duration or duration <= 0 or frame['hash']:
                    errors.append(f'{direction}: PLC duration or original provenance differs from source loss')
                    break
            else:
                if index in missing or frame['hash'] != expected['hash'] or frame.get('duration48') != expected.get('duration48'):
                    errors.append(f'{direction}: recovered original differs from surviving source packet')
                    break
                index += 1
        if index != len(sent):
            errors.append(f'{direction}: recovered media does not cover the complete source')
        if direction == 'down':
            down_frames = frames

    delivered = select_trace(observed, 'client', 'client_delivered')
    discarded = select_trace(observed, 'client', 'client_post_eos_drop')
    reconciled = sorted(delivered + discarded, key=lambda x: x['ns'])
    if list(map(pcm_key, reconciled)) != list(map(pcm_key, down_frames)):
        errors.append('down: delivered PCM and explicit discards do not reconcile with recovery')
    concealed_samples = sum(x.get('pcm_bytes', 0) // 2 for x in delivered if x.get('concealed'))
    if concealed_samples <= 0:
        errors.append('down: no concealed PCM reached playback')
    if (sum(bool(x['hash']) for x in delivered) != delivered_originals
            or sum(x.get('pcm_bytes', 0) for x in delivered) / 32 != playout_ms):
        errors.append('down: application PCM differs from Giztest playback evidence')
    return {'passed': not errors, 'errors': sorted(set(errors)), 'wire': wire,
            'recovery': recovery, 'delivered_concealed_samples': concealed_samples}


def playback(items):
    result = {'underruns': 0, 'missing_audio': not items, 'stall_ms': 0}
    buffered = 0
    started = None
    # Match peerAudioPacing.playback: start when media, not wall time,
    # reaches the prebuffer. Recovered PCM includes real PLC duration.
    def duration(item):
        return item['pcm_bytes'] / 32 if item.get('pcm_bytes') else 20

    for index, item in enumerate(items):
        buffered += duration(item)
        if buffered >= 500:
            started = index
            break
    if started is None:
        return result
    start = items[started]['ns'] / 1e6
    for item in items[started + 1:]:
        level = buffered - (item['ns'] / 1e6 - start - result['stall_ms'])
        if level < 0:
            result['underruns'] += 1
            result['stall_ms'] -= level
        buffered += duration(item)
    return result


results = []


def run_case(name, params, nodes, seed=7919):
    reset()
    case = reports / name
    case.mkdir()
    for node in nodes:
        execute(node + '-netem', 'tc', 'qdisc', 'replace', 'dev', interfaces[node], 'root',
                'netem', 'limit', '10000', *params, 'seed', str(seed))
    before = qdisc()
    started = time.time_ns()
    (case / 'qdisc-before.txt').write_text(json.dumps(before, indent=2) + '\n')
    completed = execute('test', 'env', 'GIZCLAW_NETEM_REPORT=/reports/' + name + '/client.json',
                        '/binaries/netem.test', '-test.run=^TestAudioNetemRoundTrip$', '-test.v', '-test.timeout=45s', check=False)
    (case / 'client.log').write_text(completed.stdout)
    # Read all service logs once per case and take the exact suffix. This avoids
    # second-granularity timestamp overlap between consecutive cases.
    after_log = command([*compose, 'logs', '--no-color', 'server', 'edge', '--since', str(started // 1000000000)]).stdout
    # Compose may group/interleave service logs differently, so use per-record
    # high-resolution timestamps rather than an unstable text prefix.
    all_observations = traces(after_log) + traces(completed.stdout)
    observations = [item for item in all_observations if item['ns'] >= started]
    (case / 'services.log').write_text(after_log)
    save(case / 'trace.json', observations)
    after = qdisc()
    (case / 'qdisc-after.txt').write_text(json.dumps(after, indent=2) + '\n')
    def select(node, stage):
        return sorted((item for item in observations if item['node'] == node and item['stage'] == stage), key=lambda item: item['ns'])
    uplink = compare(select('client', 'client_sent'), [x for x in select('server', 'server_input') if x['hash']])
    downlink = compare(select('server', 'server_output'), [x for x in select('client', 'client_recovered') if x['hash']])
    delivered = compare(select('server', 'server_output'), [x for x in select('client', 'client_delivered') if x['hash']])
    endpoint = endpoint_evidence(observations, 160, 80)
    required_recovery = (not params or params == ['delay', '100ms'] or
                         params == ['delay', '20ms', '5ms', 'distribution', 'normal'])
    client = json.loads((case / 'client.json').read_text()) if (case / 'client.json').exists() else {}
    result = {'name': name, 'tc_args': params, 'egress_nodes': nodes, 'seed': seed,
              'one_way_delay_scope': 'each listed node egress; bidirectional RTT increment is twice a symmetric delay',
              'test_exit': completed.returncode, 'client': client,
              'uplink': uplink, 'downlink_recovered': downlink, 'downlink_delivered': delivered,
              'endpoint_evidence': endpoint, 'endpoint_recovery_required': required_recovery,
              'playback': playback(select('client', 'client_delivered')),
              'asr_packets': len(select('server', 'asr_input')),
              'tts_packets': len(select('server', 'tts_output')),
              'tunnel_queue_drops': len([o for o in observations if o['stage'] == 'tunnel_drop']),
              'wire_up': endpoint['wire']['up'],
              'wire_down': endpoint['wire']['down'],
              'server_input_rejected': len(select('server', 'server_input_rejected')),
              'control': [o for o in observations if o['stage'].startswith('control_') or o['stage'] in ['client_bos', 'client_eos', 'asr_bos', 'asr_eos']]}
    tail_hashes = {o['hash'] for o in select('client', 'client_post_eos_drop')}
    result['sdk_policy_tail_indices'] = [i for i, o in enumerate(select('server', 'server_output')) if o['hash'] in tail_hashes]
    unexpected_delivery_loss = set(delivered['missing_indices']) - set(result['sdk_policy_tail_indices'])
    result['degraded'] = bool(completed.returncode or not endpoint['passed']
                              or unexpected_delivery_loss or result['playback']['underruns'])
    save(case / 'result.json', result)
    results.append(result)
    save(reports / 'summary.json', results)
    (reports / 'commands.sh').write_text('\n'.join(commands) + '\n')
    print(json.dumps({k: result[k] for k in ['name', 'test_exit', 'asr_packets', 'tts_packets', 'degraded']}) +
          f" up={uplink['received']}/{uplink['sent']} down={delivered['received']}/{delivered['sent']}" +
          f" reorder={len(uplink['reordered_indices'])}/{len(delivered['reordered_indices'])}", flush=True)
    if required_recovery and result['degraded']:
        raise RuntimeError(f'{name}: required recovered endpoint audio failed; inspect result.json')
    return result


def run_diagnostics():
    # Three clean repetitions come first. Injection is never interpreted without
    # a working healthy response. Every case includes both audio directions.
    for i in range(3):
        baseline = run_case('healthy-' + str(i + 1), [], [])
        if baseline['test_exit'] or baseline['asr_packets'] == 0 or baseline['client'].get('audio_bytes', 0) == 0:
            raise RuntimeError('healthy baseline did not deliver real bidirectional audio')
    matrix = [
        ('delay-20', ['delay', '20ms']),
        ('delay-50', ['delay', '50ms']),
        ('delay-100', ['delay', '100ms']),
        ('jitter-50-20', ['delay', '50ms', '20ms', 'distribution', 'normal']),
        ('loss-1', ['loss', 'random', '1%']),
        ('loss-3', ['loss', 'random', '3%']),
        ('reorder-10', ['delay', '50ms', 'reorder', '10%', '50%']),
    ]
    for direction, nodes in [('up', ['edge']), ('down', ['server'])]:
        for name, params in matrix:
            run_case(direction + '-' + name, params, nodes)
    for name, params in [
        ('both-delay50-loss1', ['delay', '50ms', 'loss', 'random', '1%']),
        ('both-delay100-jitter30-reorder10', ['delay', '100ms', '30ms', 'reorder', '10%', '50%']),
    ]:
        run_case(name, params, ['edge', 'server'])
    # Refine the observed wire jitter effect without a Cartesian product.
    # Recovery, PLC and SDK post-EOS policy are measured separately.
    for direction, nodes in [('up', ['edge']), ('down', ['server'])]:
        selected = ['delay', '50ms', '20ms', 'distribution', 'normal']
        for mean, variation in [(20, 5), (50, 5), (20, 10), (50, 10)]:
            params = ['delay', f'{mean}ms', f'{variation}ms', 'distribution', 'normal']
            trial = run_case(f'{direction}-refine-{mean}-{variation}', params, nodes)
            metric = trial['wire_up' if direction == 'up' else 'wire_down']
            if len(metric['missing_indices']) >= 4 or metric['reordered_indices']:
                selected = params
                break
        for i, seed in enumerate([7919, 104729, 15485863], 1):
            run_case(f'{direction}-minimum-repeat-{i}', selected, nodes, seed)
        # Recheck the original severe condition on two additional seeds as well.
        for i, seed in enumerate([104729, 15485863], 1):
            run_case(f'{direction}-jitter20-repeat-{i}', ['delay', '50ms', '20ms', 'distribution', 'normal'], nodes, seed)
    run_case('healthy-final', [], [])
def run_continuity():
    tone = root / 'tests/gizclaw-e2e/testdata/audio/sfu-tone.ogg'
    manifest['input'] = {'mode': 'push-to-talk', 'ogg_sha256': hashlib.sha256(tone.read_bytes()).hexdigest()}
    manifest['provider'] = {'frames': 800, 'frame_ms': 20, 'pacing': 'absolute media deadlines, no receiver acknowledgements'}
    manifest['playback'] = {'observer': 'production Giztest audio_pacing', 'prebuffer_ms': 500}
    manifest['diagnostic_only_sctp_defer_initial_abandon'] = os.environ.get('GIZCLAW_NETEM_SCTP_DEFER_INITIAL_PROBE') == '1'
    save(reports / 'manifest.json', manifest)
    cases = [
        ('continuous-healthy', [], []),
        ('continuous-delay100', ['delay', '100ms'], ['edge', 'server']),
        ('continuous-down-jitter5', ['delay', '20ms', '5ms', 'distribution', 'normal'], ['server']),
        ('continuous-up-jitter5', ['delay', '20ms', '5ms', 'distribution', 'normal'], ['edge']),
        # Loss has its own recovery verdict; its original-integrity failure is
        # retained independently and can never become an intact-audio result.
        ('continuous-down-loss1', ['delay', '50ms', 'loss', 'random', '1%'], ['server']),
    ]
    if os.environ.get('GIZCLAW_NETEM_CONTINUITY_EXTENDED') == '1':
        cases.extend([
            ('continuous-delay150-jitter30', ['delay', '150ms', '30ms', 'distribution', 'normal'], ['edge', 'server']),
            ('continuous-delay250', ['delay', '250ms'], ['edge', 'server']),
        ])
    selected_text = os.environ.get('GIZCLAW_NETEM_CONTINUITY_CASES')
    if selected_text is not None:
        selected = {name.strip() for name in selected_text.split(',') if name.strip()}
        known = {name for name, _, _ in cases}
        if not selected or selected - known:
            raise RuntimeError(f'unknown or empty continuity case selection: {sorted(selected - known)}')
        if not {'continuous-healthy', 'continuous-down-loss1'} <= selected:
            raise RuntimeError('continuity case selection must include the healthy baseline and loss recovery gate')
        cases = [case for case in cases if case[0] in selected]
    manifest['continuity_cases'] = [name for name, _, _ in cases]
    save(reports / 'manifest.json', manifest)
    measured = []
    client_policies = []
    for name, params, nodes in cases:
        reset()
        case = reports / name
        case.mkdir()
        for node in nodes:
            execute(node + '-netem', 'tc', 'qdisc', 'replace', 'dev', interfaces[node], 'root',
                    'netem', 'limit', '10000', *params, 'seed', '7919')
        save(case / 'qdisc-before.json', qdisc())
        loss_case = name == 'continuous-down-loss1'
        document = '/src/tests/gizclaw-e2e/testdata/peer-audio/continuous.giztest.yaml'
        if loss_case:
            source = root / 'tests/gizclaw-e2e/testdata/peer-audio/continuous.giztest.yaml'
            (case / 'continuous-loss.giztest.yaml').write_text(loss_continuity_document(source.read_text()))
            document = '/reports/' + name + '/continuous-loss.giztest.yaml'
        started = time.time_ns()
        completed = command([*compose, 'exec', '-T', 'test', 'env',
            'GIZCLAW_TEST_ENDPOINT=edge:9821', 'LATENCY_TONE=' + base64.b64encode(tone.read_bytes()).decode(),
            '/binaries/gizclaw', 'test', 'run', document,
            '--parallel', '1', '--report', '/reports/' + name + '/giztest.json'], check=False, timeout=180)
        (case / 'client.log').write_text(completed.stdout)
        services = command([*compose, 'logs', '--no-color', '--since', str(started // 1000000000), 'server', 'edge']).stdout
        (case / 'services.log').write_text(services)
        observed = [x for x in traces(services) + traces(completed.stdout) if x['ns'] >= started]
        client_policies.extend(x for x in observed if x['node'] == 'client' and x['stage'] == 'packet_policy')
        save(case / 'trace.json', observed)
        save(case / 'qdisc-after.json', qdisc())
        path = case / 'giztest.json'
        report = json.loads(path.read_text()) if path.exists() else {}
        rounds = []
        for task in report.get('tasks', []):
            for step in task.get('steps', []):
                if step['id'].startswith('continuous_round_'):
                    evidence = step.get('evidence', {})
                    rounds.append({'id': step['id'], 'status': step['status'], 'error': step.get('error'),
                                   'input_packets': evidence.get('input_packets'),
                                   'pushed_packets': evidence.get('pushed_packets'),
                                   'pacing': evidence.get('audio_pacing'),
                                   'audio_eos_ms': evidence.get('audio_eos_ms'),
                                   'text_eos_ms': evidence.get('text_eos_ms')})
        packet_evidence = continuity_evidence(observed, rounds, manifest['provider']['frames'])
        wire_up = packet_evidence['wire']['up']
        wire_down = packet_evidence['wire']['down']
        recovered = loss_recovery_evidence(observed, rounds, manifest['provider']['frames']) if loss_case else None
        passed = (completed.returncode == 0 and report.get('status') == 'passed'
                  and (recovered['passed'] if loss_case else packet_evidence['passed']))
        result = {'case': name, 'passed': passed, 'giztest_exit': completed.returncode,
                  'parameters': params, 'egress': nodes, 'seed': 7919, 'rounds': rounds,
                  'wire_up': wire_up, 'wire_down': wire_down, 'packet_evidence': packet_evidence,
                  'loss_recovery': recovered}
        measured.append(result)
        save(case / 'result.json', result)
        save(reports / 'continuity-summary.json', measured)
        print(json.dumps({'case': name, 'passed': passed, 'giztest_exit': completed.returncode, 'rounds': rounds,
                          'up': [wire_up['received'], wire_up['sent']], 'down': [wire_down['received'], wire_down['sent']]}), flush=True)
        if name == 'continuous-healthy' and not passed:
            raise RuntimeError('healthy sustained playback baseline failed')
    service_policies = [x for x in traces(command([*compose, 'logs', '--no-color', 'server', 'edge']).stdout)
                        if x['stage'] == 'packet_policy']
    policies = service_policies + client_policies
    expected_default = 'ordered=false,lifetime_ms=-1,max_retransmits=0'
    by_node = {node: collections.Counter(x['detail'] for x in policies if x['node'] == node)
               for node in ['server', 'edge', 'client']}
    if manifest['diagnostic_only_ordered_timed_packet_policy']:
        expected_upstream = 'ordered=true,lifetime_ms=1000,max_retransmits=-1'
        # The seed tool makes an ordinary direct Server connection as well.
        policy_passed = (set(by_node['server']) <= {expected_default, expected_upstream}
                         and by_node['server'][expected_upstream] > 0
                         and by_node['server'][expected_upstream] == by_node['edge'][expected_upstream]
                         and set(by_node['edge']) == {expected_default, expected_upstream}
                         and set(by_node['client']) == {expected_default}
                         and by_node['edge'][expected_default] == by_node['client'][expected_default])
    else:
        # Client and upstream channels share the same unreliable policy. Require
        # observations at every node and extra Edge channels for its upstreams.
        policy_passed = (all(set(counts) == {expected_default} for counts in by_node.values())
                         and by_node['edge'][expected_default] > by_node['client'][expected_default])
    save(reports / 'policy-verification.json', {'passed': policy_passed, 'observed': by_node,
         'diagnostic_policy_probe': manifest['diagnostic_only_ordered_timed_packet_policy'],
         'diagnostic_sctp_probe': manifest['diagnostic_only_sctp_defer_initial_abandon']})
    if not policy_passed:
        raise RuntimeError('negotiated Packet DataChannel policies differ; inspect policy-verification.json')
    if not all(x['passed'] for x in measured):
        raise RuntimeError('sustained audio failed; inspect continuity-summary.json (EOS success does not override media failure)')


try:
    if os.environ.get('GIZCLAW_NETEM_CONTINUITY') == '1':
        run_continuity()
    else:
        run_diagnostics()
finally:
    reset()
    save(reports / 'qdisc-cleanup.json', qdisc())
    (reports / 'commands.sh').write_text('\n'.join(commands) + '\n')
