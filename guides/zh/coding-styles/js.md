# JavaScript 与 TypeScript

本规范适用于 `sdk/js`、共享 JavaScript package、Wails frontend、生成的 OpenAPI client，以及 Node 或 Playwright test harness。

## 类型与边界

- 优先使用 Schema 生成的类型和 client，不重复拼接 endpoint 或手写已有 request/response shape。
- 外部 JSON、event、SSE、WebRTC 与 RPC payload 必须在边界验证；不要用 `any`、unchecked cast 或类型断言掩盖不确定输入。
- browser、Wails、Node 和 test runtime 对 global、storage、crypto、URL、stream 与 timer 的支持不同，使用前必须明确目标运行时。
- 修改 OpenAPI/RPC contract 后重新生成 TypeScript surface，不直接手改生成文件。

## 异步与生命周期

- Promise 必须被 `await`、返回或显式处理；不得吞掉 rejection。
- 长请求、stream、subscription 和 UI effect 应接受或创建明确的 abort/cancel 路径，并在卸载或结束时清理。
- timeout、retry 和 reconnect 必须有上限，并保留最终失败给调用方。
- stream parser 必须处理分片消息、空消息、畸形 payload、顺序、重连和 terminal state。

## SDK 与前端

- SDK method、cursor、stream shape 和 error payload 必须与 server contract 一致。
- UI flow 应明确 loading、empty、error、success、stale data 和 permission denied 状态。
- 表单在发送请求前校验输入，并保留服务端错误供用户理解。
- 交互组件必须具备键盘可达性、focus 行为、label、disabled state 和稳定的 button 语义。
- 不把 placeholder 或 roadmap 行为呈现成已经交付的功能。

## 依赖、测试与验证

- 新 dependency 必须确有必要；同步提交正确 lockfile，并评估 browser bundle 和 provider coupling。
- parser、转换、SDK helper 和错误处理使用 unit test；跨路由、API 和 UI state 使用 component 或 integration test；依赖真实 browser/Wails 行为时使用 E2E。
- 优先运行 package 自己定义的 script，例如：

```sh
npm --prefix sdk/js test
npm --prefix sdk/js/gizclaw test
```

Schema 变更还应运行对应生成命令并验证生成 diff。
