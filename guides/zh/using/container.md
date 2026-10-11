# 容器镜像

正式 Release 为 Runtime 和 Mem0 各提供 `linux/amd64`、`linux/arm64` 的 Docker save
gzip 镜像包。Runtime 包含 GizClaw 程序及运行库、系统 CA 和 curl。
Mem0、LiveKit、数据库独立运行；生产可以连接托管 PostgreSQL。

## 选择与校验 Release

下载所选 Release 的文件，按[仓库发布](../developing/tooling#仓库发布)校验
`SHA256SUMS`、schema 8 `release-manifest.json` 和 source commit。然后按主机架构
选择 schema 2 `container-image.json` 中的归档，校验并加载：

```sh
arch=amd64 # arm64 主机设为 arm64
entry="$(jq -ce --arg arch "$arch" '.images[] | select(.component == "runtime" and .architecture == $arch)' container-image.json)"
archive="$(jq -er .name <<<"$entry")"
printf '%s  %s\n' "$(jq -er .sha256 <<<"$entry")" "$archive" | sha256sum --check
docker load --input "$archive"
reference="$(jq -er .image_reference <<<"$entry")"
export GIZCLAW_IMAGE="$(docker image inspect --format '{{.Id}}' "$reference")"
jq -e --arg id "$GIZCLAW_IMAGE" '$id == .config_digest or $id == .archive_manifest_digest or $id == .archive_index_digest' <<<"$entry"
docker image inspect "$GIZCLAW_IMAGE" | jq -e --arg arch "$arch" \
  --arg version "$(jq -er .version <<<"$entry")" --arg commit "$(jq -er .source_commit <<<"$entry")" '
  .[0] | .Os == "linux" and .Architecture == $arch and
  .Config.Labels["org.opencontainers.image.version"] == $version and
  .Config.Labels["org.opencontainers.image.revision"] == $commit'
docker run --rm --pull never "$GIZCLAW_IMAGE" --version
docker run --rm --pull never "$GIZCLAW_IMAGE" --help
```

加载后的 tag 为 `gizclaw-runtime:<tag>-<arch>`；运行和 Compose 固定经验证的本地
image ID。归档 SHA256 与 image ID 是不同身份，不作为 registry digest 使用。
Mem0 选择 `.component == "mem0"` 并采用相同校验和加载步骤，配置与运行见
[自托管 Mem0](../developing/stores/memory#自托管-mem0-服务)。完整文件命名、清单字段
及下游消费契约见[容器镜像归档](../developing/tooling#容器镜像归档)。

## Workspace 与配置

ENTRYPOINT 为 `/usr/local/bin/gizclaw-entrypoint`，最终执行 `/usr/bin/gizclaw`，默认参数为
`serve --force /var/lib/gizclaw`，显式允许在容器内前台运行；入口先取得 workspace 文件锁，再清理 stale
`serve.pid` 并 exec Server，进程直接处理 SIGTERM。
运行账户为 UID/GID `10001:10001`，bind mount 的可写目录必须归该账户所有。
新建 named volume 会继承镜像 workspace 的权限。

完整 `config.yaml` 挂载到 `/var/lib/gizclaw/config.yaml`，数据持久化在
`/var/lib/gizclaw`。Storage、证书与 credential 文件的相对路径从 workspace 解析。
以 [Server 完整配置](../developing/gizclaw/server/main#storage、store-与-service-组合)
为基础准备部署配置。只读挂载前显式提供 `identity.private-key`；省略时 Server 会生成
identity 并尝试回写 config。重启保留 identity 和数据；每个 workspace 只能由一个
Server 使用，副本必须有独立 workspace。

将 `webrtc.listen` 与第一个 HTTP listener 设为 `0.0.0.0:9820`，
`webrtc.endpoint` 设为可达的 Server endpoint。TCP/UDP 9820 承载 HTTP、signaling 与
ICE；额外配置的 ICE 端口需对应映射。业务 HTTP 经 Edge 路由，详见 Server 文档。
通过环境变量展开提供 PostgreSQL DSN，在 `services.sfu` 中连接独立 LiveKit 并挂载
credential 文件；外部 Mem0 的 MemoryLayout 通过 Admin resources 配置。
镜像不负责创建这些服务。

## Compose

将准备好的配置放在 `./gizclaw/config.yaml`，导出 YAML 引用的环境变量，包括
identity、admin public key 和 PostgreSQL DSN。下例采用上述默认 listener：

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

按部署拓扑补齐其他服务、network 和只读 credential 文件挂载；secret 不进入镜像。
先运行 `docker compose config` 校验展开，再运行 `docker compose up -d gizclaw`。
使用 `docker compose ps` 和 `/server-info` 确认健康状态、版本与 build commit。
此健康检查证明 Server HTTP readiness，provider 与 SFU 另有验收；修改 listener 端口
或 TLS 时同步调整检查。

停止使用 `docker compose stop gizclaw` 或 `docker stop --time 30 <container>`。
正常停止会删除 `serve.pid` 并关闭 stores。保留数据 volume，需保留状态时不要运行
`docker compose down -v`。默认命令支持强制停止后的自动恢复；并发使用同一 workspace 的容器会在修改 PID 文件前
被拒绝。不要将该 workspace 与主机 Server 共享。自定义 CLI 参数直接执行，
不使用这个默认 workspace 锁。

## 本地验证

Linux 原生 CI 为两个架构从源码构建，执行与 Release 相同的归档与运行 gate。
对已校验 package 复现：

```sh
mkdir -p .tmp/container-assets
cp "$PACKAGE" ".tmp/container-assets/gizclaw_${VERSION}_${ARCH}.deb"
build/build-runtime-image.sh "$PACKAGE" "$VERSION" "$SOURCE_COMMIT" "$SOURCE_EPOCH" "$ARCH"
build/package-container-image.sh runtime "gizclaw-runtime:${SOURCE_COMMIT}-${ARCH}" \
  .tmp/container-assets .tmp/container-metadata "v$VERSION" "$SOURCE_COMMIT" "$ARCH"
build/check-container-archive.sh ".tmp/container-metadata/container-runtime-${ARCH}.json" \
  "v$VERSION" "$SOURCE_COMMIT" .tmp/container-assets
```

Gate 校验压缩文件与 image ID，再 `docker load` 实际交付包。运行验证使用临时配置与
独立 volume，无需 provider credential，退出时清理容器与 volume。保留 CLI、运行库、
CA、非 root、配置与数据挂载、健康检查、数据与 identity 跨重启保留、并发 workspace
拒绝、优雅停止与强制停止恢复。正式发布前还在两个新的原生 runner 上加载并重复验收。
