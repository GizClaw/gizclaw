#!/usr/bin/env python3
"""Inspect Docker save archives and validate the Release container contract."""

import gzip
from functools import lru_cache
import hashlib
import json
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
import tarfile
import threading


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


def json_file(path):
    regular_file(path)
    require(path.stat().st_size <= 1024**2, "oversized container metadata")
    return parse_json(path.read_bytes())


def digest(stream):
    return hashlib.file_digest(stream, "sha256").hexdigest()


def reference_aliases(reference):
    """Accept Docker Hub name normalization without changing repository or tag."""
    require(isinstance(reference, str), "invalid archive image reference")
    name, separator, tag = reference.rpartition(":")
    require(separator and re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}", tag),
            "archive image reference requires an explicit tag")
    first, slash, rest = name.partition("/")
    if first in ("docker.io", "index.docker.io"):
        require(slash, "missing Docker Hub repository")
        repository = rest
    elif slash and ("." in first or ":" in first or first == "localhost"):
        return {reference}  # Other registries have no Docker Hub aliases.
    else:
        repository = name
    require(re.fullmatch(r"[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*(?:/[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*)*",
                         repository), "invalid Docker Hub repository")
    if "/" not in repository:
        repository = "library/" + repository
    aliases = {f"{prefix}{repository}:{tag}" for prefix in ("", "docker.io/", "index.docker.io/")}
    if repository.startswith("library/") and repository.count("/") == 1:
        aliases.update(f"{prefix}{repository.removeprefix('library/')}:{tag}"
                       for prefix in ("", "docker.io/", "index.docker.io/"))
    return aliases


def matches_reference(tags, reference):
    return isinstance(tags, list) and len(tags) == 1 and tags[0] in reference_aliases(reference)


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


