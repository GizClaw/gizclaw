# Reference

这里汇总 GizClaw 的 Schema、RPC 与 SDK Reference。HTTP/OpenAPI 使用独立的 API Portal；VitePress 不生成或承载 Flutter Dartdoc 和 TypeScript TypeDoc。

## API Reference

- [Admin API](/api/)：使用 Scalar 浏览 Admin、Peer HTTP、Desktop Pod 与 OpenAI-compatible API。
- [RPC API Reference](./rpc)：全部 RPC method ID、method name 与用途。

## SDK Reference

- [Go SDK Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli)

## Flutter SDK 本地查看

在 Flutter SDK 目录生成临时 Dartdoc：

```sh
cd sdk/flutter/gizclaw
flutter pub get
dart doc
```

从同一目录启动静态服务器：

```sh
python3 -m http.server 4174 --directory doc/api
```

访问 `http://127.0.0.1:4174/`。`doc/api/` 是本地临时输出，不提交到 Git。

## TypeScript SDK 本地查看

先安装 Guide 的 Node dependencies，再从仓库根目录运行 TypeDoc：

```sh
npm ci --prefix guides
npm --prefix guides exec -- typedoc --options typedoc.json
```

启动独立静态服务器：

```sh
python3 -m http.server 4175 --directory guides/references/typescript
```

访问 `http://127.0.0.1:4175/`。生成目录是本地临时输出，不提交到 Git，也不进入 VitePress build。
