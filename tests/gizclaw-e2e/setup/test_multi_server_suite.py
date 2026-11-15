import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tempfile
from unittest import TestCase, main, mock


E2E = Path(__file__).resolve().parents[1]
SHARED = E2E / "run_tests.sh"
MULTI = E2E / "setup/run_multi_server_suite.sh"


def shell_function(source, name):
    match = re.search(r"^" + name + r"\(\) \{\n.*?^\}\n", source, re.MULTILINE | re.DOTALL)
    if match is None:
        raise AssertionError(f"missing shell function: {name}")
    return match.group()


class SharedScenarioDiscoveryTests(TestCase):
    def test_all_four_language_suites_exclude_dedicated_usage_cases(self):
        source = SHARED.read_text()
        filter_function = shell_function(source, "filter_shared_stack_giztests")
        dedicated = {path.name for path in (E2E / "giztest").glob("tikv.usage.*.giztest.yaml")}
        self.assertEqual(len(dedicated), 7)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            scenarios = root / "giztest"
            scenarios.mkdir()
            for name in dedicated | {"all.ping.giztest.yaml", "server.shared.giztest.yaml"}:
                (scenarios / name).write_text("")
            # Execute the actual find/filter pipelines, without starting any SDK,
            # compiled runner or Docker process.
            for language, name in [("Go", "run_standard_giztest"),
                                   ("JS/Flutter", "standard_sdk_giztest_files"),
                                   ("C", "run_c_giztest")]:
                with self.subTest(language=language):
                    body = shell_function(source, name)
                    command = re.search(r"^[ \t]*find[^\n]*", body.replace("\\\n", " "), re.MULTILINE)
                    self.assertIsNotNone(command)
                    result = subprocess.run(
                        ["bash", "-c", filter_function + '\nscript_dir="$1"\ngiztest_dir="$1/giztest"\n' + command.group(),
                         "scenario-discovery", str(root)],
                        check=True, capture_output=True, text=True, timeout=5)
                    selected = {Path(path).name for path in result.stdout.splitlines()}
                    self.assertEqual(selected, {"all.ping.giztest.yaml", "server.shared.giztest.yaml"})
            for name in ["run_js_giztest", "run_flutter_giztest"]:
                self.assertIn("done < <(standard_sdk_giztest_files)", shell_function(source, name))

    def test_dedicated_runner_still_selects_exactly_seven_documents(self):
        source = MULTI.read_text()
        match = re.search(r'if \[\[ "\$tikv_usage" == 1 \]\]; then\n  scenarios=\((.*?)\n  \)', source, re.DOTALL)
        self.assertIsNotNone(match)
        selected = re.findall(r'"\$giztest_dir/([^"\n]+)"', match.group(1))
        expected = {path.name for path in (E2E / "giztest").glob("tikv.usage.*.giztest.yaml")}
        self.assertEqual(len(selected), 7)
        self.assertEqual(set(selected), expected)
        self.assertIn('compose run --rm -T giztest "${scenarios[@]}"', source)


