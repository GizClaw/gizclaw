# Container Image

Formal Releases provide `ghcr.io/gizclaw/gizclaw:vMAJOR.MINOR.PATCH` for
`linux/amd64` and `linux/arm64`. The image contains the GizClaw executable and
its runtime libraries, CA certificates and curl. Run Mem0, LiveKit and database
services separately; production may use managed PostgreSQL.

## Select and verify a Release

Download the selected Release's files, verify `SHA256SUMS` and
`release-manifest.json` using [Repository Releases](../developing/tooling#repository-releases),
and read the immutable image reference:

```sh
export GIZCLAW_IMAGE="$(jq -er .reference container-image.json)"
docker pull "$GIZCLAW_IMAGE"
docker run --rm "$GIZCLAW_IMAGE" --version
docker run --rm "$GIZCLAW_IMAGE" --help
```

Use the index digest in Compose. Docker selects the host architecture. OCI
labels and receipt bind the version to the source commit; receipt also contains
the individual platform and executable digests. The Package is public and needs
no login for pulls. Publishing and first-package visibility are described in the
[GHCR release contract](../developing/tooling#ghcr-runtime-image).

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
    image: ${GIZCLAW_IMAGE:?set the Release index digest reference}
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

Native Linux CI builds each architecture from source and runs the same runtime
gate used by Release. To reproduce it for a verified package:

```sh
build/build-runtime-image.sh "$PACKAGE" "$VERSION" "$SOURCE_COMMIT" "$SOURCE_EPOCH" "$ARCH"
build/check-runtime-image.sh "gizclaw-runtime:${SOURCE_COMMIT}-${ARCH}" \
  "$VERSION" "$SOURCE_COMMIT" "$ARCH" "$BINARY_SHA256"
```

The gate uses temporary config, an isolated volume and no provider credentials;
it removes its containers and volume on exit. It verifies CLI, runtime libraries,
CA, non-root execution, config/data mounts, health, persistence, identity across
restart, graceful stop and forced-stop recovery. Publication repeats these checks after anonymous
pulls by digest on both native runners.
