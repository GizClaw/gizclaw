# Gameplay Assets

`实现文件：rpc_gameplay_pixa.go`

处理 Pet 和 BadgeDef 的 pixa asset download streaming RPC，并提供共享的 metadata 加 binary frame 下载流程。

Gameplay asset 选择和权限属于 `services/gameplay`；这里负责 RPC stream 适配。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `rpcGameplayPixaDownloadService` | Pixa download 所需的最小 Gameplay interface。 |
| `handlePetPixaDownload` | 下载指定 Peer pet 的 pixa。 |
| `handleBadgeDefPixaDownload` | 下载 BadgeDef pixa。 |
| `writeRPCDownload` | 统一写入 typed metadata、binary frames 和 EOS。 |