class MultiServerPhaseFailureTests(TestCase):
    provider_free = [
        "sfu.presence.cross-server.giztest.yaml",
        "sfu.friend.cross-server.audio-bytes.giztest.yaml",
        "sfu.friend-group.remove-readd.audio-bytes.giztest.yaml",
        "sfu.friend-group.reconnect-readd.audio-bytes.giztest.yaml",
        "sfu.friend-group.rapid-readd.audio-bytes.giztest.yaml",
        "sfu.friend-group.mixed-server-members.audio-bytes.giztest.yaml",
        "sfu.friend.delete-readd.audio-bytes.giztest.yaml",
        "sfu.friend-group.delete-recreate.audio-bytes.giztest.yaml",
        "sfu.workspace.isolation-switch.audio-bytes.giztest.yaml",
        "sfu.workspace.stop-reconnect.audio-bytes.giztest.yaml",
        "sfu.friend-group.members-in-room.giztest.yaml",
    ]

    def run_phases(self, suite, go_exit=0, giztest_exit=0, provider=0):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        (root / "e2e-run.json").write_text("{}")
        source = MULTI.read_text()
        # Execute the actual phase dispatch and EXIT cleanup. Compose is a
        # recorder/stub; builds and service startup are outside this excerpt.
        setup = '''set -euo pipefail
report_dir="$1"
suite="$2"
go_exit="$3"
giztest_exit="$4"
GIZCLAW_E2E_SFU_PROVIDER="$5"
script_dir="$6"
calls="$report_dir/calls"
sfu_case= probe_dir=
giztest_dir=tests/gizclaw-e2e/giztest
native_only=0 tikv_contention=0 tikv_usage=0
GIZCLAW_E2E_SOCIAL_PING_ONLY=0
GIZCLAW_E2E_SFU_EVIDENCE_DIR= GIZCLAW_E2E_KEEP_BINARIES=1
case "$suite" in
  native-tikv-lag) native_only=1 ;;
  tikv-contention) tikv_contention=1 ;;
  tikv-usage-giztest) tikv_usage=1 ;;
  social-ping) GIZCLAW_E2E_SOCIAL_PING_ONLY=1 ;;
esac
compose() {
  printf '%s\\n' "$*" >> "$calls"
  case "$*" in
    'run --rm -T tests '*) printf 'Go fixture output\\n'; return "$go_exit" ;;
    'run --rm -T giztest '*) return "$giztest_exit" ;;
    exec* | logs* | ps* | down*) return 0 ;;
    *) printf 'unexpected Compose call: %s\\n' "$*" >&2; return 99 ;;
  esac
}
docker() { printf 'unexpected Docker call\\n' >&2; return 99; }
'''
        script = (setup + shell_function(source, "cleanup") + '\ntrap cleanup EXIT\n'
                  + source[source.index("go_status=0\n"):])
        result = subprocess.run(
            ["bash", "-c", script, "multi-server-phases", str(root), suite,
             str(go_exit), str(giztest_exit), str(provider), str(E2E)],
            capture_output=True, text=True, timeout=10)
        calls = (root / "calls").read_text().splitlines()
        self.assertNotIn("unexpected", result.stderr)
        self.assertEqual(calls[-1], "down --volumes --remove-orphans")
        self.assertEqual(json.loads((root / "e2e-run.json").read_text())["exit_status"], result.returncode)
        return result, calls, root

    def test_full_runs_all_sfu_documents_and_preserves_phase_failures(self):
        for go_exit, giztest_exit, expected_exit in [(0, 0, 0), (7, 0, 7), (0, 9, 1), (7, 9, 1)]:
            with self.subTest(go_exit=go_exit, giztest_exit=giztest_exit):
                result, calls, root = self.run_phases("full", go_exit, giztest_exit)
                self.assertEqual(result.returncode, expected_exit, result.stderr)
                self.assertEqual(calls[0], "run --rm -T tests -test.run . -test.parallel 1 -test.timeout=10m")
                self.assertEqual((root / "go-tests.log").read_text(), "Go fixture output\n")
                self.assertEqual([call for call in calls if call.startswith("run --rm -T giztest ")],
                                 ["run --rm -T giztest " + " ".join(
                                     "tests/gizclaw-e2e/giztest/" + name for name in self.provider_free)])
                if go_exit:
                    self.assertIn("Go multi-server tests failed (exit status 7)", result.stderr)
                if expected_exit:
                    self.assertNotIn("==> multi-server e2e completed", result.stdout)

    def test_full_keeps_provider_scenarios_after_go_failure(self):
        result, calls, _ = self.run_phases("full", go_exit=7, provider=1)
        self.assertEqual(result.returncode, 7, result.stderr)
        invocation = next(call for call in calls if call.startswith("run --rm -T giztest "))
        selected = [Path(path).name for path in invocation.split()[4:]]
        self.assertEqual(selected, self.provider_free + [
            "sfu.friend.cross-server.giztest.yaml",
            "sfu.friend-group.cross-server.giztest.yaml",
            "sfu.workspace.switch.giztest.yaml",
        ])

    def test_dedicated_go_suites_stop_immediately_on_failure(self):
        for suite, pattern in [
            ("native-tikv-lag", "^TestTiKVLearnerLagThroughBothEdges$"),
            ("tikv-contention", "^TestConcurrentServersResolveResidualSecondary$"),
            ("social-ping", "^TestSocialPingCrossServer$"),
        ]:
            with self.subTest(suite=suite):
                result, calls, _ = self.run_phases(suite, go_exit=7)
                self.assertEqual(result.returncode, 7, result.stderr)
                self.assertEqual(calls[0], f"run --rm -T tests -test.run {pattern} -test.parallel 1 -test.timeout=10m")
                self.assertFalse(any(call.startswith("exec ") or call.startswith("run --rm -T giztest ") for call in calls))
                self.assertNotIn("continuing with SFU", result.stderr)

    def test_dedicated_usage_failure_does_not_run_sfu(self):
        result, calls, root = self.run_phases("tikv-usage-giztest", giztest_exit=9)
        self.assertEqual(result.returncode, 1, result.stderr)
        invocations = [call for call in calls if call.startswith("run ")]
        self.assertEqual(len(invocations), 1)
        selected = [Path(path).name for path in invocations[0].split()[4:]]
        self.assertEqual(len(selected), 7)
        self.assertEqual(set(selected), {path.name for path in (E2E / "giztest").glob("tikv.usage.*.giztest.yaml")})
        self.assertFalse((root / "go-tests.log").exists())


