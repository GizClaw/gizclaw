"""Fail-closed controls for sustained audio's physical-packet evidence."""
import ast
import collections
import json
import pathlib
import unittest


# Load the actual pure checks without running the matrix's Docker setup.
source = pathlib.Path(__file__).with_name('run_audio_netem.py')
functions = [node for node in ast.parse(source.read_text()).body
             if isinstance(node, ast.FunctionDef) and node.name in {
                 'traces', 'compare', 'select_trace', 'recovery_summary', 'endpoint_evidence', 'continuity_evidence',
                 'loss_continuity_document', 'loss_recovery_evidence'}]
namespace = {'collections': collections, 'json': json}
exec(compile(ast.Module(body=functions, type_ignores=[]), str(source), 'exec'), namespace)
traces = namespace['traces']
continuity_evidence = namespace['continuity_evidence']


def fixture(post_eos=0):
    rounds = [{'id': f'continuous_round_{index}', 'status': 'passed', 'input_packets': 100,
               'pushed_packets': 100, 'pacing': {'packets': 800, 'audio_ms': 16000,
                   'prebuffer_ms': 500, 'underruns': 0, 'minimum_buffer_ms': 100}}
              for index in range(1, 4)]
    observed = []

    def append(node, stage, value, detail='', sequence=None):
        item = {'node': node, 'stage': stage, 'ns': len(observed) + 1,
                'hash': f'{value:064x}', 'bytes': 40, 'detail': detail,
                'sequence': value % 65536 if sequence is None else sequence,
                'timestamp': (value * 960) % 2**32, 'duration48': 960, 'source': 'session/ssrc'}
        if stage.startswith('tunnel_wire_'):
            # Wire observations carry SessionID and SSRC, not a local SourceID.
            item.update(wire_version=3, wire_hash=f'{value + 1000000:064x}',
                        detail='00000000000000000000000000000001', ssrc=9, source='')
        if stage in {'media_recovered', 'server_input', 'client_recovered', 'client_delivered', 'client_post_eos_drop'}:
            item.update(pcm_hash=f'{value + 2000000:064x}', pcm_bytes=640, concealed=False)
        observed.append(item)

    for value in range(300):
        for node, stage in [('client', 'rtp_write_sample'), ('edge', 'rtp_read'),
                            ('edge', 'tunnel_wire_send'), ('server', 'tunnel_wire_read'),
                            ('server', 'media_recovered'), ('server', 'server_input')]:
            append(node, stage, value)
    for value in range(2400 + post_eos):
        for node, stage in [('server', 'server_output'), ('server', 'tunnel_wire_send'),
                            ('edge', 'tunnel_wire_read'), ('edge', 'rtp_write_packet'),
                            ('client', 'rtp_read'), ('client', 'media_recovered'),
                            ('client', 'client_recovered')]:
            append(node, stage, 10000 + value)
        append('client', 'client_post_eos_drop' if value >= 2400 else 'client_delivered', 10000 + value)
    for stream in range(3):
        for sequence in range(800):
            append('server', 'tts_output', 20000 + sequence, f'reply-{stream}', sequence)
    return observed, rounds


def loss_fixture():
    observed, rounds = fixture()
    affected = {('edge', 'tunnel_wire_read'), ('edge', 'rtp_write_packet'), ('client', 'rtp_read'),
                ('client', 'media_recovered'), ('client', 'client_recovered'), ('client', 'client_delivered')}
    lost = {10010, 10011}
    result = []
    for item in observed:
        if (item['node'], item['stage']) in affected and int(item['hash'], 16) in lost:
            if item['stage'] in {'media_recovered', 'client_recovered', 'client_delivered'} and int(item['hash'], 16) == min(lost):
                result.append({**item, 'hash': '', 'bytes': 0, 'concealed': True,
                               'pcm_bytes': 1280, 'pcm_hash': 'f' * 64, 'duration48': 1920})
        else:
            result.append(item)
    for item in rounds:
        item['pacing']['playout_ms'] = 16000
    rounds[0]['pacing'].update(packets=798, audio_ms=15960)
    return result, rounds


