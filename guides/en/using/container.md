# Container Image

Formal Releases provide Docker save gzip archives for Runtime and Mem0 on
`linux/amd64` and `linux/arm64`. Runtime includes the GizClaw executable,
runtime libraries, CA certificates and curl. Run Mem0, LiveKit and databases
separately; production may use managed PostgreSQL.

## Select and verify a Release

Download the selected Release's files and verify `SHA256SUMS`, schema 8
`release-manifest.json` and source commit using
[Repository Releases](../developing/tooling#repository-releases). Select the host
architecture in schema 2 `container-image.json`, verify and load the archive. Run
these commands from the matching source-tag checkout; adjust downloaded file paths as needed:

```sh
arch=amd64 # Set arm64 on an arm64 host.
entry="$(jq -ce --arg arch "$arch" '.images[] | select(.component == "runtime" and .architecture == $arch)' container-image.json)"
archive="$(jq -er .name <<<"$entry")"
printf '%s  %s\n' "$(jq -er .sha256 <<<"$entry")" "$archive" | sha256sum --check
docker load --input "$archive"
reference="$(jq -er .image_reference <<<"$entry")"
export GIZCLAW_IMAGE="$(printf '%s\n' "$entry" | python3 build/container_archive.py loaded - "$reference" "$arch")"
docker image inspect "$GIZCLAW_IMAGE" | jq -e --arg arch "$arch" \
  --arg version "$(jq -er .version <<<"$entry")" --arg commit "$(jq -er .source_commit <<<"$entry")" '
  .[0] | .Os == "linux" and .Architecture == $arch and
  .Config.Labels["org.opencontainers.image.version"] == $version and
  .Config.Labels["org.opencontainers.image.revision"] == $commit'
docker run --rm --pull never "$GIZCLAW_IMAGE" --version
docker run --rm --pull never "$GIZCLAW_IMAGE" --help
```

Loading restores `gizclaw-runtime:<tag>-<arch>`. Pin the verified local image ID
for running and Compose. Archive SHA256 and image ID identify different objects
and are not registry digests. For Mem0, select `.component == "mem0"` and follow
the same verification/loading steps; configuration and startup are documented in
[self-hosted Mem0](../developing/stores/memory#self-hosted-mem0-service). See
[Container image archives](../developing/tooling#container-image-archives) for
filenames, receipt fields and the downstream consumption contract.

## Workspace and configuration

The entrypoint is `/usr/local/bin/gizclaw-entrypoint`, which executes
`/usr/bin/gizclaw`. Default arguments are
`serve --force /var/lib/gizclaw`, which explicitly enables foreground serving
inside the container. The wrapper claims a workspace file lock, removes a stale
`serve.pid` under that lock and executes the Server, which handles SIGTERM directly. It runs as UID/GID
`10001:10001`; bind-mounted writable directories must belong to that account.
A fresh named volume inherits the image workspace ownership.

Mount a complete `config.yaml` at `/var/lib/gizclaw/config.yaml` and persistent
data at `/var/lib/gizclaw`. Relative storage, certificate and credential-file
paths resolve from that workspace. Use the complete
[Server configuration](../developing/gizclaw/server/main#storage-store-and-service-composition)
as a starting point. Supply an existing `identity.private-key` before mounting
config read-only: if omitted, Server generates a key and attempts to write it
to the config. Keep identity and data across restarts. One workspace belongs to
one Server; replicas need separate workspaces.

Set `webrtc.listen` and the first HTTP listener to `0.0.0.0:9820`, and set
`webrtc.endpoint` to the reachable Server endpoint. TCP and UDP 9820 serve HTTP,
signaling and ICE; additional configured ICE ports need corresponding mappings.
Direct business HTTP is routed through Edge as documented by Server. Bind the
PostgreSQL DSN through environment expansion, configure LiveKit in `services.sfu`
with mounted credential files, and provision the appropriate MemoryLayout for
external Mem0 through Admin resources. The image does not provision those services.

## Compose

Place a prepared config at `./gizclaw/config.yaml`. Export the required values
referenced by that YAML, including identity, admin public key and PostgreSQL DSN.
The example assumes the default listener above:

```yaml
services:
  gizclaw:
    image: ${GIZCLAW_IMAGE:?set the verified local image ID}
    pull_policy: never
    init: true
    restart: unless-stopped
    stop_grace_period: 30s
    environment:
      GIZCLAW_SERVER_PRIVATE_KEY: ${GIZCLAW_SERVER_PRIVATE_KEY:?required}
      GIZCLAW_ADMIN_PUBLIC_KEY: ${GIZCLAW_ADMIN_PUBLIC_KEY:?required}
      GIZCLAW_POSTGRES_DSN: ${GIZCLAW_POSTGRES_DSN:?required}
    volumes:
      - gizclaw-data:/var/lib/gizclaw
      - ./gizclaw/config.yaml:/var/lib/gizclaw/config.yaml:ro
    ports:
      - "9820:9820/tcp"
      - "9820:9820/udp"
    healthcheck:
      test: ["CMD", "curl", "-fsS", "--max-time", "2", "http://127.0.0.1:9820/server-info"]
      interval: 10s
      timeout: 3s
      retries: 6
      start_period: 30s
volumes:
  gizclaw-data:
```

Add service/network and read-only credential-file mounts appropriate to your
deployment. Keep deployment secrets outside the image. Use `docker compose config`
to check interpolation, then `docker compose up -d gizclaw`. Inspect
`docker compose ps` and `/server-info` for the expected version and build commit.
This health check proves Server HTTP readiness; provider and SFU acceptance have
their own checks. Adapt it if listener port or TLS changes.

Stop with `docker compose stop gizclaw` or `docker stop --time 30 <container>`.
Normal shutdown removes `serve.pid` and closes stores. Retain the data volume;
avoid `docker compose down -v` when preserving state. The default command recovers automatically after a forced stop; a concurrent
container using the same workspace is rejected before modifying the PID file.
Never share that workspace with a host Server. Custom CLI arguments execute
directly without this default-workspace lock.

## Local runtime verification

Native Linux CI builds each architecture and runs the same archive/runtime
gate used by Release. To reproduce it for a verified package:

```sh
mkdir -p .tmp/container-assets
cp "$PACKAGE" ".tmp/container-assets/gizclaw_${VERSION}_${ARCH}.deb"
build/build-runtime-image.sh "$PACKAGE" "$VERSION" "$SOURCE_COMMIT" "$SOURCE_EPOCH" "$ARCH"
build/package-container-image.sh runtime "gizclaw-runtime:${SOURCE_COMMIT}-${ARCH}" \
  .tmp/container-assets .tmp/container-metadata "v$VERSION" "$SOURCE_COMMIT" "$ARCH"
build/check-container-archive.sh ".tmp/container-metadata/container-runtime-${ARCH}.json" \
  "v$VERSION" "$SOURCE_COMMIT" .tmp/container-assets
```

The gate verifies archive bytes and image ID before loading the delivered package.
Runtime acceptance uses temporary config and an isolated volume without provider
credentials; containers and the volume are removed on exit. It retains CLI,
runtime libraries, CA, non-root execution, mounts, health, persistence, identity
across restart, exclusive workspace ownership, graceful stop and forced-stop
recovery. Before publication, two fresh native runners load and repeat acceptance.
