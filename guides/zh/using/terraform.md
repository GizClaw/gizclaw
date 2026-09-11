# Terraform Provider

`terraform-provider-gizclaw` 用 Terraform 声明式管理 GizClaw Admin Resource。Provider
直接使用本地 GizClaw CLI context 中的 identity 连接 Server 的 Admin 服务，不调用本地
`gizclaw` 可执行文件。它是纯 Go、`CGO_ENABLED=0` 构建的二进制，随每个正式 Release
发布 macOS 与 Linux 版本。

| 项目 | 值 |
| --- | --- |
| Provider source address | `gizclaw.local/gizclaw/gizclaw` |
| Provider 版本 | 与 GizClaw Release 版本相同，例如 `v0.17.6` 对应 `0.17.6` |
| 资源类型 | `gizclaw_resource` |
| 数据源 | `gizclaw_catalog` |
| 支持平台 | `darwin_amd64`、`darwin_arm64`、`linux_amd64`、`linux_arm64` |

`gizclaw.local` 不是公开 registry。Terraform 只能从 filesystem mirror 或
`provider_installation` 中配置的本地目录安装该 provider。

## 安装

每个 Release 包含四个 provider 包：

```text
terraform-provider-gizclaw_<version>_darwin_amd64.zip
terraform-provider-gizclaw_<version>_darwin_arm64.zip
terraform-provider-gizclaw_<version>_linux_amd64.zip
terraform-provider-gizclaw_<version>_linux_arm64.zip
```

每个 zip 只包含一个可执行文件 `terraform-provider-gizclaw_v<version>`。这些文件同时列在
`SHA256SUMS` 与 `release-manifest.json` 中。下载后先校验 checksum，再把 zip 原样放入
packed filesystem mirror：

```sh
version=0.17.6
mirror="$HOME/.terraform.d/gizclaw-mirror"
dir="$mirror/gizclaw.local/gizclaw/gizclaw"
mkdir -p "$dir"
gh release download "v$version" --repo GizClaw/gizclaw --dir "$dir" \
  --pattern "terraform-provider-gizclaw_${version}_*.zip" --pattern SHA256SUMS
(cd "$dir" && sha256sum --ignore-missing --check SHA256SUMS && rm SHA256SUMS)
```

macOS 没有 `sha256sum` 时使用 `shasum -a 256 --ignore-missing --check SHA256SUMS`。

在 Terraform CLI 配置（`~/.terraformrc`，或 `TF_CLI_CONFIG_FILE` 指向的文件）中让
`gizclaw.local` 只从该目录安装：

```hcl
provider_installation {
  filesystem_mirror {
    path    = "/Users/me/.terraform.d/gizclaw-mirror"
    include = ["gizclaw.local/*/*"]
  }
  direct {
    exclude = ["gizclaw.local/*/*"]
  }
}
```

`path` 必须是绝对路径。Terraform root 固定 provider 版本：

```hcl
terraform {
  required_providers {
    gizclaw = {
      source  = "gizclaw.local/gizclaw/gizclaw"
      version = "0.17.6"
    }
  }
}
```

`terraform init` 会把 zip 的 hash 写入 `.terraform.lock.hcl`。需要在多个平台使用同一个
lock 文件时，把四个平台的 zip 都放入 mirror，再运行
`terraform providers lock -fs-mirror=<mirror> -platform=darwin_arm64 -platform=linux_amd64 ...`。

从源码构建与 Release 相同的包：

```sh
build/build-terraform-provider.sh \
  --version 0.17.6 \
  --source-commit "$(git rev-parse HEAD)" \
  --source-epoch "$(git show -s --format=%ct HEAD)" \
  --output-dir .tmp/terraform-provider
```

脚本需要 `go`、`zip` 与 `unzip`，只构建 `CGO_ENABLED=0` 的四个平台，不需要 Git LFS、
cgo 工具链或 Console 构建产物。

## Provider 配置

```hcl
provider "gizclaw" {
  context  = "prod-admin"
  endpoint = "https://admin.example.com:443"
}
```

