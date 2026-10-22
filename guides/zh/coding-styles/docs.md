# 文档

本规范适用于 Guides、README、配置说明、架构说明、示例和 workflow 文档。

## 内容边界

- 文档描述最终形态，不记录不影响使用和兼容性的迁移过程。
- 已实现行为、未来工作、开放问题和 non-goal 必须分开；不能把 roadmap 写成当前能力。
- 开发指引解释项目组成、目录职责、模块边界和请求路径；Go symbol 的签名与注释由 Go doc 或 Reference 负责。
- 用户说明、内部设计、API Reference 和 issue acceptance criteria 面向不同读者，不应混写。
- 临时问题和重构进度可记录在 PR 或 Issue 中，稳定文档不记录这些过程或临时 Issue 编号。

## 事实来源

- 代码、Schema、脚本和配置是已实现行为的唯一事实来源（SSoT）。文档只解释用途、模块边界、调用关系和使用方法，不定义第二套行为契约。
- 不手工维护函数清单、类型定义、字段表、枚举数值、默认值、校验阈值、路由注册表或 SQL 表结构；引用对应源码、Go doc 或由源码生成的 Reference。使用示例只展示完成操作所需的部分，不复制完整定义。
- 文档与实现不一致时，按当前源码修正文档；待实现需求留在 PR 或 Issue 中。
- 描述实现前先核对当前代码、Schema、生成文件、script 和配置。
- endpoint、method、package、directory、config key、environment variable、port 和命令必须与仓库一致。
- 术语使用代码和 contract 中已有的正式名称，不为解释方便创造新的角色或 abstraction。
- 图、表格和目录树也属于事实陈述，必须与文字及源码保持一致。
- 示例不得包含 secret、credential、token、私人路径或机器特有状态。

## 组织方式

- 根目录 `README.md` 是仓库唯一的 first-party README。模块、SDK、App、example、
  test harness 和 tool 的稳定说明必须进入 `guides/` 对应页面，不能在子目录维护第二份
  README。Third-party submodule 内由 upstream 拥有的 README 不属于此限制。
- 有子页面的目录入口只负责导航，具体说明放在“总览”页面。
- 模块文档按能力和职责组织，用源码或 API Reference 链接定位实现，不逐文件、逐函数复述代码。
- 独立 Go package 在页面顶部提供 Go API Reference 或源码链接；同一 package 内的模块链接对应实现文件。
- 其他页面通过链接引用所属模块的说明，不复制完整段落。
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
