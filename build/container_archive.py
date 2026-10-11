#!/usr/bin/env python3
"""Inspect Docker save archives and validate the Release container contract."""

import gzip
import hashlib
import json
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
import tarfile


def require(condition, message):
    if not condition:
        raise ValueError(message)


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, f"duplicate JSON key: {key}")
        result[key] = value
    return result


def parse_json(data):
    return json.loads(data, object_pairs_hook=unique_object)


def regular_file(path):
    require(path.is_file() and not path.is_symlink() and path.stat().st_size > 0,
            f"expected regular nonempty file: {path}")


def digest(stream):
    return hashlib.file_digest(stream, "sha256").hexdigest()


def identity(component, tag, commit, arch):
    require(re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag),
            "invalid canonical release tag")
    require(re.fullmatch(r"[0-9a-f]{40}", commit), "invalid source commit")
    require(component in ("runtime", "mem0") and arch in ("amd64", "arm64"),
            "invalid container component or architecture")
    return {
        "name": f"gizclaw-{component}_{tag[1:]}_linux_{arch}.tar.gz",
        "kind": "container-archive", "component": component,
        "os": "linux", "architecture": arch, "format": "docker-save+gzip",
        "version": tag[1:], "source_commit": commit,
        "image_reference": f"gizclaw-{component}:{tag}-{arch}",
    }


class HashingReader:
    """Hash compressed bytes while gzip streams their uncompressed identity."""

    def __init__(self, stream):
        self.stream = stream
        self.sha256 = hashlib.sha256()

    def read(self, size=-1):
        data = self.stream.read(size)
        self.sha256.update(data)
        return data


