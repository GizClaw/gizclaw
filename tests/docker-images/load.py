#!/usr/bin/env python3
"""Load declared E2E dependencies from immutable TOS archives, without Registry fallback."""

import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "build"))
import container_archive

DEFAULT_ORIGIN = "https://static-volc.gizclaw.com/releases/GizClaw/deploy/container-images/v1"


def require(condition, message):
    if not condition:
        raise ValueError(message)


def catalog():
    document = container_archive.parse_json((Path(__file__).parent / "catalog.json").read_bytes())
    require(document["schema_version"] == 1 and document["platforms"] == ["linux/amd64", "linux/arm64"],
            "unsupported E2E image catalog")
    for name, source in document["images"].items():
        require(re.fullmatch(r"[a-z][a-z0-9-]*", name) and
                re.fullmatch(r"docker\.io/(?:[a-z0-9._-]+/)+[a-z0-9._-]+:[a-zA-Z0-9._-]+@sha256:[0-9a-f]{64}", source),
                f"E2E image source must be canonical and digest-pinned: {name}")
    return document["images"]


def source_identity(name, arch):
    require(arch in ("amd64", "arm64"), "unsupported Docker architecture")
    images = catalog()
    require(name in images, f"unknown E2E dependency: {name}")
    source = images[name]
    key = hashlib.sha256(source.encode()).hexdigest()
    return source, key, f"gizclaw-tos/cache:{key}-{arch}"


def docker(*args, check=True):
    return subprocess.run(["docker", *args], check=check, text=True, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, timeout=300)


def native_architecture():
    arch = docker("info", "--format", "{{.Architecture}}").stdout.strip()
    arch = {"aarch64": "arm64", "x86_64": "amd64"}.get(arch, arch)
    require(arch in ("amd64", "arm64"), f"unsupported Docker architecture: {arch}")
    return arch


def origin_url():
    value = os.environ.get("GIZCLAW_CONTAINER_IMAGE_BASE_URL", DEFAULT_ORIGIN).rstrip("/")
    parsed = urllib.parse.urlsplit(value)
    local = False
    try:
        local = ipaddress.ip_address(parsed.hostname).is_loopback
    except ValueError:
        pass
    require(parsed.scheme == "https" or (parsed.scheme == "http" and local),
            "image origin requires HTTPS; HTTP is allowed only for explicit loopback fixtures")
    require(parsed.hostname and not parsed.username and not parsed.password and not parsed.query and not parsed.fragment,
            "invalid image origin")
    return value


class SameOriginRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, message, headers, new_url):
        previous = urllib.parse.urlsplit(request.full_url)
        target = urllib.parse.urlsplit(new_url)
        require((previous.scheme, previous.netloc) == (target.scheme, target.netloc),
                "cross-origin image redirect refused")
        return super().redirect_request(request, fp, code, message, headers, new_url)


def fetch(url, destination, max_size, refresh=False):
    require(not destination.is_symlink(), f"symlink cache object refused: {destination}")
    if destination.exists():
        container_archive.regular_file(destination)
        require(destination.stat().st_size <= max_size, "oversized cached image object")
        if not refresh or os.environ.get("GIZCLAW_CONTAINER_CACHE_ONLY") == "1":
            return
    require(os.environ.get("GIZCLAW_CONTAINER_CACHE_ONLY") != "1",
            f"required cached image object is missing: {destination}")
    print(f"E2E image preflight: download {url}", file=sys.stderr)
    partial = None
    try:
        opener = urllib.request.build_opener(SameOriginRedirect())
        with opener.open(url, timeout=30) as response:
            with tempfile.NamedTemporaryFile(dir=destination.parent, prefix=".download-", delete=False) as output:
                partial = Path(output.name)
                size = 0
                while chunk := response.read(1024 * 1024):
                    size += len(chunk)
                    require(size <= max_size, "download exceeds declared size limit")
                    output.write(chunk)
        # An immutable cache object is never replaced, including concurrent loads.
        try:
            os.link(partial, destination)
        except FileExistsError:
            container_archive.regular_file(destination)
            if refresh:
                require(destination.read_bytes() == partial.read_bytes(), "cached manifest differs from immutable TOS metadata")
    finally:
        if partial is not None:
            partial.unlink(missing_ok=True)


