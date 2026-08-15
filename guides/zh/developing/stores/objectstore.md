# pkgs/store/objectstore

`pkgs/store/objectstore` 定义 provider-neutral、prefix-addressable 的 binary object
storage。Object name 是相对的 slash-separated key；调用方可以流式读写单个 object、
列举或删除 prefix，并设置 deadline 或 TTL。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/objectstore)

## 契约与 ownership

| 符号 | 作用 |
| --- | --- |
| `ObjectStore` | Get、流式覆盖、expiration、幂等 Delete、DeletePrefix 与 List |
| `ObjectInfo` | Object name、size 与 deadline |
| `Root` / `NewRoot` | 借用 rooted local filesystem handle |
| `NewVolcTOS`、`NewAliyunOSS`、`NewGCS`、`NewAzureBlob` | 借用由物理 Storage 持有的 official cloud SDK client |
| `LocalDirProvider` | 只识别 local filesystem-backed store |

Name 必须是相对且规范化的 slash key。空 object name、absolute path、parent
traversal 和保留的 `.objectstore-meta` namespace 会被拒绝。Put 精确替换一个
object，并只在 backend 确认后返回。Get 缺失或过期 object 的错误匹配
`fs.ErrNotExist`；Delete 幂等。List 消费 provider 全部分页，以完整 path segment
限定 prefix，并按 name 确定性字典序返回。

Cloud object 的 expiration 使用 GizClaw-owned metadata `gizclaw-deadline`；zero
deadline 会移除该 metadata。Get 与 List 隐藏过期 object，并尽力删除。Filesystem
backend 使用语义相同的私有 sidecar metadata。调用方不能依赖 provider lifecycle
rule 来实现此契约。

Resource content type、authorization、命名策略和版本规则仍属于调用领域。
ObjectStore 不创建 bucket/container、不提供 public URL，也不向 service 暴露
provider SDK type。

## Server 组合

`storage` 打开并持有物理 root 或 official SDK client，执行 30 秒有界的只读
readiness probe，并在逻辑 Stores 之后关闭 transport。逻辑 `objectstore` 借用它，
且只应用一次配置 prefix。多个不重叠的 logical prefix 可以共享 connector；相同或
父子重叠会在 listener 打开前被 registry 拒绝。

Startup probe 只确认配置身份能够找到并读取既有 bucket/container，不能证明 Put/Delete
authorization。相关权限失败会作为经过 redaction 的 runtime ObjectStore error 返回；完整
permission set 必须通过 credential-backed conformance suite 验证。

```yaml
storage:
  profile-files:
    kind: volc-tos
    endpoint: https://tos-cn-beijing.volces.com
    region: cn-beijing
    bucket: example-profiles
    access_key_id: ${VOLC_TOS_ACCESS_KEY_ID}
    access_key_secret: ${VOLC_TOS_ACCESS_KEY_SECRET}
    session_token: ${VOLC_TOS_SESSION_TOKEN}
stores:
  runtime-profiles:
    kind: objectstore
    storage: profile-files
    prefix: pprof
```

支持的物理配置如下：

| Kind | 必填字段 | 认证 |
| --- | --- | --- |
| `filesystem.dir` | `dir` | 进程 filesystem permission |
| `volc-tos` | `endpoint`、`region`、`bucket`、`access_key_id`、`access_key_secret` | 可选 `session_token` |
| `aliyun-oss` | `endpoint`、`bucket`、`access_key_id`、`access_key_secret` | 可选 `security_token` |
| `gcs` | `bucket` | ADC，或可选 `credentials_file` |
| `azure-blob` | `account_url`、`container` | `DefaultAzureCredential`（managed/workload identity、environment 或 developer credential） |

Bucket/container 必须已经存在。Production endpoint 与 account URL 必须使用
HTTPS。`${VAR}` 展开与 GCS credentials file 的 workspace 相对路径解析由
`cmd/internal/server` 完成，credential 内容不会进入日志。配置的 GCS credential
路径必须是可读 regular file。Azure credential 不进入 YAML。

每个 cloud operation 固定为 30 秒上限，并使用有界 SDK retry。Alibaba OSS 的
List response 不包含 custom metadata，所以 adapter 会为每个返回 object 再请求
metadata，这会产生请求量和计费成本；其他 adapter 使用各自 list API 返回的 metadata。
DeletePrefix 同样会读取全部页面并逐项删除，operator 应使用专用 prefix，并计入
provider 请求成本。

正常 error 文本不会暴露 credential、Authorization、signed query parameter 或 raw
response body。Production 不应记录 wrapped provider error，也不应启用 SDK wire log。
Cleanup 或 retention 失败属于未完成操作，应由拥有该 feature 的 owner 重试。

## 主要用途

Workspace 与 Gameplay assets、Agent Host runtime data、HNSW persistence 和 Server
process profile 使用 ObjectStore。Firmware OTA package 仍是 external HTTPS resource，
不存放在这里。
