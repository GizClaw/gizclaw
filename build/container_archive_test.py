#!/usr/bin/env python3
"""Offline fixtures and regression tests for delivered Docker archives."""

import copy
import gzip
import hashlib
import io
import json
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest

import container_archive as contract

TAG = "v0.0.0"
COMMIT = "1" * 40


def make_archive(directory, component="runtime", arch="amd64", layout="classic",
                 mutate_config=None, mutate_manifest=None, mutate_index=None, bad_layer=False, unsafe=False):
    expected = contract.identity(component, TAG, COMMIT, arch)
    path = directory / expected["name"]
    layer = b"layer contents for the archive integrity fixture"
    layer_digest = hashlib.sha256(layer).hexdigest()
    labels = {
        "org.opencontainers.image.version": TAG[1:],
        "org.opencontainers.image.revision": COMMIT,
        "org.opencontainers.image.source": "https://github.com/GizClaw/gizclaw",
        "org.opencontainers.image.base.digest":
        "sha256:008173c23f95b170204355c12626cb5a965d779a7e1283b09e9cffbb1bf33ca3",
    }
    config = {"os": "linux", "architecture": arch,
              "config": {"Labels": labels, "User": "10001:10001", "Entrypoint":
                         ["/usr/local/bin/gizclaw-entrypoint"] if component == "runtime"
                         else ["python", "-m", "gizclaw_mem0"]},
              "rootfs": {"type": "layers", "diff_ids": ["sha256:" + layer_digest]}}
    if mutate_config:
        mutate_config(config)
    config_bytes = json.dumps(config).encode()
    config_digest = hashlib.sha256(config_bytes).hexdigest()
    config_name = config_digest + ".json" if layout == "classic" else "blobs/sha256/" + config_digest
    layer_bytes = layer if layout == "classic" else gzip.compress(layer, mtime=0)
    layer_blob_digest = hashlib.sha256(layer_bytes).hexdigest()
    layer_name = "layer/layer.tar" if layout == "classic" else "blobs/sha256/" + layer_blob_digest
    manifest = [{"Config": config_name, "Layers": [layer_name], "RepoTags": [expected["image_reference"]]}]
    if mutate_manifest:
        mutate_manifest(manifest)
    with path.open("wb") as output:
        with gzip.GzipFile(fileobj=output, mode="wb", mtime=0, filename="") as compressed:
            with tarfile.open(fileobj=compressed, mode="w") as archive:
                members = [(config_name, config_bytes), (layer_name, layer_bytes + b"tampered" if bad_layer else layer_bytes),
                           ("manifest.json", json.dumps(manifest).encode())]
                if layout == "containerd":
                    oci_manifest = json.dumps({"schemaVersion": 2,
                                               "config": {"digest": "sha256:" + config_digest, "size": len(config_bytes)},
                                               "layers": [{"digest": "sha256:" + layer_blob_digest, "size": len(layer_bytes)}]}).encode()
                    oci_digest = hashlib.sha256(oci_manifest).hexdigest()
                    index = {"schemaVersion": 2, "manifests": [{"digest": "sha256:" + oci_digest,
                             "size": len(oci_manifest), "annotations": {
                                 "io.containerd.image.name": "docker.io/library/" + expected["image_reference"]}}]}
                    if mutate_index:
                        mutate_index(index)
                    members.extend([("blobs/sha256/" + oci_digest, oci_manifest), ("index.json", json.dumps(index).encode())])
                if unsafe:
                    members.append(("../unsafe", b"unsafe"))
                for name, data in members:
                    member = tarfile.TarInfo(name)
                    member.size = len(data)
                    archive.addfile(member, io.BytesIO(data))
    return path


