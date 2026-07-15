# PR Agent 审查

PR Agent 审查面向远程 Codex reviewer。Reviewer 只提交一次完整、可执行的审查结果，不直接修改 PR 代码。

## 输入

开始前读取：

- PR 的 base/head 与完整 diff；
- 关联 Issue、设计文档和 acceptance criteria；
- 仓库及相关目录的 `AGENTS.md`；
- 改动语言对应的[编码规范](../coding-styles/)；
- 作者声明的测试和验证结果。

PR title 和 body 不属于默认检查范围，除非当前任务明确要求审查 PR metadata。

## 审查流程

### 1. 按 ownership 分组

先列出全部 changed folder、package、Schema、生成 surface 和语言边界。逐模块使用对应规则审查，不因某个主要模块通过而跳过较小目录。

### 2. 先需求，后实现

先判断 PR 是否完成正确的问题、是否超出 scope，再检查逻辑、错误、生命周期和代码结构。需求不成立时，不应继续把实现细节的 polish 当作主要反馈。

### 3. 检查跨模块 Contract

当改动跨 Schema、Go、JavaScript、Dart/Flutter 或 C 时，把 source contract、生成文件、调用方和测试视为一个整体。任何一侧缺失都可能是 blocking finding。

### 4. 核对验证

验证命令必须能证明改动 surface。不能因为无关 Go test 通过，就认为 C、Flutter、生成 SDK 或前端行为已经验证。

### 5. 审查审查结果

提交前再次对照完整 diff：

- 所有 changed folder 是否已覆盖；
- findings 是否重复；
- 每项是否确实会阻塞健康完成；
- severity、文件、行号、风险和修复方向是否完整；
- 是否遗漏生成代码和跨语言 consumer。

## 通过条件

没有 blocking finding 时明确输出通过，并说明：

- 审查覆盖的模块；
- 已确认的 validation；
- 无法独立运行或确认的验证及剩余风险。

Reviewer 不应因“没有时间继续看”而给出通过结论。
