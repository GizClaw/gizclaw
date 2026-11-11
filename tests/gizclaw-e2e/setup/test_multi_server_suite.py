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
        if "fifo" in entries:
            self.assertEqual(report["source_untracked_special"]["fifo"], stat.S_IFIFO)
        self.assertEqual(set(report["source_untracked_sha256"]), set(entries))
        self.assertEqual(report["source_untracked_symlinks"], {})


if __name__ == "__main__":
    main()