class TiKVRegionalReadFixtureTests(TestCase):
    def test_generated_configuration_uses_labels_without_a_delay(self):
        with tempfile.TemporaryDirectory() as temporary:
            config = Path(temporary) / "config.yaml"
            config.write_text("storage:\n  memory:\n    kind: memory\nstores:\n  peers:\n    kind: keyvalue\n    storage: memory\n")
            environment = dict(os.environ, GIZCLAW_E2E_TIKV_PD="pd:2379", GIZCLAW_E2E_TIKV_STALE_DELAY="ignored-legacy-value")
            subprocess.run(["bash", str(E2E / "docker/setup/configure-tikv.sh"), str(config)],
                           env=environment, check=True, capture_output=True, text=True, timeout=5)
            text = config.read_text()
            self.assertIn("pd_addresses: [pd:2379]", text)
            self.assertIn("read_labels:\n      region: local", text)
            self.assertIn("storage: shared-tikv", text)
            self.assertNotIn("stale_read_delay", text)
            self.assertNotIn("ignored-legacy-value", text)
        for path in [MULTI, E2E / "docker/docker-compose.yaml", E2E / "docker/docker-compose.multi-server.yaml"]:
            self.assertNotIn("GIZCLAW_E2E_TIKV_STALE_DELAY", path.read_text())

    def test_usage_probe_wraps_clock_using_labels(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary)
            subprocess.run([sys.executable, "-B", str(E2E / "setup/prepare-tikv-usage-probe.py"),
                            str(E2E.parents[1]), str(output)],
                           check=True, capture_output=True, text=True, timeout=30)
            source = (output / "storage-tikv.go").read_text()
            self.assertIn("gate.WrapReadClock(store.GetOracle(), len(cfg.ReadLabels) > 0)", source)
            self.assertNotIn("cfg.StaleReadDelay", source)
            overlay = json.loads((output / "overlay.local.json").read_text())
            self.assertEqual(len(overlay["Replace"]), 3)