def inspect_docker_archive(path, reference, arch, observed_image_id=None):
    """Prove a Docker archive's sole runnable image and bound OCI attestations."""
    require(arch in ("amd64", "arm64"), "unsupported archive architecture")
    expected = {"image_reference": reference}
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
    require(matches_reference(image["RepoTags"], reference), "archive image reference mismatch")
    config_path = image["Config"]
    require(config_path in documents, "missing or oversized image config")
    config = parse_json(documents[config_path])
    config_digest = "sha256:" + hashes[config_path]
    require(config_path in (hashes[config_path] + ".json", config_digest, "blobs/sha256/" + hashes[config_path]),
            "image config filename does not match its SHA256")
    require(config["os"] == "linux" and config["architecture"] == arch, "archive platform mismatch")
    layers = image["Layers"]
    require(config["rootfs"]["type"] == "layers" and isinstance(layers, list) and len(layers) > 0 and
            ["sha256:" + diff_hashes.get(layer, "missing") for layer in layers] == config["rootfs"]["diff_ids"],
            "archive layer integrity mismatch")
    # Containerd stores an OCI index alongside Docker's manifest.json. Ensure
    # both loaders describe the same sole image, config and layer chain.
    archive_manifest_digest = None
    archive_index_digest = None
    index_ids = set()
    if "index.json" in documents:
        index = parse_json(documents["index.json"])
        require(index["schemaVersion"] == 2 and len(index["manifests"]) > 0, "invalid OCI archive index")
        named = [item for item in index["manifests"] if
                 "io.containerd.image.name" in item.get("annotations", {}) or
                 "org.opencontainers.image.ref.name" in item.get("annotations", {})]
        require(len(named) == 1, "OCI archive must name exactly one image")
        root = named[0]
        annotations = root.get("annotations", {})
        if "io.containerd.image.name" in annotations:
            require(annotations["io.containerd.image.name"] in reference_aliases(reference),
                    "OCI archive reference mismatch")
        if "org.opencontainers.image.ref.name" in annotations:
            require(annotations["org.opencontainers.image.ref.name"] == expected["image_reference"].rsplit(":", 1)[1],
                    "OCI archive tag mismatch")

        def document(descriptor):
            require(re.fullmatch(r"sha256:[0-9a-f]{64}", descriptor["digest"]), "invalid OCI descriptor digest")
            blob = "blobs/sha256/" + descriptor["digest"].removeprefix("sha256:")
            require(blob in documents and sizes[blob] == descriptor["size"], "invalid OCI JSON blob")
            return parse_json(documents[blob])

        def verify_blob(descriptor):
            require(re.fullmatch(r"sha256:[0-9a-f]{64}", descriptor["digest"]), "invalid OCI blob digest")
            blob = "blobs/sha256/" + descriptor["digest"].removeprefix("sha256:")
            require(blob in hashes and sizes[blob] == descriptor["size"], "missing or wrong-sized OCI blob")
            return blob

        leaves, attestations, visited = [], [], set()

        def walk(descriptor, depth):
            require(depth <= 4 and descriptor["digest"] not in visited, "duplicate or excessively nested OCI graph")
            visited.add(descriptor["digest"])
            node = document(descriptor)
            require(node["schemaVersion"] == 2, "invalid OCI schema")
            annotation = descriptor.get("annotations", {})
            if (annotation.get("vnd.docker.reference.type") == "attestation-manifest" or
                    "io.containerd.manifest.subject" in annotation or "subject" in node):
                require(descriptor.get("platform", {"os": "unknown", "architecture": "unknown"}) ==
                        {"os": "unknown", "architecture": "unknown"},
                        "attestation must declare an unknown platform")
                attestations.append((descriptor, node))
            elif "manifests" in node:
                require(len(node["manifests"]) > 0, "empty OCI index")
                before = len(leaves)
                for child in node["manifests"]:
                    walk(child, depth + 1)
                require(len(leaves) - before == 1, "OCI index must select one runnable image")
                index_ids.add(descriptor["digest"])
            else:
                platform = descriptor.get("platform", {"os": "linux", "architecture": arch})
                require(platform.get("os") == "linux" and platform.get("architecture") == arch,
                        "OCI runnable image platform mismatch")
                leaves.append((descriptor, node))

        if "manifests" in document(root):
            archive_index_digest = root["digest"]
        for descriptor in index["manifests"]:
            walk(descriptor, 0)
        require(len(leaves) == 1, "OCI archive must contain one runnable platform image")
        descriptor, node = leaves[0]
        archive_manifest_digest = descriptor["digest"]
        require(node["config"]["digest"] == config_digest and
                node["config"]["size"] == sizes[config_path] and
                [verify_blob(item) for item in node["layers"]] == layers,
                "OCI and Docker archive identities differ")
        for descriptor, node in attestations:
            bindings = [value for field, value in descriptor.get("annotations", {}).items()
                        if field in ("vnd.docker.reference.digest", "io.containerd.manifest.subject")]
            if "subject" in node:
                verify_blob(node["subject"])
                bindings.append(node["subject"]["digest"])
            require(len(bindings) > 0 and all(value == archive_manifest_digest for value in bindings),
                    "attestation is not bound to the runnable image")
            attestation_config = document(node["config"])
            attestation_layers = [verify_blob(item) for item in node["layers"]]
            if node["config"].get("mediaType") == "application/vnd.oci.empty.v1+json":
                require("subject" in node and documents[verify_blob(node["config"])] == b"{}" and
                        node["config"].get("data", "e30=") == "e30=",
                        "empty attestation config requires exact empty bytes and a bound subject")
            else:
                require(attestation_config.get("os") == "unknown" and
                        attestation_config.get("architecture") == "unknown" and
                        attestation_config["rootfs"]["type"] == "layers" and
                        attestation_config["rootfs"]["diff_ids"] ==
                        ["sha256:" + diff_hashes[name] for name in attestation_layers],
                        "invalid attestation config/layer identity")
            require(len(node["layers"]) > 0, "empty attestation")
            for item in node["layers"]:
                require(item["mediaType"] == "application/vnd.in-toto+json", "unsupported attestation layer")
                statement = document(item)
                require(statement.get("_type") in ("https://in-toto.io/Statement/v0.1", "https://in-toto.io/Statement/v1") and
                        len(statement.get("subject", [])) > 0 and
                        all(subject.get("digest", {}).get("sha256") == archive_manifest_digest.removeprefix("sha256:")
                            for subject in statement["subject"]), "attestation subject identity mismatch")
    with path.open("rb") as stream:
        archive_sha256 = digest(stream)
    require(observed_image_id is None or observed_image_id in (config_digest, archive_manifest_digest, archive_index_digest),
            "observed Docker image ID is absent from the archive")
    result = {"size": path.stat().st_size, "sha256": archive_sha256,
                         "image_id": observed_image_id or config_digest, "config_digest": config_digest, "_config": config}
    if archive_manifest_digest:
        result["archive_manifest_digest"] = archive_manifest_digest
    if archive_index_digest:
        result["archive_index_digest"] = archive_index_digest
    load_ids = {config_digest, *index_ids}
    if archive_manifest_digest:
        load_ids.add(archive_manifest_digest)
    # Alternate OCI/Docker media encodings can describe the identical config
    # and compressed layers. Their IDs are independently content-proved too.
    for name, data in documents.items():
        if not name.startswith("blobs/sha256/"):
            continue
        try:
            node = parse_json(data)
        except ValueError:
            continue
        if (isinstance(node, dict) and node.get("schemaVersion") == 2 and
                isinstance(node.get("config"), dict) and isinstance(node.get("layers"), list) and
                all(isinstance(item, dict) for item in node["layers"]) and
                node["config"].get("digest") == config_digest and
                node["config"].get("size") == sizes[config_path] and
                [{"digest": item.get("digest"), "size": item.get("size")} for item in node.get("layers", [])] ==
                [{"digest": "sha256:" + hashes[layer], "size": sizes[layer]} for layer in layers]):
            load_ids.add("sha256:" + hashes[name])
    result["_load_ids"] = sorted(load_ids)
    return result


