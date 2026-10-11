#!/usr/bin/env python3
"""Exercise the TOS wire/cache boundary with local archives and a recorded Docker stub."""

import gzip
import hashlib
import http.server
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import threading
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "build"))
import container_archive
import container_archive_test as fixtures

spec = importlib.util.spec_from_file_location("image_loader", Path(__file__).with_name("load.py"))
loader = importlib.util.module_from_spec(spec)
spec.loader.exec_module(loader)


class LoaderTest(unittest.TestCase):
    def setUp(self):
        (ROOT / ".tmp").mkdir(exist_ok=True)
        self.work = tempfile.TemporaryDirectory(prefix="image-contract-", dir=ROOT / ".tmp")
        self.addCleanup(self.work.cleanup)
        self.directory = Path(self.work.name)
        self.origin = self.directory / "origin"
        self.cache = self.directory / "cache"
        self.bin = self.directory / "bin"
        self.bin.mkdir()
        self.source, self.key, self.reference = loader.source_identity("ubuntu", "amd64")
        self.payload_dir = self.origin / self.key / "amd64"
        self.payload_dir.mkdir(parents=True)
        fixture = fixtures.make_archive(self.directory, "mem0", "amd64")
        self.proof = container_archive.inspect_docker_archive(fixture, "gizclaw-mem0:v0.0.0-amd64", "amd64")
        self.archive = self.payload_dir / "image.tar.gz"
        self.archive.write_bytes(fixture.read_bytes())
        self.export = self.directory / "export.tar"
        self.export.write_bytes(gzip.decompress(fixture.read_bytes()))
        self.manifest = dict(schema_version=1, source=self.source, source_key=self.key, platform="linux/amd64",
                             reference=self.reference, archive_reference="gizclaw-mem0:v0.0.0-amd64",
                             image_id=self.proof["config_digest"], load_ids=self.proof["_load_ids"],
                             archive="image.tar.gz", size=self.archive.stat().st_size,
                             sha256=hashlib.sha256(self.archive.read_bytes()).hexdigest())
        self.write_manifest()
        self.log = self.directory / "docker.log"
        self.state = self.directory / "docker-state.json"
        stub = self.bin / "docker"
        stub.write_text('''#!/usr/bin/env python3
import io,json,os,sys,tarfile
from pathlib import Path
args=sys.argv[1:]
with open(os.environ['DOCKER_LOG'],'a') as log: log.write(json.dumps(args)+'\\n')
state_file=Path(os.environ['DOCKER_STATE'])
state=json.loads(state_file.read_text()) if state_file.exists() else {}
if args[:1]==['version']:
    version='1.'+os.environ.get('DOCKER_API_MINOR','54')
    print(version+' '+version)
elif args[:2]==['image','inspect']:
    ref=args[-1]
    if not state.get('loaded') or (ref==os.environ['LOCAL_REFERENCE'] and not state.get('tagged')):
        print('Error response from daemon: No such image: '+ref,file=sys.stderr);sys.exit(1)
    arch=os.environ.get('ACTUAL_ARCH','amd64')
    if '--platform' not in args: arch=os.environ.get('DAEMON_DEFAULT_ARCH',arch)
    print(json.dumps(dict(Id=os.environ['ACTUAL_ID'],Os='linux',Architecture=arch)))
elif args[:2]==['image','save']:
    # Docker save reports the requested tag after a local image is retagged.
    output=io.BytesIO()
    with tarfile.open(os.environ['SAVED_TAR']) as source,tarfile.open(fileobj=output,mode='w') as target:
        for member in source:
            data=source.extractfile(member).read() if member.isfile() else None
            if member.name=='manifest.json':
                images=json.loads(data)
                for image in images: image['RepoTags']=[args[-1]]
                data=json.dumps(images).encode()
                member.size=len(data)
            target.addfile(member,io.BytesIO(data) if data is not None else None)
    sys.stdout.buffer.write(output.getvalue())
elif args[:1]==['load']:
    state['loaded']=True;state_file.write_text(json.dumps(state));print('Loaded fixture image')
elif args[:1]==['tag']:
    state['tagged']=True;state_file.write_text(json.dumps(state))
else:
    print('forbidden/unexpected Docker command: '+repr(args),file=sys.stderr);sys.exit(99)
''')
        stub.chmod(0o755)
        requests = []
        directory = self.origin

        class Handler(http.server.SimpleHTTPRequestHandler):
            def __init__(self, *args, **kwargs):
                super().__init__(*args, directory=str(directory), **kwargs)

            def do_GET(self):
                requests.append(self.path)
                super().do_GET()

            def log_message(self, *args):
                pass

        self.requests = requests
        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.addCleanup(self.stop_server)
        self.env = os.environ | {
            "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
            "GIZCLAW_CONTAINER_IMAGE_BASE_URL": f"http://127.0.0.1:{self.server.server_port}",
            "GIZCLAW_CONTAINER_IMAGE_CACHE_DIR": str(self.cache),
            "DOCKER_LOG": str(self.log), "DOCKER_STATE": str(self.state),
            "LOCAL_REFERENCE": self.reference, "ACTUAL_ID": self.proof["config_digest"],
            "SAVED_TAR": str(self.export),
        }
        self.env.pop("GIZCLAW_CONTAINER_CACHE_ONLY", None)

    def stop_server(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)

    def write_manifest(self):
        (self.payload_dir / "manifest.json").write_text(json.dumps(self.manifest))

    def run_loader(self, success=True):
        result = subprocess.run([sys.executable, str(Path(__file__).with_name("load.py")), "load", "ubuntu", "amd64"],
                                env=self.env, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30)
        self.assertEqual(result.returncode == 0, success, result.stderr)
        if success:
            self.assertEqual(result.stdout.strip(), self.reference)
        else:
            self.assertIn("Registry fallback is disabled", result.stderr)
        return result

    def docker_calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_cold_and_cached_loading(self):
        self.run_loader()
        self.run_loader()
        calls = self.docker_calls()
        self.assertEqual(sum(call[0] == "load" for call in calls), 1)
        self.assertEqual(sum(call[0] == "tag" for call in calls), 1)
        self.assertEqual(sum(path.endswith("image.tar.gz") for path in self.requests), 1)
        self.assertEqual(sum(path.endswith("manifest.json") for path in self.requests), 2)
        self.assertTrue(all(call[0] in ("image", "load", "tag", "version") for call in calls))

    def test_shell_preflight_exports_loaded_reference(self):
        result = subprocess.run(["/bin/bash", "-c", 'set -euo pipefail; source tests/docker-images/common.sh; '
                                 'gizclaw_image_env amd64; gizclaw_require_images ubuntu; '
                                 'printf "%s\\n" "$GIZCLAW_E2E_UBUNTU_IMAGE"'], cwd=ROOT, env=self.env,
                                text=True, capture_output=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), self.reference)

    def test_requested_platform_overrides_daemon_default(self):
        self.env["DAEMON_DEFAULT_ARCH"] = "arm64"
        self.run_loader()
        calls = [call for call in self.docker_calls() if call[:2] == ["image", "inspect"]]
        self.assertTrue(calls and all("--platform" in call for call in calls))

    def test_legacy_daemon_checks_exact_config_without_platform_flags(self):
        self.env["DOCKER_API_MINOR"] = "47"
        self.env["ACTUAL_ID"] = "sha256:" + "f" * 64
        self.run_loader()
        calls = [call for call in self.docker_calls() if call[:2] in (["image", "inspect"], ["image", "save"])]
        self.assertTrue(calls and all("--platform" not in call for call in calls))

    def test_new_containerd_id_requires_config_proof(self):
        self.env["ACTUAL_ID"] = "sha256:" + "f" * 64
        self.run_loader()
        self.run_loader()
        self.assertTrue(any(call[:2] == ["image", "save"] for call in self.docker_calls()))
        cached = json.loads((self.cache / self.key / "amd64/manifest.json").read_text())
        self.assertEqual(cached["image_id"], self.proof["config_digest"])
        self.assertNotIn(self.env["ACTUAL_ID"], cached["load_ids"])

    def test_sdk_and_empty_config_archives(self):
        for layout in ("sdk", "containerd-empty"):
            with self.subTest(layout=layout):
                reference = "docker.io/" + self.reference
                fixture = fixtures.make_archive(self.directory, "mem0", "amd64", layout=layout,
                                                reference=reference, oci_reference=reference)
                proof = container_archive.inspect_docker_archive(fixture, self.reference, "amd64")
                self.archive.write_bytes(fixture.read_bytes())
                self.export.write_bytes(gzip.decompress(fixture.read_bytes()))
                self.manifest.update(archive_reference=reference, image_id=proof["config_digest"],
                                     load_ids=proof["_load_ids"], size=self.archive.stat().st_size,
                                     sha256=hashlib.sha256(self.archive.read_bytes()).hexdigest())
                self.write_manifest()
                self.env.update(ACTUAL_ID="sha256:" + "f" * 64,
                                GIZCLAW_CONTAINER_IMAGE_CACHE_DIR=str(self.directory / ("cache-" + layout)))
                self.state.unlink(missing_ok=True)
                self.run_loader()
                self.run_loader()

    def test_wrong_cached_config_fails_without_import(self):
        self.state.write_text(json.dumps(dict(loaded=True, tagged=True)))
        self.env["ACTUAL_ID"] = "sha256:" + "f" * 64
        other = fixtures.make_archive(self.directory, "mem0", "arm64")
        self.export.write_bytes(gzip.decompress(other.read_bytes()))
        self.run_loader(False)
        self.assertFalse(any(call[0] == "load" for call in self.docker_calls()))

    def test_missing_origin_fails_before_docker(self):
        (self.payload_dir / "manifest.json").unlink()
        self.run_loader(False)
        self.assertEqual(self.docker_calls(), [])

    def test_source_platform_and_identity_mismatches(self):
        for field, value in (("source", self.source + "wrong"), ("source_key", "0" * 64),
                             ("platform", "linux/arm64"), ("reference", "other:tag"),
                             ("image_id", "sha256:" + "f" * 64), ("load_ids", ["sha256:" + "f" * 64])):
            with self.subTest(field=field):
                original = self.manifest[field]
                self.manifest[field] = value
                self.write_manifest()
                # Isolated cache per failure prevents another result becoming authority.
                self.env["GIZCLAW_CONTAINER_IMAGE_CACHE_DIR"] = str(self.directory / ("bad-" + field))
                self.run_loader(False)
                self.manifest[field] = original
                self.assertEqual(self.docker_calls(), [])

    def test_bad_archive_digest_stops_before_docker(self):
        self.archive.write_bytes(self.archive.read_bytes()[:-1] + b"x")
        self.run_loader(False)
        self.assertEqual(self.docker_calls(), [])

    def test_self_consistent_outer_hash_cannot_hide_corrupt_gzip(self):
        data = bytearray(self.archive.read_bytes())
        data[-8] ^= 1
        self.archive.write_bytes(data)
        self.manifest["sha256"] = hashlib.sha256(data).hexdigest()
        self.write_manifest()
        self.run_loader(False)
        self.assertEqual(self.docker_calls(), [])

    def test_corrupt_cache_is_not_silently_repaired(self):
        self.run_loader()
        archive = self.cache / self.key / "amd64/image.tar.gz"
        archive.write_bytes(archive.read_bytes()[:-1] + b"x")
        self.log.write_text("")
        self.run_loader(False)
        self.assertEqual(self.docker_calls(), [])

    def test_cached_manifest_must_match_authenticated_origin(self):
        self.run_loader()
        cached = self.cache / self.key / "amd64/manifest.json"
        cached.write_text(cached.read_text() + " ")
        self.log.write_text("")
        self.run_loader(False)
        self.assertEqual(self.docker_calls(), [])

    def test_offline_cache_only_mode(self):
        self.env["GIZCLAW_CONTAINER_CACHE_ONLY"] = "1"
        self.run_loader(False)
        self.assertEqual(self.requests, [])
        self.env.pop("GIZCLAW_CONTAINER_CACHE_ONLY")
        self.run_loader()
        count = len(self.requests)
        self.env["GIZCLAW_CONTAINER_CACHE_ONLY"] = "1"
        self.run_loader()
        self.assertEqual(len(self.requests), count)

    def test_bad_architecture_stops_after_import(self):
        self.env["ACTUAL_ARCH"] = "arm64"
        self.run_loader(False)
        self.assertFalse(any(call[0] == "tag" for call in self.docker_calls()))