class ArchiveTest(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory(prefix="gizclaw-container-test-")
        self.addCleanup(self.work.cleanup)
        self.directory = Path(self.work.name)

    def inspect(self, **kwargs):
        component = kwargs.get("component", "runtime")
        arch = kwargs.get("arch", "amd64")
        return contract.inspect_archive(make_archive(self.directory, **kwargs), component, TAG, COMMIT, arch)

    def test_classic_and_containerd_layouts(self):
        for layout in ("classic", "containerd"):
            for component in ("runtime", "mem0"):
                for arch in ("amd64", "arm64"):
                    with self.subTest(layout=layout, component=component, arch=arch):
                        entry = self.inspect(layout=layout, component=component, arch=arch)
                        self.assertEqual(entry["image_reference"], f"gizclaw-{component}:{TAG}-{arch}")
                        self.assertNotEqual(entry["sha256"], entry["image_id"].removeprefix("sha256:"))

    def test_wrong_config_identity(self):
        mutations = [lambda c: c.update(architecture="arm64"),
                     lambda c: c["config"].update(User="0"),
                     lambda c: c["config"].update(Entrypoint=["sh"]),
                     lambda c: c["config"]["Labels"].update({"org.opencontainers.image.version": "9.9.9"}),
                     lambda c: c["config"]["Labels"].update({"org.opencontainers.image.revision": "2" * 40}),
                     lambda c: c["config"]["Labels"].update({"org.opencontainers.image.base.digest": "sha256:wrong"})]
        for mutation in mutations:
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                self.inspect(mutate_config=mutation)

    def test_docker_storage_image_ids(self):
        path = make_archive(self.directory, layout="containerd")
        classic = contract.inspect_archive(path, "runtime", TAG, COMMIT, "amd64")
        containerd = contract.inspect_archive(path, "runtime", TAG, COMMIT, "amd64", classic["archive_manifest_digest"])
        self.assertEqual(classic["image_id"], classic["config_digest"])
        self.assertEqual(containerd["image_id"], containerd["archive_manifest_digest"])
        self.assertNotEqual(containerd["image_id"], containerd["config_digest"])
        with self.assertRaises(ValueError):
            contract.inspect_archive(path, "runtime", TAG, COMMIT, "amd64", "sha256:" + "0" * 64)

    def test_wrong_manifest_or_layer(self):
        mutations = [lambda m: m.append(copy.deepcopy(m[0])),
                     lambda m: m[0].update(RepoTags=["other:tag"]),
                     lambda m: m[0].update(Config="missing.json"),
                     lambda m: m[0].update(Layers=["missing.tar"]),
                     lambda m: m[0].update(Layers=m[0]["Layers"] * 2)]
        for mutation in mutations:
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                self.inspect(mutate_manifest=mutation)
        with self.assertRaises(ValueError):
            self.inspect(bad_layer=True)
        with self.assertRaises(ValueError):
            self.inspect(unsafe=True)

    def test_corrupt_gzip(self):
        path = make_archive(self.directory)
        data = bytearray(path.read_bytes())
        data[-8] ^= 1
        path.write_bytes(data)
        with self.assertRaises((OSError, ValueError)):
            contract.inspect_archive(path, "runtime", TAG, COMMIT, "amd64")

    def test_oci_index_conflicts(self):
        mutations = [lambda i: i["manifests"].append(copy.deepcopy(i["manifests"][0])),
                     lambda i: i["manifests"][0].update(size=1),
                     lambda i: i["manifests"][0].update(digest="sha256:" + "0" * 64),
                     lambda i: i["manifests"][0]["annotations"].update({"io.containerd.image.name": "other:tag"})]
        for mutation in mutations:
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                self.inspect(layout="containerd", mutate_index=mutation)

    def test_duplicate_json_keys(self):
        with self.assertRaises(ValueError):
            contract.parse_json('{"image_id":"one","image_id":"two"}')

    def test_entry_keys_and_identity(self):
        entry = self.inspect(component="mem0")
        contract.validate_entry(entry, TAG, COMMIT)
        for key, value in (("image_id", "sha256:short"), ("config_digest", "sha256:short"),
                           ("size", True), ("sha256", "z" * 64),
                           ("name", "../other.tar.gz"), ("format", "oci"), ("reference", "registry:tag"),
                           ("source_commit", "2" * 40), ("component", "other")):
            with self.subTest(key=key), self.assertRaises(ValueError):
                contract.validate_entry(entry | {key: value}, TAG, COMMIT)


def fixtures(directory, tag, commit, binary_digest):
    global TAG, COMMIT
    TAG, COMMIT = tag, commit
    images = []
    for component in ("mem0", "runtime"):
        for arch in ("amd64", "arm64"):
            path = make_archive(directory, component, arch)
            entry = contract.inspect_archive(path, component, tag, commit, arch)
            if component == "runtime":
                entry["binary_sha256"] = binary_digest
            images.append(entry)
    (directory / "container-image.json").write_text(json.dumps({
        "schema_version": 2, "tag": tag, "version": tag[1:], "source_commit": commit, "images": images,
    }, indent=2) + "\n")


if __name__ == "__main__":
    if len(sys.argv) == 6 and sys.argv[1] == "fixtures":
        fixtures(Path(sys.argv[2]), *sys.argv[3:])
    else:
        unittest.main()