def inspect_archive(path, component, tag, commit, arch, observed_image_id=None):
    expected = identity(component, tag, commit, arch)
    require(path.name == expected["name"], "noncanonical container archive filename")
    result = inspect_docker_archive(path, expected["image_reference"], arch, observed_image_id)
    config = result.pop("_config")
    result.pop("_load_ids")
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
    return expected | result


def verify_saved_config(stream, reference, arch, expected_digest):
    """Prove a locally re-exported image's config without trusting its inspect ID."""
    require(re.fullmatch(r"sha256:[0-9a-f]{64}", expected_digest), "invalid expected config digest")
    config_names = {expected_digest[7:] + ".json", expected_digest, "blobs/sha256/" + expected_digest[7:]}
    found, manifest, total = set(), None, 0
    with tarfile.open(fileobj=stream, mode="r|") as archive:
        for count, member in enumerate(archive):
            total += member.size
            require(count < 100000 and total <= 16 * 1024**3, "loaded image export exceeds validation bounds")
            if member.name in config_names or member.name == "manifest.json":
                require(member.isfile() and member.size <= 1024**2, "invalid exported config/manifest")
                data = archive.extractfile(member).read()
                if member.name == "manifest.json":
                    require(manifest is None, "duplicate exported Docker manifest")
                    manifest = parse_json(data)
                else:
                    config = parse_json(data)
                    require("sha256:" + hashlib.sha256(data).hexdigest() == expected_digest and
                            config["os"] == "linux" and config["architecture"] == arch,
                            "loaded image config/platform mismatch")
                    found.add(member.name)
    while chunk := stream.read(1024 * 1024):
        total += len(chunk)
        require(total <= 16 * 1024**3, "loaded image export exceeds validation bounds")
    require(isinstance(manifest, list) and len(manifest) == 1 and manifest[0]["Config"] in found and
            matches_reference(manifest[0]["RepoTags"], reference), "loaded image config identity mismatch")


@lru_cache(maxsize=1)
def docker_api_minor():
    versions = subprocess.check_output(
        ["docker", "version", "--format", "{{.Client.APIVersion}} {{.Server.APIVersion}}"], timeout=30).decode().split()
    require(len(versions) == 2 and all(re.fullmatch(r"1\.[0-9]+", version) for version in versions),
            "unsupported Docker API version")
    return min(int(version.split(".")[1]) for version in versions)


