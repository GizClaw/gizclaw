# OpenAI HTTP

`实现文件：server_openai_http.go`

为普通 Server HTTP 入口组装 Peer-scoped OpenAI-compatible handler，并接入 public login session 与对应 Peer resource view。

OpenAI API 的领域实现属于 AI service；该文件只负责 Server-level HTTP composition。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `peerOpenAIHTTPHandler` | 根据 HTTP session 中的 Peer identity 组装 OpenAI-compatible handler。 |