def inspect_archive(path, component, tag, commit, arch, observed_image_id=None):
    expected = identity(component, tag, commit, arch)
    require(path.name == expected["name"], "noncanonical container archive filename")
    regular_file(path)
    # Never extract paths. Hash every layer against the uncompressed diff IDs
    # in the config, which are part of the Docker image ID.
    hashes, diff_hashes, documents, sizes, names = {}, {}, {}, {}, set()
    with gzip.open(path, "rb") as compressed:
        with tarfile.open(fileobj=compressed, mode="r|") as archive:
            for member in archive:
                name = member.name
                require(name not in names, f"duplicate archive member: {name}")
                names.add(name)
                require(not PurePosixPath(name).is_absolute() and ".." not in PurePosixPath(name).parts,
                        f"unsafe archive member: {name}")
                if member.isdir():
                    continue
                require(member.isfile(), f"nonregular archive member: {name}")
                sizes[name] = member.size
                stream = archive.extractfile(member)
                if stream.peek(2)[:2] == b"\x1f\x8b":
                    hashed = HashingReader(stream)
                    with gzip.GzipFile(fileobj=hashed) as layer:
                        diff_hashes[name] = digest(layer)
                    hashes[name] = hashed.sha256.hexdigest()
                elif member.size <= 2 * 1024 * 1024:
                    data = stream.read()
                    hashes[name] = hashlib.sha256(data).hexdigest()
                    documents[name] = data
                    diff_hashes[name] = hashes[name]
                else:
                    hashes[name] = digest(stream)
                    diff_hashes[name] = hashes[name]
                if name.startswith("blobs/sha256/"):
                    require(name == "blobs/sha256/" + hashes[name], "archive blob SHA256 mismatch")
        while compressed.read(1024 * 1024):  # Check the gzip trailer/CRC.
            pass
    require("manifest.json" in documents, "missing Docker save manifest.json")
    manifest = parse_json(documents["manifest.json"])
    require(isinstance(manifest, list) and len(manifest) == 1, "archive must contain exactly one image")
    image = manifest[0]
    require(image["RepoTags"] == [expected["image_reference"]], "archive image reference mismatch")
    config_path = image["Config"]
    require(config_path in documents, "missing or oversized image config")
    config = parse_json(documents[config_path])
    config_digest = "sha256:" + hashes[config_path]
    require(config_path in (hashes[config_path] + ".json", "blobs/sha256/" + hashes[config_path]),
            "image config filename does not match its SHA256")
    require(config["os"] == "linux" and config["architecture"] == arch, "archive platform mismatch")
    labels = config["config"]["Labels"]
    require(labels["org.opencontainers.image.version"] == tag[1:] and
            labels["org.opencontainers.image.revision"] == commit and
            labels["org.opencontainers.image.source"] == "https://github.com/GizClaw/gizclaw",
            "archive OCI identity mismatch")
    require(config["config"]["User"] == "10001:10001", "archive must run as UID/GID 10001")
    entrypoint = (["/usr/local/bin/gizclaw-entrypoint"] if component == "runtime"
                  else ["python", "-m", "gizclaw_mem0"])
    require(config["config"]["Entrypoint"] == entrypoint, "archive entrypoint mismatch")
    if component == "runtime":
        require(labels["org.opencontainers.image.base.digest"] ==
                "sha256:008173c23f95b170204355c12626cb5a965d779a7e1283b09e9cffbb1bf33ca3",
                "runtime base image mismatch")
    layers = image["Layers"]
    require(config["rootfs"]["type"] == "layers" and isinstance(layers, list) and len(layers) > 0 and
            ["sha256:" + diff_hashes.get(layer, "missing") for layer in layers] == config["rootfs"]["diff_ids"],
            "archive layer integrity mismatch")
    # Containerd stores an OCI index alongside Docker's manifest.json. Ensure
    # both loaders describe the same sole image, config and layer chain.
    archive_manifest_digest = None
    if "index.json" in documents:
        index = parse_json(documents["index.json"])
        for _ in range(4):
            require(index["schemaVersion"] == 2 and len(index["manifests"]) == 1,
                    "OCI archive must describe exactly one image")
            descriptor = index["manifests"][0]
            annotations = descriptor.get("annotations", {})
            if "io.containerd.image.name" in annotations:
                require(annotations["io.containerd.image.name"] == "docker.io/library/" + expected["image_reference"],
                        "OCI archive reference mismatch")
            blob = "blobs/sha256/" + descriptor["digest"].removeprefix("sha256:")
            require(blob in documents and sizes[blob] == descriptor["size"], "invalid OCI manifest blob")
            index = parse_json(documents[blob])
            if "manifests" not in index:
                archive_manifest_digest = descriptor["digest"]
                break
        require(index.get("schemaVersion") == 2 and "config" in index and
                index["config"]["digest"] == config_digest and index["config"]["size"] == sizes[config_path] and
                ["blobs/sha256/" + item["digest"].removeprefix("sha256:") for item in index["layers"]] == layers and
                [item["size"] for item in index["layers"]] == [sizes[name] for name in layers],
                "OCI and Docker archive identities differ")
    with path.open("rb") as stream:
        archive_sha256 = digest(stream)
    require(observed_image_id is None or observed_image_id in (config_digest, archive_manifest_digest),
            "observed Docker image ID is absent from the archive")
    result = expected | {"size": path.stat().st_size, "sha256": archive_sha256,
                         "image_id": observed_image_id or config_digest, "config_digest": config_digest}
    if archive_manifest_digest:
        result["archive_manifest_digest"] = archive_manifest_digest
    return result


