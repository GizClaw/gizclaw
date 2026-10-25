import json
import os
from pathlib import Path
import signal
import subprocess
import tempfile
import time
import unittest


REPO_ROOT = Path(__file__).resolve().parents[3]
HELPER = REPO_ROOT / "tests/gizclaw-e2e/setup/run_docker_giztest.sh"
DOCUMENT = REPO_ROOT / "tests/gizclaw-e2e/giztest/all.ping.giztest.yaml"

FAKE_DOCKER = r'''#!/usr/bin/env python3
import json
import os
from pathlib import Path
import shutil
import sys
import time

root = Path(os.environ["FAKE_DOCKER_ROOT"])
args = sys.argv[1:]
if args[0] == "ps":
    print("fixture-server")
elif args[0] == "inspect":
    print(Path(os.environ["FAKE_DOCKER_REPO_ROOT"]) /
          "tests/gizclaw-e2e/docker/docker-compose.yaml")
elif args[0] == "exec" and "timeout" in args:
    report = root / "container" / args[args.index("--report") + 1].lstrip("/")
    report.parent.mkdir(parents=True, exist_ok=True)
    (root / "client-pid").write_text(str(os.getpid()))
    (root / "ready").touch()
    mode = os.environ["FAKE_DOCKER_MODE"]
    if mode == "block":
        deadline = time.monotonic() + 30
        while not (root / "stopped").exists() and time.monotonic() < deadline:
            time.sleep(0.01)
    status = "failed" if mode == "failed" else "passed"
    report.write_text(json.dumps({"status": status, "tasks": []}))
    sys.exit(4 if mode == "failed" else 0)
elif args[0] == "exec":
    (root / "stopped").touch()
elif args[0] == "cp":
    source = root / "container" / args[1].split(":", 1)[1].lstrip("/")
    shutil.copyfile(source, args[2])
else:
    raise SystemExit("unexpected Docker operation")
'''


@unittest.skipUnless(os.name == "posix", "the Bash wrapper uses POSIX signals")
class DockerGiztestWrapperTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="giztest-docker-wrapper-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        binary = self.root / "docker"
        binary.write_text(FAKE_DOCKER)
        binary.chmod(0o755)

    def start(self, mode, report):
        env = os.environ.copy()
        env.update(
            PATH=str(self.root) + os.pathsep + env.get("PATH", ""),
            GIZCLAW_E2E_DOCKER_PROJECT="wrapper-regression",
            GIZCLAW_TEST_REGISTRATION_TOKEN="fixture",
            FAKE_DOCKER_ROOT=str(self.root),
            FAKE_DOCKER_REPO_ROOT=str(REPO_ROOT),
            FAKE_DOCKER_MODE=mode,
        )
        return subprocess.Popen(
            ["bash", str(HELPER), str(report), str(DOCUMENT)],
            cwd=REPO_ROOT,
            env=env,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )

    def stop_group(self, child):
        try:
            os.killpg(child.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        child.communicate(timeout=5)

    def test_exit_status_and_report_are_preserved(self):
        for mode, expected in [("passed", 0), ("failed", 4)]:
            with self.subTest(mode=mode):
                report = self.root / (mode + ".json")
                report.write_text('{"status":"old"}')
                child = self.start(mode, report)
                try:
                    output, _ = child.communicate(timeout=5)
                    self.assertEqual(child.returncode, expected, output.decode())
                    self.assertEqual(json.loads(report.read_text())["status"], mode)
                    self.assertEqual(list(self.root.glob(report.name + ".*")), [])
                finally:
                    self.stop_group(child)

    def test_direct_sigterm_stops_and_reaps_the_docker_client(self):
        child = self.start("block", self.root / "cancelled.json")
        try:
            deadline = time.monotonic() + 5
            while not (self.root / "ready").exists() and time.monotonic() < deadline:
                self.assertIsNone(child.poll(), "wrapper exited before the client started")
                time.sleep(0.01)
            self.assertTrue((self.root / "ready").exists(), "Docker client did not start")
            client_pid = int((self.root / "client-pid").read_text())
            os.kill(child.pid, signal.SIGTERM)
            try:
                child.wait(timeout=3)
            except subprocess.TimeoutExpired:
                self.fail("direct SIGTERM deferred cleanup behind the Docker client")
            self.assertEqual(child.returncode, 143)
            self.assertTrue((self.root / "stopped").exists(), "remote cleanup was not requested")
            with self.assertRaises(ProcessLookupError):
                os.kill(client_pid, 0)
        finally:
            self.stop_group(child)


if __name__ == "__main__":
    unittest.main()
