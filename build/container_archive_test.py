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
                 mutate_config=None, mutate_manifest=None, mutate_index=None, bad_layer=False, unsafe=False,
                 attestation_case=None, reference=None, oci_reference=None, bad_config_name=False):
    expected = contract.identity(component, TAG, COMMIT, arch)
    if reference is not None:
        expected["image_reference"] = reference
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
    config_name = (config_digest + ".json" if layout == "classic" else
                   "sha256:" + config_digest if layout == "sdk" else "blobs/sha256/" + config_digest)
    if bad_config_name:
        config_name = config_name.replace(config_digest, "0" * 64)
    layer_bytes = layer if layout == "classic" else gzip.compress(layer, mtime=0)
    layer_blob_digest = hashlib.sha256(layer_bytes).hexdigest()
    layer_name = ("layer/layer.tar" if layout == "classic" else
                  layer_blob_digest + ".tar.gz" if layout == "sdk" else "blobs/sha256/" + layer_blob_digest)
    manifest = [{"Config": config_name, "Layers": [layer_name], "RepoTags": [expected["image_reference"]]}]
    if mutate_manifest:
        mutate_manifest(manifest)
    with path.open("wb") as output:
        with gzip.GzipFile(fileobj=output, mode="wb", mtime=0, filename="") as compressed:
            with tarfile.open(fileobj=compressed, mode="w") as archive:
                members = [(config_name, config_bytes), (layer_name, layer_bytes + b"tampered" if bad_layer else layer_bytes),
                           ("manifest.json", json.dumps(manifest).encode())]
                if layout.startswith("containerd"):
                    oci_manifest = json.dumps({"schemaVersion": 2,
                                               "config": {"digest": "sha256:" + config_digest, "size": len(config_bytes)},
                                               "layers": [{"digest": "sha256:" + layer_blob_digest, "size": len(layer_bytes)}]}).encode()
                    oci_digest = hashlib.sha256(oci_manifest).hexdigest()
                    index = {"schemaVersion": 2, "manifests": [{"digest": "sha256:" + oci_digest,
                             "size": len(oci_manifest), "annotations": {
                                 "io.containerd.image.name": oci_reference or "docker.io/library/" + expected["image_reference"]}}]}
                    if layout in ("containerd-index", "containerd-referrer", "containerd-empty"):
                        statement = {"_type": "https://in-toto.io/Statement/v1", "predicateType": "https://slsa.dev/provenance/v1",
                                     "subject": [{"name": "image", "digest": {"sha256": oci_digest}}]}
                        if attestation_case == "subject":
                            statement["subject"][0]["digest"]["sha256"] = "0" * 64
                        statement_bytes = json.dumps(statement).encode()
                        statement_digest = hashlib.sha256(statement_bytes).hexdigest()
                        att_config = {"os": "unknown", "architecture": "unknown",
                                      "rootfs": {"type": "layers", "diff_ids": ["sha256:" + statement_digest]}}
                        if attestation_case == "config":
                            att_config["architecture"] = arch
                        att_config_bytes = b"{}" if layout == "containerd-empty" else json.dumps(att_config).encode()
                        if layout == "containerd-empty" and attestation_case == "nonempty":
                            att_config_bytes = b'{"unexpected":true}'
                        att_config_digest = hashlib.sha256(att_config_bytes).hexdigest()
                        att_node = {"schemaVersion": 2,
                                       "config": {"digest": "sha256:" + att_config_digest, "size": len(att_config_bytes)},
                                       "layers": [{"digest": "sha256:" + statement_digest, "size": len(statement_bytes),
                                                   "mediaType": "application/vnd.in-toto+json"}]}
                        if layout == "containerd-empty":
                            att_node["config"].update(mediaType="application/vnd.oci.empty.v1+json", data="e30=")
                            att_node["subject"] = {"digest": "sha256:" + oci_digest, "size": len(oci_manifest)}
                            if attestation_case == "missing-subject":
                                del att_node["subject"]
                            elif attestation_case == "subject-size":
                                att_node["subject"]["size"] += 1
                            elif attestation_case == "subject-digest":
                                att_node["subject"]["digest"] = "sha256:" + "0" * 64
                            elif attestation_case == "inline-config":
                                att_node["config"]["data"] = "bm90IGVtcHR5"
                        att_manifest = json.dumps(att_node).encode()
                        att_digest = hashlib.sha256(att_manifest).hexdigest()
                        att_descriptor = {"digest": "sha256:" + att_digest, "size": len(att_manifest),
                                          "platform": {"os": "unknown", "architecture": "unknown"},
                                          "annotations": {"vnd.docker.reference.type": "attestation-manifest",
                                                          "vnd.docker.reference.digest": "sha256:" + oci_digest}}
                        if attestation_case == "target":
                            att_descriptor["annotations"]["vnd.docker.reference.digest"] = "sha256:" + "0" * 64
                        if attestation_case == "platform":
                            att_descriptor["platform"]["architecture"] = arch
                        runnable = {"digest": "sha256:" + oci_digest, "size": len(oci_manifest),
                                    "platform": {"os": "linux", "architecture": arch}}
                        inner_index = json.dumps({"schemaVersion": 2, "manifests": [runnable, att_descriptor]}).encode()
                        inner_digest = hashlib.sha256(inner_index).hexdigest()
                        index["manifests"][0].update(digest="sha256:" + inner_digest, size=len(inner_index))
                        if layout in ("containerd-referrer", "containerd-empty"):
                            att_descriptor["annotations"] = {"io.containerd.manifest.subject": "sha256:" + oci_digest}
                            att_descriptor.pop("platform")
                            if attestation_case == "target":
                                att_descriptor["annotations"]["io.containerd.manifest.subject"] = "sha256:" + "0" * 64
                            index["manifests"].append(att_descriptor)
                            index["manifests"][0].update(digest="sha256:" + oci_digest, size=len(oci_manifest))
                        members.extend([("blobs/sha256/" + statement_digest, statement_bytes),
                                        ("blobs/sha256/" + att_config_digest, att_config_bytes),
                                        ("blobs/sha256/" + att_digest, att_manifest),
                                        ("blobs/sha256/" + inner_digest, inner_index)])
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
        for layout in ("classic", "sdk", "containerd", "containerd-index", "containerd-referrer", "containerd-empty"):
            for component in ("runtime", "mem0"):
                for arch in ("amd64", "arm64"):
                    with self.subTest(layout=layout, component=component, arch=arch):
                        entry = self.inspect(layout=layout, component=component, arch=arch)
                        self.assertEqual(entry["image_reference"], f"gizclaw-{component}:{TAG}-{arch}")
                        self.assertNotEqual(entry["sha256"], entry["image_id"].removeprefix("sha256:"))

    def test_docker_hub_reference_aliases(self):
        for name, aliases in (("gizclaw-runtime", ("gizclaw-runtime", "library/gizclaw-runtime",
                                                 "docker.io/gizclaw-runtime", "index.docker.io/gizclaw-runtime",
                                                 "docker.io/library/gizclaw-runtime", "index.docker.io/library/gizclaw-runtime")),
                              ("gizclaw-tos/cache", ("gizclaw-tos/cache", "docker.io/gizclaw-tos/cache",
                                                    "index.docker.io/gizclaw-tos/cache"))):
            expected = name + ":test-tag"
            for alias in aliases:
                for layout in ("classic", "sdk", "containerd"):
                    with self.subTest(name=name, alias=alias, layout=layout):
                        path = make_archive(self.directory, layout=layout, reference=alias + ":test-tag",
                                            oci_reference=alias + ":test-tag")
                        proof = contract.inspect_docker_archive(path, expected, "amd64")
                        with gzip.open(path, "rb") as stream:
                            contract.verify_saved_config(stream, expected, "amd64", proof["config_digest"])
            for wrong in (name + ":other-tag", "other/repository:test-tag", "example.com/" + expected,
                          "registry-1.docker.io/" + expected, "docker.io/other-namespace/" + name + ":test-tag"):
                with self.subTest(wrong=wrong), self.assertRaises(ValueError):
                    path = make_archive(self.directory, reference=wrong)
                    contract.inspect_docker_archive(path, expected, "amd64")
            with self.assertRaises(ValueError):
                path = make_archive(self.directory, layout="containerd", reference=expected,
                                    oci_reference="example.com/" + expected)
                contract.inspect_docker_archive(path, expected, "amd64")

    def test_sdk_config_filename_hash(self):
        path = make_archive(self.directory, layout="sdk")
        proof = contract.inspect_archive(path, "runtime", TAG, COMMIT, "amd64")
        with gzip.open(path, "rb") as stream:
            contract.verify_saved_config(stream, proof["image_reference"], "amd64", proof["config_digest"])
        with self.assertRaises(ValueError):
            self.inspect(layout="sdk", bad_config_name=True)

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

    def test_index_image_id_and_attestations(self):
        path = make_archive(self.directory, layout="containerd-index")
        config = contract.inspect_archive(path, "runtime", TAG, COMMIT, "amd64")
        actual = contract.inspect_archive(path, "runtime", TAG, COMMIT, "amd64", config["archive_index_digest"])
        self.assertEqual(actual["image_id"], actual["archive_index_digest"])
        self.assertNotEqual(actual["image_id"], actual["archive_manifest_digest"])
        for case in ("subject", "target", "platform", "config"):
            with self.subTest(case=case), self.assertRaises(ValueError):
                self.inspect(layout="containerd-index", attestation_case=case)

    def test_subject_bound_referrers(self):
        self.inspect(layout="containerd-referrer")
        for case in ("subject", "target", "config"):
            with self.subTest(case=case), self.assertRaises(ValueError):
                self.inspect(layout="containerd-referrer", attestation_case=case)

    def test_empty_config_subject_bound_referrers(self):
        self.inspect(layout="containerd-empty")
        for case in ("subject", "target", "nonempty", "missing-subject", "subject-size", "subject-digest", "inline-config"):
            with self.subTest(case=case), self.assertRaises(ValueError):
                self.inspect(layout="containerd-empty", attestation_case=case)
        self.inspect(layout="containerd-empty", mutate_index=lambda i: i["manifests"][1].pop("annotations"))

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

    def test_reexported_classic_config_proof(self):
        path = make_archive(self.directory)
        proof = contract.inspect_archive(path, "runtime", TAG, COMMIT, "amd64")
        raw = gzip.decompress(path.read_bytes())
        contract.verify_saved_config(io.BytesIO(raw), proof["image_reference"], "amd64", proof["config_digest"])
        for reference, arch, digest in (("other:tag", "amd64", proof["config_digest"]),
                                        (proof["image_reference"], "arm64", proof["config_digest"]),
                                        (proof["image_reference"], "amd64", "sha256:" + "0" * 64)):
            with self.subTest(reference=reference, arch=arch), self.assertRaises(ValueError):
                contract.verify_saved_config(io.BytesIO(raw), reference, arch, digest)

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