class MultiServerSourceManifestTests(TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.repository = self.root / "source tree"
        self.repository.mkdir()
        self.report = self.root / "e2e-run.json"
        self.binary = self.root / "fixture-binary"
        self.binary.write_bytes(b"fixture binary")
        source = MULTI.read_text()
        marker = 'python3 -B - "$repo_root" "$report_dir/e2e-run.json"'
        self.script = source[source.index(marker):].split("<<'PY'\n", 1)[1].split("\nPY\n", 1)[0]

    def manifest(self, entries):
        def git_output(args, **kwargs):
            if args[3:] == ["diff", "HEAD"]:
                return b"fixture diff"
            if args[3:] == ["ls-files", "--others", "--exclude-standard", "-z"]:
                return b"\0".join(os.fsencode(entry) for entry in entries) + b"\0"
            if args[3:] == ["rev-parse", "HEAD"]:
                return "fixture-revision\n"
            raise AssertionError(f"unexpected subprocess: {args}")

        read_bytes = Path.read_bytes

        def regular_only(path):
            # Fail immediately if a regression attempts to follow a link or
            # read a FIFO/directory, rather than allowing a test process to hang.
            self.assertTrue(stat.S_ISREG(path.lstat().st_mode), f"followed/read nonregular source: {path}")
            return read_bytes(path)

        argv = ["manifest", str(self.repository), str(self.report), "project",
                str(self.binary), str(self.binary), str(self.binary), "0", "tikv-usage-giztest"]
        with mock.patch.object(sys, "argv", argv), \
             mock.patch.object(subprocess, "check_output", side_effect=git_output), \
             mock.patch.object(Path, "read_bytes", regular_only):
            exec(compile(self.script, str(MULTI), "exec"), {})
        return json.loads(self.report.read_text())

    def test_links_are_hashed_and_recorded_without_following_targets(self):
        ordinary = self.repository / "ordinary.go"
        ordinary.write_bytes(b"actual source")
        external = self.root / "external"
        external.write_bytes(b"outside source snapshot")
        directory = self.root / "external-directory"
        directory.mkdir()
        links = {"file-link": str(external), "directory-link": str(directory), "dangling-link": "missing-target"}
        for name, target in links.items():
            (self.repository / name).symlink_to(target)
        entries = ["ordinary.go", *links]
        report = self.manifest(entries)
        self.assertEqual(report["source_untracked_symlinks"], links)
        self.assertEqual(report["source_untracked_sha256"]["ordinary.go"], hashlib.sha256(b"actual source").hexdigest())
        for name, target in links.items():
            self.assertEqual(report["source_untracked_sha256"][name], hashlib.sha256(b"symlink\0" + os.fsencode(target)).hexdigest())
        self.assertTrue(all(isinstance(value, str) and len(value) == 64 for value in report["source_untracked_sha256"].values()))
        external.write_bytes(b"external content changed")
        self.assertEqual(self.manifest(entries)["source_untracked_sha256"], report["source_untracked_sha256"])
        (self.repository / "dangling-link").unlink()
        (self.repository / "dangling-link").symlink_to("other-target")
        self.assertNotEqual(self.manifest(entries)["source_untracked_sha256"]["dangling-link"], report["source_untracked_sha256"]["dangling-link"])

    def test_directories_and_fifos_are_described_without_reading(self):
        (self.repository / "nested").mkdir()
        entries = ["nested/"]
        if hasattr(os, "mkfifo"):
            os.mkfifo(self.repository / "fifo")
            entries.append("fifo")
        report = self.manifest(entries)
        self.assertEqual(report["source_untracked_special"]["nested/"], stat.S_IFDIR)
        self.assertEqual(report["regional_read_strategy"], "native label-matching Learner reads with replica-read synchronization; no automatic Leader data fallback")
        self.assertNotIn("stale_delay", report)
        if "fifo" in entries:
            self.assertEqual(report["source_untracked_special"]["fifo"], stat.S_IFIFO)
        self.assertEqual(set(report["source_untracked_sha256"]), set(entries))
        self.assertEqual(report["source_untracked_symlinks"], {})


if __name__ == "__main__":
    main()