def validate_entry(entry, tag, commit):
    expected = identity(entry["component"], tag, commit, entry["architecture"])
    keys = set(expected) | {"size", "sha256", "image_id", "config_digest"}
    if "archive_manifest_digest" in entry:
        keys.add("archive_manifest_digest")
        require(re.fullmatch(r"sha256:[0-9a-f]{64}", entry["archive_manifest_digest"]),
                "invalid archive manifest digest")
    if entry["component"] == "runtime":
        keys.add("binary_sha256")
        require(re.fullmatch(r"[0-9a-f]{64}", entry["binary_sha256"]), "invalid binary SHA256")
    require(set(entry) == keys and all(entry[key] == value for key, value in expected.items()),
            "invalid container archive metadata")
    require(type(entry["size"]) is int and entry["size"] > 0 and
            re.fullmatch(r"[0-9a-f]{64}", entry["sha256"]) and
            re.fullmatch(r"sha256:[0-9a-f]{64}", entry["image_id"]) and
            re.fullmatch(r"sha256:[0-9a-f]{64}", entry["config_digest"]) and
            entry["image_id"] in (entry["config_digest"], entry.get("archive_manifest_digest")),
            "invalid archive size, SHA256 or image ID")


def check_entry(entry, tag, commit, directory):
    validate_entry(entry, tag, commit)
    path = directory / entry["name"]
    regular_file(path)
    with path.open("rb") as stream:
        require(path.stat().st_size == entry["size"] and digest(stream) == entry["sha256"],
                "container archive size/SHA256 mismatch")
    actual = inspect_archive(path, entry["component"], tag, commit, entry["architecture"], entry["image_id"])
    require(actual == {key: value for key, value in entry.items() if key != "binary_sha256"},
            "container archive image identity mismatch")
    if entry["component"] == "runtime":
        package = directory / f"gizclaw_{tag[1:]}_{entry['architecture']}.deb"
        regular_file(package)
        with subprocess.Popen(["dpkg-deb", "--fsys-tarfile", str(package)], stdout=subprocess.PIPE) as process:
            with tarfile.open(fileobj=process.stdout, mode="r|") as archive:
                binary_hashes = [digest(archive.extractfile(member)) for member in archive
                                 if member.name == "./usr/bin/gizclaw" and member.isfile()]
            process.stdout.read()
            require(process.wait() == 0 and binary_hashes == [entry["binary_sha256"]],
                    "container executable digest differs from Debian package")


def main(args):
    if len(args) in (6, 7) and args[0] == "inspect":
        _, filename, component, tag, commit, arch, *observed = args
        print(json.dumps(inspect_archive(Path(filename), component, tag, commit, arch, *observed), sort_keys=True))
    elif len(args) == 5 and args[0] == "entry":
        _, filename, tag, commit, directory = args
        regular_file(Path(filename))
        check_entry(parse_json(Path(filename).read_bytes()), tag, commit, Path(directory))
    elif len(args) in (5, 7) and args[0] == "receipt":
        _, filename, tag, commit, directory, *selection = args
        regular_file(Path(filename))
        receipt = parse_json(Path(filename).read_bytes())
        require(set(receipt) == {"schema_version", "tag", "version", "source_commit", "images"} and
                receipt["schema_version"] == 2 and receipt["tag"] == tag and
                receipt["version"] == tag[1:] and receipt["source_commit"] == commit,
                "invalid container receipt identity/schema")
        images = receipt["images"]
        require([(entry["component"], entry["architecture"]) for entry in images] ==
                [(component, arch) for component in ("mem0", "runtime") for arch in ("amd64", "arm64")],
                "container receipt must list exactly four sorted images")
        for entry in images:
            validate_entry(entry, tag, commit)
        if selection:
            identity(selection[0], tag, commit, selection[1])
        for entry in images:
            if not selection or [entry["component"], entry["architecture"]] == selection:
                check_entry(entry, tag, commit, Path(directory))
    else:
        raise ValueError("usage: container_archive.py inspect ARCHIVE COMPONENT TAG COMMIT ARCH [IMAGE_ID] | "
                         "entry ENTRY TAG COMMIT DIR | receipt RECEIPT TAG COMMIT DIR [COMPONENT ARCH]")


if __name__ == "__main__":
    try:
        main(sys.argv[1:])
    except (OSError, EOFError, ValueError, KeyError, TypeError, tarfile.TarError, subprocess.SubprocessError) as error:
        sys.exit(f"container archive validation failed: {error}")
