"""Fail-closed controls for sustained audio's physical-packet evidence."""
import ast
import collections
import json
import pathlib
import unittest


# Load the actual pure checks without running the matrix's Docker setup.
source = pathlib.Path(__file__).with_name('run_audio_netem.py')
functions = [node for node in ast.parse(source.read_text()).body
             if isinstance(node, ast.FunctionDef) and node.name in {'traces', 'continuity_evidence'}]
namespace = {'collections': collections, 'json': json}
exec(compile(ast.Module(body=functions, type_ignores=[]), str(source), 'exec'), namespace)
traces = namespace['traces']
continuity_evidence = namespace['continuity_evidence']


def fixture(post_eos=0):
    rounds = [{'id': f'continuous_round_{index}', 'status': 'passed', 'input_packets': 100,
               'pushed_packets': 100, 'pacing': {'packets': 800}}
              for index in range(1, 4)]
    observed = []

    def append(node, stage, value, detail='', sequence=-1):
        observed.append({'node': node, 'stage': stage, 'ns': len(observed) + 1,
                         'hash': f'{value:064x}', 'bytes': 40, 'detail': detail,
                         'sequence': sequence, 'timestamp': -1})

    for value in range(300):
        for node, stage in [('client', 'rtp_write_sample'), ('edge', 'tunnel_wire_send'),
                            ('server', 'tunnel_wire_read'), ('server', 'server_input')]:
            append(node, stage, value)
    for value in range(2400 + post_eos):
        for node, stage in [('server', 'server_output'), ('server', 'tunnel_wire_send'),
                            ('edge', 'tunnel_wire_read'), ('edge', 'rtp_write_sample'),
                            ('client', 'client_raw')]:
            append(node, stage, 10000 + value)
        if value >= 2400:
            append('client', 'client_post_eos_drop', 10000 + value)
    for stream in range(3):
        for sequence in range(800):
            append('server', 'tts_output', 20000 + sequence, f'reply-{stream}', sequence)
    return observed, rounds


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

    def test_counts_cannot_hide_reordered_payloads(self):
        items = [item for item in self.observed
                 if item['node'] == 'edge' and item['stage'] == 'tunnel_wire_read']
        items[0]['hash'], items[1]['hash'] = items[1]['hash'], items[0]['hash']
        self.assertFalse(self.result()['passed'])

    def test_missing_round_and_missing_independent_counts(self):
        self.assertFalse(continuity_evidence(self.observed, self.rounds[:2], 800)['passed'])
        del self.rounds[2]['pushed_packets']
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
        with self.assertRaisesRegex(RuntimeError, 'malformed NETEM_TRACE'):
            traces('NETEM_TRACE {"hash":')


if __name__ == '__main__':
    unittest.main()
