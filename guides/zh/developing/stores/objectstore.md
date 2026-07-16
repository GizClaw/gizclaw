# pkgs/store/objectstore

`pkgs/store/objectstore` 定义 prefix-addressable binary object storage。Object name 是 slash-separated key；调用方可以读写单个 object、按 prefix 列举或删除，并为 object 设置 deadline 或 TTL。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/objectstore)

## 核心结构与实现

| 符号 | 作用 |
| --- | --- |
| `ObjectStore` | 定义 Get、Put、expiration、Delete、DeletePrefix 与 List。 |
| `ObjectInfo` | 返回 object name、size 和 deadline。 |
| `LocalDirProvider` | 允许调用方识别 local filesystem backend。 |
| `Dir` | 将 object keys 安全映射到指定目录，并维护 expiration metadata。 |

## 主要用途

Firmware artifacts、workspace history、Agent memory binary data、Gameplay pixa 和 HNSW vector index persistence 都使用 Object Store。

## Ownership 边界

Object Store 把目录视为实现细节，不提供任意 filesystem 操作。资源 metadata、content type、authorization 和版本规则属于调用领域；objectstore 只拥有 binary object lifecycle。

Workflow、Workspace、Peer 与 GameplayCatalog 等 owner 可以引用同一 physical ObjectStore，但必须由组装层注入不同的 scoped logical store。共享 physical storage 时，每个 logical store 的 prefix 必须非空、clean 且互不重叠；相同或父子 prefix 会在 Server 启动前被拒绝。

Resource icon 仍由领域 service 管理固定 object name、格式校验、授权和删除顺序。ResourceManager 不接收 ObjectStore，也不存在通用 AssetService、binding 或跨 owner resolver。