| 参数 | 默认值 | 含义 |
| --- | --- | --- |
| `context` | `GIZCLAW_CONTEXT`，都未设置时使用 CLI 的 current context | 读取 `$XDG_CONFIG_HOME/gizclaw/<context>/config.yaml`（未设置 `XDG_CONFIG_HOME` 时为 `~/.config/gizclaw`）中的 `identity.private-key` 与 `server.endpoint`。 |
| `endpoint` | `GIZCLAW_ENDPOINT`，都未设置时使用 context 中的 `server.endpoint` | 替换 context 的 Server endpoint，只在本次 provider 进程中生效，不修改 `config.yaml`。必须是 `https://host[:port]`，不能包含 userinfo、path、query 或 fragment。 |

Context 使用 [CLI](./cli) 的 `gizclaw context create` 创建；该 identity 必须是目标 Server
的 Admin identity。Provider 在 configure 阶段加载 context 并校验 endpoint，context 不存在、
identity 无效或 endpoint 不合法时直接报错。`context` 或 `endpoint` 在 plan 时仍为 unknown
也会报错。

同一个 identity 的第二条 WebRTC 连接会在 Server 上替换第一条连接。因此每个 provider
进程只为同一组 `context`/`endpoint` 建立一条长连接，首次需要访问 Server 时才连接，所有
资源操作都复用它，同时最多 8 个请求。连接或传输失败时丢弃该连接并重连重试，每个操作最多
3 次；Admin API 返回的结构化错误不重试。Terraform 取消操作时，正在进行的连接建立、重连等待与请求都会停止。不要让同一个 context 同时被多个 Terraform 或 CLI
进程使用。

## `gizclaw_resource`

```hcl
resource "gizclaw_resource" "openai" {
  kind        = "Credential"
  resource_id = "openai-main"
  spec = jsonencode({
    provider = "openai"
    body     = { api_key = "$${OPENAI_API_KEY}" }
  })
}
```

| 属性 | 类型 | 说明 |
| --- | --- | --- |
| `id` | computed | `<kind>/<resource_id>`。 |
| `api_version` | optional, computed | 只接受 `gizclaw.admin/v1alpha1`；省略时保留已存储的值。修改会替换资源。 |
| `kind` | required | 可按 ID 寻址的 Resource kind，例如 `Credential`、`Model`；不接受 `ResourceList` 或 `<Kind>Resource` 别名。修改会替换资源。 |
| `resource_id` | required | Resource `metadata.id`，遵守 Admin API 的 caller-defined ID 规则（非空、无首尾空白、不超过 1024 字符、不是 `.` 或 `..`）。修改会替换资源。 |
| `spec` | required, sensitive | `jsonencode(...)` 得到的 JSON object。 |
| `input_revision` | optional, sensitive | 64 位小写 SHA-256。修改它会在 `spec` 不变时重新 apply，适合密钥等只写输入发生变化的场景。 |

资源语义：

- Create 与 Update 按 `gizclaw admin apply --file <file>.json` 的规则准备 manifest，然后 apply
  并重新读取。`spec` 中的 `${NAME}` 与 `${NAME:-default}` 在 provider 进程中用环境变量展开；
  未设置且没有默认值的变量会让 apply 失败。HCL 中写 `$${NAME}` 才能得到字面量 `${NAME}`。
- Read 读取 Server 上的资源。同一时间窗口内的多个 refresh 合并为一个批次，在共享连接上并发读取。
  Server 返回 `<SCOPE>_NOT_FOUND` 错误码时资源从 state 中移除；其他错误保留 state 并报错。
- 刷新时，Server 返回的 `spec` 与配置语义一致就保留配置中的写法；不一致时记录 Server 的值，
  下一次 plan 显示差异。`Credential` 的 `spec` 永远保留配置值，因为 Server 不返回密钥。环境变量
  占位符（包括 `${NAME:-default}`）按 apply 时的规则展开后与 Server 返回值相同即视为一致。
- 除上述规则外，刷新按字面值比较。Server 会补默认值或规范化的字段需要写成规范化后的值：
  Tool 需要显式设置 `enabled`、`http.headers` 与 `http.success_status_codes`，否则每次 plan 都显示变化。
- Delete 调用 Admin delete；资源已不存在时视为成功。
- Import 使用 `<kind>/<resource_id>`：`terraform import gizclaw_resource.openai Credential/openai-main`。
  Import 不会写入 `spec`，下一次 apply 会用配置中的 `spec` 覆盖 Server 上的值。

State schema 版本为 1。版本 0 的 state 使用 `name` 表示资源 ID，provider 在读取时自动迁移为
`resource_id`。

