# Issue 审查

Issue 审查用于确认问题已经描述到可以由开发者独立实施和验证的程度。它关注产品或架构边界，不提前把实现锁死成未经验证的代码方案。

## 问题定义

- 标题准确描述要解决的一个问题或可交付目标。
- 背景说明当前行为、期望行为以及为什么需要改变。
- 关键术语、角色和 provider/consumer 方向与代码及现有 Contract 一致。
- 不把推测、未来 roadmap 或相邻问题写成当前事实。

## Scope

- 明确本 Issue 拥有什么结果，以及明确不负责什么。
- 涉及多个独立交付物时，使用 tracking Issue 与 sub-issue 分离；父 Issue 保留产品边界，具体实现由子 Issue 承担。
- 与其他 Issue、PR 或 Contract 有依赖时，写清依赖方向和先后关系。
- 不为了“完整”引入仓库尚未建立的角色、配置字段、taxonomy 或 abstraction。

## Acceptance criteria

每条 acceptance criterion 应描述可观察、可验证的完成状态，例如：

- 哪个调用方可以完成什么行为；
- 哪些错误、取消或兼容路径必须成立；
- 哪些 Schema、生成 surface 或文档必须同步；
- 使用什么测试或命令证明完成。

避免使用“完成重构”“优化代码”“支持相关功能”等无法独立判定的表述。

## 实施边界

- 指出主要 ownership 目录、source contract 和受影响 consumer。
- 如果实现方式已由架构确定，说明必须遵守的边界；否则只描述约束，不在 Issue 中伪造详细代码。
- breaking change、migration、generated code 和跨语言影响必须显式列出。
- 安全、资源生命周期、持久化和不可信输入存在风险时，应进入验收范围。

## 验证计划

Issue 应说明与风险匹配的验证：

- unit、integration、E2E、fuzz、race 或 smoke test 的适用边界；
- Schema regeneration 与所有语言 consumer 的检查；
- 文档、配置或 workflow 的静态验证；
- 当前环境无法执行的验证由谁、在哪里完成。

## 审查结论

Issue 只有在问题、scope、acceptance criteria、ownership 和验证方式都足够明确时才算 ready。审查后应直接修正文档中的歧义和错误；如果仍缺少会改变实现方向的决定，则保留未解决问题，不应假装已经可实施。
