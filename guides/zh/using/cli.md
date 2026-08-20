# CLI

## 离线校验声明式 Resource

在 apply 之前，可以用 `admin validate` 校验一个声明式 Resource 或一个 `ResourceList`：

```sh
gizclaw admin validate -f resource.yaml
gizclaw admin validate -f resource.json
printf '%s\n' '{"apiVersion":"gizclaw.admin/v1alpha1","kind":"ResourceList","spec":{"items":[]}}' \
  | gizclaw admin validate -f -
```

文件输入支持 `.json`、`.yaml` 和 `.yml`；`-f -` 从 stdin 读取 JSON。该命令与 `admin apply` 复用相同的 `${VAR}`、`${VAR:-default}` 展开规则，也接受相同的生成型 `KindResource` 兼容别名。

具体 Resource 校验成功时以状态码 `0` 退出，并输出一个紧凑 JSON object 和换行：

```json
{"valid":true,"kind":"Credential","id":"openai-main"}
```

有效列表只报告 item 数量，不输出任何 item spec：

```json
{"valid":true,"kind":"ResourceList","items":3}
```

无效输入以非零状态退出，并使用输入标识与脱敏的 JSON Pointer diagnostics 说明问题。命令不会打印 Resource spec value 或展开后的环境变量值，因此可以在 CI 中安全校验 Credential Resource，而不暴露其 body。

校验过程完全离线：不会读取 GizClaw context、连接 Server 或修改 storage。通过校验只表示展开后的 document 符合同一二进制所嵌入的 Resource OpenAPI schema，并能按声明的 kind 解码；它不能证明引用 ID 存在、凭据可认证、provider/body 组合满足 Server 业务规则、依赖服务可达，也不能证明 Resource 可以成功 apply 或运行。

## 运行 Giztest

`test validate` 离线递归校验严格的 `*.giztest.yaml`；`test run` 连接每个文件自己声明的
endpoint。所有文件先整体通过校验，才会创建临时身份或产生远端操作：

```sh
gizclaw test validate -f tests/gizclaw-e2e/giztest
gizclaw test run tests/gizclaw-e2e/giztest --parallel 10 --output report.json
```

YAML 的 `repeat` 是每个文件的任务数，`--parallel` 是所有文件共享的最大 worker 数。
目录输入递归发现文件并稳定排序。每个任务有独立的临时 clients、variables 和 cleanup；
`save_as` 只写入顶部声明的内存 output 变量，不支持 Save As 文件。
重复语音 Benchmark 可在合成步骤声明 `speech.cache: run`，按文档和展开后的请求缓存成功的
输入 fixture；每个 task 得到独立字节副本，缓存受 output `max_bytes` 限制，并在命令退出时释放。

本地 Docker E2E 会先统一 Apply Admin resources。直接测试已部署环境时，预先准备资源并设置
`GIZCLAW_TEST_ENDPOINT`、`GIZCLAW_TEST_REGISTRATION_TOKEN`；命令本身没有 Admin 权限。
人工 `review.*` 场景要求 attached terminal 和 `--parallel 1`。