class LossRecoveryTests(unittest.TestCase):
    def setUp(self):
        self.observed, self.rounds = loss_fixture()

    def result(self):
        return namespace['loss_recovery_evidence'](self.observed, self.rounds, 800)

    def test_observed_grouped_loss_reaches_playback_without_becoming_intact(self):
        result = self.result()
        self.assertTrue(result['passed'], result['errors'])
        self.assertEqual(result['delivered_concealed_samples'], 640)
        self.assertEqual(result['wire']['down']['missing_indices'], [10, 11])
        self.assertFalse(continuity_evidence(self.observed, self.rounds, 800)['passed'])

    def test_wire_identity_uses_session_and_ssrc_without_local_source(self):
        wire = [x for x in self.observed if x['stage'].startswith('tunnel_wire_')]
        self.assertTrue(wire)
        self.assertTrue(all(x['source'] == '' for x in wire))
        self.assertTrue(self.result()['passed'])

    def test_missing_or_invalid_wire_identity_and_duration_fail_closed(self):
        for field, value in [('detail', ''), ('detail', 'not-a-session'), ('ssrc', None),
                             ('ssrc', -1), ('ssrc', 2**32), ('ssrc', {}), ('duration48', 0)]:
            with self.subTest(field=field, value=value):
                self.observed, self.rounds = loss_fixture()
                item = next(x for x in self.observed if x['stage'] == 'tunnel_wire_send')
                item[field] = value
                self.assertFalse(self.result()['passed'])

    def test_ineffective_injection_cannot_qualify_plc(self):
        observed, rounds = fixture()
        for item in rounds:
            item['pacing']['playout_ms'] = 16000
        self.assertFalse(namespace['loss_recovery_evidence'](observed, rounds, 800)['passed'])

    def test_missing_plc_cannot_qualify_surviving_audio(self):
        self.observed = [item for item in self.observed if not item.get('concealed')]
        self.assertFalse(self.result()['passed'])

    def test_plc_cannot_cover_a_packet_without_observed_wire_loss(self):
        original, _ = fixture()
        self.observed.extend(item for item in original
                             if item['node'] == 'edge' and item['stage'] == 'tunnel_wire_read'
                             and int(item['hash'], 16) in {10010, 10011})
        self.assertFalse(self.result()['passed'])

    def test_wrong_plc_duration_or_clock_cannot_pass(self):
        for field, value in [('duration48', 960), ('timestamp', 10010 * 960 + 1), ('sequence', 10011)]:
            with self.subTest(field=field):
                self.observed, self.rounds = loss_fixture()
                item = next(x for x in self.observed if x['stage'] == 'media_recovered' and x.get('concealed'))
                item[field] = value
                self.assertFalse(self.result()['passed'])

    def test_fake_original_and_changed_application_pcm_cannot_pass(self):
        for field, value in [('hash', 'a' * 64), ('pcm_hash', 'a' * 64), ('pcm_bytes', 640)]:
            with self.subTest(field=field):
                self.observed, self.rounds = loss_fixture()
                item = next(x for x in self.observed if x['stage'] == 'client_delivered' and x.get('concealed'))
                item[field] = value
                self.assertFalse(self.result()['passed'])

    def test_successful_eos_cannot_override_playback_failure(self):
        for field, value in [('underruns', 1), ('playout_ms', 15960), ('minimum_buffer_ms', -1)]:
            with self.subTest(field=field):
                self.observed, self.rounds = loss_fixture()
                self.rounds[0]['pacing'][field] = value
                self.assertFalse(self.result()['passed'])

    def test_missing_source_boundary_or_playback_evidence_cannot_pass(self):
        for node, stage in [('server', 'tts_output'), ('server', 'server_output'),
                            ('server', 'tunnel_wire_send'), ('client', 'client_recovered'),
                            ('client', 'client_delivered')]:
            with self.subTest(stage=stage):
                self.observed, self.rounds = loss_fixture()
                self.observed.remove(next(x for x in self.observed if x['node'] == node and x['stage'] == stage))
                self.assertFalse(self.result()['passed'])

    def test_unknown_trailing_loss_and_discarded_plc_cannot_qualify_playback(self):
        self.observed = [item for item in self.observed
                         if not ((item['node'], item['stage']) in {
                             ('edge', 'tunnel_wire_read'), ('edge', 'rtp_write_packet'),
                             ('client', 'rtp_read'), ('client', 'media_recovered'),
                             ('client', 'client_recovered'), ('client', 'client_delivered')}
                             and item['hash'] == f'{12399:064x}')]
        self.assertFalse(self.result()['passed'])
        self.observed, self.rounds = loss_fixture()
        next(x for x in self.observed if x['stage'] == 'client_delivered' and x.get('concealed'))['stage'] = 'client_post_eos_drop'
        self.assertFalse(self.result()['passed'])

    def test_source_clock_inside_a_grouped_loss_must_remain_exact(self):
        item = next(x for x in self.observed if x['node'] == 'server' and x['stage'] == 'tunnel_wire_send'
                    and x['sequence'] == 10011)
        item['timestamp'] += 1
        self.assertFalse(self.result()['passed'])

    def test_missing_rounds_and_original_playback_counts_cannot_pass(self):
        self.assertFalse(namespace['loss_recovery_evidence'](self.observed, self.rounds[:2], 800)['passed'])
        self.rounds[0]['pacing']['packets'] += 1
        self.assertFalse(self.result()['passed'])

    def test_loss_expectations_leave_the_strict_document_unchanged(self):
        original = (source.parent.parent / 'testdata/peer-audio/continuous.giztest.yaml').read_text()
        generated = namespace['loss_continuity_document'](original)
        self.assertIn('/audio_pacing/playout_ms: {minimum: 16000}', generated)
        self.assertNotIn('/audio_pacing/packets: {minimum: 800}', generated)
        self.assertIn('/audio_pacing/packets: {minimum: 800}', original)
        self.assertEqual(original.replace('      /audio_pacing/packets: {minimum: 800}\n', '')
                         .replace('/audio_pacing/audio_ms:', '/audio_pacing/playout_ms:'), generated)
        with self.assertRaisesRegex(RuntimeError, 'expectations changed'):
            namespace['loss_continuity_document'](generated)


