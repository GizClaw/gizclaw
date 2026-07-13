# Reference

这里汇总 GizClaw 的 API、Schema 与 SDK Reference。VitePress 不承载 Flutter Dartdoc 和 TypeScript TypeDoc；这两套独立文档从 SDK source 按需生成，并使用各自的本地静态服务器查看。

## SDK Reference

- [Go SDK Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli)

## API Reference

- [GizClaw API Reference](./api/)

## 本地生成

先安装 Guide 的 Node dependencies：

```sh
npm ci --prefix guides
```

生成 Flutter Dartdoc 与 TypeScript TypeDoc：

```sh
npm --prefix guides run references
```

也可以只生成其中一套：

```sh
npm --prefix guides run reference:flutter
npm --prefix guides run reference:typescript
```

生成结果分别位于 `guides/references/flutter/` 和 `guides/references/typescript/`，这两个目录是本地生成产物，不提交到 Git。

## 本地查看

生成后，分别启动 Flutter 和 TypeScript Reference 的静态服务器：

```sh
npm --prefix guides run reference:flutter:serve
npm --prefix guides run reference:typescript:serve
```

两个命令需要在不同终端运行，然后访问：

- Flutter：`http://127.0.0.1:4174/`
- TypeScript：`http://127.0.0.1:4175/`

VitePress Project Guide 独立启动：

```sh
npm --prefix guides run dev
```

构建 VitePress Project Guide：

```sh
npm --prefix guides run build
```

正式发布后，Flutter 与 TypeScript Reference 应使用各自文档服务器的稳定地址；VitePress 只保留生成与查看说明，不复制或代理它们的静态文件。