def docker_platform_options(arch, minimum_api):
    require(arch in ("amd64", "arm64"), "unsupported Docker architecture")
    return ["--platform", "linux/" + arch] if docker_api_minor() >= minimum_api else []


def prove_loaded_image(reference, arch, proof):
    inspected = parse_json(subprocess.check_output(
        ["docker", "image", "inspect", *docker_platform_options(arch, 49), "--format", "{{json .}}", reference], timeout=30))
    require(re.fullmatch(r"sha256:[0-9a-f]{64}", inspected["Id"]) and inspected["Os"] == "linux" and inspected["Architecture"] == arch, "loaded image platform mismatch")
    known = set(proof.get("_load_ids", [])) | {proof.get(field) for field in
                                           ("config_digest", "archive_manifest_digest", "archive_index_digest")}
    if inspected["Id"] not in known:
        # Containerd may reuse compressed layers when importing a classic tar,
        # producing a new manifest ID. Only exact saved config bytes permit it.
        with subprocess.Popen(["docker", "image", "save", *docker_platform_options(arch, 48), reference], stdout=subprocess.PIPE,
                              stderr=subprocess.DEVNULL) as process:
            timer = threading.Timer(120, process.kill)
            timer.daemon = True
            timer.start()
            try:
                verify_saved_config(process.stdout, reference, arch, proof["config_digest"])
                require(process.wait(timeout=10) == 0, "loaded image export failed")
            finally:
                timer.cancel()
                if process.poll() is None:
                    process.kill()
                    process.wait()
    return inspected["Id"]


def validate_entry(entry, tag, commit):
    expected = identity(entry["component"], tag, commit, entry["architecture"])
    keys = set(expected) | {"size", "sha256", "image_id", "config_digest"}
    for field in ("archive_manifest_digest", "archive_index_digest"):
        if field in entry:
            keys.add(field)
            require(re.fullmatch(r"sha256:[0-9a-f]{64}", entry[field]), f"invalid {field}")
    require("archive_index_digest" not in entry or "archive_manifest_digest" in entry,
            "an archive index requires a runnable manifest identity")
    if entry["component"] == "runtime":
        keys.add("binary_sha256")
        require(re.fullmatch(r"[0-9a-f]{64}", entry["binary_sha256"]), "invalid binary SHA256")
    require(set(entry) == keys and all(entry[key] == value for key, value in expected.items()),
            "invalid container archive metadata")
    require(type(entry["size"]) is int and entry["size"] > 0 and
            re.fullmatch(r"[0-9a-f]{64}", entry["sha256"]) and
            re.fullmatch(r"sha256:[0-9a-f]{64}", entry["image_id"]) and
            re.fullmatch(r"sha256:[0-9a-f]{64}", entry["config_digest"]) and
            entry["image_id"] in (entry["config_digest"], entry.get("archive_manifest_digest"), entry.get("archive_index_digest")),
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
    if len(args) == 4 and args[0] == "loaded":
        _, filename, reference, arch = args
        if filename == "-":
            data = sys.stdin.buffer.read(65537)
            require(len(data) <= 65536, "oversized loaded-image metadata")
            entry = parse_json(data)
        else:
            entry = json_file(Path(filename))
        validate_entry(entry, "v" + entry["version"], entry["source_commit"])
        require(entry["image_reference"] == reference and entry["architecture"] == arch, "loaded reference mismatch")
        print(prove_loaded_image(reference, arch, entry))
    elif len(args) in (6, 7) and args[0] == "inspect":
        _, filename, component, tag, commit, arch, *observed = args
        print(json.dumps(inspect_archive(Path(filename), component, tag, commit, arch, *observed), sort_keys=True))
    elif len(args) == 5 and args[0] == "entry":
        _, filename, tag, commit, directory = args
        check_entry(json_file(Path(filename)), tag, commit, Path(directory))
    elif len(args) in (5, 7) and args[0] == "receipt":
        _, filename, tag, commit, directory, *selection = args
        receipt = json_file(Path(filename))
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