class EvidenceTests(unittest.TestCase):
    def setUp(self):
        self.observed, self.rounds = fixture()

    def result(self):
        return continuity_evidence(self.observed, self.rounds, 800)

    def test_complete_three_rounds(self):
        self.assertTrue(self.result()['passed'])

    def test_explicit_post_eos_packets_are_reconciled(self):
        self.assertTrue(continuity_evidence(*fixture(post_eos=2), 800)['passed'])

    def test_empty_vectors_do_not_pass(self):
        self.assertFalse(continuity_evidence([], self.rounds, 800)['passed'])

    def test_missing_wire_traces_with_intact_application_evidence(self):
        self.observed = [item for item in self.observed
                         if item['stage'] not in {'tunnel_wire_send', 'tunnel_wire_read'}]
        self.assertFalse(self.result()['passed'])

    def test_identical_truncated_vectors_do_not_pass(self):
        for node, stage in [('edge', 'tunnel_wire_send'), ('server', 'tunnel_wire_read'),
                            ('server', 'tunnel_wire_send'), ('edge', 'tunnel_wire_read')]:
            self.observed.remove(next(item for item in self.observed
                                      if item['node'] == node and item['stage'] == stage))
        self.assertFalse(self.result()['passed'])

    def test_missing_whole_reply_source_does_not_pass(self):
        self.observed = [item for item in self.observed
                         if not (item['stage'] == 'tts_output' and item['detail'] == 'reply-2')]
        self.assertFalse(self.result()['passed'])

    def test_incomplete_provider_ordinals_do_not_pass(self):
        item = next(item for item in self.observed if item['stage'] == 'tts_output')
        item['sequence'] = 1
        self.assertFalse(self.result()['passed'])

    def test_counts_cannot_hide_reordered_endpoint_payloads(self):
        items = [item for item in self.observed
                 if item['node'] == 'client' and item['stage'] == 'client_recovered']
        items[0]['hash'], items[1]['hash'] = items[1]['hash'], items[0]['hash']
        self.assertFalse(self.result()['passed'])

    def test_wire_reorder_remains_visible_after_successful_recovery(self):
        items = [item for item in self.observed
                 if item['node'] == 'edge' and item['stage'] == 'tunnel_wire_read']
        items[0]['ns'], items[1]['ns'] = items[1]['ns'], items[0]['ns']
        result = self.result()
        self.assertTrue(result['passed'], result['errors'])
        self.assertEqual(result['wire']['down']['reordered_indices'], [0])

    def test_rtp_identity_proves_wire_reorder_with_repeated_codec_payloads(self):
        compare = namespace['compare']
        source = [{'hash': 'same', 'wire_hash': value, 'bytes': 3, 'ns': i} for i, value in enumerate(['rtp-1', 'rtp-2'])]
        self.assertEqual(compare(source, source[::-1], 'wire_hash')['reordered_indices'], [0])
        self.assertEqual(compare(source, source[::-1])['ambiguous_source_hashes'], 1)

    def test_plc_cannot_replace_a_real_original_in_strict_continuity(self):
        for stage in ['media_recovered', 'client_recovered', 'client_delivered']:
            item = next(x for x in self.observed if x['node'] == 'client' and x['stage'] == stage)
            item.update(hash='', bytes=0, concealed=True)
        result = self.result()
        self.assertFalse(result['passed'])
        self.assertEqual(result['recovery']['down']['concealed_samples'], 320)
        self.assertEqual(result['recovery']['down']['concealed_ms'], 20)

    def test_explicit_forwarding_must_preserve_sequence_and_time(self):
        item = next(x for x in self.observed if x['stage'] == 'rtp_write_packet')
        item['timestamp'] += 960
        self.assertFalse(self.result()['passed'])

    def test_receiver_clock_and_pcm_must_reach_endpoint_unchanged(self):
        item = next(x for x in self.observed if x['node'] == 'client' and x['stage'] == 'media_recovered')
        item['sequence'] += 1
        self.assertFalse(self.result()['passed'])
        item['sequence'] -= 1
        item['pcm_hash'] = 'e' * 64
        self.assertFalse(self.result()['passed'])

    def test_legacy_wire_or_application_fallback_cannot_qualify_rtp_recovery(self):
        item = next(x for x in self.observed if x['stage'] == 'tunnel_wire_send')
        item['wire_version'] = 2
        self.assertFalse(self.result()['passed'])
        item['wire_version'] = 3
        item = next(x for x in self.observed if x['stage'] == 'server_input')
        del item['pcm_bytes']
        self.assertFalse(self.result()['passed'])

    def test_wraparound_is_contiguous(self):
        items = [{'hash': 'real', 'pcm_bytes': 640, 'concealed': False, 'source': 's', 'duration48': 960,
                  'sequence': 65535, 'timestamp': 0xfffffc40},
                 {'hash': 'real', 'pcm_bytes': 640, 'concealed': False, 'source': 's', 'duration48': 960,
                  'sequence': 0, 'timestamp': 0}]
        self.assertEqual(namespace['recovery_summary'](items)['clock_errors'], [])

    @staticmethod
    def recovered_frame(sequence, timestamp, duration48=960, concealed=False):
        return {'hash': '' if concealed else 'real', 'pcm_bytes': duration48 * 2 // 3,
                'concealed': concealed, 'source': 's', 'duration48': duration48,
                'sequence': sequence, 'timestamp': timestamp}

    def test_grouped_plc_preserves_media_clock_across_multiple_sequences(self):
        items = [self.recovered_frame(10, 0), self.recovered_frame(11, 960, 1920, True),
                 self.recovered_frame(13, 2880)]
        result = namespace['recovery_summary'](items)
        self.assertEqual(result['clock_errors'], [])
        self.assertEqual(result['concealed_samples'], 640)
        self.assertEqual(result['concealed_ms'], 40)
        items = [self.recovered_frame(65534, 2**32 - 1920),
                 self.recovered_frame(65535, 2**32 - 960, 1920, True),
                 self.recovered_frame(1, 960)]
        self.assertEqual(namespace['recovery_summary'](items)['clock_errors'], [])

    def test_grouped_plc_is_bounded_by_samples_not_one_encoded_packet(self):
        items = [self.recovered_frame(100, 0), self.recovered_frame(101, 960, 24000, True),
                 self.recovered_frame(126, 24960)]
        result = namespace['recovery_summary'](items)
        self.assertEqual(result['clock_errors'], [])
        self.assertEqual(result['concealed_ms'], 500)
        items[1] = self.recovered_frame(101, 960, 24120, True)
        self.assertIn('missing PCM/provenance/RTP metadata', namespace['recovery_summary'](items)['clock_errors'])
        real = self.recovered_frame(1, 0, 5880)
        self.assertIn('missing PCM/provenance/RTP metadata', namespace['recovery_summary']([real])['clock_errors'])

    def test_grouped_plc_does_not_hide_duplicate_backward_or_impossible_sequences(self):
        for sequence in [11, 10, 28]:
            with self.subTest(sequence=sequence):
                items = [self.recovered_frame(10, 0), self.recovered_frame(11, 960, 1920, True),
                         self.recovered_frame(sequence, 2880)]
                self.assertIn('invalid recovered RTP sequence after PLC', namespace['recovery_summary'](items)['clock_errors'])
        items = [self.recovered_frame(10, 0), self.recovered_frame(11, 960, 1920, True),
                 self.recovered_frame(13, 2881)]
        self.assertIn('noncontiguous recovered RTP timestamp', namespace['recovery_summary'](items)['clock_errors'])
        items = [self.recovered_frame(10, 0), self.recovered_frame(12, 960)]
        self.assertIn('noncontiguous recovered RTP sequence', namespace['recovery_summary'](items)['clock_errors'])

    def test_missing_round_and_missing_independent_counts(self):
        self.assertFalse(continuity_evidence(self.observed, self.rounds[:2], 800)['passed'])
        del self.rounds[2]['pushed_packets']
        self.assertFalse(self.result()['passed'])

    def test_successful_eos_cannot_override_a_playback_underrun(self):
        self.rounds[0]['pacing']['underruns'] = 1
        self.assertFalse(self.result()['passed'])

    def test_missing_playback_evidence_cannot_pass(self):
        del self.rounds[0]['pacing']['minimum_buffer_ms']
        self.assertFalse(self.result()['passed'])

    def test_duplicate_round_ids_do_not_pass(self):
        self.rounds[2]['id'] = self.rounds[1]['id']
        self.assertFalse(self.result()['passed'])

    def test_missing_client_tail_trace_does_not_shrink_expectation(self):
        observed, rounds = fixture(post_eos=2)
        observed = [item for item in observed if item['stage'] != 'client_post_eos_drop']
        self.assertFalse(continuity_evidence(observed, rounds, 800)['passed'])

    def test_noise_is_ignored_but_marked_truncation_is_rejected(self):
        item = self.observed[0]
        self.assertEqual(traces('ordinary log noise\nNETEM_TRACE ' + json.dumps(item)), [item])
        for invalid in ['{"hash":', '[]', '{}', json.dumps({**item, 'ns': 'wrong'}),
                        json.dumps({**item, 'error': 'invalid tunnel RTP packet'})]:
            with self.subTest(invalid=invalid), self.assertRaisesRegex(RuntimeError, 'malformed NETEM_TRACE'):
                traces('NETEM_TRACE ' + invalid)

    def test_marked_fake_encoded_plc_is_rejected(self):
        item = next(x for x in self.observed if x['stage'] == 'server_input')
        item['concealed'] = True
        with self.assertRaisesRegex(RuntimeError, 'malformed NETEM_TRACE'):
            traces('NETEM_TRACE ' + json.dumps(item))


if __name__ == '__main__':
    unittest.main()
