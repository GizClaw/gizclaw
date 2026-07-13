# pkgs/gizclaw/customid

`pkgs/gizclaw/customid` 保存多个 GizClaw 领域共同使用的自定义资源 ID 规则，包括稳定 ID 和 compound ID 的构造、格式化与解析边界。

## 目录位置

```text
pkgs/gizclaw/customid/
└── GizClaw cross-service resource ID rules
```

这个 package 的价值是让不同 public surface 和 service 对同一类产品资源使用一致 ID，而不是为所有数据库 key 提供统一 helper。

## Ownership 边界

应该放在 `customid`：

- 被多个 GizClaw service 或 API surface 共同识别的资源 ID 格式。
- Compound ID 中各部分的稳定组合与解析规则。
- Public/resource ID 的格式 validation。

不应该放在 `customid`：

- 单一领域内部使用的数据库 row ID。
- KV store prefix、object storage path 或临时 cache key。
- Giznet public key 和 transport identity。
- UUID、hash 或 encoding 等通用基础 library。
- 为了隐藏不稳定资源模型而新增的字符串拼接 helper。

只有当 ID 是跨 package 的 GizClaw product contract 时，才应进入 `customid`；领域私有 ID 应留在其 owner package。
