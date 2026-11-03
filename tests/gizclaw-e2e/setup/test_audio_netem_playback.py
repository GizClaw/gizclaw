"""Regression controls for the diagnostic's fixed-20-ms playback model."""
import ast
import pathlib
import unittest


# The matrix script owns live Docker setup at module scope. Load its actual
# pure helper without starting containers or evaluating fixture configuration.
source = pathlib.Path(__file__).with_name('run_audio_netem.py')
function = next(node for node in ast.parse(source.read_text()).body
                if isinstance(node, ast.FunctionDef) and node.name == 'playback')
namespace = {}
exec(compile(ast.Module(body=[function], type_ignores=[]), str(source), 'exec'), namespace)
playback = namespace['playback']


def arrivals(milliseconds):
    return [{'ns': value * 1_000_000} for value in milliseconds]


class PlaybackTests(unittest.TestCase):
    def test_missing_audio(self):
        self.assertEqual(playback([]), {'underruns': 0, 'missing_audio': True, 'stall_ms': 0})

    def test_paced_audio(self):
        result = playback(arrivals(range(0, 1600, 20)))
        self.assertEqual(result['underruns'], 0)
        self.assertEqual(result['stall_ms'], 0)

    def test_short_reply_waits_for_all_media(self):
        self.assertEqual(playback(arrivals([0, 750]))['underruns'], 0)

    def test_initial_burst_does_not_add_wall_clock_prebuffer(self):
        result = playback(arrivals([0] * 27 + [750] * 53))
        self.assertEqual(result['underruns'], 1)
        self.assertEqual(result['stall_ms'], 210)

    def test_later_stalls_accumulate(self):
        result = playback(arrivals([0] * 27 + [750] * 27 + [1500] * 26))
        self.assertEqual(result['underruns'], 2)
        self.assertEqual(result['stall_ms'], 420)


if __name__ == '__main__':
    unittest.main()