def validate_manifest(document, name, arch):
    source, key, reference = source_identity(name, arch)
    required = {"schema_version", "source", "source_key", "platform", "reference", "archive_reference",
                "image_id", "archive", "size", "sha256"}
    optional = {"load_ids", "source_config_digest", "source_load_ids",
                "source_repository", "source_tag", "source_commit"}
    require(required <= set(document) <= required | optional, "unsupported TOS image manifest fields")
    require(document["schema_version"] == 1 and document["source"] == source and
            document["source_key"] == key and document["platform"] == f"linux/{arch}" and
            document["reference"] == reference and document["archive"] == "image.tar.gz",
            "TOS image source/platform/reference mismatch")
    require(type(document["size"]) is int and 0 < document["size"] <= 8 * 1024**3 and
            re.fullmatch(r"[0-9a-f]{64}", document["sha256"]) and
            re.fullmatch(r"sha256:[0-9a-f]{64}", document["image_id"]) and
            re.fullmatch(r"[a-z0-9][a-z0-9._/:-]+", document["archive_reference"]),
            "invalid TOS image size, hash or identity")
    for field in ("load_ids", "source_load_ids"):
        if field in document:
            values = document[field]
            require(isinstance(values, list) and 0 < len(values) <= 64 and len(set(values)) == len(values) and
                    all(re.fullmatch(r"sha256:[0-9a-f]{64}", value) for value in values),
                    f"invalid TOS {field}")
    require("load_ids" not in document or document["image_id"] in document["load_ids"],
            "load IDs omit the canonical config digest")
    return key, reference


def inspect_local(reference, proof, arch):
    result = docker("image", "inspect", *container_archive.docker_platform_options(arch, 49),
                    "--format", "{{json .}}", reference, check=False)
    if result.returncode:
        # Only an explicit missing image permits import; daemon/auth failures stop.
        require("no such image" in result.stderr.lower(), f"inspect local image: {result.stderr.strip()}")
        return None
    image = container_archive.parse_json(result.stdout)
    require(image["Os"] == "linux" and image["Architecture"] == arch,
            f"cached/loaded image identity or platform mismatch: {reference}")
    if image["Id"] in proof["_load_ids"]:
        return image["Id"]
    return container_archive.prove_loaded_image(reference, arch, proof)


def load_image(name, arch):
    _, key, reference = source_identity(name, arch)
    cache_root = Path(os.environ.get("GIZCLAW_CONTAINER_IMAGE_CACHE_DIR", ROOT / ".tmp/container-images"))
    require(not cache_root.is_symlink(), "symlink image cache root refused")
    directory = cache_root / key / arch
    require(not directory.is_symlink() and not directory.parent.is_symlink(), "symlink image cache directory refused")
    directory.mkdir(parents=True, exist_ok=True)
    origin = f"{origin_url()}/{key}/{arch}"
    manifest = directory / "manifest.json"
    fetch(f"{origin}/manifest.json", manifest, 65536, refresh=True)
    require(manifest.stat().st_size <= 65536, "oversized cached image manifest")
    document = container_archive.parse_json(manifest.read_bytes())
    validate_manifest(document, name, arch)
    archive = directory / "image.tar.gz"
    fetch(f"{origin}/image.tar.gz", archive, document["size"])
    with archive.open("rb") as stream:
        require(archive.stat().st_size == document["size"] and
                container_archive.digest(stream) == document["sha256"], "image archive size/SHA256 mismatch")
    proof = container_archive.inspect_docker_archive(archive, document["archive_reference"], arch, document["image_id"])
    require(document["image_id"] == proof["config_digest"], "TOS image_id is not the canonical config digest")
    if "load_ids" in document:
        require(document["load_ids"] == proof["_load_ids"], "TOS load IDs are not proved by the archive")
    if "source_config_digest" in document:
        require(document["source_config_digest"] == proof["config_digest"], "upstream config digest mismatch")
    if "source_load_ids" in document:
        require(set(document["source_load_ids"]) <= set(proof["_load_ids"]), "unproved upstream load IDs")
    image_id = inspect_local(reference, proof, arch)
    if image_id is None:
        result = docker("load", "--input", str(archive))
        print(result.stdout.strip(), file=sys.stderr)
        image_id = inspect_local(document["archive_reference"], proof, arch)
        require(image_id is not None, "archive did not restore the declared image reference")
        docker("tag", image_id, reference)
        inspect_local(reference, proof, arch)
    return reference