class BuildPolicyTest(unittest.TestCase):
    def test_checked_in_compose_inventory(self):
        base = "tests/gizclaw-e2e/docker/"
        cases = [
            [base + "docker-compose.yaml"],
            [base + "docker-compose.admission.yaml"],
            [base + "docker-compose.multi-server.yaml"],
            [base + "compose.memory.yaml"],
            [base + "compose.monitor.yaml"],
            [base + "compose.quota.yaml"],
            [base + "docker-compose.yaml", base + "docker-compose.gateway-relay.yaml"],
            [base + "docker-compose.yaml", base + "docker-compose.observability.yaml"],
            [base + "docker-compose.yaml", base + "docker-compose.volc-log.yaml"],
            ["tests/giznet-e2e/docker-compose.coturn.yaml"],
            ["tests/locomo-e2e/docker-compose.yaml"],
        ]
        tracked = subprocess.check_output(["git", "ls-files", "--", "tests"], cwd=ROOT, text=True).splitlines()
        self.assertEqual({name for name in tracked if re.search(r"compose.*\.ya?ml$", name)},
                         {name for files in cases for name in files})
        with tempfile.TemporaryDirectory(prefix="compose-policy-") as work:
            directory = Path(work)
            empty_env = directory / "fixture.env"
            empty_env.touch()
            (directory / "runtime.env").touch()
            env = os.environ | {
                "DOCKER_DEFAULT_PLATFORM": "linux/amd64",
                "GIZCLAW_E2E_CREDENTIAL_FILE": str(empty_env),
                "GIZCLAW_E2E_GATEWAY_RELAY_ENV_FILE": str(empty_env),
                "GIZCLAW_E2E_DOCKER_BASE_IMAGE": "gizclaw-go:policy-fixture",
                "GIZCLAW_E2E_MEM0_BASE_IMAGE": "gizclaw-mem0:policy-fixture",
                "GIZCLAW_LOCOMO_E2E_MEM0_BASE_IMAGE": "gizclaw-mem0:policy-fixture",
                "GIZCLAW_MONITOR_IMAGE": "gizclaw-monitor:policy-fixture",
                "GIZCLAW_QUOTA_IMAGE": "gizclaw-quota:policy-fixture",
                "GIZCLAW_E2E_GATEWAY_CAPACITY_IMAGE": "gizclaw-capacity:policy-fixture",
                "GIZCLAW_E2E_BUILT_IMAGES": "gizclaw-monitor:policy-fixture\ngizclaw-quota:policy-fixture\ngizclaw-capacity:policy-fixture",
            }
            for name in loader.catalog():
                env["GIZCLAW_E2E_" + name.upper().replace("-", "_") + "_IMAGE"] = loader.source_identity(name, "amd64")[2]
            for files in cases:
                for filename in files:
                    for variable in re.findall(r"\$\{([A-Z][A-Z0-9_]*):\?[^}]*\}", (ROOT / filename).read_text()):
                        if variable in env:
                            continue
                        value = "fixture"
                        if variable.endswith("_PORT"):
                            value = "23000" if "MAX" not in variable else "23020"
                        elif variable.endswith("_SUBNET"):
                            value = "172.30.250.0/24"
                        elif variable.endswith("_IP"):
                            value = "172.30.250.10"
                        elif variable.endswith(("_HOST", "_BIND", "_ADDRESS")):
                            value = "127.0.0.1"
                        elif variable.endswith(("_HOME", "_DIR", "_STATE", "_REPORTS")):
                            value = work
                        env[variable] = value
            for files in cases:
                command = ["docker", "compose"]
                for filename in files:
                    command.extend(["-f", str(ROOT / filename)])
                result = subprocess.run(command + ["config", "--format", "json"], cwd=ROOT, env=env,
                                        text=True, capture_output=True, timeout=30)
                with self.subTest(files=files):
                    self.assertEqual(result.returncode, 0, result.stderr)
                    with mock.patch.dict(os.environ, env, clear=True), mock.patch.object(
                            loader, "docker", return_value=subprocess.CompletedProcess([], 0, "linux/amd64\n", "")):
                        loader.compose_dependencies(container_archive.parse_json(result.stdout), "amd64")

    def test_declared_services_and_local_builds(self):
        reference = loader.source_identity("redis", "amd64")[2]
        document = {"services": {"redis": {"image": reference, "pull_policy": "never"}}}
        self.assertEqual(loader.compose_dependencies(document, "amd64"), ["redis"])
        for service in ({"image": reference}, {"image": "redis:latest", "pull_policy": "never"},
                        {"image": "unknown:local", "pull_policy": "never"},
                        {"image": reference, "pull_policy": "never", "platform": "linux/arm64"},
                        {"image": reference, "pull_policy": "never", "build": {"pull": True}}):
            with self.subTest(service=service), self.assertRaises(ValueError):
                loader.compose_dependencies({"services": {"bad": service}}, "amd64")

    def test_external_bases_frontends_and_copy_sources_are_rejected(self):
        (ROOT / ".tmp").mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(dir=ROOT / ".tmp") as work:
            directory = Path(work)
            dockerfile = directory / "Dockerfile"
            build = dict(context=work, dockerfile="Dockerfile")
            for content in ("FROM ubuntu:24.04\n", "# syntax=docker/dockerfile:1\nFROM scratch\n",
                            "FROM scratch\nCOPY --from=registry/other:tag /file /file\n",
                            "FROM scratch\nRUN --mount=type=bind,from=registry/other:tag echo no\n"):
                dockerfile.write_text(content)
                with self.subTest(content=content), self.assertRaises(ValueError):
                    loader.validate_build_bases(build, "amd64")
            dockerfile.write_text("FROM scratch AS base\nFROM base AS final\nCOPY --from=base /file /file\n")
            loader.validate_build_bases(build, "amd64")

    def test_pinned_local_base_is_checked_before_building(self):
        reference = loader.source_identity("ubuntu", "amd64")[2]
        build = dict(context=str(ROOT / "build"), dockerfile="gizclaw/Dockerfile.cn.base", args={"BASE_IMAGE": reference})
        with mock.patch.object(loader, "docker", return_value=subprocess.CompletedProcess([], 0, "linux/amd64\n", "")) as command:
            loader.validate_build_bases(build, "amd64")
            self.assertTrue(command.called)
        with mock.patch.object(loader, "docker", return_value=subprocess.CompletedProcess([], 0, "linux/arm64\n", "")):
            with self.assertRaises(ValueError):
                loader.validate_build_bases(build, "amd64")


if __name__ == "__main__":
    unittest.main()
