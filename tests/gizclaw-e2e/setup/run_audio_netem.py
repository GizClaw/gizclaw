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
                values.append(json.loads(line.split('NETEM_TRACE ', 1)[1]))
            except json.JSONDecodeError:
                pass
    return values


def compare(source, received):
    indices = collections.defaultdict(list)
    for i, item in enumerate(source):
        indices[item['hash']].append(i)
    ambiguous = {key: ids for key, ids in indices.items() if len(ids) > 1}
    seen = collections.Counter()
    order = []
    unknown = 0
    for item in received:
        ids = indices.get(item['hash'], [])
        count = seen[item['hash']]
        seen[item['hash']] += 1
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


def playback(items):
    result = {'underruns': 0, 'missing_audio': not items, 'stall_ms': 0}
    buffered = 0
    started = None
    # Match peerAudioPacing.playback: start when media, not wall time,
    # reaches the prebuffer. Every packet in this fixture carries 20 ms.
    for index, _ in enumerate(items):
        buffered += 20
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
        buffered += 20
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
    uplink = compare(select('client', 'client_sent'), select('server', 'server_input'))
    downlink = compare(select('server', 'server_output'), select('client', 'client_raw'))
    delivered = compare(select('server', 'server_output'), select('client', 'client_delivered'))
    client = json.loads((case / 'client.json').read_text()) if (case / 'client.json').exists() else {}
    result = {'name': name, 'tc_args': params, 'egress_nodes': nodes, 'seed': seed,
              'one_way_delay_scope': 'each listed node egress; bidirectional RTT increment is twice a symmetric delay',
              'test_exit': completed.returncode, 'client': client,
              'uplink': uplink, 'downlink_raw': downlink, 'downlink_delivered': delivered,
              'playback': playback(select('client', 'client_delivered')),
              'asr_packets': len(select('server', 'asr_input')),
              'tts_packets': len(select('server', 'tts_output')),
              'tunnel_queue_drops': len([o for o in observations if o['stage'] == 'tunnel_drop']),
              'wire_up': compare(select('edge', 'tunnel_wire_send'), select('server', 'tunnel_wire_read')),
              'wire_down': compare(select('server', 'tunnel_wire_send'), select('edge', 'tunnel_wire_read')),
              'server_input_rejected': len(select('server', 'server_input_rejected')),
              'control': [o for o in observations if o['stage'].startswith('control_') or o['stage'] in ['client_bos', 'client_eos', 'asr_bos', 'asr_eos']]}
    tail_hashes = {o['hash'] for o in select('client', 'client_post_eos_drop')}
    result['sdk_policy_tail_indices'] = [i for i, o in enumerate(select('server', 'server_output')) if o['hash'] in tail_hashes]
    unexpected_delivery_loss = set(delivered['missing_indices']) - set(result['sdk_policy_tail_indices'])
    result['degraded'] = bool(completed.returncode or uplink['missing_indices'] or uplink['reordered_indices']
                              or downlink['missing_indices'] or downlink['reordered_indices']
                              or unexpected_delivery_loss or delivered['reordered_indices'] or result['playback']['underruns'])
    save(case / 'result.json', result)
    results.append(result)
    save(reports / 'summary.json', results)
    (reports / 'commands.sh').write_text('\n'.join(commands) + '\n')
    print(json.dumps({k: result[k] for k in ['name', 'test_exit', 'asr_packets', 'tts_packets', 'degraded']}) +
          f" up={uplink['received']}/{uplink['sent']} down={delivered['received']}/{delivered['sent']}" +
          f" reorder={len(uplink['reordered_indices'])}/{len(delivered['reordered_indices'])}", flush=True)
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
    # Refine the observed jitter effect without a Cartesian product. Transport
    # loss here excludes the SDK's separately counted post-EOS tail policy.
    for direction, nodes in [('up', ['edge']), ('down', ['server'])]:
        selected = ['delay', '50ms', '20ms', 'distribution', 'normal']
        for mean, variation in [(20, 5), (50, 5), (20, 10), (50, 10)]:
            params = ['delay', f'{mean}ms', f'{variation}ms', 'distribution', 'normal']
            trial = run_case(f'{direction}-refine-{mean}-{variation}', params, nodes)
            metric = trial['uplink' if direction == 'up' else 'downlink_raw']
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
    manifest['diagnostic_only_ordered_timed_packet_policy'] = os.environ.get('GIZCLAW_NETEM_ORDERED_TIMED_PACKET_PROBE') == '1'
    save(reports / 'manifest.json', manifest)
    cases = [
        ('continuous-healthy', [], []),
        ('continuous-delay100', ['delay', '100ms'], ['edge', 'server']),
        ('continuous-down-jitter5', ['delay', '20ms', '5ms', 'distribution', 'normal'], ['server']),
        ('continuous-up-jitter5', ['delay', '20ms', '5ms', 'distribution', 'normal'], ['edge']),
        # The default CI matrix must guard the measured cross-region recovery
        # boundary as well as jitter without configured loss.
        ('continuous-delay150-loss1', ['delay', '150ms', 'loss', 'random', '1%'], ['edge', 'server']),
    ]
    if os.environ.get('GIZCLAW_NETEM_CONTINUITY_EXTENDED') == '1':
        cases.extend([
            ('continuous-down-loss1', ['delay', '50ms', 'loss', 'random', '1%'], ['server']),
            ('continuous-up-loss1', ['delay', '50ms', 'loss', 'random', '1%'], ['edge']),
            ('continuous-down-reorder10', ['delay', '50ms', 'reorder', '10%', '50%'], ['server']),
            ('continuous-up-reorder10', ['delay', '50ms', 'reorder', '10%', '50%'], ['edge']),
            ('continuous-delay150-jitter30', ['delay', '150ms', '30ms', 'distribution', 'normal'], ['edge', 'server']),
            ('continuous-delay250', ['delay', '250ms'], ['edge', 'server']),
        ])
    selected_text = os.environ.get('GIZCLAW_NETEM_CONTINUITY_CASES')
    if selected_text is not None:
        selected = {name.strip() for name in selected_text.split(',') if name.strip()}
        known = {name for name, _, _ in cases}
        if not selected or selected - known:
            raise RuntimeError(f'unknown or empty continuity case selection: {sorted(selected - known)}')
        if 'continuous-healthy' not in selected:
            raise RuntimeError('continuity case selection must include the healthy baseline')
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
        started = time.time_ns()
        completed = command([*compose, 'exec', '-T', 'test', 'env',
            'GIZCLAW_TEST_ENDPOINT=edge:9821', 'LATENCY_TONE=' + base64.b64encode(tone.read_bytes()).decode(),
            '/binaries/gizclaw', 'test', 'run',
            '/src/tests/gizclaw-e2e/testdata/peer-audio/continuous.giztest.yaml',
            '--parallel', '1', '--report', '/reports/' + name + '/giztest.json'], check=False, timeout=180)
        (case / 'client.log').write_text(completed.stdout)
        services = command([*compose, 'logs', '--no-color', '--since', str(started // 1000000000), 'server', 'edge']).stdout
        (case / 'services.log').write_text(services)
        observed = [x for x in traces(services) + traces(completed.stdout) if x['ns'] >= started]
        client_policies.extend(x for x in observed if x['node'] == 'client' and x['stage'] == 'packet_policy')
        save(case / 'trace.json', observed)
        save(case / 'qdisc-after.json', qdisc())
        def select(node, stage):
            return sorted((x for x in observed if x['node'] == node and x['stage'] == stage), key=lambda x:x['ns'])
        wire_up = compare(select('edge', 'tunnel_wire_send'), select('server', 'tunnel_wire_read'))
        wire_down = compare(select('server', 'tunnel_wire_send'), select('edge', 'tunnel_wire_read'))
        path = case / 'giztest.json'
        report = json.loads(path.read_text()) if path.exists() else {}
        rounds = []
        for task in report.get('tasks', []):
            for step in task.get('steps', []):
                if step['id'].startswith('continuous_round_'):
                    evidence = step.get('evidence', {})
                    rounds.append({'id': step['id'], 'status': step['status'], 'error': step.get('error'),
                                   'pacing': evidence.get('audio_pacing'),
                                   'audio_eos_ms': evidence.get('audio_eos_ms'),
                                   'text_eos_ms': evidence.get('text_eos_ms')})
        ordered_up = ([x['hash'] for x in select('edge', 'tunnel_wire_send')]
                      == [x['hash'] for x in select('server', 'tunnel_wire_read')])
        ordered_down = ([x['hash'] for x in select('server', 'tunnel_wire_send')]
                        == [x['hash'] for x in select('edge', 'tunnel_wire_read')])
        passed = (completed.returncode == 0 and report.get('status') == 'passed' and len(rounds) == 3
                  and ordered_up and ordered_down)
        result = {'case': name, 'passed': passed, 'giztest_exit': completed.returncode,
                  'parameters': params, 'egress': nodes, 'seed': 7919, 'rounds': rounds,
                  'wire_up': wire_up, 'wire_down': wire_down}
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
    lifetime_ms = 200 if manifest['diagnostic_only_ordered_timed_packet_policy'] else 500
    expected_upstream = f'ordered=true,lifetime_ms={lifetime_ms},max_retransmits=-1'
    by_node = {node: collections.Counter(x['detail'] for x in policies if x['node'] == node)
               for node in ['server', 'edge', 'client']}
    # The seed tool makes an ordinary direct Server connection as well. Match
    # both ends of every Edge upstream and every client-to-Edge connection;
    # do not mistake the seed's default channel for an Edge upstream.
    policy_passed = (set(by_node['server']) <= {expected_default, expected_upstream}
                     and by_node['server'][expected_upstream] > 0
                     and by_node['server'][expected_upstream] == by_node['edge'][expected_upstream]
                     and set(by_node['edge']) == {expected_default, expected_upstream}
                     and set(by_node['client']) == {expected_default}
                     and by_node['edge'][expected_default] == by_node['client'][expected_default])
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