def validate_build_bases(build, arch):
    context = Path(build["context"]).resolve()
    require(context.is_relative_to(ROOT), "E2E build context must be in the current source checkout")
    dockerfile = Path(build.get("dockerfile", "Dockerfile"))
    if not dockerfile.is_absolute():
        dockerfile = context / dockerfile
    require(dockerfile.resolve().is_relative_to(ROOT), "E2E Dockerfile must be in the current source checkout")
    arguments = dict(build.get("args", {}))
    stages = {"scratch"}
    known = {source_identity(name, arch)[2] for name in catalog()}
    known.update(value for field, value in os.environ.items()
                 if field in ("GIZCLAW_E2E_DOCKER_BASE_IMAGE", "GIZCLAW_E2E_MEM0_BASE_IMAGE",
                              "GIZCLAW_LOCOMO_E2E_MEM0_BASE_IMAGE") and value)
    for line in dockerfile.read_text().splitlines():
        require(not re.match(r"\s*#\s*syntax\s*=", line), "external Dockerfile frontends are disabled for E2E")
        argument = re.match(r"\s*ARG\s+([A-Za-z_][A-Za-z0-9_]*)(?:=(.*))?$", line, re.I)
        if argument and argument[1] not in arguments:
            arguments[argument[1]] = argument[2] or ""
        stage = re.match(r"\s*FROM\s+(?:--platform=\S+\s+)?(\S+)(?:\s+AS\s+(\S+))?", line, re.I)
        if stage:
            source = re.sub(r"\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?",
                            lambda match: arguments.get(match[1], ""), stage[1])
            if source.lower() not in stages:
                require(source in known, f"undeclared E2E base image in {dockerfile.name}")
                result = docker("image", "inspect", "--format", "{{.Os}}/{{.Architecture}}", source)
                require(result.stdout.strip() == f"linux/{arch}", "missing or wrong-platform local build base")
            if stage[2]:
                stages.add(stage[2].lower())
        copy = re.search(r"\b(?:COPY|ADD)\s+.*?--from=(\S+)", line, re.I)
        if copy:
            require(copy[1].lower() in stages, "external COPY/ADD image sources are disabled for E2E")
        for mount in re.finditer(r"--mount=\S*?\bfrom=([^,\s]+)", line):
            require(mount[1].lower() in stages, "external mount image sources are disabled for E2E")


def compose_dependencies(document, arch):
    references = {source_identity(name, arch)[2]: name for name in catalog()}
    services = document["services"]
    built = {service["image"] for service in services.values() if service.get("build") and service.get("image")}
    built.update(os.environ.get("GIZCLAW_E2E_BUILT_IMAGES", "").splitlines())
    needed = set()
    for service in services.values():
        require(service.get("pull_policy") == "never", "all E2E services must forbid image pulling")
        require(service.get("platform", f"linux/{arch}") == f"linux/{arch}", "E2E service platform mismatch")
        if service.get("build"):
            # Compose omits false-valued build.pull in its canonical JSON model.
            require(service["build"].get("pull", False) is False, "E2E build pulling must be disabled")
            require(service["build"].get("platforms", [f"linux/{arch}"]) == [f"linux/{arch}"],
                    "E2E builds must select the requested platform")
            validate_build_bases(service["build"], arch)
        image = service.get("image")
        if image in references:
            needed.add(references[image])
        elif image:
            require(image in built, "undeclared E2E service image; declare a TOS dependency or build current source")
    return sorted(needed)


def main():
    require(sys.version_info >= (3, 11), "Python 3.11 or newer is required for E2E image verification")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("load", "reference", "references", "inventory", "compose", "dockerfile", "source"))
    parser.add_argument("name", nargs="?")
    parser.add_argument("arch", nargs="?")
    parser.add_argument("--context", default=str(ROOT))
    parser.add_argument("--base-image")
    args = parser.parse_args()
    if args.mode == "dockerfile":
        require(args.name is not None and args.base_image, "Dockerfile and verified base image are required")
        validate_build_bases({"context": args.context, "dockerfile": args.name,
                              "args": {"BASE_IMAGE": args.base_image}}, args.arch or native_architecture())
        return
    if args.mode == "source":
        require(args.name in catalog(), "unknown E2E image source")
        print(catalog()[args.name])
        return
    if args.mode == "compose":
        arch = args.name or native_architecture()
        print("\n".join(compose_dependencies(container_archive.parse_json(sys.stdin.buffer.read()), arch)))
        return
    if args.mode == "inventory":
        print(json.dumps(catalog(), indent=2, sort_keys=True))
        return
    if args.mode == "references":
        arch = args.name or native_architecture()
        for name in catalog():
            print(f"GIZCLAW_E2E_{name.upper().replace('-', '_')}_IMAGE\t{source_identity(name, arch)[2]}")
        return
    require(args.name is not None, "an image catalog ID is required")
    arch = args.arch or native_architecture()
    if args.mode == "reference":
        print(source_identity(args.name, arch)[2])
    else:
        print(load_image(args.name, arch))


if __name__ == "__main__":
    try:
        main()
    except (OSError, EOFError, ValueError, KeyError, TypeError, subprocess.SubprocessError) as error:
        sys.exit(f"E2E image preflight failed: {error}; Registry fallback is disabled")
