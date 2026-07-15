# 文档

本规范适用于 Guides、README、配置说明、架构说明、示例和 workflow 文档。

## 内容边界

- 文档描述最终形态，不记录不影响使用和兼容性的迁移过程。
- 已实现行为、未来工作、开放问题和 non-goal 必须分开；不能把 roadmap 写成当前能力。
- 开发指引解释项目组成、目录职责、模块边界和请求路径；Go symbol 的签名与注释由 Go doc 或 Reference 负责。
- 用户说明、内部设计、API Reference 和 issue acceptance criteria 面向不同读者，不应混写。
- 当前问题只记录在 GitHub Issues 中，稳定文档不要引用临时 issue 编号或重构状态。

## 事实来源

- 描述实现前先核对当前代码、Schema、生成文件、script 和配置。
- endpoint、method、package、directory、config key、environment variable、port 和命令必须与仓库一致。
- 术语使用代码和 contract 中已有的正式名称，不为解释方便创造新的角色或 abstraction。
- 图、表格和目录树也属于事实陈述，必须与文字及源码保持一致。
- 示例不得包含 secret、credential、token、私人路径或机器特有状态。

## 组织方式

- 有子页面的目录入口只负责导航，具体说明放在“总览”页面。
- 模块文档按能力和职责组织，不机械照抄文件名；需要定位实现时可列出对应文件、主要结构和函数。
- 独立 Go package 在页面顶部提供一个 Go API Reference 链接；同一 package 内的模块直接把公开 symbol 链接放进“核心结构与主函数”。
- 避免在多个页面复制完整 contract。每项事实应有明确 owner，其他页面使用链接和必要摘要。
- Mermaid 图只在关系、调用顺序或状态变化比文字更清楚时使用；实体数量和方向应保持可读。

## 命令与安全

- 可执行命令必须能在注明的工作目录和环境中运行。
- 涉及服务、credential、firmware、listener 或持久化状态时，说明安全默认值、失败方式和清理方法。
- 不提供没有范围限制的删除命令；不可逆操作必须说明影响范围。

## 验证

文档改动至少运行：

```sh
git diff --check
```

VitePress 内容还应执行 `guides/` 中定义的构建命令，并检查新增路由、站内链接、表格和 Mermaid。文档中的命令或 live contract 无法验证时，应明确说明未验证部分及剩余风险。