`gizclaw_resource` 的 Terraform state 保存 `spec`，其中可能包含展开前的 secret 占位符或明文
secret。State backend 需要按 secret 存储管理。

## `gizclaw_catalog`

`gizclaw_catalog` 把分层的本地 manifest 目录解析为产品定义选中的 Admin Resource。它只读取本地
文件，不发送 Admin 请求，也不建立 Server 连接；provider 块在配置阶段仍会加载 context。

```hcl
data "gizclaw_catalog" "selected" {
  sources         = ["${path.root}/catalogs/upstream", "${path.root}/catalogs/overrides"]
  product_sources = ["${path.root}/products/default"]
}

resource "gizclaw_resource" "workflows" {
  for_each    = data.gizclaw_catalog.selected.workflows
  kind        = jsondecode(each.value).kind
  resource_id = jsondecode(each.value).metadata.id
  spec        = jsonencode(jsondecode(each.value).spec)
}
```

| 属性 | 类型 | 说明 |
| --- | --- | --- |
| `sources` | required | 可复用的 catalog 目录，按优先级从低到高排列。 |
| `product_sources` | required | 包含 RuntimeProfile 与 RegistrationToken manifest 的产品目录。 |
| `credentials`、`tenants`、`voices`、`models`、`memory_layouts`、`workflows`、`firmwares`、`runtime_profiles`、`registration_tokens` | computed | 以 `<Kind>/<id>` 为 key、以选中 manifest 的 JSON 编码为值的 map。 |
| `raids` | computed | 以 raid ID 为 key、以选中 `raid.json` 原始字节为值的 map。 |
| `overridden_ids` | computed | 在多个 `sources` 中出现的 `<Kind>/<id>` 集合。 |

catalog source 可以包含 `credentials/`、`tenants/`、`voices/`、`models/`、`memory-layouts/`、
`workflows/`、`firmwares/` 与 `runtime-profiles/`；product source 只能包含 `runtime-profiles/` 与
`registration-tokens/`，且 manifest 的 kind 必须与目录一致。这些目录下的所有 `.yaml` 与 `.yml`
文件都会被读取。每个 manifest 必须包含 `apiVersion`、`kind`、`metadata.id` 与 `spec`；
`metadata.id` 遵守 `gizclaw_resource` 的 caller-defined ID 规则，`metadata.name` 会被拒绝。

解析规则：

- 后面 source 中的 `<Kind>/<id>` 替换前面 source 中的同名资源，并列入 `overridden_ids`。同一个
  source 重复定义同一 `<Kind>/<id>`，或 product source 定义已存在的 `<Kind>/<id>`，都会让读取失败。
- 选择从所有产品 manifest 开始。RegistrationToken 选中 `spec.runtime_profile_id` 指向的
  RuntimeProfile（可以来自 catalog source），设置了 `spec.firmware_id` 时再选中对应 Firmware。
  RuntimeProfile 选中 `spec.workflows.collections` 中绑定的 Workflow、`spec.resources.models` 与
  `spec.resources.voices` 中绑定的 Model 与 Voice，以及 `spec.resources.memories.*.layout_id`
  指向的 MemoryLayout。Workflow 选中 `spec.memory` 指向的 MemoryLayout，Model 或 Voice 选中
  `spec.provider.id` 指向的 Tenant，Tenant 选中 `spec.credential_id` 指向的 Credential。
- catalog source 的 `workflows/` 下的 `raid.json`，在其任一 `implementations.*.workflow_id`
  Workflow 被选中时被选中，同时选中它的 `tester.workflow_id` Workflow。同一 raid ID 以后面的
  source 为准。
- 引用缺失或无效会让读取失败；选中的 RuntimeProfile 含有 `spec.gameplay`、
  `spec.workflows.system` 或 `spec.resources.pet_defs`、`game_defs`、`badge_defs`，或选中的
  Workflow 的 `spec.driver` 为 `pet`，也会失败。

每个 manifest 值都是紧凑 JSON，字段为 `apiVersion`、`kind`、`metadata.id` 与 `spec`，`spec`
中的 object key 按字典序排列。`${NAME}` 占位符按原样保留，由 `gizclaw_resource` 在 apply 时展开。
YAML 按 YAML 1.1 标量规则解码，未加引号的 `yes`、`on` 会变成 `true`。这些值不是 sensitive，
secret 应写成占位符，而不是写进 catalog 文件。
