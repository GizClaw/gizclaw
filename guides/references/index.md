# Reference

这里汇总 GizClaw 的 API、Schema 与 SDK Reference。Flutter 与 TypeScript Reference 不提交生成结果；发布前由 SDK source 生成，本地也可以按需生成并查看。

## SDK Reference

- [Go SDK Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli)
- <a href="/references/flutter/index.html">Flutter SDK Reference（生成后可用）</a>
- <a href="/references/typescript/index.html">TypeScript SDK Reference（生成后可用）</a>

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

启动会自动刷新的 VitePress development server：

```sh
npm --prefix guides run dev
```

然后访问：

- `http://127.0.0.1:5173/references/flutter/`
- `http://127.0.0.1:5173/references/typescript/`

生成并构建包含 Reference 的 production site：

```sh
npm --prefix guides run build
```

只构建 Guide、不生成 SDK Reference：

```sh
npm --prefix guides run build:site
```

正式发布后，Reference 应使用文档服务器或 GitHub Pages 的稳定地址；不依赖 npm、pub.dev 是否提供 package 文档页面。
